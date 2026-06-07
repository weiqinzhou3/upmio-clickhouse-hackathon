# UPM API Server v1 API Reference

- Version: 0.9
- Date: 2026-06-07
- Status: Implemented through Phase 07
- Owner: zqw
- Related:
  - ../master-spec.md
  - ../phases/phase-03-upm-api-server.md
  - ../phases/phase-04-healthcheck.md
  - ../phases/phase-06-day2-diagnostics.md
  - ../phases/phase-07-backup-restore.md
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
| `PROMETHEUS_UNAVAILABLE` | 503 | Configured Prometheus API cannot be queried |
| `BACKUP_STORAGE_SECRET_NOT_FOUND` | 422 | Referenced backup storage Secret does not exist |
| `BACKUP_STORAGE_SECRET_KEY_MISSING` | 422 | Backup storage Secret lacks a required S3 key |
| `BACKUP_TASK_NOT_FOUND` | 404 | Backup or restore Job is absent or is not API-managed for the cluster |
| `BACKUP_SCHEDULE_ALREADY_EXISTS` | 409 | API-managed backup schedule already exists |
| `BACKUP_SCHEDULE_NOT_FOUND` | 404 | Backup schedule CronJob is absent or is not API-managed for the cluster |
| `CLICKHOUSE_POD_NOT_AVAILABLE` | 422 | No ClickHouse Server Pod is available to derive task runtime |
| `CLICKHOUSE_IMAGE_NOT_FOUND` | 422 | ClickHouse task image cannot be discovered from a server Pod |
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

## 7. Metrics API

### 7.1 Get Prometheus Metrics Summary

```http
GET /api/v1/clusters/{namespace}/{name}/metrics/summary
```

Purpose:

- Compare the expected ClickHouse Server Pods with real Prometheus target
  results.
- Return fixed-query CPU, memory, PVC/storage, and ClickHouse native metric
  summaries.
- This API does not expose arbitrary user-supplied PromQL.

Path parameters:

| Parameter | Required | Description |
|---|---:|---|
| `namespace` | Yes | Managed ClickHouse cluster namespace |
| `name` | Yes | Managed ClickHouse cluster name |

Query parameters: none.

Request body: none.

Success HTTP status: `200 OK`.

Status behavior:

| Status | Meaning |
|---|---|
| `READY` | PodMonitor exists, every expected target is up, and all required metric categories contain samples |
| `DEGRADED` | Prometheus is available, but a target or required metric category is missing |

Prometheus unavailable returns HTTP `503` with error code
`PROMETHEUS_UNAVAILABLE`.

Response fields:

| Field | Type | Description |
|---|---|---|
| `namespace` | string | Managed cluster namespace |
| `name` / `cluster` | string | Managed cluster name |
| `status` | string | `READY` or `DEGRADED` |
| `collectedAt` | timestamp | Summary collection time |
| `podMonitor.name` | string | Expected UnitSet-generated PodMonitor name |
| `podMonitor.exists` | boolean | Whether the API server can read the PodMonitor |
| `targets[]` | array | Expected ClickHouse Server Pods and real Prometheus up state |
| `summary.cpu[]` | array | Per-Pod CPU samples in cores |
| `summary.memory[]` | array | Per-Pod working-set memory samples in bytes |
| `summary.storage[]` | array | Storage usage samples from kubelet PVC metrics, with ClickHouse native disk used/total/available metrics as the local-path fallback |
| `summary.clickhouse[]` | array | Fixed ClickHouse native query/write/memory metrics |
| `warnings[]` | array | Missing target, PodMonitor, or metric evidence |
| `requestId` | string | Request correlation ID |

Usage:

```bash
curl -fsS \
  "${UPM_API_SERVER_URL}/api/v1/clusters/upm-clickhouse-phase03-runtime/clickhouse-phase03/metrics/summary" \
  | jq .
```

## 8. Diagnostics API

### 8.1 Run Day2 Read-only Diagnostics

```http
GET /api/v1/clusters/{namespace}/{name}/diagnostics
```

Purpose:

- Return read-only Day2 diagnostics for one managed ClickHouse cluster.
- Inspect replica state, replication queue, parts/partitions, merges,
  mutations, Keeper role state, PVC capacity/binding, write-client visibility,
  and optional write-quality row count.
- This API returns recommendations only. It does not execute remediation,
  mutation kill, OPTIMIZE, TTL changes, backup/restore, or data correction.

Path parameters:

| Parameter | Required | Description |
|---|---:|---|
| `namespace` | Yes | Managed ClickHouse cluster namespace |
| `name` | Yes | Managed ClickHouse cluster name |

Query parameters:

| Parameter | Required | Description |
|---|---:|---|
| `database` | No | Restrict table-scoped diagnostics and enable row-count validation when used with `table` |
| `table` | No | Restrict table-scoped diagnostics and enable row-count validation when used with `database` |
| `partition` | No | Optional `_partition_id` filter for row-count validation |
| `timeColumn` | No | Optional time column for row-count validation |
| `startTime` | No | Optional lower time bound; requires `timeColumn` |
| `endTime` | No | Optional upper time bound; requires `timeColumn` |
| `expectedRows` | No | Optional expected row count. Mismatch returns a `WARN` finding |
| `severity` | No | Exact finding filter: `INFO`, `WARN`, `CRITICAL`, or `UNKNOWN` |
| `limit` | No | Maximum evidence rows per diagnostic area, default `20`, maximum `100` |

Request body: none.

Success HTTP status: `200 OK`.

Report status behavior:

| Status | Meaning |
|---|---|
| `PASS` | All returned findings are `INFO`, or the severity filter returned no findings |
| `WARN` | At least one returned finding is `WARN` and none is `CRITICAL` |
| `FAIL` | At least one returned finding is `CRITICAL` |
| `UNKNOWN` | At least one returned finding is `UNKNOWN` and none is `WARN` / `CRITICAL` |

Finding severity values:

| Severity | Meaning |
|---|---|
| `INFO` | Read-only evidence is available and within configured thresholds |
| `WARN` | DBA review is recommended, but no automatic action is taken |
| `CRITICAL` | Human review is required before further operations |
| `UNKNOWN` | Required evidence is unavailable, for example `system.query_log` is disabled |

Response fields:

| Field | Type | Description |
|---|---|---|
| `namespace` | string | Managed cluster namespace |
| `name` / `cluster` | string | Managed cluster name |
| `status` | string | Aggregated diagnostics status |
| `generatedAt` | timestamp | Diagnostics generation time |
| `filters` | object | Applied query filters |
| `thresholds` | object | Runtime diagnostics thresholds from API-server configuration |
| `summary.info` | integer | Count of `INFO` findings |
| `summary.warnings` | integer | Count of `WARN` findings |
| `summary.critical` | integer | Count of `CRITICAL` findings |
| `summary.unknown` | integer | Count of `UNKNOWN` findings |
| `findings[]` | array | Category findings with severity, evidence, recommendation, and human-review flag |
| `requestId` | string | Request correlation ID |

Required finding categories in the normal full response:

| Category | Evidence Source |
|---|---|
| `replica` | `system.replicas` |
| `replication_queue` | `system.replication_queue` |
| `parts` | `system.parts` |
| `merges` | `system.merges` |
| `mutations` | `system.mutations` |
| `keeper` | Keeper `mntr` probe |
| `storage` | Kubernetes PVC status/capacity |
| `write_client_stats` | `system.query_log` if enabled, otherwise `UNKNOWN` |
| `write_quality` | Optional read-only row count when `database` and `table` are supplied |

Usage:

```bash
curl -fsS \
  "${UPM_API_SERVER_URL}/api/v1/clusters/upm-clickhouse-phase03-runtime/clickhouse-phase03/diagnostics?limit=20" \
  | jq .
```

Optional read-only row-count validation:

```bash
curl -fsS \
  "${UPM_API_SERVER_URL}/api/v1/clusters/upm-clickhouse-phase03-runtime/clickhouse-phase03/diagnostics?database=upm_healthcheck&table=dist_events&expectedRows=8&limit=20" \
  | jq '.status, .summary, [.findings[] | {category,severity,title}]'
```

## 9. Backup And Restore APIs

Phase 07 implements backup, restore, task status, and API-managed scheduled
backup through Kubernetes `Job` and `CronJob` resources created by
`upm-api-server`.

Credential rule:

- Request bodies may reference Kubernetes Secrets by name.
- Request bodies must not contain ClickHouse passwords, AES keys, S3 access
  keys, S3 secret keys, or any credential plaintext.

Required backup storage Secret:

| Key | Required | Description |
|---|---:|---|
| `S3_ENDPOINT` | Yes | S3-compatible endpoint URL, for example `http://upm-backup-minio.namespace.svc.cluster.local:9000` |
| `S3_BUCKET` | Yes | Bucket name |
| `S3_ACCESS_KEY` | Yes | Object storage access key |
| `S3_SECRET_KEY` | Yes | Object storage secret key |
| `S3_USE_SSL` | No | Reserved optional flag; endpoint scheme remains authoritative |

### 9.1 Create Backup Task

```http
POST /api/v1/clusters/{namespace}/{name}/backup
```

Purpose:

- Create an immediate Kubernetes `Job` that runs ClickHouse native `BACKUP`
  against a single validation database/table scope.

Path parameters:

| Parameter | Required | Description |
|---|---:|---|
| `namespace` | Yes | Managed ClickHouse cluster namespace |
| `name` | Yes | Managed ClickHouse cluster name |

Query parameters: none.

Request body:

| Field | Required | Description |
|---|---:|---|
| `scope.database` | Yes | Source database; ClickHouse identifier `[A-Za-z_][A-Za-z0-9_]*` |
| `scope.table` | Yes | Source table; ClickHouse identifier `[A-Za-z_][A-Za-z0-9_]*` |
| `storage.type` | No | Only `s3` is supported; defaults to `s3` |
| `storage.secretRef` | Yes | Kubernetes Secret containing S3 credentials |
| `storage.path` | No | Relative object path. Defaults to `backups/{cluster}/manual-{timestamp}` |
| `execution.type` | No | Only `kubernetesJob` is supported; defaults to `kubernetesJob` |
| `dryRun` | No | Kubernetes server dry-run for Job creation when `true`; default `false` |

Success HTTP status: `202 Accepted`.

Example:

```bash
curl -fsS -X POST \
  -H 'Content-Type: application/json' \
  "${UPM_API_SERVER_URL}/api/v1/clusters/upm-clickhouse-phase03-runtime/clickhouse-phase03/backup" \
  -d '{
    "scope": {"database": "upm_backup_validation", "table": "events"},
    "storage": {
      "type": "s3",
      "secretRef": "clickhouse-backup-secret",
      "path": "backups/clickhouse-phase03/manual-20260607T120000Z"
    },
    "execution": {"type": "kubernetesJob"},
    "dryRun": false
  }' | jq .
```

### 9.2 Create Restore Task

```http
POST /api/v1/clusters/{namespace}/{name}/restore
```

Purpose:

- Create a Kubernetes `Job` that restores an existing backup object into a
  different validation database/table target.
- Restore requires explicit confirmation and a reason.
- The target table must not already exist. For validation restores, the task
  creates an empty target table and restores data into it without reusing the
  source ReplicatedMergeTree Keeper path.

Path parameters:

| Parameter | Required | Description |
|---|---:|---|
| `namespace` | Yes | Managed ClickHouse cluster namespace |
| `name` | Yes | Managed ClickHouse cluster name |

Query parameters: none.

Request body:

| Field | Required | Description |
|---|---:|---|
| `backupRef` | Yes | Relative backup object path. Defaults `storage.path` when omitted |
| `source.database` | Yes | Database/table name inside the backup object |
| `source.table` | Yes | Source table name inside the backup object |
| `target.database` | Yes | Restore target database; must differ from source object |
| `target.table` | Yes | Restore target table; must differ from source object |
| `storage.type` | No | Only `s3` is supported; defaults to `s3` |
| `storage.secretRef` | Yes | Kubernetes Secret containing S3 credentials |
| `storage.path` | No | Relative backup object path; defaults to `backupRef` |
| `execution.type` | No | Only `kubernetesJob` is supported; defaults to `kubernetesJob` |
| `confirm` | Yes | Must be `true` |
| `reason` | Yes | Human-readable restore reason, maximum 512 characters |
| `dryRun` | No | Kubernetes server dry-run for Job creation when `true`; default `false` |

Success HTTP status: `202 Accepted`.

Example:

```bash
curl -fsS -X POST \
  -H 'Content-Type: application/json' \
  "${UPM_API_SERVER_URL}/api/v1/clusters/upm-clickhouse-phase03-runtime/clickhouse-phase03/restore" \
  -d '{
    "backupRef": "backups/clickhouse-phase03/manual-20260607T120000Z",
    "source": {"database": "upm_backup_validation", "table": "events"},
    "target": {"database": "upm_restore_validation", "table": "events_restored"},
    "storage": {
      "type": "s3",
      "secretRef": "clickhouse-backup-secret"
    },
    "execution": {"type": "kubernetesJob"},
    "confirm": true,
    "reason": "phase 07 restore validation"
  }' | jq .
```

### 9.3 Get Backup Or Restore Task

```http
GET /api/v1/clusters/{namespace}/{name}/tasks/{taskName}
```

Purpose:

- Read Kubernetes `Job`, latest Pod, exit status, and redacted log evidence for
  a backup or restore task.

Path parameters:

| Parameter | Required | Description |
|---|---:|---|
| `namespace` | Yes | Managed ClickHouse cluster namespace |
| `name` | Yes | Managed ClickHouse cluster name |
| `taskName` | Yes | API-created Kubernetes Job name |

Query parameters: none.

Request body: none.

Success HTTP status: `200 OK`.

Task status values:

| Value | Meaning |
|---|---|
| `Pending` | Job exists but no active/success/failed status is available yet |
| `Running` | Job has active Pods |
| `Succeeded` | Job completed successfully |
| `Failed` | Job failed |
| `Unknown` | Reserved for future status mapping |

Response fields:

| Field | Type | Description |
|---|---|---|
| `name` | string | Kubernetes Job name |
| `namespace` | string | Kubernetes namespace |
| `cluster` | string | Managed cluster name |
| `type` | string | `backup` or `restore` |
| `status` | string | Task status |
| `message` | string | Kubernetes Job condition message or derived task message |
| `startTime` | timestamp | Job start time when available |
| `completionTime` | timestamp | Job completion time when available |
| `jobRef` | object | Job namespace/name |
| `podRef` | object | Latest task Pod namespace/name when available |
| `evidence.backupPath` | string | Manual backup/restore object path |
| `evidence.backupPathPrefix` | string | Scheduled backup path prefix when applicable |
| `evidence.source` | string | Source database/table |
| `evidence.target` | string | Restore target database/table when applicable |
| `evidence.storageSecretRef` | string | Referenced storage Secret name, not Secret value |
| `evidence.logsRedacted` | boolean | `true` when log evidence is redacted |
| `evidence.logTail` | string | Last task log lines, with known secret values redacted |
| `requestId` | string | Request correlation ID |

Usage:

```bash
curl -fsS \
  "${UPM_API_SERVER_URL}/api/v1/clusters/upm-clickhouse-phase03-runtime/clickhouse-phase03/tasks/clickhouse-phase03-backup-abcde" \
  | jq .
```

### 9.4 Create Backup Schedule

```http
POST /api/v1/clusters/{namespace}/{name}/backup-schedules
```

Purpose:

- Create an API-managed Kubernetes `CronJob` for recurring ClickHouse backup.
- Every scheduled run appends a UTC timestamp and Pod hostname to
  `storage.pathPrefix` to avoid overwriting previous backups.

Path parameters:

| Parameter | Required | Description |
|---|---:|---|
| `namespace` | Yes | Managed ClickHouse cluster namespace |
| `name` | Yes | Managed ClickHouse cluster name |

Query parameters: none.

Request body:

| Field | Required | Description |
|---|---:|---|
| `name` | Yes | DNS-safe schedule name |
| `schedule` | Yes | Five-field Kubernetes CronJob expression |
| `timeZone` | No | Kubernetes CronJob timezone, for example `Asia/Shanghai` |
| `scope.database` | Yes | Backup source database |
| `scope.table` | Yes | Backup source table |
| `storage.type` | No | Only `s3` is supported; defaults to `s3` |
| `storage.secretRef` | Yes | Kubernetes Secret containing S3 credentials |
| `storage.pathPrefix` | Yes | Relative object path prefix for scheduled runs |
| `execution.type` | No | Only `kubernetesCronJob` is supported; defaults to `kubernetesCronJob` |
| `execution.concurrencyPolicy` | No | `Allow`, `Forbid`, or `Replace`; defaults to `Forbid` |
| `execution.successfulJobsHistoryLimit` | No | Successful child Jobs retained by Kubernetes; defaults to `1` |
| `execution.failedJobsHistoryLimit` | No | Failed child Jobs retained by Kubernetes; defaults to `1` |
| `suspend` | No | Create the CronJob suspended when `true`; default `false` |

Success HTTP status: `202 Accepted`.

Example:

```bash
curl -fsS -X POST \
  -H 'Content-Type: application/json' \
  "${UPM_API_SERVER_URL}/api/v1/clusters/upm-clickhouse-phase03-runtime/clickhouse-phase03/backup-schedules" \
  -d '{
    "name": "validation-every-minute",
    "schedule": "*/1 * * * *",
    "timeZone": "Asia/Shanghai",
    "scope": {"database": "upm_backup_validation", "table": "events"},
    "storage": {
      "type": "s3",
      "secretRef": "clickhouse-backup-secret",
      "pathPrefix": "backups/clickhouse-phase03/scheduled"
    },
    "execution": {
      "type": "kubernetesCronJob",
      "concurrencyPolicy": "Forbid",
      "successfulJobsHistoryLimit": 1,
      "failedJobsHistoryLimit": 1
    },
    "suspend": false
  }' | jq .
```

### 9.5 List Backup Schedules

```http
GET /api/v1/clusters/{namespace}/{name}/backup-schedules
```

Purpose:

- List API-managed backup schedules for one managed ClickHouse cluster.

Path parameters:

| Parameter | Required | Description |
|---|---:|---|
| `namespace` | Yes | Managed ClickHouse cluster namespace |
| `name` | Yes | Managed ClickHouse cluster name |

Query parameters: none.

Request body: none.

Success HTTP status: `200 OK`.

Response:

```json
{
  "items": []
}
```

### 9.6 Get Backup Schedule

```http
GET /api/v1/clusters/{namespace}/{name}/backup-schedules/{scheduleName}
```

Purpose:

- Read one API-managed backup CronJob, active jobs, and recent child Job
  evidence.

Path parameters:

| Parameter | Required | Description |
|---|---:|---|
| `namespace` | Yes | Managed ClickHouse cluster namespace |
| `name` | Yes | Managed ClickHouse cluster name |
| `scheduleName` | Yes | Backup schedule name from the create request |

Query parameters: none.

Request body: none.

Success HTTP status: `200 OK`.

Response fields:

| Field | Type | Description |
|---|---|---|
| `name` | string | API schedule name |
| `namespace` | string | Kubernetes namespace |
| `cluster` | string | Managed cluster name |
| `schedule` | string | Kubernetes CronJob expression |
| `timeZone` | string | CronJob timezone when set |
| `suspend` | boolean | CronJob suspend flag |
| `activeJobs` | array | Active child Job references |
| `lastScheduleTime` | timestamp | Last CronJob schedule time when available |
| `lastSuccessfulTime` | timestamp | Last successful CronJob time when available |
| `recentJobs` | array | Recent child Jobs using the task status shape from section 9.3 |
| `storageSecretRef` | string | Referenced storage Secret name, not Secret value |
| `backupPathPrefix` | string | Scheduled backup object prefix |
| `successfulJobsHistoryLimit` | integer | Successful child Jobs retained by Kubernetes |
| `failedJobsHistoryLimit` | integer | Failed child Jobs retained by Kubernetes |
| `requestId` | string | Request correlation ID |

Usage:

```bash
curl -fsS \
  "${UPM_API_SERVER_URL}/api/v1/clusters/upm-clickhouse-phase03-runtime/clickhouse-phase03/backup-schedules/validation-every-minute" \
  | jq .
```

### 9.7 Delete Backup Schedule

```http
DELETE /api/v1/clusters/{namespace}/{name}/backup-schedules/{scheduleName}
```

Purpose:

- Delete one API-managed backup CronJob.
- The API verifies the CronJob belongs to the requested managed cluster before
  deleting it.

Path parameters:

| Parameter | Required | Description |
|---|---:|---|
| `namespace` | Yes | Managed ClickHouse cluster namespace |
| `name` | Yes | Managed ClickHouse cluster name |
| `scheduleName` | Yes | Backup schedule name from the create request |

Query parameters: none.

Request body: none.

Success HTTP status: `204 No Content`.

Usage:

```bash
curl -fsS -X DELETE \
  "${UPM_API_SERVER_URL}/api/v1/clusters/upm-clickhouse-phase03-runtime/clickhouse-phase03/backup-schedules/validation-every-minute"
```

## 10. Not Supported After Phase 07

No Phase 08 lifecycle/scaling APIs are supported by this reference yet. They
must not be claimed until `docs/phases/phase-08-config-lifecycle-scaling.md` is
implemented and validated.
