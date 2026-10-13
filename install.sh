#!/usr/bin/env bash
# Instal & jalankan MyMoney dengan Docker (Mac/Linux).
set -e
command -v docker >/dev/null || { echo "Docker belum terpasang. Pasang Docker Desktop: https://www.docker.com/products/docker-desktop/"; exit 1; }
docker info >/dev/null 2>&1 || { echo "Docker belum berjalan. Buka aplikasi Docker Desktop dulu, lalu jalankan ulang skrip ini."; exit 1; }
if [ ! -f .env ]; then
  cp .env.example .env
  read -rp "Buat kata sandi login MyMoney: " PW
  read -rp "ANTHROPIC_API_KEY (Enter untuk lewati): " KEY
  sed -i.bak "s|^APP_PASSWORD=.*|APP_PASSWORD=$PW|; s|^ANTHROPIC_API_KEY=.*|ANTHROPIC_API_KEY=$KEY|" .env && rm -f .env.bak
fi
docker compose up -d --build
IP=$( (ipconfig getifaddr en0 || hostname -I | awk '{print $1}') 2>/dev/null )
echo ""
echo "✅ MyMoney berjalan!"
echo "   Komputer : http://localhost:7428"
[ -n "$IP" ] && echo "   iPhone   : http://$IP:8080  (Wi-Fi yang sama, lalu Bagikan → Tambahkan ke Layar Utama)"
