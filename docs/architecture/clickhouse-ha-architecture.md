# ClickHouse HA Architecture

- Version: 0.4
- Date: 2026-06-02
- Status: Sealed
- Owner: zqw
- Related:
  - ../master-spec.md
  - ../design/upm-packages-clickhouse-design.md

## 1. Purpose

This document describes the ClickHouse topology targeted by the project and clarifies MVP vs future production-grade expansion.

## 2. MVP Topology

```text
ClickHouse Cluster MVP

Keeper Ensemble
  - keeper-0
  - keeper-1
  - keeper-2

ClickHouse Server
  Shard 01
    - replica-01
    - replica-02
```

MVP topology:

- 3 Keeper nodes.
- 1 shard x 2 replicas.
- ReplicatedMergeTree backed by Keeper.
- Native TCP, HTTP, Interserver, and metrics endpoints.
- PodMonitor for Prometheus discovery.

This is a valid high-availability baseline for small production-like deployments because replica redundancy is present. It is not the final horizontal scaling topology.

## 3. Target Production Topology

Target future topology:

```text
ClickHouse Cluster Target

Keeper Ensemble
  - keeper-0
  - keeper-1
  - keeper-2

Shard 01
  - replica-01
  - replica-02

Shard 02
  - replica-01
  - replica-02

Future: N shards x M replicas
```

## 4. Replica vs Shard

| Concept | Purpose |
|---|---|
| Replica | High availability, read redundancy, replica recovery |
| Shard | Horizontal scaling, data distribution, capacity and query parallelism |
| Keeper | Coordination for ReplicatedMergeTree and distributed operations |
| Distributed table | Query/write routing layer across shards |

Increasing replicas within one shard is not the same as adding shards. Increasing UnitSet `units` without topology changes would produce more replicas in the same shard, not multiple shards.

## 5. Required Multi-shard Capabilities

Future N shards x M replicas requires:

- `topology.shards`;
- `topology.replicasPerShard`;
- dynamic `remote_servers` generation;
- dynamic `macros.shard` and `macros.replica`;
- Keeper path strategy;
- distributed DDL strategy;
- Distributed table strategy;
- write routing strategy;
- topology status aggregation;
- post-scale healthcheck.

## 6. MVP Limitations

MVP does not implement:

- automatic shard scale-out;
- data redistribution;
- automatic Distributed table rebuild;
- complete distributed DDL workflow;
- ClickHouse-specific compose CRD.

These are roadmap items, not exclusions from the project.

## 7. Open Design Points

| Topic | Current Position | Resolution Phase |
|---|---|---|
| Distributed table initialization | Resolved: Manager owns validation-only Distributed tables for healthcheck; production Manager may expose database management API/UI, while business table lifecycle remains DBA/application-owned | Phase 02 decision; Phase 04 validation implementation; production database API/UI later |
| Multi-shard write routing | Resolved: applications write/query through Distributed tables via a stable query service; direct local table writes are validation/admin-only | Phase 02 |
| DDL idempotency | Resolved: use idempotent SQL plus desired-state recording and drift verification; `IF NOT EXISTS` alone is not production-grade safety | Phase 02 decision; Phase 04+ implementation |
| ClickHouse topology CRD | Future productization | After MVP |

## 8. Production-grade Phase 02 Decisions

Distributed table ownership:

- Manager may create deterministic validation Distributed tables for Day1
  healthcheck.
- Manager must not silently create, alter, or drop user business tables.
- Future database management may be exposed through Manager APIs and UI
  workflows.
- User business local table and Distributed table lifecycle remains
  DBA/application-owned.
- Manager may provide DDL examples, topology guidance, validation tables, and
  read-only table metadata for diagnostics.

Write routing:

- Applications should use Distributed tables through a stable query service.
- Shard-local or replica-local endpoints remain operational/admin paths, not
  the default business write path.

DDL idempotency:

- Validation DDL must be repeatable.
- Manager-generated DDL is limited to database lifecycle and Manager-owned
  validation objects.
- Manager-generated DDL must combine idempotent syntax, target recording,
  checksum/version comparison, and drift failure for Manager-owned objects.
- Destructive schema changes require a future explicit approval workflow.
