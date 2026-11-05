package store

import (
	"context"
	"database/sql"
	"errors"
)

// OBSConnection guarda a configuração de conexão com o obs-websocket.
// PasswordEnc é a senha cifrada (DPAPI no Windows); nunca em texto puro.
type OBSConnection struct {
	Host           string
	Port           int
	PasswordEnc    []byte
	AutoConnect    bool
	LastOBSVersion string
	LastWSVersion  string
	UpdatedAt      int64
}

// GetOBSConnection retorna a configuração do OBS ou os padrões.
func (s *Store) GetOBSConnection(ctx context.Context) (*OBSConnection, error) {
	c := &OBSConnection{}
	var auto int
	err := s.db.QueryRowContext(ctx, `SELECT host, port, password_enc, auto_connect, last_obs_version, last_ws_version, updated_at
		FROM obs_connections WHERE id = 1`).Scan(&c.Host, &c.Port, &c.PasswordEnc, &auto, &c.LastOBSVersion, &c.LastWSVersion, &c.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return &OBSConnection{Host: "127.0.0.1", Port: 4455, AutoConnect: true}, nil
	}
	if err != nil {
		return nil, err
	}
	c.AutoConnect = auto == 1
	return c, nil
}

// SaveOBSConnection grava a configuração do OBS.
func (s *Store) SaveOBSConnection(ctx context.Context, c *OBSConnection) error {
	c.UpdatedAt = s.nowMs()
	_, err := s.db.ExecContext(ctx, `INSERT INTO obs_connections (id, host, port, password_enc, auto_connect, last_obs_version, last_ws_version, updated_at)
		VALUES (1, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET host = excluded.host, port = excluded.port, password_enc = excluded.password_enc,
		auto_connect = excluded.auto_connect, last_obs_version = excluded.last_obs_version,
		last_ws_version = excluded.last_ws_version, updated_at = excluded.updated_at`,
		c.Host, c.Port, c.PasswordEnc, boolToInt(c.AutoConnect), c.LastOBSVersion, c.LastWSVersion, c.UpdatedAt)
	return err
}

// SourceMapping associa uma câmera à fonte e à cena que o SacraLink criou no OBS.
type SourceMapping struct {
	CameraID  string `json:"cameraId"`
	InputName string `json:"inputName"`
	InputUUID string `json:"inputUuid"`
	InputKind string `json:"inputKind"`
	SceneName string `json:"sceneName"`
	SceneUUID string `json:"sceneUuid"`
	UpdatedAt int64  `json:"updatedAt"`
}

// ListSourceMappings lista os mapeamentos persistidos.
func (s *Store) ListSourceMappings(ctx context.Context) ([]*SourceMapping, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT camera_id, input_name, input_uuid, input_kind, scene_name, scene_uuid, updated_at
		FROM obs_source_mappings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*SourceMapping
	for rows.Next() {
		m := &SourceMapping{}
		if err := rows.Scan(&m.CameraID, &m.InputName, &m.InputUUID, &m.InputKind, &m.SceneName, &m.SceneUUID, &m.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// GetSourceMapping busca o mapeamento de uma câmera.
func (s *Store) GetSourceMapping(ctx context.Context, cameraID string) (*SourceMapping, error) {
	m := &SourceMapping{}
	err := s.db.QueryRowContext(ctx, `SELECT camera_id, input_name, input_uuid, input_kind, scene_name, scene_uuid, updated_at
		FROM obs_source_mappings WHERE camera_id = ?`, cameraID).Scan(&m.CameraID, &m.InputName, &m.InputUUID, &m.InputKind,
		&m.SceneName, &m.SceneUUID, &m.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return m, err
}

// UpsertSourceMapping grava o mapeamento de uma câmera.
func (s *Store) UpsertSourceMapping(ctx context.Context, m *SourceMapping) error {
	m.UpdatedAt = s.nowMs()
	_, err := s.db.ExecContext(ctx, `INSERT INTO obs_source_mappings (camera_id, input_name, input_uuid, input_kind, scene_name, scene_uuid, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(camera_id) DO UPDATE SET input_name = excluded.input_name, input_uuid = excluded.input_uuid,
		input_kind = excluded.input_kind, scene_name = excluded.scene_name, scene_uuid = excluded.scene_uuid,
		updated_at = excluded.updated_at`,
		m.CameraID, m.InputName, m.InputUUID, m.InputKind, m.SceneName, m.SceneUUID, m.UpdatedAt)
	return err
}

// DeleteSourceMapping remove o mapeamento (a fonte no OBS não é apagada).
func (s *Store) DeleteSourceMapping(ctx context.Context, cameraID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM obs_source_mappings WHERE camera_id = ?`, cameraID)
	return err
}
