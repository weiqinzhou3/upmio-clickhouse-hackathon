# Phase 02: ClickHouse Topology Parameterization

- Version: 0.5
- Date: 2026-06-02
- Status: Confirmed
- Priority: P1
- Owner: zqw
- Depends on:
  - ../master-spec.md
  - ../evidence-summary.md
  - ../architecture/clickhouse-ha-architecture.md
  - ../design/upm-packages-clickhouse-design.md
  - ../design/resource-model.md
  - ../design/api-design.md
  - ../runtime/07-clickhouse-topology-decision.md

## 1. Purpose

Extend the ClickHouse package/topology design from the MVP 1 shard x 2 replicas baseline toward generic `N shards x M replicas` static topology generation.

This phase must not implement automatic resharding or data redistribution. It designs and validates topology rendering for new clusters.

## 2. Scope

In scope:

1. Define `topology.shards` and `topology.replicasPerShard` values.
2. Render `remote_servers` for multiple shards and replicas.
3. Render per-instance `macros.shard` and `macros.replica`.
4. Define Keeper path strategy for `ReplicatedMergeTree`.
5. Define service naming conventions for per-unit and cluster access.
6. Decide how Distributed tables are initialized or deferred.
7. Decide multi-shard write routing exposure for MVP/Future.
8. Define DDL idempotency strategy for generated SQL/tasks.
9. Provide example values for 1s2r, 2s2r, 2s3r, and 4s2r.

## 3. Non-Goals

Out of scope:

- Automatic shard scale-out with data movement.
- Historical data redistribution.
- Production-grade resharding workflow.
- New ClickHouse compose-operator CRD.
- Full frontend implementation.
- Backup/restore implementation.

## 4. Topology Design Options

### Option A: Single UnitSet for all ClickHouse Server instances

Model:

```text
UnitSet clickhouse-runtime units = shards * replicasPerShard
unit index -> shard/replica mapping
0 -> shard01 replica01
1 -> shard01 replica02
2 -> shard02 replica01
3 -> shard02 replica02
```

Pros:

- Minimal UPMIO object count.
- Consistent with current package model.
- Easier MVP extension from existing 1s2r runtime evidence.

Cons:

- Template must compute shard/replica from unit index.
- Per-shard lifecycle is less explicit.
- Future per-shard scaling may be harder.

Recommendation:

- Use this as the first package-level topology implementation if it can be rendered cleanly.

### Option B: One UnitSet per shard

Model:

```text
UnitSet clickhouse-shard01 units = replicasPerShard
UnitSet clickhouse-shard02 units = replicasPerShard
```

Pros:

- Shard identity is explicit.
- Per-shard lifecycle is clearer.
- Easier future shard-level operations.

Cons:

- Cross-UnitSet `remote_servers` generation becomes more complex.
- Manager or future topology controller must coordinate multiple UnitSets.
- Higher object count and operational complexity.

Recommendation:

- Keep as future productization option if single-UnitSet topology becomes too limiting.

### Option C: ClickHouse-specific topology CRD

Model:

```text
ClickHouseCluster / ClickHouseTopology CRD
  -> reconciles Keeper UnitSet + ClickHouse UnitSets + config + status
```

Pros:

- Cleanest long-term product model.
- Can own topology status and reconciliation.

Cons:

- Requires compose-operator/API/controller work.
- Too high-risk for MVP.

Recommendation:

- Do not implement in MVP. Keep in roadmap.

## 5. Required Technical Requirements

### 5.1 Values schema

Required fields:

```yaml
topology:
  shards: 2
  replicasPerShard: 2
cluster:
  name: upm_cluster
keeper:
  replicas: 3
```

Validation rules:

- `shards >= 1`
- `replicasPerShard >= 1`
- total server units = `shards * replicasPerShard`
- MVP-supported runtime baseline remains `1 x 2`
- 2x2 and above require static rendering validation before runtime claim

### 5.2 remote_servers rendering

Rendered config must contain exactly:

```text
number of <shard> blocks = topology.shards
number of <replica> blocks per shard = topology.replicasPerShard
```

Hostnames must follow the selected UnitSet/service strategy and be stable across restart.

### 5.3 macros rendering

Each ClickHouse instance must render:

```xml
<macros>
  <shard>shardXX</shard>
  <replica>replicaYY or POD_NAME-derived stable replica</replica>
</macros>
```

The mapping must be deterministic.

### 5.4 Keeper path strategy

Replicated tables must use shard-aware Keeper paths such as:

```text
/clickhouse/tables/{cluster}/{shard}/{database}/{table}
```

The exact path must be documented and used consistently by examples and healthcheck SQL.

### 5.5 Distributed table initialization decision

Decision:

- MVP / hackathon stage:
  - Manager may initialize only deterministic validation Distributed tables used
    by healthcheck.
  - Manager must not automatically create, alter, or drop user business
    Distributed tables during cluster creation.
  - Package topology rendering must not imply ownership of business schema.
- Future production behavior:
  - Manager should expose controlled database management APIs and UI workflows.
  - User business local table and Distributed table lifecycle remains
    DBA/application-owned and is not a Manager product responsibility.
  - Manager may provide table DDL examples, topology/write-routing guidance, and
    read-only metadata/diagnostics, but must not create, alter, or drop business
    tables.
  - Manager-owned validation objects must use reserved names and must be
    idempotent, drift-checked, and safe to recreate.

Rationale:

Distributed tables encode business schema and sharding semantics. Production
Manager should manage cluster/database operations and validation objects, not
take over application table lifecycle.

### 5.6 Multi-shard write routing decision

Decision:

- Applications should write and query through explicitly defined Distributed
  tables.
- Applications should connect through a stable ClickHouse query service rather
  than hard-coding individual Pod or shard endpoints.
- Direct writes to local `ReplicatedMergeTree` tables are reserved for
  controlled validation, diagnostics, bootstrap, or DBA-admin workflows.
- The Manager should expose topology/service guidance and health evidence; it
  should not hide business write-routing decisions behind implicit table
  creation.

Rationale:

In multi-shard ClickHouse, local table writes bypass cluster routing. A stable
query service plus Distributed tables gives applications an explicit routing
contract while keeping shard-local endpoints available for operations.

### 5.7 DDL idempotency decision

Decision:

- Validation SQL must be idempotent and deterministic.
- Use `IF NOT EXISTS` where ClickHouse supports it.
- `IF NOT EXISTS` alone is not sufficient for production-grade safety.
- Manager-generated DDL is limited to database lifecycle and Manager-owned
  validation objects.
- Manager-generated DDL must be treated as desired state:
  - record target cluster, database, validation object name, engine, and
    checksum/version in a future Manager/audit store or approved Kubernetes
    metadata path;
  - compare existing Manager-owned object definitions before applying DDL;
  - fail with a drift error if an existing Manager-owned object differs from
    the requested definition;
  - avoid destructive changes unless a future explicit approval workflow exists.
- `ON CLUSTER` may be used only when distributed DDL configuration is rendered
  and validated. Without distributed DDL, Manager must use controlled
  per-instance execution and verify convergence.

Rationale:

Production DDL safety requires both idempotent syntax and drift detection.
Blindly using `IF NOT EXISTS` can hide schema mismatches and produce false
success.

## 6. Files Likely Changed

```text
upm-packages/clickhouse/<version>/charts/values.yaml
upm-packages/clickhouse/<version>/charts/files/clickhouseTemplate.tpl
upm-packages/clickhouse/<version>/README.md
examples/clickhouse/1s2r-values.yaml
examples/clickhouse/2s2r-values.yaml
examples/clickhouse/2s3r-values.yaml
examples/clickhouse/4s2r-values.yaml
docs/design/upm-packages-clickhouse-design.md
docs/architecture/clickhouse-ha-architecture.md
docs/design/api-design.md
docs/design/product-design.md
```

## 7. Acceptance Criteria

1. `topology.shards` and `topology.replicasPerShard` are defined in package values or design with clear validation rules.
2. Rendered 1s2r config still matches the Phase 01 executable baseline.
3. Rendered 2s2r config contains exactly 2 shard blocks and 2 replicas per shard.
4. Rendered 2s3r config contains exactly 2 shard blocks and 3 replicas per shard.
5. Rendered 4s2r config contains exactly 4 shard blocks and 2 replicas per shard.
6. No rendered config hardcodes `<shard>01</shard>` for all instances when topology requires multiple shards.
7. Keeper path strategy is documented.
8. Distributed table initialization decision is documented in this phase and linked from Open Questions if still not implemented.
9. Multi-shard write routing decision is documented.
10. DDL idempotency decision is documented.
11. Runtime support is not claimed for 2x2 unless actually validated.

## 8. Verification Commands

```bash
# Render topology examples
helm template ch-1s2r upm-packages/clickhouse/<version>/charts --values examples/clickhouse/1s2r-values.yaml >/tmp/ch-1s2r.yaml
helm template ch-2s2r upm-packages/clickhouse/<version>/charts --values examples/clickhouse/2s2r-values.yaml >/tmp/ch-2s2r.yaml
helm template ch-2s3r upm-packages/clickhouse/<version>/charts --values examples/clickhouse/2s3r-values.yaml >/tmp/ch-2s3r.yaml
helm template ch-4s2r upm-packages/clickhouse/<version>/charts --values examples/clickhouse/4s2r-values.yaml >/tmp/ch-4s2r.yaml

# Static topology checks
python3 scripts/validate_clickhouse_topology.py --rendered /tmp/ch-2s2r.yaml --shards 2 --replicas-per-shard 2
python3 scripts/validate_clickhouse_topology.py --rendered /tmp/ch-2s3r.yaml --shards 2 --replicas-per-shard 3
python3 scripts/validate_clickhouse_topology.py --rendered /tmp/ch-4s2r.yaml --shards 4 --replicas-per-shard 2

# Kubernetes dry-run for rendered resources
kubectl apply --dry-run=server -f /tmp/ch-2s2r.yaml
```

If a validation script does not exist yet, create a small deterministic script or document equivalent grep/xmllint checks.

## 9. Risks and Open Questions

| Risk / Question | Handling |
|---|---|
| Single UnitSet becomes hard to scale by shard | Keep Option B and future CRD path documented |
| Distributed DDL is not configured | Phase 02 documents the rule: use `ON CLUSTER` only after distributed DDL is rendered and validated; otherwise use controlled per-instance execution for validation objects |
| Multi-shard runtime testing exceeds hackathon time | Static rendering is acceptable for Phase 02 unless user mandates runtime 2x2 |
| Application write routing requires business schema knowledge | Manager provides validation tables and guidance; user business local/Distributed table lifecycle remains DBA/application-owned |

## 10. Changelog

| Version | Date | Changes |
|---|---|---|
| 0.1 | 2026-05-27 | Initial phase draft |
| 0.2 | 2026-05-27 | Added topology design options and acceptance criteria |
| 0.3 | 2026-05-27 | Added metadata and red-team fix structure |
| 0.4 | 2026-05-27 | Restored topology option comparison, multi-shard requirements, Open Question closure obligations, and objective verification |
| 0.5 | 2026-06-02 | Sealed production-grade decisions for Distributed table ownership, write routing, and DDL idempotency before Phase 02 implementation |
