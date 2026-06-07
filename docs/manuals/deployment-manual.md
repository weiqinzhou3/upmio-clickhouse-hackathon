# Deployment Manual

## Scope

This manual describes the validated hackathon environment path:

1. Kubernetes cluster.
2. UPMIO operators and ClickHouse packages.
3. `upm-api-server`.
4. kube-prometheus-stack and Grafana dashboard.
5. ClickHouse cluster deployment and validation.

The repository does not install application services directly on host nodes.
Host operations are limited to Kubernetes/container runtime prerequisites and
image import for the isolated lab.

## Lab Topology

| Role | Hostname | IP |
|---|---|---|
| Control plane | `upm-k8s-master01` | `192.168.35.201` |
| Worker | `upm-k8s-worker01` | `192.168.35.202` |
| Worker | `upm-k8s-worker02` | `192.168.35.203` |
| Worker | `upm-k8s-worker03` | `192.168.35.204` |

Validated baseline:

```text
Kubernetes: v1.35.5
OS:         Red Hat Enterprise Linux 9.6
Runtime:    containerd 2.1.5
CNI:        Calico
Storage:    local-path
Helm:       v3.19.0
```

## Local Tooling

Required on the operator workstation:

```bash
kubectl version --client
helm version
go version
sshpass -V
jq --version
curl --version
markdownlint --version
gitleaks version
```

## Kubernetes Installation

The full installation record is stored in
[`docs/discovery/01-kubernetes-installation-record.md`](../discovery/01-kubernetes-installation-record.md).

High-level steps:

1. Prepare all RHEL nodes with swap disabled, kernel modules, sysctl, CNI
   binaries, containerd, runc, kubeadm, kubelet, and kubectl.
2. Initialize the control plane with `kubeadm init`.
3. Install Calico.
4. Install local-path storage and mark it as default.
5. Join three worker nodes.
6. Verify node readiness.

Validation:

```bash
kubectl get nodes -o wide
kubectl get sc
kubectl cluster-info
```

Expected:

```text
4 nodes Ready
local-path StorageClass default
Kubernetes control plane reachable at 192.168.35.201:6443
```

## UPMIO Operator Installation

The validated UPMIO installation record is stored in
[`docs/runtime/01-upmio-operator-installation.md`](../runtime/01-upmio-operator-installation.md).

Install operators:

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
kubectl api-resources | grep -Ei 'unit|unitset|grpccall'
```

Expected:

```text
unit-operator Running
compose-operator Running
projects/unitsets/units/grpccalls CRDs present
```

## ClickHouse Package Installation

ClickHouse package charts live under `upm-packages/`. The runtime expects the
ClickHouse package, ClickHouse Keeper package, package ConfigMaps, and
PodTemplates to exist in `upm-system`.

Representative install commands:

```bash
helm upgrade --install clickhouse-package \
  ./upm-packages/clickhouse/26.3.9.8/charts/clickhouse \
  -n upm-system --create-namespace \
  -f clickhouse/phase-02/values/runtime-2s2r-package-values.yaml

helm upgrade --install clickhouse-keeper-package \
  ./upm-packages/clickhouse-keeper/26.3.9.8/charts/clickhouse-keeper \
  -n upm-system --create-namespace
```

If chart paths differ after upstream changes, inspect `upm-packages/` and keep
the release names and namespace consistent with the runtime evidence.

Validate:

```bash
helm list -n upm-system
kubectl -n upm-system get configmap,podtemplate | grep -Ei 'clickhouse|keeper'
```

## Runtime Image Synchronization

The lab uses node-local containerd images because VM registry access can be
unreliable.

Synchronize the ClickHouse runtime image:

```bash
SSH_PASSWORD=root clickhouse/scripts/sync-runtime-image-to-nodes.sh
```

Synchronize `upm-api-server`:

```bash
SSH_PASSWORD=root clickhouse/scripts/sync-upm-api-server-image-to-nodes.sh
```

The scripts import images into all four nodes:

```text
192.168.35.201 192.168.35.202 192.168.35.203 192.168.35.204
```

## upm-api-server Deployment

Apply:

```bash
kubectl apply -f clickhouse/phase-03/manifests/upm-api-server.yaml
kubectl -n upm-system rollout status deploy/upm-api-server --timeout=180s
```

Validate:

```bash
curl -fsS http://192.168.35.201:30083/api/v1/healthz | jq .
kubectl -n upm-system get deploy,svc upm-api-server -o wide
```

Expected:

```json
{
  "service": "upm-api-server",
  "status": "ok"
}
```

## kube-prometheus-stack And Grafana

kube-prometheus-stack is an external environment prerequisite, not an UPMIO
product feature. It provides Prometheus Operator CRDs, Prometheus, and Grafana.

The Phase 05 spec records the external environment boundary and dashboard
requirements in
[`docs/phases/phase-05-monitoring.md`](../phases/phase-05-monitoring.md).

Expected runtime naming:

```text
Namespace: monitoring
Prometheus service: kube-prometheus-stack-prometheus
Grafana service: kube-prometheus-stack-grafana
Grafana datasource UID: prometheus
Grafana dashboard UID: upm-clickhouse-overview
```

Representative install command:

```bash
helm upgrade --install kube-prometheus-stack \
  prometheus-community/kube-prometheus-stack \
  -n monitoring --create-namespace
```

Provision the adapted ClickHouse dashboard asset:

```text
clickhouse/grafana/upm-clickhouse-23285-dashboard.json
```

The current repository validates the resulting stack through:

```bash
clickhouse/scripts/validate-phase05-monitoring-runtime.sh
```

Expected final line:

```text
PASS phase05_monitoring_runtime_validation
```

## Deploy Or Reuse ClickHouse Cluster

The current validated demo cluster is:

```text
Namespace: upm-clickhouse-phase03-runtime
Cluster:   clickhouse-phase03
Topology:  2 shards x 2 replicas, 3 Keeper replicas
```

Create or validate the API-managed cluster:

```bash
clickhouse/scripts/validate-phase03-upm-api-server-runtime.sh
```

Run the end-to-end validation chain:

```bash
clickhouse/scripts/validate-phase04-healthcheck-runtime.sh
clickhouse/scripts/validate-phase05-monitoring-runtime.sh
clickhouse/scripts/validate-phase06-diagnostics-runtime.sh
clickhouse/scripts/validate-phase07-backup-restore-runtime.sh
```

## Known Environment Boundaries

- The lab NodePort API has no application-layer authentication. It is for the
  isolated hackathon environment only.
- Image import is lab-specific. A production deployment should use an internal
  registry instead of node-local image sync.
- Backup validation uses a MinIO object store in the ClickHouse namespace.
- Scheduled backup retention cleanup is intentionally out of MVP scope.
- Frontend UI is not implemented in the current MVP; see
  [Hackathon Stage 2 Assessment](hackathon-stage2-assessment.md).
