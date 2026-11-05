package core

import (
	"testing"
	"time"

	"github.com/gersongabrieldev/sacralink/apps/server/internal/tally"
)

func TestDeriveState(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name string
		in   stateInput
		want string
	}{
		{"sem nada", stateInput{now: now}, StateOffline},
		{"só controle", stateInput{now: now, controlConnected: true}, StateConnected},
		{"negociando WHIP", stateInput{now: now, controlConnected: true, negotiatingAt: now.Add(-2 * time.Second)}, StateConnecting},
		{"negociação expirada", stateInput{now: now, controlConnected: true, negotiatingAt: now.Add(-20 * time.Second)}, StateConnected},
		{"bytes chegando", stateInput{now: now, online: true, lastBytesAt: now.Add(-time.Second)}, StateStreaming},
		{"online sem bytes recentes", stateInput{now: now, online: true, controlConnected: true, lastBytesAt: now.Add(-5 * time.Second)}, StateConnected},
		{"caiu há pouco", stateInput{now: now, lostAt: now.Add(-10 * time.Second)}, StateReconnecting},
		{"caiu há muito", stateInput{now: now, lostAt: now.Add(-2 * time.Minute)}, StateOffline},
		{"caiu com controle ativo", stateInput{now: now, controlConnected: true, lostAt: now.Add(-5 * time.Second)}, StateReconnecting},
		{"erro prevalece", stateInput{now: now, online: true, lastBytesAt: now, errorReason: "codec"}, StateError},
	}
	for _, c := range cases {
		if got := deriveState(c.in); got != c.want {
			t.Errorf("%s: esperado %s, obtido %s", c.name, c.want, got)
		}
	}
}

func TestDeviceTally(t *testing.T) {
	if d := deviceTally(tally.State{Known: true, Program: true}, true, true); d.State != "on_air" || !d.Live {
		t.Fatalf("on_air: %+v", d)
	}
	if d := deviceTally(tally.State{Known: true, Program: true, Preview: true}, false, true); d.State != "on_air" || !d.Preview {
		t.Fatalf("program+preview: %+v", d)
	}
	if d := deviceTally(tally.State{Known: true, Preview: true}, false, true); d.State != "preview" {
		t.Fatalf("preview: %+v", d)
	}
	if d := deviceTally(tally.State{Known: true}, false, true); d.State != "connected" {
		t.Fatalf("connected: %+v", d)
	}
	// OBS desconectado: nunca NO AR.
	if d := deviceTally(tally.State{Known: true, Program: true}, true, false); d.State != "unknown" || d.Program {
		t.Fatalf("unknown: %+v", d)
	}
}
