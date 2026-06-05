#!/usr/bin/env bash
set -euo pipefail

API_SERVER_NS="${API_SERVER_NS:-upm-system}"
API_SERVER_DEPLOYMENT="${API_SERVER_DEPLOYMENT:-upm-api-server}"
API_SERVER_SERVICE="${API_SERVER_SERVICE:-upm-api-server}"
API_SERVER_SERVICE_PORT="${API_SERVER_SERVICE_PORT:-8080}"
API_SERVER_LOCAL_PORT="${API_SERVER_LOCAL_PORT:-18083}"
API_SERVER_URL="${API_SERVER_URL:-http://127.0.0.1:${API_SERVER_LOCAL_PORT}}"
START_PORT_FORWARD="${START_PORT_FORWARD:-auto}"

PHASE02_NS="${PHASE02_NS:-upm-clickhouse-phase02-runtime}"
PHASE02_CLUSTER="${PHASE02_CLUSTER:-clickhouse-phase02}"
PHASE02_KEEPER="${PHASE02_KEEPER:-clickhouse-phase02-keeper}"

PHASE03_CREATE_E2E="${PHASE03_CREATE_E2E:-0}"
CREATE_NS="${CREATE_NS:-upm-clickhouse-phase03-runtime}"
CREATE_CLUSTER="${CREATE_CLUSTER:-clickhouse-phase03}"
CREATE_VERSION="${CREATE_VERSION:-26.3.9.8}"
CREATE_STORAGE_CLASS="${CREATE_STORAGE_CLASS:-local-path}"
CREATE_SERVER_SIZE="${CREATE_SERVER_SIZE:-20Gi}"
CREATE_KEEPER_SIZE="${CREATE_KEEPER_SIZE:-10Gi}"
CREATE_ADMIN_SECRET="${CREATE_ADMIN_SECRET:-clickhouse-phase03-secret}"

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

  local aes_key admin_password
  aes_key="$(openssl rand -hex 16)"
  admin_password="$(openssl rand -base64 24)"

  encrypt_value() {
    local value="$1"
    local iv
    iv="$(openssl rand -hex 16)"
    {
      printf "%s" "$iv" | xxd -r -p
      printf "%s" "$value" | openssl enc -aes-256-ctr -K "$(printf "%s" "$aes_key" | od -t x1 -An -v | tr -d ' \n')" -iv "$iv"
    } | base64 | tr -d '\n'
  }

  local encrypted
  encrypted="$(encrypt_value "$admin_password")"

  kubectl create secret generic aes-secret-key \
    -n "$namespace" \
    --from-literal=AES_SECRET_KEY="$aes_key" \
    --dry-run=client -o yaml | kubectl apply -f -

  kubectl create secret generic "$secret_name" \
    -n "$namespace" \
    --from-literal=CLICKHOUSE_ADMIN_PASSWORD="$encrypted" \
    --from-literal=admin="$encrypted" \
    --from-literal=default="$encrypted" \
    --dry-run=client -o yaml | kubectl apply -f -
}

validate_prerequisites() {
  echo "== Real environment prerequisites =="
  kubectl get crd projects.upm.syntropycloud.io >/dev/null
  kubectl get crd unitsets.upm.syntropycloud.io >/dev/null
  kubectl get crd units.upm.syntropycloud.io >/dev/null
  kubectl get pods -n upm-system -l app.kubernetes.io/name=unit-operator >/dev/null
  kubectl wait --for=jsonpath='{.status.readyUnits}'=3 \
    "unitset/${PHASE02_KEEPER}" -n "$PHASE02_NS" --timeout=360s
  kubectl wait --for=jsonpath='{.status.readyUnits}'=4 \
    "unitset/${PHASE02_CLUSTER}" -n "$PHASE02_NS" --timeout=360s
}

validate_api_server_deployment() {
  echo "== UPM API Server Kubernetes deployment =="
  kubectl get namespace "$API_SERVER_NS" >/dev/null
  kubectl -n "$API_SERVER_NS" get serviceaccount "$API_SERVER_SERVICE" >/dev/null
  kubectl -n "$API_SERVER_NS" get deployment "$API_SERVER_DEPLOYMENT" >/dev/null
  kubectl -n "$API_SERVER_NS" get service "$API_SERVER_SERVICE" >/dev/null
  kubectl -n "$API_SERVER_NS" rollout status \
    "deployment/${API_SERVER_DEPLOYMENT}" --timeout=180s
}

validate_healthz() {
  echo "== UPM API Server healthz =="
  api_get "/api/v1/healthz" >"${tmpdir}/healthz.json"
  cat "${tmpdir}/healthz.json"
  echo
}

validate_structured_error() {
  echo "== Structured error response =="
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
  if [[ "$http_code" =~ ^2 ]]; then
    echo "ERROR: invalid cluster request returned HTTP ${http_code}" >&2
    cat "${tmpdir}/invalid-response.json" >&2
    exit 1
  fi
  jq -e '.code and .message and .requestId' "${tmpdir}/invalid-response.json" >/dev/null
  assert_no_secret_leak "${tmpdir}/invalid-response.json"
  cat "${tmpdir}/invalid-response.json"
  echo
}

validate_existing_cluster_read_paths() {
  echo "== Existing real cluster list/resources =="
  api_get "/api/v1/clusters" >"${tmpdir}/clusters.json"
  jq -e --arg ns "$PHASE02_NS" --arg name "$PHASE02_CLUSTER" \
    '.. | objects | select((.namespace? == $ns) and (.name? == $name))' \
    "${tmpdir}/clusters.json" >/dev/null
  assert_no_secret_leak "${tmpdir}/clusters.json"

  api_get "/api/v1/clusters/${PHASE02_NS}/${PHASE02_CLUSTER}/resources" >"${tmpdir}/resources.json"
  grep -q "$PHASE02_CLUSTER" "${tmpdir}/resources.json"
  grep -q "$PHASE02_KEEPER" "${tmpdir}/resources.json"
  grep -Eiq "unitset|pod|pvc|service|endpoint" "${tmpdir}/resources.json"
  assert_no_secret_leak "${tmpdir}/resources.json"
  jq . "${tmpdir}/resources.json" >/dev/null
}

validate_create_e2e() {
  echo "== Optional real create E2E =="
  prepare_clickhouse_secret "$CREATE_NS" "$CREATE_ADMIN_SECRET"

  cat >"${tmpdir}/create-cluster.json" <<JSON
{
  "namespace": "${CREATE_NS}",
  "name": "${CREATE_CLUSTER}",
  "version": "${CREATE_VERSION}",
  "topology": {
    "shards": 1,
    "replicasPerShard": 2,
    "keeperReplicas": 3
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

  kubectl wait --for=jsonpath='{.status.readyUnits}'=3 \
    "unitset/${CREATE_CLUSTER}-keeper" -n "$CREATE_NS" --timeout=600s
  kubectl wait --for=jsonpath='{.status.readyUnits}'=2 \
    "unitset/${CREATE_CLUSTER}" -n "$CREATE_NS" --timeout=600s

  api_get "/api/v1/clusters/${CREATE_NS}/${CREATE_CLUSTER}/resources" >"${tmpdir}/created-resources.json"
  grep -q "$CREATE_CLUSTER" "${tmpdir}/created-resources.json"
  assert_no_secret_leak "${tmpdir}/created-resources.json"
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
