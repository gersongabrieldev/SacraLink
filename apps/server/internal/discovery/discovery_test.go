package discovery

import (
	"slices"
	"strings"
	"testing"
)

func TestTXTSemSegredos(t *testing.T) {
	txt := TXT(Info{ServerID: "s123", ServerName: "Paróquia", Port: 7443, FingerprintHex: "ab"})
	if !slices.Contains(txt, "id=s123") || !slices.Contains(txt, "v=1") {
		t.Fatalf("TXT: %v", txt)
	}
	for _, r := range txt {
		if strings.Contains(r, "token") || strings.Contains(r, "secret") || strings.Contains(r, "pass") {
			t.Fatalf("TXT com dado sensível: %v", txt)
		}
	}
}

func TestInstanceName(t *testing.T) {
	if got := InstanceName("Paróquia São José"); got != "SacraLink Paróquia São José" {
		t.Fatalf("nome: %q", got)
	}
	if got := InstanceName("a.b\\c\n"); strings.ContainsAny(got, ".\\\n") {
		t.Fatalf("caracteres inválidos: %q", got)
	}
	if got := InstanceName(strings.Repeat("ç", 80)); len(got) > 63 {
		t.Fatalf("nome longo demais: %d bytes", len(got))
	}
}
