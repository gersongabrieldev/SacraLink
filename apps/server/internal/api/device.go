package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gersongabrieldev/sacralink/apps/server/internal/buildinfo"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/control"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/devices"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/ratelimit"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/store"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/webcam"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/whip"
)

// limitadores da API dos dispositivos (exposta à rede).
var (
	deviceLimiter  = ratelimit.New(20, 40, 10, 5*time.Minute, 10*time.Minute)
	pairingLimiter = ratelimit.New(0.2, 5, 5, 10*time.Minute, 15*time.Minute)
)

// DeviceHandler retorna o handler da API dos dispositivos (servida em TLS).
func (s *Server) DeviceHandler() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("GET /device/v1/info", s.deviceInfo)
	m.HandleFunc("POST /device/v1/pair", s.devicePair)
	m.HandleFunc("GET /device/v1/ws", s.deviceAuthWS(s.deviceWS))
	m.HandleFunc("POST /device/v1/ws-ticket", s.deviceAuth(s.deviceWSTicket))
	// Câmera pelo navegador (página servida pelo próprio computador da igreja).
	m.Handle("GET /cam/", webcam.Handler())
	m.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/cam/", http.StatusFound) })
	m.HandleFunc("POST /device/v1/whip", s.deviceAuth(s.whipOffer))
	m.HandleFunc("PATCH /device/v1/whip/sessions/{sid}", s.deviceAuth(s.whipPatch))
	m.HandleFunc("DELETE /device/v1/whip/sessions/{sid}", s.deviceAuth(s.whipDelete))
	m.HandleFunc("POST /device/v1/reachability", s.deviceAuth(s.reachability))
	m.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, &Error{http.StatusNotFound, "not_found", "recurso inexistente"})
	})
	return securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !deviceLimiter.Allow(clientIP(r)) {
			writeError(w, &Error{http.StatusTooManyRequests, "rate_limited", "muitas requisições"})
			return
		}
		w.Header().Set("Strict-Transport-Security", "max-age=31536000")
		m.ServeHTTP(w, r)
	}))
}

type deviceCtxKey struct{}

type deviceAuthInfo struct {
	device *store.Device
	camera *store.Camera
}

func deviceFrom(ctx context.Context) deviceAuthInfo {
	v, _ := ctx.Value(deviceCtxKey{}).(deviceAuthInfo)
	return v
}

// deviceAuth autentica o aparelho e bloqueia IPs com falhas repetidas.
func (s *Server) deviceAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)
		if deviceLimiter.Blocked(ip) {
			writeError(w, &Error{http.StatusTooManyRequests, "rate_limited", "muitas tentativas inválidas"})
			return
		}
		d, c, err := s.d.Devices.Authenticate(r.Context(), r.Header.Get("Authorization"))
		if err != nil {
			if errors.Is(err, devices.ErrUnauthorized) {
				deviceLimiter.Fail(ip)
				_ = s.d.Store.Audit(r.Context(), "dispositivo", "device.auth", "", ip, false, err.Error())
			}
			writeError(w, err)
			return
		}
		deviceLimiter.Success(ip)
		next(w, r.WithContext(context.WithValue(r.Context(), deviceCtxKey{}, deviceAuthInfo{d, c})))
	}
}

// deviceInfo identifica o servidor (descoberta e teste de alcance). Não
// expõe dados sensíveis.
func (s *Server) deviceInfo(w http.ResponseWriter, r *http.Request) {
	e := s.d.System.Endpoints()
	writeJSON(w, http.StatusOK, map[string]any{
		"product": "SacraLink", "serverId": e.ServerID, "serverName": e.ServerName,
		"version": buildinfo.Version, "apiVersion": 1, "controlProtocol": control.ProtocolVersion,
	})
}

func (s *Server) devicePair(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if !pairingLimiter.Allow(ip) {
		writeError(w, &Error{http.StatusTooManyRequests, "rate_limited", "muitas tentativas de pareamento; aguarde"})
		return
	}
	var in struct {
		Token  string             `json:"token"`
		Device devices.DeviceInfo `json:"device"`
	}
	if err := decode(r, &in); err != nil {
		writeError(w, err)
		return
	}
	res, err := s.d.Devices.Pair(r.Context(), in.Token, in.Device, ip)
	if err != nil {
		if errors.Is(err, store.ErrPairingInvalid) {
			pairingLimiter.Fail(ip)
		}
		_ = s.d.Store.Audit(r.Context(), "dispositivo", "device.pair", "", ip, false, err.Error())
		writeError(w, err)
		return
	}
	_ = s.d.Store.Audit(r.Context(), "dispositivo", "device.pair", res.DeviceID, ip, true, in.Device.Name)
	_ = s.d.Core.Reload(r.Context())
	if s.d.Events != nil {
		s.d.Events.Publish("device_paired", map[string]any{"deviceId": res.DeviceID, "cameraId": res.CameraID, "name": in.Device.Name})
	}
	writeJSON(w, http.StatusCreated, res)
}

func (s *Server) deviceWS(w http.ResponseWriter, r *http.Request) {
	a := deviceFrom(r.Context())
	s.d.Core.Control().Serve(w, r, control.Peer{DeviceID: a.device.ID, CameraID: a.camera.ID, RemoteAddr: clientIP(r)})
}

func (s *Server) whipOffer(w http.ResponseWriter, r *http.Request) {
	a := deviceFrom(r.Context())
	if !devices.HasPermission(a.device, store.PermPublishVideo) {
		writeError(w, &Error{http.StatusForbidden, "no_permission", "dispositivo sem permissão para transmitir"})
		return
	}
	if st := s.d.Media.Status(); st.State != "running" {
		w.Header().Set("Retry-After", "2")
		writeError(w, &Error{http.StatusServiceUnavailable, "media_starting", "o servidor de mídia está iniciando; tente novamente em instantes"})
		return
	}
	if err := s.d.Core.CanPublish(a.camera.ID); err != nil {
		writeError(w, err)
		return
	}
	body, err := readBody(r)
	if err != nil {
		writeError(w, err)
		return
	}
	resp, err := s.d.Proxy.Offer(r.Context(), whip.Publish, a.camera.ID, a.camera.MediaPath(), body, clientIP(r), "/device/v1/whip/sessions/")
	if err != nil {
		writeError(w, &Error{http.StatusBadGateway, "media_error", err.Error()})
		return
	}
	resp.Write(w)
}

func (s *Server) whipPatch(w http.ResponseWriter, r *http.Request) {
	a := deviceFrom(r.Context())
	body, err := readBody(r)
	if err != nil {
		writeError(w, err)
		return
	}
	resp, err := s.d.Proxy.Patch(r.Context(), whip.Publish, r.PathValue("sid"), a.camera.ID, body, r.Header.Get("Content-Type"), r.Header.Get("If-Match"))
	if err != nil {
		writeError(w, err)
		return
	}
	resp.Write(w)
}

func (s *Server) whipDelete(w http.ResponseWriter, r *http.Request) {
	a := deviceFrom(r.Context())
	st, err := s.d.Proxy.Delete(r.Context(), whip.Publish, r.PathValue("sid"), a.camera.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(st)
}

// reachability registra que um celular alcançou o servidor de fora da rede
// local — o único teste conclusivo de acesso externo sem servidor de terceiros.
func (s *Server) reachability(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Network string `json:"network"`
	}
	_ = decode(r, &in)
	if in.Network == "" {
		in.Network = "desconhecida"
	}
	s.d.System.RecordReachability(clientIP(r), in.Network)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "seenIp": clientIP(r)})
}

// deviceWSTicket emite um ticket de uso único para o WebSocket do navegador.
func (s *Server) deviceWSTicket(w http.ResponseWriter, r *http.Request) {
	a := deviceFrom(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{"ticket": s.devTickets.issue(a.device.ID), "expiresInSec": int(TicketTTL.Seconds())})
}

// deviceAuthWS aceita o cabeçalho Authorization (app nativo) ou um ticket de
// uso único na URL (navegador).
func (s *Server) deviceAuthWS(next http.HandlerFunc) http.HandlerFunc {
	withHeader := s.deviceAuth(next)
	return func(w http.ResponseWriter, r *http.Request) {
		t := r.URL.Query().Get("ticket")
		if t == "" {
			withHeader(w, r)
			return
		}
		ip := clientIP(r)
		deviceID, ok := s.devTickets.use(t)
		if !ok {
			deviceLimiter.Fail(ip)
			writeError(w, &Error{http.StatusUnauthorized, "unauthorized", "ticket inválido ou expirado"})
			return
		}
		d, err := s.d.Store.GetDevice(r.Context(), deviceID)
		if err != nil {
			writeError(w, devices.ErrUnauthorized)
			return
		}
		switch d.Status {
		case store.DeviceRevoked:
			writeError(w, devices.ErrRevoked)
			return
		case store.DeviceBlocked:
			writeError(w, devices.ErrBlocked)
			return
		}
		c, err := s.d.Store.GetCameraByDevice(r.Context(), d.ID)
		if err != nil {
			writeError(w, devices.ErrRevoked)
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), deviceCtxKey{}, deviceAuthInfo{d, c})))
	}
}
