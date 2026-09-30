# 官方版本兼容合并台账

本文只记录可由 Git 历史、官方 Release tag、测试结果和脱敏部署证据复核的事实。`pending` 不得写成已完成；源码候选可以继续验证，但门禁未完成前不得创建 Release、生产镜像或宣称已上线。

## 1. 版本总表

| 官方基线 | 官方 Release commit | 私有状态 | 私有兼容节点/说明 |
| --- | --- | --- | --- |
| `v0.1.164` | `cd8bb98c44303b2c8f04c0da340447c992f0cb7d` | completed (historical) | 私有 `d5bea143b` 合并；保留聚合分组、媒体冻结、积分和首页谱系。 |
| `v0.1.168` | `99c8e4bf7564823bafbab369acab6539e734c1bb` | completed (historical) | 私有 `d30c42da` 合并；后续由 `9f1b6bae`、`e4179147`、`55ac503b`、`7e598fbb` 补齐媒体、积分同库和首页。 |
| `v0.1.169` | `26d894ef4f50645a4bf1030e378ac892f17d0223` | completed (historical) | 私有 `3da18b9dd` 合并；保留官方安全修复、定价资源和调度语义，积分/额度卡独立兼容。 |
| `v0.1.172` | `155c494964c3ea6ecc31f52679525c1034bf0f16` | completed (historical) | 私有 `d6cfece20` 合并，`62d636672` 合入 tag 后官方热修复。 |
| `v0.1.173` | `29009f0b2ea14edf3b11ae2564fb617ff91a03b4` | superseded by candidate | 其 Grok/xAI、渠道监控 V2、计费、注册限制与迁移变化已由 `v0.1.175` 兼容候选整体吸收。 |
| `v0.1.175` | `93c32fa1a2450351561abc46156d2e28cb5f74ca` | completed (historical) | annotated tag object `b898c60c422d1de059968c56aca22f6643f1fed4`；由 `v0.1.176` 兼容线承接。 |
| `v0.1.176` | `e803e3851c0a7e222cfadeafad7b8636ab959d11` | completed (historical) | annotated tag object `14e6d7ee7bdb1e4cb6bc59129a7ee1dd1110c52a`；私有 `v0.1.176-52t.1` 已由 `.4` 手工切换替代。 |
| `v0.1.183` | `e8cb019fabf8b55199436229044cbf9aa7a82564` | `.4` released; superseded in production | annotated tag object `c21fd3382a1c39fe491a96ac6780bac927327ae4`；双父合并提交 `e973f23ad474586cb607b8c6b4b6a1fa5c60c60c` 从私有 `ceb2326d740235852d9d81bbca6bee669a342130` 合入官方源码。该版本 `backend/cmd/server/VERSION=0.1.183-52t.4`，私有发布提交与 Tag 均指向 `b21d92c5239a2aabd47d867e3b3bbb311d2b4272`，GitHub run `33105459243` 全绿，Release、manifest、GHCR 双架构镜像和服务器缓存均已完成，manifest digest 为 `sha256:02ae7c6248110ddb862358701fb912202da9429ec5a535a8918c1a9bf7bf95bf`。维护者曾于 `2026-08-28 07:26:27 +08` 完成人工切换并通过长上下文计费冒烟，后续已由 `v0.2.1-52t.3` 取代。`v0.1.183-52t.3` 因 lint 门禁失败且未生成 Release 或镜像，只保留失败证据。 |
| `v0.2.1` | `ab99d56e9626e6cd731592dae8553c9758a0efa2` | `v0.2.1-52t.3` released; superseded by `v0.2.5-52t.1` | `.2` 完成首轮兼容发布，`.3` 在同一官方基线上恢复 KeyingPay V2；`.3` commit `15f24da8b0ed607a864dd4cc81f24bc2ee15b4d8`，GHCR manifest `sha256:f27bbe666ae8cb582adc6755c27b29ce2d4ba95dc0156546788b1d236ce90ff0`。`2026-09-17` 服务器只读证据当时确认该版本运行；后续 `v0.2.5-52t.1` 已切换，当前运行状态见最新运维核验。 |
| `v0.2.5` | `86f93c28ee34cc74b629dafb748bd5ac5ca8c5ea` | `v0.2.5-52t.1` released and running before `v0.2.11` candidate | 从私有 `main` `19d74426a7d41183b128e8aa3b08d79d5d89d533` 合并，官方 tag 后 53 个未发布提交只回移 `611c30f04` grpc 安全修复，其余不纳入。发布源码提交、私有 annotated Tag peeled commit 与 OCI revision 均为 `58d2b2f85f34fc11c103437b21945dae248ffe2a`；Release run `35237673572` 全绿，GHCR manifest 为 `sha256:c6c6f534fa1aa6d2e396961b0351e3ee95c2bc549fb2da5b3b878653498d1f26`。本轮升级前只读服务器确认生产运行该版本；人工切换和生产冒烟属于维护者操作。 |
| `v0.2.11` | `96f4c115c9749078f90cbf210a01d39baf3f53b6` | `v0.2.11-52t.1` compatibility candidate in progress | 从私有 `main` `3c990be5c8fe5b2105a7abfeed8a6f27679227a4` 合并；merge-base 为 `v0.2.5` commit `86f93c28ee34cc74b629dafb748bd5ac5ca8c5ea`。核心服务契约和全量门禁待完成；Release/镜像/服务器缓存/人工切换均为 `pending`。官方 `VERSION` 文件仍为 `0.2.10`，私有候选明确设为 `0.2.11-52t.1`。 |

### v0.2.11 当前兼容候选

- 官方版本重点变化：GPT-6.1 Sol 与订阅套餐识别、Claude 原生限额重置兑换、Codex 远程模型目录、API Key 数量/频率限制、Astra Ultrafast 能力识别、余额模式请求在途预占，以及内容审核元数据、推理强度渠道倍率和联盟账本操作 ID。
- 官方网关、协议、计费、调度、缓存、重试和前端作为唯一核心实现；本轮仅恢复保证既有私有积分、额度卡/提链、媒体冻结/结算、TTFT、Codex 头部保护、KeyingPay、首页/帮助契约所需的适配。
- 新增官方 SQL 迁移为 `238b_content_moderation_engine_meta.sql`、`239_channel_reasoning_effort_multipliers.sql`、`240_affiliate_ledger_operation_id.sql`。当前与私有基线比较仅新增官方迁移，不重命名或覆盖旧私有迁移；部署前仍须在 Release 记录中附完整文件名和 checksum。
- Seedance 新视频路径经维护者确认维持官方请求后计费；它不使用既有媒体冻结机制。原有图片与视频路径的冻结、失败释放和成功核销仍按私有契约运行。
- 当前源码验证：前端 lint/typecheck/build 与全量 Vitest `350` 文件、`2695` 项通过；积分系统 Go test/vet/build 通过。后端 profile/媒体服务接口仍在修复；Go 全量测试、CI、安全扫描、PR、Release、镜像和服务器缓存均为 `pending`，不得提前标记完成。

官方 `v0.1.173` tag 的源码 `VERSION` 仍为 `0.1.172`，官方 `v0.1.175` tag 树内的 `VERSION` 仍为 `0.1.173`，官方 `v0.1.176` tag 树内的 `VERSION` 仍为 `0.1.175`，官方 `v0.1.183` tag 树内的 `VERSION` 仍为 `0.1.182`，官方 `v0.2.5` tag 树内的 `VERSION` 仍为 `0.2.4`。发布审计不得只看 tag 名称，必须同时记录 annotated tag object、peeled commit、私有 `VERSION`、manifest source commit 和构建产物 revision。

### v0.2.5 / v0.2.5-52t.1 已发布候选

- 官方稳定 Release 固定为 `v0.2.5` tag object `4af0e80db1b0bc7626dfb8fb76ccaffc6bb0dc17`、peeled commit `86f93c28ee34cc74b629dafb748bd5ac5ca8c5ea`。官方 `main` 比该 Release 多 53 个未发布提交；除 `611c30f04` 的 grpc 安全修复外，本轮禁止混入。
- 私有起点固定为 `19d74426a7d41183b128e8aa3b08d79d5d89d533`，工作分支 `codex/upgrade-v0.2.5-compat`，源码版本 `0.2.5-52t.1`。PR `#13` 已合入私有 `main`；最终 merge commit 为 `58d2b2f85f34fc11c103437b21945dae248ffe2a`，annotated Tag object 为 `c776adaaf01d85389dc0bcc5abc8f9f1485b9481`，peeled commit 同为 `58d2b2f85f34fc11c103437b21945dae248ffe2a`。
- 官方优先范围包括 Responses/Chat/Anthropic/Gemini 网关、WS 连接池与执行作用域、抢占/重试、计费/缓存/调度、OpenAI OAuth Images、Grok 媒体、支付订单状态机、模型可见目录、OpenCode、MiniMax、站点类型及批量管理。私有代码只接回已记录的产品契约，不维护平行网关或计费旁路。
- 私有保留范围为积分/签到桥接、提链/额度卡资金账本与欠费恢复、图片/视频冻结释放核销、统一视频 API、真实 TTFT、`x-codex-turn-state` 单值 `48 KiB`、KeyingPay V2、可用渠道/Q 群、当前首页/帮助页/Logo。独立积分和生图工作台容器不属于本轮替换范围。
- 官方新增 `235_group_model_allowlist.sql`、`236_group_model_allowlist_repair.sql`、`237_add_minimax_platform.sql`、`238_opencode_go_platform.sql`、`238_purge_unlimited_user_platform_quotas.sql` 原名保留；两份 `238` 按完整文件名共存。私有 `173-179`、`192_media_balance_hold_reconciliation_index_notx.sql`、`193_points_balance_credit_ledger.sql`、`194_link_cards.sql` 的文件和 checksum 不变。
- `235` 会把旧 `models_list_config` 升级为请求准入白名单。候选已增加提链 Key allow/deny 回归；生产切换前仍须只读盘点所有启用旧列表的分组，确保普通 Key、提链 Key 和媒体实际使用模型均在精确项或通配规则内。
- 当前 API Key 认证快照版本为 `v24`：历史私有 `v21` 长上下文/逐模型价格字段继续保留，官方随后以 `v22`、`v23`、`v24` 依次加入免费 Fast、Codex 模型清单和 `model_allowlist`。版本提升会让旧 Redis 快照自然失效并回源重建，不需要清空 Redis，也不会修改数据库或历史账单。
- 首轮 PR 安全门禁在 `grpc v1.82.1` 命中 `GO-2026-6443` 与 `GO-2026-6348`。候选精确回移官方提交 `611c30f04fac9081d41bb6f4fa2712d8c7bbe359`，升级到 `grpc v1.83.2` 并接受 `go mod tidy` 的最小版本选择结果；没有回移该提交之外的未发布功能。
- 本轮跨数据库迁移、Ent/生成代码、后端和前端，固定为 `image-update-required`。后台二进制热更新不得安装；自动化只发布和缓存不可变镜像，生产由维护者手工 Compose 切换。
- 当前证据：26 个直接冲突已清零，Wire/Ent 重生成一致；本地后端 default/unit/integration、`go vet`、`golangci-lint v2.13.0`、`govulncheck v1.8.0`，积分 test/vet/build，前端 lint/typecheck/全量 Vitest/build，Caddy 与视频 Worker 均通过。PR、私有 `main` CI/安全扫描及 Release run [`35237673572`](https://github.com/hxly520/sub2api/actions/runs/35237673572) 全绿；Release [`v0.2.5-52t.1`](https://github.com/hxly520/sub2api/releases/tag/v0.2.5-52t.1) 已发布。GHCR manifest digest 为 `sha256:c6c6f534fa1aa6d2e396961b0351e3ee95c2bc549fb2da5b3b878653498d1f26`，amd64/arm64 manifest 分别为 `sha256:10502066ddb8880fde154cd026e265800c2207aafe3af19c5281609fab05ab1d`、`sha256:5487351ede4bc029d7e6efc2dc3f044d228b425252728f91a3014e2b2c968b05`，两平台 OCI revision 均为 `58d2b2f85f34fc11c103437b21945dae248ffe2a`。服务器仅完成候选缓存；数据库备份、维护者人工 Compose 切换和生产冒烟保持 `pending`。
- Windows 本地门禁只调整两处测试夹具，不修改生产实现：账号替代候选用例补齐官方要求的持久化冷却字段；Ollama CAS 用例基于首次冷却值构造确定性差值，避免低精度时钟把两次 `time.Now()+5s` 生成为同一时间。

### v0.2.1 / v0.2.1-52t.2 与 v0.2.1-52t.3

> 本候选已把在线更新器和 GitHub Release 客户端恢复为官方 `v0.2.1`，发布工作流则在官方 GoReleaser 结构外保留私有 Tag/VERSION/CI 安全门禁。官方在线更新器只识别 `Wei-Shaw/sub2api` Release；私有候选仅通过当前仓库精确 annotated Tag/Actions 生成 GHCR 镜像，并由维护者手工 Compose 切换。下文旧版本中的私有 manifest/热更新记录仅是历史证据，不代表当前代码仍提供该能力。

- 官方源码基线固定为 `v0.2.1` commit `ab99d56e9626e6cd731592dae8553c9758a0efa2`；升级前生产基线固定为 `v0.1.183-52t.4`。私有发布 commit 与 Tag 指向 `102c921eeb09f75614313c8561f7cd0ca9d87e6e`，annotated tag object 为 `83fddf24baa11fccd90b0a559e87aa7eb6b20057`；GitHub Actions run [`33995383939`](https://github.com/hxly520/sub2api/actions/runs/33995383939) 全绿，Release 已发布，GHCR manifest digest 为 `sha256:90d4a5b70ea6155e12aaf90291b24ea128a74cf44596d80366203b5994462c80`，amd64/arm64 OCI revision 均为该 commit。生产切换与服务器拉取仍为 `pending`。
- 官方优先范围为网关、协议转换、重试、计费、缓存和账号调度。私有代码不得在这些核心路径维护另一套广泛重放或计费旁路；只在官方入口接回积分、提链/额度卡、媒体冻结/释放/核销、视频、真实 TTFT、Codex turn-state 单值 `48 KiB` 限制、当前首页及帮助页所需的最小兼容逻辑。
- 官方 `231-234` 迁移与私有 `173-179`、`192-194` 迁移继续按完整文件名和 checksum 共存。`231`、`232`、`233`、`234` 均存在同号文件的可能，迁移审查和执行不得按数字前缀覆盖、重命名、合并或跳过；源码和 CI 迁移校验已通过，数据库演练与生产迁移仍为 `pending`。
- 本轮包含官方迁移、后端/生成代码与前端跨度，策略固定为 `image-update-required`。后台二进制热更新不得安装该候选；自动化最多发布并核验不可变镜像，生产只允许维护者在数据库备份和回滚点确认后手工 Compose 切换。
- `.github/workflows/release.yml` 只接受 `vX.Y.Z-52t.N` annotated Tag；Tag 树内 `backend/cmd/server/VERSION` 必须与 Tag 去掉前导 `v` 后一致，并先通过同一精确 Tag 的 `backend-ci.yml` 与 `security-scan.yml`。校验或门禁失败时不运行 GoReleaser、不发布 Release/GHCR，也不改写默认分支。
- 当前证据状态：源码最终收口 completed；`git diff --check` completed；后端/前端/积分及保护模块定向与全量测试 completed；GitHub Actions completed；Release completed；GHCR 镜像与 digest/OCI revision completed；服务器拉取、生产切换与生产冒烟 pending。任何一项均不得引用 `v0.1.183-52t.4` 的旧证据代替。

### v0.1.183

- 本轮工作分支为 `codex/upgrade-v0.1.183-compat`，私有起点为 `ceb2326d740235852d9d81bbca6bee669a342130`，官方 tag object 为 `c21fd3382a1c39fe491a96ac6780bac927327ae4`，peeled commit 为 `e8cb019fabf8b55199436229044cbf9aa7a82564`，merge-base 为官方 `v0.1.176` peeled commit `e803e3851c0a7e222cfadeafad7b8636ab959d11`。最终源码合并节点 `e973f23ad474586cb607b8c6b4b6a1fa5c60c60c` 的两个父提交依次为该私有起点与官方 peeled commit。`v0.1.183-52t.1` Tag 已推送到提交 `6d46eebe501ee5a816c8b51fee37390146c06662`，但质量门禁失败，未创建 Release 或 GHCR 镜像；`v0.1.183-52t.2` annotated Tag 指向 `cc2165e5fd6a14685700eb6c3607a5fb51baee09`，其 manifest source commit 与 amd64/arm64 OCI revision 均与之完全一致，作为上一候选的已发布证据保留。
- 当前 `.4` 收口以官方网关、计费、重试和协议终态为优先基线：Responses/Chat/Anthropic/Gemini/WS 的官方转发与 usage 结算路径不再被私有广泛重放或首响应故障切换旁路覆盖。积分、额度卡/提链、媒体冻结、生图/视频、长上下文计费、公开首页/帮助及私有更新源继续保留，冲突时迁移私有约束到官方入口并保留回归测试。
- 客户端断开是请求终止边界：服务端停止下游写入并关闭重放窗口，只在有界 drain 中收集真实 usage；断开后的 EOF、读取错误、容量错误或缺终态不得发起第二次上游请求。OAuth 429 的官方恢复窗口继续存在，但每个请求受 request-local retry limit/`SameAccountRetryMax` 约束，专用账号不得在 deadline 内无限同账号重试。
- `x-codex-turn-state` 只接受 trim 后不超过 `48 KiB` 的单值，空值和超限值丢弃；Nginx API 示例同步设置 `proxy_buffer_size 128k` 与 `proxy_buffers 8 128k`。该边界已通过 `.4` 发布门禁，并随生产切换完成只读冒烟复核。
- 官方插件出站传输、国产供应商 Kimi/Zhipu/DeepSeek 一等支持、复合分组、渠道监控配额模式、分组用量汇总、Codex 指纹种子、OpenAI Responses/WS 与调度修复按官方实现优先。官方 Fast/Flex service tier、渠道倍率和分时段/仅工作日定价继续共用统一计费解析，不另建私有旁路。
- 私有积分/签到与同库隔离、提链/额度卡资金账本和后扣准入、媒体冻结/核销、统一视频兼容、公开首页/帮助、私有 Release 热更新源、跨协议终态校验和精确容量拒绝有界重试均保留。发生同类实现冲突时使用官方入口、结构和调度语义，再移植私有约束及回归测试。
- API Key 认证快照继续使用私有 `v21`，双向保存 `LongContextPricingEnabled` 与 `ModelPricing`。长上下文按分组开关或账号开关任一开启的 OR 语义生效；渠道显式区间优先，不能再叠加官方长上下文倍率。生产同型 GPT-5.6 回归固定验证：账号 `openai_long_context_billing_enabled=true` 即使分组开关关闭也可启用；渠道无显式区间价且未缓存输入、cache write、cache read 合计严格大于 `272000` 时，三类输入费用均为 `2x`、输出费用为 `1.5x`，分组倍率只在总费用上乘一次，并写入 `long_context_billing_applied=true`。`backend/internal/service/openai_gpt56_long_context_billing_test.go` 覆盖 HTTP Responses、WebSocket HTTP bridge 与 WebSocket v2；Fast/Flex、分时倍率和长上下文档位仍须通过同一费用明细验证，禁止重复计费。
- 官方新增迁移 `222-230` 均原名保留；`225_backfill_codex_fingerprint_seed.sql` 与 `225_channel_model_time_pricing.sql`、`226_add_usage_log_effective_model_indexes_notx.sql` 与 `226_channel_monitor_quota_mode.sql` 分别同号共存。此前三份 `194` 及私有 `173-179`、`192-194` 继续保留，迁移 runner 只按完整文件名和 checksum 判定，不得按数字前缀覆盖、改名或合并。
- 本轮跨越数据库迁移、Ent/生成代码、后端、前端和发布资产，固定为 `image-update-required`。私有 Tag `v0.1.183-52t.4` 的 message 必须包含 `[image-update-required]`；后台热更新不得安装，GitHub Actions 只能构建并发布不可变 GHCR 镜像，生产 Sub2API 必须由维护者在备份和回滚点确认后手动执行 Compose 切换。Release 工作流必须先证明 Tag 与默认分支存在祖先关系，并校验 Tag 树内 `VERSION` 完全一致；发布过程不得修改或推进代表当前生产版本的 `main`。
- 上一 `.2` 的本地门禁及 GitHub run `33069107472` 已完成并仅作历史证据；`.3` 的 run `33103107425` 因 8 个未使用旧函数触发 lint 门禁而失败，未生成 Release 或 GHCR；`.4` 的本地门禁、GitHub run `33105459243`、Release 附件、manifest、OCI revision、GHCR digest、服务器缓存、维护者人工切换和生产长上下文计费冒烟均已完成，不得以 `.2` 或 `.3` 的证据代替 `.4` 证据。数据库备份是否在本次切换窗口完成不由本次只读账单核验推断，仍按维护者备份记录复核。

### v0.1.183-52t.4 生产计费只读复核（2026-08-28）

- 当前容器为 `ghcr.io/hxly520/sub2api:0.1.183-52t.4`，manifest digest `sha256:02ae7c6248110ddb862358701fb912202da9429ec5a535a8918c1a9bf7bf95bf`，状态 `healthy`、restart count `0`。
- 新容器启动后截至 `07:59:56 +08`，`gpt-5.6-sol` 共 `138` 条记录；上下文严格大于 `272000` 的 `8` 条全部写入 `long_context_billing_applied=true`，漏标记 `0` 条。8 条均对应唯一 `usage_billing_dedup` 记录，未发现重复扣费。
- 记录 `usage_logs.id=616232` 的输入与两类缓存按 `2x`、输出按 `1.5x`，分组倍率只应用一次；详见 [`PRODUCTION_OPERATIONS_CN.md`](PRODUCTION_OPERATIONS_CN.md) 第 0.13 节。该复核只覆盖切换后的新请求，历史账单不自动回写。

### v0.1.176

- 本轮从私有 `v0.1.175` 兼容收口点合入官方 annotated tag `v0.1.176`，官方 tag object 为 `14e6d7ee7bdb1e4cb6bc59129a7ee1dd1110c52a`，peeled commit 为 `e803e3851c0a7e222cfadeafad7b8636ab959d11`；合并过程中保留私有媒体路由并接入官方 `/x_search`。
- 官方 Grok 4.6/订阅档位识别、Grok 模型配额、分组逐模型定价、长上下文阶梯开关和原生 `x_search` 按官方实现优先；私有缓存、账号调度、媒体冻结/核销、积分、额度卡/提链、首页/帮助和协议终态回归继续保留。
- 视频计费沿用官方分组/渠道逐模型定价解析；旧视频字段保持兼容。`minimax-h3-2k` 继续由现有视频分组、倍率和 `/v1/videos` 透传链路承载，不新增专属模型族或计费规则；回归测试覆盖 `2k`、15 秒、5 图和 3 音频统一 JSON 请求。
- 长上下文阶梯采用官方 `5b2a386ed` 的 OR 语义：分组开关或账号开关任一开启即生效，账号 `false` 不否决已开启的分组。渠道显式区间仍优先且不再叠加官方倍率；两者都关闭时只选择最低渠道区间或内置基础价。
- 官方 `674570ca1` 补齐认证快照中的 `LongContextPricingEnabled` 与 `ModelPricing`。私有 `v20` 已用于提链及 search/audio/video 字段，因此兼容实现使用 `v21`，使旧 Redis 快照立即失效并从数据库重建；不需要清空 Redis，也不修改数据库或历史账单。
- 新增 `backend/migrations/221_group_model_pricing.sql`，与既有迁移按完整文件名和 checksum 独立执行。由于数据库、Ent schema、前端和二进制均有变化，本轮发布策略为 `image-update-required`，不得使用后台热更新代替 Compose/GHCR 镜像切换。
- 原 `v0.1.176-52t.1` 发布门禁已通过并由维护者完成生产切换。`2026-08-26` 热修复已通过认证快照、开关矩阵、渠道区间和生产同型 GPT-5.6 长上下文定向测试；新 Release、GHCR digest 和生产切换仍为 `pending`，不得把源码修复状态写成已上线。

## 2. 已完成合并结论

### v0.1.164

- 私有合并保留官方聚合分组、Ollama Cloud 用量、支付入口和安全修复。
- 私有媒体任务遵守“创建最多一次、失败/空结果退款、未知终态不擅自退款”；该约束在后续版本继续继承。
- 旧 v164 生产节点只作为历史回滚谱系，不能作为当前版本判断依据。

### v0.1.168

- 官方 Passkey、模型广场、Kimi K3、声明列更新和协议/安全修复按官方实现为主。
- 私有媒体核销、积分同库隔离、视频统一接口、协议终态、可见渠道、公开首页和余额缓存保护保留。
- 迁移、积分 schema、独立工作台和宿主 Nginx 不得因镜像更新被覆盖或重跑。

### v0.1.169

- 官方上游 URL 路径校验、`no-new-privileges`、价格 fallback、调度熔断和 GPT-5.6 价格更新保留。
- 私有积分策略、签到金额安全上限、邮箱列级 ACL、中文双工作区、登录邮箱展示、容量错误精确重试和额度卡原子计费保留。
- 官方同类行为冲突时以官方结构为主；私有测试必须移植到官方入口，而不是并行维护两套实现。

### v0.1.172

- `d6cfece20` 的第二父节点是官方 `v0.1.172`；`62d636672` 继续合入 tag 后 OAuth/路由提示等官方修复。
- 私有最终节点 `0948f0191c18045d8d04ccbf275ac4688d2c39af` 及其后文档/额度卡修复保留：媒体冻结回收、Redis fail-close、跨协议正式终态、提链 Key 资金与并发门禁、积分同库和公开首页。
- 迁移 runner 按完整文件名和 checksum 识别；私有 `194_link_cards.sql` 与官方同号迁移不得重命名、覆盖或按数字前缀猜测。
- 候选镜像和生产切换必须分别记录；自动化不得替换 Sub2API 容器。

### v0.1.175

- 本轮从私有起点 `689627207313a24cd5e445dd46cf3552fd3d9bc0` 合入官方 annotated tag `v0.1.175`，merge-base 为 `cc67b1aca1d3b590609abef2fcd3a6ca31c5c651`。Git 冲突已全部解决，并以保留官方父节点的双父 merge commit 收口；发布身份以最终私有 Tag 指向的完整 commit、manifest 和 OCI revision 为准。
- 官方 Grok/xAI、渠道监控 V2、响应模型计费、Gemini 图片统计、音频/搜索/视频价格、注册/凭证安全和 Codex 指纹能力保留；私有积分、提链、媒体冻结、容量精确重试、首页/帮助和私有更新源均保留。
- OpenAI 首输出状态机同时满足三条约束：空完成响应 failover；metadata-only EOF 不写下游并返回 `UpstreamFailoverError`；reasoning/item 结构进度解除超时但不提前记录 TTFT。旧身份测试夹具已补真实 output/usage，生产空响应保护没有放宽。
- 三份 `194` 迁移按完整文件名共存：`194_add_usage_log_upstream_response_model.sql`、`194_channel_monitor_v2.sql`、`194_link_cards.sql`。官方 `195-206`、`217-220` 也原名保留，发布策略固定为 `image-update-required`。
- 首轮验证曾通过本地门禁，但 Tag CI 暴露了两处测试调用签名落后、Grok usage 错误未处理、格式问题，以及容量重试重新生成 Codex 指纹导致请求体变化。`v0.1.175-52t.1` Release 已取消且无 GHCR 产物；修复候选需重新执行完整门禁，不能沿用首轮结论。生产数据库迁移和上线冒烟仍待维护者在 Compose 窗口完成。

## 3. v0.1.183 兼容矩阵与发布缺口

在创建私有兼容分支前，逐项填写状态和测试证据：

| 范围 | 官方变化 | 私有风险 | 处理状态 |
| --- | --- | --- | --- |
| 官方协议与调度 | Responses/WS、容量恢复、OpenAI/Kimi/Antigravity 状态与复合分组修复 | 私有终态、粘性、容量精确重试不得放宽或重复重放 | source merged；本地定向与 GitHub 全量门禁通过 |
| 国产供应商与插件 | Kimi/Zhipu/DeepSeek 多协议、配额/余额监控、OAuth 出站插件 | 不得绕过分组、账号、凭据和私有响应脱敏边界 | source merged；本地路由/权限/插件及 GitHub 全量门禁通过 |
| 计费 | Fast/Flex、渠道倍率、分时时段/工作日、上下文阶梯和统一 Token 路径 | `v21` 快照、OR 开关、渠道区间优先、提链倍率和媒体报价不能重复计费 | source merged；本地计费矩阵及 GitHub 全量门禁通过 |
| 迁移 | 官方 `222-230`，其中 `225`、`226` 各有同号文件 | 私有 `173-179`、`192-194` 与三份 `194` 不得丢失或改 checksum | source merged；按完整 filename/checksum 共存；数据库演练 pending |
| 产品与前端 | 模型广场、渠道监控、插件和管理页面 | 积分、提链、首页/帮助和嵌入导航不得覆盖 | source merged；本地与 GitHub 前端门禁通过 |
| 发布 | Tag、manifest、GHCR、数据库备份和人工切换 | 不能把源码候选或镜像构建误当生产 | Tag/Release/GHCR/服务器缓存 completed；策略为 `image-update-required`；备份及维护者手动 Compose 切换 pending |

### v0.1.183 发布门禁

1. 以双父合并提交 `e973f23ad474586cb607b8c6b4b6a1fa5c60c60c` 为源码节点执行并记录 `git diff --check`、后端全量测试/vet 和前端 lint/typecheck/Vitest/build；最终发布提交允许只追加门禁/文档收口，但不得改写该合并谱系。
2. 使用 Go `1.27.0` 运行积分系统全量测试，并定向覆盖 `v21` 快照、GPT-5.6 长上下文阈值/倍率/跨协议矩阵、长上下文 OR 开关、渠道区间不重复计费、Fast/Flex/分时计价、提链资金守恒、媒体冻结释放、视频兼容、容量精确重试、首页和帮助入口。
3. 创建私有 annotated Tag 与 `update-manifest.json`，Tag message 必须包含 `[image-update-required]`；确认 Tag `^{commit}`、manifest source commit 和二进制/OCI revision 一致。
4. GitHub Actions run `33105459243` 已全绿并发布不可变 `ghcr.io/hxly520/sub2api:0.1.183-52t.4`；manifest digest 为 `sha256:02ae7c6248110ddb862358701fb912202da9429ec5a535a8918c1a9bf7bf95bf`，OCI revision 为 `b21d92c5239a2aabd47d867e3b3bbb311d2b4272`，服务器缓存已完成。不得通过后台热更新安装，也不得由自动化替换生产 Sub2API 容器。
5. 维护者切换前备份数据库并核对私有迁移及官方 `194-230` 的完整 filename/checksum；维护者手动 Compose 切换后再验证积分、提链、媒体冻结、视频、渠道计费、OpenAI/Grok/Gemini/CN 供应商、插件和生产健康。
6. 发布观察期内持续检查未结算 claim、冻结余额、提链在途额度、异步任务终态和长上下文/Fast/Flex/分时费用明细；异常时回滚镜像，但不得回退已执行的 forward-only 迁移。

## 4. 记录模板

每次官方兼容合并追加一节，至少包含：

```text
官方 Release/tag：vX.Y.Z / OFFICIAL_COMMIT
私有分支与合并提交：BRANCH / PRIVATE_COMMIT
merge-base：MERGE_BASE
迁移：新增、冲突、checksum、是否 forward-only
官方优先冲突：文件/行为/测试证据
私有保留功能：代码入口与回归测试
发布策略：hot-update-safe | image-update-recommended | image-update-required
镜像与回滚：GHCR_TAG、DIGEST、BACKUP、数据库兼容边界
未解决问题：明确负责人和阻断条件
```

没有证据的项目保持 `pending`，不得以“看起来兼容”结案。
