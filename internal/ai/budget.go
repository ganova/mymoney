package ai

import (
	"context"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/ganova/mymoney/internal/money"
)

const budgetPrompt = `Kamu membantu pengguna aplikasi keuangan pribadi menyusun budget bulanan. Pengguna menempelkan daftar rencana anggaran (dari catatan, Google Sheet, chat, dsb.) atau mendeskripsikannya dengan kalimat bebas. Ubah menjadi baris budget per kategori.

Aturan:
- Petakan setiap item ke kategori yang SUDAH ADA bila cocok (isi category_id, new_category null). Bila beberapa item cocok ke kategori yang sama (mis. "Uang sehari-hari minggu 1..4"), gabungkan nominalnya menjadi satu baris dan sebutkan rinciannya di note.
- Bila tidak ada kategori yang cocok, usulkan kategori baru lewat new_category (name singkat, group salah satu dari: "Tagihan Bulanan", "Sehari-hari", "Belanja Bulanan", "Cicilan & Utang", "Lain-lain", atau "Pemasukan" untuk pemasukan; icon satu emoji; kind expense/income) dan category_id null.
- Pemasukan rutin (gaji, dsb.) juga dijadikan baris budget dengan kategori pemasukan.
- due_day: tanggal jatuh tempo bila item adalah tagihan tetap dengan tanggal bayar (mis. "tgl 28", kolom "Tgl Act" berisi 28). 0 untuk pengeluaran fleksibel.
- Abaikan item bernilai 0 atau baris kosong/"-", kecuali pengguna jelas ingin mencatatnya.
- amount: bilangan bulat rupiah.
- summary: 1-2 kalimat ringkasan dalam bahasa Indonesia (total pemasukan, total pengeluaran, selisih).`

func budgetSchema() map[string]any {
	newCat := map[string]any{
		"anyOf": []any{
			map[string]any{
				"type": "object", "additionalProperties": false,
				"required": []string{"name", "group", "icon", "kind"},
				"properties": map[string]any{
					"name":  map[string]any{"type": "string"},
					"group": map[string]any{"type": "string"},
					"icon":  map[string]any{"type": "string"},
					"kind":  map[string]any{"type": "string", "enum": []string{"expense", "income"}},
				},
			},
			map[string]any{"type": "null"},
		},
	}
	line := map[string]any{
		"type": "object", "additionalProperties": false,
		"required": []string{"label", "category_id", "new_category", "amount", "due_day", "note"},
		"properties": map[string]any{
			"label":        map[string]any{"type": "string", "description": "Nama item seperti ditulis pengguna"},
			"category_id":  nullableInt("ID kategori yang sudah ada"),
			"new_category": newCat,
			"amount":       map[string]any{"type": "integer"},
			"due_day":      map[string]any{"type": "integer", "description": "1-31 untuk tagihan bertanggal, 0 bila fleksibel"},
			"note":         map[string]any{"type": "string"},
		},
	}
	return map[string]any{
		"type": "object", "additionalProperties": false,
		"required": []string{"lines", "summary"},
		"properties": map[string]any{
			"lines":   map[string]any{"type": "array", "items": line},
			"summary": map[string]any{"type": "string"},
		},
	}
}

// NewCategory adalah usulan kategori baru dari AI.
type NewCategory struct {
	Name  string `json:"name"`
	Group string `json:"group"`
	Icon  string `json:"icon"`
	Kind  string `json:"kind"`
}

// BudgetLine adalah usulan satu baris budget.
type BudgetLine struct {
	Label       string       `json:"label"`
	CategoryID  *int64       `json:"category_id"`
	NewCategory *NewCategory `json:"new_category"`
	Amount      int64        `json:"amount"`
	DueDay      int          `json:"due_day"`
	Note        string       `json:"note"`
}

// BudgetSuggestion adalah hasil penyusunan budget oleh AI.
type BudgetSuggestion struct {
	Lines   []BudgetLine `json:"lines"`
	Summary string       `json:"summary"`
}

// SuggestBudget mengubah daftar/kalimat bebas menjadi baris budget.
func (s *Service) SuggestBudget(ctx context.Context, text string) (*BudgetSuggestion, error) {
	c, err := LoadContext(ctx, s.store)
	if err != nil {
		return nil, err
	}
	p := s.baseParams(s.cfg.VisionEffort, 32000)
	p.System = s.systemBlocks(c, budgetPrompt)
	p.Messages = []anthropic.BetaMessageParam{userText("Rencana budget saya:\n<data>\n" + strings.TrimSpace(text) + "\n</data>")}
	p.OutputConfig.Format = jsonFormat(budgetSchema())
	msg, err := s.send(ctx, "budget", p)
	if err != nil {
		return nil, err
	}
	var out BudgetSuggestion
	if err := decodeJSON(msg, &out); err != nil {
		return nil, err
	}
	valid := out.Lines[:0]
	for _, l := range out.Lines {
		if l.CategoryID != nil {
			if _, ok := c.catByID[*l.CategoryID]; !ok {
				l.CategoryID = nil
			}
		}
		if l.CategoryID != nil {
			l.NewCategory = nil
		} else if l.NewCategory == nil || strings.TrimSpace(l.NewCategory.Name) == "" {
			l.NewCategory = &NewCategory{Name: l.Label, Group: "Lain-lain", Icon: "📌", Kind: "expense"}
		}
		if l.DueDay < 0 || l.DueDay > 31 {
			l.DueDay = 0
		}
		if l.Amount < 0 {
			l.Amount = -l.Amount
		}
		valid = append(valid, l)
	}
	out.Lines = valid
	return &out, nil
}

const insightPrompt = `Kamu adalah perencana keuangan pribadi yang hangat dan jujur. Berdasarkan ringkasan data satu periode keuangan pengguna, tulis analisis singkat dalam bahasa Indonesia santai (pakai "kamu").

Format Markdown sederhana (tanpa tabel):
- Mulai dengan 1 kalimat kesimpulan kondisi keuangan periode ini.
- "### Sorotan" — 3–5 poin temuan paling penting, dengan angka (format Rp25.000): kategori yang melonjak/terlampaui, pola belanja yang menonjol (mis. sering jajan minimarket), perbandingan dengan periode lalu, tagihan yang belum dibayar.
- "### Saran" — 2–4 langkah konkret dan realistis untuk sisa periode/periode depan (mis. batas harian, kategori yang perlu direm, target tabungan).
- Bila periode masih berjalan, sebut jatah aman harian.
Jangan mengarang data yang tidak ada. Maksimal ±200 kata.`

// Insight menulis analisis AI untuk satu periode.
func (s *Service) Insight(ctx context.Context, key string) (string, error) {
	c, err := LoadContext(ctx, s.store)
	if err != nil {
		return "", err
	}
	p, err := s.store.ResolvePeriod(ctx, key)
	if err != nil {
		return "", err
	}
	d, err := s.store.Dashboard(ctx, p)
	if err != nil {
		return "", err
	}
	r, err := s.store.Report(ctx, p, 4)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString(SummarizeDashboard(c, d))
	b.WriteString("\nPengeluaran terbesar per deskripsi (deskripsi | total | frekuensi):\n")
	for _, t := range r.Top {
		fmt.Fprintf(&b, "- %s | %s | %dx\n", t.Description, money.Format(t.Total), t.Count)
	}
	b.WriteString("\nTren beberapa periode (periode | pemasukan | pengeluaran):\n")
	for _, h := range r.History {
		fmt.Fprintf(&b, "- %s | %s | %s\n", h.Label, money.Format(h.Income), money.Format(h.Expense))
	}
	params := s.baseParams("medium", 16000)
	params.System = []anthropic.BetaTextBlockParam{{Text: insightPrompt}}
	params.Messages = []anthropic.BetaMessageParam{userText(b.String())}
	msg, err := s.send(ctx, "insight", params)
	if err != nil {
		return "", err
	}
	return textOf(msg), nil
}
