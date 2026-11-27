package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ganova/mymoney/internal/ai"
	"github.com/ganova/mymoney/internal/mailimport"
	"github.com/ganova/mymoney/internal/store"
)

// emailConfig disimpan di data/email-import.json (bukan di database), agar
// App Password tidak ikut tersalin ke backup yang mungkin tersinkron ke cloud.
type emailConfig struct {
	Enabled   bool   `json:"enabled"`
	Email     string `json:"email"`
	Password  string `json:"password"`
	Sender    string `json:"sender"`
	LedgerID  int64  `json:"ledger_id"`
	AccountID int64  `json:"account_id"`
	Since     string `json:"since"` // YYYY-MM-DD: email sebelum tanggal ini diabaikan
}

func (s *Server) emailConfigPath() string { return filepath.Join(s.cfg.DataDir, "email-import.json") }

func (s *Server) loadEmailConfig() emailConfig {
	c := emailConfig{Sender: "bca.co.id", LedgerID: store.DefaultLedger}
	if b, err := os.ReadFile(s.emailConfigPath()); err == nil {
		json.Unmarshal(b, &c)
	}
	if c.LedgerID == 0 {
		c.LedgerID = store.DefaultLedger
	}
	return c
}

func (s *Server) saveEmailConfig(c emailConfig) error {
	b, _ := json.MarshalIndent(c, "", "  ")
	if err := os.MkdirAll(s.cfg.DataDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(s.emailConfigPath(), b, 0o600)
}

func (s *Server) mailConfig(c emailConfig) mailimport.Config {
	return mailimport.Config{Addr: s.imapAddr, Insecure: s.imapInsecure, Email: strings.TrimSpace(c.Email), Password: strings.ReplaceAll(c.Password, " ", ""), Sender: c.Sender}
}

func (s *Server) emailSince(c emailConfig) time.Time {
	if t, err := time.ParseInLocation("2006-01-02", c.Since, s.cfg.Location); err == nil {
		return t
	}
	return time.Now().In(s.cfg.Location).AddDate(0, 0, -3)
}

// emailResult merangkum satu putaran pemeriksaan email.
type emailResult struct {
	Checked int `json:"checked"`
	Saved   int `json:"saved"`
	Skipped int `json:"skipped"`
	Failed  int `json:"failed"`
}

// processEmails memeriksa Gmail dan mencatat transaksi dari email baru.
// Setiap email diproses sekali (dicatat di email_imports) apa pun hasilnya.
func (s *Server) processEmails(ctx context.Context) (emailResult, error) {
	s.emailMu.Lock()
	defer s.emailMu.Unlock()
	var res emailResult
	c := s.loadEmailConfig()
	if !c.Enabled || c.Email == "" || c.Password == "" {
		return res, nil
	}
	msgs, err := mailimport.Fetch(s.mailConfig(c), s.emailSince(c), 50)
	now := time.Now().In(s.cfg.Location).Format("2006-01-02 15:04")
	s.store.SetSetting(ctx, "email_last_check", now)
	if err != nil {
		s.store.SetSetting(ctx, "email_last_error", err.Error())
		return res, err
	}
	s.store.SetSetting(ctx, "email_last_error", "")
	since := s.emailSince(c)
	lctx := store.WithLedger(ctx, c.LedgerID)
	for _, m := range msgs {
		if !m.Date.IsZero() && m.Date.Before(since) || s.store.HasEmailImport(ctx, m.MessageID) {
			continue
		}
		res.Checked++
		rec := store.EmailImport{LedgerID: c.LedgerID, MessageID: m.MessageID, Sender: m.From, Subject: m.Subject, ReceivedAt: m.Date.In(s.cfg.Location).Format("2006-01-02 15:04")}
		saved, note, err := s.importEmail(lctx, c, m)
		switch {
		case err != nil:
			rec.Status, rec.Note = "gagal", err.Error()
			res.Failed++
		case len(saved) == 0:
			rec.Status, rec.Note = "dilewati", note
			res.Skipped++
		default:
			ids := make([]string, len(saved))
			for i, t := range saved {
				ids[i] = strconv.FormatInt(t.ID, 10)
			}
			rec.Status, rec.TxIDs, rec.Note = "tersimpan", strings.Join(ids, ","), note
			res.Saved += len(saved)
		}
		if err := s.store.AddEmailImport(ctx, rec); err != nil {
			return res, err
		}
	}
	if res.Saved > 0 {
		log.Printf("email: %d transaksi baru dari %d email", res.Saved, res.Checked)
	}
	return res, nil
}

// importEmail mengubah satu email menjadi transaksi: AI bila aktif, selain
// itu (atau bila AI gagal) pembaca email tanpa AI. Duplikat dilewati.
func (s *Server) importEmail(ctx context.Context, c emailConfig, m mailimport.Message) ([]store.Transaction, string, error) {
	text := "Subjek: " + m.Subject + "\nWaktu email: " + m.Date.In(s.cfg.Location).Format("2006-01-02 15:04") + "\n\n" + m.Text
	if len(text) > 8000 {
		text = text[:8000]
	}
	var drafts []ai.Draft
	note := ""
	if s.aiOn(ctx) {
		r, err := s.ai.Extract(ctx, ai.ExtractInput{Text: text, AccountID: c.AccountID, Kind: "email"})
		if err == nil {
			drafts = r.Drafts
			note = "dibaca AI"
		} else {
			note = "AI gagal (" + err.Error() + "), dibaca tanpa AI"
		}
	}
	if len(drafts) == 0 {
		d, err := ai.ParseEmailOffline(ctx, s.store, m.Subject, m.Text, m.Date, c.AccountID, "email")
		if err != nil {
			return nil, note, err
		}
		drafts = d
		if note == "" {
			note = "dibaca tanpa AI"
		}
	}
	var txs []store.Transaction
	dup := 0
	for _, d := range drafts {
		if d.Amount <= 0 {
			continue
		}
		t := d.Transaction()
		t.Source = "email"
		if s.store.FindDuplicate(ctx, t) != 0 {
			dup++
			continue
		}
		txs = append(txs, t)
	}
	if len(txs) == 0 {
		if dup > 0 {
			return nil, "sudah tercatat sebelumnya", nil
		}
		return nil, "tidak ditemukan transaksi di email ini", nil
	}
	saved, err := s.store.CreateTransactions(ctx, txs)
	if err != nil {
		return nil, note, err
	}
	return saved, note, nil
}

// RunEmailImport memeriksa email setiap 5 menit selama aplikasi berjalan.
func (s *Server) RunEmailImport(ctx context.Context) {
	tick := func() {
		if _, err := s.processEmails(ctx); err != nil {
			log.Printf("email: %v", err)
		}
	}
	tick()
	t := time.NewTicker(5 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			tick()
		}
	}
}

// ---------- handler ----------

func (s *Server) emailStatus(ctx context.Context) map[string]any {
	c := s.loadEmailConfig()
	last, _ := s.store.GetSetting(ctx, "email_last_check")
	lastErr, _ := s.store.GetSetting(ctx, "email_last_error")
	return map[string]any{
		"enabled": c.Enabled, "email": c.Email, "has_password": c.Password != "", "sender": c.Sender,
		"ledger_id": c.LedgerID, "account_id": c.AccountID, "since": s.emailSince(c).Format("2006-01-02"),
		"last_check": last, "last_error": lastErr,
	}
}

func (s *Server) handleEmailConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.emailStatus(r.Context()))
}

func (s *Server) handleSaveEmailConfig(w http.ResponseWriter, r *http.Request) {
	var in emailConfig
	if err := decode(r, &in); err != nil {
		fail(w, err)
		return
	}
	c := s.loadEmailConfig()
	c.Enabled = in.Enabled
	c.Email = strings.TrimSpace(in.Email)
	if strings.TrimSpace(in.Password) != "" { // kosong = pakai App Password yang sudah tersimpan
		c.Password = strings.ReplaceAll(strings.TrimSpace(in.Password), " ", "")
	}
	if strings.TrimSpace(in.Sender) != "" {
		c.Sender = strings.TrimSpace(in.Sender)
	}
	if in.LedgerID > 0 && s.store.ValidLedger(r.Context(), in.LedgerID) {
		c.LedgerID = in.LedgerID
	}
	c.AccountID = in.AccountID
	if c.AccountID != 0 {
		a, err := s.store.GetAccount(r.Context(), c.AccountID)
		if err != nil || a.LedgerID != c.LedgerID {
			fail(w, &store.ValidationError{Msg: "akun tujuan tidak ada di buku yang dipilih"})
			return
		}
	}
	if in.Since != "" {
		if _, err := time.Parse("2006-01-02", in.Since); err != nil {
			fail(w, &store.ValidationError{Msg: "tanggal mulai tidak valid"})
			return
		}
		c.Since = in.Since
	} else if c.Since == "" {
		c.Since = time.Now().In(s.cfg.Location).Format("2006-01-02")
	}
	if c.Enabled && (c.Email == "" || c.Password == "") {
		fail(w, &store.ValidationError{Msg: "isi alamat Gmail dan App Password dulu"})
		return
	}
	if err := s.saveEmailConfig(c); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, s.emailStatus(r.Context()))
}

func (s *Server) handleTestEmail(w http.ResponseWriter, r *http.Request) {
	c := s.loadEmailConfig()
	if c.Email == "" || c.Password == "" {
		writeJSON(w, 200, map[string]any{"ok": false, "message": "Isi alamat Gmail dan App Password, lalu simpan."})
		return
	}
	n, err := mailimport.Test(s.mailConfig(c), s.emailSince(c))
	if err != nil {
		writeJSON(w, 200, map[string]any{"ok": false, "message": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "message": fmt.Sprintf("Terhubung. %d email dari %q sejak %s.", n, c.Sender, s.emailSince(c).Format("2 Jan 2006"))})
}

func (s *Server) handleRunEmail(w http.ResponseWriter, r *http.Request) {
	res, err := s.processEmails(r.Context())
	if err != nil {
		fail(w, &store.ValidationError{Msg: err.Error()})
		return
	}
	writeJSON(w, 200, res)
}

func (s *Server) handleListEmailImports(w http.ResponseWriter, r *http.Request) {
	l, err := s.store.ListEmailImports(r.Context(), 50)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, l)
}

func (s *Server) handleDeleteEmailImport(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		fail(w, err)
		return
	}
	if err := s.store.DeleteEmailImport(r.Context(), id); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}
