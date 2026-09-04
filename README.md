# GeeTest Service Platform

GeeTest Service Platform is a multi-user developer platform placed in front of an already deployed, stateless GeeTest solver. The platform owns CDK activation, user sessions, user API Keys, shared quota, rate and concurrency limits, idempotent invocation, call logs, and administrator operations.

The underlying solver is an internal dependency. This repository must not modify its algorithm, expose its address to browser clients, or store its service key outside server-side runtime configuration.

## Documentation

- [V1 product and architecture specification](docs/specs/2026-09-04-geetest-service-platform-v1-design.md)
- [V1 implementation plan](docs/plans/2026-09-04-geetest-service-platform-v1-implementation.md)

## Chosen Delivery Baseline

- Backend: Python 3.12, FastAPI, SQLAlchemy 2, Alembic, PostgreSQL 16, Redis 7, HTTPX.
- Frontend: React 19, TypeScript, Vite, TanStack Router and Query, React Hook Form, Zod, TanStack Table, Lucide React.
- Deployment: one backend service, one web build, PostgreSQL, Redis, reverse proxy with TLS.

The first implementation step is defined in the implementation plan. Do not put runtime secrets, API Keys, CDKs, database dumps, or solver credentials into this repository.
