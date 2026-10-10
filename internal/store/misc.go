package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"strings"
)

// APIToken dipakai oleh iOS Shortcuts / integrasi lain.
type APIToken struct {
	ID         int64   `json:"id"`
	Name       string  `json:"name"`
	Prefix     string  `json:"prefix"`
	CreatedAt  string  `json:"created_at"`
	LastUsedAt *string `json:"last_used_at"`
}

func hashToken(t string) string {
	h := sha256.Sum256([]byte(t))
	return hex.EncodeToString(h[:])
}

// CreateToken membuat token baru; nilai mentahnya hanya dikembalikan sekali.
func (s *Store) CreateToken(ctx context.Context, name string) (string, APIToken, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "iPhone Shortcut"
	}
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", APIToken{}, err
	}
	raw := "mm_" + hex.EncodeToString(b)
	prefix := raw[:10]
	res, err := s.DB.ExecContext(ctx, `INSERT INTO api_tokens (name, token_hash, prefix) VALUES (?,?,?)`, name, hashToken(raw), prefix)
	if err != nil {
		return "", APIToken{}, err
	}
	id, _ := res.LastInsertId()
	var t APIToken
	err = s.DB.QueryRowContext(ctx, `SELECT id, name, prefix, created_at, last_used_at FROM api_tokens WHERE id = ?`, id).
		Scan(&t.ID, &t.Name, &t.Prefix, &t.CreatedAt, &t.LastUsedAt)
	return raw, t, err
}

// ListTokens mengembalikan semua token (tanpa nilai rahasia).
func (s *Store) ListTokens(ctx context.Context) ([]APIToken, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, name, prefix, created_at, last_used_at FROM api_tokens ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []APIToken{}
	for rows.Next() {
		var t APIToken
		if err := rows.Scan(&t.ID, &t.Name, &t.Prefix, &t.CreatedAt, &t.LastUsedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// DeleteToken mencabut token.
func (s *Store) DeleteToken(ctx context.Context, id int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM api_tokens WHERE id = ?`, id)
	return err
}

// VerifyToken memeriksa token mentah dan mencatat waktu pemakaian.
func (s *Store) VerifyToken(ctx context.Context, raw string) bool {
	if !strings.HasPrefix(raw, "mm_") {
		return false
	}
	var id int64
	err := s.DB.QueryRowContext(ctx, `SELECT id FROM api_tokens WHERE token_hash = ?`, hashToken(raw)).Scan(&id)
	if err != nil {
		return false
	}
	s.DB.ExecContext(ctx, `UPDATE api_tokens SET last_used_at = datetime('now') WHERE id = ?`, id)
	return true
}

// AILog adalah catatan pemakaian AI (untuk memantau biaya).
type AILog struct {
	Kind             string
	Model            string
	InputTokens      int64
	OutputTokens     int64
	CacheReadTokens  int64
	CacheWriteTokens int64
	DurationMS       int64
	Error            string
}

// LogAI menyimpan catatan pemakaian AI.
func (s *Store) LogAI(ctx context.Context, l AILog) {
	s.DB.ExecContext(ctx, `INSERT INTO ai_logs (kind, model, input_tokens, output_tokens, cache_read_tokens, cache_write_tokens, duration_ms, error) VALUES (?,?,?,?,?,?,?,?)`,
		l.Kind, l.Model, l.InputTokens, l.OutputTokens, l.CacheReadTokens, l.CacheWriteTokens, l.DurationMS, l.Error)
}

// AIUsage adalah ringkasan pemakaian AI per model.
type AIUsage struct {
	Model            string `json:"model"`
	Calls            int64  `json:"calls"`
	Errors           int64  `json:"errors"`
	InputTokens      int64  `json:"input_tokens"`
	OutputTokens     int64  `json:"output_tokens"`
	CacheReadTokens  int64  `json:"cache_read_tokens"`
	CacheWriteTokens int64  `json:"cache_write_tokens"`
}

// AIUsageSince merangkum pemakaian AI sejak tanggal tertentu.
func (s *Store) AIUsageSince(ctx context.Context, since string) ([]AIUsage, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT model, COUNT(*), SUM(error != ''), SUM(input_tokens), SUM(output_tokens), SUM(cache_read_tokens), SUM(cache_write_tokens)
		FROM ai_logs WHERE created_at >= ? GROUP BY model ORDER BY COUNT(*) DESC`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AIUsage{}
	for rows.Next() {
		var u AIUsage
		if err := rows.Scan(&u.Model, &u.Calls, &u.Errors, &u.InputTokens, &u.OutputTokens, &u.CacheReadTokens, &u.CacheWriteTokens); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// Insight adalah analisis AI yang disimpan per periode.
type Insight struct {
	Period    string `json:"period"`
	Content   string `json:"content"`
	CreatedAt string `json:"created_at"`
}

func (s *Store) GetInsight(ctx context.Context, key string) (*Insight, error) {
	var in Insight
	err := s.DB.QueryRowContext(ctx, `SELECT period, content, created_at FROM insights WHERE period = ?`, key).Scan(&in.Period, &in.Content, &in.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &in, err
}

func (s *Store) SaveInsight(ctx context.Context, key, content string) (*Insight, error) {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO insights (period, content) VALUES (?, ?) ON CONFLICT(period) DO UPDATE SET content = excluded.content, created_at = datetime('now')`, key, content)
	if err != nil {
		return nil, err
	}
	return s.GetInsight(ctx, key)
}
