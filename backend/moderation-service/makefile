DATABASE_URL ?= postgres://postgres:postgres@localhost:5432/social_media?sslmode=disable&x-migrations-table=moderation_schema_migrations
MIGRATIONS_PATH=migrations

build-and-migrate:
	@echo "--- Running Migrations ---"
	migrate -path $(MIGRATIONS_PATH) -database "$(DATABASE_URL)" up
	@echo "--- Building Go Binary ---"
	go build -o ./tmp/main.exe ./cmd/server

migrate-up:
	migrate -path $(MIGRATIONS_PATH) -database "$(DATABASE_URL)" up

migrate-down:
	migrate -path $(MIGRATIONS_PATH) -database "$(DATABASE_URL)" down 1
