#!/usr/bin/env bash
set -euo pipefail

API_BASE="${API_BASE:-http://192.168.35.201:30083}"
NS="${NS:-upm-clickhouse-phase03-runtime}"
NAME="${NAME:-clickhouse-phase03}"
POD="${POD:-${NAME}-0}"
OUT_DIR="${OUT_DIR:-clickhouse/phase-07}"

SOURCE_DB="${SOURCE_DB:-upm_backup_validation}"
SOURCE_TABLE="${SOURCE_TABLE:-events}"
SOURCE_DIST_TABLE="${SOURCE_DIST_TABLE:-events_dist}"
RESTORE_DB="${RESTORE_DB:-upm_restore_validation}"
RESTORE_TABLE="${RESTORE_TABLE:-events_restored}"
SCHEDULED_RESTORE_TABLE="${SCHEDULED_RESTORE_TABLE:-events_scheduled_restored}"

BACKUP_SECRET="${BACKUP_SECRET:-clickhouse-backup-secret}"
MINIO_NAME="${MINIO_NAME:-upm-backup-minio}"
MINIO_IMAGE="${MINIO_IMAGE:-quay.io/minio/minio:RELEASE.2025-04-22T22-12-26Z}"
MC_IMAGE="${MC_IMAGE:-quay.io/minio/mc:RELEASE.2025-04-16T18-13-26Z}"
MINIO_ACCESS_KEY="${MINIO_ACCESS_KEY:-}"
MINIO_SECRET_KEY="${MINIO_SECRET_KEY:-}"
MINIO_BUCKET="${MINIO_BUCKET:-clickhouse-backups}"

SCHEDULE_NAME="${SCHEDULE_NAME:-validation-every-minute}"
SCHEDULE_CRON="${SCHEDULE_CRON:-*/1 * * * *}"
DELETE_SCHEDULE_AFTER_VALIDATION="${DELETE_SCHEDULE_AFTER_VALIDATION:-true}"

RUN_ID="$(date -u +%Y%m%dT%H%M%SZ)"
MANUAL_PATH="${MANUAL_PATH:-backups/${NAME}/manual-${RUN_ID}}"
SCHEDULE_PREFIX="${SCHEDULE_PREFIX:-backups/${NAME}/scheduled}"

BACKUP_RESPONSE="${OUT_DIR}/backup-response.json"
BACKUP_TASK="${OUT_DIR}/backup-task.json"
RESTORE_RESPONSE="${OUT_DIR}/restore-response.json"
RESTORE_TASK="${OUT_DIR}/restore-task.json"
SCHEDULE_RESPONSE="${OUT_DIR}/backup-schedule-response.json"
SCHEDULE_DETAIL="${OUT_DIR}/backup-schedule-detail.json"
SCHEDULED_RESTORE_RESPONSE="${OUT_DIR}/scheduled-restore-response.json"
SCHEDULED_RESTORE_TASK="${OUT_DIR}/scheduled-restore-task.json"
SOURCE_CHECKSUM="${OUT_DIR}/source-checksum.tsv"
RESTORE_CHECKSUM="${OUT_DIR}/restore-checksum.tsv"
SCHEDULED_RESTORE_CHECKSUM="${OUT_DIR}/scheduled-restore-checksum.tsv"

mkdir -p "$OUT_DIR"

command -v kubectl >/dev/null
command -v curl >/dev/null
command -v jq >/dev/null

random_hex() {
  local bytes="$1"
  od -An -N "$bytes" -tx1 /dev/urandom | tr -d ' \n'
}

existing_secret_value() {
  local secret="$1"
  local key="$2"
  kubectl -n "$NS" get secret "$secret" -o json 2>/dev/null \
    | jq -r --arg key "$key" '.data[$key] // empty' \
    | base64 -d 2>/dev/null || true
}

if [[ -z "$MINIO_ACCESS_KEY" ]]; then
  MINIO_ACCESS_KEY="$(existing_secret_value "${MINIO_NAME}-credentials" MINIO_ROOT_USER)"
fi
if [[ -z "$MINIO_SECRET_KEY" ]]; then
  MINIO_SECRET_KEY="$(existing_secret_value "${MINIO_NAME}-credentials" MINIO_ROOT_PASSWORD)"
fi
if [[ -z "$MINIO_ACCESS_KEY" ]]; then
  MINIO_ACCESS_KEY="upm$(random_hex 6)"
fi
if [[ -z "$MINIO_SECRET_KEY" ]]; then
  MINIO_SECRET_KEY="upm$(random_hex 16)"
fi

api_json() {
  local method="$1"
  local path="$2"
  local body="${3:-}"
  if [[ -n "$body" ]]; then
    curl -fsS -X "$method" \
      -H "Content-Type: application/json" \
      -H "X-Actor: phase-07-runtime-validation" \
      --data-binary "$body" \
      "${API_BASE}${path}"
  else
    curl -fsS -X "$method" \
      -H "X-Actor: phase-07-runtime-validation" \
      "${API_BASE}${path}"
  fi
}

run_sql() {
  kubectl exec -i -n "$NS" "$POD" -c clickhouse -- bash -lc 'service-ctl.sh login --multiquery'
}

query_tsv() {
  local sql="$1"
  kubectl exec -n "$NS" "$POD" -c clickhouse -- service-ctl.sh login --query "$sql" | tr -d '\r'
}

wait_task() {
  local task_name="$1"
  local output="$2"
  local deadline=$((SECONDS + 240))
  while (( SECONDS < deadline )); do
    api_json GET "/api/v1/clusters/${NS}/${NAME}/tasks/${task_name}" | jq . >"${output}.tmp"
    mv "${output}.tmp" "$output"
    local status
    status="$(jq -r '.status' "$output")"
    case "$status" in
      Succeeded)
        return 0
        ;;
      Failed)
        jq . "$output"
        echo "ERROR: task ${task_name} failed" >&2
        return 1
        ;;
      Pending|Running|Unknown)
        sleep 5
        ;;
      *)
        jq . "$output"
        echo "ERROR: task ${task_name} returned unexpected status ${status}" >&2
        return 1
        ;;
    esac
  done
  jq . "$output" || true
  echo "ERROR: timed out waiting for task ${task_name}" >&2
  return 1
}

wait_scheduled_backup() {
  local deadline=$((SECONDS + 240))
  while (( SECONDS < deadline )); do
    api_json GET "/api/v1/clusters/${NS}/${NAME}/backup-schedules/${SCHEDULE_NAME}" | jq . >"${SCHEDULE_DETAIL}.tmp"
    mv "${SCHEDULE_DETAIL}.tmp" "$SCHEDULE_DETAIL"
    if jq -e '.recentJobs[]? | select(.status == "Failed")' "$SCHEDULE_DETAIL" >/dev/null; then
      jq . "$SCHEDULE_DETAIL"
      echo "ERROR: scheduled backup child Job failed" >&2
      return 1
    fi
    if jq -e '.recentJobs[]? | select(.status == "Succeeded")' "$SCHEDULE_DETAIL" >/dev/null; then
      return 0
    fi
    sleep 10
  done
  jq . "$SCHEDULE_DETAIL" || true
  echo "ERROR: timed out waiting for scheduled backup child Job" >&2
  return 1
}

checksum_sql() {
  local database="$1"
  local table="$2"
  printf "SELECT count() AS rows, sum(cityHash64(id, shard_key, message)) AS checksum FROM clusterAllReplicas('upm_cluster', %s, %s) FORMAT TSV" "$database" "$table"
}

assert_no_secret_leakage() {
  local file="$1"
  if grep -Eiq "CLICKHOUSE_ADMIN_PASSWORD|AES_SECRET_KEY|S3_ACCESS_KEY|S3_SECRET_KEY|${MINIO_ACCESS_KEY}|${MINIO_SECRET_KEY}" "$file"; then
    echo "ERROR: ${file} contains forbidden secret-like content" >&2
    exit 1
  fi
}

echo "== Runtime prerequisites =="
kubectl -n upm-system rollout status deploy/upm-api-server --timeout=180s
kubectl get namespace "$NS" >/dev/null
kubectl get pod -n "$NS" "$POD" >/dev/null
kubectl wait --for=jsonpath='{.status.readyUnits}'=3 "unitset/${NAME}-keeper" -n "$NS" --timeout=180s
kubectl wait --for=jsonpath='{.status.readyUnits}'=4 "unitset/${NAME}" -n "$NS" --timeout=180s
api_json GET /api/v1/healthz | jq .

echo
echo "== Prepare validation object storage =="
kubectl -n "$NS" apply -f - <<EOF
apiVersion: v1
kind: Secret
metadata:
  name: ${MINIO_NAME}-credentials
type: Opaque
stringData:
  MINIO_ROOT_USER: ${MINIO_ACCESS_KEY}
  MINIO_ROOT_PASSWORD: ${MINIO_SECRET_KEY}
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ${MINIO_NAME}
  labels:
    app.kubernetes.io/name: ${MINIO_NAME}
    app.kubernetes.io/part-of: upmio-validation
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: ${MINIO_NAME}
  template:
    metadata:
      labels:
        app.kubernetes.io/name: ${MINIO_NAME}
        app.kubernetes.io/part-of: upmio-validation
    spec:
      containers:
      - name: minio
        image: ${MINIO_IMAGE}
        imagePullPolicy: IfNotPresent
        args: ["server", "/data"]
        envFrom:
        - secretRef:
            name: ${MINIO_NAME}-credentials
        ports:
        - name: api
          containerPort: 9000
        volumeMounts:
        - name: data
          mountPath: /data
      volumes:
      - name: data
        emptyDir: {}
---
apiVersion: v1
kind: Service
metadata:
  name: ${MINIO_NAME}
  labels:
    app.kubernetes.io/name: ${MINIO_NAME}
    app.kubernetes.io/part-of: upmio-validation
spec:
  selector:
    app.kubernetes.io/name: ${MINIO_NAME}
  ports:
  - name: api
    port: 9000
    targetPort: api
EOF
kubectl -n "$NS" rollout status "deploy/${MINIO_NAME}" --timeout=180s

mc_pod="${MINIO_NAME}-mc-$(printf '%s' "$RUN_ID" | tr '[:upper:]' '[:lower:]')"
kubectl -n "$NS" run "$mc_pod" \
  --restart=Never \
  --rm \
  --attach=true \
  --image="$MC_IMAGE" \
  --image-pull-policy=IfNotPresent \
  --env="MINIO_ACCESS_KEY=${MINIO_ACCESS_KEY}" \
  --env="MINIO_SECRET_KEY=${MINIO_SECRET_KEY}" \
  --command -- sh -ec \
  "mc alias set validation http://${MINIO_NAME}:9000 \"\$MINIO_ACCESS_KEY\" \"\$MINIO_SECRET_KEY\" && mc mb --ignore-existing validation/${MINIO_BUCKET} && mc ls validation/${MINIO_BUCKET}"

kubectl -n "$NS" apply -f - <<EOF
apiVersion: v1
kind: Secret
metadata:
  name: ${BACKUP_SECRET}
  labels:
    app.kubernetes.io/part-of: upmio-validation
type: Opaque
stringData:
  S3_ENDPOINT: http://${MINIO_NAME}.${NS}.svc.cluster.local:9000
  S3_BUCKET: ${MINIO_BUCKET}
  S3_ACCESS_KEY: ${MINIO_ACCESS_KEY}
  S3_SECRET_KEY: ${MINIO_SECRET_KEY}
  S3_USE_SSL: "false"
EOF
echo "PASS object_storage_ready"

echo
echo "== Prepare ClickHouse validation data =="
run_sql <<SQL
DROP DATABASE IF EXISTS ${RESTORE_DB} ON CLUSTER upm_cluster SYNC;
CREATE DATABASE IF NOT EXISTS ${SOURCE_DB} ON CLUSTER upm_cluster;
DROP TABLE IF EXISTS ${SOURCE_DB}.${SOURCE_DIST_TABLE} ON CLUSTER upm_cluster SYNC;
DROP TABLE IF EXISTS ${SOURCE_DB}.${SOURCE_TABLE} ON CLUSTER upm_cluster SYNC;
CREATE TABLE ${SOURCE_DB}.${SOURCE_TABLE} ON CLUSTER upm_cluster
(
  id UInt64,
  shard_key UInt64,
  message String
)
ENGINE = ReplicatedMergeTree('/clickhouse/tables/{cluster}/{shard}/${SOURCE_DB}/${SOURCE_TABLE}', '{replica}')
ORDER BY id;
CREATE TABLE ${SOURCE_DB}.${SOURCE_DIST_TABLE} ON CLUSTER upm_cluster
AS ${SOURCE_DB}.${SOURCE_TABLE}
ENGINE = Distributed(upm_cluster, ${SOURCE_DB}, ${SOURCE_TABLE}, shard_key);
INSERT INTO ${SOURCE_DB}.${SOURCE_DIST_TABLE}
SELECT number + 1 AS id, number % 8 AS shard_key, concat('phase07-', toString(number + 1)) AS message
FROM numbers(16);
SQL
query_tsv "$(checksum_sql "$SOURCE_DB" "$SOURCE_TABLE")" >"$SOURCE_CHECKSUM"
cat "$SOURCE_CHECKSUM"
echo "PASS validation_source_ready"

echo
echo "== Backup through API-created Kubernetes Job =="
backup_body="$(
  jq -n \
    --arg db "$SOURCE_DB" \
    --arg table "$SOURCE_TABLE" \
    --arg secret "$BACKUP_SECRET" \
    --arg path "$MANUAL_PATH" \
    '{scope:{database:$db,table:$table},storage:{type:"s3",secretRef:$secret,path:$path},execution:{type:"kubernetesJob"},dryRun:false}'
)"
api_json POST "/api/v1/clusters/${NS}/${NAME}/backup" "$backup_body" | jq . >"$BACKUP_RESPONSE"
backup_task="$(jq -r '.name' "$BACKUP_RESPONSE")"
kubectl -n "$NS" get job "$backup_task" >/dev/null
wait_task "$backup_task" "$BACKUP_TASK"
jq -e '.status == "Succeeded" and .type == "backup" and .evidence.logsRedacted == true' "$BACKUP_TASK" >/dev/null
assert_no_secret_leakage "$BACKUP_RESPONSE"
assert_no_secret_leakage "$BACKUP_TASK"
echo "PASS manual_backup task=${backup_task}"

echo
echo "== Restore manual backup through API-created Kubernetes Job =="
restore_body="$(
  jq -n \
    --arg backupRef "$MANUAL_PATH" \
    --arg sourceDb "$SOURCE_DB" \
    --arg sourceTable "$SOURCE_TABLE" \
    --arg targetDb "$RESTORE_DB" \
    --arg targetTable "$RESTORE_TABLE" \
    --arg secret "$BACKUP_SECRET" \
    '{backupRef:$backupRef,source:{database:$sourceDb,table:$sourceTable},target:{database:$targetDb,table:$targetTable},storage:{type:"s3",secretRef:$secret,path:$backupRef},execution:{type:"kubernetesJob"},confirm:true,reason:"phase 07 manual restore validation",dryRun:false}'
)"
api_json POST "/api/v1/clusters/${NS}/${NAME}/restore" "$restore_body" | jq . >"$RESTORE_RESPONSE"
restore_task="$(jq -r '.name' "$RESTORE_RESPONSE")"
kubectl -n "$NS" get job "$restore_task" >/dev/null
wait_task "$restore_task" "$RESTORE_TASK"
query_tsv "$(checksum_sql "$RESTORE_DB" "$RESTORE_TABLE")" >"$RESTORE_CHECKSUM"
diff -u "$SOURCE_CHECKSUM" "$RESTORE_CHECKSUM"
assert_no_secret_leakage "$RESTORE_RESPONSE"
assert_no_secret_leakage "$RESTORE_TASK"
echo "PASS manual_restore task=${restore_task}"

echo
echo "== Scheduled backup through API-created Kubernetes CronJob =="
api_json DELETE "/api/v1/clusters/${NS}/${NAME}/backup-schedules/${SCHEDULE_NAME}" >/dev/null 2>&1 || true
kubectl -n "$NS" delete job -l "upm.api/service-group.name=${NAME},upm.api/backup.schedule-name=${SCHEDULE_NAME}" --ignore-not-found >/dev/null

schedule_body="$(
  jq -n \
    --arg name "$SCHEDULE_NAME" \
    --arg schedule "$SCHEDULE_CRON" \
    --arg db "$SOURCE_DB" \
    --arg table "$SOURCE_TABLE" \
    --arg secret "$BACKUP_SECRET" \
    --arg prefix "$SCHEDULE_PREFIX" \
    '{name:$name,schedule:$schedule,timeZone:"Asia/Shanghai",scope:{database:$db,table:$table},storage:{type:"s3",secretRef:$secret,pathPrefix:$prefix},execution:{type:"kubernetesCronJob",concurrencyPolicy:"Forbid",successfulJobsHistoryLimit:1,failedJobsHistoryLimit:1},suspend:false}'
)"
api_json POST "/api/v1/clusters/${NS}/${NAME}/backup-schedules" "$schedule_body" | jq . >"$SCHEDULE_RESPONSE"
kubectl -n "$NS" get cronjob "${NAME}-backup-${SCHEDULE_NAME}" >/dev/null
wait_scheduled_backup
jq -e '.recentJobs[] | select(.status == "Succeeded")' "$SCHEDULE_DETAIL" >/dev/null
assert_no_secret_leakage "$SCHEDULE_RESPONSE"
assert_no_secret_leakage "$SCHEDULE_DETAIL"
scheduled_path="$(
  jq -r '[.recentJobs[] | select(.status == "Succeeded")][0].evidence.logTail // ""' "$SCHEDULE_DETAIL" \
    | sed -nE 's/.*backup completed path=([^[:space:]]+).*/\1/p' \
    | tail -n 1
)"
if [[ -z "$scheduled_path" ]]; then
  jq . "$SCHEDULE_DETAIL"
  echo "ERROR: could not extract scheduled backup path from redacted task logs" >&2
  exit 1
fi
echo "PASS scheduled_backup path=${scheduled_path}"

echo
echo "== Restore scheduled backup object through API =="
run_sql <<SQL
DROP TABLE IF EXISTS ${RESTORE_DB}.${SCHEDULED_RESTORE_TABLE} ON CLUSTER upm_cluster SYNC;
SQL
scheduled_restore_body="$(
  jq -n \
    --arg backupRef "$scheduled_path" \
    --arg sourceDb "$SOURCE_DB" \
    --arg sourceTable "$SOURCE_TABLE" \
    --arg targetDb "$RESTORE_DB" \
    --arg targetTable "$SCHEDULED_RESTORE_TABLE" \
    --arg secret "$BACKUP_SECRET" \
    '{backupRef:$backupRef,source:{database:$sourceDb,table:$sourceTable},target:{database:$targetDb,table:$targetTable},storage:{type:"s3",secretRef:$secret,path:$backupRef},execution:{type:"kubernetesJob"},confirm:true,reason:"phase 07 scheduled backup restore validation",dryRun:false}'
)"
api_json POST "/api/v1/clusters/${NS}/${NAME}/restore" "$scheduled_restore_body" | jq . >"$SCHEDULED_RESTORE_RESPONSE"
scheduled_restore_task="$(jq -r '.name' "$SCHEDULED_RESTORE_RESPONSE")"
wait_task "$scheduled_restore_task" "$SCHEDULED_RESTORE_TASK"
query_tsv "$(checksum_sql "$RESTORE_DB" "$SCHEDULED_RESTORE_TABLE")" >"$SCHEDULED_RESTORE_CHECKSUM"
diff -u "$SOURCE_CHECKSUM" "$SCHEDULED_RESTORE_CHECKSUM"
assert_no_secret_leakage "$SCHEDULED_RESTORE_RESPONSE"
assert_no_secret_leakage "$SCHEDULED_RESTORE_TASK"
echo "PASS scheduled_restore task=${scheduled_restore_task}"

echo
echo "== Backup schedule list/delete API =="
api_json GET "/api/v1/clusters/${NS}/${NAME}/backup-schedules" | jq . >"${OUT_DIR}/backup-schedule-list.json"
jq -e --arg schedule "$SCHEDULE_NAME" '.items[] | select(.name == $schedule)' "${OUT_DIR}/backup-schedule-list.json" >/dev/null
if [[ "$DELETE_SCHEDULE_AFTER_VALIDATION" == "true" ]]; then
  api_json DELETE "/api/v1/clusters/${NS}/${NAME}/backup-schedules/${SCHEDULE_NAME}" >/dev/null
  if api_json GET "/api/v1/clusters/${NS}/${NAME}/backup-schedules/${SCHEDULE_NAME}" >/dev/null 2>&1; then
    echo "ERROR: backup schedule still exists after delete" >&2
    exit 1
  fi
  echo "PASS backup_schedule_deleted"
else
  echo "PASS backup_schedule_retained"
fi

echo
echo "PASS phase07_backup_restore_runtime_validation"
