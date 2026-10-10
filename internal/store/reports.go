package store

import (
	"context"
	"database/sql"
	"fmt"
	"sort"

	"github.com/ganova/mymoney/internal/money"
	"github.com/ganova/mymoney/internal/period"
)

// CatTotal adalah total per kategori.
type CatTotal struct {
	CategoryID *int64 `json:"category_id"`
	Kind       string `json:"kind"`
	Total      int64  `json:"total"`
	Count      int64  `json:"count"`
}

// DayTotal adalah total harian.
type DayTotal struct {
	Date    string `json:"date"`
	Expense int64  `json:"expense"`
	Income  int64  `json:"income"`
}

// Alert adalah peringatan yang tampil di beranda.
type Alert struct {
	Level string `json:"level"` // info | warn | danger
	Icon  string `json:"icon"`
	Text  string `json:"text"`
}

// Dashboard adalah ringkasan satu periode.
type Dashboard struct {
	Period      period.Info `json:"period"`
	Today       string      `json:"today"`
	IsCurrent   bool        `json:"is_current"`
	DaysLeft    int         `json:"days_left"`
	DaysElapsed int         `json:"days_elapsed"`

	Income      int64 `json:"income"`
	Expense     int64 `json:"expense"`
	Net         int64 `json:"net"`
	PrevIncome  int64 `json:"prev_income"`
	PrevExpense int64 `json:"prev_expense"`
	SpentToday  int64 `json:"spent_today"`

	PlanIncome     int64 `json:"plan_income"`
	PlanExpense    int64 `json:"plan_expense"`
	BudgetSpent    int64 `json:"budget_spent"`
	Unbudgeted     int64 `json:"unbudgeted"`
	FlexPlan       int64 `json:"flex_plan"`
	FlexSpent      int64 `json:"flex_spent"`
	FlexRemaining  int64 `json:"flex_remaining"`
	DailyAllowance int64 `json:"daily_allowance"`
	BillsTotal     int64 `json:"bills_total"`
	BillsPaid      int64 `json:"bills_paid"`

	ByCategory []CatTotal `json:"by_category"`
	Daily      []DayTotal `json:"daily"`
	Budgets    []Budget   `json:"budgets"`

	Accounts      []Account `json:"accounts"`
	TotalBalance  int64     `json:"total_balance"`
	CreditBalance int64     `json:"credit_balance"`
	DebtRemaining int64     `json:"debt_remaining"`

	Recent           []Transaction `json:"recent"`
	Alerts           []Alert       `json:"alerts"`
	BudgetAutoCopied bool          `json:"budget_auto_copied"`
}

// totals menghitung total pemasukan & pengeluaran pada rentang tanggal.
func (s *Store) totals(ctx context.Context, from, to string) (income, expense int64, err error) {
	err = s.DB.QueryRowContext(ctx, `SELECT
		COALESCE(SUM(CASE WHEN type='income' THEN amount END),0),
		COALESCE(SUM(CASE WHEN type='expense' THEN amount END),0)
		FROM transactions WHERE date BETWEEN ? AND ?`, from, to).Scan(&income, &expense)
	return
}

func (s *Store) byCategory(ctx context.Context, from, to string) ([]CatTotal, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT category_id, type, SUM(amount), COUNT(*) FROM transactions
		WHERE date BETWEEN ? AND ? AND type IN ('income','expense') GROUP BY category_id, type ORDER BY SUM(amount) DESC`, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CatTotal{}
	for rows.Next() {
		var c CatTotal
		var cat sql.NullInt64
		if err := rows.Scan(&cat, &c.Kind, &c.Total, &c.Count); err != nil {
			return nil, err
		}
		c.CategoryID = ptr(cat)
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) daily(ctx context.Context, p period.Period) ([]DayTotal, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT date,
		COALESCE(SUM(CASE WHEN type='expense' THEN amount END),0),
		COALESCE(SUM(CASE WHEN type='income' THEN amount END),0)
		FROM transactions WHERE date BETWEEN ? AND ? GROUP BY date`, p.StartStr(), p.EndStr())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[string]DayTotal{}
	for rows.Next() {
		var d DayTotal
		if err := rows.Scan(&d.Date, &d.Expense, &d.Income); err != nil {
			return nil, err
		}
		m[d.Date] = d
	}
	out := make([]DayTotal, 0, p.Days())
	for d := p.Start; !d.After(p.End); d = d.AddDate(0, 0, 1) {
		k := d.Format("2006-01-02")
		v := m[k]
		v.Date = k
		out = append(out, v)
	}
	return out, rows.Err()
}

// Dashboard menyusun ringkasan periode untuk beranda.
func (s *Store) Dashboard(ctx context.Context, p period.Period) (*Dashboard, error) {
	today := s.Today()
	d := &Dashboard{Period: p.Info(), Today: today.Format("2006-01-02")}
	d.IsCurrent = p.Contains(today)
	switch {
	case d.IsCurrent:
		d.DaysLeft = int(p.End.Sub(today).Hours()/24) + 1
		d.DaysElapsed = p.Days() - d.DaysLeft + 1
	case today.Before(p.Start):
		d.DaysLeft = p.Days()
	default:
		d.DaysElapsed = p.Days()
	}

	var err error
	if d.BudgetAutoCopied, err = s.EnsureBudget(ctx, p); err != nil {
		return nil, err
	}
	if d.Income, d.Expense, err = s.totals(ctx, p.StartStr(), p.EndStr()); err != nil {
		return nil, err
	}
	d.Net = d.Income - d.Expense
	prev := p.Prev()
	if d.PrevIncome, d.PrevExpense, err = s.totals(ctx, prev.StartStr(), prev.EndStr()); err != nil {
		return nil, err
	}
	if _, d.SpentToday, err = s.totals(ctx, d.Today, d.Today); err != nil {
		return nil, err
	}
	if d.ByCategory, err = s.byCategory(ctx, p.StartStr(), p.EndStr()); err != nil {
		return nil, err
	}
	if d.Daily, err = s.daily(ctx, p); err != nil {
		return nil, err
	}
	if d.Budgets, err = s.ListBudgets(ctx, p); err != nil {
		return nil, err
	}
	if d.Accounts, err = s.ListAccounts(ctx); err != nil {
		return nil, err
	}
	for _, a := range d.Accounts {
		if a.Archived {
			continue
		}
		if a.Type == "credit" {
			d.CreditBalance += a.Balance
		} else {
			d.TotalBalance += a.Balance
		}
	}
	debts, err := s.ListDebts(ctx)
	if err != nil {
		return nil, err
	}
	for _, x := range debts {
		if !x.Archived {
			d.DebtRemaining += x.Remaining
		}
	}
	if d.Recent, err = s.ListTransactions(ctx, TxFilter{Limit: 8}); err != nil {
		return nil, err
	}

	cats, err := s.ListCategories(ctx)
	if err != nil {
		return nil, err
	}
	catName := map[int64]string{}
	for _, c := range cats {
		catName[c.ID] = c.Name
	}

	budgeted := map[int64]bool{}
	for _, b := range d.Budgets {
		if b.Kind == "income" {
			d.PlanIncome += b.Amount
			continue
		}
		budgeted[b.CategoryID] = true
		d.PlanExpense += b.Amount
		d.BudgetSpent += b.Actual
		if b.DueDay > 0 {
			d.BillsTotal += b.Amount
			d.BillsPaid += min(b.Actual, b.Amount)
		} else {
			d.FlexPlan += b.Amount
			d.FlexSpent += b.Actual
			if rem := b.Amount - b.Actual; rem > 0 {
				d.FlexRemaining += rem
			}
		}
	}
	for _, c := range d.ByCategory {
		if c.Kind == "expense" && (c.CategoryID == nil || !budgeted[*c.CategoryID]) {
			d.Unbudgeted += c.Total
		}
	}
	if d.DaysLeft > 0 && d.FlexRemaining > 0 {
		d.DailyAllowance = d.FlexRemaining / int64(d.DaysLeft)
	}

	d.Alerts = s.alerts(d, p, catName)
	return d, nil
}

func (s *Store) alerts(d *Dashboard, p period.Period, catName map[int64]string) []Alert {
	out := []Alert{}
	if d.BudgetAutoCopied {
		out = append(out, Alert{"info", "📋", "Budget periode ini disalin otomatis dari periode sebelumnya. Sesuaikan bila perlu."})
	}
	type scored struct {
		a     Alert
		score float64
	}
	var list []scored
	for _, b := range d.Budgets {
		if b.Kind != "expense" {
			continue
		}
		name := catName[b.CategoryID]
		if b.DueDay > 0 {
			if b.Actual >= b.Amount || !d.IsCurrent {
				continue
			}
			days := daysBetween(d.Today, b.DueDate)
			switch {
			case days < 0:
				list = append(list, scored{Alert{"danger", "⏰", fmt.Sprintf("Tagihan %s lewat jatuh tempo (%s), sisa %s", name, shortDate(b.DueDate), money.Format(b.Amount-b.Actual))}, 100})
			case days <= 3:
				when := fmt.Sprintf("%d hari lagi", days)
				if days == 0 {
					when = "hari ini"
				}
				list = append(list, scored{Alert{"warn", "🔔", fmt.Sprintf("Tagihan %s jatuh tempo %s (%s)", name, when, money.Format(b.Amount-b.Actual))}, 90 - float64(days)})
			}
			continue
		}
		if b.Amount <= 0 {
			if b.Actual > 0 {
				list = append(list, scored{Alert{"warn", "⚠️", fmt.Sprintf("%s terpakai %s padahal budget-nya 0", name, money.Format(b.Actual))}, 50})
			}
			continue
		}
		ratio := float64(b.Actual) / float64(b.Amount)
		switch {
		case ratio > 1:
			list = append(list, scored{Alert{"danger", "🔥", fmt.Sprintf("Budget %s terlampaui %s", name, money.Format(b.Actual-b.Amount))}, 80 + ratio})
		case ratio >= 0.85 && d.IsCurrent:
			list = append(list, scored{Alert{"warn", "⚠️", fmt.Sprintf("Budget %s sudah terpakai %d%%, sisa %s", name, int(ratio*100), money.Format(b.Amount-b.Actual))}, 60 + ratio})
		}
	}
	if d.IsCurrent && d.FlexPlan > 0 && d.DaysElapsed > 3 {
		used := float64(d.FlexSpent) / float64(d.FlexPlan)
		elapsed := float64(d.DaysElapsed) / float64(p.Days())
		if used > elapsed+0.15 && used <= 1 {
			list = append(list, scored{Alert{"warn", "🏃", fmt.Sprintf("Uang sehari-hari terpakai %d%% padahal periode baru berjalan %d%%. Jatah aman %s/hari.", int(used*100), int(elapsed*100), money.Format(d.DailyAllowance))}, 70})
		}
	}
	sort.SliceStable(list, func(i, j int) bool { return list[i].score > list[j].score })
	for i, x := range list {
		if i >= 6 {
			break
		}
		out = append(out, x.a)
	}
	return out
}

func daysBetween(from, to string) int {
	a, err1 := parseDate(from)
	b, err2 := parseDate(to)
	if err1 != nil || err2 != nil {
		return 0
	}
	return int(b.Sub(a).Hours() / 24)
}

func shortDate(s string) string {
	t, err := parseDate(s)
	if err != nil {
		return s
	}
	return fmt.Sprintf("%d %s", t.Day(), period.ShortMonthName(t.Month()))
}

// Report adalah laporan lengkap satu periode.
type Report struct {
	Period     period.Info  `json:"period"`
	Income     int64        `json:"income"`
	Expense    int64        `json:"expense"`
	ByCategory []CatTotal   `json:"by_category"`
	Daily      []DayTotal   `json:"daily"`
	Top        []TopItem    `json:"top"`
	ByAccount  []AccTotal   `json:"by_account"`
	History    []PeriodStat `json:"history"`
}

// TopItem adalah deskripsi pengeluaran terbesar.
type TopItem struct {
	Description string `json:"description"`
	CategoryID  *int64 `json:"category_id"`
	Total       int64  `json:"total"`
	Count       int64  `json:"count"`
}

// AccTotal adalah pengeluaran per akun.
type AccTotal struct {
	AccountID int64 `json:"account_id"`
	Expense   int64 `json:"expense"`
	Income    int64 `json:"income"`
}

// PeriodStat adalah ringkasan periode untuk grafik tren.
type PeriodStat struct {
	Key     string `json:"key"`
	Label   string `json:"label"`
	Income  int64  `json:"income"`
	Expense int64  `json:"expense"`
}

// Report menyusun laporan periode.
func (s *Store) Report(ctx context.Context, p period.Period, historyN int) (*Report, error) {
	r := &Report{Period: p.Info()}
	var err error
	if r.Income, r.Expense, err = s.totals(ctx, p.StartStr(), p.EndStr()); err != nil {
		return nil, err
	}
	if r.ByCategory, err = s.byCategory(ctx, p.StartStr(), p.EndStr()); err != nil {
		return nil, err
	}
	if r.Daily, err = s.daily(ctx, p); err != nil {
		return nil, err
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT description, MAX(category_id), SUM(amount), COUNT(*) FROM transactions
		WHERE date BETWEEN ? AND ? AND type = 'expense' GROUP BY lower(description) ORDER BY SUM(amount) DESC LIMIT 15`, p.StartStr(), p.EndStr())
	if err != nil {
		return nil, err
	}
	r.Top = []TopItem{}
	for rows.Next() {
		var t TopItem
		var cat sql.NullInt64
		if err := rows.Scan(&t.Description, &cat, &t.Total, &t.Count); err != nil {
			rows.Close()
			return nil, err
		}
		t.CategoryID = ptr(cat)
		r.Top = append(r.Top, t)
	}
	rows.Close()
	rows, err = s.DB.QueryContext(ctx, `SELECT account_id,
		COALESCE(SUM(CASE WHEN type='expense' THEN amount END),0), COALESCE(SUM(CASE WHEN type='income' THEN amount END),0)
		FROM transactions WHERE date BETWEEN ? AND ? AND type IN ('income','expense') GROUP BY account_id ORDER BY 2 DESC`, p.StartStr(), p.EndStr())
	if err != nil {
		return nil, err
	}
	r.ByAccount = []AccTotal{}
	for rows.Next() {
		var a AccTotal
		if err := rows.Scan(&a.AccountID, &a.Expense, &a.Income); err != nil {
			rows.Close()
			return nil, err
		}
		r.ByAccount = append(r.ByAccount, a)
	}
	rows.Close()
	if historyN <= 0 {
		historyN = 6
	}
	q := p
	for i := 0; i < historyN; i++ {
		inc, exp, err := s.totals(ctx, q.StartStr(), q.EndStr())
		if err != nil {
			return nil, err
		}
		r.History = append([]PeriodStat{{Key: q.Key, Label: q.Label, Income: inc, Expense: exp}}, r.History...)
		q = q.Prev()
	}
	return r, nil
}
