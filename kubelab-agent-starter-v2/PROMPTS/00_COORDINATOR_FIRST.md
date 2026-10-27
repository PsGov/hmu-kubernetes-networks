# Prompt 0 — Coordinator / first prompt to paste

You are the KubeLab lead engineer. Read `AGENTS.md`, `PRODUCT_SPEC.md`, `API_CONTRACT.md`, `KUBERNETES_SPEC.md`, `ACCEPTANCE_TESTS.md`, and `CONTRACT_FREEZE.md` thoroughly.

**This first phase is CONTRACT REVIEW ONLY. Do not implement application code, Dockerfiles, Compose, Kubernetes manifests or monitoring yet.**

1. Identify any contradictory or underspecified behavior among the files and resolve it with the smallest possible changes. Keep scope: static frontend, Go API, synchronous Go HTTP Worker/Task Service, PostgreSQL; create/list/delete only. No background processing, queue, authentication or task status.
2. Freeze the exact HTTP/JSON contract, DB table schema, env var names, error handling, readiness behavior, and ownership boundaries. Prefer existing `API_CONTRACT.md` rather than designing a different API.
3. Require configurable PostgreSQL credentials provided by the operator. Never hardcode `postgres/toor` (or any other username/password) in code/config/repo. Specify ignored `.env` for Compose and `postgres-credentials` Secret for Kubernetes.
4. Update `CONTRACT_FREEZE.md` and the relevant specs only if needed, making all documents mutually consistent.
5. Produce a brief implementable handoff for each agent: frontend owns `frontend/`, API owns `api/`, worker owns `worker/`. Shared root/deployment files belong to a later integration agent.
6. End with: files changed, contract decisions, open risks, test plan, and clear GO/NO-GO for launching the three agents. Do not falsely claim tests passed.

Important: I am the instructor preparing a Kubernetes networking lab. Simplicity and reproducible 2–3 hour instructor walkthrough matter more than sophistication.
