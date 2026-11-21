// Comando sacralink-server-console: compilado como sacralink-server.com, ao
// lado do sacralink-server.exe, para os comandos de terminal funcionarem
// como em qualquer programa de console.
//
// O sacralink-server.exe é um aplicativo de janela (não abre console ao
// iniciar com o Windows), e o PowerShell e o cmd não esperam aplicativos de
// janela nem mostram a saída deles. Ao digitar "sacralink-server" sem
// extensão, o Windows escolhe o .com antes do .exe (ordem do PATHEXT); este
// programa executa o .exe com a entrada e a saída do terminal, espera o
// término e devolve o mesmo código de saída. É o mesmo arranjo do
// devenv.com/devenv.exe do Visual Studio.
//
// Uso (na pasta de instalação):
//
//	.\sacralink-server admin reset --confirm
//	.\sacralink-server version
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

const target = "sacralink-server.exe"

func main() {
	self, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, "não foi possível localizar o", target+":", err)
		os.Exit(1)
	}
	cmd := exec.Command(filepath.Join(filepath.Dir(self), target), os.Args[1:]...)
	cmd.Stdin = os.Stdin
	// A saída passa por um pipe (o Writer embrulhado impede o exec de repassar
	// o handle do console): o .exe não está conectado ao console e gravaria
	// UTF-8 cru, com os acentos trocados. Aqui o Go converte para o console.
	cmd.Stdout, cmd.Stderr = struct{ io.Writer }{os.Stdout}, struct{ io.Writer }{os.Stderr}
	err = cmd.Run()
	var exit *exec.ExitError
	switch {
	case errors.As(err, &exit):
		os.Exit(exit.ExitCode())
	case err != nil:
		fmt.Fprintln(os.Stderr, "não foi possível executar o", target+":", err)
		os.Exit(1)
	}
}
