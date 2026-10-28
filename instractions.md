# KubeLab — Kubernetes Networking and Observability Laboratory

## 1. Project Overview

**KubeLab** is a practical laboratory environment designed to help students understand how Kubernetes manages containerized applications, networking, persistent storage, service discovery, scaling, and monitoring.

The goal is to provide students with a realistic but relatively simple application that they can deploy, explore, modify, and troubleshoot.

Instead of studying Kubernetes resources independently, students interact with an application consisting of multiple communicating services.

The laboratory runs locally using Docker, K3s, and k3d, so no cloud infrastructure is required.

The initial implementation focuses on:

- Kubernetes Pods, Deployments, and replicas.
- Kubernetes Services and internal DNS.
- Ingress and HTTP routing.
- ConfigMaps and Secrets.
- PersistentVolumes and PersistentVolumeClaims.
- Load balancing and application availability.
- Pod recovery and Kubernetes reconciliation.
- Prometheus and Grafana for observability.

The application is designed to support a practical laboratory session of approximately 2–3 hours, depending on the exercises selected.

---

## 2. Application Architecture

We developed a small task-management application consisting of four main components.

### Frontend

A static web interface built with HTML, CSS, and JavaScript, served using Nginx.

It allows users to create, view, and delete tasks and interact with the backend through HTTP requests.

### API Service

A backend service developed in Go.

Its responsibilities include:

- Exposing REST API endpoints.
- Receiving requests from the frontend.
- Communicating with the internal Worker service.
- Providing health and readiness endpoints.
- Exposing Pod and Node information through `/api/whoami`.
- Providing a `/metrics` endpoint for observability.

The API is deployed with two replicas to demonstrate Kubernetes load balancing and self-healing.

### Worker Service

A second Go service responsible for processing task operations and communicating with PostgreSQL.

The API does not access the database directly. Instead, it communicates with the Worker through its internal Kubernetes Service.

This separation allows students to examine communication between different application components.

### PostgreSQL

PostgreSQL provides persistent storage for the tasks.

It uses a Kubernetes PersistentVolumeClaim so that application data is not lost when the PostgreSQL Pod is deleted and recreated.

### Communication Flow

```text
                  Browser
                     |
                     v
              Traefik Ingress
                     |
           +---------+---------+
           |                   |
           v                   v
    Frontend Service       API Service
           |                   |
           v                   v
     Frontend Pod          API Pods (2)
                               |
                               v
                         Worker Service
                               |
                               v
                          Worker Pod
                               |
                               v
                       PostgreSQL Service
                               |
                               v
                        PostgreSQL Pod
                               |
                               v
                         PersistentVolume
```

The Frontend and API are accessible through the Ingress, while the Worker and PostgreSQL communicate internally through Kubernetes Services.

---

## 3. Initial Development and Docker Testing

Before deploying to Kubernetes, we developed the application components independently.

We used Codex CLI agents working in separate Git branches/worktrees to implement the Frontend, API, and Worker services.

The components were later merged into the main branch.

Each service was packaged into its own Docker image:

- `kubelab-frontend`
- `kubelab-api`
- `kubelab-worker`

PostgreSQL uses an official PostgreSQL Docker image.

We initially tested communication using a Docker network named `kubelab-net`.

This allowed us to verify the application functionality without introducing Kubernetes-related complexity.

During integration, we resolved several issues involving Nginx configuration, JavaScript MIME types, and communication between Docker containers.

Once the application worked correctly in Docker, we proceeded to Kubernetes.

---

## 4. Creating the Kubernetes Cluster

We used **k3d**, which runs K3s Kubernetes nodes as Docker containers.

The cluster was named `kubelab` and configured with:

- One K3s server/control-plane node.
- Two K3s agent nodes.
- A load balancer exposing HTTP on host port `8085`.

The cluster creation configuration was equivalent to:

```bash
k3d cluster create kubelab \
  --servers 1 \
  --agents 2 \
  -p "8085:80@loadbalancer"
```

We verified the cluster using:

```bash
kubectl get nodes
```

All three nodes were reported as `Ready`.

We also checked the available IngressClass and StorageClass:

```bash
kubectl get ingressclass
kubectl get storageclass
```

The cluster included:

- **Traefik** as its Ingress controller.
- **local-path** as its default StorageClass.

Traefik handles HTTP routing into the application, while local-path provides storage for PostgreSQL.

---

## 5. Creating the Kubernetes Resources

We created Kubernetes manifests under:

```text
deploy/k8s/base/
```

The main resources include:

### Namespace

A dedicated namespace called `kubelab` contains the application resources.

This keeps the application logically separated from Kubernetes system components and the monitoring stack.

### ConfigMap

A ConfigMap named `kubelab-config` defines non-sensitive settings, including:

- Application ports.
- Worker service URL.
- PostgreSQL Service hostname.
- Database port.
- Database name.

For example, the API communicates with the Worker through:

```text
http://worker-service:8080
```

### Secret

A Kubernetes Secret named `postgres-credentials` stores the PostgreSQL username and password.

The credentials are injected into the PostgreSQL and Worker containers as environment variables.

### Deployments

We created four Deployments:

| Deployment | Replicas |
|---|---:|
| Frontend | 1 |
| API | 2 |
| Worker | 1 |
| PostgreSQL | 1 |

The Deployments define the required container images, environment variables, resource requests and limits, and health probes.

### Services

Each component has a ClusterIP Service:

- `frontend-service`
- `api-service`
- `worker-service`
- `postgres-service`

These provide stable internal DNS names and allow communication without relying on individual Pod IP addresses.

### Ingress

We created an Ingress resource using Traefik.

It defines two main routing rules:

```text
/       → frontend-service
/api    → api-service
```

Consequently, the user can access both the frontend and API through the same entry point:

```text
http://localhost:8085
```

### PersistentVolumeClaim

PostgreSQL uses a PVC named `postgres-data`, requesting 1 GiB of storage.

Kubernetes provisions storage using the default `local-path` StorageClass.

This separates the database data lifecycle from the PostgreSQL Pod lifecycle.

---

## 6. Building and Deploying the Application

We created a Makefile to simplify the deployment procedure.

The main commands are:

```bash
make build
make load-images
make deploy
make smoke
```

`make build` builds the application Docker images.

`make load-images` imports them into the k3d cluster.

`make deploy` applies the Kubernetes manifests, creates the PostgreSQL Secret from environment variables, and waits for the application Deployments to become ready.

`make smoke` tests the application through Traefik.

During the initial Kubernetes deployment, the frontend failed because its Nginx configuration still referenced a container hostname used in the earlier Docker environment.

We corrected the frontend configuration, rebuilt its image as `kubelab-frontend:0.1.1`, imported it into k3d, and updated the Kubernetes Deployment.

After this change, all five application Pods became healthy.

---

## 7. Functional Testing

We verified the deployed application through a smoke test.

The test performed the following operations:

1. Accessed the frontend through Traefik.
2. Called the `/api/whoami` endpoint.
3. Retrieved the existing tasks.
4. Created a new task.
5. Retrieved the tasks again to confirm that the new task had been stored.

All of these operations succeeded.

This confirmed that communication worked across the full application:

```text
Ingress → API → Worker → PostgreSQL
```

---

## 8. Testing Load Balancing

To demonstrate load balancing, we deployed the API with two replicas.

We then executed multiple requests to:

```text
http://localhost:8085/api/whoami
```

The endpoint returns the identity of the API Pod processing each request, including:

- Pod name.
- Namespace.
- Node name.
- Pod IP address.

During testing, requests were served by two different API Pods located on different Kubernetes nodes.

This demonstrated that the API Service could distribute traffic between multiple healthy replicas.

---

## 9. Testing Self-Healing

We manually deleted one of the API Pods:

```bash
kubectl delete pod <api-pod-name>
```

We monitored the Pods using:

```bash
kubectl get pods -w
```

Kubernetes automatically created a replacement Pod, which subsequently became Ready.

The API Deployment returned to its desired replica count of two.

This demonstrated Kubernetes' reconciliation mechanism: the system attempts to maintain the desired state defined by the Deployment.

---

## 10. Testing PostgreSQL Persistence

After creating tasks in the application, we connected to PostgreSQL using `kubectl exec` and the `psql` command-line client.

We inspected the database tables and confirmed that application tasks had been stored.

Next, we deleted the PostgreSQL Pod manually.

Kubernetes created a replacement Pod, which mounted the existing persistent storage.

After PostgreSQL became Ready, we verified that the previously created tasks were still available.

This demonstrated that deleting a Pod does not necessarily delete its persistent data when a PVC is used.

The experiment also illustrated the relationship between:

- PersistentVolumeClaim.
- PersistentVolume.
- StorageClass.
- Pod scheduling and storage mounting.

---

## 11. Adding Monitoring with Prometheus and Grafana

After validating the application, we proceeded with the observability infrastructure.

We chose the `kube-prometheus-stack` Helm chart, which provides a collection of Kubernetes monitoring components.

The monitoring stack is installed in a separate namespace named `monitoring`.

Its components include:

- **Prometheus:** Collects and stores metrics.
- **Grafana:** Provides dashboards and visualizations.
- **Prometheus Operator:** Manages Prometheus instances and related Kubernetes resources.
- **kube-state-metrics:** Exposes information about Kubernetes resource states.
- **Node Exporter:** Provides node-level system metrics.
- **Alertmanager:** Supports alert management.

The purpose of the monitoring environment is to allow students to observe resource utilization and Kubernetes workload behavior.

During installation, the Helm process was interrupted, which initially left the release marked as failed. However, Prometheus, Alertmanager, the Operator, and the exporters were observed running.

Grafana was still starting during the last recorded status check.

The intended next verification is to confirm Grafana is Ready and that the Helm release has been reconciled successfully.

### Grafana Dashboards

The monitoring exercise focuses on existing Kubernetes dashboards, particularly:

- Cluster CPU and memory utilization.
- Node resource utilization.
- Namespace and Pod resource usage.
- Application replica counts and health.
- Network metrics, where available.

The Go API also exposes `/metrics`, allowing application-specific Prometheus monitoring to be added through a ServiceMonitor as a future extension.

---

## 12. Proposed Student Exercises

The practical laboratory can be divided into several short exercises.

**Exercise 1 — Explore the cluster**

Students inspect the nodes, namespaces, Pods, Deployments, Services, Ingress, and storage resources.

**Exercise 2 — Understand Kubernetes networking**

Students trace a request from the browser through Traefik, the API Service, the Worker Service, and PostgreSQL.

**Exercise 3 — Load balancing**

Students send repeated requests to `/api/whoami` and identify which API replicas process them.

**Exercise 4 — Self-healing**

Students delete an API Pod, observe its replacement, and explain the role of the Deployment controller.

**Exercise 5 — Persistent storage**

Students create application tasks, delete the PostgreSQL Pod, and verify that the data remains available.

**Exercise 6 — Monitoring**

Students explore Grafana dashboards, inspect application and cluster resource consumption, and compare workload behavior before and after selected operations.

---

## 13. Expected Deliverables

Students should submit a short technical report, approximately 2–3 pages, documenting their observations.

The report should include:

1. A description of the deployed application and its main Kubernetes resources.
2. An explanation of how requests travel through the application.
3. Evidence of load balancing across API replicas.
4. Evidence of Kubernetes self-healing.
5. Verification of PostgreSQL persistence.
6. Grafana screenshots and an explanation of the metrics observed.

The goal is not simply to execute Kubernetes commands, but to understand why the components behave as they do.

---

## 14. Current Status and Future Extensions

The initial application has been successfully deployed and functionally tested.

Confirmed completed features include:

- Three-node k3d/K3s cluster.
- Frontend, API, Worker and PostgreSQL Deployments.
- Kubernetes Services and DNS.
- Traefik Ingress routing.
- ConfigMap and Secret configuration.
- PostgreSQL persistent storage.
- API load balancing.
- Pod self-healing.
- End-to-end application functionality.

The monitoring components have been installed, with Grafana and the final Helm release status still requiring verification.

Potential future extensions include:

- Kubernetes NetworkPolicies to restrict inter-service communication.
- Horizontal Pod Autoscaling.
- Custom Prometheus metrics and ServiceMonitors.
- Grafana dashboards for HTTP request rates, errors and latency.
- More detailed failure and recovery scenarios.

## Conclusion

KubeLab provides a practical environment for studying Kubernetes networking, container orchestration, persistent storage, availability and observability.

Students work with an actual multi-service application rather than isolated Kubernetes examples.

Through deployment, experimentation and monitoring, they can observe how Kubernetes manages communication between services, distributes requests across replicas, replaces failed Pods, preserves persistent data, and exposes operational metrics.

The initial version is suitable for demonstrating the proposed laboratory concept, with the final Grafana verification remaining before the monitoring exercise is considered complete.
