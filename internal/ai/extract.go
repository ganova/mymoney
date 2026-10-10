package ai

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/ganova/mymoney/internal/period"
	"github.com/ganova/mymoney/internal/store"
)

const extractPrompt = `Kamu adalah mesin pembaca mutasi rekening dan riwayat transaksi untuk aplikasi keuangan pribadi di Indonesia. Tugasmu mengekstrak SEMUA transaksi dari sumber yang diberikan (screenshot aplikasi bank/e-wallet, teks mutasi, atau isi spreadsheet) menjadi data terstruktur, lalu mengkategorikannya memakai data pengguna.

Cara membaca:
- Pahami format umum: BCA (TRSF E-BANKING DB/CR, TRANSAKSI DEBIT ... QR/QRC, BI-FAST DB/CR, DB INTERCHANGE, TARIKAN ATM, SETORAN, BIAYA ADM, BUNGA, PAJAK), Mandiri, BRI, BNI, Jago, SeaBank, blu, GoPay, OVO, DANA, ShopeePay, Flazz, dan sejenisnya.
- DB / Debit / tanda minus / warna merah / "keluar" = uang keluar. CR / Kredit / tanda plus / warna hijau / "masuk" = uang masuk.
- Uang keluar → type "expense", KECUALI perpindahan ke akun milik pengguna sendiri → type "transfer" dengan to_account_id: top up GoPay/ShopeePay/DANA/OVO/Flazz, tarik tunai/TARIKAN ATM (→ akun Tunai), transfer ke rekening sendiri yang ada di daftar akun.
- Uang masuk → type "income", KECUALI dari akun milik pengguna sendiri → transfer (akun asal ke akun ini).
- Abaikan baris yang bukan transaksi: saldo awal/akhir, total, header, iklan, tombol, transaksi gagal/dibatalkan, dan baris "pending" yang jelas belum terjadi (tulis di notes).
- Jangan menggandakan transaksi bila satu transaksi tampil di beberapa baris teks. Bila beberapa gambar saling tumpang tindih, transaksi yang sama cukup ditulis sekali.
- amount selalu positif (bilangan bulat rupiah, tanpa sen). "189000.00" = 189000.
- date: format YYYY-MM-DD. Bila tahun tidak tertulis, pilih tahun yang membuat tanggal tidak berada di masa depan relatif terhadap hari ini. Pada mutasi BCA, "0505" di keterangan biasanya DDMM. Bila sumber hanya memuat angka hari (mis. 28, 29, 1, 2), tafsirkan di dalam periode yang disebutkan di bawah.
- raw: salin teks asli baris transaksi (satukan menjadi satu baris).
- description: nama merchant/tujuan/keperluan yang bersih dan mudah dibaca. Contoh: "TRANSAKSI DEBIT TGL: 0506 QR 914 00000.00Nasi Bebek" → "Nasi Bebek"; "TRSF E-BANKING DB 0505/FTSCY/WS95271 80000.00 bayar jahit nova KAYOMI" → "Bayar jahit Nova"; "TRSF E-BANKING DB 0705/FTFVA/WS95271 70001/GOPAY TOPUP" → "Top up GoPay"; "IDM INDOMA" → "Indomaret"; "BI-FAST CR TRANSFER DR 013 NUSA SATU INTI ART" → "Transfer masuk dari Nusa Satu Inti Artha".
- category_id: pilih kategori yang paling cocok dari data pengguna (utamakan aturan dan kebiasaan pengguna). null hanya untuk transfer.
- account_id: akun sumber mutasi (lihat instruksi akun di bawah). debt_id: isi bila transaksi adalah pembayaran utang/paylater yang terdaftar (mis. SPAYLATER, KREDIVO).
- amount_estimated selalu false kecuali nominal benar-benar tidak terbaca. confidence mencerminkan keyakinan atas kategori dan keterbacaan.
- source: nama aplikasi/bank yang terdeteksi. notes: catatan singkat untuk pengguna (mis. baris yang dilewati), boleh kosong.`

func extractSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"source", "account_id", "transactions", "notes"},
		"properties": map[string]any{
			"source":       map[string]any{"type": "string", "description": "Aplikasi/bank sumber yang terdeteksi"},
			"account_id":   nullableInt("ID akun pengguna yang paling cocok dengan sumber mutasi"),
			"transactions": map[string]any{"type": "array", "items": draftSchema(true)},
			"notes":        map[string]any{"type": "string"},
		},
	}
}

// ExtractInput adalah sumber data untuk ekstraksi transaksi.
type ExtractInput struct {
	Images    []Image  // screenshot
	PDFs      [][]byte // e-statement / rekening koran PDF
	Text      string   // teks mutasi / isi spreadsheet
	AccountID int64    // 0 = deteksi otomatis
	Period    string   // periode acuan untuk tanggal berupa angka hari saja
	Kind      string   // "ai_image" atau "import"
}

// Image adalah gambar yang diunggah.
type Image struct {
	Data      []byte
	MediaType string // image/jpeg, image/png, image/webp, image/gif
}

// ExtractResult adalah hasil ekstraksi.
type ExtractResult struct {
	Source    string  `json:"source"`
	AccountID int64   `json:"account_id"`
	Notes     string  `json:"notes"`
	Drafts    []Draft `json:"drafts"`
}

// Extract membaca transaksi dari screenshot atau teks, lalu menandai
// kemungkinan duplikat dengan transaksi yang sudah tersimpan.
func (s *Service) Extract(ctx context.Context, in ExtractInput) (*ExtractResult, error) {
	c, err := LoadContext(ctx, s.store)
	if err != nil {
		return nil, err
	}
	p := c.Period
	if in.Period != "" {
		if q, err := period.Parse(in.Period, c.Settings.PeriodStartDay); err == nil {
			p = q
		}
	}
	var instr strings.Builder
	if in.AccountID != 0 {
		fmt.Fprintf(&instr, "Akun sumber mutasi: id=%d (%s). Gunakan sebagai account_id untuk semua transaksi (kecuali transfer masuk dari akun lain).\n", in.AccountID, c.accName(in.AccountID))
	} else {
		instr.WriteString("Akun sumber mutasi: deteksi otomatis dari tampilan/isi (mis. logo/nama aplikasi myBCA → akun BCA). Bila tidak jelas, gunakan akun default.\n")
	}
	fmt.Fprintf(&instr, "Periode acuan: %s (%s s/d %s). Angka hari ≥ %d berada di bulan %s %d; angka hari lainnya di bulan %s %d.\n",
		p.Label, p.StartStr(), p.EndStr(), p.Start.Day(),
		period.MonthName(p.Start.Month()), p.Start.Year(), period.MonthName(p.End.Month()), p.End.Year())

	var content []anthropic.BetaContentBlockParamUnion
	for i, img := range in.Images {
		content = append(content, anthropic.NewBetaTextBlock(fmt.Sprintf("Gambar %d:", i+1)))
		content = append(content, anthropic.NewBetaImageBlock(anthropic.BetaBase64ImageSourceParam{
			Data:      base64.StdEncoding.EncodeToString(img.Data),
			MediaType: anthropic.BetaBase64ImageSourceMediaType(img.MediaType),
		}))
	}
	for i, pdf := range in.PDFs {
		content = append(content, anthropic.NewBetaTextBlock(fmt.Sprintf("Dokumen PDF %d:", i+1)))
		content = append(content, anthropic.NewBetaDocumentBlock(anthropic.BetaBase64PDFSourceParam{
			Data: base64.StdEncoding.EncodeToString(pdf),
		}))
	}
	if t := strings.TrimSpace(in.Text); t != "" {
		content = append(content, anthropic.NewBetaTextBlock("Sumber data (teks):\n<data>\n"+t+"\n</data>"))
	}
	if len(content) == 0 {
		return nil, fmt.Errorf("tidak ada gambar, PDF, atau teks untuk dibaca")
	}
	content = append(content, anthropic.NewBetaTextBlock(instr.String()+"\nEkstrak semua transaksinya sekarang."))

	params := s.baseParams(s.cfg.VisionEffort, 64000)
	params.System = s.systemBlocks(c, extractPrompt)
	params.Messages = []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(content...)}
	params.OutputConfig.Format = jsonFormat(extractSchema())

	kind := in.Kind
	if kind == "" {
		kind = "ai_image"
	}
	msg, err := s.send(ctx, kind, params)
	if err != nil {
		return nil, err
	}
	var out struct {
		Source       string  `json:"source"`
		AccountID    *int64  `json:"account_id"`
		Transactions []Draft `json:"transactions"`
		Notes        string  `json:"notes"`
	}
	if err := decodeJSON(msg, &out); err != nil {
		return nil, err
	}
	res := &ExtractResult{Source: out.Source, Notes: out.Notes, Drafts: []Draft{}}
	if in.AccountID != 0 {
		res.AccountID = in.AccountID
	} else if out.AccountID != nil {
		res.AccountID = *out.AccountID
	}
	seen := map[string]bool{}
	for _, d := range out.Transactions {
		if in.AccountID != 0 && d.Type != "income" && d.Type != "transfer" {
			d.AccountID = in.AccountID
		}
		d = c.Sanitize(d, kind)
		key := fmt.Sprintf("%s|%s|%d|%d|%s", d.Date, d.Type, d.Amount, d.AccountID, strings.ToLower(strings.TrimSpace(d.Raw)))
		if seen[key] {
			d.DuplicateInScan = true
		}
		seen[key] = true
		d.DuplicateOf = s.store.FindDuplicate(ctx, d.Transaction())
		res.Drafts = append(res.Drafts, d)
	}
	return res, nil
}

// MarkDuplicates menandai draf yang kemungkinan sudah tercatat.
func MarkDuplicates(ctx context.Context, st *store.Store, drafts []Draft) {
	for i := range drafts {
		drafts[i].DuplicateOf = st.FindDuplicate(ctx, drafts[i].Transaction())
	}
}
