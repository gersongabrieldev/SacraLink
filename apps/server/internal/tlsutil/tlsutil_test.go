package tlsutil

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLoadOrCreatePersiste(t *testing.T) {
	dir := t.TempDir()
	a, err := LoadOrCreate(dir, "SacraLink teste")
	if err != nil {
		t.Fatal(err)
	}
	b, err := LoadOrCreate(dir, "SacraLink teste")
	if err != nil {
		t.Fatal(err)
	}
	if a.FingerprintHex != b.FingerprintHex || len(a.FingerprintHex) != 64 {
		t.Fatalf("o certificado deveria ser reaproveitado: %s %s", a.FingerprintHex, b.FingerprintHex)
	}
}

func TestPinning(t *testing.T) {
	id, err := LoadOrCreate(t.TempDir(), "SacraLink teste")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) }))
	srv.TLS = id.ServerConfig()
	srv.StartTLS()
	defer srv.Close()

	ok := &http.Client{Transport: &http.Transport{TLSClientConfig: PinnedClientConfig(id.FingerprintHex)}}
	if resp, err := ok.Get(srv.URL); err != nil {
		t.Fatalf("pinning correto deveria conectar: %v", err)
	} else {
		resp.Body.Close()
	}
	other, _ := LoadOrCreate(t.TempDir(), "outro")
	bad := &http.Client{Transport: &http.Transport{TLSClientConfig: PinnedClientConfig(other.FingerprintHex)}}
	if _, err := bad.Get(srv.URL); err == nil {
		t.Fatal("fingerprint errado deveria ser rejeitado")
	}
	if id.ServerConfig().MinVersion != tls.VersionTLS13 {
		t.Fatal("TLS mínimo deveria ser 1.3")
	}
}
