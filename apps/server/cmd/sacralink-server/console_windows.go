//go:build windows

package main

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

// attachParentConsole faz os comandos de terminal (admin, firewall, version)
// aparecerem quando o sacralink-server.exe é chamado direto de um console.
//
// O .exe é um aplicativo de janela (-H windowsgui, para não abrir console ao
// iniciar com o Windows): não fica conectado ao console de quem o chamou e,
// mesmo recebendo o handle do console, grava UTF-8 cru (acentos trocados).
// O caminho recomendado é o sacralink-server.com (cmd/sacralink-server-console),
// que entrega a saída por pipe e faz o terminal esperar. Isto é a reserva para
// quem digita ".exe": a mensagem aparece certa, mas o PowerShell e o cmd não
// esperam um aplicativo de janela e podem mostrar o prompt antes dela.
func attachParentConsole() {
	out, _ := windows.GetStdHandle(windows.STD_OUTPUT_HANDLE)
	errh, _ := windows.GetStdHandle(windows.STD_ERROR_HANDLE)
	if redirected(out) && redirected(errh) {
		return // arquivo ou pipe: sacralink-server.com, "> arquivo", instalador
	}
	attach := windows.NewLazySystemDLL("kernel32.dll").NewProc("AttachConsole")
	const attachParentProcess = ^uint32(0) // ATTACH_PARENT_PROCESS
	if r, _, _ := attach.Call(uintptr(attachParentProcess)); r == 0 {
		return // sem console no processo pai (instalador, painel, Explorer)
	}
	stdout, stderr := useConsole(out), useConsole(errh)
	if !stdout && !stderr {
		return
	}
	// Leitura e escrita: o GetConsoleMode exige leitura, e sem ele o Go não
	// reconhece o console e grava UTF-8 cru (acentos trocados).
	con, err := os.OpenFile("CONOUT$", os.O_RDWR, 0)
	if err != nil {
		return
	}
	if stdout {
		os.Stdout = con
	}
	if stderr {
		os.Stderr = con
	}
	fmt.Fprintln(con) // o prompt pode já ter voltado: começa numa linha nova
}

// redirected informa se a saída vai para um arquivo ou pipe.
func redirected(h windows.Handle) bool {
	if h == 0 || h == windows.InvalidHandle {
		return false
	}
	t, err := windows.GetFileType(h)
	return err == nil && (t == windows.FILE_TYPE_DISK || t == windows.FILE_TYPE_PIPE)
}

// useConsole (depois do AttachConsole): sem saída definida ou com o handle do
// próprio console. Um "> NUL" continua valendo.
func useConsole(h windows.Handle) bool {
	if h == 0 || h == windows.InvalidHandle {
		return true
	}
	var mode uint32
	return windows.GetConsoleMode(h, &mode) == nil
}
