# Frontend

Single page static Task List UI served by Nginx on container port `8080`. It uses plain HTML, CSS and JavaScript, with no build step or runtime environment variables.

The browser calls same-origin `GET/POST /api/tasks`, `DELETE /api/tasks/{id}`, and `GET /api/whoami`. Kubernetes Ingress must route `/api` to the API Service and `/` to this frontend. Local Compose needs an integration-owned reverse proxy with the same routes; this Nginx container serves static files only. Keep the `/api` prefix when forwarding to the API.

The UI loads tasks and API identity on page open. Refresh and Check identity repeat those requests. Task titles and identity values are inserted with `textContent`. Title checks provide immediate feedback; the API remains authoritative. A successful DELETE `204` is handled without parsing a body.

Run focused JavaScript checks with `node --test frontend/app.test.mjs` from the repository root. Build the image with `docker build -t kubelab-frontend:local frontend` when Docker is available. The official pinned Nginx Alpine image supports amd64 and arm64.
