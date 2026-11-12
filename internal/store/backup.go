package store

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const backupPrefix = "mymoney-"

// BackupInfo menggambarkan satu berkas backup.
type BackupInfo struct {
	Name    string    `json:"name"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"mod_time"`
}

// Backup menyalin database ke dir sebagai berkas bertanggal, lalu membuang
// salinan terlama bila jumlahnya melebihi keep (keep <= 0 = simpan semua).
func (s *Store) Backup(ctx context.Context, dir string, keep int) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("folder backup tidak bisa dibuat: %w", err)
	}
	name := filepath.Join(dir, backupPrefix+time.Now().In(s.Loc).Format("2006-01-02-150405")+".db")
	if _, err := os.Stat(name); err == nil {
		name = strings.TrimSuffix(name, ".db") + fmt.Sprintf("-%d.db", time.Now().UnixNano()%1000)
	}
	if _, err := s.DB.ExecContext(ctx, `VACUUM INTO ?`, name); err != nil {
		os.Remove(name)
		return "", fmt.Errorf("backup gagal: %w", err)
	}
	if keep > 0 {
		list, err := ListBackups(dir)
		if err == nil && len(list) > keep {
			for _, old := range list[keep:] {
				os.Remove(filepath.Join(dir, old.Name))
			}
		}
	}
	return name, nil
}

// ListBackups mengembalikan berkas backup di dir, terbaru dahulu.
func ListBackups(dir string) ([]BackupInfo, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := []BackupInfo{}
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasPrefix(n, backupPrefix) || !strings.HasSuffix(n, ".db") {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, BackupInfo{Name: n, Size: fi.Size(), ModTime: fi.ModTime()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name > out[j].Name })
	return out, nil
}
