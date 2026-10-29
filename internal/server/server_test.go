package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/ganova/mymoney/internal/ai"
	"github.com/ganova/mymoney/internal/config"
	"github.com/ganova/mymoney/internal/store"
)

type client struct {
	t    *testing.T
	base string
	http *http.Client
}

func newTestServer(t *testing.T, password string) (*client, *store.Store) {
	t.Helper()
	loc, _ := time.LoadLocation("Asia/Jakarta")
	st, err := store.OpenMemory(loc)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	cfg := config.Config{Password: password, Location: loc}
	static := fstest.MapFS{"index.html": {Data: []byte("<!doctype html><title>MyMoney</title>")}, "assets/app.js": {Data: []byte("console.log(1)")}}
	srv, err := New(cfg, st, ai.New(ai.Config{}, st), static)
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
