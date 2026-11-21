package update

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestCompare(t *testing.T) {
	order := []string{"0.1.0", "0.1.1", "0.2.0-rc.1", "0.2.0-rc.2", "0.2.0-rc.10", "0.2.0", "1.0.0-alpha", "1.0.0-alpha.1", "1.0.0-beta", "1.0.0", "1.10.0"}
	for i := 0; i+1 < len(order); i++ {
		a, ok1 := parse(order[i])
		b, ok2 := parse("v" + order[i+1])
		if !ok1 || !ok2 {
			t.Fatalf("parse %q/%q", order[i], order[i+1])
		}
		if compare(a, b) >= 0 || compare(b, a) <= 0 {
			t.Fatalf("%s deveria ser menor que %s", order[i], order[i+1])
		}
	}
	for _, bad := range []string{"", "1.0", "1.0.0.0", "a.b.c", "1.0.0-", "-1.0.0"} {
		if _, ok := parse(bad); ok {
			t.Fatalf("%q deveria ser inválida", bad)
		}
	}
}

const releases = `[
 {"tag_name":"v0.3.0","name":"rascunho","draft":true,"prerelease":true,"html_url":"https://github.com/gersongabrieldev/SacraLink/releases/draft"},
 {"tag_name":"v0.2.0","name":"SacraLink 0.2.0","draft":false,"prerelease":true,"html_url":"https://github.com/gersongabrieldev/SacraLink/releases/v0.2.0",
  "published_at":"2026-11-21T18:00:00Z",
  "assets":[{"name":"SHA256SUMS.txt","browser_download_url":"https://github.com/gersongabrieldev/SacraLink/releases/sums"},{"name":"SacraLink-Setup-x64.exe","browser_download_url":"https://github.com/gersongabrieldev/SacraLink/releases/setup.exe"}]},
 {"tag_name":"v0.1.0","name":"SacraLink 0.1.0","draft":false,"prerelease":true,"html_url":"https://github.com/gersongabrieldev/SacraLink/releases/v0.1.0"},
 {"tag_name":"nightly","draft":false,"prerelease":true}
]`

func server(t *testing.T, body string, hits *atomic.Int32) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.Header.Get("User-Agent") == "" {
			t.Error("GitHub exige User-Agent")
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(s.Close)
	return s
}

func TestCheckFindsNewerRelease(t *testing.T) {
	var hits atomic.Int32
	s := server(t, releases, &hits)
	c := New("0.1.0", nil, nil)
	c.URL = s.URL
	st, err := c.CheckNow(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !st.Available || st.Latest == nil || st.Latest.Version != "0.2.0" || st.Latest.InstallerURL != "https://github.com/gersongabrieldev/SacraLink/releases/setup.exe" {
		t.Fatalf("esperava 0.2.0 com instalador (o rascunho 0.3.0 não conta): %+v", st.Latest)
	}
	// Pedido repetido em menos de 1 min usa o resultado guardado.
	if _, err := c.CheckNow(context.Background()); err != nil || hits.Load() != 1 {
		t.Fatalf("consultas = %d, erro %v", hits.Load(), err)
	}
}

func TestCheckUpToDateAndStableChannel(t *testing.T) {
	var hits atomic.Int32
	s := server(t, releases, &hits)
	c := New("0.2.0", nil, nil)
	c.URL = s.URL
	if st, err := c.CheckNow(context.Background()); err != nil || st.Available {
		t.Fatalf("0.2.0 já é a mais nova: %+v %v", st, err)
	}
	// Numa versão estável (1.x), pré-lançamentos não são oferecidos.
	stable := `[{"tag_name":"v1.1.0-rc.1","prerelease":true},{"tag_name":"v1.0.1","prerelease":false,"html_url":"https://github.com/gersongabrieldev/SacraLink/releases/v1.0.1"}]`
	s2 := server(t, stable, &hits)
	c2 := New("1.0.0", nil, nil)
	c2.URL = s2.URL
	st, err := c2.CheckNow(context.Background())
	if err != nil || !st.Available || st.Latest.Version != "1.0.1" {
		t.Fatalf("esperava 1.0.1 (sem o rc): %+v %v", st.Latest, err)
	}
}

func TestCheckDisabledAndDevBuild(t *testing.T) {
	var hits atomic.Int32
	s := server(t, releases, &hits)
	c := New("0.1.0", func() bool { return false }, nil)
	c.URL = s.URL
	_, _ = c.CheckNow(context.Background())
	if st := c.Status(); st.Enabled || st.Available || hits.Load() != 0 {
		t.Fatalf("desligado não pode consultar nem avisar: %+v (consultas %d)", st, hits.Load())
	}
	dev := New("0.0.0-dev", nil, nil)
	dev.URL = s.URL
	if _, err := dev.CheckNow(context.Background()); !errors.Is(err, ErrDevBuild) {
		t.Fatalf("build de desenvolvimento: %v", err)
	}
}

func TestCheckIgnoresForeignLinks(t *testing.T) {
	var hits atomic.Int32
	evil := `[{"tag_name":"v9.0.0","html_url":"https://evil.example/r",
	  "assets":[{"name":"SacraLink-Setup-x64.exe","browser_download_url":"http://evil.example/setup.exe"}]}]`
	s := server(t, evil, &hits)
	c := New("0.1.0", nil, nil)
	c.URL = s.URL
	st, err := c.CheckNow(context.Background())
	if err != nil || st.Latest == nil || st.Latest.URL != "" || st.Latest.InstallerURL != "" {
		t.Fatalf("links fora do GitHub não podem chegar ao painel: %+v %v", st.Latest, err)
	}
}

func TestCheckOffline(t *testing.T) {
	c := New("0.1.0", nil, nil)
	c.URL = "http://127.0.0.1:1/releases"
	st, err := c.CheckNow(context.Background())
	if err == nil || st.Available || st.Error == "" {
		t.Fatalf("sem rede deveria registrar o erro sem avisar: %+v %v", st, err)
	}
}
