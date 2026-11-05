package control

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
)

type fakeHandler struct {
	mu          sync.Mutex
	hellos      int
	telemetry   []Telemetry
	disconnects []string
	results     []CommandResult
}

func (f *fakeHandler) OnHello(p Peer, sid string, h Hello) (HelloAck, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.hellos++
	return HelloAck{DeviceID: p.DeviceID, CameraID: p.CameraID, CameraName: "Câmera 01", Slot: 1}, nil
}
func (f *fakeHandler) OnTelemetry(p Peer, t Telemetry) {
	f.mu.Lock()
	f.telemetry = append(f.telemetry, t)
	f.mu.Unlock()
}
func (f *fakeHandler) OnLocalChange(Peer, LocalChange) {}
func (f *fakeHandler) OnCommandResult(p Peer, r CommandResult) {
	f.mu.Lock()
	f.results = append(f.results, r)
	f.mu.Unlock()
}
func (f *fakeHandler) OnDisconnect(p Peer, sid, reason string) {
	f.mu.Lock()
	f.disconnects = append(f.disconnects, reason)
	f.mu.Unlock()
}

type device struct {
	t    *testing.T
	conn *websocket.Conn
}

func (d *device) send(typ string, data any) {
	raw, _ := json.Marshal(data)
	b, _ := json.Marshal(Envelope{V: 1, Type: typ, TS: time.Now().UnixMilli(), Data: raw})
	if err := d.conn.Write(context.Background(), websocket.MessageText, b); err != nil {
		d.t.Fatal(err)
	}
}

func (d *device) recv(typ string) Envelope {
	d.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for {
		_, b, err := d.conn.Read(ctx)
		if err != nil {
			d.t.Fatalf("aguardando %s: %v", typ, err)
		}
		var env Envelope
		json.Unmarshal(b, &env)
		if env.Type == typ {
			return env
		}
	}
}

func setup(t *testing.T) (*Hub, *fakeHandler, string) {
	f := &fakeHandler{}
	h := NewHub(nil, f)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.Serve(w, r, Peer{DeviceID: "d1", CameraID: "c1", RemoteAddr: r.RemoteAddr})
	}))
	t.Cleanup(srv.Close)
	return h, f, "ws" + strings.TrimPrefix(srv.URL, "http")
}

func connect(t *testing.T, url string) *device {
	c, _, err := websocket.Dial(context.Background(), url, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.CloseNow() })
	d := &device{t: t, conn: c}
	d.send(MsgHello, Hello{AppVersion: "0.1.0", Platform: "android"})
	return d
}

func TestHelloTallyEComando(t *testing.T) {
	h, _, url := setup(t)
	h.SendTally("c1", Tally{State: "preview", Preview: true, OBSConnected: true})
	d := connect(t, url)
	ack := d.recv(MsgHelloAck)
	var a HelloAck
	json.Unmarshal(ack.Data, &a)
	if a.SessionID == "" || a.CameraName != "Câmera 01" || a.HeartbeatSec != HeartbeatSec {
		t.Fatalf("ack inesperado: %+v", a)
	}
	// O tally memorizado é entregue logo após conectar.
	var tl Tally
	json.Unmarshal(d.recv(MsgTally).Data, &tl)
	if tl.State != "preview" {
		t.Fatalf("tally inicial: %+v", tl)
	}

	go func() {
		env := d.recv(MsgCommand)
		var c Command
		json.Unmarshal(env.Data, &c)
		if c.SessionID != a.SessionID || c.Name != CmdZoomSet || c.ExpiresAt <= c.IssuedAt {
			t.Errorf("comando inesperado: %+v", c)
		}
		d.send(MsgCommandResult, CommandResult{CommandID: c.CommandID, Status: ResultExecuted, State: map[string]any{"zoom": 2.0}})
	}()
	r, err := h.SendCommand(context.Background(), "c1", CmdZoomSet, map[string]any{"value": 2.0}, false)
	if err != nil || r.Status != ResultExecuted || r.State["zoom"] != 2.0 {
		t.Fatalf("resultado: %+v %v", r, err)
	}
}

func TestComandoInvalidoNaoEhEnviado(t *testing.T) {
	h, _, _ := setup(t)
	for _, c := range []struct {
		name string
		p    map[string]any
	}{
		{CmdZoomSet, map[string]any{"value": 500.0}},
		{CmdFocusPoint, map[string]any{"x": 2.0, "y": 0.5}},
		{"system.exec", nil},
		{CmdTorchSet, map[string]any{"on": "sim"}},
	} {
		if _, err := h.SendCommand(context.Background(), "c1", c.name, c.p, false); !errors.Is(err, ErrInvalidCommand) {
			t.Fatalf("%s deveria ser inválido: %v", c.name, err)
		}
	}
}

func TestComandoParaCameraOffline(t *testing.T) {
	h, _, _ := setup(t)
	r, err := h.SendCommand(context.Background(), "c1", CmdCameraFlip, nil, false)
	if err != nil || r.Status != ResultOffline {
		t.Fatalf("esperava offline: %+v %v", r, err)
	}
}

func TestNovaConexaoSubstituiAnterior(t *testing.T) {
	h, f, url := setup(t)
	d1 := connect(t, url)
	d1.recv(MsgHelloAck)
	d2 := connect(t, url)
	d2.recv(MsgHelloAck)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, _, err := d1.conn.Read(ctx); err == nil {
		t.Fatal("a primeira sessão deveria ser encerrada")
	}
	time.Sleep(100 * time.Millisecond)
	if !h.Connected("c1") || len(h.Sessions()) != 1 {
		t.Fatal("a segunda sessão deveria permanecer ativa")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.hellos != 2 {
		t.Fatalf("hellos: %d", f.hellos)
	}
}

func TestTelemetriaEPing(t *testing.T) {
	_, f, url := setup(t)
	d := connect(t, url)
	d.recv(MsgHelloAck)
	d.send(MsgTelemetry, map[string]any{"thermal": "serious", "battery": map[string]any{"level": 0.42}})
	d.send(MsgPing, nil)
	d.recv(MsgPong)
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.telemetry) != 1 || f.telemetry[0].Thermal != "serious" || f.telemetry[0].Battery.Level != 0.42 {
		t.Fatalf("telemetria: %+v", f.telemetry)
	}
}

func TestSemHelloEncerra(t *testing.T) {
	_, f, url := setup(t)
	c, _, err := websocket.Dial(context.Background(), url, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()
	b, _ := json.Marshal(Envelope{V: 1, Type: MsgTelemetry})
	c.Write(context.Background(), websocket.MessageText, b)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, _, err := c.Read(ctx); websocket.CloseStatus(err) != websocket.StatusPolicyViolation {
		t.Fatalf("esperava fechamento por violação de política: %v", err)
	}
	if f.hellos != 0 {
		t.Fatal("sessão sem hello não pode ser registrada")
	}
}
