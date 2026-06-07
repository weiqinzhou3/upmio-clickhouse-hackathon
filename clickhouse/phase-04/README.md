# Phase 04 Runtime Healthcheck Validation

Phase 04 adds runtime healthcheck APIs to `upm-api-server`.

Current lab target:

- Namespace: `upm-clickhouse-phase03-runtime`
- Cluster: `clickhouse-phase03`
- Topology: `2 shards x 2 replicas + 3 Keeper`
- API endpoint: `http://192.168.35.201:30083`

## Deploy Updated API Server

```bash
SSH_PASSWORD=root ./clickhouse/scripts/sync-upm-api-server-image-to-nodes.sh
kubectl apply -f clickhouse/phase-03/manifests/upm-api-server.yaml
kubectl rollout status deploy/upm-api-server -n upm-system --timeout=180s
kubectl get svc upm-api-server -n upm-system -o wide
```

Expected state:

- `upm-api-server` Deployment is successfully rolled out.
- `upm-api-server` Service is `NodePort` and exposes `30083`.
- Image tag in the running Pod is `localhost/upmio/upm-api-server:phase-04`.

## Run Real Runtime Validation

```bash
clickhouse/scripts/validate-phase04-healthcheck-runtime.sh
```

Expected final output:

```text
PASS phase04_healthcheck_runtime_validation
```

The script saves:

- `clickhouse/phase-04/runtime-healthcheck-report.json`
- `clickhouse/phase-04/runtime-healthcheck-latest.json`

## Manual API Calls

Run healthcheck:

```bash
curl -sS -X POST \
  http://192.168.35.201:30083/api/v1/clusters/upm-clickhouse-phase03-runtime/clickhouse-phase03/healthcheck | jq .
```

Read latest in-memory healthcheck report:

```bash
curl -sS \
  http://192.168.35.201:30083/api/v1/clusters/upm-clickhouse-phase03-runtime/clickhouse-phase03/healthcheck/latest | jq .
```

Expected report:

- `.status` is `PASS` or `WARN`.
- `.summary.failed` is `0`.
- Critical checks such as `keeper_ruok`, `system_clusters_topology`, `write_read_probe`, and `replica_health` are `PASS`.
- `metrics_endpoint` is `PASS` when the local metrics endpoint is reachable, otherwise `WARN`.
- The report must not contain secret field names or secret values.
