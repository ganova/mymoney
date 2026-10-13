// MyMoney: aplikasi keuangan pribadi dengan AI.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
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

	st, err := store.Open(cfg.DataDir, cfg.Location)
	if err != nil {
		log.Fatalf("membuka database: %v", err)
	}
	defer st.Close()

	aiSvc := ai.New(ai.Config{
		APIKey: cfg.AnthropicKey, BaseURL: cfg.AnthropicBaseURL, Model: cfg.AIModel,
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
