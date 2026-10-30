package store

import (
	"context"

	"github.com/ganova/mymoney/internal/period"
)

// OutlookMonth adalah proyeksi satu periode ke depan.
type OutlookMonth struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Range    string `json:"range"`
	Incoming int64  `json:"incoming"`  // pendapatan tertunda yang diperkirakan cair
	DebtDue  int64  `json:"debt_due"`  // cicilan yang harus dibayar periode ini
	DebtPaid int64  `json:"debt_paid"` // sudah dibayar (periode berjalan)
	Net      int64  `json:"net"`       // incoming - debt_due
}

// OutlookDebt adalah rangkuman jadwal satu utang.
type OutlookDebt struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Remaining  int64  `json:"remaining"`
	Monthly    int64  `json:"monthly"`
	MonthsLeft int    `json:"months_left"` // 0 = tanpa jadwal cicilan
	LastLabel  string `json:"last_label"`  // periode pelunasan
}

// Outlook adalah gambaran pendapatan tertunda dan utang ke depan.
type Outlook struct {
	Months []OutlookMonth `json:"months"`

	PendingTotal       int64 `json:"pending_total"`
	PendingCount       int   `json:"pending_count"`
	PendingUnscheduled int64 `json:"pending_unscheduled"` // belum ada estimasi tanggal

	DebtRemaining   int64         `json:"debt_remaining"`
	DebtMonthly     int64         `json:"debt_monthly"`     // total cicilan per bulan saat ini
	DebtUnscheduled int64         `json:"debt_unscheduled"` // sisa utang tanpa cicilan bulanan
	DebtFreeLabel   string        `json:"debt_free_label"`  // perkiraan lunas semua cicilan
	Debts           []OutlookDebt `json:"debts"`
}

// Outlook menyusun proyeksi n periode mulai periode berjalan.
func (s *Store) Outlook(ctx context.Context, n int) (*Outlook, error) {
	if n <= 0 || n > 36 {
		n = 12
	}
	cur, err := s.CurrentPeriod(ctx)
	if err != nil {
		return nil, err
	}
	o := &Outlook{Months: make([]OutlookMonth, n), Debts: []OutlookDebt{}}
	idx := map[string]int{}
	p := cur
	for i := 0; i < n; i++ {
		o.Months[i] = OutlookMonth{Key: p.Key, Label: p.Label, Range: p.Range()}
		idx[p.Key] = i
		p = p.Next()
	}

	// pendapatan tertunda
	recs, err := s.ListReceivables(ctx)
	if err != nil {
		return nil, err
	}
	st, _ := s.GetSettings(ctx)
	for _, r := range recs {
		if r.Status != "pending" {
			continue
		}
		o.PendingTotal += r.Amount
		o.PendingCount++
		if r.ExpectedDate == "" {
			o.PendingUnscheduled += r.Amount
			continue
		}
		d, err := parseDate(r.ExpectedDate)
		if err != nil {
			o.PendingUnscheduled += r.Amount
			continue
		}
		key := period.For(d, st.PeriodStartDay).Key
		i, ok := idx[key]
		switch {
		case ok:
		case key < cur.Key:
			i, ok = 0, true // terlambat: dianggap cair di periode berjalan
		}
		if ok {
			o.Months[i].Incoming += r.Amount
		}
	}

	// utang: jadwal cicilan bulanan sampai lunas
	debts, err := s.ListDebts(ctx)
	if err != nil {
		return nil, err
	}
	farthest, freeLabel := -1, ""
	for _, d := range debts {
		if d.Archived || d.Remaining <= 0 {
			continue
		}
		o.DebtRemaining += d.Remaining
		od := OutlookDebt{ID: d.ID, Name: d.Name, Remaining: d.Remaining, Monthly: d.Monthly}
		if d.Monthly <= 0 {
			o.DebtUnscheduled += d.Remaining
			o.Debts = append(o.Debts, od)
			continue
		}
		o.DebtMonthly += d.Monthly
		// saldo awal periode berjalan = sisa + yang sudah dibayar periode ini
		var paidNow int64
		s.DB.QueryRowContext(ctx, `SELECT COALESCE(SUM(amount),0) FROM transactions WHERE ledger_id = ? AND debt_id = ? AND type = 'expense' AND date BETWEEN ? AND ?`,
			LedgerID(ctx), d.ID, cur.StartStr(), cur.EndStr()).Scan(&paidNow)
		bal := d.Remaining + paidNow
		pp := cur
		lastIdx := -1
		for i := 0; bal > 0 && i < 600; i++ {
			pay := min(d.Monthly, bal)
			bal -= pay
			outstanding := pay
			if i == 0 {
				paid := min(paidNow, pay)
				outstanding = pay - paid
				if i < n {
					o.Months[0].DebtPaid += paid
				}
			}
			if outstanding > 0 {
				od.MonthsLeft++
				od.LastLabel = pp.Label
				lastIdx = i
				if i < n {
					o.Months[i].DebtDue += outstanding
				}
			}
			pp = pp.Next()
		}
		if lastIdx > farthest {
			farthest, freeLabel = lastIdx, od.LastLabel
		}
		o.Debts = append(o.Debts, od)
	}
	for i := range o.Months {
		o.Months[i].Net = o.Months[i].Incoming - o.Months[i].DebtDue
	}
	o.DebtFreeLabel = freeLabel
	return o, nil
}
