.PHONY: help install sqlc migrate-up migrate-down docker-up docker-down docker-build docker-logs run build clean

help:
	@echo "Available commands:"
	@echo "  make install       - Install dependencies"
	@echo "  make sqlc          - Generate SQLC code"
	@echo "  make migrate-up    - Run database migrations"
	@echo "  make migrate-down  - Rollback database migrations"
	@echo "  make docker-up     - Start all Docker services"
	@echo "  make docker-down   - Stop all Docker services"
	@echo "  make docker-build  - Build Docker images"
	@echo "  make docker-logs   - View Docker logs"
	@echo "  make run           - Run the server locally"
	@echo "  make build         - Build the server binary"
	@echo "  make clean         - Clean build artifacts"

install:
	go mod download
	go mod tidy

sqlc:
	sqlc generate

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

run:
	go run cmd/server/main.go

build:
	go build -o bin/server cmd/server/main.go

clean:
	rm -rf bin/
	rm -rf internal/database/db/
