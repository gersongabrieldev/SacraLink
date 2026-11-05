// Package api expõe as APIs HTTP do SacraLink:
//   - Admin API (127.0.0.1:7380): painel, REST + WebSocket, senha do administrador.
//   - Device API (:7443, TLS): pareamento, canal de controle e proxy WHIP.
//
// Contratos documentados em docs/api/.
package api

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/gersongabrieldev/sacralink/apps/server/internal/control"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/core"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/devices"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/obsbridge"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/store"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/whip"
)

// Error é um erro de API com código estável.
type Error struct {
	Status  int    `json:"-"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return e.Message }

func badRequest(msg string) *Error { return &Error{http.StatusBadRequest, "invalid_request", msg} }

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

// errorMap traduz erros de domínio em respostas HTTP.
var errorMap = []struct {
	err    error
	status int
	code   string
}{
	{store.ErrNotFound, http.StatusNotFound, "not_found"},
	{store.ErrPairingInvalid, http.StatusUnauthorized, "pairing_invalid"},
	{store.ErrCameraTaken, http.StatusConflict, "camera_taken"},
	{devices.ErrUnauthorized, http.StatusUnauthorized, "unauthorized"},
	{devices.ErrRevoked, http.StatusForbidden, "device_revoked"},
	{devices.ErrBlocked, http.StatusForbidden, "device_blocked"},
	{devices.ErrInvalidInput, http.StatusBadRequest, "invalid_device_info"},
	{control.ErrInvalidCommand, http.StatusBadRequest, "invalid_command"},
	{core.ErrStreamLimit, http.StatusServiceUnavailable, "stream_limit"},
	{core.ErrNoPermission, http.StatusForbidden, "no_permission"},
	{core.ErrUnknownCamera, http.StatusNotFound, "not_found"},
	{obsbridge.ErrNotConnected, http.StatusServiceUnavailable, "obs_disconnected"},
	{obsbridge.ErrStudioModeOff, http.StatusConflict, "studio_mode_off"},
	{obsbridge.ErrConfirmationRequired, http.StatusConflict, "confirmation_required"},
	{obsbridge.ErrOutputConfirmRequired, http.StatusConflict, "confirmation_required"},
	{obsbridge.ErrNoSource, http.StatusConflict, "no_source"},
	{obsbridge.ErrSourceRemovedByUser, http.StatusConflict, "source_removed"},
	{obsbridge.ErrIngestModeMismatch, http.StatusConflict, "ingest_mode_mismatch"},
	{whip.ErrUnknownSession, http.StatusNotFound, "not_found"},
}

func writeError(w http.ResponseWriter, err error) {
	var ae *Error
	if errors.As(err, &ae) {
		writeJSON(w, ae.Status, map[string]any{"error": ae})
		return
	}
	for _, m := range errorMap {
		if errors.Is(err, m.err) {
			writeJSON(w, m.status, map[string]any{"error": Error{Code: m.code, Message: err.Error()}})
			return
		}
	}
	writeJSON(w, http.StatusInternalServerError, map[string]any{"error": Error{Code: "internal", Message: err.Error()}})
}

// decode lê JSON estrito (campos desconhecidos rejeitados, limite de 1 MiB).
func decode(r *http.Request, v any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		if errors.Is(err, io.EOF) {
			return badRequest("corpo da requisição vazio")
		}
		return badRequest("JSON inválido: " + err.Error())
	}
	return nil
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		if !strings.HasPrefix(r.URL.Path, "/player/") {
			h.Set("X-Frame-Options", "DENY")
		}
		next.ServeHTTP(w, r)
	})
}
