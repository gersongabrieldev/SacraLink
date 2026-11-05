// Comando sl-genicons: gera os ícones do SacraLink (ICO e PNG) a partir da
// identidade visual desenhada em código (internal/assets).
//
//	go run ./cmd/sl-genicons -out ../../assets/icons
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gersongabrieldev/sacralink/apps/server/internal/assets"
)

func main() {
	out := flag.String("out", "icons", "diretório de saída")
	flag.Parse()
	if err := os.MkdirAll(*out, 0o755); err != nil {
		fail(err)
	}
	files := map[string][]byte{
		"sacralink.ico":    assets.ICO(false),
		"appicon.png":      assets.PNG(1024, false),
		"logo-256.png":     assets.PNG(256, false),
		"installer.ico":    assets.ICO(false, 16, 32, 48, 256),
		"sacralink-on.ico": assets.ICO(true, 16, 24, 32, 48),
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(*out, name), data, 0o644); err != nil {
			fail(err)
		}
		fmt.Println("gerado", filepath.Join(*out, name))
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
