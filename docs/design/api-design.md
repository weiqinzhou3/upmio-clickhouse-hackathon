# API Design

- Version: 0.3
- Date: 2026-05-27
- Status: Sealed
- Owner: zqw
- Related:
  - ../master-spec.md
  - product-design.md
  - resource-model.md

## 1. Purpose

This document defines the Manager Backend API design boundary.

The Manager API is a product control plane. It does not replace UPMIO operators.

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
| `GET /api/v1/clusters/{name}` | cluster detail | yes |
| `GET /api/v1/clusters/{name}/resources` | UPMIO/K8s resource status | yes |
| `POST /api/v1/clusters/{name}/healthcheck` | run Day1 acceptance healthcheck | yes |
| `GET /api/v1/clusters/{name}/healthcheck/latest` | latest healthcheck result | optional MVP |
| `GET /api/v1/clusters/{name}/diagnostics` | Day2 read-only diagnostics | yes |
| `GET /api/v1/clusters/{name}/metrics/summary` | Prometheus metric summary | yes |
| `POST /api/v1/clusters/{name}/backup` | future backup operation | future |
| `POST /api/v1/clusters/{name}/restore` | future restore operation | future |

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

Manager API may:

- validate requests;
- render UPMIO/Kubernetes resources;
- submit resources via Kubernetes API;
- read status from UPMIO/K8s;
- query ClickHouse SQL;
- query Prometheus API;
- return healthcheck/diagnostics summaries.

Manager API must not:

- reimplement UnitSet reconciliation;
- create raw Pods/PVCs/Services as the product path;
- execute destructive operations without explicit approval;
- depend on ClickHouse GrpcCall in MVP.
