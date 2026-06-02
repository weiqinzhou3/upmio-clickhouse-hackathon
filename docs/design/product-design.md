# Product Design

- Version: 0.3
- Date: 2026-06-02
- Status: Sealed
- Owner: zqw
- Related:
  - ../master-spec.md
  - api-design.md
  - day1-day2-requirement-coverage.md

## 1. Product Positioning

ClickHouse Manager is a product control plane for deploying, accepting, monitoring, and diagnosing ClickHouse HA clusters on top of UPMIO.

It turns UPMIO lower-level Operator and package capabilities into a workflow that a DBA or platform engineer can understand and demonstrate.

## 2. User Roles

| Role | Responsibilities | MVP Permissions |
|---|---|---|
| DBA / Database Operations Engineer | Define ClickHouse topology, run healthcheck, read diagnostics | create/read clusters, run checks |
| Platform SRE / Kubernetes Administrator | Prepare Kubernetes, UPMIO, registry, storage, monitoring | platform prerequisite management |
| Application Owner / Observer | Read cluster status and health results | read-only |
| Approver / Administrator | Approve high-risk future operations | future |

## 3. End-to-end User Journey

```text
Prepared Kubernetes Cluster
        |
        v
Prerequisite Check
  - UPMIO operators
  - package charts
  - Project/namespace
  - StorageClass
  - monitoring CRDs
        |
        v
Create ClickHouse HA Cluster
  - Keeper UnitSet
  - Server UnitSet
  - Secret-based users
  - Prometheus endpoint
        |
        v
Wait for Runtime Ready
  - UnitSet / Unit status
  - Pod readiness
  - PVC bound
  - Service endpoints
        |
        v
Run Day1 Acceptance
  - SELECT 1
  - system.clusters
  - system.replicas
  - write/read probe
  - Keeper status
  - metrics endpoint
        |
        v
View Acceptance Report
        |
        v
View Monitoring Summary and Day2 Diagnostics
```

## 4. Product Modules

| Module | MVP | Description |
|---|---|---|
| Cluster Management | Yes | create/list/detail for ClickHouse clusters |
| Topology View | Yes | Keeper, Server, UnitSet, Unit, Pod, PVC, Service status |
| Day1 Acceptance Report | Yes | structured pass/fail report |
| Monitoring Summary | Yes | Prometheus target and key metric summary |
| Day2 Diagnostics | Yes | read-only diagnostics for replicas, parts, merges, mutations, Keeper, storage |
| Database Management | Future | explicit database lifecycle with validation, approval/audit, and post-apply verification |
| Operation History | Partial/Future | phase-dependent; full persistence future |
| Operation Center | Future | backup, restore, scaling, config change, approval |

## 5. Wireframes

### 5.1 Cluster Creation Wizard

```text
+--------------------------------------------------+
| Create ClickHouse Cluster                        |
+--------------------------------------------------+
| Cluster Name: [ ch-demo                       ]  |
| Namespace:    [ upm-clickhouse                ]  |
| Version:      [ 26.3 LTS                      ]  |
| Topology:     [ 1 shard x 2 replicas          ]  |
| Keeper:       [ 3 nodes                       ]  |
| StorageClass: [ local-path                    ]  |
| Data Size:    [ 100Gi                         ]  |
| Admin Secret: [ existing / generate           ]  |
|                                                  |
| [Validate] [Create]                              |
+--------------------------------------------------+
```

### 5.2 Cluster Detail / Topology

```text
+--------------------------------------------------+
| Cluster: ch-demo                                 |
+--------------------------------------------------+
| Status: Running / Degraded / Failed              |
| Keeper: 3/3 Ready   Server: 2/2 Ready            |
|                                                  |
| Keeper Ensemble                                  |
|  keeper-0 follower  keeper-1 leader keeper-2 ... |
|                                                  |
| Shard 01                                         |
|  replica-01 Ready  replica-02 Ready              |
|                                                  |
| PVC: Bound    Services: Ready    PodMonitor: OK  |
+--------------------------------------------------+
```

### 5.3 Healthcheck Report

```text
+--------------------------------------------------+
| Day1 Acceptance Report                           |
+--------------------------------------------------+
| Overall: PASS / WARN / FAIL                      |
|                                                  |
| [PASS] UnitSet ready                             |
| [PASS] PVC bound                                 |
| [PASS] SELECT 1                                  |
| [PASS] system.clusters                           |
| [PASS] system.replicas                           |
| [WARN] metrics endpoint                          |
|                                                  |
| [Download JSON] [Re-run]                         |
+--------------------------------------------------+
```

### 5.4 Day2 Diagnostics

```text
+--------------------------------------------------+
| Day2 Diagnostics                                 |
+--------------------------------------------------+
| Replica readonly: 0                              |
| Replication queue: 0                             |
| Active parts TopN: ...                           |
| Mutations running/failed: ...                    |
| Keeper leader/followers: ...                     |
| Storage pressure: ...                            |
|                                                  |
| [Refresh] [Export]                               |
+--------------------------------------------------+
```

### 5.5 Future Database Management

```text
+--------------------------------------------------+
| Database Management                              |
+--------------------------------------------------+
| Database: [ analytics                         ]  |
| Owner:    [ application / analytics team      ]  |
| Access:   [ Manager account validation        ]  |
|                                                  |
| [Dry Run] [Apply with Approval] [Verify]          |
+--------------------------------------------------+
```

Rules:

- This module is a future production-grade product capability.
- User business local table and Distributed table lifecycle remains
  DBA/application-owned, not Manager-owned.
- Manager may show read-only table metadata for diagnostics and may create
  reserved validation objects for healthcheck.
- UI actions for database lifecycle must call explicit Manager APIs and show
  dry-run, drift, approval, and verification results.

## 6. Non-Goals

- MVP does not require a complete frontend implementation if a phase does not include it.
- MVP does not implement full RBAC.
- MVP does not implement destructive operations from UI.

## 7. Acceptance Criteria

- Product roles are defined.
- End-to-end user journey is clear.
- MVP pages/modules can support a demo narrative.
- Product design maps back to API and Day1/Day2 requirement coverage.
