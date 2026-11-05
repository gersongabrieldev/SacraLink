package store

import (
	"context"
	"database/sql"
	"errors"
)

// User é um usuário administrador do painel.
type User struct {
	ID             int64
	Username       string
	PasswordHash   string
	Role           string
	FailedAttempts int
	LockedUntil    int64
	CreatedAt      int64
	UpdatedAt      int64
}

// CountUsers retorna a quantidade de usuários cadastrados.
func (s *Store) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM users`).Scan(&n)
	return n, err
}

// CreateUser cria um usuário.
func (s *Store) CreateUser(ctx context.Context, username, passwordHash, role string) (*User, error) {
	now := s.nowMs()
	res, err := s.db.ExecContext(ctx, `INSERT INTO users (username, password_hash, role, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)`, username, passwordHash, role, now, now)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return &User{ID: id, Username: username, PasswordHash: passwordHash, Role: role, CreatedAt: now, UpdatedAt: now}, nil
}

// GetUserByName busca um usuário pelo nome.
func (s *Store) GetUserByName(ctx context.Context, username string) (*User, error) {
	u := &User{}
	err := s.db.QueryRowContext(ctx, `SELECT id, username, password_hash, role, failed_attempts, locked_until, created_at, updated_at
		FROM users WHERE username = ?`, username).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role,
		&u.FailedAttempts, &u.LockedUntil, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return u, err
}

// RecordLoginFailure incrementa as falhas e aplica bloqueio temporário
// quando o limite é atingido.
func (s *Store) RecordLoginFailure(ctx context.Context, id int64, maxAttempts int, lockMs int64) error {
	now := s.nowMs()
	_, err := s.db.ExecContext(ctx, `UPDATE users SET
		failed_attempts = failed_attempts + 1,
		locked_until = CASE WHEN failed_attempts + 1 >= ? THEN ? ELSE locked_until END,
		updated_at = ? WHERE id = ?`, maxAttempts, now+lockMs, now, id)
	return err
}

// RecordLoginSuccess zera o contador de falhas.
func (s *Store) RecordLoginSuccess(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE users SET failed_attempts = 0, locked_until = 0, updated_at = ? WHERE id = ?`,
		s.nowMs(), id)
	return err
}

// UpdatePassword troca o hash de senha do usuário.
func (s *Store) UpdatePassword(ctx context.Context, id int64, hash string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE users SET password_hash = ?, failed_attempts = 0, locked_until = 0, updated_at = ? WHERE id = ?`,
		hash, s.nowMs(), id)
	return err
}

// DeleteAllUsers remove todos os administradores (recuperação de senha pelo
// comando local "sacralink-server admin reset"). O próximo acesso ao painel
// volta ao assistente de criação de senha.
func (s *Store) DeleteAllUsers(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM users`)
	return err
}
