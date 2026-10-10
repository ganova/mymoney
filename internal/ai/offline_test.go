package ai

import (
	"context"
	"testing"
	"time"

	"github.com/ganova/mymoney/internal/store"
)

func testStore(t *testing.T) *store.Store {
	t.Helper()
	loc, _ := time.LoadLocation("Asia/Jakarta")
	s, err := store.OpenMemory(loc)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func names(t *testing.T, st *store.Store) (acc map[int64]string, cat map[int64]string) {
	ctx := context.Background()
	acc, cat = map[int64]string{}, map[int64]string{}
	as, _ := st.ListAccounts(ctx)
	for _, a := range as {
		acc[a.ID] = a.Name
	}
	cs, _ := st.ListCategories(ctx)
	for _, c := range cs {
		cat[c.ID] = c.Name
	}
	return
}

func TestParseOffline(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	acc, cat := names(t, st)
	today := st.Today()

	type want struct {
		typ, desc, cat, acc, to string
		amount                  int64
		date                    string
	}
	cases := map[string][]want{
		"beli makan siang 25rb":  {{"expense", "Makan siang", "Makan & Minum", "BCA", "", 25000, today.Format("2006-01-02")}},
		"bensin 50rb pakai cash": {{"expense", "Bensin", "Bensin & Transport", "Tunai", "", 50000, ""}},
		"tarik tunai 300rb":      {{"transfer", "Tarik tunai", "", "BCA", "Tunai", 300000, ""}},
		"topup gopay 100k":       {{"transfer", "Top up GoPay", "", "BCA", "GoPay", 100000, ""}},
		"gajian 11.217.866":      {{"income", "Gajian", "Gaji", "BCA", "", 11217866, ""}},
		"kemarin indomaret 53.800, parkir 5": {
			{"expense", "Indomaret", "Belanja Harian", "BCA", "", 53800, today.AddDate(0, 0, -1).Format("2006-01-02")},
			{"expense", "Parkir", "Bensin & Transport", "BCA", "", 5000, ""},
		},
	}
	for in, wants := range cases {
		drafts, err := ParseOffline(ctx, st, in, "ai_text")
		if err != nil {
			t.Fatal(err)
		}
		if len(drafts) != len(wants) {
			t.Errorf("%q: got %d drafts %+v", in, len(drafts), drafts)
			continue
		}
		for i, w := range wants {
			d := drafts[i]
			catName := ""
			if d.CategoryID != nil {
				catName = cat[*d.CategoryID]
			}
			to := ""
			if d.ToAccountID != nil {
				to = acc[*d.ToAccountID]
			}
			if d.Type != w.typ || d.Description != w.desc || catName != w.cat || acc[d.AccountID] != w.acc || to != w.to || d.Amount != w.amount {
				t.Errorf("%q[%d]: got type=%s desc=%q cat=%q acc=%s to=%s amount=%d", in, i, d.Type, d.Description, catName, acc[d.AccountID], to, d.Amount)
			}
			if w.date != "" && d.Date != w.date {
				t.Errorf("%q[%d]: date %s want %s", in, i, d.Date, w.date)
			}
		}
	}
}

func TestParseOfflineLearnsAndEstimates(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	acc, cat := names(t, st)
	var tunai, makan int64
	for id, n := range acc {
		if n == "Tunai" {
			tunai = id
		}
	}
	for id, n := range cat {
		if n == "Makan & Minum" {
			makan = id
		}
	}
	if _, err := st.CreateTransaction(ctx, store.Transaction{Type: "expense", Amount: 27000, AccountID: tunai, CategoryID: &makan, Description: "Makan siang"}); err != nil {
		t.Fatal(err)
	}
	drafts, _ := ParseOffline(ctx, st, "makan siang", "ai_text")
	if len(drafts) != 1 {
		t.Fatalf("got %+v", drafts)
	}
	d := drafts[0]
	if d.Amount != 27000 || !d.AmountEstimated || acc[d.AccountID] != "Tunai" || cat[*d.CategoryID] != "Makan & Minum" {
		t.Fatalf("got %+v", d)
	}
}
