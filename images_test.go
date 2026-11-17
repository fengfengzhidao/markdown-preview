package main

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func pastedPNG(t *testing.T) ([]byte, string) {
	t.Helper()
	picture := image.NewRGBA(image.Rect(0, 0, 2, 2))
	picture.Set(0, 0, color.RGBA{R: 128, G: 200, A: 255})
	var data bytes.Buffer
	if err := png.Encode(&data, picture); err != nil {
		t.Fatal(err)
	}
	return data.Bytes(), "data:image/png;base64," + base64.StdEncoding.EncodeToString(data.Bytes())
}

func TestPastedImagesKeepRelativePathsAndServeOriginalBytes(t *testing.T) {
	folder := t.TempDir()
	path := filepath.Join(folder, "我的 文档.md")
	if err := os.WriteFile(path, []byte("# 原文"), 0644); err != nil {
		t.Fatal(err)
	}
	store := newFileStore()
	doc := store.open([]string{path}).Documents[0]
	data, dataURL := pastedPNG(t)
	const count = 8
	results := make(chan string, count)
	errors := make(chan error, count)
	var workers sync.WaitGroup
	for range count {
		workers.Go(func() {
			relative, err := store.saveImage(doc.ID, dataURL)
			results <- relative
			errors <- err
		})
	}
	workers.Wait()
	seen := make(map[string]bool)
	for range count {
		if err := <-errors; err != nil {
			t.Fatal(err)
		}
		relative := <-results
		if filepath.IsAbs(relative) || filepath.Dir(filepath.FromSlash(relative)) != "我的 文档.assets" || seen[relative] {
			t.Fatalf("invalid or repeated image path: %q", relative)
		}
		seen[relative] = true
		stored, err := os.ReadFile(filepath.Join(folder, filepath.FromSlash(relative)))
		if err != nil || !bytes.Equal(stored, data) {
			t.Fatalf("pasted image bytes changed: %v", err)
		}
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/local-image/"+doc.ID+"?path="+url.QueryEscape(relative), nil)
		store.assetHandler(http.NotFoundHandler()).ServeHTTP(response, request)
		if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "image/png" || !bytes.Equal(response.Body.Bytes(), data) {
			t.Fatalf("saved image was not readable: %d", response.Code)
		}
	}
	content, err := os.ReadFile(path)
	if err != nil || string(content) != "# 原文" {
		t.Fatalf("pasting an image modified the document on disk: %v", err)
	}
}

func TestPastedImagesRejectInvalidDataAndUnsavedDocuments(t *testing.T) {
	store := newFileStore()
	_, valid := pastedPNG(t)
	for _, data := range []string{valid, "data:image/png;base64,?", "data:image/png;base64,dGV4dA==", "data:text/plain;base64,dGV4dA=="} {
		if _, err := store.saveImage("missing", data); err == nil {
			t.Fatalf("invalid paste succeeded: %q", data)
		}
	}
}
