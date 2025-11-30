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

install:
	go mod download
	go mod tidy
	go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
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
