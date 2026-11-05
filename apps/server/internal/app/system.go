package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/gersongabrieldev/sacralink/apps/server/internal/api"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/buildinfo"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/devices"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/netdiag"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/obsbridge"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/platform"
)

// Status implementa api.System.
func (a *App) Status(ctx context.Context) any {
	brief := a.systemBrief()
	brief["version"] = buildinfo.Get()
	brief["os"] = platform.OS()
	brief["obs"] = a.bridge.Status()
	brief["adminUrl"] = a.AdminURL()
	brief["devicePort"] = a.cfg.DevicePort
	brief["dataDir"] = a.cfg.DataDir
	brief["lanHosts"] = a.Endpoints().LANHosts
	brief["fingerprint"] = a.identity.FingerprintHex
	brief["certificateExpires"] = a.identity.NotAfter.UnixMilli()
	brief["autostart"] = platform.AutostartEnabled()
	brief["serverId"] = a.serverID
	return brief
}

// Endpoints implementa api.System e alimenta o QR Code.
func (a *App) Endpoints() devices.Endpoints {
	s := a.settings.Get()
	a.netMu.Lock()
	rep := a.report
	a.netMu.Unlock()
	var hosts []string
	if rep != nil && time.Since(time.UnixMilli(rep.GeneratedAt)) < time.Minute {
		hosts = rep.LANHosts
	} else {
		ifs, _ := netdiag.Interfaces()
		hosts = netdiag.LANHosts(ifs, netdiag.PrimaryIPv4())
	}
	e := devices.Endpoints{ServerID: a.serverID, ServerName: s.ServerName, LANHosts: hosts, Port: a.cfg.DevicePort,
		FingerprintHex: a.identity.FingerprintHex}
	if s.Remote.Enabled && s.Remote.PublicHost != "" {
		e.ExternalHost, e.ExternalPort = s.Remote.PublicHost, s.Remote.PublicPort
	}
	return e
}

func (a *App) firewallRules() []platform.FirewallRule {
	self, _ := os.Executable()
	profile := "private,domain"
	rules := []platform.FirewallRule{
		{Name: platform.FirewallRulePrefix + fmt.Sprintf("Dispositivos (TCP %d)", a.cfg.DevicePort), Program: self, Protocol: "TCP", Port: a.cfg.DevicePort, Profile: profile},
		{Name: platform.FirewallRulePrefix + fmt.Sprintf("Mídia WebRTC (UDP %d)", a.cfg.MediaPorts.ICE), Program: a.cfg.MediaExe, Protocol: "UDP", Port: a.cfg.MediaPorts.ICE, Profile: profile},
	}
	if a.settings.Get().ICETCP {
		rules = append(rules, platform.FirewallRule{Name: platform.FirewallRulePrefix + fmt.Sprintf("Mídia WebRTC (TCP %d)", a.cfg.MediaPorts.ICE),
			Program: a.cfg.MediaExe, Protocol: "TCP", Port: a.cfg.MediaPorts.ICE, Profile: profile})
	}
	return rules
}

// FirewallStatus implementa api.System.
func (a *App) FirewallStatus() any {
	type ruleView struct {
		platform.FirewallRule
		Exists bool `json:"exists"`
	}
	var out []ruleView
	for _, r := range a.firewallRules() {
		out = append(out, ruleView{r, platform.FirewallRuleExists(r.Name)})
	}
	return map[string]any{"rules": out, "elevated": platform.IsElevated()}
}

// ApplyFirewall implementa api.System. Sem privilégio de administrador,
// pede elevação pelo UAC executando o próprio servidor com "firewall apply".
func (a *App) ApplyFirewall(ctx context.Context) error {
	rules := a.firewallRules()
	if platform.IsElevated() {
		return platform.AddFirewallRules(rules)
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	args := fmt.Sprintf(`firewall apply --server-exe "%s" --mediamtx-exe "%s" --device-port %d --ice-port %d`,
		self, a.cfg.MediaExe, a.cfg.DevicePort, a.cfg.MediaPorts.ICE)
	if a.settings.Get().ICETCP {
		args += " --ice-tcp"
	}
	code, err := platform.RunElevated(self, args, 60*time.Second)
	if errors.Is(err, platform.ErrElevationCancelled) {
		return &api.Error{Status: 403, Code: "elevation_cancelled", Message: "permissão de administrador negada; as regras não foram criadas"}
	}
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("a configuração do firewall falhou (código %d)", code)
	}
	return nil
}

// SetAutostart implementa api.System.
func (a *App) SetAutostart(enabled bool) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	return platform.SetAutostart(enabled, self, "--background")
}

// Network implementa api.System.
func (a *App) Network(ctx context.Context, refresh bool) netdiag.Report {
	a.netMu.Lock()
	cached, reach := a.report, a.reach
	a.netMu.Unlock()
	if cached != nil && !refresh && time.Since(time.UnixMilli(cached.GeneratedAt)) < 30*time.Second {
		return *cached
	}
	s := a.settings.Get()
	ports := []netdiag.PortCheck{
		{Name: "Dispositivos", Protocol: "TCP", Port: a.cfg.DevicePort, Exposed: true, Listening: true},
		{Name: "Mídia WebRTC", Protocol: "UDP", Port: a.cfg.MediaPorts.ICE, Exposed: true, Listening: a.sup.Status().State == "running"},
		{Name: "Painel (somente local)", Protocol: "TCP", Port: a.cfg.AdminPort, Listening: true, FirewallRule: true},
	}
	for i, r := range a.firewallRules() {
		if i < 2 {
			ports[i].FirewallRule = platform.FirewallRuleExists(r.Name)
		}
	}
	rep := netdiag.Run(ctx, netdiag.Options{
		UseSTUN: s.Remote.UseSTUN || s.Remote.Enabled, Ports: ports, RemoteEnabled: s.Remote.Enabled,
		RemoteHost: s.Remote.PublicHost, RemotePort: s.Remote.PublicPort, ICEPort: a.cfg.MediaPorts.ICE, Reachability: reach,
	})
	a.netMu.Lock()
	a.report = &rep
	a.netMu.Unlock()
	return rep
}

// RecordReachability implementa api.System.
func (a *App) RecordReachability(ip, network string) {
	a.netMu.Lock()
	a.reach = &netdiag.Reachability{ConfirmedAt: time.Now().UnixMilli(), FromIP: ip, Network: network}
	a.netMu.Unlock()
	_, _ = a.store.AddEvent(context.Background(), "info", "external_reachability", "", "acesso externo confirmado a partir de "+ip, map[string]string{"network": network})
}

// ---------------------------------------------------------------------------
// OBS

func (a *App) loadOBSConfig(ctx context.Context) error {
	c, err := a.store.GetOBSConnection(ctx)
	if err != nil {
		return err
	}
	pw := ""
	if len(c.PasswordEnc) > 0 {
		b, err := platform.Unprotect(c.PasswordEnc)
		if err != nil {
			a.log.Warn("não foi possível decifrar a senha do OBS (outro usuário do Windows?); reconfigure", "erro", err)
		} else {
			pw = string(b)
		}
	}
	// Só conecta automaticamente se o operador já configurou o OBS.
	auto := c.AutoConnect && c.UpdatedAt > 0
	a.bridge.Configure(obsbridge.Config{Host: c.Host, Port: c.Port, Password: pw, AutoConnect: auto})
	return nil
}

// OBSConfig implementa api.System.
func (a *App) OBSConfig(ctx context.Context) (api.OBSConfigView, error) {
	c, err := a.store.GetOBSConnection(ctx)
	if err != nil {
		return api.OBSConfigView{}, err
	}
	return api.OBSConfigView{Host: c.Host, Port: c.Port, HasPassword: len(c.PasswordEnc) > 0, AutoConnect: c.AutoConnect && c.UpdatedAt > 0,
		LastOBSVersion: c.LastOBSVersion, LastWSVersion: c.LastWSVersion}, nil
}

// SaveOBSConfig implementa api.System. A senha é cifrada com DPAPI.
func (a *App) SaveOBSConfig(ctx context.Context, in api.OBSConfigInput) error {
	host := strings.TrimSpace(in.Host)
	if host != "127.0.0.1" && host != "localhost" && host != "::1" {
		// O obs-websocket nunca deve ser exposto; aceitar outro host só faz
		// sentido em redes de produção dedicadas e fica registrado.
		a.log.Warn("OBS configurado fora do loopback", "host", host)
	}
	c, err := a.store.GetOBSConnection(ctx)
	if err != nil {
		return err
	}
	c.Host, c.Port, c.AutoConnect = host, in.Port, in.AutoConnect
	if in.Password != nil {
		if *in.Password == "" {
			c.PasswordEnc = nil
		} else if c.PasswordEnc, err = platform.Protect([]byte(*in.Password)); err != nil {
			return err
		}
	}
	if err := a.store.SaveOBSConnection(ctx, c); err != nil {
		return err
	}
	return a.loadOBSConfig(ctx)
}

// ImportLocalOBSConfig implementa api.System: lê o config.json do
// obs-websocket deste usuário (ação explícita do operador).
func (a *App) ImportLocalOBSConfig(ctx context.Context) (api.OBSConfigView, error) {
	ws, err := platform.ReadOBSWebSocketConfig()
	if err != nil {
		return api.OBSConfigView{}, err
	}
	if !ws.Enabled {
		return api.OBSConfigView{}, errors.New("o servidor WebSocket do OBS está desligado (Ferramentas → Configurações do WebSocket)")
	}
	pw := ""
	if ws.AuthRequired {
		pw = ws.Password
	}
	port := ws.Port
	if port == 0 {
		port = 4455
	}
	if err := a.SaveOBSConfig(ctx, api.OBSConfigInput{Host: "127.0.0.1", Port: port, Password: &pw, AutoConnect: true}); err != nil {
		return api.OBSConfigView{}, err
	}
	return a.OBSConfig(ctx)
}

// DetectOBS implementa api.System (sem expor a senha do obs-websocket).
func (a *App) DetectOBS() any {
	inst := platform.DetectOBS()
	out := map[string]any{"found": inst.Found, "path": inst.Path, "version": inst.Version, "running": inst.Running,
		"supported": inst.Version == "" || versionAtLeast(inst.Version, 28)}
	if inst.WebSocket != nil {
		out["websocket"] = map[string]any{"enabled": inst.WebSocket.Enabled, "port": inst.WebSocket.Port, "authRequired": inst.WebSocket.AuthRequired}
	}
	return out
}

func versionAtLeast(v string, major int) bool {
	var m int
	fmt.Sscanf(v, "%d", &m)
	return m >= major
}

// Logs implementa api.System.
func (a *App) Logs() map[string][]string {
	return map[string][]string{"mediamtx": a.sup.RecentLogs()}
}

var _ api.System = (*App)(nil)
