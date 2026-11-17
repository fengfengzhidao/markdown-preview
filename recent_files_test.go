package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestRecentFilesPersistMetadataAndMoveRepeatedFileToFront(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "settings", "recent-files.json")
	store := &recentFileStore{path: path}
	first := Document{Path: filepath.Join(directory, "项目一", "README.md"), Content: "private markdown body"}
	second := Document{Path: filepath.Join(directory, "项目二", "README.md")}
	list, err := store.remember([]Document{first, second}, "")
	if err != nil || len(list.Files) != 2 || list.Files[0].Path != second.Path || list.Revision != 1 {
		t.Fatalf("unexpected recent files: %+v, %v", list, err)
	}
	list.Files[0].Name = "modified snapshot"
	loaded, err := (&recentFileStore{path: path}).list()
	if err != nil || loaded.Files[0].Name != "README.md" || loaded.Revision != 1 {
		t.Fatalf("history was not restored correctly: %+v, %v", loaded, err)
	}
	updated, err := store.remember([]Document{first}, "")
	if err != nil || len(updated.Files) != 2 || updated.Files[0].Path != first.Path || updated.Revision != 2 {
		t.Fatalf("repeat did not update recency: %+v, %v", updated, err)
	}
	data, err := os.ReadFile(path)
	if err != nil || bytes.Contains(data, []byte(first.Content)) {
		t.Fatalf("unexpected history payload: %v", err)
	}
	unchanged, err := store.remember([]Document{first}, "")
	if err != nil || unchanged.Revision != updated.Revision {
		t.Fatalf("identical history changed its revision: %+v, %v", unchanged, err)
	}
}

func TestRemovingRecentFilePersistsRecordRemoval(t *testing.T) {
	directory := t.TempDir()
	file := filepath.Join(directory, "记录.md")
	if err := os.WriteFile(file, []byte("# 保留的内容"), 0644); err != nil {
		t.Fatal(err)
	}
	store := &recentFileStore{path: filepath.Join(directory, "recent-files.json")}
	list, err := store.remember([]Document{{Path: file}}, "")
	if err != nil {
		t.Fatal(err)
	}
	removed, err := store.remove(list.Files[0].ID)
	if err != nil || len(removed.Files) != 0 || removed.Revision <= list.Revision {
		t.Fatalf("remove failed: %+v, %v", removed, err)
	}
	reloaded, err := (&recentFileStore{path: store.path}).list()
	if err != nil || len(reloaded.Files) != 0 {
		t.Fatalf("removal was not persisted: %+v, %v", reloaded, err)
	}
	data, err := os.ReadFile(file)
	if err != nil || string(data) != "# 保留的内容" {
		t.Fatalf("source document changed: %q, %v", data, err)
	}
}

func TestOpenSaveAndRenameUpdateRecentFiles(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "原文件.md")
	if err := os.WriteFile(path, []byte("# 原文"), 0644); err != nil {
		t.Fatal(err)
	}
	service := &AppService{files: newFileStore(), recent: &recentFileStore{path: filepath.Join(directory, "recent-files.json")}}
	opened := service.OpenFiles([]string{path})
	if len(opened.Documents) != 1 || opened.RecentFiles == nil || len(opened.RecentFiles.Files) != 1 {
		t.Fatalf("open did not record history: %+v", opened)
	}
	oldID := opened.Documents[0].ID
	renamed, err := service.Rename(oldID, "已改名.md", "原文件.md", nil)
	if err != nil || renamed.RecentFiles == nil || len(renamed.RecentFiles.Files) != 1 || renamed.RecentFiles.Files[0].Path != renamed.Path {
		t.Fatalf("rename history mismatch: %+v, %v", renamed, err)
	}
	created, err := service.SaveNew(filepath.Join(directory, "新文件.md"), "# 新文档", "", false)
	if err != nil || created.RecentFiles == nil || len(created.RecentFiles.Files) != 2 || created.RecentFiles.Files[0].Name != "新文件.md" {
		t.Fatalf("new document history mismatch: %+v, %v", created, err)
	}
}

func TestInvalidRecentFileSettingsPreserveSourceOpening(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "document.md")
	settings := filepath.Join(directory, "recent-files.json")
	if err := os.WriteFile(path, []byte("# 正文"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settings, []byte("{invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	service := &AppService{files: newFileStore(), recent: &recentFileStore{path: settings}}
	result := service.OpenFiles([]string{path})
	if len(result.Documents) != 1 || result.RecentError == "" || result.Documents[0].Content != "# 正文" {
		t.Fatalf("document opening did not survive history failure: %+v", result)
	}
	data, err := os.ReadFile(settings)
	if err != nil || string(data) != "{invalid" {
		t.Fatalf("invalid settings were overwritten: %q, %v", data, err)
	}
}

func TestConcurrentRecentFileUpdatesHaveIncreasingRevisions(t *testing.T) {
	directory := t.TempDir()
	store := &recentFileStore{path: filepath.Join(directory, "recent-files.json")}
	const count = 20
	var work sync.WaitGroup
	for index := range count {
		work.Add(1)
		go func() {
			defer work.Done()
			_, err := store.remember([]Document{{Path: filepath.Join(directory, fmt.Sprintf("文档-%d.md", index))}}, "")
			if err != nil {
				t.Error(err)
			}
		}()
	}
	work.Wait()
	list, err := (&recentFileStore{path: store.path}).list()
	if err != nil || len(list.Files) != count || list.Revision != count {
		t.Fatalf("concurrent history updates were lost: %+v, %v", list, err)
	}
}
