#!/usr/bin/env bash
set -euo pipefail

NS="${NS:-upm-clickhouse-phase03-runtime}"
NAME="${NAME:-clickhouse-phase03}"
EXPECTED_TARGETS="${EXPECTED_TARGETS:-4}"
API_BASE="${API_BASE:-http://192.168.35.201:30083}"
PROM_NS="${PROM_NS:-monitoring}"
PROM_SVC="${PROM_SVC:-kube-prometheus-stack-prometheus}"
PROM_PORT="${PROM_PORT:-19090}"
TARGET_OUT="${TARGET_OUT:-clickhouse/phase-05/prometheus-targets.json}"
SUMMARY_OUT="${SUMMARY_OUT:-clickhouse/phase-05/metrics-summary.json}"

command -v kubectl >/dev/null
command -v curl >/dev/null
command -v jq >/dev/null

mkdir -p "$(dirname "$TARGET_OUT")" "$(dirname "$SUMMARY_OUT")"

echo "== Runtime prerequisites =="
kubectl rollout status deploy/kube-prometheus-stack-operator -n "$PROM_NS" --timeout=180s
kubectl rollout status statefulset/prometheus-kube-prometheus-stack-prometheus -n "$PROM_NS" --timeout=300s
kubectl rollout status deploy/upm-api-server -n upm-system --timeout=180s
kubectl get podmonitor "${NAME}-exporter-podmon" -n "$NS" >/dev/null

echo
echo "== ClickHouse native endpoints =="
pods="$(
  kubectl get pods -n "$NS" \
    -l "unit-operator/unitset.name=${NAME}" \
    -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}' | sort
)"
pod_count="$(printf '%s\n' "$pods" | sed '/^$/d' | wc -l | tr -d ' ')"
if [[ "$pod_count" != "$EXPECTED_TARGETS" ]]; then
  echo "ERROR: expected ${EXPECTED_TARGETS} ClickHouse pods, got ${pod_count}" >&2
  exit 1
fi
for pod in $pods; do
  kubectl exec -n "$NS" "$pod" -c clickhouse -- \
    sh -c 'curl -fsS --max-time 5 http://127.0.0.1:9363/metrics >/tmp/phase05-metrics.out && grep -q "^ClickHouse_Info" /tmp/phase05-metrics.out'
  echo "PASS endpoint=${pod}:9363/metrics"
done

echo
echo "== Prometheus target query =="
kubectl port-forward -n "$PROM_NS" "svc/${PROM_SVC}" "${PROM_PORT}:9090" >/tmp/phase05-prometheus-port-forward.log 2>&1 &
port_forward_pid=$!
trap 'kill "$port_forward_pid" >/dev/null 2>&1 || true' EXIT
for _ in $(seq 1 30); do
  if curl -fsS "http://127.0.0.1:${PROM_PORT}/-/ready" >/dev/null 2>&1; then
    break
  fi
  sleep 1
done
curl -fsS --get "http://127.0.0.1:${PROM_PORT}/api/v1/query" \
  --data-urlencode "query=up{namespace=\"${NS}\",pod=~\"${NAME}-[0-9]+\"}" \
  | jq . >"$TARGET_OUT"

target_count="$(jq '[.data.result[] | select(.value[1]=="1")] | length' "$TARGET_OUT")"
if [[ "$target_count" != "$EXPECTED_TARGETS" ]]; then
  jq . "$TARGET_OUT"
  echo "ERROR: expected ${EXPECTED_TARGETS} up Prometheus targets, got ${target_count}" >&2
  exit 1
fi
echo "PASS prometheus_targets=${target_count}/${EXPECTED_TARGETS}"

echo
echo "== UPM API Server metrics summary =="
curl -fsS "${API_BASE}/api/v1/clusters/${NS}/${NAME}/metrics/summary" | jq . >"$SUMMARY_OUT"
jq -e --argjson expected "$EXPECTED_TARGETS" '
  .status=="READY"
  and .podMonitor.exists==true
  and ([.targets[] | select(.up==true)] | length)==$expected
  and (.summary.cpu | length)>0
  and (.summary.memory | length)>0
  and (.summary.storage | length)>0
  and (.summary.clickhouse | length)>0
  and (.warnings | length)==0
' "$SUMMARY_OUT" >/dev/null

if grep -Eiq 'CLICKHOUSE_ADMIN_PASSWORD|AES_SECRET_KEY|secretKeyRef' "$SUMMARY_OUT"; then
  echo "ERROR: metrics summary contains forbidden secret-like tokens" >&2
  exit 1
fi

jq -M '{
  status,
  cluster,
  targets: (.targets | length),
  upTargets: ([.targets[] | select(.up==true)] | length),
  cpuSamples: (.summary.cpu | length),
  memorySamples: (.summary.memory | length),
  storageSamples: (.summary.storage | length),
  clickhouseSamples: (.summary.clickhouse | length),
  warnings
}' "$SUMMARY_OUT"

echo "PASS phase05_monitoring_runtime_validation"
