# Scripts Manual

## Purpose

All current operational scripts are centralized under:

```text
clickhouse/scripts/
```

Phase directories keep evidence files, manifests, values, and historical
runtime outputs. Scripts should be invoked from the repository root.

## Script Inventory

| Script | Purpose | Main output |
|---|---|---|
| `sync-runtime-image-to-nodes.sh` | Rebuild/import ClickHouse runtime image on all K8s nodes | `PASS` after all node imports |
| `sync-upm-api-server-image-to-nodes.sh` | Build/import `upm-api-server` image on all K8s nodes | `PASS upm_api_server_image_sync` |
| `validate-phase01-runtime.sh` | Validate Phase 01 1-shard/2-replica runtime | `PASS phase01_runtime_validation` |
| `validate-phase02-rendered-topology.py` | Validate rendered topology YAML | `PASS topology_render_validation` |
| `validate-phase02-runtime-2s2r.sh` | Validate 2-shard/2-replica runtime | `PASS runtime_2s2r_validation` |
| `validate-phase03-upm-api-server-runtime.sh` | Validate `upm-api-server` and cluster API | `PASS phase03_upm_api_server_runtime_validation` |
| `validate-phase04-healthcheck-runtime.sh` | Validate healthcheck API and write/read probe | `PASS phase04_healthcheck_runtime_validation` |
| `validate-phase05-monitoring-runtime.sh` | Validate ClickHouse metrics, Prometheus, Grafana dashboard | `PASS phase05_monitoring_runtime_validation` |
| `validate-phase06-diagnostics-runtime.sh` | Validate Day2 diagnostics API and read-only evidence | `PASS phase06_diagnostics_runtime_validation` |
| `validate-phase07-backup-restore-runtime.sh` | Validate backup, restore, schedule, scheduled restore | `PASS phase07_backup_restore_runtime_validation` |

## Common Requirements

Most scripts require:

```bash
kubectl
curl
jq
bash
```

Image sync scripts also require:

```bash
go
sshpass
podman
ctr
```

`podman` and `ctr` are required on Kubernetes nodes; `go` and `sshpass` are
required on the workstation running the sync script.

## Image Sync Scripts

### ClickHouse Runtime Image

```bash
SSH_PASSWORD=root clickhouse/scripts/sync-runtime-image-to-nodes.sh
```

Optional variables:

| Variable | Default | Meaning |
|---|---|---|
| `IMAGE` | `localhost/upmio/clickhouse:26.3.9.8-runtime` | Target runtime image tag |
| `BASE_IMAGE` | same as `IMAGE` | Base image to rebuild from |
| `SERVICE_CTL` | `upm-packages/clickhouse/26.3.9.8/image/service-ctl.sh` | Runtime control script copied into image |
| `SSH_USER` | `root` | Kubernetes node SSH user |
| `NODES` | `192.168.35.201 192.168.35.202 192.168.35.203 192.168.35.204` | Target nodes |
| `SSH_PASSWORD` | required | Node SSH password |

### upm-api-server Image

```bash
SSH_PASSWORD=root clickhouse/scripts/sync-upm-api-server-image-to-nodes.sh
kubectl -n upm-system rollout restart deploy/upm-api-server
kubectl -n upm-system rollout status deploy/upm-api-server --timeout=180s
```

Optional variables:

| Variable | Default | Meaning |
|---|---|---|
| `IMAGE` | `localhost/upmio/upm-api-server:phase-07` | API server image tag |
| `GOPROXY` | `https://goproxy.cn,direct` | Go module proxy |
| `BINARY` | `api-server/bin/upm-api-server` | Local build output path |
| `SSH_USER` | `root` | Kubernetes node SSH user |
| `NODES` | `192.168.35.201 192.168.35.202 192.168.35.203 192.168.35.204` | Target nodes |
| `SSH_PASSWORD` | required | Node SSH password |

## Validation Scripts

### Phase 01 Runtime

```bash
clickhouse/scripts/validate-phase01-runtime.sh
```

Optional variables:

| Variable | Default |
|---|---|
| `NAMESPACE` | `upm-clickhouse-runtime` |
| `CLICKHOUSE_UNITSET` | `clickhouse-runtime` |
| `KEEPER_UNITSET` | `clickhouse-runtime-keeper` |
| `PACKAGE_NAMESPACE` | `upm-system` |
| `PACKAGE_PODTEMPLATE` | `clickhouse-26.3.9.8` |
| `EXPECTED_CLICKHOUSE_IMAGE` | `localhost/upmio/clickhouse:26.3.9.8-runtime` |

### Phase 02 Rendered Topology

```bash
python3 clickhouse/scripts/validate-phase02-rendered-topology.py \
  --rendered /tmp/ch-2s2r.yaml \
  --shards 2 \
  --replicas-per-shard 2
```

### Phase 02 2s2r Runtime

```bash
clickhouse/scripts/validate-phase02-runtime-2s2r.sh
```

Optional variables:

| Variable | Default |
|---|---|
| `NS` | `upm-clickhouse-phase02-runtime` |
| `CLUSTER` | `upm_cluster` |
| `CLICKHOUSE_UNITSET` | `clickhouse-phase02` |
| `KEEPER_UNITSET` | `clickhouse-phase02-keeper` |
| `POD` | `clickhouse-phase02-0` |
| `DB` | `phase02_runtime_validation` |

### Phase 03 upm-api-server Runtime

```bash
clickhouse/scripts/validate-phase03-upm-api-server-runtime.sh
```

Important variables:

| Variable | Default |
|---|---|
| `API_SERVER_URL` | `http://192.168.35.201:30083` |
| `START_PORT_FORWARD` | `0` |
| `EXISTING_NS` | `upm-clickhouse-phase03-runtime` |
| `EXISTING_CLUSTER` | `clickhouse-phase03` |
| `PHASE03_CREATE_E2E` | `0` |
| `DB_RUNTIME_VALIDATOR` | `clickhouse/scripts/validate-phase02-runtime-2s2r.sh` |

Set `PHASE03_CREATE_E2E=1` only when intentionally testing create-cluster
end-to-end.

### Phase 04 Healthcheck

```bash
clickhouse/scripts/validate-phase04-healthcheck-runtime.sh
```

Outputs:

```text
clickhouse/phase-04/runtime-healthcheck-report.json
clickhouse/phase-04/runtime-healthcheck-latest.json
```

### Phase 05 Monitoring

```bash
clickhouse/scripts/validate-phase05-monitoring-runtime.sh
```

Important variables:

| Variable | Default |
|---|---|
| `PROM_NS` | `monitoring` |
| `PROM_SVC` | `kube-prometheus-stack-prometheus` |
| `GRAFANA_SVC` | `kube-prometheus-stack-grafana` |
| `GRAFANA_USER` | `admin` |
| `GRAFANA_PASSWORD` | `admin` |
| `GRAFANA_DASHBOARD_UID` | `upm-clickhouse-overview` |

Outputs:

```text
clickhouse/phase-05/prometheus-targets.json
clickhouse/phase-05/metrics-summary.json
clickhouse/phase-05/grafana-dashboard.json
clickhouse/phase-05/grafana-panel-query-results.json
```

### Phase 06 Diagnostics

```bash
clickhouse/scripts/validate-phase06-diagnostics-runtime.sh
```

Outputs:

```text
clickhouse/phase-06/diagnostics.json
clickhouse/phase-06/diagnostics-rowcount.json
clickhouse/phase-06/system-replicas.tsv
clickhouse/phase-06/system-parts.tsv
clickhouse/phase-06/system-mutations.tsv
clickhouse/phase-06/system-merges.tsv
```

### Phase 07 Backup And Restore

```bash
clickhouse/scripts/validate-phase07-backup-restore-runtime.sh
```

Important variables:

| Variable | Default |
|---|---|
| `API_BASE` | `http://192.168.35.201:30083` |
| `NS` | `upm-clickhouse-phase03-runtime` |
| `NAME` | `clickhouse-phase03` |
| `BACKUP_SECRET` | `clickhouse-backup-secret` |
| `MINIO_NAME` | `upm-backup-minio` |
| `SCHEDULE_NAME` | `validation-every-minute` |
| `DELETE_SCHEDULE_AFTER_VALIDATION` | `true` |

Set `DELETE_SCHEDULE_AFTER_VALIDATION=false` when a live demo should leave the
CronJob running.

Outputs:

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

## Recommended Full Validation Order

For the current completed MVP environment:

```bash
clickhouse/scripts/validate-phase03-upm-api-server-runtime.sh
clickhouse/scripts/validate-phase04-healthcheck-runtime.sh
clickhouse/scripts/validate-phase05-monitoring-runtime.sh
clickhouse/scripts/validate-phase06-diagnostics-runtime.sh
clickhouse/scripts/validate-phase07-backup-restore-runtime.sh
```

Run Phase 01 and Phase 02 validators only when validating the earlier package
runtime or topology package variants.
