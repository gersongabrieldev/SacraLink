// Package events distribui atualizações em tempo real para os painéis
// conectados (WebSocket do painel), com filas limitadas por cliente.
package events

import (
	"encoding/json"
	"sync"
	"time"
)

// Message é o envelope enviado ao painel.
type Message struct {
	Type string `json:"type"`
	TS   int64  `json:"ts"`
	Data any    `json:"data"`
}

// Client é um assinante.
type Client struct {
	C      chan []byte
	closed bool
}

// Hub distribui mensagens aos assinantes.
type Hub struct {
	mu      sync.Mutex
	clients map[*Client]struct{}
}

// NewHub cria o hub.
func NewHub() *Hub { return &Hub{clients: map[*Client]struct{}{}} }

// Subscribe registra um assinante com fila de 256 mensagens.
func (h *Hub) Subscribe() *Client {
	c := &Client{C: make(chan []byte, 256)}
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
	return c
}

// Unsubscribe remove o assinante.
func (h *Hub) Unsubscribe(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.clients[c]; ok {
		delete(h.clients, c)
		if !c.closed {
			c.closed = true
			close(c.C)
		}
	}
}

// Publish envia a mensagem a todos. Assinantes com fila cheia (painel
// travado) são desconectados; ao reconectar recebem um snapshot completo.
func (h *Hub) Publish(typ string, data any) {
	b, err := json.Marshal(Message{Type: typ, TS: time.Now().UnixMilli(), Data: data})
	if err != nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients {
		select {
		case c.C <- b:
		default:
			delete(h.clients, c)
			if !c.closed {
				c.closed = true
				close(c.C)
			}
		}
	}
}

// Count retorna o número de assinantes.
func (h *Hub) Count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.clients)
}
