// Package server berisi HTTP API dan penyaji file frontend.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ganova/mymoney/internal/ai"
	"github.com/ganova/mymoney/internal/config"
	"github.com/ganova/mymoney/internal/store"
)

// Version diisi saat build.
var Version = "dev"

// Server adalah aplikasi HTTP MyMoney.
type Server struct {
	store   *store.Store
	ai      *ai.Service
	cfg     config.Config
	secret  []byte
	static  fs.FS
	limiter *loginLimiter
	mux     *http.ServeMux
}

// New menyusun server beserta seluruh rute.
func New(cfg config.Config, st *store.Store, aiSvc *ai.Service, static fs.FS) (*Server, error) {
	secret, err := sessionSecret(cfg, st)
	if err != nil {
		return nil, err
	}
	s := &Server{store: st, ai: aiSvc, cfg: cfg, secret: secret, static: static, limiter: newLoginLimiter(), mux: http.NewServeMux()}
	s.routes()
	return s, nil
}

func (s *Server) routes() {
	m := s.mux
	// publik
	m.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, map[string]string{"status": "ok"}) })
	m.HandleFunc("GET /api/auth/status", s.handleAuthStatus)
	m.HandleFunc("POST /api/auth/login", s.handleLogin)
	m.HandleFunc("POST /api/auth/logout", s.handleLogout)

	// terlindungi
	p := func(pattern string, h http.HandlerFunc) { m.Handle(pattern, s.requireAuth(h)) }
	p("GET /api/bootstrap", s.handleBootstrap)
	p("PUT /api/settings", s.handleSaveSettings)
	p("GET /api/period", s.handlePeriod)

	p("GET /api/accounts", s.handleListAccounts)
	p("POST /api/accounts", s.handleCreateAccount)
	p("PUT /api/accounts/{id}", s.handleUpdateAccount)
	p("DELETE /api/accounts/{id}", s.handleDeleteAccount)
	p("POST /api/accounts/{id}/reconcile", s.handleReconcile)

	p("GET /api/ledgers", s.handleListLedgers)
	p("PUT /api/ledgers/{id}", s.handleUpdateLedger)
	p("GET /api/income-sources", s.handleListSources)
	p("POST /api/income-sources", s.handleCreateSource)
	p("PUT /api/income-sources/{id}", s.handleUpdateSource)
	p("DELETE /api/income-sources/{id}", s.handleDeleteSource)

	p("GET /api/receivables", s.handleListReceivables)
	p("POST /api/receivables", s.handleCreateReceivable)
	p("POST /api/receivables/scan", s.handleScanReceivables)
	p("POST /api/receivables/bulk", s.handleBulkReceivables)
	p("PUT /api/receivables/{id}", s.handleUpdateReceivable)
	p("DELETE /api/receivables/{id}", s.handleDeleteReceivable)
	p("POST /api/receivables/{id}/receive", s.handleReceiveReceivable)
	p("GET /api/outlook", s.handleOutlook)

	p("GET /api/categories", s.handleListCategories)
	p("POST /api/categories", s.handleCreateCategory)
	p("PUT /api/categories/{id}", s.handleUpdateCategory)
	p("DELETE /api/categories/{id}", s.handleDeleteCategory)

	p("GET /api/transactions", s.handleListTransactions)
	p("POST /api/transactions", s.handleCreateTransaction)
	p("POST /api/transactions/bulk", s.handleBulkCreate)
	p("POST /api/transactions/delete", s.handleBulkDelete)
	p("PUT /api/transactions/{id}", s.handleUpdateTransaction)
	p("DELETE /api/transactions/{id}", s.handleDeleteTransaction)

	p("GET /api/budgets", s.handleListBudgets)
	p("PUT /api/budgets", s.handleUpsertBudget)
	p("DELETE /api/budgets/{id}", s.handleDeleteBudget)
	p("POST /api/budgets/copy", s.handleCopyBudget)
	p("POST /api/budgets/apply", s.handleApplyBudget)

	p("GET /api/dashboard", s.handleDashboard)
	p("GET /api/reports", s.handleReport)

	p("GET /api/debts", s.handleListDebts)
	p("POST /api/debts", s.handleCreateDebt)
	p("PUT /api/debts/{id}", s.handleUpdateDebt)
	p("DELETE /api/debts/{id}", s.handleDeleteDebt)

	p("GET /api/rules", s.handleListRules)
	p("DELETE /api/rules/{id}", s.handleDeleteRule)

	p("GET /api/tokens", s.handleListTokens)
	p("POST /api/tokens", s.handleCreateToken)
	p("DELETE /api/tokens/{id}", s.handleDeleteToken)

	p("POST /api/ai/ask", s.handleAsk)
	p("POST /api/ai/scan", s.handleScan)
	p("POST /api/ai/import", s.handleImport)
	p("POST /api/ai/budget", s.handleSuggestBudget)
	p("GET /api/ai/insight", s.handleGetInsight)
	p("POST /api/ai/insight", s.handleGenerateInsight)
	p("GET /api/ai/usage", s.handleAIUsage)

	p("POST /api/shortcut/ask", s.handleShortcutAsk)
	p("POST /api/shortcut/scan", s.handleShortcutScan)

	p("GET /api/export", s.handleExport)
	p("GET /api/backup", s.handleBackup)

	m.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) { writeError(w, 404, "endpoint tidak ditemukan") })
	m.Handle("/", s.staticHandler())
}

// ServeHTTP menambahkan header keamanan dan logging ringan.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "same-origin")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data: blob:; style-src 'self' 'unsafe-inline'; script-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
	start := time.Now()
	rw := &statusWriter{ResponseWriter: w, status: 200}
	if strings.HasPrefix(r.URL.Path, "/api/") {
		r = r.WithContext(s.withLedger(r))
	}
	s.mux.ServeHTTP(rw, r)
	if strings.HasPrefix(r.URL.Path, "/api/") {
		log.Printf("%s %s %d %s", r.Method, r.URL.Path, rw.status, time.Since(start).Round(time.Millisecond))
	}
}

// withLedger memilih buku keuangan aktif dari header X-Ledger atau ?ledger=
// (default: Pribadi). Buku yang tidak dikenal diabaikan.
func (s *Server) withLedger(r *http.Request) context.Context {
	v := r.Header.Get("X-Ledger")
	if v == "" {
		v = r.URL.Query().Get("ledger")
	}
	if id, err := strconv.ParseInt(v, 10, 64); err == nil && s.store.ValidLedger(r.Context(), id) {
		return store.WithLedger(r.Context(), id)
	}
	return store.WithLedger(r.Context(), store.DefaultLedger)
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// ---------- helper ----------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// fail memetakan error ke status HTTP yang sesuai.
func fail(w http.ResponseWriter, err error) {
	var ve *store.ValidationError
	switch {
	case errors.As(err, &ve):
		writeError(w, http.StatusBadRequest, ve.Msg)
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ai.ErrDisabled):
		writeError(w, http.StatusServiceUnavailable, err.Error())
	case errors.Is(err, context.Canceled):
		writeError(w, 499, "permintaan dibatalkan")
	default:
		log.Printf("error: %v", err)
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}

func decode(r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 2<<20)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		return &store.ValidationError{Msg: "format data tidak valid: " + err.Error()}
	}
	return nil
}

func pathID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, &store.ValidationError{Msg: "id tidak valid"}
	}
	return id, nil
}

func queryInt(r *http.Request, key string) int64 {
	v, _ := strconv.ParseInt(r.URL.Query().Get(key), 10, 64)
	return v
}
