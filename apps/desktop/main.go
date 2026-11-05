// SacraLink.exe — painel de controle desktop (Wails v2 + WebView2).
//
// O painel é uma janela nativa que exibe o frontend React embutido e conversa
// com o sacralink-server pela Admin API local (127.0.0.1:7380). Fechar o
// painel NÃO interrompe as câmeras: o servidor continua na bandeja (ADR-0001).
package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp()
	err := wails.Run(&options.App{
		Title:            "SacraLink",
		Width:            1440,
		Height:           900,
		MinWidth:         1024,
		MinHeight:        680,
		BackgroundColour: &options.RGBA{R: 11, G: 14, B: 20, A: 255},
		AssetServer:      &assetserver.Options{Assets: assets},
		OnStartup:        app.startup,
		Bind:             []interface{}{app},
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId:               "br.org.sacralink.painel",
			OnSecondInstanceLaunch: app.onSecondInstance,
		},
		Windows: &windows.Options{
			Theme:                windows.Dark,
			WebviewUserDataPath:  userDataPath(),
			WebviewGpuIsDisabled: false,
		},
	})
	if err != nil {
		log.Fatal(err)
	}
}
