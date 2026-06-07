# API Design

- Version: 0.5
- Date: 2026-06-07
- Status: Sealed
- Owner: zqw
- Related:
  - ../master-spec.md
  - product-design.md
  - resource-model.md

## 1. Purpose

This document defines the `upm-api-server` API design boundary.

`upm-api-server` is a UPM product control plane. It does not replace UPMIO
operators or the Kubernetes API server.

## 2. API Versioning

- MVP uses `/api/v1`.
- Breaking changes require `/api/v2`.
- No breaking changes should be made inside `/api/v1` after implementation starts.
- Deprecated fields must be documented before removal.

## 3. Authentication and Authorization

MVP assumes a trusted internal hackathon environment.

MVP does not implement full authentication/RBAC. However, API structures should keep extension points for:

- actor/user identity;
- request ID;
- approval workflow;
- operation audit.

Future productization must support authentication, role-based access, and approval flows for high-risk operations.

## 4. Core APIs

| API | Purpose | MVP |
|---|---|---|
| `POST /api/v1/clusters` | create ClickHouse cluster from logical spec | yes |
| `GET /api/v1/clusters` | list clusters | yes |
| `GET /api/v1/clusters/{namespace}/{name}` | cluster detail | yes |
| `GET /api/v1/clusters/{namespace}/{name}/resources` | UPMIO/K8s resource status | yes |
| `POST /api/v1/clusters/{namespace}/{name}/healthcheck` | run Day1 acceptance healthcheck | yes |
| `GET /api/v1/clusters/{namespace}/{name}/healthcheck/latest` | latest healthcheck result | optional MVP |
| `GET /api/v1/clusters/{namespace}/{name}/diagnostics` | Day2 read-only diagnostics | yes |
| `GET /api/v1/clusters/{namespace}/{name}/metrics/summary` | Prometheus metric summary | yes |
| `POST /api/v1/clusters/{namespace}/{name}/backup` | create an immediate backup task | yes, Phase 07 |
| `POST /api/v1/clusters/{namespace}/{name}/restore` | create a restore task with explicit confirmation | yes, Phase 07 |
| `GET /api/v1/clusters/{namespace}/{name}/tasks/{taskName}` | read backup/restore task status | yes, Phase 07 |
| `POST /api/v1/clusters/{namespace}/{name}/backup-schedules` | create an API-managed scheduled backup | yes, Phase 07 |
| `GET /api/v1/clusters/{namespace}/{name}/backup-schedules` | list API-managed backup schedules | yes, Phase 07 |
| `GET /api/v1/clusters/{namespace}/{name}/backup-schedules/{scheduleName}` | read scheduled backup status | yes, Phase 07 |
| `DELETE /api/v1/clusters/{namespace}/{name}/backup-schedules/{scheduleName}` | delete an API-managed backup schedule | yes, Phase 07 |

## 4.1 Future Database Management APIs

Enterprise production product scope should include controlled database
management APIs. These APIs are future/productization scope unless a later phase
explicitly implements them.

| API | Purpose | MVP |
|---|---|---|
| `GET /api/v1/clusters/{namespace}/{name}/databases` | list ClickHouse databases visible to the `upm-api-server` account | future |
| `POST /api/v1/clusters/{namespace}/{name}/databases` | create a database with explicit user/DBA intent | future |
| `POST /api/v1/clusters/{namespace}/{name}/databases/{database}/validate` | validate database existence and `upm-api-server` account access | future |

Rules:

- Cluster creation must not silently create user business tables.
- User business local table and Distributed table lifecycle remains
  DBA/application-owned, not API-server-owned.
- `upm-api-server` may create reserved validation objects for healthcheck and
  may inspect table metadata read-only for diagnostics.
- API-server-generated DDL is limited to database lifecycle and validation
  objects.
- Existing API-server-owned object definitions must be compared before
  applying DDL; drift must fail instead of being hidden by `IF NOT EXISTS`.

## 5. Request Model Example

```json
{
  "name": "ch-demo",
  "namespace": "upm-clickhouse",
  "version": "26.3.x",
  "topology": {
    "shards": 1,
    "replicasPerShard": 2
  },
  "keeper": {
    "replicas": 3
  },
  "storage": {
    "storageClassName": "local-path",
    "dataSize": "100Gi"
  },
  "security": {
    "adminSecretRef": "ch-demo-admin"
  },
  "metrics": {
    "enabled": true
  }
}
```

## 6. Error Response Format

```json
{
  "code": "UPMIO_UNITSET_NOT_READY",
  "message": "ClickHouse UnitSet is not ready",
  "details": {},
  "requestId": "req-xxxxxxxx"
}
```

Rules:

- Never return raw stack traces.
- Never return secrets.
- Include actionable error code.
- Include request ID for correlation.
- Log internal errors server-side with redaction.

## 7. Operation Result Format

```json
{
  "requestId": "req-xxxxxxxx",
  "cluster": "ch-demo",
  "status": "PASS",
  "summary": "All MVP checks passed",
  "checks": [
    {
      "name": "select_1",
      "status": "PASS",
      "message": "SELECT 1 returned successfully"
    }
  ]
}
```

## 8. Implementation Boundary

`upm-api-server` may:

- validate requests;
- render UPMIO/Kubernetes resources;
- submit resources via Kubernetes API;
- read status from UPMIO/K8s;
- query ClickHouse SQL;
- query Prometheus API;
- return healthcheck/diagnostics summaries.

`upm-api-server` must not:

- reimplement UnitSet reconciliation;
- create raw Pods/PVCs/Services as the product path;
- execute destructive operations without explicit approval;
- depend on ClickHouse GrpcCall in MVP.
