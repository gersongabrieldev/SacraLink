package platform

import (
	"bytes"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"testing"
	"time"
)

func TestProtectRoundTrip(t *testing.T) {
	plain := []byte("senha-do-obs-ç")
	enc, err := Protect(plain)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(enc, plain) {
		t.Fatal("dado cifrado contém o texto puro")
	}
	dec, err := Unprotect(enc)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(dec, plain) {
		t.Fatalf("round-trip falhou: %q", dec)
	}
}

func TestNetshArgs(t *testing.T) {
	args := netshAddArgs(FirewallRule{Name: FirewallRulePrefix + "Dispositivos", Program: `C:\x\s.exe`, Protocol: "TCP", Port: 7443, Profile: "private"})
	for _, want := range []string{"name=SacraLink - Dispositivos", "dir=in", "action=allow", `program=C:\x\s.exe`, "protocol=TCP", "localport=7443", "profile=private"} {
		if !slices.Contains(args, want) {
			t.Fatalf("faltando %q em %v", want, args)
		}
	}
}

func TestOSInfo(t *testing.T) {
	info := OS()
	if runtime.GOOS == "windows" && info.Major < 10 {
		t.Fatalf("versão inesperada: %+v", info)
	}
	if info.IsWindows10 && info.Warning == "" {
		t.Fatal("Windows 10 deve exibir aviso de fim de suporte")
	}
}

func TestJobObject(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("apenas Windows")
	}
	job, err := NewKillOnCloseJob()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("cmd", "/c", "ping -n 30 127.0.0.1 >NUL")
	HideWindow(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	if err := job.Assign(cmd.Process); err != nil {
		t.Fatal(err)
	}
	// Fechar o Job deve encerrar o filho imediatamente (o ping duraria ~29 s).
	job.Close()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("processo deveria ter sido encerrado pelo Job")
	}
}

func TestSingleInstance(t *testing.T) {
	name := "SacraLinkTest-" + string(rune('a'+os.Getpid()%26))
	rel, ok, err := SingleInstance(name)
	if err != nil || !ok {
		t.Fatalf("primeira instância: %v %v", ok, err)
	}
	defer rel()
	if runtime.GOOS == "windows" {
		_, ok2, err := SingleInstance(name)
		if err != nil || ok2 {
			t.Fatalf("segunda instância deveria falhar: %v %v", ok2, err)
		}
	}
}

func TestSystemStats(t *testing.T) {
	var s StatsSampler
	s.Sample()
	st := s.Sample()
	if st.NumCPU <= 0 {
		t.Fatal("NumCPU inválido")
	}
	if runtime.GOOS == "windows" && (st.MemTotalBytes == 0 || st.MemPercent <= 0) {
		t.Fatalf("memória inválida: %+v", st)
	}
}
