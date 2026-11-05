// Package settings define as configurações persistentes do SacraLink, com
// valores padrão seguros (§14: acesso remoto desligado, sem trocas diretas
// no Program, sem alterações automáticas no OBS antes do consentimento).
package settings

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/gersongabrieldev/sacralink/apps/server/internal/store"
)

const key = "app.settings"

// Modos de fallback quando a câmera NO AR perde o sinal (§13).
const (
	FallbackStay   = "stay"   // permanecer na cena atual (padrão)
	FallbackBackup = "backup" // trocar para a câmera reserva
	FallbackScene  = "scene"  // trocar para uma cena (imagem institucional ou cena padrão)
)

// Modos de prévia no painel.
const (
	PreviewAll      = "all"
	PreviewSelected = "selected"
	PreviewOff      = "off"
)

// OBS reúne as preferências de integração com o OBS.
type OBS struct {
	// AutoManageSources: o operador autorizou criar fontes/cenas SacraLink.
	AutoManageSources bool `json:"autoManageSources"`
	// AllowDirectSwitch: trocar o Program sem Studio Mode sem confirmar.
	AllowDirectSwitch bool   `json:"allowDirectSwitch"`
	DefaultScene      string `json:"defaultScene"`
	DefaultTransition string `json:"defaultTransition"`
}

// Fallback reúne a política de queda da câmera NO AR.
type Fallback struct {
	Mode      string `json:"mode"`
	SceneName string `json:"sceneName"`
	GraceSec  int    `json:"graceSec"`
}

// Remote reúne a configuração de acesso externo (4G/5G).
type Remote struct {
	Enabled bool `json:"enabled"`
	// PublicHost: DNS dinâmico ou IP público informado pelo administrador.
	PublicHost string `json:"publicHost"`
	PublicPort int    `json:"publicPort"`
	// UseSTUN permite descobrir o IP público (sem relay de mídia).
	UseSTUN bool `json:"useStun"`
}

// Audio reúne a seleção de áudio principal/reserva no OBS.
type Audio struct {
	MainInput   string `json:"mainInput"`
	BackupInput string `json:"backupInput"`
}

// Settings são as configurações da aplicação.
type Settings struct {
	ServerName       string   `json:"serverName"`
	FirstRunComplete bool     `json:"firstRunComplete"`
	MaxStreams       int      `json:"maxStreams"`
	ICETCP           bool     `json:"iceTcp"`
	PreviewMode      string   `json:"previewMode"`
	Language         string   `json:"language"`
	Theme            string   `json:"theme"`
	OBS              OBS      `json:"obs"`
	Fallback         Fallback `json:"fallback"`
	Remote           Remote   `json:"remote"`
	Audio            Audio    `json:"audio"`
}

// Defaults retorna as configurações padrão.
func Defaults() Settings {
	return Settings{
		ServerName: "SacraLink", MaxStreams: 8, ICETCP: true, PreviewMode: PreviewAll,
		Language: "pt-BR", Theme: "dark",
		Fallback: Fallback{Mode: FallbackStay, GraceSec: 3},
		Remote:   Remote{PublicPort: 7443},
	}
}

// Validate normaliza e valida as configurações.
func (s *Settings) Validate() error {
	s.ServerName = strings.TrimSpace(s.ServerName)
	if s.ServerName == "" || len([]rune(s.ServerName)) > 60 {
		return errors.New("o nome do servidor deve ter entre 1 e 60 caracteres")
	}
	if s.MaxStreams < 1 || s.MaxStreams > 8 {
		return errors.New("o limite de transmissões simultâneas deve estar entre 1 e 8")
	}
	switch s.PreviewMode {
	case PreviewAll, PreviewSelected, PreviewOff:
	default:
		return fmt.Errorf("modo de prévia inválido: %q", s.PreviewMode)
	}
	switch s.Fallback.Mode {
	case FallbackStay, FallbackBackup:
	case FallbackScene:
		if strings.TrimSpace(s.Fallback.SceneName) == "" {
			return errors.New("informe a cena de fallback")
		}
	default:
		return fmt.Errorf("modo de fallback inválido: %q", s.Fallback.Mode)
	}
	if s.Fallback.GraceSec < 1 || s.Fallback.GraceSec > 60 {
		return errors.New("a carência do fallback deve estar entre 1 e 60 segundos")
	}
	if s.Remote.PublicPort == 0 {
		s.Remote.PublicPort = 7443
	}
	if s.Remote.PublicPort < 1 || s.Remote.PublicPort > 65535 {
		return errors.New("porta pública inválida")
	}
	s.Remote.PublicHost = strings.TrimSpace(s.Remote.PublicHost)
	if s.Remote.Enabled && s.Remote.PublicHost == "" {
		return errors.New("para habilitar o acesso remoto informe o IP público ou o nome DNS")
	}
	if s.Theme != "dark" && s.Theme != "light" {
		s.Theme = "dark"
	}
	return nil
}

// Manager mantém as configurações em memória e no banco.
type Manager struct {
	st *store.Store
	mu sync.RWMutex
	s  Settings
}

// Load carrega as configurações (ou os padrões).
func Load(ctx context.Context, st *store.Store) (*Manager, error) {
	s := Defaults()
	if _, err := st.GetSetting(ctx, key, &s); err != nil {
		return nil, err
	}
	if err := s.Validate(); err != nil {
		s = Defaults()
	}
	return &Manager{st: st, s: s}, nil
}

// Get retorna uma cópia das configurações.
func (m *Manager) Get() Settings {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.s
}

// Update valida e persiste as configurações.
func (m *Manager) Update(ctx context.Context, f func(*Settings)) (Settings, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	next := m.s
	f(&next)
	if err := next.Validate(); err != nil {
		return m.s, err
	}
	if err := m.st.SetSetting(ctx, key, next); err != nil {
		return m.s, err
	}
	m.s = next
	return next, nil
}

// Secret retorna (ou cria) um segredo interno persistente, como a credencial
// de publicação do MediaMTX.
func Secret(ctx context.Context, st *store.Store, name string, gen func() string) (string, error) {
	var v string
	ok, err := st.GetSetting(ctx, "secret."+name, &v)
	if err != nil {
		return "", err
	}
	if ok && v != "" {
		return v, nil
	}
	v = gen()
	return v, st.SetSetting(ctx, "secret."+name, v)
}
