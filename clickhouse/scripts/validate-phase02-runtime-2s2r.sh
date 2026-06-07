#!/usr/bin/env bash
set -euo pipefail

NS="${NS:-upm-clickhouse-phase02-runtime}"
CLUSTER="${CLUSTER:-upm_cluster}"
CLICKHOUSE_UNITSET="${CLICKHOUSE_UNITSET:-clickhouse-phase02}"
KEEPER_UNITSET="${KEEPER_UNITSET:-clickhouse-phase02-keeper}"
POD="${POD:-clickhouse-phase02-0}"
DB="${DB:-phase02_runtime_validation}"

query() {
  kubectl exec -i -n "$NS" "$POD" -c clickhouse -- service-ctl.sh login "$@"
}

query_text() {
  kubectl exec -i -n "$NS" "$POD" -c clickhouse -- bash -lc 'service-ctl.sh login --multiquery'
}

optional_query() {
  query --query "$1" >/dev/null 2>&1 || true
}

echo "== Kubernetes readiness =="
kubectl wait --for=jsonpath='{.status.readyUnits}'=3 "unitset/${KEEPER_UNITSET}" -n "$NS" --timeout=360s
kubectl wait --for=jsonpath='{.status.readyUnits}'=4 "unitset/${CLICKHOUSE_UNITSET}" -n "$NS" --timeout=360s
kubectl get unitset,pods -n "$NS" -o wide

echo
echo "== ClickHouse cluster topology =="
query --query "SELECT cluster, shard_num, replica_num, host_name FROM system.clusters WHERE cluster='${CLUSTER}' ORDER BY shard_num, replica_num"

echo
echo "== ClickHouse macros =="
for unit in 0 1 2 3; do
  pod="${CLICKHOUSE_UNITSET}-${unit}"
  printf "%s\t" "$pod"
  kubectl exec -n "$NS" "$pod" -c clickhouse -- service-ctl.sh login \
    --query "SELECT getMacro('cluster'), getMacro('shard'), getMacro('replica')"
done

echo
echo "== Validation DDL and Distributed write/read =="
query_text <<SQL
DROP DATABASE IF EXISTS ${DB} ON CLUSTER ${CLUSTER} SYNC;
SQL

# If a previous destructive validation removed PVCs before dropping the table,
# Keeper may retain inactive replica metadata. Clean only this reserved path.
for shard in shard01 shard02; do
  for replica in replica01 replica02; do
    optional_query "SYSTEM DROP REPLICA '${replica}' FROM ZKPATH '/clickhouse/tables/${CLUSTER}/${shard}/${DB}/local_events'"
  done
done

query_text <<SQL
CREATE DATABASE IF NOT EXISTS ${DB} ON CLUSTER ${CLUSTER};
CREATE TABLE IF NOT EXISTS ${DB}.local_events ON CLUSTER ${CLUSTER}
(
    id UInt64,
    shard_key UInt64,
    message String
)
ENGINE = ReplicatedMergeTree('/clickhouse/tables/{cluster}/{shard}/${DB}/local_events', '{replica}')
ORDER BY id;
CREATE TABLE IF NOT EXISTS ${DB}.dist_events ON CLUSTER ${CLUSTER}
AS ${DB}.local_events
ENGINE = Distributed(${CLUSTER}, ${DB}, local_events, shard_key);
SET insert_distributed_sync = 1;
TRUNCATE TABLE ${DB}.local_events ON CLUSTER ${CLUSTER};
INSERT INTO ${DB}.dist_events VALUES
    (1, 1, 'phase02-a'),
    (2, 2, 'phase02-b'),
    (3, 3, 'phase02-c'),
    (4, 4, 'phase02-d'),
    (5, 5, 'phase02-e'),
    (6, 6, 'phase02-f'),
    (7, 7, 'phase02-g'),
    (8, 8, 'phase02-h');
SQL

for unit in 0 1 2 3; do
  pod="${CLICKHOUSE_UNITSET}-${unit}"
  kubectl exec -n "$NS" "$pod" -c clickhouse -- service-ctl.sh login \
    --query "SYSTEM SYNC REPLICA ${DB}.local_events"
done

query_text <<SQL
SELECT count() AS dist_rows, groupArray(id) AS ids FROM ${DB}.dist_events;
SELECT _shard_num, count() AS rows_per_shard, groupArray(id) AS ids
FROM ${DB}.dist_events
GROUP BY _shard_num
ORDER BY _shard_num;
SELECT hostName(), getMacro('shard') AS shard, getMacro('replica') AS replica,
       count() AS local_rows, groupArray(id) AS ids
FROM clusterAllReplicas('${CLUSTER}', ${DB}.local_events)
GROUP BY hostName(), shard, replica
ORDER BY shard, replica;
SELECT hostName(), replica_name, total_replicas, active_replicas, is_readonly, queue_size, absolute_delay
FROM clusterAllReplicas('${CLUSTER}', system.replicas)
WHERE database = '${DB}' AND table = 'local_events'
ORDER BY hostName();
SQL

query --query "SELECT throwIf(count() != 8, 'Distributed row count must be 8') FROM ${DB}.dist_events"
query --query "SELECT throwIf(count() != 2, 'Distributed table must read both shards') FROM (SELECT _shard_num FROM ${DB}.dist_events GROUP BY _shard_num)"
query --query "SELECT throwIf(sum(local_rows != 4) != 0, 'Each replica must contain 4 local rows') FROM (SELECT hostName(), count() AS local_rows FROM clusterAllReplicas('${CLUSTER}', ${DB}.local_events) GROUP BY hostName())"
query --query "SELECT throwIf(sum(total_replicas != 2 OR active_replicas != 2 OR is_readonly != 0 OR queue_size != 0) != 0, 'Replica health check failed') FROM clusterAllReplicas('${CLUSTER}', system.replicas) WHERE database = '${DB}' AND table = 'local_events'"

echo
echo "PASS runtime_2s2r_validation"
