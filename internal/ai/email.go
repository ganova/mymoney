package ai

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/ganova/mymoney/internal/money"
	"github.com/ganova/mymoney/internal/store"
)

var (
	// nominal bermata uang: "IDR 25,000.00", "Rp 25.000", "Rp. 1.500.000,00"
	emailAmount = regexp.MustCompile(`(?i)(?:idr|rp\.?)\s*:?\s*([0-9][0-9.,]*)`)
	// kata yang menandai nominal transaksi (bukan saldo/biaya)
	emailAmountKey = regexp.MustCompile(`(?i)(nominal|jumlah|total|amount|sebesar|nilai|transaksi)`)
	emailNotAmount = regexp.MustCompile(`(?i)(saldo|balance|biaya|fee|limit)`)
	// label penerima/merchant diikuti nilainya (boleh di baris berikutnya)
	emailField      = regexp.MustCompile(`(?im)^\s*(nama merchant|merchant|nama toko|toko|penerima|nama penerima|tujuan|rekening tujuan|keterangan|berita|keperluan|pembayaran|deskripsi|description|remark)\s*[:：]?\s*(?:\n\s*[:：]?\s*)?([^\n:：]{2,80})$`)
	emailIncome     = regexp.MustCompile(`(?i)(dana masuk|transfer masuk|uang masuk|kredit|credit|diterima|setoran|incoming|menerima)`)
	emailOutgoing   = regexp.MustCompile(`(?i)(debit|pembayaran|pembelian|payment|purchase|transfer keluar|tarik tunai|penarikan|qris)`)
	emailDetailDrop = regexp.MustCompile(`(?i)^(nomor|no\.?|tanggal|jam|waktu|status|referensi|ref)\b`)
)

// ParseEmailOffline membaca satu email notifikasi transaksi bank tanpa AI.
// Nominal diambil dari pola mata uang (IDR/Rp) yang paling mungkin nominal
// transaksi, deskripsi dari kolom merchant/penerima/keterangan, tanggal dari
// waktu email. Kategori dan jenis (mis. top up = transfer) ditebak lewat
// ParseOffline, sehingga aturan dan kebiasaan pengguna ikut dipakai.
func ParseEmailOffline(ctx context.Context, st *store.Store, subject, text string, date time.Time, accountID int64, source string) ([]Draft, error) {
	body := subject + "\n" + text
	amount := pickEmailAmount(body)
	if amount <= 0 {
		return nil, nil
	}
	desc := pickEmailField(text)
	if desc == "" {
		desc = strings.TrimSpace(subject)
	}
	desc = strings.Trim(desc, " -.:")
	if len(desc) > 80 {
		desc = desc[:80]
	}
	income := emailIncome.MatchString(body) && !emailOutgoing.MatchString(subject)

	c, err := LoadContext(ctx, st)
	if err != nil {
		return nil, err
	}
	if desc == strings.ToUpper(desc) {
		desc = titleCase(desc) // "INDOMARET PUSAT" → "Indomaret Pusat"
	}
	d := Draft{Type: "expense", Amount: amount, Description: desc, AccountID: accountID, Confidence: 0.6}
	if income {
		d.Type = "income"
	}
	// tebak kategori/jenis lewat parser kalimat (aturan, kata kunci, kebiasaan)
	if guess, ok := parseSegment(ctx, st, c, fmt.Sprintf("%s %d", strings.ToLower(desc), amount)); ok {
		if guess.Type == "transfer" && !income {
			d.Type, d.ToAccountID = "transfer", guess.ToAccountID
		} else if guess.Type == d.Type {
			d.CategoryID = guess.CategoryID
			d.IncomeSourceID = guess.IncomeSourceID
			d.Confidence = max(d.Confidence, guess.Confidence)
		}
	}
	if !date.IsZero() {
		d.Date = date.In(st.Loc).Format("2006-01-02")
	}
	d.Raw = strings.Join(strings.Fields(body), " ")
	if len(d.Raw) > 300 {
		d.Raw = d.Raw[:300]
	}
	d = c.Sanitize(d, source)
	if accountID != 0 && d.Type != "transfer" {
		d.AccountID = accountID
	}
	return []Draft{d}, nil
}

// pickEmailAmount memilih nominal mata uang yang konteksnya paling mirip
// nominal transaksi; nominal yang diawali kata saldo/biaya dihindari.
func pickEmailAmount(s string) int64 {
	var best int64
	bestScore := -1
	for _, loc := range emailAmount.FindAllStringSubmatchIndex(s, -1) {
		v, ok := money.Parse(s[loc[2]:loc[3]])
		if !ok || v <= 0 {
			continue
		}
		before := s[max(0, loc[0]-40):loc[0]]
		score := 1
		if emailAmountKey.MatchString(before) {
			score = 2
		}
		if emailNotAmount.MatchString(before) {
			score = 0
		}
		if score > bestScore {
			best, bestScore = v, score
		}
	}
	return best
}

func pickEmailField(s string) string {
	for _, m := range emailField.FindAllStringSubmatch(s, -1) {
		v := strings.TrimSpace(m[2])
		if v == "" || v == "-" || emailDetailDrop.MatchString(v) || len(money.Find(v)) > 0 && emailAmount.MatchString(v) {
			continue
		}
		return v
	}
	return ""
}
