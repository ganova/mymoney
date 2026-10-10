// Package ai berisi fitur kecerdasan buatan MyMoney yang ditenagai Claude:
// mencatat transaksi dari kalimat bebas, membaca screenshot mutasi bank,
// menjawab pertanyaan keuangan, menyusun budget, dan memberi analisis.
package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/ganova/mymoney/internal/store"
)

// Config adalah konfigurasi layanan AI.
type Config struct {
	APIKey       string
	BaseURL      string
	Model        string // model utama, default claude-opus-5-5
	Effort       string // effort untuk asisten/catat cepat (default low)
	VisionEffort string // effort untuk screenshot/impor (default medium)
	Fallbacks    bool   // aktifkan server-side fallback saat ditolak classifier
}

// Service adalah gerbang ke Claude API.
type Service struct {
	cfg    Config
	client anthropic.Client
	store  *store.Store
}

// ErrDisabled dikembalikan bila API key belum diatur.
var ErrDisabled = errors.New("fitur AI belum aktif: isi ANTHROPIC_API_KEY di file .env")

// New membuat layanan AI. Bila APIKey kosong, layanan tetap dibuat tetapi
// Enabled() bernilai false dan aplikasi memakai parser offline.
func New(cfg Config, st *store.Store) *Service {
	if cfg.Model == "" {
		cfg.Model = "claude-opus-5-5"
	}
	if cfg.Effort == "" {
		cfg.Effort = "low"
	}
	if cfg.VisionEffort == "" {
		cfg.VisionEffort = "medium"
	}
	opts := []option.RequestOption{
		option.WithAPIKey(cfg.APIKey),
		option.WithRequestTimeout(4 * time.Minute),
		option.WithMaxRetries(2),
	}
	if cfg.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(cfg.BaseURL))
	}
	return &Service{cfg: cfg, client: anthropic.NewClient(opts...), store: st}
}

// Enabled melaporkan apakah API key tersedia.
func (s *Service) Enabled() bool { return s != nil && s.cfg.APIKey != "" }

// Model mengembalikan nama model yang dipakai.
func (s *Service) Model() string { return s.cfg.Model }

// supportsFallback: server-side fallback tersedia untuk keluarga Opus 5.x,
// Sonnet 5.5, dan Fable; tidak untuk Haiku.
func supportsFallback(model string) bool {
	return strings.HasPrefix(model, "claude-opus-5") || strings.HasPrefix(model, "claude-sonnet-5-5") || strings.HasPrefix(model, "claude-fable-5")
}

// baseParams menyiapkan parameter permintaan yang sama untuk semua fitur.
func (s *Service) baseParams(effort string, maxTokens int64) anthropic.BetaMessageNewParams {
	p := anthropic.BetaMessageNewParams{
		Model:     anthropic.Model(s.cfg.Model),
		MaxTokens: maxTokens,
		OutputConfig: anthropic.BetaOutputConfigParam{
			Effort: anthropic.BetaOutputConfigEffort(effort),
		},
	}
	if s.cfg.Fallbacks && supportsFallback(s.cfg.Model) {
		p.Betas = append(p.Betas, anthropic.AnthropicBetaServerSideFallback2026_07_01)
		p.Fallbacks = anthropic.BetaFallbacksParamOfDefault()
	}
	return p
}

// send mengirim permintaan (streaming agar aman untuk keluaran panjang),
// mencatat pemakaian token, dan memeriksa stop reason.
func (s *Service) send(ctx context.Context, kind string, p anthropic.BetaMessageNewParams) (*anthropic.BetaMessage, error) {
	if !s.Enabled() {
		return nil, ErrDisabled
	}
	start := time.Now()
	stream := s.client.Beta.Messages.NewStreaming(ctx, p)
	msg := anthropic.BetaMessage{}
	for stream.Next() {
		if err := msg.Accumulate(stream.Current()); err != nil {
			stream.Close()
			return nil, s.logErr(ctx, kind, start, err)
		}
	}
	if err := stream.Err(); err != nil {
		return nil, s.logErr(ctx, kind, start, err)
	}
	model := string(msg.Model)
	if model == "" {
		model = s.cfg.Model
	}
	s.store.LogAI(ctx, store.AILog{
		Kind: kind, Model: model,
		InputTokens: msg.Usage.InputTokens, OutputTokens: msg.Usage.OutputTokens,
		CacheReadTokens: msg.Usage.CacheReadInputTokens, CacheWriteTokens: msg.Usage.CacheCreationInputTokens,
		DurationMS: time.Since(start).Milliseconds(),
	})
	switch msg.StopReason {
	case anthropic.BetaStopReasonRefusal:
		return nil, errors.New("AI menolak memproses permintaan ini. Coba ubah kalimatnya")
	case anthropic.BetaStopReasonMaxTokens:
		return nil, errors.New("jawaban AI terpotong karena terlalu panjang. Coba kirim data yang lebih sedikit")
	}
	return &msg, nil
}

func (s *Service) logErr(ctx context.Context, kind string, start time.Time, err error) error {
	s.store.LogAI(ctx, store.AILog{Kind: kind, Model: s.cfg.Model, DurationMS: time.Since(start).Milliseconds(), Error: err.Error()})
	log.Printf("ai %s: %v", kind, err)
	return friendlyError(err)
}

// friendlyError menerjemahkan galat API menjadi pesan yang bisa dipahami.
func friendlyError(err error) error {
	var apierr *anthropic.Error
	if errors.As(err, &apierr) {
		switch {
		case apierr.StatusCode == http.StatusUnauthorized:
			return errors.New("API key Anthropic tidak valid. Periksa ANTHROPIC_API_KEY")
		case apierr.StatusCode == http.StatusTooManyRequests:
			return errors.New("batas pemakaian AI tercapai, coba lagi sebentar lagi")
		case apierr.StatusCode == http.StatusBadRequest:
			return fmt.Errorf("permintaan AI ditolak: %s", apierr.Error())
		case apierr.StatusCode >= 500:
			return errors.New("layanan AI sedang sibuk, coba lagi")
		}
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return errors.New("AI terlalu lama merespons, coba lagi")
	}
	return fmt.Errorf("gagal menghubungi AI: %w", err)
}

// textOf menggabungkan seluruh blok teks dalam respons.
func textOf(msg *anthropic.BetaMessage) string {
	var b strings.Builder
	for _, block := range msg.Content {
		if t, ok := block.AsAny().(anthropic.BetaTextBlock); ok {
			b.WriteString(t.Text)
		}
	}
	return strings.TrimSpace(b.String())
}

// decodeJSON membaca keluaran structured output.
func decodeJSON(msg *anthropic.BetaMessage, v any) error {
	raw := textOf(msg)
	if err := json.Unmarshal([]byte(raw), v); err != nil {
		return fmt.Errorf("format jawaban AI tidak terbaca: %w", err)
	}
	return nil
}

func jsonFormat(schema map[string]any) anthropic.BetaJSONOutputFormatParam {
	return anthropic.BetaJSONOutputFormatParam{Schema: schema}
}

func userText(text string) anthropic.BetaMessageParam {
	return anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock(text))
}
