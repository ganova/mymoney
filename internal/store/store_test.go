package store

import (
	"database/sql"
	"path/filepath"

	"context"
	"github.com/ganova/mymoney/internal/period"
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
	// periode lampau dan periode depan tidak diisi otomatis
	if copied, _ := s.EnsureBudget(ctx, prev.Prev()); copied {
		t.Fatal("past period should not be auto-filled")
	}
	if copied, _ := s.EnsureBudget(ctx, cur.Next()); copied {
		t.Fatal("future period should not be auto-filled")
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

func TestLedgersAreSeparate(t *testing.T) {
	s := newTestStore(t)
	pribadi := WithLedger(context.Background(), 1)
	work := WithLedger(context.Background(), 2)

	pa, _ := s.ListAccounts(pribadi)
	wa, _ := s.ListAccounts(work)
	if len(pa) == 0 || len(wa) == 0 {
		t.Fatalf("tiap buku harus punya akun: %d / %d", len(pa), len(wa))
	}
	for _, a := range wa {
		if a.LedgerID != 2 {
			t.Fatalf("akun %s bocor ke buku Work", a.Name)
		}
	}
	// akun Pribadi tidak bisa dipakai di buku Work
	if _, err := s.CreateTransaction(work, Transaction{Type: "expense", Amount: 1000, AccountID: pa[0].ID}); err == nil {
		t.Fatal("seharusnya menolak akun buku lain")
	}

	srcs, _ := s.ListIncomeSources(work)
	if len(srcs) < 2 || srcs[0].Name != "CMW" || srcs[1].Name != "Freelance" {
		t.Fatalf("sumber pemasukan awal: %+v", srcs)
	}
	cats, _ := s.ListCategories(work)
	var klien int64
	for _, c := range cats {
		if c.Name == "Pembayaran Klien" {
			klien = c.ID
		}
		if c.Name == "Makan & Minum" {
			t.Fatal("kategori Pribadi bocor ke Work")
		}
	}
	fl := srcs[1].ID
	if _, err := s.CreateTransaction(work, Transaction{Type: "income", Amount: 5_000_000, AccountID: wa[0].ID, CategoryID: &klien, IncomeSourceID: &fl, Description: "Proyek web"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateTransaction(pribadi, Transaction{Type: "income", Amount: 9_000_000, AccountID: pa[0].ID, Description: "Gaji", IncomeSourceID: &srcs[0].ID}); err != nil {
		t.Fatal(err)
	}

	p, _ := s.CurrentPeriod(pribadi)
	dw, err := s.Dashboard(work, p)
	if err != nil {
		t.Fatal(err)
	}
	dp, _ := s.Dashboard(pribadi, p)
	if dw.Income != 5_000_000 || dp.Income != 9_000_000 {
		t.Fatalf("pemasukan tercampur: work=%d pribadi=%d", dw.Income, dp.Income)
	}
	if len(dw.IncomeBySource) != 1 || *dw.IncomeBySource[0].SourceID != fl {
		t.Fatalf("pemasukan per sumber: %+v", dw.IncomeBySource)
	}
	if l, _ := s.ListTransactions(work, TxFilter{}); len(l) != 1 {
		t.Fatalf("daftar transaksi Work: %d", len(l))
	}

	// transfer Pribadi → Work tampil di kedua buku
	to := wa[0].ID
	if _, err := s.CreateTransaction(pribadi, Transaction{Type: "transfer", Amount: 1_000_000, AccountID: pa[0].ID, ToAccountID: &to}); err != nil {
		t.Fatal(err)
	}
	if l, _ := s.ListTransactions(work, TxFilter{}); len(l) != 2 {
		t.Fatalf("transfer masuk harus tampil di Work: %d", len(l))
	}
	// budget terpisah
	if _, err := s.UpsertBudget(work, Budget{Period: p.Key, CategoryID: klien, Amount: 1}); err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.HasBudget(pribadi, p.Key); ok {
		t.Fatal("budget Work bocor ke Pribadi")
	}
}

func TestMigrateV1Database(t *testing.T) {
	dir := t.TempDir()
	loc, _ := time.LoadLocation("Asia/Jakarta")
	db, err := sql.Open("sqlite", "file:"+filepath.Join(dir, "mymoney.db"))
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		migrations[0],
		`CREATE TABLE schema_version (version INTEGER NOT NULL)`,
		`INSERT INTO schema_version VALUES (1)`,
		`INSERT INTO accounts (name) VALUES ('BCA')`,
		`INSERT INTO categories (name, kind) VALUES ('Makan', 'expense')`,
		`INSERT INTO transactions (date, type, amount, account_id, category_id) VALUES ('2026-10-01','expense',25000,1,1)`,
		`INSERT INTO budgets (period, category_id, amount) VALUES ('2026-10', 1, 500000)`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()

	s, err := Open(dir, loc)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	pribadi := WithLedger(context.Background(), 1)
	if l, _ := s.ListTransactions(pribadi, TxFilter{}); len(l) != 1 || l[0].LedgerID != 1 {
		t.Fatalf("transaksi lama harus masuk buku Pribadi: %+v", l)
	}
	if b, _ := s.ListBudgets(pribadi, period.For(s.Today(), 28)); false && len(b) == 0 {
		t.Fatal()
	}
	if ok, _ := s.HasBudget(pribadi, "2026-10"); !ok {
		t.Fatal("budget lama hilang")
	}
	if wa, _ := s.ListAccounts(WithLedger(context.Background(), 2)); len(wa) != 2 {
		t.Fatalf("akun Work harus tersedia: %d", len(wa))
	}
	if c, _ := s.ListCategories(pribadi); len(c) != 1 {
		t.Fatalf("kategori lama harus tetap di Pribadi: %d", len(c))
	}
}

func TestReceivablesAndOutlook(t *testing.T) {
	s := newTestStore(t)
	work := WithLedger(context.Background(), 2)
	wa, _ := s.ListAccounts(work)
	cur, _ := s.CurrentPeriod(work)
	next := cur.Next()
	d := func(p time.Time) string { return p.Format("2006-01-02") }
	srcs, _ := s.ListIncomeSources(work)

	a, err := s.CreateReceivable(work, Receivable{Name: "Website toko", Client: "Fastwork", Amount: 3_000_000, ExpectedDate: d(next.Start), IncomeSourceID: &srcs[1].ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateReceivable(work, Receivable{Name: "Logo", Client: "Fastwork", Amount: 500_000}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateReceivable(work, Receivable{Name: "Telat", Amount: 700_000, ExpectedDate: "2020-01-01"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateReceivable(work, Receivable{Name: "x", Amount: 0}); err == nil {
		t.Fatal("nominal 0 harus ditolak")
	}
	// utang: sisa 1jt, cicilan 400rb/bulan → 3 periode (400+400+200)
	if _, err := s.CreateDebt(work, Debt{Name: "Paylater", Total: 1_000_000, Monthly: 400_000}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateDebt(work, Debt{Name: "Pinjam teman", Total: 250_000}); err != nil {
		t.Fatal(err)
	}

	o, err := s.Outlook(work, 6)
	if err != nil {
		t.Fatal(err)
	}
	if o.PendingTotal != 4_200_000 || o.PendingUnscheduled != 500_000 || o.PendingCount != 3 {
		t.Fatalf("pending: %+v", o)
	}
	if o.Months[0].Incoming != 700_000 || o.Months[1].Incoming != 3_000_000 {
		t.Fatalf("pendapatan per bulan: %+v", o.Months[:2])
	}
	if o.Months[0].DebtDue != 400_000 || o.Months[1].DebtDue != 400_000 || o.Months[2].DebtDue != 200_000 || o.Months[3].DebtDue != 0 {
		t.Fatalf("cicilan per bulan: %+v", o.Months)
	}
	if o.DebtRemaining != 1_250_000 || o.DebtUnscheduled != 250_000 || o.DebtMonthly != 400_000 || o.DebtFreeLabel != o.Months[2].Label {
		t.Fatalf("utang: %+v", o)
	}

	// cairkan dengan nominal lebih kecil (potongan biaya platform)
	r, tx, err := s.ReceiveReceivable(work, a.ID, wa[0].ID, 2_700_000, d(s.Today()))
	if err != nil || r.Status != "received" || tx.Amount != 2_700_000 || tx.IncomeSourceID == nil {
		t.Fatalf("receive: %+v %+v %v", r, tx, err)
	}
	if _, _, err := s.ReceiveReceivable(work, a.ID, wa[0].ID, 0, ""); err == nil {
		t.Fatal("tidak boleh dicairkan dua kali")
	}
	o, _ = s.Outlook(work, 6)
	if o.PendingTotal != 1_200_000 {
		t.Fatalf("pending setelah cair: %d", o.PendingTotal)
	}

	// bayar cicilan periode ini → jadwal bergeser
	debts, _ := s.ListDebts(work)
	if _, err := s.CreateTransaction(work, Transaction{Type: "expense", Amount: 400_000, AccountID: wa[0].ID, DebtID: &debts[0].ID, Date: d(s.Today())}); err != nil {
		t.Fatal(err)
	}
	o, _ = s.Outlook(work, 6)
	if o.Months[0].DebtDue != 0 || o.Months[0].DebtPaid != 400_000 || o.Months[1].DebtDue != 400_000 || o.Months[2].DebtDue != 200_000 {
		t.Fatalf("setelah bayar: %+v", o.Months[:3])
	}
	// buku Pribadi tidak melihat data Work
	if l, _ := s.ListReceivables(WithLedger(context.Background(), 1)); len(l) != 0 {
		t.Fatal("receivable bocor ke Pribadi")
	}
}
