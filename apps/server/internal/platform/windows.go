//go:build windows

package platform

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// DataDir retorna %LOCALAPPDATA%\SacraLink.
func DataDir() (string, error) {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		d, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}
		base = d
	}
	return filepath.Join(base, "SacraLink"), nil
}

// OS retorna informações da versão do Windows.
func OS() OSInfo {
	v := windows.RtlGetVersion()
	info := OSInfo{
		Major: v.MajorVersion, Minor: v.MinorVersion, Build: v.BuildNumber,
		Arch: runtime.GOARCH,
	}
	switch {
	case v.MajorVersion == 10 && v.BuildNumber >= 22000:
		info.Name, info.IsWindows11, info.Supported = "Windows 11", true, true
	case v.MajorVersion == 10:
		info.Name, info.IsWindows10, info.Supported = "Windows 10", true, true
		info.Warning = Windows10EOLWarning
	default:
		info.Name = fmt.Sprintf("Windows %d.%d", v.MajorVersion, v.MinorVersion)
		info.Warning = "Versão do Windows não suportada. Use Windows 10 ou 11 64 bits."
	}
	if runtime.GOARCH != "amd64" {
		info.Supported = false
		info.Warning = "Arquitetura não suportada: o SacraLink requer Windows 64 bits (x64)."
	}
	info.SmartAppCtrl = smartAppControlState()
	return info
}

func smartAppControlState() string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Control\CI\Policy`, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	v, _, err := k.GetIntegerValue("VerifiedAndReputablePolicyState")
	if err != nil {
		return ""
	}
	switch v {
	case 0:
		return "off"
	case 1:
		return "on"
	case 2:
		return "evaluation"
	}
	return ""
}

// ---------------------------------------------------------------------------
// Job Object: garante que processos filhos (MediaMTX) morram junto com o
// servidor, mesmo em caso de travamento.

// Job agrupa processos filhos que devem ser encerrados com o servidor.
type Job struct{ h windows.Handle }

// NewKillOnCloseJob cria um Job Object com JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE.
func NewKillOnCloseJob() (*Job, error) {
	h, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, fmt.Errorf("CreateJobObject: %w", err)
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(h, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		windows.CloseHandle(h)
		return nil, fmt.Errorf("SetInformationJobObject: %w", err)
	}
	return &Job{h: h}, nil
}

// Assign associa o processo ao Job.
func (j *Job) Assign(p *os.Process) error {
	ph, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(p.Pid))
	if err != nil {
		return fmt.Errorf("OpenProcess: %w", err)
	}
	defer windows.CloseHandle(ph)
	return windows.AssignProcessToJobObject(j.h, ph)
}

// Close encerra todos os processos do Job.
func (j *Job) Close() error { return windows.CloseHandle(j.h) }

// ---------------------------------------------------------------------------
// DPAPI (escopo do usuário atual).

var dpapiEntropy = []byte("SacraLink/credenciais/v1")

func blob(b []byte) *windows.DataBlob {
	if len(b) == 0 {
		return &windows.DataBlob{}
	}
	return &windows.DataBlob{Size: uint32(len(b)), Data: &b[0]}
}

// Protect cifra dados com DPAPI no escopo do usuário do Windows.
func Protect(plain []byte) ([]byte, error) {
	var out windows.DataBlob
	if err := windows.CryptProtectData(blob(plain), nil, blob(dpapiEntropy), 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return nil, fmt.Errorf("CryptProtectData: %w", err)
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return append([]byte(nil), unsafe.Slice(out.Data, out.Size)...), nil
}

// Unprotect decifra dados protegidos por Protect.
func Unprotect(enc []byte) ([]byte, error) {
	if len(enc) == 0 {
		return nil, nil
	}
	var out windows.DataBlob
	if err := windows.CryptUnprotectData(blob(enc), nil, blob(dpapiEntropy), 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return nil, fmt.Errorf("CryptUnprotectData: %w", err)
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return append([]byte(nil), unsafe.Slice(out.Data, out.Size)...), nil
}

// ---------------------------------------------------------------------------
// Início automático (HKCU\Software\Microsoft\Windows\CurrentVersion\Run).

const runKey = `Software\Microsoft\Windows\CurrentVersion\Run`
const runValue = "SacraLink"

// SetAutostart habilita/desabilita o início com o Windows para o usuário atual.
func SetAutostart(enabled bool, exe string, args ...string) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, runKey, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if !enabled {
		if err := k.DeleteValue(runValue); err != nil && !errors.Is(err, registry.ErrNotExist) {
			return err
		}
		return nil
	}
	cmd := `"` + exe + `"`
	if len(args) > 0 {
		cmd += " " + strings.Join(args, " ")
	}
	return k.SetStringValue(runValue, cmd)
}

// AutostartEnabled informa se o início automático está configurado.
func AutostartEnabled() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	_, _, err = k.GetStringValue(runValue)
	return err == nil
}

// ---------------------------------------------------------------------------
// Instância única.

// SingleInstance cria um mutex nomeado. ok=false se já houver outra instância.
func SingleInstance(name string) (release func(), ok bool, err error) {
	p, err := windows.UTF16PtrFromString(`Local\` + name)
	if err != nil {
		return nil, false, err
	}
	h, err := windows.CreateMutex(nil, false, p)
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		windows.CloseHandle(h)
		return func() {}, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return func() { windows.CloseHandle(h) }, true, nil
}

// ---------------------------------------------------------------------------
// OBS Studio.

// DetectOBS procura a instalação do OBS e lê a configuração do obs-websocket
// (somente leitura; nada é alterado).
func DetectOBS() OBSInstall {
	inst := OBSInstall{}
	candidates := []string{}
	for _, view := range []uint32{registry.WOW64_64KEY, registry.WOW64_32KEY} {
		if k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\OBS Studio`, registry.QUERY_VALUE|view); err == nil {
			if v, _, err := k.GetStringValue(""); err == nil && v != "" {
				candidates = append(candidates, v)
			}
			k.Close()
		}
	}
	candidates = append(candidates, filepath.Join(os.Getenv("ProgramFiles"), "obs-studio"))
	for _, dir := range candidates {
		exe := filepath.Join(dir, "bin", "64bit", "obs64.exe")
		if _, err := os.Stat(exe); err == nil {
			inst.Found, inst.Path, inst.Executable = true, dir, exe
			inst.Version = fileVersion(exe)
			break
		}
	}
	inst.Running = processRunning("obs64.exe")
	if cfg, err := ReadOBSWebSocketConfig(); err == nil {
		inst.WebSocket = cfg
	}
	return inst
}

// ReadOBSWebSocketConfig lê %APPDATA%\obs-studio\plugin_config\obs-websocket\config.json.
func ReadOBSWebSocketConfig() (*OBSWebSocketConfig, error) {
	p := filepath.Join(os.Getenv("APPDATA"), "obs-studio", "plugin_config", "obs-websocket", "config.json")
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	cfg := &OBSWebSocketConfig{}
	if err := json.Unmarshal(b, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func fileVersion(path string) string {
	size, err := windows.GetFileVersionInfoSize(path, nil)
	if err != nil || size == 0 {
		return ""
	}
	buf := make([]byte, size)
	if err := windows.GetFileVersionInfo(path, 0, size, unsafe.Pointer(&buf[0])); err != nil {
		return ""
	}
	var fixed *windows.VS_FIXEDFILEINFO
	var l uint32
	if err := windows.VerQueryValue(unsafe.Pointer(&buf[0]), `\`, unsafe.Pointer(&fixed), &l); err != nil || fixed == nil {
		return ""
	}
	return fmt.Sprintf("%d.%d.%d", fixed.FileVersionMS>>16, fixed.FileVersionMS&0xffff, fixed.FileVersionLS>>16)
}

func processRunning(exeName string) bool {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(snap)
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		if strings.EqualFold(windows.UTF16ToString(e.ExeFile[:]), exeName) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Métricas do sistema.

var (
	kernel32                 = windows.NewLazySystemDLL("kernel32.dll")
	procGetSystemTimes       = kernel32.NewProc("GetSystemTimes")
	procGlobalMemoryStatusEx = kernel32.NewProc("GlobalMemoryStatusEx")
)

// memoryStatusEx espelha MEMORYSTATUSEX.
type memoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

// StatsSampler calcula o uso de CPU entre amostras consecutivas.
type StatsSampler struct {
	mu                 sync.Mutex
	idle, kernel, user uint64
	hasPrev            bool
}

func ft(f windows.Filetime) uint64 { return uint64(f.HighDateTime)<<32 | uint64(f.LowDateTime) }

// Sample retorna CPU (desde a última amostra) e memória.
func (s *StatsSampler) Sample() SystemStats {
	st := SystemStats{NumCPU: runtime.NumCPU()}
	var idle, kern, usr windows.Filetime
	r, _, _ := procGetSystemTimes.Call(uintptr(unsafe.Pointer(&idle)), uintptr(unsafe.Pointer(&kern)), uintptr(unsafe.Pointer(&usr)))
	if r != 0 {
		s.mu.Lock()
		i, k, u := ft(idle), ft(kern), ft(usr)
		if s.hasPrev {
			di, dk, du := i-s.idle, k-s.kernel, u-s.user
			if total := dk + du; total > 0 { // kernel inclui idle
				st.CPUPercent = float64(total-di) / float64(total) * 100
			}
		}
		s.idle, s.kernel, s.user, s.hasPrev = i, k, u, true
		s.mu.Unlock()
	}
	var mem memoryStatusEx
	mem.Length = uint32(unsafe.Sizeof(mem))
	if r, _, _ := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&mem))); r != 0 {
		st.MemTotalBytes = mem.TotalPhys
		st.MemUsedBytes = mem.TotalPhys - mem.AvailPhys
		if mem.TotalPhys > 0 {
			st.MemPercent = float64(st.MemUsedBytes) / float64(mem.TotalPhys) * 100
		}
	}
	return st
}

// ---------------------------------------------------------------------------
// Firewall e elevação.

// IsElevated informa se o processo atual tem privilégios de administrador.
func IsElevated() bool {
	return windows.GetCurrentProcessToken().IsElevated()
}

// AddFirewallRules cria as regras (requer processo elevado). Regras com o
// mesmo nome são substituídas, tornando a operação idempotente.
func AddFirewallRules(rules []FirewallRule) error {
	for _, r := range rules {
		_, _ = runNetsh(netshDeleteArgs(r.Name)...)
		if out, err := runNetsh(netshAddArgs(r)...); err != nil {
			return fmt.Errorf("netsh add %q: %v: %s", r.Name, err, strings.TrimSpace(out))
		}
	}
	return nil
}

// DeleteFirewallRules remove as regras indicadas (requer processo elevado).
func DeleteFirewallRules(names []string) error {
	var errs []error
	for _, n := range names {
		if out, err := runNetsh(netshDeleteArgs(n)...); err != nil && !strings.Contains(out, "No rules match") {
			errs = append(errs, fmt.Errorf("%s: %v", n, err))
		}
	}
	return errors.Join(errs...)
}

// FirewallRuleExists consulta se a regra existe (não requer elevação).
func FirewallRuleExists(name string) bool {
	_, err := runNetsh("advfirewall", "firewall", "show", "rule", "name="+name)
	return err == nil
}

type shellExecuteInfo struct {
	cbSize         uint32
	fMask          uint32
	hwnd           uintptr
	lpVerb         *uint16
	lpFile         *uint16
	lpParameters   *uint16
	lpDirectory    *uint16
	nShow          int32
	hInstApp       uintptr
	lpIDList       uintptr
	lpClass        *uint16
	hkeyClass      uintptr
	dwHotKey       uint32
	hIconOrMonitor uintptr
	hProcess       windows.Handle
}

var (
	shell32             = windows.NewLazySystemDLL("shell32.dll")
	procShellExecuteExW = shell32.NewProc("ShellExecuteExW")
)

// ErrElevationCancelled indica que o usuário recusou o prompt do UAC.
var ErrElevationCancelled = errors.New("o usuário cancelou a solicitação de permissão de administrador")

// RunElevated executa exe com "runas" (prompt do UAC) e aguarda o término.
func RunElevated(exe, args string, timeout time.Duration) (exitCode uint32, err error) {
	verb, _ := windows.UTF16PtrFromString("runas")
	file, _ := windows.UTF16PtrFromString(exe)
	params, _ := windows.UTF16PtrFromString(args)
	sei := shellExecuteInfo{
		fMask:        0x00000040 | 0x00008000, // SEE_MASK_NOCLOSEPROCESS | SEE_MASK_NO_CONSOLE
		lpVerb:       verb,
		lpFile:       file,
		lpParameters: params,
		nShow:        0, // SW_HIDE
	}
	sei.cbSize = uint32(unsafe.Sizeof(sei))
	r, _, callErr := procShellExecuteExW.Call(uintptr(unsafe.Pointer(&sei)))
	if r == 0 {
		if errors.Is(callErr, windows.ERROR_CANCELLED) {
			return 0, ErrElevationCancelled
		}
		return 0, fmt.Errorf("ShellExecuteEx: %w", callErr)
	}
	defer windows.CloseHandle(sei.hProcess)
	ev, err := windows.WaitForSingleObject(sei.hProcess, uint32(timeout.Milliseconds()))
	if err != nil {
		return 0, err
	}
	if ev == uint32(windows.WAIT_TIMEOUT) {
		return 0, errors.New("tempo esgotado aguardando o processo elevado")
	}
	if err := windows.GetExitCodeProcess(sei.hProcess, &exitCode); err != nil {
		return 0, err
	}
	return exitCode, nil
}

// OpenURL abre uma URL ou arquivo com o programa padrão do Windows.
func OpenURL(target string) error {
	cmd := exec.Command("rundll32", "url.dll,FileProtocolHandler", target)
	return cmd.Start()
}

// OpenFolder abre uma pasta no Explorer.
func OpenFolder(dir string) error {
	return exec.Command("explorer", dir).Start()
}

func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
}

// HideWindow configura o processo filho para não abrir janela de console.
func HideWindow(cmd *exec.Cmd) { hideWindow(cmd) }
