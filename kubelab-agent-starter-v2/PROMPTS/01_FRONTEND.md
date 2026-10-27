# Prompt 1 — Frontend implementation agent

Read `AGENTS.md`, `PRODUCT_SPEC.md`, `API_CONTRACT.md`, `CONTRACT_FREEZE.md`. The contract has been frozen. Implement **only** `frontend/`; do not change shared docs/contracts, `api/`, `worker/`, or root/deployment files.

Create a simple static HTML/CSS/vanilla JS Task List UI that fetches same-origin `GET /api/tasks`, posts `POST /api/tasks`, deletes `DELETE /api/tasks/{id}`, and displays `GET /api/whoami` (Pod/Node info) clearly. Follow the frozen JSON and error shapes in `API_CONTRACT.md`; treat DELETE `204` as an empty response and show loading, empty, and error states. Render user text safely; no UI framework, login, or task status. Include Nginx Dockerfile and configuration for SPA-less serving; API requests go via Ingress in Kubernetes or an integration-owned local reverse proxy in Compose. Include focused tests if practical and a concise `frontend/README.md`.

Report files changed, actual commands/tests run, failures, and exact expectations for integration.
