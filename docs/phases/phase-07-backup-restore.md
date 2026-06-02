# Phase 07: Backup and Restore Task Model

- Version: 0.5
- Date: 2026-06-02
- Status: Confirmed
- Priority: P1
- Owner: zqw
- Depends on:
  - ../master-spec.md
  - ../evidence-summary.md
  - ../design/api-design.md
  - ../design/data-architecture.md
  - ../runtime/06-clickhouse-grpccall-validation.md
  - phase-03-manager-backend.md

## 1. Purpose

Design and validate a safe ClickHouse backup/restore task model for
productization and hackathon demo coverage.

Backup and restore are required ClickHouse operations capabilities. This phase
must not drop them just because the installed `unit-operator:v1.1.0` rejects
`GrpcCall(type=clickhouse)`.

Primary path:

- Implement backup/restore through an explicitly approved Kubernetes task path,
  such as a Manager-created Kubernetes `Job`.
- The Job must use Kubernetes Secret references for ClickHouse credentials and
  backup storage credentials.
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
8. Document source-code repair spike for `GrpcCall` if time permits.

## 3. Non-Goals

Out of scope unless explicitly approved:

- Running destructive restore against production data.
- Implementing backup scheduler.
- Implementing backup retention cleanup.
- Implementing full object storage lifecycle management.
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
| Is GrpcCall used, repaired, or deferred? | Clear phase conclusion backed by runtime evidence |

## 5. Execution Path Decision

MVP / hackathon path:

- Manager creates a Kubernetes `Job` in the target namespace.
- The Job runs a controlled ClickHouse backup/restore command.
- Credentials are injected through `secretRef` and never through plaintext
  request fields, ConfigMaps, logs, or reports.
- Job status, Pod phase, exit code, and redacted logs form the task evidence.
- The Manager returns structured task status and an evidence reference.

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

Source-code repair spike:

- Investigate whether rejection is caused by CRD schema, controller validation,
  unit-agent action routing, package metadata, or image version mismatch.
- If a fix is small and verified, keep it as an enhancement.
- If the fix requires broad UPMIO operator changes, record it as future work.

## 6. Backup Model

Candidate API:

```http
POST /api/v1/clusters/{namespace}/{name}/backup
```

Candidate request:

```json
{
  "scope": {
    "database": "default",
    "table": "events"
  },
  "storage": {
    "type": "s3",
    "secretRef": "clickhouse-backup-secret",
    "path": "backups/clickhouse-runtime/2026-05-27"
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

Candidate API:

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

## 8. Files Likely Changed

```text
backend/internal/api/backup_handler.go
backend/internal/model/backup.go
backend/internal/upmio/grpccall.go
backend/internal/k8s/job.go
backend/internal/task/backup.go
backend/internal/task/restore.go
backend/internal/task/job_status.go
docs/design/api-design.md
docs/design/data-architecture.md
```

## 9. Acceptance Criteria

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
11. No backup or restore result is claimed without actual command evidence.

## 10. Verification Commands

```bash
# Verify CRD accepts GrpcCall
kubectl get crd grpccalls.upm.syntropycloud.io

# Apply logical backup runtime manifest only in validation namespace
kubectl apply -f docs/runtime/manifests/clickhouse-logical-backup-runtime.yaml
kubectl get grpccall -n upm-clickhouse-runtime clickhouse-logical-backup-runtime -o yaml
kubectl describe grpccall -n upm-clickhouse-runtime clickhouse-logical-backup-runtime

# Operator logs
kubectl logs -n upm-system deploy/unit-operator --tail=200

# Unit-agent logs
kubectl logs -n upm-clickhouse-runtime <clickhouse-pod> -c unit-agent --tail=200

# Kubernetes Job-based backup/restore path
kubectl get job -n upm-clickhouse-runtime -l app.kubernetes.io/component=clickhouse-backup
kubectl describe job -n upm-clickhouse-runtime <backup-job>
kubectl logs -n upm-clickhouse-runtime job/<backup-job> --tail=200
kubectl get job -n upm-clickhouse-runtime -l app.kubernetes.io/component=clickhouse-restore
kubectl describe job -n upm-clickhouse-runtime <restore-job>
kubectl logs -n upm-clickhouse-runtime job/<restore-job> --tail=200

# Validation query after restore
kubectl exec -n upm-clickhouse-runtime <clickhouse-pod> -c clickhouse -- \
  clickhouse-client --query "SELECT count() FROM restore_validation.events_restored"
```

If restore is not executed, record the reason explicitly.

## 11. Risks and Open Questions

| Risk / Question | Handling |
|---|---|
| Current operator image rejects clickhouse GrpcCall | Use Kubernetes Job path for MVP; keep GrpcCall repair as timeboxed spike |
| Job path bypasses UPMIO task model expectations | Keep it Manager-created, namespace-scoped, SecretRef-based, and documented as approved alternative task path |
| Restore is destructive | Require human approval and validation target |
| Object storage unavailable | Use documented skip or local test backend only |
| Credential leakage | Use Secret refs and redaction only |
| Native BACKUP/RESTORE storage backend constraints | Use an approved backup utility container or record backend limitation with evidence |

## 12. Changelog

| Version | Date | Changes |
|---|---|---|
| 0.1 | 2026-05-27 | Initial phase draft |
| 0.2 | 2026-05-27 | Added backup/restore scope and safety constraints |
| 0.3 | 2026-05-27 | Added metadata and red-team fix structure |
| 0.4 | 2026-05-27 | Restored GrpcCall runtime gate, backup/restore models, safety criteria, and verification commands |
| 0.5 | 2026-06-02 | Added Kubernetes Job backup/restore primary path and moved GrpcCall repair to a verified secondary spike |
