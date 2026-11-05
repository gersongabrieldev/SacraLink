// Comando sl-fakeobs: OBS simulado (obs-websocket 5.x) para desenvolvimento
// do painel e demonstrações sem o OBS Studio instalado. Escuta apenas em
// 127.0.0.1. NÃO substitui testes com o OBS real.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"

	"github.com/gersongabrieldev/sacralink/apps/server/internal/obs/obstest"
)

func main() {
	password := flag.String("password", "", "senha do obs-websocket simulado")
	studio := flag.Bool("studio", true, "iniciar com Studio Mode ligado")
	flag.Parse()
	srv := obstest.New(*password)
	defer srv.Close()
	srv.AddScene("Missa — Plano geral")
	srv.AddScene("Intervalo")
	srv.SetStudioMode(*studio)
	fmt.Println("OBS simulado em", srv.URL())
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)
	<-ch
}
