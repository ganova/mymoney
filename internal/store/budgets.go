package store

import (
	"context"
	"database/sql"
	"errors"

	"github.com/ganova/mymoney/internal/period"
)

// Budget adalah rencana nominal satu kategori dalam satu periode.
// Bila DueDay > 0, baris ini diperlakukan sebagai tagihan (bill) dengan
// tanggal jatuh tempo, misalnya "Bayar Rumah tiap tanggal 28".
type Budget struct {
	ID         int64  `json:"id"`
	Period     string `json:"period"`
	CategoryID int64  `json:"category_id"`
	Amount     int64  `json:"amount"`
	DueDay     int    `json:"due_day"`
	AccountID  *int64 `json:"account_id"`
	Note       string `json:"note"`
	// Dihitung:
	Actual  int64  `json:"actual"`
	Kind    string `json:"kind"`
	DueDate string `json:"due_date,omitempty"`
}

// ListBudgets mengembalikan baris budget periode beserta realisasinya.
func (s *Store) ListBudgets(ctx context.Context, p period.Period) ([]Budget, error) {
	rows, err := s.DB.QueryContext(ctx, `
SELECT b.id, b.period, b.category_id, b.amount, b.due_day, b.account_id, b.note, c.kind,
  COALESCE((SELECT SUM(t.amount) FROM transactions t
            WHERE t.category_id = b.category_id AND t.type = c.kind AND t.date BETWEEN ? AND ?), 0)
FROM budgets b JOIN categories c ON c.id = b.category_id
WHERE b.period = ?
ORDER BY c.kind DESC, c.sort, c.id`, p.StartStr(), p.EndStr(), p.Key)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Budget{}
	for rows.Next() {
		var b Budget
		var acc sql.NullInt64
		if err := rows.Scan(&b.ID, &b.Period, &b.CategoryID, &b.Amount, &b.DueDay, &acc, &b.Note, &b.Kind, &b.Actual); err != nil {
			return nil, err
		}
		b.AccountID = ptr(acc)
		if b.DueDay > 0 {
			b.DueDate = dueDate(p, b.DueDay)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// dueDate menentukan tanggal jatuh tempo di dalam periode untuk hari tertentu.
func dueDate(p period.Period, day int) string {
	for d := p.Start; !d.After(p.End); d = d.AddDate(0, 0, 1) {
		if d.Day() == day {
			return d.Format("2006-01-02")
		}
	}
	// hari tidak ada di periode (mis. 31): pakai akhir bulan terdekat
	return p.End.Format("2006-01-02")
}

// HasBudget melaporkan apakah periode sudah punya baris budget.
func (s *Store) HasBudget(ctx context.Context, key string) (bool, error) {
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM budgets WHERE period = ?`, key).Scan(&n)
	return n > 0, err
}

// EnsureBudget menyalin budget dari periode terakhir yang punya budget bila
// periode ini (periode berjalan atau sesudahnya) masih kosong. Dengan begitu
// rencana bulanan cukup diatur sekali dan otomatis terbawa ke periode berikutnya.
func (s *Store) EnsureBudget(ctx context.Context, p period.Period) (bool, error) {
	cur, err := s.CurrentPeriod(ctx)
	if err != nil {
		return false, err
	}
	if p.Key < cur.Key {
		return false, nil
	}
	has, err := s.HasBudget(ctx, p.Key)
	if err != nil || has {
		return false, err
	}
	var src string
	err = s.DB.QueryRowContext(ctx, `SELECT period FROM budgets WHERE period < ? ORDER BY period DESC LIMIT 1`, p.Key).Scan(&src)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	n, err := s.CopyBudget(ctx, src, p.Key, false)
	return n > 0, err
}

// CopyBudget menyalin baris budget dari satu periode ke periode lain.
// Bila overwrite=false, kategori yang sudah ada di tujuan tidak diubah.
func (s *Store) CopyBudget(ctx context.Context, from, to string, overwrite bool) (int64, error) {
	q := `INSERT INTO budgets (period, category_id, amount, due_day, account_id, note)
		SELECT ?, category_id, amount, due_day, account_id, note FROM budgets WHERE period = ?`
	if overwrite {
		q += ` ON CONFLICT(period, category_id) DO UPDATE SET amount = excluded.amount, due_day = excluded.due_day, account_id = excluded.account_id, note = excluded.note`
	} else {
		q += ` ON CONFLICT(period, category_id) DO NOTHING`
	}
	res, err := s.DB.ExecContext(ctx, q, to, from)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// UpsertBudget membuat atau memperbarui baris budget untuk (periode, kategori).
func (s *Store) UpsertBudget(ctx context.Context, b Budget) (Budget, error) {
	if b.Period == "" {
		return b, invalid("periode wajib diisi")
	}
	if _, err := s.GetCategory(ctx, b.CategoryID); err != nil {
		return b, invalid("kategori tidak ditemukan")
	}
	if b.Amount < 0 {
		return b, invalid("nominal budget tidak boleh negatif")
	}
	if b.DueDay < 0 || b.DueDay > 31 {
		return b, invalid("tanggal jatuh tempo harus 1–31")
	}
	err := s.DB.QueryRowContext(ctx, `INSERT INTO budgets (period, category_id, amount, due_day, account_id, note) VALUES (?,?,?,?,?,?)
		ON CONFLICT(period, category_id) DO UPDATE SET amount = excluded.amount, due_day = excluded.due_day, account_id = excluded.account_id, note = excluded.note
		RETURNING id`, b.Period, b.CategoryID, b.Amount, b.DueDay, nullInt(b.AccountID), b.Note).Scan(&b.ID)
	return b, err
}

// DeleteBudget menghapus satu baris budget.
func (s *Store) DeleteBudget(ctx context.Context, id int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM budgets WHERE id = ?`, id)
	return err
}

// ClearBudget menghapus seluruh budget pada periode.
func (s *Store) ClearBudget(ctx context.Context, key string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM budgets WHERE period = ?`, key)
	return err
}
