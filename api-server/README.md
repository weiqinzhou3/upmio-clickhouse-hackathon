# UPM API Server

`upm-api-server` is the UPM product control-plane API service. It runs inside
Kubernetes and exposes the first ClickHouse management APIs under `/api/v1`.

It uses:

- an in-cluster Kubernetes client;
- UPMIO `Project`, `UnitSet`, and `Unit` resources;
- Kubernetes resource summaries for Pod, PVC, Service, Endpoint, and Event;
- structured errors, request IDs, JSON logs, and secret-safe responses.

It does not directly create Pods, PVCs, or Services as the product path.

## Quality Gates

```bash
cd api-server
GOPROXY=https://goproxy.cn,direct go fmt ./...
GOPROXY=https://goproxy.cn,direct go vet ./...
GOPROXY=https://goproxy.cn,direct go test ./...
GOPROXY=https://goproxy.cn,direct go build ./cmd/upm-api-server
```

## Kubernetes Deployment

Build and synchronize the image to all lab nodes:

```bash
SSH_PASSWORD=<node-password> \
  clickhouse/sync-upm-api-server-image-to-nodes.sh
```

Deploy:

```bash
kubectl apply -f clickhouse/phase-03/manifests/upm-api-server.yaml
kubectl -n upm-system rollout status deploy/upm-api-server --timeout=180s
```

Runtime acceptance:

```bash
clickhouse/phase-03/scripts/validate-upm-api-server-runtime.sh
```

The supported API reference is maintained in
`docs/api/upm-api-server-v1.md`.
