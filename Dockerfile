# syntax=docker/dockerfile:1

# 1) Build frontend (Svelte → file statis)
FROM node:22-alpine AS web
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

# 2) Build backend Go; frontend disematkan ke dalam satu binary
FROM golang:1.26-alpine AS server
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY web/embed.go ./web/embed.go
COPY --from=web /web/dist ./web/dist
ARG VERSION=docker
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X github.com/ganova/mymoney/internal/server.Version=${VERSION}" \
    -o /out/mymoney ./cmd/mymoney

# 3) Image akhir dengan Tesseract OCR (Indonesia + Inggris) agar screenshot
#    mutasi bisa dibaca gratis tanpa AI.
FROM ubuntu:24.04
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates tesseract-ocr tesseract-ocr-ind tesseract-ocr-eng \
    && rm -rf /var/lib/apt/lists/* \
    && useradd -u 10001 -M -s /usr/sbin/nologin app && mkdir -p /data && chown app:app /data
COPY --from=server /out/mymoney /usr/local/bin/mymoney
USER app
ENV DATA_DIR=/data \
    PORT=8080 \
    TZ=Asia/Jakarta \
    OMP_THREAD_LIMIT=1
VOLUME /data
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s CMD ["mymoney", "healthcheck"]
ENTRYPOINT ["mymoney"]
