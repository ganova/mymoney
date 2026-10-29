package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

// Account adalah kas/dompet/rekening.
type Account struct {
	ID             int64  `json:"id"`
	Name           string `json:"name"`
	Type           string `json:"type"` // bank, cash, ewallet, credit, saving, other
	Icon           string `json:"icon"`
	Color          string `json:"color"`
	OpeningBalance int64  `json:"opening_balance"`
	Aliases        string `json:"aliases"`
	LedgerID       int64  `json:"ledger_id"`
	Archived       bool   `json:"archived"`
	Sort           int    `json:"sort"`
	Balance        int64  `json:"balance"`
	TxCount        int64  `json:"tx_count"`
}

var accountTypes = map[string]bool{"bank": true, "cash": true, "ewallet": true, "credit": true, "saving": true, "other": true}

// balanceSQL menghitung saldo setiap akun dari seluruh transaksi.
const balanceSQL = `
SELECT a.id, a.name, a.type, a.icon, a.color, a.opening_balance, a.aliases, a.archived, a.sort, a.ledger_id,
  a.opening_balance
  + COALESCE((SELECT SUM(CASE t.type WHEN 'income' THEN t.amount WHEN 'adjust' THEN t.amount ELSE -t.amount END)
              FROM transactions t WHERE t.account_id = a.id), 0)
  + COALESCE((SELECT SUM(t.amount) FROM transactions t WHERE t.to_account_id = a.id AND t.type = 'transfer'), 0) AS balance,
  (SELECT COUNT(*) FROM transactions t WHERE t.account_id = a.id OR t.to_account_id = a.id) AS tx_count
FROM accounts a`

func scanAccount(sc interface{ Scan(...any) error }) (Account, error) {
	var a Account
	err := sc.Scan(&a.ID, &a.Name, &a.Type, &a.Icon, &a.Color, &a.OpeningBalance, &a.Aliases, &a.Archived, &a.Sort, &a.LedgerID, &a.Balance, &a.TxCount)
	return a, err
}

// ListAccounts mengembalikan semua akun beserta saldonya.
func (s *Store) ListAccounts(ctx context.Context) ([]Account, error) {
	rows, err := s.DB.QueryContext(ctx, balanceSQL+` WHERE a.ledger_id = ? ORDER BY a.archived, a.sort, a.id`, LedgerID(ctx))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Account{}
	for rows.Next() {
		a, err := scanAccount(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// GetAccount mengambil satu akun.
func (s *Store) GetAccount(ctx context.Context, id int64) (Account, error) {
	a, err := scanAccount(s.DB.QueryRowContext(ctx, balanceSQL+` WHERE a.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return a, ErrNotFound
	}
	return a, err
}

func (a *Account) normalize() error {
	a.Name = strings.TrimSpace(a.Name)
	if a.Name == "" {
		return invalid("nama akun wajib diisi")
	}
	if a.Type == "" {
		a.Type = "bank"
	}
	if !accountTypes[a.Type] {
		return invalid("jenis akun tidak dikenal: %s", a.Type)
	}
	a.Aliases = normalizeList(a.Aliases)
	return nil
}

// CreateAccount menambah akun baru.
func (s *Store) CreateAccount(ctx context.Context, a Account) (Account, error) {
	if err := a.normalize(); err != nil {
		return a, err
	}
	if a.Sort == 0 {
		s.DB.QueryRowContext(ctx, `SELECT COALESCE(MAX(sort),0)+1 FROM accounts`).Scan(&a.Sort)
	}
	res, err := s.DB.ExecContext(ctx, `INSERT INTO accounts (name, type, icon, color, opening_balance, aliases, archived, sort, ledger_id) VALUES (?,?,?,?,?,?,?,?,?)`,
		a.Name, a.Type, a.Icon, a.Color, a.OpeningBalance, a.Aliases, a.Archived, a.Sort, LedgerID(ctx))
	if err != nil {
		return a, err
	}
	id, _ := res.LastInsertId()
	return s.GetAccount(ctx, id)
}

// UpdateAccount memperbarui akun.
func (s *Store) UpdateAccount(ctx context.Context, a Account) (Account, error) {
	if err := a.normalize(); err != nil {
		return a, err
	}
	res, err := s.DB.ExecContext(ctx, `UPDATE accounts SET name=?, type=?, icon=?, color=?, opening_balance=?, aliases=?, archived=?, sort=? WHERE id=?`,
		a.Name, a.Type, a.Icon, a.Color, a.OpeningBalance, a.Aliases, a.Archived, a.Sort, a.ID)
	if err != nil {
		return a, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return a, ErrNotFound
	}
	return s.GetAccount(ctx, a.ID)
}

// DeleteAccount menghapus akun yang belum punya transaksi.
func (s *Store) DeleteAccount(ctx context.Context, id int64) error {
	a, err := s.GetAccount(ctx, id)
	if err != nil {
		return err
	}
	if a.TxCount > 0 {
		return invalid("akun %s masih punya %d transaksi — arsipkan saja agar riwayat tetap utuh", a.Name, a.TxCount)
	}
	_, err = s.DB.ExecContext(ctx, `DELETE FROM accounts WHERE id = ?`, id)
	return err
}

// Reconcile menyesuaikan saldo akun agar sama dengan saldo sebenarnya
// (misalnya yang terlihat di aplikasi bank) dengan membuat transaksi penyesuaian.
func (s *Store) Reconcile(ctx context.Context, id, actual int64, date string) (*Transaction, error) {
	a, err := s.GetAccount(ctx, id)
	if err != nil {
		return nil, err
	}
	diff := actual - a.Balance
	if diff == 0 {
		return nil, nil
	}
	if date == "" {
		date = s.Today().Format("2006-01-02")
	}
	t, err := s.CreateTransaction(ctx, Transaction{
		Date: date, Type: "adjust", Amount: diff, AccountID: id,
		Description: "Penyesuaian saldo", Source: "manual",
	})
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// normalizeList merapikan daftar dipisah koma menjadi huruf kecil tanpa duplikat.
func normalizeList(s string) string {
	seen := map[string]bool{}
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.ToLower(strings.TrimSpace(p))
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return strings.Join(out, ",")
}
