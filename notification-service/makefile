DATABASE_URL ?= postgres://postgres:postgres@localhost:5432/socialflow_notifications?sslmode=disable&x-migrations-table=notification_schema_migrations
MIGRATIONS_PATH=migrations

build:
	go build -o ./tmp/main.exe ./cmd/server

test:
	go test ./...

vet:
	go vet ./...

run:
	go run ./cmd/server

migrate-up:
	migrate -path $(MIGRATIONS_PATH) -database "$(DATABASE_URL)" up

migrate-down:
	migrate -path $(MIGRATIONS_PATH) -database "$(DATABASE_URL)" down 1

migrate-create:
	migrate create -ext sql -dir $(MIGRATIONS_PATH) -seq $(name)

build-and-migrate: migrate-up build
