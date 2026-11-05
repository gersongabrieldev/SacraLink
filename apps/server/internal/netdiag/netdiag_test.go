package netdiag

import (
	"context"
	"encoding/binary"
	"net"
	"net/netip"
	"strings"
	"testing"
)

func TestClassify(t *testing.T) {
	cases := map[string]string{
		"127.0.0.1": KindLoopback, "192.168.0.10": KindPrivate, "10.1.2.3": KindPrivate, "172.16.5.5": KindPrivate,
		"100.64.0.1": KindCGNAT, "100.127.255.254": KindCGNAT, "100.128.0.1": KindPublic, "8.8.8.8": KindPublic,
		"169.254.1.1": KindLinkLocal, "fe80::1": KindLinkLocal, "fd00::1": KindULA, "2804:14c::1": KindPublic,
	}
	for ip, want := range cases {
		if got := Classify(netip.MustParseAddr(ip)); got != want {
			t.Errorf("%s: esperado %s, obtido %s", ip, want, got)
		}
	}
}

// fakeSTUN responde Binding Requests com o endereço informado.
func fakeSTUN(t *testing.T, mapped net.IP, port int) string {
	pc, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pc.Close() })
	go func() {
		buf := make([]byte, 1500)
		for {
			n, addr, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			req := buf[:n]
			resp := make([]byte, 20+12)
			binary.BigEndian.PutUint16(resp[0:], 0x0101)
			binary.BigEndian.PutUint16(resp[2:], 12)
			copy(resp[4:20], req[4:20])
			binary.BigEndian.PutUint16(resp[20:], 0x0020)
			binary.BigEndian.PutUint16(resp[22:], 8)
			resp[25] = 0x01
			binary.BigEndian.PutUint16(resp[26:], uint16(port)^uint16(stunMagicCookie>>16))
			ip4 := mapped.To4()
			for i := 0; i < 4; i++ {
				resp[28+i] = ip4[i] ^ req[4+i]
			}
			pc.WriteTo(resp, addr)
		}
	}()
	return pc.LocalAddr().String()
}

func TestSTUNBinding(t *testing.T) {
	addr := fakeSTUN(t, net.ParseIP("203.0.113.7"), 54321)
	ip, port, err := STUNBinding(context.Background(), "udp4", addr)
	if err != nil {
		t.Fatal(err)
	}
	if ip.String() != "203.0.113.7" || port != 54321 {
		t.Fatalf("mapeamento: %s:%d", ip, port)
	}
}

func TestDiagnosticoCGNAT(t *testing.T) {
	addr := fakeSTUN(t, net.ParseIP("100.70.1.2"), 1000)
	r := Run(context.Background(), Options{UseSTUN: true, STUNServers: []string{addr}, RemoteEnabled: true, RemotePort: 7443, ICEPort: 8189})
	if r.CGNAT != "yes" {
		t.Fatalf("CGNAT não detectado: %+v", r)
	}
	found := false
	for _, a := range r.Advice {
		if a.Title == "CGNAT detectado" && a.Level == "error" && strings.Contains(a.Message, "inviável") {
			found = true
		}
	}
	if !found {
		t.Fatalf("orientação de CGNAT ausente: %+v", r.Advice)
	}
}

func TestDiagnosticoIPPublicoEDNS(t *testing.T) {
	addr := fakeSTUN(t, net.ParseIP("203.0.113.7"), 1000)
	r := Run(context.Background(), Options{
		UseSTUN: true, STUNServers: []string{addr}, RemoteEnabled: true, RemoteHost: "igreja.example.org", RemotePort: 7443, ICEPort: 8189,
		Resolver: func(context.Context, string) ([]string, error) { return []string{"198.51.100.1"}, nil },
	})
	if r.PublicIPv4 != "203.0.113.7" {
		t.Fatalf("IP público: %+v", r)
	}
	titles := map[string]bool{}
	for _, a := range r.Advice {
		titles[a.Title] = true
	}
	for _, want := range []string{"Encaminhamento de portas", "DNS desatualizado", "Acesso externo não comprovado"} {
		if !titles[want] {
			t.Fatalf("orientação %q ausente: %+v", want, r.Advice)
		}
	}
}

func TestAcessoRemotoDesligado(t *testing.T) {
	r := Run(context.Background(), Options{})
	for _, a := range r.Advice {
		if a.Title == "CGNAT detectado" {
			t.Fatal("sem acesso remoto não deve haver alerta de CGNAT")
		}
	}
	if len(r.Advice) == 0 || r.Advice[len(r.Advice)-1].Title != "Acesso remoto desligado" {
		t.Fatalf("orientações: %+v", r.Advice)
	}
}

func TestLANHostsPrimeiroPrimario(t *testing.T) {
	ifs := []Interface{
		{Name: "vEthernet (WSL)", Up: true, Virtual: true, Addrs: []Addr{{IP: "172.20.0.1", Family: 4, Kind: KindPrivate}}},
		{Name: "Ethernet", Up: true, Addrs: []Addr{{IP: "192.168.0.10", Family: 4, Kind: KindPrivate}, {IP: "fe80::1", Family: 6, Kind: KindLinkLocal}}},
		{Name: "Wi-Fi", Up: true, Addrs: []Addr{{IP: "192.168.1.20", Family: 4, Kind: KindPrivate}}},
	}
	got := LANHosts(ifs, "192.168.1.20")
	if len(got) != 3 || got[0] != "192.168.1.20" || got[1] != "192.168.0.10" || got[2] != "172.20.0.1" {
		t.Fatalf("ordem: %v", got)
	}
}
