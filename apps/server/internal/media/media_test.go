package media

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func testConfig(t *testing.T) Config {
	t.Helper()
	return Config{
		Ports:       freePorts(t),
		PublishUser: "sacralink-pub", PublishPass: "0123456789abcdef0123",
		ICEListenHost: "127.0.0.1", AdditionalIPs: []string{"127.0.0.1"},
	}
}

func freePort(t *testing.T) int {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func freePorts(t *testing.T) Ports {
	return Ports{RTSP: freePort(t), WebRTC: freePort(t), ICE: freePort(t), API: freePort(t)}
}

func TestRenderRestringeAoLoopback(t *testing.T) {
	cfg := testConfig(t)
	cfg.ICEListenHost = ""
	b, err := cfg.Render()
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := yaml.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"apiAddress", "rtspAddress", "webrtcAddress"} {
		if v, _ := m[k].(string); !strings.HasPrefix(v, "127.0.0.1:") {
			t.Fatalf("%s deveria escutar só no loopback: %q", k, v)
		}
	}
	for _, k := range []string{"rtmp", "hls", "srt", "moq", "metrics", "pprof", "playback"} {
		if m[k] != false {
			t.Fatalf("%s deveria estar desabilitado: %v", k, m[k])
		}
	}
	if v, _ := m["webrtcLocalUDPAddress"].(string); !strings.HasPrefix(v, ":") {
		t.Fatalf("ICE deveria escutar em todas as interfaces: %q", v)
	}
	users := m["authInternalUsers"].([]any)
	pub := users[0].(map[string]any)
	if pub["user"] != "sacralink-pub" || len(pub["ips"].([]any)) != 2 {
		t.Fatalf("usuário de publicação inesperado: %+v", pub)
	}
	if !strings.Contains(string(b), "NÃO EDITE") {
		t.Fatal("cabeçalho de aviso ausente")
	}
}

func TestRenderExigeCredencialForte(t *testing.T) {
	cfg := testConfig(t)
	cfg.PublishPass = "curta"
	if _, err := cfg.Render(); err == nil {
		t.Fatal("deveria rejeitar senha interna fraca")
	}
}

func TestCheckPortsDetectaConflito(t *testing.T) {
	cfg := testConfig(t)
	l, err := net.Listen("tcp", hostPort("127.0.0.1", cfg.Ports.RTSP))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if err := CheckPorts(cfg); !errors.Is(err, ErrPortInUse) {
		t.Fatalf("esperava ErrPortInUse, obtive %v", err)
	}
}

func TestAPIPathsCamposV121(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"itemCount":1,"pageCount":1,"items":[{"name":"sl/cabc12345678","online":true,"available":true,
			"inboundBytes":123456,"tracks2":[{"codec":"H264","codecProps":{"width":1280,"height":720,"profile":"Baseline","level":"3.1"}},
			{"codec":"Opus","codecProps":{"channelCount":2}}],"source":{"type":"webRTCSession","id":"x"},"readers":[{"type":"rtspSession","id":"y"}]}]}`))
	}))
	defer srv.Close()
	paths, err := NewAPIWithBase(srv.URL).Paths(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	p := paths[0]
	if !p.IsOnline() || p.Bytes() != 123456 || !p.HasAudio() || p.Video().CodecProps.Width != 1280 || len(p.Readers) != 1 {
		t.Fatalf("decodificação inesperada: %+v", p)
	}
}

func TestParseLogLine(t *testing.T) {
	lvl, msg := parseLogLine("2026/11/05 16:05:00 ERR [WebRTC] falhou")
	if lvl.String() != "ERROR" || msg != "[WebRTC] falhou" {
		t.Fatalf("%v %q", lvl, msg)
	}
}

func TestVerifyChecksum(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "x.exe")
	os.WriteFile(exe, []byte("abc"), 0o600)
	if ok, err := VerifyChecksum(exe); ok || err != nil {
		t.Fatalf("sem arquivo de hash: %v %v", ok, err)
	}
	os.WriteFile(exe+".sha256", []byte("ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad  x.exe\n"), 0o600)
	if ok, err := VerifyChecksum(exe); !ok || err != nil {
		t.Fatalf("hash correto: %v %v", ok, err)
	}
	os.WriteFile(exe, []byte("abd"), 0o600)
	if _, err := VerifyChecksum(exe); err == nil {
		t.Fatal("deveria detectar binário adulterado")
	}
}

// TestSupervisorComMediaMTXReal executa o binário real (se disponível) apenas
// em portas de loopback, mata o processo e verifica o reinício automático.
func TestSupervisorComMediaMTXReal(t *testing.T) {
	exe, err := FindExecutable()
	if err != nil {
		t.Skip("mediamtx não disponível: ", err)
	}
	cfg := testConfig(t)
	var states []string
	sup, err := NewSupervisor(SupervisorOptions{
		Executable: exe, ConfigPath: filepath.Join(t.TempDir(), "mediamtx.yml"), Config: cfg,
		HealthInterval: 300 * time.Millisecond,
		OnState:        func(s Status) { states = append(states, s.State) },
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { sup.Run(ctx); close(done) }()
	defer func() { cancel(); <-done }()

	wait := func(what string, cond func() bool) {
		t.Helper()
		for deadline := time.Now().Add(15 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
			if cond() {
				return
			}
		}
		t.Fatalf("tempo esgotado: %s (status %+v)", what, sup.Status())
	}
	wait("running", func() bool { return sup.Status().State == StateRunning })
	if !strings.HasPrefix(sup.Status().Version, "v1.") {
		t.Fatalf("versão não detectada: %+v", sup.Status())
	}
	pid := sup.Status().PID
	p, _ := os.FindProcess(pid)
	p.Kill() // simula travamento/queda
	wait("reinício automático", func() bool {
		st := sup.Status()
		return st.State == StateRunning && st.PID != pid && st.Restarts >= 1
	})
	sup.Restart()
	wait("reinício solicitado", func() bool { st := sup.Status(); return st.State == StateRunning && st.Restarts >= 2 })
}
