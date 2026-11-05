//go:build !windows

package platform

// Implementações mínimas para compilar e testar fora do Windows (CI Linux).
// O produto é suportado apenas no Windows 10/11 x64.

import (
	"crypto/aes"
	"crypto/cipher"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/gersongabrieldev/sacralink/apps/server/internal/secure"
)

// DataDir retorna o diretório de dados do usuário.
func DataDir() (string, error) {
	d, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "SacraLink"), nil
}

// OS retorna informações mínimas do sistema.
func OS() OSInfo {
	return OSInfo{Name: runtime.GOOS, Arch: runtime.GOARCH, Warning: "Sistema operacional não suportado em produção."}
}

// Job é um no-op fora do Windows.
type Job struct{}

// NewKillOnCloseJob retorna um Job no-op.
func NewKillOnCloseJob() (*Job, error) { return &Job{}, nil }

// Assign é no-op.
func (j *Job) Assign(*os.Process) error { return nil }

// Close é no-op.
func (j *Job) Close() error { return nil }

// devKey é uma chave de processo usada só em testes fora do Windows.
var devKey = secure.RandomBytes(32)

// Protect cifra com AES-GCM e chave efêmera (apenas para testes).
func Protect(plain []byte) ([]byte, error) {
	block, _ := aes.NewCipher(devKey)
	gcm, _ := cipher.NewGCM(block)
	nonce := secure.RandomBytes(gcm.NonceSize())
	return gcm.Seal(nonce, nonce, plain, nil), nil
}

// Unprotect decifra dados de Protect.
func Unprotect(enc []byte) ([]byte, error) {
	if len(enc) == 0 {
		return nil, nil
	}
	block, _ := aes.NewCipher(devKey)
	gcm, _ := cipher.NewGCM(block)
	if len(enc) < gcm.NonceSize() {
		return nil, errors.New("dados inválidos")
	}
	return gcm.Open(nil, enc[:gcm.NonceSize()], enc[gcm.NonceSize():], nil)
}

// SetAutostart não é suportado fora do Windows.
func SetAutostart(bool, string, ...string) error { return errors.ErrUnsupported }

// AutostartEnabled retorna false.
func AutostartEnabled() bool { return false }

// SingleInstance sempre permite (sem mutex nomeado).
func SingleInstance(string) (func(), bool, error) { return func() {}, true, nil }

// DetectOBS não é suportado fora do Windows.
func DetectOBS() OBSInstall { return OBSInstall{} }

// ReadOBSWebSocketConfig não é suportado fora do Windows.
func ReadOBSWebSocketConfig() (*OBSWebSocketConfig, error) { return nil, errors.ErrUnsupported }

// StatsSampler retorna apenas o número de CPUs.
type StatsSampler struct{}

// Sample retorna métricas mínimas.
func (s *StatsSampler) Sample() SystemStats { return SystemStats{NumCPU: runtime.NumCPU()} }

// IsElevated retorna se o processo é root.
func IsElevated() bool { return os.Geteuid() == 0 }

// AddFirewallRules não é suportado.
func AddFirewallRules([]FirewallRule) error { return errors.ErrUnsupported }

// DeleteFirewallRules não é suportado.
func DeleteFirewallRules([]string) error { return errors.ErrUnsupported }

// FirewallRuleExists retorna false.
func FirewallRuleExists(string) bool { return false }

// ErrElevationCancelled indica que o usuário recusou a elevação.
var ErrElevationCancelled = errors.New("elevação cancelada")

// RunElevated não é suportado.
func RunElevated(string, string, time.Duration) (uint32, error) { return 0, errors.ErrUnsupported }

// OpenURL abre com xdg-open/open.
func OpenURL(target string) error {
	if runtime.GOOS == "darwin" {
		return exec.Command("open", target).Start()
	}
	return exec.Command("xdg-open", target).Start()
}

// OpenFolder abre uma pasta.
func OpenFolder(dir string) error { return OpenURL(dir) }

func hideWindow(*exec.Cmd) {}

// HideWindow é no-op.
func HideWindow(*exec.Cmd) {}
