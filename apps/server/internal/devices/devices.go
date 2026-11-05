// Package devices implementa o pareamento e a autenticação dos smartphones.
//
// Fluxo (SECURITY.md):
//  1. O administrador gera um token de pareamento de uso único (5 min).
//  2. O QR Code contém endereços do servidor, a impressão digital TLS e o
//     token — nunca a senha do administrador ou credenciais reutilizáveis.
//  3. O app chama /device/v1/pair com o token e recebe deviceId + segredo,
//     exibidos uma única vez. O servidor guarda apenas o hash do segredo.
//  4. Requisições seguintes usam "Authorization: Bearer <deviceId>.<segredo>".
package devices

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/skip2/go-qrcode"

	"github.com/gersongabrieldev/sacralink/apps/server/internal/secure"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/store"
)

// PairingTTL é a validade do token de pareamento.
const PairingTTL = 5 * time.Minute

// Erros de autenticação.
var (
	ErrUnauthorized = errors.New("credencial do dispositivo inválida")
	ErrRevoked      = errors.New("dispositivo revogado: pareie novamente")
	ErrBlocked      = errors.New("dispositivo bloqueado pelo administrador")
	ErrInvalidInput = errors.New("dados do dispositivo inválidos")
)

// Endpoints descreve como os aparelhos alcançam o servidor.
type Endpoints struct {
	ServerID       string
	ServerName     string
	LANHosts       []string // IPs locais (IPv4/IPv6)
	Port           int
	ExternalHost   string // DDNS ou IP público (vazio se acesso remoto desligado)
	ExternalPort   int
	FingerprintHex string
}

// Service implementa pareamento e autenticação.
type Service struct {
	store     *store.Store
	endpoints func() Endpoints
}

// New cria o serviço.
func New(st *store.Store, endpoints func() Endpoints) *Service {
	return &Service{store: st, endpoints: endpoints}
}

// Pairing é o resultado de CreatePairing.
type Pairing struct {
	Token     string `json:"-"`
	ExpiresAt int64  `json:"expiresAt"`
	URI       string `json:"uri"`
	QRPNG     []byte `json:"-"`
	CameraID  string `json:"cameraId,omitempty"`
}

// CreatePairing gera um token de uso único e o QR Code correspondente.
// cameraID vazio cria uma nova câmera; preenchido, o novo aparelho assume a
// identidade da câmera existente.
func (s *Service) CreatePairing(ctx context.Context, label, cameraID string) (*Pairing, error) {
	if cameraID != "" {
		if _, err := s.store.GetCamera(ctx, cameraID); err != nil {
			return nil, err
		}
	}
	tok := secure.NewToken()
	exp, err := s.store.CreatePairingToken(ctx, secure.HashSecret(tok), label, cameraID, PairingTTL.Milliseconds())
	if err != nil {
		return nil, err
	}
	uri := PairingURI(s.endpoints(), tok, exp)
	png, err := qrcode.Encode(uri, qrcode.Medium, 512)
	if err != nil {
		return nil, err
	}
	return &Pairing{Token: tok, ExpiresAt: exp, URI: uri, QRPNG: png, CameraID: cameraID}, nil
}

// PairingURI monta o conteúdo do QR Code:
//
//	sacralink://pair?v=1&id=..&n=..&h=ip1,ip2&p=7443&x=host:port&fp=..&t=..&exp=..
func PairingURI(e Endpoints, token string, expMs int64) string {
	q := url.Values{}
	q.Set("v", "1")
	q.Set("id", e.ServerID)
	q.Set("n", e.ServerName)
	q.Set("h", strings.Join(e.LANHosts, ","))
	q.Set("p", strconv.Itoa(e.Port))
	if e.ExternalHost != "" {
		port := e.ExternalPort
		if port == 0 {
			port = e.Port
		}
		q.Set("x", e.ExternalHost+":"+strconv.Itoa(port))
	}
	q.Set("fp", e.FingerprintHex)
	q.Set("t", token)
	q.Set("exp", strconv.FormatInt(expMs/1000, 10))
	return "sacralink://pair?" + q.Encode()
}

// DeviceInfo são os dados enviados pelo app no pareamento.
type DeviceInfo struct {
	Name       string `json:"name"`
	Platform   string `json:"platform"`
	Model      string `json:"model"`
	OSVersion  string `json:"osVersion"`
	AppVersion string `json:"appVersion"`
}

func clean(s string, max int) (string, error) {
	s = strings.TrimSpace(s)
	if !utf8.ValidString(s) || utf8.RuneCountInString(s) > max {
		return "", ErrInvalidInput
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return "", ErrInvalidInput
		}
	}
	return s, nil
}

// Validate normaliza e valida os dados do aparelho.
func (d *DeviceInfo) Validate() error {
	var err error
	if d.Name, err = clean(d.Name, 60); err != nil {
		return err
	}
	if d.Name == "" {
		d.Name = "Celular"
	}
	switch d.Platform {
	case "android", "ios":
	default:
		d.Platform = "other"
	}
	for _, f := range []*string{&d.Model, &d.OSVersion, &d.AppVersion} {
		if *f, err = clean(*f, 80); err != nil {
			return err
		}
	}
	return nil
}

// PairResult é devolvido ao aparelho uma única vez.
type PairResult struct {
	DeviceID   string `json:"deviceId"`
	Secret     string `json:"secret"`
	CameraID   string `json:"cameraId"`
	CameraName string `json:"cameraName"`
	Slot       int    `json:"slot"`
	ServerID   string `json:"serverId"`
	ServerName string `json:"serverName"`
}

// Pair consome o token e registra o aparelho.
func (s *Service) Pair(ctx context.Context, token string, info DeviceInfo, ip string) (*PairResult, error) {
	if len(token) < 20 || len(token) > 100 {
		return nil, store.ErrPairingInvalid
	}
	if err := info.Validate(); err != nil {
		return nil, err
	}
	secret := secure.NewToken()
	d, c, err := s.store.PairDevice(ctx, secure.HashSecret(token), store.NewDeviceInput{
		Name: info.Name, Platform: info.Platform, Model: info.Model, OSVersion: info.OSVersion,
		AppVersion: info.AppVersion, SecretHash: secure.HashSecret(secret), IP: ip,
	})
	if err != nil {
		return nil, err
	}
	e := s.endpoints()
	return &PairResult{DeviceID: d.ID, Secret: secret, CameraID: c.ID, CameraName: c.Name, Slot: c.Slot,
		ServerID: e.ServerID, ServerName: e.ServerName}, nil
}

// ParseBearer extrai deviceId e segredo de "Bearer <deviceId>.<segredo>".
func ParseBearer(header string) (deviceID, secret string, ok bool) {
	v, found := strings.CutPrefix(header, "Bearer ")
	if !found {
		return "", "", false
	}
	deviceID, secret, ok = strings.Cut(strings.TrimSpace(v), ".")
	if !ok || len(deviceID) < 8 || len(deviceID) > 32 || len(secret) < 20 || len(secret) > 100 {
		return "", "", false
	}
	return deviceID, secret, true
}

// Authenticate valida a credencial do aparelho e retorna dispositivo e câmera.
func (s *Service) Authenticate(ctx context.Context, authorization string) (*store.Device, *store.Camera, error) {
	id, secret, ok := ParseBearer(authorization)
	if !ok {
		return nil, nil, ErrUnauthorized
	}
	d, err := s.store.GetDevice(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil, ErrUnauthorized
	}
	if err != nil {
		return nil, nil, err
	}
	if !secure.EqualHash(d.SecretHash, secure.HashSecret(secret)) {
		return nil, nil, ErrUnauthorized
	}
	switch d.Status {
	case store.DeviceRevoked:
		return nil, nil, ErrRevoked
	case store.DeviceBlocked:
		return nil, nil, ErrBlocked
	}
	if d.CameraID == "" {
		return nil, nil, fmt.Errorf("%w: aparelho sem câmera associada", ErrRevoked)
	}
	c, err := s.store.GetCamera(ctx, d.CameraID)
	if err != nil {
		return nil, nil, err
	}
	return d, c, nil
}

// HasPermission verifica uma permissão do dispositivo.
func HasPermission(d *store.Device, perm string) bool {
	for _, p := range d.Permissions {
		if p == perm {
			return true
		}
	}
	return false
}
