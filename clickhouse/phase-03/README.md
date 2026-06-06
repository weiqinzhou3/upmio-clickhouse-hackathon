# Phase 03 UPM API Server Evidence

This directory contains hackathon-facing validation assets for Repository
Phase 03.

Phase 03 implements the Go `upm-api-server` product control plane. The API
server must use UPMIO and Kubernetes APIs as the product path, and it must
validate against the real lab cluster rather than relying only on mocks or
dry-run output.

`upm-api-server` is not a ClickHouse-only control service. It is the
higher-level UPM API surface that starts with ClickHouse management and can
later expose MySQL, Redis, and other database APIs from the same system service.

The complete Phase 03 API reference is maintained in
`docs/api/upm-api-server-v1.md`.

The latest real-environment acceptance evidence is maintained in
`clickhouse/phase-03/runtime-validation.md`.

## Manual Acceptance

Set the remotely reachable API URL:

```bash
export UPM_API_SERVER_URL=http://192.168.35.201:30083
```

Confirm that only the current Phase 03 ClickHouse cluster remains:

```bash
kubectl get unitsets -A
```

Expected UnitSets:

```text
upm-clickhouse-phase03-runtime   clickhouse-phase03
upm-clickhouse-phase03-runtime   clickhouse-phase03-keeper
```

Confirm the Kubernetes API service:

```bash
kubectl -n upm-system get deploy/upm-api-server svc/upm-api-server
```

Expected state:

```text
deployment.apps/upm-api-server   1/1
service/upm-api-server           NodePort   8080:30083/TCP
```

Call the API directly from a machine that can reach the Kubernetes nodes:

```bash
curl -fsS "${UPM_API_SERVER_URL}/api/v1/healthz" | jq .
curl -fsS "${UPM_API_SERVER_URL}/api/v1/clusters" | jq .
curl -fsS \
  "${UPM_API_SERVER_URL}/api/v1/clusters/upm-clickhouse-phase03-runtime/clickhouse-phase03/resources" \
  | jq .
```

Expected cluster summary:

```text
status: Running
topology: 2 shards x 2 replicas + 3 Keeper
ready.keeper: 3/3
ready.server: 4/4
```

Run the real ClickHouse topology, replication, and Distributed-table
read/write acceptance:

```bash
NS=upm-clickhouse-phase03-runtime \
CLICKHOUSE_UNITSET=clickhouse-phase03 \
KEEPER_UNITSET=clickhouse-phase03-keeper \
POD=clickhouse-phase03-0 \
DB=phase03_manual_acceptance \
  clickhouse/phase-02/scripts/validate-runtime-2s2r.sh
```

Expected final line:

```text
PASS runtime_2s2r_validation
```

## Runtime Validation Script

Use this script after `upm-api-server` is implemented:

```bash
clickhouse/phase-03/scripts/validate-upm-api-server-runtime.sh
```

The script validates a real Kubernetes/UPMIO environment:

- UPMIO CRDs exist.
- `unit-operator` is running.
- Phase 02 ClickHouse runtime exists and is ready.
- `upm-api-server` is deployed as `Deployment/Service/ServiceAccount` in
  Kubernetes.
- `upm-api-server` `/api/v1/healthz` responds.
- Invalid cluster creation returns the expected structured error with
  `requestId`.
- `GET /api/v1/clusters` can discover the real Phase 03 runtime cluster.
- `GET /api/v1/clusters/{namespace}/{name}/resources` returns real UnitSet,
  Unit, Pod, PVC, Service, and Endpoint status without leaking Secret values.

Expected final line:

```text
PASS upm_api_server_runtime_validation
```

The expected invalid-request test also prints:

```text
PASS expected_structured_error
```

The structured `VALIDATION_ERROR` JSON printed immediately before that line is
the expected result, not a script failure.

The script does not start a local binary. If `API_SERVER_URL` is not already
reachable, set `START_PORT_FORWARD=auto` to allow a temporary
`kubectl port-forward` to the Kubernetes Service:

```bash
API_SERVER_NS=upm-system \
API_SERVER_SERVICE=upm-api-server \
API_SERVER_LOCAL_PORT=18083 \
API_SERVER_SERVICE_PORT=8080 \
  clickhouse/phase-03/scripts/validate-upm-api-server-runtime.sh
```

Supported script environment variables:

| Variable | Default | Purpose |
|---|---|---|
| `API_SERVER_NS` | `upm-system` | Namespace where `upm-api-server` runs |
| `API_SERVER_DEPLOYMENT` | `upm-api-server` | Deployment name |
| `API_SERVER_SERVICE` | `upm-api-server` | Service and ServiceAccount name |
| `API_SERVER_SERVICE_PORT` | `8080` | Service port used by port-forward |
| `API_SERVER_LOCAL_PORT` | `18083` | Local port used only for port-forward access |
| `API_SERVER_URL` | `http://192.168.35.201:30083` | API URL used by curl |
| `START_PORT_FORWARD` | `0` | Set to `auto` to allow port-forward fallback |
| `EXISTING_NS` | `upm-clickhouse-phase03-runtime` | Existing cluster namespace checked by read APIs |
| `EXISTING_CLUSTER` | `clickhouse-phase03` | Existing ClickHouse UnitSet checked by read APIs |
| `EXISTING_KEEPER` | `clickhouse-phase03-keeper` | Existing Keeper UnitSet checked by read APIs |

## Optional Create E2E Mode

After `POST /api/v1/clusters` is implemented, run the same script with:

```bash
PHASE03_CREATE_E2E=1 \
  clickhouse/phase-03/scripts/validate-upm-api-server-runtime.sh
```

This mode creates a reserved test namespace/cluster through `upm-api-server`
and waits for real Keeper and ClickHouse UnitSets to become ready. It prepares
only the credential Secret needed for the ClickHouse package; `upm-api-server`
must still create/apply the UPMIO Project and UnitSet resources.

For the default 2x2 topology, the create E2E also runs the Phase 02 database
runtime validator against the newly API-created cluster. This validates
`system.clusters`, macros, distributed DDL, Distributed-table write/read,
replica synchronization, and replica health.

The reserved default target is:

```text
namespace: upm-clickhouse-phase03-runtime
cluster:   clickhouse-phase03
topology:  2 shards x 2 replicas + 3 Keeper
```

The requested topology must match the ClickHouse package topology installed in
`upm-system`. Override `CREATE_SHARDS`, `CREATE_REPLICAS_PER_SHARD`, and
`CREATE_KEEPER_REPLICAS` only after installing a matching package topology.

To repeat the create E2E from a clean reserved validation namespace:

```bash
PHASE03_CREATE_E2E=1 \
PHASE03_CREATE_E2E_RESET=1 \
  clickhouse/phase-03/scripts/validate-upm-api-server-runtime.sh
```

`PHASE03_CREATE_E2E_RESET=1` deletes only the reserved `CREATE_NS` namespace
before recreation. It is disabled by default and must not target a business
namespace.
