package server

import (
	"context"
	"log"
	"net/http"
	"path/filepath"
	"time"

	"github.com/ganova/mymoney/internal/store"
)

// RunBackups membuat backup otomatis: sekali saat start bila backup terakhir
// sudah lebih tua dari every, lalu memeriksa setiap jam. every <= 0 = mati.
func (s *Server) RunBackups(ctx context.Context, dir string, keep int, every time.Duration) {
	if every <= 0 {
		log.Printf("backup otomatis dimatikan (BACKUP_EVERY_HOURS=0)")
		return
	}
	check := func() {
		list, _ := store.ListBackups(dir)
		if len(list) > 0 && time.Since(list[0].ModTime) < every {
			return
		}
		p, err := s.store.Backup(ctx, dir, keep)
		if err != nil {
			log.Printf("backup otomatis gagal: %v", err)
			return
		}
		log.Printf("backup otomatis tersimpan: %s", p)
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

// BackupDir menentukan folder backup: BACKUP_DIR bila diisi, selain itu data/backups.
func BackupDir(dataDir, configured string) string {
	if configured != "" {
		return configured
	}
	return filepath.Join(dataDir, "backups")
}

// BackupFolder mengembalikan folder backup yang dipakai server.
func (s *Server) BackupFolder() string { return s.backupDir }

// handleBackupStatus memberi tahu lokasi dan backup terakhir untuk halaman pengaturan.
func (s *Server) handleBackupStatus(w http.ResponseWriter, r *http.Request) {
	list, _ := store.ListBackups(s.backupDir)
	out := map[string]any{"dir": s.backupDir, "count": len(list), "keep": s.backupKeep, "every_hours": int(s.backupEvery.Hours())}
	if len(list) > 0 {
		out["last"] = list[0]
	}
	writeJSON(w, 200, out)
}

// handleBackupNow membuat backup sekarang.
func (s *Server) handleBackupNow(w http.ResponseWriter, r *http.Request) {
	p, err := s.store.Backup(r.Context(), s.backupDir, s.backupKeep)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]string{"path": p})
}
