.PHONY: help install sqlc swagger migrate-up migrate-down docker-up docker-down docker-build docker-logs dev run build clean

help:
	@echo "Available commands:"
	@echo "  make install       - Install dependencies"
	@echo "  make sqlc          - Generate SQLC code"
	@echo "  make swagger       - Generate Swagger documentation"
	@echo "  make migrate-up    - Run database migrations"
	@echo "  make migrate-down  - Rollback database migrations"
	@echo "  make docker-up     - Start all Docker services"
	@echo "  make docker-down   - Stop all Docker services"
	@echo "  make docker-build  - Build Docker images"
	@echo "  make docker-logs   - View Docker logs"
	@echo "  make dev           - Run with hot-reload (Air)"
	@echo "  make run           - Run the server locally"
	@echo "  make build         - Build the server binary"
	@echo "  make clean         - Clean build artifacts"

SQLC_VERSION := 1.31.0
SQLC_OS := $(shell uname -s | tr '[:upper:]' '[:lower:]')
SQLC_ARCH := $(shell uname -m | sed -e 's/x86_64/amd64/' -e 's/aarch64/arm64/')

install:
	go mod download
	go mod tidy
	curl -fsSL https://downloads.sqlc.dev/sqlc_$(SQLC_VERSION)_$(SQLC_OS)_$(SQLC_ARCH).tar.gz | tar -xz -C $(shell go env GOPATH)/bin sqlc
	go install github.com/swaggo/swag/cmd/swag@latest
	go install github.com/air-verse/air@latest

sqlc:
	sqlc generate

swagger:
	swag init -g cmd/server/main.go -o docs

migrate-up:
	migrate -path migrations -database "postgresql://postgres:postgres@localhost:5432/live_platform?sslmode=disable" up

migrate-down:
	migrate -path migrations -database "postgresql://postgres:postgres@localhost:5432/live_platform?sslmode=disable" down

docker-up:
	docker-compose up -d

docker-down:
	docker-compose down

docker-build:
	docker-compose build

docker-logs:
	docker-compose logs -f app

dev:
	air

run:
	go run cmd/server/main.go

build:
	go build -o bin/server cmd/server/main.go

clean:
	rm -rf bin/
	rm -rf tmp/
	rm -rf internal/database/db/
	rm -rf docs/
