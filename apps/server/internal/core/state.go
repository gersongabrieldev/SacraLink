// Package core é o orquestrador do SacraLink: mantém o estado de cada câmera
// a partir das sessões de controle, do MediaMTX e do OBS; envia o tally aos
// celulares; despacha comandos; aplica limites e a política de fallback.
package core

import (
	"time"

	"github.com/gersongabrieldev/sacralink/apps/server/internal/control"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/obsbridge"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/store"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/tally"
)

// Estados de conexão da câmera (§5.3; definições técnicas no plano §6.3).
const (
	StateOffline      = "OFFLINE"
	StateConnecting   = "CONNECTING"
	StateConnected    = "CONNECTED"
	StateStreaming    = "STREAMING"
	StateReconnecting = "RECONNECTING"
	StateError        = "ERROR"
)

// Janelas de tempo da máquina de estados.
const (
	StreamStaleAfter = 3 * time.Second  // sem bytes novos → fluxo parado
	NegotiationGrace = 15 * time.Second // WHIP em negociação → CONNECTING
	ReconnectWindow  = 60 * time.Second // após queda → RECONNECTING
)

// stateInput são os fatos usados na derivação do estado.
type stateInput struct {
	now              time.Time
	errorReason      string
	controlConnected bool
	online           bool
	lastBytesAt      time.Time
	negotiatingAt    time.Time
	lostAt           time.Time
}

// deriveState calcula o estado de conexão. É uma função pura.
func deriveState(in stateInput) string {
	streaming := in.online && !in.lastBytesAt.IsZero() && in.now.Sub(in.lastBytesAt) < StreamStaleAfter
	switch {
	case in.errorReason != "":
		return StateError
	case streaming:
		return StateStreaming
	case !in.negotiatingAt.IsZero() && in.now.Sub(in.negotiatingAt) < NegotiationGrace && (in.controlConnected || in.online):
		return StateConnecting
	case !in.lostAt.IsZero() && in.now.Sub(in.lostAt) < ReconnectWindow:
		return StateReconnecting
	case in.controlConnected:
		return StateConnected
	}
	return StateOffline
}

// DeviceView resume o aparelho associado à câmera.
type DeviceView struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Platform   string `json:"platform"`
	Model      string `json:"model"`
	AppVersion string `json:"appVersion"`
	Status     string `json:"status"`
}

// CameraView é a representação da câmera para o painel e a API.
type CameraView struct {
	ID                string `json:"id"`
	Slot              int    `json:"slot"`
	Name              string `json:"name"`
	OperatorName      string `json:"operatorName"`
	ProfileID         string `json:"profileId"`
	IngestMode        string `json:"ingestMode"`
	IsBackup          bool   `json:"isBackup"`
	AudioSyncOffsetMs int    `json:"audioSyncOffsetMs"`
	MediaPath         string `json:"mediaPath"`

	Device *DeviceView `json:"device,omitempty"`

	State            string `json:"state"`
	Tally            string `json:"tally"` // on_air | preview | none | unknown
	Program          bool   `json:"program"`
	Preview          bool   `json:"preview"`
	ControlConnected bool   `json:"controlConnected"`
	ConnectedSince   int64  `json:"connectedSince,omitempty"`
	StreamingSince   int64  `json:"streamingSince,omitempty"`
	RemoteAddr       string `json:"remoteAddr,omitempty"`
	Error            string `json:"error,omitempty"`

	BitrateKbps float64 `json:"bitrateKbps"`
	Width       int     `json:"width,omitempty"`
	Height      int     `json:"height,omitempty"`
	FPS         float64 `json:"fps,omitempty"`
	VideoCodec  string  `json:"videoCodec,omitempty"`
	HasAudio    bool    `json:"hasAudio"`
	PacketsLost uint64  `json:"packetsLost"`
	JitterMs    float64 `json:"jitterMs,omitempty"`
	RTTMs       float64 `json:"rttMs,omitempty"`

	Battery  *float64 `json:"battery,omitempty"`
	Charging bool     `json:"charging"`
	Thermal  string   `json:"thermal,omitempty"`
	Network  string   `json:"network,omitempty"`
	Zoom     float64  `json:"zoom,omitempty"`
	LensID   string   `json:"lensId,omitempty"`
	Facing   string   `json:"facing,omitempty"`
	MicMuted bool     `json:"micMuted"`
	Torch    bool     `json:"torch"`

	Capabilities *control.Capabilities  `json:"capabilities,omitempty"`
	OBSSource    obsbridge.SourceStatus `json:"obsSource"`
	LastLocal    *LocalChangeView       `json:"lastLocalChange,omitempty"`
	LastCommand  *CommandView           `json:"lastCommand,omitempty"`
}

// LocalChangeView registra a última alteração feita no celular.
type LocalChangeView struct {
	Param string `json:"param"`
	At    int64  `json:"at"`
}

// CommandView registra o último comando remoto.
type CommandView struct {
	CommandID string `json:"commandId"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	Message   string `json:"message,omitempty"`
	At        int64  `json:"at"`
}

// camState é o estado interno mutável de uma câmera.
type camState struct {
	cam    store.Camera
	device *DeviceView

	controlConnected bool
	connectedSince   time.Time
	remoteAddr       string
	sessionID        string
	caps             *control.Capabilities
	tel              control.Telemetry

	online        bool
	lastBytes     uint64
	lastBytesAt   time.Time
	lastSampleAt  time.Time
	bitrateKbps   float64
	width, height int
	codec         string
	hasAudio      bool
	packetsLost   uint64
	jitterMs      float64
	negotiatingAt time.Time
	lostAt        time.Time
	streamingAt   time.Time
	streamSession string
	errorReason   string

	tally tally.State
	state string

	lastLocal   *LocalChangeView
	lastCommand *CommandView
	lastMetric  time.Time
}

func (c *camState) view(src obsbridge.SourceStatus) CameraView {
	v := CameraView{
		ID: c.cam.ID, Slot: c.cam.Slot, Name: c.cam.Name, OperatorName: c.cam.OperatorName,
		ProfileID: c.cam.ProfileID, IngestMode: c.cam.IngestMode, IsBackup: c.cam.IsBackup,
		AudioSyncOffsetMs: c.cam.AudioSyncOffsetMs, MediaPath: c.cam.MediaPath(),
		Device: c.device, State: c.state, Tally: c.tally.Label(), Program: c.tally.Program, Preview: c.tally.Preview,
		ControlConnected: c.controlConnected, RemoteAddr: c.remoteAddr, Error: c.errorReason,
		BitrateKbps: c.bitrateKbps, Width: c.width, Height: c.height, VideoCodec: c.codec, HasAudio: c.hasAudio,
		PacketsLost: c.packetsLost, JitterMs: c.jitterMs,
		Capabilities: c.caps, OBSSource: src, LastLocal: c.lastLocal, LastCommand: c.lastCommand,
	}
	if src.State == "" {
		v.OBSSource = obsbridge.SourceStatus{CameraID: c.cam.ID, State: "none"}
	}
	if !c.connectedSince.IsZero() && c.controlConnected {
		v.ConnectedSince = c.connectedSince.UnixMilli()
	}
	if !c.streamingAt.IsZero() && c.state == StateStreaming {
		v.StreamingSince = c.streamingAt.UnixMilli()
	}
	if t := c.tel; c.controlConnected {
		if t.Battery != nil {
			lvl := t.Battery.Level
			v.Battery, v.Charging = &lvl, t.Battery.Charging
		}
		v.Thermal = t.Thermal
		if t.Network != nil {
			v.Network = t.Network.Type
		}
		if t.Stream != nil {
			v.FPS, v.RTTMs = t.Stream.FPS, t.Stream.RTTMs
			if v.Width == 0 {
				v.Width, v.Height = t.Stream.Width, t.Stream.Height
			}
		}
		if t.Camera != nil {
			v.Zoom, v.LensID, v.Facing, v.MicMuted, v.Torch = t.Camera.Zoom, t.Camera.LensID, t.Camera.Facing, t.Camera.MicMuted, t.Camera.Torch
		}
	}
	return v
}

// deviceTally converte o tally do OBS na mensagem para o celular.
func deviceTally(st tally.State, live, obsConnected bool) control.Tally {
	t := control.Tally{Program: st.Program, Preview: st.Preview, Live: live, OBSConnected: obsConnected}
	switch {
	case !obsConnected || !st.Known:
		t.State, t.Program, t.Preview = "unknown", false, false
	case st.Program:
		t.State = "on_air"
	case st.Preview:
		t.State = "preview"
	default:
		t.State = "connected"
	}
	return t
}
