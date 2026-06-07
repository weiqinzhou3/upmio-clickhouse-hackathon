# Demo Guide

## Demo Goal

Show that the project is not only a design document: it is a Kubernetes-native
ClickHouse operations prototype with API, UPMIO CRD orchestration, monitoring,
diagnostics, backup, restore, runtime validation, and AI collaboration
evidence.

## Demo Environment

```bash
export UPM_API_SERVER_URL=http://192.168.35.201:30083
export NS=upm-clickhouse-phase03-runtime
export NAME=clickhouse-phase03
```

## Recommended Demo Flow

### 1. Show Running Control Plane

```bash
kubectl -n upm-system get deploy,svc upm-api-server -o wide
curl -fsS "${UPM_API_SERVER_URL}/api/v1/healthz" | jq .
```

Message:

- `upm-api-server` is deployed inside Kubernetes.
- The API is reachable through NodePort `30083`.

### 2. Show UPMIO-Managed ClickHouse Resources

```bash
kubectl -n "${NS}" get unitsets,units,pods,pvc,svc,podmonitor -o wide
curl -fsS "${UPM_API_SERVER_URL}/api/v1/clusters/${NS}/${NAME}/resources" | jq .
```

Message:

- ClickHouse is not hand-installed on nodes.
- UPMIO `Project`, `UnitSet`, and package assets drive the runtime.

### 3. Run Online Healthcheck

```bash
clickhouse/scripts/validate-phase04-healthcheck-runtime.sh
```

Expected final line:

```text
PASS phase04_healthcheck_runtime_validation
```

Message:

- The API checks Kubernetes readiness, Keeper, ClickHouse SQL, topology,
  write/read, replica health, metrics endpoint, and secret leakage.

### 4. Show Metrics And Grafana

```bash
clickhouse/scripts/validate-phase05-monitoring-runtime.sh
```

Expected final line:

```text
PASS phase05_monitoring_runtime_validation
```

Useful evidence files:

```text
clickhouse/phase-05/prometheus-targets.json
clickhouse/phase-05/metrics-summary.json
clickhouse/phase-05/grafana-panel-query-results.json
```

Message:

- The chain is ClickHouse `/metrics` -> PodMonitor -> Prometheus ->
  `upm-api-server` summary -> Grafana dashboard.
- The dashboard panel queries were validated through Grafana's datasource proxy.

### 5. Run Day2 Diagnostics

```bash
clickhouse/scripts/validate-phase06-diagnostics-runtime.sh
```

Expected final line:

```text
PASS phase06_diagnostics_runtime_validation
```

Message:

- Diagnostics are read-only.
- They cover replicas, replication queue, parts, merges, mutations, Keeper,
  storage, write client stats, and write quality row count.

### 6. Run Backup, Restore, And Scheduled Backup

```bash
clickhouse/scripts/validate-phase07-backup-restore-runtime.sh
```

Expected final line:

```text
PASS phase07_backup_restore_runtime_validation
```

Message:

- Immediate backup creates a Kubernetes Job through API.
- Restore creates a Kubernetes Job through API and restores into a fresh target
  validation table.
- Scheduled backup creates a Kubernetes CronJob through API.
- Data correctness is checked by row count and checksum.

### 7. Show AI Evidence And Review Loop

```bash
sed -n '1,120p' docs/ai-usage-phase02.md
find docs/review -maxdepth 1 -type f | sort
git log --oneline --decorate --all | head -20
```

Message:

- AI output was not accepted as evidence by itself.
- Codex implemented and validated.
- Claude Code reviewed.
- Human decisions adjusted scope and accepted or rejected findings.
- Every phase has runtime evidence and review artifacts.

## Short Demo Script

When time is limited, run only:

```bash
curl -fsS "${UPM_API_SERVER_URL}/api/v1/healthz" | jq .
clickhouse/scripts/validate-phase04-healthcheck-runtime.sh
clickhouse/scripts/validate-phase05-monitoring-runtime.sh
clickhouse/scripts/validate-phase07-backup-restore-runtime.sh
```

This shows control plane, healthcheck, monitoring/Grafana, backup, restore, and
scheduled backup.

## Demo Risks

| Risk | Mitigation |
|---|---|
| External image pull is slow | Use node-local image sync scripts before demo |
| CronJob takes up to one minute | Start Phase 07 validation early or explain the wait |
| Grafana admin password differs | Set `GRAFANA_USER` and `GRAFANA_PASSWORD` before Phase 05 validation |
| Old Jobs confuse viewers | Keep only final successful Jobs before demo |
| NodePort is unreachable | Use `kubectl port-forward -n upm-system svc/upm-api-server 18083:8080` |
