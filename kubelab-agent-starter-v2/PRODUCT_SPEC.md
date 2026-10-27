# KubeLab — Product & Architecture Specification (v1)

## 1. Goal
A tiny, reliable **Task List** web application **whose purpose is to teach Kubernetes**, not application development. A student can open a page, add a task, list tasks, and delete a task. The instructor provides built images. Students focus on nodes, Pods, Deployments, Services, CoreDNS, Ingress, ConfigMaps, Secrets, PVCs, NetworkPolicies, Prometheus and Grafana.

**Instructor time target:** once built and documented, an experienced K8s user should complete the student lab in **2–3 hours**, excluding initial image authoring. Students may take longer. Minimize moving parts, memory use, and manual typing.

## 2. Logical data path
```text
Browser
  |
  v
k3d host port 8080 -> Traefik Ingress
  |                           |
  +--- / -----------------> frontend-service -> frontend Pod (static UI)
  +--- /api/* ------------> api-service ------> API Pods (2 replicas)
                                                 |
                                                 | HTTP via worker-service DNS
                                                 v
                                          worker-service -> Worker Pod
                                                               |
                                                               | PostgreSQL TCP via postgres-service DNS
                                                               v
                                                        PostgreSQL + PVC

Prometheus scrapes API + Worker /metrics via Services/ServiceMonitors.
Grafana queries Prometheus. Nodes/Pods are inspected via kubectl or optional k9s.
```
**Important:** this is request/response HTTP; the Worker is an internal **Task Service**, not a background queue consumer. Name the workload `worker` for the lab, but explain its synchronous nature plainly. Any artificial processing delay must be opt-in and disabled by default.

## 3. Components
### Frontend
- Single static web page in Nginx, vanilla JS. Task input, add button, current list, delete action, success/error feedback, and a small area displaying API `/whoami` details (Pod / Node / IP) after a request.
- Calls relative `/api/...` URLs so the same origin works behind Ingress; no hard-coded localhost or cluster hostname in JS. Nginx should not require its own API reverse proxy.
- HTML-escape all displayed user content; no need for auth or SPA routing.

### API (Go, 2 replicas)
- Stateless HTTP façade. Implements public API contract; proxies task operations to Worker using `WORKER_URL` env var and explicit timeout. No database driver/DB secrets.
- `/api/whoami` reads `POD_NAME`, `POD_NAMESPACE`, `NODE_NAME`, `POD_IP` provided via Kubernetes Downward API (with safe local defaults). May include response header `X-KubeLab-Pod` to make replica demonstration easier.
- Has liveness, readiness, Prometheus metrics endpoints.
- Errors from an unavailable Worker should surface as `503`/`502` with concise JSON, not panic or hang. Do not make readiness depend on Worker (avoids cascading removal of healthy APIs from Service when Worker breaks).

### Worker / Task Service (Go, 1 replica)
- Synchronous HTTP server owning all task persistence. Only this service connects to PostgreSQL using env-supplied config and credentials.
- Simple SQL schema: `tasks(id UUID PRIMARY KEY, title TEXT NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT NOW())`. Generate UUID v4 in Go. Use parameterized queries; trim title and validate 1–120 Unicode code points as frozen in `API_CONTRACT.md`.
- On startup, use idempotent `CREATE TABLE IF NOT EXISTS` for v1; avoid requiring manual SQL on student machines.
- Connect with short bounded retry window on startup; readiness should verify DB connectivity using a bounded ping. Liveness should not require live DB access.
- Has metrics, health endpoints. Can expose internal `/whoami` as useful for debugging, but this is optional.

### PostgreSQL
- Official pinned container image, one replica, ClusterIP service and PVC (e.g., 1Gi); intended **only for local training**, never promoted as HA production storage.
- Student creates a Secret holding user-selected `POSTGRES_USER` and `POSTGRES_PASSWORD` values. Use a ConfigMap for `API_PORT=8080`, `WORKER_PORT=8080`, `DB_NAME=kubelab`, `DB_HOST=postgres-service`, `DB_PORT=5432` and `WORKER_URL=http://worker-service:8080`. Map `DB_NAME` to the PostgreSQL image's `POSTGRES_DB`.
- Database credentials are accessible only to Worker and PostgreSQL, never API or frontend.
- Keep demo password out of any git-tracked Secret manifest; provide `kubectl create secret ...` command or a generator script that prints command rather than checking in credentials.

## 4. Scope and non-goals
**Required:** create/list/delete tasks; stable HTTP error behavior; minimal UI; health/readiness; metrics; Compose local development; 3 images; k3d examples; routing; DNS; PVC demo; one NetworkPolicy exercise; prebuilt Grafana dashboard; README & smoke test.

**Not required:** UPDATE, user registration/login, TLS/cert-manager, OAuth, queues, concurrency control beyond ordinary SQL, message brokers, custom schedulers, HPA, cluster-wide dashboards, advanced tracing, logging stack, cloud or public domain. HPA is strictly optional bonus after all required items work.

## 5. Safety & educational expectations
- Explain that a local HTTP port and demo credentials are for an isolated educational environment only; no prescribed or hardcoded password.
- Secrets are only base64-encoded in standard Kubernetes manifests (not encrypted by default). Explain ConfigMap vs Secret and RBAC/etcd limits at a high level.
- Namespace `kubelab` for workloads; namespace `monitoring` for observability. Label pods/services consistently (`app.kubernetes.io/name`, `app.kubernetes.io/part-of: kubelab`).
- Test on 1 k3d server + 2 k3d agents, but don't assert the scheduler always spreads Pods across nodes; show placement with `kubectl get pods -o wide`.
- Keep an optional k9s recommendation; all assessed steps can be completed with `kubectl`.

## 6. Instructor deliverables
- Repo with working source, Dockerfiles, Compose, manifests, scripts, docs.
- Multiarch or documented platform-specific image builds and pushed immutable tags on a chosen registry (do not invent registry account/repo names; leave documented placeholders).
- Student handout with progressively staged tasks, hints, 2–4 sentence explanations, screenshots and commands (in spirit of last year's assignment).
- Author answer key / test checklist separate from student templates.
