# Phase 05 Monitoring Runtime Validation

Phase 05 connects the existing ClickHouse native metrics endpoint and UnitSet
PodMonitor to a real external Prometheus Server, then exposes a summarized view
through `upm-api-server`.

Current lab target:

- Namespace: `upm-clickhouse-phase03-runtime`
- Cluster: `clickhouse-phase03`
- Expected ClickHouse Server targets: `4`
- API endpoint: `http://192.168.35.201:30083`
- External monitoring environment assets: `../kube-prometheus-stack/`

The external kube-prometheus-stack assets are intentionally not stored in this
repository because they are environment readiness assets, not UPMIO product
code.

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
