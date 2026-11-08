package api

import (
	"sync"
	"time"

	"github.com/gersongabrieldev/sacralink/apps/server/internal/secure"
)

// deviceTickets emite tickets de uso único para o WebSocket de controle
// aberto pelo navegador (a API WebSocket do navegador não envia o cabeçalho
// Authorization). O ticket vale 30 s e só pode ser usado uma vez.
type deviceTickets struct {
	mu sync.Mutex
	m  map[string]deviceTicket
}

type deviceTicket struct {
	deviceID string
	expires  time.Time
}

func (t *deviceTickets) issue(deviceID string) string {
	tok := secure.NewToken()
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.m == nil {
		t.m = map[string]deviceTicket{}
	}
	now := time.Now()
	for k, v := range t.m {
		if now.After(v.expires) {
			delete(t.m, k)
		}
	}
	t.m[tok] = deviceTicket{deviceID: deviceID, expires: now.Add(TicketTTL)}
	return tok
}

func (t *deviceTickets) use(tok string) (string, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	v, ok := t.m[tok]
	delete(t.m, tok)
	if !ok || time.Now().After(v.expires) {
		return "", false
	}
	return v.deviceID, true
}
