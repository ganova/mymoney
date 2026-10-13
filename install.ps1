# Instal & jalankan MyMoney dengan Docker (Windows PowerShell).
$ErrorActionPreference = "Stop"
if (-not (Get-Command docker -ErrorAction SilentlyContinue)) { Write-Host "Docker belum terpasang. Pasang Docker Desktop: https://www.docker.com/products/docker-desktop/"; exit 1 }
docker info *> $null; if ($LASTEXITCODE -ne 0) { Write-Host "Buka aplikasi Docker Desktop dulu, lalu jalankan ulang skrip ini."; exit 1 }
if (-not (Test-Path .env)) {
  $key = Read-Host "ANTHROPIC_API_KEY (Enter untuk lewati)"
  (Get-Content .env.example) -replace '^ANTHROPIC_API_KEY=.*', "ANTHROPIC_API_KEY=$key" | Set-Content .env
}
docker compose up -d --build
$ip = (Get-NetIPAddress -AddressFamily IPv4 | Where-Object { $_.IPAddress -like '192.168.*' -or $_.IPAddress -like '10.*' } | Select-Object -First 1).IPAddress
Write-Host "`nMyMoney berjalan!`n  Komputer : http://localhost:7428"
if ($ip) { Write-Host "  iPhone   : http://${ip}:8080  (Wi-Fi yang sama)" }
