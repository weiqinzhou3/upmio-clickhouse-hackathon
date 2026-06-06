#!/usr/bin/env bash
set -euo pipefail

NS="${NS:-upm-clickhouse-phase03-runtime}"
NAME="${NAME:-clickhouse-phase03}"
EXPECTED_TARGETS="${EXPECTED_TARGETS:-4}"
API_BASE="${API_BASE:-http://192.168.35.201:30083}"
PROM_NS="${PROM_NS:-monitoring}"
PROM_SVC="${PROM_SVC:-kube-prometheus-stack-prometheus}"
PROM_PORT="${PROM_PORT:-19090}"
GRAFANA_SVC="${GRAFANA_SVC:-kube-prometheus-stack-grafana}"
GRAFANA_PORT="${GRAFANA_PORT:-13000}"
GRAFANA_USER="${GRAFANA_USER:-admin}"
GRAFANA_PASSWORD="${GRAFANA_PASSWORD:-admin}"
GRAFANA_DASHBOARD_UID="${GRAFANA_DASHBOARD_UID:-upm-clickhouse-overview}"
TARGET_OUT="${TARGET_OUT:-clickhouse/phase-05/prometheus-targets.json}"
SUMMARY_OUT="${SUMMARY_OUT:-clickhouse/phase-05/metrics-summary.json}"
GRAFANA_DASHBOARD_OUT="${GRAFANA_DASHBOARD_OUT:-clickhouse/phase-05/grafana-dashboard.json}"
GRAFANA_PANEL_OUT="${GRAFANA_PANEL_OUT:-clickhouse/phase-05/grafana-panel-query-results.json}"

command -v kubectl >/dev/null
command -v curl >/dev/null
command -v jq >/dev/null

mkdir -p "$(dirname "$TARGET_OUT")" "$(dirname "$SUMMARY_OUT")" "$(dirname "$GRAFANA_DASHBOARD_OUT")" "$(dirname "$GRAFANA_PANEL_OUT")"

cleanup() {
  if [[ -n "${prometheus_port_forward_pid:-}" ]]; then
    kill "$prometheus_port_forward_pid" >/dev/null 2>&1 || true
  fi
  if [[ -n "${grafana_port_forward_pid:-}" ]]; then
    kill "$grafana_port_forward_pid" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

echo "== Runtime prerequisites =="
kubectl rollout status deploy/kube-prometheus-stack-operator -n "$PROM_NS" --timeout=180s
kubectl rollout status statefulset/prometheus-kube-prometheus-stack-prometheus -n "$PROM_NS" --timeout=300s
kubectl rollout status deploy/kube-prometheus-stack-grafana -n "$PROM_NS" --timeout=300s
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
prometheus_port_forward_pid=$!
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

echo
echo "== Grafana datasource and dashboard =="
kubectl port-forward -n "$PROM_NS" "svc/${GRAFANA_SVC}" "${GRAFANA_PORT}:80" >/tmp/phase05-grafana-port-forward.log 2>&1 &
grafana_port_forward_pid=$!
for _ in $(seq 1 60); do
  if curl -fsS -u "${GRAFANA_USER}:${GRAFANA_PASSWORD}" "http://127.0.0.1:${GRAFANA_PORT}/api/health" >/dev/null 2>&1; then
    break
  fi
  sleep 1
done
curl -fsS -u "${GRAFANA_USER}:${GRAFANA_PASSWORD}" \
  "http://127.0.0.1:${GRAFANA_PORT}/api/datasources/uid/prometheus" \
  | jq -e '.type=="prometheus" and .url=="http://kube-prometheus-stack-prometheus.monitoring.svc:9090"' >/dev/null
curl -fsS -u "${GRAFANA_USER}:${GRAFANA_PASSWORD}" \
  "http://127.0.0.1:${GRAFANA_PORT}/api/dashboards/uid/${GRAFANA_DASHBOARD_UID}" \
  | jq . >"$GRAFANA_DASHBOARD_OUT"
jq -e '.dashboard.title=="UPM ClickHouse Monitoring Overview" and (.dashboard.panels | length)>=11' "$GRAFANA_DASHBOARD_OUT" >/dev/null

dashboard_panel_count="$(jq '[.dashboard.panels[] | select(.targets[0].expr? != null)] | length' "$GRAFANA_DASHBOARD_OUT")"
if [[ "$dashboard_panel_count" != "11" ]]; then
  jq '.dashboard.panels[] | {title, targets}' "$GRAFANA_DASHBOARD_OUT"
  echo "ERROR: expected 11 Grafana panel queries, got ${dashboard_panel_count}" >&2
  exit 1
fi

panel_tmp="${GRAFANA_PANEL_OUT}.tmp"
printf '[' >"$panel_tmp"
first_panel=1
run_grafana_panel_query() {
  local panel="$1"
  local query="$2"
  local response count
  response="$(
    curl -fsS -u "${GRAFANA_USER}:${GRAFANA_PASSWORD}" --get \
      "http://127.0.0.1:${GRAFANA_PORT}/api/datasources/proxy/uid/prometheus/api/v1/query" \
      --data-urlencode "query=${query}"
  )"
  count="$(printf '%s' "$response" | jq '[.data.result[]] | length')"
  if [[ "$count" == "0" ]]; then
    printf '%s\n' "$response" | jq .
    echo "ERROR: Grafana dashboard panel ${panel} returned no data" >&2
    exit 1
  fi
  if [[ "$first_panel" == "0" ]]; then
    printf ',\n' >>"$panel_tmp"
  fi
  first_panel=0
  jq -nc --arg panel "$panel" --arg query "$query" --argjson resultCount "$count" \
    '{panel:$panel, query:$query, resultCount:$resultCount}' >>"$panel_tmp"
  echo "PASS grafana_panel=${panel} samples=${count}"
}

while IFS=$'\t' read -r panel query; do
  run_grafana_panel_query "$panel" "$query"
done < <(
  jq -r --arg ns "$NS" --arg cluster "$NAME" '
    .dashboard.panels[]
    | select(.targets[0].expr? != null)
    | [
        .title,
        (
          .targets[0].expr
          | gsub("\\$namespace"; $ns)
          | gsub("\\$cluster"; $cluster)
        )
      ]
    | @tsv
  ' "$GRAFANA_DASHBOARD_OUT"
)
printf ']\n' >>"$panel_tmp"
jq . "$panel_tmp" >"$GRAFANA_PANEL_OUT"
rm -f "$panel_tmp"

echo "PASS grafana_dashboard=${GRAFANA_DASHBOARD_UID}"
echo "PASS phase05_monitoring_runtime_validation"
