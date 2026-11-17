package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func TestSecondInstanceQueuesFilesBeforeFrontendReady(t *testing.T) {
	directory := t.TempDir()
	firstDir := filepath.Join(directory, "第一个目录")
	secondDir := filepath.Join(directory, "另一个 目录")
	for _, dir := range []string{firstDir, secondDir} {
		if err := os.Mkdir(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# "+filepath.Base(dir)), 0644); err != nil {
			t.Fatal(err)
		}
	}
	service := &AppService{files: newFileStore()}
	service.handleSecondInstance(application.SecondInstanceData{
		Args:       []string{"MarkdownLite.exe", "README.md", filepath.Join(secondDir, "README.md")},
		WorkingDir: firstDir,
	})
	opened := service.TakeLaunchFiles()
	if len(opened.Errors) != 0 || len(opened.Documents) != 2 {
		t.Fatalf("unexpected forwarded files: %+v", opened)
	}
	if opened.Documents[0].Path != filepath.Join(firstDir, "README.md") || opened.Documents[1].Path != filepath.Join(secondDir, "README.md") {
		t.Fatalf("paths were resolved against the wrong working directory: %+v", opened.Documents)
	}
	if opened.Documents[0].ID == opened.Documents[1].ID {
		t.Fatal("different files with the same basename shared a tab identity")
	}
	if again := service.TakeLaunchFiles(); len(again.Documents) != 0 || len(again.Errors) != 0 {
		t.Fatalf("forwarded files were delivered more than once: %+v", again)
	}
}

func TestSecondInstanceWithoutFilesAndRepeatedPaths(t *testing.T) {
	service := &AppService{files: newFileStore()}
	service.handleSecondInstance(application.SecondInstanceData{})
	service.handleSecondInstance(application.SecondInstanceData{Args: []string{"MarkdownLite.exe"}})
	if opened := service.TakeLaunchFiles(); len(opened.Documents) != 0 || len(opened.Errors) != 0 {
		t.Fatalf("executable was treated as a document: %+v", opened)
	}
	path := filepath.Join(t.TempDir(), "重复 文件.md")
	if err := os.WriteFile(path, []byte("# 内容"), 0644); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		service.handleSecondInstance(application.SecondInstanceData{Args: []string{"MarkdownLite.exe", path}})
	}
	opened := service.TakeLaunchFiles()
	if len(opened.Errors) != 0 || len(opened.Documents) != 2 || opened.Documents[0].ID != opened.Documents[1].ID {
		t.Fatalf("repeated files lost their existing identity: %+v", opened)
	}
}

func TestConcurrentSecondInstanceRequestsKeepEveryFile(t *testing.T) {
	directory := t.TempDir()
	const count = 32
	for index := range count {
		path := filepath.Join(directory, fmt.Sprintf("文件-%02d.md", index))
		if err := os.WriteFile(path, []byte("# 文档"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	service := &AppService{files: newFileStore()}
	var requests sync.WaitGroup
	for index := range count {
		requests.Add(1)
		go func() {
			defer requests.Done()
			service.handleSecondInstance(application.SecondInstanceData{
				Args: []string{"MarkdownLite.exe", fmt.Sprintf("文件-%02d.md", index)}, WorkingDir: directory,
			})
		}()
	}
	requests.Wait()
	opened := service.TakeLaunchFiles()
	if len(opened.Errors) != 0 || len(opened.Documents) != count {
		t.Fatalf("concurrent requests were lost: %+v", opened)
	}
	identities := make(map[string]bool)
	for _, document := range opened.Documents {
		identities[document.ID] = true
	}
	if len(identities) != count {
		t.Fatal("concurrent requests duplicated documents")
	}
}
