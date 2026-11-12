// MyMoney: aplikasi keuangan pribadi dengan AI.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"
	_ "time/tzdata" // zona waktu tetap tersedia di image minimal

	"github.com/ganova/mymoney/internal/ai"
	"github.com/ganova/mymoney/internal/config"
	"github.com/ganova/mymoney/internal/server"
	"github.com/ganova/mymoney/internal/store"
	"github.com/ganova/mymoney/web"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("mymoney ")
	cfg := config.Load()
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		healthcheck(cfg)
		return
	}

	if runtime.GOOS == "windows" {
		// versi Windows berjalan tanpa jendela: tulis log ke file
		os.MkdirAll(cfg.DataDir, 0o755)
		if f, err := os.OpenFile(filepath.Join(cfg.DataDir, "mymoney.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
			log.SetOutput(f)
		}
	}
	st, err := store.Open(cfg.DataDir, cfg.Location)
	if err != nil {
		log.Fatalf("membuka database: %v", err)
	}
	defer st.Close()

	// Kedua CLI dicari sekali di awal; pilihan yang dipakai diatur di Pengaturan → AI.
	apiKey := cfg.AnthropicKey
	codexPath := ai.FindCodex(cfg.CodexPath)
	claudePath := ai.FindClaude(cfg.ClaudePath)
	switch cfg.AIProvider {
	case "offline":
		apiKey, codexPath, claudePath = "", "", ""
	case "codex":
		apiKey, claudePath = "", ""
		if codexPath == "" {
			log.Printf("AI_PROVIDER=codex tetapi Codex CLI tidak ditemukan (npm i -g @openai/codex); memakai mode offline")
		}
	case "claude":
		apiKey, codexPath = "", ""
		if claudePath == "" {
			log.Printf("AI_PROVIDER=claude tetapi Claude CLI tidak ditemukan (npm i -g @anthropic-ai/claude-code); memakai mode offline")
		}
	case "api":
		codexPath, claudePath = "", ""
	}
	if codexPath != "" {
		log.Printf("Codex CLI terdeteksi: %s", codexPath)
	}
	if claudePath != "" {
		log.Printf("Claude CLI terdeteksi: %s", claudePath)
	}
	aiSvc := ai.New(ai.Config{
		CodexPath: codexPath, CodexModel: cfg.CodexModel,
		ClaudePath: claudePath, ClaudeModel: cfg.ClaudeModel,
		APIKey: apiKey, BaseURL: cfg.AnthropicBaseURL, Model: cfg.AIModel,
		Effort: cfg.AIEffort, VisionEffort: cfg.AIVisionEffort, Fallbacks: cfg.AIFallbacks,
	}, st)

	srv, err := server.New(cfg, st, aiSvc, web.Dist())
	if err != nil {
		log.Fatalf("menyiapkan server: %v", err)
	}

	if cfg.Password == "" {
		log.Printf("PERINGATAN: APP_PASSWORD kosong — aplikasi terbuka tanpa login. Isi APP_PASSWORD bila bisa diakses dari luar rumah.")
	}
	if aiSvc.Enabled() {
		log.Printf("AI aktif (model %s)", cfg.AIModel)
	} else {
		log.Printf("AI belum aktif — isi ANTHROPIC_API_KEY untuk fitur AI. Parser offline tetap bisa dipakai.")
	}

	bctx, stopBackups := context.WithCancel(context.Background())
	defer stopBackups()
	go srv.RunBackups(bctx, cfg.BackupKeep, cfg.BackupEvery)

	httpSrv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srv,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	go func() {
		log.Printf("berjalan di http://localhost%s (data: %s, zona waktu: %s)", cfg.Addr, cfg.DataDir, cfg.Location)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	httpSrv.Shutdown(ctx)
	log.Printf("berhenti")
}

// healthcheck dipakai HEALTHCHECK Docker: keluar 0 bila server menjawab.
func healthcheck(cfg config.Config) {
	c := http.Client{Timeout: 3 * time.Second}
	res, err := c.Get("http://127.0.0.1" + cfg.Addr + "/api/health")
	if err != nil || res.StatusCode != http.StatusOK {
		os.Exit(1)
	}
}
