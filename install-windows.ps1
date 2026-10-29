# Pasang MyMoney langsung di Windows (tanpa Docker).
# Jalankan dari folder yang berisi mymoney.exe:
#   powershell -ExecutionPolicy Bypass -File install-windows.ps1
$ErrorActionPreference = "Stop"
$src = Split-Path -Parent $MyInvocation.MyCommand.Path
$dst = Join-Path $env:LOCALAPPDATA "MyMoney"
$exe = Join-Path $dst "mymoney.exe"

if (-not (Test-Path (Join-Path $src "mymoney.exe"))) { Write-Host "mymoney.exe tidak ditemukan di $src"; exit 1 }

# hentikan versi lama bila sedang berjalan
Get-Process mymoney -ErrorAction SilentlyContinue | Stop-Process -Force
Start-Sleep -Milliseconds 500

New-Item -ItemType Directory -Force -Path $dst | Out-Null
Copy-Item (Join-Path $src "mymoney.exe") $exe -Force
$envFile = Join-Path $dst ".env"
if (-not (Test-Path $envFile)) {
  @"
# Konfigurasi MyMoney. Setelah mengubah, jalankan ulang install-windows.ps1
PORT=7428
TZ=Asia/Jakarta
ANTHROPIC_API_KEY=
AI_MODEL=claude-haiku-5-5
"@ | Set-Content -Encoding ASCII $envFile
}

# jalan otomatis setiap login (tanpa jendela, tanpa hak admin)
$startup = [Environment]::GetFolderPath("Startup")
$lnk = (New-Object -ComObject WScript.Shell).CreateShortcut((Join-Path $startup "MyMoney.lnk"))
$lnk.TargetPath = $exe
$lnk.WorkingDirectory = $dst
$lnk.Save()

# buka port untuk iPhone (butuh admin; dilewati bila tidak)
try {
  if (-not (Get-NetFirewallRule -DisplayName "MyMoney" -ErrorAction SilentlyContinue)) {
    New-NetFirewallRule -DisplayName "MyMoney" -Direction Inbound -Protocol TCP -LocalPort 7428 -Action Allow | Out-Null
  }
} catch { Write-Host "Catatan: jalankan PowerShell sebagai Administrator sekali agar iPhone bisa mengakses (firewall)." }

Start-Process -FilePath $exe -WorkingDirectory $dst -WindowStyle Hidden
Start-Sleep -Seconds 2

$ip = (Get-NetIPAddress -AddressFamily IPv4 | Where-Object { $_.IPAddress -like '192.168.*' -or $_.IPAddress -like '10.*' } | Select-Object -First 1).IPAddress
Write-Host ""
Write-Host "MyMoney terpasang di $dst dan berjalan di latar belakang."
Write-Host "  Komputer : http://localhost:7428"
if ($ip) { Write-Host "  iPhone   : http://${ip}:7428" }
Write-Host "Otomatis menyala setiap Windows login. Data: $dst\data"
Start-Process "http://localhost:7428"
