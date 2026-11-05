package core

import (
	"context"
	"errors"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/gersongabrieldev/sacralink/apps/server/internal/media"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/obs/obstest"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/obsbridge"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/secure"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/settings"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/store"
)

// fakeMedia simula a API do MediaMTX.
type fakeMedia struct {
	mu    sync.Mutex
	paths map[string]*media.Path
}

func (f *fakeMedia) Paths(context.Context) ([]media.Path, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []media.Path
	for _, p := range f.paths {
		cp := *p
		out = append(out, cp)
	}
	return out, nil
}
func (f *fakeMedia) WebRTCSessions(context.Context) ([]media.WebRTCSession, error) { return nil, nil }
func (f *fakeMedia) KickWebRTCSession(context.Context, string) error               { return nil }

// publish simula bytes chegando no path (chamar periodicamente).
func (f *fakeMedia) publish(path string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p := f.paths[path]
	if p == nil {
		p = &media.Path{Name: path, Online: true, Tracks: []media.Track{{Codec: "H264"}, {Codec: "Opus"}}}
		f.paths[path] = p
	}
	p.InboundBytes += 50_000
}

func (f *fakeMedia) stop(path string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.paths, path)
}

type env struct {
	t     *testing.T
	st    *store.Store
	set   *settings.Manager
	media *fakeMedia
	core  *Core
	srv   *obstest.Server
	cams  []*store.Camera
}

func newEnv(t *testing.T, ncams int) *env {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "c.db"))
	if err != nil {
		t.Fatal(err)
	}
	e := &env{t: t, st: st, media: &fakeMedia{paths: map[string]*media.Path{}}, srv: obstest.New("")}
	for i := 0; i < ncams; i++ {
		tok := secure.NewToken()
		st.CreatePairingToken(ctx, secure.HashSecret(tok), "", "", 60_000)
		_, c, err := st.PairDevice(ctx, secure.HashSecret(tok), store.NewDeviceInput{Name: "Cel", Platform: "android", SecretHash: "h"})
		if err != nil {
			t.Fatal(err)
		}
		e.cams = append(e.cams, c)
	}
	e.set, _ = settings.Load(ctx, st)
	e.set.Update(ctx, func(s *settings.Settings) { s.OBS.AutoManageSources = true })
	e.core = New(Options{Store: st, Settings: e.set, Media: e.media, PollInterval: 50 * time.Millisecond,
		Logger: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))})
	b := obsbridge.New(obsbridge.Options{
		Store: st, RTSPBase: "rtsp://127.0.0.1:8554/", PlayerBase: "http://127.0.0.1:7380/player/",
		Cameras: e.core.CameraRefs, AutoManage: func() bool { return e.set.Get().OBS.AutoManageSources },
		OnChange: e.core.MarkDirtyOBS, OnTally: e.core.OnTally,
	})
	e.core.SetOBS(b)
	u, _ := url.Parse(e.srv.URL())
	port, _ := strconv.Atoi(u.Port())
	b.Configure(obsbridge.Config{Host: "127.0.0.1", Port: port, AutoConnect: true})
	rctx, cancel := context.WithCancel(ctx)
	go b.Run(rctx)
	go e.core.Run(rctx)
	t.Cleanup(func() { cancel(); e.srv.Close(); st.Close() })
	e.waitFor("OBS conectado", func() bool { return b.Status().State == obsbridge.StateConnected })
	return e
}

func (e *env) waitFor(what string, cond func() bool) {
	e.t.Helper()
	for deadline := time.Now().Add(6 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if cond() {
			return
		}
	}
	e.t.Fatalf("tempo esgotado: %s (cam0=%s %s)", what, e.view(0).State, e.view(0).Tally)
}

func (e *env) view(i int) CameraView {
	v, _ := e.core.Camera(e.cams[i].ID)
	return v
}

// feed mantém bytes chegando nos paths indicados até stop ser fechado.
func (e *env) feed(stop chan struct{}, paths ...string) {
	go func() {
		for {
			select {
			case <-stop:
				return
			case <-time.After(30 * time.Millisecond):
				for _, p := range paths {
					e.media.publish(p)
				}
			}
		}
	}()
}

func TestFluxoDetectadoCriaFonteNoOBS(t *testing.T) {
	e := newEnv(t, 1)
	stop := make(chan struct{})
	defer close(stop)
	e.feed(stop, e.cams[0].MediaPath())
	e.waitFor("STREAMING", func() bool { return e.view(0).State == StateStreaming })
	e.waitFor("fonte criada", func() bool { return e.srv.Input("SacraLink — Câmera 01") != nil })
	e.waitFor("bitrate medido", func() bool { return e.view(0).BitrateKbps > 0 })
	v := e.view(0)
	if !v.HasAudio || v.VideoCodec != "H264" || v.BitrateKbps <= 0 {
		t.Fatalf("métricas do fluxo: %+v", v)
	}
	sessions, _ := e.st.ListStreamSessions(context.Background(), 10)
	if len(sessions) != 1 || sessions[0].EndedAt != 0 {
		t.Fatalf("sessão de transmissão não registrada: %+v", sessions)
	}
}

func TestQuedaVaiParaReconnecting(t *testing.T) {
	e := newEnv(t, 1)
	stop := make(chan struct{})
	e.feed(stop, e.cams[0].MediaPath())
	e.waitFor("STREAMING", func() bool { return e.view(0).State == StateStreaming })
	close(stop)
	e.media.stop(e.cams[0].MediaPath())
	e.waitFor("RECONNECTING", func() bool { return e.view(0).State == StateReconnecting })
	evs, _ := e.st.ListEvents(context.Background(), 10)
	found := false
	for _, ev := range evs {
		if ev.Kind == "camera_lost" {
			found = true
		}
	}
	if !found {
		t.Fatalf("evento de perda não registrado: %+v", evs)
	}
}

func TestLimiteDeTransmissoes(t *testing.T) {
	e := newEnv(t, 2)
	e.set.Update(context.Background(), func(s *settings.Settings) { s.MaxStreams = 1 })
	stop := make(chan struct{})
	defer close(stop)
	e.feed(stop, e.cams[0].MediaPath())
	e.waitFor("STREAMING", func() bool { return e.view(0).State == StateStreaming })
	if err := e.core.CanPublish(e.cams[1].ID); !errors.Is(err, ErrStreamLimit) {
		t.Fatalf("esperava ErrStreamLimit, obtive %v", err)
	}
	// A própria câmera pode republicar (reconexão após troca de rede).
	if err := e.core.CanPublish(e.cams[0].ID); err != nil {
		t.Fatalf("republicação deveria ser permitida: %v", err)
	}
}

func TestFallbackParaCameraReserva(t *testing.T) {
	e := newEnv(t, 2)
	ctx := context.Background()
	backup := true
	e.core.UpdateCamera(ctx, "teste", e.cams[1].ID, store.CameraPatch{IsBackup: &backup})
	e.set.Update(ctx, func(s *settings.Settings) { s.Fallback = settings.Fallback{Mode: settings.FallbackBackup, GraceSec: 1} })

	stopMain := make(chan struct{})
	stopBackup := make(chan struct{})
	defer close(stopBackup)
	e.feed(stopMain, e.cams[0].MediaPath())
	e.feed(stopBackup, e.cams[1].MediaPath())
	e.waitFor("duas câmeras transmitindo", func() bool {
		return e.view(0).State == StateStreaming && e.view(1).State == StateStreaming &&
			e.view(0).OBSSource.State == "present" && e.view(1).OBSSource.State == "present"
	})
	if err := e.core.OBS().SendCameraToProgram(ctx, e.cams[0].ID, true); err != nil {
		t.Fatal(err)
	}
	e.waitFor("câmera 1 NO AR", func() bool { return e.view(0).Tally == "on_air" })

	close(stopMain)
	e.media.stop(e.cams[0].MediaPath())
	e.waitFor("troca automática para a reserva", func() bool { return e.srv.Program() == "SacraLink — Cena Câmera 02" })
	e.waitFor("reserva NO AR", func() bool { return e.view(1).Tally == "on_air" })
}

func TestSemFallbackProgramNaoMuda(t *testing.T) {
	e := newEnv(t, 1)
	ctx := context.Background()
	stop := make(chan struct{})
	e.feed(stop, e.cams[0].MediaPath())
	e.waitFor("STREAMING", func() bool { return e.view(0).State == StateStreaming })
	e.waitFor("fonte", func() bool { return e.view(0).OBSSource.State == "present" })
	e.core.OBS().SendCameraToProgram(ctx, e.cams[0].ID, true)
	e.waitFor("NO AR", func() bool { return e.view(0).Tally == "on_air" })
	close(stop)
	e.media.stop(e.cams[0].MediaPath())
	e.waitFor("RECONNECTING", func() bool { return e.view(0).State == StateReconnecting })
	time.Sleep(1500 * time.Millisecond)
	if e.srv.Program() != "SacraLink — Cena Câmera 01" {
		t.Fatalf("o Program não pode mudar sem fallback habilitado: %q", e.srv.Program())
	}
}
