# Phase 06 Review Report

- Version: 1.0
- Date: 2026-06-07
- Reviewer: Claude (Opus 4.7)
- Owner: 周钦伟 (zqw)
- Scope: Review of Codex's Phase 06 implementation against
  `docs/phases/phase-06-day2-diagnostics.md` v0.5
- Verdict: **PASS — proceed to Phase 07**（5 条非阻塞发现见 §7）

## 1. Phase 06 背景

Phase 06 实现了只读 Day2 诊断功能，覆盖 ClickHouse 的副本、复制队列、分区
(Parts)、合并 (Merge)、变更 (Mutation)、Keeper 状态、存储容量、写入客户端统计
以及写入质量校验。所有诊断均为只读查询，不执行任何修复操作。

范围外（spec §3）：

- 备份/恢复、终止变更、OPTIMIZE 执行、TTL 变更
- 用户业务数据修正、自动分片/副本修复
- 高风险写入操作

## 2. Review 方法

1. 重读 `docs/phases/phase-06-day2-diagnostics.md` v0.5 及相关设计文档
2. 检查所有变更文件
3. 在 review 环境中重新执行质量门：
   - `gofmt -l .`（通过）
   - `go vet ./...`（通过）
   - `go test -race -count=1 ./...`（全部 PASS）
   - `go build ./...`（通过）
   - `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build ./cmd/upm-api-server`（通过）
   - `bash -n` on validation script（通过）
4. 交叉验证运行时证据

## 3. 验收标准核对

| # | 标准 | 状态 | 证据 |
|---|---|---|---|
| 1 | `/diagnostics` 返回结构化输出 | PASS | `model/diagnostics.go` 定义完整模型。`server.go:235-258` 注册路由并处理请求。`diagnostics.go:17-66` 编排所有诊断类别 |
| 2 | 副本诊断查询 `system.replicas` 并分类 readonly/session-expired | PASS | `diagnostics.go:68-156`：查询 `clusterAllReplicas` 的 `system.replicas`，检查 `is_readonly`/`is_session_expired` → CRITICAL，检查 `absolute_delay`/`queue_size` 与阈值的比较 → WARN/CRITICAL |
| 3 | 分区诊断按表/分区分组活跃/非活跃分区 | PASS | `diagnostics.go:231-318`：同时查询按表汇总和按分区分组，分别校验 inactive parts 和 active parts per partition 阈值 |
| 4 | 变更诊断识别未完成或失败的变更 | PASS | `diagnostics.go:377-447`：查询 `system.mutations`，`is_done=0` 且超时 → WARN，`latest_fail_reason != ""` → CRITICAL |
| 5 | 合并诊断列出正在运行的合并 | PASS | `diagnostics.go:320-375`：查询 `system.merges`，elapsed 超阈值 → WARN |
| 6 | Keeper 诊断报告 leader/follower 状态或 UNKNOWN | PASS | `diagnostics.go:449-481`：exec `mntr` 命令，解析角色，验证恰好一个 leader。leader != 1 或有失败 → CRITICAL |
| 7 | 存储诊断包含 PVC 状态/容量 | PASS | `diagnostics.go:483-533`：列出 PVC，提取 phase/capacity/storageClass/volumeName。未绑定或数量不对 → CRITICAL |
| 8 | 写入客户端统计：query_log 不可用时返回 UNKNOWN | PASS | `diagnostics.go:535-583`：先检查 `EXISTS TABLE system.query_log`，不存在则返回 UNKNOWN 加上解释。存在则查询近 24h Insert 记录。运行时脚本独立验证：`system.query_log` 不存在 → assert UNKNOWN |
| 9 | 写入质量校验有 API/设计钩子和只读行数机制 | PASS | `diagnostics.go:585-638`：需要 database + table 参数，支持 partition/timeColumn/startTime/endTime/expectedRows。查询 `SELECT count()`，与 expectedRows 比较。无参数 → UNKNOWN，匹配 → INFO，不匹配 → WARN |
| 10 | 诊断查询不修改数据 | PASS | 所有 SQL 均为 `SELECT`。`clusterAllReplicas` 仅读取。无 INSERT/TRUNCATE/ALTER/DROP 语句。`diagnostics.go` 中零写入操作 |
| 11 | 高风险操作仅为建议，不执行 | PASS | 每个 finding 都有 `recommendation` 字段和 `humanReviewRequired` 标记。代码中无 kill/optimize/delete 逻辑 |
| 12 | 单元测试覆盖严重性分类逻辑 | PASS | `diagnostics_test.go`：`TestDiagnosticsReportStatusAggregation`（INFO→PASS, WARN→WARN, CRITICAL→FAIL, UNKNOWN→UNKNOWN）、`TestDiagnosticsFilterValidation`（limit/severity/timeColumn 校验）、`TestDiagnosticsSeverityFilter`（按 severity 过滤 finding） |
| 13 | 运行时验证调用真实 NodePort API | PASS | 脚本保存 `diagnostics.json`、`diagnostics-rowcount.json`、4 个 `.tsv` SQL 直出证据 |
| 14 | 健康集群返回所有必需类别且无 CRITICAL | PASS | 脚本 assert 全部 9 个类别存在，`[.findings[] \| select(.severity == "CRITICAL")] \| length == 0`。运行时报告确认 14 passed / 0 failed |
| 15 | query_log 不可用时 write_client_stats 返回 UNKNOWN | PASS | 脚本检查 `EXISTS TABLE system.query_log`，为 `0` 时 assert `write_client_stats.severity == "UNKNOWN"` |

全部 15 条标准通过。

## 4. 代码质量检查 — Review 中重新执行

```text
gofmt -l .                                                          (clean)
go vet ./...                                                        (clean)
go test -race -count=1 ./...                                        (6 packages PASS)
go build ./...                                                      (clean)
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build ./cmd/upm-api-server (ELF/static)
bash -n clickhouse/phase-06/scripts/validate-diagnostics-runtime.sh (OK)
```

测试配置变化：

```text
Package              Tests  Status
internal/api         13     PASS — +2 (TestRunDiagnostics, TestRunDiagnosticsRejectsInvalidFilter)
internal/kube        12     PASS — 不变（diagnostics.go 无直接单测）
internal/model        9     PASS — +3 (聚合 / 过滤校验 / 严重性过滤)
internal/prometheus   2     PASS
```

要点审查：

- **SQL 注入防护**：`diagnosticsWhere` 中使用 `clickHouseStringLiteral` 将所有用户输入的 filter 值用单引号包裹并转义 `\` 和 `'`。表名/列名使用 `clickHouseIdentifier` 用反引号包裹并转义 ``` `` ```。用户提供的 filter 值仅作为 SQL 字面量出现在 WHERE/GROUP BY 中，不是原始拼接。
- **集群范围查询**：所有 SQL 使用 `clusterAllReplicas('upm_cluster', ...)`，确保诊断覆盖所有分片和副本，而非仅检查被选中的 Pod。
- **Query log 存在性检查**：`addWriteClientStatsDiagnostics` 先检查 `EXISTS TABLE system.query_log`，不依赖错误消息推断。优雅降级。
- **失败的诊断查询不报 API 错误**：`addDiagnosticQueryFailure` 返回 `UNKNOWN` finding 而非 error，确保单个 SQL 失败不影响其他诊断类别。符合 spec 的"只读观察"理念。
- **阈值可配置**：8 个环境变量 → `Config.DiagnosticsThresholds` → `Store.diagnosticsThresholds` → `report.Thresholds`。`WithDefaults()` 确保零值回退到规范默认值。
- **filter 校验两次**：在 `diagnosticsFilterFromQuery`（server.go）和 `RunDiagnostics` 开头（diagnostics.go）各调用一次 `Validate()`。冗余但防御性正确。
- **`Finalize` 支持 severity 过滤**：先过滤 finding（`filter.Severity != ""`），再聚合统计和状态。过滤后的报告状态仅基于保留的 finding。
- **`copyStringAnyMap` 避免数据共享**：在收集 issues 时复制 map，防止后续修改意外影响 evidence。
- **SQL 转义正确性**：`clickHouseStringLiteral("test'value")` → `'test\'value'`，`clickHouseIdentifier("test`name")` → `` `test``name` ``

## 5. 运行时证据 Review

validation script（106 行）验证以下完整链路：

1. **直接 SQL 证据**：kubectl exec 执行 4 条直连 SQL（system.replicas / parts / mutations / merges），保存 TSV 文件
2. **诊断 API**：GET `/diagnostics?limit=20`，验证 9 个必需类别全部存在、无 CRITICAL finding、无 secret 泄露
3. **Query_log 回退**：独立检查 `EXISTS TABLE system.query_log`，为 0 时 assert `write_client_stats` 为 UNKNOWN
4. **行数校验**：从 `upm_healthcheck.dist_events` 读取实际行数，作为 `expectedRows` 参数调用 diagnostics API，验证 `write_quality` 报告 INFO 且 actualRows == expectedRows
5. **Secret 扫描**：两次（diagnostics 和 rowcount 输出）均 grep secret 模式

运行时证据可信。非匹配 expectedRows 的 WARN 路径未直接测试（需要不同于实际行数的 expectedRows 值），但这属于手动测试场景。

## 6. 已解决的之前 Phase 发现

| 发现 | 状态 |
|---|---|
| Phase 04 F1: `internal/clickhouse` dead code | **已解决**（Phase 05 删除） |
| Phase 04 F3: Secret scan 仅覆盖 3 个模式 | **Phase 05 已加强**（实际 Secret 值重写） |
| Phase 05 F1: container="clickhouse" 硬编码 | **未解决** — 不影响 Phase 06 |
| Phase 05 F3: Grafana dashboard 重复 | **未解决** — 不影响 Phase 06 |

## 7. 发现 — 非阻塞

### F1. `parseInt`/`parseInt64`/`parseFloat` 在解析失败时静默返回 0（低）

`diagnostics.go:692-705` 中的三个解析函数在 `strconv` 失败时返回 0。对于 `is_readonly=0`、`queue_size=0` 等正常情况，这没问题。但如果 ClickHouse 返回非数字值（如 `NULL`、`\N` 或损坏数据），会返回 0 从而可能掩盖问题。

目前这些值来自 ClickHouse TSV 格式，数字列通常不会返回非数字值，所以实际风险很低。但 `parseInt("")`（空字符串）也会返回 0，这可能发生——例如当 TSV 行中某列为空时。

**建议后续**：将解析函数改为返回 `(int, error)` 并在调用处记录 UNKNOWN finding，或至少在列值不为空但解析失败时记录。

### F2. `diagnostics.go` 中部分 纯函数 缺少单测（低）

以下函数没有直接单元测试（与 Phase 04 F2 / Phase 05 F2 相同模式）：

- `diagnosticsWhere` — WHERE 子句构造
- `clickHouseStringLiteral` / `clickHouseIdentifier` — SQL 转义
- `diagnosticSeverityRank` / `maxDiagnosticSeverity` — 严重性排序
- `parseInt` / `parseInt64` / `parseFloat` — 数字解析
- `copyStringAnyMap` — map 浅拷贝

这些是纯函数，不需要 Kubernetes 客户端即可测试。尤其是 `clickHouseStringLiteral` 的转义正确性对 SQL 注入防护至关重要。

**建议后续**：在 `diagnostics_test.go` 中增加这些纯函数的表驱动测试。

### F3. 过滤校验在执行了两次（信息性）

`filter.Validate()` 在请求处理中被调用了两次：

1. `server.go:diagnosticsFilterFromQuery` 末尾
2. `diagnostics.go:RunDiagnostics` 开头

这是防御性正确但轻微冗余。如果将来 `Validate()` 有副作用，重复调用可能产生意外行为。当前无副作用，因此纯粹是风格问题。

**建议后续**：移除非规范路径中不需要的一次调用。

### F4. `addWriteClientStatsDiagnostics` 中 tables 列使用 `arrayStringConcat` 可能在极端情况下被截断（低）

`diagnostics.go:549` 使用 `arrayStringConcat(tables, ',')` 聚合表名。当 INSERT 涉及大量表时，ClickHouse 可能截断此字段。当前在多租户场景可能产生影响，但在当前单集群 demo 中不构成实际问题。

**建议后续**：添加 `LIMIT` 或使用 `any(tables)` 限制，避免超大表名列。

### F5. `RunDiagnostics` 编排代码无直接单测（中 — 与 Phase 04 F2 / Phase 05 F2 一致）

`diagnostics.go:17-66` (`RunDiagnostics`) 编排全部 9 个诊断类别，但 `kube/store_test.go` 中无覆盖。handler 测试通过 `fakeStore` 返回合成报告，不测试实际编排逻辑。

最有价值的未测试逻辑：Pod 列表失败时 Keeper/Storage 诊断仍可运行（有 pod 才跑 SQL 诊断）的降级路径、Query failure → UNKNOWN finding 而非 API error 的语义。

**建议后续**：与 Phase 04/05 的建议一致 —— 为编排逻辑添加表驱动测试。

## 8. 命令

**推荐通过，进入 Phase 07。** 全部 15 条验收标准通过，质量门全部通过（gofmt / vet / test -race / build / cross-build / bash -n），运行时证据可信。所有 SQL 查询均为只读 SELECT，用户输入经过正确的 ClickHouse 字面量转义，query_log 不可用时优雅降级。实现严格遵循 "观察但不操作" 的设计理念。

Phase 07 建议前置条件（不阻塞启动）：

1. 为 `diagnostics.go` 中的纯函数增加单测（F2）—— 特别是 SQL 转义函数
2. 为 `RunDiagnostics` 编排逻辑增加表驱动测试（F5）
3. 考虑给 `parseInt` 系列函数加上错误返回值（F1）

## 9. 变更日志

| Version | Date | Changes |
|---|---|---|
| 1.0 | 2026-06-07 | Phase 06 v0.5 初始 review；PASS |
