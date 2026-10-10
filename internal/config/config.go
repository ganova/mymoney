// Package config membaca konfigurasi dari environment variable.
package config

import (
	"os"
	"strings"
	"time"
)

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
	loc, err := time.LoadLocation(env("TZ", "Asia/Jakarta"))
	if err != nil {
		loc = time.FixedZone("WIB", 7*3600)
	}
	port := env("PORT", "8080")
	return Config{
		Addr:             env("ADDR", ":"+port),
		DataDir:          env("DATA_DIR", "./data"),
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
