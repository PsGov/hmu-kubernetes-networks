# KubeLab — Start here (multi-agent v2)

This folder contains **specifications, not application code**. The first action is to freeze interfaces; do not start three implementation agents before that review is done.

## 1. First prompt (paste into coordinator agent)

See `PROMPTS/00_COORDINATOR_FIRST.md`. The coordinator reviews consistency, freezes the API/DB/environment contracts and prepares independently implementable work, **without writing app code yet**.

## 2. Then run three implementation agents in parallel

Use `PROMPTS/01_FRONTEND.md`, `PROMPTS/02_API.md`, and `PROMPTS/03_WORKER.md` in separate Git worktrees or branches. Every agent reads `AGENTS.md`, `PRODUCT_SPEC.md`, and `API_CONTRACT.md`. They must follow the frozen contract and edit only their owned paths.

## 3. Integration

After reviewing and merging, use `PROMPTS/04_INTEGRATION.md`: make Compose work and pass smoke tests **before** attempting k3d/K3s, Ingress, PVC, NetworkPolicies or monitoring.

## Non-negotiable decisions

- Static frontend; Go HTTP API; Go synchronous HTTP Task Service called worker; PostgreSQL.
- Task create/list/delete (no update, no queue, no simulated asynchronous processing or status).
- API -> worker -> PostgreSQL; no database access or secrets in API.
- Credentials supplied by the operator via `.env` (ignored) for Compose and Kubernetes Secret for k3d. No hardcoded `postgres/toor` or other demo password.
- ConfigMap for non-secret application config; Secret for PostgreSQL username/password; PVC for PostgreSQL storage.
- Student lab should be completable by the experienced instructor in 2–3 hours after a clean setup.
- All work remains inspectable and repeatable; report commands/tests honestly.
