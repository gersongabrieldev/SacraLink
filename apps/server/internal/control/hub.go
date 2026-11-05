package control

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"

	"github.com/gersongabrieldev/sacralink/apps/server/internal/secure"
)

// HeartbeatSec é o intervalo de ping esperado do aparelho.
const HeartbeatSec = 5

// CommandTimeout é o tempo máximo para o aparelho confirmar um comando.
const CommandTimeout = 5 * time.Second

// Peer identifica o aparelho autenticado que abriu a sessão.
type Peer struct {
	DeviceID   string
	CameraID   string
	RemoteAddr string
}

// Handler recebe os eventos das sessões (implementado pelo núcleo).
type Handler interface {
	OnHello(p Peer, sessionID string, h Hello) (HelloAck, error)
	OnTelemetry(p Peer, t Telemetry)
	OnLocalChange(p Peer, c LocalChange)
	OnCommandResult(p Peer, r CommandResult)
	OnDisconnect(p Peer, sessionID string, reason string)
}

// Session é uma conexão de controle ativa.
type Session struct {
	ID          string
	Peer        Peer
	ConnectedAt time.Time
	Hello       Hello

	conn    *websocket.Conn
	send    chan []byte
	done    chan struct{}
	once    sync.Once
	mu      sync.Mutex
	pending map[string]chan CommandResult
	lastTel time.Time
}

// close encerra a sessão sem bloquear: o handshake de fechamento do WebSocket
// espera a resposta do aparelho, que pode estar sem rede (troca Wi-Fi → 5G).
func (s *Session) close(code websocket.StatusCode, reason string) {
	s.once.Do(func() {
		close(s.done)
		go s.conn.Close(code, reason)
	})
}

// Hub gerencia as sessões de controle (uma por câmera).
type Hub struct {
	log     *slog.Logger
	handler Handler

	mu       sync.Mutex
	sessions map[string]*Session // cameraID → sessão
	tallies  map[string]Tally
	seq      atomic.Uint64
}

// NewHub cria o hub.
func NewHub(log *slog.Logger, h Handler) *Hub {
	if log == nil {
		log = slog.Default()
	}
	return &Hub{log: log.With("componente", "controle"), handler: h, sessions: map[string]*Session{}, tallies: map[string]Tally{}}
}

func encode(msgType, id string, data any) []byte {
	var raw json.RawMessage
	if data != nil {
		raw, _ = json.Marshal(data)
	}
	b, _ := json.Marshal(Envelope{V: ProtocolVersion, Type: msgType, ID: id, TS: time.Now().UnixMilli(), Data: raw})
	return b
}

// Serve atende uma conexão WebSocket já autenticada.
func (h *Hub) Serve(w http.ResponseWriter, r *http.Request, p Peer) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{Subprotocols: []string{"sacralink.control.v1"}})
	if err != nil {
		return
	}
	conn.SetReadLimit(64 << 10)
	ctx := r.Context()

	// 1) Hello obrigatório em até 10 s.
	hctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	env, err := readEnvelope(hctx, conn)
	cancel()
	if err != nil || env.Type != MsgHello {
		conn.Close(websocket.StatusPolicyViolation, "hello esperado")
		return
	}
	var hello Hello
	if err := json.Unmarshal(env.Data, &hello); err != nil {
		conn.Close(websocket.StatusPolicyViolation, "hello inválido")
		return
	}
	s := &Session{
		ID: secure.NewID("s", 15), Peer: p, ConnectedAt: time.Now(), Hello: hello,
		conn: conn, send: make(chan []byte, 64), done: make(chan struct{}), pending: map[string]chan CommandResult{},
	}
	ack, err := h.handler.OnHello(p, s.ID, hello)
	if err != nil {
		conn.Close(websocket.StatusPolicyViolation, err.Error())
		return
	}
	ack.SessionID, ack.HeartbeatSec = s.ID, HeartbeatSec

	// 2) Registra (substitui sessão anterior da mesma câmera).
	h.mu.Lock()
	if old := h.sessions[p.CameraID]; old != nil {
		old.close(websocket.StatusGoingAway, "substituída por nova conexão")
	}
	h.sessions[p.CameraID] = s
	tally, hasTally := h.tallies[p.CameraID]
	h.mu.Unlock()
	h.log.Info("celular conectado", "camera", p.CameraID, "dispositivo", p.DeviceID, "ip", p.RemoteAddr, "app", hello.AppVersion)

	s.send <- encode(MsgHelloAck, "", ack)
	if hasTally {
		s.send <- encode(MsgTally, "", tally)
	}

	go h.writeLoop(ctx, s)
	reason := h.readLoop(ctx, s)

	s.close(websocket.StatusNormalClosure, "")
	h.mu.Lock()
	if h.sessions[p.CameraID] == s {
		delete(h.sessions, p.CameraID)
	}
	h.mu.Unlock()
	s.mu.Lock()
	for id, ch := range s.pending {
		ch <- CommandResult{CommandID: id, Status: ResultOffline, Message: "aparelho desconectou"}
		delete(s.pending, id)
	}
	s.mu.Unlock()
	h.log.Info("celular desconectado", "camera", p.CameraID, "motivo", reason)
	h.handler.OnDisconnect(p, s.ID, reason)
}

func readEnvelope(ctx context.Context, c *websocket.Conn) (*Envelope, error) {
	typ, b, err := c.Read(ctx)
	if err != nil {
		return nil, err
	}
	if typ != websocket.MessageText {
		return nil, errors.New("mensagem binária não suportada")
	}
	env := &Envelope{}
	if err := json.Unmarshal(b, env); err != nil {
		return nil, err
	}
	if env.V != ProtocolVersion {
		return nil, errors.New("versão de protocolo incompatível")
	}
	return env, nil
}

func (h *Hub) readLoop(ctx context.Context, s *Session) string {
	for {
		rctx, cancel := context.WithTimeout(ctx, 3*HeartbeatSec*time.Second)
		env, err := readEnvelope(rctx, s.conn)
		cancel()
		if err != nil {
			select {
			case <-s.done:
				return "substituída ou encerrada"
			default:
			}
			if errors.Is(err, context.DeadlineExceeded) {
				return "sem heartbeat"
			}
			return err.Error()
		}
		switch env.Type {
		case MsgPing:
			s.trySend(encode(MsgPong, env.ID, nil))
		case MsgTelemetry:
			if time.Since(s.lastTel) < 100*time.Millisecond {
				continue // limita a 10 Hz
			}
			s.lastTel = time.Now()
			var t Telemetry
			if json.Unmarshal(env.Data, &t) == nil {
				h.handler.OnTelemetry(s.Peer, t)
			}
		case MsgLocalChange:
			var c LocalChange
			if json.Unmarshal(env.Data, &c) == nil {
				h.handler.OnLocalChange(s.Peer, c)
			}
		case MsgCommandResult:
			var r CommandResult
			if json.Unmarshal(env.Data, &r) != nil {
				continue
			}
			s.mu.Lock()
			ch := s.pending[r.CommandID]
			delete(s.pending, r.CommandID)
			s.mu.Unlock()
			if ch != nil {
				ch <- r
			}
			h.handler.OnCommandResult(s.Peer, r)
		}
	}
}

func (h *Hub) writeLoop(ctx context.Context, s *Session) {
	ping := time.NewTicker(10 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-s.done:
			return
		case <-ctx.Done():
			return
		case b := <-s.send:
			wctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			err := s.conn.Write(wctx, websocket.MessageText, b)
			cancel()
			if err != nil {
				s.close(websocket.StatusInternalError, "falha de escrita")
				return
			}
		case <-ping.C:
			pctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			err := s.conn.Ping(pctx)
			cancel()
			if err != nil {
				s.close(websocket.StatusGoingAway, "sem resposta ao ping")
				return
			}
		}
	}
}

func (s *Session) trySend(b []byte) bool {
	select {
	case s.send <- b:
		return true
	case <-s.done:
		return false
	default:
		return false // fila cheia: aparelho lento; o próximo envio corrige o estado
	}
}

func (h *Hub) session(cameraID string) *Session {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.sessions[cameraID]
}

// Connected informa se a câmera tem sessão de controle ativa.
func (h *Hub) Connected(cameraID string) bool { return h.session(cameraID) != nil }

// SendTally envia (e memoriza) o tally de uma câmera.
func (h *Hub) SendTally(cameraID string, t Tally) {
	t.Seq = h.seq.Add(1)
	h.mu.Lock()
	prev, had := h.tallies[cameraID]
	h.tallies[cameraID] = t
	s := h.sessions[cameraID]
	h.mu.Unlock()
	if had && prev.State == t.State && prev.Live == t.Live && prev.OBSConnected == t.OBSConnected && prev.Preview == t.Preview && prev.Program == t.Program {
		return // sem mudança
	}
	if s != nil {
		s.trySend(encode(MsgTally, "", t))
	}
}

// SendConfig envia configuração (ex.: perfil de qualidade, nome da câmera).
func (h *Hub) SendConfig(cameraID string, cfg map[string]any) bool {
	if s := h.session(cameraID); s != nil {
		return s.trySend(encode(MsgConfig, "", cfg))
	}
	return false
}

// SendCommand envia um comando e aguarda a confirmação do aparelho.
func (h *Hub) SendCommand(ctx context.Context, cameraID, name string, params map[string]any, force bool) (CommandResult, error) {
	if err := ValidateCommand(name, params); err != nil {
		return CommandResult{}, err
	}
	s := h.session(cameraID)
	if s == nil {
		return CommandResult{Status: ResultOffline, Message: "câmera sem conexão de controle"}, nil
	}
	now := time.Now()
	cmd := Command{
		CommandID: secure.NewID("k", 15), SessionID: s.ID, Name: name, Params: params,
		IssuedAt: now.UnixMilli(), ExpiresAt: now.Add(CommandTimeout).UnixMilli(), Force: force,
	}
	ch := make(chan CommandResult, 1)
	s.mu.Lock()
	s.pending[cmd.CommandID] = ch
	s.mu.Unlock()
	if !s.trySend(encode(MsgCommand, cmd.CommandID, cmd)) {
		s.mu.Lock()
		delete(s.pending, cmd.CommandID)
		s.mu.Unlock()
		return CommandResult{CommandID: cmd.CommandID, Status: ResultOffline, Message: "fila de envio cheia"}, nil
	}
	t := time.NewTimer(CommandTimeout)
	defer t.Stop()
	select {
	case r := <-ch:
		return r, nil
	case <-t.C:
	case <-ctx.Done():
	}
	s.mu.Lock()
	delete(s.pending, cmd.CommandID)
	s.mu.Unlock()
	return CommandResult{CommandID: cmd.CommandID, Status: ResultTimeout, Message: "o aparelho não confirmou o comando a tempo"}, nil
}

// Disconnect encerra a sessão de uma câmera (ex.: dispositivo revogado).
func (h *Hub) Disconnect(cameraID, reason string) {
	if s := h.session(cameraID); s != nil {
		s.close(websocket.StatusPolicyViolation, reason)
	}
}

// SessionInfo é um resumo de sessão para diagnóstico.
type SessionInfo struct {
	SessionID   string `json:"sessionId"`
	CameraID    string `json:"cameraId"`
	DeviceID    string `json:"deviceId"`
	RemoteAddr  string `json:"remoteAddr"`
	ConnectedAt int64  `json:"connectedAt"`
	AppVersion  string `json:"appVersion"`
}

// Sessions lista as sessões ativas.
func (h *Hub) Sessions() []SessionInfo {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]SessionInfo, 0, len(h.sessions))
	for _, s := range h.sessions {
		out = append(out, SessionInfo{SessionID: s.ID, CameraID: s.Peer.CameraID, DeviceID: s.Peer.DeviceID,
			RemoteAddr: s.Peer.RemoteAddr, ConnectedAt: s.ConnectedAt.UnixMilli(), AppVersion: s.Hello.AppVersion})
	}
	return out
}
