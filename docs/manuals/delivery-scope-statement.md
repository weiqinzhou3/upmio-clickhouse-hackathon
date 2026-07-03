# 黑客松交付边界说明

- Version: 0.1
- Date: 2026-07-03
- Status: For judge / reviewer clarification
- Scope: Clarify implemented delivery scope versus future roadmap

## 1. 结论

当前工程化交付范围是 Phase 01 到 Phase 07。

Phase 01 到 Phase 07 已完成实现、运行时验证、review 修复、closeout，并已
合并到 `main`。

Phase 08 到 Phase 12 没有作为当前黑客松 MVP 的实现交付项完成。它们是对
第一阶段 Spec 中未纳入当前 MVP 的 Day2/生产化能力所做的未来迭代规划，
不是遗漏、延期失败或未完成的当前交付。

## 2. 为什么 Phase 08 到 Phase 12 不是当前交付缺口

黑客松第一阶段交付的 Spec 采用 MVP 优先原则：

- 先完成可运行、可演示、可验收的最小闭环；
- 将不适合当前 MVP 的高风险或生产化能力明确映射到未来阶段；
- 不在 Demo 阶段承诺未经过真实 Kubernetes 验证的能力。

因此，当前第二阶段 Demo 的交付重点是：

- UPMIO 方式部署 ClickHouse；
- `upm-api-server` 产品 API；
- 健康检查；
- Prometheus / Grafana 监控闭环；
- Day2 只读诊断；
- 即时备份；
- 恢复；
- 定时备份；
- 运行时验收脚本和证据。

这些能力均已在 Phase 01 到 Phase 07 中完成。

## 3. 已完成交付

| Phase | 当前状态 | 是否属于本次可验收交付 |
|---|---|---|
| Phase 01 | 已实现 | 是 |
| Phase 02 | 已实现 | 是 |
| Phase 03 | 已实现 | 是 |
| Phase 04 | 已实现 | 是 |
| Phase 05 | 已实现 | 是 |
| Phase 06 | 已实现 | 是 |
| Phase 07 | 已实现 | 是 |

## 4. 未来迭代方向

| Phase | 当前状态 | 说明 |
|---|---|---|
| Phase 08 | 未来规划入口 | 配置变更、生命周期、扩缩容的设计边界，不是当前实现交付 |
| Phase 09 | Draft | Demo Web Console 前端，计划能力 |
| Phase 10 | Draft | 版本升级、资源调整、存储扩容，计划能力 |
| Phase 11 | Draft | Replica / Shard 拓扑扩展，计划能力 |
| Phase 12 | Draft | 备份保留、操作历史、审计、认证/RBAC，计划能力 |

## 5. 评审时建议表述

建议在答辩或文档中这样表述：

```text
本次黑客松第二阶段的可运行 MVP 已完成 Phase 01 到 Phase 07。它覆盖了
UPMIO 部署、API 管理、健康检查、监控、Grafana、诊断、备份、恢复和
定时备份，并保留了真实 K8s 验收脚本和证据。

Phase 08 到 Phase 12 是第一阶段 Spec 方法论下对未来生产化能力的路线图，
包括配置变更、生命周期、升级、扩容、前端、审计和权限等。这些能力未被
包装成当前已实现功能，也不会在 Demo 中宣称已支持。
```

## 6. 不应宣称已经支持的能力

以下能力当前不能作为已实现功能宣称：

- 自定义前端控制台；
- 版本升级或滚动升级；
- CPU / 内存 / IO / 存储在线扩容；
- 滚动重启；
- 配置变更和回滚；
- 在线新增 replica；
- 在线新增 shard；
- 历史数据 resharding / 自动重分布；
- 备份保留自动清理；
- 持久化操作历史；
- 审计和认证/RBAC；
- 用户/角色/Profile/Quota 全生命周期管理。

这些能力已经被记录为未来规划或候选阶段，不能被理解为当前未完成项。

## 7. 当前可验收能力清单

当前可以通过脚本、API 和运行时证据验收的能力包括：

- UPMIO `UnitSet` 部署 ClickHouse / Keeper；
- `2 shards x 2 replicas + 3 Keeper` 运行时验证；
- `upm-api-server` Kubernetes 内部署；
- 集群创建、列表、详情、资源视图；
- 健康检查；
- Prometheus 指标摘要；
- Grafana Dashboard 验证；
- Day2 只读诊断；
- 即时备份；
- 恢复；
- 定时备份；
- 任务状态查询；
- 完整 Demo Runbook；
- AI 使用证据和 review 修复记录。
