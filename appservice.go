package main

import (
	"fmt"
	"net/url"
	"os"
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
		return OpenResult{}, err
	}
	return s.files.open(paths), nil
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
