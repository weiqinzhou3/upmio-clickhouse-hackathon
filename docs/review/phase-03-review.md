# Phase 03 Review Report

- Version: 1.0
- Date: 2026-06-06
- Reviewer: Claude (Opus 4.7)
- Owner: 周钦伟 (zqw)
- Scope: Review of Codex's Phase 03 implementation against
  `docs/phases/phase-03-upm-api-server.md` v0.8
- Verdict: **PASS — proceed to Phase 04** (four non-blocking follow-ups in §7)

## 1. Hackathon Context Recap

Phase 03 introduces the product control plane: a Go service named
`upm-api-server` that runs **inside Kubernetes** in `upm-system` and exposes
the first `/api/v1` surface (health, cluster create / list / get / resources).
It is deliberately scoped to be more than ClickHouse-only — the same instance
must be able to manage multiple ClickHouse clusters today and host MySQL /
Redis APIs in future phases.

Out of scope (per spec §3 and §4):

- Frontend, full auth / RBAC at the application layer.
- Persistent operation-history database.
- Raw Pod / PVC / Service creation as the product path.
- Backup / restore, healthcheck, monitoring, diagnostics endpoints
  (Phase 04 / 05 / 06 / 07).
- New UPMIO CRDs.
- Automatic remediation.

Phase 03 v0.8 (2026-06-06) also raised the bar from "local binary OK" to
"Kubernetes-only runtime acceptance" (v0.6) and added the package-topology /
Secret prerequisite checks (v0.7). This review verifies that both raised
bars are met.

## 2. Review Method

1. Re-read `docs/phases/phase-03-upm-api-server.md` v0.8 plus the linked
   `docs/master-spec.md`, `docs/design/api-design.md`,
   `docs/design/resource-model.md`, `docs/design/data-architecture.md`,
   `docs/api/upm-api-server-v1.md`.
2. Inspect every file Codex changed in this branch:
   - `api-server/cmd/upm-api-server/main.go`
   - `api-server/internal/{api,model,kube,clickhouse,config,platform}/*.go`
   - `api-server/internal/api/server_test.go`,
     `api-server/internal/model/cluster_test.go`,
     `api-server/internal/kube/store_test.go`
   - `api-server/Dockerfile`, `api-server/go.mod`, `api-server/README.md`
   - `clickhouse/phase-03/**` (`README.md`, `runtime-validation.md`,
     `manifests/upm-api-server.yaml`,
     `scripts/validate-upm-api-server-runtime.sh`)
   - `clickhouse/sync-upm-api-server-image-to-nodes.sh`
   - `docs/api/upm-api-server-v1.md`
3. Re-run the quality gates locally on the review host:
   - `gofmt -l .` (clean)
   - `go vet ./...` (clean)
   - `go test ./...` and `go test -race ./...` (all packages PASS)
   - `go build ./...` (clean)
   - `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build ./cmd/upm-api-server`
     → ELF 64-bit, statically linked, x86-64
   - `bash -n` on both shell scripts
   - `yq eval-all 'true'` on `upm-api-server.yaml`
4. Cross-check runtime claims in
   `clickhouse/phase-03/runtime-validation.md` against the runtime script
   that produces them.

The review host has no live lab-cluster access today, so the runtime PASS
line is read from `runtime-validation.md` (Date: 2026-06-06); the script that
emits it (`validate-upm-api-server-runtime.sh`) was inspected line by line.

## 3. Acceptance Criteria Verification

Phase 03 spec §10 lists 12 criteria. Verified line-by-line.

| # | Criterion | Status | Evidence |
|---|---|---|---|
| 1 | `upm-api-server` runs inside K8s as Deployment+Service in `upm-system` | PASS | `manifests/upm-api-server.yaml` defines ServiceAccount, ClusterRole/Binding, ConfigMap, Deployment, NodePort Service all in `upm-system`; `runtime-validation.md §2` records `Deployment/upm-api-server: 1/1 Ready` and `Service/upm-api-server NodePort 8080:30083/TCP` |
| 2 | Exposes `/api/v1/healthz` (or equivalent) through K8s Service | PASS | `server.go:31` registers `GET /api/v1/healthz`; readiness/liveness probes hit it (`upm-api-server.yaml:119-130`); runtime evidence: `200 OK` through NodePort 30083 on all four nodes |
| 3 | `POST /api/v1/clusters` validates request and returns structured error | PASS | `model.CreateClusterRequest.Validate()` covers namespace/name/version/topology/storage/security with DNS-1123 checks and `resource.ParseQuantity`; `server.go:71-100` rejects malformed JSON (`INVALID_JSON`) and trailing JSON, then maps validation errors to `VALIDATION_ERROR` 400; runtime script asserts `code == "VALIDATION_ERROR" and message and requestId` on the bad-request fixture |
| 4 | `GET /api/v1/clusters` lists managed clusters by UPMIO labels | PASS | `kube.Store.ListClusters` filters UnitSets by labels `upm.api/service-group.type=clickhouse-sg` ∧ `upm.api/service.type=clickhouse`; supports optional `?namespace=` query (validated); `runtime-validation.md` confirms discovery of `upm-clickhouse-phase03-runtime/clickhouse-phase03` via this endpoint |
| 5 | `GET /clusters/{ns}/{name}/resources` returns UnitSet+Unit+Pod+PVC+Service+Endpoint summary | PASS | `kube.Store.GetClusterResources` returns all six collections plus Events; runtime script greps for `unitset`, `pod`, `pvc`, `service`, `endpoint` keywords and asserts the cluster + keeper names are present |
| 6 | Never logs Secret values | PASS | `server.go` logger only emits `requestId`, `actor`, `method`, `path`, `durationMs`, `code`, `error.Message`. `validateSecretKey` returns `Secret %s/%s` ↔ namespace/name only. Response models contain no `Security` field. Runtime script `assert_no_secret_leak` greps every response for `CLICKHOUSE_ADMIN_PASSWORD`, `AES_SECRET_KEY`, `password:` patterns and exits non-zero on any hit |
| 7 | Does not create raw Pods/PVCs/Services as product path | PASS | ClusterRole in `upm-api-server.yaml:21-40` grants only `get/list/watch` on pods/pvcs/services/endpoints/events — no `create`/`update`/`patch`/`delete`. UPMIO CRDs `projects`/`unitsets` get the write verbs. `kube/store.go` never calls `Pods().Create`/`Services().Create`. Runtime-validation §2 records explicit `kubectl auth can-i create pods` → `no` |
| 8 | Operation-history decision documented and consistently implemented | PASS | Spec §7.3 v0.7 sealed: "MVP no DB; operation result derived from current state". Implementation matches: `CreateCluster` returns `ClusterSummary` directly with `Status: "Provisioning"` and resource refs; no persistence layer in code; no DB-related env vars in ConfigMap |
| 9 | Unit tests cover request validation and error formatting | PASS | `model/cluster_test.go` exercises the happy path plus 7 failure mutations (bad ns, missing version, zero shards/replicas/keepers, invalid storage qty, missing admin secret). `api/server_test.go` tests healthz, validation-error path, trailing-JSON rejection, secret-free response, and empty list. `kube/store_test.go` tests UnitSet rendering (no password in env), installed package topology parsing (quoted strings), secret-key validation, and cluster-name boundary matching. All pass with `-race` |
| 10 | K8s deployment assets run in `upm-system` with RBAC-scoped access | PASS | Single manifest applies in `upm-system`; ClusterRole verbs match spec §4 allowed responsibilities exactly: read core resources, create+manage UPMIO projects/unitsets, read Units; the Deployment runs with `runAsNonRoot`, `seccompProfile: RuntimeDefault`, `readOnlyRootFilesystem`, `allowPrivilegeEscalation: false`, `capabilities.drop: [ALL]`; envFrom the ConfigMap (not Secret) — no secret material in env |
| 11 | `docs/api/upm-api-server-v1.md` lists every supported API with parameters/examples | PASS | v0.4 of the doc covers §4 error model (17 codes), §5 healthz, §6.1–§6.4 cluster APIs (paths, methods, request fields with required/type, validation rules, success status, response body, curl examples). §7 explicitly enumerates Phase 04+ APIs as "Not Supported in Phase 03" so the reference doesn't overpromise |
| 12 | `go test ./...` passes | PASS | Re-executed locally on this host: `ok` for `internal/api` (0.012s), `internal/kube` (0.032s), `internal/model` (0.008s); `?` for `cmd/upm-api-server`, `internal/clickhouse`, `internal/config`, `internal/platform` (no test files, acceptable). `-race` also passes |

All 12 criteria are met.

## 4. Quality Gates — Re-executed in This Review

```text
gofmt -l .                                                          (clean)
go vet ./...                                                        (clean)
go test ./...                                                       (all PASS)
go test -race ./...                                                 (all PASS)
go build ./...                                                      (clean)
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build ./cmd/upm-api-server (ELF/static)
bash -n clickhouse/phase-03/scripts/validate-upm-api-server-runtime.sh
bash -n clickhouse/sync-upm-api-server-image-to-nodes.sh            (both OK)
yq eval-all 'true' clickhouse/phase-03/manifests/upm-api-server.yaml (OK)
```

Spot inspection notes from the rendered code:

- HTTP routing uses Go 1.22+ method-aware `mux.HandleFunc("METHOD /path"…)`,
  so non-listed methods get correct 405s without bespoke routing code.
- Request body is bounded with `http.MaxBytesReader(w, r.Body, 1<<20)` and
  `DisallowUnknownFields()` — defends against unbounded reads and silent
  schema drift.
- A second `decoder.Decode(&struct{}{}) != io.EOF` check rejects trailing
  JSON ("JSON smuggling") with `INVALID_JSON`. Covered by
  `TestCreateClusterRejectsTrailingJSON`.
- `APIError` implements `Unwrap`, so callers can compose; `errors.As` is used
  correctly in `writeError`.
- `internalError` recognizes Kubernetes `Forbidden` responses and maps them
  to `KUBERNETES_FORBIDDEN` 403 — useful when RBAC drifts in production.
- `kube.Store.CreateCluster` is genuinely idempotent at the keeper level:
  it lists existing UnitSets with the cluster's service-group label, errors
  with `CLUSTER_ALREADY_EXISTS`/`CLUSTER_RESOURCE_CONFLICT` for unexpected
  state, and skips Keeper creation when the same shape already exists. This
  is exactly the right read of the Phase 02 idempotency discipline.

## 5. Runtime Evidence Review

`runtime-validation.md` (2026-06-06, PASS) lines up with the script that
produced it:

1. **Prerequisites.** `validate_prerequisites` requires the three UPMIO CRDs,
   the `unit-operator` Pod, and the existing Phase 02 Keeper+Server UnitSets
   to be Ready. No paper substitution.
2. **Deployment.** `validate_api_server_deployment` checks namespace,
   ServiceAccount, Deployment, Service, rollout status, and the NodePort
   `30083` literal. Image sync (`sync-upm-api-server-image-to-nodes.sh`)
   builds linux/amd64 with `CGO_ENABLED=0` and imports into containerd
   on all four nodes; PASS line emitted.
3. **Health.** `/api/v1/healthz` is hit through the NodePort, and the
   runtime evidence records `200 OK` from all four nodes — confirming the
   NodePort is reachable on every node, not just `.201`.
4. **Structured error.** `validate_structured_error` posts a fully malformed
   body and asserts HTTP 400, `code=="VALIDATION_ERROR"`, non-empty
   `message`, non-empty `requestId`, **and** runs the same
   `assert_no_secret_leak` greps. Then prints `PASS expected_structured_error`.
5. **Read paths.** Discovers the existing Phase 03 cluster by `/clusters`,
   then by `/clusters/{ns}/{name}` (asserting topology shape), then by
   `/clusters/{ns}/{name}/resources` (asserting keeper+server names appear
   and resource keywords are present). Every payload runs through the
   secret-leak grep.
6. **Optional E2E create.** With `PHASE03_CREATE_E2E=1`, the script:
   - Optionally deletes only the reserved validation namespace
     (`PHASE03_CREATE_E2E_RESET=1`).
   - Creates the AES Secret + admin-password Secret with the binary
     AES-CTR layout the package expects (raw IV prefix + AES-CTR ciphertext).
     The fix that this must be `--from-file` (binary) rather than base64 text
     is recorded honestly in `runtime-validation.md §5`.
   - Posts the create request, waits for both UnitSets to reach exactly
     `readyUnits=keeperReplicas` and `readyUnits=shards*replicasPerShard`,
     then re-reads `/resources` through the API to confirm visibility.
   - For the default 2x2 topology, hands off to
     `clickhouse/phase-02/scripts/validate-runtime-2s2r.sh` — the same
     Phase 02 hard-asserting validator that ran on the manually applied
     cluster — to prove the API-created cluster is functionally equivalent
     at the SQL layer. Runtime evidence: `system.clusters` shape correct,
     8 distributed rows, 4 per shard, every replica healthy.

The chain "API call → UnitSets ready → SQL works" is reproduced end-to-end
in a script with hard `throwIf` assertions. False PASS would require the
cluster to be genuinely healthy at all three layers. The runtime PASS is
credible.

Two real-execution-only fixes are documented (§5 of the runtime doc) —
quoted-string topology values in the package ConfigMap and binary AES-CTR
Secret layout. Both are visible in code: `kube.topologyInt` accepts `int`,
`float64`, or numeric `string`; the script uses `--from-file=… --from-file=…`
instead of `--from-literal=…`. These are exactly the kind of issues that
static review cannot find — recording them and re-shipping the fixes is the
right behaviour.

## 6. Scope and Decision Boundaries — Held Cleanly

Verified against the spec's explicit boundary lists:

- **No frontend code.** Confirmed — only `api-server/` Go code in
  Phase 03 commits.
- **No application-layer auth/RBAC.** Confirmed — and explicitly
  acknowledged in `docs/api/upm-api-server-v1.md §2` ("Phase 03 NodePort
  has no application-layer authentication … for the isolated hackathon
  lab only"). The `X-Actor` header is a stub extension point, not an
  enforced principal.
- **No persistent operation history DB.** Confirmed — spec §7.3 v0.7
  sealed the decision; implementation has no DB, no migrations, no
  persistence package.
- **No raw Pod/PVC/Service creation as product path.** Confirmed at
  three layers: (a) `kube/store.go` only calls `Get`/`List` on
  pods/pvcs/services/endpoints/events; (b) ClusterRole grants only read
  verbs on those resources; (c) runtime-validation §2 records the
  `kubectl auth can-i create pods → no` check.
- **No Phase 04+ endpoints exposed.** Confirmed by reading the mux:
  only `/healthz`, `POST /clusters`, `GET /clusters`,
  `GET /clusters/{ns}/{name}`, `GET /clusters/{ns}/{name}/resources` are
  registered. The reference doc §7 explicitly lists healthcheck / metrics
  / diagnostics / backup as "Not Supported in Phase 03".
- **One API server, many clusters.** Confirmed — the mux has no cluster
  identity baked in; all cluster ops are path-scoped by `{namespace}/{name}`.
- **Package-topology guard.** Confirmed and validated:
  `validatePackagePrerequisites` reads the topology from the installed
  `clickhouse-<version>-config-value` ConfigMap and returns
  `PACKAGE_TOPOLOGY_MISMATCH` (422) with `{requested, installed}` details
  when the API request and the installed package disagree. This is exactly
  the Phase 02 "single source of truth for shards/replicas" invariant
  enforced at the API boundary. Real runtime test runs with matching `2x2`.

## 7. Findings — Non-blocking

None of these block Phase 04. Tracked for forward visibility.

### F1. `internal/clickhouse` is dead code in Phase 03 (informational)

`api-server/internal/clickhouse/client.go` defines a minimal HTTP query
client (4 KB), but nothing imports it (`grep -rln 'internal/clickhouse'
api-server/` returns no matches). It is scaffolding for Phase 04's
healthcheck SQL.

Spec §2 #5 lists "ClickHouse client wrapper for read-only SQL" as in-scope,
and shipping a tested-by-implication library now is defensible, but readers
of `phase-03` would benefit from either (a) a one-line `// Used by Phase 04
healthcheck` package comment or (b) deferring the package to the Phase 04
PR. Not a quality issue — `go vet` is clean and the package compiles.

### F2. Secret-leak guard in unit tests is structurally weak (low)

`TestCreateClusterDoesNotEchoSecretMaterial` uses the fake store, which
returns a `ClusterSummary{}` that never carries the security ref by
design. So the test would still pass even if a future change accidentally
serialised `Security.AdminSecretRef` somewhere else (e.g. error details).
The strong guarantee comes from (a) the model having no `Security` field
on response types and (b) the runtime `assert_no_secret_leak` grep — both
of which hold today.

Suggested follow-up: add a model-level test that JSON-marshals
`CreateClusterRequest` *into a response shape* and grep-asserts absence
of secret keys, OR add a `slog` log-line capture test that confirms the
admin secret name does not appear in any logged field. Not in scope for
Phase 03.

### F3. `CreateCluster` is partially synchronous — no polling hint in 202 response (low)

`kube.Store.CreateCluster` blocks until the Keeper UnitSet is Ready, then
creates the Server UnitSet **without waiting** and returns 202 Accepted
with `Ready.Server = "0/<N>"`. This is honest, but a client receiving 202
must guess where to poll. The response carries `Resources.ServerUnitSet`,
so the polling target is derivable, but the response has no `Location`
header and no documented "poll this endpoint" hint.

Suggested follow-up: when Phase 04 introduces the healthcheck workflow,
add a `Links` field (or `Location` header) pointing to
`/api/v1/clusters/{ns}/{name}` so clients have a canonical state-poll
endpoint. Not required for Phase 03 acceptance.

### F4. Lab-node-local image sync is fragile (medium, same shape as Phase 02 F1)

`clickhouse/sync-upm-api-server-image-to-nodes.sh` ties Phase 03
acceptance to four specific lab IPs and SSH password. If the lab gets
re-IPed or the image is rebuilt without re-running the sync, the
Deployment will silently boot the previous image on whichever nodes were
not synced — and the read APIs would still appear healthy on the synced
node, masking the divergence.

Mitigation already chosen by the owner: documented in
`runtime-validation.md §7` and `clickhouse/phase-03/README.md`. Suggested
follow-up: when Phase 04+ touches the deployment manifest, switch
`image: localhost/upmio/upm-api-server:phase-03` to a registry-published
immutable digest (`@sha256:…`) and remove `imagePullPolicy: IfNotPresent`
silently masking missed pulls. Not required to start Phase 04.

## 8. Recommendation — Go / No-Go for Phase 04

**Go.** All 12 acceptance criteria are met with reproducible static
evidence (re-executed in this review: gofmt / vet / test / -race / build /
cross-build / yaml lint / bash -n) and credible runtime evidence (read
from a hard-asserting validation script that re-uses the Phase 02 SQL
gauntlet against the API-created cluster). Scope was held. The
operation-history Open Question is sealed in spec and matches the code.
RBAC enforces the "no raw Pod creation" boundary at the Kubernetes layer,
not just in code.

Suggested entry conditions for Phase 04 (none block the start):

1. Wire `internal/clickhouse` into the Phase 04 healthcheck handler so it
   stops being dead code (F1). The `Client.Query` shape is fine, but it
   currently uses `string` for password — Phase 04 should add a helper
   that reads the password from `core.CoreV1().Secrets(ns).Get(...)` per
   request so credentials are not stored in process memory beyond the
   request lifetime.
2. Phase 04 should reuse `kube.Store`'s existing label conventions when
   discovering cluster pods — do not invent a parallel "find the
   ClickHouse pod" path.
3. Phase 04 should add a `Links`/`Location` polling hint to the 202
   response from `POST /clusters` (F3) and document it in the API
   reference, since the healthcheck endpoint is the natural state-poll
   target for newly created clusters.
4. Strengthen the secret-leak unit test (F2) at the same time the Phase 04
   healthcheck endpoint is added — that endpoint is the first one that
   will need to render ClickHouse error strings, so the leak-test
   coverage should grow with it.

## 9. Changelog

| Version | Date | Changes |
|---|---|---|
| 1.0 | 2026-06-06 | Initial review against Phase 03 v0.8; PASS verdict |
