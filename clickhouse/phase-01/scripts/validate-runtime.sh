#!/usr/bin/env bash
set -euo pipefail

namespace="${NAMESPACE:-upm-clickhouse-runtime}"
clickhouse_unitset="${CLICKHOUSE_UNITSET:-clickhouse-runtime}"
keeper_unitset="${KEEPER_UNITSET:-clickhouse-runtime-keeper}"
package_namespace="${PACKAGE_NAMESPACE:-upm-system}"
package_podtemplate="${PACKAGE_PODTEMPLATE:-clickhouse-26.3.9.8}"
expected_image="${EXPECTED_CLICKHOUSE_IMAGE:-localhost/upmio/clickhouse:26.3.9.8-runtime}"

section() {
  printf "\n## %s\n" "$1"
}

require_equal() {
  local name="$1"
  local expected="$2"
  local actual="$3"
  if [[ "$actual" != "$expected" ]]; then
    printf "FAIL %s expected=%s actual=%s\n" "$name" "$expected" "$actual" >&2
    exit 1
  fi
  printf "PASS %s=%s\n" "$name" "$actual"
}

section "Runtime Objects"
kubectl get unitsets,units,pods,pvc,svc,podmonitor -n "$namespace" -o wide

keeper_ready="$(kubectl get unitset "$keeper_unitset" -n "$namespace" -o jsonpath='{.status.readyUnits}')"
keeper_current="$(kubectl get unitset "$keeper_unitset" -n "$namespace" -o jsonpath='{.status.units}')"
clickhouse_ready="$(kubectl get unitset "$clickhouse_unitset" -n "$namespace" -o jsonpath='{.status.readyUnits}')"
clickhouse_current="$(kubectl get unitset "$clickhouse_unitset" -n "$namespace" -o jsonpath='{.status.units}')"
require_equal "keeper_current" "3" "$keeper_current"
require_equal "keeper_ready" "3" "$keeper_ready"
require_equal "clickhouse_current" "2" "$clickhouse_current"
require_equal "clickhouse_ready" "2" "$clickhouse_ready"

section "Package Image"
actual_image="$(kubectl get podtemplate "$package_podtemplate" -n "$package_namespace" -o jsonpath='{.template.spec.containers[?(@.name=="clickhouse")].image}')"
image_pull_policy="$(kubectl get podtemplate "$package_podtemplate" -n "$package_namespace" -o jsonpath='{.template.spec.containers[?(@.name=="clickhouse")].imagePullPolicy}')"
require_equal "clickhouse_image" "$expected_image" "$actual_image"
printf "PASS imagePullPolicy=%s\n" "$image_pull_policy"

clickhouse_pod="$(kubectl get pod -n "$namespace" -l "unit-operator/unitset.name=$clickhouse_unitset" -o jsonpath='{.items[0].metadata.name}')"

section "Config Checks"
kubectl exec -n "$namespace" "$clickhouse_pod" -c clickhouse -- bash -lc 'grep -q "<prometheus>" /var/lib/clickhouse/conf/config.xml'
printf "PASS prometheus-config=present\n"
kubectl exec -n "$namespace" "$clickhouse_pod" -c clickhouse -- bash -lc '! grep -q "<no_password" /var/lib/clickhouse/conf/config.xml'
printf "PASS no_password=absent\n"
kubectl exec -n "$namespace" "$clickhouse_pod" -c clickhouse -- bash -lc '! grep -q "max_concurrent_queries" /var/lib/clickhouse/conf/config.xml'
printf "PASS max_concurrent_queries=absent\n"
kubectl exec -n "$namespace" "$clickhouse_pod" -c clickhouse -- bash -lc 'grep -q "password_sha256_hex" /var/lib/clickhouse/conf/config.xml'
printf "PASS password_sha256_hex=present\n"

section "Keeper Health"
leader_count=0
follower_count=0
for pod in $(kubectl get pod -n "$namespace" -l "unit-operator/unitset.name=$keeper_unitset" -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}'); do
  ruok="$(kubectl exec -n "$namespace" "$pod" -c clickhouse-keeper -- timeout 3 bash -lc 'exec 3<>/dev/tcp/127.0.0.1/9181; printf ruok >&3; dd bs=1 count=4 <&3 2>/dev/null')"
  state="$(kubectl exec -n "$namespace" "$pod" -c clickhouse-keeper -- timeout 3 bash -lc 'exec 3<>/dev/tcp/127.0.0.1/9181; printf mntr >&3; cat <&3' | awk '/zk_server_state/ {print $2}')"
  printf "%s ruok=%s state=%s\n" "$pod" "$ruok" "$state"
  [[ "$ruok" == "imok" ]] || exit 1
  case "$state" in
    leader) leader_count=$((leader_count + 1)) ;;
    follower) follower_count=$((follower_count + 1)) ;;
    *) printf "FAIL unexpected_keeper_state=%s pod=%s\n" "$state" "$pod" >&2; exit 1 ;;
  esac
done
require_equal "keeper_leaders" "1" "$leader_count"
require_equal "keeper_followers" "2" "$follower_count"

section "Authentication"
kubectl exec -n "$namespace" "$clickhouse_pod" -c clickhouse -- service-ctl.sh health
printf "PASS service_ctl_health=pass\n"
kubectl exec -n "$namespace" "$clickhouse_pod" -c clickhouse -- bash -lc 'if clickhouse-client --query "SELECT 1" >/tmp/no-password.out 2>/tmp/no-password.err; then exit 1; else exit 0; fi'
printf "PASS no_password_login=rejected\n"

section "SQL Topology"
kubectl exec -n "$namespace" "$clickhouse_pod" -c clickhouse -- service-ctl.sh login --query "SELECT 1 AS ok"
topology_rows="$(kubectl exec -n "$namespace" "$clickhouse_pod" -c clickhouse -- service-ctl.sh login --query "SELECT count() FROM system.clusters WHERE cluster='upm_cluster'")"
require_equal "upm_cluster_replicas" "2" "$topology_rows"
kubectl exec -n "$namespace" "$clickhouse_pod" -c clickhouse -- service-ctl.sh login --query "SELECT cluster, shard_num, replica_num, host_name, port FROM system.clusters WHERE cluster='upm_cluster' ORDER BY shard_num, replica_num"

section "Replication"
for pod in $(kubectl get pod -n "$namespace" -l "unit-operator/unitset.name=$clickhouse_unitset" -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}'); do
  row_result="$(kubectl exec -n "$namespace" "$pod" -c clickhouse -- service-ctl.sh login --query "SELECT concat(toString(count()), ' ', any(message)) FROM runtime_validation.events")"
  printf "%s %s\n" "$pod" "$row_result"
  [[ "$row_result" == "1 runtime-ok" ]] || exit 1
done
replica_status="$(kubectl exec -n "$namespace" "$clickhouse_pod" -c clickhouse -- service-ctl.sh login --query "SELECT concat(toString(sum(is_readonly)), ' ', toString(sum(is_session_expired)), ' ', toString(sum(absolute_delay)), ' ', toString(sum(queue_size))) FROM system.replicas WHERE database='runtime_validation' AND table='events'")"
require_equal "replica_status_sums" "0 0 0 0" "$replica_status"

section "Metrics"
kubectl exec -n "$namespace" "$clickhouse_pod" -c clickhouse -- bash -lc 'curl -sS --max-time 5 http://127.0.0.1:9363/metrics > /tmp/clickhouse-metrics.out && grep -q "ClickHouse_Info" /tmp/clickhouse-metrics.out && head -n 5 /tmp/clickhouse-metrics.out'
printf "PASS metrics_endpoint=ready\n"

section "Summary"
printf "PASS phase01_runtime_validation\n"
