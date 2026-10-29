package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// Transaction adalah satu catatan arus uang.
//
// Jenis:
//   - expense:  uang keluar dari AccountID
//   - income:   uang masuk ke AccountID
//   - transfer: pindah dari AccountID ke ToAccountID (mis. tarik tunai, top up)
//   - adjust:   penyesuaian saldo; Amount boleh negatif, tidak masuk laporan
type Transaction struct {
	ID          int64  `json:"id"`
	Date        string `json:"date"`
	Type        string `json:"type"`
	Amount      int64  `json:"amount"`
	AccountID   int64  `json:"account_id"`
	ToAccountID *int64 `json:"to_account_id"`
	CategoryID  *int64 `json:"category_id"`
	DebtID      *int64 `json:"debt_id"`
	// IncomeSourceID: sumber pemasukan (mis. CMW / Freelance), hanya untuk income.
	IncomeSourceID *int64 `json:"income_source_id"`
	LedgerID       int64  `json:"ledger_id"`
	Description    string `json:"description"`
	Note           string `json:"note"`
	Source         string `json:"source"`
	Raw            string `json:"raw,omitempty"`
	CreatedAt      string `json:"created_at"`
}

const txCols = `id, date, type, amount, account_id, to_account_id, category_id, debt_id, income_source_id, ledger_id, description, note, source, raw, created_at`

func scanTx(sc interface{ Scan(...any) error }) (Transaction, error) {
	var t Transaction
	var to, cat, debt, isrc sql.NullInt64
	err := sc.Scan(&t.ID, &t.Date, &t.Type, &t.Amount, &t.AccountID, &to, &cat, &debt, &isrc, &t.LedgerID, &t.Description, &t.Note, &t.Source, &t.Raw, &t.CreatedAt)
	t.ToAccountID, t.CategoryID, t.DebtID, t.IncomeSourceID = ptr(to), ptr(cat), ptr(debt), ptr(isrc)
	return t, err
}

var txSources = map[string]bool{"manual": true, "ai_text": true, "ai_image": true, "import": true, "shortcut": true, "bill": true}

// validate memeriksa dan merapikan transaksi sebelum disimpan.
func (s *Store) validate(ctx context.Context, t *Transaction) error {
	t.Date = strings.TrimSpace(t.Date)
	if t.Date == "" {
		t.Date = s.Today().Format("2006-01-02")
	}
	if _, err := time.Parse("2006-01-02", t.Date); err != nil {
		return invalid("tanggal tidak valid: %s", t.Date)
	}
	switch t.Type {
	case "expense", "income", "transfer":
		if t.Amount <= 0 {
			return invalid("nominal harus lebih dari 0")
		}
	case "adjust":
		if t.Amount == 0 {
			return invalid("nominal penyesuaian tidak boleh 0")
		}
	default:
		return invalid("jenis transaksi tidak dikenal: %s", t.Type)
	}
	acc, err := s.GetAccount(ctx, t.AccountID)
	if err != nil {
		return invalid("akun tidak ditemukan")
	}
	if acc.LedgerID != LedgerID(ctx) {
		return invalid("akun %s bukan bagian dari buku ini", acc.Name)
	}
	t.LedgerID = acc.LedgerID
	if t.Type == "transfer" {
		if t.ToAccountID == nil || *t.ToAccountID == 0 {
			return invalid("transfer butuh akun tujuan")
		}
		if *t.ToAccountID == t.AccountID {
			return invalid("akun asal dan tujuan tidak boleh sama")
		}
		if _, err := s.GetAccount(ctx, *t.ToAccountID); err != nil {
			return invalid("akun tujuan tidak ditemukan")
		}
		t.CategoryID, t.DebtID = nil, nil
	} else {
		t.ToAccountID = nil
	}
	if t.Type == "adjust" {
		t.CategoryID, t.DebtID = nil, nil
	}
	if t.CategoryID != nil && *t.CategoryID == 0 {
		t.CategoryID = nil
	}
	if t.CategoryID != nil {
		c, err := s.GetCategory(ctx, *t.CategoryID)
		if err != nil {
			return invalid("kategori tidak ditemukan")
		}
		if c.Kind != t.Type {
			// kategori tidak cocok dengan jenis transaksi: abaikan saja
			t.CategoryID = nil
		} else if strings.TrimSpace(t.Description) == "" {
			t.Description = c.Name
		}
	}
	if t.DebtID != nil && *t.DebtID == 0 {
		t.DebtID = nil
	}
	if t.DebtID != nil {
		if t.Type != "expense" {
			t.DebtID = nil
		} else if _, err := s.GetDebt(ctx, *t.DebtID); err != nil {
			return invalid("utang tidak ditemukan")
		}
	}
	if t.IncomeSourceID != nil && (*t.IncomeSourceID == 0 || t.Type != "income") {
		t.IncomeSourceID = nil
	}
	if t.IncomeSourceID != nil {
		var n int
		s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM income_sources WHERE id = ?`, *t.IncomeSourceID).Scan(&n)
		if n == 0 {
			return invalid("sumber pemasukan tidak ditemukan")
		}
	}
	t.Description = strings.TrimSpace(t.Description)
	if len(t.Description) > 200 {
		t.Description = t.Description[:200]
	}
	t.Note = strings.TrimSpace(t.Note)
	if t.Source == "" || !txSources[t.Source] {
		t.Source = "manual"
	}
	return nil
}

// CreateTransaction menyimpan satu transaksi.
func (s *Store) CreateTransaction(ctx context.Context, t Transaction) (Transaction, error) {
	if err := s.validate(ctx, &t); err != nil {
		return t, err
	}
	res, err := s.DB.ExecContext(ctx, `INSERT INTO transactions (date, type, amount, account_id, to_account_id, category_id, debt_id, income_source_id, ledger_id, description, note, source, raw)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		t.Date, t.Type, t.Amount, t.AccountID, nullInt(t.ToAccountID), nullInt(t.CategoryID), nullInt(t.DebtID), nullInt(t.IncomeSourceID), t.LedgerID, t.Description, t.Note, t.Source, t.Raw)
	if err != nil {
		return t, err
	}
	id, _ := res.LastInsertId()
	s.learn(ctx, t)
	return s.GetTransaction(ctx, id)
}

// CreateTransactions menyimpan banyak transaksi sekaligus (semua atau tidak sama sekali).
func (s *Store) CreateTransactions(ctx context.Context, list []Transaction) ([]Transaction, error) {
	for i := range list {
		if err := s.validate(ctx, &list[i]); err != nil {
			return nil, invalid("baris %d: %s", i+1, err.Error())
		}
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	ids := make([]int64, 0, len(list))
	for _, t := range list {
		res, err := tx.ExecContext(ctx, `INSERT INTO transactions (date, type, amount, account_id, to_account_id, category_id, debt_id, income_source_id, ledger_id, description, note, source, raw)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			t.Date, t.Type, t.Amount, t.AccountID, nullInt(t.ToAccountID), nullInt(t.CategoryID), nullInt(t.DebtID), nullInt(t.IncomeSourceID), t.LedgerID, t.Description, t.Note, t.Source, t.Raw)
		if err != nil {
			return nil, err
		}
		id, _ := res.LastInsertId()
		ids = append(ids, id)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	out := make([]Transaction, 0, len(ids))
	for i, id := range ids {
		s.learn(ctx, list[i])
		t, err := s.GetTransaction(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

// GetTransaction mengambil satu transaksi.
func (s *Store) GetTransaction(ctx context.Context, id int64) (Transaction, error) {
	t, err := scanTx(s.DB.QueryRowContext(ctx, `SELECT `+txCols+` FROM transactions WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return t, ErrNotFound
	}
	return t, err
}

// UpdateTransaction memperbarui transaksi.
func (s *Store) UpdateTransaction(ctx context.Context, t Transaction) (Transaction, error) {
	old, err := s.GetTransaction(ctx, t.ID)
	if err != nil {
		return t, err
	}
	if t.Source == "" {
		t.Source = old.Source
	}
	if t.Raw == "" {
		t.Raw = old.Raw
	}
	if err := s.validate(ctx, &t); err != nil {
		return t, err
	}
	_, err = s.DB.ExecContext(ctx, `UPDATE transactions SET date=?, type=?, amount=?, account_id=?, to_account_id=?, category_id=?, debt_id=?, income_source_id=?, ledger_id=?, description=?, note=?, source=?, raw=?, updated_at=datetime('now') WHERE id=?`,
		t.Date, t.Type, t.Amount, t.AccountID, nullInt(t.ToAccountID), nullInt(t.CategoryID), nullInt(t.DebtID), nullInt(t.IncomeSourceID), t.LedgerID, t.Description, t.Note, t.Source, t.Raw, t.ID)
	if err != nil {
		return t, err
	}
	// koreksi pengguna adalah sinyal belajar paling kuat
	if !sameInt(old.CategoryID, t.CategoryID) || old.AccountID != t.AccountID || old.Type != t.Type {
		s.learn(ctx, t)
	}
	return s.GetTransaction(ctx, t.ID)
}

// DeleteTransactions menghapus satu atau lebih transaksi.
func (s *Store) DeleteTransactions(ctx context.Context, ids []int64) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx, `DELETE FROM transactions WHERE id = ?`, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// TxFilter adalah filter daftar transaksi.
type TxFilter struct {
	From, To   string
	AccountID  int64
	CategoryID int64
	DebtID     int64
	Type       string
	Q          string
	Limit      int
	Offset     int
}

// ListTransactions mencari transaksi sesuai filter, terbaru dahulu.
func (s *Store) ListTransactions(ctx context.Context, f TxFilter) ([]Transaction, error) {
	where, args := []string{"(ledger_id = ? OR to_account_id IN (SELECT id FROM accounts WHERE ledger_id = ?))"}, []any{LedgerID(ctx), LedgerID(ctx)}
	if f.From != "" {
		where, args = append(where, "date >= ?"), append(args, f.From)
	}
	if f.To != "" {
		where, args = append(where, "date <= ?"), append(args, f.To)
	}
	if f.AccountID != 0 {
		where, args = append(where, "(account_id = ? OR to_account_id = ?)"), append(args, f.AccountID, f.AccountID)
	}
	if f.CategoryID != 0 {
		where, args = append(where, "category_id = ?"), append(args, f.CategoryID)
	}
	if f.DebtID != 0 {
		where, args = append(where, "debt_id = ?"), append(args, f.DebtID)
	}
	if f.Type != "" {
		where, args = append(where, "type = ?"), append(args, f.Type)
	}
	if q := strings.TrimSpace(f.Q); q != "" {
		like := "%" + strings.ToLower(q) + "%"
		where = append(where, "(lower(description) LIKE ? OR lower(note) LIKE ? OR lower(raw) LIKE ? OR CAST(amount AS TEXT) LIKE ?)")
		args = append(args, like, like, like, "%"+q+"%")
	}
	if f.Limit <= 0 || f.Limit > 1000 {
		f.Limit = 200
	}
	args = append(args, f.Limit, f.Offset)
	rows, err := s.DB.QueryContext(ctx, `SELECT `+txCols+` FROM transactions WHERE `+strings.Join(where, " AND ")+
		` ORDER BY date DESC, id DESC LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Transaction{}
	for rows.Next() {
		t, err := scanTx(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// FindDuplicate mencari transaksi yang kemungkinan sama (akun, jenis, nominal
// sama dengan tanggal berselisih maksimal 2 hari). Mengembalikan 0 bila tidak ada.
func (s *Store) FindDuplicate(ctx context.Context, t Transaction) int64 {
	d, err := time.Parse("2006-01-02", t.Date)
	if err != nil {
		return 0
	}
	from := d.AddDate(0, 0, -2).Format("2006-01-02")
	to := d.AddDate(0, 0, 2).Format("2006-01-02")
	var id int64
	err = s.DB.QueryRowContext(ctx, `SELECT id FROM transactions WHERE ledger_id = ? AND account_id = ? AND type = ? AND amount = ? AND date BETWEEN ? AND ?
		ORDER BY abs(julianday(date) - julianday(?)) LIMIT 1`, LedgerID(ctx), t.AccountID, t.Type, t.Amount, from, to, t.Date).Scan(&id)
	if err != nil {
		return 0
	}
	return id
}

// Habit adalah pola transaksi yang sering dilakukan pengguna.
type Habit struct {
	Description string `json:"description"`
	Type        string `json:"type"`
	CategoryID  *int64 `json:"category_id"`
	AccountID   int64  `json:"account_id"`
	ToAccountID *int64 `json:"to_account_id"`
	Count       int    `json:"count"`
	AvgAmount   int64  `json:"avg_amount"`
	LastAmount  int64  `json:"last_amount"`
	LastDate    string `json:"last_date"`
}

// Habits mengembalikan pola transaksi tersering dalam `days` hari terakhir.
func (s *Store) Habits(ctx context.Context, days, limit int) ([]Habit, error) {
	since := s.Today().AddDate(0, 0, -days).Format("2006-01-02")
	rows, err := s.DB.QueryContext(ctx, `
SELECT description, type, category_id, account_id, to_account_id, COUNT(*) AS n, CAST(AVG(amount) AS INTEGER),
  (SELECT t2.amount FROM transactions t2 WHERE lower(t2.description) = lower(t.description) AND t2.type = t.type ORDER BY t2.date DESC, t2.id DESC LIMIT 1),
  MAX(date)
FROM transactions t
WHERE ledger_id = ? AND date >= ? AND type != 'adjust' AND description != ''
GROUP BY lower(description), type, category_id, account_id, to_account_id
ORDER BY n DESC, MAX(date) DESC
LIMIT ?`, LedgerID(ctx), since, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Habit{}
	for rows.Next() {
		var h Habit
		var cat, to sql.NullInt64
		if err := rows.Scan(&h.Description, &h.Type, &cat, &h.AccountID, &to, &h.Count, &h.AvgAmount, &h.LastAmount, &h.LastDate); err != nil {
			return nil, err
		}
		h.CategoryID, h.ToAccountID = ptr(cat), ptr(to)
		out = append(out, h)
	}
	return out, rows.Err()
}

func sameInt(a, b *int64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
