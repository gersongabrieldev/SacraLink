// Comando genres: gera o recurso Windows (.syso) com ícone, manifesto
// (DPI por monitor, Common Controls v6, sem elevação) e informações de versão
// para os executáveis do SacraLink — dispensa a CLI do Wails no CI.
//
//	go run ./tools/genres -icon ../../assets/icons/sacralink.ico -name SacraLink \
//	    -desc "Painel do SacraLink" -version 0.1.0 -out rsrc_windows_amd64.syso
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/tc-hib/winres"
	"github.com/tc-hib/winres/version"
)

func main() {
	iconPath := flag.String("icon", "", "arquivo .ico")
	kind := flag.String("kind", "painel", "executável: painel | servidor")
	ver := flag.String("version", "0.0.0", "versão semântica")
	out := flag.String("out", "rsrc_windows_amd64.syso", "arquivo de saída")
	flag.Parse()
	// Textos acentuados ficam aqui (UTF-8): o PowerShell 5.1 corrompe
	// argumentos não ASCII passados a programas nativos.
	name, file, desc := "SacraLink", "SacraLink.exe", "SacraLink — Painel de controle"
	if *kind == "servidor" {
		file, desc = "sacralink-server.exe", "SacraLink — Servidor de câmeras"
	}

	rs := winres.ResourceSet{}
	f, err := os.Open(*iconPath)
	if err != nil {
		fail(err)
	}
	icon, err := winres.LoadICO(f)
	f.Close()
	if err != nil {
		fail(err)
	}
	if err := rs.SetIcon(winres.Name("APPICON"), icon); err != nil {
		fail(err)
	}
	rs.SetManifest(winres.AppManifest{
		Identity:            winres.AssemblyIdentity{Name: "br.org.sacralink." + strings.ToLower(strings.TrimSuffix(file, ".exe"))},
		Description:         desc,
		Compatibility:       winres.Win10AndAbove,
		ExecutionLevel:      winres.AsInvoker,
		DPIAwareness:        winres.DPIPerMonitorV2,
		UseCommonControlsV6: true,
		LongPathAware:       true,
	})

	v := numericVersion(*ver)
	vi := version.Info{}
	vi.SetFileVersion(v)
	vi.SetProductVersion(v)
	for k, val := range map[string]string{
		version.ProductName:      name,
		version.FileDescription:  desc,
		version.CompanyName:      "SacraLink",
		version.LegalCopyright:   "Copyright © 2026 Gerson Gabriel e colaboradores do SacraLink (MIT)",
		version.OriginalFilename: file,
	} {
		if err := vi.Set(0x0416, k, val); err != nil { // pt-BR
			fail(err)
		}
	}
	rs.SetVersionInfo(vi)

	o, err := os.Create(*out)
	if err != nil {
		fail(err)
	}
	defer o.Close()
	if err := rs.WriteObject(o, winres.ArchAMD64); err != nil {
		fail(err)
	}
	fmt.Println("gerado", *out)
}

// numericVersion converte "0.1.0-dev" em "0.1.0.0".
func numericVersion(v string) string {
	v = strings.SplitN(v, "-", 2)[0]
	parts := strings.Split(v, ".")
	for len(parts) < 4 {
		parts = append(parts, "0")
	}
	return strings.Join(parts[:4], ".")
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "genres:", err)
	os.Exit(1)
}
