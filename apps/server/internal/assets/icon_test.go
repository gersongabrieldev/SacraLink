package assets

import (
	"bytes"
	"encoding/binary"
	"image/png"
	"testing"
)

func TestLogoPNG(t *testing.T) {
	img, err := png.Decode(bytes.NewReader(PNG(64, true)))
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 64 {
		t.Fatal("tamanho incorreto")
	}
	// Centro da cruz deve ser branco; canto deve ser transparente.
	if r, g, b, _ := img.At(32, 32).RGBA(); r>>8 < 240 || g>>8 < 240 || b>>8 < 240 {
		t.Fatalf("centro não é branco: %v", img.At(32, 32))
	}
	if _, _, _, a := img.At(0, 0).RGBA(); a != 0 {
		t.Fatal("canto deveria ser transparente (cantos arredondados)")
	}
}

func TestICO(t *testing.T) {
	ico := ICO(false, 16, 32, 256)
	var hdr [3]uint16
	binary.Read(bytes.NewReader(ico), binary.LittleEndian, &hdr)
	if hdr[1] != 1 || hdr[2] != 3 {
		t.Fatalf("cabeçalho ICO: %v", hdr)
	}
}
