package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/ganova/mymoney/internal/money"
	"github.com/ganova/mymoney/internal/store"
)

const assistantPrompt = `Kamu adalah asisten keuangan pribadi di aplikasi MyMoney milik satu orang pengguna di Indonesia. Pengguna kebanyakan mengetik singkat dari iPhone, sering tanpa tanda baca, campur bahasa gaul.

Ada dua jenis permintaan:
1. MENCATAT transaksi (mis. "beli makan siang", "bensin 50rb cash", "tarik tunai 300rb", "gajian masuk 11,2jt", "kemarin topup gopay 100k"). Untuk ini, panggil tool catat_transaksi tepat satu kali berisi semua transaksi di pesan itu. Jangan bertanya balik — tebak dengan cerdas; pengguna akan mengonfirmasi atau mengedit draf sebelum disimpan.
2. BERTANYA tentang keuangannya (mis. "sisa budget makan berapa?", "pengeluaran terbesar bulan ini apa?", "bandingkan dengan bulan lalu"). Untuk ini gunakan tool data (ringkasan_periode, cari_transaksi, saldo_dan_utang) lalu jawab. Jangan pernah mengarang angka; semua angka harus berasal dari hasil tool atau konteks.

Aturan mencatat:
- type: "expense" untuk uang keluar, "income" untuk uang masuk, "transfer" untuk perpindahan antar akun milik pengguna sendiri. Contoh transfer: tarik tunai (bank → Tunai), top up GoPay/ShopeePay/DANA/Flazz (bank → e-wallet), setor tunai (Tunai → bank), bayar tagihan kartu kredit (bank → akun kredit).
- Nominal: pahami "25rb", "25 ribu", "25k", "1,5jt", "1.5 juta", "11.217.866", "dua puluh lima ribu", "goceng" (5.000), "ceban" (10.000), "gocap" (50.000), "seratus ribu", "sejuta". Angka tanpa satuan di bawah 1.000 untuk belanja berarti ribuan (mis. "parkir 5" = 5.000, "makan 25" = 25.000).
- Bila nominal tidak disebut, perkirakan dari bagian "Kebiasaan transaksi" untuk deskripsi serupa (pakai nominal terakhir atau rata-rata) dan set amount_estimated=true. Bila tidak ada acuan sama sekali, isi amount=0 dan amount_estimated=true.
- Tanggal: default hari ini. Pahami "kemarin", "tadi", "semalam" (kemarin bila diucapkan pagi hari tidak jelas → pakai kemarin), "senin kemarin", "tgl 5", "2 hari lalu". Format YYYY-MM-DD, tidak boleh di masa depan kecuali pengguna jelas menyebut tanggal depan.
- Akun: bila disebut (cash/tunai, gopay, shopeepay, dana, flazz, qris, debit, transfer, bca, dsb.) cocokkan dengan kata kunci akun. "qris", "debit", "transfer", "tf" tanpa nama akun → akun bank default. Bila tidak disebut, ikuti aturan/kebiasaan untuk deskripsi serupa; bila tetap tidak ada, pakai [AKUN DEFAULT].
- Kategori: pilih id kategori yang paling cocok sesuai jenisnya (expense/income). Utamakan "Aturan yang dipelajari" bila polanya cocok, lalu kebiasaan, lalu kata kunci kategori.
- Pembayaran cicilan/paylater yang tercantum di "Utang / cicilan aktif" → type expense, kategori cicilan, isi debt_id.
- description: singkat dan rapi dalam bahasa pengguna, huruf awal kapital, tanpa nominal dan tanpa nama akun (mis. "Makan siang", "Bensin motor", "Tarik tunai", "Gaji").
- confidence: seberapa yakin (0..1). Turunkan bila menebak kategori/akun/nominal.
- message: konfirmasi singkat dan ramah (1 kalimat), mis. "Siap, makan siang Rp25.000 dari Tunai 👍". Sebut bila nominal hanya perkiraan.

Aturan menjawab pertanyaan:
- Format uang "Rp25.000" (titik pemisah ribuan). Jawab ringkas, hangat, to the point, dalam bahasa Indonesia santai. Boleh pakai poin dan emoji secukupnya.
- Bila relevan, beri satu saran praktis yang konkret (mis. sisa jatah harian, kategori yang perlu direm).
- Jangan menyebut id internal, nama tool, atau istilah teknis kepada pengguna.
- Bila pertanyaan di luar keuangan pribadi, jawab singkat lalu arahkan kembali.`

// HistoryItem adalah satu giliran percakapan sebelumnya (teks saja).
type HistoryItem struct {
	Role string `json:"role"` // user | assistant
	Text string `json:"text"`
}

// AssistantResult adalah hasil satu giliran asisten.
type AssistantResult struct {
	Reply  string  `json:"reply"`
	Drafts []Draft `json:"drafts"`
}

func tool(name, desc string, props map[string]any, required []string) anthropic.BetaToolUnionParam {
	if required == nil {
		required = []string{}
	}
	t := anthropic.BetaToolParam{
		Name:        name,
		Description: anthropic.String(desc),
		Strict:      anthropic.Bool(true),
		InputSchema: anthropic.BetaToolInputSchemaParam{
			Properties:  props,
			Required:    required,
			ExtraFields: map[string]any{"additionalProperties": false},
		},
	}
	return anthropic.BetaToolUnionParam{OfTool: &t}
}

func assistantTools() []anthropic.BetaToolUnionParam {
	return []anthropic.BetaToolUnionParam{
		tool("catat_transaksi",
			"Ajukan satu atau lebih transaksi untuk dicatat dari pesan pengguna. Hasilnya ditampilkan sebagai draf yang dikonfirmasi pengguna.",
			map[string]any{
				"transactions": map[string]any{"type": "array", "items": draftSchema(false)},
				"message":      map[string]any{"type": "string", "description": "Konfirmasi singkat untuk pengguna"},
			}, []string{"transactions", "message"}),
		tool("ringkasan_periode",
			"Ringkasan satu periode keuangan: total pemasukan/pengeluaran, pengeluaran per kategori, budget vs realisasi, tagihan, jatah harian, serta perbandingan dengan periode sebelumnya.",
			map[string]any{
				"period": map[string]any{"type": "string", "description": "Kunci periode YYYY-MM (bulan akhir periode). String kosong = periode berjalan."},
			}, []string{"period"}),
		tool("cari_transaksi",
			"Cari daftar transaksi dengan filter. Gunakan untuk pertanyaan detail (kapan terakhir bayar X, total belanja di merchant Y, transaksi terbesar, dsb.).",
			map[string]any{
				"from":        map[string]any{"type": "string", "description": "Tanggal awal YYYY-MM-DD, kosong = tanpa batas"},
				"to":          map[string]any{"type": "string", "description": "Tanggal akhir YYYY-MM-DD, kosong = tanpa batas"},
				"type":        map[string]any{"type": "string", "enum": []string{"", "expense", "income", "transfer"}},
				"category_id": nullableInt("Filter kategori"),
				"account_id":  nullableInt("Filter akun"),
				"search":      map[string]any{"type": "string", "description": "Kata pencarian di deskripsi/catatan, kosong = semua"},
				"limit":       map[string]any{"type": "integer", "description": "Jumlah maksimal baris (1-200)"},
			}, []string{"from", "to", "type", "category_id", "account_id", "search", "limit"}),
		tool("saldo_dan_utang",
			"Saldo terkini setiap akun beserta total kekayaan bersih, serta daftar utang/cicilan dan sisanya.",
			map[string]any{}, nil),
	}
}

func (s *Service) systemBlocks(c *Context, extra string) []anthropic.BetaTextBlockParam {
	return []anthropic.BetaTextBlockParam{
		{Text: extra},
		{Text: "# Data pengguna\n" + c.Describe(), CacheControl: anthropic.NewBetaCacheControlEphemeralParam()},
	}
}

// Ask menjalankan satu giliran asisten: mencatat transaksi atau menjawab pertanyaan.
func (s *Service) Ask(ctx context.Context, history []HistoryItem, text, source string) (*AssistantResult, error) {
	if !s.apiReady() {
		return s.codexAsk(ctx, history, text, source)
	}
	c, err := LoadContext(ctx, s.store)
	if err != nil {
		return nil, err
	}
	var messages []anthropic.BetaMessageParam
	if len(history) > 12 {
		history = history[len(history)-12:]
	}
	for _, h := range history {
		t := strings.TrimSpace(h.Text)
		if t == "" {
			continue
		}
		if h.Role == "assistant" {
			if len(messages) == 0 {
				continue // percakapan harus dimulai oleh pengguna
			}
			messages = append(messages, anthropic.BetaMessageParam{
				Role:    anthropic.BetaMessageParamRoleAssistant,
				Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock(t)},
			})
		} else {
			messages = append(messages, userText(t))
		}
	}
	messages = append(messages, userText(text))
	messages = mergeSameRole(messages)

	res := &AssistantResult{Drafts: []Draft{}}
	var reply strings.Builder
	for i := 0; i < 6; i++ {
		p := s.baseParams(s.cfg.Effort, 16000)
		p.System = s.systemBlocks(c, assistantPrompt)
		p.Tools = assistantTools()
		p.Messages = messages
		msg, err := s.send(ctx, "assistant", p)
		if err != nil {
			return nil, err
		}
		messages = append(messages, msg.ToParam())
		var results []anthropic.BetaContentBlockParamUnion
		recorded := false
		for _, block := range msg.Content {
			switch b := block.AsAny().(type) {
			case anthropic.BetaTextBlock:
				if t := strings.TrimSpace(b.Text); t != "" {
					if reply.Len() > 0 {
						reply.WriteString("\n\n")
					}
					reply.WriteString(t)
				}
			case anthropic.BetaToolUseBlock:
				raw := b.JSON.Input.Raw()
				if b.Name == "catat_transaksi" {
					var in struct {
						Transactions []Draft `json:"transactions"`
						Message      string  `json:"message"`
					}
					if err := json.Unmarshal([]byte(raw), &in); err != nil {
						return nil, fmt.Errorf("format draf AI tidak terbaca: %w", err)
					}
					for _, d := range in.Transactions {
						res.Drafts = append(res.Drafts, c.Sanitize(d, source))
					}
					if m := strings.TrimSpace(in.Message); m != "" {
						if reply.Len() > 0 {
							reply.WriteString("\n\n")
						}
						reply.WriteString(m)
					}
					recorded = true
					continue
				}
				out, isErr := s.runTool(ctx, c, b.Name, raw)
				results = append(results, anthropic.NewBetaToolResultBlock(b.ID, out, isErr))
			}
		}
		if recorded || msg.StopReason != anthropic.BetaStopReasonToolUse || len(results) == 0 {
			break
		}
		messages = append(messages, anthropic.NewBetaUserMessage(results...))
	}
	res.Reply = reply.String()
	if res.Reply == "" && len(res.Drafts) == 0 {
		res.Reply = "Maaf, aku belum paham. Coba tulis seperti \"makan siang 25rb\" atau tanyakan \"sisa budget bulan ini?\""
	}
	return res, nil
}

// mergeSameRole menggabungkan pesan berurutan dengan peran yang sama.
func mergeSameRole(in []anthropic.BetaMessageParam) []anthropic.BetaMessageParam {
	var out []anthropic.BetaMessageParam
	for _, m := range in {
		if n := len(out); n > 0 && out[n-1].Role == m.Role {
			out[n-1].Content = append(out[n-1].Content, m.Content...)
			continue
		}
		out = append(out, m)
	}
	return out
}

// runTool menjalankan tool data dan mengembalikan hasil berupa teks.
func (s *Service) runTool(ctx context.Context, c *Context, name, raw string) (string, bool) {
	switch name {
	case "ringkasan_periode":
		var in struct {
			Period string `json:"period"`
		}
		json.Unmarshal([]byte(raw), &in)
		p, err := s.store.ResolvePeriod(ctx, strings.TrimSpace(in.Period))
		if err != nil {
			return err.Error(), true
		}
		d, err := s.store.Dashboard(ctx, p)
		if err != nil {
			return err.Error(), true
		}
		return SummarizeDashboard(c, d), false
	case "cari_transaksi":
		var in struct {
			From       string `json:"from"`
			To         string `json:"to"`
			Type       string `json:"type"`
			CategoryID *int64 `json:"category_id"`
			AccountID  *int64 `json:"account_id"`
			Search     string `json:"search"`
			Limit      int    `json:"limit"`
		}
		if err := json.Unmarshal([]byte(raw), &in); err != nil {
			return "input tidak valid", true
		}
		f := store.TxFilter{From: in.From, To: in.To, Type: in.Type, Q: in.Search, Limit: in.Limit}
		if in.CategoryID != nil {
			f.CategoryID = *in.CategoryID
		}
		if in.AccountID != nil {
			f.AccountID = *in.AccountID
		}
		if f.Limit <= 0 || f.Limit > 200 {
			f.Limit = 50
		}
		list, err := s.store.ListTransactions(ctx, f)
		if err != nil {
			return err.Error(), true
		}
		if len(list) == 0 {
			return "Tidak ada transaksi yang cocok.", false
		}
		var b strings.Builder
		var total int64
		fmt.Fprintf(&b, "%d transaksi (terbaru dahulu): tanggal | jenis | nominal | deskripsi | kategori | akun\n", len(list))
		for _, t := range list {
			acc := c.accName(t.AccountID)
			if t.ToAccountID != nil {
				acc += " → " + c.accName(*t.ToAccountID)
			}
			fmt.Fprintf(&b, "%s | %s | %s | %s | %s | %s\n", t.Date, t.Type, money.Format(t.Amount), t.Description, c.catName(t.CategoryID), acc)
			if t.Type == "expense" {
				total += t.Amount
			}
		}
		fmt.Fprintf(&b, "Total pengeluaran dari daftar ini: %s", money.Format(total))
		return b.String(), false
	case "saldo_dan_utang":
		accs, err := s.store.ListAccounts(ctx)
		if err != nil {
			return err.Error(), true
		}
		debts, _ := s.store.ListDebts(ctx)
		var b strings.Builder
		var net int64
		b.WriteString("Saldo akun:\n")
		for _, a := range accs {
			if a.Archived {
				continue
			}
			net += a.Balance
			fmt.Fprintf(&b, "- %s (%s): %s\n", a.Name, accountTypeName[a.Type], money.Format(a.Balance))
		}
		var owed int64
		if len(debts) > 0 {
			b.WriteString("Utang/cicilan:\n")
			for _, d := range debts {
				if d.Archived {
					continue
				}
				owed += d.Remaining
				fmt.Fprintf(&b, "- %s: sisa %s dari %s, sudah dibayar %s, cicilan/bulan %s, jatuh tempo tgl %d, terakhir bayar %s\n",
					d.Name, money.Format(d.Remaining), money.Format(d.Total), money.Format(d.Paid), money.Format(d.Monthly), d.DueDay, d.LastPaid)
			}
		}
		fmt.Fprintf(&b, "Total saldo semua akun: %s. Total sisa utang: %s. Kekayaan bersih: %s.", money.Format(net), money.Format(owed), money.Format(net-owed))
		return b.String(), false
	}
	return "tool tidak dikenal", true
}

// SummarizeDashboard menuliskan ringkasan periode sebagai teks untuk AI.
func SummarizeDashboard(c *Context, d *store.Dashboard) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Periode %s (%s s/d %s). ", d.Period.Label, d.Period.Start, d.Period.End)
	if d.IsCurrent {
		fmt.Fprintf(&b, "Sedang berjalan: hari ke-%d, sisa %d hari.\n", d.DaysElapsed, d.DaysLeft)
	} else {
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "Pemasukan %s, pengeluaran %s, selisih %s. Periode sebelumnya: pemasukan %s, pengeluaran %s.\n",
		money.Format(d.Income), money.Format(d.Expense), money.Format(d.Net), money.Format(d.PrevIncome), money.Format(d.PrevExpense))
	if d.IsCurrent {
		fmt.Fprintf(&b, "Pengeluaran hari ini: %s.\n", money.Format(d.SpentToday))
	}
	if d.PlanExpense > 0 || d.PlanIncome > 0 {
		fmt.Fprintf(&b, "Rencana: pemasukan %s, pengeluaran %s (tagihan tetap %s, uang fleksibel %s). Sisa uang fleksibel %s; jatah aman per hari %s. Tagihan terbayar %s dari %s. Pengeluaran di luar budget %s.\n",
			money.Format(d.PlanIncome), money.Format(d.PlanExpense), money.Format(d.BillsTotal), money.Format(d.FlexPlan),
			money.Format(d.FlexRemaining), money.Format(d.DailyAllowance), money.Format(d.BillsPaid), money.Format(d.BillsTotal), money.Format(d.Unbudgeted))
	} else {
		b.WriteString("Belum ada budget untuk periode ini.\n")
	}
	if len(d.Budgets) > 0 {
		b.WriteString("Budget vs realisasi (kategori | rencana | terpakai | sisa | jatuh tempo):\n")
		for _, x := range d.Budgets {
			due := "-"
			if x.DueDay > 0 {
				due = x.DueDate
			}
			id := x.CategoryID
			fmt.Fprintf(&b, "- %s | %s | %s | %s | %s\n", c.catName(&id), money.Format(x.Amount), money.Format(x.Actual), money.Format(x.Amount-x.Actual), due)
		}
	}
	if len(d.ByCategory) > 0 {
		b.WriteString("Realisasi per kategori (kategori | jenis | total | jumlah transaksi):\n")
		for _, x := range d.ByCategory {
			fmt.Fprintf(&b, "- %s | %s | %s | %d\n", c.catName(x.CategoryID), x.Kind, money.Format(x.Total), x.Count)
		}
	}
	for _, a := range d.Alerts {
		fmt.Fprintf(&b, "Peringatan: %s\n", a.Text)
	}
	return b.String()
}
