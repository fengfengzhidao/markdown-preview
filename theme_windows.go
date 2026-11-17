package main

import (
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/w32"
)

func (s *AppService) SetDarkMode(enabled bool) {
	titleColour := uint32(0x00fbfcfc)
	textColour := uint32(0x006e776a)
	borderColour := uint32(0x00e8eae8)
	background := application.NewRGB(255, 255, 255)
	if enabled {
		titleColour = 0x0021251e
		textColour = 0x00bcc5b8
		borderColour = 0x00343b30
		background = application.NewRGB(24, 30, 27)
	}
	s.window.SetBackgroundColour(background)
	application.InvokeSync(func() {
		hwnd := uintptr(s.window.NativeWindow())
		if hwnd == 0 {
			return
		}
		w32.SetTheme(hwnd, enabled)
		w32.SetTitleBarColour(hwnd, titleColour)
		w32.SetTitleTextColour(hwnd, textColour)
		w32.SetBorderColour(hwnd, borderColour)
	})
}
