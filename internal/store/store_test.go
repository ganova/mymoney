package store

import (
	"context"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	loc, _ := time.LoadLocation("Asia/Jakarta")
	s, err := OpenMemory(loc)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func idOf(t *testing.T, s *Store, name string) int64 {
	t.Helper()
	ctx := context.Background()
	accs, _ := s.ListAccounts(ctx)
	for _, a := range accs {
		if a.Name == name {
			return a.ID
		}
	}
	cats, _ := s.ListCategories(ctx)
	for _, c := range cats {
		if c.Name == name {
			return c.ID
		}
	}
	t.Fatalf("not found: %s", name)
	return 0
}

func i64(v int64) *int64 { return &v }

func TestSeedAndBalances(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	st, err := s.GetSettings(ctx)
	if err != nil || st.PeriodStartDay != 28 || st.DefaultAccountID == 0 {
		t.Fatalf("settings %+v %v", st, err)
	}
	bca, tunai := idOf(t, s, "BCA"), idOf(t, s, "Tunai")
	gaji, makan := idOf(t, s, "Gaji"), idOf(t, s, "Makan & Minum")

	mustCreate := func(tx Transaction) Transaction {
		t.Helper()
		out, err := s.CreateTransaction(ctx, tx)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	mustCreate(Transaction{Date: "2026-09-28", Type: "income", Amount: 11217866, AccountID: bca, CategoryID: i64(gaji)})
	mustCreate(Transaction{Date: "2026-09-28", Type: "transfer", Amount: 300000, AccountID: bca, ToAccountID: i64(tunai), Description: "Tarik tunai"})
	tx := mustCreate(Transaction{Date: "2026-09-30", Type: "expense", Amount: 24500, AccountID: tunai, CategoryID: i64(makan), Description: "Makan siang"})
	if tx.Description != "Makan siang" || tx.Source != "manual" {
		t.Fatalf("tx %+v", tx)
	}
	b, _ := s.GetAccount(ctx, bca)
	c, _ := s.GetAccount(ctx, tunai)
	if b.Balance != 11217866-300000 || c.Balance != 300000-24500 {
		t.Fatalf("balances bca=%d tunai=%d", b.Balance, c.Balance)
	}

	// rekonsiliasi saldo tunai
	adj, err := s.Reconcile(ctx, tunai, 270000, "2026-10-01")
	if err != nil || adj == nil || adj.Amount != 270000-275500 {
		t.Fatalf("reconcile %+v %v", adj, err)
	}
	c, _ = s.GetAccount(ctx, tunai)
	if c.Balance != 270000 {
		t.Fatalf("tunai after reconcile %d", c.Balance)
	}

	// validasi
	if _, err := s.CreateTransaction(ctx, Transaction{Type: "transfer", Amount: 1, AccountID: bca, ToAccountID: i64(bca)}); err == nil {
		t.Fatal("expected same-account transfer error")
	}
	if _, err := s.CreateTransaction(ctx, Transaction{Type: "expense", Amount: 0, AccountID: bca}); err == nil {
		t.Fatal("expected zero amount error")
	}
	// kategori pemasukan pada pengeluaran diabaikan
	x := mustCreate(Transaction{Type: "expense", Amount: 1000, AccountID: bca, CategoryID: i64(gaji), Description: "salah"})
	if x.CategoryID != nil {
		t.Fatal("mismatched category should be dropped")
	}
	// akun dengan transaksi tidak boleh dihapus
	if err := s.DeleteAccount(ctx, bca); err == nil {
		t.Fatal("expected delete error")
	}
}

func TestRulesLearning(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	bca, tunai, makan := idOf(t, s, "BCA"), idOf(t, s, "Tunai"), idOf(t, s, "Makan & Minum")
	if _, err := s.CreateTransaction(ctx, Transaction{Type: "expense", Amount: 25000, AccountID: tunai, CategoryID: i64(makan), Description: "Nasi Bebek Pak Slamet"}); err != nil {
		t.Fatal(err)
	}
	r := s.MatchRule(ctx, "kemarin beli nasi bebek pak slamet 30rb")
	if r == nil || *r.CategoryID != makan || *r.AccountID != tunai {
		t.Fatalf("rule %+v", r)
	}
	// duplikat
	tx, _ := s.CreateTransaction(ctx, Transaction{Date: "2026-10-05", Type: "expense", Amount: 50000, AccountID: bca, Description: "x"})
	if id := s.FindDuplicate(ctx, Transaction{Date: "2026-10-06", Type: "expense", Amount: 50000, AccountID: bca}); id != tx.ID {
		t.Fatalf("dup %d", id)
	}
	if id := s.FindDuplicate(ctx, Transaction{Date: "2026-10-10", Type: "expense", Amount: 50000, AccountID: bca}); id != 0 {
		t.Fatalf("unexpected dup %d", id)
	}
}

func TestBudgetCarryOverAndDashboard(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	cur, _ := s.CurrentPeriod(ctx)
	prev := cur.Prev()
	bca := idOf(t, s, "BCA")
	makan, rumah := idOf(t, s, "Makan & Minum"), idOf(t, s, "Cicilan Rumah")
	if _, err := s.UpsertBudget(ctx, Budget{Period: prev.Key, CategoryID: makan, Amount: 2000000}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpsertBudget(ctx, Budget{Period: prev.Key, CategoryID: rumah, Amount: 4638000, DueDay: 28}); err != nil {
		t.Fatal(err)
	}
	s.CreateTransaction(ctx, Transaction{Date: cur.StartStr(), Type: "expense", Amount: 4638000, AccountID: bca, CategoryID: i64(rumah)})
	s.CreateTransaction(ctx, Transaction{Date: s.Today().Format("2006-01-02"), Type: "expense", Amount: 100000, AccountID: bca, CategoryID: i64(makan)})

	d, err := s.Dashboard(ctx, cur)
	if err != nil {
		t.Fatal(err)
	}
	if !d.BudgetAutoCopied || len(d.Budgets) != 2 {
		t.Fatalf("carry over: copied=%v budgets=%d", d.BudgetAutoCopied, len(d.Budgets))
	}
	if d.PlanExpense != 6638000 || d.BillsPaid != 4638000 || d.FlexRemaining != 1900000 {
		t.Fatalf("plan=%d billsPaid=%d flexRem=%d", d.PlanExpense, d.BillsPaid, d.FlexRemaining)
	}
	if d.DaysLeft <= 0 || d.DailyAllowance != 1900000/int64(d.DaysLeft) {
		t.Fatalf("allowance %d days %d", d.DailyAllowance, d.DaysLeft)
	}
	if d.SpentToday < 100000 || d.Expense != 4738000 {
		t.Fatalf("today=%d expense=%d", d.SpentToday, d.Expense)
	}
	// periode lampau tidak ikut disalin
	old := prev.Prev()
	if copied, _ := s.EnsureBudget(ctx, old); copied {
		t.Fatal("past period should not be auto-filled")
	}
	r, err := s.Report(ctx, cur, 3)
	if err != nil || len(r.History) != 3 || r.History[2].Key != cur.Key {
		t.Fatalf("report %+v %v", r, err)
	}
}

func TestDebts(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	bca, cic := idOf(t, s, "BCA"), idOf(t, s, "Cicilan & Paylater")
	d, err := s.CreateDebt(ctx, Debt{Name: "Kredivo", Total: 2000000, Monthly: 500000, DueDay: 5})
	if err != nil {
		t.Fatal(err)
	}
	s.CreateTransaction(ctx, Transaction{Type: "expense", Amount: 714000, AccountID: bca, CategoryID: i64(cic), DebtID: i64(d.ID)})
	d, _ = s.GetDebt(ctx, d.ID)
	if d.Paid != 714000 || d.Remaining != 2000000-714000 {
		t.Fatalf("debt %+v", d)
	}
}

func TestTokens(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	raw, tok, err := s.CreateToken(ctx, "Shortcut")
	if err != nil || tok.ID == 0 || !s.VerifyToken(ctx, raw) || s.VerifyToken(ctx, raw+"x") {
		t.Fatalf("token %v %v", tok, err)
	}
}
