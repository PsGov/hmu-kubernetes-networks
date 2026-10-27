# KubeLab — Agent Instructions

You are implementing **KubeLab**, a deliberately small educational application used to teach Kubernetes networking, deployments, configuration, persistence, and observability. The owner is a Go backend engineer who will personally reproduce and verify the entire lab. **Optimize for reproducibility and teachability over features.**

## Mandatory reading order
1. `PRODUCT_SPEC.md` — purpose, boundaries, flows.
2. `API_CONTRACT.md` — exact endpoints and error behavior.
3. `KUBERNETES_SPEC.md` — k3d/K3s deployment and lab exercises.
4. `ACCEPTANCE_TESTS.md` — objective completion criteria.

## Engineering rules
- Build three application images: `frontend`, `api`, `worker`. Use the official PostgreSQL image; never build a custom database image.
- Use **Go standard library `net/http`** for API and worker. Prefer `database/sql` with `pgx` stdlib driver; keep the dependency footprint small. Frontend is static HTML/CSS/vanilla JS served by Nginx.
- API **must not** connect to PostgreSQL. Worker is the only application service with database access. API calls Worker over HTTP using Kubernetes Service DNS.
- No authentication, Redis, brokers, background job queue, cloud infrastructure, paid services, CI/CD, or frontend framework.
- Container images and source must run on both arm64 and amd64, or document the build/publish steps for both architectures.
- Use pinned non-`latest` versions for dependencies, Docker base images, Helm chart (lock in values/doc), and sample deployment images. Never commit real secrets; PostgreSQL username/password are user-selected and supplied via environment variables locally and a Kubernetes Secret in the cluster. Never embed credentials or defaults in code, YAML, or tracked files.
- Every web endpoint must have bounded timeouts; `GET /healthz` and `GET /readyz` must be independent of the business path where appropriate (see API contract). Use structured but human-readable logs to stdout; do not log secrets.
- Instrument API and Worker with Prometheus HTTP request metrics; avoid labels containing task IDs, paths with IDs, pod IPs or other unbounded values.
- Use clean, readable, modestly tested code; **do not** add generic abstractions/hexagonal architecture just for style.
- Implement and verify incrementally. Start with Compose, then k3d, then observability, then exercises. Do not claim tests passed if they have not been run.
- Avoid `make` commands that hide all educational Kubernetes interactions. Convenience targets are for authors; student exercises must show underlying `kubectl`, `helm`, and `k3d` commands.
- Before introducing a nontrivial dependency or architectural change, explain the reason in a short decision note or PR summary. Do not enlarge the scope on your own.

## Expected repository structure
```text
kubelab/
  AGENTS.md
  docs/                         # these specifications + student lab guide
  frontend/                     # static assets and Dockerfile
  api/                          # Go service and Dockerfile
  worker/                       # Go service, migrations and Dockerfile
  deploy/k8s/base/              # templates/manifests for authors
  deploy/k8s/student/           # student TODO templates
  deploy/monitoring/            # pinned Helm values, dashboard JSON, ServiceMonitor
  scripts/                      # smoke tests, small HTTP traffic generator
  compose.yaml
  Makefile
  README.md
```

## Suggested phases and checkpoints
1. **App:** Compose up -> create/list/delete task -> refresh -> persistent data -> whoami/health/metrics work.
2. **Cluster:** k3d up -> deploy -> browser access -> internal DNS -> replicas/self-healing -> PVC restart.
3. **Networking:** default allow, then selective NetworkPolicies, and verification from allowed/denied test Pods.
4. **Observability:** Helm install Prometheus/Grafana -> targets UP -> dashboard sees request/latency/error data.
5. **Education:** exact student instructions with commands, TODO manifests, expected outputs, troubleshooting, cleanup.

## Definition of done
All items in `ACCEPTANCE_TESTS.md` pass on a clean machine (or are clearly and truthfully marked unverified), and the owner can run the complete student path with the provided instructions.

## Multi-agent collaboration rules (v2)
- Coordinator MUST first freeze endpoint paths, JSON schemas, error responses, environment variable names, and SQL schema in `API_CONTRACT.md` and `CONTRACT_FREEZE.md` before implementation starts. If changes are required, coordinator edits the specs and announces the change to all agents.
- Frontend agent owns `frontend/` only; API agent owns `api/` only; Worker agent owns `worker/` only. Integration agent owns `compose.yaml`, `.env.example`, `Makefile`, `deploy/`, `scripts/`, root README and cross-service tests. No concurrent edits to shared files.
- Never commit `.env`, kubeconfig, secrets, actual passwords, or generated credentials. `.env.example` must contain placeholders and descriptive instructions, **not live values**. Require user-provided credentials for local Compose.
- API/worker follow synchronous HTTP create/list/delete as the contract currently says. Do not invent a background queue, `pending/completed` statuses, JWT, Redis or additional routes.
- Database credentials are not hardcoded. The PostgreSQL image initialization and Worker connection must use the SAME user-selected credentials. Document that changing initialized PostgreSQL credentials requires a DB operation, not merely a Secret update.
- In worktrees, do not merge or cherry-pick another agent's changes without explicit integration coordination.
