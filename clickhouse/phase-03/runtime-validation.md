# Phase 03 Runtime Validation

- Date: 2026-06-06
- Status: PASS
- Branch: `phase-03`

## 1. Result

Phase 03 implementation and Codex runtime acceptance passed in the real lab
Kubernetes cluster.

This is implementation evidence, not the final phase closeout. The phase still
requires Claude review, any required repair work, and owner approval before it
is merged into `main`.

## 2. Kubernetes Deployment Evidence

Server-side validation passed for all deployment resources:

```text
serviceaccount/upm-api-server created (server dry run)
clusterrole.rbac.authorization.k8s.io/upm-api-server created (server dry run)
clusterrolebinding.rbac.authorization.k8s.io/upm-api-server created (server dry run)
configmap/upm-api-server-config created (server dry run)
deployment.apps/upm-api-server created (server dry run)
service/upm-api-server created (server dry run)
```

The automated image synchronization script built the Linux/amd64 static binary
and imported `localhost/upmio/upm-api-server:phase-03` into containerd on:

```text
192.168.35.201
192.168.35.202
192.168.35.203
192.168.35.204
PASS upm_api_server_image_sync
```

Real deployment state:

```text
Deployment/upm-api-server: 1/1 Ready
Pod/upm-api-server:         Running
Service/upm-api-server:     NodePort, port 8080, nodePort 30083
```

Lab API URL:

```text
http://192.168.35.201:30083
```

The health API returned `200 OK` through NodePort `30083` on all four nodes:

```text
192.168.35.201
192.168.35.202
192.168.35.203
192.168.35.204
```

RBAC verification for
`system:serviceaccount:upm-system:upm-api-server`:

```text
get secrets:     yes
create UnitSets: yes
create Pods:     no
```

The API server can create UPMIO resources but cannot bypass the operator path
by creating raw Pods.

## 3. API Runtime Evidence

The default real-environment validation passed:

```text
clickhouse/phase-03/scripts/validate-upm-api-server-runtime.sh
PASS upm_api_server_runtime_validation
```

It verified:

- `/api/v1/healthz` through the Kubernetes Service.
- Expected invalid-request HTTP `400` and `VALIDATION_ERROR` with `requestId`.
- Discovery of the existing real Phase 03 cluster.
- Real cluster detail and resource aggregation.
- Secret values are absent from API responses.

After runtime cleanup, the Kubernetes cluster retains only:

```text
upm-clickhouse-phase03-runtime/clickhouse-phase03
upm-clickhouse-phase03-runtime/clickhouse-phase03-keeper
```

## 4. Create Cluster and Database E2E Evidence

The following command recreated the reserved validation namespace and created a
new cluster through `POST /api/v1/clusters`:

```bash
PHASE03_CREATE_E2E=1 \
PHASE03_CREATE_E2E_RESET=1 \
  clickhouse/phase-03/scripts/validate-upm-api-server-runtime.sh
```

Created target:

```text
namespace: upm-clickhouse-phase03-runtime
cluster:   clickhouse-phase03
topology:  2 shards x 2 replicas + 3 Keeper
```

Kubernetes/UPMIO result:

```text
Keeper UnitSet:     expected=3 current=3 ready=3
ClickHouse UnitSet: expected=4 current=4 ready=4
Keeper Pods:        3, all 2/2 Running
ClickHouse Pods:    4, all 2/2 Running
PVCs:               7, all Bound
```

Database-level verification:

```text
system.clusters: 2 shards x 2 replicas
macros:          shard01/replica01, shard01/replica02,
                 shard02/replica01, shard02/replica02
Distributed validation table: 8 rows
shard 1: 4 rows [2,4,6,8]
shard 2: 4 rows [1,3,5,7]
each replica: 4 rows
total_replicas=2
active_replicas=2
is_readonly=0
queue_size=0
PASS runtime_2s2r_validation
PASS upm_api_server_runtime_validation
```

This proves that the API-created topology is usable at the ClickHouse layer,
including Distributed-table writes, reads, sharding, replication, and replica
health.

## 5. Runtime-Discovered Fixes

Real execution found and fixed two issues that static analysis did not expose:

1. Installed package topology values are stored as quoted strings in the
   ConfigMap. The API server now accepts both string and numeric values when
   validating package topology.
2. The ClickHouse package requires the AES-CTR credential data as binary Secret
   files. The E2E script now creates the Secret with `kubectl --from-file`
   instead of storing base64 text as file content.

## 6. Static Validation Results

Passed:

```text
go test ./...
go test -race ./...
go vet ./...
go build ./...
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build ./cmd/upm-api-server
bash -n clickhouse/sync-upm-api-server-image-to-nodes.sh
bash -n clickhouse/phase-03/scripts/validate-upm-api-server-runtime.sh
shellcheck clickhouse/sync-upm-api-server-image-to-nodes.sh
shellcheck clickhouse/phase-03/scripts/validate-upm-api-server-runtime.sh
yq eval-all 'true' clickhouse/phase-03/manifests/upm-api-server.yaml
markdownlint "docs/**/*.md" "clickhouse/**/*.md" "api-server/**/*.md"
gitleaks detect --source . --redact --no-git
git diff --check
```

Linux build result:

```text
ELF 64-bit LSB executable, x86-64, statically linked
```

## 7. Known Risk

The lab uses an automated node-local image synchronization script with the
same image tag on all four nodes. This is acceptable for the hackathon lab, but
production deployment should publish immutable images to a registry.
