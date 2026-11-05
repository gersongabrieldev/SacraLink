// Package buildinfo expõe metadados de versão injetados na compilação com
// -ldflags "-X github.com/gersongabrieldev/sacralink/apps/server/internal/buildinfo.Version=..."
package buildinfo

import "runtime"

var (
	// Version é a versão semântica do build (ex.: "0.1.0").
	Version = "0.0.0-dev"
	// Commit é o hash do commit git do build.
	Commit = "unknown"
	// Date é a data do build no formato RFC 3339.
	Date = "unknown"
)

// Info é um retrato serializável dos metadados do build.
type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	Date      string `json:"date"`
	GoVersion string `json:"goVersion"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
}

// Get retorna os metadados do binário em execução.
func Get() Info {
	return Info{
		Version:   Version,
		Commit:    Commit,
		Date:      Date,
		GoVersion: runtime.Version(),
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
	}
}
