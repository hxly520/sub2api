# 2026-07-31 积分激活与后续候选交接

本文记录 `2026-07-31 CST` 已确认的生产事实、当前源码候选和下一次发布边界。它不保存密码、数据库连接串、HMAC secret、会话密钥、证书私钥或原始日志。`2026-07-30` 的首次同库部署证据仍保存在 [`PRODUCTION_DEPLOYMENT_20260730_CN.md`](PRODUCTION_DEPLOYMENT_20260730_CN.md)，但其中旧积分镜像和 disabled policy 不再代表现状。

## 1. 版本边界

| 对象 | 当前生产 | 当前源码/下一候选 | 允许的动作 |
| --- | --- | --- | --- |
| Sub2API | `0.1.168-339422728b2c`，revision `339422728b2ceb87b4a81bb08229d370c4ca589d`，容器 `bfd162bb...`，healthy | 官方 `v0.1.169` 加私有兼容提交，源码 `VERSION=0.1.169`；跨协议终态、容量精确重试、新 Vue 首页、积分桥接和 v0.1.169 官方能力均已进入待发布候选，最终 commit 以本轮推送后的 `main` 为准 | 只构建、推送并上传/缓存候选；维护者手工切换，自动化不得替换或重启 |
| 积分服务 | `0.1.168-28e760bc8c6d`，revision `28e760bc8c6d66414595ef2af213d301a423acf2`，容器 `7b745c53...`，healthy | `1e33e7f7a` 已收口冷灰/电蓝双工作区、上传 Logo、镜像内回退、用户名展示和软删除会话失效；候选由与 Sub2API 相同的最终 `main` commit 构建 | 先备份并执行存量角色 `username` 单列授权，再独立更新积分服务并验证；不得重跑历史基线，且不得改动 Sub2API 容器 |
| exact-root 首页 | Nginx 从宿主 `/home/api/sub2api-deploy/public/index.html` 提供旧生产文件 | 必须把 `deploy/public-landing/index.html` 与新版 Vue `/home` 同步为冷灰/电蓝、左文右图的数据化风格；最终状态以提交前静态页测试和文件 diff 为准 | 备份后原子替换并核对 SHA256；不 reload Nginx，不重启 Sub2API |

仓库候选版本为 `backend/cmd/server/VERSION=0.1.169`，本轮合并目标为官方 Release `v0.1.169` commit `26d894ef4f50645a4bf1030e378ac892f17d0223`；合并提交的第二父节点必须精确指向该 release。本轮重要私有节点：

- `d6b367f31d606771c464ada4928a9b5eb622bd68`：历史积分激活和模型容量精确错误重试。
- `28e760bc8c6d66414595ef2af213d301a423acf2`：管理员用户积分明细、删除积分侧手工赠送/快照刷新入口、发放任务只处理签到奖励。
- `874255bcd37d4820989eb1e13cdaf84f417996aa`：用户与管理员嵌入式积分工作区视觉及信息层级优化。
- `1e33e7f7a`：未登录首页、同源上传 Logo、同步生图冻结时限、用户名积分与最小 ACL、连续自然日积分曲线、全量签到发放汇总、中文状态和跨协议会话终态/断连排空修复的升级前收口点。
- `3da18b9dd2d0ecc890a5605a4d1cf97093a8659e`：v0.1.169 正式兼容 merge，双亲为私有收口 `1e33e7f7a` 与官方 release `26d894ef4`；保留上述二开并增加官方上游路径校验、代理流熔断 fail-open、订阅配额显示、价格资源和容器加固，同时修复 Composite 分组模型快照以及 Gemini 收到 `finishReason` 后等待 EOF 的挂起。
- 截至候选源码收口尚未生成新的镜像 tag、OCI revision 或 registry digest，也没有切换生产。制品标识必须来自最终推送 commit 和镜像工作流，禁止用 `1e33e7f7a`、旧生产 revision、image ID 或本地 archive SHA256 猜测。

## 2. 积分激活与历史基线

- 当前 policy 为 v3，`enabled=true`、`1 U = 10.00` 积分、每日 `00:05` 汇总前一自然日、签到关闭。启用积分展示不等于启用签到。
- 一次性历史作业 `5174eef7-5f0a-4a17-b4f1-f50840940f64` 已进入 `succeeded`。当前有 29 个积分账户、316 条每日快照、311 条积分账本，`needs_review=0`，签到和余额发放记录均为 0。
- 用户 ID 1 的对账值为总积分 `7514.94`、昨日积分 `938.07`。该值用于本次交接抽查，不应硬编码到页面或测试。
- 当前 `points` schema 已应用 `001_init.sql`、`002_balance_grant_outbox.sql`、`003_usage_history_backfill.sql`，共 21 张表、3 条积分迁移；Sub2API `public` 迁移与 `points.points_schema_migrations` 继续严格分离。
- 当前生产 `points_app` 仍只有 `public.users.id/deleted_at` 的列级读取权限，尚未执行用户名候选所需的 `SELECT (username)` 单列授权。仓库新增授权模板不代表线上 ACL 已改变；积分候选切换前必须按第 9 节备份、计数、执行并保存审计结果。
- 历史基线在整个 schema 生命周期中只允许成功一次。后续镜像更新不得执行 `points-history-backfill activate/plan/apply/resume`，也不得新建重叠作业。正常积分只由 `00:05` 自动调度和滚动差量对账维护；比例变更次日生效且只影响新消费。
- 4 个遗留 `points_shared_*` 测试 schema 已清理，清理后数量为 0，正式 `points` 数据计数未变化。清理前备份为 `/home/api/sub2api-deploy/backups/points-test-schema-cleanup-20260731-090001`，dump SHA256 `ecfc41fb6d3fbd332b3b0b86f9f8707257a81d4c266d9c6a7d6290c9ce661c29`，catalog 659 行。

## 3. 用户入口与签到状态

- 运行中的 Sub2API 仍加载 `POINTS_SYSTEM_ENABLED=false`；仓库外 `bridge-secrets.ready.env` 已准备为 `true`，但进程不会热加载该文件。因此当前普通用户菜单仍隐藏，不能把 policy v3 已启用误写成 Sub2API 用户入口已上线。
- 维护者手工切换下一 Sub2API 候选后，ready 配置才随新容器加载并显示 `/points`。切换前必须核对状态接口的 enabled/configured/active、积分 Origin、Key ID、TTL 和时钟偏差；接口不得返回密钥原值。
- 签到继续关闭。后续启用必须追加最早次日生效的新策略，完整配置每日次数、仅消费用户、昨日/总积分依据、最低昨日消费、固定/百分比阶梯及单次/用户每日/平台每日金额上限。
- 用户入口和管理员入口都是 Sub2API 内置页面，不得把 `points.52token.org` 配成普通自定义菜单。积分域名根路径保持 404，没有一次性 ticket/session 时 `/app/` 和 `/admin/` 必须拒绝。

## 4. 积分界面与权限

- Sub2API 在右侧内容区嵌入积分 iframe，左侧导航、Header 和上传 Logo 保持可见；`ui_mode=embedded` 只改变展示，不改变角色、ticket、session 或 API 授权。
- 用户工作区采用冷灰底、深墨导航、电蓝四指标、宽幅趋势图和个人双明细的数据化大屏，只显示总积分、昨日积分、今日/累计签到赠送、7/30/90 日积分趋势、个人积分记录和签到奖励记录。签到关闭时按钮必须清晰锁定，页面不得出现策略、全站用户列表、发放重试、冲正或其他管理员控件。
- 用户页和管理员页顶部身份、管理员全站用户积分明细首列以及签到余额发放任务用户列都显示 Sub2API 用户名。数值用户 ID 只保留在服务端账户关联、财务记录、审计和幂等处理中；浏览器 API 需要身份时返回 `username`，不得返回无必要的 `user_id`。
- 管理员工作区沿用同一视觉系统但保持紧凑运维布局，提供运行概览、全站用户积分明细、策略管理和签到发放任务。用户明细以未删除 Sub2API 用户为基准，按用户名显示总/昨日积分、累计/昨日成功消费和昨日结算状态，零消费用户补零显示。
- 7/30/90 日曲线必须返回今天之前的连续完整自然日，缺少快照的日期补 `0` 积分、`0` 消费和 `empty`，不得把“最近 N 条有消费记录”冒充“近 N 日”。管理员顶部发放汇总必须按全部 `checkin` 记录在服务端分组计数，不能只统计页面最新 100 条；`reversal_permanently_failed` 纳入失败告警。
- 积分服务不提供手工余额赠送；直接调余额继续使用 Sub2API 原有管理能力。积分服务不提供手工快照刷新；快照是自动调度生成的内部幂等结算记录。发放任务只列出 `checkin` 类型，可对失败任务审计重试，对已结算任务按安全状态审计冲正。
- 用户和管理员页面使用独立 HTML/脚本并执行双向精确角色校验。普通用户不得下载管理员脚本、打开管理员页面或调用 `/api/v1/admin/*`；管理员也不得调用用户账户、积分、签到或个人发放 API。
- `empty`、`needs_review`、`disabled` 在界面中分别显示为“无消费”“待复核”“未启用”，不得直接向用户展示内部英文状态；`needs_review` 使用告警样式，不能与普通中性状态混淆。
- 两个积分工作区的品牌图均由服务端注入精确父站 `POINTS_EMBED_PARENT_ORIGIN/api/v1/settings/logo`，只把该父站 Origin 加入 CSP `img-src`。加载失败时由页面一次性回退到积分镜像内、受积分会话保护的 `/assets/logo.svg`；不得再使用“积”或“管”文字占位，也不得为 Logo 放宽 iframe、ticket、session 或角色边界。

## 5. 未登录首页与 Logo

- 本轮只修改未登录 Vue `/home` 和 exact-root 静态首页，不改登录后的 Dashboard、用户导航或管理员业务页面。两者必须同步采用生图参考稿确定的冷灰/电蓝数据化风格、左文右图主视觉和一致信息层级；文案只描述功能、稳定性和管理能力，不出现具体国外模型或商业中转宣传名称。
- 页面统一使用后台上传 Logo。Sub2API 候选新增同源 `GET /api/v1/settings/logo`，只输出不超过 2 MiB 且文件签名与声明类型一致的 PNG/JPEG/WebP/GIF Base64 Data URI，并设置 `nosniff`；后台站点 Logo 上传控件同步只接受这四类栅格格式。SVG、AVIF、伪造类型、异常值或未配置时临时重定向 `/logo.svg`。该接口不回传设置 JSON，避免脚本型内容进入公开图片响应。
- exact-root 文件不在镜像层。发布时先运行 `frontend/src/utils/__tests__/publicStaticPages.spec.ts`，计算仓库文件 SHA256，备份宿主旧文件，以同目录临时文件原子 rename，再核对宿主文件、`/` 和 `/index.html` 响应体 SHA256 全部一致。此过程不需要 reload Nginx 或重启 Sub2API。
- 静态页在新 Sub2API 候选切换前必须能使用 `/logo.svg` 回退，不能先发布一个依赖尚未上线接口且无回退的版本。
- 新版 Vue 主视觉位于前端静态资源中；exact-root 必须保持单文件自包含，或把其新增视觉资产列入同一次原子发布和 SHA256 对账。只切换 Sub2API 镜像或只修改 `HomeView.vue` 都不会改变生产根路径。

## 6. 模型容量错误精确重试

源码候选只对去除首尾空白后精确等于以下文本的上游错误启用额外重试：

```text
Selected model is at capacity. Please try a different model.
```

- 适用于 HTTP 错误、HTTP 200 `response.failed` 和 SSE `error`，前提是尚未向客户端输出任何语义正文。
- 复用原请求体，最多额外两次，退避 `100ms`、`200ms`；优先选择其他可调度账号，没有其他候选时才使用同一账号的剩余预算。
- 不降低账号健康分，不写 runtime block，不触发账号冷却；每次计划写 `openai.model_capacity_retry_scheduled` 结构化日志且不记录请求正文。
- 大小写变化、近似文案、附加前后缀、一般 5xx/超时/断流、已经输出正文的流以及所有图片/视频创建均不适用。
- 当前生产 Sub2API `339422728b2c` 不包含该提交；只有维护者手工切换后续候选后才生效。

## 7. 生图与冻结复核

- `2026-07-31 09:18 CST` 只读盘点发现 10 条已发送但无出图终态的同步图片冻结，共 `1.02 U`：用户 1 为 2 条/`0.20 U`，用户 160 为 8 条/`0.82 U`。它们均无成功 usage、无成功媒体任务且已超过 30 分钟；旧生产逻辑仍按未知结果保留 24 小时，因此尚未进入到期核销。
- 操作前 root-only 备份为 `/home/api/sub2api-deploy/backups/media-hold-no-output-refund-20260731-092046`。`financial-tables.dump` SHA256 为 `77c60a5611000d4c3ae945f0ce71f85e99005393e1416a843a1ae5c98a9706b7`，候选记录与受影响用户快照 SHA256 分别为 `2583ee81a9154516c4c228ab3bb934b3026c82f8b41919cc9732c414f1ce3848`、`cee6c87aa83521cac9a732e912a0301c081e3af09e0fbde2a00c273889b8ae56`。
- 退款在单一事务中按固定 hold ID、固定总额、两名用户和无成功任务证据执行断言；10 条全部转为 `released`，用户 1 与用户 160 冻结余额均归零。审计记录为 `audit_logs.id=3814`、request ID `hold-refund-no-output-20260731`。操作后全站 active hold 为 0、非零冻结用户为 0；Sub2API 容器 ID、启动时间、镜像和 restart count 均未变化。
- 随后使用生产 `infinite-canvas` 的精确 3:2 自动质量参数调用主生图分组：`gpt-image-2`、`1536x1024`、`n=1`、`b64_json`、PNG，不发送 `quality/async/stream`。请求 99 秒后 HTTP 200，得到 `1248x832` PNG，SHA256 `bc8a8bcabcbdc33429d035e9be90d61bef539fbc03811b8bc53f575a93f5b6c6`；生产 root-only 记录位于 `/home/api/sub2api-deploy/image-reference-tests/20260731-094025-main`。
- 本次成功请求只生成一条 usage（`usage_logs.id=219522`、`0.10 U`、1 图），hold `408` 精确转为 `captured`/`0.10 U`；请求后全站 active hold 仍为 0，非零冻结用户仍为 0。此前 `high`/`low` 两次固定质量请求均由上游长期无响应后取消，不能据此把自动质量路径判断为不可用。
- 10:43 CST 按相同工作台参数再次验证主路径：约 `97.1s` 后 HTTP 200，hold `409` 与 usage `221890` 均为一次 `0.10 U` 结算，最终全站 active hold 和非零冻结用户仍为 0。上游未遵守 `b64_json` 请求而返回 URL，但图片下载成功；本地脱敏制品为 `1248x832` PNG，SHA256 `33a56196234cf9417896aad039778913fb137e4b807d49a1129964899d25fda4`。主路径成功，未调用备用分组。
- 下一 Sub2API 候选将同步 `/v1/images/generations` 的未知终态冻结时限设为 30 分钟；明确失败仍立即退款，异步图片/视频及其他媒体任务仍保留通用 24 小时窗口。当前生产 `339422728b2c` 在维护者手工切换前仍是 24 小时，不能把候选行为误写成已上线。

## 8. 会话中断排查与跨协议候选修复

- 用户 1 当日窗口内有 26 条 Nginx `502`，均可关联真实上游 `502/503`；另有 5 条 Nginx `499`，表示客户端先关闭连接。5 条中 4 条叠加上游 `524`，另 1 条在上游首响应等待约 `125.766s` 后由客户端取消。
- 生效 Nginx API 超时为 `1800s`；上述中断发生在数秒至约 125 秒，且 Nginx error log 没有对应代理超时，因此没有证据支持“Nginx 到时主动掐断”。10:47 附近存在客户端关闭后上游继续完成并正常落账的样本，优先归因客户端任务取消、本地网络或 Codex Desktop 连接生命周期。
- 10:23:00 与 10:24:05 有两条高置信静默候选：响应约 `602` bytes、`output_tokens=0`、账号 7，且没有对应 ops error。生产未保存原始响应体，不能据此绝对证明上游只返回 `[DONE]`；但该表现与旧代码把 Responses 通用 `[DONE]` 哨兵误判为成功终态完全一致。
- 后续候选把正式成功终态校验扩展到 Responses、原生 Chat、Anthropic 转 Chat、Gemini 转 Chat/Messages、Responses WS v2 和 WS-to-HTTP bridge。Responses 只接受 `response.completed/done`；原生 Chat 要求每个 choice 的有效 `finish_reason`，流式还要求 `[DONE]`；Anthropic 转 Chat 要求 `message_stop` 或带 `stop_reason` 的 `message_delta`；Gemini 路径要求 `finishReason` 并在处理完该事件的 parts/usage 后立即完成，不等待 EOF；WS 路径要求正式 Responses 终态。HTTP 200、EOF 和通用 framing 哨兵本身都不是成功证据。
- 上游 EOF/读取错误、SSE/WebSocket `error` 或缺终态时，未输出语义正文的请求进入既有安全故障切换判断；已经输出部分正文且连接可写时发送对应下游协议的显式错误，禁止合成 `[DONE]`、`finish_reason=stop`、`message_stop` 或其他成功终止事件。正式上游错误仍按既有账号故障分类处理；一般 5xx、读错误和部分输出不因本修复变成可安全重放。
- 客户端断开后所有协议都停止下游写入并关闭重放窗口，同时继续有限 drain 上游终态和 usage 供真实计费收口；后续 EOF、容量错误或读取错误不得触发第二次上游请求。请求 context 取消和明确下游写失败都会触发统一 guard；默认 5 秒，配置的更短正数 `stream_data_interval_timeout` 可收紧但不能延长，超时关闭上游 body，避免永久占用账号/并发槽。Responses 继续写脱敏断开日志，并以不继承取消信号的上下文保存 `response_id` 账号绑定。
- 本节全部为只读排查和候选代码结论。运行中的 Sub2API 仍是 `0.1.168-339422728b2c`，没有替换或重启；修复要等维护者手工切换最终候选镜像后才生效。

## 9. 发布与验证

本轮已完成的源码验证包括后端 `go test ./...`、`go vet ./...` 和 build；前端 ESLint、typecheck、全量 Vitest 和 production build；积分 `go test ./...`、`go vet ./...` 和 build；已渲染的桌面/移动 Vue 与积分页面无横向溢出且趋势图非空。exact-root 同步完成后仍必须重新运行静态页测试和最终截图检查；镜像工作流还须在最终 `main` commit 上执行自身测试和冒烟，不能用本地通过替代制品证据。

1. 推送最终 `main` 后触发 `.github/workflows/points-image.yml`，`publish_version_tag=true`；记录 tag、完整 revision、registry digest 和 workflow run。
2. 触发 `.github/workflows/cachecompat-image.yml`，`version=0.1.169`、`publish_latest=false`；记录同样的不可变制品信息。
3. 若 GitHub runner 仍在分配前被计费门禁终止，沿用受控本机构建标准 Docker archive 的既有流程；服务器只执行校验和 `docker load`，不编译源码或构建镜像。
4. Sub2API 候选只上传、导入或缓存到服务器，不执行 `docker compose up`，不修改当前容器。把候选 tag/digest 和人工切换命令交给维护者。
5. 更新积分前新建并校验数据库备份，记录旧容器、镜像、积分账户/快照/账本和 Sub2API 用户计数。生产 `points_app` 当前尚未具备用户名列权限；不得重跑 `shared-database-bootstrap.sql.example`，应通过 root-only stdin 提供既有 `points_app_role`，单独运行 `points-system/deploy/shared-database-users-username-upgrade.sql.example`。该事务必须只产生 `GRANT SELECT (username)`，不改 PUBLIC ACL；保存脚本输出的执行人、目标角色、事务号和前后断言，授权后复核上述数据计数完全不变。任一预检或断言失败时停止积分更新并保留当前容器。
6. 完成单列授权后，仅对积分服务执行 `docker compose up -d --no-deps points-system`。更新后核对 21 表/3 迁移、policy v3 和历史 job ID 不变，对比更新前计数与期间正常调度增量，确认无重复历史账本、无异常减少、`needs_review=0`、签到/发放仍为 0；同时检查用户/管理员顶部、管理员用户明细和签到发放用户列只显示用户名，并断言浏览器响应没有无必要的 `user_id`，Sub2API 容器 ID、启动时间和镜像引用完全不变。
7. exact-root 首页按第 5 节独立原子发布。它不依赖镜像切换，也不需要 Nginx reload，但线上 SHA256 必须与仓库候选一致。

在 GitHub 构建完成前，下一候选的镜像 tag、OCI revision 和 registry digest 仍为空。推送最终 `main` 后必须把 commit、两个 workflow run、不可变 tag 和 registry digest 追加到本节或同日续录，禁止把源码工作树、旧 registry digest、image ID 或 archive SHA256 互相冒充。

## 10. 不可破坏约束

- 不自动替换、重启或重建生产 Sub2API；普通用户入口由维护者手工切换候选后加载 ready 配置。
- 积分更新前备份，更新后不重跑历史基线，不删除历史 job、快照、积分账本、审计或 outbox。
- 生产存量 `points_app` 的用户名单列授权尚未执行；切换用户名积分候选前只能运行专用升级模板，禁止重跑 bootstrap、授予用户表整表读取或修改 PUBLIC ACL。
- 签到保持关闭；积分展示开放不能隐式启用余额奖励。
- `/api/internal/points/credits` 公网 `POST/OPTIONS` 继续精确 404，只允许积分容器经 Docker 网络调用；HMAC、幂等 UUID、Redis fail-close 限流和余额缓存代次保护必须保留。
- `/launch` 是唯一可关闭 access log 的积分路径，其他页面、API、拒绝、越权和限流请求保留日志；任何日志均不得记录 ticket、cookie、HMAC、密钥或请求正文。
- 后续合并官方版本时，以官方结构与通用行为为主，同时逐项回归媒体冻结核销、积分同库、历史积分、双工作区、管理员用户明细、签到发放安全、容量精确重试、跨协议正式终态/断流/客户端断开、Vue 与 exact-root 同步首页以及同源上传 Logo。
