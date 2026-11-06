package api

import (
	"math"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/gersongabrieldev/sacralink/apps/server/internal/store"
)

func (s *Server) listPresets(w http.ResponseWriter, r *http.Request) {
	ps, err := s.d.Store.ListPresets(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ps)
}

// createPreset salva um preset com parâmetros informados ou com a posição
// atual da câmera (fromCurrent).
func (s *Server) createPreset(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name        string              `json:"name"`
		FromCurrent bool                `json:"fromCurrent"`
		Params      *store.PresetParams `json:"params"`
	}
	if err := decode(r, &in); err != nil {
		writeError(w, err)
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || utf8.RuneCountInString(in.Name) > 40 || strings.ContainsAny(in.Name, "\x00\r\n\t") {
		writeError(w, badRequest("o nome do preset deve ter de 1 a 40 caracteres"))
		return
	}
	id := r.PathValue("id")
	var params store.PresetParams
	switch {
	case in.FromCurrent:
		p, err := s.d.Core.PresetFromCurrent(id)
		if err != nil {
			writeError(w, err)
			return
		}
		params = p
	case in.Params != nil:
		params = *in.Params
	default:
		writeError(w, badRequest("informe params ou fromCurrent"))
		return
	}
	if err := s.validatePreset(r, params); err != nil {
		writeError(w, err)
		return
	}
	p, err := s.d.Store.CreatePreset(r.Context(), id, in.Name, params)
	if err != nil {
		writeError(w, err)
		return
	}
	_ = s.d.Store.Audit(r.Context(), userFrom(r.Context()), "camera.preset_create", id, clientIP(r), true, in.Name)
	writeJSON(w, http.StatusCreated, p)
}

func (s *Server) validatePreset(r *http.Request, p store.PresetParams) error {
	if p.Zoom != nil && (math.IsNaN(*p.Zoom) || *p.Zoom < 0.1 || *p.Zoom > 100) {
		return badRequest("zoom do preset fora da faixa 0,1–100")
	}
	if p.LensID != nil && (len(*p.LensID) == 0 || len(*p.LensID) > 64) {
		return badRequest("lente inválida")
	}
	if p.ProfileID != nil {
		if _, err := s.d.Store.GetProfile(r.Context(), *p.ProfileID); err != nil {
			return badRequest("perfil de qualidade desconhecido")
		}
	}
	return nil
}

func (s *Server) deletePreset(w http.ResponseWriter, r *http.Request) {
	if err := s.d.Store.DeletePreset(r.Context(), r.PathValue("id"), r.PathValue("pid")); err != nil {
		writeError(w, err)
		return
	}
	_ = s.d.Store.Audit(r.Context(), userFrom(r.Context()), "camera.preset_delete", r.PathValue("id"), clientIP(r), true, r.PathValue("pid"))
	w.WriteHeader(http.StatusNoContent)
}

// applyPreset envia o preset ao celular como comando com confirmação.
func (s *Server) applyPreset(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Force bool `json:"force"`
	}
	_ = decode(r, &in)
	s.runCommand(w, r, commandInput{Name: "preset.apply", Params: map[string]any{"presetId": r.PathValue("pid")}, Force: in.Force})
}
