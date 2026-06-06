# Phase 06: Day2 Read-only Diagnostics

- Version: 0.4
- Date: 2026-05-27
- Status: Confirmed
- Priority: P1
- Owner: zqw
- Depends on:
  - ../master-spec.md
  - ../evidence-summary.md
  - ../design/day2-diagnostics-design.md
  - ../design/api-design.md
  - ../design/data-architecture.md
  - ../architecture/clickhouse-ha-architecture.md
  - phase-03-upm-api-server.md
  - phase-04-healthcheck.md

## 1. Purpose

Implement read-only Day2 diagnostics for ClickHouse operations.

This phase covers observability and diagnosis, not automatic remediation.

## 2. Scope

In scope:

1. Replica diagnostics.
2. Replication queue diagnostics.
3. Part and partition diagnostics.
4. Merge diagnostics.
5. Mutation diagnostics.
6. Keeper status diagnostics.
7. Storage/capacity diagnostics.
8. Write-path visibility diagnostics.
9. Write client statistics requirement mapping.
10. Write quality validation requirement mapping.
11. Structured diagnostics API and output model.

## 3. Non-Goals

Out of scope:

- Backup/restore.
- Kill mutation.
- OPTIMIZE execution.
- TTL changes.
- User business data correction.
- Automatic shard/replica repair.
- High-risk write operations.

## 4. Diagnostics API

Required endpoint:

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/api/v1/clusters/{namespace}/{name}/diagnostics` | Return read-only Day2 diagnostics |

Optional filters:

```text
?database=<db>&table=<table>&severity=WARN&limit=20
```

## 5. Output Model

Minimum output:

```json
{
  "cluster": "clickhouse-runtime",
  "namespace": "upm-clickhouse-runtime",
  "status": "WARN",
  "generatedAt": "...",
  "findings": [
    {
      "category": "replica",
      "severity": "WARN",
      "title": "Replica queue is accumulating",
      "evidence": {},
      "recommendation": "Check Keeper connectivity and write pressure"
    }
  ]
}
```

Severity values:

- `INFO`
- `WARN`
- `CRITICAL`
- `UNKNOWN`

## 6. Required SQL / Data Sources

| Diagnostic Area | Source | Required Fields / Query Intent |
|---|---|---|
| Replica state | `system.replicas` | `is_readonly`, `is_session_expired`, `absolute_delay`, `queue_size`, `future_parts` |
| Replication queue | `system.replication_queue` | queue size by table, oldest task age, exception text |
| Parts | `system.parts` | active/inactive parts by database/table/partition |
| Large tables | `system.parts` | bytes/rows aggregated by table |
| Large partitions | `system.parts` | bytes/rows aggregated by partition |
| Merges | `system.merges` | running merges, elapsed, progress |
| Mutations | `system.mutations` | unfinished/failed mutations, command, latest_failed_reason |
| Storage | Kubernetes PVC + ClickHouse system tables | PVC status/capacity, disk usage if available |
| Keeper | Keeper `ruok`/`mntr` or metrics | leader/follower/session/request state |
| Write client stats | `system.query_log` if enabled | user, client IP, target table for inserts |
| Write quality validation | `system.parts` / user query input | row count by time/partition/table if configured |

Do not assume `system.query_log` is enabled. If unavailable, return `UNKNOWN` with clear explanation.

## 7. Decision Rules

| Rule | Severity |
|---|---|
| Any replica `is_readonly=1` | CRITICAL |
| Any replica `is_session_expired=1` | CRITICAL |
| Replication queue grows beyond configured threshold | WARN/CRITICAL based on threshold |
| `absolute_delay` exceeds threshold | WARN/CRITICAL |
| Active parts per partition exceed threshold | WARN |
| Inactive parts accumulate beyond threshold | WARN |
| Mutation has `is_done=0` for longer than threshold | WARN |
| Mutation has latest failure reason | CRITICAL |
| Keeper leader cannot be identified | CRITICAL |
| PVC not Bound or endpoint missing | CRITICAL |
| Write client stats unavailable because query_log disabled | UNKNOWN |
| Write quality validation lacks user-provided expected count | INFO/UNKNOWN |

Thresholds must be configurable. Do not hardcode production thresholds without design approval.

## 8. Write-path Requirement Coverage

This phase must explicitly cover the two requirements previously flagged by red-team:

| Requirement | MVP Behavior | Future Behavior |
|---|---|---|
| Write client statistics by user/client IP/target table | Read-only summary from `system.query_log` if available; otherwise `UNKNOWN` | Persistent write analytics and dashboard |
| Write quality validation by time/partition/table row count | Provide API/design hook and optional read-only row-count query when user supplies criteria | Scheduled validation jobs and alerts |

## 9. Files Likely Changed

```text
backend/internal/api/diagnostics_handler.go
backend/internal/model/diagnostics.go
backend/internal/clickhouse/system_tables.go
backend/internal/clickhouse/diagnostics.go
backend/internal/k8s/storage.go
backend/internal/keeper/status.go
docs/design/day2-diagnostics-design.md
```

## 10. Acceptance Criteria

1. `/diagnostics` returns structured output with category/severity/evidence/recommendation.
2. Replica diagnostics query `system.replicas` and classify readonly/session-expired states.
3. Part diagnostics aggregate active/inactive parts by table/partition.
4. Mutation diagnostics identifies unfinished or failed mutations.
5. Merge diagnostics lists running merges.
6. Keeper diagnostics report leader/follower status or `UNKNOWN` with reason.
7. Storage diagnostics includes PVC status/capacity at minimum.
8. Write client stats requirement is implemented or explicitly returns `UNKNOWN` when query_log unavailable.
9. Write quality validation requirement has a documented API/design path and at least a read-only row-count mechanism when criteria are provided.
10. No diagnostic query mutates data.
11. All high-risk operations are recommendations only, not executed.
12. Unit tests cover severity classification logic.

## 11. Verification Commands

```bash
# API
curl -sS http://127.0.0.1:<port>/api/v1/clusters/upm-clickhouse-runtime/clickhouse-runtime/diagnostics | jq .

# SQL evidence
kubectl exec -n upm-clickhouse-runtime <clickhouse-pod> -c clickhouse -- clickhouse-client --query "SELECT database, table, is_readonly, is_session_expired, absolute_delay, queue_size FROM system.replicas FORMAT Vertical"
kubectl exec -n upm-clickhouse-runtime <clickhouse-pod> -c clickhouse -- clickhouse-client --query "SELECT database, table, partition, count() AS active_parts, sum(rows) AS rows, sum(bytes_on_disk) AS bytes FROM system.parts WHERE active GROUP BY database, table, partition ORDER BY active_parts DESC LIMIT 20"
kubectl exec -n upm-clickhouse-runtime <clickhouse-pod> -c clickhouse -- clickhouse-client --query "SELECT database, table, mutation_id, command, is_done, latest_failed_reason FROM system.mutations ORDER BY create_time DESC LIMIT 20"
kubectl exec -n upm-clickhouse-runtime <clickhouse-pod> -c clickhouse -- clickhouse-client --query "SELECT database, table, elapsed, progress FROM system.merges ORDER BY elapsed DESC LIMIT 20"

# Tests
cd backend
go test ./...
```

## 12. Risks and Open Questions

| Risk / Question | Handling |
|---|---|
| `system.query_log` disabled | Return `UNKNOWN`, document requirement for enabling query_log |
| Thresholds too environment-specific | Make thresholds configurable |
| Queries expensive on large clusters | Use limits and aggregation; avoid full scans where possible |
| Diagnostic recommendation may be mistaken for action | Label as recommendation; no execution in this phase |

## 13. Changelog

| Version | Date | Changes |
|---|---|---|
| 0.1 | 2026-05-27 | Initial phase draft |
| 0.2 | 2026-05-27 | Added Day2 diagnostics scope and SQL areas |
| 0.3 | 2026-05-27 | Added metadata and red-team fix structure |
| 0.4 | 2026-05-27 | Restored concrete SQL/data sources, decision rules, write-path requirements, and objective verification commands |
