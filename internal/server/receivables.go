package server

import (
	"net/http"

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
