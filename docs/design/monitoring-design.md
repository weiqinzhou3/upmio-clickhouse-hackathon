# Monitoring Design

- Version: 0.4
- Date: 2026-06-06
- Status: Sealed
- Owner: zqw
- Related:
  - ../master-spec.md
  - upm-packages-clickhouse-design.md

## 1. Purpose

This document defines ClickHouse monitoring integration for MVP and future productization.

## 2. Monitoring Stack Boundary

Prometheus and Grafana are external dependencies.

Recommended stack:

- `kube-prometheus-stack`
- Prometheus Operator CRDs
- Prometheus Server
- Grafana

UPMIO provides integration points such as PodMonitor. It does not provide a full monitoring platform in public repos.

For the hackathon environment, `kube-prometheus-stack` installation assets are
kept outside this repository under `../kube-prometheus-stack/`. They are
environment readiness assets, not UPMIO product deliverables. Those assets
must install Prometheus and Grafana, provision a Prometheus datasource, and
import the MVP ClickHouse dashboard automatically.

## 3. MVP Metric Exposure

MVP uses ClickHouse native Prometheus endpoint.

MVP does not add a clickhouse-exporter sidecar.

Required:

- enable ClickHouse `<prometheus>` configuration;
- expose metrics port;
- configure `UnitSet.spec.podMonitor`;
- verify PodMonitor creation;
- verify endpoint is listening;
- verify Prometheus target discovery when stack is available.

## 4. Metric Categories

| Category | Source |
|---|---|
| Pod CPU/memory | kubelet/cAdvisor via Prometheus |
| PVC/storage | kube-state-metrics / node metrics |
| ClickHouse query/write | ClickHouse native metrics / system tables |
| ClickHouse data filesystem | kubelet PVC volume metrics when available; ClickHouse native disk used/total/available metrics for local-path fallback |
| Replica state | `system.replicas` and Prometheus summary |
| Parts/merges/mutations | system tables and future Prometheus rules |
| Keeper state | Keeper probes and future metrics |

## 5. UPM API Server Responsibilities

`upm-api-server` should:

- check PodMonitor existence;
- check metrics endpoint readiness;
- query Prometheus API for summary metrics;
- present a basic monitoring summary;
- report missing Prometheus stack as a prerequisite issue, not as a ClickHouse failure.

The API server exposes only a fixed internal PromQL query set. It must not
expose arbitrary user-supplied PromQL through the MVP metrics summary API.

Prometheus status behavior:

- unavailable Prometheus: structured HTTP `503`;
- available Prometheus with missing targets or optional metric gaps:
  `DEGRADED` summary with warnings;
- all expected ClickHouse targets and required metrics present: `READY`.

## 6. Fixed MVP PromQL Set

The metrics summary API does not accept user-provided PromQL. It renders the
following fixed queries with validated namespace and cluster-name values:

| Category | Fixed query intent |
|---|---|
| Targets | `up` for expected ClickHouse Server Pods |
| CPU | per-Pod `rate(container_cpu_usage_seconds_total[2m])` for the `clickhouse` container |
| Memory | per-Pod `container_memory_working_set_bytes` for the `clickhouse` container |
| Storage | `kubelet_volume_stats_used_bytes` for Server data PVCs, or ClickHouse native disk used/total/available metrics when kubelet volume statistics are unavailable |
| ClickHouse | native Query, InsertQuery, InsertedRows, InsertedBytes, and MemoryTracking metrics |

Only stable summary labels are returned:

- namespace;
- Pod;
- node, when relevant;
- persistent volume claim, when relevant.

Container IDs, image names, scrape instances, jobs, and other unstable
infrastructure labels are filtered from the API response.

## 7. Future Enhancements

- PrometheusRule alerting;
- richer ClickHouse PromQL library;
- exporter sidecar if native endpoint is insufficient;
- backup metrics;
- capacity trend analysis.

## 8. Acceptance Criteria

- PodMonitor is created for ClickHouse Server pods.
- ClickHouse metrics endpoint returns Prometheus-format output.
- Prometheus can discover and scrape the target when stack is installed.
- `upm-api-server` can query and summarize key metrics.
- All expected ClickHouse Server Pods are compared with Prometheus target
  results so a silently missing target cannot produce a false `READY`.
- Grafana is installed as part of the external monitoring environment.
- Grafana datasource and MVP ClickHouse dashboard are provisioned
  automatically.
- Every required dashboard panel query returns non-empty data through the
  Grafana datasource proxy.
