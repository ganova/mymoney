// Package ocr membaca teks dari gambar memakai Tesseract (gratis, offline).
package ocr

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// ErrUnavailable dikembalikan bila program tesseract tidak terpasang.
var ErrUnavailable = errors.New("OCR offline tidak tersedia (tesseract belum terpasang)")

// find mencari program di PATH, variabel env, atau lokasi instal umum Windows.
func find(name, envKey string, winDirs ...string) string {
	if p := os.Getenv(envKey); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	if runtime.GOOS == "windows" {
		for _, d := range winDirs {
			matches, _ := filepath.Glob(filepath.Join(d, name+".exe"))
			if len(matches) > 0 {
				return matches[len(matches)-1]
			}
		}
	}
	return ""
}

func tesseractPath() string {
	return find("tesseract", "TESSERACT_PATH",
		`C:\Program Files\Tesseract-OCR`, `C:\Program Files (x86)\Tesseract-OCR`,
		filepath.Join(os.Getenv("LOCALAPPDATA"), `Programs\Tesseract-OCR`))
}

// Available melaporkan apakah tesseract bisa dipakai.
func Available() bool { return tesseractPath() != "" }

// langs memilih bahasa yang terpasang (ind+eng bila ada).
func langs(bin string) string {
	out, err := exec.Command(bin, "--list-langs").CombinedOutput()
	if err != nil {
		return "eng"
	}
	have := map[string]bool{}
	for _, l := range strings.Fields(string(out)) {
		have[l] = true
	}
	var ls []string
	for _, l := range []string{"ind", "eng"} {
		if have[l] {
			ls = append(ls, l)
		}
	}
	if len(ls) == 0 {
		return "eng"
	}
	return strings.Join(ls, "+")
}

// Read mengubah gambar menjadi teks (bahasa Indonesia + Inggris).
func Read(ctx context.Context, img []byte) (string, error) {
	return read(ctx, img, "6")
}

// ReadTable membaca tampilan tabel/daftar berkolom (mis. daftar pesanan).
// Gambar kecil diperbesar dulu, lalu dibaca dengan dua mode tata letak
// (per baris dan per blok) karena hasil terbaiknya berbeda antar gambar;
// pemanggil memilih tafsiran yang paling masuk akal.
func ReadTable(ctx context.Context, img []byte) ([]string, error) {
	big := upscale(img)
	var out []string
	var firstErr error
	for _, psm := range []string{"6", "4"} {
		t, err := read(ctx, big, psm)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		out = append(out, t)
	}
	if len(out) == 0 {
		return nil, firstErr
	}
	return out, nil
}

func read(ctx context.Context, img []byte, psm string) (string, error) {
	bin := tesseractPath()
	if bin == "" {
		return "", ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	// psm 6: blok teks seragam — cocok untuk daftar mutasi
	cmd := exec.CommandContext(ctx, bin, "stdin", "stdout", "-l", langs(bin), "--psm", psm)
	hideWindow(cmd)
	// satu thread per proses: beberapa OCR berjalan bersamaan tanpa saling memperlambat
	cmd.Env = append(os.Environ(), "OMP_THREAD_LIMIT=1")
	cmd.Stdin = bytes.NewReader(img)
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("OCR gagal: %v %s", err, strings.TrimSpace(stderr.String()))
	}
	return out.String(), nil
}
