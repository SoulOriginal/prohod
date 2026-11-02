package main

import (
	"embed"
	"fmt"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
	"github.com/wailsapp/wails/v2/pkg/options/mac"

	"prohod/internal/core"
	"prohod/internal/guard"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var icon []byte

func main() {
	if len(os.Args) > 1 && os.Args[1] == guard.Flag {
		guard.Run(os.Args[2:])
		return
	}
	ctl, err := core.New()
	if err != nil {
		fmt.Fprintln(os.Stderr, "init:", err)
		os.Exit(1)
	}
	ctl.StartGuard()
	app := NewApp(ctl)

	err = wails.Run(&options.App{
		Title:            "Prohod",
		Width:            400,
		Height:           680,
		MinWidth:         380,
		MinHeight:        600,
		AssetServer:      &assetserver.Options{Assets: assets},
		BackgroundColour: &options.RGBA{R: 14, G: 17, B: 23, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Bind:             []interface{}{app},
		Mac: &mac.Options{
			TitleBar:             mac.TitleBarHiddenInset(),
			WebviewIsTransparent: true,
			About:                &mac.AboutInfo{Title: "Prohod", Message: "Version " + Version, Icon: icon},
		},
		Linux: &linux.Options{Icon: icon, ProgramName: "prohod"},
	})
	if err != nil {
		ctl.Shutdown()
		fmt.Fprintln(os.Stderr, "Error:", err)
	}
}
