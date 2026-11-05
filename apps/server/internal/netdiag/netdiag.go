package netdiag

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"sort"
	"strings"
	"sync"
	"time"
)

// Classificações de endereço.
const (
	KindLoopback  = "loopback"
	KindLinkLocal = "link_local"
	KindPrivate   = "private" // RFC 1918
	KindCGNAT     = "cgnat"   // 100.64.0.0/10 (RFC 6598)
	KindULA       = "ula"     // fc00::/7
	KindPublic    = "public"
)

var cgnat = netip.MustParsePrefix("100.64.0.0/10")

// Classify classifica um endereço IP.
func Classify(ip netip.Addr) string {
	ip = ip.Unmap()
	switch {
	case ip.IsLoopback():
		return KindLoopback
	case ip.IsLinkLocalUnicast():
		return KindLinkLocal
	case ip.Is4() && cgnat.Contains(ip):
		return KindCGNAT
	case ip.Is4() && ip.IsPrivate():
		return KindPrivate
	case ip.Is6() && ip.IsPrivate():
		return KindULA
	case ip.IsGlobalUnicast():
		return KindPublic
	}
	return KindLinkLocal
}

// Addr é um endereço de interface.
type Addr struct {
	IP     string `json:"ip"`
	Family int    `json:"family"`
	Kind   string `json:"kind"`
}

// Interface é uma interface de rede.
type Interface struct {
	Name    string `json:"name"`
	Up      bool   `json:"up"`
	Virtual bool   `json:"virtual"`
	Addrs   []Addr `json:"addrs"`
}

var virtualHints = []string{"vethernet", "virtualbox", "vmware", "hyper-v", "wsl", "docker", "loopback", "tailscale", "zerotier", "vpn", "tap-", "wintun"}

func isVirtual(name string) bool {
	n := strings.ToLower(name)
	for _, h := range virtualHints {
		if strings.Contains(n, h) {
			return true
		}
	}
	return false
}

// Interfaces lista as interfaces com endereços classificados.
func Interfaces() ([]Interface, error) {
	ifs, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var out []Interface
	for _, nif := range ifs {
		if nif.Flags&net.FlagLoopback != 0 {
			continue
		}
		it := Interface{Name: nif.Name, Up: nif.Flags&net.FlagUp != 0, Virtual: isVirtual(nif.Name)}
		addrs, _ := nif.Addrs()
		for _, a := range addrs {
			pfx, err := netip.ParsePrefix(a.String())
			if err != nil {
				continue
			}
			ip := pfx.Addr()
			fam := 4
			if ip.Is6() && !ip.Is4In6() {
				fam = 6
			}
			it.Addrs = append(it.Addrs, Addr{IP: ip.String(), Family: fam, Kind: Classify(ip)})
		}
		if len(it.Addrs) > 0 {
			out = append(out, it)
		}
	}
	return out, nil
}

// PrimaryIPv4 retorna o IPv4 local usado para a rota padrão (sem enviar
// pacotes: um "connect" UDP apenas consulta a tabela de rotas).
func PrimaryIPv4() string {
	c, err := net.Dial("udp4", "192.0.2.1:9") // TEST-NET-1, nunca roteado de fato
	if err != nil {
		return ""
	}
	defer c.Close()
	if a, ok := c.LocalAddr().(*net.UDPAddr); ok {
		return a.IP.String()
	}
	return ""
}

// LANHosts retorna os endereços para o QR Code: IPv4 primário primeiro,
// depois outros IPv4 privados de interfaces físicas (até 4).
func LANHosts(ifs []Interface, primary string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(ip string) {
		if ip != "" && !seen[ip] && len(out) < 4 {
			seen[ip] = true
			out = append(out, ip)
		}
	}
	add(primary)
	for _, virtualPass := range []bool{false, true} {
		for _, it := range ifs {
			if !it.Up || it.Virtual != virtualPass {
				continue
			}
			for _, a := range it.Addrs {
				if a.Family == 4 && (a.Kind == KindPrivate || a.Kind == KindCGNAT) {
					add(a.IP)
				}
			}
		}
	}
	return out
}

// PortCheck é o estado de uma porta do SacraLink.
type PortCheck struct {
	Name         string `json:"name"`
	Protocol     string `json:"protocol"`
	Port         int    `json:"port"`
	Exposed      bool   `json:"exposed"` // deve ser acessível pela rede
	Listening    bool   `json:"listening"`
	FirewallRule bool   `json:"firewallRule"`
}

// Advice é uma orientação ao administrador.
type Advice struct {
	Level   string `json:"level"` // ok | info | warn | error
	Title   string `json:"title"`
	Message string `json:"message"`
}

// Report é o resultado do diagnóstico.
type Report struct {
	GeneratedAt   int64         `json:"generatedAt"`
	Interfaces    []Interface   `json:"interfaces"`
	PrimaryIPv4   string        `json:"primaryIpv4"`
	LANHosts      []string      `json:"lanHosts"`
	PublicIPv4    string        `json:"publicIpv4,omitempty"`
	PublicIPv6    string        `json:"publicIpv6,omitempty"`
	GlobalIPv6    []string      `json:"globalIpv6,omitempty"`
	STUNError     string        `json:"stunError,omitempty"`
	Internet      bool          `json:"internet"`
	BehindNAT     bool          `json:"behindNat"`
	CGNAT         string        `json:"cgnat"` // no | likely | yes | unknown
	Ports         []PortCheck   `json:"ports"`
	RemoteEnabled bool          `json:"remoteEnabled"`
	RemoteHost    string        `json:"remoteHost,omitempty"`
	RemoteHostIPs []string      `json:"remoteHostIps,omitempty"`
	Reachability  *Reachability `json:"externalReachability,omitempty"`
	Advice        []Advice      `json:"advice"`
}

// Reachability registra a confirmação de acesso externo feita por um
// celular fora da rede local (único teste conclusivo sem servidor externo).
type Reachability struct {
	ConfirmedAt int64  `json:"confirmedAt"`
	FromIP      string `json:"fromIp"`
	Network     string `json:"network"`
}

// Options configura o diagnóstico.
type Options struct {
	STUNServers   []string
	UseSTUN       bool
	Ports         []PortCheck
	RemoteEnabled bool
	RemoteHost    string
	RemotePort    int
	ICEPort       int
	Reachability  *Reachability
	Resolver      func(ctx context.Context, host string) ([]string, error)
}

// DefaultSTUNServers são servidores STUN públicos (apenas descoberta).
var DefaultSTUNServers = []string{"stun.l.google.com:19302", "stun.cloudflare.com:3478"}

// Run executa o diagnóstico.
func Run(ctx context.Context, o Options) Report {
	r := Report{GeneratedAt: time.Now().UnixMilli(), CGNAT: "unknown", Ports: o.Ports,
		RemoteEnabled: o.RemoteEnabled, RemoteHost: o.RemoteHost, Reachability: o.Reachability}
	r.Interfaces, _ = Interfaces()
	r.PrimaryIPv4 = PrimaryIPv4()
	r.LANHosts = LANHosts(r.Interfaces, r.PrimaryIPv4)
	localPublic4 := false
	for _, it := range r.Interfaces {
		for _, a := range it.Addrs {
			if a.Kind == KindPublic && a.Family == 6 && it.Up {
				r.GlobalIPv6 = append(r.GlobalIPv6, a.IP)
			}
			if a.Kind == KindPublic && a.Family == 4 && it.Up {
				localPublic4 = true
			}
		}
	}
	sort.Strings(r.GlobalIPv6)

	if o.UseSTUN {
		servers := o.STUNServers
		if len(servers) == 0 {
			servers = DefaultSTUNServers
		}
		var wg sync.WaitGroup
		var mu sync.Mutex
		var errs []string
		for _, fam := range []string{"udp4", "udp6"} {
			wg.Add(1)
			go func(fam string) {
				defer wg.Done()
				for _, s := range servers {
					sctx, cancel := context.WithTimeout(ctx, 2*time.Second)
					ip, _, err := STUNBinding(sctx, fam, s)
					cancel()
					mu.Lock()
					if err == nil {
						if fam == "udp4" {
							r.PublicIPv4 = ip.String()
						} else {
							r.PublicIPv6 = ip.String()
						}
						mu.Unlock()
						return
					}
					errs = append(errs, fmt.Sprintf("%s (%s): %v", s, fam, err))
					mu.Unlock()
				}
			}(fam)
		}
		wg.Wait()
		r.Internet = r.PublicIPv4 != "" || r.PublicIPv6 != ""
		if !r.Internet && len(errs) > 0 {
			r.STUNError = errs[0]
		}
	}
	if r.PublicIPv4 != "" {
		pub, _ := netip.ParseAddr(r.PublicIPv4)
		r.BehindNAT = !localPublic4
		switch {
		case Classify(pub) == KindCGNAT:
			r.CGNAT = "yes"
		case localPublic4:
			r.CGNAT = "no"
		default:
			// Não há como confirmar CGNAT sem ver o IP WAN do roteador.
			r.CGNAT = "unknown"
		}
	}
	for _, a := range r.LANHosts {
		if ip, err := netip.ParseAddr(a); err == nil && Classify(ip) == KindCGNAT {
			r.CGNAT = "likely"
		}
	}
	if o.RemoteEnabled && o.RemoteHost != "" {
		resolve := o.Resolver
		if resolve == nil {
			resolve = net.DefaultResolver.LookupHost
		}
		if ip, err := netip.ParseAddr(o.RemoteHost); err == nil {
			r.RemoteHostIPs = []string{ip.String()}
		} else if ips, err := resolve(ctx, o.RemoteHost); err == nil {
			r.RemoteHostIPs = ips
		}
	}
	r.Advice = advise(r, o)
	return r
}

func advise(r Report, o Options) []Advice {
	var a []Advice
	add := func(level, title, msg string) { a = append(a, Advice{level, title, msg}) }

	if len(r.LANHosts) == 0 {
		add("error", "Sem rede local", "Nenhum endereço IPv4 de rede local foi encontrado. Conecte o computador ao roteador (de preferência por cabo).")
	} else {
		add("ok", "Rede local", fmt.Sprintf("Os celulares no mesmo Wi-Fi podem conectar em %s (porta %d). A rede local funciona mesmo sem internet.", r.LANHosts[0], portOf(o.Ports, "Dispositivos")))
	}
	for _, p := range r.Ports {
		if !p.Listening {
			add("error", "Porta "+p.Name, fmt.Sprintf("%s %d não está escutando. Reinicie os serviços pelo painel; se persistir, outro programa pode estar usando a porta.", p.Protocol, p.Port))
		} else if p.Exposed && !p.FirewallRule {
			add("warn", "Firewall: "+p.Name, fmt.Sprintf("Não encontramos a regra do Firewall do Windows para %s %d. Use \"Configurar firewall\" no painel (requer permissão de administrador).", p.Protocol, p.Port))
		}
	}
	if !o.UseSTUN {
		add("info", "Internet não verificada", "A descoberta do IP público (STUN) está desligada. Ela só é necessária para o acesso remoto por 4G/5G.")
	} else if !r.Internet {
		add("info", "Sem acesso à internet", "Não foi possível descobrir o IP público. O SacraLink continua funcionando na rede local; o acesso por 4G/5G não estará disponível.")
	}
	if !o.RemoteEnabled {
		add("info", "Acesso remoto desligado", "Somente celulares da rede local podem conectar. Habilite o acesso remoto apenas se precisar transmitir por 4G/5G (ex.: procissões).")
		return a
	}
	switch r.CGNAT {
	case "yes", "likely":
		msg := "Sua conexão está atrás de CGNAT (endereço na faixa 100.64.0.0/10). Nesse cenário a operadora não encaminha conexões de entrada por IPv4 e a conexão direta pela internet é inviável."
		if len(r.GlobalIPv6) > 0 {
			msg += " Há IPv6 global neste computador: celulares com IPv6 podem conectar se o firewall do roteador permitir entrada nas portas do SacraLink."
		} else {
			msg += " Soluções: solicitar IP público (fixo ou dinâmico) ou IPv6 à operadora, ou contratar plano empresarial. O SacraLink não usa VPS/relay por projeto."
		}
		add("error", "CGNAT detectado", msg)
	case "unknown":
		if r.PublicIPv4 != "" {
			add("warn", "Verifique o roteador", fmt.Sprintf("O IP público visto pela internet é %s. Se o IP WAN mostrado no seu roteador for diferente ou começar com 100.64–100.127, a operadora usa CGNAT e a conexão por IPv4 não funcionará.", r.PublicIPv4))
		}
	}
	if r.PublicIPv4 != "" && r.CGNAT != "yes" {
		add("info", "Encaminhamento de portas", fmt.Sprintf("No roteador, encaminhe TCP %d e UDP %d (e TCP %d) para %s. Sem isso, celulares em 4G/5G não alcançam o computador.",
			o.RemotePort, o.ICEPort, o.ICEPort, r.PrimaryIPv4))
	}
	if len(r.GlobalIPv6) > 0 {
		add("info", "IPv6 disponível", "Este computador tem IPv6 global. Muitas operadoras móveis usam IPv6; libere a entrada no firewall do roteador para as portas do SacraLink.")
	}
	if o.RemoteHost != "" && r.PublicIPv4 != "" && len(r.RemoteHostIPs) > 0 {
		match := false
		for _, ip := range r.RemoteHostIPs {
			if ip == r.PublicIPv4 || ip == r.PublicIPv6 {
				match = true
			}
		}
		if !match {
			add("warn", "DNS desatualizado", fmt.Sprintf("%s aponta para %s, mas o IP público atual é %s. Verifique o serviço de DNS dinâmico.", o.RemoteHost, strings.Join(r.RemoteHostIPs, ", "), r.PublicIPv4))
		}
	}
	if r.Reachability != nil {
		add("ok", "Acesso externo confirmado", fmt.Sprintf("Um celular conectou de fora da rede (%s, %s).", r.Reachability.FromIP, r.Reachability.Network))
	} else {
		add("warn", "Acesso externo não comprovado", "O único teste conclusivo é feito de fora: no app, desligue o Wi-Fi e use \"Testar conexão externa\" pela rede móvel.")
	}
	return a
}

func portOf(ports []PortCheck, name string) int {
	for _, p := range ports {
		if strings.Contains(p.Name, name) {
			return p.Port
		}
	}
	return 0
}
