# CaptchaFlow Service Platform

CaptchaFlow Service Platform is a multi-user developer platform for integrating a stable captcha-solving API. The platform owns CDK activation, user sessions, user API Keys, shared quota, rate and concurrency limits, idempotent invocation, call logs, and administrator operations.

The underlying solver is an internal dependency. This repository must not modify its algorithm, expose its address to browser clients, or store its service key outside server-side runtime configuration.

## Documentation

- [V1 product and architecture specification](docs/specs/2026-09-04-geetest-service-platform-v1-design.md)
- [V1 implementation plan](docs/plans/2026-09-04-geetest-service-platform-v1-implementation.md)
- [Production deployment guide](docs/deployment.md)

## Delivery Baseline

- Backend: Go 1.25, stdlib `net/http` router (Go 1.22+ patterns), pgx v5, PostgreSQL 16, Redis 7.
- Frontend: React 19, TypeScript, Vite, Lucide React icons.
- Deployment: one backend service, one web build, PostgreSQL, Redis, reverse proxy with TLS.

## Implemented V1 Features

- CDK activation / re-entry sessions (HttpOnly cookie, no passwords), one-time default API key.
- API Key lifecycle: create (secret shown once), rename, enable/disable, soft delete.
- `POST /v1/captcha/solve`: Bearer auth, Idempotency-Key replay, Redis token-bucket rate limit, concurrency leases, atomic quota RESERVE/CONFIRM/REFUND ledger, typed solver errors with automatic refunds.
- User queries: `GET /v1/account`, `GET /v1/usage`, `GET /v1/calls` (cursor pagination), `GET /v1/calls/{request_id}` (ownership enforced).
- Console debug: `POST /v1/tools/captcha/solve` (session + owned key; no plaintext key handling in the browser).
- Admin surface `/admin/v1`: Argon2id login, dashboard, CDK batch generation (plaintext codes returned once), ledger-based quota adjustments, user suspension with immediate session revocation, immutable audit logs, sanitized solver health probe.
- Call logs are redacted: no solver tokens, key plaintext, or full response bodies (replay payloads live in a dedicated `idempotency_responses` table).

## Backend Quick Start

```bash
make up            # start PostgreSQL 16 + Redis 7 via Docker Compose
make api-migrate   # apply the embedded forward-only migrations
make api-seed      # insert a local demo CDK (CAPTCHA-DEMO-2026)
make api-run       # serve the API on http://localhost:8000
```

Provision an operator account. On first startup the API creates it automatically from `ADMIN_BOOTSTRAP_USERNAME` / `ADMIN_BOOTSTRAP_PASSWORD` in `.env` (only when `admin_users` is still empty, and the password is never logged). A manual re-bootstrap is also available:

```bash
ADMIN_BOOTSTRAP_USERNAME=admin ADMIN_BOOTSTRAP_PASSWORD=<secret> make admin-bootstrap
```

Local end-to-end testing with a fake solver:

```bash
make mock-solver   # listens on 127.0.0.1:18081
# point GEETEST_SOLVER_URL=http://127.0.0.1:18081 and
# GEETEST_SERVICE_API_KEY=demo-service-key in .env, then restart the API
```

Useful checks:

```bash
make api-test                  # unit tests
make api-integration-test      # full HTTP suite against an empty test database
make lint                      # go vet + gofmt, eslint
```

Backend layout (`apps/api`): `cmd/{api,migrate,seed,admin-bootstrap}`, `internal/{config,crypto,domain,store,service,httpapi,ratelimit,solver}`. Migrations are embedded SQL under `internal/store/migrations` and applied exactly once by `cmd/migrate`.

## Frontend Quick Start

```bash
cd apps/web
pnpm install
pnpm dev          # http://localhost:5173, /v1 and /admin are proxied to :8000
pnpm test         # vitest
pnpm build        # production build to dist/
```

## Deployment

`deploy/compose.yml` adds the API service on top of the PostgreSQL/Redis development stack. The API image (built from the repo root so the web console is compiled and embedded into the Go binary) serves both the SPA and the JSON API on `:8000` — history routes like `/console/keys` or `/admin/users` survive hard refreshes via an index.html fallback, while `/v1/*`, `/admin/v1/*`, and `/healthz` always answer with the JSON envelope. Production `APP_ENV=production` rejects placeholder secrets, localhost addresses, and `example.com` URLs at startup.

Do not put runtime secrets, API Keys, CDKs, database dumps, or solver credentials into this repository.
