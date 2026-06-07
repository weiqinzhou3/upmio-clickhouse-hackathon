#!/usr/bin/env bash
set -euo pipefail

API_BASE="${API_BASE:-http://192.168.35.201:30083}"
NS="${NS:-upm-clickhouse-phase03-runtime}"
NAME="${NAME:-clickhouse-phase03}"
POD="${POD:-clickhouse-phase03-0}"
OUT_DIR="${OUT_DIR:-clickhouse/phase-06}"
DIAGNOSTICS_OUT="${DIAGNOSTICS_OUT:-${OUT_DIR}/diagnostics.json}"
ROWCOUNT_OUT="${ROWCOUNT_OUT:-${OUT_DIR}/diagnostics-rowcount.json}"

mkdir -p "$OUT_DIR"

command -v kubectl >/dev/null
command -v curl >/dev/null
command -v jq >/dev/null

echo "== Runtime prerequisites =="
kubectl -n upm-system rollout status deploy/upm-api-server --timeout=180s
kubectl get pod -n "$NS" "$POD" >/dev/null

echo
echo "== Direct read-only SQL evidence =="
kubectl exec -n "$NS" "$POD" -c clickhouse -- service-ctl.sh login --query \
  "SELECT database, table, is_readonly, is_session_expired, absolute_delay, queue_size FROM system.replicas FORMAT TSV" \
  >"${OUT_DIR}/system-replicas.tsv"
kubectl exec -n "$NS" "$POD" -c clickhouse -- service-ctl.sh login --query \
  "SELECT database, table, partition, count() AS active_parts, sum(rows) AS rows, sum(bytes_on_disk) AS bytes FROM system.parts WHERE active GROUP BY database, table, partition ORDER BY active_parts DESC LIMIT 20 FORMAT TSV" \
  >"${OUT_DIR}/system-parts.tsv"
kubectl exec -n "$NS" "$POD" -c clickhouse -- service-ctl.sh login --query \
  "SELECT database, table, mutation_id, command, is_done, latest_fail_reason FROM system.mutations ORDER BY create_time DESC LIMIT 20 FORMAT TSV" \
  >"${OUT_DIR}/system-mutations.tsv"
kubectl exec -n "$NS" "$POD" -c clickhouse -- service-ctl.sh login --query \
  "SELECT database, table, elapsed, progress FROM system.merges ORDER BY elapsed DESC LIMIT 20 FORMAT TSV" \
  >"${OUT_DIR}/system-merges.tsv"
echo "PASS direct_sql_evidence_saved"

echo
echo "== Diagnostics API =="
curl -fsS "${API_BASE}/api/v1/clusters/${NS}/${NAME}/diagnostics?limit=20" | jq . >"$DIAGNOSTICS_OUT"
jq '{status, summary, categories:[.findings[].category]}' "$DIAGNOSTICS_OUT"

required_categories=(
  replica
  replication_queue
  parts
  merges
  mutations
  keeper
  storage
  write_client_stats
  write_quality
)
for category in "${required_categories[@]}"; do
  jq -e --arg category "$category" '.findings[] | select(.category == $category)' "$DIAGNOSTICS_OUT" >/dev/null
  echo "PASS category=${category}"
done

jq -e '[.findings[] | select(.severity == "CRITICAL")] | length == 0' "$DIAGNOSTICS_OUT" >/dev/null
echo "PASS no_critical_findings"

query_log_exists="$(
  kubectl exec -n "$NS" "$POD" -c clickhouse -- service-ctl.sh login --query \
    "EXISTS TABLE system.query_log FORMAT TSV" | tr -d '[:space:]'
)"
if [[ "$query_log_exists" == "0" ]]; then
  jq -e '.findings[] | select(.category == "write_client_stats" and .severity == "UNKNOWN")' "$DIAGNOSTICS_OUT" >/dev/null
  echo "PASS query_log_absent_returns_unknown"
else
  jq -e '.findings[] | select(.category == "write_client_stats")' "$DIAGNOSTICS_OUT" >/dev/null
  echo "PASS query_log_finding_present"
fi

if grep -Eiq 'CLICKHOUSE_ADMIN_PASSWORD|AES_SECRET_KEY|secretKeyRef|password' "$DIAGNOSTICS_OUT"; then
  echo "ERROR: diagnostics output contains forbidden secret-like tokens" >&2
  exit 1
fi
echo "PASS diagnostics_secret_scan"

echo
echo "== Read-only row-count validation =="
expected_rows="$(
  kubectl exec -n "$NS" "$POD" -c clickhouse -- service-ctl.sh login --query \
    "SELECT count() FROM upm_healthcheck.dist_events FORMAT TSV" | tr -d '[:space:]'
)"
if [[ -z "$expected_rows" || ! "$expected_rows" =~ ^[0-9]+$ ]]; then
  echo "ERROR: could not read upm_healthcheck.dist_events row count" >&2
  exit 1
fi

curl -fsS "${API_BASE}/api/v1/clusters/${NS}/${NAME}/diagnostics?database=upm_healthcheck&table=dist_events&expectedRows=${expected_rows}&limit=20" \
  | jq . >"$ROWCOUNT_OUT"
jq '{status, summary, writeQuality:[.findings[] | select(.category=="write_quality")][0]}' "$ROWCOUNT_OUT"
jq -e --argjson expected "$expected_rows" \
  '.findings[] | select(.category == "write_quality" and .severity == "INFO" and .evidence.actualRows == $expected and .evidence.expectedRows == $expected)' \
  "$ROWCOUNT_OUT" >/dev/null
echo "PASS write_quality_rowcount expectedRows=${expected_rows}"

if grep -Eiq 'CLICKHOUSE_ADMIN_PASSWORD|AES_SECRET_KEY|secretKeyRef|password' "$ROWCOUNT_OUT"; then
  echo "ERROR: rowcount diagnostics output contains forbidden secret-like tokens" >&2
  exit 1
fi
echo "PASS rowcount_secret_scan"

echo
echo "PASS phase06_diagnostics_runtime_validation"
