package obs_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gersongabrieldev/sacralink/apps/server/internal/obs"
	"github.com/gersongabrieldev/sacralink/apps/server/internal/obs/obstest"
)

func ctx(t *testing.T) context.Context {
	c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return c
}

func TestAuthStringVetorConhecido(t *testing.T) {
	// Vetor calculado de forma independente para senha/salt/challenge fixos.
	got := obs.AuthString("supersecretpassword", "lM1GncleQOaCu9lT1yeUZhFYnqhsLLP1G5lAGo3ixaI=", "+IxH4CnCiqpX1rM9scsNynZzbOe4KhDeYcTNS3PDaeY=")
	if got != "1Ct943GAT+6YQUUX47Ia/ncufilbe6+oD6lY+5kaCu4=" {
		t.Fatalf("AuthString inesperado: %s", got)
	}
}

func TestHandshakeComSenha(t *testing.T) {
	srv := obstest.New("segredo")
	defer srv.Close()

	c, err := obs.Dial(ctx(t), obs.Options{URL: srv.URL(), Password: "segredo"})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	v, err := c.GetVersion(ctx(t))
	if err != nil {
		t.Fatal(err)
	}
	if v.OBSVersion != "32.0.1" || c.Hello().OBSWebSocketVersion != "5.6.3" {
		t.Fatalf("versões inesperadas: %+v %+v", v, c.Hello())
	}
}

func TestHandshakeSenhaErrada(t *testing.T) {
	srv := obstest.New("segredo")
	defer srv.Close()
	_, err := obs.Dial(ctx(t), obs.Options{URL: srv.URL(), Password: "errada"})
	if !errors.Is(err, obs.ErrAuthFailed) {
		t.Fatalf("esperava ErrAuthFailed, obtive %v", err)
	}
}

func TestHandshakeSemSenhaConfigurada(t *testing.T) {
	srv := obstest.New("segredo")
	defer srv.Close()
	_, err := obs.Dial(ctx(t), obs.Options{URL: srv.URL()})
	if !errors.Is(err, obs.ErrAuthRequired) {
		t.Fatalf("esperava ErrAuthRequired, obtive %v", err)
	}
}

func TestErroDeRequisicao(t *testing.T) {
	srv := obstest.New("")
	defer srv.Close()
	c, err := obs.Dial(ctx(t), obs.Options{URL: srv.URL()})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	err = c.SetCurrentPreviewScene(ctx(t), "Cena")
	if !obs.IsCode(err, obs.StatusStudioModeNotActive) {
		t.Fatalf("esperava StudioModeNotActive, obtive %v", err)
	}
}

func TestEventosEConexaoEncerrada(t *testing.T) {
	srv := obstest.New("")
	defer srv.Close()
	c, err := obs.Dial(ctx(t), obs.Options{URL: srv.URL()})
	if err != nil {
		t.Fatal(err)
	}
	srv.AddScene("Altar")
	srv.SetProgram("Altar")
	got := map[string]bool{}
	timeout := time.After(3 * time.Second)
	for !got["CurrentProgramSceneChanged"] {
		select {
		case ev := <-c.Events():
			got[ev.Type] = true
		case <-timeout:
			t.Fatalf("eventos não recebidos: %v", got)
		}
	}
	srv.DropConnections()
	select {
	case <-c.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("Done não sinalizado após queda do OBS")
	}
	if err := c.Call(ctx(t), "GetVersion", nil, nil); !errors.Is(err, obs.ErrClosed) {
		t.Fatalf("esperava ErrClosed, obtive %v", err)
	}
}

func TestStreamKeyNaoVaza(t *testing.T) {
	srv := obstest.New("")
	defer srv.Close()
	c, err := obs.Dial(ctx(t), obs.Options{URL: srv.URL()})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	info, err := c.GetStreamServiceInfo(ctx(t))
	if err != nil {
		t.Fatal(err)
	}
	if info.Service != "YouTube - RTMPS" {
		t.Fatalf("serviço inesperado: %+v", info)
	}
}
