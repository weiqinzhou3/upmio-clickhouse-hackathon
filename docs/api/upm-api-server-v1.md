# UPM API Server v1 API Reference

- Version: 0.1
- Date: 2026-06-05
- Status: Phase 03 Target
- Owner: zqw
- Related:
  - ../master-spec.md
  - ../phases/phase-03-upm-api-server.md
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

Recommended terminal access for validation:

```bash
kubectl -n upm-system port-forward svc/upm-api-server 18083:8080
```

Base URL:

```text
http://127.0.0.1:18083
```

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

Success response:

```json
{
  "status": "ok",
  "service": "upm-api-server"
}
```

Usage:

```bash
curl -sS http://127.0.0.1:18083/api/v1/healthz | jq .
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
- Storage sizes must be valid Kubernetes quantities.
- `security.adminSecretRef` must reference an existing Secret unless the phase
  explicitly enables generated-secret behavior.

Success response:

```json
{
  "requestId": "req-xxxxxxxx",
  "namespace": "upm-clickhouse-runtime",
  "name": "clickhouse-runtime",
  "status": "Running",
  "resources": {
    "project": "upm-clickhouse-runtime",
    "keeperUnitSet": "clickhouse-runtime-keeper",
    "serverUnitSet": "clickhouse-runtime"
  }
}
```

Usage:

```bash
curl -sS -X POST http://127.0.0.1:18083/api/v1/clusters \
  -H "Content-Type: application/json" \
  --data @cluster-create.json | jq .
```

Invalid request usage:

```bash
curl -sS -X POST http://127.0.0.1:18083/api/v1/clusters \
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
curl -sS http://127.0.0.1:18083/api/v1/clusters | jq .
```

Namespace-filtered usage:

```bash
curl -sS "http://127.0.0.1:18083/api/v1/clusters?namespace=upm-clickhouse-runtime" | jq .
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
  http://127.0.0.1:18083/api/v1/clusters/upm-clickhouse-runtime/clickhouse-runtime \
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
  http://127.0.0.1:18083/api/v1/clusters/upm-clickhouse-runtime/clickhouse-runtime/resources \
  | jq .
```

## 7. Not Supported in Phase 03

These APIs are registered in later phase specs and must not be claimed as
supported until implemented and validated:

| API | Phase |
|---|---|
| `POST /api/v1/clusters/{namespace}/{name}/healthcheck` | Phase 04 |
| `GET /api/v1/clusters/{namespace}/{name}/healthcheck/latest` | Phase 04 |
| `GET /api/v1/clusters/{namespace}/{name}/metrics/summary` | Phase 05 |
| `GET /api/v1/clusters/{namespace}/{name}/diagnostics` | Phase 06 |
| `POST /api/v1/clusters/{namespace}/{name}/backup` | Phase 07 |
| `POST /api/v1/clusters/{namespace}/{name}/restore` | Phase 07 |
