.PHONY: build test lint migrate migrate-down migrate-create

build:
	go build -o bin/server ./cmd/server

test:
	go test ./...

lint:
	go vet ./...

# Run all pending up-migrations against the database specified by DATABASE_URL.
#
# Requires golang-migrate CLI: https://github.com/golang-migrate/migrate
# Install: go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
#
# Usage:
#   DATABASE_URL=postgres://user:pass@localhost:5432/dbname?sslmode=disable make migrate
#
# The migrate tool reads migration files from db/migrations/ and applies them
# in order. Migration state is tracked in the schema_migrations table.
migrate:
	@if command -v migrate > /dev/null 2>&1; then \
		migrate -path db/migrations -database "$${DATABASE_URL}" up; \
	else \
		echo "golang-migrate not found. Install with:"; \
		echo "  go install -tags postgres github.com/golang-migrate/migrate/v4/cmd/migrate@latest"; \
		echo ""; \
		echo "Alternatively, apply manually:"; \
		echo "  psql $${DATABASE_URL} -f db/migrations/001_init.up.sql"; \
		exit 1; \
	fi

# Roll back the most recent migration.
migrate-down:
	@if command -v migrate > /dev/null 2>&1; then \
		migrate -path db/migrations -database "$${DATABASE_URL}" down 1; \
	else \
		echo "golang-migrate not found. Apply manually:"; \
		echo "  psql $${DATABASE_URL} -f db/migrations/001_init.down.sql"; \
		exit 1; \
	fi

# Convenience target to create a new timestamped migration pair.
# Usage: make migrate-create NAME=add_indexes
migrate-create:
	migrate create -ext sql -dir db/migrations -seq $${NAME}
