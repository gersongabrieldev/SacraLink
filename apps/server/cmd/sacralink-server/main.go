// Comando sacralink-server: servidor do SacraLink (APIs, orquestração, OBS,
// supervisor do MediaMTX) com ícone na bandeja do Windows.
//
// Uso:
//
//	sacralink-server [--background] [--open] [--no-tray] [--dev] [--log-level info]
//	sacralink-server firewall apply|remove [opções]   (executado com elevação UAC)
//	sacralink-server admin reset --confirm            (recupera o acesso ao painel)
//	sacralink-server version
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/gersongabrieldev/sacralink/apps/server/internal/app"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/assets"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/buildinfo"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/logging"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/platform"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/store"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/tray"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "firewall":
			os.Exit(firewallCmd(os.Args[2:]))
		case "admin":
			os.Exit(adminCmd(os.Args[2:]))
		case "version", "--version":
			b := buildinfo.Get()
			fmt.Printf("sacralink-server %s (%s, %s, %s)\n", b.Version, b.Commit, b.Date, b.GoVersion)
			return
		}
	}
	os.Exit(serve())
}

func serve() int {
	def := app.DefaultConfig()
	fs := flag.NewFlagSet("sacralink-server", flag.ExitOnError)
	dataDir := fs.String("data-dir", "", "diretório de dados (padrão %LOCALAPPDATA%\\SacraLink)")
	adminPort := fs.Int("admin-port", def.AdminPort, "porta da API de administração (127.0.0.1)")
	devicePort := fs.Int("device-port", def.DevicePort, "porta da API dos dispositivos (TLS)")
	mediaExe := fs.String("mediamtx", "", "caminho do mediamtx.exe")
	noTray := fs.Bool("no-tray", false, "não exibir ícone na bandeja")
	noMDNS := fs.Bool("no-mdns", false, "não anunciar o servidor na rede local (mDNS)")
	dev := fs.Bool("dev", false, "modo desenvolvimento (log no console, aceita origem do Vite)")
	level := fs.String("log-level", "info", "nível de log: debug, info, warn, error")
	_ = fs.Bool("background", false, "iniciado automaticamente com o Windows")
	open := fs.Bool("open", false, "abrir o painel após iniciar")
	deviceHost := fs.String("device-host", "", "endereço da API dos dispositivos (desenvolvimento; padrão: todas as interfaces)")
	iceHost := fs.String("ice-host", "", "endereço ICE do MediaMTX (desenvolvimento; padrão: todas as interfaces)")
	_ = fs.Parse(os.Args[1:])

	if *dataDir == "" {
		d, err := platform.DataDir()
		if err != nil {
			tray.Alert("SacraLink", "Não foi possível localizar a pasta de dados: "+err.Error())
			return 1
		}
		*dataDir = d
	}
	log, closer, err := logging.Setup(logging.Options{Dir: filepath.Join(*dataDir, "logs"), Level: *level, Console: *dev})
	if err != nil {
		tray.Alert("SacraLink", "Não foi possível criar os logs: "+err.Error())
		return 1
	}
	defer closer.Close()
	slog.SetDefault(log)

	release, ok, err := platform.SingleInstance("SacraLinkServer")
	if err != nil {
		log.Error("falha ao verificar instância única", "erro", err)
	}
	if !ok {
		log.Info("o SacraLink já está em execução")
		if *open {
			openDashboard(def.AdminPort)
		}
		return 0
	}
	defer release()

	osInfo := platform.OS()
	log.Info("iniciando SacraLink", "versao", buildinfo.Version, "windows", osInfo.Name, "build", osInfo.Build, "smart_app_control", osInfo.SmartAppCtrl)
	if osInfo.Warning != "" {
		log.Warn(osInfo.Warning)
	}

	exe, _ := os.Executable()
	cfg := def
	cfg.DataDir, cfg.AdminPort, cfg.DevicePort, cfg.MediaExe, cfg.DevOrigins, cfg.Logger = *dataDir, *adminPort, *devicePort, *mediaExe, *dev, log
	cfg.DashboardDir = filepath.Join(filepath.Dir(exe), "dashboard")
	cfg.DeviceHost, cfg.ICEListenHost = *deviceHost, *iceHost
	cfg.MDNS = !*noMDNS && *deviceHost == ""

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	a, err := app.New(ctx, cfg)
	if err != nil {
		log.Error("falha ao iniciar", "erro", err)
		tray.Alert("SacraLink — erro ao iniciar", friendly(err))
		return 1
	}

	runErr := make(chan error, 1)
	go func() {
		err := a.Run(ctx)
		runErr <- err
		tray.Quit()
	}()
	if *open {
		go func() { time.Sleep(time.Second); openDashboard(cfg.AdminPort) }()
	}

	if *noTray || !tray.Supported {
		err := <-runErr
		return exitCode(log, err)
	}
	tray.Run(tray.Options{
		Icon:    assets.ICO(false, 16, 24, 32, 48),
		Tooltip: "SacraLink",
		Status: func() string {
			connected, streaming := a.Core().Counts()
			obs := "OBS desconectado"
			if a.Core().OBS().Status().State == "connected" {
				obs = "OBS conectado"
			}
			return fmt.Sprintf("%d câmera(s) conectada(s), %d transmitindo · %s", connected, streaming, obs)
		},
		OnOpen:         func() { openDashboard(cfg.AdminPort) },
		OnRestartMedia: func() { a.RestartMedia() },
		OnOpenLogs:     func() { _ = platform.OpenFolder(filepath.Join(*dataDir, "logs")) },
		OnQuit: func() bool {
			if _, streaming := a.Core().Counts(); streaming > 0 {
				if !tray.Confirm("Sair do SacraLink?", fmt.Sprintf("%d câmera(s) estão transmitindo. Ao sair, elas serão desconectadas e as fontes do OBS ficarão sem imagem.\n\nDeseja realmente sair?", streaming)) {
					return false
				}
			}
			stop()
			return true
		},
	})
	stop()
	select {
	case err := <-runErr:
		return exitCode(log, err)
	case <-time.After(10 * time.Second):
		log.Warn("encerramento demorou mais que o esperado")
		return 0
	}
}

func exitCode(log *slog.Logger, err error) int {
	if err != nil {
		log.Error("SacraLink encerrou com erro", "erro", err)
		tray.Alert("SacraLink", friendly(err))
		return 1
	}
	return 0
}

// friendly traduz erros comuns de inicialização em mensagens para o operador.
func friendly(err error) string {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "mediamtx não encontrado"):
		return "O componente de mídia (MediaMTX) não foi encontrado. Reinstale o SacraLink.\n\n" + msg
	case strings.Contains(msg, "em uso"):
		return "Uma porta necessária está em uso. Verifique se outro SacraLink ou programa está aberto.\n\n" + msg
	case strings.Contains(msg, "integridade"):
		return "Um componente do SacraLink foi alterado ou corrompido. Reinstale o SacraLink.\n\n" + msg
	}
	return msg
}

// openDashboard abre o SacraLink.exe (painel) ou, na falta dele, o navegador.
func openDashboard(adminPort int) {
	exe, _ := os.Executable()
	gui := filepath.Join(filepath.Dir(exe), "SacraLink.exe")
	if _, err := os.Stat(gui); err == nil {
		if err := exec.Command(gui).Start(); err == nil {
			return
		}
	}
	_ = platform.OpenURL(fmt.Sprintf("http://127.0.0.1:%d/", adminPort))
}

// firewallCmd cria/remove as regras (deve rodar elevado).
func firewallCmd(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "uso: sacralink-server firewall apply|remove [opções]")
		return 2
	}
	fs := flag.NewFlagSet("firewall", flag.ContinueOnError)
	serverExe := fs.String("server-exe", "", "caminho do sacralink-server.exe")
	mediaExe := fs.String("mediamtx-exe", "", "caminho do mediamtx.exe")
	devicePort := fs.Int("device-port", 7443, "porta dos dispositivos")
	icePort := fs.Int("ice-port", 8189, "porta ICE do WebRTC")
	iceTCP := fs.Bool("ice-tcp", false, "incluir ICE sobre TCP")
	profile := fs.String("profile", "private,domain", "perfis de rede do firewall")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	names := []string{
		fmt.Sprintf("%sDispositivos (TCP %d)", platform.FirewallRulePrefix, *devicePort),
		fmt.Sprintf("%sMídia WebRTC (UDP %d)", platform.FirewallRulePrefix, *icePort),
		fmt.Sprintf("%sMídia WebRTC (TCP %d)", platform.FirewallRulePrefix, *icePort),
		platform.FirewallRulePrefix + "Descoberta na rede (UDP 5353)",
	}
	switch args[0] {
	case "apply":
		if *serverExe == "" || *mediaExe == "" {
			fmt.Fprintln(os.Stderr, "informe --server-exe e --mediamtx-exe")
			return 2
		}
		rules := []platform.FirewallRule{
			{Name: names[0], Program: *serverExe, Protocol: "TCP", Port: *devicePort, Profile: *profile},
			{Name: names[1], Program: *mediaExe, Protocol: "UDP", Port: *icePort, Profile: *profile},
			{Name: names[3], Program: *serverExe, Protocol: "UDP", Port: 5353, Profile: *profile},
		}
		if *iceTCP {
			rules = append(rules, platform.FirewallRule{Name: names[2], Program: *mediaExe, Protocol: "TCP", Port: *icePort, Profile: *profile})
		}
		if err := platform.AddFirewallRules(rules); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	case "remove":
		if err := platform.DeleteFirewallRules(names); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	default:
		fmt.Fprintln(os.Stderr, "ação desconhecida:", args[0])
		return 2
	}
	return 0
}

// adminCmd: "admin reset --confirm" remove a senha do administrador.
func adminCmd(args []string) int {
	fs := flag.NewFlagSet("admin", flag.ContinueOnError)
	confirm := fs.Bool("confirm", false, "confirma a operação")
	dataDir := fs.String("data-dir", "", "diretório de dados")
	if len(args) == 0 || args[0] != "reset" {
		fmt.Fprintln(os.Stderr, "uso: sacralink-server admin reset --confirm")
		return 2
	}
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	if !*confirm {
		fmt.Fprintln(os.Stderr, "isto remove a senha do administrador; repita com --confirm")
		return 2
	}
	if *dataDir == "" {
		d, err := platform.DataDir()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		*dataDir = d
	}
	st, err := store.Open(context.Background(), filepath.Join(*dataDir, "sacralink.db"))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer st.Close()
	if err := st.DeleteAllUsers(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	_ = st.Audit(context.Background(), "cli", "auth.reset", "", "local", true, "")
	fmt.Println("Senha do administrador removida. Abra o painel para criar uma nova.")
	return 0
}
