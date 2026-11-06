package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/gersongabrieldev/sacralink/apps/server/internal/secure"
)

func openTest(t *testing.T) *Store {
	t.Helper()
	s, err := Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestMigrationsIdempotentes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "m.db")
	ctx := context.Background()
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	v1, _ := s.SchemaVersion(ctx)
	s.Close()
	s2, err := Open(ctx, path) // reabrir não pode reaplicar migrations
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	v2, _ := s2.SchemaVersion(ctx)
	if v1 != v2 || v1 < 1 {
		t.Fatalf("versões inesperadas: %d %d", v1, v2)
	}
	ps, err := s2.ListProfiles(ctx)
	if err != nil || len(ps) != 5 {
		t.Fatalf("perfis padrão: %v %d", err, len(ps))
	}
}

func TestSettings(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	var v struct{ A int }
	ok, err := s.GetSetting(ctx, "x", &v)
	if ok || err != nil {
		t.Fatalf("esperava ausente: %v %v", ok, err)
	}
	if err := s.SetSetting(ctx, "x", map[string]int{"A": 7}); err != nil {
		t.Fatal(err)
	}
	ok, err = s.GetSetting(ctx, "x", &v)
	if !ok || err != nil || v.A != 7 {
		t.Fatalf("leitura: %v %v %+v", ok, err, v)
	}
}

func pair(t *testing.T, s *Store, cameraID string) (*Device, *Camera, error) {
	t.Helper()
	ctx := context.Background()
	tok := secure.NewToken()
	if _, err := s.CreatePairingToken(ctx, secure.HashSecret(tok), "teste", cameraID, 60_000); err != nil {
		t.Fatal(err)
	}
	return s.PairDevice(ctx, secure.HashSecret(tok), NewDeviceInput{
		Name: "Celular", Platform: "android", SecretHash: secure.HashSecret("segredo"),
	})
}

func TestPareamentoCriaCamerasComSlotsSequenciais(t *testing.T) {
	s := openTest(t)
	for i := 1; i <= 3; i++ {
		d, c, err := pair(t, s, "")
		if err != nil {
			t.Fatal(err)
		}
		if c.Slot != i || c.DeviceID != d.ID || d.CameraID != c.ID {
			t.Fatalf("slot/vínculo inesperado: %+v %+v", c, d)
		}
		if len(d.Permissions) != len(DefaultDevicePermissions) {
			t.Fatalf("permissões: %v", d.Permissions)
		}
	}
}

func TestTokenDeUsoUnicoEExpiracao(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	tok := secure.HashSecret("tok")
	if _, err := s.CreatePairingToken(ctx, tok, "", "", 60_000); err != nil {
		t.Fatal(err)
	}
	in := NewDeviceInput{Name: "A", Platform: "ios", SecretHash: "h"}
	if _, _, err := s.PairDevice(ctx, tok, in); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.PairDevice(ctx, tok, in); !errors.Is(err, ErrPairingInvalid) {
		t.Fatalf("reuso do token deveria falhar: %v", err)
	}

	exp := secure.HashSecret("expira")
	if _, err := s.CreatePairingToken(ctx, exp, "", "", 1000); err != nil {
		t.Fatal(err)
	}
	s.SetClock(func() time.Time { return time.Now().Add(2 * time.Second) })
	if _, _, err := s.PairDevice(ctx, exp, in); !errors.Is(err, ErrPairingInvalid) {
		t.Fatalf("token expirado deveria falhar: %v", err)
	}
}

func TestReatribuicaoDeCamera(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	d1, c1, err := pair(t, s, "")
	if err != nil {
		t.Fatal(err)
	}
	// Com o aparelho original ativo, a câmera não pode ser tomada.
	if _, _, err := pair(t, s, c1.ID); !errors.Is(err, ErrCameraTaken) {
		t.Fatalf("esperava ErrCameraTaken, obtive %v", err)
	}
	if err := s.SetDeviceStatus(ctx, d1.ID, DeviceRevoked); err != nil {
		t.Fatal(err)
	}
	d2, c2, err := pair(t, s, c1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if c2.ID != c1.ID || c2.Slot != c1.Slot || c2.DeviceID != d2.ID {
		t.Fatalf("identidade não preservada: %+v", c2)
	}
	old, _ := s.GetDevice(ctx, d1.ID)
	if old.CameraID != "" {
		t.Fatalf("aparelho antigo ainda vinculado: %+v", old)
	}
}

func TestRemoverDispositivoPreservaCamera(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	d, c, err := pair(t, s, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteDevice(ctx, d.ID); err != nil {
		t.Fatal(err)
	}
	c2, err := s.GetCamera(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if c2.DeviceID != "" {
		t.Fatalf("câmera deveria ficar livre: %+v", c2)
	}
	// Novo pareamento sem câmera escolhida reutiliza o menor slot livre? Não:
	// o slot 1 continua ocupado pela câmera existente, então vai para o 2.
	_, c3, err := pair(t, s, "")
	if err != nil {
		t.Fatal(err)
	}
	if c3.Slot != 2 {
		t.Fatalf("slot esperado 2, obtido %d", c3.Slot)
	}
}

func TestRetencao(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	s.SetClock(func() time.Time { return time.Now().Add(-40 * 24 * time.Hour) })
	if _, err := s.AddEvent(ctx, "info", "teste", "", "antigo", nil); err != nil {
		t.Fatal(err)
	}
	s.SetClock(time.Now)
	if _, err := s.AddEvent(ctx, "info", "teste", "", "novo", nil); err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyRetention(ctx, DefaultRetention); err != nil {
		t.Fatal(err)
	}
	evs, _ := s.ListEvents(ctx, 10)
	if len(evs) != 1 || evs[0].Message != "novo" {
		t.Fatalf("retenção não aplicada: %+v", evs)
	}
}

func TestBloqueioDeLogin(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	u, err := s.CreateUser(ctx, "admin", "hash", "admin")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if err := s.RecordLoginFailure(ctx, u.ID, 5, 60_000); err != nil {
			t.Fatal(err)
		}
	}
	u2, _ := s.GetUserByName(ctx, "admin")
	if u2.LockedUntil == 0 || u2.FailedAttempts != 5 {
		t.Fatalf("bloqueio não aplicado: %+v", u2)
	}
	_ = s.RecordLoginSuccess(ctx, u.ID)
	u3, _ := s.GetUserByName(ctx, "admin")
	if u3.LockedUntil != 0 || u3.FailedAttempts != 0 {
		t.Fatalf("desbloqueio não aplicado: %+v", u3)
	}
}

func TestPresets(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	_, c, err := pair(t, s, "")
	if err != nil {
		t.Fatal(err)
	}
	zoom, lens := 2.5, "0"
	p, err := s.CreatePreset(ctx, c.ID, "Altar fechado", PresetParams{Zoom: &zoom, LensID: &lens})
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.GetPreset(ctx, c.ID, p.ID)
	if err != nil || *got.Params.Zoom != 2.5 || got.Params.ProfileID != nil {
		t.Fatalf("preset lido: %+v %v", got, err)
	}
	// Outra câmera não acessa o preset.
	_, c2, _ := pair(t, s, "")
	if _, err := s.GetPreset(ctx, c2.ID, p.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("isolamento entre câmeras: %v", err)
	}
	for i := 1; i < MaxPresetsPerCamera; i++ {
		if _, err := s.CreatePreset(ctx, c.ID, "x", PresetParams{}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.CreatePreset(ctx, c.ID, "excesso", PresetParams{}); !errors.Is(err, ErrTooManyPresets) {
		t.Fatalf("limite: %v", err)
	}
	if err := s.DeletePreset(ctx, c.ID, p.ID); err != nil {
		t.Fatal(err)
	}
	list, _ := s.ListPresets(ctx, c.ID)
	if len(list) != MaxPresetsPerCamera-1 {
		t.Fatalf("lista: %d", len(list))
	}
}
