package main

import (
	"embed"
	"fmt"
	"log"
	"os"
	"runtime/debug"

	"github.com/ddombrow/malachi/coding"
	"github.com/ddombrow/malachi/sandbox"
	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var appIcon []byte

// appID matches build/config.yml's productIdentifier and the installed
// .desktop file's name.
const appID = "io.github.ddombrow.malachi-desktop"

func main() {
	// malachi runs in this process. On Linux a sandboxed command starts as
	// this binary in helper mode, which must confine itself before anything
	// else runs, exactly as in malachi's own main.
	sandbox.MaybeRunHelper()
	// Run with --memory-mcp, this binary is the sessions' malachi-memory
	// server (memory.go), not the app.
	runMemoryServer()
	if err := sandbox.ProtectProcess(); err != nil {
		fmt.Fprintln(os.Stderr, "malachi-desktop: cannot protect process environment from child commands:", err)
		os.Exit(1)
	}
	stampMalachiVersion()
	adoptLoginShellPath()

	app := application.New(application.Options{
		Name:        "malachi",
		Description: "Desktop frontend for the malachi coding agent",
		Icon:        appIcon,
		Services: []application.Service{
			application.NewService(&MalachiService{}),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
		// Wayland compositors find the window's icon through its app_id,
		// matched to an installed <appID>.desktop (task install:desktop).
		Linux: application.LinuxOptions{
			ApplicationID: appID,
		},
	})

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "malachi",
		Width:            1200,
		Height:           820,
		MinWidth:         640,
		MinHeight:        420,
		BackgroundColour: application.NewRGB(17, 18, 22),
		URL:              "/",
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 40,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

// stampMalachiVersion reports malachi's own module version (from this
// binary's build info) instead of the desktop app's, which is what malachi
// would find as the main module. Providers see it in the User-Agent.
func stampMalachiVersion() {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}
	for _, dep := range info.Deps {
		if dep.Path != "github.com/ddombrow/malachi" {
			continue
		}
		v := dep.Version
		if dep.Replace != nil && dep.Replace.Version != "" {
			v = dep.Replace.Version
		}
		if v != "" && v != "(devel)" {
			coding.Version = v
			coding.UserAgent = "malachi/" + v
		}
		return
	}
}
