# UPM API Server v1 API Reference

- Version: 0.6
- Date: 2026-06-06
- Status: Implemented and runtime validated through Phase 04
- Owner: zqw
- Related:
  - ../master-spec.md
  - ../phases/phase-03-upm-api-server.md
  - ../phases/phase-04-healthcheck.md
  - ../design/api-design.md

## 1. Purpose

This document is the local API reference for `upm-api-server`.

It must list every API supported by the current implementation, including path
parameters, query parameters, request bodies, response fields, and usage
examples.

When a later phase adds a feature, the feature must be registered in
`upm-api-server` and this document must be updated in the same phase.

## 2. Runtime Access

`upm-api-server` must run inside Kubernetes. Local binary deployment is not a
supported acceptance path.

The hackathon lab exposes the API through a fixed NodePort:

```bash
export UPM_API_SERVER_URL=http://192.168.35.201:30083
curl -fsS "${UPM_API_SERVER_URL}/api/v1/healthz" | jq .
```

The same NodePort is reachable through any available Kubernetes node:

```text
http://192.168.35.201:30083
http://192.168.35.202:30083
http://192.168.35.203:30083
http://192.168.35.204:30083
```

For temporary administrator access when NodePort is unavailable:

```bash
kubectl -n upm-system port-forward svc/upm-api-server 18083:8080
export UPM_API_SERVER_URL=http://127.0.0.1:18083
```

The Phase 03 NodePort has no application-layer authentication. It is for the
isolated hackathon lab only. Production exposure requires an authenticated
Gateway/Ingress or equivalent protected access path.

## 3. Common Rules

API prefix:

```text
/api/v1
```

Common response safety:

- Responses must not include Kubernetes Secret values.
- Responses must not include ClickHouse passwords, AES keys, or credential
  plaintext.
- Errors must use the structured error format.
- Every request should have a request ID. If the client does not provide one,
  `upm-api-server` should generate one.

Optional request headers:

| Header | Required | Purpose |
|---|---:|---|
| `X-Request-Id` | No | Client-provided request correlation ID |
| `X-Actor` | No | Trusted internal actor/user extension point for MVP |

## 4. Structured Error

All non-2xx API errors use this shape:

```json
{
  "code": "VALIDATION_ERROR",
  "message": "topology.shards must be greater than 0",
  "details": {
    "field": "topology.shards"
  },
  "requestId": "req-xxxxxxxx"
}
```

Fields:

| Field | Type | Required | Description |
|---|---|---:|---|
| `code` | string | Yes | Stable machine-readable error code |
| `message` | string | Yes | Human-readable summary without secret content |
| `details` | object | No | Field-level or resource-level diagnostic details |
| `requestId` | string | Yes | Request correlation ID |

Current error codes:

| Code | HTTP | Meaning |
|---|---:|---|
| `INVALID_JSON` | 400 | Request body is invalid or contains multiple JSON values |
| `VALIDATION_ERROR` | 400 | Path, query, or body parameter validation failed |
| `CLUSTER_ALREADY_EXISTS` | 409 | Server UnitSet for the logical cluster already exists |
| `CLUSTER_RESOURCE_CONFLICT` | 409 | Existing partial resource does not match the requested cluster |
| `ADMIN_SECRET_NOT_FOUND` | 422 | Referenced admin Secret does not exist |
| `ADMIN_SECRET_KEY_MISSING` | 422 | Admin Secret lacks `CLICKHOUSE_ADMIN_PASSWORD` |
| `AES_SECRET_NOT_FOUND` | 422 | Secret `aes-secret-key` does not exist |
| `AES_SECRET_KEY_MISSING` | 422 | Secret `aes-secret-key` lacks `AES_SECRET_KEY` |
| `PACKAGE_VERSION_NOT_INSTALLED` | 422 | Required package ConfigMap or PodTemplate is absent |
| `PACKAGE_TOPOLOGY_INVALID` | 422 | Installed package topology cannot be parsed |
| `PACKAGE_TOPOLOGY_MISMATCH` | 422 | Requested topology differs from installed package topology |
| `CLUSTER_NOT_FOUND` | 404 | Managed cluster does not exist |
| `HEALTHCHECK_REPORT_NOT_FOUND` | 404 | Latest in-memory healthcheck report does not exist |
| `KUBERNETES_FORBIDDEN` | 403 | API server RBAC does not permit the operation |
| `UPMIO_UNITSET_NOT_READY` | 504 | Keeper UnitSet did not become ready before timeout |
| `KUBERNETES_API_ERROR` | 500 | Kubernetes API operation failed |
| `INTERNAL_ERROR` | 500 | Unexpected internal failure |

## 5. Health API

### 5.1 Get Process Health

```http
GET /api/v1/healthz
```

Purpose:

- Check that `upm-api-server` is running and serving requests.

Path parameters: none.

Query parameters: none.

Request body: none.

Success HTTP status: `200 OK`.

Success response:

```json
{
  "status": "ok",
  "service": "upm-api-server"
}
```

Usage:

```bash
curl -sS "${UPM_API_SERVER_URL}/api/v1/healthz" | jq .
```

## 6. Cluster APIs

### 6.1 Create ClickHouse Cluster

```http
POST /api/v1/clusters
```

Purpose:

- Create UPMIO `Project`, Keeper `UnitSet`, and ClickHouse Server `UnitSet`
  resources for one logical ClickHouse cluster.
- The API must not create raw Pods, PVCs, or Services as the product path.

Path parameters: none.

Query parameters: none.

Request body:

```json
{
  "namespace": "upm-clickhouse-runtime",
  "name": "clickhouse-runtime",
  "version": "26.3.9.8",
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

Request fields:

| Field | Type | Required | Description |
|---|---|---:|---|
| `namespace` | string | Yes | Kubernetes namespace for the managed cluster |
| `name` | string | Yes | Logical cluster name and server UnitSet name |
| `version` | string | Yes | ClickHouse package/runtime version |
| `topology.shards` | integer | Yes | Number of ClickHouse shards |
| `topology.replicasPerShard` | integer | Yes | Number of replicas per shard |
| `topology.keeperReplicas` | integer | Yes | Number of Keeper instances |
| `storage.className` | string | Yes | Kubernetes StorageClass name |
| `storage.serverDataSize` | string | Yes | PVC size for ClickHouse Server data |
| `storage.keeperDataSize` | string | Yes | PVC size for Keeper data |
| `security.adminSecretRef` | string | Yes | Existing Kubernetes Secret name for ClickHouse credentials |
| `monitoring.enabled` | boolean | No | Whether to enable PodMonitor/metrics integration |

Validation rules:

- `namespace` must be a valid Kubernetes namespace name.
- `name` must be a valid Kubernetes resource name.
- `topology.shards` must be greater than `0`.
- `topology.replicasPerShard` must be greater than `0`.
- `topology.keeperReplicas` must be `3` or another explicitly supported value.
- Requested topology must match the topology rendered by the installed
  ClickHouse package version. A mismatch returns
  `PACKAGE_TOPOLOGY_MISMATCH` instead of creating inconsistent UnitSets.
- Storage sizes must be valid Kubernetes quantities.
- `security.adminSecretRef` must reference an existing Secret containing
  `CLICKHOUSE_ADMIN_PASSWORD`.
- The target namespace must also contain Secret `aes-secret-key` with key
  `AES_SECRET_KEY`.
- If the target namespace does not exist, `upm-api-server` creates it first and
  then returns the missing-Secret error. Add the required Secrets and retry the
  same request.
- `upm-api-server` checks Secret/key existence but never returns or logs Secret
  values.

Success HTTP status: `202 Accepted`.

Success response:

```json
{
  "requestId": "req-xxxxxxxx",
  "namespace": "upm-clickhouse-runtime",
  "name": "clickhouse-runtime",
  "status": "Provisioning",
  "resources": {
    "project": "upm-clickhouse-runtime",
    "keeperUnitSet": "clickhouse-runtime-keeper",
    "serverUnitSet": "clickhouse-runtime"
  }
}
```

Usage:

```bash
curl -sS -X POST "${UPM_API_SERVER_URL}/api/v1/clusters" \
  -H "Content-Type: application/json" \
  --data @cluster-create.json | jq .
```

Invalid request usage:

```bash
curl -sS -X POST "${UPM_API_SERVER_URL}/api/v1/clusters" \
  -H "Content-Type: application/json" \
  -d '{"namespace":"bad namespace","name":"x"}' | jq .
```

### 6.2 List ClickHouse Clusters

```http
GET /api/v1/clusters
```

Purpose:

- List managed ClickHouse clusters discovered from UPMIO labels and UnitSets.

Path parameters: none.

Query parameters:

| Parameter | Type | Required | Description |
|---|---|---:|---|
| `namespace` | string | No | Limit results to one namespace |

Request body: none.

Success HTTP status: `200 OK`.

Success response:

```json
{
  "items": [
    {
      "namespace": "upm-clickhouse-runtime",
      "name": "clickhouse-runtime",
      "status": "Running",
      "version": "26.3.9.8",
      "topology": {
        "shards": 1,
        "replicasPerShard": 2,
        "keeperReplicas": 3
      },
      "ready": {
        "keeper": "3/3",
        "server": "2/2"
      }
    }
  ]
}
```

Usage:

```bash
curl -sS "${UPM_API_SERVER_URL}/api/v1/clusters" | jq .
```

Namespace-filtered usage:

```bash
curl -sS "${UPM_API_SERVER_URL}/api/v1/clusters?namespace=upm-clickhouse-phase03-runtime" | jq .
```

### 6.3 Get ClickHouse Cluster Summary

```http
GET /api/v1/clusters/{namespace}/{name}
```

Purpose:

- Return one logical ClickHouse cluster summary.

Path parameters:

| Parameter | Type | Required | Description |
|---|---|---:|---|
| `namespace` | string | Yes | Kubernetes namespace |
| `name` | string | Yes | Logical cluster name |

Query parameters: none.

Request body: none.

Success HTTP status: `200 OK`.

Success response:

```json
{
  "namespace": "upm-clickhouse-runtime",
  "name": "clickhouse-runtime",
  "status": "Running",
  "version": "26.3.9.8",
  "topology": {
    "shards": 1,
    "replicasPerShard": 2,
    "keeperReplicas": 3
  },
  "resources": {
    "keeperUnitSet": "clickhouse-runtime-keeper",
    "serverUnitSet": "clickhouse-runtime"
  }
}
```

Usage:

```bash
curl -sS \
  "${UPM_API_SERVER_URL}/api/v1/clusters/upm-clickhouse-phase03-runtime/clickhouse-phase03" \
  | jq .
```

### 6.4 Get ClickHouse Cluster Resources

```http
GET /api/v1/clusters/{namespace}/{name}/resources
```

Purpose:

- Return UPMIO and Kubernetes resource status for one logical cluster.

Path parameters:

| Parameter | Type | Required | Description |
|---|---|---:|---|
| `namespace` | string | Yes | Kubernetes namespace |
| `name` | string | Yes | Logical cluster name |

Query parameters: none.

Request body: none.

Success HTTP status: `200 OK`.

Success response:

```json
{
  "namespace": "upm-clickhouse-runtime",
  "name": "clickhouse-runtime",
  "unitSets": [],
  "units": [],
  "pods": [],
  "pvcs": [],
  "services": [],
  "endpoints": [],
  "events": []
}
```

Response collections:

| Field | Description |
|---|---|
| `unitSets` | Keeper and ClickHouse Server UnitSet summaries |
| `units` | UPMIO Unit summaries |
| `pods` | Pod readiness and container status summaries |
| `pvcs` | PVC phase, size, and storage class summaries |
| `services` | Service type, ports, selector, and endpoint availability |
| `endpoints` | Endpoint readiness summaries |
| `events` | Recent relevant Kubernetes events |

Usage:

```bash
curl -sS \
  "${UPM_API_SERVER_URL}/api/v1/clusters/upm-clickhouse-phase03-runtime/clickhouse-phase03/resources" \
  | jq .
```

### 6.5 Run ClickHouse Runtime Healthcheck

```http
POST /api/v1/clusters/{namespace}/{name}/healthcheck
```

Purpose:

- Run a live runtime healthcheck for one managed ClickHouse cluster.
- Validate Kubernetes resources, Keeper quorum, ClickHouse SQL reachability,
  `system.clusters` topology, reserved Distributed write/read behavior,
  replicated table health, metrics endpoint reachability, and response safety.
- Store the completed report in the API server process memory as the latest
  report for `{namespace}/{name}`.

Path parameters:

| Parameter | Type | Required | Description |
|---|---|---:|---|
| `namespace` | string | Yes | Kubernetes namespace |
| `name` | string | Yes | Logical cluster name |

Query parameters: none.

Request body: none.

Healthcheck data boundary:

- The API creates or reuses reserved database `upm_healthcheck`.
- The API creates or reuses reserved tables `local_events` and `dist_events`.
- The API writes deterministic validation rows only into these reserved tables.
- Business databases and business tables are not created, modified, or dropped.
- Existing reserved table definition drift is treated as a failed healthcheck.
- Existing reserved table definitions are validated before truncate or insert.
- Concurrent healthchecks for the same cluster are serialized.
- The report must not contain Secret values, ClickHouse password values, AES
  keys, or credential plaintext.

Success HTTP status: `200 OK`.

Overall status rules:

| Status | Meaning |
|---|---|
| `PASS` | All critical and warning checks passed |
| `WARN` | All critical checks passed, but one or more warning checks failed |
| `FAIL` | One or more critical checks failed |

Success response shape:

```json
{
  "namespace": "upm-clickhouse-phase03-runtime",
  "name": "clickhouse-phase03",
  "cluster": "clickhouse-phase03",
  "status": "PASS",
  "startedAt": "2026-06-06T08:00:00Z",
  "completedAt": "2026-06-06T08:00:12Z",
  "durationMs": 12000,
  "summary": {
    "passed": 14,
    "warnings": 0,
    "failed": 0,
    "skipped": 0
  },
  "checks": [
    {
      "name": "write_read_probe",
      "status": "PASS",
      "severity": "critical",
      "message": "Distributed write/read probe succeeded through reserved healthcheck tables",
      "evidence": {
        "database": "upm_healthcheck",
        "localTable": "local_events",
        "distributedTable": "dist_events",
        "insertedRows": 8
      },
      "startedAt": "2026-06-06T08:00:05Z",
      "endedAt": "2026-06-06T08:00:10Z"
    }
  ],
  "requestId": "req-xxxxxxxx"
}
```

Current check names:

| Check | Severity | Description |
|---|---|---|
| `upmio_project_exists` | critical | UPMIO Project exists |
| `keeper_unitset_ready` | critical | Keeper UnitSet ready units match topology |
| `server_unitset_ready` | critical | ClickHouse Server UnitSet ready units match topology |
| `pods_ready` | critical | Managed Keeper and ClickHouse Pods are running and ready |
| `pvc_bound` | critical | Managed data PVCs are bound |
| `services_endpoints` | critical | Managed Services have ready Endpoints and ClickHouse TCP/HTTP/interserver/metrics ports |
| `keeper_ruok` | critical | Keeper Pods answer `ruok` with `imok` |
| `keeper_leader_follower` | critical | Keeper quorum has one leader and followers |
| `clickhouse_select_1` | critical | ClickHouse SQL endpoint accepts `SELECT 1` |
| `system_clusters_topology` | critical | `system.clusters` matches the expected topology |
| `write_read_probe` | critical | Reserved Distributed write/read probe succeeds |
| `replica_health` | critical | Reserved replicated table replicas are active and caught up |
| `metrics_endpoint` | warning | Local Prometheus metrics endpoint is reachable |
| `no_secret_leakage` | critical | Returned report avoids forbidden secret-like tokens and known Secret values |

Usage:

```bash
curl -sS -X POST \
  "${UPM_API_SERVER_URL}/api/v1/clusters/upm-clickhouse-phase03-runtime/clickhouse-phase03/healthcheck" \
  | jq .
```

### 6.6 Get Latest ClickHouse Runtime Healthcheck

```http
GET /api/v1/clusters/{namespace}/{name}/healthcheck/latest
```

Purpose:

- Return the latest in-memory healthcheck report produced by
  `POST /healthcheck` for the same `{namespace}/{name}`.

Path parameters:

| Parameter | Type | Required | Description |
|---|---|---:|---|
| `namespace` | string | Yes | Kubernetes namespace |
| `name` | string | Yes | Logical cluster name |

Query parameters: none.

Request body: none.

Success HTTP status: `200 OK`.

Success response: same shape as `POST /healthcheck`.

Cache boundary:

- The latest report is stored in `upm-api-server` process memory.
- Restarting the Pod clears the latest report.
- No Kubernetes object, PVC, ConfigMap, or CRD is used for Phase 04 latest
  report persistence.
- If no report exists after startup, the API returns
  `HEALTHCHECK_REPORT_NOT_FOUND`.

Usage:

```bash
curl -sS \
  "${UPM_API_SERVER_URL}/api/v1/clusters/upm-clickhouse-phase03-runtime/clickhouse-phase03/healthcheck/latest" \
  | jq .
```

Not-found response:

```json
{
  "code": "HEALTHCHECK_REPORT_NOT_FOUND",
  "message": "latest healthcheck report not found",
  "details": {
    "namespace": "upm-clickhouse-phase03-runtime",
    "name": "clickhouse-phase03"
  },
  "requestId": "req-xxxxxxxx"
}
```

## 7. Not Supported After Phase 04

These APIs are registered in later phase specs and must not be claimed as
supported until implemented and validated:

| API | Phase |
|---|---|
| `GET /api/v1/clusters/{namespace}/{name}/metrics/summary` | Phase 05 |
| `GET /api/v1/clusters/{namespace}/{name}/diagnostics` | Phase 06 |
| `POST /api/v1/clusters/{namespace}/{name}/backup` | Phase 07 |
| `POST /api/v1/clusters/{namespace}/{name}/restore` | Phase 07 |
