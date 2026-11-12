package server

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/xuri/excelize/v2"

	"github.com/ganova/mymoney/internal/money"
	"github.com/ganova/mymoney/internal/period"
	"github.com/ganova/mymoney/internal/store"
)

// ensureMonthlyReports membuat laporan bulanan (xlsx) periode SEBELUMNYA untuk
// setiap buku yang punya data, di folder dir. Karena periode berganti pada
// tanggal mulai periode (bawaan 28), laporan muncul otomatis di tanggal itu;
// bila PC sedang mati, dibuat saat aplikasi berikutnya berjalan. Berkas yang
// sudah ada tidak ditimpa kecuali overwrite = true.
func (s *Server) ensureMonthlyReports(ctx context.Context, dir string, overwrite bool) ([]string, error) {
	ledgers, err := s.store.ListLedgers(ctx)
	if err != nil {
		return nil, err
	}
	var made []string
	for _, l := range ledgers {
		lctx := store.WithLedger(ctx, l.ID)
		cur, err := s.store.CurrentPeriod(lctx)
		if err != nil {
			return made, err
		}
		prev := cur.Prev()
		path := filepath.Join(dir, reportFileName(prev, l.Name))
		if _, err := os.Stat(path); err == nil && !overwrite {
			continue
		}
		x, ok, err := s.buildMonthlyReport(lctx, prev, l.Name)
		if err != nil {
			return made, err
		}
		if !ok {
			continue // buku tanpa transaksi di periode itu
		}
		err = x.SaveAs(path)
		x.Close()
		if err != nil {
			return made, fmt.Errorf("menyimpan %s: %w", path, err)
		}
		made = append(made, path)
	}
	return made, nil
}

// reportFileName: "Laporan Oktober 2026 - Utama.xlsx".
func reportFileName(p period.Period, ledger string) string {
	clean := strings.Map(func(r rune) rune {
		if strings.ContainsRune(`\/:*?"<>|`, r) {
			return '-'
		}
		return r
	}, strings.TrimSpace(ledger))
	if clean == "" {
		clean = "Buku"
	}
	return fmt.Sprintf("Laporan %s - %s.xlsx", p.Label, clean)
}

// buildMonthlyReport menyusun workbook laporan lengkap satu periode untuk buku
// pada ctx. ok = false bila periode itu tidak punya transaksi.
func (s *Server) buildMonthlyReport(ctx context.Context, p period.Period, ledger string) (*excelize.File, bool, error) {
	f := store.TxFilter{From: p.StartStr(), To: p.EndStr(), Limit: 1000}
	var txs []store.Transaction
	for {
		page, err := s.store.ListTransactions(ctx, f)
		if err != nil {
			return nil, false, err
		}
		txs = append(txs, page...)
		if len(page) < f.Limit {
			break
		}
		f.Offset += f.Limit
	}
	if len(txs) == 0 {
		return nil, false, nil
	}
	d, err := s.store.Dashboard(ctx, p)
	if err != nil {
		return nil, false, err
	}
	rep, err := s.store.Report(ctx, p, 6)
	if err != nil {
		return nil, false, err
	}
	accs, _ := s.store.ListAccounts(ctx)
	cats, _ := s.store.ListCategories(ctx)
	srcs, _ := s.store.ListIncomeSources(ctx)
	debts, _ := s.store.ListDebts(ctx)
	accName, catName, catGroup, srcName := map[int64]string{}, map[int64]string{}, map[int64]string{}, map[int64]string{}
	for _, a := range accs {
		accName[a.ID] = a.Name
	}
	for _, c := range cats {
		catName[c.ID], catGroup[c.ID] = c.Name, c.Group
	}
	for _, x := range srcs {
		srcName[x.ID] = x.Name
	}
	nameOf := func(id *int64, m map[int64]string, empty string) string {
		if id == nil {
			return empty
		}
		if v, ok := m[*id]; ok {
			return v
		}
		return empty
	}

	x := excelize.NewFile()
	title, _ := x.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true, Size: 16}})
	head, _ := x.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}, Fill: excelize.Fill{Type: "pattern", Color: []string{"#E8EEF7"}, Pattern: 1}})
	sect, _ := x.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true, Size: 12, Color: "#1C5CAB"}})
	bold, _ := x.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	rp, _ := x.NewStyle(&excelize.Style{CustomNumFmt: strPtr(`"Rp"#,##0;[Red]-"Rp"#,##0`)})
	rpBold, _ := x.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}, CustomNumFmt: strPtr(`"Rp"#,##0;[Red]-"Rp"#,##0`)})
	cell := func(col, row int) string {
		c, _ := excelize.CoordinatesToCellName(col, row)
		return c
	}
	// table menulis satu tabel berjudul di (sheet, row) dan mengembalikan baris kosong berikutnya.
	table := func(sheet string, row int, heading string, header []any, rows [][]any, moneyCols ...int) int {
		if heading != "" {
			x.SetCellValue(sheet, cell(1, row), heading)
			x.SetCellStyle(sheet, cell(1, row), cell(1, row), sect)
			row++
		}
		x.SetSheetRow(sheet, cell(1, row), &header)
		x.SetCellStyle(sheet, cell(1, row), cell(len(header), row), head)
		first := row + 1
		for i, r := range rows {
			vals := r
			x.SetSheetRow(sheet, cell(1, first+i), &vals)
		}
		last := first + len(rows) - 1
		if len(rows) > 0 {
			for _, c := range moneyCols {
				x.SetCellStyle(sheet, cell(c, first), cell(c, last), rp)
			}
		}
		return last + 2
	}

	// ---------- Ringkasan ----------
	sum := "Ringkasan"
	x.SetSheetName("Sheet1", sum)
	x.SetCellValue(sum, "A1", "Laporan Keuangan "+p.Label)
	x.SetCellStyle(sum, "A1", "A1", title)
	x.SetCellValue(sum, "A2", fmt.Sprintf("Buku: %s  ·  Periode: %s  ·  Dibuat: %s", ledger, p.Range(), s.store.Today().Format("2006-01-02")))
	row := 4
	kv := [][]any{
		{"Pemasukan", d.Income}, {"Pengeluaran", d.Expense}, {"Selisih", d.Net},
		{"Rencana pemasukan", d.PlanIncome}, {"Rencana pengeluaran", d.PlanExpense},
		{"Budget wajib (rencana)", d.BillsTotal}, {"Budget wajib terbayar", d.BillsPaid},
		{"Budget fleksibel (rencana)", d.FlexPlan}, {"Budget fleksibel terpakai", d.FlexSpent},
		{"Pengeluaran di luar budget", d.Unbudgeted},
		{"Pengeluaran periode sebelumnya", d.PrevExpense},
	}
	for i, r := range kv {
		vals := r
		x.SetSheetRow(sum, cell(1, row+i), &vals)
	}
	x.SetCellStyle(sum, cell(2, row), cell(2, row+len(kv)-1), rp)
	x.SetCellStyle(sum, cell(1, row), cell(1, row+2), bold)
	x.SetCellStyle(sum, cell(2, row), cell(2, row+2), rpBold)
	row += len(kv) + 1

	if len(d.IncomeBySource) > 0 {
		var rows [][]any
		for _, it := range d.IncomeBySource {
			rows = append(rows, []any{nameOf(it.SourceID, srcName, "Tanpa sumber"), it.Total, it.Count})
		}
		row = table(sum, row, "Pemasukan per sumber", []any{"Sumber", "Total", "Transaksi"}, rows, 2)
	}
	if w := d.Weekly; w != nil && len(w.Weeks) > 0 {
		var rows [][]any
		for _, wk := range w.Weeks {
			rows = append(rows, []any{fmt.Sprintf("Minggu %d", wk.Index), wk.Start + " s/d " + wk.End, wk.Days, wk.Base, wk.Carry, wk.Allowance, wk.Spent, wk.Remaining})
		}
		rows = append(rows, []any{"Total", "", p.Days(), w.Pool, "", "", w.Spent, w.Remaining})
		row = table(sum, row, fmt.Sprintf("Jatah mingguan (pendapatan %s − wajib %s = %s)", money.Format(w.Income), money.Format(w.Mandatory), money.Format(w.Pool)),
			[]any{"Minggu", "Tanggal", "Hari", "Jatah dasar", "Dibawa", "Jatah", "Terpakai", "Sisa"}, rows, 4, 5, 6, 7, 8)
	}
	if len(rep.ByAccount) > 0 {
		var rows [][]any
		for _, a := range rep.ByAccount {
			rows = append(rows, []any{accName[a.AccountID], a.Income, a.Expense})
		}
		row = table(sum, row, "Per akun", []any{"Akun", "Masuk", "Keluar"}, rows, 2, 3)
	}
	if len(rep.History) > 0 {
		var rows [][]any
		for _, h := range rep.History {
			rows = append(rows, []any{h.Label, h.Income, h.Expense, h.Income - h.Expense})
		}
		row = table(sum, row, "Tren 6 periode", []any{"Periode", "Pemasukan", "Pengeluaran", "Selisih"}, rows, 2, 3, 4)
	}
	{
		var rows [][]any
		for _, a := range accs {
			if !a.Archived {
				rows = append(rows, []any{a.Name, a.Balance})
			}
		}
		row = table(sum, row, "Saldo akun (saat laporan dibuat)", []any{"Akun", "Saldo"}, rows, 2)
	}
	{
		var rows [][]any
		for _, x := range debts {
			if !x.Archived && x.Remaining > 0 {
				rows = append(rows, []any{x.Name, x.Total, x.Paid, x.Remaining, x.Monthly})
			}
		}
		if len(rows) > 0 {
			table(sum, row, "Utang & cicilan (saat laporan dibuat)", []any{"Utang", "Total", "Terbayar", "Sisa", "Cicilan/bulan"}, rows, 2, 3, 4, 5)
		}
	}
	x.SetColWidth(sum, "A", "A", 34)
	x.SetColWidth(sum, "B", "H", 18)

	// ---------- Per Kategori ----------
	sh := "Per Kategori"
	x.NewSheet(sh)
	{
		var rows [][]any
		for _, c := range d.ByCategory {
			base := d.Expense
			jenis := "Pengeluaran"
			if c.Kind == "income" {
				base, jenis = d.Income, "Pemasukan"
			}
			pct := 0.0
			if base > 0 {
				pct = float64(c.Total) / float64(base)
			}
			rows = append(rows, []any{nameOf(c.CategoryID, catName, "Tanpa kategori"), nameOf(c.CategoryID, catGroup, ""), jenis, c.Total, c.Count, pct})
		}
		end := table(sh, 1, "", []any{"Kategori", "Kelompok", "Jenis", "Total", "Transaksi", "Porsi"}, rows, 4)
		pctStyle, _ := x.NewStyle(&excelize.Style{NumFmt: 9})
		if len(rows) > 0 {
			x.SetCellStyle(sh, "F2", cell(6, end-2), pctStyle)
		}
		x.SetColWidth(sh, "A", "B", 26)
		x.SetColWidth(sh, "C", "F", 14)
		x.SetPanes(sh, &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"})
	}

	// ---------- Budget ----------
	if len(d.Budgets) > 0 {
		sh := "Budget"
		x.NewSheet(sh)
		var rows [][]any
		for _, b := range d.Budgets {
			jenis := "Fleksibel"
			switch {
			case b.Kind == "income":
				jenis = "Pemasukan"
			case b.Mandatory:
				jenis = "Wajib"
			}
			due := ""
			if b.DueDay > 0 {
				due = b.DueDate
			}
			rows = append(rows, []any{catName[b.CategoryID], jenis, b.Amount, b.Actual, b.Amount - b.Actual, due, b.Note})
		}
		table(sh, 1, "", []any{"Kategori", "Jenis", "Rencana", "Aktual", "Sisa", "Jatuh tempo", "Catatan"}, rows, 3, 4, 5)
		x.SetColWidth(sh, "A", "A", 26)
		x.SetColWidth(sh, "B", "F", 15)
		x.SetColWidth(sh, "G", "G", 40)
		x.SetPanes(sh, &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"})
	}

	// ---------- Harian ----------
	{
		sh := "Harian"
		x.NewSheet(sh)
		var rows [][]any
		for _, day := range d.Daily {
			rows = append(rows, []any{day.Date, day.Income, day.Expense})
		}
		table(sh, 1, "", []any{"Tanggal", "Pemasukan", "Pengeluaran"}, rows, 2, 3)
		x.SetColWidth(sh, "A", "C", 16)
	}

	// ---------- Pengeluaran Terbesar ----------
	if len(rep.Top) > 0 {
		sh := "Pengeluaran Terbesar"
		x.NewSheet(sh)
		var rows [][]any
		for i, t := range rep.Top {
			rows = append(rows, []any{i + 1, t.Description, nameOf(t.CategoryID, catName, ""), t.Total, t.Count})
		}
		table(sh, 1, "", []any{"No", "Deskripsi", "Kategori", "Total", "Transaksi"}, rows, 4)
		x.SetColWidth(sh, "B", "C", 30)
		x.SetColWidth(sh, "D", "E", 15)
	}

	// ---------- Transaksi ----------
	{
		sh := "Transaksi"
		x.NewSheet(sh)
		typeName := map[string]string{"expense": "Pengeluaran", "income": "Pemasukan", "transfer": "Transfer", "adjust": "Penyesuaian"}
		var rows [][]any
		for i := len(txs) - 1; i >= 0; i-- { // urut tanggal naik
			t := txs[i]
			rows = append(rows, []any{t.Date, typeName[t.Type], t.Amount, t.Description, nameOf(t.CategoryID, catName, ""), accName[t.AccountID],
				nameOf(t.ToAccountID, accName, ""), nameOf(t.IncomeSourceID, srcName, ""), t.Note, t.Source})
		}
		table(sh, 1, "", []any{"Tanggal", "Jenis", "Nominal", "Deskripsi", "Kategori", "Akun", "Akun tujuan", "Sumber pemasukan", "Catatan", "Asal data"}, rows, 3)
		x.SetColWidth(sh, "A", "B", 13)
		x.SetColWidth(sh, "C", "C", 16)
		x.SetColWidth(sh, "D", "D", 34)
		x.SetColWidth(sh, "E", "I", 20)
		x.SetPanes(sh, &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"})
	}
	if idx, err := x.GetSheetIndex(sum); err == nil {
		x.SetActiveSheet(idx)
	}
	return x, true, nil
}
