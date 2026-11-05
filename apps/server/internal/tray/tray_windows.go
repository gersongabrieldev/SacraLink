//go:build windows

// Package tray implementa o ícone do SacraLink na bandeja do Windows.
package tray

import (
	"time"
	"unsafe"

	"fyne.io/systray"
	"golang.org/x/sys/windows"
)

// Options configura a bandeja.
type Options struct {
	Icon           []byte
	Tooltip        string
	Status         func() string
	OnOpen         func()
	OnRestartMedia func()
	OnOpenLogs     func()
	// OnQuit retorna true para encerrar (pode pedir confirmação).
	OnQuit func() bool
}

// Run exibe a bandeja e bloqueia até Quit ser chamado.
func Run(o Options) {
	systray.Run(func() { ready(o) }, func() {})
}

// Quit remove o ícone e libera Run.
func Quit() { systray.Quit() }

func ready(o Options) {
	systray.SetIcon(o.Icon)
	systray.SetTitle("SacraLink")
	systray.SetTooltip(o.Tooltip)
	open := systray.AddMenuItem("Abrir painel", "Abrir o painel de controle")
	status := systray.AddMenuItem("Iniciando…", "")
	status.Disable()
	systray.AddSeparator()
	restart := systray.AddMenuItem("Reiniciar servidor de mídia", "Reinicia o MediaMTX (as câmeras reconectam sozinhas)")
	logs := systray.AddMenuItem("Abrir pasta de logs", "")
	systray.AddSeparator()
	quit := systray.AddMenuItem("Sair do SacraLink", "Encerra o servidor e as transmissões das câmeras")
	systray.SetOnTapped(func() { go o.OnOpen() })

	go func() {
		t := time.NewTicker(3 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-open.ClickedCh:
				go o.OnOpen()
			case <-restart.ClickedCh:
				go o.OnRestartMedia()
			case <-logs.ClickedCh:
				go o.OnOpenLogs()
			case <-quit.ClickedCh:
				if o.OnQuit() {
					systray.Quit()
					return
				}
			case <-t.C:
				if o.Status != nil {
					s := o.Status()
					status.SetTitle(s)
					systray.SetTooltip("SacraLink — " + s)
				}
			}
		}
	}()
}

var (
	user32          = windows.NewLazySystemDLL("user32.dll")
	procMessageBoxW = user32.NewProc("MessageBoxW")
)

// Confirm exibe uma caixa de diálogo Sim/Não nativa.
func Confirm(title, message string) bool {
	t, _ := windows.UTF16PtrFromString(title)
	m, _ := windows.UTF16PtrFromString(message)
	const mbYesNo, mbIconWarning, mbTopmost, idYes = 0x4, 0x30, 0x40000, 6
	r, _, _ := procMessageBoxW.Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), mbYesNo|mbIconWarning|mbTopmost)
	return r == idYes
}

// Alert exibe uma mensagem de erro nativa.
func Alert(title, message string) {
	t, _ := windows.UTF16PtrFromString(title)
	m, _ := windows.UTF16PtrFromString(message)
	const mbOK, mbIconError, mbTopmost = 0x0, 0x10, 0x40000
	procMessageBoxW.Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), mbOK|mbIconError|mbTopmost)
}

// Supported informa se a bandeja está disponível.
const Supported = true
