# Phase 03: UPM API Server

- Version: 0.6
- Date: 2026-06-05
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

Implement the Go `upm-api-server` as the product control-plane API service
for UPM-managed databases. This phase exposes the first ClickHouse API surface
for deployment management, status aggregation, and later
healthcheck/diagnostics APIs.

`upm-api-server` is intentionally higher-level than a ClickHouse-specific
control service. It must be able to manage multiple ClickHouse clusters from one
service instance and leave room for future MySQL, Redis, and other database API
surfaces.

This phase must produce the API server skeleton and
UPMIO/Kubernetes/ClickHouse integration boundaries. It must not implement every
Day2 operation.

## 2. Scope

In scope:

1. Go `upm-api-server` project skeleton.
2. HTTP API server under `/api/v1`.
3. UPMIO adapter for `Project`, `UnitSet`, `Unit`, and future `GrpcCall` read paths.
4. Kubernetes reader for Pod, PVC, Service, Endpoint, Secret reference existence, Event.
5. ClickHouse client wrapper for read-only SQL and validation SQL.
6. Basic cluster create/read/status APIs.
7. Structured error response model.
8. Structured logging with redaction.
9. Request ID and actor extension fields.
10. Operation history storage decision for MVP and future.
11. Kubernetes deployment assets for running `upm-api-server` as a system-level
    service in `upm-system`.
12. API reference documentation that lists every supported API, all parameters,
    response shape, and usage examples.

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

The `upm-api-server` must not replace UPMIO operators or the Kubernetes API
server.

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
- Require one API server instance per ClickHouse cluster.

## 4.1 Runtime Shape

Production/demo runtime shape:

```text
Kubernetes cluster
  upm-system
    Deployment/upm-api-server
    Service/upm-api-server
    ServiceAccount/upm-api-server
    Role/ClusterRole + binding
    ConfigMap/upm-api-server-config
```

One `upm-api-server` instance must manage multiple logical clusters through
namespaced API resources:

```text
UPM API Server
  -> namespace-a / clickhouse-prod
  -> namespace-a / clickhouse-test
  -> namespace-b / clickhouse-analytics
```

Local binary deployment is not an acceptance path. The phase may run Go build
and unit-test commands locally, but the service must be deployed and validated
inside Kubernetes.

Every new product capability implemented after this phase must be integrated
and registered through `upm-api-server` unless the phase explicitly declares it
as package-only, operator-only, or evidence-only work.

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

The API may be synchronous for MVP if timeouts are controlled. Future
implementation may introduce asynchronous operation records.

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

For MVP, operation result may be derived from current resource state and
returned in the response. Persistent DB is not required.

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
api-server/
  cmd/upm-api-server/      # process entrypoint
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
api-server/go.mod
api-server/cmd/upm-api-server/main.go
api-server/internal/api/routes.go
api-server/internal/api/cluster_handler.go
api-server/internal/model/cluster.go
api-server/internal/model/error.go
api-server/internal/upmio/client.go
api-server/internal/upmio/project.go
api-server/internal/upmio/unitset.go
api-server/internal/k8s/resources.go
api-server/internal/clickhouse/client.go
api-server/internal/logging/logger.go
api-server/Dockerfile
api-server/README.md
clickhouse/phase-03/manifests/upm-api-server.yaml
docs/api/upm-api-server-v1.md
```

If the repository chooses a different API server root, preserve the same
responsibilities.

## 10. Acceptance Criteria

1. `upm-api-server` runs inside Kubernetes as a Deployment/Service in
   `upm-system`.
2. `upm-api-server` exposes `/api/v1/healthz` or equivalent process health endpoint through the Kubernetes Service.
3. `POST /api/v1/clusters` validates request and rejects invalid topology/storage/security fields with structured error response.
4. `GET /api/v1/clusters` can list clusters by UPMIO labels/UnitSets when connected to a cluster.
5. `GET /api/v1/clusters/{namespace}/{name}/resources` returns UnitSet, Unit, Pod, PVC, Service, and Endpoint summary.
6. `upm-api-server` never logs Secret values.
7. `upm-api-server` does not create raw Pods/PVCs/Services as product path.
8. MVP operation history decision is documented and implemented consistently.
9. Unit tests cover request validation and error response formatting.
10. Kubernetes deployment assets run `upm-api-server` in `upm-system` with
    RBAC-scoped access to the required UPMIO/Kubernetes resources.
11. `docs/api/upm-api-server-v1.md` lists all supported APIs, path/query/body
    parameters, request/response examples, and usage notes.
12. `go test ./...` passes.

## 11. Verification Commands

```bash
cd api-server

go fmt ./...
go vet ./...
go test ./...
go build ./cmd/upm-api-server

# Kubernetes deployment check
cd ..
kubectl apply -f clickhouse/phase-03/manifests/upm-api-server.yaml
kubectl -n upm-system rollout status deploy/upm-api-server --timeout=180s
kubectl -n upm-system get deploy/upm-api-server svc/upm-api-server sa/upm-api-server

# API validation examples
kubectl -n upm-system port-forward svc/upm-api-server 18083:8080

curl -sS http://127.0.0.1:18083/api/v1/healthz

curl -sS -X POST http://127.0.0.1:18083/api/v1/clusters \
  -H 'Content-Type: application/json' \
  -d '{"namespace":"bad namespace","name":"x"}' | jq .

# Cluster-connected checks, when kubeconfig is available
curl -sS http://127.0.0.1:18083/api/v1/clusters | jq .
curl -sS http://127.0.0.1:18083/api/v1/clusters/upm-clickhouse-runtime/clickhouse-runtime/resources | jq .

# Full runtime acceptance
clickhouse/phase-03/scripts/validate-upm-api-server-runtime.sh
```

## 12. Risks and Open Questions

| Risk / Question | Handling |
|---|---|
| API server needs a DB for operation history | Do not add DB in MVP; document future model |
| Cluster API calls may block | Use timeout/context for all external calls |
| Auth is not implemented in MVP | Keep actor field extension points and record trusted internal assumption |
| UPMIO API versions change | Centralize UPMIO adapter and avoid scattering dynamic client logic |
| Name may be confused with Kubernetes API server | Use `upm-api-server` consistently and describe it as a UPM product control-plane API, not Kubernetes apiserver |
| API surface drifts from implementation | Keep `docs/api/upm-api-server-v1.md` updated in every feature commit and validate documented examples during closeout |

## 13. Changelog

| Version | Date | Changes |
|---|---|---|
| 0.1 | 2026-05-27 | Initial phase draft |
| 0.2 | 2026-05-27 | Added backend/API flow and module boundaries |
| 0.3 | 2026-05-27 | Added metadata and red-team fix structure |
| 0.4 | 2026-05-27 | Restored create-cluster flow, API table, backend responsibilities, operation-history decision, and objective acceptance criteria |
| 0.5 | 2026-06-05 | Renamed Phase 03 product surface to `upm-api-server`, clarified multi-cluster/system-level runtime, and added Kubernetes deployment acceptance |
| 0.6 | 2026-06-05 | Removed local binary deployment from acceptance, required K8s-only runtime validation, and added API reference deliverable |
