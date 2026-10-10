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

# 3) Image akhir yang kecil (~25 MB). Data zona waktu sudah tertanam di
#    binary dan sertifikat CA disalin dari builder, jadi tidak perlu apk.
FROM alpine:3.22
RUN adduser -D -H -u 10001 app && mkdir -p /data && chown app:app /data
COPY --from=server /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=server /out/mymoney /usr/local/bin/mymoney
USER app
ENV DATA_DIR=/data \
    PORT=8080 \
    TZ=Asia/Jakarta
VOLUME /data
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s \
    CMD wget -qO- "http://127.0.0.1:${PORT}/api/health" >/dev/null || exit 1
ENTRYPOINT ["mymoney"]
