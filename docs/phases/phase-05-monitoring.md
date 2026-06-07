# Phase 05: Monitoring Integration

- Version: 0.8
- Date: 2026-06-07
- Status: Confirmed
- Priority: P0
- Owner: zqw
- Depends on:
  - ../master-spec.md
  - ../evidence-summary.md
  - ../design/monitoring-design.md
  - ../design/upm-packages-clickhouse-design.md
  - ../design/api-design.md
  - ../runtime/05-clickhouse-monitoring-runtime-validation.md
  - phase-01-upm-packages-clickhouse.md
  - phase-03-upm-api-server.md

## 1. Purpose

Enable ClickHouse monitoring integration for MVP:

- Native ClickHouse Prometheus endpoint.
- UPMIO `PodMonitor` integration.
- External kube-prometheus-stack discovery.
- External Grafana datasource and ClickHouse dashboard.
- `upm-api-server` metrics summary API.

## 2. Scope

In scope:

1. Ensure ClickHouse package enables native Prometheus endpoint.
2. Ensure metrics service port is named and exposed consistently.
3. Ensure `UnitSet.spec.podMonitor` can generate PodMonitor.
4. Validate Prometheus Operator CRDs or kube-prometheus-stack presence.
5. Implement `upm-api-server` Prometheus API client for summary queries.
6. Define MVP PromQL query set.
7. Enable external Grafana through kube-prometheus-stack environment assets.
8. Provision a Prometheus datasource and ClickHouse dashboard automatically.
9. Validate every MVP dashboard panel returns real data through Grafana.
10. Prove the full monitoring chain against a real Prometheus Server and
    Grafana dashboard.

## 3. Non-Goals

Out of scope:

- Installing Prometheus/Grafana as part of UPMIO package.
- Managing Grafana as an `upm-api-server` product feature.
- PrometheusRule/alert rule implementation in MVP.
- ClickHouse exporter sidecar unless native endpoint fails and user approves.
- Long-term metric retention design.
- Installing or managing `kube-prometheus-stack` as an UPMIO product feature.

## 4. Monitoring Boundary

| Component | Responsibility |
|---|---|
| kube-prometheus-stack | Provides Prometheus Operator, Prometheus Server, Grafana |
| UPMIO UnitSet | Creates PodMonitor when CRD exists and `podMonitor.enable=true` |
| ClickHouse package | Enables native `/metrics` endpoint and exposes metrics port |
| `upm-api-server` | Queries Prometheus API and returns summarized view |
| Grafana | External visualization platform with provisioned MVP ClickHouse dashboard |

### 4.1 Required Monitoring Chain

This phase must prove the full MVP monitoring chain when the external
Prometheus stack is available:

```text
ClickHouse native /metrics endpoint
  -> metrics port exposed on ClickHouse Pod/Service
  -> UPMIO UnitSet PodMonitor
  -> Prometheus target discovery
  -> upm-api-server /metrics/summary API
  -> Grafana datasource and ClickHouse dashboard panels
```

Kubernetes `metrics-server` alone is not sufficient for this phase. It can
support resource-level CPU and memory visibility, but it does not scrape
ClickHouse native metrics and does not prove Prometheus target discovery.

The hackathon environment must provide a real Prometheus Server before this
phase can close. ClickHouse endpoint and PodMonitor-only validation is useful
diagnostic evidence, but it is not sufficient Phase 05 acceptance evidence.

`kube-prometheus-stack` is an external environment prerequisite, not UPMIO
product code. Its local installation assets must be kept outside this
repository under:

```text
../kube-prometheus-stack/
```

The repository may document the prerequisite and runtime result, but must not
copy the external chart or environment-only installation assets into a phase
delivery directory. The external environment assets must automate:

- image synchronization to Kubernetes nodes when registry access is unreliable;
- kube-prometheus-stack installation or upgrade;
- Prometheus datasource provisioning in Grafana;
- ClickHouse dashboard ConfigMap creation from the repository dashboard asset;
- Grafana service exposure for demo access.

## 5. Required Metrics Categories

MVP should query or prepare queries for:

| Category | Source | MVP Output |
|---|---|---|
| Target up/down | Prometheus `up` | target status |
| CPU/memory/storage | kubelet/cAdvisor metrics | resource summary |
| ClickHouse query/write | ClickHouse metrics endpoint | query/write counters if available |
| Replica state | ClickHouse SQL in Phase 06 | not solely Prometheus |
| Part/Merge/Mutation | ClickHouse SQL in Phase 06 | not solely Prometheus |
| Keeper role/status | Keeper probes/metrics if available | basic role/status summary |

For storage summary, query kubelet PVC volume metrics when the StorageClass
supports them. When the lab `local-path` provisioner does not expose
`kubelet_volume_stats_*`, use ClickHouse native
`ClickHouseAsyncMetrics_DiskUsed/Total/Available_default` as the real
filesystem-capacity evidence and record that fallback.

## 6. UPM API Server Metrics API

Required endpoint:

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/api/v1/clusters/{namespace}/{name}/metrics/summary` | Return Prometheus-based summary |

Minimum response:

```json
{
  "cluster": "clickhouse-runtime",
  "namespace": "upm-clickhouse-runtime",
  "status": "READY",
  "targets": [
    {"name": "clickhouse-runtime-0", "up": true}
  ],
  "summary": {
    "cpu": {},
    "memory": {},
    "storage": {},
    "clickhouse": {}
  },
  "warnings": []
}
```

Required behavior:

- `upm-api-server` uses a configured `PROMETHEUS_BASE_URL`.
- Prometheus queries use an internal fixed query set; arbitrary user-provided
  PromQL is not exposed.
- The expected target list is derived from the managed ClickHouse UnitSet/Pods
  and compared with Prometheus results.
- Prometheus unavailable returns HTTP `503` with
  `PROMETHEUS_UNAVAILABLE`.
- Prometheus available with missing targets or optional metric gaps returns
  HTTP `200`, status `DEGRADED`, and actionable warnings.
- All expected targets up and all required summary queries successful returns
  HTTP `200`, status `READY`.

## 7. Grafana Dashboard

This phase must close the Grafana ownership question and still deliver a
usable demo dashboard.

MVP decision:

- `upm-api-server` does not generate or manage Grafana dashboards.
- Grafana and its dashboards are external observability environment assets.
- The external `../kube-prometheus-stack/` scripts must provision the adapted
  ClickHouse dashboard automatically.
- The dashboard must be variable-driven and discover the deployed ClickHouse
  cluster from Prometheus labels rather than hardcoding individual Pod IPs.
- The dashboard asset is stored at
  `clickhouse/grafana/upm-clickhouse-23285-dashboard.json`.
- The dashboard is adapted from the user-provided Grafana dashboard
  `23285_rev1.json` and keeps only panels whose visible target queries return
  real samples in the Phase 05 runtime.
- Node-exporter-only panels and standalone Keeper service panels are excluded
  unless those metrics are added to the external environment.

Minimum MVP coverage:

| Panel | Required data source |
|---|---|
| Target up/down | Prometheus `up` from ClickHouse PodMonitor |
| CPU | kubelet/cAdvisor |
| Memory | kubelet/cAdvisor |
| Disk used/available/total | ClickHouse native disk metrics or kubelet PVC metrics |
| Query count | ClickHouse native profile events |
| Insert query count | ClickHouse native profile events |
| Inserted rows | ClickHouse native profile events |
| Inserted bytes | ClickHouse native profile events |
| ClickHouse memory tracking | ClickHouse native metrics |

The adapted dashboard may include additional validated ClickHouse panels for
queries, inserts, selects, Kafka counters, merges, mutations, background pools,
backup/restore thread counters, IO, replicas, cache, parts, distributed
inserts, and ClickHouse client-to-Keeper activity.

## 8. Files Likely Changed

```text
api-server/internal/prometheus/client.go
api-server/internal/prometheus/queries.go
api-server/internal/api/server.go
api-server/internal/model/metrics.go
api-server/internal/platform/store.go
api-server/internal/kube/store.go
clickhouse/phase-03/manifests/upm-api-server.yaml
clickhouse/scripts/validate-phase05-monitoring-runtime.sh
clickhouse/phase-05/runtime-validation.md
docs/design/monitoring-design.md
docs/api/upm-api-server-v1.md
```

## 9. Acceptance Criteria

1. ClickHouse pod listens on the configured metrics port, default `9363` unless changed by design.
2. `curl http://127.0.0.1:9363/metrics` returns Prometheus text from inside the ClickHouse container.
3. `UnitSet.spec.podMonitor.enable=true` creates PodMonitor when PodMonitor CRD exists.
4. PodMonitor selects ClickHouse pods by correct labels.
5. A real Prometheus Server discovers and scrapes every expected ClickHouse
   Server Pod.
6. Prometheus target evidence shows expected targets equal actual targets and
   all expected targets are up.
7. `upm-api-server` `/metrics/summary` returns structured response and does not leak secrets.
8. `/metrics/summary` returns real target, CPU, memory, PVC/storage, and
   ClickHouse query/write metric evidence from Prometheus.
9. `metrics-server` output, if available, is treated only as supplementary resource evidence and not as a substitute for the Prometheus scrape chain.
10. Grafana dashboard ownership decision is recorded.
11. Grafana is installed by the external environment scripts, exposed for demo
    access, and has a provisioned Prometheus datasource.
12. ClickHouse dashboard is automatically imported from the repository
    dashboard asset and contains the required MVP coverage.
13. Every visible dashboard target query included in the imported dashboard
    returns non-empty data through the Grafana datasource proxy.
14. Prometheus unavailability and partial target failures follow the defined
    structured API error/status behavior.
15. A real-environment validation script proves the full chain and saves its
    result under `clickhouse/phase-05/`.

## 10. Verification Commands

```bash
# CRDs
kubectl get crd | grep -Ei 'podmonitors|servicemonitors|prometheuses|prometheusrules'

# PodMonitor
kubectl get podmonitor -A | grep -i clickhouse
kubectl get podmonitor clickhouse-phase03-exporter-podmon -n upm-clickhouse-phase03-runtime -o yaml

# Endpoint
kubectl exec -n upm-clickhouse-phase03-runtime <clickhouse-pod> -c clickhouse -- curl -sS --max-time 5 http://127.0.0.1:9363/metrics | head

# Prometheus target
kubectl port-forward -n monitoring svc/<prometheus-service> 9090:9090
curl -sS 'http://127.0.0.1:9090/api/v1/query?query=up{namespace="upm-clickhouse-phase03-runtime"}' | jq .

# Optional resource metrics only; this does not replace Prometheus validation
kubectl top pods -n upm-clickhouse-phase03-runtime

# UPM API Server
curl -sS http://192.168.35.201:30083/api/v1/clusters/upm-clickhouse-phase03-runtime/clickhouse-phase03/metrics/summary | jq .

# Grafana
curl -sS -u admin:admin http://192.168.35.201:30300/api/health | jq .

# Full real-environment acceptance
clickhouse/scripts/validate-phase05-monitoring-runtime.sh
```

## 11. Risks and Open Questions

| Risk / Question | Handling |
|---|---|
| Prometheus images unavailable | Resolve the external environment prerequisite before Phase 05 closeout |
| Confusing `metrics-server` with Prometheus | Use `metrics-server` only for supplementary resource evidence; require `/metrics` and PodMonitor validation |
| Native endpoint lacks desired metrics | Keep exporter sidecar as future option |
| Metrics label names differ by version | Query by stable labels and document assumptions |
| Grafana dashboard drifts from PromQL | Validate dashboard panel queries through Grafana datasource proxy |

## 12. Changelog

| Version | Date | Changes |
|---|---|---|
| 0.1 | 2026-05-27 | Initial phase draft |
| 0.2 | 2026-05-27 | Added metrics and PodMonitor scope |
| 0.3 | 2026-05-27 | Added metadata and red-team fix structure |
| 0.4 | 2026-05-27 | Restored concrete monitoring boundary, metrics API, Grafana ownership decision, and runtime verification commands |
| 0.5 | 2026-06-02 | Added explicit `/metrics -> PodMonitor -> Prometheus -> upm-api-server` closure and clarified that `metrics-server` is supplementary only |
| 0.6 | 2026-06-06 | Required real Prometheus scrape/API closure, defined external environment asset location, API status semantics, query safety boundary, and real-environment validation |
| 0.7 | 2026-06-06 | Added external Grafana datasource/dashboard automation and per-panel data validation to Phase 05 acceptance |
| 0.8 | 2026-06-07 | Replaced the temporary dashboard with an adapted Grafana 23285 dashboard asset and required visible target query validation |
