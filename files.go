package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"unicode/utf8"
)

type Document struct {
	ID       string `json:"id"`
	Path     string `json:"path"`
	Name     string `json:"name"`
	Content  string `json:"content"`
	Revision string `json:"revision"`
}

type OpenResult struct {
	Documents []Document `json:"documents"`
	Errors    []string   `json:"errors"`
}

type SaveResult struct {
	Revision string `json:"revision"`
	Conflict bool   `json:"conflict"`
}

type fileStore struct {
	mu    sync.Mutex
	paths map[string]string
}

func newFileStore() *fileStore {
	return &fileStore{paths: make(map[string]string)}
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func (s *fileStore) open(paths []string) OpenResult {
	result := OpenResult{Documents: []Document{}, Errors: []string{}}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, path := range paths {
		doc, err := readDocument(path)
		if err != nil {
			result.Errors = append(result.Errors, err.Error())
			continue
		}
		s.paths[doc.ID] = doc.Path
		result.Documents = append(result.Documents, doc)
	}
	return result
}

func readDocument(path string) (Document, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return Document{}, fmt.Errorf("文件路径无效 %w", err)
	}
	// Canonical paths make alternate casing and symbolic links select one tab.
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return Document{}, fmt.Errorf("无法打开 %s，%w", filepath.Base(path), err)
	}
	extension := strings.ToLower(filepath.Ext(resolved))
	if extension != ".md" && extension != ".markdown" {
		return Document{}, fmt.Errorf("%s 需要是 .md 或 .markdown 文件", filepath.Base(path))
	}
	data, err := os.ReadFile(resolved)
	if err != nil {
		return Document{}, fmt.Errorf("无法读取 %s，%w", filepath.Base(path), err)
	}
	if !utf8.Valid(data) || bytes.Contains(data, []byte{0}) {
		return Document{}, fmt.Errorf("%s 需要使用 UTF-8 文本编码", filepath.Base(path))
	}
	identity := resolved
	if runtime.GOOS == "windows" {
		identity = strings.ToLower(identity)
	}
	return Document{
		ID: digest([]byte(identity)), Path: resolved, Name: filepath.Base(resolved),
		Content: string(data), Revision: digest(data),
	}, nil
}

func (s *fileStore) reload(id string) (Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	path, ok := s.paths[id]
	if !ok {
		return Document{}, fmt.Errorf("文件尚未打开")
	}
	return readDocument(path)
}

func (s *fileStore) save(id, content, revision string, force bool) (SaveResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	path, ok := s.paths[id]
	if !ok {
		return SaveResult{}, fmt.Errorf("文件尚未打开")
	}
	current, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return SaveResult{}, fmt.Errorf("读取保存目标失败，%w", err)
	}
	if !force && (os.IsNotExist(err) || digest(current) != revision) {
		return SaveResult{Conflict: true}, nil
	}
	mode := os.FileMode(0644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
		if mode&0222 == 0 {
			return SaveResult{}, fmt.Errorf("文件为只读，请检查文件权限")
		}
	}
	data := []byte(content)
	if err := writeAtomic(path, data, mode); err != nil {
		return SaveResult{}, fmt.Errorf("保存失败，%w", err)
	}
	return SaveResult{Revision: digest(data)}, nil
}

func writeAtomic(path string, data []byte, mode os.FileMode) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".markdown-lite-*")
	if err != nil {
		return err
	}
	defer os.Remove(temporary.Name())
	defer temporary.Close()
	if _, err := temporary.Write(data); err != nil {
		return err
	}
	if err := temporary.Chmod(mode); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporary.Name(), path)
}

func (s *fileStore) assetHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/local-image/") {
			next.ServeHTTP(w, r)
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/local-image/")
		s.mu.Lock()
		documentPath, ok := s.paths[id]
		s.mu.Unlock()
		if !ok {
			http.NotFound(w, r)
			return
		}
		imagePath := r.URL.Query().Get("path")
		if strings.HasPrefix(strings.ToLower(imagePath), "file:") {
			parsed, err := url.Parse(imagePath)
			if err != nil {
				http.NotFound(w, r)
				return
			}
			imagePath = parsed.Path
			if runtime.GOOS == "windows" {
				imagePath = strings.TrimPrefix(imagePath, "/")
			}
		}
		imagePath = filepath.FromSlash(imagePath)
		if !filepath.IsAbs(imagePath) {
			imagePath = filepath.Join(filepath.Dir(documentPath), imagePath)
		}
		contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(imagePath)))
		if !strings.HasPrefix(contentType, "image/") {
			http.NotFound(w, r)
			return
		}
		file, err := os.Open(imagePath)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil || !info.Mode().IsRegular() {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeContent(w, r, info.Name(), info.ModTime(), file)
	})
}
