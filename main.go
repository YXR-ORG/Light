package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
)

// Version is injected at build time via -ldflags "-X main.Version=x.y.z"
var Version = "dev"

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp()

	err := wails.Run(&options.App{
		Title:  "Light",
		Width:  1200,
		Height: 800,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		Mac: &mac.Options{
			TitleBar: mac.TitleBarHiddenInset(),
		},
		OnStartup: app.startup,
		Bind: []interface{}{
			app,
			app.chatHandler,
			app.conversationHandler,
			app.settingsHandler,
			app.mcpHandler,
			app.providerHandler,
			app.agentHandler,
			app.skillHandler,
			app.backupHandler,
			app.knowledgeHandler,
			app.taskHandler,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
