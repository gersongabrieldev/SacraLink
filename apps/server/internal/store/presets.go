package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/gersongabrieldev/sacralink/apps/server/internal/secure"
)

// MaxPresetsPerCamera limita os presets de cada câmera.
const MaxPresetsPerCamera = 20

// ErrTooManyPresets indica que a câmera atingiu o limite de presets.
var ErrTooManyPresets = errors.New("limite de presets da câmera atingido")

// PresetParams são os ajustes guardados em um preset. Campos nil não são
// alterados ao aplicar.
type PresetParams struct {
	LensID    *string  `json:"lensId,omitempty"`
	Zoom      *float64 `json:"zoom,omitempty"`
	ProfileID *string  `json:"profileId,omitempty"`
	Torch     *bool    `json:"torch,omitempty"`
	MicMuted  *bool    `json:"micMuted,omitempty"`
}

// Preset é um conjunto de ajustes nomeado de uma câmera.
type Preset struct {
	ID        string       `json:"id"`
	CameraID  string       `json:"cameraId"`
	Name      string       `json:"name"`
	Params    PresetParams `json:"params"`
	CreatedAt int64        `json:"createdAt"`
}

// ListPresets lista os presets de uma câmera.
func (s *Store) ListPresets(ctx context.Context, cameraID string) ([]*Preset, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, camera_id, name, params, created_at FROM camera_presets
		WHERE camera_id = ? ORDER BY created_at`, cameraID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Preset{}
	for rows.Next() {
		p, err := scanPreset(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func scanPreset(sc interface{ Scan(...any) error }) (*Preset, error) {
	p := &Preset{}
	var raw string
	if err := sc.Scan(&p.ID, &p.CameraID, &p.Name, &raw, &p.CreatedAt); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(raw), &p.Params); err != nil {
		return nil, err
	}
	return p, nil
}

// GetPreset busca um preset de uma câmera.
func (s *Store) GetPreset(ctx context.Context, cameraID, id string) (*Preset, error) {
	p, err := scanPreset(s.db.QueryRowContext(ctx, `SELECT id, camera_id, name, params, created_at FROM camera_presets
		WHERE id = ? AND camera_id = ?`, id, cameraID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return p, err
}

// CreatePreset grava um preset (respeitando o limite por câmera).
func (s *Store) CreatePreset(ctx context.Context, cameraID, name string, params PresetParams) (*Preset, error) {
	if _, err := s.GetCamera(ctx, cameraID); err != nil {
		return nil, err
	}
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM camera_presets WHERE camera_id = ?`, cameraID).Scan(&n); err != nil {
		return nil, err
	}
	if n >= MaxPresetsPerCamera {
		return nil, ErrTooManyPresets
	}
	raw, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	p := &Preset{ID: secure.NewID("p", 11), CameraID: cameraID, Name: name, Params: params, CreatedAt: s.nowMs()}
	_, err = s.db.ExecContext(ctx, `INSERT INTO camera_presets (id, camera_id, name, params, created_at) VALUES (?, ?, ?, ?, ?)`,
		p.ID, cameraID, name, string(raw), p.CreatedAt)
	return p, err
}

// DeletePreset remove um preset.
func (s *Store) DeletePreset(ctx context.Context, cameraID, id string) error {
	return s.execOne(ctx, `DELETE FROM camera_presets WHERE id = ? AND camera_id = ?`, id, cameraID)
}
