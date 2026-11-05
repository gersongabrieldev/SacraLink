package obsbridge

import (
	"context"
	"errors"
	"net/url"
	"path/filepath"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/gersongabrieldev/sacralink/apps/server/internal/obs/obstest"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/secure"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/store"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/tally"
)

type harness struct {
	t          *testing.T
	srv        *obstest.Server
	st         *store.Store
	b          *Bridge
	cam        CameraRef
	mu         sync.Mutex
	last       map[string]tally.State
	sawUnknown bool
	cancel     context.CancelFunc
}

func newHarness(t *testing.T, password string) *harness {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	tok := secure.NewToken()
	if _, err := st.CreatePairingToken(ctx, secure.HashSecret(tok), "", "", 60_000); err != nil {
		t.Fatal(err)
	}
	_, c, err := st.PairDevice(ctx, secure.HashSecret(tok), store.NewDeviceInput{Name: "Celular", Platform: "android", SecretHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	h := &harness{t: t, srv: obstest.New(password), st: st,
		cam: CameraRef{ID: c.ID, Slot: c.Slot, Name: c.Name, MediaPath: c.MediaPath(), IngestMode: store.IngestRTSP}}
	h.b = New(Options{
		Store: st, RTSPBase: "rtsp://127.0.0.1:8554/", PlayerBase: "http://127.0.0.1:7380/player/",
		OnTally: func(m map[string]tally.State, _ bool) {
			h.mu.Lock()
			h.last = m
			if st, ok := m[h.cam.ID]; ok && !st.Known {
				h.sawUnknown = true
			}
			h.mu.Unlock()
		},
	})
	u, _ := url.Parse(h.srv.URL())
	port, _ := strconv.Atoi(u.Port())
	h.b.Configure(Config{Host: u.Hostname(), Port: port, Password: password, AutoConnect: true})
	rctx, cancel := context.WithCancel(ctx)
	h.cancel = cancel
	go h.b.Run(rctx)
	t.Cleanup(func() { cancel(); h.srv.Close(); st.Close() })
	return h
}

func (h *harness) waitFor(what string, cond func() bool) {
	h.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	h.t.Fatalf("tempo esgotado aguardando: %s (status=%+v tally=%+v)", what, h.b.Status(), h.tally())
}

func (h *harness) tally() tally.State {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.last[h.cam.ID]
}

func (h *harness) connected() {
	h.waitFor("conexão", func() bool { return h.b.Status().State == StateConnected })
}

func (h *harness) ensure() *store.SourceMapping {
	h.t.Helper()
	m, err := h.b.EnsureSource(context.Background(), h.cam, EnsureOptions{})
	if err != nil {
		h.t.Fatal(err)
	}
	return m
}

func count(calls []string, name string) int {
	n := 0
	for _, c := range calls {
		if c == name {
			n++
		}
	}
	return n
}

func TestCriaFonteECenaUmaVez(t *testing.T) {
	h := newHarness(t, "senha")
	h.connected()
	m := h.ensure()
	if m.InputName != "SacraLink — Câmera 01" || m.SceneName != "SacraLink — Cena Câmera 01" || m.InputKind != KindMediaSource {
		t.Fatalf("mapeamento inesperado: %+v", m)
	}
	in := h.srv.Input(m.InputName)
	if in == nil || in.Settings["input"] != "rtsp://127.0.0.1:8554/"+h.cam.MediaPath || in.Settings["is_local_file"] != false {
		t.Fatalf("configuração da fonte: %+v", in)
	}
	// Idempotência: reconexões/novas chamadas não duplicam.
	h.ensure()
	h.ensure()
	if n := count(h.srv.Calls(), "CreateInput"); n != 1 {
		t.Fatalf("CreateInput chamado %d vezes", n)
	}
	// Persistido no banco.
	if _, err := h.st.GetSourceMapping(context.Background(), h.cam.ID); err != nil {
		t.Fatal(err)
	}
}

func TestTallyProgramPreviewEAninhada(t *testing.T) {
	h := newHarness(t, "")
	h.connected()
	m := h.ensure()
	h.waitFor("tally inicial", func() bool { return h.tally().Known && !h.tally().Program })

	// Sem Studio Mode: troca direta exige confirmação.
	err := h.b.SendCameraToProgram(context.Background(), h.cam.ID, false)
	if !errors.Is(err, ErrConfirmationRequired) {
		t.Fatalf("esperava ErrConfirmationRequired, obtive %v", err)
	}
	if h.srv.Program() == m.SceneName {
		t.Fatal("Program não pode mudar sem confirmação")
	}
	if err := h.b.SendCameraToProgram(context.Background(), h.cam.ID, true); err != nil {
		t.Fatal(err)
	}
	h.waitFor("NO AR", func() bool { return h.tally().Program })

	// Operador troca manualmente para outra cena no OBS.
	h.srv.SetProgram("Cena")
	h.waitFor("fora do ar", func() bool { return !h.tally().Program })

	// Cena "Missa" contendo a cena da câmera aninhada.
	h.srv.AddScene("Missa")
	h.srv.AddSceneItem("Missa", m.SceneName, true)
	h.srv.SetProgram("Missa")
	h.waitFor("NO AR via cena aninhada", func() bool { return h.tally().Program })

	// Ocultar a cena aninhada tira a câmera do ar.
	h.srv.SetItemEnabled("Missa", m.SceneName, false)
	h.waitFor("oculta", func() bool { return !h.tally().Program })
}

func TestStudioModePreviewETransicao(t *testing.T) {
	h := newHarness(t, "")
	h.connected()
	h.ensure()
	if err := h.b.SendCameraToPreview(context.Background(), h.cam.ID); !errors.Is(err, ErrStudioModeOff) {
		t.Fatalf("esperava ErrStudioModeOff, obtive %v", err)
	}
	h.srv.SetStudioMode(true)
	h.waitFor("studio mode", func() bool { return h.b.Status().StudioMode })
	if err := h.b.SendCameraToPreview(context.Background(), h.cam.ID); err != nil {
		t.Fatal(err)
	}
	h.waitFor("PREVIEW", func() bool { return h.tally().Preview && !h.tally().Program })
	if err := h.b.Transition(context.Background()); err != nil {
		t.Fatal(err)
	}
	h.waitFor("NO AR após transição", func() bool { return h.tally().Program })
	// Com Studio Mode, SetProgramScene passa pelo Preview e não exige confirmação.
	if err := h.b.SetProgramScene(context.Background(), "Cena", false); err != nil {
		t.Fatal(err)
	}
	h.waitFor("fora do ar", func() bool { return !h.tally().Program })
}

func TestOBSReiniciaETallyFicaDesconhecido(t *testing.T) {
	h := newHarness(t, "")
	h.connected()
	h.ensure()
	_ = h.b.SendCameraToProgram(context.Background(), h.cam.ID, true)
	h.waitFor("NO AR", func() bool { return h.tally().Program })

	h.mu.Lock()
	h.sawUnknown = false
	h.mu.Unlock()
	h.srv.DropConnections() // OBS fechou
	h.waitFor("tally desconhecido emitido", func() bool {
		h.mu.Lock()
		defer h.mu.Unlock()
		return h.sawUnknown
	})
	// Reconexão automática restaura o estado real.
	h.waitFor("reconexão + NO AR", func() bool { return h.tally().Known && h.tally().Program })
}

func TestFonteRemovidaPeloUsuarioNaoERecriada(t *testing.T) {
	h := newHarness(t, "")
	h.connected()
	m := h.ensure()
	h.srv.RemoveInputByUser(m.InputName)
	h.waitFor("fonte ausente", func() bool { return h.b.Sources()[h.cam.ID].State == "missing" })
	if _, err := h.b.EnsureSource(context.Background(), h.cam, EnsureOptions{}); !errors.Is(err, ErrSourceRemovedByUser) {
		t.Fatalf("esperava ErrSourceRemovedByUser, obtive %v", err)
	}
	m2, err := h.b.EnsureSource(context.Background(), h.cam, EnsureOptions{Recreate: true})
	if err != nil {
		t.Fatal(err)
	}
	if m2.SceneName != m.SceneName {
		t.Fatalf("deveria reutilizar a cena existente: %q vs %q", m2.SceneName, m.SceneName)
	}
	if h.srv.Input(m2.InputName) == nil {
		t.Fatal("fonte não recriada")
	}
}

func TestRenomeacaoNoOBSEhSeguida(t *testing.T) {
	h := newHarness(t, "")
	h.connected()
	m := h.ensure()
	h.srv.RenameInputByUser(m.InputName, "Câmera do Altar")
	h.waitFor("mapeamento renomeado", func() bool { return h.b.Mapping(h.cam.ID).InputName == "Câmera do Altar" })
	_ = h.b.SendCameraToProgram(context.Background(), h.cam.ID, true)
	h.waitFor("NO AR após renomear", func() bool { return h.tally().Program })
	h.ensure()
	if n := count(h.srv.Calls(), "CreateInput"); n != 1 {
		t.Fatalf("renomeação não pode gerar duplicata (CreateInput=%d)", n)
	}
}

func TestAdocaoSemMapeamento(t *testing.T) {
	h := newHarness(t, "")
	h.connected()
	m := h.ensure()
	// Simula perda do banco: mapeamento some, a fonte continua no OBS.
	if err := h.st.DeleteSourceMapping(context.Background(), h.cam.ID); err != nil {
		t.Fatal(err)
	}
	m2 := h.ensure()
	if m2.InputName != m.InputName || count(h.srv.Calls(), "CreateInput") != 1 {
		t.Fatalf("deveria adotar a fonte existente: %+v", m2)
	}
}

func TestNomeDeTerceiroNaoEhTocado(t *testing.T) {
	h := newHarness(t, "")
	h.connected()
	// Usuário já tem uma fonte com o nome padrão, apontando para outra coisa.
	h.srv.AddInput("Cena", "SacraLink — Câmera 01", "ffmpeg_source")
	m := h.ensure()
	if m.InputName != "SacraLink — Câmera 01 (2)" {
		t.Fatalf("deveria usar nome alternativo: %q", m.InputName)
	}
	if h.srv.Input("SacraLink — Câmera 01").Settings["input"] != nil {
		t.Fatal("fonte do usuário foi alterada")
	}
}

func TestTrocaDeModoDeIngestao(t *testing.T) {
	h := newHarness(t, "")
	h.connected()
	h.ensure()
	h.cam.IngestMode = store.IngestWHEP
	if _, err := h.b.EnsureSource(context.Background(), h.cam, EnsureOptions{}); !errors.Is(err, ErrIngestModeMismatch) {
		t.Fatalf("esperava ErrIngestModeMismatch, obtive %v", err)
	}
	m, err := h.b.EnsureSource(context.Background(), h.cam, EnsureOptions{Recreate: true})
	if err != nil {
		t.Fatal(err)
	}
	in := h.srv.Input(m.InputName)
	if in.Kind != KindBrowserSource || in.Settings["url"] != "http://127.0.0.1:7380/player/"+h.cam.ID || in.Settings["reroute_audio"] != true {
		t.Fatalf("fonte WHEP inesperada: %+v", in)
	}
	if !slices.Contains(h.srv.SceneItems(m.SceneName), m.InputName) {
		t.Fatal("fonte WHEP fora da cena da câmera")
	}
}

func TestSenhaErrada(t *testing.T) {
	h := newHarness(t, "certa")
	h.b.Configure(Config{Host: "127.0.0.1", Port: portOf(h.srv.URL()), Password: "errada", AutoConnect: true})
	h.waitFor("auth_failed", func() bool { return h.b.Status().State == StateAuthFailed })
	if h.tally().Known {
		t.Fatal("tally deveria ser desconhecido sem conexão")
	}
}

func TestTransmissaoExigeConfirmacao(t *testing.T) {
	h := newHarness(t, "")
	h.connected()
	if err := h.b.SetOutput(context.Background(), OutputStream, true, false); !errors.Is(err, ErrOutputConfirmRequired) {
		t.Fatalf("esperava ErrOutputConfirmRequired, obtive %v", err)
	}
	if err := h.b.SetOutput(context.Background(), OutputStream, true, true); err != nil {
		t.Fatal(err)
	}
	h.waitFor("transmitindo", func() bool { return h.b.Status().Streaming && h.b.Live() })
	info, err := h.b.Outputs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.Destination == nil || info.Destination.Service == "" {
		t.Fatalf("destino ausente: %+v", info.Destination)
	}
}

func TestCenasListamCameras(t *testing.T) {
	h := newHarness(t, "")
	h.connected()
	m := h.ensure()
	h.srv.AddScene("Multicam")
	h.srv.AddGroup("Multicam", "PiP", m.InputName)
	h.waitFor("grupo sincronizado", func() bool {
		scenes, err := h.b.Scenes(context.Background())
		if err != nil {
			return false
		}
		for _, s := range scenes {
			if s.Name == "Multicam" && slices.Contains(s.CameraIDs, h.cam.ID) {
				return true
			}
		}
		return false
	})
}

func portOf(u string) int {
	p, _ := url.Parse(u)
	n, _ := strconv.Atoi(p.Port())
	return n
}
