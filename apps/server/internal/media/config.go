// Package media gerencia o MediaMTX: gera a configuração (restrita ao
// loopback, ADR-0003), supervisiona o processo e consulta sua API.
package media

import (
	"bytes"
	"fmt"
	"net"
	"strconv"

	"gopkg.in/yaml.v3"
)

// PathPattern é a expressão regular dos paths de câmeras do SacraLink.
const PathPattern = "~^sl/[a-z0-9]{8,32}$"

// Ports são as portas usadas pelo MediaMTX.
type Ports struct {
	RTSP   int // loopback
	WebRTC int // HTTP WHIP/WHEP, loopback
	ICE    int // UDP/TCP ICE, exposta
	API    int // loopback
}

// DefaultPorts são as portas padrão (docs/architecture/00-plano-tecnico.md §4).
var DefaultPorts = Ports{RTSP: 8554, WebRTC: 8889, ICE: 8189, API: 9997}

// Config descreve a configuração gerada.
type Config struct {
	Ports         Ports
	PublishUser   string
	PublishPass   string
	ICEListenHost string   // "" = todas as interfaces; "127.0.0.1" em testes
	ICETCP        bool     // habilitar ICE sobre TCP (fallback para redes que bloqueiam UDP)
	AdditionalIPs []string // IP público / DDNS quando o acesso externo está habilitado
	STUNServers   []string // opcional: apenas descoberta, nunca relay
	LogLevel      string
	IPsFromIfaces bool
}

type permission struct {
	Action string `yaml:"action"`
	Path   string `yaml:"path,omitempty"`
}

type internalUser struct {
	User        string       `yaml:"user"`
	Pass        string       `yaml:"pass"`
	IPs         []string     `yaml:"ips"`
	Permissions []permission `yaml:"permissions"`
}

type iceServer struct {
	URL string `yaml:"url"`
}

type pathConf struct {
	OverridePublisher bool `yaml:"overridePublisher"`
}

// file espelha as chaves do mediamtx.yml v1.21 (ordem preservada).
type file struct {
	LogLevel        string         `yaml:"logLevel"`
	LogDestinations []string       `yaml:"logDestinations"`
	LogStructured   bool           `yaml:"logStructured"`
	ReadTimeout     string         `yaml:"readTimeout"`
	WriteTimeout    string         `yaml:"writeTimeout"`
	WriteQueueSize  int            `yaml:"writeQueueSize"`
	UDPReadBuffer   int            `yaml:"udpReadBufferSize"`
	AuthMethod      string         `yaml:"authMethod"`
	AuthUsers       []internalUser `yaml:"authInternalUsers"`

	API            bool     `yaml:"api"`
	APIAddress     string   `yaml:"apiAddress"`
	Metrics        bool     `yaml:"metrics"`
	PProf          bool     `yaml:"pprof"`
	Playback       bool     `yaml:"playback"`
	RTSP           bool     `yaml:"rtsp"`
	RTSPTransports []string `yaml:"rtspTransports"`
	RTSPEncryption string   `yaml:"rtspEncryption"`
	RTSPAddress    string   `yaml:"rtspAddress"`
	RTMP           bool     `yaml:"rtmp"`
	HLS            bool     `yaml:"hls"`
	SRT            bool     `yaml:"srt"`
	MoQ            bool     `yaml:"moq"`

	WebRTC               bool        `yaml:"webrtc"`
	WebRTCAddress        string      `yaml:"webrtcAddress"`
	WebRTCEncryption     bool        `yaml:"webrtcEncryption"`
	WebRTCAllowOrigins   []string    `yaml:"webrtcAllowOrigins"`
	WebRTCTrustedProxies []string    `yaml:"webrtcTrustedProxies"`
	WebRTCLocalUDP       string      `yaml:"webrtcLocalUDPAddress"`
	WebRTCLocalTCP       string      `yaml:"webrtcLocalTCPAddress"`
	WebRTCIPsFromIfaces  bool        `yaml:"webrtcIPsFromInterfaces"`
	WebRTCAdditionalHost []string    `yaml:"webrtcAdditionalHosts"`
	WebRTCICEServers     []iceServer `yaml:"webrtcICEServers2"`

	PathDefaults pathConf            `yaml:"pathDefaults"`
	Paths        map[string]pathConf `yaml:"paths"`
}

var loopback = []string{"127.0.0.1", "::1"}

func hostPort(host string, port int) string {
	return net.JoinHostPort(host, strconv.Itoa(port))
}

// Render gera o conteúdo do mediamtx.yml.
func (c Config) Render() ([]byte, error) {
	if c.PublishUser == "" || len(c.PublishPass) < 16 {
		return nil, fmt.Errorf("credencial interna de publicação ausente ou fraca")
	}
	if c.LogLevel == "" {
		c.LogLevel = "info"
	}
	iceHost := c.ICEListenHost
	f := file{
		LogLevel: c.LogLevel, LogDestinations: []string{"stdout"},
		ReadTimeout: "10s", WriteTimeout: "10s", WriteQueueSize: 512,
		// Buffer UDP maior absorve as rajadas de keyframes de até 8 câmeras e
		// reduz perdas de pacotes (padrão do SO costuma ser pequeno).
		UDPReadBuffer: 8 << 20,
		AuthMethod:    "internal",
		AuthUsers: []internalUser{
			{
				// Somente o proxy WHIP do servidor (loopback + credencial) publica.
				User: c.PublishUser, Pass: c.PublishPass, IPs: loopback,
				Permissions: []permission{{Action: "publish", Path: PathPattern}},
			},
			{
				// OBS (RTSP), painel (WHEP via proxy) e o servidor (API) leem
				// apenas a partir do próprio computador.
				User: "any", Pass: "", IPs: loopback,
				Permissions: []permission{{Action: "read"}, {Action: "api"}},
			},
		},
		API: true, APIAddress: hostPort("127.0.0.1", c.Ports.API),
		RTSP: true, RTSPTransports: []string{"tcp"}, RTSPEncryption: "no",
		RTSPAddress: hostPort("127.0.0.1", c.Ports.RTSP),

		WebRTC: true, WebRTCAddress: hostPort("127.0.0.1", c.Ports.WebRTC),
		WebRTCAllowOrigins:   []string{"*"}, // só alcançável via loopback
		WebRTCTrustedProxies: loopback,
		WebRTCLocalUDP:       hostPort(iceHost, c.Ports.ICE),
		WebRTCIPsFromIfaces:  c.IPsFromIfaces,
		WebRTCAdditionalHost: append([]string{}, c.AdditionalIPs...),
		WebRTCICEServers:     []iceServer{},

		PathDefaults: pathConf{OverridePublisher: true},
		Paths:        map[string]pathConf{PathPattern: {OverridePublisher: true}},
	}
	if c.ICETCP {
		f.WebRTCLocalTCP = hostPort(iceHost, c.Ports.ICE)
	}
	for _, s := range c.STUNServers {
		f.WebRTCICEServers = append(f.WebRTCICEServers, iceServer{URL: s})
	}
	var buf bytes.Buffer
	buf.WriteString("# Gerado automaticamente pelo SacraLink. NÃO EDITE: alterações serão sobrescritas.\n")
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(f); err != nil {
		return nil, err
	}
	return buf.Bytes(), enc.Close()
}
