package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenAndSavePreservesSourceBytes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "中文 Markdown.md")
	original := "\ufeff# 标题\r\n\r\n  保留空格  \r\n"
	if err := os.WriteFile(path, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}
	store := newFileStore()
	opened := store.open([]string{path})
	if len(opened.Documents) != 1 || len(opened.Errors) != 0 {
		t.Fatalf("open failed: %+v", opened)
	}
	doc := opened.Documents[0]
	if doc.Content != original {
		t.Fatalf("source was changed: %q", doc.Content)
	}
	updated := original + "新内容\r\n"
	result, err := store.save(doc.ID, updated, doc.Revision, false)
	if err != nil || result.Conflict {
		t.Fatalf("save failed: %+v, %v", result, err)
	}
	actual, err := os.ReadFile(path)
	if err != nil || string(actual) != updated || result.Revision != digest(actual) {
		t.Fatalf("saved bytes differ: %q, %v", actual, err)
	}
}

func TestSaveDetectsExternalChangesAndDeletedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conflict.md")
	if err := os.WriteFile(path, []byte("original"), 0644); err != nil {
		t.Fatal(err)
	}
	store := newFileStore()
	doc := store.open([]string{path}).Documents[0]
	if err := os.WriteFile(path, []byte("external"), 0644); err != nil {
		t.Fatal(err)
	}
	result, err := store.save(doc.ID, "editor", doc.Revision, false)
	if err != nil || !result.Conflict {
		t.Fatalf("external change was overwritten: %+v, %v", result, err)
	}
	actual, _ := os.ReadFile(path)
	if string(actual) != "external" {
		t.Fatal("conflict changed disk contents")
	}
	result, err = store.save(doc.ID, "editor", doc.Revision, true)
	if err != nil || result.Conflict {
		t.Fatalf("explicit overwrite failed: %+v, %v", result, err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	result, err = store.save(doc.ID, "recreated", result.Revision, false)
	if err != nil || !result.Conflict {
		t.Fatalf("deleted file conflict missing: %+v, %v", result, err)
	}
	result, err = store.save(doc.ID, "recreated", "", true)
	if err != nil || result.Conflict {
		t.Fatalf("recreation failed: %+v, %v", result, err)
	}
}

func TestBatchOpenKeepsValidFilesAndCanonicalIdentity(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "good.md")
	if err := os.WriteFile(path, []byte("# okay"), 0644); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(dir, "binary.md")
	if err := os.WriteFile(binary, []byte{0xff, 0x00}, 0644); err != nil {
		t.Fatal(err)
	}
	store := newFileStore()
	result := store.open([]string{binary, filepath.Join(dir, "missing.md"), path, filepath.Join(dir, ".", "good.md")})
	if len(result.Documents) != 2 || len(result.Errors) != 2 || result.Documents[0].ID != result.Documents[1].ID {
		t.Fatalf("unexpected batch: %+v", result)
	}
	if _, err := store.save("unknown", "test", "", true); err == nil {
		t.Fatal("unopened file was writable")
	}
}

func TestRelativeImageAndNonImageAsset(t *testing.T) {
	dir := t.TempDir()
	notes := filepath.Join(dir, "notes")
	if err := os.Mkdir(notes, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(notes, "note.md")
	if err := os.WriteFile(path, []byte("![示例](../本地 图片.png)"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "本地 图片.png"), []byte("test-image"), 0644); err != nil {
		t.Fatal(err)
	}
	store := newFileStore()
	doc := store.open([]string{path}).Documents[0]
	handler := store.assetHandler(http.NotFoundHandler())
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest("GET", "/local-image/"+doc.ID+"?path="+url.QueryEscape("../本地 图片.png"), nil))
	if response.Code != 200 || response.Header().Get("Content-Type") != "image/png" || response.Body.String() != "test-image" {
		t.Fatalf("relative image failed: %+v", response)
	}
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest("GET", "/local-image/"+doc.ID+"?path=note.md", nil))
	if response.Code != 404 {
		t.Fatal("non-image served as image")
	}
}
