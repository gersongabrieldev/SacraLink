// Package whip implementa o proxy de sinalização WHIP/WHEP entre os clientes
// (celulares e painel/OBS) e o MediaMTX restrito ao loopback (ADR-0003).
// Só o SDP passa pelo proxy; a mídia vai direto à porta ICE do MediaMTX.
package whip

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// MaxSDPBytes limita o tamanho de ofertas SDP e fragmentos ICE.
const MaxSDPBytes = 64 << 10

// Mode indica publicação (WHIP) ou leitura (WHEP).
type Mode string

// Modos.
const (
	Publish Mode = "whip"
	Read    Mode = "whep"
)

// ErrUnknownSession indica sessão inexistente ou de outra câmera.
var ErrUnknownSession = errors.New("sessão WHIP/WHEP desconhecida")

// Proxy encaminha a sinalização ao MediaMTX.
type Proxy struct {
	base     string // http://127.0.0.1:8889
	user     string // credencial interna de publicação
	pass     string
	http     *http.Client
	mu       sync.Mutex
	sessions map[string]session // id externo → sessão
}

type session struct {
	cameraID string
	mode     Mode
	location string // URL interna do MediaMTX
	created  time.Time
}

// New cria o proxy.
func New(mediaBase, user, pass string) *Proxy {
	return &Proxy{base: strings.TrimRight(mediaBase, "/"), user: user, pass: pass,
		http: &http.Client{Timeout: 15 * time.Second}, sessions: map[string]session{}}
}

// Response é a resposta a repassar ao cliente.
type Response struct {
	Status  int
	Header  http.Header
	Body    []byte
	Session string // id externo da sessão criada (Offer)
}

var passHeaders = []string{"Content-Type", "ETag", "Link", "Accept-Patch"}

// Offer encaminha a oferta SDP. basePath é o prefixo externo usado para
// reescrever Location (ex.: "/device/v1/whip/sessions/").
func (p *Proxy) Offer(ctx context.Context, mode Mode, cameraID, mediaPath string, sdp []byte, clientIP, basePath string) (*Response, error) {
	if len(sdp) == 0 || len(sdp) > MaxSDPBytes || !bytes.HasPrefix(bytes.TrimSpace(sdp), []byte("v=0")) {
		return nil, errors.New("oferta SDP inválida")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.base+"/"+mediaPath+"/"+string(mode), bytes.NewReader(sdp))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/sdp")
	if clientIP != "" {
		req.Header.Set("X-Forwarded-For", clientIP)
	}
	if mode == Publish {
		req.SetBasicAuth(p.user, p.pass)
	}
	resp, err := p.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("servidor de mídia indisponível: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, MaxSDPBytes))
	out := &Response{Status: resp.StatusCode, Header: http.Header{}, Body: body}
	for _, h := range passHeaders {
		for _, v := range resp.Header.Values(h) {
			out.Header.Add(h, v)
		}
	}
	if resp.StatusCode == http.StatusCreated {
		loc := resp.Header.Get("Location")
		if loc == "" {
			return nil, errors.New("servidor de mídia não retornou Location")
		}
		id := loc[strings.LastIndex(loc, "/")+1:]
		if !strings.HasPrefix(loc, "http") {
			loc = p.base + loc
		}
		p.mu.Lock()
		p.gcLocked()
		p.sessions[id] = session{cameraID: cameraID, mode: mode, location: loc, created: time.Now()}
		p.mu.Unlock()
		out.Session = id
		out.Header.Set("Location", basePath+id)
	}
	return out, nil
}

// gcLocked descarta sessões antigas (o MediaMTX encerra sessões órfãs).
func (p *Proxy) gcLocked() {
	for id, s := range p.sessions {
		if time.Since(s.created) > 12*time.Hour {
			delete(p.sessions, id)
		}
	}
}

func (p *Proxy) lookup(id, cameraID string, mode Mode) (session, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	s, ok := p.sessions[id]
	if !ok || s.mode != mode || (cameraID != "" && s.cameraID != cameraID) {
		return session{}, ErrUnknownSession
	}
	return s, nil
}

// Patch encaminha trickle ICE / ICE restart (RFC 9725).
func (p *Proxy) Patch(ctx context.Context, mode Mode, id, cameraID string, body []byte, contentType, ifMatch string) (*Response, error) {
	s, err := p.lookup(id, cameraID, mode)
	if err != nil {
		return nil, err
	}
	if len(body) > MaxSDPBytes {
		return nil, errors.New("fragmento ICE muito grande")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, s.location, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	if ifMatch != "" {
		req.Header.Set("If-Match", ifMatch)
	}
	if mode == Publish {
		req.SetBasicAuth(p.user, p.pass)
	}
	resp, err := p.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	rb, _ := io.ReadAll(io.LimitReader(resp.Body, MaxSDPBytes))
	out := &Response{Status: resp.StatusCode, Header: http.Header{}, Body: rb}
	for _, h := range passHeaders {
		if v := resp.Header.Get(h); v != "" {
			out.Header.Set(h, v)
		}
	}
	return out, nil
}

// Delete encerra a sessão.
func (p *Proxy) Delete(ctx context.Context, mode Mode, id, cameraID string) (int, error) {
	s, err := p.lookup(id, cameraID, mode)
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, s.location, nil)
	if err != nil {
		return 0, err
	}
	if mode == Publish {
		req.SetBasicAuth(p.user, p.pass)
	}
	resp, err := p.http.Do(req)
	if err != nil {
		return 0, err
	}
	resp.Body.Close()
	p.mu.Lock()
	delete(p.sessions, id)
	p.mu.Unlock()
	return resp.StatusCode, nil
}

// Write escreve a resposta no ResponseWriter.
func (r *Response) Write(w http.ResponseWriter) {
	for k, vs := range r.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(r.Status)
	w.Write(r.Body)
}
