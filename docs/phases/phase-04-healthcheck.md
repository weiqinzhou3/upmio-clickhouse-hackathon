# Phase 04: Healthcheck and Day1 Acceptance Report

- Version: 0.4
- Date: 2026-05-27
- Status: Confirmed
- Priority: P0
- Owner: zqw
- Depends on:
  - ../master-spec.md
  - ../evidence-summary.md
  - ../design/api-design.md
  - ../design/resource-model.md
  - ../design/data-architecture.md
  - ../design/product-design.md
  - ../architecture/clickhouse-ha-architecture.md
  - phase-03-manager-backend.md

## 1. Purpose

Implement Day1 healthcheck and structured acceptance report for the ClickHouse MVP cluster.

The healthcheck must prove that the cluster is deployed, reachable, replicated, monitored at the endpoint level, and safe enough for demo acceptance.

## 2. Scope

In scope:

1. Healthcheck API endpoint.
2. Structured healthcheck report model.
3. Kubernetes/UPMIO resource checks.
4. Keeper health checks.
5. ClickHouse SQL checks.
6. ReplicatedMergeTree validation checks.
7. Storage/PVC checks.
8. Service/endpoint checks.
9. Metrics endpoint availability check.
10. DDL idempotency rules for validation SQL.
11. Decision on validation Distributed table behavior.

## 3. Non-Goals

Out of scope:

- Full backup/restore validation.
- Full Grafana dashboard validation.
- Automatic remediation.
- Production-grade synthetic workload.
- User business schema management.
- GrpcCall dependency.

## 4. Healthcheck API

Required endpoints:

| Method | Path | Purpose |
|---|---|---|
| `POST` | `/api/v1/clusters/{namespace}/{name}/healthcheck` | Run healthcheck now |
| `GET` | `/api/v1/clusters/{namespace}/{name}/healthcheck/latest` | Return latest retained report if supported |

If the MVP Manager is stateless, `latest` may return `501 Not Implemented` or a clear structured error until persistence is implemented. This must match the data architecture decision.

## 5. Healthcheck Report Model

Minimum report:

```json
{
  "cluster": "clickhouse-runtime",
  "namespace": "upm-clickhouse-runtime",
  "status": "PASS",
  "startedAt": "...",
  "completedAt": "...",
  "summary": {
    "passed": 12,
    "failed": 0,
    "warnings": 1
  },
  "checks": [
    {
      "name": "system_clusters_topology",
      "status": "PASS",
      "severity": "critical",
      "message": "1 shard and 2 replicas detected",
      "evidence": {}
    }
  ]
}
```

Status values:

- `PASS`
- `WARN`
- `FAIL`
- `SKIPPED`

## 6. Required Checks

| Check | Data Source | Decision Rule | Severity |
|---|---|---|---|
| `upmio_project_exists` | Kubernetes API | Project exists for namespace | critical |
| `keeper_unitset_ready` | UnitSet/Unit | expected/current/ready = 3/3/3 | critical |
| `server_unitset_ready` | UnitSet/Unit | expected/current/ready = 2/2/2 for MVP | critical |
| `pods_ready` | Pod status | all ClickHouse/Keeper containers ready | critical |
| `pvc_bound` | PVC status | all expected PVCs Bound | critical |
| `services_endpoints` | Service/Endpoint | TCP/HTTP/Interserver/metrics service endpoints exist | critical |
| `keeper_ruok` | Keeper command | all Keeper nodes return `imok` | critical |
| `keeper_leader_follower` | Keeper `mntr` | exactly one leader for 3-node Keeper | critical |
| `clickhouse_select_1` | ClickHouse SQL | `SELECT 1` returns 1 | critical |
| `system_clusters_topology` | ClickHouse SQL | MVP shows 1 shard and 2 replicas | critical |
| `replica_health` | `system.replicas` | no readonly/session expired; queue stable | critical |
| `write_read_probe` | ClickHouse SQL | idempotent test insert/select succeeds | critical |
| `metrics_endpoint` | HTTP curl | `/metrics` responds with Prometheus text | warning/critical depending on phase |
| `no_secret_leakage` | report rendering | report does not include credential values | critical |

## 7. DDL and Distributed Table Decisions

This phase must close or refine these Master Spec Open Questions:

| Question | Phase 04 output |
|---|---|
| Should Distributed tables be initialized by Manager or left to application users? | Healthcheck may create validation-only database/table; business Distributed tables are not auto-managed in MVP unless approved |
| How should DDL idempotency be guaranteed? | All validation DDL must use idempotent patterns and isolated validation namespace/table |

Minimum DDL policy:

- Use a dedicated validation database such as `upm_healthcheck`.
- Use `CREATE DATABASE IF NOT EXISTS` and `CREATE TABLE IF NOT EXISTS` where supported.
- Use a unique validation table or deterministic cleanup-safe table.
- Do not mutate user business tables.
- If `ON CLUSTER` is unavailable, record as warning or phase-specific limitation; do not hide it.

## 8. Files Likely Changed

```text
backend/internal/api/health_handler.go
backend/internal/model/healthcheck.go
backend/internal/clickhouse/healthcheck.go
backend/internal/k8s/health_resources.go
backend/internal/upmio/status.go
backend/internal/report/health_report.go
docs/design/api-design.md
docs/design/data-architecture.md
```

## 9. Acceptance Criteria

1. `POST /healthcheck` returns a structured report with all required check names.
2. Report status is `PASS` only when all critical checks pass.
3. Keeper `ruok` and `mntr` evidence is included or summarized.
4. `SELECT 1` evidence is included.
5. `system.clusters` evidence shows MVP topology.
6. `system.replicas` evidence shows no readonly/session-expired state.
7. Write/read probe is idempotent and isolated from business data.
8. Healthcheck does not require ClickHouse `GrpcCall`.
9. Healthcheck report redacts secrets.
10. If latest report persistence is not implemented, API behavior is explicit and documented.
11. Unit tests cover report status aggregation logic.
12. Failure cases return structured error responses.

## 10. Verification Commands

```bash
# Backend tests
cd backend
go test ./...

# Trigger healthcheck
curl -sS -X POST http://127.0.0.1:<port>/api/v1/clusters/upm-clickhouse-runtime/clickhouse-runtime/healthcheck | jq .

# Check required fields
curl -sS -X POST http://127.0.0.1:<port>/api/v1/clusters/upm-clickhouse-runtime/clickhouse-runtime/healthcheck \
  | jq '.status, .summary, [.checks[].name]'

# Direct SQL verification used by healthcheck
kubectl exec -n upm-clickhouse-runtime <clickhouse-pod> -c clickhouse -- clickhouse-client --query 'SELECT 1'
kubectl exec -n upm-clickhouse-runtime <clickhouse-pod> -c clickhouse -- clickhouse-client --query "SELECT cluster, shard_num, replica_num, host_name, port FROM system.clusters WHERE cluster='upm_cluster' ORDER BY shard_num, replica_num"
kubectl exec -n upm-clickhouse-runtime <clickhouse-pod> -c clickhouse -- clickhouse-client --query "SELECT database, table, is_readonly, is_session_expired, absolute_delay, queue_size FROM system.replicas FORMAT Vertical"

# Keeper verification
kubectl exec -n upm-clickhouse-runtime <keeper-pod> -c clickhouse-keeper -- bash -lc 'echo ruok | nc 127.0.0.1 9181'
kubectl exec -n upm-clickhouse-runtime <keeper-pod> -c clickhouse-keeper -- bash -lc 'echo mntr | nc 127.0.0.1 9181'
```

## 11. Risks and Open Questions

| Risk / Question | Handling |
|---|---|
| Healthcheck DDL creates residual objects | Use isolated validation database/table and idempotent names |
| `ON CLUSTER` unavailable | Report explicitly; do not treat as pass for distributed DDL |
| Metrics endpoint still unavailable | Fail/warn depending on Phase 05 completion state |
| Latest report persistence not available | Return clear structured response and keep persistence in future roadmap |

## 12. Changelog

| Version | Date | Changes |
|---|---|---|
| 0.1 | 2026-05-27 | Initial phase draft |
| 0.2 | 2026-05-27 | Added healthcheck checks and report model |
| 0.3 | 2026-05-27 | Added metadata and red-team fix structure |
| 0.4 | 2026-05-27 | Restored concrete check table, report model, DDL/idempotency decisions, and objective verification commands |
