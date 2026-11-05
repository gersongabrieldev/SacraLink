package api

import (
	_ "embed"
	"html/template"
	"net/http"
)

//go:embed player.html
var playerHTML string

var playerTmpl = template.Must(template.New("player").Parse(playerHTML))

// player serve a página WHEP usada pela Browser Source do OBS (ADR-0004,
// alternativa B). A página fala apenas com esta mesma origem (loopback).
func (s *Server) player(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	cam, err := s.d.Store.GetCamera(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; connect-src 'self'; media-src 'self' blob:")
	_ = playerTmpl.Execute(w, map[string]any{"CameraID": cam.ID, "Name": cam.Name, "Muted": r.URL.Query().Get("muted") == "1"})
}
