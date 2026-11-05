//go:build !windows

// Package tray: sem bandeja fora do Windows (desenvolvimento/CI).
package tray

import "fmt"

// Options configura a bandeja.
type Options struct {
	Icon           []byte
	Tooltip        string
	Status         func() string
	OnOpen         func()
	OnRestartMedia func()
	OnOpenLogs     func()
	OnQuit         func() bool
}

var quit = make(chan struct{})

// Run bloqueia até Quit.
func Run(Options) { <-quit }

// Quit libera Run.
func Quit() {
	select {
	case <-quit:
	default:
		close(quit)
	}
}

// Confirm sempre confirma fora do Windows.
func Confirm(title, message string) bool { return true }

// Alert imprime a mensagem.
func Alert(title, message string) { fmt.Println(title + ": " + message) }

// Supported informa se a bandeja está disponível.
const Supported = false
