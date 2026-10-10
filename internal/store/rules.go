package store

import (
	"context"
	"database/sql"
	"regexp"
	"strings"
)

// Rule adalah pola yang dipelajari dari kebiasaan pengguna: deskripsi tertentu
// biasanya masuk kategori X dan dibayar dari akun Y.
type Rule struct {
	ID         int64  `json:"id"`
	Pattern    string `json:"pattern"`
	Type       string `json:"type"`
	CategoryID *int64 `json:"category_id"`
	AccountID  *int64 `json:"account_id"`
	Hits       int    `json:"hits"`
	UpdatedAt  string `json:"updated_at"`
}

var nonWord = regexp.MustCompile(`[^a-z\s&]+`)
var spaces = regexp.MustCompile(`\s+`)

// NormalizePattern mengubah deskripsi menjadi pola: huruf kecil, tanpa angka
// dan tanda baca, maksimal 4 kata.
func NormalizePattern(desc string) string {
	s := strings.ToLower(desc)
	s = nonWord.ReplaceAllString(s, " ")
	s = strings.TrimSpace(spaces.ReplaceAllString(s, " "))
	words := strings.Fields(s)
	if len(words) > 4 {
		words = words[:4]
	}
	s = strings.Join(words, " ")
	if len(s) < 3 {
		return ""
	}
	return s
}

// learn memperbarui aturan dari transaksi yang baru disimpan/dikoreksi.
func (s *Store) learn(ctx context.Context, t Transaction) {
	if t.Type == "adjust" {
		return
	}
	p := NormalizePattern(t.Description)
	if p == "" {
		return
	}
	if t.Type != "transfer" && t.CategoryID == nil {
		return
	}
	acc := t.AccountID
	s.DB.ExecContext(ctx, `INSERT INTO rules (pattern, type, category_id, account_id) VALUES (?,?,?,?)
		ON CONFLICT(pattern) DO UPDATE SET
		  hits = CASE WHEN rules.category_id IS excluded.category_id AND rules.account_id IS excluded.account_id THEN rules.hits + 1 ELSE 1 END,
		  type = excluded.type, category_id = excluded.category_id, account_id = excluded.account_id, updated_at = datetime('now')`,
		p, t.Type, nullInt(t.CategoryID), acc)
}

// ListRules mengembalikan aturan terpopuler.
func (s *Store) ListRules(ctx context.Context, limit int) ([]Rule, error) {
	if limit <= 0 {
		limit = 500
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT id, pattern, type, category_id, account_id, hits, updated_at FROM rules ORDER BY hits DESC, updated_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Rule{}
	for rows.Next() {
		var r Rule
		var cat, acc sql.NullInt64
		if err := rows.Scan(&r.ID, &r.Pattern, &r.Type, &cat, &acc, &r.Hits, &r.UpdatedAt); err != nil {
			return nil, err
		}
		r.CategoryID, r.AccountID = ptr(cat), ptr(acc)
		out = append(out, r)
	}
	return out, rows.Err()
}

// DeleteRule menghapus satu aturan.
func (s *Store) DeleteRule(ctx context.Context, id int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM rules WHERE id = ?`, id)
	return err
}

// MatchRule mencari aturan terbaik yang polanya muncul di teks.
func (s *Store) MatchRule(ctx context.Context, text string) *Rule {
	full := " " + strings.TrimSpace(spaces.ReplaceAllString(nonWord.ReplaceAllString(strings.ToLower(text), " "), " ")) + " "
	rules, err := s.ListRules(ctx, 1000)
	if err != nil {
		return nil
	}
	var best *Rule
	for i := range rules {
		r := &rules[i]
		if !strings.Contains(full, " "+r.Pattern+" ") {
			continue
		}
		if best == nil || len(r.Pattern) > len(best.Pattern) || (len(r.Pattern) == len(best.Pattern) && r.Hits > best.Hits) {
			best = r
		}
	}
	return best
}
