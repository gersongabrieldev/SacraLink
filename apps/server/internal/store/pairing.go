package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/gersongabrieldev/sacralink/apps/server/internal/secure"
)

// Erros do pareamento.
var (
	ErrPairingInvalid = errors.New("token de pareamento inválido, expirado ou já utilizado")
	ErrCameraTaken    = errors.New("a câmera escolhida já está associada a outro aparelho ativo")
)

// CreatePairingToken persiste o hash de um token de pareamento de uso único.
// cameraID vazio significa "criar uma nova câmera".
func (s *Store) CreatePairingToken(ctx context.Context, tokenHash, label, cameraID string, ttlMs int64) (expiresAt int64, err error) {
	now := s.nowMs()
	expiresAt = now + ttlMs
	_, err = s.db.ExecContext(ctx, `INSERT INTO pairing_tokens (token_hash, label, camera_id, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?)`, tokenHash, label, cameraID, now, expiresAt)
	return expiresAt, err
}

// NewDeviceInput são os dados enviados pelo aparelho no pareamento.
type NewDeviceInput struct {
	Name       string
	Platform   string
	Model      string
	OSVersion  string
	AppVersion string
	SecretHash string
	IP         string
}

// PairDevice consome o token (uso único) e cria o dispositivo e sua câmera
// de forma atômica. Se o token apontar para uma câmera existente, o novo
// aparelho assume essa identidade (o anterior é desvinculado).
func (s *Store) PairDevice(ctx context.Context, tokenHash string, in NewDeviceInput) (*Device, *Camera, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback()

	now := s.nowMs()
	var cameraID string
	err = tx.QueryRowContext(ctx, `SELECT camera_id FROM pairing_tokens
		WHERE token_hash = ? AND used_at = 0 AND expires_at > ?`, tokenHash, now).Scan(&cameraID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, ErrPairingInvalid
	}
	if err != nil {
		return nil, nil, err
	}

	deviceID := secure.NewID("d", 15)
	if _, err := tx.ExecContext(ctx, `INSERT INTO devices (id, name, platform, model, os_version, app_version, secret_hash,
		status, paired_at, last_seen_at, last_ip) VALUES (?, ?, ?, ?, ?, ?, ?, 'active', ?, ?, ?)`,
		deviceID, in.Name, in.Platform, in.Model, in.OSVersion, in.AppVersion, in.SecretHash, now, now, in.IP); err != nil {
		return nil, nil, fmt.Errorf("criar dispositivo: %w", err)
	}
	for _, p := range DefaultDevicePermissions {
		if _, err := tx.ExecContext(ctx, `INSERT INTO device_permissions (device_id, permission) VALUES (?, ?)`, deviceID, p); err != nil {
			return nil, nil, err
		}
	}

	if cameraID != "" {
		// Reatribuição de identidade: só permitida se o aparelho atual da câmera
		// não estiver ativo (revogado/bloqueado/removido).
		var prevDevice, prevStatus sql.NullString
		err := tx.QueryRowContext(ctx, `SELECT c.device_id, d.status FROM cameras c LEFT JOIN devices d ON d.id = c.device_id
			WHERE c.id = ?`, cameraID).Scan(&prevDevice, &prevStatus)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, ErrPairingInvalid
		}
		if err != nil {
			return nil, nil, err
		}
		if prevStatus.Valid && prevStatus.String == DeviceActive {
			return nil, nil, ErrCameraTaken
		}
		if _, err := tx.ExecContext(ctx, `UPDATE cameras SET device_id = ?, updated_at = ? WHERE id = ?`, deviceID, now, cameraID); err != nil {
			return nil, nil, err
		}
	} else {
		var slot int
		// Menor slot livre começando em 1.
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MIN(s.slot + 1), 1) FROM (SELECT 0 AS slot UNION ALL SELECT slot FROM cameras) s
			WHERE NOT EXISTS (SELECT 1 FROM cameras c WHERE c.slot = s.slot + 1)`).Scan(&slot); err != nil {
			return nil, nil, err
		}
		cameraID = secure.NewID("c", 11)
		if _, err := tx.ExecContext(ctx, `INSERT INTO cameras (id, slot, name, device_id, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)`, cameraID, slot, fmt.Sprintf("Câmera %02d", slot), deviceID, now, now); err != nil {
			return nil, nil, fmt.Errorf("criar câmera: %w", err)
		}
	}

	res, err := tx.ExecContext(ctx, `UPDATE pairing_tokens SET used_at = ?, used_by_device = ? WHERE token_hash = ? AND used_at = 0`,
		now, deviceID, tokenHash)
	if err != nil {
		return nil, nil, err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return nil, nil, ErrPairingInvalid
	}
	if err := tx.Commit(); err != nil {
		return nil, nil, err
	}
	d, err := s.GetDevice(ctx, deviceID)
	if err != nil {
		return nil, nil, err
	}
	c, err := s.GetCamera(ctx, cameraID)
	if err != nil {
		return nil, nil, err
	}
	return d, c, nil
}

// PurgeExpiredPairingTokens remove tokens vencidos há mais de 1 dia.
func (s *Store) PurgeExpiredPairingTokens(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM pairing_tokens WHERE expires_at < ?`, s.nowMs()-86_400_000)
	return err
}
