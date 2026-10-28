CLUSTER := kubelab
NAMESPACE := kubelab
IMAGE_TAG := 0.1.0
IMAGES := kubelab-frontend:$(IMAGE_TAG) kubelab-api:$(IMAGE_TAG) kubelab-worker:$(IMAGE_TAG)

.PHONY: build cluster-up load-images deploy smoke cluster-down

build:
	docker build -t kubelab-frontend:$(IMAGE_TAG) ./frontend
	docker build -t kubelab-api:$(IMAGE_TAG) ./api
	docker build -t kubelab-worker:$(IMAGE_TAG) ./worker

cluster-up:
	k3d cluster create $(CLUSTER) --servers 1 --agents 2 -p '8085:80@loadbalancer'

load-images:
	k3d image import $(IMAGES) -c $(CLUSTER)

deploy:
	@test -n "$$POSTGRES_USER" || { echo 'Set POSTGRES_USER in the environment'; exit 1; }
	@test -n "$$POSTGRES_PASSWORD" || { echo 'Set POSTGRES_PASSWORD in the environment'; exit 1; }
	kubectl apply -f deploy/k8s/base/namespace.yaml
	kubectl apply -f deploy/k8s/base/configmap.yaml
	@kubectl -n $(NAMESPACE) create secret generic postgres-credentials --from-literal=POSTGRES_USER="$$POSTGRES_USER" --from-literal=POSTGRES_PASSWORD="$$POSTGRES_PASSWORD" --dry-run=client -o yaml | kubectl apply -f - >/dev/null
	kubectl apply -f deploy/k8s/base/workloads.yaml
	kubectl -n $(NAMESPACE) rollout status deployment/postgres --timeout=180s
	kubectl -n $(NAMESPACE) rollout status deployment/worker --timeout=180s
	kubectl -n $(NAMESPACE) rollout status deployment/api --timeout=180s
	kubectl -n $(NAMESPACE) rollout status deployment/frontend --timeout=180s

smoke:
	curl -fsS -o /dev/null http://localhost:8085/
	curl -fsS http://localhost:8085/api/whoami
	curl -fsS http://localhost:8085/api/tasks
	curl -fsS -X POST -H 'Content-Type: application/json' -d '{"title":"k3d-smoke"}' http://localhost:8085/api/tasks
	curl -fsS http://localhost:8085/api/tasks
	kubectl -n $(NAMESPACE) get deploy,pods,services,ingress,pvc -o wide

cluster-down:
	k3d cluster delete $(CLUSTER)
