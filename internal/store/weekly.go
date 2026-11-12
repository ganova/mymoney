package store

import (
	"context"
	"time"

	"github.com/ganova/mymoney/internal/period"
)

// WeekPlan adalah jatah fleksibel satu minggu dalam periode keuangan.
type WeekPlan struct {
	Index     int    `json:"index"` // 1..4
	Start     string `json:"start"`
	End       string `json:"end"`
	Days      int    `json:"days"`
	Allowance int64  `json:"allowance"` // jatah minggu ini
	Spent     int64  `json:"spent"`     // pengeluaran fleksibel di minggu ini
	Remaining int64  `json:"remaining"` // allowance - spent (boleh negatif)
	Current   bool   `json:"current"`
	Past      bool   `json:"past"`
	DaysLeft  int    `json:"days_left"`  // sisa hari (termasuk hari ini) bila minggu berjalan
	DailyLeft int64  `json:"daily_left"` // sisa jatah per hari bila minggu berjalan
}

// WeeklyPlan membagi uang fleksibel periode menjadi 4 minggu.
//
// Pool = pendapatan periode − budget wajib. Minggu 1–3 masing-masing 8 hari
// dan minggu 4 sisanya sampai akhir periode; pool dibagi sebanding jumlah hari.
// Pemasukan memakai nilai terbesar antara realisasi dan rencana; budget wajib
// memakai nilai terbesar antara rencana dan realisasi (wajib yang membengkak
// ikut mengurangi uang fleksibel).
type WeeklyPlan struct {
	Income         int64      `json:"income"`    // dasar pendapatan
	Mandatory      int64      `json:"mandatory"` // total budget wajib
	Pool           int64      `json:"pool"`      // pendapatan − wajib
	Weeks          []WeekPlan `json:"weeks"`
	Current        int        `json:"current"` // indeks minggu berjalan (0 = di luar periode)
	Spent          int64      `json:"spent"`
	Remaining      int64      `json:"remaining"`
	DailyRate      int64      `json:"daily_rate"` // pool / jumlah hari
	IncomeIsActual bool       `json:"income_is_actual"`
}

// weeklyPlan menyusun rencana mingguan untuk periode p.
func (s *Store) weeklyPlan(ctx context.Context, p period.Period, d *Dashboard) (*WeeklyPlan, error) {
	w := &WeeklyPlan{Income: max(d.Income, d.PlanIncome), IncomeIsActual: d.Income >= d.PlanIncome}
	mandatoryCats := []int64{}
	for _, b := range d.Budgets {
		if b.Kind == "expense" && b.Mandatory {
			w.Mandatory += max(b.Amount, b.Actual)
			mandatoryCats = append(mandatoryCats, b.CategoryID)
		}
	}
	w.Pool = w.Income - w.Mandatory
	pool := max(w.Pool, 0)
	days := p.Days()
	if days <= 0 {
		return w, nil
	}
	w.DailyRate = pool / int64(days)

	today := d.Today
	start := p.Start
	var allocated int64
	for i := 0; i < 4; i++ {
		n := 8
		if i == 3 {
			n = days - 24
		}
		if n <= 0 {
			break
		}
		from := start.AddDate(0, 0, i*8)
		to := from.AddDate(0, 0, n-1)
		wk := WeekPlan{Index: i + 1, Start: from.Format("2006-01-02"), End: to.Format("2006-01-02"), Days: n}
		if i == 3 {
			wk.Allowance = pool - allocated // sisa agar total pas
		} else {
			wk.Allowance = pool * int64(n) / int64(days)
			allocated += wk.Allowance
		}
		spent, err := s.flexSpent(ctx, wk.Start, wk.End, p.Key, mandatoryCats)
		if err != nil {
			return nil, err
		}
		wk.Spent = spent
		wk.Remaining = wk.Allowance - wk.Spent
		switch {
		case today > wk.End:
			wk.Past = true
		case today >= wk.Start && today <= wk.End:
			wk.Current = true
			w.Current = wk.Index
			t, _ := time.Parse("2006-01-02", today)
			e, _ := time.Parse("2006-01-02", wk.End)
			wk.DaysLeft = int(e.Sub(t).Hours()/24) + 1
			if wk.Remaining > 0 && wk.DaysLeft > 0 {
				wk.DailyLeft = wk.Remaining / int64(wk.DaysLeft)
			}
		}
		w.Spent += wk.Spent
		w.Weeks = append(w.Weeks, wk)
	}
	w.Remaining = pool - w.Spent
	return w, nil
}

// flexSpent menjumlahkan pengeluaran yang BUKAN kategori budget wajib
// (kategori fleksibel dan yang belum punya budget).
func (s *Store) flexSpent(ctx context.Context, from, to, periodKey string, mandatoryCats []int64) (int64, error) {
	q := `SELECT COALESCE(SUM(amount), 0) FROM transactions WHERE ledger_id = ? AND type = 'expense' AND date BETWEEN ? AND ?`
	args := []any{LedgerID(ctx), from, to}
	if len(mandatoryCats) > 0 {
		q += ` AND (category_id IS NULL OR category_id NOT IN (`
		for i, id := range mandatoryCats {
			if i > 0 {
				q += ","
			}
			q += "?"
			args = append(args, id)
		}
		q += `))`
	}
	var v int64
	err := s.DB.QueryRowContext(ctx, q, args...).Scan(&v)
	return v, err
}
