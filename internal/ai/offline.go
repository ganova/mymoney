package ai

import (
	"context"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/ganova/mymoney/internal/money"
	"github.com/ganova/mymoney/internal/store"
)

// Parser offline dipakai bila API key belum diisi atau AI sedang tidak bisa
// dihubungi. Ia memahami pola umum seperti "makan siang 25rb cash",
// "tarik tunai 300rb", "gajian 11jt", memakai kata kunci kategori, alias akun,
// dan aturan yang dipelajari dari kebiasaan pengguna.

var segmentSplit = regexp.MustCompile(`(?i)\n|;|,\s+|\s+dan\s+|\s+terus\s+|\s+lalu\s+|\s+\+\s+`)
var daysAgo = regexp.MustCompile(`(?i)(\d+)\s*hari\s*(?:yang\s*)?(?:lalu|kemarin)`)
var fillerLead = regexp.MustCompile(`(?i)^(beli|membeli|buat|untuk|utk|abis|habis|tadi|barusan|catat|tolong catat)\s+`)
var viaAccount = regexp.MustCompile(`(?i)\s*\b(pakai|pake|pk|via|lewat|dari|dengan|dgn|ke)\b\s*$`)
var dateWords = regexp.MustCompile(`(?i)\b(kemarin|kmrn|kemaren|hari ini|tadi pagi|tadi siang|tadi malam|tadi sore|tadi|barusan)\b`)
var slangAmount = map[string]int64{"goceng": 5000, "ceban": 10000, "gocap": 50000, "sejuta": 1000000, "seratus ribu": 100000, "lima puluh ribu": 50000, "dua puluh ribu": 20000, "sepuluh ribu": 10000}

var incomeWords = []string{"gaji", "gajian", "terima", "diterima", "dapat", "dapet", "masuk", "bonus", "thr", "cashback", "refund", "dibayar", "jual", "hasil"}

func containsWord(hay, needle string) bool {
	if needle == "" {
		return false
	}
	idx := 0
	for {
		i := strings.Index(hay[idx:], needle)
		if i < 0 {
			return false
		}
		i += idx
		before := i == 0 || !isWordRune(rune(hay[i-1]))
		after := i+len(needle) >= len(hay) || !isWordRune(rune(hay[i+len(needle)]))
		if before && after {
			return true
		}
		idx = i + 1
	}
}

func isWordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }

type accMention struct {
	id  int64
	pos int
	typ string
	hit string
}

// ParseOffline mengubah kalimat bebas menjadi draf transaksi tanpa AI.
func ParseOffline(ctx context.Context, st *store.Store, text, source string) ([]Draft, error) {
	c, err := LoadContext(ctx, st)
	if err != nil {
		return nil, err
	}
	var out []Draft
	for _, seg := range segmentSplit.Split(text, -1) {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}
		if d, ok := parseSegment(ctx, st, c, seg); ok {
			out = append(out, c.Sanitize(d, source))
		}
	}
	return out, nil
}

func parseSegment(ctx context.Context, st *store.Store, c *Context, seg string) (Draft, bool) {
	lower := strings.ToLower(seg)
	d := Draft{Confidence: 0.55, Date: c.Today.Format("2006-01-02")}

	// tanggal
	if m := daysAgo.FindStringSubmatch(lower); m != nil {
		n, _ := strconv.Atoi(m[1])
		d.Date = c.Today.AddDate(0, 0, -n).Format("2006-01-02")
		lower = strings.Replace(lower, m[0], " ", 1)
	} else if containsWord(lower, "kemarin") || containsWord(lower, "kmrn") || containsWord(lower, "kemaren") {
		d.Date = c.Today.AddDate(0, 0, -1).Format("2006-01-02")
	}

	// nominal
	desc := lower
	matches := money.Find(lower)
	if len(matches) > 0 {
		best := matches[0]
		for _, m := range matches[1:] {
			if (m.Unit != "" && best.Unit == "") || (m.Unit == best.Unit && m.Value > best.Value) {
				best = m
			}
		}
		d.Amount = best.Value
		if best.Unit == "" && best.Value < 1000 {
			d.Amount = best.Value * 1000
		}
		desc = lower[:best.Start] + " " + lower[best.End:]
	} else {
		for w, v := range slangAmount {
			if strings.Contains(lower, w) {
				d.Amount = v
				desc = strings.Replace(desc, w, " ", 1)
				break
			}
		}
	}

	// akun yang disebut
	var mentions []accMention
	for _, a := range c.Accounts {
		if a.Archived {
			continue
		}
		names := append([]string{strings.ToLower(a.Name)}, strings.Split(a.Aliases, ",")...)
		for _, n := range names {
			n = strings.TrimSpace(n)
			if len(n) < 2 {
				continue
			}
			// alias generik bank tidak dianggap penyebutan akun eksplisit
			if n == "transfer" || n == "tf" {
				continue
			}
			if containsWord(lower, n) {
				mentions = append(mentions, accMention{a.ID, strings.Index(lower, n), a.Type, n})
				break
			}
		}
	}
	sort.Slice(mentions, func(i, j int) bool { return mentions[i].pos < mentions[j].pos })
	for _, m := range mentions {
		desc = strings.Replace(desc, m.hit, " ", 1)
	}

	firstOfType := func(typ string) int64 {
		for _, a := range c.Accounts {
			if !a.Archived && a.Type == typ {
				return a.ID
			}
		}
		return 0
	}
	defaultBank := c.Settings.DefaultAccountID
	cash := firstOfType("cash")

	// jenis transaksi
	d.Type = "expense"
	switch {
	case strings.Contains(lower, "tarik tunai") || strings.Contains(lower, "ambil uang") || strings.Contains(lower, "tarik atm") || strings.Contains(lower, "ambil atm"):
		d.Type = "transfer"
		d.AccountID = defaultBank
		for _, m := range mentions {
			if m.typ != "cash" {
				d.AccountID = m.id
				break
			}
		}
		if cash != 0 {
			d.ToAccountID = &cash
		}
		d.Description = "Tarik tunai"
	case strings.Contains(lower, "setor tunai"):
		d.Type = "transfer"
		d.AccountID = cash
		to := defaultBank
		for _, m := range mentions {
			if m.typ != "cash" {
				to = m.id
				break
			}
		}
		d.ToAccountID = &to
		d.Description = "Setor tunai"
	case strings.Contains(lower, "top up") || strings.Contains(lower, "topup") || strings.Contains(lower, "isi saldo") || strings.HasPrefix(lower, "isi "):
		var target int64
		for _, m := range mentions {
			if m.typ == "ewallet" {
				target = m.id
				break
			}
		}
		if target != 0 {
			d.Type = "transfer"
			d.AccountID = defaultBank
			for _, m := range mentions {
				if m.id != target {
					d.AccountID = m.id
					break
				}
			}
			d.ToAccountID = &target
			d.Description = "Top up " + c.accName(target)
		}
	default:
		for _, w := range incomeWords {
			if containsWord(lower, w) {
				d.Type = "income"
				break
			}
		}
	}
	if d.Type != "transfer" && len(mentions) > 0 {
		d.AccountID = mentions[0].id
	}

	// deskripsi bersih
	if d.Description == "" {
		desc = dateWords.ReplaceAllString(desc, " ")
		desc = strings.ReplaceAll(desc, "rp", " ")
		desc = strings.Join(strings.Fields(desc), " ")
		for {
			n := fillerLead.ReplaceAllString(desc, "")
			n = viaAccount.ReplaceAllString(n, "")
			n = strings.TrimSpace(n)
			if n == desc {
				break
			}
			desc = n
		}
		desc = strings.Trim(desc, " -:.")
		if desc != "" {
			d.Description = strings.ToUpper(desc[:1]) + desc[1:]
		}
	}

	// aturan yang dipelajari
	if d.Type != "transfer" {
		if r := st.MatchRule(ctx, seg); r != nil && (r.Type == d.Type || r.Type == "") {
			d.CategoryID = r.CategoryID
			if len(mentions) == 0 && r.AccountID != nil {
				d.AccountID = *r.AccountID
			}
			d.Confidence = 0.8
		}
	}

	// kata kunci kategori
	if d.Type != "transfer" && d.CategoryID == nil {
		bestLen := 0
		for _, cat := range c.Categories {
			if cat.Archived || cat.Kind != d.Type {
				continue
			}
			for _, kw := range append([]string{strings.ToLower(cat.Name)}, strings.Split(cat.Keywords, ",")...) {
				kw = strings.TrimSpace(kw)
				if len(kw) > bestLen && containsWord(lower, kw) {
					id := cat.ID
					d.CategoryID = &id
					bestLen = len(kw)
				}
			}
		}
		if d.CategoryID != nil {
			d.Confidence = max(d.Confidence, 0.65)
		}
	}

	// perkiraan nominal dari kebiasaan
	if d.Amount == 0 {
		d.AmountEstimated = true
		key := store.NormalizePattern(d.Description)
		for _, h := range c.Habits {
			if key != "" && store.NormalizePattern(h.Description) == key {
				d.Amount = h.LastAmount
				if d.CategoryID == nil {
					d.CategoryID = h.CategoryID
				}
				break
			}
		}
		d.Confidence = min(d.Confidence, 0.4)
	}
	if d.AccountID == 0 {
		d.AccountID = defaultBank
	}
	if d.Description == "" && d.Amount == 0 {
		return d, false
	}
	return d, true
}
