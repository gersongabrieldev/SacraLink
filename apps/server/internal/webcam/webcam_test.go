package webcam

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPaginaComCSP(t *testing.T) {
	h := Handler()
	for _, tc := range []struct{ host, want string }{
		{"192.168.0.10:7443", "connect-src 'self' wss://192.168.0.10:7443;"},
		{"[fe80::1]:7443", "connect-src 'self' wss://[fe80::1]:7443;"},
		{"igreja.ddns.net:8443", "connect-src 'self' wss://igreja.ddns.net:8443;"},
		// Host malicioso não injeta diretivas na política.
		{"x; script-src *", "connect-src 'self';"},
	} {
		r := httptest.NewRequest(http.MethodGet, "/cam/", nil)
		r.Host = tc.host
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		csp := w.Header().Get("Content-Security-Policy")
		if w.Code != 200 || !strings.Contains(csp, tc.want) || strings.Contains(csp, "script-src *") {
			t.Fatalf("host %q: %d %q", tc.host, w.Code, csp)
		}
		if !strings.Contains(w.Body.String(), `src="app.js"`) {
			t.Fatal("index.html sem o script da câmera")
		}
	}
	for _, p := range []string{"/cam/app.js", "/cam/style.css", "/cam/manifest.webmanifest", "/cam/logo.svg"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, p, nil))
		if w.Code != 200 || w.Body.Len() == 0 {
			t.Fatalf("%s: %d", p, w.Code)
		}
	}
}
