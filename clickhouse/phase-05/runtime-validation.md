# Phase 05 Runtime Validation

- Date: 2026-06-06
- Namespace: `upm-clickhouse-phase03-runtime`
- Cluster: `clickhouse-phase03`
- API endpoint: `http://192.168.35.201:30083`
- API server image: `localhost/upmio/upm-api-server:phase-05`
- External Prometheus chart: `kube-prometheus-stack-86.2.0`
- External environment assets: `../kube-prometheus-stack/`
- Grafana endpoint: `http://192.168.35.201:30300`
- Grafana dashboard UID: `upm-clickhouse-overview`
- Grafana datasource UID: `prometheus`

## Environment Boundary

`kube-prometheus-stack` is an external environment prerequisite. Its pinned
values, installation script, and automated image synchronization script are
kept outside this repository under `../kube-prometheus-stack/`.

The lab deployment enables:

- Prometheus Operator;
- Prometheus Server;
- kubelet/cAdvisor scraping;
- cross-namespace PodMonitor discovery.
- Grafana Server;
- Prometheus datasource provisioning;
- ClickHouse dashboard provisioning.

Alertmanager, kube-state-metrics, node-exporter, admission webhooks, and
default alert rules are disabled because they are not required by Phase 05.

## Commands

```bash
SSH_PASSWORD=root ../kube-prometheus-stack/sync-images-to-nodes.sh
../kube-prometheus-stack/install.sh

cd api-server
go fmt ./...
go test ./...
go test -race ./...
go vet ./...
go build ./...
cd ..

SSH_PASSWORD=root ./clickhouse/sync-upm-api-server-image-to-nodes.sh
kubectl apply -f clickhouse/phase-03/manifests/upm-api-server.yaml
kubectl rollout restart deploy/upm-api-server -n upm-system
kubectl rollout status deploy/upm-api-server -n upm-system --timeout=180s

clickhouse/phase-05/scripts/validate-monitoring-runtime.sh
```

## Full Chain Result

The validation script completed with:

```text
PASS phase05_monitoring_runtime_validation
```

Observed summary:

```json
{
  "status": "READY",
  "cluster": "clickhouse-phase03",
  "targets": 4,
  "upTargets": 4,
  "cpuSamples": 4,
  "memorySamples": 4,
  "storageSamples": 12,
  "clickhouseSamples": 20,
  "warnings": []
}
```

Saved evidence:

- `clickhouse/phase-05/prometheus-targets.json`
- `clickhouse/phase-05/metrics-summary.json`
- `clickhouse/phase-05/grafana-dashboard.json`
- `clickhouse/phase-05/grafana-panel-query-results.json`

The proven chain is:

```text
ClickHouse native /metrics
  -> UnitSet PodMonitor
  -> Prometheus discovers 4/4 expected targets
  -> fixed PromQL queries return real samples
  -> upm-api-server returns READY metrics summary
  -> Grafana datasource reaches Prometheus
  -> Grafana ClickHouse dashboard is imported
  -> every required dashboard panel query returns real samples
```

## Grafana Result

Grafana is exposed through the external environment service:

```text
http://192.168.35.201:30300
```

The lab credential is `admin/admin`.

The validation script verified:

- datasource `prometheus` points to
  `http://kube-prometheus-stack-prometheus.monitoring.svc:9090`;
- dashboard `upm-clickhouse-overview` exists with at least 11 panels;
- all required panel queries returned 4 samples;
- panel queries are extracted from the imported Grafana dashboard JSON and
  executed through the Grafana datasource proxy after substituting the
  `namespace` and `cluster` dashboard variables.

Panel query result:

```text
PASS grafana_panel=Target Up samples=4
PASS grafana_panel=CPU Cores samples=4
PASS grafana_panel=Memory Working Set samples=4
PASS grafana_panel=Disk Used samples=4
PASS grafana_panel=Disk Available samples=4
PASS grafana_panel=Disk Total samples=4
PASS grafana_panel=Queries samples=4
PASS grafana_panel=Insert Queries samples=4
PASS grafana_panel=Inserted Rows samples=4
PASS grafana_panel=Inserted Bytes samples=4
PASS grafana_panel=ClickHouse Memory Tracking samples=4
PASS grafana_dashboard=upm-clickhouse-overview
```

## Storage Evidence Decision

The lab uses the `local-path` StorageClass. Kubelet does not expose
`kubelet_volume_stats_used_bytes` for these volumes.

The metrics API therefore uses a fixed PromQL fallback to ClickHouse native
filesystem metrics:

- `ClickHouseAsyncMetrics_DiskUsed_default`
- `ClickHouseAsyncMetrics_DiskTotal_default`
- `ClickHouseAsyncMetrics_DiskAvailable_default`

These are real data-filesystem measurements from every ClickHouse Server Pod.

## Prometheus Failure Result

Prometheus unavailability was validated by temporarily pointing the API server
at an unreachable local URL. The API returned HTTP `503`:

```json
{
  "code": "PROMETHEUS_UNAVAILABLE",
  "message": "Prometheus is unavailable",
  "requestId": "req-627983cacb1016f4"
}
```

The configured Prometheus URL was restored and the full-chain validation
passed again.

## Security Result

The saved metrics evidence contains no matches for:

```text
CLICKHOUSE_ADMIN_PASSWORD
AES_SECRET_KEY
secretKeyRef
password
```

The API also filters unstable infrastructure labels such as container IDs,
image names, scrape instances, and jobs from the summary response.
