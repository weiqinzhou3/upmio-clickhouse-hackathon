# AI Usage Statement - Hackathon Second Stage

- Version: 0.3
- Date: 2026-06-02
- Status: Working
- Owner: zqw
- Audience: Hackathon reviewers
- Related:
  - ai-usage.md
  - review/phase01-score-response.md

## 1. Purpose

This document records AI usage evidence for the hackathon second stage:
implementation, testing, review, repair, deployment validation, and demo
preparation.

It extends the Phase 01 AI usage statement. It is not an instruction document
for coding agents. Coding agents should follow `AGENTS.md`, `docs/master-spec.md`,
`docs/evidence-summary.md`, the current phase spec, and referenced design docs.

## 2. Collaboration Model

| Role | Stage-Two Responsibility | Evidence |
|---|---|---|
| Codex | Primary implementation, tests, command execution, runtime validation, repair reports | code diffs, command output summaries, changed files, completion reports |
| Claude Code | Stage review, adversarial code/spec review, issue finding | review notes supplied by the owner or saved under `docs/review/` |
| ChatGPT | Requirement clarification, ambiguous design discussion, option analysis | owner-provided "AI evidence material" summaries |
| Human owner | Scope decision, final requirement judgment, safety approval, review disposition | decision records in this document |

## 3. Evidence Rules

- AI completion statements are not evidence by themselves.
- Accepted evidence includes command output, tests, diffs, Kubernetes runtime
  state, ClickHouse SQL output, Prometheus query output, review comments, and
  human decision records.
- Raw chats do not need to be stored in full. Concise summaries are acceptable
  when they include the question, options, decision, rationale, and artifact
  impact.
- Secret values must not be pasted into this document.

## 4. How to Submit AI Evidence Material

When the human owner discusses a topic with ChatGPT or Claude Code, the preferred
summary format is:

```text
Topic:
Date:
AI tool:
Question:
Options considered:
AI recommendation:
Human decision:
Rationale:
Files or specs affected:
Follow-up action:
```

Codex will summarize submitted evidence and append it to the log below.

## 5. Running Evidence Log

### Entry 001 - Phase 01 Review Repair Planning

- Date: 2026-06-02
- AI tools: Codex, ChatGPT discussion summarized by owner
- Topic: Pre-development repair items after hackathon Phase 01 scoring
- Question: How should the project repair review gaps before starting hackathon
  second-stage implementation?
- Options considered:
  - Mark backup/restore as deferred when GrpcCall is unsupported.
  - Implement backup/restore through an approved Kubernetes task path.
  - Modify UPMIO source code to repair ClickHouse GrpcCall.
  - Clarify whether Kubernetes `metrics-server` solves monitoring acceptance.
- Human decision:
  - Backup/restore is required and must not be dropped.
  - Use Kubernetes Job-based backup/restore as the primary MVP/demo path if
    GrpcCall remains unsupported.
  - Keep GrpcCall source repair as a secondary spike, not as the critical path.
  - Treat `metrics-server` as supplementary resource evidence only; require the
    ClickHouse `/metrics -> PodMonitor -> Prometheus -> Manager` monitoring
    chain.
  - Maintain this document as the stage-two AI usage evidence log.
- Rationale:
  - Backup/restore is a core database operations capability.
  - The hackathon should avoid putting a required demo capability behind an
    uncertain UPMIO operator source-code fix.
  - Prometheus scrape validation is distinct from Kubernetes resource metrics.
- Files affected:
  - `docs/master-spec.md`
  - `docs/design/day1-day2-requirement-coverage.md`
  - `docs/phases/phase-05-monitoring.md`
  - `docs/phases/phase-07-backup-restore.md`
  - `docs/review/phase01-score-response.md`
  - `docs/ai-usage-phase02.md`
  - `AGENTS.md`
- Follow-up action:
  - Before each implementation phase, Codex reads the current phase spec and
    explains capability, boundaries, risks, and reasonableness before coding.

### Entry 002 - Phase 01 Package Implementation

- Date: 2026-06-02
- AI tools: Codex
- Topic: Implement Phase 01 package fixes and hackathon evidence assets
- Question: How to make ClickHouse package deployment repeatable without manual
  ConfigMap patching while keeping hackathon-facing evidence under `clickhouse/`?
- Human decision:
  - Use Secret key name `CLICKHOUSE_ADMIN_PASSWORD`.
  - Add an `admin` ClickHouse user.
  - Keep new ClickHouse evidence assets under `clickhouse/`.
  - Modify `upm-packages/clickhouse` source for the required package fixes, and
    expose symlinks from `clickhouse/phase-01/modified-files/`.
- Implementation summary:
  - Removed the invalid `max_concurrent_queries` setting from ClickHouse package
    template, default values, and parameter metadata.
  - Added native ClickHouse Prometheus config for `/metrics` on port `9363`.
  - Replaced `<no_password/>` behavior with Secret-backed
    `password_sha256_hex` for `default` and `admin`.
  - Updated `service-ctl.sh` to decrypt the admin password from the mounted UPMIO
    Secret and use a temporary `clickhouse-client` config file instead of
    exposing the password in process arguments.
  - Added Phase 01 manifests, values, README, and symlinks under
    `clickhouse/phase-01/`.
- Evidence:
  - `bash -n` passed for ClickHouse and Keeper service control scripts.
  - `python3 -m json.tool` passed for ClickHouse parameter metadata JSON.
  - Helm lint/template passed against a temporary local `common` helper chart
    because OCI dependency download from Docker Hub hung in this environment.
  - Rendered ClickHouse config check passed:
    no `<no_password/>`, no `max_concurrent_queries`, includes `<prometheus>`,
    includes `CLICKHOUSE_ADMIN_PASSWORD`, and includes `password_sha256_hex`.
  - Local `service-ctl.sh` behavior test passed with a temporary encrypted
    Secret and stubbed `clickhouse-client`: decrypted admin password correctly,
    XML-escaped special characters, supported `login --query`, and did not use
    `--password` process arguments.
  - Phase 01 YAML manifests parsed with `yq`.
  - Remote Kubernetes server-side dry-run on `192.168.35.201` passed for the
    Phase 01 Namespace/Project, Secret example, Keeper UnitSet, and ClickHouse
    UnitSet manifests.
- Runtime boundary:
  - The existing remote ClickHouse runtime is still using the old package chart
    and old container image. It was not replaced automatically.
  - Existing runtime `SELECT 1` still succeeds, but `127.0.0.1:9363/metrics`
    still refuses connection because the old runtime has not been upgraded to
    this Phase 01 package/image.
  - Full runtime acceptance for password auth and metrics requires an explicit
    package chart upgrade plus a ClickHouse image containing the updated
    `service-ctl.sh`.
- Files affected:
  - `upm-packages/clickhouse/26.3.9.8/charts/files/clickhouseTemplate.tpl`
  - `upm-packages/clickhouse/26.3.9.8/charts/files/clickhouseValue.yaml`
  - `upm-packages/clickhouse/26.3.9.8/charts/files/clickhouseParametersDetail.json`
  - `upm-packages/clickhouse/26.3.9.8/image/service-ctl.sh`
  - `clickhouse/phase-01/`
  - `docs/ai-usage-phase02.md`
- Follow-up action:
  - Decide whether the next step should be image build/package upgrade runtime
    validation, or proceed to the next confirmed repository phase.

### Entry 003 - Old Runtime Cleanup

- Date: 2026-06-02
- AI tools: Codex
- Topic: Remove stale ClickHouse runtime that could interfere with Phase 01
  acceptance
- Human decision:
  - The ClickHouse runtime deployed during earlier UPMIO validation may be
    deleted.
  - Stale runtime evidence should not remain in the cluster during hackathon
    acceptance.
- Actions executed:
  - Deleted `clickhouse-runtime` and `clickhouse-runtime-keeper` UnitSets.
  - Deleted namespace `upm-clickhouse-runtime`.
  - Removed a stale Secret finalizer left by the old runtime so the namespace
    could terminate.
  - Deleted Project `upm-clickhouse-runtime`.
  - Upgraded `clickhouse-package` and `clickhouse-keeper-package` Helm releases
    in `upm-system` from the current local Phase 01 package chart source.
- Evidence:
  - `upm-clickhouse-runtime` namespace is absent.
  - `upm-clickhouse-runtime` Project is absent.
  - No UnitSet, Unit, Pod, PVC, Service, ConfigMap, Secret, or PodMonitor remains
    in `upm-clickhouse-runtime`.
  - `unit-operator` and `compose-operator` Pods remain Running in `upm-system`.
  - `clickhouse-package` and `clickhouse-keeper-package` are deployed as Helm
    revision 2.
  - Updated ClickHouse package ConfigMap no longer contains `<no_password/>` or
    `max_concurrent_queries`, and contains Prometheus plus
    `CLICKHOUSE_ADMIN_PASSWORD` template references.
- Runtime boundary:
  - No new ClickHouse runtime Pods were created in this cleanup.
  - Full Phase 01 runtime validation still requires a ClickHouse image containing
    the updated `service-ctl.sh`.

### Entry 004 - Phase 01 Runtime Upgrade Validation

- Date: 2026-06-02
- AI tools: Codex
- Topic: Validate Phase 01 package and runtime image fixes on Kubernetes
- Actions executed:
  - Built a product-tagged derived image
    `localhost/upmio/clickhouse:26.3.9.8-runtime` from the existing
    `quay.io/upmio/clickhouse:26.3.9.8`, replacing only `service-ctl.sh`.
  - Imported the image into containerd `k8s.io` namespace on the master and all
    three worker nodes.
  - Upgraded `clickhouse-package` in `upm-system` to use the local runtime image.
  - Created a clean runtime namespace, Project, Secret, 3-node Keeper UnitSet,
    and 2-replica ClickHouse UnitSet.
  - Generated runtime Secret values only on the remote cluster; no plaintext
    Secret values were written to Git or evidence files.
- Evidence:
  - `clickhouse-runtime-keeper` reached expected/current/ready `3/3/3`.
  - `clickhouse-runtime` reached expected/current/ready `2/2/2`.
  - All five runtime Pods reached `2/2 Running` with zero restarts during
    validation.
  - ClickHouse server Pods used
    `localhost/upmio/clickhouse:26.3.9.8-runtime`.
  - Config checks passed: Prometheus config present, `<no_password/>` absent,
    `max_concurrent_queries` absent, and `password_sha256_hex` present.
  - Keeper checks passed: `ruok=imok`; one leader and two followers.
  - Authentication checks passed: `service-ctl.sh health` succeeded and
    no-password `clickhouse-client` login was rejected.
  - SQL topology showed `upm_cluster` with one shard and two replicas.
  - ReplicatedMergeTree validation table replicated `runtime-ok` to both
    ClickHouse Pods.
  - `system.replicas` showed `is_readonly=0`, `is_session_expired=0`,
    `absolute_delay=0`, and `queue_size=0`.
  - `curl http://127.0.0.1:9363/metrics` returned Prometheus output including
    `ClickHouse_Info`.
- Files affected:
  - `clickhouse/phase-01/runtime-validation.md`
  - `clickhouse/phase-01/README.md`
  - `clickhouse/phase-01/manifests/01-runtime-secret.example.yaml`
  - `docs/ai-usage-phase02.md`
- Follow-up action:
  - Keep the running runtime available for future Manager/healthcheck phases, or
    explicitly clean it before a fresh deployment test.

### Entry 005 - Phase Workflow and Claude Review Remediation

- Date: 2026-06-02
- AI tools: Codex, Claude Code, ChatGPT discussion summarized by owner
- Topic: Establish repeatable phase workflow and close Phase 01 review findings
- Owner methodology:
  - Before each phase, Codex explains phase objective, boundary, acceptance
    commands, and expected good-state output.
  - After implementation, Claude Code performs an adversarial review and writes
    a report under `docs/review/`.
  - The human owner and ChatGPT review the phase result and decide which review
    findings must be fixed.
  - Codex applies accepted fixes, reruns quality gates, records evidence, and
    produces a phase closeout.
  - Phase specs, master spec, and review dispositions are updated only when the
    validated implementation changes the sealed plan or evidence.
- Review prompt summary:
  - Claude Code was asked to understand the hackathon requirements in `exam/`,
    review Codex Phase 01 work against `docs/phases/phase-01-upm-packages-clickhouse.md`,
    decide whether Phase 01 passes, and summarize the next repository phase only
    if Phase 01 passes.
- Claude Code review result:
  - Verdict: Phase 01 passes and may proceed.
  - Required cleanup: commit Phase 01 source changes, update canonical
    ClickHouse UnitSet examples, and align ClickHouse Secret env injection with
    existing UPM package podtemplate conventions.
  - Phase 07 signal: revalidate ClickHouse `GrpcCall` support on the current
    `unit-operator` source because newer commits may have closed the earlier
    runtime gap.
- Human decision:
  - Accept the review cleanup items as Phase 01 closeout work before repository
    Phase 02 starts.
  - Keep backup/restore as a required database operations capability; Phase 07
    must re-check both Kubernetes Job and GrpcCall paths.
- Codex remediation summary:
  - Added package-level `SECRET_MOUNT` / `AES_SECRET_KEY` env injection to the
    ClickHouse and ClickHouse Keeper podtemplates using the established
    `unit-secret` extraVolume and `aes-secret-key` convention.
  - Updated canonical `unit-operator/examples/unitsets/clickhouse*.yaml`
    examples with Secret env, log emptyDir, `unit-secret` extraVolume, Keeper
    service annotation, and PodMonitor where applicable.
  - Split Phase 01 Secret examples and runtime creation instructions into
    `aes-secret-key` for the AES key and `clickhouse-runtime-secret` for
    encrypted ClickHouse credentials.
  - Updated the ClickHouse package validation script to remove the obsolete
    `max_concurrent_queries` whitelist entry and build Helm dependencies before
    lint/template.
- Evidence:
  - Review report: `docs/review/phase-01-review.md`.
  - Updated source files and examples are validated by the Phase 01 closeout
    quality gates recorded in the completion report.

### Entry 006 - Phase 02 Open Question Closure

- Date: 2026-06-02
- AI tools: Codex, ChatGPT discussion summarized by owner
- Topic: Close Phase 02 topology open questions before implementation
- Question:
  - Should Distributed table initialization be owned by Manager or users?
  - How should multi-shard write routing be exposed?
  - How should DDL idempotency be defined for production-grade safety?
- Human decision:
  - Do not define the decisions by the smallest MVP. Define them by
    enterprise production expectations.
  - A future product may expose database management APIs and UI workflows.
  - User business table management should remain DBA/application-owned; the
    owner corrected the earlier over-expanded table-management interpretation.
  - Multi-shard write routing through Distributed tables and a stable service is
    acceptable.
  - DDL idempotency should be decided by Codex using production safety
    principles.
- Codex decision:
  - Manager may create only deterministic validation Distributed tables during
    MVP healthcheck.
  - Production Manager may expose explicit database management APIs; UI
    workflows should call those APIs.
  - Manager must not silently create or mutate business tables as a side effect
    of cluster creation.
  - User business local table and Distributed table lifecycle is out of Manager
    ownership; Manager may provide validation objects, topology guidance, and
    read-only diagnostics.
  - Application write/query path should use Distributed tables through a stable
    ClickHouse service; local-table writes are validation/admin-only.
  - DDL safety for Manager-owned database lifecycle and validation objects
    requires idempotent SQL plus desired-state recording, definition
    comparison, drift failure, dry-run, approval/audit extension fields, and
    post-apply verification. `IF NOT EXISTS` alone is not sufficient.
- Files affected:
  - `docs/master-spec.md`
  - `docs/phases/phase-02-clickhouse-topology.md`
  - `docs/architecture/clickhouse-ha-architecture.md`
  - `docs/design/api-design.md`
  - `docs/design/product-design.md`
- Evidence:
  - Phase 02 open questions are resolved in the Master Spec decision record and
    no longer left as `TBD`.

### Entry 007 - Phase 02 Topology Rendering Implementation

- Date: 2026-06-05
- AI tools: Codex
- Topic: Implement repository Phase 02 static topology rendering
- Human instruction:
  - Resume hackathon work after a temporary pause for AI Coding methodology
    material.
  - Confirm unrelated AI Coding files were moved away.
  - Confirm Kubernetes API is reachable again.
  - Start Repository Phase 02 coding.
- Codex phase-start review:
  - Re-read `docs/master-spec.md`, `docs/evidence-summary.md`,
    `docs/phases/phase-02-clickhouse-topology.md`, and referenced
    architecture/design/runtime decision documents.
  - Confirmed this phase implements package-level topology parameterization,
    not Manager Backend, backup/restore, business table management, automatic
    resharding, or a 2x2 runtime success claim.
- Implementation decisions:
  - Keep the UPMIO package/runtime-template path instead of bypassing UPMIO
    with native Kubernetes-only configuration.
  - Render `remote_servers` through Helm using `topology.shards` and
    `topology.replicasPerShard`, so static `helm template` output can be
    objectively counted.
  - Keep per-Pod `macros.shard` and `macros.replica` as runtime template values
    derived from `unit-operator/unit.sn`, because one PodTemplate is shared by
    all ClickHouse units.
  - Use single-UnitSet mapping: `unitIndex = shardIndex *
    replicasPerShard + replicaIndex`.
  - Keep the recommended Keeper path strategy as
    `/clickhouse/tables/{cluster}/{shard}/{database}/{table}`.
  - Preserve the business-table boundary: topology rendering does not imply
    Manager ownership of user business local or Distributed tables.
- Files affected:
  - `upm-packages/clickhouse/26.3.9.8/charts/values.yaml`
  - `upm-packages/clickhouse/26.3.9.8/charts/templates/configTemplate.yaml`
  - `upm-packages/clickhouse/26.3.9.8/charts/templates/configValue.yaml`
  - `upm-packages/clickhouse/26.3.9.8/charts/templates/podtemplate.yaml`
  - `upm-packages/clickhouse/26.3.9.8/charts/files/clickhouseTemplate.tpl`
  - `upm-packages/clickhouse/26.3.9.8/image/service-ctl.sh`
  - `upm-packages/clickhouse/26.3.9.8/charts/README.md`
  - `clickhouse/phase-02/`
- Evidence:
  - Phase 02 evidence values and verification script are stored under
    `clickhouse/phase-02/`.
  - Source modification symlinks are stored under
    `clickhouse/phase-02/modified-files/`.

### Entry 008 - Phase 02 Runtime Validation Repair

- Date: 2026-06-05
- AI tools: Codex, human review challenge
- Topic: Replace paper-only Phase 02 acceptance with real Kubernetes and
  ClickHouse SQL validation
- Human challenge:
  - Dry-run and static analysis are not sufficient acceptance evidence.
  - Phase 02 must be validated by real `kubectl apply`, post-apply Kubernetes
    state, ClickHouse topology checks, and read/write SQL tests.
- Runtime findings:
  - Real 2x2 ClickHouse apply initially failed because the node runtime image
    still contained the old `service-ctl.sh`, which rejected `UNIT_COUNT=4`.
  - After rebuilding/importing the local runtime image on all Kubernetes nodes
    through a repeatable script, the 4 ClickHouse Pods reached `2/2 Running`.
  - `ON CLUSTER` DDL initially failed because the ClickHouse config lacked
    `distributed_ddl`.
  - Distributed table writes initially failed because `remote_servers` lacked a
    cluster secret, so remote shard connections attempted default-user
    authentication.
- Codex remediation summary:
  - Added `clickhouse/sync-runtime-image-to-nodes.sh` to
    rebuild/import `localhost/upmio/clickhouse:26.3.9.8-runtime` on all
    Kubernetes nodes with the updated `service-ctl.sh`.
  - Kept GHCR/Harbor as the production-grade image distribution path; the
    node-local image sync is a reproducible lab-cluster step, not the target
    production model.
  - Added service-scoped `distributed_ddl` to the ClickHouse package config:
    `/clickhouse/task_queue/ddl/<unitset-name>`.
  - Added runtime-derived `remote_servers` cluster secret based on existing
    Secret material, avoiding plaintext remote passwords in the cluster config.
  - Added `clickhouse/phase-02/scripts/validate-runtime-2s2r.sh` for full
    runtime validation.
  - Updated Phase 02 docs and design docs so 2x2 runtime support is not claimed
    without real apply and SQL evidence.
- Evidence:
  - `clickhouse-phase02-keeper` reached expected/current/ready `3/3/3`.
  - `clickhouse-phase02` reached expected/current/ready `4/4/4`.
  - `system.clusters` returned 4 rows for `upm_cluster`: 2 shards x 2 replicas.
  - Macros mapped as expected:
    `0 -> shard01/replica01`, `1 -> shard01/replica02`,
    `2 -> shard02/replica01`, `3 -> shard02/replica02`.
  - `ON CLUSTER` created a validation database, a ReplicatedMergeTree local
    table, and a Distributed table.
  - Distributed write/read validation inserted 8 rows, read 4 rows from each
    shard, and confirmed every replica had 4 local rows.
  - Replica health query returned `total_replicas=2`, `active_replicas=2`,
    `is_readonly=0`, `queue_size=0`, and `absolute_delay=0` for all replicas.
  - Runtime script ended with `PASS runtime_2s2r_validation`.
- Files affected:
  - `upm-packages/clickhouse/26.3.9.8/charts/files/clickhouseTemplate.tpl`
  - `upm-packages/clickhouse/26.3.9.8/charts/README.md`
  - `docs/phases/phase-02-clickhouse-topology.md`
  - `docs/design/upm-packages-clickhouse-design.md`
  - `clickhouse/phase-02/README.md`
  - `clickhouse/phase-02/scripts/validate-rendered-topology.py`
  - `clickhouse/sync-runtime-image-to-nodes.sh`
  - `clickhouse/phase-02/scripts/validate-runtime-2s2r.sh`

### Entry 009 - Phase 02 Review Closeout

- Date: 2026-06-05
- AI tools: Claude Code, Codex
- Topic: Review Phase 02 completion and decide whether to enter Phase 03
- Review artifact:
  - `docs/review/phase-02-review.md`
- Claude Code verdict:
  - PASS, proceed to Phase 03.
  - All 13 Phase 02 acceptance criteria are marked PASS.
  - Three findings are non-blocking follow-ups:
    lab-local runtime image distribution, topology env/default coupling, and
    cluster secret entropy/rotation dependency.
- Codex verification after reading the report:
  - Re-ran static topology rendering checks for 1s2r, 2s2r, 2s3r, and 4s2r;
    all ended with `PASS topology_render_validation`.
  - Re-ran `markdownlint "docs/**/*.md" "clickhouse/**/*.md"`; passed.
  - Re-ran `gitleaks detect --source . --redact --no-git`; passed with no leaks
    found.
  - Checked live Phase 02 Kubernetes runtime; Keeper UnitSet was `3/3/3`, and
    ClickHouse UnitSet was `4/4/4`.
  - Re-ran `clickhouse/phase-02/scripts/validate-runtime-2s2r.sh`; it ended
    with `PASS runtime_2s2r_validation`.
- Human decision pending:
  - Phase 02 can be treated as closed after committing this review report and
    evidence entry.
  - Phase 03 can start after the owner confirms the Phase 03 start review.

### Entry 010 - Phase 03 UPM API Server Review Closeout

- Date: 2026-06-06
- AI tools: Codex, Claude Code
- Topic: Review Phase 03 `upm-api-server` implementation and decide whether
  to enter Phase 04.
- Review artifact:
  - `docs/review/phase-03-review.md`
- Claude Code verdict:
  - PASS, proceed to Phase 04.
  - All 12 Phase 03 acceptance criteria are marked PASS.
  - Four findings are non-blocking:
    `internal/clickhouse` is Phase 04 scaffolding, secret-leak unit tests can
    be stronger, `POST /clusters` should later expose a polling hint, and the
    lab-local image sync should eventually move to immutable registry images.
- Codex disposition:
  - Accepted F1 and added a package comment for `internal/clickhouse`, making
    the Phase 04 healthcheck purpose explicit.
  - Accepted F2 and strengthened tests so request secret references are not
    logged and response models cannot add `secret`/`password` fields silently.
  - Deferred F3 to Phase 04 because polling links belong with the healthcheck
    state API and would otherwise change the Phase 03 response contract late.
  - Deferred F4 to productization/Phase 04+ because the owner accepted
    node-local image sync for the isolated lab, while the docs already record
    the production registry/digest direction.
- Runtime evidence from Codex:
  - Cleaned old ClickHouse runtime namespaces and UPMIO Projects so the lab
    retains only `upm-clickhouse-phase03-runtime`.
  - Exposed `upm-api-server` as NodePort `30083` and validated healthz on
    `192.168.35.201`, `.202`, `.203`, and `.204`.
  - `GET /api/v1/clusters` returns only
    `upm-clickhouse-phase03-runtime/clickhouse-phase03`, status `Running`,
    Keeper `3/3`, Server `4/4`.
  - `clickhouse/phase-03/scripts/validate-upm-api-server-runtime.sh` ends with
    `PASS expected_structured_error` and
    `PASS upm_api_server_runtime_validation`.
  - `clickhouse/phase-02/scripts/validate-runtime-2s2r.sh`, pointed at the
    Phase 03 cluster, ends with `PASS runtime_2s2r_validation`.
- Closeout decision:
  - Phase 03 is ready to close after committing the review report, accepted
    repairs, evidence entry, and final quality-gate output.
  - Per `AGENTS.md` phase branch rules, merge `phase-03` into `main` after
    closeout so third-party reviewers can see the complete code on `main`
    while the historical `phase-03` branch remains available.

### Entry 011 - Phase 04 Review Red-Team Closeout

- Date: 2026-06-06
- AI tools: Claude Code, Codex
- Topic: Review and repair the Phase 04 Day1 healthcheck implementation
- Review artifact:
  - `docs/review/phase-04-review.md`
- Claude Code verdict:
  - PASS, proceed to Phase 05.
  - Four non-blocking findings: unused ClickHouse HTTP client, limited kube
    healthcheck unit tests, narrow secret-leak scan, and a suggested
    cluster-ready precondition.
- Codex red-team verification:
  - Accepted removal of the unused ClickHouse HTTP client.
  - Accepted stronger pure-function and Service/Endpoint tests.
  - Strengthened secret protection from literal field-name matching to actual
    referenced Secret-value redaction and validation.
  - Rejected the suggested cluster-ready early return because healthcheck must
    diagnose degraded and provisioning clusters rather than stop before
    producing individual check results.
  - Found additional gaps not identified in the Claude report: drift was
    checked after table mutation, replica row validation hardcoded `4`,
    Service ports were not validated, same-cluster probes were not serialized,
    and the report omitted `cluster`/`durationMs`.
- Methodology evidence:
  - A PASS review is treated as evidence, not an automatic merge decision.
  - Codex re-compared the implementation with the confirmed phase spec and
    challenged both the implementation and reviewer recommendations.
  - Review findings were accepted, rejected, or expanded based on product
    behavior and executable evidence.
- Resolution artifact:
  - `docs/review/phase-04-review-response.md`

### Entry 012 - Phase 05 Monitoring Start Review and Implementation

- Date: 2026-06-06
- AI tool: Codex
- Topic: Start-review and implement the real Prometheus monitoring closure
- Human decisions:
  - Accepted the Phase 05 start-review findings.
  - Required `kube-prometheus-stack` environment assets to live outside the
    UPM repository because they are environment readiness, not UPMIO product
    code.
- Codex decisions and implementation:
  - Rejected PodMonitor-only or paper validation as sufficient Phase 05
    acceptance; required a real Prometheus Server and 4/4 target proof.
  - Kept external chart values and automated image synchronization under
    `../kube-prometheus-stack/`, outside Git.
  - Implemented a fixed-query Prometheus client and registered
    `/api/v1/clusters/{namespace}/{name}/metrics/summary` in
    `upm-api-server`.
  - Compared expected Kubernetes ClickHouse Pods with Prometheus target state
    to prevent partial discovery from producing a false `READY`.
  - Used ClickHouse native disk used/total/available metrics as the real
    storage fallback after runtime evidence proved that the lab `local-path`
    provisioner does not expose kubelet PVC volume statistics.
  - Reduced returned metric labels to stable user-relevant fields after
    inspecting the first real API response.
- Runtime evidence:
  - Full chain passed with 4/4 targets up, CPU 4, memory 4, storage 12,
    ClickHouse 20, and no warnings.
  - Prometheus-unavailable behavior returned structured HTTP `503`.
  - Evidence is stored under `clickhouse/phase-05/`.

### Entry 013 - Phase 05 Grafana Repair After Human Challenge

- Date: 2026-06-06
- AI tool: Codex
- Topic: Repair Phase 05 monitoring to include Grafana dashboard automation and
  panel-level validation
- Human challenge:
  - Prometheus-only validation was not enough for a demo-ready monitoring
    phase.
  - Grafana and Prometheus installation/configuration should be automated.
  - Deployed ClickHouse should be discovered automatically through PodMonitor,
    and Grafana panels must be proven to return real values.
- Codex repair:
  - Extended the Phase 05 spec from Prometheus/API-only monitoring to include
    an external Grafana datasource and ClickHouse dashboard.
  - Kept kube-prometheus-stack and Grafana assets outside the UPM repository
    under `../kube-prometheus-stack/`, because they are environment readiness
    assets rather than UPMIO source code.
  - Added automated Grafana image synchronization, Helm values, datasource
    provisioning, dashboard ConfigMap import, NodePort exposure, and rollout
    checks to the external environment scripts.
  - Strengthened `clickhouse/phase-05/scripts/validate-monitoring-runtime.sh`
    to verify Grafana health, datasource URL, dashboard import, and all
    required panel queries through the Grafana datasource proxy.
- Runtime evidence:
  - Grafana is reachable at `http://192.168.35.201:30300`.
  - Dashboard `upm-clickhouse-overview` is imported.
  - Initial temporary dashboard validation proved the Grafana datasource path
    with 11 required panel queries.
  - The final validation ended with `PASS phase05_monitoring_runtime_validation`.

### Entry 014 - Phase 05 Existing Grafana Dashboard Adaptation

- Date: 2026-06-07
- AI tool: Codex
- Topic: Replace the temporary Grafana dashboard with an adapted existing
  dashboard and record future one-click environment preparation needs
- Human challenge:
  - The generated dashboard was not good enough.
  - Use the existing user-provided `23285_rev1.json` dashboard and tune it.
  - Save and register the dashboard.
  - After all MVP work, prepare documentation and one-click scripts for
    Kubernetes, UPMIO, kube-prometheus-stack, API-driven ClickHouse
    deployment, API operations, and inspection.
- Codex repair:
  - Adapted the source dashboard
    `ClickHouse and Keeper Comprehensive Dashboard` into
    `clickhouse/grafana/upm-clickhouse-23285-dashboard.json`.
  - Rewrote dashboard datasource and PromQL label selectors to the Phase 05
    `namespace`/`pod` PodMonitor label model.
  - Dropped node-exporter-only panels because node-exporter is not enabled in
    the Phase 05 external monitoring environment.
  - Dropped the standalone Keeper service overview because standalone Keeper
    metrics are not scraped in Phase 05.
  - Kept ClickHouse client-to-Keeper panels because those metrics are native
    ClickHouse profile events exposed by the server pods.
  - Registered the dashboard through the Grafana sidecar ConfigMap and fixed
    the large-dashboard ConfigMap workflow by using delete/create instead of
    `kubectl apply`, avoiding the Kubernetes last-applied annotation limit.
  - Reset Grafana admin password to match the external environment values so
    sidecar reload and validation use consistent credentials.
  - Updated validation to execute every visible dashboard target query through
    the Grafana datasource proxy.
- Runtime evidence:
  - Dashboard `upm-clickhouse-overview` is registered with title
    `UPM ClickHouse Operational Dashboard`.
  - The imported dashboard has 185 panels.
  - 170 data panels and 207 visible target queries returned samples.
  - The final validation ended with `PASS phase05_monitoring_runtime_validation`.

### Entry 015 - Phase 05 Review Response

- Date: 2026-06-07
- AI tools: Claude Code, Codex
- Topic: Review and closeout repair for Phase 05 monitoring
- Review artifact:
  - `docs/review/phase-05-review.md`
- Claude Code verdict:
  - PASS, proceed to Phase 06.
  - Five non-blocking findings: hardcoded ClickHouse container name in PromQL,
    missing direct `GetMetricsSummary` orchestration tests, dashboard canonical
    versus runtime evidence relationship, probe row-count documentation, and
    `firstAvailableMetrics` empty-result semantics.
- Codex response:
  - Accepted the PASS verdict but treated non-blocking findings as closeout
    hardening work where the repair was low risk.
  - Added a `clickHouseContainerName` constant for CPU/memory PromQL.
  - Added direct table-driven `GetMetricsSummary` orchestration tests covering
    READY, DEGRADED, missing target, query error, missing PodMonitor, storage
    fallback, and all-storage-empty behavior.
  - Documented the canonical Grafana dashboard asset versus the runtime
    Grafana API evidence copy.
  - Clarified the healthcheck probe row-count multiplier.
  - Changed `firstAvailableMetrics` to distinguish all-empty successful query
    results from all-error query results.
- Resolution artifact:
  - `docs/review/phase-05-review-response.md`

### Entry 016 - Phase 06 Day2 Diagnostics Implementation

- Date: 2026-06-07
- AI tool: Codex
- Topic: Phase 06 startup review, spec repair, implementation, and runtime
  validation for Day2 read-only diagnostics
- Human instruction:
  - Continue after Phase 05 closeout.
  - Before coding, explain the Phase 06 target, boundary, risks, and real
    validation approach.
  - Do not rely on local binary deployment; validate through Kubernetes
    `upm-api-server`.
- Codex work:
  - Performed Phase 06 startup review against `master-spec`,
    `evidence-summary`, Phase 06 spec, and referenced design documents.
  - Repaired Phase 06 spec from the stale `backend/` path to the current
    `api-server/` path.
  - Registered `GET /api/v1/clusters/{namespace}/{name}/diagnostics` in
    `upm-api-server`.
  - Implemented read-only diagnostics for replica state, replication queue,
    parts/partitions, merges, mutations, Keeper leader/follower state, PVC
    storage metadata, write client stats, and write quality row-count checks.
  - Added configurable diagnostics thresholds through the API server ConfigMap.
  - Preserved the Phase 06 boundary: recommendations only, no mutation kill,
    no OPTIMIZE, no data correction, no backup/restore, no GrpcCall dependency.
- Runtime evidence:
  - `upm-api-server` was rebuilt as
    `localhost/upmio/upm-api-server:phase-06`, imported into all four K8s
    nodes, and deployed in `upm-system`.
  - Full runtime validation saved evidence under `clickhouse/phase-06/`.
  - The diagnostics API returned all required categories with zero `CRITICAL`
    findings.
  - Current `system.query_log` is absent, so write client stats correctly
    returned `UNKNOWN` instead of failing the API request.
  - Read-only row-count validation against `upm_healthcheck.dist_events`
    returned `actualRows=8` and `expectedRows=8`.
  - The final validation ended with
    `PASS phase06_diagnostics_runtime_validation`.

### Entry 017 - Phase 06 Review Response

- Date: 2026-06-07
- AI tools: Claude Code, Codex
- Topic: Review and closeout repair for Phase 06 Day2 read-only diagnostics
- Review artifact:
  - `docs/review/phase-06-review.md`
- Claude Code verdict:
  - PASS, proceed to Phase 07.
  - Five non-blocking findings: numeric parser errors were hidden as zero,
    pure diagnostics helpers lacked direct tests, filter validation was
    duplicated, query-log table aggregation could become large in extreme
    cases, and `RunDiagnostics` orchestration lacked direct tests.
- Codex response:
  - Accepted the PASS verdict but repaired the low-risk implementation gaps
    before closeout.
  - Changed numeric parsers to return errors and made diagnostics record
    `numeric_parse_error` evidence with `UNKNOWN` severity instead of silently
    treating malformed evidence as zero.
  - Added tests for SQL quoting, WHERE construction, severity ranking, numeric
    parsing, parse issue evidence, map copying, no server Pod behavior, and SQL
    exec failure degradation.
  - Kept duplicate filter validation intentionally because store-level
    validation is a defensive boundary for future non-HTTP callers.
  - Deferred query-log table aggregation hardening to future persistent write
    analytics or multi-tenant query-log work.
- Resolution artifact:
  - `docs/review/phase-06-review-response.md`

### Entry 018 - Phase 07 Scope Amendment for Scheduled Backup

- Date: 2026-06-07
- AI tool: Codex
- Topic: Add API-managed scheduled backup to Phase 07 before implementation
- Human decision:
  - Scheduled backup must be implemented in the hackathon MVP.
  - Scheduled backup must be invoked and managed through `upm-api-server` APIs.
- Codex response:
  - Identified that the previous `master-spec` and Phase 07 spec treated the
    backup scheduler as out of MVP scope.
  - Recorded a new owner-approved MVP decision: Phase 07 includes a minimal
    API-managed scheduled backup validation slice using Kubernetes CronJob.
  - Preserved the safety boundary: no production backup policy engine, no
    retention cleanup, no destructive restore into business tables, and no
    plaintext credentials in API requests, ConfigMaps, logs, or Git.
  - Updated Phase 07 to include manual backup API, restore API, task status
    API, scheduled backup create/list/get/delete APIs, and real runtime
    validation through Kubernetes Job/CronJob evidence.
- Files affected:
  - `docs/master-spec.md`
  - `docs/design/day1-day2-requirement-coverage.md`
  - `docs/design/api-design.md`
  - `docs/api/upm-api-server-v1.md`
  - `docs/phases/phase-07-backup-restore.md`
  - `docs/ai-usage-phase02.md`

### Entry 019 - Phase 07 Backup/Restore Runtime Implementation

- Date: 2026-06-07
- AI tool: Codex
- Topic: Implement and validate backup, restore, task status, and scheduled
  backup APIs
- Human requirement:
  - Backup/restore is a required database operations capability.
  - Scheduled backup must be exposed through `upm-api-server`.
  - Validation must use real Kubernetes apply/runtime checks and database
    read/write verification, not dry-run-only evidence.
- Codex implementation:
  - Added `upm-api-server` model, routing, store interface, K8s Job/CronJob
    rendering, task status, schedule status, and unit tests for Phase 07.
  - Added RBAC for `jobs`, `cronjobs`, and `pods/log`.
  - Added API documentation for all Phase 07 endpoints, request parameters,
    response fields, error codes, and usage examples.
  - Added `clickhouse/phase-07/scripts/validate-backup-restore-runtime.sh`.
- Runtime evidence and AI correction loop:
  - First runtime backup attempt failed because ClickHouse rejects archive
    backup paths such as `.zip` for `BACKUP ... ON CLUSTER`. Codex changed the
    default path model to S3 directory-style paths.
  - First runtime restore attempt failed because direct restore of a
    ReplicatedMergeTree backup reused the source Keeper path. Codex changed the
    restore Job to verify that the target table does not exist, create an empty
    validation target table, and restore data with `allow_different_table_def`.
  - The final validation completed manual backup, manual restore, scheduled
    backup, scheduled backup restore, schedule list, and schedule delete through
    API calls and Kubernetes Job/CronJob evidence.
- Final runtime result:
  - `PASS phase07_backup_restore_runtime_validation`
