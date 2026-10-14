package ocr

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ErrPDFPassword dikembalikan bila PDF terkunci dan kata sandi salah/kosong.
var ErrPDFPassword = errors.New("PDF terkunci kata sandi. Isi kata sandi PDF (mis. tanggal lahir untuk e-Statement BCA) lalu coba lagi")

// PDFAvailable melaporkan apakah poppler-utils terpasang.
func PDFAvailable() bool {
	_, err1 := exec.LookPath("pdftotext")
	_, err2 := exec.LookPath("pdftoppm")
	return err1 == nil && err2 == nil
}

// ReadPDF mengambil teks dari PDF. PDF berisi teks (e-Statement) dibaca
// langsung; PDF hasil scan diubah menjadi gambar lalu dibaca dengan OCR.
func ReadPDF(ctx context.Context, data []byte, password string) (string, error) {
	if !PDFAvailable() {
		return "", errors.New("pembaca PDF offline tidak tersedia (poppler-utils belum terpasang)")
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	dir, err := os.MkdirTemp("", "mymoney-pdf-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	in := filepath.Join(dir, "in.pdf")
	if err := os.WriteFile(in, data, 0o600); err != nil {
		return "", err
	}
	pw := []string{}
	if password != "" {
		pw = []string{"-upw", password, "-opw", password}
	}

	args := append(append([]string{"-layout"}, pw...), in, "-")
	text, err := run(ctx, "pdftotext", args...)
	if err != nil {
		return "", err
	}
	if len(strings.Fields(text)) >= 15 {
		return text, nil
	}

	// tidak ada lapisan teks: OCR tiap halaman (maks. 10 halaman)
	if !Available() {
		return "", ErrUnavailable
	}
	args = append(append([]string{"-r", "200", "-png", "-l", "10"}, pw...), in, filepath.Join(dir, "p"))
	if _, err := run(ctx, "pdftoppm", args...); err != nil {
		return "", err
	}
	pages, _ := filepath.Glob(filepath.Join(dir, "p*.png"))
	sort.Strings(pages)
	var b strings.Builder
	for _, p := range pages {
		img, err := os.ReadFile(p)
		if err != nil {
			return "", err
		}
		t, err := Read(ctx, img)
		if err != nil {
			return "", err
		}
		b.WriteString(t)
		b.WriteString("\n\n")
	}
	return b.String(), nil
}

func run(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		msg := stderr.String()
		if strings.Contains(strings.ToLower(msg), "password") || strings.Contains(msg, "Incorrect") {
			return "", ErrPDFPassword
		}
		return "", fmt.Errorf("gagal membaca PDF: %s", strings.TrimSpace(msg))
	}
	return out.String(), nil
}
