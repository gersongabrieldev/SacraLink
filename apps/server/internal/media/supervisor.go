package media

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gersongabrieldev/sacralink/apps/server/internal/platform"
)

// Estados do supervisor.
const (
	StateStopped    = "stopped"
	StateStarting   = "starting"
	StateRunning    = "running"
	StateRestarting = "restarting"
	StateFailed     = "failed"
)

// Status é o estado público do servidor de mídia.
type Status struct {
	State     string `json:"state"`
	PID       int    `json:"pid,omitempty"`
	StartedAt int64  `json:"startedAt,omitempty"`
	Restarts  int    `json:"restarts"`
	LastError string `json:"lastError,omitempty"`
	Version   string `json:"version,omitempty"`
	Exe       string `json:"executable"`
}

// SupervisorOptions configura o supervisor.
type SupervisorOptions struct {
	Executable string
	ConfigPath string
	Config     Config
	Logger     *slog.Logger
	OnState    func(Status)
	// Intervalo do health check (padrão 5 s).
	HealthInterval time.Duration
}

// Supervisor inicia, monitora e reinicia o MediaMTX.
type Supervisor struct {
	opt     SupervisorOptions
	log     *slog.Logger
	api     *API
	job     *platform.Job
	restart chan struct{}

	mu     sync.Mutex
	status Status
	cfg    Config
	logs   []string
	// udpFallback: o SO recusou o buffer UDP ampliado; manter o padrão.
	udpFallback bool
}

// ErrPortInUse indica conflito de porta com outro programa.
var ErrPortInUse = errors.New("porta em uso por outro programa")

// NewSupervisor cria o supervisor.
func NewSupervisor(opt SupervisorOptions) (*Supervisor, error) {
	if opt.Logger == nil {
		opt.Logger = slog.Default()
	}
	if opt.HealthInterval == 0 {
		opt.HealthInterval = 5 * time.Second
	}
	job, err := platform.NewKillOnCloseJob()
	if err != nil {
		return nil, err
	}
	return &Supervisor{
		opt: opt, log: opt.Logger.With("componente", "mediamtx"),
		api: NewAPI(opt.Config.Ports.API), job: job, restart: make(chan struct{}, 1),
		status: Status{State: StateStopped, Exe: opt.Executable}, cfg: opt.Config,
	}, nil
}

// API retorna o cliente da API do MediaMTX.
func (s *Supervisor) API() *API { return s.api }

// Status retorna o estado atual.
func (s *Supervisor) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

// RecentLogs retorna as últimas linhas de log do MediaMTX.
func (s *Supervisor) RecentLogs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.logs...)
}

// Restart reinicia o MediaMTX de forma controlada.
func (s *Supervisor) Restart() {
	select {
	case s.restart <- struct{}{}:
	default:
	}
}

// UpdateConfig troca a configuração e reinicia o MediaMTX.
func (s *Supervisor) UpdateConfig(cfg Config) {
	s.mu.Lock()
	if s.udpFallback {
		cfg.UDPReadBuffer = 0
	}
	s.cfg = cfg
	s.mu.Unlock()
	s.Restart()
}

func (s *Supervisor) set(f func(*Status)) {
	s.mu.Lock()
	f(&s.status)
	st := s.status
	s.mu.Unlock()
	if s.opt.OnState != nil {
		s.opt.OnState(st)
	}
}

// Run executa o ciclo de vida até ctx ser cancelado. O MediaMTX é encerrado
// junto (inclusive se o servidor travar, via Job Object).
func (s *Supervisor) Run(ctx context.Context) {
	defer s.job.Close()
	backoff := time.Second
	for ctx.Err() == nil {
		started := time.Now()
		err := s.runOnce(ctx)
		if ctx.Err() != nil {
			s.set(func(st *Status) { st.State, st.PID = StateStopped, 0 })
			return
		}
		if errors.Is(err, errRestartRequested) {
			s.set(func(st *Status) { st.State, st.LastError, st.PID = StateRestarting, "", 0; st.Restarts++ })
			continue // reinício controlado: sem espera
		}
		if time.Since(started) > time.Minute {
			backoff = time.Second // estava estável: reinício rápido
		}
		state := StateRestarting
		msg := ""
		if err != nil {
			msg = err.Error()
			if errors.Is(err, ErrPortInUse) || errors.Is(err, os.ErrNotExist) {
				state, backoff = StateFailed, 10*time.Second
			}
			s.log.Error("servidor de mídia parou", "erro", err, "nova_tentativa_em", backoff)
		}
		s.set(func(st *Status) { st.State, st.LastError, st.PID = state, msg, 0; st.Restarts++ })
		select {
		case <-ctx.Done():
			s.set(func(st *Status) { st.State = StateStopped })
			return
		case <-time.After(backoff):
		case <-s.restart:
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

var errRestartRequested = errors.New("reinício solicitado")

func (s *Supervisor) runOnce(ctx context.Context) error {
	s.mu.Lock()
	cfg := s.cfg
	s.mu.Unlock()
	if _, err := os.Stat(s.opt.Executable); err != nil {
		return fmt.Errorf("mediamtx.exe não encontrado em %s: %w", s.opt.Executable, err)
	}
	data, err := cfg.Render()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.opt.ConfigPath), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(s.opt.ConfigPath, data, 0o600); err != nil {
		return err
	}
	if err := CheckPorts(cfg); err != nil {
		return err
	}

	cmd := exec.Command(s.opt.Executable, s.opt.ConfigPath)
	cmd.Dir = filepath.Dir(s.opt.ConfigPath)
	platform.HideWindow(cmd)
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	s.set(func(st *Status) { st.State, st.LastError = StateStarting, "" })
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("iniciar mediamtx: %w", err)
	}
	if err := s.job.Assign(cmd.Process); err != nil {
		s.log.Warn("não foi possível associar o MediaMTX ao Job Object", "erro", err)
	}
	go s.pipeLogs(stdout)
	go s.pipeLogs(stderr)
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	stop := func(reason error) error {
		_ = cmd.Process.Kill()
		<-exited
		return reason
	}

	// Aguarda a API responder.
	deadline := time.Now().Add(15 * time.Second)
	for {
		pctx, cancel := context.WithTimeout(ctx, time.Second)
		err := s.api.Ping(pctx)
		cancel()
		if err == nil {
			break
		}
		select {
		case e := <-exited:
			if cfg.UDPReadBuffer > 0 && s.logsContain("unable to set UDP read buffer size") {
				// O SO não permite o buffer pedido: volta ao padrão do sistema.
				s.log.Warn("o sistema recusou o buffer UDP ampliado; usando o padrão do sistema operacional", "pedido", cfg.UDPReadBuffer)
				s.mu.Lock()
				s.cfg.UDPReadBuffer = 0
				s.udpFallback = true
				s.mu.Unlock()
				return errRestartRequested
			}
			return fmt.Errorf("mediamtx encerrou na inicialização: %v", e)
		case <-ctx.Done():
			return stop(nil)
		case <-time.After(200 * time.Millisecond):
		}
		if time.Now().After(deadline) {
			return stop(errors.New("mediamtx não respondeu em 15 s"))
		}
	}
	s.set(func(st *Status) {
		st.State, st.PID, st.StartedAt = StateRunning, cmd.Process.Pid, time.Now().UnixMilli()
	})
	s.log.Info("servidor de mídia em execução", "pid", cmd.Process.Pid)

	tick := time.NewTicker(s.opt.HealthInterval)
	defer tick.Stop()
	failures := 0
	for {
		select {
		case e := <-exited:
			return fmt.Errorf("mediamtx encerrou inesperadamente: %v", e)
		case <-ctx.Done():
			return stop(nil)
		case <-s.restart:
			s.log.Info("reiniciando servidor de mídia a pedido do operador")
			stop(nil)
			return errRestartRequested
		case <-tick.C:
			pctx, cancel := context.WithTimeout(ctx, 2*time.Second)
			err := s.api.Ping(pctx)
			cancel()
			if err == nil {
				failures = 0
				continue
			}
			failures++
			if failures >= 3 {
				return stop(fmt.Errorf("mediamtx travado (API sem resposta 3 vezes): %v", err))
			}
		}
	}
}

func (s *Supervisor) pipeLogs(r io.Reader) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		s.mu.Lock()
		s.logs = append(s.logs, line)
		if len(s.logs) > 200 {
			s.logs = s.logs[len(s.logs)-200:]
		}
		if s.status.Version == "" {
			if i := strings.Index(line, "MediaMTX v"); i >= 0 {
				v := line[i+len("MediaMTX "):]
				if j := strings.IndexByte(v, ','); j > 0 {
					v = v[:j]
				}
				s.status.Version = v
			}
		}
		s.mu.Unlock()
		lvl, msg := parseLogLine(line)
		s.log.Log(context.Background(), lvl, msg)
	}
}

// parseLogLine interpreta "2026/11/05 16:05:00 INF [RTSP] ..." do MediaMTX.
func parseLogLine(line string) (slog.Level, string) {
	parts := strings.SplitN(line, " ", 4)
	if len(parts) < 4 {
		return slog.LevelInfo, line
	}
	switch parts[2] {
	case "ERR":
		return slog.LevelError, parts[3]
	case "WAR":
		return slog.LevelWarn, parts[3]
	case "DEB":
		return slog.LevelDebug, parts[3]
	case "INF":
		return slog.LevelInfo, parts[3]
	}
	return slog.LevelInfo, line
}

// CheckPorts verifica se as portas do MediaMTX estão livres.
func CheckPorts(cfg Config) error {
	tcp := map[string]string{
		"RTSP":            hostPort("127.0.0.1", cfg.Ports.RTSP),
		"WebRTC (HTTP)":   hostPort("127.0.0.1", cfg.Ports.WebRTC),
		"API do MediaMTX": hostPort("127.0.0.1", cfg.Ports.API),
	}
	if cfg.ICETCP {
		tcp["ICE (TCP)"] = hostPort(cfg.ICEListenHost, cfg.Ports.ICE)
	}
	for name, addr := range tcp {
		l, err := net.Listen("tcp", addr)
		if err != nil {
			return fmt.Errorf("%w: %s (%s)", ErrPortInUse, name, addr)
		}
		l.Close()
	}
	pc, err := net.ListenPacket("udp", hostPort(cfg.ICEListenHost, cfg.Ports.ICE))
	if err != nil {
		return fmt.Errorf("%w: ICE (UDP %s)", ErrPortInUse, hostPort(cfg.ICEListenHost, cfg.Ports.ICE))
	}
	pc.Close()
	return nil
}

// FindExecutable procura o mediamtx.exe: ao lado do servidor (instalação),
// em SACRALINK_MEDIAMTX, ou em third_party/bin/mediamtx (desenvolvimento).
func FindExecutable() (string, error) {
	name := "mediamtx"
	if os.PathSeparator == '\\' {
		name += ".exe"
	}
	if p := os.Getenv("SACRALINK_MEDIAMTX"); p != "" {
		return p, nil
	}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		for _, c := range []string{filepath.Join(dir, name), filepath.Join(dir, "mediamtx", name)} {
			if _, err := os.Stat(c); err == nil {
				return c, nil
			}
		}
	}
	if wd, err := os.Getwd(); err == nil {
		for dir := wd; ; {
			c := filepath.Join(dir, "third_party", "bin", "mediamtx", name)
			if _, err := os.Stat(c); err == nil {
				return c, nil
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	return "", fmt.Errorf("mediamtx não encontrado (reinstale o SacraLink ou rode scripts/fetch-mediamtx.ps1): %w", os.ErrNotExist)
}

// VerifyChecksum confere o SHA-256 do executável contra <exe>.sha256, quando
// o arquivo existir (gerado pelo build do instalador).
func VerifyChecksum(exe string) (verified bool, err error) {
	want, err := os.ReadFile(exe + ".sha256")
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	f, err := os.Open(exe)
	if err != nil {
		return false, err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return false, err
	}
	got := hex.EncodeToString(h.Sum(nil))
	expected := strings.ToLower(strings.Fields(string(want))[0])
	if got != expected {
		return false, fmt.Errorf("integridade do MediaMTX comprometida: SHA-256 %s, esperado %s", got, expected)
	}
	return true, nil
}

// logsContain informa se alguma linha recente de log contém o texto.
func (s *Supervisor) logsContain(text string) bool {
	// Dá tempo para o leitor de stdout processar as últimas linhas.
	time.Sleep(100 * time.Millisecond)
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, l := range s.logs {
		if strings.Contains(l, text) {
			return true
		}
	}
	return false
}
