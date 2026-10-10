// Package period menghitung periode keuangan yang dimulai pada tanggal gajian.
//
// Dengan tanggal mulai 28, periode "2026-10" (Oktober 2026) berlangsung dari
// 28 September 2026 sampai 27 Oktober 2026. Label periode selalu mengikuti
// bulan tempat periode itu berakhir. Jika tanggal mulai melebihi jumlah hari
// dalam bulan (misalnya 31 di bulan Februari), tanggal mulai dijepit ke hari
// terakhir bulan tersebut.
package period

import (
	"fmt"
	"time"
)

const layout = "2006-01-02"

// Period adalah satu siklus keuangan.
type Period struct {
	Key   string    `json:"key"`   // "2026-10"
	Label string    `json:"label"` // "Oktober 2026"
	Start time.Time `json:"-"`
	End   time.Time `json:"-"` // inklusif

	StartDay int `json:"-"`
}

var bulan = [...]string{"Januari", "Februari", "Maret", "April", "Mei", "Juni", "Juli", "Agustus", "September", "Oktober", "November", "Desember"}
var bulanPendek = [...]string{"Jan", "Feb", "Mar", "Apr", "Mei", "Jun", "Jul", "Agu", "Sep", "Okt", "Nov", "Des"}

// MonthName mengembalikan nama bulan dalam bahasa Indonesia.
func MonthName(m time.Month) string { return bulan[m-1] }

// ShortMonthName mengembalikan singkatan bulan dalam bahasa Indonesia.
func ShortMonthName(m time.Month) string { return bulanPendek[m-1] }

func daysIn(y int, m time.Month) int {
	return time.Date(y, m+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

func clampDate(y int, m time.Month, day int) time.Time {
	// normalisasi bulan (mis. bulan 0 -> Desember tahun sebelumnya)
	t := time.Date(y, m, 1, 0, 0, 0, 0, time.UTC)
	y, m = t.Year(), t.Month()
	if d := daysIn(y, m); day > d {
		day = d
	}
	return time.Date(y, m, day, 0, 0, 0, 0, time.UTC)
}

// startOf mengembalikan tanggal mulai periode berlabel (y, m).
func startOf(y int, m time.Month, startDay int) time.Time {
	if startDay <= 1 {
		return time.Date(y, m, 1, 0, 0, 0, 0, time.UTC)
	}
	return clampDate(y, m-1, startDay)
}

func normalize(startDay int) int {
	if startDay < 1 {
		return 1
	}
	if startDay > 31 {
		return 31
	}
	return startDay
}

// Of membangun periode berlabel (year, month).
func Of(year int, month time.Month, startDay int) Period {
	startDay = normalize(startDay)
	t := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
	year, month = t.Year(), t.Month()
	start := startOf(year, month, startDay)
	next := time.Date(year, month+1, 1, 0, 0, 0, 0, time.UTC)
	end := startOf(next.Year(), next.Month(), startDay).AddDate(0, 0, -1)
	return Period{
		Key:      fmt.Sprintf("%04d-%02d", year, int(month)),
		Label:    fmt.Sprintf("%s %d", MonthName(month), year),
		Start:    start,
		End:      end,
		StartDay: startDay,
	}
}

// For mengembalikan periode yang memuat tanggal d.
func For(d time.Time, startDay int) Period {
	d = time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC)
	p := Of(d.Year(), d.Month(), startDay)
	if d.After(p.End) {
		return p.Next()
	}
	if d.Before(p.Start) {
		return p.Prev()
	}
	return p
}

// Parse membaca kunci "YYYY-MM".
func Parse(key string, startDay int) (Period, error) {
	t, err := time.Parse("2006-01", key)
	if err != nil {
		return Period{}, fmt.Errorf("periode tidak valid: %q", key)
	}
	return Of(t.Year(), t.Month(), startDay), nil
}

func (p Period) month() (int, time.Month) {
	t, _ := time.Parse("2006-01", p.Key)
	return t.Year(), t.Month()
}

// Next mengembalikan periode berikutnya.
func (p Period) Next() Period {
	y, m := p.month()
	return Of(y, m+1, p.StartDay)
}

// Prev mengembalikan periode sebelumnya.
func (p Period) Prev() Period {
	y, m := p.month()
	return Of(y, m-1, p.StartDay)
}

// StartStr / EndStr dalam format YYYY-MM-DD.
func (p Period) StartStr() string { return p.Start.Format(layout) }
func (p Period) EndStr() string   { return p.End.Format(layout) }

// Days adalah jumlah hari dalam periode.
func (p Period) Days() int { return int(p.End.Sub(p.Start).Hours()/24) + 1 }

// Contains melaporkan apakah tanggal d berada di dalam periode.
func (p Period) Contains(d time.Time) bool {
	d = time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC)
	return !d.Before(p.Start) && !d.After(p.End)
}

// Range adalah teks rentang singkat, mis. "28 Sep – 27 Okt 2026".
func (p Period) Range() string {
	s, e := p.Start, p.End
	if s.Year() != e.Year() {
		return fmt.Sprintf("%d %s %d – %d %s %d", s.Day(), ShortMonthName(s.Month()), s.Year(), e.Day(), ShortMonthName(e.Month()), e.Year())
	}
	return fmt.Sprintf("%d %s – %d %s %d", s.Day(), ShortMonthName(s.Month()), e.Day(), ShortMonthName(e.Month()), e.Year())
}

// Info adalah representasi JSON periode untuk API.
type Info struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Start string `json:"start"`
	End   string `json:"end"`
	Range string `json:"range"`
	Days  int    `json:"days"`
	Prev  string `json:"prev"`
	Next  string `json:"next"`
}

func (p Period) Info() Info {
	return Info{
		Key: p.Key, Label: p.Label, Start: p.StartStr(), End: p.EndStr(),
		Range: p.Range(), Days: p.Days(), Prev: p.Prev().Key, Next: p.Next().Key,
	}
}
