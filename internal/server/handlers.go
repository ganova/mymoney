package server

import (
	"net/http"
	"strings"

	"github.com/ganova/mymoney/internal/store"
)

// ---------- bootstrap & pengaturan ----------

func (s *Server) handleBootstrap(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	st, err := s.store.GetSettings(ctx)
	if err != nil {
		fail(w, err)
		return
	}
	accs, err := s.store.ListAccounts(ctx)
	if err != nil {
		fail(w, err)
		return
	}
	cats, err := s.store.ListCategories(ctx)
	if err != nil {
		fail(w, err)
		return
	}
	debts, err := s.store.ListDebts(ctx)
	if err != nil {
		fail(w, err)
		return
	}
	cur, err := s.store.CurrentPeriod(ctx)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{
		"settings":      st,
		"accounts":      accs,
		"categories":    cats,
		"debts":         debts,
		"period":        cur.Info(),
		"today":         s.store.Today().Format("2006-01-02"),
		"ai":            map[string]any{"enabled": s.ai.Enabled(), "model": s.ai.Model()},
		"auth_required": s.authRequired(),
		"version":       Version,
	})
}

func (s *Server) handleSaveSettings(w http.ResponseWriter, r *http.Request) {
	var in store.Settings
	if err := decode(r, &in); err != nil {
		fail(w, err)
		return
	}
	if err := s.store.SaveSettings(r.Context(), in); err != nil {
		fail(w, err)
		return
	}
	st, err := s.store.GetSettings(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, st)
}

func (s *Server) handlePeriod(w http.ResponseWriter, r *http.Request) {
	p, err := s.store.ResolvePeriod(r.Context(), r.URL.Query().Get("period"))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, p.Info())
}

// ---------- akun ----------

func (s *Server) handleListAccounts(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListAccounts(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, list)
}

func (s *Server) handleCreateAccount(w http.ResponseWriter, r *http.Request) {
	var in store.Account
	if err := decode(r, &in); err != nil {
		fail(w, err)
		return
	}
	a, err := s.store.CreateAccount(r.Context(), in)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 201, a)
}

func (s *Server) handleUpdateAccount(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		fail(w, err)
		return
	}
	var in store.Account
	if err := decode(r, &in); err != nil {
		fail(w, err)
		return
	}
	in.ID = id
	a, err := s.store.UpdateAccount(r.Context(), in)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, a)
}

func (s *Server) handleDeleteAccount(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		fail(w, err)
		return
	}
	if err := s.store.DeleteAccount(r.Context(), id); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) handleReconcile(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		fail(w, err)
		return
	}
	var in struct {
		Balance int64  `json:"balance"`
		Date    string `json:"date"`
	}
	if err := decode(r, &in); err != nil {
		fail(w, err)
		return
	}
	tx, err := s.store.Reconcile(r.Context(), id, in.Balance, in.Date)
	if err != nil {
		fail(w, err)
		return
	}
	a, err := s.store.GetAccount(r.Context(), id)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"account": a, "transaction": tx})
}

// ---------- kategori ----------

func (s *Server) handleListCategories(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListCategories(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, list)
}

func (s *Server) handleCreateCategory(w http.ResponseWriter, r *http.Request) {
	var in store.Category
	if err := decode(r, &in); err != nil {
		fail(w, err)
		return
	}
	c, err := s.store.CreateCategory(r.Context(), in)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 201, c)
}

func (s *Server) handleUpdateCategory(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		fail(w, err)
		return
	}
	var in store.Category
	if err := decode(r, &in); err != nil {
		fail(w, err)
		return
	}
	in.ID = id
	c, err := s.store.UpdateCategory(r.Context(), in)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, c)
}

func (s *Server) handleDeleteCategory(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		fail(w, err)
		return
	}
	if err := s.store.DeleteCategory(r.Context(), id); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// ---------- transaksi ----------

func (s *Server) handleListTransactions(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := store.TxFilter{
		From: q.Get("from"), To: q.Get("to"), Type: q.Get("type"), Q: q.Get("q"),
		AccountID: queryInt(r, "account"), CategoryID: queryInt(r, "category"), DebtID: queryInt(r, "debt"),
		Limit: int(queryInt(r, "limit")), Offset: int(queryInt(r, "offset")),
	}
	if key := q.Get("period"); key != "" {
		p, err := s.store.ResolvePeriod(r.Context(), key)
		if err != nil {
			fail(w, err)
			return
		}
		f.From, f.To = p.StartStr(), p.EndStr()
	}
	list, err := s.store.ListTransactions(r.Context(), f)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, list)
}

func (s *Server) handleCreateTransaction(w http.ResponseWriter, r *http.Request) {
	var in store.Transaction
	if err := decode(r, &in); err != nil {
		fail(w, err)
		return
	}
	t, err := s.store.CreateTransaction(r.Context(), in)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 201, t)
}

func (s *Server) handleBulkCreate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Transactions []store.Transaction `json:"transactions"`
	}
	if err := decode(r, &in); err != nil {
		fail(w, err)
		return
	}
	if len(in.Transactions) == 0 {
		writeError(w, 400, "tidak ada transaksi")
		return
	}
	list, err := s.store.CreateTransactions(r.Context(), in.Transactions)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 201, list)
}

func (s *Server) handleUpdateTransaction(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		fail(w, err)
		return
	}
	var in store.Transaction
	if err := decode(r, &in); err != nil {
		fail(w, err)
		return
	}
	in.ID = id
	t, err := s.store.UpdateTransaction(r.Context(), in)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, t)
}

func (s *Server) handleDeleteTransaction(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		fail(w, err)
		return
	}
	if err := s.store.DeleteTransactions(r.Context(), []int64{id}); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) handleBulkDelete(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs []int64 `json:"ids"`
	}
	if err := decode(r, &in); err != nil {
		fail(w, err)
		return
	}
	if err := s.store.DeleteTransactions(r.Context(), in.IDs); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "deleted": len(in.IDs)})
}

// ---------- budget ----------

func (s *Server) handleListBudgets(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	p, err := s.store.ResolvePeriod(ctx, r.URL.Query().Get("period"))
	if err != nil {
		fail(w, err)
		return
	}
	copied, err := s.store.EnsureBudget(ctx, p)
	if err != nil {
		fail(w, err)
		return
	}
	list, err := s.store.ListBudgets(ctx, p)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"period": p.Info(), "budgets": list, "auto_copied": copied})
}

func (s *Server) handleUpsertBudget(w http.ResponseWriter, r *http.Request) {
	var in store.Budget
	if err := decode(r, &in); err != nil {
		fail(w, err)
		return
	}
	b, err := s.store.UpsertBudget(r.Context(), in)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, b)
}

func (s *Server) handleDeleteBudget(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		fail(w, err)
		return
	}
	if err := s.store.DeleteBudget(r.Context(), id); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) handleCopyBudget(w http.ResponseWriter, r *http.Request) {
	var in struct {
		From      string `json:"from"`
		To        string `json:"to"`
		Overwrite bool   `json:"overwrite"`
	}
	if err := decode(r, &in); err != nil {
		fail(w, err)
		return
	}
	n, err := s.store.CopyBudget(r.Context(), in.From, in.To, in.Overwrite)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"copied": n})
}

// handleApplyBudget menyimpan hasil usulan budget AI (membuat kategori baru bila perlu).
func (s *Server) handleApplyBudget(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in struct {
		Period  string `json:"period"`
		Replace bool   `json:"replace"`
		Lines   []struct {
			CategoryID  *int64 `json:"category_id"`
			NewCategory *struct {
				Name  string `json:"name"`
				Group string `json:"group"`
				Icon  string `json:"icon"`
				Kind  string `json:"kind"`
			} `json:"new_category"`
			Amount int64  `json:"amount"`
			DueDay int    `json:"due_day"`
			Note   string `json:"note"`
		} `json:"lines"`
	}
	if err := decode(r, &in); err != nil {
		fail(w, err)
		return
	}
	p, err := s.store.ResolvePeriod(ctx, in.Period)
	if err != nil {
		fail(w, err)
		return
	}
	if in.Replace {
		if err := s.store.ClearBudget(ctx, p.Key); err != nil {
			fail(w, err)
			return
		}
	}
	// gabungkan baris yang menuju kategori sama, lalu simpan
	type agg struct {
		amount int64
		due    int
		notes  []string
	}
	order := []int64{}
	byCat := map[int64]*agg{}
	for _, l := range in.Lines {
		var catID int64
		switch {
		case l.CategoryID != nil && *l.CategoryID > 0:
			catID = *l.CategoryID
		case l.NewCategory != nil && strings.TrimSpace(l.NewCategory.Name) != "":
			c, err := s.store.CreateCategory(ctx, store.Category{Name: l.NewCategory.Name, Group: l.NewCategory.Group, Icon: l.NewCategory.Icon, Kind: l.NewCategory.Kind})
			if err != nil {
				fail(w, err)
				return
			}
			catID = c.ID
		default:
			continue
		}
		a, ok := byCat[catID]
		if !ok {
			a = &agg{}
			byCat[catID] = a
			order = append(order, catID)
		}
		a.amount += l.Amount
		a.due = max(a.due, l.DueDay)
		if n := strings.TrimSpace(l.Note); n != "" {
			a.notes = append(a.notes, n)
		}
	}
	saved := 0
	for _, catID := range order {
		a := byCat[catID]
		if _, err := s.store.UpsertBudget(ctx, store.Budget{Period: p.Key, CategoryID: catID, Amount: a.amount, DueDay: a.due, Note: strings.Join(a.notes, "; ")}); err != nil {
			fail(w, err)
			return
		}
		saved++
	}
	writeJSON(w, 200, map[string]any{"saved": saved})
}

// ---------- ringkasan ----------

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	p, err := s.store.ResolvePeriod(r.Context(), r.URL.Query().Get("period"))
	if err != nil {
		fail(w, err)
		return
	}
	d, err := s.store.Dashboard(r.Context(), p)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, d)
}

func (s *Server) handleReport(w http.ResponseWriter, r *http.Request) {
	p, err := s.store.ResolvePeriod(r.Context(), r.URL.Query().Get("period"))
	if err != nil {
		fail(w, err)
		return
	}
	rep, err := s.store.Report(r.Context(), p, 6)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, rep)
}

// ---------- utang ----------

func (s *Server) handleListDebts(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListDebts(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, list)
}

func (s *Server) handleCreateDebt(w http.ResponseWriter, r *http.Request) {
	var in store.Debt
	if err := decode(r, &in); err != nil {
		fail(w, err)
		return
	}
	d, err := s.store.CreateDebt(r.Context(), in)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 201, d)
}

func (s *Server) handleUpdateDebt(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		fail(w, err)
		return
	}
	var in store.Debt
	if err := decode(r, &in); err != nil {
		fail(w, err)
		return
	}
	in.ID = id
	d, err := s.store.UpdateDebt(r.Context(), in)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, d)
}

func (s *Server) handleDeleteDebt(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		fail(w, err)
		return
	}
	if err := s.store.DeleteDebt(r.Context(), id); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// ---------- aturan & token ----------

func (s *Server) handleListRules(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListRules(r.Context(), 500)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, list)
}

func (s *Server) handleDeleteRule(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		fail(w, err)
		return
	}
	if err := s.store.DeleteRule(r.Context(), id); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) handleListTokens(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListTokens(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, list)
}

func (s *Server) handleCreateToken(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name string `json:"name"`
	}
	decode(r, &in)
	raw, t, err := s.store.CreateToken(r.Context(), in.Name)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 201, map[string]any{"token": raw, "info": t})
}

func (s *Server) handleDeleteToken(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		fail(w, err)
		return
	}
	if err := s.store.DeleteToken(r.Context(), id); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}
