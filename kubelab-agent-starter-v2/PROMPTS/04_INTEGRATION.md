# Prompt 4 — Integration agent (run after merging 3 agents)

Read all root specs, `CONTRACT_FREEZE.md`, `ACCEPTANCE_TESTS.md` and inspect frontend/api/worker implementations. Own ONLY shared root files, Compose, `deploy/`, `scripts/`, and integration tests; request explicit approval before modifying another agent's files.

PHASE A: Create `compose.yaml`, `.env.example` (placeholders only), `.gitignore`, `Makefile`, and README. `docker compose up --build` should fail clearly if DB credentials aren't supplied. Keep DB credentials consistent across Worker/PostgreSQL. Route browser same-origin `/api/` calls to API. Verify create/list/delete, whoami, readiness, metrics and persistence using smoke tests. **Stop and report results before continuing to Kubernetes.**

PHASE B (only when instructed): k3d/K3s multi-node cluster, Deployments/Services, Traefik Ingress on localhost:8080, ConfigMap, operator-created Secret, PVC, optional NetworkPolicy exercise, Prometheus/Grafana via pinned Helm chart and ServiceMonitors, lightweight prebuilt dashboard, verification scripts. Keep all commands reproducible and document observed platform prerequisites. Treat NetworkPolicies and resource-heavy monitoring as staged steps. Do not put credentials in git.

At each phase report changed files, exact commands and results, skipped tests, blockers and how to clean up.
