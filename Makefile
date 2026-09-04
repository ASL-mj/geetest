COMPOSE_FILE := deploy/compose.yml
TEST_DATABASE_URL ?= postgres://postgres:postgres@localhost:5432/geetest_platform_test?sslmode=disable

.PHONY: api-run api-migrate api-seed api-test api-integration-test api-lint web-test web-lint web-typecheck lint typecheck test up down

api-run:
	cd apps/api && go run ./cmd/api

api-migrate:
	cd apps/api && go run ./cmd/migrate

api-seed:
	cd apps/api && go run ./cmd/seed -code CAPTCHA-DEMO-2026 -quota 100

api-test:
	cd apps/api && go test ./...

api-integration-test:
	docker compose -f $(COMPOSE_FILE) exec -T postgres psql -U postgres -c "DROP DATABASE IF EXISTS geetest_platform_test" -c "CREATE DATABASE geetest_platform_test"
	cd apps/api && TEST_DATABASE_URL="$(TEST_DATABASE_URL)" go test ./internal/integration/

api-lint:
	cd apps/api && go vet ./... && test -z "$$(gofmt -l .)"

web-test:
	cd apps/web && pnpm test -- --run

web-lint:
	cd apps/web && pnpm lint

web-typecheck:
	cd apps/web && pnpm typecheck

lint: api-lint web-lint
typecheck: web-typecheck
test: api-test web-test

up:
	docker compose -f $(COMPOSE_FILE) up -d

down:
	docker compose -f $(COMPOSE_FILE) down --remove-orphans
