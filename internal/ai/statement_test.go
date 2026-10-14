package ai

import (
	"context"
	"os"
	"testing"

	"github.com/ganova/mymoney/internal/ocr"
)

type wantRow struct {
	typ, desc, cat, to string
	amount             int64
	date               string
}

func checkRows(t *testing.T, res *ExtractResult, acc, cat map[int64]string, want []wantRow) {
	t.Helper()
	if len(res.Drafts) != len(want) {
		for _, d := range res.Drafts {
			t.Logf("got %s %d %q %s", d.Type, d.Amount, d.Description, d.Date)
		}
		t.Fatalf("got %d drafts, want %d", len(res.Drafts), len(want))
	}
	for i, w := range want {
		d := res.Drafts[i]
		c, to := "", ""
		if d.CategoryID != nil {
			c = cat[*d.CategoryID]
		}
		if d.ToAccountID != nil {
			to = acc[*d.ToAccountID]
		}
		if d.Type != w.typ || d.Amount != w.amount || (w.desc != "" && d.Description != w.desc) || (w.cat != "" && c != w.cat) || to != w.to || (w.date != "" && d.Date != w.date) {
			t.Errorf("[%d] got type=%s amount=%d desc=%q cat=%q to=%q date=%s; want %+v", i, d.Type, d.Amount, d.Description, c, to, d.Date, w)
		}
	}
}

const mybcaOCR = `myBCA Mutasi Rekening

10 Okt 2026

TRANSAKSI DEBIT QR 914 -Rp 44.000
Nasi Bebek

TRSF E-BANKING DB GOPAY -Rp 100.000
TOPUP

TARIKAN ATM -Rp 300.000
9 Okt 2026

TRANSAKSI DEBIT QRC IDM -Rp 53.800
INDOMARET

BI-FAST CR TRANSFER DR +Rp 4.499.000
NUSA SATU

TRSF E-BANKING DB spp kaira -Rp 550.000
PURBANUARA

Saldo Akhir

Saldo Rp 8.120.500`

var mybcaWant = []wantRow{
	{"expense", "Nasi Bebek", "Makan & Minum", "", 44000, "2026-10-10"},
	{"transfer", "Top up GoPay", "", "GoPay", 100000, "2026-10-10"},
	{"transfer", "Tarik tunai", "", "Tunai", 300000, "2026-10-10"},
	{"expense", "", "Belanja Harian", "", 53800, "2026-10-09"},
	{"income", "", "", "", 4499000, "2026-10-09"},
	{"expense", "", "Sekolah Anak", "", 550000, "2026-10-09"},
}

func TestStatementOfflineFromOCRText(t *testing.T) {
	st := testStore(t)
	acc, cat := names(t, st)
	res, err := ParseStatementOffline(context.Background(), st, mybcaOCR, 0, "", "ai_image")
	if err != nil {
		t.Fatal(err)
	}
	if acc[res.AccountID] != "BCA" {
		t.Errorf("account detected %q", acc[res.AccountID])
	}
	checkRows(t, res, acc, cat, mybcaWant)
}

func TestStatementOfflineKlikBCALines(t *testing.T) {
	st := testStore(t)
	acc, cat := names(t, st)
	text := "05/10 TRANSAKSI DEBIT TGL: 0510 QR 008 00000.00WATERLAND 43,000.00 DB\n" +
		"05/10 TRSF E-BANKING DB 0505/FTSCY/WS95271 80000.00 bayar jahit nova KAYOMI 80,000.00 DB\n" +
		"06/10 BI-FAST CR TRANSFER DR 013 NUSA SATU INTI ART 4,499,000.00 CR\n" +
		"SALDO AKHIR 3,885,740.00"
	res, err := ParseStatementOffline(context.Background(), st, text, 0, "", "import")
	if err != nil {
		t.Fatal(err)
	}
	checkRows(t, res, acc, cat, []wantRow{
		{"expense", "", "Hiburan & Rekreasi", "", 43000, ""},
		{"expense", "Bayar Jahit Nova Kayomi", "Pakaian & Perawatan", "", 80000, ""},
		{"income", "", "", "", 4499000, ""},
	})
}

func TestStatementOfflineRealOCR(t *testing.T) {
	if !ocr.Available() {
		t.Skip("tesseract tidak terpasang")
	}
	img, err := os.ReadFile("testdata/mybca.png")
	if err != nil {
		t.Fatal(err)
	}
	text, err := ocr.Read(context.Background(), img)
	if err != nil {
		t.Fatal(err)
	}
	st := testStore(t)
	acc, cat := names(t, st)
	res, err := ParseStatementOffline(context.Background(), st, text, 0, "", "ai_image")
	if err != nil {
		t.Fatal(err)
	}
	checkRows(t, res, acc, cat, mybcaWant)
}

func TestStatementOfflinePDF(t *testing.T) {
	if !ocr.PDFAvailable() || !ocr.Available() {
		t.Skip("poppler/tesseract tidak terpasang")
	}
	st := testStore(t)
	acc, cat := names(t, st)
	ctx := context.Background()
	klik := []wantRow{
		{"expense", "", "Hiburan & Rekreasi", "", 43000, "2026-10-05"},
		{"expense", "Bayar Jahit Nova Kayomi", "Pakaian & Perawatan", "", 80000, "2026-10-05"},
		{"income", "", "", "", 4499000, "2026-10-06"},
		{"transfer", "Top up GoPay", "", "GoPay", 21000, "2026-10-07"},
	}
	read := func(name, pw string) string {
		data, err := os.ReadFile("testdata/" + name)
		if err != nil {
			t.Fatal(err)
		}
		text, err := ocr.ReadPDF(ctx, data, pw)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		return text
	}
	res, _ := ParseStatementOffline(ctx, st, read("estatement.pdf", ""), 0, "", "import")
	checkRows(t, res, acc, cat, klik)

	// PDF terkunci: tanpa sandi ditolak dengan pesan jelas, dengan sandi terbaca
	data, _ := os.ReadFile("testdata/locked.pdf")
	if _, err := ocr.ReadPDF(ctx, data, ""); err != ocr.ErrPDFPassword {
		t.Fatalf("expected password error, got %v", err)
	}
	res, _ = ParseStatementOffline(ctx, st, read("locked.pdf", "01011990"), 0, "", "import")
	checkRows(t, res, acc, cat, klik)

	// PDF hasil scan → OCR per halaman
	res, _ = ParseStatementOffline(ctx, st, read("scan.pdf", ""), 0, "", "import")
	checkRows(t, res, acc, cat, mybcaWant)
}
