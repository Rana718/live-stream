FROM golang:1.26-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git curl

COPY go.mod go.sum ./
RUN go mod download

RUN curl -fsSL https://downloads.sqlc.dev/sqlc_1.31.0_linux_amd64.tar.gz \
    | tar -xz -C /usr/local/bin sqlc && \
    go install github.com/swaggo/swag/cmd/swag@latest

COPY . .

RUN sqlc generate && \
    swag init -g cmd/server/main.go -o docs && \
    go mod tidy

RUN CGO_ENABLED=0 GOOS=linux go build -o /app/server cmd/server/main.go

FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/

COPY --from=builder /app/server .
COPY --from=builder /app/.env.example .env

EXPOSE 3000

CMD ["./server"]
