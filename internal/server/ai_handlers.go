package server

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"

	"github.com/ganova/mymoney/internal/ai"
	"github.com/ganova/mymoney/internal/money"
	"github.com/ganova/mymoney/internal/ocr"
	"github.com/ganova/mymoney/internal/store"
)

const maxUpload = 40 << 20

type askResponse struct {
	Reply  string     `json:"reply"`
	Drafts []ai.Draft `json:"drafts"`
	Mode   string     `json:"mode"` // ai | offline
}

// ask menjalankan asisten AI, dengan parser offline sebagai cadangan.
func (s *Server) ask(ctx context.Context, history []ai.HistoryItem, text, source string) (*askResponse, error) {
	if !s.ai.Enabled() {
		drafts, err := ai.ParseOffline(ctx, s.store, text, source)
		if err != nil {
			return nil, err
		}
		reply := "Mode gratis (tanpa AI): draf dibuat dari kata kunci & kebiasaanmu. Cek sebelum menyimpan."
		if len(drafts) == 0 {
			reply = "Fitur tanya-jawab butuh AI. Isi ANTHROPIC_API_KEY di file .env, atau ketik transaksi seperti \"makan siang 25rb\"."
		}
		return &askResponse{Reply: reply, Drafts: nonNil(drafts), Mode: "offline"}, nil
	}
	res, err := s.ai.Ask(ctx, history, text, source)
	if err != nil {
		drafts, perr := ai.ParseOffline(ctx, s.store, text, source)
		if perr == nil && len(drafts) > 0 {
			return &askResponse{Reply: "⚠️ " + err.Error() + ". Draf dibuat dengan parser offline — cek sebelum disimpan.", Drafts: drafts, Mode: "offline"}, nil
		}
		return nil, err
	}
	return &askResponse{Reply: res.Reply, Drafts: nonNil(res.Drafts), Mode: "ai"}, nil
}

func nonNil(d []ai.Draft) []ai.Draft {
	if d == nil {
		return []ai.Draft{}
	}
	return d
}

func (s *Server) handleAsk(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Text    string           `json:"text"`
		History []ai.HistoryItem `json:"history"`
	}
	if err := decode(r, &in); err != nil {
		fail(w, err)
		return
	}
	in.Text = strings.TrimSpace(in.Text)
	if in.Text == "" {
		writeError(w, 400, "tulis sesuatu dulu")
		return
	}
	res, err := s.ask(r.Context(), in.History, in.Text, "ai_text")
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, res)
}

// extract membaca transaksi dari screenshot/teks: pakai AI bila aktif,
// selain itu (atau bila AI gagal) pakai OCR + parser offline yang gratis.
func (s *Server) extract(ctx context.Context, in ai.ExtractInput) (*ai.ExtractResult, error) {
	if !s.ai.Enabled() {
		return s.extractOffline(ctx, in)
	}
	res, err := s.ai.Extract(ctx, in)
	if err == nil {
		return res, nil
	}
	if off, oerr := s.extractOffline(ctx, in); oerr == nil && len(off.Drafts) > 0 {
		off.Notes = "⚠️ " + err.Error() + ". " + off.Notes
		return off, nil
	}
	return nil, err
}

func (s *Server) extractOffline(ctx context.Context, in ai.ExtractInput) (*ai.ExtractResult, error) {
	var b strings.Builder
	b.WriteString(in.Text)
	for _, img := range in.Images {
		text, err := ocr.Read(ctx, img.Data)
		if err != nil {
			return nil, &store.ValidationError{Msg: err.Error()}
		}
		b.WriteString("\n\n")
		b.WriteString(text)
	}
	if strings.TrimSpace(b.String()) == "" {
		if len(in.PDFs) > 0 {
			return nil, &store.ValidationError{Msg: "PDF hanya bisa dibaca dengan AI. Tanpa AI: buka PDF, salin teks mutasinya, lalu tempel di tab Tempel teks"}
		}
		return nil, &store.ValidationError{Msg: "tidak ada teks yang bisa dibaca"}
	}
	kind := in.Kind
	if kind == "" {
		kind = "ai_image"
	}
	return ai.ParseStatementOffline(ctx, s.store, b.String(), in.AccountID, in.Period, kind)
}

var imageTypes = map[string]bool{"image/jpeg": true, "image/png": true, "image/webp": true, "image/gif": true}

func readImage(fh *multipart.FileHeader) (ai.Image, error) {
	f, err := fh.Open()
	if err != nil {
		return ai.Image{}, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 15<<20))
	if err != nil {
		return ai.Image{}, err
	}
	mt := http.DetectContentType(data)
	if !imageTypes[mt] {
		return ai.Image{}, &store.ValidationError{Msg: fmt.Sprintf("format gambar %s tidak didukung (pakai JPG/PNG)", fh.Filename)}
	}
	return ai.Image{Data: data, MediaType: mt}, nil
}

func (s *Server) handleScan(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload)
	if err := r.ParseMultipartForm(maxUpload); err != nil {
		writeError(w, 400, "unggahan terlalu besar atau rusak")
		return
	}
	files := r.MultipartForm.File["images"]
	if len(files) == 0 {
		writeError(w, 400, "pilih minimal satu screenshot")
		return
	}
	if len(files) > 10 {
		writeError(w, 400, "maksimal 10 gambar sekaligus")
		return
	}
	in := ai.ExtractInput{Kind: "ai_image", Period: r.FormValue("period")}
	in.AccountID, _ = strconv.ParseInt(r.FormValue("account_id"), 10, 64)
	for _, fh := range files {
		img, err := readImage(fh)
		if err != nil {
			fail(w, err)
			return
		}
		in.Images = append(in.Images, img)
	}
	res, err := s.extract(r.Context(), in)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, res)
}

// spreadsheetText mengubah file xlsx menjadi teks CSV per sheet.
func spreadsheetText(data []byte) (string, error) {
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return "", &store.ValidationError{Msg: "file Excel tidak bisa dibaca: " + err.Error()}
	}
	defer f.Close()
	var b strings.Builder
	for _, sheet := range f.GetSheetList() {
		rows, err := f.GetRows(sheet)
		if err != nil {
			continue
		}
		var buf bytes.Buffer
		cw := csv.NewWriter(&buf)
		n := 0
		for _, row := range rows {
			empty := true
			for _, c := range row {
				if strings.TrimSpace(c) != "" {
					empty = false
					break
				}
			}
			if empty {
				continue
			}
			cw.Write(row)
			n++
		}
		cw.Flush()
		if n == 0 {
			continue
		}
		fmt.Fprintf(&b, "## Sheet: %s\n%s\n", sheet, buf.String())
	}
	return b.String(), nil
}

func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload)
	if err := r.ParseMultipartForm(maxUpload); err != nil {
		writeError(w, 400, "unggahan terlalu besar atau rusak")
		return
	}
	in := ai.ExtractInput{Kind: "import", Period: r.FormValue("period"), Text: r.FormValue("text")}
	in.AccountID, _ = strconv.ParseInt(r.FormValue("account_id"), 10, 64)
	if fhs := r.MultipartForm.File["file"]; len(fhs) > 0 {
		fh := fhs[0]
		f, err := fh.Open()
		if err != nil {
			fail(w, err)
			return
		}
		data, err := io.ReadAll(f)
		f.Close()
		if err != nil {
			fail(w, err)
			return
		}
		switch ext := strings.ToLower(filepath.Ext(fh.Filename)); {
		case ext == ".xlsx" || ext == ".xlsm":
			txt, err := spreadsheetText(data)
			if err != nil {
				fail(w, err)
				return
			}
			in.Text = strings.TrimSpace(in.Text + "\n" + txt)
		case ext == ".pdf" || http.DetectContentType(data) == "application/pdf":
			in.PDFs = append(in.PDFs, data)
		case imageTypes[http.DetectContentType(data)]:
			img, err := readImage(fh)
			if err != nil {
				fail(w, err)
				return
			}
			in.Images = append(in.Images, img)
			in.Kind = "ai_image"
		default: // csv, txt, dsb.
			in.Text = strings.TrimSpace(in.Text + "\n" + string(data))
		}
	}
	if len(in.Text) > 300_000 {
		writeError(w, 400, "data terlalu besar; pecah per bulan lalu impor bertahap")
		return
	}
	if strings.TrimSpace(in.Text) == "" && len(in.PDFs) == 0 && len(in.Images) == 0 {
		writeError(w, 400, "tempel teks mutasi atau pilih file dulu")
		return
	}
	res, err := s.extract(r.Context(), in)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, res)
}

func (s *Server) handleSuggestBudget(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload)
	ct := r.Header.Get("Content-Type")
	var text string
	if strings.HasPrefix(ct, "multipart/") {
		if err := r.ParseMultipartForm(maxUpload); err != nil {
			writeError(w, 400, "unggahan rusak")
			return
		}
		text = r.FormValue("text")
		if fhs := r.MultipartForm.File["file"]; len(fhs) > 0 {
			f, err := fhs[0].Open()
			if err != nil {
				fail(w, err)
				return
			}
			data, _ := io.ReadAll(f)
			f.Close()
			if strings.HasSuffix(strings.ToLower(fhs[0].Filename), ".xlsx") {
				txt, err := spreadsheetText(data)
				if err != nil {
					fail(w, err)
					return
				}
				text += "\n" + txt
			} else {
				text += "\n" + string(data)
			}
		}
	} else {
		var in struct {
			Text string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeError(w, 400, "format data tidak valid")
			return
		}
		text = in.Text
	}
	if strings.TrimSpace(text) == "" {
		writeError(w, 400, "tempel daftar budget dulu")
		return
	}
	res, err := s.ai.SuggestBudget(r.Context(), text)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, res)
}

func (s *Server) handleGetInsight(w http.ResponseWriter, r *http.Request) {
	p, err := s.store.ResolvePeriod(r.Context(), r.URL.Query().Get("period"))
	if err != nil {
		fail(w, err)
		return
	}
	in, err := s.store.GetInsight(r.Context(), p.Key)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"insight": in})
}

func (s *Server) handleGenerateInsight(w http.ResponseWriter, r *http.Request) {
	p, err := s.store.ResolvePeriod(r.Context(), r.URL.Query().Get("period"))
	if err != nil {
		fail(w, err)
		return
	}
	text, err := s.ai.Insight(r.Context(), p.Key)
	if err != nil {
		fail(w, err)
		return
	}
	in, err := s.store.SaveInsight(r.Context(), p.Key, text)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"insight": in})
}

func (s *Server) handleAIUsage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	p, err := s.store.CurrentPeriod(ctx)
	if err != nil {
		fail(w, err)
		return
	}
	usage, err := s.store.AIUsageSince(ctx, p.StartStr())
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"since": p.StartStr(), "usage": usage, "enabled": s.ai.Enabled(), "model": s.ai.Model()})
}

// ---------- iOS Shortcuts ----------

// shortcutReply menulis jawaban teks (atau JSON bila ?format=json).
func shortcutReply(w http.ResponseWriter, r *http.Request, status int, text string, data any) {
	if r.URL.Query().Get("format") == "json" {
		writeJSON(w, status, map[string]any{"message": text, "data": data})
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	io.WriteString(w, text)
}

// handleShortcutAsk: kirim teks dari Siri/Shortcut, transaksi langsung disimpan.
func (s *Server) handleShortcutAsk(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	body, _ := io.ReadAll(r.Body)
	text := strings.TrimSpace(string(body))
	var in struct {
		Text string `json:"text"`
	}
	if strings.HasPrefix(text, "{") && json.Unmarshal(body, &in) == nil {
		text = strings.TrimSpace(in.Text)
	}
	if q := r.URL.Query().Get("text"); text == "" && q != "" {
		text = q
	}
	if text == "" {
		shortcutReply(w, r, 400, "Teks kosong.", nil)
		return
	}
	res, err := s.ask(r.Context(), nil, text, "shortcut")
	if err != nil {
		shortcutReply(w, r, 500, "Gagal: "+err.Error(), nil)
		return
	}
	if len(res.Drafts) == 0 {
		shortcutReply(w, r, 200, res.Reply, res)
		return
	}
	saved, msg, err := s.saveDrafts(r.Context(), res.Drafts, false)
	if err != nil {
		shortcutReply(w, r, 400, "Gagal menyimpan: "+err.Error(), nil)
		return
	}
	shortcutReply(w, r, 200, msg, saved)
}

// handleShortcutScan: kirim screenshot dari Share Sheet, transaksi baru disimpan.
func (s *Server) handleShortcutScan(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload)
	in := ai.ExtractInput{Kind: "ai_image"}
	in.AccountID = queryInt(r, "account_id")
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
		if err := r.ParseMultipartForm(maxUpload); err != nil {
			shortcutReply(w, r, 400, "Unggahan rusak.", nil)
			return
		}
		if v := r.FormValue("account_id"); v != "" {
			in.AccountID, _ = strconv.ParseInt(v, 10, 64)
		}
		for _, key := range []string{"image", "images", "file"} {
			for _, fh := range r.MultipartForm.File[key] {
				img, err := readImage(fh)
				if err != nil {
					shortcutReply(w, r, 400, err.Error(), nil)
					return
				}
				in.Images = append(in.Images, img)
			}
		}
	} else {
		data, _ := io.ReadAll(r.Body)
		mt := http.DetectContentType(data)
		if !imageTypes[mt] {
			shortcutReply(w, r, 400, "Kirim gambar JPG/PNG (gunakan aksi \"Konversi Gambar\" ke JPEG di Shortcut).", nil)
			return
		}
		in.Images = append(in.Images, ai.Image{Data: data, MediaType: mt})
	}
	if len(in.Images) == 0 {
		shortcutReply(w, r, 400, "Tidak ada gambar.", nil)
		return
	}
	res, err := s.extract(r.Context(), in)
	if err != nil {
		shortcutReply(w, r, 500, "Gagal membaca screenshot: "+err.Error(), nil)
		return
	}
	saved, msg, err := s.saveDrafts(r.Context(), res.Drafts, true)
	if err != nil {
		shortcutReply(w, r, 400, "Gagal menyimpan: "+err.Error(), nil)
		return
	}
	shortcutReply(w, r, 200, msg, saved)
}

// saveDrafts menyimpan draf (melewati duplikat & nominal kosong) dan
// menyusun ringkasan teks.
func (s *Server) saveDrafts(ctx context.Context, drafts []ai.Draft, skipDup bool) ([]store.Transaction, string, error) {
	var list []store.Transaction
	var estimated []bool
	skipped := 0
	for _, d := range drafts {
		if d.Amount <= 0 || (skipDup && (d.DuplicateOf != 0 || d.DuplicateInScan)) {
			skipped++
			continue
		}
		list = append(list, d.Transaction())
		estimated = append(estimated, d.AmountEstimated)
	}
	if len(list) == 0 {
		return nil, fmt.Sprintf("Tidak ada transaksi baru yang disimpan (%d dilewati: duplikat atau nominal kosong).", skipped), nil
	}
	saved, err := s.store.CreateTransactions(ctx, list)
	if err != nil {
		return nil, "", err
	}
	accs, _ := s.store.ListAccounts(ctx)
	accName := map[int64]string{}
	for _, a := range accs {
		accName[a.ID] = a.Name
	}
	var b strings.Builder
	fmt.Fprintf(&b, "✅ %d transaksi tersimpan:\n", len(saved))
	for i, t := range saved {
		if i >= 15 {
			fmt.Fprintf(&b, "…dan %d lainnya\n", len(saved)-15)
			break
		}
		acc := accName[t.AccountID]
		if t.ToAccountID != nil {
			acc += " → " + accName[*t.ToAccountID]
		}
		est := ""
		if estimated[i] {
			est = " (perkiraan)"
		}
		fmt.Fprintf(&b, "• %s %s%s · %s\n", t.Description, money.Format(t.Amount), est, acc)
	}
	if skipped > 0 {
		fmt.Fprintf(&b, "%d dilewati (duplikat/nominal kosong).\n", skipped)
	}
	if p, err := s.store.CurrentPeriod(ctx); err == nil {
		if d, err := s.store.Dashboard(ctx, p); err == nil && d.DailyAllowance > 0 {
			fmt.Fprintf(&b, "Jatah aman: %s/hari (%d hari lagi).", money.Format(d.DailyAllowance), d.DaysLeft)
		}
	}
	return saved, strings.TrimSpace(b.String()), nil
}

// ---------- ekspor & backup ----------

func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	p, err := s.store.ResolvePeriod(ctx, r.URL.Query().Get("period"))
	if err != nil {
		fail(w, err)
		return
	}
	f := store.TxFilter{Limit: 1000}
	if r.URL.Query().Get("all") != "1" {
		f.From, f.To = p.StartStr(), p.EndStr()
	}
	var txs []store.Transaction
	for {
		page, err := s.store.ListTransactions(ctx, f)
		if err != nil {
			fail(w, err)
			return
		}
		txs = append(txs, page...)
		if len(page) < f.Limit {
			break
		}
		f.Offset += f.Limit
	}
	accs, _ := s.store.ListAccounts(ctx)
	cats, _ := s.store.ListCategories(ctx)
	accName, catName := map[int64]string{}, map[int64]string{}
	for _, a := range accs {
		accName[a.ID] = a.Name
	}
	for _, c := range cats {
		catName[c.ID] = c.Name
	}
	typeName := map[string]string{"expense": "Pengeluaran", "income": "Pemasukan", "transfer": "Transfer", "adjust": "Penyesuaian"}
	header := []string{"Tanggal", "Jenis", "Nominal", "Deskripsi", "Kategori", "Akun", "Akun Tujuan", "Catatan", "Sumber"}
	row := func(t store.Transaction) []any {
		cat, to := "", ""
		if t.CategoryID != nil {
			cat = catName[*t.CategoryID]
		}
		if t.ToAccountID != nil {
			to = accName[*t.ToAccountID]
		}
		return []any{t.Date, typeName[t.Type], t.Amount, t.Description, cat, accName[t.AccountID], to, t.Note, t.Source}
	}
	name := "mymoney-" + p.Key
	if r.URL.Query().Get("all") == "1" {
		name = "mymoney-semua-" + time.Now().Format("20060102")
	}

	if r.URL.Query().Get("format") == "csv" {
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="`+name+`.csv"`)
		w.Write([]byte("\xEF\xBB\xBF")) // BOM agar Excel membaca UTF-8
		cw := csv.NewWriter(w)
		cw.Write(header)
		for _, t := range txs {
			rec := []string{}
			for _, v := range row(t) {
				rec = append(rec, fmt.Sprint(v))
			}
			cw.Write(rec)
		}
		cw.Flush()
		return
	}

	x := excelize.NewFile()
	defer x.Close()
	sheet := "Transaksi"
	x.SetSheetName("Sheet1", sheet)
	bold, _ := x.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}, Fill: excelize.Fill{Type: "pattern", Color: []string{"#E8EEF7"}, Pattern: 1}})
	rp, _ := x.NewStyle(&excelize.Style{CustomNumFmt: strPtr(`"Rp"#,##0;-"Rp"#,##0`)})
	for i, h := range header {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		x.SetCellValue(sheet, cell, h)
	}
	x.SetRowStyle(sheet, 1, 1, bold)
	for i, t := range txs {
		vals := row(t)
		x.SetSheetRow(sheet, fmt.Sprintf("A%d", i+2), &vals)
	}
	x.SetColStyle(sheet, "C", rp)
	x.SetColWidth(sheet, "A", "A", 12)
	x.SetColWidth(sheet, "D", "D", 32)
	x.SetColWidth(sheet, "E", "G", 18)
	x.SetPanes(sheet, &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"})

	if budgets, err := s.store.ListBudgets(ctx, p); err == nil && len(budgets) > 0 {
		bs := "Budget " + p.Key
		x.NewSheet(bs)
		x.SetSheetRow(bs, "A1", &[]any{"Kategori", "Jenis", "Rencana", "Terpakai", "Sisa", "Jatuh Tempo", "Catatan"})
		x.SetRowStyle(bs, 1, 1, bold)
		for i, b := range budgets {
			due := ""
			if b.DueDay > 0 {
				due = b.DueDate
			}
			x.SetSheetRow(bs, fmt.Sprintf("A%d", i+2), &[]any{catName[b.CategoryID], b.Kind, b.Amount, b.Actual, b.Amount - b.Actual, due, b.Note})
		}
		x.SetColStyle(bs, "C:E", rp)
		x.SetColWidth(bs, "A", "A", 24)
		x.SetColWidth(bs, "C", "F", 14)
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`.xlsx"`)
	x.Write(w)
}

func strPtr(s string) *string { return &s }
