COMPOSE_FILE := deploy/compose.yml

.PHONY: api-test web-test lint typecheck up down

api-test:
	cd apps/api && uv run pytest -q

web-test:
	cd apps/web && pnpm test -- --run

lint:
	cd apps/api && uv run ruff check .
	cd apps/web && pnpm lint

typecheck:
	cd apps/api && uv run mypy .
	cd apps/web && pnpm typecheck

up:
	docker compose -f $(COMPOSE_FILE) up -d

down:
	docker compose -f $(COMPOSE_FILE) down --remove-orphans
