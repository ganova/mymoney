package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/ganova/mymoney/internal/period"
)

// FindCodex mencari program Codex CLI: CODEX_PATH, PATH, atau folder npm global di Windows.
func FindCodex(explicit string) string {
	if explicit != "" {
		if p, err := exec.LookPath(explicit); err == nil {
			return p
		}
		return ""
	}
	// Di Windows, codex dari npm (-g) lebih mudah diperbarui daripada codex.exe
	// bawaan aplikasi desktop yang bisa tertinggal versinya, jadi dicek lebih dulu.
	if runtime.GOOS == "windows" {
		for _, base := range []string{os.Getenv("APPDATA"), filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Roaming")} {
			if base == "" {
				continue
			}
			for _, n := range []string{"codex.cmd", "codex.exe"} {
				p := filepath.Join(base, "npm", n)
				if _, err := os.Stat(p); err == nil {
					return p
				}
			}
		}
	}
	if p, err := exec.LookPath("codex"); err == nil {
		return p
	}
	return ""
}

func (s *Service) codexReady() bool { return s != nil && s.cfg.CodexPath != "" }

// runCodex menjalankan `codex exec` sekali jalan dan mengembalikan JSON jawabannya.
func (s *Service) runCodex(ctx context.Context, prompt string, schema map[string]any, imgs []Image) ([]byte, error) {
	dir, err := os.MkdirTemp("", "mymoney-codex-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	sb, _ := json.Marshal(schema)
	schemaFile, outFile := filepath.Join(dir, "schema.json"), filepath.Join(dir, "out.json")
	if err := os.WriteFile(schemaFile, sb, 0o600); err != nil {
		return nil, err
	}
	args := []string{"exec", "--skip-git-repo-check", "--ephemeral", "-s", "read-only", "--color", "never",
		"-C", dir, "--output-schema", schemaFile, "-o", outFile}
	if s.cfg.CodexModel != "" {
		args = append(args, "-m", s.cfg.CodexModel)
	}
	exts := map[string]string{"image/png": ".png", "image/webp": ".webp", "image/gif": ".gif"}
	for i, img := range imgs {
		ext := exts[img.MediaType]
		if ext == "" {
			ext = ".jpg"
		}
		p := filepath.Join(dir, fmt.Sprintf("gambar%d%s", i+1, ext))
		if err := os.WriteFile(p, img.Data, 0o600); err != nil {
			return nil, err
		}
		args = append(args, "-i", p)
	}

	ctx, cancel := context.WithTimeout(ctx, 4*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, s.cfg.CodexPath, args...)
	cmd.Stdin = strings.NewReader(prompt) // tanpa argumen prompt, Codex membaca stdin
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	hideWindow(cmd)
	start := time.Now()
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if len(msg) > 300 {
			msg = msg[len(msg)-300:]
		}
		err = fmt.Errorf("Codex CLI gagal (%v): %s. Pastikan sudah `codex login`", err, msg)
		return nil, s.logErr(ctx, "codex", start, err)
	}
	out, err := os.ReadFile(outFile)
	if err != nil || len(bytes.TrimSpace(out)) == 0 {
		return nil, fmt.Errorf("Codex CLI tidak memberi jawaban")
	}
	return out, nil
}

const codexAskPrompt = `Kamu adalah asisten keuangan pribadi di aplikasi MyMoney milik satu pengguna di Indonesia. Pengguna mengetik singkat, sering tanpa tanda baca dan campur bahasa gaul.
Jangan membaca atau menjalankan perintah apa pun; cukup jawab dari teks di bawah.

Tugas:
- Bila pesan adalah transaksi (mis. "beli makan siang 25rb", "tarik tunai 300rb", "gajian 11,2jt"), isi "transactions" dengan satu draf per transaksi dan "reply" berupa konfirmasi satu kalimat. Jangan bertanya balik; tebak dengan cerdas, pengguna akan mengecek draf.
- Bila pesan pertanyaan, jawab di "reply" memakai hanya data pengguna di bawah; jangan mengarang angka. "transactions" kosong.
- Untuk pemasukan isi income_source_id dari daftar sumber pemasukan (mis. CMW/Freelance) bila jelas, selain itu null.
- Nominal rupiah bulat positif ("25rb"=25000, "1,5jt"=1500000). date YYYY-MM-DD ("kemarin" dihitung dari hari ini). Tanpa tanggal = hari ini.
- Pilih account_id & category_id HANYA dari ID di daftar. Utamakan aturan & kebiasaan pengguna. Transfer antar akun sendiri (top up e-wallet, tarik tunai) = type transfer dengan to_account_id, category_id null.
- Jawab bahasa Indonesia, singkat.`

func codexAskSchema() map[string]any {
	return map[string]any{
		"type": "object", "additionalProperties": false,
		"required": []string{"reply", "transactions"},
		"properties": map[string]any{
			"reply":        map[string]any{"type": "string"},
			"transactions": map[string]any{"type": "array", "items": draftSchema(false)},
		},
	}
}

func (s *Service) codexAsk(ctx context.Context, history []HistoryItem, text, source string) (*AssistantResult, error) {
	c, err := LoadContext(ctx, s.store)
	if err != nil {
		return nil, err
	}
	if len(history) > 8 {
		history = history[len(history)-8:]
	}
	var b strings.Builder
	b.WriteString(codexAskPrompt + "\n\n# Data pengguna\n" + c.Describe() + "\n")
	if len(history) > 0 {
		b.WriteString("\n# Percakapan sebelumnya\n")
		for _, h := range history {
			if t := strings.TrimSpace(h.Text); t != "" {
				fmt.Fprintf(&b, "%s: %s\n", h.Role, t)
			}
		}
	}
	b.WriteString("\n# Pesan pengguna sekarang\n" + text + "\n")
	raw, err := s.runCodex(ctx, b.String(), codexAskSchema(), nil)
	if err != nil {
		return nil, err
	}
	var out struct {
		Reply        string  `json:"reply"`
		Transactions []Draft `json:"transactions"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("jawaban Codex tidak valid: %w", err)
	}
	res := &AssistantResult{Reply: out.Reply, Drafts: []Draft{}}
	for _, d := range out.Transactions {
		res.Drafts = append(res.Drafts, c.Sanitize(d, source))
	}
	MarkDuplicates(ctx, s.store, res.Drafts)
	return res, nil
}

// codexPrompt menyusun instruksi ekstraksi mutasi untuk Codex.
func (s *Service) codexExtractPrompt(c *Context, in ExtractInput, p period.Period) string {
	var b strings.Builder
	b.WriteString(extractPrompt)
	b.WriteString("\n\nJangan membaca atau menjalankan perintah apa pun. Gambar terlampir adalah sumber datanya.\n\n# Data pengguna\n" + c.Describe() + "\n")
	b.WriteString(s.extractInstr(c, in, p))
	if t := strings.TrimSpace(in.Text); t != "" {
		b.WriteString("\nSumber data (teks):\n<data>\n" + t + "\n</data>\n")
	}
	b.WriteString("\nEkstrak semua transaksinya sekarang.")
	return b.String()
}
