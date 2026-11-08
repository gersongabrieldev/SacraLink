package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/coder/websocket"

	"github.com/gersongabrieldev/sacralink/apps/server/internal/buildinfo"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/core"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/devices"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/events"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/media"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/netdiag"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/obsbridge"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/settings"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/store"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/whip"
)

// OBSConfigView é a configuração do OBS exibida no painel (sem a senha).
type OBSConfigView struct {
	Host           string `json:"host"`
	Port           int    `json:"port"`
	HasPassword    bool   `json:"hasPassword"`
	AutoConnect    bool   `json:"autoConnect"`
	LastOBSVersion string `json:"lastObsVersion,omitempty"`
	LastWSVersion  string `json:"lastWsVersion,omitempty"`
}

// OBSConfigInput é a alteração da configuração do OBS. Password nil mantém a
// senha atual; "" remove.
type OBSConfigInput struct {
	Host        string  `json:"host"`
	Port        int     `json:"port"`
	Password    *string `json:"password,omitempty"`
	AutoConnect bool    `json:"autoConnect"`
}

// System reúne operações de sistema implementadas pela aplicação.
type System interface {
	Status(ctx context.Context) any
	Network(ctx context.Context, refresh bool) netdiag.Report
	RecordReachability(ip, network string)
	FirewallStatus() any
	ApplyFirewall(ctx context.Context) error
	SetAutostart(enabled bool) error
	OBSConfig(ctx context.Context) (OBSConfigView, error)
	SaveOBSConfig(ctx context.Context, in OBSConfigInput) error
	ImportLocalOBSConfig(ctx context.Context) (OBSConfigView, error)
	DetectOBS() any
	Logs() map[string][]string
	Endpoints() devices.Endpoints
	ApplySettings(ctx context.Context, s settings.Settings)
	SelfTest(ctx context.Context) any
}

// Deps são as dependências das APIs.
type Deps struct {
	Store      *store.Store
	Settings   *settings.Manager
	Core       *core.Core
	Devices    *devices.Service
	Proxy      *whip.Proxy
	Media      *media.Supervisor
	Events     *events.Hub
	System     System
	Logger     *slog.Logger
	AdminPort  int
	DevOrigins bool // aceitar o servidor de desenvolvimento do Vite
}

// Server implementa as duas APIs.
type Server struct {
	d          Deps
	devTickets deviceTickets
	auth       *Auth
	log        *slog.Logger
	origins    []string
	hosts      []string
}

// New cria o servidor de APIs.
func New(d Deps) *Server {
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	port := strconv.Itoa(d.AdminPort)
	s := &Server{d: d, auth: NewAuth(d.Store), log: d.Logger.With("componente", "api"),
		origins: []string{"http://wails.localhost", "wails://wails", "http://wails.localhost:34115", "http://127.0.0.1:" + port},
		hosts:   []string{"127.0.0.1:" + port, "localhost:" + port, "[::1]:" + port},
	}
	if d.DevOrigins {
		s.origins = append(s.origins, "http://localhost:5173", "http://127.0.0.1:5173")
	}
	return s
}

// Auth expõe o gerenciador de autenticação (testes).
func (s *Server) Auth() *Auth { return s.auth }

// adminGuard protege a API local contra DNS rebinding (Host) e CSRF (Origin)
// e aplica CORS apenas às origens do painel.
func (s *Server) adminGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !slices.Contains(s.hosts, r.Host) {
			writeError(w, &Error{http.StatusMisdirectedRequest, "invalid_host", "host não permitido"})
			return
		}
		origin := r.Header.Get("Origin")
		if origin != "" {
			if !slices.Contains(s.origins, origin) {
				writeError(w, &Error{http.StatusForbidden, "invalid_origin", "origem não permitida"})
				return
			}
			h := w.Header()
			h.Set("Access-Control-Allow-Origin", origin)
			h.Set("Vary", "Origin")
			h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, If-Match")
			h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			h.Set("Access-Control-Expose-Headers", "Location, ETag, Link")
			h.Set("Access-Control-Allow-Private-Network", "true")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// AdminHandler retorna o handler da API de administração.
func (s *Server) AdminHandler() http.Handler {
	m := http.NewServeMux()
	a := s.auth.require

	m.HandleFunc("GET /api/v1/health", s.health)
	m.HandleFunc("GET /api/v1/setup/status", s.setupStatus)
	m.HandleFunc("POST /api/v1/setup/admin", s.setupAdmin)
	m.HandleFunc("POST /api/v1/setup/complete", a(s.setupComplete))
	m.HandleFunc("POST /api/v1/auth/login", s.login)
	m.HandleFunc("POST /api/v1/auth/logout", a(s.logout))
	m.HandleFunc("POST /api/v1/auth/password", a(s.changePassword))
	m.HandleFunc("POST /api/v1/auth/ws-ticket", a(s.wsTicket))
	m.HandleFunc("GET /api/v1/ws", s.ws)

	m.HandleFunc("GET /api/v1/system/status", a(s.systemStatus))
	m.HandleFunc("GET /api/v1/system/logs", a(s.systemLogs))
	m.HandleFunc("POST /api/v1/system/selftest", a(s.selfTest))
	m.HandleFunc("POST /api/v1/system/media/restart", a(s.restartMedia))
	m.HandleFunc("GET /api/v1/system/firewall", a(s.firewallStatus))
	m.HandleFunc("POST /api/v1/system/firewall", a(s.applyFirewall))
	m.HandleFunc("POST /api/v1/system/autostart", a(s.autostart))
	m.HandleFunc("GET /api/v1/settings", a(s.getSettings))
	m.HandleFunc("PUT /api/v1/settings", a(s.putSettings))
	m.HandleFunc("GET /api/v1/network/status", a(s.network(false)))
	m.HandleFunc("POST /api/v1/network/diagnose", a(s.network(true)))

	m.HandleFunc("GET /api/v1/devices", a(s.listDevices))
	m.HandleFunc("POST /api/v1/devices/pair", a(s.createPairing))
	m.HandleFunc("PATCH /api/v1/devices/{id}", a(s.renameDevice))
	m.HandleFunc("POST /api/v1/devices/{id}/{action}", a(s.deviceAction))
	m.HandleFunc("DELETE /api/v1/devices/{id}", a(s.deleteDevice))

	m.HandleFunc("GET /api/v1/profiles", a(s.profiles))
	m.HandleFunc("GET /api/v1/cameras", a(s.cameras))
	m.HandleFunc("GET /api/v1/cameras/{id}", a(s.camera))
	m.HandleFunc("PATCH /api/v1/cameras/{id}", a(s.patchCamera))
	m.HandleFunc("POST /api/v1/cameras/{id}/commands", a(s.command))
	m.HandleFunc("POST /api/v1/cameras/{id}/zoom", a(s.commandAlias("zoom")))
	m.HandleFunc("POST /api/v1/cameras/{id}/lens", a(s.commandAlias("lens")))
	m.HandleFunc("POST /api/v1/cameras/{id}/quality", a(s.commandAlias("quality")))
	m.HandleFunc("GET /api/v1/cameras/{id}/metrics", a(s.cameraMetrics))
	m.HandleFunc("GET /api/v1/cameras/{id}/presets", a(s.listPresets))
	m.HandleFunc("POST /api/v1/cameras/{id}/presets", a(s.createPreset))
	m.HandleFunc("DELETE /api/v1/cameras/{id}/presets/{pid}", a(s.deletePreset))
	m.HandleFunc("POST /api/v1/cameras/{id}/presets/{pid}/apply", a(s.applyPreset))

	m.HandleFunc("GET /api/v1/obs/status", a(s.obsStatus))
	m.HandleFunc("GET /api/v1/obs/detect", a(s.obsDetect))
	m.HandleFunc("GET /api/v1/obs/config", a(s.obsConfig))
	m.HandleFunc("PUT /api/v1/obs/config", a(s.putOBSConfig))
	m.HandleFunc("POST /api/v1/obs/config/import", a(s.importOBSConfig))
	m.HandleFunc("GET /api/v1/obs/scenes", a(s.obsScenes))
	m.HandleFunc("POST /api/v1/obs/preview", a(s.obsPreview))
	m.HandleFunc("POST /api/v1/obs/program", a(s.obsProgram))
	m.HandleFunc("POST /api/v1/obs/transition", a(s.obsTransition))
	m.HandleFunc("GET /api/v1/obs/transitions", a(s.obsTransitions))
	m.HandleFunc("PUT /api/v1/obs/transitions/current", a(s.obsSetTransition))
	m.HandleFunc("GET /api/v1/obs/outputs", a(s.obsOutputs))
	m.HandleFunc("POST /api/v1/obs/outputs/{output}", a(s.obsOutput))
	m.HandleFunc("POST /api/v1/obs/sources/sync", a(s.obsSync))
	m.HandleFunc("POST /api/v1/obs/cameras/{id}/source", a(s.obsEnsureSource))
	m.HandleFunc("GET /api/v1/obs/audio", a(s.obsAudio))
	m.HandleFunc("PATCH /api/v1/obs/audio/{input}", a(s.obsSetAudio))

	m.HandleFunc("GET /api/v1/streams", a(s.streams))
	m.HandleFunc("GET /api/v1/metrics", a(s.metrics))
	m.HandleFunc("GET /api/v1/events", a(s.listEvents))
	m.HandleFunc("GET /api/v1/audit", a(s.listAudit))
	m.HandleFunc("GET /api/v1/sessions", a(s.listSessions))

	m.HandleFunc("GET /player/{id}", s.player)
	m.HandleFunc("POST /whep/{id}", s.whepOffer)
	m.HandleFunc("PATCH /whep/{id}/sessions/{sid}", s.whepPatch)
	m.HandleFunc("DELETE /whep/{id}/sessions/{sid}", s.whepDelete)

	return securityHeaders(s.adminGuard(m))
}

// ---------------------------------------------------------------------------
// Setup e autenticação

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "version": buildinfo.Version, "time": time.Now().UnixMilli()})
}

func (s *Server) setupStatus(w http.ResponseWriter, r *http.Request) {
	ok, err := s.auth.Configured(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	set := s.d.Settings.Get()
	writeJSON(w, http.StatusOK, map[string]any{
		"adminConfigured": ok, "firstRunComplete": set.FirstRunComplete, "serverName": set.ServerName,
		"version": buildinfo.Get(),
	})
}

func (s *Server) setupAdmin(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Password   string `json:"password"`
		ServerName string `json:"serverName"`
	}
	if err := decode(r, &in); err != nil {
		writeError(w, err)
		return
	}
	tok, err := s.auth.Setup(r.Context(), in.Password, clientIP(r))
	if err != nil {
		writeError(w, err)
		return
	}
	if strings.TrimSpace(in.ServerName) != "" {
		_, _ = s.d.Settings.Update(r.Context(), func(st *settings.Settings) { st.ServerName = in.ServerName })
	}
	writeJSON(w, http.StatusCreated, map[string]any{"token": tok, "expiresInSec": int(SessionTTL.Seconds())})
}

func (s *Server) setupComplete(w http.ResponseWriter, r *http.Request) {
	set, err := s.d.Settings.Update(r.Context(), func(st *settings.Settings) { st.FirstRunComplete = true })
	if err != nil {
		writeError(w, badRequest(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, set)
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decode(r, &in); err != nil {
		writeError(w, err)
		return
	}
	tok, err := s.auth.Login(r.Context(), in.Username, in.Password, clientIP(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"token": tok, "expiresInSec": int(SessionTTL.Seconds())})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	s.auth.Logout(bearer(r))
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) changePassword(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Current string `json:"current"`
		New     string `json:"new"`
	}
	if err := decode(r, &in); err != nil {
		writeError(w, err)
		return
	}
	tok, err := s.auth.ChangePassword(r.Context(), userFrom(r.Context()), in.Current, in.New, clientIP(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"token": tok})
}

func (s *Server) wsTicket(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ticket": s.auth.NewTicket(userFrom(r.Context())), "expiresInSec": int(TicketTTL.Seconds())})
}

// ws envia o snapshot e depois os eventos em tempo real ao painel.
func (s *Server) ws(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.auth.UseTicket(r.URL.Query().Get("ticket")); !ok {
		writeError(w, &Error{http.StatusUnauthorized, "unauthorized", "ticket inválido ou expirado"})
		return
	}
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: []string{"wails.localhost", "wails", "localhost:5173", "127.0.0.1:5173", "wails.localhost:34115"}})
	if err != nil {
		return
	}
	defer conn.CloseNow()
	ctx := conn.CloseRead(r.Context())
	sub := s.d.Events.Subscribe()
	defer s.d.Events.Unsubscribe(sub)
	snap, _ := jsonMessage("snapshot", s.d.Core.Snapshot())
	if err := conn.Write(ctx, websocket.MessageText, snap); err != nil {
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case b, ok := <-sub.C:
			if !ok {
				conn.Close(websocket.StatusTryAgainLater, "painel lento; reconecte")
				return
			}
			wctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			err := conn.Write(wctx, websocket.MessageText, b)
			cancel()
			if err != nil {
				return
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Sistema, rede e configurações

func (s *Server) systemStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.d.System.Status(r.Context()))
}

func (s *Server) systemLogs(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.d.System.Logs())
}

func (s *Server) restartMedia(w http.ResponseWriter, r *http.Request) {
	s.d.Media.Restart()
	_ = s.d.Store.Audit(r.Context(), userFrom(r.Context()), "system.media_restart", "", clientIP(r), true, "")
	writeJSON(w, http.StatusAccepted, map[string]any{"status": "restarting"})
}

func (s *Server) firewallStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.d.System.FirewallStatus())
}

func (s *Server) applyFirewall(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Confirm bool `json:"confirm"`
	}
	if err := decode(r, &in); err != nil {
		writeError(w, err)
		return
	}
	if !in.Confirm {
		writeError(w, &Error{http.StatusConflict, "confirmation_required", "confirme a criação das regras de firewall"})
		return
	}
	err := s.d.System.ApplyFirewall(r.Context())
	_ = s.d.Store.Audit(r.Context(), userFrom(r.Context()), "system.firewall_apply", "", clientIP(r), err == nil, errString(err))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.d.System.FirewallStatus())
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func (s *Server) autostart(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Enabled bool `json:"enabled"`
	}
	if err := decode(r, &in); err != nil {
		writeError(w, err)
		return
	}
	if err := s.d.System.SetAutostart(in.Enabled); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"enabled": in.Enabled})
}

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.d.Settings.Get())
}

func (s *Server) putSettings(w http.ResponseWriter, r *http.Request) {
	var in settings.Settings
	if err := decode(r, &in); err != nil {
		writeError(w, err)
		return
	}
	prev := s.d.Settings.Get()
	in.FirstRunComplete = prev.FirstRunComplete || in.FirstRunComplete
	out, err := s.d.Settings.Update(r.Context(), func(st *settings.Settings) { *st = in })
	if err != nil {
		writeError(w, badRequest(err.Error()))
		return
	}
	_ = s.d.Store.Audit(r.Context(), userFrom(r.Context()), "settings.update", "", clientIP(r), true, "")
	s.d.System.ApplySettings(r.Context(), out)
	s.d.Events.Publish("settings", out)
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) network(refresh bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, s.d.System.Network(r.Context(), refresh))
	}
}

// ---------------------------------------------------------------------------
// Dispositivos

func (s *Server) listDevices(w http.ResponseWriter, r *http.Request) {
	ds, err := s.d.Store.ListDevices(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	if ds == nil {
		ds = []*store.Device{}
	}
	writeJSON(w, http.StatusOK, ds)
}

func (s *Server) createPairing(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Label    string `json:"label"`
		CameraID string `json:"cameraId"`
	}
	if err := decode(r, &in); err != nil {
		writeError(w, err)
		return
	}
	p, err := s.d.Devices.CreatePairing(r.Context(), in.Label, in.CameraID)
	if err != nil {
		writeError(w, err)
		return
	}
	_ = s.d.Store.Audit(r.Context(), userFrom(r.Context()), "device.pairing_created", in.CameraID, clientIP(r), true, "")
	writeJSON(w, http.StatusCreated, map[string]any{
		"expiresAt": p.ExpiresAt, "uri": p.URI, "cameraId": p.CameraID,
		"qrPng": "data:image/png;base64," + base64.StdEncoding.EncodeToString(p.QRPNG),
	})
}

func (s *Server) renameDevice(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name string `json:"name"`
	}
	if err := decode(r, &in); err != nil {
		writeError(w, err)
		return
	}
	info := devices.DeviceInfo{Name: in.Name, Platform: "other"}
	if err := info.Validate(); err != nil {
		writeError(w, err)
		return
	}
	id := r.PathValue("id")
	if err := s.d.Store.RenameDevice(r.Context(), id, info.Name); err != nil {
		writeError(w, err)
		return
	}
	_ = s.d.Core.Reload(r.Context())
	d, err := s.d.Store.GetDevice(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) deviceAction(w http.ResponseWriter, r *http.Request) {
	id, action := r.PathValue("id"), r.PathValue("action")
	ctx := r.Context()
	d, err := s.d.Store.GetDevice(ctx, id)
	if err != nil {
		writeError(w, err)
		return
	}
	switch action {
	case "revoke":
		err = s.d.Store.SetDeviceStatus(ctx, id, store.DeviceRevoked)
	case "block":
		err = s.d.Store.SetDeviceStatus(ctx, id, store.DeviceBlocked)
	case "unblock":
		err = s.d.Store.SetDeviceStatus(ctx, id, store.DeviceActive)
	case "disconnect":
	default:
		writeError(w, &Error{http.StatusNotFound, "not_found", "ação desconhecida"})
		return
	}
	if err != nil {
		writeError(w, err)
		return
	}
	if d.CameraID != "" && action != "unblock" {
		s.d.Core.DisconnectDevice(ctx, d.CameraID, "ação do administrador: "+action)
	}
	_ = s.d.Core.Reload(ctx)
	_ = s.d.Store.Audit(ctx, userFrom(ctx), "device."+action, id, clientIP(r), true, "")
	d, _ = s.d.Store.GetDevice(ctx, id)
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) deleteDevice(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("confirm") != "true" {
		writeError(w, &Error{http.StatusConflict, "confirmation_required", "confirme a remoção do dispositivo (?confirm=true)"})
		return
	}
	id := r.PathValue("id")
	ctx := r.Context()
	d, err := s.d.Store.GetDevice(ctx, id)
	if err != nil {
		writeError(w, err)
		return
	}
	if d.CameraID != "" {
		s.d.Core.DisconnectDevice(ctx, d.CameraID, "dispositivo removido")
	}
	if err := s.d.Store.DeleteDevice(ctx, id); err != nil {
		writeError(w, err)
		return
	}
	_ = s.d.Core.Reload(ctx)
	_ = s.d.Store.Audit(ctx, userFrom(ctx), "device.delete", id, clientIP(r), true, "")
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// Câmeras

func (s *Server) profiles(w http.ResponseWriter, r *http.Request) {
	ps, err := s.d.Store.ListProfiles(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ps)
}

func (s *Server) cameras(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.d.Core.Cameras())
}

func (s *Server) camera(w http.ResponseWriter, r *http.Request) {
	v, ok := s.d.Core.Camera(r.PathValue("id"))
	if !ok {
		writeError(w, store.ErrNotFound)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) patchCamera(w http.ResponseWriter, r *http.Request) {
	var p store.CameraPatch
	if err := decode(r, &p); err != nil {
		writeError(w, err)
		return
	}
	if p.Name != nil {
		info := devices.DeviceInfo{Name: *p.Name, Platform: "other"}
		if err := info.Validate(); err != nil || strings.TrimSpace(*p.Name) == "" {
			writeError(w, badRequest("nome de câmera inválido"))
			return
		}
		*p.Name = info.Name
	}
	if p.ProfileID != nil {
		if _, err := s.d.Store.GetProfile(r.Context(), *p.ProfileID); err != nil {
			writeError(w, badRequest("perfil desconhecido"))
			return
		}
	}
	if p.Orientation != nil && *p.Orientation != store.OrientationLandscape && *p.Orientation != store.OrientationPortrait {
		writeError(w, badRequest("orientação inválida: use landscape (horizontal) ou portrait (vertical)"))
		return
	}
	cam, err := s.d.Core.UpdateCamera(r.Context(), userFrom(r.Context()), r.PathValue("id"), p)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, cam)
}

type commandInput struct {
	Name   string         `json:"name"`
	Params map[string]any `json:"params"`
	Force  bool           `json:"force"`
}

func (s *Server) runCommand(w http.ResponseWriter, r *http.Request, in commandInput) {
	res, err := s.d.Core.Command(r.Context(), userFrom(r.Context()), r.PathValue("id"), in.Name, in.Params, in.Force)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) command(w http.ResponseWriter, r *http.Request) {
	var in commandInput
	if err := decode(r, &in); err != nil {
		writeError(w, err)
		return
	}
	s.runCommand(w, r, in)
}

// commandAlias implementa os atalhos /zoom, /lens e /quality da especificação.
func (s *Server) commandAlias(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var p map[string]any
		if err := decode(r, &p); err != nil {
			writeError(w, err)
			return
		}
		force, _ := p["force"].(bool)
		delete(p, "force")
		in := commandInput{Params: p, Force: force}
		switch kind {
		case "zoom":
			in.Name = "zoom.set"
			if _, ok := p["delta"]; ok {
				in.Name = "zoom.step"
			}
		case "lens":
			in.Name = "lens.select"
		case "quality":
			in.Name = "quality.set"
		}
		s.runCommand(w, r, in)
	}
}

func (s *Server) cameraMetrics(w http.ResponseWriter, r *http.Request) {
	since, _ := strconv.ParseInt(r.URL.Query().Get("since"), 10, 64)
	if since == 0 {
		since = time.Now().Add(-time.Hour).UnixMilli()
	}
	ms, err := s.d.Store.ListMetrics(r.Context(), r.PathValue("id"), since)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ms)
}

// ---------------------------------------------------------------------------
// OBS

func (s *Server) obs() *obsbridge.Bridge { return s.d.Core.OBS() }

func (s *Server) obsStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": s.obs().Status(), "sources": s.obs().Sources()})
}

func (s *Server) obsDetect(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.d.System.DetectOBS())
}

func (s *Server) obsConfig(w http.ResponseWriter, r *http.Request) {
	c, err := s.d.System.OBSConfig(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *Server) putOBSConfig(w http.ResponseWriter, r *http.Request) {
	var in OBSConfigInput
	if err := decode(r, &in); err != nil {
		writeError(w, err)
		return
	}
	if in.Host == "" {
		in.Host = "127.0.0.1"
	}
	if in.Port <= 0 || in.Port > 65535 {
		writeError(w, badRequest("porta inválida"))
		return
	}
	if err := s.d.System.SaveOBSConfig(r.Context(), in); err != nil {
		writeError(w, err)
		return
	}
	_ = s.d.Store.Audit(r.Context(), userFrom(r.Context()), "obs.config_update", in.Host, clientIP(r), true, "")
	s.obsConfig(w, r)
}

func (s *Server) importOBSConfig(w http.ResponseWriter, r *http.Request) {
	c, err := s.d.System.ImportLocalOBSConfig(r.Context())
	if err != nil {
		writeError(w, &Error{http.StatusNotFound, "obs_config_not_found", "não foi possível ler a configuração do obs-websocket deste computador: " + err.Error()})
		return
	}
	_ = s.d.Store.Audit(r.Context(), userFrom(r.Context()), "obs.config_import", "", clientIP(r), true, "")
	writeJSON(w, http.StatusOK, c)
}

func (s *Server) obsScenes(w http.ResponseWriter, r *http.Request) {
	sc, err := s.obs().Scenes(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, sc)
}

type sceneTarget struct {
	CameraID  string `json:"cameraId"`
	SceneName string `json:"sceneName"`
	Confirm   bool   `json:"confirm"`
}

func (s *Server) obsPreview(w http.ResponseWriter, r *http.Request) {
	var in sceneTarget
	if err := decode(r, &in); err != nil {
		writeError(w, err)
		return
	}
	var err error
	if in.CameraID != "" {
		err = s.obs().SendCameraToPreview(r.Context(), in.CameraID)
	} else {
		err = s.obs().SetPreviewScene(r.Context(), in.SceneName)
	}
	if err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) obsProgram(w http.ResponseWriter, r *http.Request) {
	var in sceneTarget
	if err := decode(r, &in); err != nil {
		writeError(w, err)
		return
	}
	var err error
	target := in.SceneName
	if in.CameraID != "" {
		target = in.CameraID
		err = s.obs().SendCameraToProgram(r.Context(), in.CameraID, in.Confirm)
	} else {
		err = s.obs().SetProgramScene(r.Context(), in.SceneName, in.Confirm)
	}
	_ = s.d.Store.Audit(r.Context(), userFrom(r.Context()), "obs.program", target, clientIP(r), err == nil, errString(err))
	if err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) obsTransition(w http.ResponseWriter, r *http.Request) {
	if err := s.obs().Transition(r.Context()); err != nil {
		writeError(w, err)
		return
	}
	_ = s.d.Store.Audit(r.Context(), userFrom(r.Context()), "obs.transition", "", clientIP(r), true, "")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) obsTransitions(w http.ResponseWriter, r *http.Request) {
	t, err := s.obs().Transitions(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) obsSetTransition(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name string `json:"name"`
	}
	if err := decode(r, &in); err != nil {
		writeError(w, err)
		return
	}
	if err := s.obs().SetTransition(r.Context(), in.Name); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) obsOutputs(w http.ResponseWriter, r *http.Request) {
	o, err := s.obs().Outputs(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, o)
}

func (s *Server) obsOutput(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Action  string `json:"action"`
		Confirm bool   `json:"confirm"`
	}
	if err := decode(r, &in); err != nil {
		writeError(w, err)
		return
	}
	out := obsbridge.Output(r.PathValue("output"))
	if (out != obsbridge.OutputStream && out != obsbridge.OutputRecord) || (in.Action != "start" && in.Action != "stop") {
		writeError(w, badRequest("use /outputs/stream ou /outputs/record com action start|stop"))
		return
	}
	err := s.obs().SetOutput(r.Context(), out, in.Action == "start", in.Confirm)
	_ = s.d.Store.Audit(r.Context(), userFrom(r.Context()), "obs."+string(out)+"."+in.Action, "", clientIP(r), err == nil, errString(err))
	if err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) obsSync(w http.ResponseWriter, r *http.Request) {
	s.obs().SyncCameras(r.Context(), s.d.Core.CameraRefs())
	writeJSON(w, http.StatusOK, s.obs().Sources())
}

func (s *Server) obsEnsureSource(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Recreate bool `json:"recreate"`
	}
	_ = decode(r, &in)
	id := r.PathValue("id")
	for _, ref := range s.d.Core.CameraRefs() {
		if ref.ID == id {
			m, err := s.obs().EnsureSource(r.Context(), ref, obsbridge.EnsureOptions{Recreate: in.Recreate})
			_ = s.d.Store.Audit(r.Context(), userFrom(r.Context()), "obs.ensure_source", id, clientIP(r), err == nil, errString(err))
			if err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, m)
			return
		}
	}
	writeError(w, store.ErrNotFound)
}

func (s *Server) obsAudio(w http.ResponseWriter, r *http.Request) {
	a, err := s.obs().AudioInputs(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, a)
}

func (s *Server) obsSetAudio(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Muted        *bool    `json:"muted"`
		VolumeDb     *float64 `json:"volumeDb"`
		SyncOffsetMs *int     `json:"syncOffsetMs"`
	}
	if err := decode(r, &in); err != nil {
		writeError(w, err)
		return
	}
	if in.SyncOffsetMs != nil && (*in.SyncOffsetMs < -950 || *in.SyncOffsetMs > 20000) {
		writeError(w, badRequest("atraso de áudio deve estar entre -950 e 20000 ms"))
		return
	}
	if err := s.obs().SetAudio(r.Context(), r.PathValue("input"), in.Muted, in.VolumeDb, in.SyncOffsetMs); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// Fluxos, métricas e registros

func (s *Server) streams(w http.ResponseWriter, r *http.Request) {
	paths, err := s.d.Media.API().Paths(r.Context())
	if err != nil {
		writeError(w, &Error{http.StatusServiceUnavailable, "media_unavailable", "servidor de mídia indisponível: " + err.Error()})
		return
	}
	if paths == nil {
		paths = []media.Path{}
	}
	writeJSON(w, http.StatusOK, paths)
}

func (s *Server) metrics(w http.ResponseWriter, r *http.Request) {
	connected, streaming := s.d.Core.Counts()
	out := map[string]any{"camerasConnected": connected, "camerasStreaming": streaming, "media": s.d.Media.Status()}
	if o, err := s.obs().Outputs(r.Context()); err == nil {
		out["obs"] = o
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) listEvents(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	evs, err := s.d.Store.ListEvents(r.Context(), limit)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, evs)
}

func (s *Server) listAudit(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	a, err := s.d.Store.ListAudit(r.Context(), limit)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, a)
}

func (s *Server) listSessions(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{"control": s.d.Core.Control().Sessions()}
	if st, err := s.d.Store.ListStreamSessions(r.Context(), 100); err == nil {
		out["streams"] = st
	}
	writeJSON(w, http.StatusOK, out)
}

// ---------------------------------------------------------------------------
// Player WHEP (Browser Source do OBS e prévia do painel)

func (s *Server) whepAllowed(r *http.Request) bool {
	// Mesma origem (página /player servida aqui) ou painel autenticado.
	if o := r.Header.Get("Origin"); o == "" || o == "http://"+r.Host {
		return true
	}
	_, ok := s.auth.Validate(bearer(r))
	return ok
}

func (s *Server) whepOffer(w http.ResponseWriter, r *http.Request) {
	if !s.whepAllowed(r) {
		writeError(w, &Error{http.StatusUnauthorized, "unauthorized", "não autorizado"})
		return
	}
	id := r.PathValue("id")
	cam, err := s.d.Store.GetCamera(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	body, err := readBody(r)
	if err != nil {
		writeError(w, err)
		return
	}
	resp, err := s.d.Proxy.Offer(r.Context(), whip.Read, id, cam.MediaPath(), body, "", "/whep/"+id+"/sessions/")
	if err != nil {
		writeError(w, &Error{http.StatusBadGateway, "media_error", err.Error()})
		return
	}
	resp.Write(w)
}

func (s *Server) whepPatch(w http.ResponseWriter, r *http.Request) {
	if !s.whepAllowed(r) {
		writeError(w, &Error{http.StatusUnauthorized, "unauthorized", "não autorizado"})
		return
	}
	body, err := readBody(r)
	if err != nil {
		writeError(w, err)
		return
	}
	resp, err := s.d.Proxy.Patch(r.Context(), whip.Read, r.PathValue("sid"), r.PathValue("id"), body, r.Header.Get("Content-Type"), r.Header.Get("If-Match"))
	if err != nil {
		writeError(w, err)
		return
	}
	resp.Write(w)
}

func (s *Server) whepDelete(w http.ResponseWriter, r *http.Request) {
	if !s.whepAllowed(r) {
		writeError(w, &Error{http.StatusUnauthorized, "unauthorized", "não autorizado"})
		return
	}
	st, err := s.d.Proxy.Delete(r.Context(), whip.Read, r.PathValue("sid"), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(st)
}

func readBody(r *http.Request) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r.Body, whip.MaxSDPBytes+1))
	if err != nil {
		return nil, badRequest("falha ao ler o corpo da requisição")
	}
	if len(b) > whip.MaxSDPBytes {
		return nil, badRequest("corpo muito grande")
	}
	return b, nil
}

func jsonMessage(typ string, data any) ([]byte, error) {
	return json.Marshal(events.Message{Type: typ, TS: time.Now().UnixMilli(), Data: data})
}

// selfTest executa o teste de funcionamento (somente leitura).
func (s *Server) selfTest(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.d.System.SelfTest(r.Context()))
}
