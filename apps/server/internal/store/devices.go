package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// Estados persistidos de autorização de um dispositivo.
const (
	DeviceActive  = "active"
	DeviceRevoked = "revoked"
	DeviceBlocked = "blocked"
)

// Permissões de dispositivo.
const (
	PermPublishVideo = "publish.video"
	PermPublishAudio = "publish.audio"
	PermRemoteCtrl   = "remote.control"
)

// DefaultDevicePermissions são concedidas no pareamento.
var DefaultDevicePermissions = []string{PermPublishVideo, PermPublishAudio, PermRemoteCtrl}

// Device é um smartphone pareado.
type Device struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Platform    string   `json:"platform"`
	Model       string   `json:"model"`
	OSVersion   string   `json:"osVersion"`
	AppVersion  string   `json:"appVersion"`
	SecretHash  string   `json:"-"`
	Status      string   `json:"status"`
	PairedAt    int64    `json:"pairedAt"`
	LastSeenAt  int64    `json:"lastSeenAt"`
	LastIP      string   `json:"lastIp"`
	Permissions []string `json:"permissions"`
	CameraID    string   `json:"cameraId"`
}

const deviceCols = `d.id, d.name, d.platform, d.model, d.os_version, d.app_version, d.secret_hash, d.status,
	d.paired_at, d.last_seen_at, d.last_ip, COALESCE(c.id, ''),
	COALESCE((SELECT group_concat(permission, ',') FROM device_permissions p WHERE p.device_id = d.id), '')`

func scanDevice(sc interface{ Scan(...any) error }) (*Device, error) {
	d := &Device{}
	var perms string
	if err := sc.Scan(&d.ID, &d.Name, &d.Platform, &d.Model, &d.OSVersion, &d.AppVersion, &d.SecretHash,
		&d.Status, &d.PairedAt, &d.LastSeenAt, &d.LastIP, &d.CameraID, &perms); err != nil {
		return nil, err
	}
	d.Permissions = []string{}
	if perms != "" {
		d.Permissions = strings.Split(perms, ",")
	}
	return d, nil
}

// GetDevice busca um dispositivo pelo ID.
func (s *Store) GetDevice(ctx context.Context, id string) (*Device, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+deviceCols+` FROM devices d LEFT JOIN cameras c ON c.device_id = d.id WHERE d.id = ?`, id)
	d, err := scanDevice(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return d, err
}

// ListDevices lista todos os dispositivos pareados.
func (s *Store) ListDevices(ctx context.Context) ([]*Device, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+deviceCols+` FROM devices d LEFT JOIN cameras c ON c.device_id = d.id ORDER BY d.paired_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Device
	for rows.Next() {
		d, err := scanDevice(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// SetDeviceStatus altera o estado de autorização (active/revoked/blocked).
func (s *Store) SetDeviceStatus(ctx context.Context, id, status string) error {
	switch status {
	case DeviceActive, DeviceRevoked, DeviceBlocked:
	default:
		return fmt.Errorf("status inválido: %q", status)
	}
	return s.execOne(ctx, `UPDATE devices SET status = ? WHERE id = ?`, status, id)
}

// RenameDevice altera o nome exibido do dispositivo.
func (s *Store) RenameDevice(ctx context.Context, id, name string) error {
	return s.execOne(ctx, `UPDATE devices SET name = ? WHERE id = ?`, name, id)
}

// TouchDevice registra o último contato do dispositivo.
func (s *Store) TouchDevice(ctx context.Context, id, ip, appVersion, osVersion string) error {
	return s.execOne(ctx, `UPDATE devices SET last_seen_at = ?, last_ip = ?,
		app_version = CASE WHEN ? <> '' THEN ? ELSE app_version END,
		os_version = CASE WHEN ? <> '' THEN ? ELSE os_version END WHERE id = ?`,
		s.nowMs(), ip, appVersion, appVersion, osVersion, osVersion, id)
}

// DeleteDevice remove o dispositivo. A câmera associada permanece (identidade
// persistente) e fica livre para receber outro aparelho.
func (s *Store) DeleteDevice(ctx context.Context, id string) error {
	return s.execOne(ctx, `DELETE FROM devices WHERE id = ?`, id)
}

func (s *Store) execOne(ctx context.Context, q string, args ...any) error {
	res, err := s.db.ExecContext(ctx, q, args...)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
