# Full Demo Runbook

- Version: 0.1
- Date: 2026-06-07
- Status: Draft
- Scope: From a prepared Kubernetes lab to complete hackathon demonstration

## 1. Goal

Run the complete UPMIO ClickHouse operations MVP demo from environment
preparation through API validation, monitoring, diagnostics, backup, restore,
scheduled backup, and evidence presentation.

## 2. Assumptions

Validated lab nodes:

| Role | IP |
|---|---|
| Control plane | `192.168.35.201` |
| Worker | `192.168.35.202` |
| Worker | `192.168.35.203` |
| Worker | `192.168.35.204` |

The runbook assumes:

- Kubernetes is installed or can be installed following the existing
  installation record.
- `kubectl` points to the lab cluster.
- The repository has submodules initialized.
- Node SSH user is `root`.
- Node SSH password is available as `SSH_PASSWORD`.

## 3. Repository Preparation

```bash
git clone --recurse-submodules <repo-url>
cd upm

# If submodules are missing:
git submodule update --init --recursive
```

Confirm that `exam/` is not part of the public Git history:

```bash
git ls-files exam
```

Expected: no output.

## 4. Workstation Tool Check

```bash
kubectl version --client
helm version
curl --version
jq --version
gitleaks version
markdownlint --version
```

## 5. Kubernetes Baseline Check

```bash
kubectl get nodes -o wide
kubectl get sc
kubectl cluster-info
```

Expected:

```text
4 nodes Ready
local-path StorageClass available and default
Kubernetes API reachable
```

If Kubernetes is not installed, follow:

```text
docs/discovery/01-kubernetes-installation-record.md
```

## 6. Install UPMIO Operators

```bash
helm upgrade --install unit-operator ./unit-operator/charts/unit-operator \
  -n upm-system --create-namespace \
  -f docs/runtime/manifests/unit-operator-install-values.yaml

helm upgrade --install compose-operator ./compose-operator/charts/compose-operator \
  -n upm-system --create-namespace \
  -f docs/runtime/manifests/compose-operator-install-values.yaml
```

Validate:

```bash
kubectl -n upm-system get pods
kubectl get crd | grep -Ei 'projects|unitsets|units|grpccalls'
```

Expected:

```text
unit-operator Running
compose-operator Running
UPMIO CRDs present
```

## 7. Install ClickHouse Packages

Install package charts:

```bash
helm upgrade --install clickhouse-package \
  ./upm-packages/clickhouse/26.3.9.8/charts/clickhouse \
  -n upm-system --create-namespace \
  -f clickhouse/phase-02/values/runtime-2s2r-package-values.yaml

helm upgrade --install clickhouse-keeper-package \
  ./upm-packages/clickhouse-keeper/26.3.9.8/charts/clickhouse-keeper \
  -n upm-system --create-namespace
```

Validate package assets:

```bash
helm list -n upm-system
kubectl -n upm-system get configmap,podtemplate | grep -Ei 'clickhouse|keeper'
```

## 8. Synchronize Images To Nodes

```bash
export SSH_PASSWORD=root

clickhouse/scripts/sync-runtime-image-to-nodes.sh
clickhouse/scripts/sync-upm-api-server-image-to-nodes.sh
```

Expected:

```text
PASS
PASS upm_api_server_image_sync
```

## 9. Deploy upm-api-server

```bash
kubectl apply -f clickhouse/phase-03/manifests/upm-api-server.yaml
kubectl -n upm-system rollout status deploy/upm-api-server --timeout=180s
```

Set API variables:

```bash
export UPM_API_SERVER_URL=http://192.168.35.201:30083
export NS=upm-clickhouse-phase03-runtime
export NAME=clickhouse-phase03
```

Validate:

```bash
curl -fsS "${UPM_API_SERVER_URL}/api/v1/healthz" | jq .
```

Expected:

```json
{
  "service": "upm-api-server",
  "status": "ok"
}
```

## 10. Install kube-prometheus-stack And Grafana Dashboard

Install the monitoring stack if it is not already installed:

```bash
helm upgrade --install kube-prometheus-stack \
  prometheus-community/kube-prometheus-stack \
  -n monitoring --create-namespace
```

Expected runtime names:

```text
Namespace: monitoring
Prometheus service: kube-prometheus-stack-prometheus
Grafana service: kube-prometheus-stack-grafana
Grafana dashboard UID: upm-clickhouse-overview
```

Dashboard asset:

```text
clickhouse/grafana/upm-clickhouse-23285-dashboard.json
```

If the dashboard is not provisioned yet, import this JSON into Grafana or use
the environment provisioning flow prepared for the lab.

## 11. Prepare ClickHouse Runtime Secrets

Create or confirm the target namespace:

```bash
kubectl get ns "${NS}" || kubectl create ns "${NS}"
```

Create required Secrets if they do not exist:

```bash
export CLICKHOUSE_ADMIN_PASSWORD='<admin-password>'
export AES_SECRET_KEY='<aes-secret-key>'

kubectl -n "${NS}" create secret generic clickhouse-phase03-secret \
  --from-literal=CLICKHOUSE_ADMIN_PASSWORD="${CLICKHOUSE_ADMIN_PASSWORD}" \
  --dry-run=client -o yaml | kubectl apply -f -

kubectl -n "${NS}" create secret generic aes-secret-key \
  --from-literal=AES_SECRET_KEY="${AES_SECRET_KEY}" \
  --dry-run=client -o yaml | kubectl apply -f -
```

Do not commit these values.

## 12. Create Or Validate ClickHouse Cluster

For the validated demo path, run:

```bash
clickhouse/scripts/validate-phase03-upm-api-server-runtime.sh
```

Expected final line:

```text
PASS phase03_upm_api_server_runtime_validation
```

Confirm UPMIO resources:

```bash
kubectl -n "${NS}" get unitsets,units,pods,pvc,svc,podmonitor -o wide
curl -fsS "${UPM_API_SERVER_URL}/api/v1/clusters/${NS}/${NAME}/resources" | jq .
```

Expected:

```text
3 Keeper Pods Ready
4 ClickHouse Server Pods Ready
PVCs Bound
PodMonitor present
```

## 13. Run Healthcheck Demo

```bash
clickhouse/scripts/validate-phase04-healthcheck-runtime.sh
```

Expected final line:

```text
PASS phase04_healthcheck_runtime_validation
```

Demo message:

```text
The API validates Kubernetes readiness, Keeper, ClickHouse SQL, topology,
write/read behavior, replica health, metrics endpoint, and secret safety.
```

## 14. Run Monitoring And Grafana Demo

```bash
clickhouse/scripts/validate-phase05-monitoring-runtime.sh
```

Expected final line:

```text
PASS phase05_monitoring_runtime_validation
```

Evidence files:

```text
clickhouse/phase-05/prometheus-targets.json
clickhouse/phase-05/metrics-summary.json
clickhouse/phase-05/grafana-panel-query-results.json
```

Demo message:

```text
ClickHouse metrics are collected by Prometheus through PodMonitor and rendered
by the adapted Grafana dashboard.
```

## 15. Run Day2 Diagnostics Demo

```bash
clickhouse/scripts/validate-phase06-diagnostics-runtime.sh
```

Expected final line:

```text
PASS phase06_diagnostics_runtime_validation
```

Evidence files:

```text
clickhouse/phase-06/diagnostics.json
clickhouse/phase-06/system-replicas.tsv
clickhouse/phase-06/system-parts.tsv
clickhouse/phase-06/system-mutations.tsv
clickhouse/phase-06/system-merges.tsv
```

Demo message:

```text
Diagnostics are read-only and cover replica, queue, parts, merges, mutations,
Keeper, storage, and write quality.
```

## 16. Run Backup, Restore, And Scheduled Backup Demo

```bash
clickhouse/scripts/validate-phase07-backup-restore-runtime.sh
```

Expected final line:

```text
PASS phase07_backup_restore_runtime_validation
```

Evidence files:

```text
clickhouse/phase-07/backup-task.json
clickhouse/phase-07/restore-task.json
clickhouse/phase-07/backup-schedule-detail.json
clickhouse/phase-07/source-checksum.tsv
clickhouse/phase-07/restore-checksum.tsv
clickhouse/phase-07/scheduled-restore-checksum.tsv
```

Demo message:

```text
Immediate backup, restore, and scheduled backup are all API-driven. The runtime
uses Kubernetes Job/CronJob and validates restored data by row count and
checksum.
```

## 17. Show API Reference

```bash
sed -n '1,160p' docs/api/upm-api-server-v1.md
sed -n '1,220p' docs/manuals/api-manual.md
```

Key APIs to mention:

```text
GET  /api/v1/healthz
POST /api/v1/clusters
GET  /api/v1/clusters
GET  /api/v1/clusters/{namespace}/{name}/resources
POST /api/v1/clusters/{namespace}/{name}/healthcheck
GET  /api/v1/clusters/{namespace}/{name}/metrics/summary
GET  /api/v1/clusters/{namespace}/{name}/diagnostics
POST /api/v1/clusters/{namespace}/{name}/backup
POST /api/v1/clusters/{namespace}/{name}/restore
POST /api/v1/clusters/{namespace}/{name}/backup-schedules
GET  /api/v1/clusters/{namespace}/{name}/tasks/{taskName}
```

## 18. Show AI Collaboration Evidence

```bash
sed -n '1,120p' docs/ai-usage-phase02.md
find docs/review -maxdepth 1 -type f | sort
git log --oneline --decorate --all | head -20
```

Demo message:

```text
AI output was not accepted as evidence by itself. Every phase used spec,
implementation, runtime validation, review, repair, and closeout.
```

## 19. Current Limitation Statement

Use this wording in the demo:

```text
The current MVP validates the 2 shards x 2 replicas + 3 Keeper architecture.
It supports deployment, healthcheck, monitoring, diagnostics, backup, restore,
and scheduled backup. Version upgrade, resource expansion, online topology
expansion, rolling restart, backup retention, and frontend UI are planned as
post-MVP phases unless implemented and validated later.
```

## 20. Short Demo Path

When demo time is short, run only:

```bash
curl -fsS "${UPM_API_SERVER_URL}/api/v1/healthz" | jq .
clickhouse/scripts/validate-phase04-healthcheck-runtime.sh
clickhouse/scripts/validate-phase05-monitoring-runtime.sh
clickhouse/scripts/validate-phase06-diagnostics-runtime.sh
clickhouse/scripts/validate-phase07-backup-restore-runtime.sh
```

## 21. Failure Handling

| Failure | First Check |
|---|---|
| API unreachable | `kubectl -n upm-system get deploy,svc upm-api-server -o wide` |
| NodePort blocked | `kubectl -n upm-system port-forward svc/upm-api-server 18083:8080` |
| Pods not Ready | `kubectl -n "${NS}" describe pod <pod>` |
| Prometheus target missing | `kubectl -n "${NS}" get podmonitor -o yaml` |
| Grafana panel empty | Check datasource UID and Prometheus service name |
| Backup fails | Check `clickhouse-backup-secret`, MinIO/S3, and Job logs |
| Restore fails | Ensure target table/database does not already exist |
| CronJob takes time | Wait one schedule interval and query child Jobs |

## 22. Cleanup Guidance

Do not delete evidence files before judging.

If cleanup is required:

```bash
kubectl -n "${NS}" get job,cronjob
kubectl -n "${NS}" delete cronjob validation-every-minute --ignore-not-found
```

Avoid deleting the validated ClickHouse cluster immediately before demo.
