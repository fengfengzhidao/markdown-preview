package main

import (
	"path/filepath"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func (s *AppService) handleSecondInstance(data application.SecondInstanceData) {
	s.launchMu.Lock()
	// The first argument is the executable. Relative filenames belong to the caller's directory.
	for index, path := range data.Args {
		if index == 0 {
			continue
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(data.WorkingDir, path)
		}
		s.pendingLaunchFiles = append(s.pendingLaunchFiles, filepath.Clean(path))
	}
	s.launchMu.Unlock()
	if s.ready.Load() {
		// Keep the single-instance message receiver free while Wails dispatches UI work.
		go func() {
			s.app.Event.Emit("app:open-files")
			s.window.UnMinimise()
			s.window.Show()
			s.window.Focus()
		}()
	}
}

func (s *AppService) TakeLaunchFiles() OpenResult {
	s.launchMu.Lock()
	paths := s.pendingLaunchFiles
	s.pendingLaunchFiles = nil
	s.launchMu.Unlock()
	return s.openDocuments(paths)
}
