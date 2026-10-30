//go:build !windows

package ai

import (
	"testing"
	"time"
)

func TestParseOrdersOffline(t *testing.T) {
	text := `IDPesanan — Tanggal jatuh tempo Partner kerja Status Diperbarui pada Harga
Pembuatan Website
Ecommerce, ERP, JIS7X6N8 © u Ujang Sedang dikerjakan 3 jam yang lalu Rp4.750.000,00
Web Digital space
komputer WTVKOH4E (© —_Jatuh tempo 31 Okt (sisa 1 hari) @ Digital space Komputer Sedang dikerjakan 19 jam yang lalu Rp1.000.000,00
‘© Tahap 1/2
Pembuatan Website
Ecommerce,ERP.Comp... G7VNKMRE (© 1 Levandi D.N Menunggu persetujuan 2 hari yang lalu Rp8.700.000,00
‘© Tahap 3/4
Pembuatan Website
Ecommerce, ERP, 8CFWEUBI Jatuh tempo 11 Nov Ujang Sedang dikerjakan 12/10/2026 10:41 Rp3.060.000,00
Tahap 2/4
Layanan lama 11ABCDEF Selesai Rp500.000,00`
	got := ParseOrdersOffline(text, time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC))
	if len(got) != 4 {
		t.Fatalf("harus 4 pesanan, dapat %d: %+v", len(got), got)
	}
	want := []struct {
		ref    string
		amount int64
		due    string
		stage  string
		status string
	}{
		{"JIS7X6N8", 4750000, "", "", "Sedang dikerjakan"},
		{"WTVKOH4E", 1000000, "2026-10-31", "Tahap 1/2", "Sedang dikerjakan"},
		{"G7VNKMRE", 8700000, "", "Tahap 3/4", "Menunggu persetujuan"},
		{"8CFWEUBI", 3060000, "2026-11-11", "Tahap 2/4", "Sedang dikerjakan"},
	}
	for i, w := range want {
		g := got[i]
		if g.Ref != w.ref || g.Amount != w.amount || g.DueDate != w.due || g.Stage != w.stage || g.Status != w.status {
			t.Errorf("baris %d: %+v, harapan %+v", i, g, w)
		}
	}
	if got[0].Name != "Pembuatan Website Ecommerce, ERP" {
		t.Errorf("nama baris 1: %q", got[0].Name)
	}
}

func TestOrdersCodex(t *testing.T) {
	s := New(Config{CodexPath: fakeCodex(t, `{"orders":[{"ref":"AB12CD34","name":"Web toko...","client":"Fastwork · Ujang","amount":3000000,"due_date":"2026-11-11","status":"Sedang dikerjakan","stage":"Tahap 2/4"},{"ref":"","name":"","client":"","amount":0,"due_date":"","status":"","stage":""}],"notes":""}`)}, testStore(t))
	res, err := s.ExtractOrders(t.Context(), []Image{{Data: []byte("x"), MediaType: "image/png"}})
	if err != nil || len(res.Orders) != 1 || res.Orders[0].Name != "Web toko" || res.Orders[0].DueDate != "2026-11-11" {
		t.Fatalf("hasil: %+v %v", res, err)
	}
}
