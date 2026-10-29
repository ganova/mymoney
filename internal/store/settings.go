package store

import (
	"context"
	"strconv"

	"github.com/ganova/mymoney/internal/period"
)

// Settings adalah preferensi pengguna.
type Settings struct {
	PeriodStartDay   int    `json:"period_start_day"`
	DefaultAccountID int64  `json:"default_account_id"`
	AIAutosave       bool   `json:"ai_autosave"`
	OwnerName        string `json:"owner_name"`
}

// defaultAccountKey: akun bawaan disimpan per buku (buku Pribadi memakai kunci lama).
func defaultAccountKey(ctx context.Context) string {
	if id := LedgerID(ctx); id != DefaultLedger {
		return "default_account_id_" + strconv.FormatInt(id, 10)
	}
	return "default_account_id"
}

func (s *Store) rawSettings(ctx context.Context) (map[string]string, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT key, value FROM settings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		m[k] = v
	}
	return m, rows.Err()
}

// GetSettings membaca seluruh pengaturan.
func (s *Store) GetSettings(ctx context.Context) (Settings, error) {
	m, err := s.rawSettings(ctx)
	if err != nil {
		return Settings{}, err
	}
	st := Settings{PeriodStartDay: 28}
	if v, err := strconv.Atoi(m["period_start_day"]); err == nil && v >= 1 && v <= 31 {
		st.PeriodStartDay = v
	}
	st.DefaultAccountID, _ = strconv.ParseInt(m[defaultAccountKey(ctx)], 10, 64)
	if a, err := s.GetAccount(ctx, st.DefaultAccountID); err != nil || a.LedgerID != LedgerID(ctx) || a.Archived {
		// akun bawaan harus milik buku aktif; bila tidak, pakai akun pertama buku ini
		st.DefaultAccountID = 0
		s.DB.QueryRowContext(ctx, `SELECT id FROM accounts WHERE ledger_id = ? AND archived = 0 ORDER BY sort, id LIMIT 1`, LedgerID(ctx)).Scan(&st.DefaultAccountID)
	}
	st.AIAutosave = m["ai_autosave"] == "true"
	st.OwnerName = m["owner_name"]
	return st, nil
}

// SaveSettings menyimpan pengaturan.
func (s *Store) SaveSettings(ctx context.Context, st Settings) error {
	if st.PeriodStartDay < 1 || st.PeriodStartDay > 31 {
		return invalid("tanggal mulai periode harus 1–31")
	}
	vals := map[string]string{
		"period_start_day":     strconv.Itoa(st.PeriodStartDay),
		defaultAccountKey(ctx): strconv.FormatInt(st.DefaultAccountID, 10),
		"ai_autosave":          strconv.FormatBool(st.AIAutosave),
		"owner_name":           st.OwnerName,
	}
	for k, v := range vals {
		if err := s.SetSetting(ctx, k, v); err != nil {
			return err
		}
	}
	return nil
}

// GetSetting / SetSetting untuk kunci internal (mis. secret sesi).
func (s *Store) GetSetting(ctx context.Context, key string) (string, error) {
	var v string
	err := s.DB.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if err != nil {
		return "", err
	}
	return v, nil
}

func (s *Store) SetSetting(ctx context.Context, key, value string) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

// CurrentPeriod mengembalikan periode berjalan.
func (s *Store) CurrentPeriod(ctx context.Context) (period.Period, error) {
	st, err := s.GetSettings(ctx)
	if err != nil {
		return period.Period{}, err
	}
	return period.For(s.Today(), st.PeriodStartDay), nil
}

// ResolvePeriod membaca kunci periode, atau periode berjalan bila kosong.
func (s *Store) ResolvePeriod(ctx context.Context, key string) (period.Period, error) {
	st, err := s.GetSettings(ctx)
	if err != nil {
		return period.Period{}, err
	}
	if key == "" {
		return period.For(s.Today(), st.PeriodStartDay), nil
	}
	p, err := period.Parse(key, st.PeriodStartDay)
	if err != nil {
		return p, invalid("%s", err.Error())
	}
	return p, nil
}
