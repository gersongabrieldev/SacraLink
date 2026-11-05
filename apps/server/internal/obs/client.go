package obs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
)

// Options configura a conexão.
type Options struct {
	// URL do obs-websocket, ex.: ws://127.0.0.1:4455
	URL      string
	Password string
	// Eventos assinados; zero significa SubAll.
	Subscriptions EventSubscription
	// Tamanho da fila de eventos (padrão 1024).
	EventBuffer int
	Logger      *slog.Logger
}

// Client é uma conexão autenticada com o obs-websocket.
type Client struct {
	conn  *websocket.Conn
	hello Hello
	log   *slog.Logger

	seq     atomic.Uint64
	mu      sync.Mutex
	pending map[string]chan requestResponse

	events   chan Event
	overflow atomic.Bool

	done    chan struct{}
	errOnce sync.Once
	err     error
	cancel  context.CancelFunc
}

// Dial conecta e autentica no obs-websocket. O ctx limita apenas o handshake.
func Dial(ctx context.Context, opt Options) (*Client, error) {
	if opt.Logger == nil {
		opt.Logger = slog.Default()
	}
	if opt.Subscriptions == 0 {
		opt.Subscriptions = SubAll
	}
	if opt.EventBuffer <= 0 {
		opt.EventBuffer = 1024
	}
	conn, _, err := websocket.Dial(ctx, opt.URL, &websocket.DialOptions{Subprotocols: []string{Subprotocol}})
	if err != nil {
		return nil, fmt.Errorf("obs: não foi possível conectar em %s: %w", opt.URL, err)
	}
	conn.SetReadLimit(64 << 20) // listas de cenas grandes / screenshots

	hello, err := handshake(ctx, conn, opt)
	if err != nil {
		conn.CloseNow()
		return nil, err
	}

	loopCtx, cancel := context.WithCancel(context.Background())
	c := &Client{
		conn:    conn,
		hello:   hello,
		log:     opt.Logger,
		pending: map[string]chan requestResponse{},
		events:  make(chan Event, opt.EventBuffer),
		done:    make(chan struct{}),
		cancel:  cancel,
	}
	go c.readLoop(loopCtx)
	return c, nil
}

func readEnvelope(ctx context.Context, conn *websocket.Conn) (*envelope, error) {
	_, data, err := conn.Read(ctx)
	if err != nil {
		return nil, err
	}
	env := &envelope{}
	if err := json.Unmarshal(data, env); err != nil {
		return nil, fmt.Errorf("obs: mensagem inválida: %w", err)
	}
	return env, nil
}

func writeJSON(ctx context.Context, conn *websocket.Conn, op int, d any) error {
	b, err := json.Marshal(map[string]any{"op": op, "d": d})
	if err != nil {
		return err
	}
	return conn.Write(ctx, websocket.MessageText, b)
}

func mapCloseError(err error) error {
	switch websocket.CloseStatus(err) {
	case CloseAuthenticationFailed:
		return ErrAuthFailed
	case CloseUnsupportedRPCVersion:
		return ErrUnsupportedRPC
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return ErrHandshakeTimeout
	}
	return err
}

func handshake(ctx context.Context, conn *websocket.Conn, opt Options) (Hello, error) {
	var hello Hello
	env, err := readEnvelope(ctx, conn)
	if err != nil {
		return hello, mapCloseError(err)
	}
	if env.Op != OpHello {
		return hello, fmt.Errorf("obs: esperava Hello, recebeu op %d", env.Op)
	}
	if err := json.Unmarshal(env.D, &hello); err != nil {
		return hello, err
	}
	id := identify{RPCVersion: RPCVersion, EventSubscriptions: opt.Subscriptions}
	if hello.Authentication != nil {
		if opt.Password == "" {
			return hello, ErrAuthRequired
		}
		id.Authentication = AuthString(opt.Password, hello.Authentication.Salt, hello.Authentication.Challenge)
	}
	if err := writeJSON(ctx, conn, OpIdentify, id); err != nil {
		return hello, err
	}
	env, err = readEnvelope(ctx, conn)
	if err != nil {
		return hello, mapCloseError(err)
	}
	if env.Op != OpIdentified {
		return hello, fmt.Errorf("obs: esperava Identified, recebeu op %d", env.Op)
	}
	var idd identified
	if err := json.Unmarshal(env.D, &idd); err != nil {
		return hello, err
	}
	if idd.NegotiatedRPCVersion != RPCVersion {
		return hello, ErrUnsupportedRPC
	}
	return hello, nil
}

// Hello retorna a mensagem Hello recebida (versão do obs-websocket).
func (c *Client) Hello() Hello { return c.hello }

// Events retorna o canal de eventos. Ele é fechado quando a conexão termina.
func (c *Client) Events() <-chan Event { return c.events }

// Done é fechado quando a conexão termina.
func (c *Client) Done() <-chan struct{} { return c.done }

// Err retorna o motivo do encerramento (após Done).
func (c *Client) Err() error {
	select {
	case <-c.done:
		return c.err
	default:
		return nil
	}
}

// Close encerra a conexão.
func (c *Client) Close() error {
	c.shutdown(ErrClosed)
	return c.conn.Close(websocket.StatusNormalClosure, "")
}

func (c *Client) shutdown(err error) {
	c.errOnce.Do(func() {
		c.err = err
		c.cancel()
		c.mu.Lock()
		for id, ch := range c.pending {
			close(ch)
			delete(c.pending, id)
		}
		c.mu.Unlock()
		close(c.done)
	})
}

func (c *Client) readLoop(ctx context.Context) {
	defer close(c.events)
	for {
		env, err := readEnvelope(ctx, c.conn)
		if err != nil {
			c.shutdown(mapCloseError(err))
			return
		}
		switch env.Op {
		case OpEvent:
			var ev Event
			if err := json.Unmarshal(env.D, &ev); err != nil {
				c.log.Warn("obs: evento inválido", "err", err)
				continue
			}
			c.deliver(ev)
		case OpRequestResponse:
			var resp requestResponse
			if err := json.Unmarshal(env.D, &resp); err != nil {
				c.log.Warn("obs: resposta inválida", "err", err)
				continue
			}
			c.mu.Lock()
			ch, ok := c.pending[resp.RequestID]
			delete(c.pending, resp.RequestID)
			c.mu.Unlock()
			if ok {
				ch <- resp
			}
		}
	}
}

func (c *Client) deliver(ev Event) {
	if c.overflow.Load() {
		select {
		case c.events <- Event{Type: EventOverflow}:
			c.overflow.Store(false)
		default:
			return
		}
	}
	select {
	case c.events <- ev:
	default:
		if !c.overflow.Swap(true) {
			c.log.Warn("obs: fila de eventos cheia; estado será ressincronizado")
		}
	}
}

// Call executa uma requisição e decodifica responseData em out (pode ser nil).
func (c *Client) Call(ctx context.Context, requestType string, data any, out any) error {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
	}
	id := strconv.FormatUint(c.seq.Add(1), 10)
	ch := make(chan requestResponse, 1)
	c.mu.Lock()
	select {
	case <-c.done:
		c.mu.Unlock()
		return ErrClosed
	default:
	}
	c.pending[id] = ch
	c.mu.Unlock()

	if err := writeJSON(ctx, c.conn, OpRequest, request{RequestType: requestType, RequestID: id, RequestData: data}); err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return fmt.Errorf("obs: enviar %s: %w", requestType, err)
	}
	select {
	case resp, ok := <-ch:
		if !ok {
			return ErrClosed
		}
		if !resp.RequestStatus.Result {
			return &RequestError{RequestType: requestType, Code: resp.RequestStatus.Code, Comment: resp.RequestStatus.Comment}
		}
		if out != nil && len(resp.ResponseData) > 0 {
			if err := json.Unmarshal(resp.ResponseData, out); err != nil {
				return fmt.Errorf("obs: decodificar %s: %w", requestType, err)
			}
		}
		return nil
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return fmt.Errorf("obs: %s: %w", requestType, ctx.Err())
	case <-c.done:
		return ErrClosed
	}
}
