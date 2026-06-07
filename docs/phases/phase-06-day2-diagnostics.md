# Phase 06: Day2 Read-only Diagnostics

- Version: 0.5
- Date: 2026-06-07
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
?database=<db>&table=<table>&partition=<partition>&timeColumn=<column>&startTime=<time>&endTime=<time>&expectedRows=<n>&severity=WARN&limit=20
```

Filter behavior:

- `database` / `table`: restrict system table diagnostics where applicable.
- `partition`: optional row-count validation filter for `_partition_id`.
- `timeColumn` + `startTime` / `endTime`: optional row-count validation
  time window.
- `expectedRows`: optional row-count assertion. A mismatch returns `WARN`;
  no destructive correction is executed.
- `severity`: exact finding severity filter, one of `INFO`, `WARN`,
  `CRITICAL`, `UNKNOWN`.
- `limit`: maximum rows per diagnostic evidence set, default `20`, maximum
  `100`.

## 5. Output Model

Minimum output:

```json
{
  "cluster": "clickhouse-runtime",
  "namespace": "upm-clickhouse-runtime",
  "status": "WARN",
  "generatedAt": "...",
  "filters": {
    "database": "upm_healthcheck",
    "table": "dist_events",
    "expectedRows": 8,
    "limit": 20
  },
  "thresholds": {
    "replicaDelayWarnSeconds": 60,
    "replicaDelayCriticalSeconds": 300,
    "replicationQueueWarn": 50,
    "replicationQueueCritical": 500,
    "activePartsPerPartitionWarn": 150,
    "inactivePartsWarn": 50,
    "mergeElapsedWarnSeconds": 1800,
    "mutationAgeWarnSeconds": 600
  },
  "summary": {
    "info": 7,
    "warnings": 0,
    "critical": 0,
    "unknown": 1
  },
  "findings": [
    {
      "category": "replica",
      "severity": "WARN",
      "title": "Replica queue is accumulating",
      "evidence": {},
      "recommendation": "Check Keeper connectivity and write pressure",
      "humanReviewRequired": true
    }
  ]
}
```

Severity values:

- `INFO`
- `WARN`
- `CRITICAL`
- `UNKNOWN`

Report status values:

- `PASS`: findings are all `INFO` or the severity filter returned no findings.
- `WARN`: at least one `WARN` and no `CRITICAL`.
- `FAIL`: at least one `CRITICAL`.
- `UNKNOWN`: at least one `UNKNOWN` and no `WARN` / `CRITICAL`.

## 6. Required SQL / Data Sources

| Diagnostic Area | Source | Required Fields / Query Intent |
|---|---|---|
| Replica state | `system.replicas` | `is_readonly`, `is_session_expired`, `absolute_delay`, `queue_size`, `future_parts` |
| Replication queue | `system.replication_queue` | queue size by table, oldest task age, exception text |
| Parts | `system.parts` | active/inactive parts by database/table/partition |
| Large tables | `system.parts` | bytes/rows aggregated by table |
| Large partitions | `system.parts` | bytes/rows aggregated by partition |
| Merges | `system.merges` | running merges, elapsed, progress |
| Mutations | `system.mutations` | unfinished/failed mutations, command, `latest_fail_reason` in ClickHouse 26.3 |
| Storage | Kubernetes PVC metadata | PVC status/capacity at minimum |
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
| PVC not Bound or expected PVC missing | CRITICAL |
| Write client stats unavailable because query_log disabled | UNKNOWN |
| Write quality validation lacks user-provided expected count | INFO/UNKNOWN |

Thresholds must be configurable. Do not hardcode production thresholds without
design approval. MVP configuration is through the `upm-api-server` ConfigMap /
environment variables:

| Environment Variable | Default | Meaning |
|---|---:|---|
| `DIAGNOSTICS_REPLICA_DELAY_WARN_SECONDS` | `60` | Replica delay WARN threshold |
| `DIAGNOSTICS_REPLICA_DELAY_CRITICAL_SECONDS` | `300` | Replica delay CRITICAL threshold |
| `DIAGNOSTICS_REPLICATION_QUEUE_WARN` | `50` | Replication queue WARN threshold |
| `DIAGNOSTICS_REPLICATION_QUEUE_CRITICAL` | `500` | Replication queue CRITICAL threshold |
| `DIAGNOSTICS_ACTIVE_PARTS_PER_PARTITION_WARN` | `150` | Active parts per partition WARN threshold |
| `DIAGNOSTICS_INACTIVE_PARTS_WARN` | `50` | Inactive parts WARN threshold |
| `DIAGNOSTICS_MERGE_ELAPSED_WARN_SECONDS` | `1800` | Running merge elapsed WARN threshold |
| `DIAGNOSTICS_MUTATION_AGE_WARN_SECONDS` | `600` | Unfinished mutation age WARN threshold |

## 8. Write-path Requirement Coverage

This phase must explicitly cover the two requirements previously flagged by red-team:

| Requirement | MVP Behavior | Future Behavior |
|---|---|---|
| Write client statistics by user/client IP/target table | Read-only summary from `system.query_log` if available; otherwise `UNKNOWN` | Persistent write analytics and dashboard |
| Write quality validation by time/partition/table row count | Provide API/design hook and optional read-only row-count query when user supplies criteria | Scheduled validation jobs and alerts |

## 9. Files Likely Changed

```text
api-server/internal/api/server.go
api-server/internal/api/server_test.go
api-server/internal/config/config.go
api-server/internal/kube/diagnostics.go
api-server/internal/kube/store.go
api-server/internal/kube/store_test.go
api-server/internal/model/diagnostics.go
api-server/internal/model/diagnostics_test.go
api-server/internal/platform/store.go
api-server/cmd/upm-api-server/main.go
clickhouse/phase-03/manifests/upm-api-server.yaml
clickhouse/phase-06/README.md
clickhouse/phase-06/scripts/validate-diagnostics-runtime.sh
docs/api/upm-api-server-v1.md
docs/design/day2-diagnostics-design.md
```

Do not create a new `backend/` tree. Do not restore deleted dead
`internal/clickhouse` code. Phase 06 extends the current Kubernetes-deployed
`api-server` service.

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
13. Runtime validation calls the real NodePort API in the lab environment and
    saves evidence under `clickhouse/phase-06/`.
14. Normal runtime diagnostics return all required categories and no
    `CRITICAL` findings for the current healthy lab cluster.
15. When `system.query_log` is unavailable, write client stats return an
    `UNKNOWN` finding rather than a failed API call.

## 11. Verification Commands

```bash
# API through real NodePort service
export UPM_API_SERVER_URL=http://192.168.35.201:30083
curl -fsS \
  "${UPM_API_SERVER_URL}/api/v1/clusters/upm-clickhouse-phase03-runtime/clickhouse-phase03/diagnostics?limit=20" \
  | jq .

# Optional read-only row-count validation against API-server-owned healthcheck
# objects created by Phase 04 healthcheck.
curl -fsS \
  "${UPM_API_SERVER_URL}/api/v1/clusters/upm-clickhouse-phase03-runtime/clickhouse-phase03/diagnostics?database=upm_healthcheck&table=dist_events&expectedRows=8&limit=20" \
  | jq .

# SQL evidence
kubectl exec -n upm-clickhouse-phase03-runtime clickhouse-phase03-0 -c clickhouse -- service-ctl.sh login --query "SELECT database, table, is_readonly, is_session_expired, absolute_delay, queue_size FROM system.replicas FORMAT Vertical"
kubectl exec -n upm-clickhouse-phase03-runtime clickhouse-phase03-0 -c clickhouse -- service-ctl.sh login --query "SELECT database, table, partition, count() AS active_parts, sum(rows) AS rows, sum(bytes_on_disk) AS bytes FROM system.parts WHERE active GROUP BY database, table, partition ORDER BY active_parts DESC LIMIT 20"
kubectl exec -n upm-clickhouse-phase03-runtime clickhouse-phase03-0 -c clickhouse -- service-ctl.sh login --query "SELECT database, table, mutation_id, command, is_done, latest_fail_reason FROM system.mutations ORDER BY create_time DESC LIMIT 20"
kubectl exec -n upm-clickhouse-phase03-runtime clickhouse-phase03-0 -c clickhouse -- service-ctl.sh login --query "SELECT database, table, elapsed, progress FROM system.merges ORDER BY elapsed DESC LIMIT 20"

# Tests
cd api-server
go test ./...

# Full runtime gate
clickhouse/phase-06/scripts/validate-diagnostics-runtime.sh
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
| 0.5 | 2026-06-07 | Aligned implementation paths with `api-server`, added NodePort runtime validation, documented query filters, report status aggregation, and configurable diagnostics thresholds |
