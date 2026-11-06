// Package app é a raiz de composição do sacralink-server: cria e conecta
// todos os componentes e controla o ciclo de vida dos serviços.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/gersongabrieldev/sacralink/apps/server/internal/api"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/control"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/core"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/devices"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/discovery"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/events"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/media"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/netdiag"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/obsbridge"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/platform"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/secure"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/settings"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/store"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/tlsutil"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/whip"
)

// Config configura a aplicação.
type Config struct {
	DataDir    string
	AdminPort  int // 127.0.0.1
	DevicePort int // TLS
	// DeviceHost "" = todas as interfaces; "127.0.0.1" em testes.
	DeviceHost string
	MediaPorts media.Ports
	MediaExe   string
	// ICEListenHost "" = todas as interfaces; "127.0.0.1" em testes.
	ICEListenHost string
	DevOrigins    bool
	DashboardDir  string // build do painel para servir em http://127.0.0.1:7380/ (opcional)
	// MDNS habilita o anúncio na rede local (desligado nos testes).
	MDNS   bool
	Logger *slog.Logger
}

// DefaultConfig retorna a configuração padrão de produção.
func DefaultConfig() Config {
	return Config{AdminPort: 7380, DevicePort: 7443, MediaPorts: media.DefaultPorts}
}

// App é a aplicação em execução.
type App struct {
	cfg      Config
	log      *slog.Logger
	store    *store.Store
	settings *settings.Manager
	identity *tlsutil.Identity
	sup      *media.Supervisor
	proxy    *whip.Proxy
	core     *core.Core
	bridge   *obsbridge.Bridge
	ctrl     *control.Hub
	events   *events.Hub
	devices  *devices.Service
	api      *api.Server
	serverID string
	pubUser  string
	pubPass  string
	started  time.Time
	sampler  platform.StatsSampler

	netMu  sync.Mutex
	report *netdiag.Report
	reach  *netdiag.Reachability

	mdns *discovery.Announcer

	mediaMu  sync.Mutex
	mediaCfg media.Config
}

// New cria a aplicação (abre banco, certificados e componentes).
func New(ctx context.Context, cfg Config) (*App, error) {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.DataDir == "" {
		d, err := platform.DataDir()
		if err != nil {
			return nil, err
		}
		cfg.DataDir = d
	}
	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return nil, fmt.Errorf("criar diretório de dados: %w", err)
	}
	if cfg.MediaExe == "" {
		exe, err := media.FindExecutable()
		if err != nil {
			return nil, err
		}
		cfg.MediaExe = exe
	}
	if ok, err := media.VerifyChecksum(cfg.MediaExe); err != nil {
		return nil, err
	} else if !ok {
		cfg.Logger.Warn("integridade do MediaMTX não verificada (arquivo .sha256 ausente)", "exe", cfg.MediaExe)
	}

	st, err := store.Open(ctx, filepath.Join(cfg.DataDir, "sacralink.db"))
	if err != nil {
		return nil, err
	}
	a := &App{cfg: cfg, log: cfg.Logger, store: st, events: events.NewHub(), started: time.Now()}
	if err := a.init(ctx); err != nil {
		st.Close()
		return nil, err
	}
	return a, nil
}

func (a *App) init(ctx context.Context) error {
	var err error
	if a.settings, err = settings.Load(ctx, a.store); err != nil {
		return err
	}
	if a.serverID, err = settings.Secret(ctx, a.store, "server.id", func() string { return secure.NewID("s", 12) }); err != nil {
		return err
	}
	a.pubUser = "sacralink-publisher"
	if a.pubPass, err = settings.Secret(ctx, a.store, "mediamtx.publisher", secure.NewToken); err != nil {
		return err
	}
	host, _ := os.Hostname()
	if a.identity, err = tlsutil.LoadOrCreate(filepath.Join(a.cfg.DataDir, "tls"), "SacraLink "+host); err != nil {
		return err
	}

	a.mediaCfg = a.buildMediaConfig(a.settings.Get())
	if a.sup, err = media.NewSupervisor(media.SupervisorOptions{
		Executable: a.cfg.MediaExe, ConfigPath: filepath.Join(a.cfg.DataDir, "mediamtx", "mediamtx.yml"),
		Config: a.mediaCfg, Logger: a.log,
		OnState: func(s media.Status) { a.events.Publish("media", s) },
	}); err != nil {
		return err
	}
	a.proxy = whip.New("http://127.0.0.1:"+strconv.Itoa(a.cfg.MediaPorts.WebRTC), a.pubUser, a.pubPass)
	a.core = core.New(core.Options{Store: a.store, Settings: a.settings, Media: a.sup.API(), Events: a.events, Logger: a.log})
	a.ctrl = control.NewHub(a.log, a.core)
	a.core.SetControl(a.ctrl)
	a.bridge = obsbridge.New(obsbridge.Options{
		Store: a.store, Logger: a.log,
		RTSPBase:          "rtsp://127.0.0.1:" + strconv.Itoa(a.cfg.MediaPorts.RTSP) + "/",
		PlayerBase:        "http://127.0.0.1:" + strconv.Itoa(a.cfg.AdminPort) + "/player/",
		Cameras:           a.core.CameraRefs,
		AutoManage:        func() bool { return a.settings.Get().OBS.AutoManageSources },
		AllowDirectSwitch: func() bool { return a.settings.Get().OBS.AllowDirectSwitch },
		OnChange:          a.core.MarkDirtyOBS,
		OnTally:           a.core.OnTally,
		OnAudio:           a.core.OnAudio,
	})
	a.core.SetOBS(a.bridge)
	a.devices = devices.New(a.store, a.Endpoints)
	a.mdns = discovery.New(a.log)
	a.api = api.New(api.Deps{
		Store: a.store, Settings: a.settings, Core: a.core, Devices: a.devices, Proxy: a.proxy,
		Media: a.sup, Events: a.events, System: a, Logger: a.log, AdminPort: a.cfg.AdminPort, DevOrigins: a.cfg.DevOrigins,
	})
	return a.loadOBSConfig(ctx)
}

func (a *App) buildMediaConfig(s settings.Settings) media.Config {
	c := media.Config{
		Ports: a.cfg.MediaPorts, PublishUser: a.pubUser, PublishPass: a.pubPass,
		ICEListenHost: a.cfg.ICEListenHost, ICETCP: s.ICETCP, IPsFromIfaces: true,
		UDPReadBuffer: 8 << 20,
	}
	if a.cfg.ICEListenHost == "127.0.0.1" {
		c.IPsFromIfaces, c.AdditionalIPs = false, []string{"127.0.0.1"}
	}
	if s.Remote.Enabled && s.Remote.PublicHost != "" {
		c.AdditionalIPs = append(c.AdditionalIPs, s.Remote.PublicHost)
		if s.Remote.UseSTUN {
			for _, srv := range netdiag.DefaultSTUNServers {
				c.STUNServers = append(c.STUNServers, "stun:"+srv)
			}
		}
	}
	return c
}

// ApplySettings implementa api.System: reconfigura o MediaMTX somente se a
// configuração de mídia mudou (reiniciar derruba as transmissões).
func (a *App) ApplySettings(ctx context.Context, s settings.Settings) {
	a.applyMDNS(s)
	next := a.buildMediaConfig(s)
	a.mediaMu.Lock()
	changed := fmt.Sprint(next) != fmt.Sprint(a.mediaCfg)
	a.mediaCfg = next
	a.mediaMu.Unlock()
	if changed {
		a.log.Info("configuração de mídia alterada; reiniciando o MediaMTX")
		a.sup.UpdateConfig(next)
	}
}

// AdminURL retorna o endereço da API de administração.
func (a *App) AdminURL() string { return "http://127.0.0.1:" + strconv.Itoa(a.cfg.AdminPort) }

// Fingerprint retorna a impressão digital TLS (testes e diagnóstico).
func (a *App) Fingerprint() string { return a.identity.FingerprintHex }

// Core expõe o núcleo (testes).
func (a *App) Core() *core.Core { return a.core }

// Store expõe o banco (testes).
func (a *App) Store() *store.Store { return a.store }

// DataDir retorna o diretório de dados.
func (a *App) DataDir() string { return a.cfg.DataDir }

// Run inicia os serviços e bloqueia até ctx ser cancelado.
func (a *App) Run(ctx context.Context) error {
	defer a.store.Close()
	adminLn, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(a.cfg.AdminPort)))
	if err != nil {
		return fmt.Errorf("a porta %d (painel) está em uso — outro SacraLink já está aberto? %w", a.cfg.AdminPort, err)
	}
	deviceLn, err := net.Listen("tcp", net.JoinHostPort(a.cfg.DeviceHost, strconv.Itoa(a.cfg.DevicePort)))
	if err != nil {
		adminLn.Close()
		return fmt.Errorf("a porta %d (dispositivos) está em uso: %w", a.cfg.DevicePort, err)
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var wg sync.WaitGroup
	run := func(f func(context.Context)) {
		wg.Add(1)
		go func() { defer wg.Done(); f(ctx) }()
	}
	run(a.sup.Run)
	run(a.bridge.Run)
	run(a.core.Run)
	run(a.publishSystem)

	adminSrv := &http.Server{Handler: a.adminHandler(), ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 120 * time.Second,
		ErrorLog: slog.NewLogLogger(a.log.Handler(), slog.LevelDebug)}
	deviceSrv := &http.Server{Handler: a.api.DeviceHandler(), TLSConfig: a.identity.ServerConfig(),
		ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 120 * time.Second, MaxHeaderBytes: 16 << 10,
		ErrorLog: slog.NewLogLogger(a.log.Handler(), slog.LevelDebug)}
	errc := make(chan error, 2)
	go func() { errc <- adminSrv.Serve(adminLn) }()
	go func() { errc <- deviceSrv.ServeTLS(deviceLn, "", "") }()
	a.applyMDNS(a.settings.Get())
	defer a.mdns.Stop()
	a.log.Info("SacraLink em execução", "painel", a.AdminURL(), "dispositivos", deviceLn.Addr().String(), "dados", a.cfg.DataDir)

	var runErr error
	select {
	case <-ctx.Done():
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			runErr = err
		}
	}
	sctx, scancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer scancel()
	_ = adminSrv.Shutdown(sctx)
	_ = deviceSrv.Shutdown(sctx)
	cancel()
	wg.Wait()
	a.log.Info("SacraLink encerrado")
	return runErr
}

// adminHandler serve a API e, se disponível, o build do painel (navegador).
func (a *App) adminHandler() http.Handler {
	apiH := a.api.AdminHandler()
	if a.cfg.DashboardDir == "" {
		return apiH
	}
	if _, err := os.Stat(filepath.Join(a.cfg.DashboardDir, "index.html")); err != nil {
		return apiH
	}
	files := http.FileServer(http.Dir(a.cfg.DashboardDir))
	mux := http.NewServeMux()
	mux.Handle("/api/", apiH)
	mux.Handle("/player/", apiH)
	mux.Handle("/whep/", apiH)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if _, err := os.Stat(filepath.Join(a.cfg.DashboardDir, filepath.Clean(r.URL.Path))); err != nil {
			http.ServeFile(w, r, filepath.Join(a.cfg.DashboardDir, "index.html")) // SPA
			return
		}
		files.ServeHTTP(w, r)
	})
	return mux
}

// publishSystem publica métricas do sistema no painel a cada 2 s.
func (a *App) publishSystem(ctx context.Context) {
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if a.events.Count() == 0 {
				continue
			}
			a.events.Publish("system", a.systemBrief())
		}
	}
}

func (a *App) systemBrief() map[string]any {
	connected, streaming := a.core.Counts()
	var inbound float64
	for _, c := range a.core.Cameras() {
		inbound += c.BitrateKbps
	}
	return map[string]any{
		"stats": a.sampler.Sample(), "camerasConnected": connected, "camerasStreaming": streaming,
		"inboundKbps": inbound, "media": a.sup.Status(), "uptimeSec": int(time.Since(a.started).Seconds()),
		"live": a.bridge.Live(),
	}
}

// RestartMedia reinicia o servidor de mídia (bandeja e painel).
func (a *App) RestartMedia() { a.sup.Restart() }

// applyMDNS liga, atualiza ou desliga o anúncio na rede local.
func (a *App) applyMDNS(s settings.Settings) {
	if !a.cfg.MDNS || !s.MDNS {
		a.mdns.Stop()
		return
	}
	err := a.mdns.Start(discovery.Info{ServerID: a.serverID, ServerName: s.ServerName, Port: a.cfg.DevicePort, FingerprintHex: a.identity.FingerprintHex})
	if err != nil {
		a.log.Warn("não foi possível anunciar o servidor na rede local", "erro", err)
	}
}
