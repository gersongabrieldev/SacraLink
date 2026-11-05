package core

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/gersongabrieldev/sacralink/apps/server/internal/control"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/events"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/media"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/obsbridge"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/secure"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/settings"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/store"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/tally"
)

// MediaSource é o subconjunto da API do MediaMTX usado pelo núcleo.
type MediaSource interface {
	Paths(ctx context.Context) ([]media.Path, error)
	WebRTCSessions(ctx context.Context) ([]media.WebRTCSession, error)
	KickWebRTCSession(ctx context.Context, id string) error
}

// Erros do núcleo.
var (
	ErrStreamLimit   = errors.New("limite de transmissões simultâneas atingido")
	ErrNoPermission  = errors.New("o dispositivo não tem permissão para esta ação")
	ErrUnknownCamera = errors.New("câmera desconhecida")
)

// Options configura o núcleo.
type Options struct {
	Store    *store.Store
	Settings *settings.Manager
	Media    MediaSource
	Events   *events.Hub
	Logger   *slog.Logger
	// PollInterval do MediaMTX (padrão 1 s).
	PollInterval time.Duration
}

// Core é o orquestrador.
type Core struct {
	opt  Options
	log  *slog.Logger
	obs  *obsbridge.Bridge
	ctrl *control.Hub

	mu         sync.Mutex
	cams       map[string]*camState
	dirty      map[string]bool
	obsDirty   bool
	mediaUp    bool
	lastTally  map[string]tally.State
	live       bool
	fallbackAt map[string]*time.Timer
}

// New cria o núcleo. Conecte OBS e controle com SetOBS/SetControl antes de Run.
func New(opt Options) *Core {
	if opt.Logger == nil {
		opt.Logger = slog.Default()
	}
	if opt.PollInterval == 0 {
		opt.PollInterval = time.Second
	}
	return &Core{
		opt: opt, log: opt.Logger.With("componente", "nucleo"),
		cams: map[string]*camState{}, dirty: map[string]bool{}, lastTally: map[string]tally.State{},
		fallbackAt: map[string]*time.Timer{},
	}
}

// SetOBS associa a ponte do OBS.
func (c *Core) SetOBS(b *obsbridge.Bridge) { c.obs = b }

// SetControl associa o hub de controle.
func (c *Core) SetControl(h *control.Hub) { c.ctrl = h }

// OBS retorna a ponte do OBS.
func (c *Core) OBS() *obsbridge.Bridge { return c.obs }

// Control retorna o hub de controle.
func (c *Core) Control() *control.Hub { return c.ctrl }

// Reload recarrega câmeras e dispositivos do banco (após pareamento, edição etc.).
func (c *Core) Reload(ctx context.Context) error {
	cams, err := c.opt.Store.ListCameras(ctx)
	if err != nil {
		return err
	}
	devs, err := c.opt.Store.ListDevices(ctx)
	if err != nil {
		return err
	}
	byID := map[string]*store.Device{}
	for _, d := range devs {
		byID[d.ID] = d
	}
	c.mu.Lock()
	seen := map[string]bool{}
	for _, cam := range cams {
		seen[cam.ID] = true
		st := c.cams[cam.ID]
		if st == nil {
			st = &camState{state: StateOffline}
			c.cams[cam.ID] = st
		}
		st.cam = *cam
		st.device = nil
		if d := byID[cam.DeviceID]; d != nil {
			st.device = &DeviceView{ID: d.ID, Name: d.Name, Platform: d.Platform, Model: d.Model, AppVersion: d.AppVersion, Status: d.Status}
		}
		c.dirty[cam.ID] = true
	}
	for id := range c.cams {
		if !seen[id] {
			delete(c.cams, id)
		}
	}
	c.mu.Unlock()
	return nil
}

// CameraRefs retorna as câmeras no formato da ponte OBS.
func (c *Core) CameraRefs() []obsbridge.CameraRef {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]obsbridge.CameraRef, 0, len(c.cams))
	for _, s := range c.cams {
		out = append(out, refOf(s.cam))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slot < out[j].Slot })
	return out
}

func refOf(cam store.Camera) obsbridge.CameraRef {
	return obsbridge.CameraRef{ID: cam.ID, Slot: cam.Slot, Name: cam.Name, MediaPath: cam.MediaPath(),
		IngestMode: cam.IngestMode, AudioSyncOffsetMs: cam.AudioSyncOffsetMs}
}

// Cameras retorna a visão de todas as câmeras, por slot.
func (c *Core) Cameras() []CameraView {
	var sources map[string]obsbridge.SourceStatus
	if c.obs != nil {
		sources = c.obs.Sources()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]CameraView, 0, len(c.cams))
	for id, s := range c.cams {
		out = append(out, s.view(sources[id]))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slot < out[j].Slot })
	return out
}

// Camera retorna a visão de uma câmera.
func (c *Core) Camera(id string) (CameraView, bool) {
	for _, v := range c.Cameras() {
		if v.ID == id {
			return v, true
		}
	}
	return CameraView{}, false
}

// Run executa os laços do núcleo até ctx ser cancelado.
func (c *Core) Run(ctx context.Context) {
	if err := c.opt.Store.CloseDanglingSessions(ctx); err != nil {
		c.log.Warn("falha ao fechar sessões pendentes", "erro", err)
	}
	if err := c.Reload(ctx); err != nil {
		c.log.Error("falha ao carregar câmeras", "erro", err)
	}
	poll := time.NewTicker(c.opt.PollInterval)
	flush := time.NewTicker(250 * time.Millisecond)
	housekeeping := time.NewTicker(time.Hour)
	defer poll.Stop()
	defer flush.Stop()
	defer housekeeping.Stop()
	_ = c.opt.Store.ApplyRetention(ctx, store.DefaultRetention)
	for {
		select {
		case <-ctx.Done():
			c.closeAllSessions()
			return
		case <-poll.C:
			c.pollMedia(ctx)
		case <-flush.C:
			c.flush()
		case <-housekeeping.C:
			if err := c.opt.Store.ApplyRetention(ctx, store.DefaultRetention); err != nil {
				c.log.Warn("falha na retenção de registros", "erro", err)
			}
		}
	}
}

func (c *Core) closeAllSessions() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, s := range c.cams {
		if s.streamSession != "" {
			_ = c.opt.Store.EndStreamSession(context.Background(), s.streamSession, "server_stop", int64(s.lastBytes))
			s.streamSession = ""
		}
	}
}

// pollMedia lê os paths do MediaMTX e atualiza fluxos e estados.
func (c *Core) pollMedia(ctx context.Context) {
	pctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	paths, err := c.opt.Media.Paths(pctx)
	var sessions []media.WebRTCSession
	if err == nil {
		sessions, _ = c.opt.Media.WebRTCSessions(pctx)
	}
	cancel()
	now := time.Now()
	byPath := map[string]*media.Path{}
	for i := range paths {
		byPath[paths[i].Name] = &paths[i]
	}
	pubSessions := map[string]*media.WebRTCSession{}
	for i := range sessions {
		if sessions[i].State == "publish" {
			pubSessions[sessions[i].Path] = &sessions[i]
		}
	}

	var started, stopped []string
	c.mu.Lock()
	c.mediaUp = err == nil
	for id, s := range c.cams {
		p := byPath[s.cam.MediaPath()]
		prevState := s.state
		if p != nil && p.IsOnline() {
			bytes := p.Bytes()
			if !s.lastSampleAt.IsZero() && bytes >= s.lastBytes {
				dt := now.Sub(s.lastSampleAt).Seconds()
				if dt > 0 {
					s.bitrateKbps = float64(bytes-s.lastBytes) * 8 / dt / 1000
				}
			}
			if bytes > s.lastBytes || s.lastBytesAt.IsZero() && bytes > 0 {
				s.lastBytesAt = now
			}
			s.lastBytes, s.lastSampleAt, s.online = bytes, now, true
			if v := p.Video(); v != nil {
				s.codec = v.Codec
				if v.CodecProps != nil {
					s.width, s.height = v.CodecProps.Width, v.CodecProps.Height
				}
			}
			s.hasAudio = p.HasAudio()
			if ws := pubSessions[p.Name]; ws != nil {
				s.packetsLost, s.jitterMs = ws.InboundRTPPacketsLost, ws.InboundRTPPacketsJitter*1000
				if s.remoteAddr == "" {
					s.remoteAddr = ws.RemoteAddr
				}
			}
			if s.codec != "" && s.codec != "H264" {
				s.errorReason = fmt.Sprintf("codec de vídeo %s não suportado pelo OBS Media Source; use H.264", s.codec)
			} else if s.errorReason != "" && s.codec == "H264" {
				s.errorReason = ""
			}
		} else {
			s.online, s.bitrateKbps, s.lastSampleAt = false, 0, time.Time{}
			s.lastBytes = 0
		}
		s.state = deriveState(stateInput{
			now: now, errorReason: s.errorReason, controlConnected: s.controlConnected, online: s.online,
			lastBytesAt: s.lastBytesAt, negotiatingAt: s.negotiatingAt, lostAt: s.lostAt,
		})
		if s.state == StateStreaming && prevState != StateStreaming {
			s.streamingAt, s.lostAt, s.negotiatingAt = now, time.Time{}, time.Time{}
			started = append(started, id)
		}
		if prevState == StateStreaming && s.state != StateStreaming {
			s.lostAt = now
			s.state = deriveState(stateInput{now: now, errorReason: s.errorReason, controlConnected: s.controlConnected, lostAt: now})
			stopped = append(stopped, id)
		}
		if s.state != prevState {
			c.log.Debug("estado da câmera mudou", "camera", id, "de", prevState, "para", s.state, "online", s.online)
		}
		if s.state != prevState || s.online {
			c.dirty[id] = true
		}
	}
	c.mu.Unlock()

	for _, id := range started {
		c.onStreamStarted(ctx, id)
	}
	for _, id := range stopped {
		c.onStreamStopped(ctx, id)
	}
}

func (c *Core) onStreamStarted(ctx context.Context, id string) {
	c.mu.Lock()
	s := c.cams[id]
	if s == nil {
		c.mu.Unlock()
		return
	}
	sid := secure.NewID("t", 15)
	s.streamSession = sid
	name, ref, addr := s.cam.Name, refOf(s.cam), s.remoteAddr
	if t := c.fallbackAt[id]; t != nil {
		t.Stop()
		delete(c.fallbackAt, id)
	}
	c.mu.Unlock()
	_ = c.opt.Store.StartStreamSession(ctx, sid, id, addr)
	c.event(ctx, "info", "camera_streaming", id, name+" começou a transmitir")
	if c.obs != nil && c.opt.Settings.Get().OBS.AutoManageSources {
		go func() {
			if _, err := c.obs.EnsureSource(ctx, ref, obsbridge.EnsureOptions{}); err != nil &&
				!errors.Is(err, obsbridge.ErrNotConnected) && !errors.Is(err, obsbridge.ErrSourceRemovedByUser) {
				c.log.Warn("falha ao criar fonte no OBS", "camera", id, "erro", err)
			}
		}()
	}
}

func (c *Core) onStreamStopped(ctx context.Context, id string) {
	c.mu.Lock()
	s := c.cams[id]
	if s == nil {
		c.mu.Unlock()
		return
	}
	sid, bytes, name, onAir := s.streamSession, s.lastBytes, s.cam.Name, s.tally.Program
	s.streamSession = ""
	c.mu.Unlock()
	if sid != "" {
		_ = c.opt.Store.EndStreamSession(ctx, sid, "signal_lost", int64(bytes))
	}
	if onAir {
		c.event(ctx, "error", "camera_lost_on_air", id, name+" perdeu o sinal enquanto estava NO AR")
		c.scheduleFallback(id)
	} else {
		c.event(ctx, "warn", "camera_lost", id, name+" perdeu o sinal")
	}
}

// event grava e publica um evento operacional.
func (c *Core) event(ctx context.Context, level, kind, cameraID, msg string) {
	e, err := c.opt.Store.AddEvent(ctx, level, kind, cameraID, msg, nil)
	if err != nil {
		c.log.Warn("falha ao gravar evento", "erro", err)
		return
	}
	switch level {
	case "error":
		c.log.Error(msg, "camera", cameraID)
	case "warn":
		c.log.Warn(msg, "camera", cameraID)
	default:
		c.log.Info(msg, "camera", cameraID)
	}
	if c.opt.Events != nil {
		c.opt.Events.Publish("event", e)
	}
}

// flush publica no painel as câmeras alteradas (no máximo 4 vezes/s).
func (c *Core) flush() {
	c.mu.Lock()
	ids := make([]string, 0, len(c.dirty))
	for id := range c.dirty {
		ids = append(ids, id)
	}
	c.dirty = map[string]bool{}
	obsDirty := c.obsDirty
	c.obsDirty = false
	c.mu.Unlock()
	if c.opt.Events == nil {
		return
	}
	if len(ids) > 0 {
		views := c.Cameras()
		want := map[string]bool{}
		for _, id := range ids {
			want[id] = true
		}
		for _, v := range views {
			if want[v.ID] {
				c.opt.Events.Publish("camera", v)
			}
		}
	}
	if obsDirty && c.obs != nil {
		c.opt.Events.Publish("obs", c.obs.Status())
	}
}

// MarkDirtyOBS sinaliza mudança no estado do OBS (callback da ponte).
func (c *Core) MarkDirtyOBS() {
	c.mu.Lock()
	c.obsDirty = true
	for id := range c.cams {
		c.dirty[id] = true // estado da fonte pode ter mudado
	}
	c.mu.Unlock()
}

// OnTally recebe o tally da ponte OBS e o envia aos celulares.
func (c *Core) OnTally(m map[string]tally.State, live bool) {
	obsConnected := c.obs != nil && c.obs.Status().State == obsbridge.StateConnected
	c.mu.Lock()
	c.live = live
	type send struct {
		id string
		t  control.Tally
	}
	var out []send
	for id, s := range c.cams {
		st, ok := m[id]
		if !ok {
			st = tally.State{Known: obsConnected}
		}
		if s.tally != st {
			c.dirty[id] = true
		}
		s.tally = st
		out = append(out, send{id, deviceTally(st, live, obsConnected)})
	}
	c.mu.Unlock()
	if c.ctrl != nil {
		for _, o := range out {
			c.ctrl.SendTally(o.id, o.t)
		}
	}
}

// OnAudio repassa níveis de áudio ao painel.
func (c *Core) OnAudio(levels []obsbridge.AudioLevel) {
	if c.opt.Events != nil {
		c.opt.Events.Publish("audio", levels)
	}
}

// CanPublish verifica o limite de transmissões simultâneas e marca a câmera
// como em negociação.
func (c *Core) CanPublish(cameraID string) error {
	max := c.opt.Settings.Get().MaxStreams
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.cams[cameraID]
	if s == nil {
		return ErrUnknownCamera
	}
	active := 0
	for id, o := range c.cams {
		if id == cameraID {
			continue
		}
		if o.state == StateStreaming || o.state == StateConnecting && o.online {
			active++
		}
	}
	if active >= max {
		return fmt.Errorf("%w (%d)", ErrStreamLimit, max)
	}
	s.negotiatingAt = time.Now()
	c.dirty[cameraID] = true
	return nil
}

// Snapshot é o estado completo enviado ao painel ao conectar.
type Snapshot struct {
	Cameras  []CameraView      `json:"cameras"`
	OBS      obsbridge.Status  `json:"obs"`
	MediaUp  bool              `json:"mediaUp"`
	Settings settings.Settings `json:"settings"`
}

// Snapshot retorna o estado completo.
func (c *Core) Snapshot() Snapshot {
	s := Snapshot{Cameras: c.Cameras(), Settings: c.opt.Settings.Get()}
	if c.obs != nil {
		s.OBS = c.obs.Status()
	}
	c.mu.Lock()
	s.MediaUp = c.mediaUp
	c.mu.Unlock()
	return s
}

// Counts retorna (câmeras conectadas, câmeras transmitindo).
func (c *Core) Counts() (connected, streaming int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, s := range c.cams {
		if s.controlConnected || s.state == StateStreaming {
			connected++
		}
		if s.state == StateStreaming {
			streaming++
		}
	}
	return
}
