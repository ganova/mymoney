package server

import (
	"bytes"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/emersion/go-imap/v2/imapserver/imapmemserver"

	"github.com/ganova/mymoney/internal/ai"
	"github.com/ganova/mymoney/internal/config"
	"github.com/ganova/mymoney/internal/store"
)

type lit struct{ *bytes.Reader }

func (l lit) Size() int64 { return l.Reader.Size() }

// startIMAP menjalankan server IMAP tiruan di memori dan mengembalikan alamat & pengguna.
func startIMAP(t *testing.T) (string, *imapmemserver.User) {
	t.Helper()
	mem := imapmemserver.New()
	user := imapmemserver.NewUser("saya@gmail.com", "abcdefghijklmnop")
	if err := user.Create("INBOX", nil); err != nil {
		t.Fatal(err)
	}
	mem.AddUser(user)
	srv := imapserver.New(&imapserver.Options{
		NewSession: func(*imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
			return mem.NewSession(), nil, nil
		},
		Caps:         imap.CapSet{imap.CapIMAP4rev1: {}},
		InsecureAuth: true,
		Logger:       nil,
	})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go srv.Serve(ln)
	t.Cleanup(func() { srv.Close() })
	return ln.Addr().String(), user
}

func addMail(t *testing.T, u *imapmemserver.User, from, subject, contentType, body, msgID string, at time.Time) {
	t.Helper()
	raw := "From: " + from + "\r\nTo: saya@gmail.com\r\nSubject: " + subject + "\r\nMessage-ID: <" + msgID + ">\r\nDate: " + at.Format(time.RFC1123Z) +
		"\r\nMIME-Version: 1.0\r\nContent-Type: " + contentType + "; charset=utf-8\r\n\r\n" + body + "\r\n"
	if _, err := u.Append("INBOX", lit{bytes.NewReader([]byte(raw))}, &imap.AppendOptions{Time: at}); err != nil {
		t.Fatal(err)
	}
}

func TestEmailImportFromGmail(t *testing.T) {
	addr, user := startIMAP(t)
	now := time.Now()
	addMail(t, user, "BCA <bca@bca.co.id>", "Internet Transaction Journal", "text/html",
		`<html><body><table><tr><td>Nama Merchant</td><td>: INDOMARET</td></tr><tr><td>Nominal Transaksi</td><td>: IDR 25,000.00</td></tr><tr><td>Saldo Akhir</td><td>: IDR 1,234,000.00</td></tr></table></body></html>`,
		"tx1@bca.co.id", now.Add(-2*time.Hour))
	addMail(t, user, "BCA <bca@bca.co.id>", "Dana Masuk", "text/plain",
		"Anda menerima dana masuk\nPengirim : PT CMW\nNominal : Rp 1.500.000,00", "tx2@bca.co.id", now.Add(-time.Hour))
	addMail(t, user, "Toko Online <promo@toko.com>", "Diskon IDR 50,000", "text/plain", "Belanja sekarang", "promo@toko.com", now)

	loc, _ := time.LoadLocation("Asia/Jakarta")
	st, err := store.OpenMemory(loc)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	srv, err := New(config.Config{Location: loc, DataDir: t.TempDir()}, st, ai.New(ai.Config{}, st), fstest.MapFS{"index.html": {Data: []byte("x")}})
	if err != nil {
		t.Fatal(err)
	}
	srv.imapAddr, srv.imapInsecure = addr, true
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)
	jar, _ := cookiejar.New(nil)
	c := &client{t: t, base: ts.URL, http: &http.Client{Jar: jar}}

	var bca int64
	accs, _ := st.ListAccounts(t.Context())
	for _, a := range accs {
		if a.Name == "BCA" {
			bca = a.ID
		}
	}
	since := now.In(loc).AddDate(0, 0, -2).Format("2006-01-02")
	if code, out := c.do("PUT", "/api/email/config", map[string]any{"enabled": true, "email": "saya@gmail.com", "password": "abcd efgh ijkl mnop", "account_id": bca, "since": since}); code != 200 {
		t.Fatalf("simpan: %d %s", code, out)
	}
	var cfg map[string]any
	c.json("GET", "/api/email/config", nil, &cfg)
	if _, leaked := cfg["password"]; leaked || cfg["has_password"] != true {
		t.Fatalf("App Password tidak boleh dikirim balik: %v", cfg)
	}

	var test struct {
		OK      bool   `json:"ok"`
		Message string `json:"message"`
	}
	c.json("POST", "/api/email/test", nil, &test)
	if !test.OK || !strings.Contains(test.Message, "2 email") {
		t.Fatalf("tes koneksi: %+v", test)
	}

	var run emailResult
	if code := c.json("POST", "/api/email/run", nil, &run); code != 200 || run.Saved != 2 || run.Checked != 2 {
		t.Fatalf("putaran 1: %d %+v", code, run)
	}
	list, _ := st.ListTransactions(t.Context(), store.TxFilter{})
	if len(list) != 2 {
		t.Fatalf("transaksi: %+v", list)
	}
	var exp, inc *store.Transaction
	for i := range list {
		if list[i].Type == "expense" {
			exp = &list[i]
		} else if list[i].Type == "income" {
			inc = &list[i]
		}
	}
	if exp == nil || exp.Amount != 25_000 || exp.Description != "Indomaret" || exp.AccountID != bca || exp.Source != "email" || exp.CategoryID == nil {
		t.Fatalf("pengeluaran dari email: %+v", exp)
	}
	if c, _ := st.GetCategory(t.Context(), *exp.CategoryID); c.Name != "Belanja Harian" {
		t.Fatalf("kategori Indomaret: %s", c.Name)
	}
	if inc == nil || inc.Amount != 1_500_000 {
		t.Fatalf("pemasukan dari email: %+v", inc)
	}

	// putaran kedua: tidak ada yang dicatat ulang
	run = emailResult{}
	c.json("POST", "/api/email/run", nil, &run)
	if run.Saved != 0 || run.Checked != 0 {
		t.Fatalf("putaran 2 harus kosong: %+v", run)
	}
	var imports []store.EmailImport
	c.json("GET", "/api/email/imports", nil, &imports)
	if len(imports) != 2 || imports[0].Status != "tersimpan" {
		t.Fatalf("riwayat: %+v", imports)
	}

	// App Password salah → pesan yang jelas, bukan crash
	c.do("PUT", "/api/email/config", map[string]any{"enabled": true, "email": "saya@gmail.com", "password": "salah", "account_id": bca})
	c.json("POST", "/api/email/test", nil, &test)
	if test.OK || test.Message == "" {
		t.Fatalf("login salah: %+v", test)
	}
}
