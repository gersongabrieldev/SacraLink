package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

// GetSetting lê uma configuração JSON e a decodifica em out.
// Retorna (false, nil) se a chave não existir.
func (s *Store) GetSetting(ctx context.Context, key string, out any) (bool, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, json.Unmarshal([]byte(raw), out)
}

// SetSetting grava uma configuração codificada em JSON.
func (s *Store) SetSetting(ctx context.Context, key string, value any) error {
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO settings (key, value, updated_at) VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		key, string(b), s.nowMs())
	return err
}
