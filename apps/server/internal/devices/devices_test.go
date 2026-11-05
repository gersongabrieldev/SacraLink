package devices

import (
	"context"
	"errors"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gersongabrieldev/sacralink/apps/server/internal/store"
)

func newService(t *testing.T) (*Service, *store.Store) {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "d.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return New(st, func() Endpoints {
		return Endpoints{ServerID: "srv1", ServerName: "Paróquia São José", LANHosts: []string{"192.168.0.10"},
			Port: 7443, FingerprintHex: strings.Repeat("ab", 32)}
	}), st
}

func TestQRNaoContemSegredosReutilizaveis(t *testing.T) {
	s, _ := newService(t)
	p, err := s.CreatePairing(context.Background(), "", "")
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(p.URI)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if u.Scheme != "sacralink" || q.Get("t") != p.Token || q.Get("fp") == "" || q.Get("h") != "192.168.0.10" || q.Get("n") != "Paróquia São José" {
		t.Fatalf("URI inesperada: %s", p.URI)
	}
	for k := range q {
		if k == "pass" || k == "password" || k == "secret" {
			t.Fatalf("QR não pode conter %s", k)
		}
	}
	if len(p.QRPNG) < 100 {
		t.Fatal("PNG do QR não gerado")
	}
}

func TestFluxoCompletoDePareamento(t *testing.T) {
	s, st := newService(t)
	ctx := context.Background()
	p, _ := s.CreatePairing(ctx, "", "")
	res, err := s.Pair(ctx, p.Token, DeviceInfo{Name: "  iPhone do Pedro ", Platform: "ios", Model: "iPhone 15"}, "192.168.0.50")
	if err != nil {
		t.Fatal(err)
	}
	if res.Secret == "" || res.CameraName != "Câmera 01" || res.ServerName != "Paróquia São José" {
		t.Fatalf("resultado inesperado: %+v", res)
	}
	d, c, err := s.Authenticate(ctx, "Bearer "+res.DeviceID+"."+res.Secret)
	if err != nil {
		t.Fatal(err)
	}
	if d.Name != "iPhone do Pedro" || c.ID != res.CameraID {
		t.Fatalf("autenticação inesperada: %+v %+v", d, c)
	}
	// O segredo nunca é armazenado em texto puro.
	stored, _ := st.GetDevice(ctx, res.DeviceID)
	if strings.Contains(stored.SecretHash, res.Secret) {
		t.Fatal("segredo armazenado em texto puro")
	}
	// Token de uso único.
	if _, err := s.Pair(ctx, p.Token, DeviceInfo{Name: "x", Platform: "android"}, ""); !errors.Is(err, store.ErrPairingInvalid) {
		t.Fatalf("reuso do token deveria falhar: %v", err)
	}
	// Segredo errado.
	if _, _, err := s.Authenticate(ctx, "Bearer "+res.DeviceID+".segredo-errado-xxxxxxxxxxxxxxx"); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("segredo errado: %v", err)
	}
	// Revogação e bloqueio.
	_ = st.SetDeviceStatus(ctx, res.DeviceID, store.DeviceRevoked)
	if _, _, err := s.Authenticate(ctx, "Bearer "+res.DeviceID+"."+res.Secret); !errors.Is(err, ErrRevoked) {
		t.Fatalf("revogado: %v", err)
	}
	_ = st.SetDeviceStatus(ctx, res.DeviceID, store.DeviceBlocked)
	if _, _, err := s.Authenticate(ctx, "Bearer "+res.DeviceID+"."+res.Secret); !errors.Is(err, ErrBlocked) {
		t.Fatalf("bloqueado: %v", err)
	}
}

func TestValidacaoDeEntrada(t *testing.T) {
	d := DeviceInfo{Name: "ok\x00", Platform: "android"}
	if err := d.Validate(); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("caractere de controle deveria ser rejeitado: %v", err)
	}
	d = DeviceInfo{Name: strings.Repeat("a", 61)}
	if err := d.Validate(); !errors.Is(err, ErrInvalidInput) {
		t.Fatal("nome longo deveria ser rejeitado")
	}
	d = DeviceInfo{Name: "", Platform: "symbian"}
	if err := d.Validate(); err != nil || d.Name != "Celular" || d.Platform != "other" {
		t.Fatalf("normalização: %+v %v", d, err)
	}
}

func TestParseBearer(t *testing.T) {
	for _, h := range []string{"", "Basic abc", "Bearer semponto", "Bearer a.b", "Bearer dxxxxxxxx." + strings.Repeat("s", 200)} {
		if _, _, ok := ParseBearer(h); ok {
			t.Fatalf("deveria rejeitar %q", h)
		}
	}
	id, sec, ok := ParseBearer("Bearer dabcdefgh." + strings.Repeat("s", 43))
	if !ok || id != "dabcdefgh" || len(sec) != 43 {
		t.Fatal("bearer válido rejeitado")
	}
}
