package server

import (
	"context"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ganova/mymoney/internal/store"
)

// RunBackups menjalankan pekerjaan terjadwal: backup database (sekali saat
// start bila backup terakhir sudah lebih tua dari every, lalu diperiksa tiap
// jam) dan laporan bulanan periode sebelumnya. every <= 0 mematikan backup,
// laporan bulanan tetap dibuat.
func (s *Server) RunBackups(ctx context.Context, keep int, every time.Duration) {
	if every <= 0 {
		log.Printf("backup otomatis dimatikan (BACKUP_EVERY_HOURS=0)")
	}
	check := func() {
		dir, _ := s.backupFolder(ctx)
		if every > 0 {
			list, _ := store.ListBackups(dir)
			if len(list) == 0 || time.Since(list[0].ModTime) >= every {
				if p, err := s.store.Backup(ctx, dir, keep); err != nil {
					log.Printf("backup otomatis gagal: %v", err)
				} else {
					log.Printf("backup otomatis tersimpan: %s", p)
				}
			}
		}
		if files, err := s.ensureMonthlyReports(ctx, dir, false); err != nil {
			log.Printf("laporan bulanan gagal: %v", err)
		} else {
			for _, f := range files {
				log.Printf("laporan bulanan tersimpan: %s", f)
			}
		}
	}
	check()
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			check()
		}
	}
}

// BackupDir menentukan folder bawaan: BACKUP_DIR bila diisi, selain itu data/backups.
func BackupDir(dataDir, configured string) string {
	if configured != "" {
		return configured
	}
	return filepath.Join(dataDir, "backups")
}

// backupFolder mengembalikan folder backup & laporan yang dipakai sekarang:
// pilihan di Pengaturan, lalu BACKUP_DIR, lalu data/backups. Bila folder
// pilihan tidak bisa dipakai (mis. Google Drive belum terpasang), jatuh ke
// data/backups dan mengembalikan peringatan agar data tetap tercadang.
func (s *Server) backupFolder(ctx context.Context) (dir, warn string) {
	fallback := filepath.Join(s.cfg.DataDir, "backups")
	want := s.backupDir
	if v, err := s.store.GetSetting(ctx, "backup_dir"); err == nil && strings.TrimSpace(v) != "" {
		want = strings.TrimSpace(v)
	}
	if err := os.MkdirAll(want, 0o755); err != nil {
		if want == fallback {
			return want, "folder backup tidak bisa dibuat: " + err.Error()
		}
		os.MkdirAll(fallback, 0o755)
		return fallback, "Folder " + want + " tidak bisa diakses (" + err.Error() + "); sementara memakai " + fallback
	}
	return want, ""
}

// handleBackupStatus memberi tahu lokasi dan backup terakhir untuk halaman pengaturan.
func (s *Server) handleBackupStatus(w http.ResponseWriter, r *http.Request) {
	dir, warn := s.backupFolder(r.Context())
	list, _ := store.ListBackups(dir)
	out := map[string]any{"dir": dir, "warning": warn, "count": len(list), "keep": s.backupKeep, "every_hours": int(s.backupEvery.Hours()), "reports": listReports(dir)}
	if len(list) > 0 {
		out["last"] = list[0]
	}
	writeJSON(w, 200, out)
}

// handleBackupNow membuat backup sekarang.
func (s *Server) handleBackupNow(w http.ResponseWriter, r *http.Request) {
	dir, _ := s.backupFolder(r.Context())
	p, err := s.store.Backup(r.Context(), dir, s.backupKeep)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]string{"path": p})
}

// handleSetBackupDir menyimpan folder backup & laporan pilihan pengguna.
// Folder dicoba dibuat lebih dulu agar salah ketik langsung ketahuan.
func (s *Server) handleSetBackupDir(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Dir string `json:"dir"`
	}
	if err := decode(r, &in); err != nil {
		fail(w, err)
		return
	}
	dir := strings.TrimSpace(in.Dir)
	if dir != "" {
		if !filepath.IsAbs(dir) {
			fail(w, &store.ValidationError{Msg: "tulis lokasi lengkap, mis. G:\\My Drive\\My Money"})
			return
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			fail(w, &store.ValidationError{Msg: "folder tidak bisa dibuat atau diakses: " + err.Error()})
			return
		}
	}
	if err := s.store.SetSetting(r.Context(), "backup_dir", dir); err != nil {
		fail(w, err)
		return
	}
	s.handleBackupStatus(w, r)
}

// handleMonthlyReportNow membuat ulang laporan bulanan periode sebelumnya.
func (s *Server) handleMonthlyReportNow(w http.ResponseWriter, r *http.Request) {
	dir, _ := s.backupFolder(r.Context())
	files, err := s.ensureMonthlyReports(r.Context(), dir, true)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"files": files, "dir": dir})
}

// listReports mengembalikan nama berkas laporan bulanan di dir, terbaru dahulu (maks 6).
func listReports(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []string{}
	}
	type item struct {
		name string
		mod  time.Time
	}
	var items []item
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "Laporan ") || !strings.HasSuffix(e.Name(), ".xlsx") {
			continue
		}
		if fi, err := e.Info(); err == nil {
			items = append(items, item{e.Name(), fi.ModTime()})
		}
	}
	for i := 0; i < len(items); i++ {
		for j := i + 1; j < len(items); j++ {
			if items[j].mod.After(items[i].mod) {
				items[i], items[j] = items[j], items[i]
			}
		}
	}
	out := []string{}
	for i, it := range items {
		if i >= 6 {
			break
		}
		out = append(out, it.name)
	}
	return out
}
