# Monitoring Design

- Version: 0.3
- Date: 2026-05-27
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

## 6. Future Enhancements

- Grafana dashboard templates;
- PrometheusRule alerting;
- richer ClickHouse PromQL library;
- exporter sidecar if native endpoint is insufficient;
- backup metrics;
- capacity trend analysis.

## 7. Acceptance Criteria

- PodMonitor is created for ClickHouse Server pods.
- ClickHouse metrics endpoint returns Prometheus-format output.
- Prometheus can discover and scrape the target when stack is installed.
- `upm-api-server` can query and summarize key metrics.
