package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/ganova/mymoney/internal/store"
)

// findNPMCLI mencari program CLI: lokasi eksplisit, folder npm global (Windows),
// lalu PATH. Di Windows folder npm dicek dulu karena versi yang dipasang lewat
// npm lebih mudah diperbarui daripada salinan lama dari aplikasi desktop.
func findNPMCLI(name, explicit string) string {
	if explicit != "" {
		if p, err := exec.LookPath(explicit); err == nil {
			return p
		}
		return ""
	}
	if runtime.GOOS == "windows" {
		home := os.Getenv("USERPROFILE")
		for _, p := range []string{
			filepath.Join(os.Getenv("APPDATA"), "npm", name+".cmd"),
			filepath.Join(os.Getenv("APPDATA"), "npm", name+".exe"),
			filepath.Join(home, ".local", "bin", name+".exe"),
		} {
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	return ""
}

// FindClaude mencari Claude Code CLI (`claude`).
func FindClaude(explicit string) string { return findNPMCLI("claude", explicit) }

func (s *Service) claudeReady() bool { return s != nil && s.cfg.ClaudePath != "" }

// useAPI: API key Claude dipakai bila tersedia dan pengguna tidak memilih CLI.
func (s *Service) useAPI(ctx context.Context) bool {
	if !s.apiReady() {
		return false
	}
	pref := s.cliPreference(ctx)
	return pref == "auto" || pref == "api"
}

// cliPreference membaca pilihan "Sumber AI" dari pengaturan (auto|api|codex|claude).
func (s *Service) cliPreference(ctx context.Context) string {
	if s.store == nil {
		return "auto"
	}
	v, err := s.store.GetSetting(ctx, "ai_cli")
	if err != nil {
		return "auto"
	}
	switch v {
	case "api", "codex", "claude":
		return v
	}
	return "auto"
}

// pickCLI memilih program CLI yang dipakai: sesuai pilihan pengguna bila
// tersedia, selain itu yang terpasang (Codex lebih dulu). "" bila tidak ada.
func (s *Service) pickCLI(ctx context.Context) string {
	switch s.cliPreference(ctx) {
	case "codex":
		if s.codexReady() {
			return "codex"
		}
		return ""
	case "claude":
		if s.claudeReady() {
			return "claude"
		}
		return ""
	}
	if s.codexReady() {
		return "codex"
	}
	if s.claudeReady() {
		return "claude"
	}
	return ""
}

// CLIName menyebut jalur AI yang aktif untuk tampilan: API Claude, Codex CLI, Claude CLI, atau kosong.
func (s *Service) CLIName(ctx context.Context) string {
	if s.useAPI(ctx) {
		return "API Claude"
	}
	switch s.pickCLI(ctx) {
	case "codex":
		return "Codex CLI"
	case "claude":
		return "Claude CLI"
	}
	return ""
}

// Status merangkum ketersediaan jalur AI untuk halaman pengaturan.
type Status struct {
	Active      string `json:"active"`       // nama jalur yang dipakai, kosong bila tidak ada
	APIKey      bool   `json:"api_key"`      // ANTHROPIC_API_KEY terisi
	CodexFound  bool   `json:"codex_found"`  // Codex CLI terpasang
	ClaudeFound bool   `json:"claude_found"` // Claude CLI terpasang
}

// Status mengembalikan ketersediaan semua jalur AI.
func (s *Service) Status(ctx context.Context) Status {
	return Status{Active: s.CLIName(ctx), APIKey: s.apiReady(), CodexFound: s.codexReady(), ClaudeFound: s.claudeReady()}
}

// runCLI menjalankan prompt lewat CLI yang dipilih (Codex atau Claude) dan
// mengembalikan JSON sesuai skema.
func (s *Service) runCLI(ctx context.Context, prompt string, schema map[string]any, imgs []Image) ([]byte, error) {
	switch s.pickCLI(ctx) {
	case "codex":
		return s.runCodex(ctx, prompt, schema, imgs)
	case "claude":
		return s.runClaude(ctx, prompt, schema, imgs)
	}
	return nil, ErrDisabled
}

// runClaude menjalankan `claude -p` sekali jalan. Gambar disimpan ke folder
// sementara dan dibaca lewat alat Read (satu-satunya alat yang diizinkan).
func (s *Service) runClaude(ctx context.Context, prompt string, schema map[string]any, imgs []Image) ([]byte, error) {
	dir, err := os.MkdirTemp("", "mymoney-claude-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	sb, _ := json.Marshal(schema)
	full := prompt + "\n\nBalas HANYA dengan satu objek JSON valid yang sesuai skema berikut, tanpa teks lain dan tanpa pagar kode.\nSkema: " + string(sb)
	args := []string{"-p", "--output-format", "text"}
	if len(imgs) > 0 {
		exts := map[string]string{"image/png": ".png", "image/webp": ".webp", "image/gif": ".gif"}
		var list strings.Builder
		for i, img := range imgs {
			ext := exts[img.MediaType]
			if ext == "" {
				ext = ".jpg"
			}
			p := filepath.Join(dir, fmt.Sprintf("gambar%d%s", i+1, ext))
			if err := os.WriteFile(p, img.Data, 0o600); err != nil {
				return nil, err
			}
			fmt.Fprintf(&list, "- %s\n", p)
		}
		full = prompt + "\n\nGambar yang harus dibaca (buka SETIAP berkas dengan alat Read, lalu ekstrak datanya; jangan membaca berkas lain):\n" + list.String() +
			"\nBalas HANYA dengan satu objek JSON valid yang sesuai skema berikut, tanpa teks lain dan tanpa pagar kode.\nSkema: " + string(sb)
		args = append(args, "--allowedTools", "Read")
	} else {
		args = append(args, "--disallowedTools", "Bash", "Edit", "Write", "NotebookEdit", "WebFetch", "WebSearch", "Read", "Glob", "Grep")
	}
	if s.cfg.ClaudeModel != "" {
		args = append(args, "--model", s.cfg.ClaudeModel)
	}

	ctx, cancel := context.WithTimeout(ctx, 4*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, s.cfg.ClaudePath, args...)
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(full)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	hideWindow(cmd)
	start := time.Now()
	if err := cmd.Run(); err != nil {
		return nil, s.logErr(ctx, "claude", start, friendlyClaudeError(ctx, err, stderr.String()+"\n"+stdout.String()))
	}
	s.store.LogAI(ctx, store.AILog{Kind: "claude", Model: "Claude CLI", DurationMS: time.Since(start).Milliseconds()})

	out := stdout.String()
	i, j := strings.Index(out, "{"), strings.LastIndex(out, "}")
	if i < 0 || j < i {
		return nil, fmt.Errorf("jawaban Claude CLI tidak berisi JSON. Coba lagi")
	}
	return []byte(out[i : j+1]), nil
}

// friendlyClaudeError mengubah keluaran `claude` yang panjang menjadi satu kalimat.
func friendlyClaudeError(ctx context.Context, err error, output string) error {
	low := strings.ToLower(output)
	var ee *exec.Error
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return errors.New("Claude CLI terlalu lama merespons")
	case errors.As(err, &ee):
		return errors.New("Claude CLI tidak ditemukan. Pasang: npm i -g @anthropic-ai/claude-code")
	case strings.Contains(low, "/login") || strings.Contains(low, "not logged in") || strings.Contains(low, "invalid api key") || strings.Contains(low, "authentication"):
		return errors.New("Claude CLI belum login (jalankan: claude, lalu ketik /login)")
	case strings.Contains(low, "usage limit") || strings.Contains(low, "rate limit") || strings.Contains(low, "limit reached"):
		return errors.New("kuota Claude (langganan) sedang habis atau dibatasi, coba lagi nanti")
	case strings.Contains(low, "network") || strings.Contains(low, "enotfound") || strings.Contains(low, "econnrefused") || strings.Contains(low, "timed out"):
		return errors.New("Claude CLI tidak bisa terhubung ke internet")
	}
	last := ""
	for _, l := range strings.Split(output, "\n") {
		if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(l, "at ") {
			last = l
		}
	}
	if len(last) > 160 {
		last = last[:160] + "…"
	}
	if last == "" {
		return fmt.Errorf("Claude CLI gagal (%v)", err)
	}
	return fmt.Errorf("Claude CLI gagal: %s", last)
}
