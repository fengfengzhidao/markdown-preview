package main

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type AppService struct {
	app        *application.App
	window     *application.WebviewWindow
	files      *fileStore
	ready      atomic.Bool
	allowClose atomic.Bool
}

func (s *AppService) Ready() OpenResult {
	s.ready.Store(true)
	return s.files.open(os.Args[1:])
}

func (s *AppService) PickFiles() (OpenResult, error) {
	paths, err := s.app.Dialog.OpenFile().
		SetTitle("打开 Markdown 文件").
		AddFilter("Markdown 文件", "*.md;*.markdown").
		AttachToWindow(s.window).
		PromptForMultipleSelection()
	if err != nil {
		if isDialogCancelled(err) {
			return OpenResult{}, nil
		}
		return OpenResult{}, err
	}
	return s.files.open(paths), nil
}

func isDialogCancelled(err error) bool {
	// Wails keeps its Windows dialog cancellation error in an internal package.
	return err != nil && err.Error() == "cancelled by user"
}

func (s *AppService) PickSaveTarget(name string) (*Document, error) {
	dialog := s.app.Dialog.SaveFile()
	dialog.SetOptions(&application.SaveFileDialogOptions{
		Title: "保存 Markdown 文件", Filename: name, Window: s.window,
		CanCreateDirectories: true,
		Filters:              []application.FileFilter{{DisplayName: "Markdown 文件", Pattern: "*.md;*.markdown"}},
	})
	path, err := dialog.PromptForSingleSelection()
	if isDialogCancelled(err) || (err == nil && path == "") {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	target, err := prepareSaveTarget(path)
	if err != nil {
		return nil, err
	}
	return &target, nil
}

func (s *AppService) SaveNew(path, content, revision string, force bool) (SaveNewResult, error) {
	return s.files.saveNew(path, content, revision, force)
}

func (s *AppService) OpenFiles(paths []string) OpenResult {
	return s.files.open(paths)
}

func (s *AppService) Reload(id string) (Document, error) {
	return s.files.reload(id)
}

func (s *AppService) Save(id, content, revision string, force bool) (SaveResult, error) {
	return s.files.save(id, content, revision, force)
}

func (s *AppService) OpenFolder(id string) error {
	folder, err := s.files.folder(id)
	if err != nil {
		return err
	}
	return s.app.Env.OpenFileManager(folder, false)
}

func (s *AppService) Rename(id, name, currentName string, openedIDs []string) (RenameResult, error) {
	if id == "" {
		name, err := normalizeDocumentName(name, filepath.Ext(currentName))
		return RenameResult{Name: name}, err
	}
	return s.files.rename(id, name, openedIDs...)
}

func (s *AppService) Copy(content string) error {
	if !s.app.Clipboard.SetText(content) {
		return fmt.Errorf("剪贴板暂时无法写入，请重试")
	}
	return nil
}

func (s *AppService) OpenLink(address string) error {
	parsed, err := url.Parse(address)
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http" && parsed.Scheme != "mailto") {
		return fmt.Errorf("该链接无法通过浏览器打开")
	}
	return s.app.Browser.OpenURL(address)
}

func (s *AppService) Close() {
	s.allowClose.Store(true)
	s.window.Close()
}
