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
- Invalid cluster creation returns a structured error with `requestId`.
- `GET /api/v1/clusters` can discover the real Phase 02 runtime cluster.
- `GET /api/v1/clusters/{namespace}/{name}/resources` returns real UnitSet,
  Unit, Pod, PVC, Service, and Endpoint status without leaking Secret values.

Expected final line:

```text
PASS upm_api_server_runtime_validation
```

The script does not start a local binary. If `API_SERVER_URL` is not already
reachable, it opens a temporary `kubectl port-forward` to the Kubernetes
Service:

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
| `API_SERVER_URL` | `http://127.0.0.1:${API_SERVER_LOCAL_PORT}` | API URL used by curl |
| `START_PORT_FORWARD` | `auto` | Set to `0` to require an already reachable API URL |

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

The reserved default target is:

```text
namespace: upm-clickhouse-phase03-runtime
cluster:   clickhouse-phase03
```
