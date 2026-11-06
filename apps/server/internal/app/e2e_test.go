package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gersongabrieldev/sacralink/apps/server/internal/app"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/media"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/obs/obstest"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/testpub"
)

func freePort(t *testing.T) int {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

type adminClient struct {
	t     *testing.T
	base  string
	token string
}

func (c *adminClient) do(method, path string, in any, out any) int {
	c.t.Helper()
	var body io.Reader
	if in != nil {
		b, _ := json.Marshal(in)
		body = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, c.base+path, body)
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if out != nil && len(b) > 0 {
		if err := json.Unmarshal(b, out); err != nil {
			c.t.Fatalf("%s %s: resposta inválida %s", method, path, b)
		}
	}
	return resp.StatusCode
}

func waitFor(t *testing.T, what string, timeout time.Duration, cond func() bool) {
	t.Helper()
	for deadline := time.Now().Add(timeout); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		if cond() {
			return
		}
	}
	t.Fatalf("tempo esgotado: %s", what)
}

// TestPontaAPonta valida o fluxo real: pareamento → controle → WHIP → MediaMTX
// → RTSP (o que a Media Source do OBS consome) → fonte no OBS → tally NO AR no
// celular → comando de zoom com confirmação.
func TestPontaAPonta(t *testing.T) {
	exe, err := media.FindExecutable()
	if err != nil {
		t.Skip("MediaMTX não disponível (rode scripts/fetch-mediamtx.ps1): ", err)
	}
	if testing.Short() {
		t.Skip("teste ponta a ponta")
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

	// 1) Primeiro acesso: senha do administrador.
	var tok struct{ Token string }
	if st := admin.do("POST", "/api/v1/setup/admin", map[string]string{"password": "senha-admin-123", "serverName": "Paróquia Teste"}, &tok); st != 201 {
		t.Fatalf("setup: %d", st)
	}
	admin.token = tok.Token
	if st := admin.do("POST", "/api/v1/setup/admin", map[string]string{"password": "outra-senha-123"}, nil); st != 409 {
		t.Fatalf("setup repetido deveria falhar: %d", st)
	}

	// 2) Consentimento para gerenciar fontes e conexão com o OBS.
	var set map[string]any
	admin.do("GET", "/api/v1/settings", nil, &set)
	set["obs"].(map[string]any)["autoManageSources"] = true
	if st := admin.do("PUT", "/api/v1/settings", set, nil); st != 200 {
		t.Fatalf("settings: %d", st)
	}
	u, _ := url.Parse(obsSrv.URL())
	obsPort, _ := strconv.Atoi(u.Port())
	pw := "senha-obs"
	if st := admin.do("PUT", "/api/v1/obs/config", map[string]any{"host": "127.0.0.1", "port": obsPort, "password": pw, "autoConnect": true}, nil); st != 200 {
		t.Fatalf("obs config: %d", st)
	}
	waitFor(t, "OBS conectado", 10*time.Second, func() bool {
		var s struct{ Status struct{ State string } }
		admin.do("GET", "/api/v1/obs/status", nil, &s)
		return s.Status.State == "connected"
	})

	// 3) Pareamento pelo QR Code.
	var pairing struct{ URI, QrPng string }
	if st := admin.do("POST", "/api/v1/devices/pair", map[string]string{}, &pairing); st != 201 || !strings.HasPrefix(pairing.QrPng, "data:image/png;base64,") {
		t.Fatalf("pairing: %d", st)
	}
	creds, err := testpub.Pair(ctx, pairing.URI, "Celular de teste", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if creds.Fingerprint != a.Fingerprint() {
		t.Fatal("fingerprint do QR difere do certificado")
	}

	// 4) Canal de controle.
	ctl, err := testpub.Connect(ctx, creds)
	if err != nil {
		t.Fatal(err)
	}
	defer ctl.Close()
	waitFor(t, "hello_ack", 5*time.Second, func() bool { return ctl.HelloAck().CameraID == creds.CameraID })

	waitFor(t, "servidor de mídia em execução", 20*time.Second, func() bool {
		var s struct{ Media struct{ State string } }
		admin.do("GET", "/api/v1/system/status", nil, &s)
		return s.Media.State == "running"
	})

	// 5) Publicação WHIP com vídeo H.264 e áudio Opus reais.
	pub, err := testpub.Publish(ctx, creds, testpub.PublishOptions{IncludeLoopback: true})
	if err != nil {
		t.Fatal(err)
	}
	defer pub.Close()
	waitFor(t, "WebRTC conectado", 15*time.Second, pub.Connected)

	camPath := "/api/v1/cameras/" + creds.CameraID
	var cam struct {
		State, Tally, VideoCodec string
		HasAudio                 bool
		BitrateKbps              float64
		Width, Height            int
		ObsSource                struct{ State, InputName string }
	}
	waitFor(t, "câmera STREAMING com métricas", 15*time.Second, func() bool {
		admin.do("GET", camPath, nil, &cam)
		return cam.State == "STREAMING" && cam.BitrateKbps > 0 && cam.HasAudio
	})
	if cam.VideoCodec != "H264" || cam.Width != 160 || cam.Height != 96 {
		t.Fatalf("fluxo inesperado: %+v", cam)
	}

	// 6) Fonte criada automaticamente no OBS apontando para o RTSP local.
	rtspURL := fmt.Sprintf("rtsp://127.0.0.1:%d/sl/%s", cfg.MediaPorts.RTSP, creds.CameraID)
	waitFor(t, "fonte no OBS", 10*time.Second, func() bool {
		in := obsSrv.Input("SacraLink — Câmera 01")
		return in != nil && in.Settings["input"] == rtspURL
	})

	// 7) O que o OBS consome: RTSP com H.264 + Opus e pacotes chegando.
	probe, err := testpub.ProbeRTSP(ctx, rtspURL, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(probe.Codecs, "H264") || !slices.ContainsFunc(probe.Codecs, func(c string) bool { return strings.EqualFold(c, "opus") }) {
		t.Fatalf("codecs RTSP inesperados: %v\n%s", probe.Codecs, probe.SDP)
	}
	if probe.Packets[0] < 10 || probe.Packets[1] < 10 {
		var logs map[string][]string
		admin.do("GET", "/api/v1/system/logs", nil, &logs)
		t.Fatalf("poucos pacotes RTSP recebidos: %v\nSDP:\n%s\nMediaMTX:\n%s", probe.Packets, probe.SDP, strings.Join(logs["mediamtx"], "\n"))
	}
	t.Logf("RTSP OK: codecs=%v pacotes=%v bitrate=%.0f kbps", probe.Codecs, probe.Packets, cam.BitrateKbps)

	// 8) Program sem Studio Mode exige confirmação; com confirmação vai ao ar.
	var apiErr struct{ Error struct{ Code string } }
	if st := admin.do("POST", "/api/v1/obs/program", map[string]any{"cameraId": creds.CameraID}, &apiErr); st != 409 || apiErr.Error.Code != "confirmation_required" {
		t.Fatalf("troca direta sem confirmação deveria falhar: %d %+v", st, apiErr)
	}
	if st := admin.do("POST", "/api/v1/obs/program", map[string]any{"cameraId": creds.CameraID, "confirm": true}, nil); st != 204 {
		t.Fatalf("program: %d", st)
	}
	waitFor(t, "celular recebe NO AR", 5*time.Second, func() bool {
		tl, ok := ctl.LastTally()
		return ok && tl.State == "on_air" && tl.OBSConnected
	})

	// 9) Operador troca de cena no próprio OBS → celular sai do ar.
	obsSrv.SetProgram("Cena")
	waitFor(t, "celular recebe CONECTADA", 5*time.Second, func() bool {
		tl, _ := ctl.LastTally()
		return tl.State == "connected"
	})

	// 10) Zoom remoto com confirmação do aparelho.
	var res struct {
		Status string
		State  map[string]any
	}
	if st := admin.do("POST", camPath+"/zoom", map[string]any{"value": 2.5}, &res); st != 200 || res.Status != "executed" || res.State["zoom"] != 2.5 {
		t.Fatalf("zoom: %d %+v", st, res)
	}

	// 10b) Preset: salva a posição atual (zoom 2,5×), muda o zoom e reaplica.
	var preset struct {
		ID     string
		Params struct{ Zoom float64 }
	}
	if st := admin.do("POST", camPath+"/presets", map[string]any{"name": "Plano fechado", "fromCurrent": true}, &preset); st != 201 || preset.Params.Zoom != 2.5 {
		t.Fatalf("preset: %d %+v", st, preset)
	}
	admin.do("POST", camPath+"/zoom", map[string]any{"value": 1.0}, &res)
	if st := admin.do("POST", camPath+"/presets/"+preset.ID+"/apply", map[string]any{}, &res); st != 200 || res.Status != "executed" || res.State["zoom"] != 2.5 {
		t.Fatalf("aplicar preset: %d %+v", st, res)
	}
	if st := admin.do("POST", camPath+"/presets/pinexistente/apply", map[string]any{}, nil); st != 400 {
		t.Fatalf("preset inexistente deveria ser recusado: %d", st)
	}

	// 11) Player WHEP para Browser Source.
	resp, err := http.Get(a.AdminURL() + "/player/" + creds.CameraID)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("player: %v %v", err, resp)
	}
	resp.Body.Close()

	// 12) Queda do celular → RECONNECTING; a fonte do OBS permanece.
	pub.Close()
	waitFor(t, "RECONNECTING", 10*time.Second, func() bool {
		admin.do("GET", camPath, nil, &cam)
		return cam.State == "RECONNECTING"
	})
	if obsSrv.Input("SacraLink — Câmera 01") == nil {
		t.Fatal("a fonte do OBS não pode ser removida quando a câmera cai")
	}

	// 13) Segurança: API de dispositivos rejeita credencial errada.
	bad := *creds
	bad.Secret = strings.Repeat("x", 43)
	if _, err := testpub.Connect(ctx, &bad); err == nil {
		t.Fatal("credencial errada deveria ser rejeitada")
	}
}
