//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

// detach inicia o servidor desvinculado do painel (sem console e fora do
// grupo de processos), para que ele continue rodando após fechar o painel.
func detach(cmd *exec.Cmd) {
	const detachedProcess = 0x00000008
	const createNewProcessGroup = 0x00000200
	const createNoWindow = 0x08000000
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: detachedProcess | createNewProcessGroup | createNoWindow}
}
