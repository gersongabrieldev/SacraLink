package app_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/gersongabrieldev/sacralink/apps/server/internal/app"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/media"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/obs/obstest"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/testpub"
)

// TestCameraNoNavegador valida a câmera pelo navegador com um Chromium real
// (Edge ou Chrome em modo headless) usando a câmera e o microfone simulados
// do próprio navegador — o mesmo motor WebRTC do Chrome do Android:
//
//	link do QR → pareamento na página → controle por ticket → WHIP →
//	MediaMTX → RTSP (o que o OBS consome) → NO AR na página → comandos
//	remotos confirmados → prioridade local → revogação.
//
// Opcional (abre um navegador): SACRALINK_BROWSER_TEST=1. SACRALINK_BROWSER
// aponta um executável específico.
func TestCameraNoNavegador(t *testing.T) {
	if os.Getenv("SACRALINK_BROWSER_TEST") != "1" {
		t.Skip("defina SACRALINK_BROWSER_TEST=1 para testar a câmera com um navegador real")
	}
	browser := findBrowser()
	if browser == "" {
		t.Fatal("navegador Chromium (Edge/Chrome) não encontrado; defina SACRALINK_BROWSER")
	}
	exe, err := media.FindExecutable()
	if err != nil {
		t.Fatal("MediaMTX não disponível: ", err)
	}

	obsSrv := obstest.New("senha-obs")
	defer obsSrv.Close()
	cfg := app.Config{
		DataDir: t.TempDir(), AdminPort: freePort(t), DevicePort: freePort(t), DeviceHost: "127.0.0.1",
		MediaPorts:    media.Ports{RTSP: freePort(t), WebRTC: freePort(t), ICE: freePort(t), API: freePort(t)},
		MediaExe:      exe,
		ICEListenHost: "127.0.0.1",
		Logger:        slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})),
	}
	ctx, cancel := context.WithCancel(context.Background())
	a, err := app.New(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- a.Run(ctx) }()
	defer func() { cancel(); <-done }()

	admin := &adminClient{t: t, base: a.AdminURL()}
	waitFor(t, "API de administração", 10*time.Second, func() bool {
		resp, err := http.Get(admin.base + "/api/v1/health")
		if err == nil {
			resp.Body.Close()
		}
		return err == nil && resp.StatusCode == 200
	})
	var tok struct{ Token string }
	if st := admin.do("POST", "/api/v1/setup/admin", map[string]string{"password": "senha-admin-123", "serverName": "Paróquia Navegador"}, &tok); st != 201 {
		t.Fatalf("setup: %d", st)
	}
	admin.token = tok.Token
	var set map[string]any
	admin.do("GET", "/api/v1/settings", nil, &set)
	set["obs"].(map[string]any)["autoManageSources"] = true
	admin.do("PUT", "/api/v1/settings", set, nil)
	u, _ := url.Parse(obsSrv.URL())
	obsPort, _ := strconv.Atoi(u.Port())
	admin.do("PUT", "/api/v1/obs/config", map[string]any{"host": "127.0.0.1", "port": obsPort, "password": "senha-obs", "autoConnect": true}, nil)
	waitFor(t, "servidor de mídia em execução", 20*time.Second, func() bool {
		var s struct{ Media struct{ State string } }
		admin.do("GET", "/api/v1/system/status", nil, &s)
		return s.Media.State == "running"
	})

	var pairing struct{ URI string }
	if st := admin.do("POST", "/api/v1/devices/pair", map[string]string{}, &pairing); st != 201 {
		t.Fatalf("pairing: %d", st)
	}
	link, err := url.Parse(pairing.URI)
	if err != nil || link.Scheme != "https" || link.Path != "/cam/" || link.Port() != strconv.Itoa(cfg.DevicePort) {
		t.Fatalf("o QR deveria abrir a câmera no navegador: %s", pairing.URI)
	}
	// O QR traz os IPs da rede local; no teste o servidor escuta só em 127.0.0.1.
	link.Host = fmt.Sprintf("127.0.0.1:%d", cfg.DevicePort)

	page := launchBrowser(t, ctx, browser, link.String())
	js := func(expr string, out any) {
		t.Helper()
		if err := page.eval(ctx, expr, out); err != nil {
			t.Fatalf("%s: %v", expr, err)
		}
	}
	waitJS := func(what string, timeout time.Duration, cond string) {
		t.Helper()
		deadline := time.Now().Add(timeout)
		for {
			var ok bool
			if err := page.eval(ctx, cond, &ok); err == nil && ok {
				return
			}
			if time.Now().After(deadline) {
				var dump string
				_ = page.eval(ctx, `JSON.stringify({screen: document.body.dataset.screen, tally: document.body.dataset.tally,
					stream: document.body.dataset.stream, control: document.body.dataset.control,
					chips: document.getElementById('chips').innerText, notices: document.getElementById('notices').innerText,
					message: document.getElementById('message-text').innerText, pair: document.getElementById('pair-status').innerText})`, &dump)
				t.Fatalf("tempo esgotado: %s\npágina: %s", what, dump)
			}
			time.Sleep(150 * time.Millisecond)
		}
	}

	// 1) O link abre a tela de pareamento e o token sai da barra de endereço.
	waitJS("tela de pareamento", 15*time.Second, `document.body.dataset.screen === 'pair'`)
	page.screenshot(t, ctx, "camera-navegador-pareamento")
	var hash string
	js(`location.hash`, &hash)
	if hash != "" {
		t.Fatalf("o token deveria sair do histórico: %q", hash)
	}
	js(`document.getElementById('pair-name').value = 'Navegador de teste'; document.getElementById('pair-btn').click(); true`, nil)

	// 2) Câmera aberta e canal de controle conectado (ticket de uso único).
	waitJS("câmera aberta e controle conectado", 20*time.Second,
		`document.body.dataset.screen === 'camera' && document.body.dataset.control === 'on'`)
	var creds struct{ DeviceID, CameraID string }
	js(`JSON.parse(localStorage.getItem('sacralink.cam.v1'))`, &creds)
	if creds.CameraID == "" {
		t.Fatal("credenciais não salvas no navegador")
	}
	camPath := "/api/v1/cameras/" + creds.CameraID
	var cam struct {
		State, Tally, VideoCodec string
		HasAudio                 bool
		BitrateKbps              float64
		Width, Height            int
		Device                   struct{ Name, Model, AppVersion string }
	}
	admin.do("GET", camPath, nil, &cam)
	if cam.Device.Name != "Navegador de teste" || !strings.Contains(cam.Device.Model, "navegador") || !strings.HasPrefix(cam.Device.AppVersion, "web") {
		t.Fatalf("aparelho do navegador inesperado: %+v", cam.Device)
	}

	// 3) Transmissão WHIP com a câmera e o microfone simulados.
	js(`document.getElementById('start').click(); true`, nil)
	waitFor(t, "câmera STREAMING com áudio", 25*time.Second, func() bool {
		admin.do("GET", camPath, nil, &cam)
		return cam.State == "STREAMING" && cam.BitrateKbps > 0 && cam.HasAudio
	})
	waitJS("página transmitindo", 10*time.Second, `document.body.dataset.stream === 'live'`)
	if cam.VideoCodec != "H264" {
		t.Fatalf("o navegador deveria enviar H.264 (compatível com o OBS): %s", cam.VideoCodec)
	}

	// 4) O que o OBS consome: RTSP com H.264 + Opus.
	rtspURL := fmt.Sprintf("rtsp://127.0.0.1:%d/sl/%s", cfg.MediaPorts.RTSP, creds.CameraID)
	probe, err := testpub.ProbeRTSP(ctx, rtspURL, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(probe.Codecs, "H264") || !slices.ContainsFunc(probe.Codecs, func(c string) bool { return strings.EqualFold(c, "opus") }) ||
		probe.Packets[0] < 10 || probe.Packets[1] < 10 {
		t.Fatalf("RTSP inesperado: codecs=%v pacotes=%v\n%s", probe.Codecs, probe.Packets, probe.SDP)
	}
	t.Logf("navegador → RTSP OK: codecs=%v pacotes=%v bitrate=%.0f kbps %dx%d", probe.Codecs, probe.Packets, cam.BitrateKbps, cam.Width, cam.Height)

	// 5) Tally real na página: NO AR e CONECTADA.
	waitFor(t, "fonte no OBS", 10*time.Second, func() bool { return obsSrv.Input("SacraLink — Câmera 01") != nil })
	if st := admin.do("POST", "/api/v1/obs/program", map[string]any{"cameraId": creds.CameraID, "confirm": true}, nil); st != 204 {
		t.Fatalf("program: %d", st)
	}
	waitJS("página NO AR", 5*time.Second,
		`document.body.dataset.tally === 'on_air' && document.getElementById('tally').textContent.includes('NO AR')`)
	time.Sleep(2500 * time.Millisecond) // telemetria e métricas aparecem na tela
	page.screenshot(t, ctx, "camera-navegador-no-ar")
	admin.do("GET", camPath, nil, &cam)
	var sent string
	js(`document.getElementById('chips').innerText.replace(/\n/g, ' | ')`, &sent)
	t.Logf("após alguns segundos: servidor %dx%d %.0f kbps; página: %s", cam.Width, cam.Height, cam.BitrateKbps, sent)
	obsSrv.SetProgram("Cena")
	waitJS("página CONECTADA", 5*time.Second, `document.body.dataset.tally === 'connected'`)

	// 6) Comando remoto com confirmação do navegador.
	var res struct {
		Status, Message string
		State           map[string]any
	}
	cmd := func(name string, params map[string]any, force bool) {
		t.Helper()
		if st := admin.do("POST", camPath+"/commands", map[string]any{"name": name, "params": params, "force": force}, &res); st != 200 {
			t.Fatalf("%s: HTTP %d %+v", name, st, res)
		}
	}
	cmd("audio.mute", map[string]any{"muted": true}, false)
	if res.Status != "executed" || res.State["micMuted"] != true {
		t.Fatalf("mudo remoto: %+v", res)
	}
	waitJS("botão de microfone mudo", 3*time.Second, `document.getElementById('mic').classList.contains('off')`)

	// Zoom: executado se o navegador expõe zoom; senão "unsupported" (nunca finge).
	cmd("zoom.set", map[string]any{"value": 2.0}, false)
	if res.Status != "executed" && res.Status != "unsupported" {
		t.Fatalf("zoom: %+v", res)
	}
	t.Logf("zoom no navegador: %s %v", res.Status, res.State["zoom"])

	// 7) Prioridade do operador do celular por 3 s; "force" do operador central.
	js(`document.getElementById('mic').click(); true`, nil)
	cmd("audio.mute", map[string]any{"muted": true}, false)
	if res.Status != "conflict" {
		t.Fatalf("deveria haver conflito com o ajuste local: %+v", res)
	}
	cmd("audio.mute", map[string]any{"muted": true}, true)
	if res.Status != "executed" || res.State["micMuted"] != true {
		t.Fatalf("force: %+v", res)
	}

	// 8) Parar na página encerra a sessão WHIP.
	js(`document.getElementById('start').click(); true`, nil)
	waitJS("página parada", 5*time.Second, `document.body.dataset.stream === 'idle'`)
	waitFor(t, "servidor sem transmissão", 10*time.Second, func() bool {
		admin.do("GET", camPath, nil, &cam)
		return cam.State != "STREAMING"
	})

	// 9) Revogação no painel: a página apaga a credencial e pede novo QR.
	if st := admin.do("POST", "/api/v1/devices/"+creds.DeviceID+"/revoke", map[string]any{}, nil); st != 200 {
		t.Fatalf("revogar: %d", st)
	}
	waitJS("página pede novo pareamento", 20*time.Second,
		`document.body.dataset.screen === 'message' && localStorage.getItem('sacralink.cam.v1') === null`)
}

// findBrowser procura Edge ou Chrome.
func findBrowser() string {
	if p := os.Getenv("SACRALINK_BROWSER"); p != "" {
		return p
	}
	var candidates []string
	switch runtime.GOOS {
	case "windows":
		for _, base := range []string{os.Getenv("ProgramFiles(x86)"), os.Getenv("ProgramFiles"), os.Getenv("LOCALAPPDATA")} {
			if base != "" {
				candidates = append(candidates, filepath.Join(base, `Microsoft\Edge\Application\msedge.exe`), filepath.Join(base, `Google\Chrome\Application\chrome.exe`))
			}
		}
	case "darwin":
		candidates = []string{"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome", "/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge"}
	default:
		for _, n := range []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "microsoft-edge"} {
			if p, err := exec.LookPath(n); err == nil {
				candidates = append(candidates, p)
			}
		}
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c
		}
	}
	return ""
}

// cdpPage é um cliente mínimo do Chrome DevTools Protocol (Runtime.evaluate).
type cdpPage struct {
	c    *websocket.Conn
	next int
}

func launchBrowser(t *testing.T, ctx context.Context, browser, startURL string) *cdpPage {
	t.Helper()
	dir, err := os.MkdirTemp("", "sacralink-navegador-")
	if err != nil {
		t.Fatal(err)
	}
	args := []string{
		"--headless=new", "--no-first-run", "--no-default-browser-check", "--disable-extensions", "--disable-sync",
		"--use-fake-device-for-media-stream", "--use-fake-ui-for-media-stream", // câmera/microfone simulados e permissão concedida
		"--ignore-certificate-errors",         // equivale a aceitar o aviso do certificado autoassinado no celular
		"--allow-loopback-in-peer-connection", // ICE do MediaMTX em 127.0.0.1 no teste
		"--autoplay-policy=no-user-gesture-required",
		"--remote-debugging-port=0", "--user-data-dir=" + dir, "--window-size=915,412", "--force-device-scale-factor=2", // celular na horizontal
	}
	if runtime.GOOS == "linux" {
		args = append(args, "--no-sandbox", "--disable-dev-shm-usage") // contêiner do CI
	}
	cmd := exec.Command(browser, append(args, startURL)...)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		for range 20 { // processos filhos liberam o perfil em instantes
			if os.RemoveAll(dir) == nil {
				return
			}
			time.Sleep(250 * time.Millisecond)
		}
	})

	var port string
	waitFor(t, "porta do DevTools", 20*time.Second, func() bool {
		b, err := os.ReadFile(filepath.Join(dir, "DevToolsActivePort"))
		if err != nil {
			return false
		}
		port, _, _ = strings.Cut(string(b), "\n")
		port = strings.TrimSpace(port)
		return port != ""
	})
	var wsURL string
	waitFor(t, "aba da câmera", 15*time.Second, func() bool {
		resp, err := http.Get("http://127.0.0.1:" + port + "/json/list")
		if err != nil {
			return false
		}
		defer resp.Body.Close()
		var targets []struct{ Type, URL, WebSocketDebuggerURL string }
		if json.NewDecoder(resp.Body).Decode(&targets) != nil {
			return false
		}
		for _, tg := range targets {
			if tg.Type == "page" && strings.Contains(tg.URL, "/cam/") && tg.WebSocketDebuggerURL != "" {
				wsURL = tg.WebSocketDebuggerURL
				return true
			}
		}
		return false
	})
	c, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	c.SetReadLimit(8 << 20)
	t.Cleanup(func() { c.CloseNow() })
	return &cdpPage{c: c}
}

// call envia um método do DevTools e devolve o "result" bruto.
func (p *cdpPage) call(ctx context.Context, method string, params map[string]any) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	p.next++
	id := p.next
	msg, _ := json.Marshal(map[string]any{"id": id, "method": method, "params": params})
	if err := p.c.Write(ctx, websocket.MessageText, msg); err != nil {
		return nil, err
	}
	for {
		_, data, err := p.c.Read(ctx)
		if err != nil {
			return nil, err
		}
		var r struct {
			ID     int             `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(data, &r) != nil || r.ID != id {
			continue // eventos e respostas antigas
		}
		if r.Error != nil {
			return nil, errors.New(r.Error.Message)
		}
		return r.Result, nil
	}
}

func (p *cdpPage) eval(ctx context.Context, expr string, out any) error {
	raw, err := p.call(ctx, "Runtime.evaluate", map[string]any{"expression": expr, "awaitPromise": true, "returnByValue": true})
	if err != nil {
		return err
	}
	var r struct {
		Result struct {
			Value json.RawMessage `json:"value"`
		} `json:"result"`
		ExceptionDetails *struct {
			Text      string `json:"text"`
			Exception struct {
				Description string `json:"description"`
			} `json:"exception"`
		} `json:"exceptionDetails"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return err
	}
	if ex := r.ExceptionDetails; ex != nil {
		return fmt.Errorf("exceção na página: %s %s", ex.Text, ex.Exception.Description)
	}
	if out != nil && len(r.Result.Value) > 0 {
		return json.Unmarshal(r.Result.Value, out)
	}
	return nil
}

// screenshot salva a página em PNG quando SACRALINK_BROWSER_SHOTS aponta uma
// pasta (conferência visual e imagens da documentação).
func (p *cdpPage) screenshot(t *testing.T, ctx context.Context, name string) {
	t.Helper()
	dir := os.Getenv("SACRALINK_BROWSER_SHOTS")
	if dir == "" {
		return
	}
	raw, err := p.call(ctx, "Page.captureScreenshot", map[string]any{"format": "png"})
	if err != nil {
		t.Logf("captura %s: %v", name, err)
		return
	}
	var r struct{ Data []byte }
	if err := json.Unmarshal(raw, &r); err != nil {
		t.Logf("captura %s: %v", name, err)
		return
	}
	path := filepath.Join(dir, name+".png")
	if err := os.WriteFile(path, r.Data, 0o644); err != nil {
		t.Logf("captura %s: %v", name, err)
		return
	}
	t.Logf("captura salva: %s", path)
}
