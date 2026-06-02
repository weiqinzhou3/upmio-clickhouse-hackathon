# Phase 01 Review Report

- Version: 1.0
- Date: 2026-06-02
- Reviewer: Claude (Opus 4.7)
- Owner: 周钦伟 (zqw)
- Scope: Review of Codex's Phase 01 implementation against
  `docs/phases/phase-01-upm-packages-clickhouse.md`
- Verdict: **PASS — proceed to Phase 02** (with follow-up cleanup before
  Phase 02 commits, see §8)

## 1. Hackathon Context Recap

Hackathon goal (per `exam/BSG AI编程黑客松活动介绍.pdf`): build a ClickHouse
operations management product on top of the UPMIO operator stack
(`unit-operator`, `compose-operator`, `upm-packages`), with phase-spec-driven
development as the methodology. The hackathon's first stage produced a Master
Spec and phase plan; the second stage is the coding/demo stage.

zqw's Phase 01 score (per `exam/Phase01_成绩排名报告_递交版(1).pdf`): **85.0 /
100, ranked #2 of 7**.

| Dimension | Weight | Score | Notes from review |
|---|---:|---:|---|
| A. Master Spec quality | 30% | 9.0 | Evidence-based, AI-consumable, sealed decisions |
| B. Requirement understanding | 25% | 8.0 | Solid Day1/Day2 coverage; GrpcCall gap honestly recorded |
| C. AI tool effectiveness | 20% | 8.5 | Multi-tool workflow with runtime-evidence-first discipline |
| D. Design / architecture | 15% | 8.0 | Strong baseline; room to grow on UX/prototype assets |
| E. Phase planning | 10% | 9.0 | 8 phases with 10–13 verifiable acceptance criteria each |

Two practical gaps were called out for stage two: (1) ClickHouse `GrpcCall`
fallback path needed to be concrete, and (2) the metrics endpoint /
PodMonitor / Prometheus chain needed end-to-end validation. Both are tracked
in `docs/review/phase01-score-response.md` and routed to Phase 07 and Phase 05
respectively. **Phase 01 (this review) is not on the critical path for those
two repairs**; this review only assesses the package-foundation work.

## 2. What Codex Was Asked to Do

`docs/phases/phase-01-upm-packages-clickhouse.md` §1–§3 defines the phase:

> Make the ClickHouse and ClickHouse Keeper packages repeatably deployable
> through UPMIO without manual ConfigMap patching, producing the first
> executable baseline:
>
> - 3-node ClickHouse Keeper,
> - 1 shard × 2 replicas ClickHouse Server,
> - Secret-based password initialization,
> - native ClickHouse Prometheus endpoint,
> - UPMIO `UnitSet`-based lifecycle.

Out of scope: Manager Backend, multi-shard topology, new compose CRD,
GrpcCall, full Grafana / PrometheusRule, host-level ClickHouse, `<no_password/>`.

13 acceptance criteria are listed in §6 of the phase spec.

## 3. Review Method

1. Read `docs/master-spec.md`, `docs/evidence-summary.md`,
   `docs/phases/phase-01-upm-packages-clickhouse.md`, and the design docs
   referenced from the phase spec.
2. Inspect Codex's actual changes in three places:
   - `upm-packages/` working tree (uncommitted modifications),
   - `clickhouse/phase-01/` (phase-facing deliverables and runtime evidence),
   - `unit-operator/examples/unitsets/` (canonical example files).
3. Run the recommended quality gates locally: `helm dependency update`,
   `helm lint`, `helm template`, and static greps against the rendered
   output (the runtime kubectl checks could not be re-executed because the
   review host has no cluster access).
4. Compare Codex's Secret-handling pattern against established UPM patterns
   in `mysql-community`, `redis`, `kibana`, `minio`, `proxysql`.
5. Cross-check Codex's runtime claims in
   `clickhouse/phase-01/runtime-validation.md` against the runtime evidence
   trail in `docs/runtime/04` and `docs/runtime/05`.

## 4. What Codex Produced

### 4.1 Source modifications in `upm-packages/clickhouse/26.3.9.8`

Four files modified (working tree, **not yet committed**):

| File | Change | Aligns with phase spec |
|---|---|---|
| `charts/files/clickhouseTemplate.tpl` | Added `<prometheus>` block on port 9363 / `/metrics`; rendered `password_sha256_hex` for `default` and a new `admin` user via `AESCTRDecrypt (secretRead …)`; restricted `default` to loopback; removed `max_concurrent_queries` | §4.1 password, §4.2 invalid setting, §4.4 metrics |
| `charts/files/clickhouseParametersDetail.json` | Removed `max_concurrent_queries` entry | §4.2 |
| `charts/files/clickhouseValue.yaml` | Removed `max_concurrent_queries` default | §4.2 |
| `image/service-ctl.sh` | Added `decrypt_secret`, `admin_password`, `xml_escape`, `write_client_config`; rewrote `health` and `login` to authenticate using the AES-CTR-encrypted admin password from `SECRET_MOUNT`; added new exit codes (41, 51, 52) | §4.1 password, §4.3 runtime mounts |

Diff stats: `+104 / −17` lines across the four files.

### 4.2 Phase-facing assets in `clickhouse/phase-01/`

| Path | Purpose |
|---|---|
| `README.md` | Operator-runbook: chart-render, Secret creation with AES-CTR encryption, UnitSet apply, automated acceptance script invocation, manual SQL checks |
| `runtime-validation.md` | Real cluster run dated 2026-06-02 on 192.168.35.201–204, result `PASS`, summary of every required check |
| `manifests/00-namespace-project.yaml` | Namespace + UPMIO `Project` |
| `manifests/01-runtime-secret.example.yaml` | **Placeholder-only** Secret template (no real cipher) |
| `manifests/02-clickhouse-keeper-unitset.yaml` | 3-unit Keeper UnitSet with `SECRET_MOUNT`/`AES_SECRET_KEY` env, extraVolume, log emptyDir |
| `manifests/03-clickhouse-unitset.yaml` | 2-unit ClickHouse UnitSet with full env set (SECRET_NAME / CLICKHOUSE_ADMIN_SECRET_NAME / CLICKHOUSE_ADMIN_PASSWORD_KEY / CLICKHOUSE_ADMIN_USER / CLICKHOUSE_METRICS_PORT / CLICKHOUSE_METRICS_PATH), extraVolume, `podMonitor.enable: true` |
| `values/clickhouse-values.yaml`, `values/clickhouse-keeper-values.yaml` | Helm values for chart rendering |
| `scripts/validate-runtime.sh` | 96-line acceptance script: UnitSet ready, PodTemplate image, in-pod config grep (prometheus/no_password/max_concurrent/sha256), Keeper ruok+mntr leader/follower count, `service-ctl.sh health`, no-password rejection, `system.clusters` row count, replicated read on both pods, `system.replicas` sums, metrics endpoint check; exits `PASS phase01_runtime_validation` |
| `modified-files/` | Symlinks back to the 4 modified package source files (review-aid only) |

### 4.3 What was deliberately NOT changed

- No Manager Backend code (correct: out of scope per §3).
- No new compose-operator CRD (correct: out of scope).
- No GrpcCall integration (correct: out of scope).
- No Grafana dashboards or PrometheusRule (correct: out of scope).
- No changes to `unit-operator/examples/unitsets/clickhouse-unitset.yaml` or
  `unit-operator/examples/unitsets/clickhouse-keeper-unitset.yaml` (see §8.2:
  this is a follow-up gap, not a phase-01 blocker).

## 5. Acceptance Criteria — Line-by-Line Verdict

Acceptance criteria are from `docs/phases/phase-01-upm-packages-clickhouse.md`
§6. Evidence column lists whether the verdict comes from this review's local
re-run (`helm template` of the current working tree) or from Codex's recorded
runtime run.

| # | Criterion | Verdict | Evidence |
|---|---|---|---|
| 1 | Rendered config contains no `<no_password/>` | PASS | Local re-run: `grep -c '<no_password' /tmp/ph01-rendered/clickhouse.yaml` = `0`. Runtime in-pod check `no_password=absent` in `runtime-validation.md` |
| 2 | Rendered config does not include `max_concurrent_queries` | PASS | Local re-run: count = `0`. In-pod grep: `max_concurrent_queries=absent` |
| 3 | Rendered config includes password-based auth sourced from Secret placeholders | PASS | Local re-run: `password_sha256_hex` × 2 (`default`+`admin`), template body contains `AESCTRDecrypt (secretRead …)`. In-pod: `password_sha256_hex=present` |
| 4 | Rendered config includes native Prometheus endpoint configuration | PASS | Local re-run: `<prometheus>` × 1, port `9363` × 2 (template + container port). In-pod: `prometheus-config=present` |
| 5 | Keeper `UnitSet` reaches 3/3/3 | PASS (runtime) | `runtime-validation.md`: `clickhouse-runtime-keeper expected/current/ready = 3/3/3` and all 3 pods ready 2/2 |
| 6 | ClickHouse Server `UnitSet` reaches 2/2/2 | PASS (runtime) | `runtime-validation.md`: `clickhouse-runtime expected/current/ready = 2/2/2`, pods `clickhouse-runtime-0/1` both ready 2/2 |
| 7 | `SELECT 1` succeeds from at least one ClickHouse pod | PASS (runtime) | `service-ctl.sh health=pass`, `SELECT 1 AS ok -> 1`, no-password login rejected |
| 8 | `system.clusters` shows `upm_cluster` with 1 shard 2 replicas | PASS (runtime) | `runtime-validation.md` shows shard=1 replica=1/2 with correct headless DNS hostnames |
| 9 | `system.replicas` for runtime validation table is healthy | PASS (runtime) | `is_readonly=0, is_session_expired=0, absolute_delay=0, queue_size=0` |
| 10 | Keeper `ruok->imok` and `mntr` shows 1 leader / 2 followers | PASS (runtime) | `keeper-0/1=follower, keeper-2=leader`, all `ruok=imok` |
| 11 | `curl http://127.0.0.1:9363/metrics` returns Prometheus metrics | PASS (runtime) | `ClickHouse_Info{name="ClickHouse",version="26.3.9.8",…}` returned. Note: closes the monitoring gap that runtime evidence file `docs/runtime/05` originally recorded as `Connection refused` |
| 12 | No real secrets in generated files / logs / reports / commits | PASS | `manifests/01-runtime-secret.example.yaml` contains only `REPLACE_WITH_*` placeholders. Secret-generation flow runs locally on the K8s node and is documented as "not committed" in `runtime-validation.md` §Notes. No real cipher text in any of the committed artifacts |
| 13 | Runtime no longer requires manual ConfigMap patching | PASS | `runtime-validation.md` shows direct apply succeeds; old patches in `docs/runtime/04` (`perl -0pi -e` on the ConfigMap) are no longer needed because the template ships the fixes |

**13/13 acceptance criteria pass with documented evidence.**

## 6. Local Quality-Gate Re-run

Run on the review host on 2026-06-02 against the working tree:

```bash
cd upm-packages
helm dependency update clickhouse/26.3.9.8/charts          # bitnami common 2.31.3 pulled
helm dependency update clickhouse-keeper/26.3.9.8/charts   # bitnami common 2.31.3 pulled
helm lint clickhouse/26.3.9.8/charts                       # 1 chart(s) linted, 0 failed
helm lint clickhouse-keeper/26.3.9.8/charts                # 1 chart(s) linted, 0 failed
helm template test-clickhouse clickhouse/26.3.9.8/charts        > /tmp/ph01-rendered/clickhouse.yaml
helm template test-clickhouse-keeper clickhouse-keeper/26.3.9.8/charts > /tmp/ph01-rendered/keeper.yaml
bash scripts/validate-clickhouse-packages.sh   # all 7 non-helm gates PASS
```

Static counts in `/tmp/ph01-rendered/clickhouse.yaml`:

| Pattern | Count | Expectation |
|---|---:|---|
| `<no_password` | 0 | must be 0 |
| `max_concurrent_queries` | 0 | must be 0 |
| `<prometheus>` | 1 | must be ≥1 |
| `9363` | 2 | container port + template port |
| `password_sha256_hex` | 2 | `default` + `admin` user |
| `AESCTRDecrypt` | 1 | template uses AES decrypt |
| `secretRead` | 1 | template reads Secret |
| `name: metrics` | 1 | metrics port exposed on container |
| `SECRET_MOUNT` | 0 | **expected at package level but absent** (see §7.2) |
| `AES_SECRET_KEY` | 0 | **expected at package level but absent** (see §7.2) |

`bash scripts/validate-clickhouse-packages.sh` reports all 7 non-helm
sub-gates `PASS` (structure / versions / metadata / templates / docs / agent /
manager). The script's `validate_helm` sub-gate was failing in its current
form because it does not call `helm dependency update` before lint — a script
nit, not a package defect.

## 7. Findings

### 7.1 Strong points

1. **Real runtime evidence, dated, on a real cluster.** Codex did not stop
   at "helm template renders without error." It re-deployed against the
   four-node K8s cluster, rebuilt the image (`localhost/upmio/clickhouse:
   26.3.9.8-runtime`) so the new `service-ctl.sh` ships with it, ran the
   automated acceptance script, and recorded all results. This matches the
   "runtime evidence overrides source-code assumption" decision bias in
   `master-spec.md` §12.
2. **No-password login is now actively rejected at runtime.** This is the
   harder half of the security baseline — not just "we removed `<no_password/>`
   from the template" but "we proved that an unauthenticated `clickhouse-client
   --query 'SELECT 1'` fails." That belt-and-suspenders posture closes a class
   of regressions.
3. **Monitoring endpoint gap from `docs/runtime/05` is now closed.** The
   original validation showed `Connection refused` on 9363 because the
   template did not emit `<prometheus>`. The new template does, and the
   runtime check confirms `ClickHouse_Info` is served. This unblocks Phase 05.
4. **AES-CTR encoding convention is correctly mirrored.** The
   `decrypt_secret` shell function reads a 16-byte IV prefix and decrypts the
   tail with `openssl enc -aes-256-ctr -K $key_hex -iv $iv_hex`, matching what
   the helm template function `AESCTRDecrypt` expects on the rendering side.
   The Secret-creation snippet in `README.md` produces the same on-disk
   format.
5. **Scope discipline is good.** No Manager code, no compose CRD, no
   GrpcCall, no Grafana — exactly the Phase 01 non-goals.
6. **Acceptance script is the right shape.** `validate-runtime.sh` is
   idempotent, exits non-zero on first failure, and prints a single
   machine-grep-able final line (`PASS phase01_runtime_validation`). Future
   re-runs will be cheap, which matters because Phase 03+ work will keep
   touching the same baseline.

### 7.2 Gaps / risks (none blocking phase exit)

1. **Codex's `upm-packages` modifications are still in the working tree —
   not committed.** `AGENTS.md` §7 requires one branch per phase and commit
   messages of the form `phase-XX: concise description`. Until those four
   files are committed, the `clickhouse/phase-01/runtime-validation.md`
   evidence is technically validating an unsaved state of the source. **This
   is the most important follow-up.**

2. **Secret-handling environment is injected at the UnitSet level, not in
   `podtemplate.yaml`.** Every other UPM package that uses
   `secretRead + AESCTRDecrypt` (mysql-community, redis, redis-sentinel,
   minio, kibana, proxysql) hard-wires `SECRET_MOUNT` and `AES_SECRET_KEY`
   directly into all containers in `podtemplate.yaml`. The ClickHouse
   package does not; it relies on the operator user to add `spec.env` and
   `spec.extraVolume` blocks to every `UnitSet` (as
   `clickhouse/phase-01/manifests/03-clickhouse-unitset.yaml` does). This is
   functionally equivalent at runtime — and Codex's runtime evidence proves
   it works — but:
   - it diverges from the cross-package convention,
   - it makes the failure mode silent if a future user copies only the
     canonical example and forgets the env block,
   - phase-01-spec §4.3 says "Explicit `SECRET_MOUNT` and `AES_SECRET_KEY`
     environment variables for unit-agent." The current solution satisfies
     "explicit and required at deploy time" but not "explicit in the
     package itself."
   Recommended: in a follow-up commit, mirror MySQL's pattern in
   `clickhouse/26.3.9.8/charts/templates/podtemplate.yaml` for both
   init-container, `clickhouse`, and `unit-agent`. Same for
   `clickhouse-keeper`.

3. **Canonical example `unit-operator/examples/unitsets/clickhouse-unitset.yaml`
   was not updated.** It still lacks `env`, `extraVolume`, the
   keeper-service-name annotation, and `podMonitor`. Applying it as-is after
   Phase 01 source changes will produce a pod that fails to render its
   config. This file is the most-discoverable starting point for any new
   user — the runnable example should match the runnable manifest in
   `clickhouse/phase-01/manifests/03`. Same gap for the keeper example.

4. **`scripts/validate-clickhouse-packages.sh` still whitelists
   `max_concurrent_queries`** as an approved dynamic setting (line 120). It's
   only a whitelist (so removal of the entry from the JSON does not trigger
   a failure), but the dead entry is inconsistent with §4.2 of the phase
   spec and will confuse a future reader of the validator. The same script's
   `validate_helm` sub-gate fails because it does not run `helm dependency
   update` first — local validation needs the bitnami `common` subchart.

5. **Runtime-validation summary is concise but not raw.**
   `clickhouse/phase-01/runtime-validation.md` records the pass/fail line
   for each check but does not preserve the full `kubectl get ... -o yaml` /
   `clickhouse-client --query` outputs the way `docs/runtime/03–05` did.
   The automated script exists and is reproducible, so this is not blocking,
   but a "raw snapshot" file under `docs/runtime/` would keep the evidence
   trail consistent with the audit pattern that earned the project its
   Phase 01 A=9.0 score.

6. **Image is `localhost/upmio/clickhouse:26.3.9.8-runtime`, not a public
   registry tag.** Expected for a hackathon environment with image-pull
   constraints (recorded in `evidence-summary.md` §2). For the demo, document
   how to rebuild the image so reviewers can reproduce.

7. **`EXIT_DIR_NOT_FOUND` (41) is used for both "directory not found" and
   "file not found"** in `service-ctl.sh` (lines 44 and 47). Cosmetic
   naming inconsistency, not a bug; a future `EXIT_FILE_NOT_FOUND` would be
   clearer.

8. **Master-spec sealed decision: "Use Kubernetes Job path for backup/restore
   validation when GrpcCall remains unsupported" was added on 2026-06-02 —
   but `unit-operator` now ships ClickHouse GrpcCall routing**
   (commits `1bb205d feat(clickhouse): route grpc operations` and
   `e942ad3 feat(clickhouse): implement agent operations` on 2026-04-28). The
   `master-spec.md` §5.3 still says "Runtime validation showed that the
   installed unit-operator:v1.1.0 does not support GrpcCall(type=clickhouse)."
   This is **not a Phase 01 problem** — Phase 01 deliberately does not use
   GrpcCall — but Phase 07 should re-validate, because the original blocker
   may already be gone in the new operator image.

### 7.3 Out-of-scope observations

These items are correctly NOT addressed by Phase 01 and are listed only so
they don't get lost when planning Phase 02+:

- The phase-spec §5 expected example values at
  `examples/clickhouse/1s2r-values.yaml`. Codex put them at
  `clickhouse/phase-01/values/`. The phase-spec explicitly allows this
  ("if the actual repository uses different file names, keep the same
  responsibility split"), so this is OK but worth noting when Phase 02 adds
  2s2r / 2s3r / 4s2r values — keep them in one consistent directory.
- The phase-spec §5 also expected
  `upm-packages/clickhouse/<version>/charts/values.yaml` to change. Codex did
  not modify it (the topology lives in `clickhouseTemplate.tpl` driven by
  env vars, and image / registry remain default). Acceptable for Phase 01;
  Phase 02 will need to add `topology.shards` and `topology.replicasPerShard`
  here.

## 8. Verdict and Follow-ups

### 8.1 Verdict

**Phase 01 PASSES.** Every one of the 13 acceptance criteria is met with
documented evidence; static gates pass on the working tree; runtime evidence
on the real 4-node cluster is real and reproducible via
`clickhouse/phase-01/scripts/validate-runtime.sh`. The work is in scope, the
non-goals are respected, and the security baseline is enforced both at
template-render time and at runtime.

**The project may proceed to Phase 02.**

### 8.2 Required cleanup before Phase 02 commits land

These three items should be done before Phase 02 work begins committing to
the same files — they're cheap now and expensive later.

1. **Commit the `upm-packages` working-tree changes.** Single commit on a
   `phase-01-*` branch, with the message:
   `phase-01: render clickhouse password, prometheus endpoint, secret-aware service-ctl`.
   This makes the Phase 01 source state immutable evidence.
2. **Update `unit-operator/examples/unitsets/clickhouse-unitset.yaml` and
   `clickhouse-keeper-unitset.yaml`** to match the working configuration in
   `clickhouse/phase-01/manifests/03` (add `env`, `extraVolume`, the
   `upm.api/clickhouse-keeper.service-name` annotation, and `podMonitor`).
   Otherwise the canonical example will silently break.
3. **Choose one** of:
   - (a) Mirror the MySQL pattern by hard-wiring `SECRET_MOUNT` /
     `AES_SECRET_KEY` env in `podtemplate.yaml` (preferred, matches all
     other UPM packages), or
   - (b) Document in `upm-packages/clickhouse/README.md` and the phase
     deliverable README that UnitSet `spec.env` / `spec.extraVolume`
     injection is the deliberate convention for ClickHouse and is required
     in every deployment.

### 8.3 Nice-to-have follow-ups

4. Remove `max_concurrent_queries` from the whitelist in
   `scripts/validate-clickhouse-packages.sh` line 120, and add `helm
   dependency update` to `validate_helm` before `helm lint`.
5. Add a `docs/runtime/08-phase01-runtime-revalidation.md` with the raw
   kubectl/SQL outputs from the validate-runtime.sh run, so the evidence
   pattern stays uniform with `docs/runtime/03–07`.
6. Re-test `GrpcCall(type=clickhouse)` against the current `unit-operator`
   `main`, since it now carries dedicated ClickHouse routing
   (`1bb205d`, `e942ad3`). If it now works, that's load-bearing input for
   Phase 07 and would let `master-spec.md` §5.3 / §10 / Decision Record be
   re-sealed.

## 9. References

- `docs/master-spec.md` — sealed master spec.
- `docs/phases/phase-01-upm-packages-clickhouse.md` — phase spec under review.
- `docs/phases/phase-02-clickhouse-topology.md` — next phase.
- `docs/review/phase01-score-response.md` — hackathon Phase 01 review
  repair plan (separate from this review).
- `clickhouse/phase-01/README.md` — deployment runbook.
- `clickhouse/phase-01/runtime-validation.md` — Codex's runtime evidence.
- `clickhouse/phase-01/scripts/validate-runtime.sh` — automated acceptance
  script.
- `upm-packages/clickhouse/26.3.9.8/charts/files/clickhouseTemplate.tpl` —
  the template that ships the fixes.
- `upm-packages/clickhouse/26.3.9.8/image/service-ctl.sh` — the script that
  enforces password-based login at runtime.
