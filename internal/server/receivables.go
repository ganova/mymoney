package server

import (
	"net/http"

	"github.com/ganova/mymoney/internal/ai"
	"github.com/ganova/mymoney/internal/ocr"

	"github.com/ganova/mymoney/internal/store"
)

func (s *Server) handleListReceivables(w http.ResponseWriter, r *http.Request) {
	l, err := s.store.ListReceivables(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, l)
}

func (s *Server) handleCreateReceivable(w http.ResponseWriter, r *http.Request) {
	var in store.Receivable
	if err := decode(r, &in); err != nil {
		fail(w, err)
		return
	}
	x, err := s.store.CreateReceivable(r.Context(), in)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 201, x)
}

func (s *Server) handleUpdateReceivable(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		fail(w, err)
		return
	}
	var in store.Receivable
	if err := decode(r, &in); err != nil {
		fail(w, err)
		return
	}
	in.ID = id
	x, err := s.store.UpdateReceivable(r.Context(), in)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, x)
}

func (s *Server) handleDeleteReceivable(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		fail(w, err)
		return
	}
	if err := s.store.DeleteReceivable(r.Context(), id); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// handleReceiveReceivable mencairkan: membuat transaksi pemasukan lalu menandai diterima.
func (s *Server) handleReceiveReceivable(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		fail(w, err)
		return
	}
	var in struct {
		AccountID int64  `json:"account_id"`
		Amount    int64  `json:"amount"`
		Date      string `json:"date"`
	}
	if err := decode(r, &in); err != nil {
		fail(w, err)
		return
	}
	rec, tx, err := s.store.ReceiveReceivable(r.Context(), id, in.AccountID, in.Amount, in.Date)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"receivable": rec, "transaction": tx})
}

func (s *Server) handleOutlook(w http.ResponseWriter, r *http.Request) {
	o, err := s.store.Outlook(r.Context(), int(queryInt(r, "months")))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, o)
}

// handleScanReceivables membaca screenshot daftar pesanan platform freelance
// (mis. Fastwork) menjadi usulan pendapatan tertunda. AI bila aktif; bila
// tidak/gagal, OCR + pembaca teks offline. Belum menyimpan apa pun.
func (s *Server) handleScanReceivables(w http.ResponseWriter, r *http.Request) {
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
	if len(files) > 6 {
		writeError(w, 400, "maksimal 6 gambar sekaligus")
		return
	}
	var imgs []ai.Image
	for _, fh := range files {
		img, err := readImage(fh)
		if err != nil {
			fail(w, err)
			return
		}
		imgs = append(imgs, img)
	}
	ctx := r.Context()
	var res *ai.OrderResult
	var aiErr error
	if s.ai.Enabled() {
		res, aiErr = s.ai.ExtractOrders(ctx, imgs)
	}
	if res == nil || len(res.Orders) == 0 {
		var all []ai.Order
		var ocrErr error
		for _, img := range imgs {
			variants, err := ocr.ReadTable(ctx, img.Data)
			if err != nil {
				ocrErr = err
				break
			}
			// tiap mode baca memberi tafsiran berbeda; ambil yang paling banyak pesanannya
			var best []ai.Order
			for _, t := range variants {
				if o := ai.ParseOrdersOffline(t, s.store.Today()); len(o) > len(best) {
					best = o
				}
			}
			all = append(all, best...)
		}
		if ocrErr == nil {
			if orders := all; len(orders) > 0 {
				note := "Dibaca dengan OCR tanpa AI: cek nama, klien, dan tanggal sebelum menyimpan."
				if aiErr != nil {
					note = "⚠️ " + aiErr.Error() + ". " + note
				}
				res = &ai.OrderResult{Orders: orders, Notes: note, Mode: "offline"}
			}
		}
		if res == nil {
			switch {
			case aiErr != nil:
				fail(w, aiErr)
			case ocrErr != nil:
				fail(w, &store.ValidationError{Msg: ocrErr.Error()})
			default:
				writeError(w, 422, "tidak ada pesanan yang terbaca. Coba screenshot yang lebih jelas dan memuat kolom harga.")
			}
			return
		}
	}
	for i := range res.Orders {
		o := &res.Orders[i]
		o.Exists = s.store.HasReceivable(ctx, o.Ref, o.Name, o.Amount)
	}
	writeJSON(w, 200, res)
}

// handleBulkReceivables menyimpan beberapa pendapatan tertunda sekaligus (hasil scan yang dikonfirmasi).
func (s *Server) handleBulkReceivables(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Items []store.Receivable `json:"items"`
	}
	if err := decode(r, &in); err != nil {
		fail(w, err)
		return
	}
	if len(in.Items) == 0 {
		writeError(w, 400, "tidak ada data untuk disimpan")
		return
	}
	out, err := s.store.CreateReceivables(r.Context(), in.Items)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 201, out)
}
