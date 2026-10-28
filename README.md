# KubeLab on k3d

This lab runs a static frontend, two API Pods, a synchronous Worker task service, and PostgreSQL on a local K3s cluster. Browser traffic enters through Traefik at <http://localhost:8080/>. The API calls `worker-service` over HTTP; only Worker connects to `postgres-service`. PostgreSQL uses one local PersistentVolumeClaim. It is suitable for a local exercise, not a highly available database.

## Prerequisites

- Docker Engine, k3d, kubectl, `make`, and `curl`. Docker needs to be running, with enough free memory for three k3d nodes plus the four workloads. Available memory varies by host, so check Docker's allocation if Pods stay Pending or are OOMKilled.
- Host port 8080 free.
- Docker image builds run for the host's native `linux/amd64` or `linux/arm64` architecture. All three Dockerfiles use multi-architecture base images. Build and import separately on each architecture. The PostgreSQL manifest uses the official versioned `postgres:16.14-alpine3.24` image. Application images use local tag `0.1.0`.

## Quick start

Run these commands from the repository root. Choose your own PostgreSQL username and password in the shell; never commit them. The two variables must be exported because `make deploy` reads the environment.

```sh
export POSTGRES_USER='your-selected-user'
export POSTGRES_PASSWORD='your-selected-password'
make build
make cluster-up
make load-images
make deploy
make smoke
```

`make build` runs three `docker build -t kubelab-{frontend,api,worker}:0.1.0 ...` commands. `make cluster-up` runs `k3d cluster create kubelab --servers 1 --agents 2 -p '8080:80@loadbalancer'`. `make load-images` runs `k3d image import` for the three local application tags. `make deploy` applies the namespace and ConfigMap, creates or updates a Kubernetes Secret from the exported variables, applies the workloads, then waits for rollouts. `make smoke` checks the frontend, calls `/api/whoami` and `/api/tasks` through Ingress, creates a `k3d-smoke` task, lists tasks again, and shows the Kubernetes objects. `make cluster-down` runs only `k3d cluster delete kubelab`.

For an explicit first-time install, these are the underlying Kubernetes commands:

```sh
kubectl get nodes -o wide
kubectl get ingressclass
kubectl get storageclass
kubectl apply -f deploy/k8s/base/namespace.yaml
kubectl apply -f deploy/k8s/base/configmap.yaml
kubectl -n kubelab create secret generic postgres-credentials \
  --from-literal=POSTGRES_USER="$POSTGRES_USER" \
  --from-literal=POSTGRES_PASSWORD="$POSTGRES_PASSWORD"
kubectl apply -f deploy/k8s/base/workloads.yaml
kubectl -n kubelab get pods,svc,ingress,pvc -o wide
```

The cluster should show one server and two agents. Check that `traefik` is an IngressClass and that a default StorageClass exists (normally `local-path` in k3d). The manifests rely on those k3d defaults. The `postgres-credentials` Secret has exactly `POSTGRES_USER` and `POSTGRES_PASSWORD`; only PostgreSQL and Worker reference it. Kubernetes Secrets are base64 encoded in manifests, not encrypted by default. Limit who can read them with appropriate access controls in any shared cluster.

## Check the application

```sh
curl -fsS http://localhost:8080/
curl -fsS http://localhost:8080/api/whoami
curl -fsS http://localhost:8080/api/tasks
curl -fsS -X POST -H 'Content-Type: application/json' \
  -d '{"title":"Learn Kubernetes Services"}' http://localhost:8080/api/tasks
curl -fsS http://localhost:8080/api/tasks
kubectl -n kubelab get pods -o wide
kubectl -n kubelab get endpointslices
```

`/api/whoami` reports the API Pod name, namespace, node and IP from the Downward API. API replicas may land on the same node; placement is visible with `kubectl get pods -o wide`. The `/api` Ingress path has priority over `/`, so browser requests reach the API and static page respectively. Worker and PostgreSQL have ClusterIP Services only and no Ingress path.

To demonstrate persistence, create a task, then delete the PostgreSQL Pod and wait for the Deployment to replace it:

```sh
kubectl -n kubelab delete pod -l app.kubernetes.io/name=postgres
kubectl -n kubelab rollout status deployment/postgres --timeout=180s
kubectl -n kubelab rollout status deployment/worker --timeout=180s
kubectl -n kubelab get pvc postgres-data
curl -fsS http://localhost:8080/api/tasks
```

The PVC should remain `Bound` and the task should still be listed. The volume is local to one k3d node; do not use node deletion as the persistence test. Delete one API Pod with `kubectl -n kubelab delete pod -l app.kubernetes.io/name=api` and observe replacement Pods with `kubectl -n kubelab get pods -w`.

## Configuration and image updates

The ConfigMap holds ports, Worker URL, and DB host/name/port. Environment variables are read when a Pod starts. After editing `deploy/k8s/base/configmap.yaml`, run `kubectl apply -f deploy/k8s/base/configmap.yaml`, then `kubectl -n kubelab rollout restart deployment/api deployment/worker` (and `deployment/postgres` if its database name changes). Changing PostgreSQL credentials in the Secret does **not** change credentials in an initialized database. Change them in PostgreSQL separately, or intentionally recreate disposable lab storage.

Rebuild images with `make build`, import them with `make load-images`, then run `kubectl -n kubelab rollout restart deployment/frontend deployment/api deployment/worker`. The images use `IfNotPresent`; for a clean experiment, prefer incrementing the versioned image tag in both Makefile and manifests. For a registry-based class, replace each `kubelab-*:0.1.0` reference with your own immutable `REGISTRY/IMAGE:TAG`, build and push for both `linux/amd64` and `linux/arm64` with Docker Buildx, and verify that students can pull the images without your credentials. No registry name is prescribed here.

## Troubleshooting and cleanup

If an application Pod shows `ImagePullBackOff`, run `make load-images`, confirm the image tag matches the manifest, and restart its Deployment. If the PVC is Pending, inspect `kubectl get storageclass` and `kubectl -n kubelab describe pvc postgres-data`. If Ingress fails, inspect `kubectl get ingressclass`, `kubectl -n kube-system get pods`, and `kubectl -n kubelab describe ingress kubelab`. For failed rollouts, use `kubectl -n kubelab describe pod POD_NAME` and `kubectl -n kubelab logs deployment/worker` or `deployment/postgres`.

`make cluster-down` deletes only the `kubelab` k3d cluster, including its local lab storage. It does not remove images from Docker or touch other clusters.

Monitoring and NetworkPolicies are deferred to separate work.
