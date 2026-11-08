// Package webcam serve a câmera pelo navegador: página que transforma o
// celular em câmera do SacraLink sem instalar aplicativo. Ela usa as mesmas
// APIs do app nativo (pareamento, canal de controle v1 e WHIP).
package webcam

import (
	"embed"
	"io/fs"
	"net/http"
	"regexp"
)

//go:embed static
var static embed.FS

// validHost aceita host[:porta] (nome, IPv4 ou [IPv6]) para entrar na CSP sem
// permitir injeção de diretivas.
var validHost = regexp.MustCompile(`^[A-Za-z0-9.\-]+(:[0-9]{1,5})?$|^\[[0-9A-Fa-f:.]+\](:[0-9]{1,5})?$`)

// Handler serve /cam/ com cabeçalhos de segurança estritos.
func Handler() http.Handler {
	sub, err := fs.Sub(static, "static")
	if err != nil {
		panic(err)
	}
	files := http.StripPrefix("/cam/", http.FileServer(http.FS(sub)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		// Somente recursos desta origem; WebSocket apenas para este servidor.
		connect := "'self'"
		if validHost.MatchString(r.Host) {
			connect += " wss://" + r.Host
		}
		h.Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self' data:; "+
			"media-src 'self' blob: mediastream:; connect-src "+connect+"; manifest-src 'self'; "+
			"base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
		h.Set("Permissions-Policy", "camera=(self), microphone=(self), fullscreen=(self), screen-wake-lock=(self)")
		h.Set("Cache-Control", "no-cache") // atualizações do servidor chegam na hora
		h.Set("X-Content-Type-Options", "nosniff")
		files.ServeHTTP(w, r)
	})
}
