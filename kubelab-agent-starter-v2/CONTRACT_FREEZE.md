# KubeLab contract freeze — v1

Status: **GO for independent `frontend/`, `api/` and `worker/` implementation.** This is a documentation/interface freeze only; no application code or runtime tests have been completed. `API_CONTRACT.md` is authoritative for exact routes, JSON, errors, SQL and environment variables. Any interface change after implementation starts requires coordinator review, a spec edit, and notice to all three agents before code changes.

## Frozen decisions

- Public task routes are `GET/POST /api/tasks` and `DELETE /api/tasks/{id}`; API also owns `GET /api/whoami`, `GET /healthz`, `GET /readyz`, `GET /metrics`. Worker owns `GET/POST /tasks`, `DELETE /tasks/{id}` and its own `GET /healthz`, `GET /readyz`, `GET /metrics`. There is no update, task status, background queue or public Worker route.
- Request is `{"title":"..."}`; task is `{"id":"uuid","title":"...","created_at":"RFC3339 UTC"}`; list is `{"tasks":[...]}` including an empty array. Worker generates UUID v4; PostgreSQL timestamps rows. Title is Unicode trimmed and limited to 1–120 code points; SQL ordering is `created_at DESC, id DESC`.
- Error is exactly `{"error":"code","message":"..."}`. Status/code pairs: `400/validation_error`, `404/not_found`, `405/method_not_allowed`, `500/internal_error`, `502/bad_gateway`, `503/service_unavailable`. API passes valid Worker responses through, maps unavailable/timeouts to 503 and malformed/unexpected Worker responses to 502.
- Worker alone owns the `tasks(id UUID PRIMARY KEY, title TEXT NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT NOW())` table, idempotent startup initialization and DB access. API has no SQL driver, DB config or DB credentials.
- Environment names are `API_PORT`, `WORKER_PORT`, `WORKER_URL`, `DB_HOST`, `DB_PORT`, `DB_NAME`, `POSTGRES_USER`, `POSTGRES_PASSWORD`, and API identity vars `POD_NAME`, `POD_NAMESPACE`, `NODE_NAME`, `POD_IP`. PostgreSQL `POSTGRES_DB` is mapped from `DB_NAME`. API/Worker listen ports default to 8080; `WORKER_URL` and Worker DB settings are required. Username/password are required and have no defaults.
- Local Compose receives operator-chosen credentials from ignored `.env`; tracked `.env.example` has empty placeholders. Cluster PostgreSQL and Worker read the same keys from operator-created `postgres-credentials` Secret. No credential reaches API or frontend. Existing PostgreSQL storage needs an explicit DB credential change if the Secret values change.
- Frontend uses same-origin `/api/...`. Ingress sends `/api` to API and `/` to frontend. Compose routing is owned by integration; frontend Nginx need not proxy API. Readiness is local for API and DB-checked for Worker; liveness remains independent of dependencies.

## Implementation handoffs

### Frontend agent — `frontend/` only

Build static HTML/CSS/vanilla JS and its Nginx image/config. Call the four public API task/identity routes with relative same-origin URLs. Render task title as text, show loading/empty/error states, and handle 204 as an empty response. Validate the title for immediate feedback, but treat API errors as authoritative. Do not edit specs, service code or shared Compose/deployment files. Verify basic UI behavior where practical; report actual commands and any integration assumptions.

### API agent — `api/` only

Build the Go `net/http` façade and image. Validate the public task request/UUID, delegate synchronously to `WORKER_URL` using bounded timeouts, and implement the frozen pass-through/error mapping. Expose local whoami, health, readiness and bounded-label metrics. Read identity env vars; use no DB package, DB env var or credential. Test with a mock Worker for success, validation, malformed replies and unavailability. Do not edit other agent or shared files.

### Worker agent — `worker/` only

Build the Go `net/http` Task Service, image and in-directory SQL initialization. Use `database/sql` with a PostgreSQL driver and required env config. Generate UUID v4 in Go, normalize/validate title, initialize the exact table, implement ordered list/create/delete, bounded DB calls, DB-backed readiness and process-only liveness. Add bounded-label metrics. Test validation and SQL behavior where a disposable DB is available; report skipped DB tests honestly. Do not edit other agent or shared files.

### Later integration owner

Own `compose.yaml`, `.env.example`, `.gitignore`, `Makefile`, root README, `deploy/`, `scripts/` and cross-service tests. Supply matching PostgreSQL/Worker credentials, map `DB_NAME` to `POSTGRES_DB`, route same-origin `/api` locally, then verify Compose before Kubernetes. Do not modify agent-owned directories without coordination.

## Outstanding issues and verification

No unresolved interface blocker. Runtime image compatibility, exact pinned dependency/chart versions, resource sizing and the monitoring stack choice require implementation-time verification; they do not change the frozen service contract. The three agents can start independently now.

Contract review checks: inspect all six specifications and handoff prompts for route/config consistency. Implementation verification remains **NOT RUN**: frontend UI, API mock-Worker tests, Worker DB tests, Compose smoke tests, and Kubernetes acceptance checks in `ACCEPTANCE_TESTS.md`. Do not report any as passed until executed.
