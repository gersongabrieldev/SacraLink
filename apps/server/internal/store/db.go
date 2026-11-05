// Package store implementa a persistência do SacraLink em SQLite (driver Go
// puro, sem CGO), com migrations versionadas e embutidas no binário.
package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite" // registra o driver "sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// ErrNotFound indica que o registro não existe.
var ErrNotFound = errors.New("registro não encontrado")

// Store encapsula a conexão com o banco.
type Store struct {
	db  *sql.DB
	now func() time.Time
}

// Open abre (ou cria) o banco em path e aplica as migrations pendentes.
// Use ":memory:" para testes.
func Open(ctx context.Context, path string) (*Store, error) {
	var dsn string
	if path == ":memory:" {
		dsn = "file::memory:?_pragma=foreign_keys(1)"
	} else {
		abs, err := filepath.Abs(path)
		if err != nil {
			return nil, err
		}
		q := url.Values{}
		q.Add("_pragma", "busy_timeout(5000)")
		q.Add("_pragma", "journal_mode(WAL)")
		q.Add("_pragma", "synchronous(NORMAL)")
		q.Add("_pragma", "foreign_keys(1)")
		dsn = "file:" + filepath.ToSlash(abs) + "?" + q.Encode()
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("abrir banco: %w", err)
	}
	// Uma conexão serializa as escritas e evita SQLITE_BUSY; a carga do
	// SacraLink (dezenas de operações por segundo) cabe folgadamente.
	db.SetMaxOpenConns(1)
	s := &Store{db: db, now: time.Now}
	if err := s.migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close fecha o banco.
func (s *Store) Close() error { return s.db.Close() }

// DB expõe a conexão para casos avançados (diagnóstico).
func (s *Store) DB() *sql.DB { return s.db }

func (s *Store) nowMs() int64 { return s.now().UnixMilli() }

// SetClock substitui o relógio (apenas para testes).
func (s *Store) SetClock(f func() time.Time) { s.now = f }

type migration struct {
	version int
	name    string
	sql     string
}

func loadMigrations() ([]migration, error) {
	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return nil, err
	}
	var out []migration
	for _, e := range entries {
		name := e.Name()
		prefix, _, ok := strings.Cut(name, "_")
		if !ok {
			return nil, fmt.Errorf("nome de migration inválido: %s", name)
		}
		v, err := strconv.Atoi(prefix)
		if err != nil {
			return nil, fmt.Errorf("versão de migration inválida: %s", name)
		}
		b, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return nil, err
		}
		out = append(out, migration{version: v, name: name, sql: string(b)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].version < out[j].version })
	return out, nil
}

func (s *Store) migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY, name TEXT NOT NULL, applied_at INTEGER NOT NULL)`); err != nil {
		return fmt.Errorf("criar schema_migrations: %w", err)
	}
	migs, err := loadMigrations()
	if err != nil {
		return err
	}
	for _, m := range migs {
		var exists int
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM schema_migrations WHERE version = ?`, m.version).Scan(&exists); err != nil {
			return err
		}
		if exists > 0 {
			continue
		}
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, m.sql); err != nil {
			tx.Rollback()
			return fmt.Errorf("aplicar migration %s: %w", m.name, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, ?, ?)`,
			m.version, m.name, s.nowMs()); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// SchemaVersion retorna a maior versão de migration aplicada.
func (s *Store) SchemaVersion(ctx context.Context) (int, error) {
	var v sql.NullInt64
	err := s.db.QueryRowContext(ctx, `SELECT MAX(version) FROM schema_migrations`).Scan(&v)
	return int(v.Int64), err
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
