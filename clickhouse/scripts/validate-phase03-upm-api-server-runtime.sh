#!/usr/bin/env bash
set -euo pipefail

API_SERVER_NS="${API_SERVER_NS:-upm-system}"
API_SERVER_DEPLOYMENT="${API_SERVER_DEPLOYMENT:-upm-api-server}"
API_SERVER_SERVICE="${API_SERVER_SERVICE:-upm-api-server}"
API_SERVER_SERVICE_PORT="${API_SERVER_SERVICE_PORT:-8080}"
API_SERVER_LOCAL_PORT="${API_SERVER_LOCAL_PORT:-18083}"
API_SERVER_URL="${API_SERVER_URL:-http://192.168.35.201:30083}"
START_PORT_FORWARD="${START_PORT_FORWARD:-0}"

EXISTING_NS="${EXISTING_NS:-upm-clickhouse-phase03-runtime}"
EXISTING_CLUSTER="${EXISTING_CLUSTER:-clickhouse-phase03}"
EXISTING_KEEPER="${EXISTING_KEEPER:-clickhouse-phase03-keeper}"

PHASE03_CREATE_E2E="${PHASE03_CREATE_E2E:-0}"
PHASE03_CREATE_E2E_RESET="${PHASE03_CREATE_E2E_RESET:-0}"
CREATE_NS="${CREATE_NS:-upm-clickhouse-phase03-runtime}"
CREATE_CLUSTER="${CREATE_CLUSTER:-clickhouse-phase03}"
CREATE_VERSION="${CREATE_VERSION:-26.3.9.8}"
CREATE_SHARDS="${CREATE_SHARDS:-2}"
CREATE_REPLICAS_PER_SHARD="${CREATE_REPLICAS_PER_SHARD:-2}"
CREATE_KEEPER_REPLICAS="${CREATE_KEEPER_REPLICAS:-3}"
CREATE_STORAGE_CLASS="${CREATE_STORAGE_CLASS:-local-path}"
CREATE_SERVER_SIZE="${CREATE_SERVER_SIZE:-20Gi}"
CREATE_KEEPER_SIZE="${CREATE_KEEPER_SIZE:-10Gi}"
CREATE_ADMIN_SECRET="${CREATE_ADMIN_SECRET:-clickhouse-phase03-secret}"
DB_RUNTIME_VALIDATOR="${DB_RUNTIME_VALIDATOR:-clickhouse/scripts/validate-phase02-runtime-2s2r.sh}"

port_forward_pid=""
tmpdir=""

cleanup() {
  if [[ -n "$port_forward_pid" ]]; then
    kill "$port_forward_pid" >/dev/null 2>&1 || true
    wait "$port_forward_pid" >/dev/null 2>&1 || true
  fi
  if [[ -n "$tmpdir" ]]; then
    rm -rf "$tmpdir"
  fi
}
trap cleanup EXIT

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "ERROR: required command not found: $1" >&2
    exit 1
  }
}

api_get() {
  local path="$1"
  curl -fsS "${API_SERVER_URL}${path}"
}

api_post_json() {
  local path="$1"
  local body="$2"
  local output="$3"
  curl -sS -o "$output" -w "%{http_code}" \
    -X POST "${API_SERVER_URL}${path}" \
    -H "Content-Type: application/json" \
    --data-binary @"$body"
}

wait_api_server() {
  local deadline=$((SECONDS + 60))
  until curl -fsS "${API_SERVER_URL}/api/v1/healthz" >/dev/null 2>&1; do
    if (( SECONDS > deadline )); then
      echo "ERROR: upm-api-server did not become ready at ${API_SERVER_URL}" >&2
      exit 1
    fi
    sleep 1
  done
}

connect_api_server() {
  if curl -fsS "${API_SERVER_URL}/api/v1/healthz" >/dev/null 2>&1; then
    echo "upm-api-server: using reachable Kubernetes service endpoint ${API_SERVER_URL}"
    return
  fi

  if [[ "$START_PORT_FORWARD" == "0" ]]; then
    echo "ERROR: upm-api-server is not reachable and START_PORT_FORWARD=0" >&2
    exit 1
  fi

  echo "upm-api-server: port-forward svc/${API_SERVER_SERVICE} ${API_SERVER_LOCAL_PORT}:${API_SERVER_SERVICE_PORT}"
  kubectl -n "$API_SERVER_NS" port-forward \
    "svc/${API_SERVER_SERVICE}" \
    "${API_SERVER_LOCAL_PORT}:${API_SERVER_SERVICE_PORT}" \
    >"${tmpdir}/upm-api-server-port-forward.log" 2>&1 &
  port_forward_pid="$!"
  wait_api_server
}

assert_no_secret_leak() {
  local file="$1"
  if grep -Eiq "CLICKHOUSE_ADMIN_PASSWORD|AES_SECRET_KEY|password[\" ]*:" "$file"; then
    echo "ERROR: response appears to contain secret material: $file" >&2
    cat "$file" >&2
    exit 1
  fi
}

prepare_clickhouse_secret() {
  local namespace="$1"
  local secret_name="$2"

  kubectl create namespace "$namespace" --dry-run=client -o yaml | kubectl apply -f -

  local aes_key admin_password iv_hex key_hex secret_dir
  aes_key="$(openssl rand -hex 16)"
  admin_password="$(openssl rand -base64 24)"
  iv_hex="$(openssl rand -hex 16)"
  key_hex="$(printf "%s" "$aes_key" | od -t x1 -An -v | tr -d ' \n')"
  secret_dir="${tmpdir}/clickhouse-secret"
  mkdir -p "$secret_dir"

  printf "%s" "$iv_hex" | xxd -r -p >"${secret_dir}/CLICKHOUSE_ADMIN_PASSWORD"
  printf "%s" "$admin_password" | openssl enc -aes-256-ctr -K "$key_hex" -iv "$iv_hex" >>"${secret_dir}/CLICKHOUSE_ADMIN_PASSWORD"
  cp "${secret_dir}/CLICKHOUSE_ADMIN_PASSWORD" "${secret_dir}/admin"
  cp "${secret_dir}/CLICKHOUSE_ADMIN_PASSWORD" "${secret_dir}/default"

  kubectl create secret generic aes-secret-key \
    -n "$namespace" \
    --from-literal=AES_SECRET_KEY="$aes_key" \
    --dry-run=client -o yaml | kubectl apply -f -

  kubectl create secret generic "$secret_name" \
    -n "$namespace" \
    --from-file=CLICKHOUSE_ADMIN_PASSWORD="${secret_dir}/CLICKHOUSE_ADMIN_PASSWORD" \
    --from-file=admin="${secret_dir}/admin" \
    --from-file=default="${secret_dir}/default" \
    --dry-run=client -o yaml | kubectl apply -f -
}

validate_prerequisites() {
  echo "== Real environment prerequisites =="
  kubectl get crd projects.upm.syntropycloud.io >/dev/null
  kubectl get crd unitsets.upm.syntropycloud.io >/dev/null
  kubectl get crd units.upm.syntropycloud.io >/dev/null
  kubectl get pods -n upm-system -l app.kubernetes.io/name=unit-operator >/dev/null
  kubectl wait --for=jsonpath='{.status.readyUnits}'=3 \
    "unitset/${EXISTING_KEEPER}" -n "$EXISTING_NS" --timeout=360s
  kubectl wait --for=jsonpath='{.status.readyUnits}'=4 \
    "unitset/${EXISTING_CLUSTER}" -n "$EXISTING_NS" --timeout=360s
}

validate_api_server_deployment() {
  echo "== UPM API Server Kubernetes deployment =="
  kubectl get namespace "$API_SERVER_NS" >/dev/null
  kubectl -n "$API_SERVER_NS" get serviceaccount "$API_SERVER_SERVICE" >/dev/null
  kubectl -n "$API_SERVER_NS" get deployment "$API_SERVER_DEPLOYMENT" >/dev/null
  kubectl -n "$API_SERVER_NS" get service "$API_SERVER_SERVICE" >/dev/null
  kubectl -n "$API_SERVER_NS" rollout status \
    "deployment/${API_SERVER_DEPLOYMENT}" --timeout=180s
  kubectl -n "$API_SERVER_NS" get service "$API_SERVER_SERVICE" \
    -o jsonpath='{.spec.ports[?(@.name=="http")].nodePort}' | grep -qx '30083'
}

validate_healthz() {
  echo "== UPM API Server healthz =="
  api_get "/api/v1/healthz" >"${tmpdir}/healthz.json"
  cat "${tmpdir}/healthz.json"
  echo
}

validate_structured_error() {
  echo "== Expected structured error response =="
  cat >"${tmpdir}/invalid-cluster.json" <<'JSON'
{
  "namespace": "bad namespace",
  "name": "x",
  "version": "",
  "topology": {
    "shards": 0,
    "replicasPerShard": 0,
    "keeperReplicas": 0
  }
}
JSON

  local http_code
  http_code="$(api_post_json "/api/v1/clusters" "${tmpdir}/invalid-cluster.json" "${tmpdir}/invalid-response.json")"
  if [[ "$http_code" != "400" ]]; then
    echo "ERROR: invalid cluster request returned HTTP ${http_code}, expected 400" >&2
    cat "${tmpdir}/invalid-response.json" >&2
    exit 1
  fi
  jq -e 'select(.code == "VALIDATION_ERROR" and .message and .requestId)' \
    "${tmpdir}/invalid-response.json" >/dev/null
  assert_no_secret_leak "${tmpdir}/invalid-response.json"
  cat "${tmpdir}/invalid-response.json"
  echo
  echo "PASS expected_structured_error"
}

validate_existing_cluster_read_paths() {
  echo "== Existing real cluster list/resources =="
  api_get "/api/v1/clusters" >"${tmpdir}/clusters.json"
  jq -e --arg ns "$EXISTING_NS" --arg name "$EXISTING_CLUSTER" \
    '.. | objects | select((.namespace? == $ns) and (.name? == $name))' \
    "${tmpdir}/clusters.json" >/dev/null
  assert_no_secret_leak "${tmpdir}/clusters.json"

  api_get "/api/v1/clusters/${EXISTING_NS}/${EXISTING_CLUSTER}" >"${tmpdir}/cluster.json"
  jq -e --arg ns "$EXISTING_NS" --arg name "$EXISTING_CLUSTER" \
    'select((.namespace == $ns) and (.name == $name) and (.topology.shards >= 1) and (.topology.replicasPerShard >= 1))' \
    "${tmpdir}/cluster.json" >/dev/null
  assert_no_secret_leak "${tmpdir}/cluster.json"

  api_get "/api/v1/clusters/${EXISTING_NS}/${EXISTING_CLUSTER}/resources" >"${tmpdir}/resources.json"
  grep -q "$EXISTING_CLUSTER" "${tmpdir}/resources.json"
  grep -q "$EXISTING_KEEPER" "${tmpdir}/resources.json"
  grep -Eiq "unitset|pod|pvc|service|endpoint" "${tmpdir}/resources.json"
  assert_no_secret_leak "${tmpdir}/resources.json"
  jq . "${tmpdir}/resources.json" >/dev/null
}

validate_create_e2e() {
  echo "== Optional real create E2E =="
  if [[ "$PHASE03_CREATE_E2E_RESET" == "1" ]]; then
    echo "reset: deleting reserved validation namespace ${CREATE_NS}"
    kubectl delete namespace "$CREATE_NS" --ignore-not-found --wait=true --timeout=300s
  fi
  prepare_clickhouse_secret "$CREATE_NS" "$CREATE_ADMIN_SECRET"

  cat >"${tmpdir}/create-cluster.json" <<JSON
{
  "namespace": "${CREATE_NS}",
  "name": "${CREATE_CLUSTER}",
  "version": "${CREATE_VERSION}",
  "topology": {
    "shards": ${CREATE_SHARDS},
    "replicasPerShard": ${CREATE_REPLICAS_PER_SHARD},
    "keeperReplicas": ${CREATE_KEEPER_REPLICAS}
  },
  "storage": {
    "className": "${CREATE_STORAGE_CLASS}",
    "serverDataSize": "${CREATE_SERVER_SIZE}",
    "keeperDataSize": "${CREATE_KEEPER_SIZE}"
  },
  "security": {
    "adminSecretRef": "${CREATE_ADMIN_SECRET}"
  },
  "monitoring": {
    "enabled": true
  }
}
JSON

  local http_code
  http_code="$(api_post_json "/api/v1/clusters" "${tmpdir}/create-cluster.json" "${tmpdir}/create-response.json")"
  if [[ ! "$http_code" =~ ^2 ]]; then
    echo "ERROR: create cluster returned HTTP ${http_code}" >&2
    cat "${tmpdir}/create-response.json" >&2
    exit 1
  fi
  assert_no_secret_leak "${tmpdir}/create-response.json"

  kubectl wait --for=jsonpath="{.status.readyUnits}"="${CREATE_KEEPER_REPLICAS}" \
    "unitset/${CREATE_CLUSTER}-keeper" -n "$CREATE_NS" --timeout=600s
  kubectl wait --for=jsonpath="{.status.readyUnits}"="$((CREATE_SHARDS * CREATE_REPLICAS_PER_SHARD))" \
    "unitset/${CREATE_CLUSTER}" -n "$CREATE_NS" --timeout=600s

  api_get "/api/v1/clusters/${CREATE_NS}/${CREATE_CLUSTER}/resources" >"${tmpdir}/created-resources.json"
  grep -q "$CREATE_CLUSTER" "${tmpdir}/created-resources.json"
  assert_no_secret_leak "${tmpdir}/created-resources.json"

  echo "== Created ClickHouse database topology/read-write validation =="
  kubectl exec -n "$CREATE_NS" "${CREATE_CLUSTER}-0" -c clickhouse -- \
    service-ctl.sh login --query "SELECT 1"
  kubectl exec -n "$CREATE_NS" "${CREATE_CLUSTER}-0" -c clickhouse -- \
    service-ctl.sh login --query \
    "SELECT throwIf(count() != $((CREATE_SHARDS * CREATE_REPLICAS_PER_SHARD)), 'system.clusters topology mismatch') FROM system.clusters WHERE cluster='upm_cluster'"

  if [[ "$CREATE_SHARDS" == "2" && "$CREATE_REPLICAS_PER_SHARD" == "2" && "$CREATE_KEEPER_REPLICAS" == "3" ]]; then
    if [[ ! -x "$DB_RUNTIME_VALIDATOR" ]]; then
      echo "ERROR: 2x2 database runtime validator not executable: ${DB_RUNTIME_VALIDATOR}" >&2
      exit 1
    fi
    NS="$CREATE_NS" \
      CLICKHOUSE_UNITSET="$CREATE_CLUSTER" \
      KEEPER_UNITSET="${CREATE_CLUSTER}-keeper" \
      POD="${CREATE_CLUSTER}-0" \
      DB="phase03_api_validation" \
      "$DB_RUNTIME_VALIDATOR"
  fi
}

main() {
  require_cmd curl
  require_cmd jq
  require_cmd kubectl
  require_cmd openssl
  require_cmd xxd

  tmpdir="$(mktemp -d /tmp/phase03-upm-api-server-runtime.XXXXXX)"

  validate_prerequisites
  validate_api_server_deployment
  connect_api_server
  validate_healthz
  validate_structured_error
  validate_existing_cluster_read_paths

  if [[ "$PHASE03_CREATE_E2E" == "1" ]]; then
    validate_create_e2e
  fi

  echo
  echo "PASS upm_api_server_runtime_validation"
}

main "$@"
