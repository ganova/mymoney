// Package store adalah lapisan data SQLite untuk MyMoney.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// ErrNotFound dikembalikan bila baris tidak ditemukan.
var ErrNotFound = errors.New("data tidak ditemukan")

// ValidationError adalah kesalahan input yang aman ditampilkan ke pengguna.
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

func invalid(format string, a ...any) error { return &ValidationError{Msg: fmt.Sprintf(format, a...)} }

// Store membungkus koneksi database.
type Store struct {
	DB  *sql.DB
	Loc *time.Location
	// Path file database (untuk fitur backup).
	Path string
}

// Open membuka (dan bila perlu membuat) database di dataDir.
func Open(dataDir string, loc *time.Location) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dataDir, "mymoney.db")
	dsn := "file:" + path + "?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// SQLite paling aman dengan satu penulis; baca tetap cepat lewat WAL.
	db.SetMaxOpenConns(1)
	s := &Store{DB: db, Loc: loc, Path: path}
	if err := s.migrate(context.Background()); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrasi: %w", err)
	}
	return s, nil
}

// OpenMemory membuka database in-memory (untuk tes).
func OpenMemory(loc *time.Location) (*Store, error) {
	db, err := sql.Open("sqlite", "file::memory:?_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{DB: db, Loc: loc}
	if err := s.migrate(context.Background()); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.DB.Close() }

// Today mengembalikan tanggal hari ini di zona waktu pengguna.
func (s *Store) Today() time.Time {
	n := time.Now().In(s.Loc)
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, time.UTC)
}

var migrations = []string{
	// v1: skema awal
	`
CREATE TABLE settings (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);

CREATE TABLE accounts (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,
  name            TEXT NOT NULL,
  type            TEXT NOT NULL DEFAULT 'bank',
  icon            TEXT NOT NULL DEFAULT '',
  color           TEXT NOT NULL DEFAULT '',
  opening_balance INTEGER NOT NULL DEFAULT 0,
  aliases         TEXT NOT NULL DEFAULT '',
  archived        INTEGER NOT NULL DEFAULT 0,
  sort            INTEGER NOT NULL DEFAULT 0,
  created_at      TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE categories (
  id       INTEGER PRIMARY KEY AUTOINCREMENT,
  name     TEXT NOT NULL,
  kind     TEXT NOT NULL DEFAULT 'expense',
  grp      TEXT NOT NULL DEFAULT '',
  icon     TEXT NOT NULL DEFAULT '',
  color    TEXT NOT NULL DEFAULT '',
  keywords TEXT NOT NULL DEFAULT '',
  archived INTEGER NOT NULL DEFAULT 0,
  sort     INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE debts (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  name       TEXT NOT NULL,
  total      INTEGER NOT NULL DEFAULT 0,
  monthly    INTEGER NOT NULL DEFAULT 0,
  due_day    INTEGER NOT NULL DEFAULT 0,
  note       TEXT NOT NULL DEFAULT '',
  archived   INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE transactions (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  date          TEXT NOT NULL,
  type          TEXT NOT NULL CHECK (type IN ('expense','income','transfer','adjust')),
  amount        INTEGER NOT NULL,
  account_id    INTEGER NOT NULL REFERENCES accounts(id),
  to_account_id INTEGER REFERENCES accounts(id),
  category_id   INTEGER REFERENCES categories(id) ON DELETE SET NULL,
  debt_id       INTEGER REFERENCES debts(id) ON DELETE SET NULL,
  description   TEXT NOT NULL DEFAULT '',
  note          TEXT NOT NULL DEFAULT '',
  source        TEXT NOT NULL DEFAULT 'manual',
  raw           TEXT NOT NULL DEFAULT '',
  created_at    TEXT NOT NULL DEFAULT (datetime('now')),
  updated_at    TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX idx_tx_date ON transactions(date);
CREATE INDEX idx_tx_account ON transactions(account_id, date);
CREATE INDEX idx_tx_to_account ON transactions(to_account_id);
CREATE INDEX idx_tx_category ON transactions(category_id, date);
CREATE INDEX idx_tx_debt ON transactions(debt_id);

CREATE TABLE budgets (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  period      TEXT NOT NULL,
  category_id INTEGER NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
  amount      INTEGER NOT NULL DEFAULT 0,
  due_day     INTEGER NOT NULL DEFAULT 0,
  account_id  INTEGER REFERENCES accounts(id) ON DELETE SET NULL,
  note        TEXT NOT NULL DEFAULT '',
  UNIQUE (period, category_id)
);

CREATE TABLE rules (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  pattern     TEXT NOT NULL UNIQUE,
  type        TEXT NOT NULL DEFAULT '',
  category_id INTEGER REFERENCES categories(id) ON DELETE CASCADE,
  account_id  INTEGER REFERENCES accounts(id) ON DELETE SET NULL,
  hits        INTEGER NOT NULL DEFAULT 1,
  updated_at  TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE api_tokens (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  name         TEXT NOT NULL,
  token_hash   TEXT NOT NULL UNIQUE,
  prefix       TEXT NOT NULL,
  created_at   TEXT NOT NULL DEFAULT (datetime('now')),
  last_used_at TEXT
);

CREATE TABLE ai_logs (
  id                 INTEGER PRIMARY KEY AUTOINCREMENT,
  kind               TEXT NOT NULL,
  model              TEXT NOT NULL,
  input_tokens       INTEGER NOT NULL DEFAULT 0,
  output_tokens      INTEGER NOT NULL DEFAULT 0,
  cache_read_tokens  INTEGER NOT NULL DEFAULT 0,
  cache_write_tokens INTEGER NOT NULL DEFAULT 0,
  duration_ms        INTEGER NOT NULL DEFAULT 0,
  error              TEXT NOT NULL DEFAULT '',
  created_at         TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE insights (
  period     TEXT PRIMARY KEY,
  content    TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (datetime('now'))
);
`,
}

func (s *Store) migrate(ctx context.Context) error {
	if _, err := s.DB.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_version (version INTEGER NOT NULL)`); err != nil {
		return err
	}
	var v int
	err := s.DB.QueryRowContext(ctx, `SELECT version FROM schema_version`).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		if _, err := s.DB.ExecContext(ctx, `INSERT INTO schema_version (version) VALUES (0)`); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	fresh := v == 0
	for i := v; i < len(migrations); i++ {
		tx, err := s.DB.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, migrations[i]); err != nil {
			tx.Rollback()
			return fmt.Errorf("v%d: %w", i+1, err)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE schema_version SET version = ?`, i+1); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	if fresh {
		return s.seed(ctx)
	}
	return nil
}

// nullInt mengubah 0 menjadi NULL.
func nullInt(v *int64) any {
	if v == nil || *v == 0 {
		return nil
	}
	return *v
}

func ptr(n sql.NullInt64) *int64 {
	if !n.Valid {
		return nil
	}
	v := n.Int64
	return &v
}

func parseDate(s string) (time.Time, error) { return time.Parse("2006-01-02", s) }
