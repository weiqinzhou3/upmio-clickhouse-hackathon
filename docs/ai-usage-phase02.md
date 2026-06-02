# AI Usage Statement - Hackathon Second Stage

- Version: 0.2
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
