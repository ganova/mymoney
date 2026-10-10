package server

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ganova/mymoney/internal/config"
	"github.com/ganova/mymoney/internal/store"
)

const cookieName = "mm_session"
const sessionTTL = 365 * 24 * time.Hour // login sekali, awet di iPhone

func sessionSecret(cfg config.Config, st *store.Store) ([]byte, error) {
	if cfg.SessionSecret != "" {
		return []byte(cfg.SessionSecret), nil
	}
	ctx := context.Background()
	if v, err := st.GetSetting(ctx, "session_secret"); err == nil && v != "" {
		return hex.DecodeString(v)
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	if err := st.SetSetting(ctx, "session_secret", hex.EncodeToString(b)); err != nil {
		return nil, err
	}
	return b, nil
}

func (s *Server) authRequired() bool { return s.cfg.Password != "" }

// sign menghasilkan token sesi "expiry.signature". Kata sandi ikut ditandatangani
// sehingga mengganti APP_PASSWORD otomatis mengeluarkan semua sesi lama.
func (s *Server) sign(exp int64) string {
	mac := hmac.New(sha256.New, s.secret)
	fmt.Fprintf(mac, "%d|%s", exp, s.cfg.Password)
	return strconv.FormatInt(exp, 10) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (s *Server) validSession(r *http.Request) bool {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return false
	}
	expStr, _, ok := strings.Cut(c.Value, ".")
	if !ok {
		return false
	}
	exp, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil || time.Now().Unix() > exp {
		return false
	}
	return hmac.Equal([]byte(c.Value), []byte(s.sign(exp)))
}

func (s *Server) validBearer(r *http.Request) bool {
	h := r.Header.Get("Authorization")
	tok, ok := strings.CutPrefix(h, "Bearer ")
	if !ok {
		return false
	}
	return s.store.VerifyToken(r.Context(), strings.TrimSpace(tok))
}

func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.authRequired() || s.validSession(r) || s.validBearer(r) {
			next.ServeHTTP(w, r)
			return
		}
		writeError(w, http.StatusUnauthorized, "silakan login dulu")
	})
}

func (s *Server) secureRequest(r *http.Request) bool {
	if s.cfg.SecureCookie {
		return true
	}
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func (s *Server) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]bool{
		"auth_required": s.authRequired(),
		"authenticated": !s.authRequired() || s.validSession(r) || s.validBearer(r),
	})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if !s.limiter.allow(ip) {
		writeError(w, http.StatusTooManyRequests, "terlalu banyak percobaan, tunggu 1 menit")
		return
	}
	var in struct {
		Password string `json:"password"`
	}
	if err := decode(r, &in); err != nil {
		fail(w, err)
		return
	}
	if !s.authRequired() {
		writeJSON(w, 200, map[string]bool{"ok": true})
		return
	}
	if subtle.ConstantTimeCompare([]byte(in.Password), []byte(s.cfg.Password)) != 1 {
		s.limiter.fail(ip)
		writeError(w, http.StatusUnauthorized, "kata sandi salah")
		return
	}
	exp := time.Now().Add(sessionTTL).Unix()
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: s.sign(exp), Path: "/", HttpOnly: true,
		SameSite: http.SameSiteLaxMode, Secure: s.secureRequest(r), Expires: time.Unix(exp, 0),
	})
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode})
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// clientIP sengaja tidak memercayai X-Forwarded-For agar pembatas login tidak
// bisa diakali dengan header palsu. Di balik reverse proxy, semua percobaan
// dihitung bersama — wajar untuk aplikasi satu pengguna.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// loginLimiter membatasi percobaan login gagal: 5 kali per menit per IP.
type loginLimiter struct {
	mu   sync.Mutex
	hits map[string][]time.Time
}

func newLoginLimiter() *loginLimiter { return &loginLimiter{hits: map[string][]time.Time{}} }

func (l *loginLimiter) recent(ip string) []time.Time {
	cut := time.Now().Add(-time.Minute)
	var keep []time.Time
	for _, t := range l.hits[ip] {
		if t.After(cut) {
			keep = append(keep, t)
		}
	}
	l.hits[ip] = keep
	return keep
}

func (l *loginLimiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.recent(ip)) < 5
}

func (l *loginLimiter) fail(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.hits[ip] = append(l.recent(ip), time.Now())
}
