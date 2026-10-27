# KubeLab — Kubernetes / k3d / Observability Specification (v1)

## 1. Tooling
- Local Docker engine, `k3d`, `kubectl`, `helm`; **k9s optional** as visualization tool, not a prerequisite.
- One k3d server and **two agents**; expose host port `8080` to cluster load balancer port `80`.
- Example author command (verify with installed k3d version):
```bash
k3d cluster create kubelab --servers 1 --agents 2 -p '8080:80@loadbalancer'
```
- K3s commonly includes Traefik Ingress, CoreDNS, and local-path storage; validate actual cluster defaults rather than assuming add-ons. The K3s built-in NetworkPolicy controller can implement standard Kubernetes NetworkPolicies; do not also install Calico unnecessarily.
- For beginner simplicity: **Ingress path-based routing** on `http://localhost:8080/` (no host file / DNS required). `/api` to API service, `/` to frontend service. Choose Ingress paths carefully so `/api` wins; use `pathType: Prefix`. Demonstrate host-based routing as an optional explanation only.

## 2. Namespaces / objects
- `kubelab`: API Deployment (replicas 2), Worker Deployment (replicas 1), frontend Deployment (replicas 1), PostgreSQL Deployment (replicas 1) or StatefulSet if a small extra complexity is justified; ClusterIP Services for all four; Ingress; PVC; ConfigMap; Secret generated separately; NetworkPolicy(s).
- `monitoring`: Helm-managed Prometheus/Grafana, appropriate metrics discovery.
- Provide resource requests and limits suitable for laptops; document host RAM guidance rather than guaranteeing one number on all OSes.
- Use startup/readiness/liveness probes for API/Worker/Postgres where appropriate. Probes must not make failure experiments impossible.

## 3. ConfigMap + Secret wiring
ConfigMap data (non-sensitive):
```yaml
API_PORT: "8080"
WORKER_PORT: "8080"
WORKER_URL: "http://worker-service:8080"
DB_HOST: "postgres-service"
DB_PORT: "5432"
DB_NAME: "kubelab"
```
Student creates Secret using (local demo only):
```bash
kubectl -n kubelab create secret generic postgres-credentials \
  --from-literal=POSTGRES_USER="$POSTGRES_USER" \
  --from-literal=POSTGRES_PASSWORD="$POSTGRES_PASSWORD"
```
- PostgreSQL image expects `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB`; set `POSTGRES_DB` from the `DB_NAME` ConfigMap key.
- Worker expects `WORKER_PORT`, `DB_HOST`, `DB_PORT`, `DB_NAME`, `POSTGRES_USER`, `POSTGRES_PASSWORD`; API expects `WORKER_URL`, `API_PORT` and the four Downward API identity variables. There are no DB aliases or credentials in API.
- Read env variables using `configMapKeyRef` / `secretKeyRef`. Show a restart rollout after changing environment-backed ConfigMap data.

## 4. Persistent storage
- PVC for PostgreSQL using local-path default storage class if available, 1Gi `ReadWriteOnce`.
- This is **single-node local persistence**, not portable multi-node HA: a PVC bound on one node may restrict rescheduling. The student experiment **deletes the DB Pod** and confirms data persistence; **do not** require draining/deleting the volume's node.
- Provide a deterministic command or UI step to create a task before DB Pod deletion and verify afterward.

## 5. Networking & NetworkPolicy
- Services named `frontend-service`, `api-service`, `worker-service`, `postgres-service`; `ClusterIP` throughout.
- API -> Worker: `http://worker-service:8080`.
- Worker -> DB: `postgres-service:5432`.
- Traefik is the only HTTP entry point from host. No `LoadBalancer` frontend Service required.
- Default exercise: deploy before policy and demonstrate permitted connectivity; then apply namespace-scoped policy allowing database TCP/5432 ingress only from Pods labeled `app.kubernetes.io/name: worker`; other in-namespace Pods should be denied. Use `kubectl run` test Pods or a temporary test Deployment to show allowed vs denied behavior.
- Standard NetworkPolicy enforces Pod traffic, not user identities. Watch namespace selectors and labels. Avoid accidentally blocking Prometheus scrapes or DNS if introducing general namespace-wide default-deny policies. Start with DB-specific ingress isolation.

## 6. Monitoring
- Use Helm to install a **pinned, validated** version of `prometheus-community/kube-prometheus-stack` with lightweight `values.yaml` overrides, or a smaller Prometheus+Grafana installation if the stack cannot fit realistic student machines. State the chosen tradeoff in README. Do not assume Grafana's default password is stable: retrieve from Secret/document exact deployment setup.
- Expose `/metrics` on API and Worker. Use `ServiceMonitor` resources compatible with the installed operator and appropriate namespace/label selection. Verify both targets **UP** in Prometheus; simply deploying `/metrics` is insufficient.
- Provision a basic Grafana dashboard from source-controlled JSON (request rate, error count/rate, latency; optionally Pod CPU/memory); distinguish sample charts from live results.
- Access Grafana by `kubectl port-forward` to service, but this is only for tooling; the teaching application's entry point uses Ingress. Do not publish Grafana externally by default.
- Provide short load test script that generates requests through the **Ingress**, with bounded concurrency. Expect some request/CPU changes; do not promise a specific CPU increase.

## 7. Student exercise outline (target: instructor 2–3h)
1. Create cluster; `kubectl get nodes -o wide`; optionally inspect via k9s.
2. Create namespace, ConfigMap, Secret; deploy ready images with TODO templates; inspect Pods/replicas.
3. Delete API Pod and observe self-healing; call `/api/whoami` repeatedly and inspect Pod/Node identities (do not guarantee round-robin each request due to connection reuse).
4. Verify API -> Worker by Kubernetes DNS; inspect Worker logs, Service endpoints and Pod IP changes.
5. Create/use Ingress on localhost:8080; test frontend and `/api/tasks` routes.
6. Create a task; delete database Pod; observe persisted task after database becomes ready.
7. Apply DB isolation NetworkPolicy and compare allowed and denied connectivity from test Pods.
8. Helm monitoring, check Prometheus targets, generate traffic, capture Grafana panels.

For each section ask: relevant command, screenshot, and **2–4 sentences explaining the observed behavior**. Provide reference instructor manifests and intentionally incomplete **student** templates separately. HPA is optional bonus only.

## 8. Author convenience commands
Implement and document targets such as `make build`, `make compose-up`, `make cluster-up`, `make load-images` (for local images), `make deploy`, `make monitoring-up`, `make smoke`, `make cluster-down`. Avoid hidden dangerous cleanup (deleting unrelated clusters, global docker prune, deleting external PVCs). Every target must mention explicit underlying commands in docs.

## 9. Image distribution
- In development, `k3d image import` supports running locally built images without needing a registry.
- For students, publish **versioned** multi-platform images to a registry selected by instructor (use placeholder `REGISTRY/IMAGE:TAG` until configured). Verify pull permissions on an unauthenticated student machine. Do not ship only `latest`.


## Configurable-credential requirement (v2)
The `postgres-credentials` Secret is created by the student/operator using arbitrary selected values for `POSTGRES_USER` and `POSTGRES_PASSWORD`. Commands may reference shell variables but must not embed literals such as `postgres/toor`. PostgreSQL and Worker read the exact same Secret keys. API receives no DB credentials. DB name/host/port remain in ConfigMap. Use a fresh PVC for first-time initialization, and document that changing Secret values does not reinitialize an existing PostgreSQL data directory. Never check generated Secret manifests into Git.
