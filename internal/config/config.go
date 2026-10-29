// Package config membaca konfigurasi dari environment variable.
package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// loadEnvFile membaca file .env (KEY=VALUE) di samping program, untuk
// instalasi tanpa Docker. Variabel yang sudah ada di environment tidak ditimpa.
func loadEnvFile() {
	dirs := []string{"."}
	if exe, err := os.Executable(); err == nil {
		dirs = append([]string{filepath.Dir(exe)}, dirs...)
	}
	for _, d := range dirs {
		f, err := os.Open(filepath.Join(d, ".env"))
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			k, v, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}
			k, v = strings.TrimSpace(k), strings.Trim(strings.TrimSpace(v), `"'`)
			if _, exists := os.LookupEnv(k); !exists {
				os.Setenv(k, v)
			}
		}
		f.Close()
		return
	}
}

// defaultDataDir: folder "data" di samping program (Windows), atau ./data.
func defaultDataDir() string {
	if exe, err := os.Executable(); err == nil && !strings.Contains(exe, "go-build") {
		return filepath.Join(filepath.Dir(exe), "data")
	}
	return "./data"
}

// Config adalah konfigurasi aplikasi.
type Config struct {
	Addr          string
	DataDir       string
	Password      string // kosong = tanpa login (hanya untuk jaringan pribadi)
	SessionSecret string
	Location      *time.Location
	SecureCookie  bool

	AnthropicKey     string
	AnthropicBaseURL string
	AIModel          string
	AIEffort         string
	AIVisionEffort   string
	AIFallbacks      bool
}

func env(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

// Load membaca konfigurasi.
func Load() Config {
	loadEnvFile()
	loc, err := time.LoadLocation(env("TZ", "Asia/Jakarta"))
	if err != nil {
		loc = time.FixedZone("WIB", 7*3600)
	}
	port := env("PORT", "7428")
	return Config{
		Addr:             env("ADDR", ":"+port),
		DataDir:          env("DATA_DIR", defaultDataDir()),
		Password:         os.Getenv("APP_PASSWORD"),
		SessionSecret:    os.Getenv("SESSION_SECRET"),
		Location:         loc,
		SecureCookie:     env("SECURE_COOKIE", "auto") == "true",
		AnthropicKey:     os.Getenv("ANTHROPIC_API_KEY"),
		AnthropicBaseURL: os.Getenv("ANTHROPIC_BASE_URL"),
		AIModel:          env("AI_MODEL", "claude-opus-5-5"),
		AIEffort:         env("AI_EFFORT", "low"),
		AIVisionEffort:   env("AI_VISION_EFFORT", "medium"),
		AIFallbacks:      env("AI_FALLBACKS", "true") != "false",
	}
}
