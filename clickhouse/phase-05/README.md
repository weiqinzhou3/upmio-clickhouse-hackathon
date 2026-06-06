# Phase 05 Monitoring Runtime Validation

Phase 05 connects the existing ClickHouse native metrics endpoint and UnitSet
PodMonitor to a real external Prometheus Server, then exposes a summarized view
through `upm-api-server`. The external monitoring environment also provisions a
Grafana datasource and ClickHouse dashboard for demo validation.

Current lab target:

- Namespace: `upm-clickhouse-phase03-runtime`
- Cluster: `clickhouse-phase03`
- Expected ClickHouse Server targets: `4`
- API endpoint: `http://192.168.35.201:30083`
- Grafana endpoint: `http://192.168.35.201:30300`
- Grafana dashboard UID: `upm-clickhouse-overview`
- External monitoring environment assets: `../kube-prometheus-stack/`

The external kube-prometheus-stack assets are intentionally not stored in this
repository because they are environment readiness assets, not UPMIO product
code.

The lab Grafana credential is `admin/admin`.

## Deploy Updated API Server

```bash
SSH_PASSWORD=root ./clickhouse/sync-upm-api-server-image-to-nodes.sh
kubectl apply -f clickhouse/phase-03/manifests/upm-api-server.yaml
kubectl rollout restart deploy/upm-api-server -n upm-system
kubectl rollout status deploy/upm-api-server -n upm-system --timeout=180s
```

## Run Real Runtime Validation

```bash
clickhouse/phase-05/scripts/validate-monitoring-runtime.sh
```

Expected final output:

```text
PASS phase05_monitoring_runtime_validation
```

The script saves:

- `clickhouse/phase-05/prometheus-targets.json`
- `clickhouse/phase-05/metrics-summary.json`
- `clickhouse/phase-05/grafana-dashboard.json`
- `clickhouse/phase-05/grafana-panel-query-results.json`

Grafana panel validation is based on the actual imported dashboard JSON. The
script reads each panel expression, substitutes the `namespace` and `cluster`
dashboard variables, and executes the query through the Grafana datasource
proxy.

## Manual API Call

```bash
curl -fsS \
  http://192.168.35.201:30083/api/v1/clusters/upm-clickhouse-phase03-runtime/clickhouse-phase03/metrics/summary \
  | jq .
```

Expected state:

- `status=READY`
- PodMonitor exists.
- Four expected ClickHouse targets are present and up.
- CPU, memory, storage, and ClickHouse summary arrays contain real Prometheus
  samples.
- `warnings` is empty.

## Manual Grafana Checks

```bash
curl -fsS -u admin:admin http://192.168.35.201:30300/api/health | jq .
curl -fsS -u admin:admin \
  http://192.168.35.201:30300/api/dashboards/uid/upm-clickhouse-overview \
  | jq '.dashboard.title, (.dashboard.panels | length)'
```

Expected state:

- Grafana API returns `database=ok`.
- The dashboard title is `UPM ClickHouse Monitoring Overview`.
- The dashboard has at least 11 panels.
