package app_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/png"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gersongabrieldev/sacralink/apps/server/internal/app"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/media"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/obs"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/platform"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/testpub"
)

// TestOBSReal valida o SacraLink contra o OBS Studio REAL instalado neste
// computador. Só roda com SACRALINK_REAL_OBS=1 e o OBS aberto.
//
// Salvaguardas: aborta se o OBS estiver transmitindo ou gravando; trabalha
// numa coleção de cenas nova ("SacraLink — Teste …") sem tocar nas cenas do
// usuário; ao final volta para a coleção original (a coleção de teste fica
// na lista do OBS e pode ser apagada pelo menu Coleção de cenas).
func TestOBSReal(t *testing.T) {
	if os.Getenv("SACRALINK_REAL_OBS") != "1" {
		t.Skip("defina SACRALINK_REAL_OBS=1 com o OBS aberto")
	}
	exe, err := media.FindExecutable()
	if err != nil {
		t.Fatal(err)
	}
	ws, err := platform.ReadOBSWebSocketConfig()
	if err != nil || !ws.Enabled {
		t.Fatalf("obs-websocket desligado ou não configurado neste usuário: %v", err)
	}
	pw := ""
	if ws.AuthRequired {
		pw = ws.Password
	}
	port := ws.Port
	if port == 0 {
		port = 4455
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dctx, dcancel := context.WithTimeout(ctx, 5*time.Second)
	direct, err := obs.Dial(dctx, obs.Options{URL: fmt.Sprintf("ws://127.0.0.1:%d", port), Password: pw})
	dcancel()
	if err != nil {
		t.Fatalf("não foi possível conectar ao OBS (ele está aberto?): %v", err)
	}
	defer direct.Close()
	v, _ := direct.GetVersion(ctx)
	t.Logf("OBS real: %s (obs-websocket %s)", v.OBSVersion, v.OBSWebSocketVersion)

	st, _ := direct.GetStreamStatus(ctx)
	rec, _ := direct.GetRecordStatus(ctx)
	if (st != nil && st.OutputActive) || (rec != nil && rec.OutputActive) {
		t.Fatal("o OBS está transmitindo ou gravando: teste abortado sem alterar nada")
	}
	cols, err := direct.GetSceneCollectionList(ctx)
	if err != nil {
		t.Fatal(err)
	}
	original := cols.Current
	testCol := "SacraLink — Teste " + time.Now().Format("2006-01-02 15h04m05")
	if err := direct.CreateSceneCollection(ctx, testCol); err != nil {
		t.Fatalf("criar coleção de teste: %v", err)
	}
	t.Logf("coleção de teste %q criada (original: %q)", testCol, original)
	defer func() {
		rctx, rcancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer rcancel()
		if err := direct.SetCurrentSceneCollection(rctx, original); err != nil {
			t.Errorf("ATENÇÃO: não foi possível voltar para a coleção %q: %v", original, err)
		} else {
			t.Logf("coleção original %q restaurada", original)
		}
	}()
	time.Sleep(2 * time.Second) // o OBS recarrega a coleção

	cfg := app.Config{
		DataDir: t.TempDir(), AdminPort: freePort(t), DevicePort: freePort(t), DeviceHost: "127.0.0.1",
		MediaPorts: media.Ports{RTSP: freePort(t), WebRTC: freePort(t), ICE: freePort(t), API: freePort(t)},
		MediaExe:   exe, ICEListenHost: "127.0.0.1",
		Logger: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})),
	}
	actx, acancel := context.WithCancel(ctx)
	a, err := app.New(actx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- a.Run(actx) }()
	defer func() { acancel(); <-done }()

	admin := &adminClient{t: t, base: a.AdminURL()}
	waitFor(t, "API", 10*time.Second, func() bool { return admin.do("GET", "/api/v1/health", nil, nil) == 200 })
	var tok struct{ Token string }
	admin.do("POST", "/api/v1/setup/admin", map[string]string{"password": "teste-obs-real-123"}, &tok)
	admin.token = tok.Token
	var set map[string]any
	admin.do("GET", "/api/v1/settings", nil, &set)
	set["obs"].(map[string]any)["autoManageSources"] = true
	admin.do("PUT", "/api/v1/settings", set, nil)
	if st := admin.do("POST", "/api/v1/obs/config/import", map[string]any{}, nil); st != 200 {
		t.Fatalf("importar configuração do OBS: %d", st)
	}
	waitFor(t, "SacraLink conectado ao OBS real", 15*time.Second, func() bool {
		var s struct{ Status struct{ State string } }
		admin.do("GET", "/api/v1/obs/status", nil, &s)
		return s.Status.State == "connected"
	})
	waitFor(t, "mídia", 20*time.Second, func() bool {
		var s struct{ Media struct{ State string } }
		admin.do("GET", "/api/v1/system/status", nil, &s)
		return s.Media.State == "running"
	})

	var pairing struct{ URI string }
	admin.do("POST", "/api/v1/devices/pair", map[string]string{}, &pairing)
	creds, err := testpub.Pair(ctx, pairing.URI, "Celular de teste (OBS real)", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	ctl, err := testpub.Connect(ctx, creds)
	if err != nil {
		t.Fatal(err)
	}
	defer ctl.Close()
	pub, err := testpub.Publish(ctx, creds, testpub.PublishOptions{Width: 320, Height: 176, FPS: 15, IncludeLoopback: true})
	if err != nil {
		t.Fatal(err)
	}
	defer pub.Close()

	var cam struct {
		State     string
		ObsSource struct{ State, InputName, SceneName string }
	}
	waitFor(t, "fonte criada no OBS real", 30*time.Second, func() bool {
		admin.do("GET", "/api/v1/cameras/"+creds.CameraID, nil, &cam)
		return cam.State == "STREAMING" && cam.ObsSource.State == "present"
	})
	t.Logf("fonte %q na cena %q", cam.ObsSource.InputName, cam.ObsSource.SceneName)

	// Prova de decodificação: captura da fonte feita pelo próprio OBS.
	var img image.Image
	var raw []byte
	waitFor(t, "OBS decodificando o vídeo (captura não vazia)", 30*time.Second, func() bool {
		data, err := direct.GetSourceScreenshot(ctx, cam.ObsSource.InputName, "png", 320)
		if err != nil {
			return false
		}
		raw, err = base64.StdEncoding.DecodeString(data[strings.Index(data, ",")+1:])
		if err != nil {
			return false
		}
		img, _, err = image.Decode(bytes.NewReader(raw))
		return err == nil && lumaVariance(img) > 100
	})
	out := filepath.Join(os.TempDir(), "sacralink-obs-real-"+creds.CameraID+".png")
	_ = os.WriteFile(out, raw, 0o644)
	t.Logf("captura da fonte pelo OBS real: %dx%d, variância de luminância %.0f — salva em %s",
		img.Bounds().Dx(), img.Bounds().Dy(), lumaVariance(img), out)

	// Tally a partir do OBS real.
	if st := admin.do("POST", "/api/v1/obs/program", map[string]any{"cameraId": creds.CameraID, "confirm": true}, nil); st != 204 {
		t.Fatalf("program: %d", st)
	}
	waitFor(t, "celular recebe NO AR do OBS real", 10*time.Second, func() bool {
		tl, ok := ctl.LastTally()
		return ok && tl.State == "on_air"
	})
	active, err := direct.GetSourceActive(ctx, cam.ObsSource.InputName)
	if err != nil || !active.VideoActive {
		t.Fatalf("o OBS deveria reportar a fonte ativa no Program: %+v %v", active, err)
	}
	var audio []struct{ InputName, CameraID string }
	admin.do("GET", "/api/v1/obs/audio", nil, &audio)
	hasAudio := false
	for _, a := range audio {
		if a.CameraID == creds.CameraID {
			hasAudio = true
		}
	}
	if !hasAudio {
		t.Fatal("a fonte da câmera deveria aparecer no mixer de áudio do OBS")
	}
	t.Log("OBS real: fonte criada, vídeo decodificado, tally NO AR, fonte ativa e áudio no mixer")
}

// lumaVariance mede se a imagem tem conteúdo (uma tela preta/vazia ≈ 0).
func lumaVariance(img image.Image) float64 {
	b := img.Bounds()
	var sum, sq, n float64
	for y := b.Min.Y; y < b.Max.Y; y += 2 {
		for x := b.Min.X; x < b.Max.X; x += 2 {
			r, g, bl, _ := img.At(x, y).RGBA()
			l := (0.299*float64(r) + 0.587*float64(g) + 0.114*float64(bl)) / 257
			sum += l
			sq += l * l
			n++
		}
	}
	if n == 0 {
		return 0
	}
	m := sum / n
	return sq/n - m*m
}
