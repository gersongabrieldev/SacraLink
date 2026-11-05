package main

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// Version é injetada no build (-X main.Version=...).
var Version = "0.0.0-dev"

// App expõe ao frontend as poucas operações que dependem do desktop.
type App struct {
	ctx       context.Context
	serverURL string
}

// NewApp cria o App.
func NewApp() *App {
	url := os.Getenv("SACRALINK_SERVER_URL")
	if url == "" {
		url = "http://127.0.0.1:7380"
	}
	return &App{serverURL: url}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	go a.ensureServer()
}

func (a *App) onSecondInstance(options.SecondInstanceData) {
	if a.ctx == nil {
		return
	}
	runtime.WindowUnminimise(a.ctx)
	runtime.Show(a.ctx)
}

// ServerURL retorna o endereço da Admin API (chamado pelo frontend).
func (a *App) ServerURL() string { return a.serverURL }

// AppVersion retorna a versão do painel.
func (a *App) AppVersion() string { return Version }

// OpenLogsFolder abre a pasta de logs do SacraLink no Explorer.
func (a *App) OpenLogsFolder() {
	_ = exec.Command("explorer", filepath.Join(dataDir(), "logs")).Start()
}

// ensureServer inicia o sacralink-server.exe (instalado ao lado do painel)
// se a Admin API não responder. O servidor sobrevive ao fechamento do painel.
func (a *App) ensureServer() {
	if healthy(a.serverURL) {
		return
	}
	exe, err := os.Executable()
	if err != nil {
		return
	}
	server := filepath.Join(filepath.Dir(exe), "sacralink-server.exe")
	if _, err := os.Stat(server); err != nil {
		return // desenvolvimento: o servidor é iniciado manualmente
	}
	cmd := exec.Command(server)
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return
	}
	_ = cmd.Process.Release()
	for i := 0; i < 40 && !healthy(a.serverURL); i++ {
		time.Sleep(250 * time.Millisecond)
	}
}

func healthy(base string) bool {
	c := http.Client{Timeout: 800 * time.Millisecond}
	resp, err := c.Get(base + "/api/v1/health")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func dataDir() string {
	if d := os.Getenv("LOCALAPPDATA"); d != "" {
		return filepath.Join(d, "SacraLink")
	}
	d, _ := os.UserConfigDir()
	return filepath.Join(d, "SacraLink")
}

// userDataPath mantém o perfil do WebView2 junto dos dados do SacraLink.
func userDataPath() string { return filepath.Join(dataDir(), "webview2") }
