# ClickHouse Grafana Dashboard

This directory stores reusable Grafana dashboard assets for the UPM ClickHouse
hackathon environment.

## Dashboard

- File: `upm-clickhouse-23285-dashboard.json`
- Runtime UID: `upm-clickhouse-overview`
- Runtime title: `UPM ClickHouse Operational Dashboard`
- Source: user-provided `23285_rev1.json`
- Source title: `ClickHouse and Keeper Comprehensive Dashboard`

## Adaptation Boundary

The source dashboard expects `cluster` and `instance` Prometheus labels, plus
node-exporter and standalone Keeper metrics.

The Phase 05 runtime uses Kubernetes PodMonitor labels from the ClickHouse
native metrics endpoint. The adapted dashboard therefore:

- uses datasource UID `prometheus`;
- uses `namespace` and `cluster` dashboard variables;
- rewrites ClickHouse metric selectors to `namespace` and `pod` labels;
- keeps panels whose visible PromQL targets return real samples;
- drops node-exporter-only panels because node-exporter is not enabled in the
  Phase 05 external environment;
- drops the independent Keeper service overview because standalone Keeper
  metrics are not scraped in Phase 05;
- keeps ClickHouse client-to-Keeper panels because those are ClickHouse native
  profile events exposed by the server pods.

Runtime validation evidence is stored under `clickhouse/phase-05/`.
