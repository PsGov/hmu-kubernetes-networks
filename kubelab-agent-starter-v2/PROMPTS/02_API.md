# Prompt 2 — API implementation agent

Read `AGENTS.md`, `PRODUCT_SPEC.md`, `API_CONTRACT.md`, `CONTRACT_FREEZE.md`. Implement **only** `api/`; do not change shared files or other service directories.

Use Go `net/http`. Implement the exact public routes and validation in `API_CONTRACT.md`. Forward task requests via required `WORKER_URL` to the corresponding Worker `/tasks` routes with context propagation, the frozen body limit and timeout, and the frozen 502/503 mapping. API `/readyz` must not call Worker. No SQL, database credentials or DB package imports. `/api/whoami` reads `POD_NAME`, `POD_NAMESPACE`, `NODE_NAME`, `POD_IP` env vars. Add bounded Prometheus metrics (templated routes; no UUID labels), logging, tests with mock upstream, and multi-stage Dockerfile.

Do not invent async processing or change task JSON fields. Report exact tests run and integration requirements.
