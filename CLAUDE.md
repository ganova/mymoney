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
- Dua buku (`ledgers`: Utama=1, Freelance=2) dipilih per permintaan lewat header `X-Ledger` → `store.WithLedger(ctx)`; akun, kategori, utang, budget, transaksi dan laporan tersaring per buku. Pemasukan punya `income_source_id` (CMW/Freelance).
- `receivables` = pendapatan tertunda per buku (status pending/received/cancelled); `Store.Outlook` memproyeksikan pendapatan tertunda dan jadwal cicilan utang per periode.
- Jatah mingguan (`internal/store/weekly.go`, `Dashboard.Weekly`): pool = pendapatan − budget wajib, 4 minggu (8+8+8+sisa hari), terpakai = pengeluaran non-wajib; pengaturan per buku (`weekly_allowance_<ledger>`, bawaan aktif hanya di Utama). `DailyAllowance` mengikuti sisa minggu berjalan saat aktif.
- Jatah mingguan membawa sisa/kekurangan minggu yang sudah selesai ke minggu berikutnya (`WeekPlan.Base` + `Carry` = `Allowance`).
- Backup (`internal/store/backup.go`, `server/backup_loop.go`): `VACUUM INTO` ke `BACKUP_DIR` (bawaan `data/backups`), dijadwalkan tiap `BACKUP_EVERY_HOURS`, disimpan `BACKUP_KEEP` versi.
- Folder backup: setting DB `backup_dir` → `BACKUP_DIR` → `data/backups` (`Server.backupFolder`, jatuh ke bawaan bila tak bisa diakses). Laporan bulanan xlsx periode sebelumnya per buku dibuat oleh `ensureMonthlyReports` (`server/report_xlsx.go`) dari loop yang sama.
- Budget per (periode, kategori); `due_day > 0` = tagihan. `EnsureBudget` menyalin budget terakhir ke periode berjalan bila kosong.
- Aturan (`rules`) dipelajari otomatis saat transaksi disimpan/dikoreksi dan dimasukkan ke prompt AI serta parser offline.
- Draf AI (`ai.Draft`) selalu melewati `Context.Sanitize` agar ID akun/kategori valid; pengguna mengonfirmasi sebelum disimpan (kecuali Shortcut/autosave).
- Tanpa API key: `ai.ParseOffline` (kalimat) dan `ocr` (Tesseract + poppler untuk PDF) + `ai.ParseStatementOffline` (mutasi). `Server.extract` memakai AI bila aktif, offline bila tidak/gagal.
- Semua teks UI dan pesan error berbahasa Indonesia.
- Desktop (≥1024px, `ui.desktop`): `Sidebar`, panel Asisten (`Assistant docked`, `ui.dock`), `CommandPalette` (Ctrl+K), pintasan di `App.svelte`. Tata letak halaman lebar memakai `@container page (...)` pada `.page.wide`; kelas `desk-only`/`mob-only` menyembunyikan elemen per mode. Jangan mengubah tampilan ponsel saat menambah fitur desktop.
