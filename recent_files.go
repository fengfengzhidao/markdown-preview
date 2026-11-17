package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
)

type RecentFile struct {
	ID   string `json:"id"`
	Path string `json:"path"`
	Name string `json:"name"`
}

type RecentFileList struct {
	Revision uint64       `json:"revision"`
	Files    []RecentFile `json:"files"`
}

type recentFileStore struct {
	mu     sync.Mutex
	path   string
	loaded bool
	state  RecentFileList
}

func (s *recentFileStore) loadLocked() error {
	if s.loaded {
		return nil
	}
	if s.path == "" {
		folder, err := os.UserConfigDir()
		if err != nil {
			return fmt.Errorf("无法获取文件列表保存目录，%w", err)
		}
		s.path = filepath.Join(folder, "Moye", "recent-files.json")
	}
	data, err := os.ReadFile(s.path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("无法读取文件列表，%w", err)
	}
	state := RecentFileList{Files: []RecentFile{}}
	if err == nil {
		if err := json.Unmarshal(data, &state); err != nil {
			return fmt.Errorf("文件列表格式无效，%w", err)
		}
	}
	state.Files = normalizeRecentFiles(state.Files)
	s.state = state
	s.loaded = true
	return nil
}

func normalizeRecentFiles(files []RecentFile) []RecentFile {
	result := make([]RecentFile, 0, len(files))
	seen := make(map[string]bool, len(files))
	for _, file := range files {
		extension := strings.ToLower(filepath.Ext(file.Path))
		if !filepath.IsAbs(file.Path) || (extension != ".md" && extension != ".markdown") {
			continue
		}
		path := filepath.Clean(file.Path)
		id := documentID(path)
		if !seen[id] {
			result = append(result, RecentFile{ID: id, Path: path, Name: filepath.Base(path)})
			seen[id] = true
		}
	}
	return result
}

func (s *recentFileStore) snapshotLocked() RecentFileList {
	return RecentFileList{Revision: s.state.Revision, Files: append([]RecentFile{}, s.state.Files...)}
}

func (s *recentFileStore) saveLocked(files []RecentFile) (RecentFileList, error) {
	if slices.Equal(files, s.state.Files) {
		return s.snapshotLocked(), nil
	}
	next := RecentFileList{Revision: s.state.Revision + 1, Files: files}
	data, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return RecentFileList{}, err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return RecentFileList{}, fmt.Errorf("无法创建文件列表目录，%w", err)
	}
	if err := writeAtomic(s.path, data, 0600); err != nil {
		return RecentFileList{}, fmt.Errorf("文件列表保存失败，%w", err)
	}
	s.state = next
	return s.snapshotLocked(), nil
}

func (s *recentFileStore) list() (RecentFileList, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadLocked(); err != nil {
		return RecentFileList{}, err
	}
	return s.snapshotLocked(), nil
}

func (s *recentFileStore) remember(documents []Document, previousID string) (RecentFileList, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadLocked(); err != nil {
		return RecentFileList{}, err
	}
	files := make([]RecentFile, 0, len(documents)+len(s.state.Files))
	for index := len(documents) - 1; index >= 0; index-- {
		document := documents[index]
		files = append(files, RecentFile{ID: document.ID, Path: document.Path, Name: document.Name})
	}
	for _, file := range s.state.Files {
		if file.ID != previousID {
			files = append(files, file)
		}
	}
	return s.saveLocked(normalizeRecentFiles(files))
}

func (s *recentFileStore) remove(id string) (RecentFileList, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadLocked(); err != nil {
		return RecentFileList{}, err
	}
	files := make([]RecentFile, 0, len(s.state.Files))
	for _, file := range s.state.Files {
		if file.ID != id {
			files = append(files, file)
		}
	}
	return s.saveLocked(files)
}
