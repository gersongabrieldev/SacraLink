// Package assets gera a identidade visual do SacraLink em código (ícone da
// bandeja, .ico e PNG do aplicativo), evitando binários opacos no repositório.
package assets

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"math"
)

// Cores da identidade (iguais a assets/logo.svg).
var (
	Background = color.RGBA{0x14, 0x18, 0x21, 0xff}
	Gold       = color.RGBA{0xF0, 0xA8, 0x3A, 0xff}
	White      = color.RGBA{0xff, 0xff, 0xff, 0xff}
	Live       = color.RGBA{0xE5, 0x48, 0x4D, 0xff}
)

// Logo desenha o logotipo em size×size pixels (com antisserrilhamento).
// live=true acrescenta o ponto vermelho de "ao vivo".
func Logo(size int, live bool) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	s := float64(size) / 128
	const ss = 4 // supersampling
	for py := 0; py < size; py++ {
		for px := 0; px < size; px++ {
			var acc [4]float64 // bg, gold, white, red coverage
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					x := (float64(px) + (float64(sx)+0.5)/ss) / s
					y := (float64(py) + (float64(sy)+0.5)/ss) / s
					switch {
					case inCross(x, y):
						acc[2]++
					case live && math.Hypot(x-64, y-104) <= 6:
						acc[3]++
					case inArcs(x, y):
						acc[1]++
					case inRoundRect(x, y, 0, 0, 128, 128, 28):
						acc[0]++
					}
				}
			}
			n := float64(ss * ss)
			total := acc[0] + acc[1] + acc[2] + acc[3]
			if total == 0 {
				continue
			}
			cols := [4]color.RGBA{Background, Gold, White, Live}
			var r, g, b float64
			for i, c := range cols {
				r += acc[i] * float64(c.R)
				g += acc[i] * float64(c.G)
				b += acc[i] * float64(c.B)
			}
			alpha := total / n
			// image.RGBA usa alfa pré-multiplicado.
			img.SetRGBA(px, py, color.RGBA{uint8(r / total * alpha), uint8(g / total * alpha), uint8(b / total * alpha), uint8(255 * alpha)})
		}
	}
	return img
}

func inRoundRect(x, y, x0, y0, w, h, r float64) bool {
	if x < x0 || y < y0 || x > x0+w || y > y0+h {
		return false
	}
	cx := math.Max(x0+r, math.Min(x, x0+w-r))
	cy := math.Max(y0+r, math.Min(y, y0+h-r))
	return math.Hypot(x-cx, y-cy) <= r
}

func inCross(x, y float64) bool {
	return inRoundRect(x, y, 59, 34, 10, 60, 3) || inRoundRect(x, y, 48, 49, 32, 10, 3)
}

// inArcs desenha os dois pares de arcos de "sinal" ao redor da cruz.
func inArcs(x, y float64) bool {
	for _, a := range []struct{ r, w float64 }{{24, 7}, {40, 7}} {
		d := math.Hypot(x-64, y-64)
		if math.Abs(d-a.r) > a.w/2 {
			continue
		}
		ang := math.Abs(math.Atan2(y-64, x-64)) // 0 = direita, π = esquerda
		if ang < math.Pi/4.2 || ang > math.Pi-math.Pi/4.2 {
			return true
		}
	}
	return false
}

// PNG codifica o logotipo em PNG.
func PNG(size int, live bool) []byte {
	var b bytes.Buffer
	_ = png.Encode(&b, Logo(size, live))
	return b.Bytes()
}

// ICO gera um arquivo .ico com imagens PNG embutidas (Windows Vista+).
func ICO(live bool, sizes ...int) []byte {
	if len(sizes) == 0 {
		sizes = []int{16, 24, 32, 48, 64, 256}
	}
	pngs := make([][]byte, len(sizes))
	for i, s := range sizes {
		pngs[i] = PNG(s, live)
	}
	var b bytes.Buffer
	binary.Write(&b, binary.LittleEndian, [3]uint16{0, 1, uint16(len(sizes))})
	offset := 6 + 16*len(sizes)
	for i, s := range sizes {
		dim := byte(s)
		if s >= 256 {
			dim = 0
		}
		b.Write([]byte{dim, dim, 0, 0})
		binary.Write(&b, binary.LittleEndian, uint16(1))  // planes
		binary.Write(&b, binary.LittleEndian, uint16(32)) // bpp
		binary.Write(&b, binary.LittleEndian, uint32(len(pngs[i])))
		binary.Write(&b, binary.LittleEndian, uint32(offset))
		offset += len(pngs[i])
	}
	for _, p := range pngs {
		b.Write(p)
	}
	return b.Bytes()
}
