#!/usr/bin/env bash
set -euo pipefail

NS="${NS:-upm-clickhouse-phase03-runtime}"
NAME="${NAME:-clickhouse-phase03}"
API_BASE="${API_BASE:-http://192.168.35.201:30083}"
POD="${POD:-${NAME}-0}"
OUT="${OUT:-clickhouse/phase-04/runtime-healthcheck-report.json}"
LATEST_OUT="${LATEST_OUT:-clickhouse/phase-04/runtime-healthcheck-latest.json}"

command -v kubectl >/dev/null
command -v curl >/dev/null
command -v jq >/dev/null

mkdir -p "$(dirname "$OUT")"

echo "== Kubernetes target readiness =="
kubectl get namespace "$NS" >/dev/null
kubectl wait --for=jsonpath='{.status.readyUnits}'=3 "unitset/${NAME}-keeper" -n "$NS" --timeout=180s
kubectl wait --for=jsonpath='{.status.readyUnits}'=4 "unitset/${NAME}" -n "$NS" --timeout=180s
kubectl rollout status deploy/upm-api-server -n upm-system --timeout=180s

echo
echo "== Run healthcheck through NodePort API =="
curl -sS -X POST "${API_BASE}/api/v1/clusters/${NS}/${NAME}/healthcheck" -o "${OUT}.tmp"
jq . "${OUT}.tmp" >"$OUT"
rm -f "${OUT}.tmp"

status="$(jq -r '.status' "$OUT")"
if [[ "$status" != "PASS" && "$status" != "WARN" ]]; then
  jq . "$OUT"
  echo "ERROR: expected healthcheck status PASS or WARN, got ${status}" >&2
  exit 1
fi

failed="$(jq -r '.summary.failed' "$OUT")"
if [[ "$failed" != "0" ]]; then
  jq '.checks[] | select(.status=="FAIL")' "$OUT"
  echo "ERROR: expected zero failed checks, got ${failed}" >&2
  exit 1
fi

for check in \
  upmio_project_exists \
  keeper_unitset_ready \
  server_unitset_ready \
  pods_ready \
  pvc_bound \
  services_endpoints \
  keeper_ruok \
  keeper_leader_follower \
  clickhouse_select_1 \
  system_clusters_topology \
  write_read_probe \
  replica_health \
  no_secret_leakage
do
  jq -e --arg check "$check" '.checks[] | select(.name==$check and .status=="PASS")' "$OUT" >/dev/null
done

jq -e '.checks[] | select(.name=="metrics_endpoint" and (.status=="PASS" or .status=="WARN"))' "$OUT" >/dev/null
jq -e '.cluster==.name and (.durationMs >= 0)' "$OUT" >/dev/null

if grep -Eiq 'CLICKHOUSE_ADMIN_PASSWORD|AES_SECRET_KEY|secretKeyRef' "$OUT"; then
  echo "ERROR: healthcheck report contains forbidden secret-like tokens" >&2
  exit 1
fi

echo
echo "== Verify latest report cache =="
curl -sS "${API_BASE}/api/v1/clusters/${NS}/${NAME}/healthcheck/latest" -o "${LATEST_OUT}.tmp"
jq . "${LATEST_OUT}.tmp" >"$LATEST_OUT"
rm -f "${LATEST_OUT}.tmp"
jq -e --arg startedAt "$(jq -r '.startedAt' "$OUT")" '.startedAt==$startedAt' "$LATEST_OUT" >/dev/null
jq -e --arg status "$status" '.status==$status' "$LATEST_OUT" >/dev/null

echo
echo "== Cross-check ClickHouse healthcheck data =="
probe_rows="$(jq -r '.checks[] | select(.name=="write_read_probe") | .evidence.insertedRows' "$OUT")"
expected_shards="$(jq -r '.checks[] | select(.name=="system_clusters_topology") | .evidence.expectedShards' "$OUT")"
expected_replicas="$(jq -r '.checks[] | select(.name=="system_clusters_topology") | .evidence.expectedReplicas' "$OUT")"
row_count="$(
  kubectl exec -n "$NS" "$POD" -c clickhouse -- \
    service-ctl.sh login --query 'SELECT count() FROM upm_healthcheck.dist_events FORMAT TSV' | tr -d '\r'
)"
if [[ "$row_count" != "$probe_rows" ]]; then
  echo "ERROR: expected upm_healthcheck.dist_events row count ${probe_rows}, got ${row_count}" >&2
  exit 1
fi

kubectl exec -n "$NS" "$POD" -c clickhouse -- service-ctl.sh login --query \
  "SELECT throwIf(count() != ${expected_shards}, 'Distributed table must read every shard') FROM (SELECT _shard_num FROM upm_healthcheck.dist_events GROUP BY _shard_num)" >/dev/null

kubectl exec -n "$NS" "$POD" -c clickhouse -- service-ctl.sh login --query \
  "SELECT throwIf(sum(total_replicas != ${expected_replicas} OR active_replicas != ${expected_replicas} OR is_readonly != 0 OR queue_size != 0) != 0, 'Replica health check failed') FROM clusterAllReplicas('upm_cluster', system.replicas) WHERE database = 'upm_healthcheck' AND table = 'local_events'" >/dev/null

echo
echo "PASS phase04_healthcheck_runtime_validation"
