package main

import (
	"embed"
	"log"
	"net/http"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var appIcon []byte

func main() {
	service := &AppService{files: newFileStore()}
	titleBarColour := uint32(0x00fbfcfc)
	titleTextColour := uint32(0x006e776a)
	borderColour := uint32(0x00e8eae8)
	windowTheme := &application.WindowTheme{
		TitleBarColour:  &titleBarColour,
		TitleTextColour: &titleTextColour,
		BorderColour:    &borderColour,
	}
	app := application.New(application.Options{
		Name:        "墨页",
		Icon:        appIcon,
		Description: "简约的本地 Markdown 阅读器",
		Services:    []application.Service{application.NewService(service)},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
			Middleware: func(next http.Handler) http.Handler {
				return service.files.assetHandler(next)
			},
		},
		Mac: application.MacOptions{ApplicationShouldTerminateAfterLastWindowClosed: true},
	})
	service.app = app
	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title: "墨页", Width: 1380, Height: 900,
		MinWidth: 900, MinHeight: 560,
		BackgroundColour: application.NewRGB(255, 255, 255),
		Windows: application.WindowsWindow{
			Theme: application.Light,
			CustomTheme: application.ThemeSettings{
				LightModeActive:   windowTheme,
				LightModeInactive: windowTheme,
			},
		},
		EnableFileDrop: true, URL: "/",
	})
	service.window = window
	window.OnWindowEvent(events.Common.WindowFilesDropped, func(event *application.WindowEvent) {
		app.Event.Emit("files:drop", event.Context().DroppedFiles())
	})
	window.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
		if service.ready.Load() && !service.allowClose.Load() {
			event.Cancel()
			app.Event.Emit("app:close-request")
		}
	})
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
