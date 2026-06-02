# Phase 03: Manager Backend

- Version: 0.4
- Date: 2026-05-27
- Status: Confirmed
- Priority: P0
- Owner: zqw
- Depends on:
  - ../master-spec.md
  - ../evidence-summary.md
  - ../architecture/upmio-architecture.md
  - ../design/api-design.md
  - ../design/resource-model.md
  - ../design/data-architecture.md
  - ../design/product-design.md

## 1. Purpose

Implement the Go Manager Backend as the product control plane for ClickHouse deployment management, status aggregation, and later healthcheck/diagnostics APIs.

This phase must produce the backend skeleton and UPMIO/Kubernetes/ClickHouse integration boundaries. It must not implement every Day2 operation.

## 2. Scope

In scope:

1. Go backend project skeleton.
2. HTTP API server under `/api/v1`.
3. UPMIO adapter for `Project`, `UnitSet`, `Unit`, and future `GrpcCall` read paths.
4. Kubernetes reader for Pod, PVC, Service, Endpoint, Secret reference existence, Event.
5. ClickHouse client wrapper for read-only SQL and validation SQL.
6. Basic cluster create/read/status APIs.
7. Structured error response model.
8. Structured logging with redaction.
9. Request ID and actor extension fields.
10. Operation history storage decision for MVP and future.

## 3. Non-Goals

Out of scope:

- Frontend implementation.
- Full authentication/RBAC.
- Persistent operation history database unless explicitly approved.
- Backup/restore execution.
- New UPMIO CRDs.
- Raw Pod/PVC/Service creation as product path.
- Automatic remediation.

## 4. Backend Boundaries

The Manager Backend must not replace UPMIO operators.

Allowed responsibilities:

- Validate user request.
- Create or update UPMIO CRDs and supporting Secrets/config inputs.
- Read UPMIO/Kubernetes status.
- Execute ClickHouse read-only/validation SQL.
- Query Prometheus in Phase 05.
- Produce structured reports.

Disallowed responsibilities:

- Directly reconcile Pods/PVCs/Services.
- Implement a new Operator loop in MVP.
- Store ClickHouse credentials in ConfigMap or logs.
- Treat source-code assumptions as runtime readiness.

## 5. Create Cluster Flow

The initial cluster create API must follow this flow:

```text
1. Receive CreateClusterRequest.
2. Validate namespace/name/topology/version/storage/resources/security fields.
3. Check UPMIO runtime prerequisites:
   - CRDs exist.
   - unit-operator is running.
   - required package version is installed/available.
4. Ensure Kubernetes Namespace and UPMIO Project exist.
5. Ensure required Secret references exist or create generated Secret if explicitly allowed.
6. Render or select Keeper UnitSet definition.
7. Apply Keeper UnitSet through Kubernetes API.
8. Wait/read Keeper UnitSet and Units until Ready or timeout.
9. Render or select ClickHouse Server UnitSet definition.
10. Apply ClickHouse Server UnitSet through Kubernetes API.
11. Return operation result with resource references and next-step healthcheck link.
```

The API may be synchronous for MVP if timeouts are controlled. Future implementation may introduce asynchronous operation records.

## 6. Core API Endpoints

| Method | Path | Purpose | MVP |
|---|---|---|---|
| `POST` | `/api/v1/clusters` | Create ClickHouse cluster resources | Yes |
| `GET` | `/api/v1/clusters` | List managed clusters from labels/UnitSets | Yes |
| `GET` | `/api/v1/clusters/{namespace}/{name}` | Get logical cluster summary | Yes |
| `GET` | `/api/v1/clusters/{namespace}/{name}/resources` | Return UnitSet/Unit/Pod/PVC/Service/Endpoint summary | Yes |
| `POST` | `/api/v1/clusters/{namespace}/{name}/healthcheck` | Trigger Day1 healthcheck | Phase 04 |
| `GET` | `/api/v1/clusters/{namespace}/{name}/healthcheck/latest` | Return latest report if retained | Phase 04 |
| `GET` | `/api/v1/clusters/{namespace}/{name}/diagnostics` | Return read-only Day2 diagnostics | Phase 06 |
| `GET` | `/api/v1/clusters/{namespace}/{name}/metrics/summary` | Return Prometheus metrics summary | Phase 05 |

## 7. Logical Models

### 7.1 CreateClusterRequest

Required fields:

```json
{
  "namespace": "upm-clickhouse-runtime",
  "name": "clickhouse-runtime",
  "version": "26.3.x",
  "topology": {
    "shards": 1,
    "replicasPerShard": 2,
    "keeperReplicas": 3
  },
  "storage": {
    "className": "local-path",
    "serverDataSize": "20Gi",
    "keeperDataSize": "10Gi"
  },
  "security": {
    "adminSecretRef": "clickhouse-runtime-secret"
  },
  "monitoring": {
    "enabled": true
  }
}
```

### 7.2 ErrorResponse

Must match `design/api-design.md`:

```json
{
  "code": "UPMIO_UNITSET_NOT_READY",
  "message": "ClickHouse UnitSet is not ready",
  "details": {},
  "requestId": "req-xxxxxxxx"
}
```

### 7.3 OperationResult

For MVP, operation result may be derived from current resource state and returned in the response. Persistent DB is not required.

This phase must close the Master Spec Open Question:

| Question | Required Phase 03 output |
|---|---|
| Does operation history require persistent storage? | Decide MVP behavior and future storage option |

Expected decision:

- MVP: no independent DB; operation result is returned and logs/events are used for troubleshooting.
- Future: persistent operation history can be introduced after API/approval model is stable.

## 8. Suggested Module Boundaries

Directory names may change, but responsibilities must remain clear:

```text
backend/
  cmd/server/              # process entrypoint
  internal/api/            # routes and handlers
  internal/model/          # request/response models
  internal/upmio/          # Project/UnitSet/Unit/GrpcCall adapter
  internal/k8s/            # Pod/PVC/Service/Endpoint/Event reader
  internal/clickhouse/     # SQL client and system table queries
  internal/logging/        # structured logger/redaction
  internal/config/         # runtime config
```

Do not implement Phase 05 Prometheus client or Phase 06 diagnostics unless this phase explicitly depends on them.

## 9. Files Likely Changed

```text
backend/go.mod
backend/cmd/server/main.go
backend/internal/api/routes.go
backend/internal/api/cluster_handler.go
backend/internal/model/cluster.go
backend/internal/model/error.go
backend/internal/upmio/client.go
backend/internal/upmio/project.go
backend/internal/upmio/unitset.go
backend/internal/k8s/resources.go
backend/internal/clickhouse/client.go
backend/internal/logging/logger.go
backend/Dockerfile
backend/README.md
```

If the repository chooses a different backend root, preserve the same responsibilities.

## 10. Acceptance Criteria

1. Backend starts locally without requiring Kubernetes cluster connection when run in mock/config-check mode.
2. Backend exposes `/api/v1/healthz` or equivalent process health endpoint.
3. `POST /api/v1/clusters` validates request and rejects invalid topology/storage/security fields with structured error response.
4. `GET /api/v1/clusters` can list clusters by UPMIO labels/UnitSets when connected to a cluster.
5. `GET /api/v1/clusters/{namespace}/{name}/resources` returns UnitSet, Unit, Pod, PVC, Service, and Endpoint summary.
6. Backend never logs Secret values.
7. Backend does not create raw Pods/PVCs/Services as product path.
8. MVP operation history decision is documented and implemented consistently.
9. Unit tests cover request validation and error response formatting.
10. `go test ./...` passes.

## 11. Verification Commands

```bash
cd backend

go fmt ./...
go vet ./...
go test ./...
go build ./cmd/server

# Local process check
./server --config ./config/example.yaml &
curl -sS http://127.0.0.1:<port>/api/v1/healthz

# API validation examples
curl -sS -X POST http://127.0.0.1:<port>/api/v1/clusters \
  -H 'Content-Type: application/json' \
  -d '{"namespace":"bad namespace","name":"x"}' | jq .

# Cluster-connected checks, when kubeconfig is available
curl -sS http://127.0.0.1:<port>/api/v1/clusters | jq .
curl -sS http://127.0.0.1:<port>/api/v1/clusters/upm-clickhouse-runtime/clickhouse-runtime/resources | jq .
```

## 12. Risks and Open Questions

| Risk / Question | Handling |
|---|---|
| Backend needs a DB for operation history | Do not add DB in MVP; document future model |
| Cluster API calls may block | Use timeout/context for all external calls |
| Auth is not implemented in MVP | Keep actor field extension points and record trusted internal assumption |
| UPMIO API versions change | Centralize UPMIO adapter and avoid scattering dynamic client logic |

## 13. Changelog

| Version | Date | Changes |
|---|---|---|
| 0.1 | 2026-05-27 | Initial phase draft |
| 0.2 | 2026-05-27 | Added backend/API flow and module boundaries |
| 0.3 | 2026-05-27 | Added metadata and red-team fix structure |
| 0.4 | 2026-05-27 | Restored create-cluster flow, API table, backend responsibilities, operation-history decision, and objective acceptance criteria |
