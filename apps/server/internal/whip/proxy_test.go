package whip

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func fakeMediaMTX(t *testing.T) (*httptest.Server, *[]string) {
	var log []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		log = append(log, r.Method+" "+r.URL.Path)
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/sl/cam1/whip":
			if !ok || user != "pub" || pass != "segredo" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			b, _ := io.ReadAll(r.Body)
			if r.Header.Get("Content-Type") != "application/sdp" || string(b[:3]) != "v=0" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/sdp")
			w.Header().Set("Location", "/sl/cam1/whip/abc-123")
			w.Header().Set("ETag", `"x1"`)
			w.Header().Add("Link", `<stun:stun.example>; rel="ice-server"`)
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte("v=0\r\nresposta"))
		case r.Method == http.MethodPatch && r.URL.Path == "/sl/cam1/whip/abc-123":
			if r.Header.Get("If-Match") != `"x1"` {
				w.WriteHeader(http.StatusPreconditionFailed)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodDelete && r.URL.Path == "/sl/cam1/whip/abc-123":
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && r.URL.Path == "/sl/cam1/whep":
			if ok {
				t.Error("leitura não deve enviar credencial de publicação")
			}
			w.Header().Set("Location", "/sl/cam1/whep/def-456")
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte("v=0\r\n"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &log
}

func TestCicloWHIP(t *testing.T) {
	srv, _ := fakeMediaMTX(t)
	p := New(srv.URL, "pub", "segredo")
	ctx := context.Background()
	r, err := p.Offer(ctx, Publish, "cam1", "sl/cam1", []byte("v=0\r\noffer"), "192.168.0.50", "/device/v1/whip/sessions/")
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != http.StatusCreated || r.Header.Get("Location") != "/device/v1/whip/sessions/abc-123" ||
		r.Header.Get("ETag") != `"x1"` || r.Header.Get("Link") == "" || r.Session != "abc-123" {
		t.Fatalf("resposta inesperada: %+v", r)
	}
	// Outra câmera não pode manipular a sessão.
	if _, err := p.Patch(ctx, Publish, "abc-123", "cam2", []byte("a=x"), "application/trickle-ice-sdpfrag", `"x1"`); !errors.Is(err, ErrUnknownSession) {
		t.Fatalf("esperava ErrUnknownSession, obtive %v", err)
	}
	pr, err := p.Patch(ctx, Publish, "abc-123", "cam1", []byte("a=x"), "application/trickle-ice-sdpfrag", `"x1"`)
	if err != nil || pr.Status != http.StatusNoContent {
		t.Fatalf("patch: %+v %v", pr, err)
	}
	if st, err := p.Delete(ctx, Publish, "abc-123", "cam1"); err != nil || st != http.StatusOK {
		t.Fatalf("delete: %d %v", st, err)
	}
	if _, err := p.Delete(ctx, Publish, "abc-123", "cam1"); !errors.Is(err, ErrUnknownSession) {
		t.Fatal("sessão deveria ter sido removida")
	}
}

func TestSDPInvalidoRejeitado(t *testing.T) {
	srv, log := fakeMediaMTX(t)
	p := New(srv.URL, "pub", "segredo")
	for _, sdp := range [][]byte{nil, []byte("GET / HTTP/1.1"), make([]byte, MaxSDPBytes+1)} {
		if _, err := p.Offer(context.Background(), Publish, "cam1", "sl/cam1", sdp, "", "/x/"); err == nil {
			t.Fatal("SDP inválido deveria ser rejeitado")
		}
	}
	if len(*log) != 0 {
		t.Fatal("SDP inválido não pode chegar ao MediaMTX")
	}
}

func TestWHEPSemCredencial(t *testing.T) {
	srv, _ := fakeMediaMTX(t)
	p := New(srv.URL, "pub", "segredo")
	r, err := p.Offer(context.Background(), Read, "cam1", "sl/cam1", []byte("v=0\r\n"), "", "/whep/cam1/sessions/")
	if err != nil || r.Header.Get("Location") != "/whep/cam1/sessions/def-456" {
		t.Fatalf("whep: %+v %v", r, err)
	}
}
