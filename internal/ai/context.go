package ai

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ganova/mymoney/internal/money"
	"github.com/ganova/mymoney/internal/period"
	"github.com/ganova/mymoney/internal/store"
)

// Draft adalah usulan transaksi dari AI yang menunggu konfirmasi pengguna.
type Draft struct {
	Type            string  `json:"type"`
	Amount          int64   `json:"amount"`
	AmountEstimated bool    `json:"amount_estimated"`
	Date            string  `json:"date"`
	Description     string  `json:"description"`
	CategoryID      *int64  `json:"category_id"`
	AccountID       int64   `json:"account_id"`
	ToAccountID     *int64  `json:"to_account_id"`
	DebtID          *int64  `json:"debt_id"`
	Note            string  `json:"note"`
	Raw             string  `json:"raw,omitempty"`
	Confidence      float64 `json:"confidence"`
	DuplicateOf     int64   `json:"duplicate_of,omitempty"`
	DuplicateInScan bool    `json:"duplicate_in_scan,omitempty"`
	Source          string  `json:"source"`
}

// Transaction mengubah draft menjadi transaksi siap simpan.
func (d Draft) Transaction() store.Transaction {
	return store.Transaction{
		Date: d.Date, Type: d.Type, Amount: d.Amount, AccountID: d.AccountID,
		ToAccountID: d.ToAccountID, CategoryID: d.CategoryID, DebtID: d.DebtID,
		Description: d.Description, Note: d.Note, Source: d.Source, Raw: d.Raw,
	}
}

func nullableInt(desc string) map[string]any {
	return map[string]any{"anyOf": []any{map[string]any{"type": "integer"}, map[string]any{"type": "null"}}, "description": desc}
}

// draftSchema adalah JSON schema satu transaksi (dipakai tool & structured output).
func draftSchema(withRaw bool) map[string]any {
	props := map[string]any{
		"type":             map[string]any{"type": "string", "enum": []string{"expense", "income", "transfer"}, "description": "expense=uang keluar, income=uang masuk, transfer=pindah antar akun milik sendiri"},
		"amount":           map[string]any{"type": "integer", "description": "Nominal rupiah, bilangan bulat positif. 0 bila tidak diketahui."},
		"amount_estimated": map[string]any{"type": "boolean", "description": "true bila nominal ditebak (tidak disebut pengguna)"},
		"date":             map[string]any{"type": "string", "description": "Tanggal transaksi, format YYYY-MM-DD"},
		"description":      map[string]any{"type": "string", "description": "Deskripsi singkat dan rapi tanpa nominal, mis. 'Makan siang'"},
		"category_id":      nullableInt("ID kategori dari daftar; null untuk transfer"),
		"account_id":       map[string]any{"type": "integer", "description": "ID akun sumber (yang dipotong / yang menerima untuk income)"},
		"to_account_id":    nullableInt("ID akun tujuan, hanya untuk transfer"),
		"debt_id":          nullableInt("ID utang bila ini pembayaran cicilan/paylater yang terdaftar"),
		"note":             map[string]any{"type": "string", "description": "Catatan tambahan (boleh kosong)"},
		"confidence":       map[string]any{"type": "number", "description": "Keyakinan 0..1 atas kategori, akun, dan nominal"},
	}
	required := []string{"type", "amount", "amount_estimated", "date", "description", "category_id", "account_id", "to_account_id", "debt_id", "note", "confidence"}
	if withRaw {
		props["raw"] = map[string]any{"type": "string", "description": "Teks asli baris transaksi seperti tertulis di sumber"}
		required = append(required, "raw")
	}
	return map[string]any{"type": "object", "additionalProperties": false, "properties": props, "required": required}
}

// Context adalah data pengguna yang dibutuhkan AI untuk memahami input.
type Context struct {
	Settings   store.Settings
	Period     period.Period
	Today      time.Time
	Accounts   []store.Account
	Categories []store.Category
	Debts      []store.Debt
	Rules      []store.Rule
	Habits     []store.Habit

	accByID map[int64]store.Account
	catByID map[int64]store.Category
	debtIDs map[int64]bool
}

var hari = [...]string{"Minggu", "Senin", "Selasa", "Rabu", "Kamis", "Jumat", "Sabtu"}

// LoadContext mengumpulkan data pengguna terbaru.
func LoadContext(ctx context.Context, st *store.Store) (*Context, error) {
	c := &Context{Today: st.Today()}
	var err error
	if c.Settings, err = st.GetSettings(ctx); err != nil {
		return nil, err
	}
	c.Period = period.For(c.Today, c.Settings.PeriodStartDay)
	if c.Accounts, err = st.ListAccounts(ctx); err != nil {
		return nil, err
	}
	if c.Categories, err = st.ListCategories(ctx); err != nil {
		return nil, err
	}
	if c.Debts, err = st.ListDebts(ctx); err != nil {
		return nil, err
	}
	if c.Rules, err = st.ListRules(ctx, 80); err != nil {
		return nil, err
	}
	if c.Habits, err = st.Habits(ctx, 120, 60); err != nil {
		return nil, err
	}
	c.accByID = map[int64]store.Account{}
	for _, a := range c.Accounts {
		c.accByID[a.ID] = a
	}
	c.catByID = map[int64]store.Category{}
	for _, x := range c.Categories {
		c.catByID[x.ID] = x
	}
	c.debtIDs = map[int64]bool{}
	for _, d := range c.Debts {
		c.debtIDs[d.ID] = true
	}
	return c, nil
}

func (c *Context) catName(id *int64) string {
	if id == nil {
		return "-"
	}
	if x, ok := c.catByID[*id]; ok {
		return x.Name
	}
	return "-"
}

func (c *Context) accName(id int64) string {
	if a, ok := c.accByID[id]; ok {
		return a.Name
	}
	return "-"
}

var accountTypeName = map[string]string{"bank": "rekening bank", "cash": "uang tunai", "ewallet": "dompet digital", "credit": "kartu kredit/paylater", "saving": "tabungan", "other": "lainnya"}

// Describe menuliskan data pengguna sebagai teks untuk system prompt.
func (c *Context) Describe() string {
	var b strings.Builder
	t := c.Today
	fmt.Fprintf(&b, "Hari ini: %s, %d %s %d (%s).\n", hari[t.Weekday()], t.Day(), period.MonthName(t.Month()), t.Year(), t.Format("2006-01-02"))
	fmt.Fprintf(&b, "Periode keuangan dimulai tanggal %d setiap bulan (tanggal gajian). Periode berjalan: %s = %s s/d %s (kunci %s). Label periode = bulan tempat periode berakhir.\n",
		c.Settings.PeriodStartDay, c.Period.Label, c.Period.StartStr(), c.Period.EndStr(), c.Period.Key)
	if c.Settings.OwnerName != "" {
		fmt.Fprintf(&b, "Nama pengguna: %s.\n", c.Settings.OwnerName)
	}

	b.WriteString("\n## Akun (kas/dompet)\n")
	for _, a := range c.Accounts {
		if a.Archived {
			continue
		}
		def := ""
		if a.ID == c.Settings.DefaultAccountID {
			def = " [AKUN DEFAULT]"
		}
		fmt.Fprintf(&b, "- id=%d %s (%s)%s; saldo %s; kata kunci: %s\n", a.ID, a.Name, accountTypeName[a.Type], def, money.Format(a.Balance), a.Aliases)
	}

	b.WriteString("\n## Kategori\n")
	group := ""
	for _, x := range c.Categories {
		if x.Archived {
			continue
		}
		if x.Group != group {
			group = x.Group
			fmt.Fprintf(&b, "### %s\n", group)
		}
		kw := ""
		if x.Keywords != "" {
			kw = "; kata kunci: " + x.Keywords
		}
		fmt.Fprintf(&b, "- id=%d %s (%s)%s\n", x.ID, x.Name, x.Kind, kw)
	}

	active := 0
	for _, d := range c.Debts {
		if !d.Archived {
			active++
		}
	}
	if active > 0 {
		b.WriteString("\n## Utang / cicilan aktif\n")
		for _, d := range c.Debts {
			if d.Archived {
				continue
			}
			fmt.Fprintf(&b, "- id=%d %s; sisa %s; cicilan/bulan %s; jatuh tempo tgl %d\n", d.ID, d.Name, money.Format(d.Remaining), money.Format(d.Monthly), d.DueDay)
		}
	}

	if len(c.Rules) > 0 {
		b.WriteString("\n## Aturan yang dipelajari dari koreksi pengguna (pola deskripsi → kategori, akun)\n")
		for _, r := range c.Rules {
			acc := "-"
			if r.AccountID != nil {
				acc = c.accName(*r.AccountID)
			}
			fmt.Fprintf(&b, "- \"%s\" → %s, %s, akun %s (%dx)\n", r.Pattern, r.Type, c.catName(r.CategoryID), acc, r.Hits)
		}
	}

	if len(c.Habits) > 0 {
		b.WriteString("\n## Kebiasaan transaksi 4 bulan terakhir (deskripsi | jenis | kategori | akun | frekuensi | nominal rata-rata | nominal terakhir)\n")
		for _, h := range c.Habits {
			acc := c.accName(h.AccountID)
			if h.ToAccountID != nil {
				acc += " → " + c.accName(*h.ToAccountID)
			}
			fmt.Fprintf(&b, "- %s | %s | %s | %s | %dx | %s | %s\n", h.Description, h.Type, c.catName(h.CategoryID), acc, h.Count, money.Format(h.AvgAmount), money.Format(h.LastAmount))
		}
	}
	return b.String()
}

// Sanitize memastikan draft merujuk ke data yang benar-benar ada dan
// mengisi nilai default yang masuk akal.
func (c *Context) Sanitize(d Draft, source string) Draft {
	d.Source = source
	d.Description = strings.TrimSpace(d.Description)
	if _, err := time.Parse("2006-01-02", d.Date); err != nil {
		d.Date = c.Today.Format("2006-01-02")
	}
	switch d.Type {
	case "expense", "income", "transfer":
	default:
		d.Type = "expense"
	}
	if d.Amount < 0 {
		d.Amount = -d.Amount
	}
	if d.Amount == 0 {
		d.AmountEstimated = true
	}
	if a, ok := c.accByID[d.AccountID]; !ok || a.Archived {
		d.AccountID = c.Settings.DefaultAccountID
		if _, ok := c.accByID[d.AccountID]; !ok && len(c.Accounts) > 0 {
			d.AccountID = c.Accounts[0].ID
		}
		d.Confidence = min(d.Confidence, 0.6)
	}
	if d.Type == "transfer" {
		d.CategoryID, d.DebtID = nil, nil
		if d.ToAccountID == nil || *d.ToAccountID == d.AccountID {
			d.ToAccountID = nil
		} else if _, ok := c.accByID[*d.ToAccountID]; !ok {
			d.ToAccountID = nil
		}
		if d.ToAccountID == nil {
			// transfer tanpa tujuan yang jelas → anggap pengeluaran
			d.Type = "expense"
			d.Confidence = min(d.Confidence, 0.5)
		}
	} else {
		d.ToAccountID = nil
	}
	if d.CategoryID != nil {
		x, ok := c.catByID[*d.CategoryID]
		if !ok || x.Kind != d.Type {
			d.CategoryID = nil
		}
	}
	if d.CategoryID == nil && d.Type != "transfer" {
		// pakai kategori "Lain-lain" / "Pemasukan Lain" sebagai cadangan
		for _, x := range c.Categories {
			if x.Kind == d.Type && !x.Archived && (x.Name == "Lain-lain" || x.Name == "Pemasukan Lain") {
				id := x.ID
				d.CategoryID = &id
				d.Confidence = min(d.Confidence, 0.5)
				break
			}
		}
	}
	if d.DebtID != nil && (!c.debtIDs[*d.DebtID] || d.Type != "expense") {
		d.DebtID = nil
	}
	if d.Description == "" {
		d.Description = c.catName(d.CategoryID)
		if d.Type == "transfer" && d.ToAccountID != nil {
			d.Description = "Transfer ke " + c.accName(*d.ToAccountID)
		}
	}
	if d.Confidence < 0 {
		d.Confidence = 0
	}
	if d.Confidence > 1 {
		d.Confidence = 1
	}
	return d
}
