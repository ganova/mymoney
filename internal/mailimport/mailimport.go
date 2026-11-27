// Package mailimport membaca email notifikasi transaksi (mis. dari BCA) lewat
// IMAP, tanpa menandainya sudah dibaca.
package mailimport

import (
	"bytes"
	"errors"
	"fmt"
	"html"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	_ "github.com/emersion/go-message/charset" // charset selain UTF-8
	"github.com/emersion/go-message/mail"
)

// GmailAddr adalah server IMAP Gmail.
const GmailAddr = "imap.gmail.com:993"

// Config adalah pengaturan koneksi.
type Config struct {
	Addr     string // host:port, bawaan Gmail
	Insecure bool   // tanpa TLS (hanya untuk tes)
	Email    string
	Password string // App Password Google, bukan kata sandi biasa
	Sender   string // potongan alamat pengirim, mis. "bca.co.id"
}

// Message adalah satu email yang sudah diubah menjadi teks.
type Message struct {
	UID       uint32
	MessageID string
	From      string
	Subject   string
	Date      time.Time
	Text      string
}

func dial(cfg Config) (*imapclient.Client, error) {
	addr := cfg.Addr
	if addr == "" {
		addr = GmailAddr
	}
	var (
		c   *imapclient.Client
		err error
	)
	if cfg.Insecure {
		c, err = imapclient.DialInsecure(addr, nil)
	} else {
		c, err = imapclient.DialTLS(addr, nil)
	}
	if err != nil {
		return nil, fmt.Errorf("tidak bisa terhubung ke %s: %w", addr, err)
	}
	if err := c.Login(cfg.Email, cfg.Password).Wait(); err != nil {
		c.Close()
		return nil, friendlyLogin(err)
	}
	return c, nil
}

func friendlyLogin(err error) error {
	low := strings.ToLower(err.Error())
	switch {
	case strings.Contains(low, "application-specific password") || strings.Contains(low, "app password"):
		return errors.New("Gmail meminta App Password: buat di myaccount.google.com/apppasswords (perlu Verifikasi 2 Langkah)")
	case strings.Contains(low, "invalid credentials") || strings.Contains(low, "authenticationfailed") || strings.Contains(low, "authentication failed"):
		return errors.New("login Gmail ditolak: periksa alamat email dan App Password (16 huruf, bukan kata sandi biasa)")
	case strings.Contains(low, "imap") && strings.Contains(low, "disabled"):
		return errors.New("IMAP belum aktif di Gmail: Setelan → Lihat semua setelan → Penerusan dan POP/IMAP → Aktifkan IMAP")
	}
	return fmt.Errorf("login Gmail gagal: %w", err)
}

// mailboxAll memilih folder "Semua Email" Gmail (memuat email yang diarsipkan
// atau masuk tab Promosi) bila ada; selain itu INBOX.
func mailboxAll(c *imapclient.Client) string {
	list, err := c.List("", "*", nil).Collect()
	if err != nil {
		return "INBOX"
	}
	for _, mb := range list {
		for _, a := range mb.Attrs {
			if a == imap.MailboxAttrAll {
				return mb.Mailbox
			}
		}
	}
	return "INBOX"
}

// Test login dan menghitung email dari pengirim sejak since.
func Test(cfg Config, since time.Time) (int, error) {
	c, err := dial(cfg)
	if err != nil {
		return 0, err
	}
	defer closeClient(c)
	if _, err := c.Select(mailboxAll(c), &imap.SelectOptions{ReadOnly: true}).Wait(); err != nil {
		return 0, fmt.Errorf("membuka kotak surat: %w", err)
	}
	data, err := c.UIDSearch(criteria(cfg, since), nil).Wait()
	if err != nil {
		return 0, fmt.Errorf("mencari email: %w", err)
	}
	return len(data.AllUIDs()), nil
}

func criteria(cfg Config, since time.Time) *imap.SearchCriteria {
	sc := &imap.SearchCriteria{Since: since}
	if s := strings.TrimSpace(cfg.Sender); s != "" {
		sc.Header = []imap.SearchCriteriaHeaderField{{Key: "From", Value: s}}
	}
	return sc
}

// Fetch mengambil email dari pengirim sejak since (maksimal limit terbaru),
// dalam mode baca-saja sehingga status "belum dibaca" di Gmail tidak berubah.
func Fetch(cfg Config, since time.Time, limit int) ([]Message, error) {
	c, err := dial(cfg)
	if err != nil {
		return nil, err
	}
	defer closeClient(c)
	if _, err := c.Select(mailboxAll(c), &imap.SelectOptions{ReadOnly: true}).Wait(); err != nil {
		return nil, fmt.Errorf("membuka kotak surat: %w", err)
	}
	data, err := c.UIDSearch(criteria(cfg, since), nil).Wait()
	if err != nil {
		return nil, fmt.Errorf("mencari email: %w", err)
	}
	uids := data.AllUIDs()
	if len(uids) == 0 {
		return nil, nil
	}
	if limit > 0 && len(uids) > limit {
		uids = uids[len(uids)-limit:]
	}
	section := &imap.FetchItemBodySection{Peek: true}
	msgs, err := c.Fetch(imap.UIDSetNum(uids...), &imap.FetchOptions{UID: true, Envelope: true, BodySection: []*imap.FetchItemBodySection{section}}).Collect()
	if err != nil {
		return nil, fmt.Errorf("mengambil email: %w", err)
	}
	var out []Message
	for _, m := range msgs {
		raw := m.FindBodySection(section)
		msg, err := Parse(raw)
		if err != nil {
			continue
		}
		msg.UID = uint32(m.UID)
		if m.Envelope != nil {
			if msg.Subject == "" {
				msg.Subject = m.Envelope.Subject
			}
			if msg.MessageID == "" {
				msg.MessageID = m.Envelope.MessageID
			}
			if msg.Date.IsZero() {
				msg.Date = m.Envelope.Date
			}
		}
		if msg.MessageID == "" {
			msg.MessageID = fmt.Sprintf("uid-%d-%d", m.UID, msg.Date.Unix())
		}
		out = append(out, msg)
	}
	return out, nil
}

// Parse mengubah email mentah (RFC 5322) menjadi teks: bagian text/plain bila
// ada, selain itu text/html yang sudah dibersihkan dari tag.
func Parse(raw []byte) (Message, error) {
	var m Message
	mr, err := mail.CreateReader(bytes.NewReader(raw))
	if err != nil {
		return m, err
	}
	m.Subject, _ = mr.Header.Subject()
	m.Date, _ = mr.Header.Date()
	m.MessageID, _ = mr.Header.MessageID()
	if from, err := mr.Header.AddressList("From"); err == nil && len(from) > 0 {
		m.From = from[0].Address
	}
	var plain, htm string
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
		h, ok := p.Header.(*mail.InlineHeader)
		if !ok {
			continue
		}
		ct, _, _ := h.ContentType()
		b, _ := io.ReadAll(io.LimitReader(p.Body, 1<<20))
		switch {
		case strings.HasPrefix(ct, "text/plain") && plain == "":
			plain = string(b)
		case strings.HasPrefix(ct, "text/html") && htm == "":
			htm = string(b)
		}
	}
	switch {
	case strings.TrimSpace(plain) != "":
		m.Text = tidy(plain)
	case htm != "":
		m.Text = HTMLToText(htm)
	}
	return m, nil
}

var (
	reDrop  = regexp.MustCompile(`(?is)<(style|script|head)[^>]*>.*?</(style|script|head)>`)
	reBreak = regexp.MustCompile(`(?i)<\s*(br|/p|/div|/tr|/li|/h[1-6]|/table)[^>]*>`)
	reCell  = regexp.MustCompile(`(?i)<\s*/t[dh]\s*>`)
	reTag   = regexp.MustCompile(`(?s)<[^>]+>`)
	reSpace = regexp.MustCompile(`[ \t\x{00a0}]+`)
	reLines = regexp.MustCompile(`\n\s*\n+`)
)

// HTMLToText membuang tag HTML: baris tabel menjadi baris teks dan sel
// dipisah spasi, sehingga "Nominal | IDR 25.000" tetap satu baris.
func HTMLToText(s string) string {
	s = reDrop.ReplaceAllString(s, " ")
	s = reBreak.ReplaceAllString(s, "\n")
	s = reCell.ReplaceAllString(s, "  ")
	s = reTag.ReplaceAllString(s, " ")
	return tidy(html.UnescapeString(s))
}

func tidy(s string) string {
	s = strings.ReplaceAll(s, "\r", "")
	var lines []string
	for _, l := range strings.Split(s, "\n") {
		l = strings.TrimSpace(reSpace.ReplaceAllString(l, " "))
		lines = append(lines, l)
	}
	return strings.TrimSpace(reLines.ReplaceAllString(strings.Join(lines, "\n"), "\n"))
}

// closeClient logout lalu menutup koneksi. (Catatan: "defer c.Logout().Wait()"
// salah karena c.Logout() langsung dieksekusi saat defer didaftarkan.)
func closeClient(c *imapclient.Client) {
	c.Logout().Wait()
	c.Close()
}
