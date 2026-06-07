# Phase 07: Backup and Restore Task Model

- Version: 0.6
- Date: 2026-06-07
- Status: Confirmed
- Priority: P1
- Owner: zqw
- Depends on:
  - ../master-spec.md
  - ../evidence-summary.md
  - ../design/api-design.md
  - ../design/data-architecture.md
  - ../runtime/06-clickhouse-grpccall-validation.md
  - phase-03-upm-api-server.md

## 1. Purpose

Implement and validate a safe ClickHouse backup/restore task model for
productization and hackathon demo coverage.

Backup and restore are required ClickHouse operations capabilities. This phase
must not drop them just because the installed `unit-operator:v1.1.0` rejects
`GrpcCall(type=clickhouse)`.

Primary path:

- Implement backup/restore through an explicitly approved Kubernetes task path,
  such as a `upm-api-server`-created Kubernetes `Job`.
- Implement API-managed scheduled backup through a
  `upm-api-server`-created Kubernetes `CronJob`.
- The Job must use Kubernetes Secret references for ClickHouse credentials and
  backup storage credentials.
- The CronJob must use the same SecretRef-only credential model and must create
  timestamped backup paths.
- Restore validation must restore into a validation database/table by default
  and must not overwrite business data.

Secondary path:

- Timebox source-code investigation or repair of ClickHouse `GrpcCall` support.
- Do not place the hackathon MVP backup/restore demo on the critical path of
  fixing `GrpcCall`.
- Do not claim `GrpcCall(type=clickhouse)` support until runtime evidence proves
  it.

## 2. Scope

In scope:

1. Define backup/restore user journey and safety gates.
2. Verify which unit-operator/unit-agent image supports ClickHouse `GrpcCall`.
3. Define Kubernetes Job-based backup/restore execution path.
4. Define object storage credential model.
5. Define backup task request/response model.
6. Define restore task request/response model.
7. Define task status read path from Kubernetes Job/Pod status and logs.
8. Define scheduled backup request/response model.
9. Define scheduled backup status read path from Kubernetes CronJob, child Jobs,
   and Pod logs.
10. Document source-code repair spike for `GrpcCall` if time permits.

## 3. Non-Goals

Out of scope unless explicitly approved:

- Running destructive restore against production data.
- Implementing backup retention cleanup.
- Implementing full object storage lifecycle management.
- Implementing a production backup policy engine.
- Bypassing safety confirmation for restore.
- Claiming GrpcCall support without runtime evidence.
- Making `GrpcCall` repair the only backup/restore implementation path.

## 4. Required Decisions

| Decision | Required Output |
|---|---|
| Which unit-operator image supports `GrpcCall(type=clickhouse)`? | Runtime-verified image tag or statement that no compatible image was found |
| What is the backup storage backend for validation? | Object storage, local/minio test bucket, or skipped with reason |
| Where do credentials live? | Kubernetes Secret or approved UPMIO secret mechanism |
| How is restore protected? | Human approval flag, target validation, and explicit destructive warning |
| What is the primary execution path if GrpcCall remains unsupported? | Kubernetes Job-based task path with evidence |
| How is scheduled backup exposed? | API-created Kubernetes CronJob with SecretRef-only credentials |
| Is GrpcCall used, repaired, or deferred? | Clear phase conclusion backed by runtime evidence |

## 5. Execution Path Decision

MVP / hackathon path:

- `upm-api-server` creates a Kubernetes `Job` in the target namespace.
- The Job runs a controlled ClickHouse backup/restore command.
- `upm-api-server` creates Kubernetes `CronJob` resources for scheduled backup
  in the target namespace.
- Each scheduled backup run creates a child Job and uses a timestamped backup
  path.
- Credentials are injected through `secretRef` and never through plaintext
  request fields, ConfigMaps, logs, or reports.
- Job status, Pod phase, exit code, and redacted logs form the task evidence.
- CronJob status, recent child Jobs, Pod phase, exit code, and redacted logs
  form the scheduled backup evidence.
- `upm-api-server` returns structured task status and an evidence reference.

Candidate implementations:

1. ClickHouse native `BACKUP` / `RESTORE` statements against object storage or a
   local validation backend.
2. A controlled ClickHouse backup utility container, if native backup is blocked
   by storage backend constraints.

The first demo target should be a narrow backup/restore slice:

- Create an isolated validation database/table.
- Insert deterministic validation rows.
- Back up the validation object.
- Restore into a separate validation database/table.
- Verify row count and, where feasible, checksum.
- Create a scheduled backup through API using the same validation scope.
- Wait for at least one scheduled child Job to complete.
- Verify that the scheduled backup object can be used as restore input, or
  record the exact storage/backend limitation if restore from that object is
  blocked.

Source-code repair spike:

- Investigate whether rejection is caused by CRD schema, controller validation,
  unit-agent action routing, package metadata, or image version mismatch.
- If a fix is small and verified, keep it as an enhancement.
- If the fix requires broad UPMIO operator changes, record it as future work.

## 6. Backup Model

Required API:

```http
POST /api/v1/clusters/{namespace}/{name}/backup
```

Candidate request:

```json
{
  "scope": {
    "database": "upm_backup_validation",
    "table": "events"
  },
  "storage": {
    "type": "s3",
    "secretRef": "clickhouse-backup-secret",
    "path": "backups/clickhouse-phase03/manual-20260607"
  },
  "execution": {
    "type": "kubernetesJob"
  },
  "dryRun": false
}
```

MVP/P2 behavior:

- If Kubernetes Job execution is available, use it as the primary task path.
- If GrpcCall is unsupported, do not disable backup/restore automatically.
- If no approved execution path is available, return a structured
  `BACKUP_EXECUTION_UNAVAILABLE` error with the exact reason.
- Do not fake backup success.

## 7. Restore Model

Required API:

```http
POST /api/v1/clusters/{namespace}/{name}/restore
```

Required safety fields:

```json
{
  "backupRef": "...",
  "target": {
    "database": "restore_validation",
    "table": "events_restored"
  },
  "execution": {
    "type": "kubernetesJob"
  },
  "confirm": true,
  "reason": "restore validation drill"
}
```

Restore must require explicit confirmation and must avoid overwriting existing data unless a later phase defines a safe overwrite protocol.

For ReplicatedMergeTree validation objects, restore must not reuse the source
table's Keeper path. The Kubernetes Job path should verify that the target table
does not exist, create an empty validation target table, and restore data into
that target without overwriting existing objects.

## 8. Task Status Model

Required API:

```http
GET /api/v1/clusters/{namespace}/{name}/tasks/{taskName}
```

The task status response must include:

```json
{
  "name": "clickhouse-phase03-backup-20260607-001",
  "namespace": "upm-clickhouse-phase03-runtime",
  "cluster": "clickhouse-phase03",
  "type": "backup",
  "status": "Succeeded",
  "message": "backup job completed",
  "startTime": "...",
  "completionTime": "...",
  "jobRef": {
    "namespace": "upm-clickhouse-phase03-runtime",
    "name": "clickhouse-phase03-backup-20260607-001"
  },
  "podRef": {
    "namespace": "upm-clickhouse-phase03-runtime",
    "name": "clickhouse-phase03-backup-20260607-001-xxxxx"
  },
  "evidence": {
    "backupPath": "backups/clickhouse-phase03/manual-20260607",
    "logsRedacted": true
  }
}
```

Status values:

- `Pending`
- `Running`
- `Succeeded`
- `Failed`
- `Unknown`

## 9. Scheduled Backup Model

Required APIs:

| Method | Path | Purpose |
|---|---|---|
| `POST` | `/api/v1/clusters/{namespace}/{name}/backup-schedules` | Create an API-managed backup CronJob |
| `GET` | `/api/v1/clusters/{namespace}/{name}/backup-schedules` | List backup schedules for a cluster |
| `GET` | `/api/v1/clusters/{namespace}/{name}/backup-schedules/{scheduleName}` | Read one backup schedule and recent child Jobs |
| `DELETE` | `/api/v1/clusters/{namespace}/{name}/backup-schedules/{scheduleName}` | Delete an API-managed backup schedule |

Create request:

```json
{
  "name": "validation-every-minute",
  "schedule": "*/1 * * * *",
  "timeZone": "Asia/Shanghai",
  "scope": {
    "database": "upm_backup_validation",
    "table": "events"
  },
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
}
```

Rules:

- `name` must be a DNS-safe stable schedule name.
- `schedule` must be a Kubernetes CronJob-compatible cron expression.
- `timeZone` is optional; if omitted, Kubernetes CronJob default behavior is
  used.
- `pathPrefix` is not a full object path. Each run must append a timestamp or
  Job UID to avoid overwriting previous backups.
- Default `concurrencyPolicy` is `Forbid`.
- Default `successfulJobsHistoryLimit` and `failedJobsHistoryLimit` are `1`.
- The API may expose `suspend`, but retention cleanup is still out of scope.
- The schedule must be labeled as owned by `upm-api-server` and the target
  ClickHouse cluster so it can be listed without owning unrelated CronJobs.

Scheduled backup validation must prove:

- the API creates a Kubernetes CronJob;
- the CronJob creates at least one child Job;
- the child Job completes successfully;
- the child Job output contains no plaintext Secret value;
- task status can be read through the API.

## 10. Files Likely Changed

```text
api-server/internal/api/server.go
api-server/internal/api/server_test.go
api-server/internal/model/backup.go
api-server/internal/model/backup_test.go
api-server/internal/kube/backup.go
api-server/internal/kube/backup_test.go
api-server/internal/kube/store.go
api-server/internal/platform/store.go
clickhouse/phase-03/manifests/upm-api-server.yaml
clickhouse/phase-07/README.md
clickhouse/phase-07/scripts/validate-backup-restore-runtime.sh
docs/design/api-design.md
docs/design/data-architecture.md
docs/api/upm-api-server-v1.md
```

## 11. Acceptance Criteria

1. Phase verifies or rejects ClickHouse GrpcCall runtime support with command evidence.
2. Kubernetes Job execution path is defined and used when GrpcCall remains unsupported.
3. If a compatible image is found, `GrpcCall(type=clickhouse, action=logical-backup)` reaches unit-agent and returns status.
4. If no compatible image is found, backup/restore still has an approved Job path or returns `BACKUP_EXECUTION_UNAVAILABLE` with structured reason.
5. Backup request model references Secret, not plaintext credentials.
6. Restore request model requires explicit confirmation.
7. Restore does not overwrite existing business data in validation path.
8. Demo restore writes only to validation database/table unless explicitly approved otherwise.
9. Restored validation data is verified by row count and, where feasible, checksum.
10. Task status model includes result, message, startTime, completionTime, Kubernetes Job reference, and evidence reference.
11. Scheduled backup API creates a Kubernetes CronJob with SecretRef-only credentials.
12. Scheduled backup CronJob creates at least one child Job during runtime validation.
13. Scheduled backup status API returns CronJob status and recent child Job status.
14. Deleting a schedule through API deletes the API-managed CronJob and does not delete backup objects.
15. No backup, restore, or scheduled backup result is claimed without actual command evidence.

## 12. Verification Commands

```bash
# Verify CRD accepts GrpcCall
kubectl get crd grpccalls.upm.syntropycloud.io

# Apply logical backup runtime manifest only in validation namespace
kubectl apply -f docs/runtime/manifests/clickhouse-logical-backup-runtime.yaml
kubectl get grpccall -n upm-clickhouse-phase03-runtime clickhouse-logical-backup-runtime -o yaml
kubectl describe grpccall -n upm-clickhouse-phase03-runtime clickhouse-logical-backup-runtime

# Operator logs
kubectl logs -n upm-system deploy/unit-operator --tail=200

# Unit-agent logs
kubectl logs -n upm-clickhouse-phase03-runtime <clickhouse-pod> -c unit-agent --tail=200

# API-driven backup/restore/scheduled backup path
export UPM_API_SERVER_URL=http://192.168.35.201:30083
clickhouse/phase-07/scripts/validate-backup-restore-runtime.sh

# Kubernetes Job/CronJob evidence
kubectl get job -n upm-clickhouse-phase03-runtime -l app.kubernetes.io/component=clickhouse-backup
kubectl describe job -n upm-clickhouse-phase03-runtime <backup-job>
kubectl logs -n upm-clickhouse-phase03-runtime job/<backup-job> --tail=200
kubectl get job -n upm-clickhouse-phase03-runtime -l app.kubernetes.io/component=clickhouse-restore
kubectl describe job -n upm-clickhouse-phase03-runtime <restore-job>
kubectl logs -n upm-clickhouse-phase03-runtime job/<restore-job> --tail=200
kubectl get cronjob -n upm-clickhouse-phase03-runtime -l app.kubernetes.io/component=clickhouse-backup-schedule

# Validation query after restore
kubectl exec -n upm-clickhouse-phase03-runtime <clickhouse-pod> -c clickhouse -- \
  clickhouse-client --query "SELECT count() FROM restore_validation.events_restored"
```

If restore is not executed, record the reason explicitly.

Expected final script output:

```text
PASS phase07_backup_restore_runtime_validation
```

## 13. Risks and Open Questions

| Risk / Question | Handling |
|---|---|
| Current operator image rejects clickhouse GrpcCall | Use Kubernetes Job path for MVP; keep GrpcCall repair as timeboxed spike |
| Job path bypasses UPMIO task model expectations | Keep it `upm-api-server`-created, namespace-scoped, SecretRef-based, and documented as approved alternative task path |
| Restore is destructive | Require human approval and validation target |
| Object storage unavailable | Prepare MinIO/S3 validation backend or record backend limitation with evidence |
| Credential leakage | Use Secret refs and redaction only |
| Native BACKUP/RESTORE storage backend constraints | Use an approved backup utility container or record backend limitation with evidence |
| CronJob creates repeated backups | Use validation scope, timestamped paths, short validation schedule, and delete schedule through API after validation |
| Scheduled backup without retention cleanup | Accepted MVP limitation; no backup object deletion in Phase 07 |

## 14. Changelog

| Version | Date | Changes |
|---|---|---|
| 0.1 | 2026-05-27 | Initial phase draft |
| 0.2 | 2026-05-27 | Added backup/restore scope and safety constraints |
| 0.3 | 2026-05-27 | Added metadata and red-team fix structure |
| 0.4 | 2026-05-27 | Restored GrpcCall runtime gate, backup/restore models, safety criteria, and verification commands |
| 0.5 | 2026-06-02 | Added Kubernetes Job backup/restore primary path and moved GrpcCall repair to a verified secondary spike |
| 0.6 | 2026-06-07 | Added owner-required API-managed scheduled backup through Kubernetes CronJob, updated current `api-server` paths, and aligned validation namespace |
