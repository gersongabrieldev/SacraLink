package secure

import (
	"regexp"
	"testing"
)

func TestNewIDFormato(t *testing.T) {
	re := regexp.MustCompile(`^c[a-z2-7]{11}$`)
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		id := NewID("c", 11)
		if !re.MatchString(id) {
			t.Fatalf("id fora do formato: %q", id)
		}
		if seen[id] {
			t.Fatalf("id repetido: %q", id)
		}
		seen[id] = true
	}
}

func TestHashSecretDeterministico(t *testing.T) {
	a := HashSecret("abc")
	if a != HashSecret("abc") {
		t.Fatal("hash não determinístico")
	}
	if EqualHash(a, HashSecret("abd")) {
		t.Fatal("hashes diferentes considerados iguais")
	}
	if !EqualHash(a, HashSecret("abc")) {
		t.Fatal("hashes iguais considerados diferentes")
	}
}

func TestSenha(t *testing.T) {
	if _, err := HashPassword("curta"); err != ErrPasswordTooShort {
		t.Fatalf("esperava ErrPasswordTooShort, obtive %v", err)
	}
	h, err := HashPassword("senha-forte-123")
	if err != nil {
		t.Fatal(err)
	}
	if !CheckPassword(h, "senha-forte-123") {
		t.Fatal("senha correta rejeitada")
	}
	if CheckPassword(h, "senha-errada-123") {
		t.Fatal("senha errada aceita")
	}
}

func TestFingerprint(t *testing.T) {
	fp := Fingerprint([]byte("x"))
	if len(fp) != 32*3-1 {
		t.Fatalf("tamanho inesperado: %d", len(fp))
	}
}
