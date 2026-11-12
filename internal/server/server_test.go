package server

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/ganova/mymoney/internal/ai"
	"github.com/ganova/mymoney/internal/config"
	"github.com/ganova/mymoney/internal/ocr"
	"github.com/ganova/mymoney/internal/store"
	"github.com/xuri/excelize/v2"
)

type client struct {
	t    *testing.T
	base string
	http *http.Client
}

func newTestServer(t *testing.T, password string) (*client, *store.Store) {
	t.Helper()
	return newTestServerAI(t, password, ai.Config{})
}

func newTestServerAI(t *testing.T, password string, aiCfg ai.Config) (*client, *store.Store) {
	t.Helper()
	loc, _ := time.LoadLocation("Asia/Jakarta")
	st, err := store.OpenMemory(loc)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	cfg := config.Config{Password: password, Location: loc, DataDir: t.TempDir(), BackupKeep: 5}
	static := fstest.MapFS{"index.html": {Data: []byte("<!doctype html><title>MyMoney</title>")}, "assets/app.js": {Data: []byte("console.log(1)")}}
	srv, err := New(cfg, st, ai.New(aiCfg, st), static)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)
	jar, _ := cookiejar.New(nil)
	return &client{t: t, base: ts.URL, http: &http.Client{Jar: jar}}, st
}

func (c *client) do(method, path string, body any, headers ...string) (int, []byte) {
	c.t.Helper()
	var r io.Reader
	if s, ok := body.(string); ok {
		r = strings.NewReader(s)
	} else if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, c.base+path, r)
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	res, err := c.http.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer res.Body.Close()
	out, _ := io.ReadAll(res.Body)
	return res.StatusCode, out
}

func (c *client) json(method, path string, body any, v any) int {
	c.t.Helper()
	code, out := c.do(method, path, body)
	if v != nil && code < 300 {
		if err := json.Unmarshal(out, v); err != nil {
			c.t.Fatalf("%s %s: %v (%s)", method, path, err, out)
		}
	}
	return code
}

func TestTransactionsAndDashboard(t *testing.T) {
	c, st := newTestServer(t, "")
	var boot struct {
		Accounts   []store.Account  `json:"accounts"`
		Categories []store.Category `json:"categories"`
	}
	c.json("GET", "/api/bootstrap", nil, &boot)
	bca, tunai := boot.Accounts[0].ID, boot.Accounts[1].ID
	var makan int64
	for _, x := range boot.Categories {
		if x.Name == "Makan & Minum" {
			makan = x.ID
		}
	}

	var created []store.Transaction
	code := c.json("POST", "/api/transactions/bulk", map[string]any{"transactions": []map[string]any{
		{"type": "expense", "amount": 25000, "account_id": tunai, "category_id": makan, "description": "Makan siang"},
		{"type": "transfer", "amount": 300000, "account_id": bca, "to_account_id": tunai, "description": "Tarik tunai"},
	}}, &created)
	if code != 201 || len(created) != 2 {
		t.Fatalf("bulk %d %+v", code, created)
	}
	// validasi ditolak dengan pesan yang jelas
	code, out := c.do("POST", "/api/transactions", map[string]any{"type": "expense", "amount": 0, "account_id": bca})
	if code != 400 || !strings.Contains(string(out), "nominal") {
		t.Fatalf("validation %d %s", code, out)
	}

	var d store.Dashboard
	if code := c.json("GET", "/api/dashboard", nil, &d); code != 200 || d.Expense != 25000 || len(d.Recent) != 2 {
		t.Fatalf("dashboard %d expense=%d recent=%d", code, d.Expense, len(d.Recent))
	}
	var list []store.Transaction
	c.json("GET", "/api/transactions?q=makan", nil, &list)
	if len(list) != 1 {
		t.Fatalf("search %+v", list)
	}

	// hapus massal (dipakai tombol Urungkan)
	if code := c.json("POST", "/api/transactions/delete", map[string]any{"ids": []int64{created[0].ID}}, nil); code != 200 {
		t.Fatalf("delete %d", code)
	}
	if _, err := st.GetTransaction(t.Context(), created[0].ID); err == nil {
		t.Fatal("transaction should be deleted")
	}

	// ekspor
	code, csvOut := c.do("GET", "/api/export?format=csv", nil)
	if code != 200 || !strings.Contains(string(csvOut), "Tarik tunai") {
		t.Fatalf("csv %d %s", code, csvOut)
	}
	if code, xl := c.do("GET", "/api/export", nil); code != 200 || !bytes.HasPrefix(xl, []byte("PK")) {
		t.Fatalf("xlsx %d", code)
	}
}

func TestAskOfflineAndShortcut(t *testing.T) {
	c, st := newTestServer(t, "rahasia")
	c.do("POST", "/api/auth/login", map[string]string{"password": "rahasia"})
	var res struct {
		Mode   string     `json:"mode"`
		Drafts []ai.Draft `json:"drafts"`
	}
	if code := c.json("POST", "/api/ai/ask", map[string]string{"text": "bensin 50rb cash"}, &res); code != 200 || res.Mode != "offline" || len(res.Drafts) != 1 || res.Drafts[0].Amount != 50000 {
		t.Fatalf("ask %d %+v", code, res)
	}

	raw, _, err := st.CreateToken(t.Context(), "tes")
	if err != nil {
		t.Fatal(err)
	}
	anon := &client{t: t, base: c.base, http: &http.Client{}}
	code, out := anon.do("POST", "/api/shortcut/ask", "makan siang 25rb", "Authorization", "Bearer "+raw)
	if code != 200 || !strings.Contains(string(out), "Makan siang") || !strings.Contains(string(out), "Rp25.000") {
		t.Fatalf("shortcut %d %s", code, out)
	}
	list, _ := st.ListTransactions(t.Context(), store.TxFilter{})
	if len(list) != 1 || list[0].Source != "shortcut" {
		t.Fatalf("saved %+v", list)
	}
	// fitur AI tanpa key memberi 503 yang jelas
	if code, out := c.do("POST", "/api/ai/budget", map[string]string{"text": "gaji 10jt"}); code != 503 || !strings.Contains(string(out), "ANTHROPIC_API_KEY") {
		t.Fatalf("budget without key: %d %s", code, out)
	}
}

func TestBudgetApplyAndStatic(t *testing.T) {
	c, _ := newTestServer(t, "")
	var boot struct {
		Period struct{ Key string } `json:"period"`
	}
	c.json("GET", "/api/bootstrap", nil, &boot)
	var r struct{ Saved int }
	code := c.json("POST", "/api/budgets/apply", map[string]any{
		"period": boot.Period.Key, "replace": true,
		"lines": []map[string]any{
			{"new_category": map[string]any{"name": "Jajan Anak", "group": "Sehari-hari", "icon": "🍭", "kind": "expense"}, "amount": 300000, "due_day": 0},
			{"new_category": map[string]any{"name": "Jajan Anak", "group": "Sehari-hari", "icon": "🍭", "kind": "expense"}, "amount": 200000, "due_day": 0, "note": "minggu 2"},
		},
	}, &r)
	if code != 200 || r.Saved != 1 {
		t.Fatalf("apply %d %+v", code, r)
	}
	var b struct {
		Budgets []store.Budget `json:"budgets"`
	}
	c.json("GET", "/api/budgets", nil, &b)
	if len(b.Budgets) != 1 || b.Budgets[0].Amount != 500000 {
		t.Fatalf("budgets %+v", b.Budgets)
	}

	// SPA: rute tak dikenal → index.html; aset statis disajikan apa adanya
	code, out := c.do("GET", "/budget/apa-saja", nil)
	if code != 200 || !strings.Contains(string(out), "MyMoney") {
		t.Fatalf("spa fallback %d %s", code, out)
	}
	if code, out := c.do("GET", "/assets/app.js", nil); code != 200 || string(out) != "console.log(1)" {
		t.Fatalf("asset %d %s", code, out)
	}
	if code, _ := c.do("GET", "/api/tidak-ada", nil); code != 404 {
		t.Fatalf("unknown api %d", code)
	}
}

func TestLedgerHeader(t *testing.T) {
	c, _ := newTestServer(t, "")
	var pribadi, work struct {
		LedgerID int64           `json:"ledger_id"`
		Accounts []store.Account `json:"accounts"`
		Sources  []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		} `json:"income_sources"`
		Other []store.Account `json:"other_accounts"`
	}
	c.json("GET", "/api/bootstrap", nil, &pribadi)
	code, out := c.do("GET", "/api/bootstrap", nil, "X-Ledger", "2")
	if code != 200 || json.Unmarshal(out, &work) != nil {
		t.Fatalf("bootstrap work: %d %s", code, out)
	}
	if pribadi.LedgerID != 1 || work.LedgerID != 2 {
		t.Fatalf("buku aktif: %d / %d", pribadi.LedgerID, work.LedgerID)
	}
	if len(work.Accounts) != 2 || work.Accounts[0].LedgerID != 2 || len(work.Other) != len(pribadi.Accounts) {
		t.Fatalf("akun Work/other: %+v %d", work.Accounts, len(work.Other))
	}
	if len(work.Sources) < 2 {
		t.Fatalf("sumber pemasukan: %+v", work.Sources)
	}
	// catat via kalimat di buku Work: pemasukan freelance mengenali sumbernya
	code, out = c.do("POST", "/api/ai/ask", map[string]string{"text": "terima proyek freelance 3jt"}, "X-Ledger", "2")
	if code != 200 || !strings.Contains(string(out), `"income_source_id":`+strconv.FormatInt(work.Sources[1].ID, 10)) {
		t.Fatalf("ask work: %d %s", code, out)
	}
	// akun buku Pribadi ditolak di buku Work
	code, _ = c.do("POST", "/api/transactions", map[string]any{"type": "expense", "amount": 1000, "account_id": pribadi.Accounts[0].ID}, "X-Ledger", "2")
	if code != 400 {
		t.Fatalf("akun lintas buku harus ditolak: %d", code)
	}
}

func TestReceivablesAPI(t *testing.T) {
	c, _ := newTestServer(t, "")
	h := []string{"X-Ledger", "2"}
	code, out := c.do("POST", "/api/receivables", map[string]any{"name": "Web toko", "client": "Fastwork", "amount": 2000000}, h...)
	if code != 201 {
		t.Fatalf("create: %d %s", code, out)
	}
	var rec struct {
		ID int64 `json:"id"`
	}
	json.Unmarshal(out, &rec)
	var boot struct {
		Accounts []store.Account `json:"accounts"`
	}
	_, b := c.do("GET", "/api/bootstrap", nil, h...)
	json.Unmarshal(b, &boot)
	code, out = c.do("GET", "/api/outlook?months=3", nil, h...)
	if code != 200 || !strings.Contains(string(out), `"pending_total":2000000`) {
		t.Fatalf("outlook: %d %s", code, out)
	}
	code, out = c.do("POST", "/api/receivables/"+strconv.FormatInt(rec.ID, 10)+"/receive", map[string]any{"account_id": boot.Accounts[0].ID, "amount": 1800000}, h...)
	if code != 200 || !strings.Contains(string(out), `"status":"received"`) {
		t.Fatalf("receive: %d %s", code, out)
	}
	// buku Pribadi tidak bisa menyentuh data Work
	code, _ = c.do("PUT", "/api/receivables/"+strconv.FormatInt(rec.ID, 10), map[string]any{"name": "x", "amount": 1}, "X-Ledger", "1")
	if code != 404 {
		t.Fatalf("lintas buku: %d", code)
	}
}

func TestScanReceivablesOffline(t *testing.T) {
	if !ocr.Available() {
		t.Skip("tesseract tidak terpasang")
	}
	c, _ := newTestServer(t, "")
	data, err := os.ReadFile("testdata/fastwork.png")
	if err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("images", "fastwork.png")
	fw.Write(data)
	mw.Close()
	req, _ := http.NewRequest("POST", c.base+"/api/receivables/scan", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-Ledger", "2")
	res, err := c.http.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var out struct {
		Mode   string `json:"mode"`
		Orders []struct {
			Ref     string `json:"ref"`
			Amount  int64  `json:"amount"`
			DueDate string `json:"due_date"`
			Exists  bool   `json:"exists"`
		} `json:"orders"`
	}
	json.NewDecoder(res.Body).Decode(&out)
	if res.StatusCode != 200 || out.Mode != "offline" || len(out.Orders) != 4 {
		t.Fatalf("scan: %d %+v", res.StatusCode, out)
	}
	sum := int64(0)
	for _, o := range out.Orders {
		sum += o.Amount
	}
	if sum != 4_750_000+1_000_000+8_700_000+3_060_000 || out.Orders[1].Ref != "WTVKOH4E" || out.Orders[1].DueDate == "" {
		t.Fatalf("isi scan: %+v", out.Orders)
	}
	// simpan lalu scan ulang: ditandai sudah ada
	items := []map[string]any{{"name": "Web Digital space komputer", "ref": out.Orders[1].Ref, "amount": out.Orders[1].Amount, "client": "Fastwork"}}
	if code, o := c.do("POST", "/api/receivables/bulk", map[string]any{"items": items}, "X-Ledger", "2"); code != 201 {
		t.Fatalf("bulk: %d %s", code, o)
	}
	body2 := bytes.Buffer{}
	mw2 := multipart.NewWriter(&body2)
	fw2, _ := mw2.CreateFormFile("images", "fastwork.png")
	fw2.Write(data)
	mw2.Close()
	req2, _ := http.NewRequest("POST", c.base+"/api/receivables/scan", &body2)
	req2.Header.Set("Content-Type", mw2.FormDataContentType())
	req2.Header.Set("X-Ledger", "2")
	res2, err := c.http.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	defer res2.Body.Close()
	json.NewDecoder(res2.Body).Decode(&out)
	if !out.Orders[1].Exists || out.Orders[0].Exists {
		t.Fatalf("deteksi duplikat: %+v", out.Orders)
	}
}

// fakeCodexScript membuat program `codex` tiruan: stderr diisi errMsg dan keluar
// dengan status 1 bila errMsg tidak kosong; selain itu menulis reply ke berkas -o.
func fakeCodexScript(t *testing.T, errMsg, reply string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "codex")
	body := "#!/bin/sh\nout=\nwhile [ $# -gt 0 ]; do [ \"$1\" = -o ] && out=$2; shift; done\ncat >/dev/null\n"
	if errMsg != "" {
		body += "echo '" + errMsg + "' >&2\nexit 1\n"
	} else {
		body += "cat > \"$out\" <<'EOF2'\n" + reply + "\nEOF2\n"
	}
	if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestAIFailureFallsBackToOffline(t *testing.T) {
	codex := fakeCodexScript(t, `ERROR: {"type":"error","status":400,"error":{"message":"The 'gpt-5.6-sol' model requires a newer version of Codex. Please upgrade to the latest app or CLI"}}`, "")
	c, _ := newTestServerAI(t, "", ai.Config{CodexPath: codex})

	// AI gagal, tetapi kalimat dikenali parser offline → draf tetap ada
	var res struct {
		Reply  string `json:"reply"`
		Mode   string `json:"mode"`
		Drafts []struct {
			Amount int64 `json:"amount"`
		} `json:"drafts"`
	}
	if code := c.json("POST", "/api/ai/ask", map[string]string{"text": "makan siang 25rb"}, &res); code != 200 {
		t.Fatalf("ask: %d", code)
	}
	if res.Mode != "offline" || len(res.Drafts) != 1 || res.Drafts[0].Amount != 25000 ||
		!strings.Contains(res.Reply, "AI sedang tidak bisa diakses") || !strings.Contains(res.Reply, "perlu diperbarui") {
		t.Fatalf("fallback: %+v", res)
	}

	// kalimat yang tidak dipahami parser offline: tetap 200 dengan penjelasan, bukan error
	res.Reply = ""
	if code := c.json("POST", "/api/ai/ask", map[string]string{"text": "berapa sisa budget makan?"}, &res); code != 200 || res.Reply == "" || len(res.Drafts) != 0 {
		t.Fatalf("tanpa draf: code=%d %+v", code, res)
	}

	// tes koneksi menunjukkan pesan yang jelas
	var test struct {
		OK      bool   `json:"ok"`
		Message string `json:"message"`
	}
	c.json("POST", "/api/ai/test", nil, &test)
	if test.OK || !strings.Contains(test.Message, "npm i -g @openai/codex@latest") {
		t.Fatalf("tes AI: %+v", test)
	}
}

func TestAIModeSetting(t *testing.T) {
	good := `{"reply":"Dicatat oleh AI.","transactions":[]}`
	c, _ := newTestServerAI(t, "", ai.Config{CodexPath: fakeCodexScript(t, "", good)})
	var boot struct {
		Settings map[string]any `json:"settings"`
		AI       struct {
			Enabled   bool   `json:"enabled"`
			Available bool   `json:"available"`
			Mode      string `json:"mode"`
		} `json:"ai"`
	}
	c.json("GET", "/api/bootstrap", nil, &boot)
	if !boot.AI.Enabled || !boot.AI.Available || boot.AI.Mode != "ai" {
		t.Fatalf("awal: %+v", boot.AI)
	}
	var out struct {
		Reply string `json:"reply"`
		Mode  string `json:"mode"`
	}
	c.json("POST", "/api/ai/ask", map[string]string{"text": "halo"}, &out)
	if out.Mode != "ai" || out.Reply != "Dicatat oleh AI." {
		t.Fatalf("mode ai: %+v", out)
	}

	// pilih "Tanpa AI" di pengaturan: AI tidak dipanggil, bootstrap melaporkan nonaktif
	boot.Settings["ai_mode"] = "offline"
	if code, b := c.do("PUT", "/api/settings", boot.Settings); code != 200 {
		t.Fatalf("simpan: %d %s", code, b)
	}
	c.json("GET", "/api/bootstrap", nil, &boot)
	if boot.AI.Enabled || !boot.AI.Available || boot.AI.Mode != "offline" {
		t.Fatalf("setelah offline: %+v", boot.AI)
	}
	c.json("POST", "/api/ai/ask", map[string]string{"text": "makan siang 25rb"}, &out)
	if out.Mode != "offline" || strings.Contains(out.Reply, "Dicatat oleh AI") || strings.Contains(out.Reply, "tidak bisa diakses") {
		t.Fatalf("mode offline: %+v", out)
	}
}

func TestMonthlyReportAndBackupDir(t *testing.T) {
	c, st := newTestServer(t, "")
	ctx := t.Context()
	cur, _ := st.CurrentPeriod(ctx)
	prev := cur.Prev()
	var boot struct {
		Accounts   []store.Account  `json:"accounts"`
		Categories []store.Category `json:"categories"`
	}
	c.json("GET", "/api/bootstrap", nil, &boot)
	cat := func(name string) int64 {
		for _, x := range boot.Categories {
			if x.Name == name {
				return x.ID
			}
		}
		t.Fatalf("kategori %s tidak ada", name)
		return 0
	}
	mk := func(typ string, amt int64, catID int64, date string) {
		if _, err := st.CreateTransaction(ctx, store.Transaction{Type: typ, Amount: amt, AccountID: boot.Accounts[0].ID, CategoryID: &catID, Date: date, Description: "uji"}); err != nil {
			t.Fatal(err)
		}
	}
	mk("income", 10_000_000, cat("Gaji"), prev.StartStr())
	mk("expense", 250_000, cat("Makan & Minum"), prev.Start.AddDate(0, 0, 3).Format("2006-01-02"))
	mk("expense", 99_000, cat("Makan & Minum"), cur.StartStr()) // periode berjalan: tidak masuk laporan

	// pilih folder lewat API
	dir := filepath.Join(t.TempDir(), "My Drive", "My Money")
	var status struct {
		Dir     string   `json:"dir"`
		Warning string   `json:"warning"`
		Reports []string `json:"reports"`
	}
	if code := c.json("PUT", "/api/backup/dir", map[string]string{"dir": dir}, &status); code != 200 || status.Dir != dir || status.Warning != "" {
		t.Fatalf("set dir: %d %+v", code, status)
	}
	if code, _ := c.do("PUT", "/api/backup/dir", map[string]string{"dir": "relatif/saja"}); code != 400 {
		t.Fatalf("lokasi relatif harus ditolak: %d", code)
	}

	// laporan periode lalu: hanya buku yang punya transaksi (Utama)
	var made struct {
		Files []string `json:"files"`
	}
	if code := c.json("POST", "/api/backup/report", nil, &made); code != 200 || len(made.Files) != 1 {
		t.Fatalf("laporan: %d %+v", code, made)
	}
	want := filepath.Join(dir, "Laporan "+prev.Label+" - Utama.xlsx")
	if made.Files[0] != want {
		t.Fatalf("nama berkas: %s, harapan %s", made.Files[0], want)
	}
	x, err := excelize.OpenFile(want)
	if err != nil {
		t.Fatal(err)
	}
	defer x.Close()
	for _, sh := range []string{"Ringkasan", "Per Kategori", "Harian", "Transaksi"} {
		if i, _ := x.GetSheetIndex(sh); i < 0 {
			t.Fatalf("sheet %s tidak ada: %v", sh, x.GetSheetList())
		}
	}
	if v, _ := x.GetCellValue("Ringkasan", "B4", excelize.Options{RawCellValue: true}); v != "10000000" {
		t.Fatalf("pemasukan di ringkasan: %q", v)
	}
	if v, _ := x.GetCellValue("Ringkasan", "B5", excelize.Options{RawCellValue: true}); v != "250000" {
		t.Fatalf("pengeluaran hanya periode lalu: %q", v)
	}
	rows, _ := x.GetRows("Transaksi")
	if len(rows) != 3 { // header + 2 transaksi periode lalu
		t.Fatalf("baris transaksi: %d", len(rows))
	}
	c.json("GET", "/api/backup/status", nil, &status)
	if len(status.Reports) != 1 {
		t.Fatalf("daftar laporan: %+v", status.Reports)
	}

	// folder pilihan tidak bisa diakses → jatuh ke data/backups dengan peringatan
	blocker := filepath.Join(t.TempDir(), "berkas")
	os.WriteFile(blocker, []byte("x"), 0o600)
	st.SetSetting(ctx, "backup_dir", filepath.Join(blocker, "sub"))
	c.json("GET", "/api/backup/status", nil, &status)
	if status.Warning == "" || !strings.HasSuffix(status.Dir, "backups") {
		t.Fatalf("fallback: %+v", status)
	}
}
