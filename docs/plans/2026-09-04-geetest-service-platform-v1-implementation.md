# CaptchaFlow Service Platform V1 Implementation Plan

> **Current implementation baseline (2026-09-05):** The repository has been
> delivered as a Go 1.25 modular monolith with a React 19/Vite console. The
> original Python/FastAPI and TanStack examples below are retained as the
> historical execution draft only; use `README.md`, `apps/api`, and
> `apps/web` as the authoritative implementation paths. Tasks 1-9 and the
> core of Task 10 are implemented and committed. The next delivery slice is
> the remaining administrator read surfaces (API Keys, calls, quota ledger),
> OpenAPI/deployment hardening, and browser-based release acceptance.

## Current Delivery Checklist

- [x] Platform runtime, migrations, Docker Compose, Makefile and secret boundary.
- [x] CDK activation/re-entry sessions without user passwords.
- [x] API Key lifecycle, shared quota, rate/concurrency limits and idempotency.
- [x] Solver gateway proxy, redacted call logs, user console and public docs.
- [x] Administrator authentication, CDK batches/CDKs, users, audit, health and settings UI.
- [x] Quota settlement recovery, transition-failure cleanup, JSON strict decoding and route synchronization.
- [ ] Administrator API Key/call-log/quota-ledger query pages and filters.
- [ ] OpenAPI contract, production reverse proxy and CI/release acceptance gates.

> **For agentic workers:** REQUIRED SUB-SKILL: Use `subagent-driven-development` or `executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a self-hosted multi-user API platform branded as CaptchaFlow that calls the existing solver only from the backend, while enforcing CDK entitlement, shared quota, Key authentication, idempotency, and auditability.

**Architecture:** Build a modular monolith with a FastAPI backend and a React web application. PostgreSQL is the source of truth for users, CDKs, Keys, calls and the append-only quota ledger; Redis supplies distributed rate limiting, concurrency leases and short duplicate-request waiting. `SolverGateway` is the only component allowed to call the existing GeeTest HTTP service.

**Tech Stack:** Python 3.12, FastAPI, Pydantic v2, SQLAlchemy 2 async, Alembic, PostgreSQL 16, Redis 7, HTTPX, Argon2id, React 19, TypeScript, Vite, TanStack Router, TanStack Query, React Hook Form, Zod, TanStack Table, Lucide React, pytest, Playwright, Docker Compose.

---

## Delivery Rules

- The repository starts without application code. Create the target structure in Task 1; do not retrofit an unrelated project.
- Keep the existing solver address and `X-Service-Key` in server-only environment variables. Browser code must never reference either value.
- Every database migration is forward-only and must be tested against an empty PostgreSQL database.
- Each completed task receives its own conventional commit. Do not commit `.env`, CDK exports, browser sessions, database data, API Keys, or solver credentials.
- Tests use an HTTPX mock transport or a fixture server. They never send requests to the deployed solver.
- API contract changes require an OpenAPI snapshot update and a documentation update in the same commit.

## Target Repository Structure

```text
geetest-service-platform/
├── apps/
│   ├── api/
│   │   ├── app/
│   │   │   ├── api/{public,admin}/
│   │   │   ├── application/
│   │   │   ├── domain/
│   │   │   ├── infrastructure/
│   │   │   ├── db/
│   │   │   ├── core/
│   │   │   └── main.py
│   │   ├── alembic/
│   │   ├── tests/{unit,integration}/
│   │   └── pyproject.toml
│   └── web/
│       ├── src/{app,features,components,lib}/
│       ├── public/
│       └── package.json
├── deploy/
│   ├── compose.yml
│   └── nginx/
├── docs/{specs,plans,api}/
├── .env.example
└── Makefile
```

## Task 1: Bootstrap the Repository and Runtime Contract

**Files:**
- Create: `apps/api/pyproject.toml`
- Create: `apps/api/app/main.py`
- Create: `apps/api/app/core/settings.py`
- Create: `apps/api/tests/unit/test_health.py`
- Create: `apps/web/package.json`
- Create: `apps/web/src/app/App.tsx`
- Create: `deploy/compose.yml`
- Create: `.env.example`
- Create: `Makefile`

- [ ] **Step 1: Create the failing backend health test.**

```python
from fastapi.testclient import TestClient
from app.main import create_app


def test_platform_health_is_public_and_does_not_check_solver() -> None:
    response = TestClient(create_app()).get("/healthz")
    assert response.status_code == 200
    assert response.json() == {"status": "ok"}
```

- [ ] **Step 2: Run the test before creating the application.**

Run: `cd apps/api && uv run pytest tests/unit/test_health.py -q`  
Expected: import failure because `app.main` does not exist.

- [ ] **Step 3: Create the application factory and settings boundary.**

```python
# apps/api/app/main.py
from fastapi import FastAPI


def create_app() -> FastAPI:
    app = FastAPI(title="CaptchaFlow Service Platform", version="1.0.0")

    @app.get("/healthz", include_in_schema=False)
    async def healthz() -> dict[str, str]:
        return {"status": "ok"}

    return app


app = create_app()
```

`Settings` must require `DATABASE_URL`, `REDIS_URL`, `SESSION_SECRET`, `API_KEY_PEPPER`, `CDK_PEPPER`, `GEETEST_SOLVER_URL`, and `GEETEST_SERVICE_API_KEY` in production. `.env.example` contains names and non-secret sample values only.

- [ ] **Step 4: Add deterministic local commands.**

`Makefile` must expose `api-test`, `web-test`, `lint`, `typecheck`, `up`, and `down`; `deploy/compose.yml` must define PostgreSQL and Redis with named volumes but no solver service.

- [ ] **Step 5: Verify the bootstrap contract.**

Run: `cd apps/api && uv run pytest tests/unit/test_health.py -q`  
Expected: `1 passed`.

Run: `git check-ignore -v .env`  
Expected: `.gitignore` matches `.env`.

- [ ] **Step 6: Commit.**

```bash
git add .env.example Makefile apps/api apps/web deploy
git commit -m "chore: bootstrap geetest service platform"
```

## Task 2: Create the Relational Domain Schema

**Files:**
- Create: `apps/api/app/db/base.py`
- Create: `apps/api/app/domain/models/{user,cdk,api_key,quota_ledger,api_call,admin,session,setting}.py`
- Create: `apps/api/alembic/versions/0001_platform_domain.py`
- Create: `apps/api/tests/integration/test_platform_schema.py`

- [ ] **Step 1: Write the schema assertions first.**

```python
async def test_cdk_is_bound_to_at_most_one_user(session) -> None:
    cdk = await create_cdk(session)
    await bind_cdk(session, cdk.id, user_id=uuid4())
    with pytest.raises(IntegrityError):
        await bind_cdk(session, cdk.id, user_id=uuid4())
```

Add tests for the unique API Key hash, unique request ID, unique `(user_id, operation, idempotency_key_hash)`, and unique `(api_call_id, entry_type)` ledger entry.

- [ ] **Step 2: Run schema tests before the migration exists.**

Run: `cd apps/api && uv run pytest tests/integration/test_platform_schema.py -q`  
Expected: collection or migration failure.

- [ ] **Step 3: Implement SQLAlchemy models and migration `0001_platform_domain`.**

Create the tables defined in the V1 specification: `users`, `cdk_batches`, `cdks`, `api_keys`, `quota_ledger`, `api_calls`, `admin_users`, `admin_audit_logs`, `system_settings`, `user_sessions`, and `admin_sessions`. Use UUID primary keys, `timestamptz`, `bigint` quota columns and named check constraints for non-negative quota values.

The migration must create these critical constraints:

```sql
UNIQUE (bound_user_id);
UNIQUE (key_hash);
UNIQUE (request_id);
UNIQUE (user_id, operation, idempotency_key_hash);
UNIQUE (api_call_id, entry_type);
CHECK (quota_total >= 0);
CHECK (quota_used >= 0);
CHECK (quota_reserved >= 0);
CHECK (quota_remaining >= 0);
```

- [ ] **Step 4: Create the append-only ledger database role policy.**

Create a migration that revokes `UPDATE` and `DELETE` on `quota_ledger` from the application role after table creation. Use a database trigger only when the deployment role cannot enforce grant separation.

- [ ] **Step 5: Verify migrations and indexes.**

Run: `cd apps/api && uv run alembic upgrade head && uv run pytest tests/integration/test_platform_schema.py -q`  
Expected: migration completes and all uniqueness tests pass.

- [ ] **Step 6: Commit.**

```bash
git add apps/api/app/db apps/api/app/domain apps/api/alembic apps/api/tests/integration
git commit -m "feat: add platform domain schema"
```

## Task 3: Implement CDK Identity Sessions and Activation

**Files:**
- Create: `apps/api/app/application/auth/{sessions,activate}.py`
- Create: `apps/api/app/api/public/auth.py`
- Create: `apps/api/app/infrastructure/repositories/{users,cdks,sessions}.py`
- Create: `apps/api/tests/integration/test_cdk_activation.py`

- [ ] **Step 1: Write activation tests.**

```python
async def test_activate_binds_cdk_creates_user_session_and_default_key(client, seeded_cdk):
    response = await client.post("/v1/auth/activate", json={
        "cdk": seeded_cdk.plaintext,
    })
    assert response.status_code == 201
    assert response.json()["data"]["default_api_key"].startswith("cf_live_")
    assert "session" in response.cookies
```

Add separate cases for expired, disabled, exhausted and already-bound CDKs. A repeated valid CDK must issue a new browser session without creating another user or Key. Assert that failed activation never creates a user or Key.

- [ ] **Step 2: Run the activation tests before implementation.**

Run: `cd apps/api && uv run pytest tests/integration/test_cdk_activation.py -q`
Expected: route-not-found failure.

- [ ] **Step 3: Implement CDK identity, activation and session services.**

Normalize CDKs, look them up by `HMAC-SHA-256(cdk, CDK_PEPPER)`, and lock the CDK row with `SELECT ... FOR UPDATE` within one transaction. On first use, set `expires_at` from the batch service duration, bind an internal user, create the default Key, create a revocable session record, and issue an HttpOnly, Secure, SameSite session cookie. On later use, validate the bound CDK and issue a new session without creating another user or Key.

- [ ] **Step 4: Add logout and session dependency.**

`POST /v1/auth/logout` revokes only the current session. The `require_user_session` dependency loads an active user and rejects disabled accounts. There is no end-user password login endpoint; CDK re-entry uses `POST /v1/auth/activate`.

- [ ] **Step 5: Verify race safety.**

Run: `cd apps/api && uv run pytest tests/integration/test_cdk_activation.py -q`  
Expected: all state cases pass.

Run: `cd apps/api && uv run pytest tests/integration/test_cdk_activation.py -k concurrent -q`  
Expected: exactly one concurrent activation returns `201`.

- [ ] **Step 6: Commit.**

```bash
git add apps/api/app/application/auth apps/api/app/api/public/auth.py apps/api/app/infrastructure/repositories apps/api/tests
git commit -m "feat: add cdk activation and user sessions"
```

## Task 4: Implement API Key Lifecycle and Effective Authorization

**Files:**
- Create: `apps/api/app/application/keys/{create,update,revoke,authenticate}.py`
- Create: `apps/api/app/api/public/keys.py`
- Create: `apps/api/app/api/dependencies/api_key.py`
- Create: `apps/api/tests/integration/test_api_keys.py`
- Create: `apps/api/tests/unit/test_api_key_authentication.py`

- [ ] **Step 1: Write Key lifecycle tests.**

```python
async def test_new_key_is_returned_once_and_stored_as_hash(client, user_session, db):
    response = await client.post("/v1/keys", json={"name": "worker-a"})
    plaintext = response.json()["data"]["secret"]
    assert plaintext.startswith("cf_live_")
    saved_key = await get_only_api_key(db)
    assert saved_key.key_hash != plaintext.encode()
    assert plaintext not in (await client.get("/v1/keys")).text
```

Add tests proving disabled, deleted, expired-CDK, disabled-CDK and exhausted-CDK Keys cannot authenticate the solve endpoint.

- [ ] **Step 2: Run tests before implementing routes.**

Run: `cd apps/api && uv run pytest tests/integration/test_api_keys.py tests/unit/test_api_key_authentication.py -q`  
Expected: route-not-found failure.

- [ ] **Step 3: Implement creation and display masking.**

Generate 256-bit random secrets with the `cf_live_` prefix. Store an HMAC hash using `API_KEY_PEPPER`; store `key_prefix` and `key_last4` for display. Return `secret` only from `POST /v1/keys`; list and detail responses never include it.

- [ ] **Step 4: Implement effective authorization.**

The API Key dependency must perform all checks in order: Key status, user status, CDK status, `expires_at`, then `quota_remaining`. It returns an immutable `AuthenticatedCaller` containing user, CDK and Key IDs for the request lifecycle.

- [ ] **Step 5: Verify lifecycle behavior.**

Run: `cd apps/api && uv run pytest tests/integration/test_api_keys.py tests/unit/test_api_key_authentication.py -q`  
Expected: all cases pass and no response contains the test secret after creation.

- [ ] **Step 6: Commit.**

```bash
git add apps/api/app/application/keys apps/api/app/api/public/keys.py apps/api/app/api/dependencies apps/api/tests
git commit -m "feat: add api key lifecycle and authorization"
```

## Task 5: Build Admission Control, Quota Settlement, and Idempotency

**Files:**
- Create: `apps/api/app/application/calls/{idempotency,quota,admission,settlement}.py`
- Create: `apps/api/app/infrastructure/redis/{rate_limit,semaphore}.py`
- Create: `apps/api/tests/integration/test_quota_settlement.py`
- Create: `apps/api/tests/integration/test_idempotency.py`
- Create: `apps/api/tests/unit/test_rate_limit.py`

- [ ] **Step 1: Write the quota and duplicate-request tests.**

```python
async def test_only_available_quota_requests_are_reserved(concurrent_solve_requests, cdk):
    cdk.quota_remaining = 2
    responses = await concurrent_solve_requests(3)
    assert sum(response.status_code == 200 for response in responses) == 2
    assert sum(response.status_code == 402 for response in responses) == 1
```

```python
async def test_same_idempotency_key_calls_solver_once(client, valid_headers, solver_mock):
    first = await client.post("/v1/captcha/solve", headers=valid_headers, json={"captcha_id": "c1", "risk_type": "slide"})
    second = await client.post("/v1/captcha/solve", headers=valid_headers, json={"captcha_id": "c1", "risk_type": "slide"})
    assert first.json() == second.json()
    assert solver_mock.call_count == 1
```

- [ ] **Step 2: Run tests before the admission services exist.**

Run: `cd apps/api && uv run pytest tests/integration/test_quota_settlement.py tests/integration/test_idempotency.py tests/unit/test_rate_limit.py -q`  
Expected: import failure.

- [ ] **Step 3: Implement durable idempotency creation.**

Insert an `api_calls` row using the user, operation and HMAC of `Idempotency-Key`. On unique conflict, return the stored terminal response or `409 IDEMPOTENCY_IN_PROGRESS`; never allocate a second Redis lease or reserve another quota unit.

- [ ] **Step 4: Implement quota reservation and settlement transactions.**

Reservation executes a conditional `UPDATE cdks ... WHERE quota_remaining > 0` and inserts `RESERVE` in the same transaction. Success moves one unit from `quota_reserved` to `quota_used` and inserts `CONFIRM`. Refund moves one unit from `quota_reserved` to `quota_remaining` and inserts `REFUND`. The unique ledger constraint makes settlement retry-safe.

- [ ] **Step 5: Implement Redis rate and concurrency controls.**

Use a Lua token bucket keyed by CDK ID for the per-minute limit. Use a Redis lease keyed by CDK ID for concurrency, with a TTL exceeding the HTTP solver timeout. Release the lease in `finally`; report a `Retry-After` header for rate rejection.

- [ ] **Step 6: Verify successful, rejected, and refunded outcomes.**

Run: `cd apps/api && uv run pytest tests/integration/test_quota_settlement.py tests/integration/test_idempotency.py tests/unit/test_rate_limit.py -q`  
Expected: all tests pass, including exactly-once refund under repeated settlement attempts.

- [ ] **Step 7: Commit.**

```bash
git add apps/api/app/application/calls apps/api/app/infrastructure/redis apps/api/tests
git commit -m "feat: add quota admission and idempotency"
```

## Task 6: Add the Solver Gateway and Public Solve Endpoint

**Files:**
- Create: `apps/api/app/application/solver/solve_geetest.py`
- Create: `apps/api/app/infrastructure/solver/geetest_http.py`
- Create: `apps/api/app/api/public/geetest.py`
- Create: `apps/api/tests/integration/test_geetest_solve.py`
- Create: `apps/api/tests/unit/test_geetest_http.py`

- [ ] **Step 1: Write solver gateway tests with a mocked HTTP transport.**

```python
async def test_solver_failure_refunds_reserved_quota(client, valid_headers, solver_returns_502):
    before = await quota_snapshot()
    response = await client.post("/v1/captcha/solve", headers=valid_headers, json={"captcha_id": "c1", "risk_type": "slide"})
    after = await quota_snapshot()
    assert response.status_code == 502
    assert after.remaining == before.remaining
    assert after.reserved == before.reserved
```

Add tests for solver timeout, solver 422 parameter failure, solver malformed JSON, and success response normalization.

- [ ] **Step 2: Run tests before the gateway exists.**

Run: `cd apps/api && uv run pytest tests/integration/test_geetest_solve.py tests/unit/test_geetest_http.py -q`  
Expected: import failure.

- [ ] **Step 3: Implement `SolverGateway`.**

`GeeTestHttpSolver` accepts its base URL and service key only from `Settings`. It sends `Content-Type`, `X-Service-Key`, and platform `X-Request-ID`; uses explicit connect, read and total timeouts; maps timeout and transport failures to typed domain errors; never passes the downstream error body to an API response.

- [ ] **Step 4: Implement `POST /v1/captcha/solve`.**

Validate non-empty `captcha_id` and `risk_type == "slide"` before admission. Connect `AuthenticatedCaller`, idempotency, Redis admission, quota reservation, gateway call, settlement and redacted `api_calls` persistence in one application service.

- [ ] **Step 5: Verify the no-secret boundary.**

Run: `cd apps/api && uv run pytest tests/integration/test_geetest_solve.py tests/unit/test_geetest_http.py -q`  
Expected: all outcome tests pass.

Run: `rg -n 'GEETEST_SERVICE_API_KEY|X-Service-Key' apps/web`  
Expected: no matches.

- [ ] **Step 6: Commit.**

```bash
git add apps/api/app/application/solver apps/api/app/infrastructure/solver apps/api/app/api/public/geetest.py apps/api/tests
git commit -m "feat: proxy geetest solves through platform backend"
```

## Task 7: Expose User Account, Usage, Call Log, and Key APIs

**Files:**
- Create: `apps/api/app/api/public/{account,usage,calls,tools}.py`
- Create: `apps/api/app/application/queries/{account,usage,calls}.py`
- Create: `apps/api/tests/integration/test_user_queries.py`
- Modify: `apps/api/app/api/public/keys.py`

- [ ] **Step 1: Write data-isolation tests.**

```python
async def test_user_can_only_list_own_calls(client, alice_session, bob_call):
    response = await client.get("/v1/calls", cookies=alice_session.cookies)
    assert response.status_code == 200
    assert bob_call.request_id not in response.text
```

Add pagination tests, exact `request_id` lookup tests, Key summary tests, and online test-tool parity tests.

- [ ] **Step 2: Run tests before query routes exist.**

Run: `cd apps/api && uv run pytest tests/integration/test_user_queries.py -q`  
Expected: route-not-found failure.

- [ ] **Step 3: Implement user-scoped query services.**

`GET /v1/account` returns service status and CDK summary. `GET /v1/usage` returns quota totals, current-day calls, success rate and Key aggregates. `GET /v1/calls` enforces `user_id` at query construction, supports cursor pagination, and returns redacted records only. `GET /v1/calls/{request_id}` verifies ownership before returning details.

- [ ] **Step 4: Implement the online debug endpoint.**

`POST /v1/tools/captcha/solve` requires a user session and a selected owned Key ID. It invokes the same solve application service with `source="console"`; it never accepts or returns a plaintext API Key.

- [ ] **Step 5: Verify user isolation and redaction.**

Run: `cd apps/api && uv run pytest tests/integration/test_user_queries.py -q`  
Expected: all owner and non-owner cases pass.

- [ ] **Step 6: Commit.**

```bash
git add apps/api/app/api/public apps/api/app/application/queries apps/api/tests
git commit -m "feat: add user account usage and call APIs"
```

## Task 8: Build Administrator Authentication and Operations APIs

**Files:**
- Create: `apps/api/app/application/admin/{auth,cdk_batches,cdks,users,quota,audit,health}.py`
- Create: `apps/api/app/api/admin/{auth,dashboard,cdk_batches,cdks,users,api_keys,calls,quota_ledger,settings,audit_logs,solver_health}.py`
- Create: `apps/api/tests/integration/test_admin_operations.py`
- Create: `apps/api/tests/integration/test_admin_audit.py`

- [ ] **Step 1: Write authorization and audit tests.**

```python
async def test_admin_quota_adjustment_creates_ledger_and_audit_log(client, admin_session, cdk):
    response = await client.post(f"/admin/v1/cdks/{cdk.id}/quota-adjustments", json={"delta": 100, "reason": "support case 42"})
    assert response.status_code == 201
    assert await ledger_contains(cdk.id, entry_type="ADMIN_ADJUSTMENT", delta=100)
    assert await audit_contains(action="cdk.quota_adjusted", target_id=cdk.id)
```

Add tests proving a read-only administrator receives `403` for every write endpoint and a normal user cannot access `/admin/v1`.

- [ ] **Step 2: Run tests before admin routes exist.**

Run: `cd apps/api && uv run pytest tests/integration/test_admin_operations.py tests/integration/test_admin_audit.py -q`  
Expected: route-not-found failure.

- [ ] **Step 3: Implement separate administrator sessions and RBAC.**

Create `admin_users` and `admin_sessions` dependencies separate from user authentication. Roles are `admin` and `viewer`; endpoint permissions are declared at routing time.

- [ ] **Step 4: Implement operational write paths.**

Batch CDK generation exports plaintext values only in the creation response or one generated file, then persists only hashes and prefixes. Quota adjustments must use the ledger service, never update CDK counters directly. User suspension changes effective Key availability immediately. All writes require `reason` and create immutable audit records.

- [ ] **Step 5: Implement system health and read APIs.**

`GET /admin/v1/solver-health` performs a timeout-bound backend health request and returns only status, latency, checked time and sanitized failure category. Dashboard aggregation queries do not expose full sensitive call responses.

- [ ] **Step 6: Verify administrator boundaries.**

Run: `cd apps/api && uv run pytest tests/integration/test_admin_operations.py tests/integration/test_admin_audit.py -q`  
Expected: all role, audit, batch and health tests pass.

- [ ] **Step 7: Commit.**

```bash
git add apps/api/app/application/admin apps/api/app/api/admin apps/api/tests
git commit -m "feat: add administrator operations and audit logs"
```

## Task 9: Implement the User Console

**Files:**
- Create: `apps/web/src/app/{router,providers,layout}.tsx`
- Create: `apps/web/src/features/auth/*`
- Create: `apps/web/src/features/dashboard/*`
- Create: `apps/web/src/features/keys/*`
- Create: `apps/web/src/features/usage/*`
- Create: `apps/web/src/features/calls/*`
- Create: `apps/web/src/features/docs/*`
- Create: `apps/web/src/features/account/*`
- Create: `apps/web/src/lib/api.ts`
- Create: `apps/web/src/lib/status.ts`
- Create: `apps/web/src/**/*.test.tsx`

- [ ] **Step 1: Write the Key creation UI test.**

```tsx
it("shows the new Key secret once and removes it after dismissal", async () => {
  render(<CreateKeyDialog />)
  await userEvent.type(screen.getByLabelText("Name"), "worker-a")
  await userEvent.click(screen.getByRole("button", { name: "Create Key" }))
  expect(await screen.findByText("cf_live_test_secret")).toBeVisible()
  await userEvent.click(screen.getByRole("button", { name: "Done" }))
  expect(screen.queryByText("cf_live_test_secret")).not.toBeInTheDocument()
})
```

- [ ] **Step 2: Run the frontend test before feature implementation.**

Run: `cd apps/web && pnpm test -- CreateKeyDialog`  
Expected: module-not-found failure.

- [ ] **Step 3: Build the shared application shell.**

Use a dense desktop sidebar that collapses into a mobile drawer. Use Lucide buttons with tooltips, a shared `StatusBadge`, monospace identifiers, cursor-paginated tables, detail drawers and copy actions. Cards have an 8px maximum radius.

- [ ] **Step 4: Implement user routes and query state.**

Implement `/activate`, `/dashboard`, `/keys`, `/playground`, `/usage`, `/calls`, `/docs`, and `/account`. Use HTTP-only session cookies with `credentials: "include"`; never store CDKs, user sessions or API Keys in localStorage. The playground sends a selected Key ID, not a secret.

- [ ] **Step 5: Verify desktop and narrow layouts.**

Run: `cd apps/web && pnpm test`  
Expected: all component tests pass.

Run: `cd apps/web && pnpm build`  
Expected: production build completes without TypeScript errors.

- [ ] **Step 6: Commit.**

```bash
git add apps/web
git commit -m "feat: add user developer console"
```

## Task 10: Implement the Administrator Console

**Files:**
- Create: `apps/web/src/features/admin/{auth,dashboard,cdk_batches,cdks,users,api_keys,calls,ledger,settings,audit,solver_health}/*`
- Modify: `apps/web/src/app/router.tsx`
- Create: `apps/web/src/features/admin/**/*.test.tsx`

- [ ] **Step 1: Write an audited action confirmation test.**

```tsx
it("requires an operation reason before confirming a quota adjustment", async () => {
  render(<QuotaAdjustmentDialog cdkId="cdk_1" />)
  await userEvent.click(screen.getByRole("button", { name: "Confirm" }))
  expect(screen.getByText("Reason is required")).toBeVisible()
  await userEvent.type(screen.getByLabelText("Reason"), "support case 42")
  expect(screen.getByRole("button", { name: "Confirm" })).toBeEnabled()
})
```

- [ ] **Step 2: Run the test before implementing admin features.**

Run: `cd apps/web && pnpm test -- QuotaAdjustmentDialog`  
Expected: module-not-found failure.

- [ ] **Step 3: Implement admin routes and guarded layout.**

Create `/admin/login`, `/admin/dashboard`, `/admin/cdk-batches`, `/admin/cdks`, `/admin/users`, `/admin/keys`, `/admin/calls`, `/admin/quota-ledger`, `/admin/settings`, `/admin/audit`, and `/admin/solver-health`. The route guard must load the dedicated admin session and reject user sessions.

- [ ] **Step 4: Implement operational data tables and dialogs.**

All tables expose server-side filters, cursor pagination, copy controls and an inspection drawer. Destructive actions require confirmation. CDK generation download is available once per batch creation action; later lists display prefix only.

- [ ] **Step 5: Verify front-end guard and interaction rules.**

Run: `cd apps/web && pnpm test`  
Expected: all user and admin tests pass.

Run: `cd apps/web && pnpm build`  
Expected: production build completes.

- [ ] **Step 6: Commit.**

```bash
git add apps/web
git commit -m "feat: add administrator console"
```

## Task 11: Add Deployment, API Documentation, and Operational Controls

**Files:**
- Create: `docs/api/openapi-v1.yaml`
- Create: `docs/api/error-codes.md`
- Create: `deploy/nginx/default.conf`
- Create: `deploy/systemd/geetest-platform.service`
- Modify: `deploy/compose.yml`
- Modify: `.env.example`
- Create: `apps/api/tests/integration/test_openapi_contract.py`

- [ ] **Step 1: Write the API contract test.**

```python
def test_openapi_declares_public_solve_auth_and_idempotency(client) -> None:
    operation = client.get("/openapi.json").json()["paths"]["/v1/captcha/solve"]["post"]
    assert "Authorization" in operation["parameters"][0]["name"]
    assert any(item["name"] == "Idempotency-Key" for item in operation["parameters"])
```

- [ ] **Step 2: Run the test before publishing the contract.**

Run: `cd apps/api && uv run pytest tests/integration/test_openapi_contract.py -q`  
Expected: assertion failure until the solve endpoint documents both headers.

- [ ] **Step 3: Publish API and operational documentation.**

Document all V1 request/response schemas, pagination, error codes, `Retry-After`, Key lifecycle and security expectations. Export OpenAPI from FastAPI and compare it against the committed contract in CI.

- [ ] **Step 4: Harden deployment boundaries.**

Nginx terminates TLS and proxies only platform routes. PostgreSQL and Redis do not publish host ports in production. The backend process receives solver configuration through the deployment secret store. The solver service remains external to this Compose file.

- [ ] **Step 5: Verify contract and configuration hygiene.**

Run: `cd apps/api && uv run pytest tests/integration/test_openapi_contract.py -q`  
Expected: contract test passes.

Run: `rg -n 'GEETEST_SERVICE_API_KEY=.*[^_]' .env.example deploy apps || true`  
Expected: no populated secret values.

- [ ] **Step 6: Commit.**

```bash
git add docs/api deploy .env.example apps/api/tests/integration
git commit -m "docs: add platform api and deployment contract"
```

## Task 12: Run End-to-End Verification and Release Readiness Checks

**Files:**
- Create: `apps/api/tests/integration/test_release_invariants.py`
- Create: `apps/web/e2e/platform.spec.ts`
- Create: `.github/workflows/ci.yml`
- Create: `docs/release/v1-acceptance.md`

- [ ] **Step 1: Encode release invariants as tests.**

```python
async def test_solver_failure_creates_exactly_one_refund_and_no_sensitive_log(client, valid_headers, solver_timeout):
    response = await client.post("/v1/captcha/solve", headers=valid_headers, json={"captcha_id": "c1", "risk_type": "slide"})
    assert response.status_code == 504
    assert await count_ledger_entries(entry_type="REFUND") == 1
    assert "cf_live_" not in await serialized_call_log(response.json()["request_id"])
```

Cover the following invariants: browser bundle has no solver address or service key; cross-user reads fail; exhausted CDKs do not reach the solver; three concurrent calls with quota two produce at most two solver calls; repeated idempotency keys settle once; every administrator write produces an audit row.

- [ ] **Step 2: Run invariant tests before the complete suite.**

Run: `cd apps/api && uv run pytest tests/integration/test_release_invariants.py -q`  
Expected: all invariants pass after Tasks 1-11.

- [ ] **Step 3: Add browser flows using an isolated local stack.**

Cover activation, first-Key disclosure, Key disable, online test, calls filtering, CDK generation, quota adjustment and user suspension. The browser test fixture points the backend to a mock solver, never the deployed service.

- [ ] **Step 4: Add CI gates.**

CI runs backend unit/integration tests, frontend unit tests, TypeScript build, lint, OpenAPI snapshot verification, migration upgrade on PostgreSQL, and secret-pattern scanning. The release workflow requires all gates before an image is tagged.

- [ ] **Step 5: Execute the full verification set.**

Run: `make lint && make typecheck && make api-test && make web-test`  
Expected: all commands succeed.

Run: `docker compose -f deploy/compose.yml config -q`  
Expected: configuration validates without exposing a solver credential.

- [ ] **Step 6: Commit.**

```bash
git add apps/api/tests apps/web/e2e .github docs/release
git commit -m "test: add v1 release verification gates"
```

## Plan Self-Review

| V1 requirement | Plan coverage |
|---|---|
| CDK activation, account login and session | Task 3 |
| User API Key lifecycle and effective status | Task 4 |
| Shared quota, rate, concurrency and idempotency | Task 5 |
| Solver-only backend proxy and refund | Task 6 |
| User dashboard data, logs and online test | Task 7 and Task 9 |
| Administrator operations and audit | Task 8 and Task 10 |
| API documentation, secrets boundary and deployment | Task 11 |
| Security, data isolation and acceptance verification | Task 12 |

The plan deliberately keeps payment, multiple captcha providers, teams, Webhooks, queues and multi-node execution out of V1. Each plan task has explicit paths, a pre-implementation test, a verification command and a separate commit boundary.
