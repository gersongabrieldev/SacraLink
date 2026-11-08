package api

import (
	"testing"
	"time"
)

func TestTicketDoNavegadorUsoUnico(t *testing.T) {
	var tk deviceTickets
	a := tk.issue("dev_a")
	if id, ok := tk.use(a); !ok || id != "dev_a" {
		t.Fatalf("ticket válido recusado: %q %v", id, ok)
	}
	if _, ok := tk.use(a); ok {
		t.Fatal("o ticket só pode ser usado uma vez")
	}
	if _, ok := tk.use("inexistente"); ok {
		t.Fatal("ticket inexistente aceito")
	}

	// Expirado: recusado e removido; emissões seguintes limpam os vencidos.
	b := tk.issue("dev_b")
	tk.mu.Lock()
	v := tk.m[b]
	v.expires = time.Now().Add(-time.Second)
	tk.m[b] = v
	tk.mu.Unlock()
	if _, ok := tk.use(b); ok {
		t.Fatal("ticket expirado aceito")
	}
	c := tk.issue("dev_c")
	tk.mu.Lock()
	tk.m["velho"] = deviceTicket{deviceID: "x", expires: time.Now().Add(-time.Minute)}
	tk.mu.Unlock()
	tk.issue("dev_d")
	tk.mu.Lock()
	_, velho := tk.m["velho"]
	tk.mu.Unlock()
	if velho {
		t.Fatal("tickets vencidos deveriam ser descartados")
	}
	if id, ok := tk.use(c); !ok || id != "dev_c" {
		t.Fatal("ticket válido perdido na limpeza")
	}
}
