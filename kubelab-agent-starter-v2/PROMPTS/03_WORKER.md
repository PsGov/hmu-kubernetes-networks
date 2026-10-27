# Prompt 3 — Worker/DB implementation agent

Read `AGENTS.md`, `PRODUCT_SPEC.md`, `API_CONTRACT.md`, `CONTRACT_FREEZE.md`. Implement **only** `worker/`; do not change shared files, `api/`, `frontend/`, or deployment files.

Implement a synchronous Go HTTP Task Service using `net/http`, `database/sql` and a PostgreSQL driver. Implement the exact Worker routes, JSON, validation and SQL table in `API_CONTRACT.md`. Generate UUID v4 in Go; let PostgreSQL assign the timestamp; list by `created_at DESC, id DESC`. Use idempotent startup `CREATE TABLE IF NOT EXISTS` and integration tests that skip cleanly without a live DB. Listen on `WORKER_PORT`; DB connection configuration comes only from `DB_HOST`, `DB_PORT`, `DB_NAME`, `POSTGRES_USER`, `POSTGRES_PASSWORD`; require credentials to be provided externally. `/healthz` checks process; `/readyz` checks DB. Add bounded Prometheus metrics, clean shutdown, minimal dependencies and multi-stage Dockerfile. No background queue, task statuses or extra CRUD endpoints.

Report commands/tests actually run and how integration should set up its DB.
