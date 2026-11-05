package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Modos de ingestão no OBS (ADR-0004).
const (
	IngestRTSP = "rtsp"
	IngestWHEP = "whep"
)

// Camera é a identidade persistente de uma câmera do SacraLink. Ela sobrevive
// a reconexões e à troca do aparelho.
type Camera struct {
	ID                string `json:"id"`
	Slot              int    `json:"slot"`
	Name              string `json:"name"`
	OperatorName      string `json:"operatorName"`
	DeviceID          string `json:"deviceId"`
	ProfileID         string `json:"profileId"`
	IngestMode        string `json:"ingestMode"`
	IsBackup          bool   `json:"isBackup"`
	AudioSyncOffsetMs int    `json:"audioSyncOffsetMs"`
	CreatedAt         int64  `json:"createdAt"`
	UpdatedAt         int64  `json:"updatedAt"`
}

// MediaPath retorna o path da câmera no MediaMTX.
func (c *Camera) MediaPath() string { return "sl/" + c.ID }

const cameraCols = `id, slot, name, operator_name, COALESCE(device_id, ''), profile_id, ingest_mode, is_backup,
	audio_sync_offset_ms, created_at, updated_at`

func scanCamera(sc interface{ Scan(...any) error }) (*Camera, error) {
	c := &Camera{}
	var backup int
	if err := sc.Scan(&c.ID, &c.Slot, &c.Name, &c.OperatorName, &c.DeviceID, &c.ProfileID, &c.IngestMode,
		&backup, &c.AudioSyncOffsetMs, &c.CreatedAt, &c.UpdatedAt); err != nil {
		return nil, err
	}
	c.IsBackup = backup == 1
	return c, nil
}

// GetCamera busca uma câmera pelo ID.
func (s *Store) GetCamera(ctx context.Context, id string) (*Camera, error) {
	c, err := scanCamera(s.db.QueryRowContext(ctx, `SELECT `+cameraCols+` FROM cameras WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return c, err
}

// GetCameraByDevice busca a câmera associada a um dispositivo.
func (s *Store) GetCameraByDevice(ctx context.Context, deviceID string) (*Camera, error) {
	c, err := scanCamera(s.db.QueryRowContext(ctx, `SELECT `+cameraCols+` FROM cameras WHERE device_id = ?`, deviceID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return c, err
}

// ListCameras lista as câmeras por slot.
func (s *Store) ListCameras(ctx context.Context) ([]*Camera, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+cameraCols+` FROM cameras ORDER BY slot`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Camera
	for rows.Next() {
		c, err := scanCamera(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// CameraPatch descreve alterações parciais em uma câmera (nil = manter).
type CameraPatch struct {
	Name              *string `json:"name,omitempty"`
	OperatorName      *string `json:"operatorName,omitempty"`
	ProfileID         *string `json:"profileId,omitempty"`
	IngestMode        *string `json:"ingestMode,omitempty"`
	IsBackup          *bool   `json:"isBackup,omitempty"`
	AudioSyncOffsetMs *int    `json:"audioSyncOffsetMs,omitempty"`
}

// UpdateCamera aplica um patch na câmera.
func (s *Store) UpdateCamera(ctx context.Context, id string, p CameraPatch) (*Camera, error) {
	c, err := s.GetCamera(ctx, id)
	if err != nil {
		return nil, err
	}
	if p.Name != nil {
		c.Name = *p.Name
	}
	if p.OperatorName != nil {
		c.OperatorName = *p.OperatorName
	}
	if p.ProfileID != nil {
		c.ProfileID = *p.ProfileID
	}
	if p.IngestMode != nil {
		if *p.IngestMode != IngestRTSP && *p.IngestMode != IngestWHEP {
			return nil, fmt.Errorf("modo de ingestão inválido: %q", *p.IngestMode)
		}
		c.IngestMode = *p.IngestMode
	}
	if p.IsBackup != nil {
		c.IsBackup = *p.IsBackup
	}
	if p.AudioSyncOffsetMs != nil {
		c.AudioSyncOffsetMs = *p.AudioSyncOffsetMs
	}
	c.UpdatedAt = s.nowMs()
	_, err = s.db.ExecContext(ctx, `UPDATE cameras SET name = ?, operator_name = ?, profile_id = ?, ingest_mode = ?,
		is_backup = ?, audio_sync_offset_ms = ?, updated_at = ? WHERE id = ?`,
		c.Name, c.OperatorName, c.ProfileID, c.IngestMode, boolToInt(c.IsBackup), c.AudioSyncOffsetMs, c.UpdatedAt, id)
	if err != nil {
		return nil, err
	}
	return c, nil
}

// MediaProfile é um perfil de qualidade de transmissão.
type MediaProfile struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Width          int    `json:"width"`
	Height         int    `json:"height"`
	FPS            int    `json:"fps"`
	MaxBitrateKbps int    `json:"maxBitrateKbps"`
	MinBitrateKbps int    `json:"minBitrateKbps"`
	Adaptive       bool   `json:"adaptive"`
	Builtin        bool   `json:"builtin"`
}

// ListProfiles lista os perfis de mídia.
func (s *Store) ListProfiles(ctx context.Context) ([]*MediaProfile, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, width, height, fps, max_bitrate_kbps, min_bitrate_kbps, adaptive, builtin
		FROM media_profiles ORDER BY builtin DESC, height, fps`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*MediaProfile
	for rows.Next() {
		p := &MediaProfile{}
		var adaptive, builtin int
		if err := rows.Scan(&p.ID, &p.Name, &p.Width, &p.Height, &p.FPS, &p.MaxBitrateKbps, &p.MinBitrateKbps, &adaptive, &builtin); err != nil {
			return nil, err
		}
		p.Adaptive, p.Builtin = adaptive == 1, builtin == 1
		out = append(out, p)
	}
	return out, rows.Err()
}

// GetProfile busca um perfil de mídia.
func (s *Store) GetProfile(ctx context.Context, id string) (*MediaProfile, error) {
	ps, err := s.ListProfiles(ctx)
	if err != nil {
		return nil, err
	}
	for _, p := range ps {
		if p.ID == id {
			return p, nil
		}
	}
	return nil, ErrNotFound
}
