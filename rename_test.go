package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRenameKeepsSourceBytesAndUpdatesSaveAndImages(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "original.md")
	content := "\ufeff# 标题\r\n\r\n![image](photo.png)\r\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "photo.png"), []byte("image"), 0644); err != nil {
		t.Fatal(err)
	}
	store := newFileStore()
	doc := store.open([]string{path}).Documents[0]
	result, err := store.rename(doc.ID, "中文改名")
	if err != nil || result.Name != "中文改名.md" || result.Path != filepath.Join(dir, "中文改名.md") {
		t.Fatalf("rename failed: %+v, %v", result, err)
	}
	data, err := os.ReadFile(result.Path)
	if err != nil || string(data) != content {
		t.Fatalf("rename changed source bytes: %q, %v", data, err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("old path still exists")
	}
	if _, err := store.save(doc.ID, "wrong path", doc.Revision, false); err == nil {
		t.Fatal("old ID was writable after rename")
	}
	folder, err := store.folder(result.ID)
	if err != nil || folder != dir {
		t.Fatalf("folder did not follow renamed file: %s, %v", folder, err)
	}
	response := httptest.NewRecorder()
	store.assetHandler(http.NotFoundHandler()).ServeHTTP(response, httptest.NewRequest("GET", "/local-image/"+result.ID+"?path=photo.png", nil))
	if response.Code != 200 || response.Body.String() != "image" {
		t.Fatal("relative image failed after rename")
	}
	saved, err := store.save(result.ID, "updated", doc.Revision, false)
	if err != nil || saved.Conflict {
		t.Fatalf("save after rename failed: %+v, %v", saved, err)
	}
	reloaded, err := store.reload(result.ID)
	if err != nil || reloaded.ID != result.ID || reloaded.Content != "updated" {
		t.Fatalf("reload after rename failed: %+v, %v", reloaded, err)
	}
}

func TestRenameRejectsExistingDestinationAndAllowsRetry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "source.md")
	target := filepath.Join(dir, "target.md")
	for name, content := range map[string]string{path: "source", target: "target"} {
		if err := os.WriteFile(name, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	store := newFileStore()
	doc := store.open([]string{path}).Documents[0]
	if _, err := store.rename(doc.ID, "target.md"); err == nil {
		t.Fatal("existing destination was accepted")
	}
	for name, expected := range map[string]string{path: "source", target: "target"} {
		data, err := os.ReadFile(name)
		if err != nil || string(data) != expected {
			t.Fatalf("failed rename changed %s", name)
		}
	}
	result, err := store.rename(doc.ID, "retry.markdown")
	if err != nil || result.Name != "retry.markdown" {
		t.Fatalf("retry failed: %+v, %v", result, err)
	}
}

func TestRenameValidatesNamesAndSupportsDrafts(t *testing.T) {
	invalid := []string{"", " ", ".", "..", "../other.md", `..\other.md`, "a/b.md", `a\b.md`, "a:bad.md", "a?bad.md", "a\x00.md", "a\n.md", "\xff.md", "trailing.", "note.txt"}
	if runtime.GOOS == "windows" {
		invalid = append(invalid, "CON", "aux.md", "LPT1.markdown", "COM9.md")
	}
	for _, name := range invalid {
		if _, err := normalizeDocumentName(name, ".md"); err == nil {
			t.Fatalf("invalid name accepted: %q", name)
		}
	}
	service := &AppService{files: newFileStore()}
	result, err := service.Rename("", "草稿", "未命名.markdown", nil)
	if err != nil || result.Name != "草稿.markdown" || result.ID != "" || result.Path != "" || len(service.files.paths) != 0 {
		t.Fatalf("draft rename failed: %+v, %v", result, err)
	}
	if _, err := service.files.rename("unknown", "valid.md"); err == nil {
		t.Fatal("unknown document was renamed")
	}
}

func TestRenameRejectsAnOpenMissingDestination(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.md")
	target := filepath.Join(dir, "target.md")
	for _, name := range []string{source, target} {
		if err := os.WriteFile(name, []byte("data"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	store := newFileStore()
	documents := store.open([]string{source, target}).Documents
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	if _, err := store.rename(documents[0].ID, "target.md", documents[1].ID); err == nil {
		t.Fatal("rename reused another open tab's identity")
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatal("rejected rename moved the source")
	}
}

func TestRenameHandlesUnchangedCaseOnlyAndLongPaths(t *testing.T) {
	dir := t.TempDir()
	if runtime.GOOS == "windows" {
		dir = filepath.Join(dir, strings.Repeat("a", 80), strings.Repeat("b", 80), strings.Repeat("c", 80))
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(dir, "CaseName.md")
	if err := os.WriteFile(path, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}
	store := newFileStore()
	doc := store.open([]string{path}).Documents[0]
	same, err := store.rename(doc.ID, doc.Name)
	if err != nil || same.ID != doc.ID || same.Path != doc.Path {
		t.Fatalf("unchanged name failed: %+v, %v", same, err)
	}
	result, err := store.rename(doc.ID, "casename.MD")
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 || entries[0].Name() != "casename.MD" {
		t.Fatalf("case-only name was not applied: %+v, %v", entries, err)
	}
	if runtime.GOOS == "windows" && result.ID != doc.ID {
		t.Fatal("case-only rename changed the canonical Windows identity")
	}
}

func TestConcurrentRenameHasOneWinner(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "source.md")
	if err := os.WriteFile(path, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}
	store := newFileStore()
	doc := store.open([]string{path}).Documents[0]
	type attempt struct {
		result RenameResult
		err    error
	}
	results := make(chan attempt, 2)
	for _, name := range []string{"one.md", "two.md"} {
		go func() {
			result, err := store.rename(doc.ID, name)
			results <- attempt{result, err}
		}()
	}
	winners := 0
	for range 2 {
		result := <-results
		if result.err == nil {
			winners++
			data, err := os.ReadFile(result.result.Path)
			if err != nil || string(data) != "data" {
				t.Fatal("winning rename lost data")
			}
		}
	}
	if winners != 1 || len(store.paths) != 1 {
		t.Fatalf("unexpected concurrent state: %d winners, %d paths", winners, len(store.paths))
	}
}

func TestWindowsRenameDoesNotReplaceExistingFiles(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows no-replace operation")
	}
	dir := t.TempDir()
	source := filepath.Join(dir, "source.md")
	target := filepath.Join(dir, "target.md")
	if err := os.WriteFile(source, []byte("source"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("target"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := moveDocumentFile(source, target); err == nil {
		t.Fatal("filesystem operation replaced an existing destination")
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "target" {
		t.Fatal("destination content was changed")
	}
}
