# Data Architecture

- Version: 0.3
- Date: 2026-06-06
- Status: Sealed
- Owner: zqw
- Related:
  - ../master-spec.md
  - resource-model.md
  - api-design.md

## 1. Purpose

This document defines the top-level data domains, ownership boundaries,
persistence strategy, and data flow for the `upm-api-server` MVP and roadmap.

## 2. Data Domains

| Data Domain | Owner | Storage / Source of Truth | MVP Handling |
|---|---|---|---|
| User request data | `upm-api-server` API | request payload | validated and converted into UPMIO/K8s resources |
| UPMIO control-plane state | Kubernetes + UPMIO | Kubernetes etcd through CRDs | read from Project, UnitSet, Unit, PodMonitor, future GrpcCall |
| Kubernetes native state | Kubernetes | Kubernetes etcd | read Pods, PVCs, Services, Endpoints, Secrets metadata, Events |
| ClickHouse business data | ClickHouse | ClickHouse PVCs and tables | not managed by `upm-api-server` except validation probes |
| Keeper metadata | ClickHouse Keeper | Keeper PVC/state | checked through Keeper health probes and ClickHouse system state |
| Metrics | Prometheus | Prometheus TSDB | queried through Prometheus API |
| Healthcheck reports | `upm-api-server` | generated result; in-memory latest cache in MVP | returned by API; persistent report history is future scope |
| Diagnostics outputs | `upm-api-server` | generated from SQL/K8s/Prometheus | returned by API, optional future persistence |
| Operation history | `upm-api-server` / future audit store | Structured logs and current Kubernetes state in MVP | no independent history database in MVP |

## 3. MVP Storage Decision

MVP `upm-api-server` should be stateless or near-stateless.

MVP does not introduce an independent database such as PostgreSQL, SQLite, or
MySQL. `upm-api-server` reads authoritative state from:

- Kubernetes API;
- UPMIO CRDs;
- ClickHouse SQL system tables;
- Prometheus API.

Phase 04 uses an in-memory latest-report cache keyed by cluster identity. If no
report exists in the current API server process, `GET latest` returns a
structured not-found response. Productization may introduce a formal
report/audit store.

## 4. Data Flow

```text
User / UI / API Client
        |
        v
UPM API Server
        |
        +--> Kubernetes API / UPMIO CRDs
        |       Project / UnitSet / Unit / PodMonitor / future GrpcCall
        |
        +--> Kubernetes native resources
        |       Pod / PVC / Service / Endpoint / ConfigMap / Secret metadata / Event
        |
        +--> ClickHouse SQL
        |       system.clusters / system.replicas / system.parts / system.merges / system.mutations
        |
        +--> Prometheus API
        |       target status / resource metrics / ClickHouse metrics
        |
        v
Healthcheck Report / Diagnostics Summary / Operation Result
```

## 5. Security and Retention

- Secrets must not be stored in ConfigMap.
- Secret values must not appear in API responses, logs, reports, or Git.
- API output must redact sensitive fields.
- Operation history retention is future work unless explicitly implemented by a phase.
- Healthcheck and diagnostics report retention is minimal in MVP and must be defined before productization.

## 6. Future Persistence Options

| Option | Use Case | Notes |
|---|---|---|
| Kubernetes CR / ConfigMap | lightweight latest report | simple but not ideal for long-term history |
| PostgreSQL / relational store | operation history and audit | productization candidate |
| Object storage | large report/archive | candidate for backup/report archive |
| Prometheus | time-series metrics only | not a general report store |

## 7. Open Questions

| Question | Resolution Phase |
|---|---|
| Does operation history need a persistent database before demo? | Resolved in Phase 03: no independent database in MVP; persistent audit history is future scope |
| Should latest healthcheck report be persisted in Kubernetes? | Resolved in Phase 04: no Kubernetes persistence in MVP; use in-memory latest cache |
| What retention policy is required for audit reports? | Future |
