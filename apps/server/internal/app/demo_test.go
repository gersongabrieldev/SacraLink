package app_test

import (
	"context"
	"log/slog"
	"net/url"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/gersongabrieldev/sacralink/apps/server/internal/app"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/media"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/obs/obstest"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/testpub"
)

// TestDemo sobe uma demonstração completa (servidor, MediaMTX real, OBS
// simulado e câmeras sintéticas) para revisão visual do painel e capturas
// de tela. Só roda com SACRALINK_DEMO=1.
//
//	SACRALINK_DEMO=1 SACRALINK_DEMO_DASHBOARD=../../../desktop/frontend/dist go test -run TestDemo -timeout 30m ./internal/app/
//
// Variáveis: SACRALINK_DEMO_PORT (17380), SACRALINK_DEMO_SECONDS (120),
// SACRALINK_DEMO_CAMERAS (3), SACRALINK_DEMO_TOKEN_FILE.
func TestDemo(t *testing.T) {
	if os.Getenv("SACRALINK_DEMO") != "1" {
		t.Skip("defina SACRALINK_DEMO=1")
	}
	exe, err := media.FindExecutable()
	if err != nil {
		t.Fatal(err)
	}
	env := func(k string, def int) int {
		if v, err := strconv.Atoi(os.Getenv(k)); err == nil {
			return v
		}
		return def
	}
	obsSrv := obstest.New("demo")
	defer obsSrv.Close()
	obsSrv.AddScene("Missa — Plano geral")
	obsSrv.AddScene("Intervalo")
	obsSrv.SetStudioMode(true)

	cfg := app.Config{
		DataDir: t.TempDir(), AdminPort: env("SACRALINK_DEMO_PORT", 17380), DevicePort: freePort(t), DeviceHost: "127.0.0.1",
		MediaPorts: media.Ports{RTSP: freePort(t), WebRTC: freePort(t), ICE: freePort(t), API: freePort(t)},
		MediaExe:   exe, ICEListenHost: "127.0.0.1", DashboardDir: os.Getenv("SACRALINK_DEMO_DASHBOARD"),
		Logger: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})),
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a, err := app.New(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	go a.Run(ctx)
	admin := &adminClient{t: t, base: a.AdminURL()}
	waitFor(t, "API", 10*time.Second, func() bool { return admin.do("GET", "/api/v1/health", nil, nil) == 200 })

	var tok struct{ Token string }
	admin.do("POST", "/api/v1/setup/admin", map[string]string{"password": "demo-admin-123", "serverName": "Paróquia São José"}, &tok)
	admin.token = tok.Token
	var set map[string]any
	admin.do("GET", "/api/v1/settings", nil, &set)
	set["obs"].(map[string]any)["autoManageSources"] = true
	admin.do("PUT", "/api/v1/settings", set, nil)
	u, _ := url.Parse(obsSrv.URL())
	port, _ := strconv.Atoi(u.Port())
	admin.do("PUT", "/api/v1/obs/config", map[string]any{"host": "127.0.0.1", "port": port, "password": "demo", "autoConnect": true}, nil)
	admin.do("POST", "/api/v1/setup/complete", map[string]any{}, nil)
	waitFor(t, "mídia", 20*time.Second, func() bool {
		var s struct{ Media struct{ State string } }
		admin.do("GET", "/api/v1/system/status", nil, &s)
		return s.Media.State == "running"
	})

	names := []struct{ name, operator string }{{"Altar", "Pedro"}, {"Ambão", "Ana"}, {"Nave central", ""}, {"Coro", "Lucas"}, {"Entrada", ""}, {"Batistério", ""}, {"Órgão", ""}, {"Sacristia", ""}}
	var ids []string
	for i := 0; i < env("SACRALINK_DEMO_CAMERAS", 3) && i < len(names); i++ {
		var p struct{ URI string }
		admin.do("POST", "/api/v1/devices/pair", map[string]string{}, &p)
		c, err := testpub.Pair(ctx, p.URI, "Celular "+names[i].name, "127.0.0.1")
		if err != nil {
			t.Fatal(err)
		}
		ctl, err := testpub.Connect(ctx, c)
		if err != nil {
			t.Fatal(err)
		}
		defer ctl.Close()
		pub, err := testpub.Publish(ctx, c, testpub.PublishOptions{Width: 320, Height: 176, FPS: 15, IncludeLoopback: true})
		if err != nil {
			t.Fatal(err)
		}
		defer pub.Close()
		patch := map[string]any{"name": names[i].name, "operatorName": names[i].operator}
		if i == 2 {
			patch["isBackup"] = true
		}
		admin.do("PATCH", "/api/v1/cameras/"+c.CameraID, patch, nil)
		ids = append(ids, c.CameraID)
	}
	waitFor(t, "fontes no OBS", 20*time.Second, func() bool {
		var cams []struct{ ObsSource struct{ State string } }
		admin.do("GET", "/api/v1/cameras", nil, &cams)
		for _, c := range cams {
			if c.ObsSource.State != "present" {
				return false
			}
		}
		return len(cams) == len(ids)
	})
	admin.do("POST", "/api/v1/obs/program", map[string]any{"cameraId": ids[0], "confirm": true}, nil)
	if len(ids) > 1 {
		admin.do("POST", "/api/v1/obs/preview", map[string]any{"cameraId": ids[1]}, nil)
	}
	if f := os.Getenv("SACRALINK_DEMO_TOKEN_FILE"); f != "" {
		os.WriteFile(f, []byte(tok.Token), 0o600)
	}
	t.Logf("demonstração pronta em %s (senha demo-admin-123)", a.AdminURL())
	time.Sleep(time.Duration(env("SACRALINK_DEMO_SECONDS", 120)) * time.Second)
}
