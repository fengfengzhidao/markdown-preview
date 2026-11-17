package main

import (
	"strings"

	"golang.org/x/sys/windows"
)

func extendedRenamePath(path string) string {
	if strings.HasPrefix(path, `\\?\`) {
		return path
	}
	if strings.HasPrefix(path, `\\`) {
		return `\\?\UNC\` + path[2:]
	}
	return `\\?\` + path
}

func moveDocumentFile(source, target string) error {
	from, err := windows.UTF16PtrFromString(extendedRenamePath(source))
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(extendedRenamePath(target))
	if err != nil {
		return err
	}
	// Flags zero makes an existing destination fail at the filesystem operation.
	return windows.MoveFileEx(from, to, 0)
}
