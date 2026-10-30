package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// Receivable adalah pendapatan yang sudah pasti tetapi belum cair, mis. dana
// proyek yang dibayar klien dan ditahan Fastwork sampai proyek selesai.
type Receivable struct {
	ID             int64  `json:"id"`
	Name           string `json:"name"`
	Client         string `json:"client"` // klien / platform, mis. Fastwork
	Amount         int64  `json:"amount"` // bersih yang akan diterima
	ExpectedDate   string `json:"expected_date"`
	IncomeSourceID *int64 `json:"income_source_id"`
	Status         string `json:"status"` // pending | received | cancelled
	Note           string `json:"note"`
	TxID           *int64 `json:"tx_id"`
	ReceivedAt     string `json:"received_at"`
	CreatedAt      string `json:"created_at"`
}

const recvCols = `id, name, client, amount, expected_date, income_source_id, status, note, tx_id, received_at, created_at`

func scanRecv(sc interface{ Scan(...any) error }) (Receivable, error) {
	var r Receivable
	var src, tx sql.NullInt64
	err := sc.Scan(&r.ID, &r.Name, &r.Client, &r.Amount, &r.ExpectedDate, &src, &r.Status, &r.Note, &tx, &r.ReceivedAt, &r.CreatedAt)
	r.IncomeSourceID, r.TxID = ptr(src), ptr(tx)
	return r, err
}

// ListReceivables mengembalikan pendapatan tertunda buku aktif (yang menunggu dahulu, tanggal terdekat di atas).
func (s *Store) ListReceivables(ctx context.Context) ([]Receivable, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+recvCols+` FROM receivables WHERE ledger_id = ?
		ORDER BY CASE status WHEN 'pending' THEN 0 ELSE 1 END, CASE WHEN expected_date = '' THEN 1 ELSE 0 END, expected_date, id DESC`, LedgerID(ctx))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Receivable{}
	for rows.Next() {
		r, err := scanRecv(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) GetReceivable(ctx context.Context, id int64) (Receivable, error) {
	r, err := scanRecv(s.DB.QueryRowContext(ctx, `SELECT `+recvCols+` FROM receivables WHERE id = ? AND ledger_id = ?`, id, LedgerID(ctx)))
	if errors.Is(err, sql.ErrNoRows) {
		return r, ErrNotFound
	}
	return r, err
}

func (r *Receivable) normalize() error {
	r.Name = strings.TrimSpace(r.Name)
	r.Client = strings.TrimSpace(r.Client)
	if r.Name == "" {
		return invalid("nama proyek wajib diisi")
	}
	if r.Amount <= 0 {
		return invalid("nominal harus lebih dari 0")
	}
	if r.ExpectedDate != "" {
		if _, err := time.Parse("2006-01-02", r.ExpectedDate); err != nil {
			return invalid("tanggal estimasi tidak valid")
		}
	}
	if r.IncomeSourceID != nil && *r.IncomeSourceID == 0 {
		r.IncomeSourceID = nil
	}
	switch r.Status {
	case "", "pending":
		r.Status = "pending"
	case "received", "cancelled":
	default:
		return invalid("status tidak dikenal: %s", r.Status)
	}
	return nil
}

// CreateReceivable menambah pendapatan tertunda.
func (s *Store) CreateReceivable(ctx context.Context, r Receivable) (Receivable, error) {
	if err := r.normalize(); err != nil {
		return r, err
	}
	r.Status = "pending"
	res, err := s.DB.ExecContext(ctx, `INSERT INTO receivables (ledger_id, name, client, amount, expected_date, income_source_id, status, note)
		VALUES (?,?,?,?,?,?,?,?)`, LedgerID(ctx), r.Name, r.Client, r.Amount, r.ExpectedDate, nullInt(r.IncomeSourceID), r.Status, r.Note)
	if err != nil {
		return r, err
	}
	id, _ := res.LastInsertId()
	return s.GetReceivable(ctx, id)
}

// UpdateReceivable memperbarui pendapatan tertunda (status "received" hanya lewat ReceiveReceivable).
func (s *Store) UpdateReceivable(ctx context.Context, r Receivable) (Receivable, error) {
	old, err := s.GetReceivable(ctx, r.ID)
	if err != nil {
		return r, err
	}
	if err := r.normalize(); err != nil {
		return r, err
	}
	if old.Status == "received" || r.Status == "received" {
		r.Status = old.Status // sudah cair: hanya catatan/label yang berubah
		r.Amount, r.ExpectedDate = old.Amount, old.ExpectedDate
	}
	_, err = s.DB.ExecContext(ctx, `UPDATE receivables SET name=?, client=?, amount=?, expected_date=?, income_source_id=?, status=?, note=? WHERE id=? AND ledger_id=?`,
		r.Name, r.Client, r.Amount, r.ExpectedDate, nullInt(r.IncomeSourceID), r.Status, r.Note, r.ID, LedgerID(ctx))
	if err != nil {
		return r, err
	}
	return s.GetReceivable(ctx, r.ID)
}

// DeleteReceivable menghapus catatan; transaksi pemasukan yang sudah dibuat tetap ada.
func (s *Store) DeleteReceivable(ctx context.Context, id int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM receivables WHERE id = ? AND ledger_id = ?`, id, LedgerID(ctx))
	return err
}

// ReceiveReceivable mencairkan: membuat transaksi pemasukan ke akun yang dipilih
// dan menandai pendapatan tertunda sebagai sudah diterima. amount boleh berbeda
// dari estimasi (mis. dipotong biaya platform); 0 = pakai nominal estimasi.
func (s *Store) ReceiveReceivable(ctx context.Context, id, accountID, amount int64, date string) (Receivable, *Transaction, error) {
	r, err := s.GetReceivable(ctx, id)
	if err != nil {
		return r, nil, err
	}
	if r.Status != "pending" {
		return r, nil, invalid("pendapatan ini sudah %s", map[string]string{"received": "diterima", "cancelled": "dibatalkan"}[r.Status])
	}
	if amount <= 0 {
		amount = r.Amount
	}
	if date == "" {
		date = s.Today().Format("2006-01-02")
	}
	var catID *int64
	cats, _ := s.ListCategories(ctx)
	for _, want := range []string{"Pembayaran Klien", "Pemasukan Work Lain", "Pemasukan Lain"} {
		for _, c := range cats {
			if c.Kind == "income" && !c.Archived && c.Name == want {
				id := c.ID
				catID = &id
				break
			}
		}
		if catID != nil {
			break
		}
	}
	desc := r.Name
	if r.Client != "" {
		desc += " (" + r.Client + ")"
	}
	tx, err := s.CreateTransaction(ctx, Transaction{
		Date: date, Type: "income", Amount: amount, AccountID: accountID, CategoryID: catID,
		IncomeSourceID: r.IncomeSourceID, Description: desc, Source: "manual",
	})
	if err != nil {
		return r, nil, err
	}
	if _, err := s.DB.ExecContext(ctx, `UPDATE receivables SET status='received', tx_id=?, received_at=? WHERE id=?`, tx.ID, date, id); err != nil {
		return r, &tx, err
	}
	r, err = s.GetReceivable(ctx, id)
	return r, &tx, err
}
