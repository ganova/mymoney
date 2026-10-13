// Package ocr membaca teks dari gambar memakai Tesseract (gratis, offline).
package ocr

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// ErrUnavailable dikembalikan bila program tesseract tidak terpasang.
var ErrUnavailable = errors.New("OCR offline tidak tersedia (tesseract belum terpasang)")

// Available melaporkan apakah tesseract bisa dipakai.
func Available() bool {
	_, err := exec.LookPath("tesseract")
	return err == nil
}

// Read mengubah gambar menjadi teks (bahasa Indonesia + Inggris).
func Read(ctx context.Context, img []byte) (string, error) {
	if !Available() {
		return "", ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	// psm 6: blok teks seragam — cocok untuk daftar mutasi
	cmd := exec.CommandContext(ctx, "tesseract", "stdin", "stdout", "-l", "ind+eng", "--psm", "6")
	cmd.Stdin = bytes.NewReader(img)
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("OCR gagal: %v %s", err, strings.TrimSpace(stderr.String()))
	}
	return out.String(), nil
}
