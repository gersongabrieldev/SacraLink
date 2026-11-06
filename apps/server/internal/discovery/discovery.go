// Package discovery anuncia o servidor na rede local via mDNS/DNS-SD
// (_sacralink._tcp). Serve para celulares JÁ PAREADOS reencontrarem o
// computador da igreja quando o IP muda (DHCP). O pareamento continua
// exclusivamente pelo QR Code: anúncios mDNS podem ser falsificados, e a
// identidade do servidor é sempre confirmada pelo pinning do certificado.
package discovery

import (
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"unicode"

	"github.com/libp2p/zeroconf/v2"

	"github.com/gersongabrieldev/sacralink/apps/server/internal/netdiag"
)

// ServiceType é o tipo DNS-SD do SacraLink.
const ServiceType = "_sacralink._tcp"

// Info descreve o anúncio.
type Info struct {
	ServerID       string
	ServerName     string
	Port           int
	FingerprintHex string
}

// TXT monta os registros TXT do anúncio (sem dados sensíveis).
func TXT(i Info) []string {
	return []string{"v=1", "id=" + i.ServerID, "fp=" + i.FingerprintHex}
}

// InstanceName gera um nome de instância DNS-SD seguro (até 63 bytes).
func InstanceName(serverName string) string {
	var b strings.Builder
	for _, r := range "SacraLink " + serverName {
		if unicode.IsPrint(r) && r != '.' && r != '\\' {
			b.WriteRune(r)
		}
	}
	s := strings.TrimSpace(b.String())
	for len(s) > 63 {
		r := []rune(s)
		s = string(r[:len(r)-1])
	}
	return s
}

// Announcer mantém o anúncio ativo.
type Announcer struct {
	mu     sync.Mutex
	server *zeroconf.Server
	log    *slog.Logger
}

// New cria o anunciador.
func New(log *slog.Logger) *Announcer {
	if log == nil {
		log = slog.Default()
	}
	return &Announcer{log: log.With("componente", "mdns")}
}

// physicalInterfaces evita anunciar em interfaces virtuais (WSL, VPN, Hyper-V).
func physicalInterfaces() []net.Interface {
	ifs, _ := netdiag.Interfaces()
	byName := map[string]bool{}
	for _, it := range ifs {
		if it.Up && !it.Virtual {
			byName[it.Name] = true
		}
	}
	all, _ := net.Interfaces()
	var out []net.Interface
	for _, nif := range all {
		if byName[nif.Name] && nif.Flags&net.FlagMulticast != 0 {
			out = append(out, nif)
		}
	}
	return out
}

// Start (re)inicia o anúncio.
func (a *Announcer) Start(i Info) error {
	a.Stop()
	ifaces := physicalInterfaces()
	if len(ifaces) == 0 {
		return fmt.Errorf("nenhuma interface de rede física com multicast")
	}
	srv, err := zeroconf.Register(InstanceName(i.ServerName), ServiceType, "local.", i.Port, TXT(i), ifaces)
	if err != nil {
		return fmt.Errorf("anúncio mDNS: %w", err)
	}
	a.mu.Lock()
	a.server = srv
	a.mu.Unlock()
	a.log.Info("servidor anunciado na rede local", "tipo", ServiceType, "porta", i.Port, "interfaces", len(ifaces))
	return nil
}

// Stop encerra o anúncio.
func (a *Announcer) Stop() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.server != nil {
		a.server.Shutdown()
		a.server = nil
	}
}
