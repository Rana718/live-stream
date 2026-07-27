.PHONY: help install sqlc swagger migrate-up migrate-down docker-up docker-down docker-build docker-logs dev run build clean backup restore

help:
	@echo "Available commands:"
	@echo "  make install       - Install dependencies"
	@echo "  make sqlc          - Generate SQLC code"
	@echo "  make swagger       - Generate Swagger documentation"
	@echo "  make migrate-up    - Apply database migrations (idempotent)"
	@echo "  make migrate-down  - Not supported (see comment on the target); restore from backup instead"
	@echo "  make docker-up     - Start all Docker services"
	@echo "  make docker-down   - Stop all Docker services"
	@echo "  make docker-build  - Build Docker images"
	@echo "  make docker-logs   - View Docker logs"
	@echo "  make dev           - Run with hot-reload (Air)"
	@echo "  make run           - Run the server locally"
	@echo "  make build         - Build the server binary"
	@echo "  make clean         - Clean build artifacts"
	@echo "  make backup        - Dump the DB and upload it to MinIO"
	@echo "  make restore       - Restore latest backup into a scratch DB (see docs/runbooks/backup-restore.md)"

SQLC_VERSION := 1.31.0
SQLC_OS := $(shell uname -s | tr '[:upper:]' '[:lower:]')
SQLC_ARCH := $(shell uname -m | sed -e 's/x86_64/amd64/' -e 's/aarch64/arm64/')

install:
	go mod download
	go mod tidy
	curl -fsSL https://downloads.sqlc.dev/sqlc_$(SQLC_VERSION)_$(SQLC_OS)_$(SQLC_ARCH).tar.gz | tar -xz -C $(shell go env GOPATH)/bin sqlc
	go install github.com/swaggo/swag/cmd/swag@latest
	go install github.com/air-verse/air@latest

bootstrap: docker-up
	@echo "Waiting for services to be ready..."
	@sleep 10
	$(MAKE) migrate-up
	$(MAKE) sqlc
	$(MAKE) swagger

sqlc:
	sqlc generate

swagger:
	swag init -g cmd/server/main.go -o docs

migrate-up:
	./scripts/migrate.sh

# No golang-migrate/sql-migrate "down" support: these are single
# NNN_name.sql files (not .up/.down pairs), and only one of the 43 has a
# real (fully-commented-out) Down section — there was never a working
# rollback path here. To undo a bad migration, restore from a pre-migration
# backup instead: see docs/runbooks/backup-restore.md.
migrate-down:
	@echo "Not supported — see docs/runbooks/backup-restore.md to restore from a backup instead."
	@exit 1

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

backup:
	./scripts/backup-db.sh

restore:
	./scripts/restore-db.sh latest
