# API Manual

## Purpose

`upm-api-server` is the product API entry point for the ClickHouse operations
MVP. It runs inside Kubernetes and exposes a NodePort in the hackathon lab.

Full field-level reference:
[`docs/api/upm-api-server-v1.md`](../api/upm-api-server-v1.md).

## Runtime Access

```bash
export UPM_API_SERVER_URL=http://192.168.35.201:30083
curl -fsS "${UPM_API_SERVER_URL}/api/v1/healthz" | jq .
```

The same NodePort is reachable from any Kubernetes node:

```text
http://192.168.35.201:30083
http://192.168.35.202:30083
http://192.168.35.203:30083
http://192.168.35.204:30083
```

## Supported API Groups

| Capability | API |
|---|---|
| Process health | `GET /api/v1/healthz` |
| Create ClickHouse cluster | `POST /api/v1/clusters` |
| List clusters | `GET /api/v1/clusters` |
| Get cluster summary | `GET /api/v1/clusters/{namespace}/{name}` |
| Get Kubernetes resources | `GET /api/v1/clusters/{namespace}/{name}/resources` |
| Run healthcheck | `POST /api/v1/clusters/{namespace}/{name}/healthcheck` |
| Get latest healthcheck | `GET /api/v1/clusters/{namespace}/{name}/healthcheck/latest` |
| Metrics summary | `GET /api/v1/clusters/{namespace}/{name}/metrics/summary` |
| Day2 diagnostics | `GET /api/v1/clusters/{namespace}/{name}/diagnostics` |
| Immediate backup | `POST /api/v1/clusters/{namespace}/{name}/backup` |
| Restore | `POST /api/v1/clusters/{namespace}/{name}/restore` |
| Task status | `GET /api/v1/clusters/{namespace}/{name}/tasks/{taskName}` |
| Create backup schedule | `POST /api/v1/clusters/{namespace}/{name}/backup-schedules` |
| List backup schedules | `GET /api/v1/clusters/{namespace}/{name}/backup-schedules` |
| Get backup schedule | `GET /api/v1/clusters/{namespace}/{name}/backup-schedules/{scheduleName}` |
| Delete backup schedule | `DELETE /api/v1/clusters/{namespace}/{name}/backup-schedules/{scheduleName}` |

## Common Runtime Variables

```bash
export NS=upm-clickhouse-phase03-runtime
export NAME=clickhouse-phase03
export UPM_API_SERVER_URL=http://192.168.35.201:30083
```

## Create Cluster

```bash
curl -fsS -X POST \
  -H 'Content-Type: application/json' \
  "${UPM_API_SERVER_URL}/api/v1/clusters" \
  -d '{
    "namespace": "upm-clickhouse-phase03-runtime",
    "name": "clickhouse-phase03",
    "version": "26.3.9.8",
    "topology": {
      "shards": 2,
      "replicasPerShard": 2,
      "keeperReplicas": 3
    },
    "storage": {
      "className": "local-path",
      "serverDataSize": "20Gi",
      "keeperDataSize": "10Gi"
    },
    "security": {
      "adminSecretRef": "clickhouse-phase03-secret"
    },
    "monitoring": {
      "enabled": true
    }
  }' | jq .
```

Prerequisites:

- UPMIO operators and ClickHouse packages are installed.
- The target namespace has `aes-secret-key`.
- `security.adminSecretRef` exists and contains `CLICKHOUSE_ADMIN_PASSWORD`.
- The installed package topology matches the requested topology.

## Healthcheck

```bash
curl -fsS -X POST \
  "${UPM_API_SERVER_URL}/api/v1/clusters/${NS}/${NAME}/healthcheck" \
  | jq .
```

Expected demo result:

- `status` is `PASS` or acceptable `WARN`.
- `summary.failed` is `0`.
- Checks include Kubernetes readiness, Keeper state, ClickHouse SQL, topology,
  write/read probe, replica health, metrics endpoint, and secret leakage scan.

## Metrics Summary

```bash
curl -fsS \
  "${UPM_API_SERVER_URL}/api/v1/clusters/${NS}/${NAME}/metrics/summary" \
  | jq .
```

Expected demo result:

- `status` is `READY`.
- Prometheus sees four ClickHouse targets.
- CPU, memory, storage, and ClickHouse metric categories contain samples.

## Day2 Diagnostics

```bash
curl -fsS \
  "${UPM_API_SERVER_URL}/api/v1/clusters/${NS}/${NAME}/diagnostics?limit=20" \
  | jq .
```

Optional row-count validation:

```bash
curl -fsS \
  "${UPM_API_SERVER_URL}/api/v1/clusters/${NS}/${NAME}/diagnostics?database=upm_healthcheck&table=dist_events&expectedRows=8&limit=20" \
  | jq .
```

Diagnostics are read-only. They report evidence and recommendations; they do
not kill mutations, run `OPTIMIZE`, change TTL, or repair data.

## Backup

The backup storage Secret must contain:

```text
S3_ENDPOINT
S3_BUCKET
S3_ACCESS_KEY
S3_SECRET_KEY
S3_USE_SSL
```

Create an immediate backup:

```bash
curl -fsS -X POST \
  -H 'Content-Type: application/json' \
  "${UPM_API_SERVER_URL}/api/v1/clusters/${NS}/${NAME}/backup" \
  -d '{
    "scope": {"database": "upm_backup_validation", "table": "events"},
    "storage": {
      "type": "s3",
      "secretRef": "clickhouse-backup-secret",
      "path": "backups/clickhouse-phase03/manual-demo"
    },
    "execution": {"type": "kubernetesJob"},
    "dryRun": false
  }' | tee /tmp/upm-backup.json | jq .
```

Check task status:

```bash
BACKUP_TASK="$(jq -r '.name' /tmp/upm-backup.json)"
curl -fsS \
  "${UPM_API_SERVER_URL}/api/v1/clusters/${NS}/${NAME}/tasks/${BACKUP_TASK}" \
  | jq .
```

## Restore

Restore is implemented. It creates a Kubernetes Job through `upm-api-server`.
The target table must not already exist. Restore requires `confirm: true` and a
human-readable `reason`.

```bash
BACKUP_PATH="$(jq -r '.evidence.backupPath' clickhouse/phase-07/backup-task.json)"

curl -fsS -X POST \
  -H 'Content-Type: application/json' \
  "${UPM_API_SERVER_URL}/api/v1/clusters/${NS}/${NAME}/restore" \
  -d "{
    \"backupRef\": \"${BACKUP_PATH}\",
    \"source\": {\"database\": \"upm_backup_validation\", \"table\": \"events\"},
    \"target\": {\"database\": \"upm_restore_validation\", \"table\": \"events_manual_restore_check\"},
    \"storage\": {\"type\": \"s3\", \"secretRef\": \"clickhouse-backup-secret\"},
    \"execution\": {\"type\": \"kubernetesJob\"},
    \"confirm\": true,
    \"reason\": \"manual restore validation\"
  }" | tee /tmp/upm-restore.json | jq .
```

Check task status:

```bash
RESTORE_TASK="$(jq -r '.name' /tmp/upm-restore.json)"
curl -fsS \
  "${UPM_API_SERVER_URL}/api/v1/clusters/${NS}/${NAME}/tasks/${RESTORE_TASK}" \
  | jq .
```

## Scheduled Backup

Create a one-minute validation schedule:

```bash
curl -fsS -X POST \
  -H 'Content-Type: application/json' \
  "${UPM_API_SERVER_URL}/api/v1/clusters/${NS}/${NAME}/backup-schedules" \
  -d '{
    "name": "validation-every-minute",
    "schedule": "*/1 * * * *",
    "timeZone": "Asia/Shanghai",
    "scope": {"database": "upm_backup_validation", "table": "events"},
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
  }' | jq .
```

Read schedule status:

```bash
curl -fsS \
  "${UPM_API_SERVER_URL}/api/v1/clusters/${NS}/${NAME}/backup-schedules/validation-every-minute" \
  | jq .
```

Delete schedule:

```bash
curl -fsS -X DELETE \
  "${UPM_API_SERVER_URL}/api/v1/clusters/${NS}/${NAME}/backup-schedules/validation-every-minute"
```
