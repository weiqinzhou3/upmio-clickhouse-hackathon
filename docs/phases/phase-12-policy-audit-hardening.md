# Phase 12: Policy, Retention, Audit, and Hardening

- Version: 0.1
- Date: 2026-06-07
- Status: Draft for owner decision
- Priority: P2/P3
- Owner: zqw
- Depends on:
  - ../master-spec.md
  - phase-07-backup-restore.md
  - phase-08-config-lifecycle-scaling.md

## 1. Purpose

Improve production readiness around backup policy, retention, operation
history, alerting, and access control.

## 2. Scope

Candidate capabilities:

1. Backup retention cleanup for API-managed backup prefixes.
2. Backup policy status and last-run summary.
3. Persistent operation history.
4. Audit records for mutating operations.
5. PrometheusRule / alerting lifecycle if required.
6. Authentication/RBAC design and future implementation boundary.

## 3. Non-Goals

Out of scope unless separately approved:

- Deleting backup objects outside API-managed prefixes.
- Enterprise identity integration.
- Full multi-tenant authorization model.
- Replacing Grafana or Prometheus.
- Compliance-grade audit storage.

## 4. Backup Retention

Retention cleanup must be conservative:

- operate only on API-managed path prefixes;
- support dry-run;
- require confirmation for deletion;
- record deleted object count;
- never delete the newest successful backup below minimum retention.

Candidate request fields:

```json
{
  "retention": {
    "keepLast": 7,
    "keepDays": 14
  },
  "dryRun": true,
  "actor": "zqw",
  "reason": "demo retention cleanup check",
  "confirm": false
}
```

## 5. Operation History

Minimum useful history fields:

- operation ID;
- namespace/name;
- operation type;
- actor;
- reason;
- request summary without secrets;
- started/finished time;
- status;
- related Kubernetes resource names;
- healthcheck result after operation.

Storage options:

| Option | Pros | Cons |
|---|---|---|
| Kubernetes ConfigMap/CR | Simple and cluster-native | Not ideal for large history |
| SQLite/PostgreSQL | Better querying | Adds dependency |
| Object storage log | Simple append evidence | Harder interactive queries |

## 6. Candidate APIs

| Method | Path | Purpose |
|---|---|---|
| `POST` | `/api/v1/clusters/{namespace}/{name}/backup-retention/plan` | Dry-run retention cleanup |
| `POST` | `/api/v1/clusters/{namespace}/{name}/backup-retention/apply` | Apply approved retention cleanup |
| `GET` | `/api/v1/clusters/{namespace}/{name}/operations` | List operation history |
| `GET` | `/api/v1/clusters/{namespace}/{name}/operations/{operationId}` | Get operation detail |

## 7. Acceptance Criteria

1. Retention dry-run lists candidate objects without deleting anything.
2. Retention apply requires confirmation and only deletes API-managed objects.
3. Operation history records backup, restore, schedule, and future mutating
   actions.
4. Secret values are never written to operation history.
5. If PrometheusRule lifecycle is implemented, alert rules are validated against
   Prometheus Operator CRDs.

## 8. Recommended Hackathon Decision

Backup retention cleanup is optional if the frontend is complete.

Persistent audit and auth/RBAC should remain future hardening unless judges
explicitly ask for production governance.
