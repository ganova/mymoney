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
npm run build
if ($LASTEXITCODE -ne 0) { Pop-Location; Write-Host "Build web gagal"; exit 1 }
Pop-Location

Write-Host "==> Membangun program"
$env:CGO_ENABLED = "0"
go build -ldflags "-H=windowsgui" -o (Join-Path $root "mymoney.exe") ./cmd/mymoney
if ($LASTEXITCODE -ne 0) { Write-Host "Build Go gagal"; exit 1 }

Write-Host "==> Memasang"
& (Join-Path $root "install-windows.ps1")
