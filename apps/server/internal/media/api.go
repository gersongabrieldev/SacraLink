package media

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// API é um cliente da API de controle do MediaMTX (v3, loopback).
type API struct {
	base string
	http *http.Client
}

// NewAPI cria o cliente para a porta informada em 127.0.0.1.
func NewAPI(port int) *API {
	return &API{base: "http://127.0.0.1:" + strconv.Itoa(port), http: &http.Client{Timeout: 3 * time.Second}}
}

// NewAPIWithBase cria o cliente para uma URL base (testes).
func NewAPIWithBase(base string) *API {
	return &API{base: base, http: &http.Client{Timeout: 3 * time.Second}}
}

// Track é uma faixa de mídia de um path (tracks2 na v1.21).
type Track struct {
	Codec      string `json:"codec"`
	CodecProps *struct {
		Width        int    `json:"width,omitempty"`
		Height       int    `json:"height,omitempty"`
		Profile      string `json:"profile,omitempty"`
		Level        string `json:"level,omitempty"`
		SampleRate   int    `json:"sampleRate,omitempty"`
		ChannelCount int    `json:"channelCount,omitempty"`
	} `json:"codecProps,omitempty"`
}

// Path é o estado de um path no MediaMTX.
type Path struct {
	Name          string  `json:"name"`
	Online        bool    `json:"online"`
	OnlineTime    *string `json:"onlineTime"`
	Available     bool    `json:"available"`
	InboundBytes  uint64  `json:"inboundBytes"`
	OutboundBytes uint64  `json:"outboundBytes"`
	Tracks        []Track `json:"tracks2"`
	Source        *struct {
		ID   string `json:"id"`
		Type string `json:"type"`
	} `json:"source"`
	Readers []struct {
		ID   string `json:"id"`
		Type string `json:"type"`
	} `json:"readers"`
	// Campos legados (versões anteriores do MediaMTX).
	Ready         bool   `json:"ready"`
	BytesReceived uint64 `json:"bytesReceived"`
}

// IsOnline considera o campo novo e o legado.
func (p *Path) IsOnline() bool { return p.Online || p.Ready }

// Bytes retorna os bytes recebidos (campo novo ou legado).
func (p *Path) Bytes() uint64 {
	if p.InboundBytes > 0 {
		return p.InboundBytes
	}
	return p.BytesReceived
}

// Video retorna a primeira faixa de vídeo, se houver.
func (p *Path) Video() *Track {
	for i := range p.Tracks {
		switch p.Tracks[i].Codec {
		case "H264", "H265", "VP8", "VP9", "AV1":
			return &p.Tracks[i]
		}
	}
	return nil
}

// HasAudio informa se o path tem faixa de áudio.
func (p *Path) HasAudio() bool {
	for _, t := range p.Tracks {
		switch t.Codec {
		case "Opus", "MPEG-4 Audio", "G711", "LPCM", "MPEG4Audio", "AC3":
			return true
		}
	}
	return false
}

// WebRTCSession é uma sessão WebRTC (publicação ou leitura).
type WebRTCSession struct {
	ID                        string  `json:"id"`
	Created                   string  `json:"created"`
	RemoteAddr                string  `json:"remoteAddr"`
	PeerConnectionEstablished bool    `json:"peerConnectionEstablished"`
	LocalCandidate            string  `json:"localCandidate"`
	RemoteCandidate           string  `json:"remoteCandidate"`
	State                     string  `json:"state"` // read | publish
	Path                      string  `json:"path"`
	InboundBytes              uint64  `json:"inboundBytes"`
	InboundRTPPackets         uint64  `json:"inboundRTPPackets"`
	InboundRTPPacketsLost     uint64  `json:"inboundRTPPacketsLost"`
	InboundRTPPacketsJitter   float64 `json:"inboundRTPPacketsJitter"`
	OutboundBytes             uint64  `json:"outboundBytes"`
	UserAgent                 string  `json:"userAgent"`
}

func (a *API) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.base+path, nil)
	if err != nil {
		return err
	}
	resp, err := a.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("mediamtx: GET %s: HTTP %d", path, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// Paths lista todos os paths (com paginação).
func (a *API) Paths(ctx context.Context) ([]Path, error) {
	var all []Path
	for page := 0; ; page++ {
		var r struct {
			PageCount int    `json:"pageCount"`
			Items     []Path `json:"items"`
		}
		if err := a.get(ctx, "/v3/paths/list?itemsPerPage=100&page="+strconv.Itoa(page), &r); err != nil {
			return nil, err
		}
		all = append(all, r.Items...)
		if page+1 >= r.PageCount {
			return all, nil
		}
	}
}

// WebRTCSessions lista as sessões WebRTC ativas.
func (a *API) WebRTCSessions(ctx context.Context) ([]WebRTCSession, error) {
	var all []WebRTCSession
	for page := 0; ; page++ {
		var r struct {
			PageCount int             `json:"pageCount"`
			Items     []WebRTCSession `json:"items"`
		}
		if err := a.get(ctx, "/v3/webrtc/sessions/list?itemsPerPage=100&page="+strconv.Itoa(page), &r); err != nil {
			return nil, err
		}
		all = append(all, r.Items...)
		if page+1 >= r.PageCount {
			return all, nil
		}
	}
}

// KickWebRTCSession encerra uma sessão WebRTC (ex.: dispositivo revogado).
func (a *API) KickWebRTCSession(ctx context.Context, id string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.base+"/v3/webrtc/sessions/kick/"+url.PathEscape(id), nil)
	if err != nil {
		return err
	}
	resp, err := a.http.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("mediamtx: kick %s: HTTP %d", id, resp.StatusCode)
	}
	return nil
}

// Ping verifica se a API responde.
func (a *API) Ping(ctx context.Context) error {
	var r struct {
		ItemCount int `json:"itemCount"`
	}
	return a.get(ctx, "/v3/paths/list?itemsPerPage=1", &r)
}
