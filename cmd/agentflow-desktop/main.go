package main

import (
	"log"

	"github.com/diasYuri/agentflow"
	"github.com/diasYuri/agentflow/internal/desktop/adapter"
	"github.com/diasYuri/agentflow/internal/desktop/binding"
	"github.com/wailsapp/wails/v3/pkg/application"
)

func main() {
	ad := adapter.NewDefaultAdapter()
	app := application.New(application.Options{
		Name:        "Agentflow Desktop",
		Description: "Agentflow desktop application",
		Services: []application.Service{
			application.NewService(binding.NewDesktopService(ad)),
		},
		Assets: application.AssetOptions{
			Handler: application.BundledAssetFileServer(agentflow.DesktopAssets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "Agentflow Desktop",
		Width:            1280,
		Height:           800,
		BackgroundColour: application.NewRGB(245, 245, 247),
		Mac: application.MacWindow{
			TitleBar:                application.MacTitleBarHiddenInset,
			Backdrop:                application.MacBackdropTranslucent,
			InvisibleTitleBarHeight: 40,
		},
		URL: "/",
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
