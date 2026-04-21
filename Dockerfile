################################################################################
# Stage 1 — deps: cache go.mod/go.sum separately so unrelated code changes
# don't bust the dependency layer.
################################################################################
FROM golang:1.26-alpine AS deps

WORKDIR /app
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN go mod download


################################################################################
# Stage 2 — tools: sqlc + swag for code generation. Cached independently so we
# don't re-download CLIs on every rebuild.
################################################################################
FROM golang:1.26-alpine AS tools

RUN apk add --no-cache curl && \
    curl -fsSL https://downloads.sqlc.dev/sqlc_1.31.0_linux_amd64.tar.gz \
      | tar -xz -C /usr/local/bin sqlc && \
    GOBIN=/usr/local/bin go install github.com/swaggo/swag/cmd/swag@v1.16.6


################################################################################
# Stage 3 — build: run code-gen, static-link, strip symbols, keep binary tiny.
################################################################################
FROM golang:1.26-alpine AS builder

WORKDIR /app
COPY --from=deps /go/pkg /go/pkg
COPY --from=tools /usr/local/bin/sqlc /usr/local/bin/sqlc
COPY --from=tools /usr/local/bin/swag /usr/local/bin/swag

COPY . .

RUN sqlc generate && \
    swag init -g cmd/server/main.go -o docs && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
      -trimpath \
      -ldflags="-s -w -X main.version=docker -X main.buildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
      -o /out/server ./cmd/server


################################################################################
# Stage 4 — runtime: distroless-style minimal image with a non-root user.
################################################################################
FROM alpine:3.20 AS runtime

RUN apk add --no-cache ca-certificates tzdata && \
    addgroup -S app && adduser -S app -G app -h /app

WORKDIR /app
COPY --from=builder /out/server /app/server
COPY --from=builder /app/docs /app/docs
COPY --from=builder /app/public /app/public
COPY --from=builder /app/migrations /app/migrations

USER app

EXPOSE 3000

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget -qO- http://localhost:3000/health || exit 1

ENTRYPOINT ["/app/server"]
