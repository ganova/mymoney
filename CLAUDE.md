# MyMoney — catatan untuk pengembangan

Aplikasi keuangan pribadi (bahasa Indonesia) untuk satu pengguna, dipakai dari Safari iPhone, dijalankan via Docker.

## Stack
- Backend: Go 1.26, `net/http` (pola rute Go 1.22+), SQLite via `modernc.org/sqlite` (tanpa CGO), `anthropic-sdk-go`, `excelize`.
- Frontend: Svelte 5 (runes) + Vite + TypeScript di `web/`, hasil build disematkan ke binary lewat `web/embed.go`.
- AI: Claude (`AI_MODEL`, default `claude-opus-5-5`) lewat `client.Beta.Messages.NewStreaming` dengan `fallbacks: "default"`.

## Perintah
- `go test ./...` — semua tes Go (store, period, money, ai dengan server Claude tiruan, server HTTP).
- `cd web && npm run check && npm run build` — type-check & build frontend (wajib sebelum `go build` agar `web/dist` terisi).
- `make dev` + `make web` — pengembangan lokal (backend :8080, frontend :5173).
- `docker compose up -d --build` — produksi.

## Konsep penting
- Nominal selalu bilangan bulat Rupiah (`int64`).
- Periode keuangan (`internal/period`): mulai `period_start_day` (default 28); label = bulan akhir periode. Periode "2026-10" = 28 Sep – 27 Okt.
- Jenis transaksi: `expense`, `income`, `transfer` (antar akun sendiri, tidak masuk laporan), `adjust` (penyesuaian saldo, nominal bertanda).
- Budget per (periode, kategori); `due_day > 0` = tagihan. `EnsureBudget` menyalin budget terakhir ke periode berjalan bila kosong.
- Aturan (`rules`) dipelajari otomatis saat transaksi disimpan/dikoreksi dan dimasukkan ke prompt AI serta parser offline.
- Draf AI (`ai.Draft`) selalu melewati `Context.Sanitize` agar ID akun/kategori valid; pengguna mengonfirmasi sebelum disimpan (kecuali Shortcut/autosave).
- Semua teks UI dan pesan error berbahasa Indonesia.
