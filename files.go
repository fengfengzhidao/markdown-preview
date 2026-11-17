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
	"regexp"
	"runtime"
	"slices"
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

type SaveNewResult struct {
	Document *Document `json:"document"`
	Conflict bool      `json:"conflict"`
}

type RenameResult struct {
	ID   string `json:"id"`
	Path string `json:"path"`
	Name string `json:"name"`
}

type fileStore struct {
	mu    sync.Mutex
	paths map[string]string
}

var reservedWindowsName = regexp.MustCompile(`(?i)^(CON|PRN|AUX|NUL|CONIN\$|CONOUT\$|COM[1-9¹²³]|LPT[1-9¹²³])$`)

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
	return Document{
		ID: documentID(resolved), Path: resolved, Name: filepath.Base(resolved),
		Content: string(data), Revision: digest(data),
	}, nil
}

func documentID(path string) string {
	if runtime.GOOS == "windows" {
		path = strings.ToLower(path)
	}
	return digest([]byte(path))
}

func normalizeDocumentName(name, extension string) (string, error) {
	name = strings.TrimSpace(name)
	stem := strings.TrimRight(strings.SplitN(name, ".", 2)[0], " ")
	if name == "" || name == "." || name == ".." || !utf8.ValidString(name) || !filepath.IsLocal(name) ||
		strings.ContainsAny(name, `\/:*?"<>|`) || strings.HasSuffix(name, ".") ||
		(runtime.GOOS == "windows" && reservedWindowsName.MatchString(stem)) ||
		strings.IndexFunc(name, func(value rune) bool { return value < 32 }) >= 0 {
		return "", fmt.Errorf("请输入有效文件名，文件名不能包含路径或特殊字符")
	}
	if filepath.Ext(name) == "" {
		if extension == "" {
			extension = ".md"
		}
		name += extension
	}
	extension = strings.ToLower(filepath.Ext(name))
	if extension != ".md" && extension != ".markdown" {
		return "", fmt.Errorf("文件扩展名需要是 .md 或 .markdown")
	}
	return name, nil
}

func (s *fileStore) folder(id string) (string, error) {
	s.mu.Lock()
	path, ok := s.paths[id]
	s.mu.Unlock()
	if !ok {
		return "", fmt.Errorf("文件尚未打开")
	}
	folder := filepath.Dir(path)
	info, err := os.Stat(folder)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("文件夹不可用")
	}
	return folder, nil
}

func (s *fileStore) rename(id, name string, openedIDs ...string) (RenameResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	path, ok := s.paths[id]
	if !ok {
		return RenameResult{}, fmt.Errorf("文件尚未打开")
	}
	name, err := normalizeDocumentName(name, filepath.Ext(path))
	if err != nil {
		return RenameResult{}, err
	}
	target := filepath.Join(filepath.Dir(path), name)
	newID := documentID(target)
	if newID != id && slices.Contains(openedIDs, newID) {
		return RenameResult{}, fmt.Errorf("目标文件已在其他标签页打开，请使用其他名称")
	}
	sourceInfo, err := os.Lstat(path)
	if err != nil || !sourceInfo.Mode().IsRegular() {
		return RenameResult{}, fmt.Errorf("原文件不可用，请检查文件是否已移动或删除")
	}
	if target != path {
		targetInfo, targetErr := os.Lstat(target)
		caseOnly := runtime.GOOS == "windows" && strings.EqualFold(path, target)
		if targetErr == nil && !(caseOnly && os.SameFile(sourceInfo, targetInfo)) {
			return RenameResult{}, fmt.Errorf("同名文件已存在，请换一个文件名")
		}
		if targetErr != nil && !os.IsNotExist(targetErr) {
			return RenameResult{}, fmt.Errorf("无法检查目标文件，%w", targetErr)
		}
		if err := moveDocumentFile(path, target); err != nil {
			return RenameResult{}, fmt.Errorf("重命名失败，%w", err)
		}
	}
	delete(s.paths, id)
	s.paths[newID] = target
	return RenameResult{ID: newID, Path: target, Name: name}, nil
}

func prepareSaveTarget(path string) (Document, error) {
	if path == "" {
		return Document{}, fmt.Errorf("请选择保存位置")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return Document{}, fmt.Errorf("保存路径无效，%w", err)
	}
	if filepath.Ext(absolute) == "" {
		absolute += ".md"
	}
	extension := strings.ToLower(filepath.Ext(absolute))
	if extension != ".md" && extension != ".markdown" {
		return Document{}, fmt.Errorf("请选择 .md 或 .markdown 文件名")
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if os.IsNotExist(err) {
		parent, parentErr := filepath.EvalSymlinks(filepath.Dir(absolute))
		if parentErr != nil {
			return Document{}, fmt.Errorf("保存目录不可用，%w", parentErr)
		}
		resolved = filepath.Join(parent, filepath.Base(absolute))
	} else if err != nil {
		return Document{}, fmt.Errorf("保存路径不可用，%w", err)
	}
	extension = strings.ToLower(filepath.Ext(resolved))
	if extension != ".md" && extension != ".markdown" {
		return Document{}, fmt.Errorf("保存目标需要是 .md 或 .markdown 文件")
	}
	data, err := os.ReadFile(resolved)
	if err != nil && !os.IsNotExist(err) {
		return Document{}, fmt.Errorf("无法读取保存目标，%w", err)
	}
	revision := ""
	if err == nil {
		revision = digest(data)
	}
	return Document{ID: documentID(resolved), Path: resolved, Name: filepath.Base(resolved), Revision: revision}, nil
}

func (s *fileStore) saveNew(path, content, revision string, force bool) (SaveNewResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	target, err := prepareSaveTarget(path)
	if err != nil {
		return SaveNewResult{}, err
	}
	if !force && target.Revision != revision {
		return SaveNewResult{Conflict: true}, nil
	}
	if !utf8.ValidString(content) || strings.ContainsRune(content, '\x00') {
		return SaveNewResult{}, fmt.Errorf("文档需要使用 UTF-8 文本编码")
	}
	revision, err = writeDocument(target.Path, content)
	if err != nil {
		return SaveNewResult{}, err
	}
	target.Content = content
	target.Revision = revision
	s.paths[target.ID] = target.Path
	return SaveNewResult{Document: &target}, nil
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
	revision, err = writeDocument(path, content)
	return SaveResult{Revision: revision}, err
}

func writeDocument(path, content string) (string, error) {
	mode := os.FileMode(0644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
		if mode&0222 == 0 {
			return "", fmt.Errorf("文件为只读，请检查文件权限")
		}
	}
	data := []byte(content)
	if err := writeAtomic(path, data, mode); err != nil {
		return "", fmt.Errorf("保存失败，%w", err)
	}
	return digest(data), nil
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
