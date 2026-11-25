# Build MyMoney langsung di PC ini lalu pasang/perbarui (tanpa unduh ZIP, tanpa Docker).
# Jalankan dari folder repo:  powershell -ExecutionPolicy Bypass -File update-windows.ps1
# Prasyarat sekali saja:      winget install Git.Git GoLang.Go OpenJS.NodeJS.LTS  (lalu buka PowerShell baru)
$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $MyInvocation.MyCommand.Path
Set-Location $root

foreach ($t in "git", "go", "node", "npm") {
  if (-not (Get-Command $t -ErrorAction SilentlyContinue)) {
    Write-Host "'$t' belum terpasang. Jalankan: winget install Git.Git GoLang.Go OpenJS.NodeJS.LTS"
    Write-Host "Lalu tutup dan buka PowerShell baru, dan ulangi."
    exit 1
  }
}

Write-Host "==> Mengambil versi terbaru"
git pull --ff-only
if ($LASTEXITCODE -ne 0) { Write-Host "git pull gagal (ada perubahan lokal?)"; exit 1 }

Write-Host "==> Membangun tampilan (web)"
Push-Location web
if (-not (Test-Path node_modules)) { npm install }
# batas memori Node dinaikkan agar build tidak berhenti dengan "process out of memory"
$env:NODE_OPTIONS = "--max-old-space-size=4096"
npm run build
if ($LASTEXITCODE -ne 0) {
  Write-Host "Build web gagal. Mencoba ulang dengan batas memori lebih besar..."
  $env:NODE_OPTIONS = "--max-old-space-size=8192"
  npm run build
}
if ($LASTEXITCODE -ne 0) { Pop-Location; Write-Host "Build web gagal. Tutup aplikasi berat (browser, Docker) lalu ulangi."; exit 1 }
Pop-Location

Write-Host "==> Membangun program"
$env:CGO_ENABLED = "0"
$exe = Join-Path $root "mymoney.exe"
$log = Join-Path $root "build-error.log"
# Hemat memori: compiler Go bisa kehabisan RAM saat mengompilasi SQLite.
# Percobaan 1: 2 proses paralel. Percobaan 2: satu per satu, GC lebih sering.
$attempts = @(
  @{ procs = "2"; gogc = "50"; p = "2" },
  @{ procs = "1"; gogc = "25"; p = "1" }
)
$ok = $false
# Windows PowerShell 5.1 menganggap teks stderr program (mis. "go: downloading")
# sebagai error yang menghentikan skrip bila ErrorActionPreference = Stop.
$prevEAP = $ErrorActionPreference
$ErrorActionPreference = "Continue"
foreach ($a in $attempts) {
  $env:GOMAXPROCS = $a.procs
  $env:GOGC = $a.gogc
  & go build -p $a.p -ldflags "-H=windowsgui" -o $exe ./cmd/mymoney *> $log
  if ($LASTEXITCODE -eq 0) { $ok = $true; break }
  Write-Host "Build Go gagal, mencoba ulang dengan mode lebih hemat memori..."
}
$ErrorActionPreference = $prevEAP
Remove-Item Env:GOMAXPROCS, Env:GOGC -ErrorAction SilentlyContinue
if (-not $ok) {
  Write-Host ""
  Write-Host "Build Go gagal. Awal pesan error (lengkapnya di $log):" -ForegroundColor Red
  Get-Content $log -TotalCount 15
  $free = [math]::Round((Get-CimInstance Win32_OperatingSystem).FreePhysicalMemory / 1MB, 1)
  Write-Host ""
  Write-Host "RAM kosong saat ini: $free GB. Tutup browser/aplikasi berat lalu ulangi."
  exit 1
}
Remove-Item $log -ErrorAction SilentlyContinue

Write-Host "==> Memasang"
& (Join-Path $root "install-windows.ps1")
