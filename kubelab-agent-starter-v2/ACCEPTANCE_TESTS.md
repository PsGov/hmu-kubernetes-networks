# KubeLab — Acceptance Checklist (v1)

This is the agent's Definition of Done and the instructor's reproducibility test. Mark each as PASS / FAIL / NOT RUN and attach a command/output or screenshot. **Never fabricate evidence.**

## A. Local application
- [ ] After supplying operator-chosen `POSTGRES_USER` and `POSTGRES_PASSWORD` in ignored `.env`, `docker compose up --build` starts frontend, API, Worker and PostgreSQL; without them, Compose fails clearly.
- [ ] Add task from browser; task appears in list; refreshing browser preserves it.
- [ ] Delete task; it disappears and remains gone after refresh.
- [ ] Input validation: blank title / more than 120 Unicode code points / malformed JSON / invalid UUID returns `400` JSON; unknown valid UUID deletion returns `404`.
- [ ] API has **no database connection settings/credentials**; Worker can talk to PostgreSQL.
- [ ] `GET /api/whoami` works locally with safe defaults; `/healthz`, `/readyz`, `/metrics` work.
- [ ] API and Worker fail quickly and cleanly if downstream is unavailable; no hangs or raw stack traces to clients.
- [ ] API returns `503` for unreachable Worker and `502` for malformed Worker replies; API `/readyz` remains independent of Worker. Worker `/readyz` returns `503` when DB is unavailable while Worker `/healthz` remains live.

## B. Kubernetes fundamentals
- [ ] Fresh `k3d cluster create ...` creates server and two agents; `kubectl get nodes` shows Ready nodes.
- [ ] Application deploys in `kubelab` namespace with expected labels, ConfigMap, Secret references, and 2 API replicas.
- [ ] `http://localhost:8080/` loads UI; `/api/tasks` routes to API; Worker and PostgreSQL cannot be reached from host via application Ingress.
- [ ] API calls Worker through `worker-service` DNS and Worker calls `postgres-service` DNS.
- [ ] Calling `/api/whoami` shows actual Pod name / Node / IP; `kubectl get pods -o wide` confirms placement.
- [ ] Deleting one API Pod leads to a replacement Pod without manually recreating Deployment.
- [ ] ConfigMap env change requires appropriate rollout/restart; Secret is not committed to git.

## C. Persistence / network isolation
- [ ] Task survives **PostgreSQL Pod deletion and recreation** with same PVC.
- [ ] PVC stays `Bound`, workload can mount it, and lab does not depend on cross-node volume mobility.
- [ ] Before NetworkPolicy, in-namespace test Pod can connect to PostgreSQL service TCP port.
- [ ] After database ingress policy, test Pod cannot connect while Worker -> PostgreSQL task operations still work.
- [ ] DNS lookups and metrics collection are not broken by the chosen policy.

## D. Observability
- [ ] Helm monitoring installation succeeds with recorded chart version and values.
- [ ] Prometheus scrape targets for **both** API and Worker show UP.
- [ ] Grafana dashboard loads from provisioned JSON; request rate/error/latency graphs reflect controlled traffic.
- [ ] A failed Worker produces observable API HTTP failures and metric changes; restoring Worker restores task operations.
- [ ] Document actual memory footprint / startup caveats on test host; avoid unsupported laptop requirements claims.

## E. Documentation and handoff
- [ ] README includes prerequisites, supported architectures, pinned versions, quick start, cleanup, troubleshooting.
- [ ] Student guide is doable with only `kubectl`, `helm`, `k3d`, Docker (k9s optional).
- [ ] Student guide includes commands, expected observations, screenshot prompts, and 2–4 sentence explanation questions, in line with last year's assignment style.
- [ ] All manifests required for the instructor are available; student starter manifests have explicit TODOs and an answer key exists separately.
- [ ] A complete, clean-environment **instructor walkthrough** takes approximately 2–3 hours or less; if not, reduce lab scope rather than hiding steps.
- [ ] `make cluster-down` affects only the `kubelab` cluster; no global destructive commands.
- [ ] Provide exact git commit, container image tags, and test results for reproducibility.

## Example smoke commands (agent must adjust and make functional)
```bash
curl -fsS http://localhost:8080/api/whoami
curl -fsS http://localhost:8080/api/tasks
curl -fsS -X POST -H 'Content-Type: application/json' \
  -d '{"title":"test-networking"}' http://localhost:8080/api/tasks
kubectl -n kubelab get deploy,po,svc,ingress,pvc -o wide
kubectl -n kubelab get endpointslices
```

## Explicit failure scenarios to demonstrate
1. Delete one API Pod -> Deployment heals it.
2. Delete database Pod -> data remains when same PVC is reused.
3. Apply DB isolation policy -> unauthorized Pod blocked, Worker permitted.
4. Scale Worker to zero temporarily -> API returns controlled error, Prometheus records changes; restore Worker to one.

## Deferred / bonus only
HPA, TLS, real domains, user auth, multi-node durable storage, OpenTelemetry tracing, message queues, CI/CD, performance benchmarking.
