package store

import (
	"context"
	"strings"
)

type ledgerKey struct{}

// DefaultLedger adalah buku pertama ("Pribadi").
const DefaultLedger int64 = 1

// WithLedger menandai buku keuangan aktif pada context; seluruh query akun,
// transaksi, budget, utang, dan laporan hanya melihat buku tersebut.
func WithLedger(ctx context.Context, id int64) context.Context {
	return context.WithValue(ctx, ledgerKey{}, id)
}

// LedgerID mengembalikan buku aktif pada context (default: Pribadi).
func LedgerID(ctx context.Context) int64 {
	if id, ok := ctx.Value(ledgerKey{}).(int64); ok && id > 0 {
		return id
	}
	return DefaultLedger
}

// Ledger adalah satu buku keuangan terpisah (mis. Pribadi dan Work).
type Ledger struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Icon string `json:"icon"`
	Sort int    `json:"sort"`
}

// ListLedgers mengembalikan semua buku.
func (s *Store) ListLedgers(ctx context.Context) ([]Ledger, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, name, icon, sort FROM ledgers ORDER BY sort, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Ledger{}
	for rows.Next() {
		var l Ledger
		if err := rows.Scan(&l.ID, &l.Name, &l.Icon, &l.Sort); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// ValidLedger memeriksa apakah buku ada.
func (s *Store) ValidLedger(ctx context.Context, id int64) bool {
	var n int
	s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM ledgers WHERE id = ?`, id).Scan(&n)
	return n > 0
}

// UpdateLedger mengganti nama/ikon buku.
func (s *Store) UpdateLedger(ctx context.Context, l Ledger) (Ledger, error) {
	l.Name = strings.TrimSpace(l.Name)
	if l.Name == "" {
		return l, invalid("nama buku wajib diisi")
	}
	res, err := s.DB.ExecContext(ctx, `UPDATE ledgers SET name = ?, icon = ? WHERE id = ?`, l.Name, l.Icon, l.ID)
	if err != nil {
		return l, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return l, ErrNotFound
	}
	return l, nil
}

// IncomeSource adalah sumber pemasukan (mis. CMW, Freelance).
type IncomeSource struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Icon     string `json:"icon"`
	Keywords string `json:"keywords"`
	Archived bool   `json:"archived"`
	Sort     int    `json:"sort"`
}

// ListIncomeSources mengembalikan semua sumber pemasukan.
func (s *Store) ListIncomeSources(ctx context.Context) ([]IncomeSource, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, name, icon, keywords, archived, sort FROM income_sources ORDER BY archived, sort, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []IncomeSource{}
	for rows.Next() {
		var x IncomeSource
		if err := rows.Scan(&x.ID, &x.Name, &x.Icon, &x.Keywords, &x.Archived, &x.Sort); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (x *IncomeSource) normalize() error {
	x.Name = strings.TrimSpace(x.Name)
	if x.Name == "" {
		return invalid("nama sumber pemasukan wajib diisi")
	}
	x.Keywords = normalizeList(x.Keywords)
	return nil
}

// CreateIncomeSource menambah sumber pemasukan.
func (s *Store) CreateIncomeSource(ctx context.Context, x IncomeSource) (IncomeSource, error) {
	if err := x.normalize(); err != nil {
		return x, err
	}
	s.DB.QueryRowContext(ctx, `SELECT COALESCE(MAX(sort),0)+1 FROM income_sources`).Scan(&x.Sort)
	res, err := s.DB.ExecContext(ctx, `INSERT INTO income_sources (name, icon, keywords, archived, sort) VALUES (?,?,?,?,?)`,
		x.Name, x.Icon, x.Keywords, x.Archived, x.Sort)
	if err != nil {
		return x, err
	}
	x.ID, _ = res.LastInsertId()
	return x, nil
}

// UpdateIncomeSource memperbarui sumber pemasukan.
func (s *Store) UpdateIncomeSource(ctx context.Context, x IncomeSource) (IncomeSource, error) {
	if err := x.normalize(); err != nil {
		return x, err
	}
	res, err := s.DB.ExecContext(ctx, `UPDATE income_sources SET name=?, icon=?, keywords=?, archived=?, sort=? WHERE id=?`,
		x.Name, x.Icon, x.Keywords, x.Archived, x.Sort, x.ID)
	if err != nil {
		return x, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return x, ErrNotFound
	}
	return x, nil
}

// DeleteIncomeSource menghapus sumber; transaksi lama kehilangan labelnya.
func (s *Store) DeleteIncomeSource(ctx context.Context, id int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM income_sources WHERE id = ?`, id)
	return err
}

type seedWorkCategory struct{ name, kind, grp, icon, keywords string }

var workCategories = []seedWorkCategory{
	{"Software & Langganan", "expense", "Work", "💻", "software,langganan,subscription,saas,lisensi,github,figma,notion,chatgpt,openai,claude"},
	{"Hosting & Domain", "expense", "Work", "🌐", "hosting,domain,vps,server,cloud,aws,gcp,vercel,cloudflare"},
	{"Peralatan Kerja", "expense", "Work", "🖥️", "laptop,monitor,keyboard,mouse,alat,peralatan,printer,atk"},
	{"Transport Dinas", "expense", "Work", "🚕", "transport dinas,perjalanan dinas,tiket,hotel,meeting offsite"},
	{"Makan Meeting", "expense", "Work", "🍽️", "meeting,makan klien,jamuan,coffee meeting"},
	{"Subkon & Tim", "expense", "Work", "🤝", "subkon,freelancer,tim,bayar desainer,bayar developer"},
	{"Pajak & Legal", "expense", "Work", "🧾", "pajak,pph,ppn,npwp,legal,notaris,izin"},
	{"Iklan & Promosi", "expense", "Work", "📣", "iklan,ads,promosi,marketing,meta ads,google ads"},
	{"Lain-lain Work", "expense", "Work", "📌", ""},
	{"Pembayaran Klien", "income", "Work", "🧑‍💼", "klien,client,invoice,pelunasan,dp,termin"},
	{"Pemasukan Work Lain", "income", "Work", "💵", ""},
}

// seedWork menyiapkan buku Work (akun & kategori) sekali saja, juga pada
// database lama yang baru dimigrasikan.
func (s *Store) seedWork(ctx context.Context) error {
	if v, _ := s.GetSetting(ctx, "work_seeded"); v == "1" {
		return nil
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var n int
	tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM accounts WHERE ledger_id = 2`).Scan(&n)
	if n == 0 {
		for i, a := range []seedAccount{
			{"Rekening Work", "bank", "💼", "#37474f", "work,kantor,usaha,rekening work"},
			{"Kas Work", "cash", "💵", "#2e7d32", "kas work,cash work"},
		} {
			if _, err := tx.ExecContext(ctx, `INSERT INTO accounts (name, type, icon, color, aliases, sort, ledger_id) VALUES (?,?,?,?,?,?,2)`,
				a.name, a.typ, a.icon, a.color, a.aliases, i); err != nil {
				return err
			}
		}
	}
	for i, c := range workCategories {
		if _, err := tx.ExecContext(ctx, `INSERT INTO categories (name, kind, grp, icon, keywords, sort, ledger_id) VALUES (?,?,?,?,?,?,2)`,
			c.name, c.kind, c.grp, c.icon, c.keywords, 1000+i); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES ('work_seeded', '1') ON CONFLICT(key) DO UPDATE SET value = '1'`); err != nil {
		return err
	}
	return tx.Commit()
}
