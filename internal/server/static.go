package server

import (
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"
)

// staticHandler menyajikan aplikasi web (SPA). Rute yang tidak dikenal
// diarahkan ke index.html agar navigasi sisi klien berjalan.
func (s *Server) staticHandler() http.Handler {
	fileServer := http.FileServer(http.FS(s.static))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if p == "" {
			p = "index.html"
		}
		if _, err := fs.Stat(s.static, p); err != nil {
			p = "index.html"
			r = r.Clone(r.Context())
			r.URL.Path = "/"
		}
		switch {
		case strings.HasPrefix(p, "assets/"):
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		case p == "index.html" || p == "sw.js" || strings.HasSuffix(p, ".webmanifest"):
			w.Header().Set("Cache-Control", "no-cache")
		default:
			w.Header().Set("Cache-Control", "public, max-age=86400")
		}
		if p == "index.html" {
			data, err := fs.ReadFile(s.static, "index.html")
			if err != nil {
				http.Error(w, "frontend belum di-build: jalankan `npm run build` di folder web/", http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write(data)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}

// handleBackup mengunduh salinan utuh database SQLite.
func (s *Server) handleBackup(w http.ResponseWriter, r *http.Request) {
	tmp, err := os.CreateTemp("", "mymoney-backup-*.db")
	if err != nil {
		fail(w, err)
		return
	}
	name := tmp.Name()
	tmp.Close()
	os.Remove(name)
	defer os.Remove(name)
	if _, err := s.store.DB.ExecContext(r.Context(), `VACUUM INTO ?`, name); err != nil {
		fail(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.sqlite3")
	w.Header().Set("Content-Disposition", `attachment; filename="mymoney-backup-`+s.store.Today().Format("20060102")+`.db"`)
	http.ServeFile(w, r, name)
}
