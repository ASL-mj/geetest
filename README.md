# CaptchaFlow Service Platform

CaptchaFlow Service Platform is a multi-user developer platform for integrating a stable captcha-solving API. The platform owns CDK activation, user sessions, user API Keys, shared quota, rate and concurrency limits, idempotent invocation, call logs, and administrator operations.

The underlying solver is an internal dependency. This repository must not modify its algorithm, expose its address to browser clients, or store its service key outside server-side runtime configuration.

## Documentation

- [V1 product and architecture specification](docs/specs/2026-09-04-geetest-service-platform-v1-design.md)
- [V1 implementation plan](docs/plans/2026-09-04-geetest-service-platform-v1-implementation.md)

## Chosen Delivery Baseline

- Backend: Go 1.25, chi-free stdlib `net/http` router (Go 1.22+ patterns), pgx v5, PostgreSQL 16, Redis 7.
- Frontend: React 19, TypeScript, Vite, TanStack Router and Query, React Hook Form, Zod, TanStack Table, Lucide React.
- Deployment: one backend service, one web build, PostgreSQL, Redis, reverse proxy with TLS.

## Backend Quick Start

```bash
make up            # start PostgreSQL 16 + Redis 7 via Docker Compose
make api-migrate   # apply the embedded forward-only migrations
make api-seed      # insert a local demo CDK (CAPTCHA-DEMO-2026)
make api-run       # serve the API on http://localhost:8000
```

Useful checks:

```bash
make api-test                  # unit tests
make api-integration-test      # full HTTP suite against an empty test database
make lint                      # go vet + gofmt, eslint
```

Backend layout (`apps/api`): `cmd/{api,migrate,seed}`, `internal/{config,crypto,domain,store,service,httpapi}`. Migrations are embedded SQL under `internal/store/migrations` and applied exactly once by `cmd/migrate`.

The first implementation step is defined in the implementation plan. Do not put runtime secrets, API Keys, CDKs, database dumps, or solver credentials into this repository.
