package testpub

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"

	"github.com/gersongabrieldev/sacralink/apps/server/internal/control"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/tlsutil"
)

// Credentials são as credenciais de um aparelho pareado.
type Credentials struct {
	BaseURL     string `json:"baseUrl"` // https://host:7443
	Fingerprint string `json:"fingerprint"`
	DeviceID    string `json:"deviceId"`
	Secret      string `json:"secret"`
	CameraID    string `json:"cameraId"`
	CameraName  string `json:"cameraName"`
}

func (c Credentials) bearer() string { return "Bearer " + c.DeviceID + "." + c.Secret }

func (c Credentials) client() *http.Client {
	return &http.Client{Timeout: 15 * time.Second, Transport: &http.Transport{TLSClientConfig: tlsutil.PinnedClientConfig(c.Fingerprint)}}
}

// Pair lê o URI do QR Code e pareia o aparelho simulado. hostOverride
// permite forçar o endereço (ex.: 127.0.0.1 em testes).
func Pair(ctx context.Context, uri, name, hostOverride string) (*Credentials, error) {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "sacralink" {
		return nil, errors.New("URI de pareamento inválido")
	}
	q := u.Query()
	host := strings.Split(q.Get("h"), ",")[0]
	if hostOverride != "" {
		host = hostOverride
	}
	creds := &Credentials{BaseURL: "https://" + host + ":" + q.Get("p"), Fingerprint: q.Get("fp")}
	body, _ := json.Marshal(map[string]any{"token": q.Get("t"), "device": map[string]string{
		"name": name, "platform": "android", "model": "SacraLink Test Publisher", "osVersion": "sintético", "appVersion": "0.0.0-teste",
	}})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, creds.BaseURL+"/device/v1/pair", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := creds.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("pareamento falhou: HTTP %d: %s", resp.StatusCode, b)
	}
	var r struct {
		DeviceID, Secret, CameraID, CameraName string
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, err
	}
	creds.DeviceID, creds.Secret, creds.CameraID, creds.CameraName = r.DeviceID, r.Secret, r.CameraID, r.CameraName
	return creds, nil
}

// Control é o canal de controle simulado. Ele confirma automaticamente os
// comandos (zoom etc.) como "executed" e registra o tally recebido.
type Control struct {
	conn *websocket.Conn
	mu   sync.Mutex
	// Tallies recebidos, em ordem.
	Tallies  []control.Tally
	Commands []control.Command
	Ack      control.HelloAck
	zoom     float64
	done     chan struct{}
}

// Connect abre o canal de controle e envia o hello.
func Connect(ctx context.Context, c *Credentials) (*Control, error) {
	wsURL := "wss" + strings.TrimPrefix(c.BaseURL, "https") + "/device/v1/ws"
	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPClient:   c.client(),
		HTTPHeader:   http.Header{"Authorization": {c.bearer()}},
		Subprotocols: []string{"sacralink.control.v1"},
	})
	if err != nil {
		return nil, err
	}
	ctl := &Control{conn: conn, zoom: 1, done: make(chan struct{})}
	if err := ctl.send(ctx, control.MsgHello, control.Hello{AppVersion: "0.0.0-teste", Platform: "android", Model: "Test Publisher",
		Capabilities: control.Capabilities{Lenses: []control.Lens{{ID: "0", Facing: "back", Kind: "wide", Label: "Traseira", MinZoom: 1, MaxZoom: 8}},
			MaxWidth: 160, MaxHeight: 96, MaxFPS: 30}}); err != nil {
		conn.CloseNow()
		return nil, err
	}
	go ctl.loop()
	go ctl.heartbeat()
	return ctl, nil
}

func (c *Control) send(ctx context.Context, typ string, data any) error {
	raw, _ := json.Marshal(data)
	b, _ := json.Marshal(control.Envelope{V: 1, Type: typ, TS: time.Now().UnixMilli(), Data: raw})
	return c.conn.Write(ctx, websocket.MessageText, b)
}

func (c *Control) heartbeat() {
	t := time.NewTicker(control.HeartbeatSec * time.Second)
	defer t.Stop()
	for {
		select {
		case <-c.done:
			return
		case <-t.C:
			_ = c.send(context.Background(), control.MsgPing, nil)
		}
	}
}

func (c *Control) loop() {
	defer close(c.done)
	for {
		_, b, err := c.conn.Read(context.Background())
		if err != nil {
			return
		}
		var env control.Envelope
		if json.Unmarshal(b, &env) != nil {
			continue
		}
		switch env.Type {
		case control.MsgHelloAck:
			c.mu.Lock()
			json.Unmarshal(env.Data, &c.Ack)
			c.mu.Unlock()
		case control.MsgTally:
			var t control.Tally
			json.Unmarshal(env.Data, &t)
			c.mu.Lock()
			c.Tallies = append(c.Tallies, t)
			c.mu.Unlock()
		case control.MsgCommand:
			var cmd control.Command
			json.Unmarshal(env.Data, &cmd)
			c.mu.Lock()
			c.Commands = append(c.Commands, cmd)
			if v, ok := cmd.Params["value"].(float64); ok && cmd.Name == control.CmdZoomSet {
				c.zoom = v
			}
			zoom := c.zoom
			c.mu.Unlock()
			_ = c.send(context.Background(), control.MsgCommandResult, control.CommandResult{
				CommandID: cmd.CommandID, Status: control.ResultExecuted, State: map[string]any{"zoom": zoom},
			})
		}
	}
}

// HelloAck retorna a resposta do servidor ao hello (com lock).
func (c *Control) HelloAck() control.HelloAck {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Ack
}

// LastTally retorna o último tally recebido.
func (c *Control) LastTally() (control.Tally, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.Tallies) == 0 {
		return control.Tally{}, false
	}
	return c.Tallies[len(c.Tallies)-1], true
}

// Close encerra o canal.
func (c *Control) Close() { c.conn.Close(websocket.StatusNormalClosure, "") }

// Publisher publica vídeo e áudio sintéticos via WHIP.
type Publisher struct {
	pc       *webrtc.PeerConnection
	location string
	creds    *Credentials
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	mu       sync.Mutex
	Frames   int
	State    webrtc.PeerConnectionState
}

// PublishOptions configura a publicação.
type PublishOptions struct {
	Width, Height, FPS int
	// IncludeLoopback permite candidatos ICE em 127.0.0.1 (testes locais).
	IncludeLoopback bool
}

// Publish inicia a publicação WHIP e retorna quando a sessão é aceita.
func Publish(ctx context.Context, c *Credentials, o PublishOptions) (*Publisher, error) {
	if o.Width == 0 {
		o.Width, o.Height, o.FPS = 160, 96, 15
	}
	se := webrtc.SettingEngine{}
	se.SetIncludeLoopbackCandidate(o.IncludeLoopback)
	api := webrtc.NewAPI(webrtc.WithSettingEngine(se))
	pc, err := api.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		return nil, err
	}
	video, err := webrtc.NewTrackLocalStaticSample(webrtc.RTPCodecCapability{
		MimeType: webrtc.MimeTypeH264, ClockRate: 90000,
		SDPFmtpLine: "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01e",
	}, "video", "sacralink")
	if err != nil {
		pc.Close()
		return nil, err
	}
	audio, err := webrtc.NewTrackLocalStaticSample(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus, ClockRate: 48000, Channels: 2}, "audio", "sacralink")
	if err != nil {
		pc.Close()
		return nil, err
	}
	for _, t := range []webrtc.TrackLocal{video, audio} {
		if _, err := pc.AddTransceiverFromTrack(t, webrtc.RTPTransceiverInit{Direction: webrtc.RTPTransceiverDirectionSendonly}); err != nil {
			pc.Close()
			return nil, err
		}
	}
	p := &Publisher{pc: pc, creds: c}
	pc.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
		p.mu.Lock()
		p.State = s
		p.mu.Unlock()
	})
	offer, err := pc.CreateOffer(nil)
	if err != nil {
		pc.Close()
		return nil, err
	}
	gather := webrtc.GatheringCompletePromise(pc)
	if err := pc.SetLocalDescription(offer); err != nil {
		pc.Close()
		return nil, err
	}
	select {
	case <-gather:
	case <-time.After(5 * time.Second):
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/device/v1/whip", strings.NewReader(pc.LocalDescription().SDP))
	req.Header.Set("Content-Type", "application/sdp")
	req.Header.Set("Authorization", c.bearer())
	resp, err := c.client().Do(req)
	if err != nil {
		pc.Close()
		return nil, err
	}
	answer, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		pc.Close()
		return nil, fmt.Errorf("WHIP recusado: HTTP %d: %s", resp.StatusCode, answer)
	}
	p.location = resp.Header.Get("Location")
	if err := pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: string(answer)}); err != nil {
		pc.Close()
		return nil, err
	}

	sctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	p.wg.Add(2)
	go func() { // vídeo
		defer p.wg.Done()
		enc := NewEncoder(o.Width, o.Height)
		d := time.Second / time.Duration(o.FPS)
		t := time.NewTicker(d)
		defer t.Stop()
		for {
			select {
			case <-sctx.Done():
				return
			case <-t.C:
				if err := video.WriteSample(media.Sample{Data: enc.Frame(), Duration: d}); err == nil {
					p.mu.Lock()
					p.Frames++
					p.mu.Unlock()
				}
			}
		}
	}()
	go func() { // áudio: quadros Opus de silêncio (20 ms)
		defer p.wg.Done()
		t := time.NewTicker(20 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-sctx.Done():
				return
			case <-t.C:
				_ = audio.WriteSample(media.Sample{Data: []byte{0xf8, 0xff, 0xfe}, Duration: 20 * time.Millisecond})
			}
		}
	}()
	return p, nil
}

// Connected informa se o WebRTC está conectado.
func (p *Publisher) Connected() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.State == webrtc.PeerConnectionStateConnected
}

// Close encerra a publicação (DELETE na sessão WHIP).
func (p *Publisher) Close() {
	p.cancel()
	p.wg.Wait()
	if p.location != "" {
		req, _ := http.NewRequest(http.MethodDelete, p.creds.BaseURL+p.location, nil)
		req.Header.Set("Authorization", p.creds.bearer())
		if resp, err := p.creds.client().Do(req); err == nil {
			resp.Body.Close()
		}
	}
	p.pc.Close()
}
