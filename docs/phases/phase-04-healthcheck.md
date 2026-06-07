# Phase 04: Healthcheck and Day1 Acceptance Report

- Version: 0.6
- Date: 2026-06-06
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
  - phase-03-upm-api-server.md

## 1. Purpose

Implement Day1 healthcheck and a structured acceptance report for a
UPM-managed ClickHouse cluster through the existing Kubernetes-deployed
`upm-api-server`.

The healthcheck must prove that the cluster is deployed, reachable,
replicated, has service/endpoint visibility, exposes the ClickHouse metrics
endpoint, and is safe enough for hackathon demo acceptance.

## 2. Scope

In scope:

1. Healthcheck API endpoint registered under existing `/api/v1` routes.
2. Structured healthcheck report model.
3. In-memory latest-report cache for MVP.
4. Kubernetes/UPMIO resource checks.
5. Keeper health checks.
6. ClickHouse SQL checks.
7. ReplicatedMergeTree and Distributed table validation checks.
8. Storage/PVC checks.
9. Service/endpoint checks.
10. ClickHouse native metrics endpoint availability check.
11. DDL idempotency and drift rules for validation SQL.
12. Validation-only Distributed table behavior.
13. Runtime validation script that calls the real API and verifies database
    read/write behavior.

## 3. Non-Goals

Out of scope:

- Full backup/restore validation.
- Full Prometheus/Grafana dashboard validation.
- Prometheus query summary API; that is Phase 05.
- Automatic remediation.
- Production-grade synthetic workload.
- User business schema management.
- GrpcCall dependency.
- New UPMIO CRDs.
- Persistent report database or long-term operation history.

## 4. Healthcheck API

Required endpoints:

| Method | Path | Purpose |
|---|---|---|
| `POST` | `/api/v1/clusters/{namespace}/{name}/healthcheck` | Run healthcheck now |
| `GET` | `/api/v1/clusters/{namespace}/{name}/healthcheck/latest` | Return latest in-memory report |

MVP latest-report decision:

- `upm-api-server` keeps the latest healthcheck report in process memory,
  keyed by `namespace/name`.
- `GET latest` returns `200 OK` with that report when one exists in the
  current process.
- If the API server restarted or no report has been run for the cluster,
  `GET latest` returns structured `404 HEALTHCHECK_REPORT_NOT_FOUND`.
- Phase 04 must not add PostgreSQL, SQLite, MySQL, ConfigMap persistence, or a
  new report CRD.

This matches the data architecture decision that the MVP API server is
stateless or near-stateless and does not introduce an independent database.

## 5. Healthcheck Report Model

Minimum report:

```json
{
  "cluster": "clickhouse-phase03",
  "name": "clickhouse-phase03",
  "namespace": "upm-clickhouse-phase03-runtime",
  "status": "PASS",
  "startedAt": "...",
  "completedAt": "...",
  "durationMs": 1234,
  "summary": {
    "passed": 12,
    "failed": 0,
    "warnings": 1,
    "skipped": 0
  },
  "checks": [
    {
      "name": "system_clusters_topology",
      "status": "PASS",
      "severity": "critical",
      "message": "2 shards and 2 replicas per shard detected",
      "evidence": {
        "cluster": "upm_cluster",
        "shards": 2,
        "replicasPerShard": 2
      }
    }
  ]
}
```

Report status values:

- `PASS`
- `WARN`
- `FAIL`

Check status values:

- `PASS`
- `WARN`
- `FAIL`
- `SKIPPED`

Aggregation rules:

- Report status is `FAIL` when any critical check is `FAIL`.
- Report status is `WARN` when no critical check fails but at least one check
  is `WARN`.
- Report status is `PASS` only when all critical checks pass and there are no
  warnings.
- `SKIPPED` checks must explain why they were skipped.

## 6. Required Checks

The implementation must not hardcode `1 shard x 2 replicas`. It must read the
cluster topology from Phase 03 cluster summary annotations/package topology and
verify SQL output against that expected topology.

Current lab acceptance target:

```text
namespace: upm-clickhouse-phase03-runtime
cluster:   clickhouse-phase03
topology:  2 shards x 2 replicas + 3 Keeper
```

Required checks:

| Check | Data Source | Decision Rule | Severity |
|---|---|---|---|
| `upmio_project_exists` | Kubernetes API | Project exists for namespace | critical |
| `keeper_unitset_ready` | UnitSet/Unit | expected/current/ready match expected Keeper replicas, currently `3/3/3` | critical |
| `server_unitset_ready` | UnitSet/Unit | expected/current/ready match `shards * replicasPerShard`, currently `4/4/4` | critical |
| `pods_ready` | Pod status | all ClickHouse/Keeper containers ready | critical |
| `pvc_bound` | PVC status | all expected PVCs Bound | critical |
| `services_endpoints` | Service/Endpoint | TCP/HTTP/Interserver/metrics service endpoints exist | critical |
| `keeper_ruok` | Keeper command | all Keeper nodes return `imok` | critical |
| `keeper_leader_follower` | Keeper `mntr` | exactly one leader and remaining followers for 3-node Keeper | critical |
| `clickhouse_select_1` | ClickHouse SQL | `SELECT 1` returns `1` | critical |
| `system_clusters_topology` | ClickHouse SQL | `system.clusters` matches expected shards/replicas | critical |
| `replica_health` | `system.replicas` | no readonly/session-expired state; active replicas match total replicas; queue stable | critical |
| `write_read_probe` | ClickHouse SQL | isolated idempotent Distributed-table insert/select succeeds | critical |
| `metrics_endpoint` | HTTP from ClickHouse Pod | `/metrics` responds with Prometheus text | warning |
| `no_secret_leakage` | report rendering | report does not include credential values or secret keys | critical |

Metrics endpoint is a Phase 04 warning check because Prometheus scrape and
summary are Phase 05. The report must still show the endpoint evidence so the
demo can distinguish "ClickHouse exports metrics" from "Prometheus stack is
fully integrated".

## 7. DDL and Distributed Table Decisions

This phase implements the Master Spec decisions:

| Question | Phase 04 output |
|---|---|
| Should Distributed tables be initialized by `upm-api-server` or left to application users? | `upm-api-server` may create validation-only Distributed tables for healthcheck; it must not create or mutate user business tables |
| How should DDL idempotency be guaranteed? | Validation DDL uses idempotent syntax plus API-server-owned object drift checks |

Validation DDL policy:

- Use a dedicated validation database: `upm_healthcheck`.
- Use deterministic validation table names owned by the API server, for
  example `local_events` and `dist_events`.
- Use `CREATE DATABASE IF NOT EXISTS` and `CREATE TABLE IF NOT EXISTS` where
  supported.
- Validate existing API-server-owned table definitions before using them.
- If an API-server-owned validation object exists with a different definition,
  the healthcheck must fail with a drift message; do not silently overwrite.
- Drift validation must run before `TRUNCATE` or `INSERT`; a drift failure must
  not mutate the existing validation table.
- Do not mutate user business databases or tables.
- Do not drop business objects.
- `ON CLUSTER` is required for the Distributed validation probe. If it is
  unavailable, the validation probe must not report `PASS`.
- Reserved validation cleanup may truncate only API-server-owned validation
  tables in `upm_healthcheck`.
- Concurrent healthchecks for the same `namespace/name` must be serialized so
  they cannot truncate or write the same validation tables simultaneously.

## 8. Implementation Boundaries

Implementation must extend the existing Phase 03 Go service:

```text
api-server/
  internal/api/            # healthcheck routes and handlers
  internal/model/          # healthcheck report model
  internal/kube/           # UPMIO/K8s status read helpers
  internal/clickhouse/     # ClickHouse healthcheck SQL helpers
```

Do not create a new `backend/` tree. Do not start a local binary as the
acceptance path. The service must run in Kubernetes through the existing
`upm-system` Deployment/Service.

Files likely changed:

```text
api-server/internal/api/server.go
api-server/internal/api/server_test.go
api-server/internal/model/healthcheck.go
api-server/internal/model/healthcheck_test.go
api-server/internal/kube/store.go
api-server/internal/kube/store_test.go
api-server/internal/clickhouse/client.go
api-server/internal/clickhouse/healthcheck.go
api-server/internal/clickhouse/healthcheck_test.go
docs/api/upm-api-server-v1.md
docs/design/data-architecture.md
clickhouse/phase-04/README.md
clickhouse/scripts/validate-phase04-healthcheck-runtime.sh
```

## 9. Acceptance Criteria

1. `POST /healthcheck` returns a structured report with every required check
   name.
2. Report status is `PASS` only when all critical checks pass and no warnings
   exist.
3. `GET /healthcheck/latest` returns the latest in-memory report or structured
   `404 HEALTHCHECK_REPORT_NOT_FOUND`.
4. Keeper `ruok` and `mntr` evidence is included or summarized.
5. `SELECT 1` evidence is included.
6. `system.clusters` evidence matches the expected topology dynamically,
   including the current 2x2 lab cluster.
7. `system.replicas` evidence shows no readonly/session-expired state and
   healthy replica counts.
8. Write/read probe is idempotent and isolated from business data.
9. Healthcheck does not require ClickHouse `GrpcCall`.
10. Metrics endpoint is checked and reported as warning severity in Phase 04.
11. Healthcheck report redacts secrets.
12. Unit tests cover report status aggregation logic.
13. Unit tests cover latest-report cache behavior.
14. Unit tests cover secret leakage prevention in reports.
15. Failure cases return structured error responses.
16. Runtime validation calls the real API over NodePort and verifies the
    resulting report against live Kubernetes and ClickHouse state.
17. Service/Endpoint validation confirms ClickHouse TCP, HTTP, interserver,
    and metrics ports.
18. Validation-table drift is checked before any truncate or insert.
19. Same-cluster healthcheck requests are serialized.
20. Report output redacts known admin-password and AES-key values, not only
    their field names.

## 10. Verification Commands

```bash
# Go tests
cd api-server
GOCACHE=/tmp/upm-go-cache go test ./...
GOCACHE=/tmp/upm-go-cache go test -race ./...
GOCACHE=/tmp/upm-go-cache go vet ./...
GOCACHE=/tmp/upm-go-cache go build ./...

# Build/sync image and deploy updated API server
cd ..
SSH_PASSWORD=<node-password> clickhouse/scripts/sync-upm-api-server-image-to-nodes.sh
kubectl apply -f clickhouse/phase-03/manifests/upm-api-server.yaml
kubectl -n upm-system rollout status deploy/upm-api-server --timeout=180s

# Trigger healthcheck through the real NodePort service
export UPM_API_SERVER_URL=http://192.168.35.201:30083
curl -fsS -X POST \
  "${UPM_API_SERVER_URL}/api/v1/clusters/upm-clickhouse-phase03-runtime/clickhouse-phase03/healthcheck" \
  | jq .

# Read latest in-memory report
curl -fsS \
  "${UPM_API_SERVER_URL}/api/v1/clusters/upm-clickhouse-phase03-runtime/clickhouse-phase03/healthcheck/latest" \
  | jq .

# Check required fields
curl -fsS -X POST \
  "${UPM_API_SERVER_URL}/api/v1/clusters/upm-clickhouse-phase03-runtime/clickhouse-phase03/healthcheck" \
  | jq '.status, .summary, [.checks[].name]'

# Full Phase 04 runtime gate
clickhouse/scripts/validate-phase04-healthcheck-runtime.sh
```

Direct evidence commands used by healthcheck:

```bash
kubectl exec -n upm-clickhouse-phase03-runtime clickhouse-phase03-0 \
  -c clickhouse -- service-ctl.sh login --query 'SELECT 1'

kubectl exec -n upm-clickhouse-phase03-runtime clickhouse-phase03-0 \
  -c clickhouse -- service-ctl.sh login --query \
  "SELECT cluster, shard_num, replica_num, host_name, port FROM system.clusters WHERE cluster='upm_cluster' ORDER BY shard_num, replica_num"

kubectl exec -n upm-clickhouse-phase03-runtime clickhouse-phase03-0 \
  -c clickhouse -- service-ctl.sh login --query \
  "SELECT database, table, is_readonly, is_session_expired, absolute_delay, queue_size FROM system.replicas FORMAT Vertical"

kubectl exec -n upm-clickhouse-phase03-runtime clickhouse-phase03-keeper-0 \
  -c clickhouse-keeper -- bash -lc 'exec 3<>/dev/tcp/127.0.0.1/9181; printf ruok >&3; timeout 2 cat <&3'

kubectl exec -n upm-clickhouse-phase03-runtime clickhouse-phase03-keeper-0 \
  -c clickhouse-keeper -- bash -lc 'exec 3<>/dev/tcp/127.0.0.1/9181; printf mntr >&3; timeout 2 cat <&3'

kubectl exec -n upm-clickhouse-phase03-runtime clickhouse-phase03-0 \
  -c clickhouse -- curl -fsS --max-time 5 http://127.0.0.1:9363/metrics | head
```

## 11. Risks and Open Questions

| Risk / Question | Handling |
|---|---|
| Healthcheck DDL creates residual objects | Use isolated `upm_healthcheck` database and API-server-owned deterministic table names |
| Validation object drift | Fail the healthcheck with a drift message; do not silently overwrite |
| `ON CLUSTER` unavailable | The write/read validation probe fails; do not report PASS |
| Metrics endpoint unavailable | Report `WARN` in Phase 04; Prometheus integration remains Phase 05 |
| Latest report lost after API server restart | Return structured `404 HEALTHCHECK_REPORT_NOT_FOUND`; persistent report storage is future scope |
| ClickHouse password handling | Execute SQL through the in-Pod `service-ctl.sh login` path; do not copy credential values into API responses or logs |

## 12. Changelog

| Version | Date | Changes |
|---|---|---|
| 0.1 | 2026-05-27 | Initial phase draft |
| 0.2 | 2026-05-27 | Added healthcheck checks and report model |
| 0.3 | 2026-05-27 | Added metadata and red-team fix structure |
| 0.4 | 2026-05-27 | Restored concrete check table, report model, DDL/idempotency decisions, and objective verification commands |
| 0.5 | 2026-06-06 | Aligned Phase 04 with `api-server`, current 2x2 runtime, in-memory latest report, NodePort validation, metrics WARN policy, and validation-object drift rules |
| 0.6 | 2026-06-06 | Closed review gaps: pre-write drift validation, dynamic topology assertions, required service ports, same-cluster serialization, report duration, and actual Secret-value redaction |
