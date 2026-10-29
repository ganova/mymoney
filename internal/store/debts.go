package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

// Debt adalah utang/cicilan/paylater yang sedang berjalan. Pembayarannya
// dicatat sebagai transaksi pengeluaran dengan DebtID terisi.
type Debt struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Total     int64  `json:"total"`
	Monthly   int64  `json:"monthly"`
	DueDay    int    `json:"due_day"`
	Note      string `json:"note"`
	Archived  bool   `json:"archived"`
	CreatedAt string `json:"created_at"`
	// Dihitung:
	Paid      int64  `json:"paid"`
	Remaining int64  `json:"remaining"`
	LastPaid  string `json:"last_paid"`
}

const debtSQL = `
SELECT d.id, d.name, d.total, d.monthly, d.due_day, d.note, d.archived, d.created_at,
  COALESCE((SELECT SUM(t.amount) FROM transactions t WHERE t.debt_id = d.id AND t.type = 'expense'), 0),
  COALESCE((SELECT MAX(t.date) FROM transactions t WHERE t.debt_id = d.id), '')
FROM debts d`

func scanDebt(sc interface{ Scan(...any) error }) (Debt, error) {
	var d Debt
	err := sc.Scan(&d.ID, &d.Name, &d.Total, &d.Monthly, &d.DueDay, &d.Note, &d.Archived, &d.CreatedAt, &d.Paid, &d.LastPaid)
	d.Remaining = d.Total - d.Paid
	if d.Remaining < 0 {
		d.Remaining = 0
	}
	return d, err
}

// ListDebts mengembalikan semua utang.
func (s *Store) ListDebts(ctx context.Context) ([]Debt, error) {
	rows, err := s.DB.QueryContext(ctx, debtSQL+` WHERE d.ledger_id = ? ORDER BY d.archived, d.id`, LedgerID(ctx))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Debt{}
	for rows.Next() {
		d, err := scanDebt(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// GetDebt mengambil satu utang.
func (s *Store) GetDebt(ctx context.Context, id int64) (Debt, error) {
	d, err := scanDebt(s.DB.QueryRowContext(ctx, debtSQL+` WHERE d.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return d, ErrNotFound
	}
	return d, err
}

func (d *Debt) normalize() error {
	d.Name = strings.TrimSpace(d.Name)
	if d.Name == "" {
		return invalid("nama utang wajib diisi")
	}
	if d.Total < 0 || d.Monthly < 0 {
		return invalid("nominal tidak boleh negatif")
	}
	if d.DueDay < 0 || d.DueDay > 31 {
		return invalid("tanggal jatuh tempo harus 1–31")
	}
	return nil
}

// CreateDebt menambah utang.
func (s *Store) CreateDebt(ctx context.Context, d Debt) (Debt, error) {
	if err := d.normalize(); err != nil {
		return d, err
	}
	res, err := s.DB.ExecContext(ctx, `INSERT INTO debts (name, total, monthly, due_day, note, archived, ledger_id) VALUES (?,?,?,?,?,?,?)`,
		d.Name, d.Total, d.Monthly, d.DueDay, d.Note, d.Archived, LedgerID(ctx))
	if err != nil {
		return d, err
	}
	id, _ := res.LastInsertId()
	return s.GetDebt(ctx, id)
}

// UpdateDebt memperbarui utang.
func (s *Store) UpdateDebt(ctx context.Context, d Debt) (Debt, error) {
	if err := d.normalize(); err != nil {
		return d, err
	}
	res, err := s.DB.ExecContext(ctx, `UPDATE debts SET name=?, total=?, monthly=?, due_day=?, note=?, archived=? WHERE id=?`,
		d.Name, d.Total, d.Monthly, d.DueDay, d.Note, d.Archived, d.ID)
	if err != nil {
		return d, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return d, ErrNotFound
	}
	return s.GetDebt(ctx, d.ID)
}

// DeleteDebt menghapus utang (riwayat pembayaran tetap ada sebagai transaksi).
func (s *Store) DeleteDebt(ctx context.Context, id int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM debts WHERE id = ?`, id)
	return err
}
