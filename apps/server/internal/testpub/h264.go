// Package testpub simula um celular SacraLink: pareamento, canal de controle
// e publicação WHIP de vídeo H.264 + áudio Opus sintéticos. É usado nos
// testes ponta a ponta e na ferramenta de carga (cmd/sl-testpublisher).
//
// NÃO substitui testes com aparelhos reais: o codificador abaixo gera um
// H.264 válido porém sem compressão (macroblocos I_PCM).
package testpub

// bitWriter escreve bits MSB-first.
type bitWriter struct {
	buf  []byte
	cur  byte
	nbit uint
}

func (w *bitWriter) bit(b uint) {
	w.cur = w.cur<<1 | byte(b&1)
	w.nbit++
	if w.nbit == 8 {
		w.buf = append(w.buf, w.cur)
		w.cur, w.nbit = 0, 0
	}
}

func (w *bitWriter) bits(v uint64, n uint) {
	for i := int(n) - 1; i >= 0; i-- {
		w.bit(uint(v>>uint(i)) & 1)
	}
}

// ue escreve Exp-Golomb sem sinal.
func (w *bitWriter) ue(v uint64) {
	x := v + 1
	n := uint(0)
	for t := x; t > 1; t >>= 1 {
		n++
	}
	w.bits(0, n)
	w.bits(x, n+1)
}

// se escreve Exp-Golomb com sinal.
func (w *bitWriter) se(v int64) {
	if v <= 0 {
		w.ue(uint64(-2 * v))
	} else {
		w.ue(uint64(2*v - 1))
	}
}

func (w *bitWriter) align() {
	for w.nbit != 0 {
		w.bit(0)
	}
}

func (w *bitWriter) trailing() {
	w.bit(1)
	w.align()
}

func (w *bitWriter) bytes() []byte { return w.buf }

// nal monta uma NAL unit Annex B com prevenção de emulação.
func nal(header byte, rbsp []byte) []byte {
	out := []byte{0, 0, 0, 1, header}
	zeros := 0
	for _, b := range rbsp {
		if zeros >= 2 && b <= 3 {
			out = append(out, 3)
			zeros = 0
		}
		out = append(out, b)
		if b == 0 {
			zeros++
		} else {
			zeros = 0
		}
	}
	return out
}

// Encoder gera quadros H.264 Constrained Baseline com macroblocos I_PCM.
type Encoder struct {
	mbW, mbH int
	idrID    uint64
	frame    int
	sps, pps []byte
}

// NewEncoder cria um codificador para width×height (múltiplos de 16).
func NewEncoder(width, height int) *Encoder {
	e := &Encoder{mbW: width / 16, mbH: height / 16}
	var s bitWriter
	s.bits(66, 8)   // profile_idc: Baseline
	s.bits(0xC0, 8) // constraint_set0/1: Constrained Baseline
	s.bits(30, 8)   // level 3.0
	s.ue(0)         // seq_parameter_set_id
	s.ue(0)         // log2_max_frame_num_minus4
	s.ue(2)         // pic_order_cnt_type
	s.ue(1)         // max_num_ref_frames
	s.bit(0)        // gaps_in_frame_num_value_allowed_flag
	s.ue(uint64(e.mbW - 1))
	s.ue(uint64(e.mbH - 1))
	s.bit(1) // frame_mbs_only_flag
	s.bit(1) // direct_8x8_inference_flag
	s.bit(0) // frame_cropping_flag
	s.bit(0) // vui_parameters_present_flag
	s.trailing()
	e.sps = nal(0x67, s.bytes())

	var p bitWriter
	p.ue(0)      // pic_parameter_set_id
	p.ue(0)      // seq_parameter_set_id
	p.bit(0)     // entropy_coding_mode_flag (CAVLC)
	p.bit(0)     // bottom_field_pic_order_in_frame_present_flag
	p.ue(0)      // num_slice_groups_minus1
	p.ue(0)      // num_ref_idx_l0_default_active_minus1
	p.ue(0)      // num_ref_idx_l1_default_active_minus1
	p.bit(0)     // weighted_pred_flag
	p.bits(0, 2) // weighted_bipred_idc
	p.se(0)      // pic_init_qp_minus26
	p.se(0)      // pic_init_qs_minus26
	p.se(0)      // chroma_qp_index_offset
	p.bit(0)     // deblocking_filter_control_present_flag
	p.bit(0)     // constrained_intra_pred_flag
	p.bit(0)     // redundant_pic_cnt_present_flag
	p.trailing()
	e.pps = nal(0x68, p.bytes())
	return e
}

// Frame gera o próximo quadro (SPS + PPS + IDR) em Annex B. A imagem é um
// degradê que se desloca a cada quadro, útil para conferir movimento no OBS.
func (e *Encoder) Frame() []byte {
	var s bitWriter
	s.ue(0)      // first_mb_in_slice
	s.ue(7)      // slice_type: I (todas as fatias)
	s.ue(0)      // pic_parameter_set_id
	s.bits(0, 4) // frame_num (IDR)
	s.ue(e.idrID % 2)
	s.bit(0) // no_output_of_prior_pics_flag
	s.bit(0) // long_term_reference_flag
	s.se(0)  // slice_qp_delta
	shift := e.frame * 4
	for my := 0; my < e.mbH; my++ {
		for mx := 0; mx < e.mbW; mx++ {
			s.ue(25) // mb_type I_PCM
			s.align()
			for y := 0; y < 16; y++ {
				for x := 0; x < 16; x++ {
					// Valores em 16..235 evitam sequências 0x000000 no PCM.
					s.bits(uint64(16+((mx*16+x+shift)%220)), 8)
				}
			}
			for c := 0; c < 2; c++ {
				for i := 0; i < 64; i++ {
					s.bits(uint64(128+c*40-my*6), 8)
				}
			}
		}
	}
	s.trailing()
	e.idrID++
	e.frame++
	out := append([]byte{}, e.sps...)
	out = append(out, e.pps...)
	return append(out, nal(0x65, s.bytes())...)
}
