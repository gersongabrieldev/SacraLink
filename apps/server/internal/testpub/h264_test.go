package testpub

import (
	"bytes"
	"testing"
)

func TestExpGolomb(t *testing.T) {
	var w bitWriter
	w.ue(0) // 1
	w.ue(1) // 010
	w.ue(2) // 011
	w.ue(3) // 00100
	w.bits(0, 7)
	// 1 010 011 00100 0000000 → 1010 0110 0100 0000 0000
	if got := w.bytes(); !bytes.Equal(got[:2], []byte{0xA6, 0x40}) {
		t.Fatalf("bits: %x", got)
	}
}

func TestPrevencaoDeEmulacao(t *testing.T) {
	got := nal(0x65, []byte{0, 0, 1, 0, 0, 0, 5})
	want := []byte{0, 0, 0, 1, 0x65, 0, 0, 3, 1, 0, 0, 3, 0, 5}
	if !bytes.Equal(got, want) {
		t.Fatalf("emulação: %x", got)
	}
}

func TestQuadroContemSPSPPSIDR(t *testing.T) {
	e := NewEncoder(160, 96)
	f := e.Frame()
	types := []byte{}
	for i := 0; i+4 < len(f); i++ {
		if f[i] == 0 && f[i+1] == 0 && f[i+2] == 0 && f[i+3] == 1 {
			types = append(types, f[i+4]&0x1f)
		}
	}
	if !bytes.Equal(types, []byte{7, 8, 5}) {
		t.Fatalf("NALs: %v", types)
	}
	// 60 macroblocos × 384 bytes PCM ≈ 23 KB.
	if len(f) < 60*384 || len(f) > 60*400 {
		t.Fatalf("tamanho inesperado: %d", len(f))
	}
}
