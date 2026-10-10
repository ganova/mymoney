.PHONY: dev web build test check docker up down logs

# Jalankan backend (port 8080) — buka terminal kedua untuk `make web`
dev:
	DATA_DIR=./data go run ./cmd/mymoney

# Frontend dengan hot reload di http://localhost:5173 (proxy ke :8080)
web:
	cd web && npm install && npm run dev

build:
	cd web && npm ci && npm run build
	CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o bin/mymoney ./cmd/mymoney

test:
	go test ./...

check:
	go vet ./...
	cd web && npm run check

up:
	docker compose up -d --build

down:
	docker compose down

logs:
	docker compose logs -f mymoney
