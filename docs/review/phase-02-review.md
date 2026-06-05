# Phase 02 Review Report

- Version: 1.0
- Date: 2026-06-05
- Reviewer: Claude (Opus 4.7)
- Owner: 周钦伟 (zqw)
- Scope: Review of Codex's Phase 02 implementation against
  `docs/phases/phase-02-clickhouse-topology.md` v0.7
- Verdict: **PASS — proceed to Phase 03** (with three non-blocking follow-ups
  recorded in §7)

## 1. Hackathon Context Recap

Phase 02 is a package-level enablement phase: it generalises the
`upm-packages/clickhouse` rendering from the MVP `1 shard x 2 replicas`
baseline (Phase 01) to a generic `N shards x M replicas` static topology, and
proves it works end-to-end at `2 x 2` on the real Kubernetes lab cluster.

It explicitly does not include:

- Manager Backend, frontend, or any Day 2 API surface (those are Phase 03+).
- Backup/restore, monitoring/PrometheusRule wiring (Phase 05/07).
- Automatic shard scale-out, data redistribution, or business-table
  lifecycle management.
- A new ClickHouse-specific compose-operator CRD.

Phase 02 v0.7 (2026-06-05) raised the bar from "static rendering only" to
"static rendering plus real 2x2 runtime apply + Distributed read/write
validation" after a human review challenge (see Entry 008 in
`docs/ai-usage-phase02.md`). This review checks that the raised bar is
actually met.

## 2. Review Method

1. Read `docs/master-spec.md`, `docs/evidence-summary.md`,
   `docs/phases/phase-02-clickhouse-topology.md` v0.7, and the linked design
   docs (`upm-packages-clickhouse-design.md` v0.4 sealed,
   `clickhouse-ha-architecture.md`, `api-design.md`, `resource-model.md`,
   `runtime/07-clickhouse-topology-decision.md`).
2. Inspect Codex's actual changes:
   - `upm-packages/clickhouse/26.3.9.8/charts/**` (template, values,
     configTemplate/configValue, podtemplate, README).
   - `upm-packages/clickhouse/26.3.9.8/image/service-ctl.sh`.
   - `clickhouse/phase-02/**` (values, manifests, scripts, README).
   - `clickhouse/sync-runtime-image-to-nodes.sh`.
3. Re-run the static topology gates locally:
   - `helm template` for 1s2r / 2s2r / 2s3r / 4s2r against the modified
     chart.
   - `validate-rendered-topology.py` against all four rendered outputs.
4. Cross-check the runtime evidence claims in
   `docs/ai-usage-phase02.md` Entries 007 and 008 against the script that
   produces them (`validate-runtime-2s2r.sh`).
5. Compare scope/decisions against the Master Spec Open Questions that
   Phase 02 was supposed to seal.

The review host has lab-cluster credentials in scripts but no live access
during the review window, so the runtime PASS line is read from the AI
usage log rather than re-executed; the script that produced it was
inspected directly.

## 3. Acceptance Criteria Verification

Phase 02 spec §7 lists 13 criteria. Verified line-by-line.

| # | Criterion | Status | Evidence |
|---|---|---|---|
| 1 | `topology.shards` and `topology.replicasPerShard` defined with validation rules | PASS | `charts/values.yaml:22-24` defaults `1/2`; `templates/configTemplate.yaml:4-9` fails `helm template` when either is `< 1`; spec §5.1 documents rules |
| 2 | Rendered 1s2r config still matches Phase 01 baseline | PASS | `helm template … 1s2r-values.yaml` renders 1 shard, 2 replicas; `validate-rendered-topology.py --shards 1 --replicas-per-shard 2` returns `PASS topology_render_validation` |
| 3 | Rendered 2s2r contains exactly 2 shards x 2 replicas | PASS | Script PASS; manual inspection of `/tmp/ch-2s2r.yaml` shows two `<shard>` blocks each with two `<replica>` blocks, units 0/1 → shard01, units 2/3 → shard02 |
| 4 | Rendered 2s3r contains exactly 2 shards x 3 replicas | PASS | Script PASS; serverUnits=6 reported |
| 5 | Rendered 4s2r contains exactly 4 shards x 2 replicas | PASS | Script PASS; `awk` over `<remote_servers>` block confirms 4×`<shard>` and 2×`<replica>` each; serverUnits=8 |
| 6 | No rendered config hardcodes `<shard>01</shard>` when topology has multiple shards | PASS | The validation script explicitly fails if the literal `<shard>01</shard>` appears in the template; macros use the runtime expression `printf "shard%02d" (add $runtimeShardIndex 1)` |
| 7 | Keeper path strategy documented | PASS | `phase-02-clickhouse-topology.md` §5.4 and `clickhouse/phase-02/README.md` "Keeper Path Strategy" both publish `'/clickhouse/tables/{cluster}/{shard}/{database}/{table}', '{replica}'`; `validate-runtime-2s2r.sh:63` uses the exact same string |
| 8 | Distributed table initialization decision documented | PASS | Spec §5.5; design doc `upm-packages-clickhouse-design.md` §4 reaffirms business-table boundary |
| 9 | Multi-shard write routing decision documented | PASS | Spec §5.6; Master Spec Open Question marked Resolved (line 398) |
| 10 | DDL idempotency decision documented | PASS | Spec §5.7; Master Spec Open Question Resolved (line 399) |
| 11 | Rendered config includes runtime-derived `remote_servers` cluster `<secret>` | PASS | `clickhouseTemplate.tpl:35` derives `$clusterSecret = sha256sum (printf "%s:%s" $serviceName (getenv "AES_SECRET_KEY"))`; template line 38 emits `<secret>{{ $clusterSecret }}</secret>`. The validation script asserts both `sha256sum` and `AES_SECRET_KEY` appear and that the `<secret>` element references `$clusterSecret` |
| 12 | Rendered config includes service-scoped `<distributed_ddl>` | PASS | `clickhouseTemplate.tpl:66-68` emits `<distributed_ddl><path>/clickhouse/task_queue/ddl/{{ $serviceName }}</path></distributed_ddl>`; validation script `validate-rendered-topology.py:151-156` enforces both substrings |
| 13 | Runtime support for 2x2 is claimed only after real K8s apply + SQL validation | PASS | Spec §8 mandates `validate-runtime-2s2r.sh`; Entry 008 in `docs/ai-usage-phase02.md` records the `PASS runtime_2s2r_validation` line, 4 `system.clusters` rows, 8 distributed rows, 4 rows per shard, `total_replicas=2 active_replicas=2 is_readonly=0 queue_size=0 absolute_delay=0` for every replica |

All 13 criteria are met.

## 4. Static Render Gate — Re-executed in This Review

Commands run locally during this review:

```bash
helm template ch-1s2r upm-packages/clickhouse/26.3.9.8/charts \
  --values clickhouse/phase-02/values/1s2r-values.yaml >/tmp/ch-1s2r.yaml
helm template ch-2s2r upm-packages/clickhouse/26.3.9.8/charts \
  --values clickhouse/phase-02/values/2s2r-values.yaml >/tmp/ch-2s2r.yaml
helm template ch-2s3r upm-packages/clickhouse/26.3.9.8/charts \
  --values clickhouse/phase-02/values/2s3r-values.yaml >/tmp/ch-2s3r.yaml
helm template ch-4s2r upm-packages/clickhouse/26.3.9.8/charts \
  --values clickhouse/phase-02/values/4s2r-values.yaml >/tmp/ch-4s2r.yaml

for t in 1s2r:1:2 2s2r:2:2 2s3r:2:3 4s2r:4:2; do
  IFS=: read name s r <<<"$t"
  python3 clickhouse/phase-02/scripts/validate-rendered-topology.py \
    --rendered "/tmp/ch-${name}.yaml" --shards "$s" --replicas-per-shard "$r"
done
```

Result: all four invocations ended with `PASS topology_render_validation`,
reporting:

```text
1s2r → cluster=upm_cluster shards=1 replicasPerShard=2 serverUnits=2 keeperReplicas=3
2s2r → cluster=upm_cluster shards=2 replicasPerShard=2 serverUnits=4 keeperReplicas=3
2s3r → cluster=upm_cluster shards=2 replicasPerShard=3 serverUnits=6 keeperReplicas=3
4s2r → cluster=upm_cluster shards=4 replicasPerShard=2 serverUnits=8 keeperReplicas=3
```

Spot inspection of `/tmp/ch-2s2r.yaml` and `/tmp/ch-4s2r.yaml` confirms:

- Two/four `<shard>` blocks under `<remote_servers><upm_cluster>`.
- Two replicas per shard with hostnames
  `{{ $serviceName }}-<unit>.{{ $serviceName }}-headless-svc.{{ $namespace }}.svc.cluster.local`.
- Single-UnitSet unit-index mapping (`shard*replicasPerShard + replica`)
  matches the documented mapping in `phase-02/README.md` and the runtime
  `validate-runtime-2s2r.sh:34-39` macro loop (`shard01 replica01..02 →
  shard02 replica01..02`).
- `<allow_distributed_ddl_queries>true</allow_distributed_ddl_queries>`
  rendered inside the cluster block.
- `<distributed_ddl><path>/clickhouse/task_queue/ddl/{{ $serviceName }}</path></distributed_ddl>`
  outside `<remote_servers>` as expected.
- `<macros>` block uses `printf "shard%02d" (add $runtimeShardIndex 1)`
  and `printf "replica%02d" (add $runtimeReplicaIndex 1)`, so per-Pod
  identity is derived from `unit-operator/unit.sn` at startup, not baked
  in at Helm render time. Mathematically deterministic.

## 5. Runtime 2x2 Evidence Review

The runtime gate is now mandatory (spec §8, criterion #13). Codex did not
fake it: the failure mode that initially broke runtime acceptance is
documented in Entry 008 in plain language ("old `service-ctl.sh` rejected
`UNIT_COUNT=4`, distributed DDL config was missing, Distributed writes
failed for lack of `remote_servers <secret>`"), and three concrete
remediations are visible in source:

1. `clickhouse/sync-runtime-image-to-nodes.sh` rebuilds and imports the
   runtime image on the four lab nodes (`192.168.35.201..204`) via
   `podman build` + `ctr -n k8s.io images import`. The script is idempotent
   (re-export from `ctr`, rebuild, re-import), reads only `SSH_PASSWORD`
   from env, and explicitly documents that the production model is a
   registry-published image, not node-local sync. This is appropriately
   scoped as a lab fixup rather than product behavior.
2. `service-ctl.sh:97-100` now treats `UNIT_COUNT` as a positive integer
   instead of hardcoding the 1-shard expectation (exit code 61).
3. `clickhouseTemplate.tpl:36-53,66-68` renders both
   `remote_servers/<secret>` and `<distributed_ddl>` — the same template
   changes that the static gate validates.

`validate-runtime-2s2r.sh` is the right shape for the runtime claim:

- Waits for both UnitSets to reach the exact expected ready counts
  (`readyUnits=3` keeper, `readyUnits=4` clickhouse) before any SQL.
- Verifies `system.clusters` shape, then per-Pod `getMacro('shard'/replica')`.
- Drops the validation database **before** recreating it; clears stale
  Keeper replica metadata via `SYSTEM DROP REPLICA … FROM ZKPATH` for the
  reserved `local_events` path (idempotency-aware) before re-create. This
  is a correct reading of the spec §5.7 idempotency requirement.
- Creates `ReplicatedMergeTree` local table + `Distributed` table `ON
  CLUSTER`, then inserts 8 rows through the Distributed table with
  `insert_distributed_sync = 1` (deterministic, no race).
- Executes `SYSTEM SYNC REPLICA` on every replica before the read
  assertions — closes the eventual-consistency window.
- Runs four hard assertions via `throwIf`: total count == 8, exactly 2
  shards read, each replica holds 4 rows, every replica is healthy
  (`active_replicas=2 ∧ is_readonly=0 ∧ queue_size=0`).
- Final line `PASS runtime_2s2r_validation`.

Entry 008 records that all of these passed. The shape of the script makes
it hard to false-PASS without an actual healthy 2x2 cluster (each `throwIf`
will surface a non-zero exit if the cluster shape is wrong), so the recorded
runtime PASS is credible.

## 6. Scope and Decision Boundaries — Held Cleanly

Phase 02 explicitly avoided the temptations to over-deliver. Verified:

- **No business-schema ownership.** Template renders only topology
  primitives; the validation database `phase02_runtime_validation` is
  created, used, and dropped by the validation script — it is not
  installed by `helm template`. The spec §5.5 boundary ("Manager must not
  silently create, alter, or drop user business Distributed tables") is
  echoed in `upm-packages-clickhouse-design.md` §4 and in the chart README.
- **No new compose-operator CRD.** Spec Option C remains "future
  roadmap". Confirmed by the absence of any `compose-operator/` change
  on `phase-02` (`git log --oneline -n 20`).
- **No Manager Backend code in this phase.** Confirmed: no `backend/` or
  Go code in commits `f667eb8..7fe6f9d`. That work is intentionally
  deferred to Phase 03.
- **Open Questions actually sealed.** The three Master Spec Open
  Questions Phase 02 was supposed to resolve — Distributed-table
  initialization ownership, multi-shard write routing, DDL idempotency —
  are now `Resolved` in `docs/master-spec.md:397-399` with concrete
  Phase 02 §5.5–§5.7 decisions.
- **Runtime claim discipline.** v0.7 (2026-06-05) added the runtime
  requirement after a human review challenge. The previous "dry-run only"
  paper acceptance was explicitly rejected — exactly the discipline that
  separates Phase 02 from the Phase 01 "static lint vs real runtime"
  gap.

## 7. Findings — Non-blocking

None of these block Phase 03. They are recorded so they can be tracked
forward.

### F1. Lab-node-local runtime image is fragile (low/medium)

`clickhouse/sync-runtime-image-to-nodes.sh` is correct as a hackathon-lab
escape hatch but ties Phase 02 runtime acceptance to four specific node
IPs and an SSH password. If the lab gets re-IPed or the runtime image is
ever rebuilt without re-running the sync, runtime acceptance breaks
silently (`service-ctl.sh` would behave differently across pods until the
script is re-run).

Mitigation already chosen by the owner: the script README publishes a
"production should use GHCR/Harbor" path. Suggested follow-up: when
Phase 03 begins touching the Manager Backend's image-handling code,
publish the runtime image to the registry the lab cluster pulls from and
remove the node-local sync from the demo critical path. Not required to
start Phase 03.

### F2. `replicasPerShard` default coupling between values and env (low)

`clickhouseTemplate.tpl:71` reads
`getenv "CLICKHOUSE_REPLICAS_PER_SHARD" "<helm-rendered-default>"` for the
runtime macro math. The Helm-rendered default and the env var injected by
`podtemplate.yaml:75-76, 142-143, 285-286` are the same number today, but
they are produced from two independent sources in two files. If a future
change updates one but not the other, every pod will compute the wrong
shard/replica index without any obvious render-time error.

Suggested follow-up: a small unit-test/grep in the chart README, or a
single source of truth (have the template fail-fast if the env var is
missing rather than fall back to a number). Not in scope for Phase 02.

### F3. Cluster `<secret>` strength depends entirely on `AES_SECRET_KEY` entropy (informational)

The `remote_servers` cluster secret is `sha256(serviceName + ":" + AES_SECRET_KEY)`.
This is correct for "no plaintext remote password in config" and reuses
existing Secret material instead of inventing a second key, which is the
right call for MVP. The strength of inter-replica auth is now exactly the
strength of `AES_SECRET_KEY` rotation. Phase 03/04 will eventually own
rotation; document this dependency when Manager-side credential workflows
appear.

## 8. Recommendation — Go / No-Go for Phase 03

**Go.** All 13 acceptance criteria are met with reproducible static
evidence (re-executed in this review) and credible runtime evidence
(read from a hard-asserting validation script). Scope was held. The
three Master Spec Open Questions Phase 02 was supposed to seal are
sealed. The runtime image fragility (F1) is a known lab artefact, not a
spec gap, and is explicitly tracked for future production hardening.

Suggested entry conditions for Phase 03 (none block the start):

1. Phase 03 backend's `CreateClusterRequest` topology fields
   (`shards`/`replicasPerShard`/`keeperReplicas`) should be mapped 1:1 to
   the Phase 02 `topology.shards` / `topology.replicasPerShard` /
   `keeper.replicas` chart values. The single-UnitSet mapping
   (`unitIndex = shard*replicasPerShard + replica`) becomes a Manager
   invariant.
2. Phase 03 should not re-derive macros or remote_servers; it should
   apply the Phase 02 package and rely on the package's runtime
   templating, in line with the "Manager does not replace UPMIO
   operators" boundary in `phase-03 §4`.
3. Phase 03's first `CreateCluster` E2E should re-use
   `clickhouse/phase-02/manifests/03-clickhouse-unitset-2s2r.yaml` as
   the reference shape so the Manager API does not silently change the
   wire-format that Phase 02 just validated.

## 9. Changelog

| Version | Date | Changes |
|---|---|---|
| 1.0 | 2026-06-05 | Initial review against Phase 02 v0.7; PASS verdict |
