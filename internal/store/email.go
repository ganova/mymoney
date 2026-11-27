package store

import (
	"context"
)

// EmailImport mencatat satu email notifikasi yang sudah diproses.
type EmailImport struct {
	ID         int64  `json:"id"`
	LedgerID   int64  `json:"ledger_id"`
	MessageID  string `json:"message_id"`
	Sender     string `json:"sender"`
	Subject    string `json:"subject"`
	ReceivedAt string `json:"received_at"`
	Status     string `json:"status"` // tersimpan | duplikat | dilewati | gagal
	TxIDs      string `json:"tx_ids"` // id transaksi, dipisah koma
	Note       string `json:"note"`
	CreatedAt  string `json:"created_at"`
}

// HasEmailImport melaporkan apakah email ini sudah pernah diproses.
func (s *Store) HasEmailImport(ctx context.Context, messageID string) bool {
	var id int64
	err := s.DB.QueryRowContext(ctx, `SELECT id FROM email_imports WHERE message_id = ?`, messageID).Scan(&id)
	return err == nil
}

// AddEmailImport menyimpan hasil pemrosesan email.
func (s *Store) AddEmailImport(ctx context.Context, e EmailImport) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO email_imports (ledger_id, message_id, sender, subject, received_at, status, tx_ids, note)
		VALUES (?,?,?,?,?,?,?,?) ON CONFLICT(message_id) DO NOTHING`,
		e.LedgerID, e.MessageID, e.Sender, e.Subject, e.ReceivedAt, e.Status, e.TxIDs, e.Note)
	return err
}

// ListEmailImports mengembalikan riwayat terbaru (semua buku).
func (s *Store) ListEmailImports(ctx context.Context, limit int) ([]EmailImport, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT id, ledger_id, message_id, sender, subject, received_at, status, tx_ids, note, created_at
		FROM email_imports ORDER BY received_at DESC, id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []EmailImport{}
	for rows.Next() {
		var e EmailImport
		if err := rows.Scan(&e.ID, &e.LedgerID, &e.MessageID, &e.Sender, &e.Subject, &e.ReceivedAt, &e.Status, &e.TxIDs, &e.Note, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// DeleteEmailImport menghapus catatan agar email diproses ulang pada
// pemeriksaan berikutnya (transaksi yang sudah dibuat tidak ikut terhapus).
func (s *Store) DeleteEmailImport(ctx context.Context, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM email_imports WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
