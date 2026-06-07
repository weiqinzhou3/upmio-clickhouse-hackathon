# UPMIO ClickHouse Hackathon Master Spec

- Version: 0.8
- Date: 2026-06-07
- Status: Sealed
- Project: UPMIO ClickHouse Hackathon
- Codename: upm-clickhouse
- Repository: local repository `upm-clickhouse`; final remote repository TBD
- Methodology: Phase Spec Driven Development
- Current Stage: Hackathon second-stage preparation
- Target Demo Date: TBD by hackathon schedule
- Owner: zqw
- Language: English sealed version

## 1. Project Goal

This project designs and implements a prototype ClickHouse operations management system based on UPMIO Operator capabilities.

The project does not build a ClickHouse Operator from scratch, and it does not bypass UPMIO to directly manage native Kubernetes workloads as the product path. It uses UPMIO `unit-operator`, `compose-operator`, and `upm-packages` as the capability foundation, then adds ClickHouse package adaptation, cluster deployment management, status aggregation, Day1 acceptance, monitoring integration, Day2 diagnostics, and future operation extensions.

One-sentence positioning:

> Build a ClickHouse HA cluster deployment, acceptance, monitoring, diagnostics, and operations control plane on top of UPMIO UnitSet / package / task capabilities.

## 2. Evidence Basis

This Spec is based on two rounds of Codex research and validation:

1. Discovery phase: repository analysis, CRD model analysis, MySQL Day1/Day2 implementation analysis, monitoring capability analysis, and ClickHouse mapping hypothesis.
2. Runtime phase: UPMIO Operator installation, UnitSet runtime behavior, ClickHouse Keeper validation, ClickHouse Server validation, monitoring validation, GrpcCall validation, and topology decision validation.

This file does not repeat raw evidence. The condensed evidence entry point is:

- [Evidence Summary](evidence-summary.md)

Raw evidence is kept in the local project under:

- `docs/discovery/`
- `docs/runtime/`

By default, AI coding agents only need to read:

1. `docs/master-spec.md`
2. `docs/evidence-summary.md`
3. the current phase spec
4. architecture/design documents explicitly referenced by the current phase spec

Open `docs/discovery/` or `docs/runtime/` only when a phase spec requires evidence verification, or when actual behavior conflicts with the expected behavior.

## 3. Requirement Coverage Principle

- MVP: the minimum executable, demonstrable, and acceptable loop for the second-stage demo.
- P1/P2/P3: still part of the project scope, but delivered through later phases, enhancements, or productization.
- Non-Goal: explicitly excluded from this project or the current stage to prevent scope creep.

Detailed requirement mapping:

- [Day1 / Day2 Requirement Coverage Matrix](design/day1-day2-requirement-coverage.md)

Rule: Day1 and Day2 requirements are project scope. They are not optional features that can be silently dropped. Requirements outside MVP must be mapped to a later phase, Future Roadmap, or Non-Goal.

## 4. MVP Scope

The MVP goal is to form a runnable, demonstrable, and explainable ClickHouse HA deployment and Day1 acceptance loop.

### 4.1 MVP Topology

Executable MVP baseline:

- 3-node ClickHouse Keeper.
- 1 shard x 2 replicas ClickHouse Server.
- Keeper-backed ReplicatedMergeTree.
- Kubernetes Services expose TCP / HTTP / Interserver / metrics ports.
- ClickHouse native Prometheus endpoint.
- Automatic PodMonitor discovery.

Target production architecture and future enhancement:

- N shards x M replicas.
- First target enhancement topology: 2 shards x 2 replicas.
- Future validation targets: 2 shards x 3 replicas and 4 shards x 2 replicas.

The MVP uses 1 shard x 2 replicas only as the first repeatable runtime baseline. It does not limit the target architecture. Multi-shard topology is explicit project scope and is covered by the roadmap and Phase Plan.

### 4.2 MVP Deliverables

MVP must deliver:

1. UPMIO runtime prerequisite check.
2. Basic fixes for `upm-packages/clickhouse` and `upm-packages/clickhouse-keeper`.
3. Secret-based ClickHouse default/admin user initialization.
4. ClickHouse Keeper UnitSet deployment.
5. ClickHouse Server UnitSet deployment.
6. ClickHouse native Prometheus endpoint.
7. PodMonitor discovery.
8. Basic Go `upm-api-server` APIs.
9. Day1 healthcheck and structured acceptance report.
10. Basic read-only Day2 diagnostics: Keeper, replica, parts, merges, mutations, storage, and service endpoints.
11. Narrow backup/restore validation slice for isolated validation objects,
    using an approved Kubernetes Job task path when ClickHouse GrpcCall remains
    unsupported.
12. Minimal API-managed scheduled backup validation slice using Kubernetes
    CronJob, without production retention cleanup.

### 4.3 Security Baseline

- ClickHouse must use password-based users.
- Passwords must come from Kubernetes Secret, a secure generation flow, or an approved UPMIO Secret mechanism.
- `<no_password/>` is prohibited.
- Plaintext passwords must not be written to ConfigMap, logs, reports, or Git.
- Healthcheck output, `upm-api-server` responses, and example YAML must not leak Secret content.
- High-risk write operations must reserve a human-approval mechanism. MVP does not implement high-risk automated remediation.

## 5. Non-Goals

### 5.1 Do not reimplement UPMIO base operators

Do not reimplement Unit / UnitSet reconciliation logic. Do not directly take over low-level lifecycle management for Pods, PVCs, Services, ConfigMaps, or similar native Kubernetes resources.

### 5.2 MVP does not add a ClickHouse-specific compose-operator CRD

A future product may add `ClickHouseCluster` or `ClickHouseTopology`, but MVP does not implement a new compose controller.

### 5.3 MVP does not rely on ClickHouse GrpcCall

Runtime validation showed that the installed `unit-operator:v1.1.0` does not support `GrpcCall(type=clickhouse)`. MVP does not promise backup, restore, set-variable, or create-user through GrpcCall.

### 5.4 MVP does not implement the full production backup/restore loop

Backup/restore is required ClickHouse operations scope.

The hackathon MVP may implement a narrow executable validation slice:

- backup isolated validation database/table objects;
- create an API-managed scheduled backup for isolated validation objects;
- restore into a separate validation database/table;
- verify restored data by row count and, where feasible, checksum;
- use Kubernetes Secret references for ClickHouse and storage credentials;
- use an approved Kubernetes Job task path if `GrpcCall(type=clickhouse)`
  remains unsupported.
- use Kubernetes CronJob for the minimal scheduled backup API when explicitly
  requested by the owner.

The MVP still does not implement the full production backup/restore loop:

- no production backup policy engine;
- no retention cleanup;
- no full object storage lifecycle management;
- no destructive restore into business tables;
- no restore overwrite workflow without explicit future safety design.

### 5.5 MVP does not implement automatic shard scale-out or data redistribution

Multi-shard topology is project scope. MVP does not implement automatic shard addition, data redistribution, historical data migration, or automatic Distributed table rebuild.

### 5.6 MVP does not implement complete user/permission lifecycle management

User, role, profile, and quota lifecycle management are project scope. MVP only requires secure account initialization, not full RBAC management.

### 5.7 Do not embed the Prometheus / Grafana platform

Prometheus/Grafana are external dependencies. `kube-prometheus-stack` is recommended. This project handles ClickHouse metric exposure, PodMonitor, Prometheus queries, and summary presentation, not full monitoring platform lifecycle.

### 5.8 MVP does not manage Grafana dashboards through the product API

Grafana remains an external observability dependency in MVP. The hackathon
environment may provision a versioned ClickHouse dashboard asset for demo and
validation, but `upm-api-server` does not generate, edit, or own Grafana
dashboard lifecycle. PrometheusRule alerting remains productization scope.

### 5.9 Do not install ClickHouse directly on host machines

ClickHouse Server, ClickHouse Keeper, `upm-api-server`, and monitoring
components must run inside Kubernetes.

## 6. Future Roadmap

Future Roadmap is part of project scope, but not an MVP commitment.

### 6.1 Multi-shard topology

Future support:

- 2 shards x 2 replicas;
- 2 shards x 3 replicas;
- 4 shards x 2 replicas;
- N shards x M replicas.

Required capabilities:

- `topology.shards`;
- `topology.replicasPerShard`;
- dynamic `remote_servers`;
- dynamic `macros.shard` / `macros.replica`;
- Keeper path strategy;
- Distributed table strategy;
- distributed DDL strategy;
- topology status aggregation;
- pre-scale risk assessment;
- post-scale healthcheck and replication validation.

### 6.2 ClickHouse-specific topology CRD

When package templates plus `upm-api-server` orchestration are insufficient for
complex lifecycle management, consider adding a ClickHouse topology CRD to
`compose-operator`.

Candidate directions:

- `ClickHouseCluster`
- `ClickHouseTopology`
- `ClickHouseShard`
- `ClickHouseReplicaSet`

This is a productization path and is not part of MVP.

### 6.3 Day2 operations

Future capabilities include:

- backup;
- restore;
- set-variable;
- user/profile/quota management;
- rolling restart;
- replica add/remove;
- shard scale-out;
- config change and rollback;
- mutation kill;
- optimize control;
- storage expansion;
- TTL / partition maintenance;
- operation audit and approval.

## 7. Data Architecture Summary

Detailed design:

- [Data Architecture](design/data-architecture.md)

Top-level data flow:

```text
User / UI / API Client
        |
        v
UPM API Server
        |
        +--> Kubernetes API / UPMIO CRDs
        |       - Project
        |       - UnitSet
        |       - Unit
        |       - PodMonitor
        |       - GrpcCall (future)
        |
        +--> ClickHouse SQL
        |       - system.clusters
        |       - system.replicas
        |       - system.parts
        |       - system.merges
        |       - system.mutations
        |
        +--> Prometheus API
        |       - metrics and time-series data
        |
        v
Healthcheck / Diagnostics / Operation Result
```

Top-level decisions:

- `upm-api-server` is stateless or near-stateless in MVP.
- MVP does not introduce an independent control-plane database.
- UPMIO/Kubernetes state lives in Kubernetes etcd through CRDs and native resources.
- ClickHouse business data lives in ClickHouse PVCs and tables.
- Keeper metadata lives in Keeper PVC/state.
- Metrics live in Prometheus TSDB.
- Healthcheck/diagnostic reports are generated by `upm-api-server`.
  Persistence is minimal in MVP and may be enhanced later.
- Operation history persistence is future work unless a phase explicitly implements it.

## 8. Overall Design

The project architecture is:

> UPMIO Operator capability foundation + ClickHouse package adaptation + UPM API Server product control plane

### 8.1 Architecture design

See:

- [UPMIO Architecture and Component Boundaries](architecture/upmio-architecture.md)
- [ClickHouse HA Architecture](architecture/clickhouse-ha-architecture.md)

### 8.2 Product design

See:

- [Product Design](design/product-design.md)

### 8.3 Resource model design

See:

- [Resource Model](design/resource-model.md)

### 8.4 upm-packages/clickhouse adaptation design

See:

- [upm-packages ClickHouse Design](design/upm-packages-clickhouse-design.md)

### 8.5 UPM API Server design

See:

- [API Design](design/api-design.md)
- [UPM API Server v1 API Reference](api/upm-api-server-v1.md)

### 8.6 Monitoring integration design

See:

- [Monitoring Design](design/monitoring-design.md)

### 8.7 Day2 diagnostics design

See:

- [Day2 Diagnostics Design](design/day2-diagnostics-design.md)

## 9. Phase Plan

| Phase | Name | Goal | Priority | Status | Spec |
|---|---|---|---:|---|---|
| Phase 01 | upm-packages/clickhouse foundation adaptation | Fix package so 1 shard x 2 replicas + 3 Keeper can be repeatedly deployed | P0 | Confirmed | [phase-01](phases/phase-01-upm-packages-clickhouse.md) |
| Phase 02 | ClickHouse topology parameterization | Support generic `topology.shards` / `topology.replicasPerShard` template design and static validation | P1 | Confirmed | [phase-02](phases/phase-02-clickhouse-topology.md) |
| Phase 03 | UPM API Server | Implement `upm-api-server`, UPMIO adapter, K8s reader, ClickHouse client, and K8s deployment assets | P0 | Confirmed | [phase-03](phases/phase-03-upm-api-server.md) |
| Phase 04 | Healthcheck | Implement Day1 acceptance and structured reports | P0 | Confirmed | [phase-04](phases/phase-04-healthcheck.md) |
| Phase 05 | Monitoring | Enable metrics endpoint, PodMonitor, and Prometheus query summary | P0 | Confirmed | [phase-05](phases/phase-05-monitoring.md) |
| Phase 06 | Day2 Diagnostics | Implement read-only diagnostics for replicas, parts, mutations, Keeper, storage, and write path | P1 | Confirmed | [phase-06](phases/phase-06-day2-diagnostics.md) |
| Phase 07 | Backup & Restore | Design and validate ClickHouse backup/restore task model, including a narrow Kubernetes Job validation slice when GrpcCall remains unsupported | P1 | Confirmed | [phase-07](phases/phase-07-backup-restore.md) |
| Phase 08 | Config / Lifecycle / Scaling | Design and phase implementation for config change, lifecycle, and scaling management | P2 | Confirmed | [phase-08](phases/phase-08-config-lifecycle-scaling.md) |

## 10. Risks and Constraints

| Risk | Impact | Handling |
|---|---|---|
| Registry access unreliable | Images cannot be pulled by nodes | Use private registry or local image import |
| Current package is not turnkey | ClickHouse startup may fail | Fix `upm-packages/clickhouse` first |
| `GrpcCall(type=clickhouse)` unsupported | UPMIO GrpcCall task execution unavailable | Use approved Kubernetes Job path for backup/restore validation; keep GrpcCall repair as verified spike |
| Current template does not support 2x2 | Multi-shard topology is not immediately executable | Add topology parameterization in Phase 02 |
| Metrics endpoint not enabled | Prometheus cannot scrape ClickHouse | Enable native Prometheus endpoint in Phase 01/05 |
| `<no_password/>` prohibited | Previous runtime workaround is insecure | Use Secret-based password initialization |
| Prometheus/Grafana not included | Monitoring stack absent | Declare kube-prometheus-stack as external dependency |
| Day1/Day2 scope is large | MVP may become too broad | Use requirement coverage matrix and phased priority |

## 11. AI Coding Agent Rules

Rules:

1. Do not write feature code without a confirmed phase spec.
2. Do not expand scope beyond the current phase.
3. Always read `docs/master-spec.md`, `docs/evidence-summary.md`, the current phase spec, and referenced design/architecture documents.
4. Do not read all raw discovery/runtime files unless required.
5. Do not weaken tests to make implementation pass.
6. Do not add `<no_password/>` to ClickHouse configuration.
7. Do not bypass UPMIO by directly creating Pods/PVCs/Services unless explicitly stated.
8. Do not implement a new compose-operator CRD in MVP.
9. Do not depend on ClickHouse GrpcCall until runtime support is verified.
10. When a requirement is not implemented in MVP, map it to a future phase or explicitly record it as Non-Goal.

## 12. When In Doubt / Default Decision Bias

- UPMIO-native capability vs self-implementation: prefer UPMIO-native capability.
- Safety vs convenience: prefer safety.
- Cloud-native declarative configuration vs host-level scripts: prefer cloud-native declarative configuration.
- High-risk automation vs human approval: require human approval.
- ClickHouse system tables vs custom collection: prefer system tables.
- Small executable MVP vs broad feature coverage: prefer executable MVP and map remaining scope to later phases.
- Sealed decisions vs opportunistic optimization: follow sealed decisions.
- Runtime evidence vs source-code assumption: prefer runtime evidence.

## 13. Decision Record

| Decision | Status | Rationale |
|---|---|---|
| Use UnitSet for ClickHouse Server and Keeper | Sealed | Runtime validation proved UnitSet path works |
| Modify `upm-packages/clickhouse` first | Sealed | Current package is not turnkey |
| Use password-based default/admin user | Sealed | `<no_password/>` is not acceptable |
| Use native ClickHouse Prometheus endpoint | Sealed | Simpler than exporter sidecar for MVP |
| Do not rely on GrpcCall in MVP | Sealed | Runtime image rejects `type=clickhouse` |
| Use Kubernetes Job path for backup/restore validation when GrpcCall remains unsupported | Sealed | Backup/restore is required database operations scope, and the hackathon demo should not depend on uncertain GrpcCall source repair |
| Include minimal scheduled backup API in Phase 07 | Sealed | Owner requires scheduled backup to be available through API; MVP implements this through Kubernetes CronJob without production retention cleanup |
| Do not implement new compose CRD in MVP | Sealed | Too high-risk for first demo |
| Keep N shards x M replicas in roadmap | Sealed | Required for product-grade architecture |
| First executable baseline is 1 shard x 2 replicas | Sealed | Runtime validation proved this topology is the first repeatable executable baseline. Multi-shard topologies remain in roadmap and Phase 02 |
| Treat Day1/Day2 requirements as project scope | Sealed | Requirements are not optional features; only priority differs |
| `upm-api-server` uses Go | Sealed | Aligns with Kubernetes, UPMIO, controller-runtime/client-go ecosystem |
| API uses structured error response | Sealed | Required for UI, automation, and troubleshooting |
| `upm-api-server` uses structured logging with redaction | Sealed | Required for auditability, diagnosis, and secret safety |
| MVP `upm-api-server` does not require an independent database | Sealed | Keeps MVP stateless and reduces moving parts |
| Prometheus/Grafana remain external dependencies | Sealed | UPMIO public repos provide integration points, not a full monitoring stack |
| `upm-api-server` owns validation-only Distributed table initialization in MVP | Sealed | Production-grade behavior must not silently create or mutate user business tables during cluster creation; `upm-api-server` may create deterministic validation objects for healthcheck |
| `upm-api-server` should provide controlled database management as a future production capability | Sealed | Enterprise users need API/UI workflows for database lifecycle. User business local table and Distributed table lifecycle remains DBA/application-owned; `upm-api-server` may create validation objects and read table metadata for diagnostics |
| Multi-shard write routing uses application Distributed tables through a stable query service | Sealed | Direct local-table writes bypass cluster routing and are only acceptable for controlled validation/admin workflows; production applications need an explicit Distributed table and stable service endpoint |
| `upm-api-server` generated DDL must be idempotent and drift-aware | Sealed | `IF NOT EXISTS` is required but not sufficient; API-server-generated DDL is limited to database lifecycle and validation objects, and must validate existing definitions instead of silently overwriting drift |
| One `upm-api-server` instance manages multiple database clusters | Sealed | The product control plane is not ClickHouse-specific and must be extensible to MySQL, Redis, and other database API surfaces |
| New product capabilities register through `upm-api-server` | Sealed | User-facing and automation-facing behavior should have one product API entry point; package-only/operator-only/evidence-only work must be explicitly scoped as such |
| Grafana dashboards are external environment assets in MVP | Sealed | Phase 05 owns metric exposure, Prometheus discovery/query, API summary, and a demo-ready adapted Grafana dashboard asset provisioned by external kube-prometheus-stack environment scripts; `upm-api-server` does not generate or manage dashboards |
| Final MVP demo requires one-command environment preparation | Sealed | After all MVP phases, provide scripted environment preparation for Kubernetes, UPMIO, kube-prometheus-stack, API-driven ClickHouse deployment, API operations, and inspection docs |

## 14. Open Questions

| Question | Current Status | Expected Resolution Phase |
|---|---|---|
| Should Distributed tables be initialized by `upm-api-server` or left to application users? | Resolved: `upm-api-server` initializes validation-only Distributed tables for healthcheck during MVP. Production `upm-api-server` may expose database management API/UI, but user business local table and Distributed table lifecycle remains DBA/application-owned. `upm-api-server` must not silently create business tables during cluster creation | Phase 02 decision; Phase 04 validation implementation; production database API/UI later |
| How should write routing be exposed for multi-shard clusters? | Resolved: applications write/query through Distributed tables via a stable query service; direct local table writes are validation/admin-only | Phase 02 |
| How should DDL idempotency be guaranteed? | Resolved: use idempotent SQL, record desired database/validation-object target and checksum in future `upm-api-server`/audit state, and verify API-server-owned objects for drift before applying changes | Phase 02 decision; Phase 04+ implementation |
| Which unit-operator image supports `GrpcCall(type=clickhouse)`? | TBD | Phase 07 |
| Does operation history require persistent storage? | Resolved: MVP does not introduce an independent operation-history database; synchronous API results, structured logs, and Kubernetes resource/Event state provide current evidence. Persistent audit history is future productization scope | Phase 03 decision; Productization |
| Should Grafana dashboards be generated by `upm-api-server` or managed externally? | Resolved: MVP does not generate or manage Grafana dashboards through `upm-api-server`. The hackathon environment provisions an adapted ClickHouse dashboard asset through external kube-prometheus-stack assets. Future dashboards are separately versioned observability assets or externally managed content | Phase 05 decision; Productization |
| Which authentication and authorization model is required after MVP? | TBD | Phase 03 / Future |

## 15. Seal Statement

This Master Spec is sealed for first-stage review.

Any later change to MVP scope, sealed decisions, non-goals, phase boundaries, security baseline, or UPMIO integration direction must be recorded as a new decision and reviewed before implementation.

Phase specs may still be refined before each phase starts, but they must not contradict this Master Spec.
