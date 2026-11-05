package ratelimit

import (
	"testing"
	"time"
)

func TestBloqueioPorFalhas(t *testing.T) {
	now := time.Now()
	l := New(100, 100, 3, time.Minute, 5*time.Minute)
	l.now = func() time.Time { return now }
	for i := 0; i < 3; i++ {
		l.Fail("1.2.3.4")
	}
	if l.Allow("1.2.3.4") || !l.Blocked("1.2.3.4") {
		t.Fatal("IP deveria estar bloqueado")
	}
	if !l.Allow("5.6.7.8") {
		t.Fatal("outro IP não pode ser afetado")
	}
	now = now.Add(6 * time.Minute)
	if !l.Allow("1.2.3.4") {
		t.Fatal("bloqueio deveria expirar")
	}
}

func TestLimiteDeTaxa(t *testing.T) {
	l := New(1, 2, 10, time.Minute, time.Minute)
	if !l.Allow("a") || !l.Allow("a") {
		t.Fatal("burst deveria permitir 2")
	}
	if l.Allow("a") {
		t.Fatal("terceira requisição imediata deveria ser negada")
	}
}
