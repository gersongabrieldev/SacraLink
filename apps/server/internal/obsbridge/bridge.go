// Package obsbridge integra o SacraLink ao OBS Studio: mantém a conexão com o
// obs-websocket, espelha o grafo de cenas, calcula o tally de cada câmera,
// gerencia as fontes criadas pelo SacraLink e executa as operações de cena,
// transmissão e áudio com as salvaguardas da especificação (§6).
package obsbridge

import (
	"context"
	"errors"
	"log/slog"
	"math/rand/v2"
	"strconv"
	"sync"
	"time"

	"github.com/gersongabrieldev/sacralink/apps/server/internal/obs"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/store"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/tally"
)

// Estados da conexão com o OBS.
const (
	StateDisabled     = "disabled"
	StateDisconnected = "disconnected"
	StateConnecting   = "connecting"
	StateConnected    = "connected"
	StateAuthFailed   = "auth_failed"
)

// Erros das operações.
var (
	ErrNotConnected          = errors.New("o OBS não está conectado")
	ErrStudioModeOff         = errors.New("o Studio Mode do OBS está desligado: não há Preview")
	ErrConfirmationRequired  = errors.New("confirmação necessária: com o Studio Mode desligado a troca vai direto ao ar")
	ErrNoSource              = errors.New("a câmera ainda não tem fonte no OBS")
	ErrSourceRemovedByUser   = errors.New("a fonte desta câmera foi removida no OBS; use \"Recriar fonte\" para criá-la novamente")
	ErrIngestModeMismatch    = errors.New("a fonte existente usa outro modo de ingestão; confirme a recriação da fonte")
	ErrUnknownCamera         = errors.New("câmera desconhecida")
	ErrOutputConfirmRequired = errors.New("confirmação necessária para iniciar/parar transmissão ou gravação")
)

// CameraRef descreve uma câmera para fins de fonte no OBS.
type CameraRef struct {
	ID                string
	Slot              int
	Name              string
	MediaPath         string // ex.: sl/c4k2...
	IngestMode        string // rtsp | whep
	AudioSyncOffsetMs int
}

// Config configura a conexão com o OBS.
type Config struct {
	Host        string
	Port        int
	Password    string
	AutoConnect bool
}

// Options configura a ponte.
type Options struct {
	Store  *store.Store
	Logger *slog.Logger
	// Base das URLs de ingestão (loopback).
	RTSPBase   string // ex.: rtsp://127.0.0.1:8554/
	PlayerBase string // ex.: http://127.0.0.1:7380/player/
	// Cameras retorna as câmeras atuais (usado na sincronização automática).
	Cameras func() []CameraRef
	// AutoManage informa se o operador autorizou criar fontes automaticamente.
	AutoManage func() bool
	// AllowDirectSwitch permite trocas diretas no Program sem Studio Mode.
	AllowDirectSwitch func() bool
	// Callbacks (chamados fora do lock; não devem bloquear por muito tempo).
	OnChange func()
	OnTally  func(map[string]tally.State, bool)
	OnAudio  func([]AudioLevel)
}

// Status é o estado público da integração.
type Status struct {
	State          string `json:"state"`
	Error          string `json:"error,omitempty"`
	Address        string `json:"address"`
	OBSVersion     string `json:"obsVersion,omitempty"`
	WSVersion      string `json:"wsVersion,omitempty"`
	ConnectedAt    int64  `json:"connectedAt,omitempty"`
	StudioMode     bool   `json:"studioMode"`
	ProgramScene   string `json:"programScene,omitempty"`
	PreviewScene   string `json:"previewScene,omitempty"`
	Streaming      bool   `json:"streaming"`
	Recording      bool   `json:"recording"`
	StreamStarted  int64  `json:"streamStartedAt,omitempty"`
	RecordStarted  int64  `json:"recordStartedAt,omitempty"`
	CanvasWidth    int    `json:"canvasWidth,omitempty"`
	CanvasHeight   int    `json:"canvasHeight,omitempty"`
	LastSyncAt     int64  `json:"lastSyncAt,omitempty"`
	ReconnectInSec int    `json:"reconnectInSec,omitempty"`
}

// SourceStatus é o estado da fonte de uma câmera no OBS.
type SourceStatus struct {
	CameraID  string `json:"cameraId"`
	InputName string `json:"inputName,omitempty"`
	SceneName string `json:"sceneName,omitempty"`
	InputKind string `json:"inputKind,omitempty"`
	// present | missing | none
	State string `json:"state"`
}

// Bridge é a integração com o OBS.
type Bridge struct {
	opt Options
	log *slog.Logger

	mu        sync.Mutex
	cfg       Config
	client    *obs.Client
	status    Status
	graph     *tally.Graph
	groups    map[string]bool
	active    map[string]bool
	mappings  map[string]*store.SourceMapping // cameraID → mapeamento
	present   map[string]bool                 // cameraID → fonte existe no OBS
	tallies   map[string]tally.State
	reconfig  chan struct{}
	pending   map[string]bool // cenas a ressincronizar
	pendingT  *time.Timer
	lastAudio time.Time
}

// New cria a ponte. Chame Run para iniciar a conexão.
func New(opt Options) *Bridge {
	if opt.Logger == nil {
		opt.Logger = slog.Default()
	}
	if opt.Cameras == nil {
		opt.Cameras = func() []CameraRef { return nil }
	}
	if opt.AutoManage == nil {
		opt.AutoManage = func() bool { return false }
	}
	if opt.AllowDirectSwitch == nil {
		opt.AllowDirectSwitch = func() bool { return false }
	}
	return &Bridge{
		opt: opt, log: opt.Logger.With("componente", "obs"),
		status:   Status{State: StateDisabled},
		graph:    tally.NewGraph(),
		groups:   map[string]bool{},
		active:   map[string]bool{},
		mappings: map[string]*store.SourceMapping{},
		present:  map[string]bool{},
		tallies:  map[string]tally.State{},
		reconfig: make(chan struct{}, 1),
		pending:  map[string]bool{},
	}
}

// Configure aplica uma nova configuração e força reconexão.
func (b *Bridge) Configure(cfg Config) {
	b.mu.Lock()
	b.cfg = cfg
	c := b.client
	b.mu.Unlock()
	if c != nil {
		c.Close()
	}
	select {
	case b.reconfig <- struct{}{}:
	default:
	}
}

// Status retorna o estado atual.
func (b *Bridge) Status() Status {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.status
}

// Tallies retorna o tally por câmera.
func (b *Bridge) Tallies() map[string]tally.State {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make(map[string]tally.State, len(b.tallies))
	for k, v := range b.tallies {
		out[k] = v
	}
	return out
}

// Live informa se o OBS está transmitindo ou gravando.
func (b *Bridge) Live() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.status.State == StateConnected && (b.status.Streaming || b.status.Recording)
}

// Sources retorna o estado das fontes das câmeras conhecidas.
func (b *Bridge) Sources() map[string]SourceStatus {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := map[string]SourceStatus{}
	for id, m := range b.mappings {
		st := "missing"
		if b.present[id] {
			st = "present"
		}
		if b.status.State != StateConnected {
			st = "unknown"
		}
		out[id] = SourceStatus{CameraID: id, InputName: m.InputName, SceneName: m.SceneName, InputKind: m.InputKind, State: st}
	}
	return out
}

func (b *Bridge) currentClient() *obs.Client {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.status.State != StateConnected {
		return nil
	}
	return b.client
}

func (b *Bridge) notifyChange() {
	if b.opt.OnChange != nil {
		b.opt.OnChange()
	}
}

// Run mantém a conexão com o OBS até ctx ser cancelado.
func (b *Bridge) Run(ctx context.Context) {
	backoff := time.Second
	for ctx.Err() == nil {
		b.mu.Lock()
		cfg := b.cfg
		b.mu.Unlock()
		if !cfg.AutoConnect || cfg.Host == "" {
			b.setState(StateDisabled, "", 0)
			if !b.wait(ctx, 0) {
				return
			}
			continue
		}
		// A configuração atual já foi lida: descarta sinais antigos para que
		// apenas reconfigurações feitas DURANTE a sessão encurtem a espera.
		select {
		case <-b.reconfig:
		default:
		}
		b.setState(StateConnecting, "", 0)
		err := b.session(ctx, cfg)
		if ctx.Err() != nil {
			return
		}
		state, msg := StateDisconnected, ""
		if err != nil {
			msg = err.Error()
		}
		if errors.Is(err, obs.ErrAuthFailed) || errors.Is(err, obs.ErrAuthRequired) {
			state, backoff = StateAuthFailed, 30*time.Second
		}
		if b.wasStable() {
			backoff = time.Second
		}
		jitter := time.Duration(rand.Int64N(int64(backoff / 4)))
		delay := backoff + jitter
		b.setState(state, msg, int(delay.Seconds()))
		if !b.wait(ctx, delay) {
			return
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

func (b *Bridge) wasStable() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.status.ConnectedAt > 0 && time.Since(time.UnixMilli(b.status.ConnectedAt)) > 10*time.Second
}

// wait aguarda delay (0 = indefinidamente) ou reconfiguração.
func (b *Bridge) wait(ctx context.Context, delay time.Duration) bool {
	var tc <-chan time.Time
	if delay > 0 {
		t := time.NewTimer(delay)
		defer t.Stop()
		tc = t.C
	}
	select {
	case <-ctx.Done():
		return false
	case <-b.reconfig:
		return true
	case <-tc:
		return true
	}
}

func (b *Bridge) setState(state, msg string, retry int) {
	b.mu.Lock()
	changed := b.status.State != state || b.status.Error != msg
	b.status.State, b.status.Error, b.status.ReconnectInSec = state, msg, retry
	b.status.Address = addr(b.cfg)
	if state != StateConnected {
		b.client = nil
		b.status.ConnectedAt = 0
	}
	var tallies map[string]tally.State
	if state != StateConnected {
		// OBS fora: tally desconhecido (nunca exibir NO AR desatualizado).
		tallies = b.recomputeLocked()
	}
	b.mu.Unlock()
	if tallies != nil && b.opt.OnTally != nil {
		b.opt.OnTally(tallies, false)
	}
	if changed {
		if state == StateAuthFailed {
			b.log.Warn("falha de autenticação no obs-websocket", "erro", msg)
		}
		b.notifyChange()
	}
}

func addr(c Config) string {
	if c.Host == "" {
		return ""
	}
	return "ws://" + c.Host + ":" + strconv.Itoa(c.Port)
}

// session executa uma conexão completa: handshake, sincronização e eventos.
func (b *Bridge) session(ctx context.Context, cfg Config) error {
	dctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	c, err := obs.Dial(dctx, obs.Options{
		URL: addr(cfg), Password: cfg.Password, Logger: b.log,
		Subscriptions: obs.SubAll | obs.SubInputVolumeMeters | obs.SubInputActiveStateChanged,
	})
	cancel()
	if err != nil {
		return err
	}
	defer c.Close()

	if err := b.fullSync(ctx, c); err != nil {
		return err
	}
	b.mu.Lock()
	b.client = c
	b.status.State, b.status.Error, b.status.ReconnectInSec = StateConnected, "", 0
	b.status.ConnectedAt = time.Now().UnixMilli()
	tallies := b.recomputeLocked()
	b.mu.Unlock()
	b.log.Info("conectado ao OBS", "obs", b.Status().OBSVersion, "websocket", b.Status().WSVersion)
	if b.opt.OnTally != nil {
		b.opt.OnTally(tallies, b.Live())
	}
	b.notifyChange()

	if b.opt.AutoManage() {
		go b.SyncCameras(ctx, b.opt.Cameras())
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-b.reconfig:
			// Configuração alterada (inclusive durante o handshake): reconecta.
			return nil
		case ev, ok := <-c.Events():
			if !ok {
				return c.Err()
			}
			b.handleEvent(ctx, c, ev)
		}
	}
}
