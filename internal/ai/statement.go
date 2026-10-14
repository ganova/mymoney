package ai

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ganova/mymoney/internal/money"
	"github.com/ganova/mymoney/internal/period"
	"github.com/ganova/mymoney/internal/store"
)

// Pembaca mutasi offline: mengubah teks mutasi (hasil OCR screenshot atau teks
// yang ditempel) menjadi draf transaksi tanpa memanggil AI. Mengenali pola
// umum m-banking Indonesia (myBCA/KlikBCA, Livin', BRImo, e-wallet).

var (
	// nominal harus berpemisah ribuan/desimal atau berawalan Rp agar kode
	// referensi seperti "0505" atau "914" tidak dianggap uang
	stAmountRe   = regexp.MustCompile(`(?i)([+\-−–])?\s*(rp\.?\s*)?(\d{1,3}(?:[.,]\d{3})+(?:[.,]\d{1,2})?|\d+[.,]\d{2}\b|(?:rp\.?\s*)\d{3,})(\s*(?:DB|CR|D|K)\b)?`)
	stDateTextRe = regexp.MustCompile(`(?i)\b(\d{1,2})\s*[ \-/.]?\s*(jan|feb|mar|apr|mei|may|jun|jul|agu|agt|aug|sep|okt|oct|nov|des|dec)[a-z]*\.?(?:\s*[ \-/.]?\s*(\d{4}|\d{2}))?\b`)
	stDateNumRe  = regexp.MustCompile(`\b(\d{1,2})/(\d{1,2})(?:/(\d{2,4}))?\b`)
	stTglRe      = regexp.MustCompile(`(?i)tgl\s*:?\s*(\d{2})(\d{2})`)
	stNoiseRe    = regexp.MustCompile(`(?i)\b(transaksi debit|trsf e-banking|e-banking|bi-fast|db interchange|trn mastercard|dbt|tgl\s*:?\s*\d{4}|qrc?(?:\s*\d{2,3})?|\d{4}/[a-z0-9]+/[a-z0-9]+|\d{5,}(?:\.\d{2})?|db|cr|debit|kredit|berhasil|sukses|success)\b`)
	stSkipRe     = regexp.MustCompile(`(?i)\b(saldo|total|mutasi rekening|halaman|no\.? rekening|nomor rekening|periode|tanggal\s+keterangan|keterangan|mata uang|rekening tahapan)\b`)
	stCreditRe   = regexp.MustCompile(`(?i)\b(cr|kredit|masuk|diterima|terima|dari|gaji|refund|cashback|bunga)\b`)
	stDebitRe    = regexp.MustCompile(`(?i)\b(db|debit|keluar|bayar|pembayaran|pembelian|transfer ke|tarik|tarikan|top ?up|biaya)\b`)
	stLetterRe   = regexp.MustCompile(`[A-Za-z]`)
)

var bulanAngka = map[string]time.Month{
	"jan": 1, "feb": 2, "mar": 3, "apr": 4, "mei": 5, "may": 5, "jun": 6, "jul": 7,
	"agu": 8, "agt": 8, "aug": 8, "sep": 9, "okt": 10, "oct": 10, "nov": 11, "des": 12, "dec": 12,
}

// ParseStatementOffline membaca teks mutasi tanpa AI.
func ParseStatementOffline(ctx context.Context, st *store.Store, text string, accountID int64, periodKey, source string) (*ExtractResult, error) {
	c, err := LoadContext(ctx, st)
	if err != nil {
		return nil, err
	}
	p := c.Period
	if periodKey != "" {
		if q, err := period.Parse(periodKey, c.Settings.PeriodStartDay); err == nil {
			p = q
		}
	}
	if accountID == 0 {
		accountID = detectAccount(c, text)
	}
	res := &ExtractResult{Source: "Pembaca offline", AccountID: accountID, Drafts: []Draft{}}

	curDate := ""
	seen := map[string]bool{}
	emit := func(amountLine string, loc []int, context, after []string) {
		amount, ok := money.Parse(strings.TrimSpace(amountLine[loc[2]:loc[3]]))
		if !ok || amount <= 0 {
			return
		}
		rest := strings.TrimSpace(amountLine[:loc[0]] + " " + amountLine[loc[1]:])
		parts := append(append(append([]string{}, context...), rest), after...)
		full := strings.Join(append(append(append([]string{}, context...), amountLine), after...), " ")

		credit := false
		sign := strings.ToUpper(strings.TrimSpace(amountLine[loc[0]:loc[1]]))
		switch {
		case strings.HasSuffix(sign, "CR") || strings.HasSuffix(sign, " K") || strings.HasPrefix(sign, "+"):
			credit = true
		case strings.HasPrefix(sign, "-") || strings.HasPrefix(sign, "−") || strings.HasPrefix(sign, "–") || strings.HasSuffix(sign, "DB"):
			credit = false
		default:
			credit = stCreditRe.MatchString(full) && !stDebitRe.MatchString(full)
		}
		date := curDate
		if date == "" {
			date = c.Today.Format("2006-01-02")
		}
		d := Draft{
			Type: "expense", Amount: amount, Date: date, AccountID: accountID,
			Description: titleCase(cleanDesc(strings.Join(parts, " "))), Raw: full, Confidence: 0.5,
		}
		if credit {
			d.Type = "income"
		}
		classifyStatement(ctx, st, c, &d, full)
		d = c.Sanitize(d, source)
		key := fmt.Sprintf("%s|%s|%d|%s", d.Date, d.Type, d.Amount, strings.ToLower(d.Description))
		d.DuplicateInScan = seen[key]
		seen[key] = true
		d.DuplicateOf = st.FindDuplicate(ctx, d.Transaction())
		res.Drafts = append(res.Drafts, d)
	}

	// Baris dikelompokkan per blok (dipisah baris kosong). Hasil OCR screenshot
	// biasanya satu transaksi per blok dengan keterangan sebelum/sesudah nominal.
	blocks := regexp.MustCompile(`\n\s*\n`).Split(text, -1)
	if len(blocks) == 1 {
		// teks tanpa baris kosong: tiap baris berdiri sendiri
		blocks = strings.Split(text, "\n")
	}
	var pending []string
	for _, block := range blocks {
		var lines []string
		for _, raw := range strings.Split(block, "\n") {
			if l := strings.Join(strings.Fields(raw), " "); l != "" {
				lines = append(lines, l)
			}
		}
		// buang baris saldo/total/judul, tetapi pertahankan baris lain di blok
		kept := lines[:0]
		for _, l := range lines {
			if stSkipRe.MatchString(l) {
				pending = nil
				continue
			}
			kept = append(kept, l)
		}
		lines = kept
		type amt struct {
			line string
			loc  []int
			date string
		}
		var amts []amt
		var texts, afterTexts []string
		trailingDate, lastDate := "", ""
		for _, l := range lines {
			if d := findDate(l, c.Today, p); d != "" {
				// tanggal setelah nominal adalah judul grup berikutnya
				if len(amts) > 0 && isDateOnly(l) {
					trailingDate = d
					continue
				}
				curDate = d
				lastDate = d
				if isDateOnly(l) {
					continue
				}
			}
			if loc := pickAmount(l); loc != nil {
				amts = append(amts, amt{l, loc, curDate})
			} else if len(stLetterRe.FindAllString(l, -1)) >= 3 {
				if len(amts) > 0 {
					afterTexts = append(afterTexts, l)
				} else {
					texts = append(texts, l)
				}
			}
		}
		switch {
		case len(amts) == 0:
			pending = append(pending, append(texts, afterTexts...)...)
			if len(pending) > 4 {
				pending = pending[len(pending)-4:]
			}
		case len(amts) == 1:
			curDate = amts[0].date
			emit(amts[0].line, amts[0].loc, append(pending, texts...), afterTexts)
			pending = nil
		default:
			for _, a := range amts {
				ctxLines := pending
				if len(stLetterRe.FindAllString(cleanDesc(a.line[:a.loc[0]]+a.line[a.loc[1]:]), -1)) >= 3 {
					ctxLines = nil
				}
				curDate = a.date
				emit(a.line, a.loc, ctxLines, nil)
				pending = nil
			}
		}
		if trailingDate != "" {
			curDate = trailingDate
		} else if lastDate != "" {
			curDate = lastDate
		}
	}
	res.Notes = "Dibaca tanpa AI (gratis). Periksa nominal, tanggal, dan kategori sebelum menyimpan."
	if len(res.Drafts) == 0 {
		res.Notes = "Tidak ada transaksi yang terbaca. Pastikan screenshot tajam, tidak terpotong, dan nominal terlihat jelas."
	}
	return res, nil
}

// pickAmount memilih nominal transaksi di baris: utamakan yang bertanda DB/CR
// atau +/-, selain itu nominal terakhir.
func pickAmount(line string) []int {
	var marked, last []int
	for _, m := range stAmountRe.FindAllStringSubmatchIndex(line, -1) {
		loc := []int{m[0], m[1], m[6], m[7]}
		if m[2] >= 0 || m[8] >= 0 {
			marked = loc
		}
		last = loc
	}
	if marked != nil {
		return marked
	}
	return last
}

func findDate(line string, today time.Time, p period.Period) string {
	mk := func(day int, mon time.Month, year int) string {
		if day < 1 || day > 31 || mon < 1 || mon > 12 {
			return ""
		}
		if year == 0 {
			year = today.Year()
			if time.Date(year, mon, day, 0, 0, 0, 0, time.UTC).After(today.AddDate(0, 0, 1)) {
				year--
			}
		} else if year < 100 {
			year += 2000
		}
		return time.Date(year, mon, day, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
	}
	low := strings.ToLower(line)
	if strings.Contains(low, "hari ini") {
		return today.Format("2006-01-02")
	}
	if strings.Contains(low, "kemarin") {
		return today.AddDate(0, 0, -1).Format("2006-01-02")
	}
	if m := stDateTextRe.FindStringSubmatch(line); m != nil {
		day, _ := strconv.Atoi(m[1])
		year, _ := strconv.Atoi(m[3])
		return mk(day, bulanAngka[strings.ToLower(m[2])[:3]], year)
	}
	if m := stDateNumRe.FindStringSubmatch(line); m != nil {
		day, _ := strconv.Atoi(m[1])
		mon, _ := strconv.Atoi(m[2])
		year, _ := strconv.Atoi(m[3])
		return mk(day, time.Month(mon), year)
	}
	if m := stTglRe.FindStringSubmatch(line); m != nil {
		day, _ := strconv.Atoi(m[1])
		mon, _ := strconv.Atoi(m[2])
		return mk(day, time.Month(mon), 0)
	}
	_ = p
	return ""
}

func isDateOnly(line string) bool {
	rest := stDateTextRe.ReplaceAllString(line, "")
	rest = stDateNumRe.ReplaceAllString(rest, "")
	rest = regexp.MustCompile(`(?i)\b(senin|selasa|rabu|kamis|jumat|sabtu|minggu|mon|tue|wed|thu|fri|sat|sun)\w*\b|[,\s]`).ReplaceAllString(rest, "")
	return len(rest) <= 2
}

var digitLetter = regexp.MustCompile(`(\d)([A-Za-z])`)

func cleanDesc(s string) string {
	s = digitLetter.ReplaceAllString(s, "$1 $2")
	s = stDateTextRe.ReplaceAllString(s, " ")
	s = stDateNumRe.ReplaceAllString(s, " ")
	s = stNoiseRe.ReplaceAllString(s, " ")
	s = regexp.MustCompile(`(?i)rp\.?|\b\d+\b|[|_*•>]`).ReplaceAllString(s, " ")
	s = strings.Join(strings.Fields(s), " ")
	return strings.Trim(s, " -:.,/")
}

func titleCase(s string) string {
	if s == "" {
		return s
	}
	words := strings.Fields(strings.ToLower(s))
	for i, w := range words {
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	out := strings.Join(words, " ")
	if len(out) > 60 {
		out = out[:60]
	}
	return out
}

// detectAccount menebak akun dari nama aplikasi/bank yang muncul di teks.
func detectAccount(c *Context, text string) int64 {
	low := strings.ToLower(text)
	// rekening bank diperiksa lebih dulu: mutasi bank sering menyebut nama
	// e-wallet (mis. "GOPAY TOPUP") yang bukan sumber mutasinya
	ordered := append([]store.Account{}, c.Accounts...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Type == "bank" && ordered[j].Type != "bank" })
	if strings.Contains(low, "e-banking") || strings.Contains(low, "bi-fast") || strings.Contains(low, "tahapan") || strings.Contains(low, "transaksi debit") {
		for _, a := range ordered {
			if !a.Archived && a.Type == "bank" {
				return a.ID
			}
		}
	}
	for _, a := range ordered {
		if a.Archived {
			continue
		}
		for _, n := range append([]string{strings.ToLower(a.Name)}, strings.Split(a.Aliases, ",")...) {
			n = strings.TrimSpace(n)
			if len(n) >= 3 && n != "transfer" && n != "debit" && containsWord(low, n) {
				return a.ID
			}
		}
	}
	return c.Settings.DefaultAccountID
}

// classifyStatement mengisi kategori, dan mengubah top-up/tarik tunai menjadi transfer.
func classifyStatement(ctx context.Context, st *store.Store, c *Context, d *Draft, text string) {
	low := strings.ToLower(text + " " + d.Description)
	if d.Type == "expense" {
		if strings.Contains(low, "tarik tunai") || strings.Contains(low, "tarikan atm") || strings.Contains(low, "tarik atm") {
			for _, a := range c.Accounts {
				if !a.Archived && a.Type == "cash" && a.ID != d.AccountID {
					id := a.ID
					d.Type, d.ToAccountID, d.Description, d.Confidence = "transfer", &id, "Tarik tunai", 0.75
					return
				}
			}
		}
		if strings.Contains(low, "top up") || strings.Contains(low, "topup") || strings.Contains(low, "isi saldo") {
			for _, a := range c.Accounts {
				if a.Archived || a.Type != "ewallet" || a.ID == d.AccountID {
					continue
				}
				for _, n := range append([]string{strings.ToLower(a.Name)}, strings.Split(a.Aliases, ",")...) {
					if n = strings.TrimSpace(n); len(n) >= 3 && strings.Contains(low, n) {
						id := a.ID
						d.Type, d.ToAccountID, d.Description, d.Confidence = "transfer", &id, "Top up "+a.Name, 0.75
						return
					}
				}
			}
		}
	}
	if r := st.MatchRule(ctx, d.Description+" "+text); r != nil && (r.Type == d.Type || r.Type == "") && r.CategoryID != nil {
		d.CategoryID, d.Confidence = r.CategoryID, 0.75
		return
	}
	bestLen := 0
	for _, cat := range c.Categories {
		if cat.Archived || cat.Kind != d.Type {
			continue
		}
		for _, kw := range append([]string{strings.ToLower(cat.Name)}, strings.Split(cat.Keywords, ",")...) {
			kw = strings.TrimSpace(kw)
			if len(kw) >= 3 && len(kw) > bestLen && containsWord(low, kw) {
				id := cat.ID
				d.CategoryID, bestLen = &id, len(kw)
			}
		}
	}
	if d.CategoryID != nil {
		d.Confidence = 0.65
	}
	for _, db := range c.Debts {
		if !db.Archived && d.Type == "expense" && containsWord(low, strings.ToLower(db.Name)) {
			id := db.ID
			d.DebtID = &id
		}
	}
}
