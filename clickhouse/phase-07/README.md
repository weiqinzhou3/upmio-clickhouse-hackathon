# Phase 07 Runtime Evidence

This directory contains Phase 07 backup, restore, and scheduled backup
validation assets.

## Runtime Target

```text
API Server: http://192.168.35.201:30083
Namespace:  upm-clickhouse-phase03-runtime
Cluster:    clickhouse-phase03
```

## Validation

```bash
clickhouse/phase-07/scripts/validate-backup-restore-runtime.sh
```

Expected final line:

```text
PASS phase07_backup_restore_runtime_validation
```

The script validates the real runtime chain:

1. `upm-api-server` is rolled out in Kubernetes.
2. A validation MinIO object store and `clickhouse-backup-secret` exist in the
   ClickHouse namespace.
3. A deterministic ClickHouse validation table is created and populated.
4. `POST /backup` creates a Kubernetes `Job`.
5. `GET /tasks/{taskName}` reports the backup task as `Succeeded`.
6. `POST /restore` restores the manual backup into a different validation
   table.
7. Restored data matches the source row count and checksum.
8. `POST /backup-schedules` creates a Kubernetes `CronJob`.
9. At least one scheduled child `Job` completes successfully.
10. The scheduled backup object can be restored through `POST /restore`.
11. API responses and task logs do not expose credential plaintext.

The script deletes the validation CronJob by default after it proves scheduled
backup works. Set `DELETE_SCHEDULE_AFTER_VALIDATION=false` when a live demo
needs the CronJob to remain.

## Saved Evidence

The validation script saves:

```text
clickhouse/phase-07/backup-response.json
clickhouse/phase-07/backup-task.json
clickhouse/phase-07/restore-response.json
clickhouse/phase-07/restore-task.json
clickhouse/phase-07/backup-schedule-response.json
clickhouse/phase-07/backup-schedule-detail.json
clickhouse/phase-07/backup-schedule-list.json
clickhouse/phase-07/scheduled-restore-response.json
clickhouse/phase-07/scheduled-restore-task.json
clickhouse/phase-07/source-checksum.tsv
clickhouse/phase-07/restore-checksum.tsv
clickhouse/phase-07/scheduled-restore-checksum.tsv
```
