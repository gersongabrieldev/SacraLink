// Package platform isola as integrações com o sistema operacional: Job
// Objects, DPAPI, início automático, versão do Windows, detecção do OBS,
// regras de firewall e métricas do sistema. Em sistemas não-Windows existem
// implementações mínimas para permitir compilação e testes no CI.
package platform

import (
	"fmt"
	"os/exec"
)

// OSInfo descreve o sistema operacional em execução.
type OSInfo struct {
	Name         string `json:"name"`
	Major        uint32 `json:"major"`
	Minor        uint32 `json:"minor"`
	Build        uint32 `json:"build"`
	Arch         string `json:"arch"`
	IsWindows10  bool   `json:"isWindows10"`
	IsWindows11  bool   `json:"isWindows11"`
	Supported    bool   `json:"supported"`
	Warning      string `json:"warning,omitempty"`
	SmartAppCtrl string `json:"smartAppControl,omitempty"` // "on", "evaluation", "off" ou ""
}

// Windows10EOLWarning é exibido em Windows 10 (fim do suporte em 14/10/2025).
const Windows10EOLWarning = "O Windows 10 encerrou o suporte regular em 14/10/2025 e não recebe mais " +
	"atualizações de segurança gratuitas. Expor este computador à internet (acesso remoto 4G/5G) exige " +
	"cuidados adicionais. Recomendamos migrar para o Windows 11."

// OBSInstall descreve uma instalação do OBS Studio encontrada.
type OBSInstall struct {
	Found      bool   `json:"found"`
	Path       string `json:"path,omitempty"`
	Executable string `json:"executable,omitempty"`
	Version    string `json:"version,omitempty"`
	Running    bool   `json:"running"`
	// Configuração do obs-websocket lida do perfil do usuário (somente leitura).
	WebSocket *OBSWebSocketConfig `json:"websocket,omitempty"`
}

// OBSWebSocketConfig espelha plugin_config/obs-websocket/config.json.
type OBSWebSocketConfig struct {
	Enabled      bool   `json:"server_enabled"`
	Port         int    `json:"server_port"`
	AuthRequired bool   `json:"auth_required"`
	Password     string `json:"server_password"`
	FirstLoad    bool   `json:"first_load"`
	AlertsOn     bool   `json:"alerts_enabled"`
}

// SystemStats são métricas instantâneas do sistema.
type SystemStats struct {
	CPUPercent    float64 `json:"cpuPercent"`
	MemTotalBytes uint64  `json:"memTotalBytes"`
	MemUsedBytes  uint64  `json:"memUsedBytes"`
	MemPercent    float64 `json:"memPercent"`
	NumCPU        int     `json:"numCpu"`
}

// FirewallRule descreve uma regra de entrada criada pelo SacraLink.
type FirewallRule struct {
	Name     string `json:"name"`
	Program  string `json:"program"`
	Protocol string `json:"protocol"` // TCP ou UDP
	Port     int    `json:"port"`
	Profile  string `json:"profile"` // private, domain, public ou combinação "private,domain"
}

// FirewallRulePrefix identifica as regras do SacraLink (usado na remoção).
const FirewallRulePrefix = "SacraLink - "

// netshAddArgs monta os argumentos do netsh para criar a regra. Separado para
// ser testável em qualquer SO.
func netshAddArgs(r FirewallRule) []string {
	return []string{"advfirewall", "firewall", "add", "rule",
		"name=" + r.Name,
		"dir=in", "action=allow",
		"program=" + r.Program,
		"protocol=" + r.Protocol,
		fmt.Sprintf("localport=%d", r.Port),
		"profile=" + r.Profile,
		"enable=yes",
	}
}

func netshDeleteArgs(name string) []string {
	return []string{"advfirewall", "firewall", "delete", "rule", "name=" + name}
}

// runNetsh executa o netsh (requer processo elevado para add/delete).
func runNetsh(args ...string) (string, error) {
	cmd := exec.Command("netsh", args...)
	hideWindow(cmd)
	out, err := cmd.CombinedOutput()
	return string(out), err
}
