package testpub

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// RTSPProbe é o resultado de uma leitura RTSP, equivalente ao que a Media
// Source do OBS (FFmpeg) faz: DESCRIBE, SETUP (TCP intercalado) e PLAY.
type RTSPProbe struct {
	SDP     string
	Codecs  []string    // ex.: ["H264", "opus"]
	Packets map[int]int // pacotes RTP recebidos por faixa
}

type rtspConn struct {
	c    net.Conn
	r    *bufio.Reader
	seq  int
	sess string
}

func (rc *rtspConn) request(method, u string, headers map[string]string) (int, map[string]string, string, error) {
	rc.seq++
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s RTSP/1.0\r\nCSeq: %d\r\nUser-Agent: SacraLink-Probe\r\n", method, u, rc.seq)
	if rc.sess != "" {
		fmt.Fprintf(&b, "Session: %s\r\n", rc.sess)
	}
	for k, v := range headers {
		fmt.Fprintf(&b, "%s: %s\r\n", k, v)
	}
	b.WriteString("\r\n")
	if _, err := rc.c.Write([]byte(b.String())); err != nil {
		return 0, nil, "", err
	}
	return rc.readResponse()
}

func (rc *rtspConn) readResponse() (int, map[string]string, string, error) {
	for {
		// Descarta dados intercalados que cheguem antes da resposta.
		peek, err := rc.r.Peek(1)
		if err != nil {
			return 0, nil, "", err
		}
		if peek[0] == '$' {
			if _, _, err := rc.readInterleaved(); err != nil {
				return 0, nil, "", err
			}
			continue
		}
		break
	}
	line, err := rc.r.ReadString('\n')
	if err != nil {
		return 0, nil, "", err
	}
	parts := strings.SplitN(strings.TrimSpace(line), " ", 3)
	if len(parts) < 2 {
		return 0, nil, "", fmt.Errorf("resposta RTSP inválida: %q", line)
	}
	code, _ := strconv.Atoi(parts[1])
	hdr := map[string]string{}
	for {
		l, err := rc.r.ReadString('\n')
		if err != nil {
			return 0, nil, "", err
		}
		l = strings.TrimSpace(l)
		if l == "" {
			break
		}
		if k, v, ok := strings.Cut(l, ":"); ok {
			hdr[strings.ToLower(strings.TrimSpace(k))] = strings.TrimSpace(v)
		}
	}
	body := ""
	if n, _ := strconv.Atoi(hdr["content-length"]); n > 0 {
		buf := make([]byte, n)
		if _, err := io.ReadFull(rc.r, buf); err != nil {
			return 0, nil, "", err
		}
		body = string(buf)
	}
	return code, hdr, body, nil
}

func (rc *rtspConn) readInterleaved() (int, int, error) {
	var h [4]byte
	if _, err := io.ReadFull(rc.r, h[:]); err != nil {
		return 0, 0, err
	}
	n := int(binary.BigEndian.Uint16(h[2:]))
	if _, err := rc.r.Discard(n); err != nil {
		return 0, 0, err
	}
	return int(h[1]), n, nil
}

// ProbeRTSP lê o fluxo por dur e conta os pacotes RTP de cada faixa.
func ProbeRTSP(ctx context.Context, rawURL string, dur time.Duration) (*RTSPProbe, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	d := net.Dialer{Timeout: 3 * time.Second}
	c, err := d.DialContext(ctx, "tcp", u.Host)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(dur + 10*time.Second))
	rc := &rtspConn{c: c, r: bufio.NewReader(c)}

	code, hdr, sdp, err := rc.request("DESCRIBE", rawURL, map[string]string{"Accept": "application/sdp"})
	if err != nil {
		return nil, err
	}
	if code != 200 {
		return nil, fmt.Errorf("DESCRIBE: RTSP %d", code)
	}
	base := hdr["content-base"]
	if base == "" {
		base = rawURL
	}
	probe := &RTSPProbe{SDP: sdp, Packets: map[int]int{}}
	// Agrupa por seção m= (a ordem de a=control e a=rtpmap varia).
	var controls []string
	inMedia := false
	for _, l := range strings.Split(sdp, "\n") {
		l = strings.TrimSpace(l)
		switch {
		case strings.HasPrefix(l, "m="):
			inMedia = true
			controls = append(controls, "")
		case strings.HasPrefix(l, "a=rtpmap:") && inMedia:
			if _, enc, ok := strings.Cut(l, " "); ok {
				probe.Codecs = append(probe.Codecs, strings.Split(enc, "/")[0])
			}
		case strings.HasPrefix(l, "a=control:") && inMedia:
			ctl := strings.TrimPrefix(l, "a=control:")
			if !strings.HasPrefix(ctl, "rtsp://") {
				ctl = strings.TrimRight(base, "/") + "/" + ctl
			}
			controls[len(controls)-1] = ctl
		}
	}
	for _, c := range controls {
		if c == "" {
			return probe, errors.New("faixa sem a=control no SDP")
		}
	}
	if len(controls) == 0 {
		return probe, errors.New("SDP sem faixas")
	}
	for i, ctl := range controls {
		code, hdr, _, err := rc.request("SETUP", ctl, map[string]string{
			"Transport": fmt.Sprintf("RTP/AVP/TCP;unicast;interleaved=%d-%d", 2*i, 2*i+1),
		})
		if err != nil {
			return probe, err
		}
		if code != 200 {
			return probe, fmt.Errorf("SETUP %s: RTSP %d", ctl, code)
		}
		if rc.sess == "" {
			rc.sess = strings.Split(hdr["session"], ";")[0]
		}
	}
	if code, _, _, err := rc.request("PLAY", base, map[string]string{"Range": "npt=0.000-"}); err != nil || code != 200 {
		return probe, fmt.Errorf("PLAY: RTSP %d %v", code, err)
	}
	deadline := time.Now().Add(dur)
	c.SetReadDeadline(deadline)
	for time.Now().Before(deadline) {
		b, err := rc.r.Peek(1)
		if err != nil {
			break
		}
		if b[0] != '$' {
			if _, _, _, err := rc.readResponse(); err != nil {
				break
			}
			continue
		}
		ch, _, err := rc.readInterleaved()
		if err != nil {
			break
		}
		if ch%2 == 0 {
			probe.Packets[ch/2]++
		}
	}
	return probe, nil
}
