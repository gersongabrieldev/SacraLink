// Package control implementa o canal de controle entre o servidor e cada
// celular: WebSocket autenticado (TLS com pinning) com mensagens JSON
// versionadas. Contrato documentado em packages/contracts/control-v1.md.
package control

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
)

// ProtocolVersion do canal de controle.
const ProtocolVersion = 1

// Envelope é o formato de todas as mensagens.
type Envelope struct {
	V    int             `json:"v"`
	Type string          `json:"type"`
	ID   string          `json:"id,omitempty"`
	TS   int64           `json:"ts"`
	Data json.RawMessage `json:"data,omitempty"`
}

// Tipos de mensagem do aparelho para o servidor.
const (
	MsgHello         = "hello"
	MsgTelemetry     = "telemetry"
	MsgCommandResult = "command_result"
	MsgLocalChange   = "local_change"
	MsgPing          = "ping"
)

// Tipos de mensagem do servidor para o aparelho.
const (
	MsgHelloAck = "hello_ack"
	MsgTally    = "tally"
	MsgCommand  = "command"
	MsgConfig   = "config"
	MsgPong     = "pong"
	MsgError    = "error"
)

// Lens é uma lente/câmera física reportada pelo aparelho.
type Lens struct {
	ID      string  `json:"id"`
	Facing  string  `json:"facing"` // back | front | external
	Kind    string  `json:"kind"`   // wide | ultra_wide | telephoto | front | unknown
	Label   string  `json:"label"`
	MinZoom float64 `json:"minZoom"`
	MaxZoom float64 `json:"maxZoom"`
	// OpticalZoomFactors lista fatores ópticos reais (ex.: [0.5, 1, 3]) quando
	// a plataforma informa; vazio = desconhecido (nunca prometer zoom óptico).
	OpticalZoomFactors []float64 `json:"opticalZoomFactors,omitempty"`
}

// Capabilities são as capacidades REAIS do aparelho.
type Capabilities struct {
	Lenses        []Lens `json:"lenses"`
	Torch         bool   `json:"torch"`
	FocusPoint    bool   `json:"focusPoint"`
	ExposurePoint bool   `json:"exposurePoint"`
	MaxWidth      int    `json:"maxWidth"`
	MaxHeight     int    `json:"maxHeight"`
	MaxFPS        int    `json:"maxFps"`
	HardwareH264  bool   `json:"hardwareH264"`
	Thermal       bool   `json:"thermal"`
}

// Hello é a primeira mensagem do aparelho após conectar.
type Hello struct {
	AppVersion   string       `json:"appVersion"`
	Platform     string       `json:"platform"`
	Model        string       `json:"model"`
	OSVersion    string       `json:"osVersion"`
	Capabilities Capabilities `json:"capabilities"`
}

// HelloAck é a resposta do servidor ao Hello.
type HelloAck struct {
	SessionID    string         `json:"sessionId"`
	DeviceID     string         `json:"deviceId"`
	CameraID     string         `json:"cameraId"`
	CameraName   string         `json:"cameraName"`
	Slot         int            `json:"slot"`
	ServerName   string         `json:"serverName"`
	WHIPPath     string         `json:"whipPath"`
	Profile      map[string]any `json:"profile"`
	HeartbeatSec int            `json:"heartbeatSec"`
}

// Telemetry é enviada periodicamente pelo aparelho.
type Telemetry struct {
	Battery *struct {
		Level    float64 `json:"level"` // 0..1
		Charging bool    `json:"charging"`
	} `json:"battery,omitempty"`
	Thermal string `json:"thermal,omitempty"` // nominal | fair | serious | critical
	Stream  *struct {
		State         string  `json:"state"` // idle | connecting | live | reconnecting | error
		BitrateKbps   float64 `json:"bitrateKbps"`
		FPS           float64 `json:"fps"`
		Width         int     `json:"width"`
		Height        int     `json:"height"`
		PacketsLost   int64   `json:"packetsLost"`
		RTTMs         float64 `json:"rttMs"`
		JitterMs      float64 `json:"jitterMs"`
		AvailableKbps float64 `json:"availableKbps"`
	} `json:"stream,omitempty"`
	Camera  *CameraTelemetry `json:"camera,omitempty"`
	Network *struct {
		Type    string `json:"type"` // wifi | cellular | ethernet | none
		Quality string `json:"quality,omitempty"`
	} `json:"network,omitempty"`
}

// CameraTelemetry é o estado atual da câmera no aparelho.
type CameraTelemetry struct {
	LensID   string  `json:"lensId"`
	Facing   string  `json:"facing"`
	Zoom     float64 `json:"zoom"`
	Torch    bool    `json:"torch"`
	MicMuted bool    `json:"micMuted"`
}

// LocalChange informa uma alteração feita pelo operador no próprio celular.
type LocalChange struct {
	Param string          `json:"param"`
	Value json.RawMessage `json:"value"`
}

// Tally é enviado ao aparelho sempre que o estado no OBS muda.
type Tally struct {
	State        string `json:"state"` // on_air | preview | connected | unknown
	Program      bool   `json:"program"`
	Preview      bool   `json:"preview"`
	Live         bool   `json:"live"` // OBS transmitindo ou gravando
	OBSConnected bool   `json:"obsConnected"`
	Seq          uint64 `json:"seq"`
}

// Command é um comando remoto do operador central.
type Command struct {
	CommandID string         `json:"commandId"`
	SessionID string         `json:"sessionId"`
	Name      string         `json:"name"`
	Params    map[string]any `json:"params"`
	IssuedAt  int64          `json:"issuedAt"`
	ExpiresAt int64          `json:"expiresAt"`
	// Force ignora a janela de prioridade local (confirmado pelo operador).
	Force bool `json:"force,omitempty"`
}

// Status de execução de comandos.
const (
	ResultExecuted    = "executed"
	ResultRejected    = "rejected"
	ResultUnsupported = "unsupported"
	ResultFailed      = "failed"
	ResultConflict    = "conflict" // operador do celular alterou há < 3 s
	ResultExpired     = "expired"
	ResultTimeout     = "timeout" // sem resposta do aparelho
	ResultOffline     = "offline" // aparelho desconectado
)

// CommandResult é a confirmação do aparelho.
type CommandResult struct {
	CommandID string         `json:"commandId"`
	Status    string         `json:"status"`
	Message   string         `json:"message,omitempty"`
	State     map[string]any `json:"state,omitempty"`
}

// Comandos suportados.
const (
	CmdZoomSet         = "zoom.set"
	CmdZoomStep        = "zoom.step"
	CmdLensSelect      = "lens.select"
	CmdCameraFlip      = "camera.flip"
	CmdFocusPoint      = "focus.point"
	CmdExposurePoint   = "exposure.point"
	CmdTorchSet        = "torch.set"
	CmdQualitySet      = "quality.set"
	CmdAudioMute       = "audio.mute"
	CmdStreamReconnect = "stream.reconnect"
	CmdPresetApply     = "preset.apply"
)

// ErrInvalidCommand indica comando ou parâmetros inválidos.
var ErrInvalidCommand = errors.New("comando inválido")

func num(p map[string]any, k string, min, max float64) error {
	v, ok := p[k].(float64)
	if !ok || math.IsNaN(v) || v < min || v > max {
		return fmt.Errorf("%w: %q deve ser número entre %g e %g", ErrInvalidCommand, k, min, max)
	}
	return nil
}

func str(p map[string]any, k string, maxLen int) error {
	v, ok := p[k].(string)
	if !ok || v == "" || len(v) > maxLen {
		return fmt.Errorf("%w: %q deve ser texto de até %d caracteres", ErrInvalidCommand, k, maxLen)
	}
	return nil
}

func boolean(p map[string]any, k string) error {
	if _, ok := p[k].(bool); !ok {
		return fmt.Errorf("%w: %q deve ser verdadeiro/falso", ErrInvalidCommand, k)
	}
	return nil
}

// ValidateCommand valida nome e parâmetros de um comando (lista de permissão).
func ValidateCommand(name string, p map[string]any) error {
	if p == nil {
		p = map[string]any{}
	}
	switch name {
	case CmdZoomSet:
		return num(p, "value", 0.1, 100)
	case CmdZoomStep:
		return num(p, "delta", -10, 10)
	case CmdLensSelect:
		return str(p, "lensId", 64)
	case CmdCameraFlip, CmdStreamReconnect:
		return nil
	case CmdFocusPoint, CmdExposurePoint:
		if err := num(p, "x", 0, 1); err != nil {
			return err
		}
		return num(p, "y", 0, 1)
	case CmdTorchSet:
		return boolean(p, "on")
	case CmdAudioMute:
		return boolean(p, "muted")
	case CmdQualitySet:
		if _, ok := p["profileId"]; ok {
			return str(p, "profileId", 32)
		}
		for _, f := range []struct {
			k        string
			min, max float64
		}{{"width", 160, 3840}, {"height", 120, 2160}, {"fps", 5, 60}, {"maxBitrateKbps", 150, 20000}} {
			if err := num(p, f.k, f.min, f.max); err != nil {
				return err
			}
		}
		return nil
	case CmdPresetApply:
		return str(p, "presetId", 32)
	}
	return fmt.Errorf("%w: %q não é suportado", ErrInvalidCommand, name)
}
