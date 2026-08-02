# 2026-07-31 积分激活与后续候选交接

本文记录 `2026-07-31 CST` 已确认的生产事实、当前源码候选和下一次发布边界。它不保存密码、数据库连接串、HMAC secret、会话密钥、证书私钥或原始日志。`2026-07-30` 的首次同库部署证据仍保存在 [`PRODUCTION_DEPLOYMENT_20260730_CN.md`](PRODUCTION_DEPLOYMENT_20260730_CN.md)，但其中旧积分镜像和 disabled policy 不再代表现状。

## 0. 2026-08-02 最新生产状态（优先于后文历史快照）

- 当前积分服务为 `ghcr.io/hxly520/sub2api-points:0.1.169-1d8d50522429`，OCI revision `1d8d50522429b5d943766ad1d1b4a14b82e31d80`，容器 `1e5ed38b81da...`，healthy、restart count `0`。registry digest 为 `sha256:cc798629371d94898fbd3b049f4f454166b9e79f2893cee1e0a643344bacb2c2`，image ID 为 `sha256:5d4edb7822499e7c2953f7aa1f4889d88fbd9ac630fce69742d0a4f694e192dd`，传输 archive SHA256 为 `e33e80c5b28307120881ccf269ebd7b5cae46c447173cc136182643ef56d960b`。后文 `ca18cf77a`、`bee059a1`、`e8d73f3e` 等镜像均为对应时间点的历史发布证据，不代表当前运行态。
- 当前生产 Sub2API 仍为 `ghcr.io/hxly520/sub2api:0.1.169-f79803bb73d6`，容器 `dee0f8efd24d...`，healthy、restart count `0`。修复候选 `ghcr.io/hxly520/sub2api:0.1.169-1a4a690dd999` 已在服务器完成 `docker load`，但 Compose 仍指向 `f79803bb73d6`，没有自动替换或重启 Sub2API；手工切换边界见第 11.12 节。
- 积分中心继续全体开放：`POINTS_USER_ACCESS_MODE=all`、`POINTS_USER_PREVIEW_IDS=`。签到使用独立门禁：`POINTS_CHECKIN_ACCESS_MODE=preview`、`POINTS_CHECKIN_PREVIEW_IDS=1`；因此所有合格用户仍可查看积分中心，但只有用户 ID 1 能发起签到，不能把签到预览名单误写为积分中心预览名单。
- policy v4 自 `2026-08-02` 生效：`enabled=true`、`mode=consumer_only`、`basis=yesterday`、签到开启、每日限 1 次、积分比例 `10.00:1 U`、刷新时间 `00:05`。签到按昨日原始成功消费金额的 `1000-50000 PPM`（`0.1%-5%`）计算，单次、单用户每日和平台每日三个绝对上限均为 `100 U`；`consumer_only` 仍强制昨日消费金额大于 0。
- policy v5 已通过管理员 API 保存并审计，将于 `2026-08-03` 生效：最低昨日消费 `1 U`、每日 1 次，严格按昨日原始成功余额消费使用 `[1,10) U -> 1%-5%`、`[10,50) U -> 2%-5%`、`[50,100) U -> 3%-5%`、`[100,+∞) U -> 4%-5%` 四档，三个金额上限均为 `NULL`（不限）。`2026-08-02` 当天仍由 policy v4 生效；签到运行门禁继续只允许用户 1。
- `2026-08-02 00:05 CST` 快照成功后，用户 1 的昨日成功消费为 `86.890694 U`、昨日积分为 `868.90`，按分币截断后的理论签到范围为 `0.08-4.34 U`。本次随机比例为 `35537 PPM`（`3.5537%`），实际赠送 `3.08 U`；交易 UUID `8e20f4f9-d3ab-4d16-95be-0b186c96da97` 最终为 `settled`，Sub2API credit 记录恰好 1 条，`balance_after=8993.5808432400`。
- 同一幂等键重放返回 `201`，仍复用上述 `3.08 U` settled 结果且没有重复加款；更换幂等键再次签到返回 `409 daily check-in limit reached`。用户 2 积分中心仍可访问，但签到能力为关闭并返回 `403 checkin_unavailable`；非用户 1 的签到、签到尝试和每日签到计数均为 `0`。
- 首次 credit 返回 `500` 的根因是当前旧 Sub2API 在写入 `audit_logs` 时给 `request_body` 传入 `NULL`，违反该字段约束；积分服务随后使用原交易 UUID 安全重试并成功，没有生成第二笔 credit。生产暂时保留精确兼容触发器/函数 `points_credit_audit_request_body_compat`，只对 `action='points.balance_credit' AND request_body IS NULL` 的审计行填入兼容值，不放宽列约束，也不处理其他 action。
- 本次积分切换前备份位于 `/home/api/backups/points-spend-tiers-20260802-091604`。全库 dump 为 `103,217,039` 字节，SHA256 `dae794a05a1d43bd13e2c9baab55969b35d4fb7f08420899d9cddb8ad0634e24`，`pg_restore -l` 目录共 `1,294` 行；同目录保留两套 Compose、积分 env 和两服务切换前 inspect。积分 Compose 回滚点为 `/home/api/sub2api-points/backups/compose.pre-1d8d50522429-20260802-091919.yml`。
- 最终阶梯源码为 `1d8d50522429b5d943766ad1d1b4a14b82e31d80`，积分生产镜像为 `ghcr.io/hxly520/sub2api-points:0.1.169-1d8d50522429`，manifest digest `sha256:cc798629371d94898fbd3b049f4f454166b9e79f2893cee1e0a643344bacb2c2`，传输归档 SHA256 `e33e80c5b28307120881ccf269ebd7b5cae46c447173cc136182643ef56d960b`，linux/amd64、10 层、OCI revision 标签为完整 commit。服务器已上传、核验并 `docker load`，迁移 `004_checkin_spend_tiers_and_optional_caps.sql` 已生效；启动对账 `changed_users=0`，原账户、快照、账本、签到和发放计数均未被改写。

## 1. 版本边界

| 对象 | 当前生产 | 当前源码/下一候选 | 允许的动作 |
| --- | --- | --- | --- |
| Sub2API | `0.1.169-f79803bb73d6`，revision `f79803bb73d659e36627d6f716aab065ff4d56a6`，容器 `dee0f8efd24d...`，healthy、restart count `0` | `0.1.169-1a4a690dd999` 已加载但 Compose 未切；修复 points credit 审计 `request_body=NULL` | 仅由维护者手工切换；自动化不得替换或重启 |
| 积分服务 | `0.1.169-1d8d50522429`，revision `1d8d50522429b5d943766ad1d1b4a14b82e31d80`，容器 `1e5ed38b81da...`，healthy、restart count `0` | 迁移 004 已上线；policy v4 当日生效，四阶梯 policy v5 于 `2026-08-03` 生效；积分中心全体可见、签到仅用户 1 | 不得重跑历史基线；后续仅按次日策略配置和独立积分镜像边界更新 |
| exact-root 首页 | 宿主文件与仓库 `deploy/public-landing/index.html` 已同步，SHA256 `09cd27dda14f1810c58fc0e774cc36cfb6b17cfaa209bdc72db1db6748df88a0` | 冷灰/电蓝、左文右图的数据化首页已上线 | 后续仍按备份、原子替换和三方 SHA256 对账发布；不 reload Nginx |

仓库候选版本为 `backend/cmd/server/VERSION=0.1.169`，本轮合并目标为官方 Release `v0.1.169` commit `26d894ef4f50645a4bf1030e378ac892f17d0223`；合并提交的第二父节点必须精确指向该 release。本轮重要私有节点：

- `d6b367f31d606771c464ada4928a9b5eb622bd68`：历史积分激活和模型容量精确错误重试。
- `28e760bc8c6d66414595ef2af213d301a423acf2`：管理员用户积分明细、删除积分侧手工赠送/快照刷新入口、发放任务只处理签到奖励。
- `874255bcd37d4820989eb1e13cdaf84f417996aa`：用户与管理员嵌入式积分工作区视觉及信息层级优化。
- `1e33e7f7a`：未登录首页、同源上传 Logo、同步生图冻结时限、用户名积分与最小 ACL、连续自然日积分曲线、全量签到发放汇总、中文状态和跨协议会话终态/断连排空修复的升级前收口点。
- `3da18b9dd2d0ecc890a5605a4d1cf97093a8659e`：v0.1.169 正式兼容 merge，双亲为私有收口 `1e33e7f7a` 与官方 release `26d894ef4`；保留上述二开并增加官方上游路径校验、代理流熔断 fail-open、订阅配额显示、价格资源和容器加固，同时修复 Composite 分组模型快照以及 Gemini 收到 `finishReason` 后等待 EOF 的挂起。
- `04a19ca082ee43853573795d1385727bd38f20e9`：v0.1.169 首轮发布阶段节点；补齐非流式 Gemini SSE 聚合在正式 `finishReason` 后立即完成、不等待 `[DONE]` 或 EOF 的回归边界，后续由 `f79803bb7` 预览收口取代。
- `5b27f0b80337c0d33435322a9307d84a9edac110`：管理员积分配置改为隔离新标签，增加 Sub2API 用户 1 预览名单和 iframe 实时主题同步。
- `f79803bb73d659e36627d6f716aab065ff4d56a6`：最终预览修复；用户级显式拒绝覆盖陈旧公共设置，积分服务在 ticket 交换和每次既有 user session 请求双重执行预览门禁。
- `7e50f9aa9f395f00f235365376d33f859ae7d16f`：积分用户/管理员工作区按 `design-taste-frontend` 定向重设计，补齐登录邮箱、业务发放时间、稳定游标分页、请求超时和邮箱 ACL 四阶段迁移。
- `e39c78bf8f6c00230d2756493b9c951a2c39d4fa`：隔离屏幕阅读器趋势数据表的 table layout，消除桌面约 `658px` 无效空白尾部；上一积分生产 revision。
- `e8d73f3e665596fc0d9e185d8ce706c45d04438a`：Taste 数字工作区增强，固定 `VARIANCE 6 / MOTION 4 / DENSITY 5`，新增主积分焦点、语义同步状态、8 px 面板尺度和 reduced-motion；历史积分生产 revision。
- `ca18cf77a86a921600e7324a75d09188e1e4fed7`：增加 `POINTS_CHECKIN_ACCESS_MODE` 与 `POINTS_CHECKIN_PREVIEW_IDS`，把签到可用范围与积分中心可见范围独立控制；历史积分生产 revision。
- `1d8d50522429b5d943766ad1d1b4a14b82e31d80`：增加昨日原始消费四阶梯、可空金额上限和迁移 004；当前积分生产 revision，policy v5 已按次日生效规则保存。
- `7c62dd1a8b4449b57eb5a929002906d72b3eabf1`：补齐 `100 U` 昨日消费对应 `0.10-5.00 U` 的百分比签到精确回归边界。
- `1a4a690dd999b669e2ce09522854ea157d7af984`：修复 Sub2API points credit 审计不得写入 `request_body=NULL`；对应 Sub2API 镜像已加载服务器、尚未由维护者手工切换。
- `f79803bb73d6` 的两个镜像已由受控本机构建、推送 GHCR、上传并加载服务器；积分先独立切换，Sub2API 随后已由维护者在 `2026-07-31 23:47 CST` 手工切换。registry digest、image ID 与 archive SHA256 分别记录在第 11.7 节，不得互相替代。
- `2026-08-01 22:34 CST` 仅更新积分服务：本机构建 revision `bee059a1cec5d0eb1a6d022d766670489dcf484d`，归档 SHA256 为 `bbf7d051b2295f230e65d80b77d5ecaf7dac0a049a576fa78e04eb586583ce1f`，服务器完成 SHA256 对账后 `docker load`，仅替换 `points-system` 容器。Sub2API 仍为原 `0.1.169-f79803bb73d6` 镜像；前一候选 `99fbbb5c4c8a` 仅作为同次配置切换的回滚点。
- 同一变更将 `/home/api/sub2api-points/bridge-secrets.env` 设为 `POINTS_SYSTEM_ENABLED=true`、清空 `POINTS_SYSTEM_PREVIEW_USER_IDS`，将 `points.env` 设为 `POINTS_USER_ACCESS_MODE=all`、清空 `POINTS_USER_PREVIEW_IDS`；原文件备份位于 `/home/api/sub2api-deploy/backups/points-all-users-20260801-221005`，最终 Compose 回滚点为 `/home/api/sub2api-points/backups/compose.pre-bee059a1-20260801-223426.yml`。两文件继续保持 `0600 root:root`。
- 策略控制台改为单一当前配置表单：页面不再显示历史版本列表；每次保存仍由服务端追加不可变内部版本，并强制下一自然日生效。当前 policy v4 的 `enabled=true` 因此对所有有效用户开放，关闭该开关后用户菜单、票据、会话和用户 API 均自动隐藏/拒绝，管理员入口保留；签到是否可用再由 policy 签到开关与独立签到运行门禁共同决定。

## 2. 积分激活与历史基线

- 当前 policy 为 v4，`enabled=true`、`1 U = 10.00` 积分、每日 `00:05` 汇总前一自然日；签到开启但独立运行门禁仅允许用户 1。启用积分展示不等于对全体用户启用签到。
- 个人消费积分账本的“发放时间”字段为 `awarded_at`，按 `Asia/Shanghai` 业务时间投影：消费账本行取 `business_date + 1` 发放自然日零点加该发放日实际生效策略的 `refresh_minute`，不得在正常策略切换日沿用消费日或账本绑定策略，当前默认即次日 `00:05`；只有发放日早于首个生效策略的历史回算行，才回退该账本固定 `policy_version` 的刷新分钟。非消费、旧版或同时缺少发放日/账本策略的记录回退 `created_at`。该口径只用于展示，不改写不可变账本。
- 一次性历史作业 `5174eef7-5f0a-4a17-b4f1-f50840940f64` 已进入 `succeeded`，且仍是唯一成功基线。`2026-07-31 23:27 CST` 核对时共有 29 个积分账户、316 条每日快照、311 条积分账本；`2026-08-01 00:05 CST` 正常调度完成后，最新只读计数为 29 个积分账户、328 条每日快照/修订、322 条积分账本。两个时间点均为 `needs_review=0`，签到和余额发放记录均为 0。
- `2026-07-31 23:27 CST` 用户 ID 1 的历史抽查值为总积分 `7514.94`、昨日积分 `938.07`；`2026-08-01 00:05 CST` 调度后的抽查值为总积分 `9283.00`、`2026-07-31` 业务快照积分 `1768.06`。这些值只用于对应时间点的交接抽查，不应硬编码到页面或测试。
- 当前 `points` schema 已应用 `001_init.sql`、`002_balance_grant_outbox.sql`、`003_usage_history_backfill.sql`、`004_checkin_spend_tiers_and_optional_caps.sql`，共 21 张表、4 条积分迁移；Sub2API `public` 迁移与 `points.points_schema_migrations` 继续严格分离。
- `2026-08-01 04:40 CST` 已通过事务 `1960217` 执行登录邮箱阶段 A：`points_app` 当前精确为 `id/email/username/deleted_at` 列级只读 allowlist，无整表、其他列、写权限或 grant option；用户、账户、快照和账本计数不变。阶段 B 尚未执行，必须等用户 1 与管理员真实票据确认 `login_email`、主题、分页和刷新后才撤销 `username`，最终收敛为 `id/email/deleted_at`。
- 历史基线在整个 schema 生命周期中只允许成功一次。后续镜像更新不得执行 `points-history-backfill activate/plan/apply/resume`，也不得新建重叠作业。正常积分只由 `00:05` 自动调度和滚动差量对账维护；比例变更次日生效且只影响新消费。
- 4 个遗留 `points_shared_*` 测试 schema 已清理，清理后数量为 0，正式 `points` 数据计数未变化。清理前备份为 `/home/api/sub2api-deploy/backups/points-test-schema-cleanup-20260731-090001`，dump SHA256 `ecfc41fb6d3fbd332b3b0b86f9f8707257a81d4c266d9c6a7d6290c9ce661c29`，catalog 659 行。

## 3. 用户入口与签到状态

- 历史预览阶段曾保持 Sub2API `points_system.enabled=false`、`points_system.preview_user_ids: [1]` 和积分服务 `POINTS_USER_ACCESS_MODE=preview`、`POINTS_USER_PREVIEW_IDS=1`；该阶段已在 2026-08-01 22:26 CST 结束。当前积分中心两端均为全体模式，用户可见性只由当前生效 policy 的 `enabled` 决定；签到另由 `POINTS_CHECKIN_ACCESS_MODE=preview`、`POINTS_CHECKIN_PREVIEW_IDS=1` 收窄。
- 完整白名单只保存在服务端配置中，浏览器认证状态只返回当前用户专属的 `points_system_access` 布尔值。切换候选前必须核对 configured/active、积分 Origin、Key ID、TTL、时钟偏差以及用户 1/非白名单用户的正反授权；调试无误后再由维护者显式设置 `enabled=true` 开放全体用户，接口始终不得返回密钥或完整白名单。
- 签到已通过次日生效的 policy v4 开启，仅供用户 1 生产测试：仅昨日有成功消费、依据昨日消费、每日 1 次、按昨日原始消费金额的 `0.1%-5%` 随机赠送，三层绝对上限均为 `100 U`。用户 2 和其他非预览用户必须在签到资料与 POST 两层均被拒绝，但积分中心仍保持可见。
- 用户入口和管理员入口都是 Sub2API 内置页面，不得把 `points.52token.org` 配成普通自定义菜单。用户页嵌在 Sub2API 右侧内容区；管理员从设置页打开新浏览器标签。积分域名根路径保持 404，没有一次性 ticket/session 时 `/app/` 和 `/admin/` 必须拒绝。

## 4. 积分界面与权限

- Sub2API 仅把用户积分页以 iframe 嵌入右侧内容区，左侧导航、Header、当前明暗主题和上传 Logo 保持可见；`ui_mode=embedded` 只改变展示，不改变角色、ticket、session 或 API 授权。管理员在 `/admin/settings/points` 点击“打开积分配置”后，经 step-up 生成一次性管理员票据并以 `noopener,noreferrer` 新开浏览器标签页；原设置页保留，不跳转，也不在底部渲染管理员 iframe，浏览器拦截新窗口时显示失败状态。
- 用户工作区采用与 Sub2API 一致的浅色中性层级、`dark-950/dark-800/dark-700` 深色层级和 teal 主色，使用四张紧凑等高卡片、宽幅趋势图和两张独立明细表，只显示总积分、昨日积分、今日/累计签到赠送、7/30/90 日积分趋势、个人积分记录和签到奖励记录。两张记录表默认每页 10 条并独立翻页；签到关闭时按钮必须清晰锁定，页面不得出现策略、全站用户列表、发放重试、冲正或其他管理员控件。
- 积分 iframe 接收父页明暗主题后，后续个人资料刷新不得覆盖父主题；表格表头、行、悬停行、分页按钮、状态标签和 Canvas 曲线必须同步使用主题变量，深色模式不得出现白色明细底或低对比文字。刷新按钮和签到按钮在异步成功、失败及不确定网络结果后都必须恢复到已确认状态。
- 用户页和管理员页顶部身份、管理员全站用户积分明细首列以及签到余额发放任务用户列都显示 Sub2API 登录邮箱。数值用户 ID 只保留在服务端账户关联、财务记录、审计和幂等处理中；浏览器 API 需要身份时返回 `login_email`，不得返回无必要的 `user_id`。
- 管理员工作区沿用同一视觉系统但保持紧凑运维布局，提供运行概览、全站用户积分明细、策略管理和签到发放任务。用户明细以未删除 Sub2API 用户为基准，按登录邮箱显示总/昨日积分、累计/昨日成功消费和昨日结算状态，零消费用户补零显示。
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

本轮已完成的源码验证包括后端 `go test ./...`、`go vet ./...` 和 build；前端 ESLint、typecheck、全量 Vitest 和 production build；积分 `go test ./...`、`go vet ./...` 和 build；已渲染的桌面/移动 Vue 与积分页面无横向溢出且趋势图非空。exact-root 发布前的静态页测试共 19 项通过，发布后完成宿主与公网 SHA256 对账。无论使用 GitHub runner 还是受控本地回退，制品构建路径都必须在最终 `main` commit 上另行验证镜像层内容、revision、校验和与隔离冒烟；本轮结果见第 11 节。

以下 1-7 是发布前执行清单；本轮实际执行结果、制品身份和生产断言见第 11 节。

1. 推送最终 `main` 后触发 `.github/workflows/points-image.yml`，`publish_version_tag=true`；记录 tag、完整 revision、registry digest 和 workflow run。
2. 触发 `.github/workflows/cachecompat-image.yml`，`version=0.1.169`、`publish_latest=false`；记录同样的不可变制品信息。
3. 若 GitHub runner 仍在分配前被计费门禁终止，沿用受控本机构建标准 Docker archive 的既有流程；服务器只执行校验和 `docker load`，不编译源码或构建镜像。
4. Sub2API 候选只上传、导入或缓存到服务器，不执行 `docker compose up`，不修改当前容器。把候选 tag/digest 和人工切换命令交给维护者。
5. 更新积分前新建并校验数据库备份，记录旧容器、镜像、积分账户/快照/账本和 Sub2API 用户计数。存量角色不得重跑 bootstrap 或历史 username 模板；通过 root-only stdin 执行阶段 A `shared-database-users-email-upgrade.sql.example`。该事务只产生 `GRANT SELECT (email)`，必须保留 `username` 并断言兼容 allowlist 精确为 `id/email/username/deleted_at`，不得修改 PUBLIC ACL、整表、其他列或写权限。阶段 A 后旧积分镜像必须继续正常。
6. 仅对积分服务执行 `docker compose up -d --no-deps points-system`。核对 21 表/3 迁移、policy v3、历史 job、数据计数和 Sub2API 容器身份不变，并用真实用户/管理员票据验证 `login_email`、角色隔离、用户 1 预览及用户 2 `403`。全部通过后才执行阶段 B `shared-database-users-email-finalize.sql.example` 撤销 `username` 并断言最终 `id/email/deleted_at`。阶段 B 前可直接切回旧镜像；阶段 B 后回滚必须先执行 rollback-prepare 恢复双读，旧镜像验收后再执行 rollback-finalize 撤销 `email`。
7. exact-root 首页按第 5 节独立原子发布。它不依赖镜像切换，也不需要 Nginx reload，但线上 SHA256 必须与仓库候选一致。

在制品构建完成前，下一候选的镜像 tag、OCI revision 和 registry digest 必须视为空。最终 `main`、不可变 tag、registry digest、image ID 和 archive SHA256 已在第 11 节分别补录，仍禁止把这些标识互相冒充。

`2026-07-31 18:41 CST` 对源码 commit `6d40a9d8c3b0a27662a25139d7e9801537b01f3b` 触发了两次正式构建请求：Points System Image [run 30624409567](https://github.com/hxly520/sub2api/actions/runs/30624409567)（`publish_version_tag=true`）和 Cachecompat Image [run 30624411767](https://github.com/hxly520/sub2api/actions/runs/30624411767)（`version=0.1.169`、`publish_latest=false`）。两者都在 runner 分配前被 GitHub 账户计费门禁终止，注释为近期付款失败或需要提高支出上限；`runner_id=0`、`steps=[]`，没有 checkout、测试、Docker build、registry push、镜像 tag 或 digest。该结论只表示外部构建基础设施未启动，不表示源码测试或镜像构建步骤失败。本次发布随后按第 11 节的受控本机构建流程完成，不再依赖这两个失败 run；账户门禁解除后可在新的最终 `main` 上补跑工作流验证，但新 run 不能冒充或改变本次已记录制品的身份。

## 10. 不可破坏约束

- 不自动替换、重启或重建生产 Sub2API；预览入口只由维护者手工切换候选并加载 `enabled=false`、`preview_user_ids: [1]` 后生效，禁止加载旧的全站 enabled ready 配置。
- 积分更新前备份，更新后不重跑历史基线，不删除历史 job、快照、积分账本、审计或 outbox。
- 生产当前的用户名单列授权只作为历史运行态；登录邮箱候选切换前只进入 `id/email/username/deleted_at` 双读兼容态，候选验收后才收敛为 `id/email/deleted_at`。禁止在切换前撤销旧镜像所需的 `username`，也禁止重跑 bootstrap/旧模板、授予用户表整表读取或修改 PUBLIC ACL。
- 签到保持关闭；积分展示开放不能隐式启用余额奖励。
- `/api/internal/points/credits` 公网 `POST/OPTIONS` 继续精确 404，只允许积分容器经 Docker 网络调用；HMAC、幂等 UUID、Redis fail-close 限流和余额缓存代次保护必须保留。
- `/launch` 是唯一可关闭 access log 的积分路径，其他页面、API、拒绝、越权和限流请求保留日志；任何日志均不得记录 ticket、cookie、HMAC、密钥或请求正文。
- 后续合并官方版本时，以官方结构与通用行为为主，同时逐项回归媒体冻结核销、积分同库、历史积分、双工作区、管理员用户明细、签到发放安全、容量精确重试、跨协议正式终态/断流/客户端断开、Vue 与 exact-root 同步首页以及同源上传 Logo。

## 11. v0.1.169 最终制品与生产续录

### 11.1 GPT-5.6 生产渠道价格

`2026-07-31 20:04:06 CST` 通过生产现有管理员渠道更新接口修改渠道 1“海外渠道”的 GPT-5.6 自定义价格；该接口在数据库事务提交后主动重建运行中渠道缓存，没有重启或替换 Sub2API。单位均为 USD/token，括号内为 USD/百万 token：

| 模型 | 输入 | 缓存读取 | 缓存写入 | 输出 |
| --- | ---: | ---: | ---: | ---: |
| `gpt-5.6-sol` | `0.000005`（5） | `0.0000005`（0.5） | `0.00000625`（6.25） | `0.00003`（30） |
| `gpt-5.6-terra` | `0.000002`（2） | `0.0000002`（0.2） | `0.0000025`（2.5） | `0.000012`（12） |
| `gpt-5.6-luna` | `0.0000002`（0.2） | `0.00000002`（0.02） | `0.00000025`（0.25） | `0.0000012`（1.2） |

生产所用 `0.1.168-339422728b2c` 已把公开别名 `gpt-5.6` 归一到 `gpt-5.6-sol`，因此别名没有遗漏。价格更新只影响后续成功消费；没有回写历史 usage、积分快照或积分账本。更新前后 Sub2API 容器均为 `bfd162bb9380...`，启动时间、镜像和 restart count 完全一致。

### 11.2 `04a19ca` 阶段源码与镜像

- 该阶段发布源码为 `04a19ca082ee43853573795d1385727bd38f20e9`，当时已推送私有仓库 `main`；`VERSION=0.1.169`。后续预览收口和当前生产 revision 见第 11.7、11.8 节。
- 官方最新正式版仍为 `v0.1.169` / `26d894ef4`。官方 `main` 的后续 5 个未发布提交没有混入本轮 release。
- 该阶段补丁让非流式 `collectGeminiSSE` 在解析完带正式 `finishReason` 的内容和 usage 后立即完成，不再等待 `[DONE]` 或 EOF；新增未关闭 `io.Pipe` 的阻塞回归测试。
- GitHub Actions 仍受 runner 分配前的账户计费门禁影响，本轮按既有受控本机构建流程使用 Go `1.26.5`、pnpm `9.15.9`、`CGO_ENABLED=0` 和标准 Docker archive；服务器只执行 SHA256 校验、`docker load` 和运行时冒烟，没有编译源码或构建镜像。

| 制品 | 标签 | registry digest | image ID | archive SHA256 | bytes |
| --- | --- | --- | --- | --- | ---: |
| Sub2API | `ghcr.io/hxly520/sub2api:0.1.169-04a19ca082ee`、`0.1.169` | `sha256:08013eeab760fce095ea9e8d945edf5e5ec0ac9057b8af69dad299f5522778f7` | `sha256:b2fcce5b732d42e6a3b67d6b48366dc9bf5b590781c07ce19581dfa6377b1408` | `9186d79c67f20962901af216e2c1f75de5d9f2bc5d62ea5320c1ee2d69d7f84e` | 47,931,392 |
| 积分服务 | `ghcr.io/hxly520/sub2api-points:sha-04a19ca082ee`、`0.1.169-04a19ca082ee` | `sha256:4a31fa5efb1542db3a26940f262e544dbb7124ba1d907458d704d8d26311e2c9` | `sha256:568c1af8447d1c8b03192163885a6a4426de4720aeb52408ffb0ee391808dda1` | `c25d8635358d9341084ab825097d67017aabf51a657c33fd2247361f58d137c8` | 11,846,656 |

服务器归档分别位于：

- `/home/api/sub2api-deploy/image-archives/sub2api-0.1.169-04a19ca082ee-linux-amd64.tar`
- `/home/api/sub2api-points/releases/sub2api-points-0.1.169-04a19ca082ee-linux-amd64.tar`

两份文件均为 `0600 root:root`，本地与服务器 SHA256 一致。Sub2API 候选在隔离、无网络容器中的 `-version` 输出为 `0.1.169`、完整 revision 和 release 构建时间；在该阶段制品验收完成时，生产 Sub2API 尚未切换到该候选。

### 11.3 积分独立更新与验收

- 更新前备份：`/home/api/sub2api-deploy/backups/points-04a19ca082ee-20260731T2035110800/`。包含 `points` schema custom dump、catalog、完整数据库 schema、Compose、环境文件、运行时和前后数据断言，权限均为 root-only。
- 历史专用用户名升级模板以事务 `1914532` 执行，只给既有 `points_app` 直接授予 `public.users.username` 单列 SELECT；`id/deleted_at` 原授权保留，整表 SELECT 仍为 false，其他用户列和写权限未开放。授权前后全部业务计数一致。该条是当时的执行证据，不代表下一候选仍使用 username，也不得据此重跑或改写旧模板。
- 只修改 `/home/api/sub2api-points/compose.yml` 的积分镜像标签并执行 `docker compose up -d --no-deps points-system`。新容器 `ac58ec881e35...` 使用非 root `65532:65532`、只读 rootfs、`cap_drop: ALL`、`no-new-privileges`、PID 128，healthy 且 restart count 为 0。
- 启动新增一条 `2026-07-30` 的幂等 `startup` 刷新记录：`source_users=14`、`source_rows=19056`、`changed_users=0`、消费和积分差量均为 0。账户仍为 29、快照 316、修订 316、账本 311、`needs_review=0`；历史 job 仍只有 `5174eef7-5f0a-4a17-b4f1-f50840940f64:succeeded`，40 个历史日期不变；签到、签到尝试、余额发放和发放尝试均为 0。
- 当时的管理员真实票据会话验证了旧版嵌入式管理员页、同源上传 Logo、管理员脚本、policy v3、签到关闭、余额发放空记录，以及 186 个未删除用户的两页完整明细；每条只返回非空展示用户名，不返回 `user_id`。该历史证据不代表后续管理员入口仍应嵌入或身份字段仍应使用 username；最新契约已改为从 Sub2API 设置页新开独立标签并显示 `login_email`。用户真实票据会话验证了嵌入式用户页、Logo、个人总览、39 条账本、连续 30 日曲线和空签到发放记录，同样没有 `user_id`。两类测试均正常注销，残留测试 session 已清零。
- 更新前后 Sub2API 始终为容器 `bfd162bb9380...`、镜像 `0.1.168-339422728b2c`、原启动时间、restart count 0 和 healthy；没有重建或重启。

### 11.4 exact-root 首页

`frontend/src/utils/__tests__/publicStaticPages.spec.ts` 共 19 项通过后，把 `deploy/public-landing/index.html` 原子发布到 `/home/api/sub2api-deploy/public/index.html`。备份位于 `/home/api/sub2api-deploy/backups/public-landing-04a19ca082ee-20260731T2048340800/`。仓库文件、宿主文件、`https://api.52token.org/` 和 `/index.html` 的 SHA256 均为 `09cd27dda14f1810c58fc0e774cc36cfb6b17cfaa209bdc72db1db6748df88a0`。没有 reload Nginx，Sub2API 和积分容器身份均未变化。

### 11.5 `04a19ca` 阶段维护者手工切换边界

在该阶段，服务器已经加载 `ghcr.io/hxly520/sub2api:0.1.169-04a19ca082ee`。自动化没有修改 `/home/api/sub2api-deploy/docker-compose.yml` 的 Sub2API 镜像，也没有执行该项目的 `up`。当时交付给维护者的手工切换步骤只针对 Sub2API 服务：

```bash
cd /home/api/sub2api-deploy
docker compose config -q
docker compose up -d --no-deps sub2api
```

该阶段切换前要求确认无在途媒体创建；切换后核对 `-version`、250 条 public 迁移、`enabled=false` 与 `preview_user_ids: [1]`、用户 1 的积分菜单/`/points`/user ticket、非白名单用户三层拒绝、管理员新标签配置入口、媒体冻结核销、容量精确重试和跨协议终态。PostgreSQL、Redis、积分服务和生图工作台不得随该命令重建。

`2026-07-31 20:58 CST` 最终只读复核确认：Sub2API 仍为容器 `bfd162bb9380...`、镜像 `0.1.168-339422728b2c`，积分服务仍为容器 `ac58ec881e35...`、镜像 `0.1.169-04a19ca082ee`；两者均 running、healthy、restart count 0。Sub2API 新候选只存在于服务器镜像缓存，Compose 仍引用旧生产标签。Nginx 为 active，PostgreSQL 与 Redis 均 healthy；Sub2API 本地/公网健康检查、积分本地健康检查和生图工作台本地/公网首页均返回 200，积分公网根路径与健康端点保持 404、无票据的 `/app/` 与 `/admin/` 保持 401。宿主 exact-root、`/` 和 `/index.html` 的 SHA256 继续一致。该复核没有修改配置、数据库、容器或镜像。

### 11.6 `04a19ca` 切换后的历史状态与用户 1 预览准备

- `2026-07-31 22:05 CST` 只读核对确认，维护者已经把 Sub2API 手工切换到 `ghcr.io/hxly520/sub2api:0.1.169-04a19ca082ee`。运行容器为 `c37a7a014997...`，OCI revision `04a19ca082ee43853573795d1385727bd38f20e9`，image ID `sha256:b2fcce5b...`，running、healthy、restart count `0`；Compose 已引用相同不可变标签。
- 同次核对的积分服务为容器 `e92b5ddc872b...`、镜像 `ghcr.io/hxly520/sub2api-points:0.1.169-04a19ca082ee`、同一 OCI revision，image ID `sha256:568c1af8...`，running、healthy、restart count `0`。PostgreSQL 与 Redis 均保持原容器并 healthy。
- `2026-07-31 22:05 CST` 的运行进程仍加载 `POINTS_SYSTEM_ENABLED=false`，当时尚不具备本轮新增的预览名单代码。`2026-07-31 22:06 CST` 已在不重启、不替换任何容器的前提下，把 `POINTS_SYSTEM_PREVIEW_USER_IDS=1` 原子写入 `/home/api/sub2api-points/bridge-secrets.env`；文件保持 `0600 root:root`，备份位于 `/home/api/sub2api-deploy/backups/points-preview-user1-20260731T220613+0800`，随后 `docker compose config -q` 通过。该配置随后在维护者切换 `f79803bb73d6` 时加载，完成状态见第 11.8 节。
- 下一候选必须保持全局 `enabled=false`，仅向用户 ID 1 返回 `points_system_access=true`；菜单、`/points` 路由和 user launch ticket 三层都使用该服务端决定，其他用户不得通过手工 URL 绕过。管理员配置改为隔离 opener/referrer 的新标签页，原 Sub2API 设置页保留；用户 iframe 实时跟随 Sub2API 明暗主题，并严格校验消息来源，不能再影响官方导航主题。

### 11.7 最终预览候选与积分独立切换

独立复审在首个 `5b27f0b80337` 候选中发现两项预览边界：公共设置加载失败或陈旧全局值可能覆盖当前用户的显式拒绝；从全站收窄时旧积分 user session 只复核 policy、不复核预览名单。当时生产活跃积分 session 为 0，所以没有即时越权，但该候选仍被作废且从未替换任何生产容器。最终提交 `f79803bb73d659e36627d6f716aab065ff4d56a6` 让逐用户布尔值优先，并给积分服务增加必填的 `all|preview` 运行门禁，在 ticket 交换和每一次现有 user session 请求上重复校验；非名单旧 cookie 会被拒绝并清除。

受控本机构建使用 Go `1.26.5`、pnpm `9.15.9`、`CGO_ENABLED=0`、`linux/amd64` 和标准 Docker archive。前端 typecheck、ESLint、全量 Vitest 和 production build 全部通过；积分服务 `go test ./...` 与 `go vet ./...` 通过。三个 GHCR tag 均逐一回读 manifest，revision 和 version 标签分别精确为完整 `f79803bb73d659e36627d6f716aab065ff4d56a6` 与 `0.1.169`。

| 制品 | 标签 | registry digest | image ID | archive SHA256 | bytes |
| --- | --- | --- | --- | --- | ---: |
| Sub2API 最终候选 | `ghcr.io/hxly520/sub2api:0.1.169-f79803bb73d6` | `sha256:efeac9309f33a9ccd204932ed1144955641a78260982c3041842fb89c71caa49` | `sha256:87856a8c2d35bbe1b035b2244e43dabfd385313515cb22c72853d302ee6eef97` | `757340c996aae3696cf6e740f1883e8032d7703b51f2c3c398acb5ec55174703` | 88,132,096 |
| 积分服务 | `ghcr.io/hxly520/sub2api-points:sha-f79803bb73d6`、`0.1.169-f79803bb73d6` | `sha256:d5325808dc2950632f4d4f98ff87a167265d0dbf2a45e9f0b8e446bd51c96876` | `sha256:297e7779eae338219f94545b534c3887dc1424a2401a0b9ed9a02008d1a53352` | `ac0175ab58d82d1801157fcc45e40330705f6ab64b59e345f8ba9627a3d73763` | 20,711,936 |

服务器归档分别为 `/home/api/sub2api-deploy/image-archives/sub2api-0.1.169-f79803bb73d6-linux-amd64.tar` 与 `/home/api/sub2api-points/releases/sub2api-points-0.1.169-f79803bb73d6-linux-amd64.tar`，均为 `0600 root:root`；服务器 SHA256 与本地一致。Sub2API 只执行 `docker load` 和 label/image ID 检查，没有修改 `/home/api/sub2api-deploy/docker-compose.yml`，也没有对主项目执行 `up`。

积分切换前完整备份位于 `/home/api/sub2api-deploy/backups/points-f79803bb73d6-20260731T232339+0800`，目录 `0700`、文件 `0600`。它包含 `points` custom dump、176 行 catalog、全数据库 schema-only、切换前后 Compose/环境/容器 inspect、数据库断言和 SHA256 清单；dump SHA256 为 `2fee2117562cd6405d53cc054e4ee8e5171a80a05479f2ff22c40db4bda57429`。首次使用错误容器名而提前终止的无 dump 目录已明确改名为 `points-f79803bb73d6-20260731T232205+0800.incomplete`，不得作为恢复点。

积分服务只原子增加 `POINTS_USER_ACCESS_MODE=preview`、`POINTS_USER_PREVIEW_IDS=1`，并把 `/home/api/sub2api-points/compose.yml` 镜像改为最终不可变标签；`docker compose config -q` 后仅执行 `docker compose up -d --no-deps --pull never points-system`。新容器 `05f43434fc20...` 为 healthy、restart count `0`、用户 `65532:65532`、只读 rootfs、PID 128、`cap_drop: ALL`、`no-new-privileges`，启动以来 ERROR 日志为 0。切换前后 21 表、3 迁移、29 账户、316 快照、316 修订、311 账本、`needs_review=0`、policy v3、40 天成功历史 job、签到/尝试/发放/冲正均完全一致；最新 startup 对账为 `14` 用户、`19056` 源行、零用户变更、零消费和零积分差量，未重跑历史作业。

真实签名票据验证中，非预览用户 2 在积分端返回 `403`；用户 1 的嵌入工作区、历史非空用户名字段、无 `user_id` 浏览器字段、每日积分接口和注销均正常；管理员独立工作区与注销正常，结束后活跃积分 session 为 0。该项仅记录旧镜像事实，登录邮箱候选必须重新断言 `login_email` 且不得继续返回 `username`。积分本地 health 为 200，公网根和 health 保持 404，无票据 `/app/` 保持 401，Nginx active。Sub2API 容器仍精确为 `c37a7a0149975da70d81481d2d173992628e8bbdd93b79b872c7904462060844`，启动时间 `2026-07-31T13:08:41.194414812Z`，镜像仍为 `0.1.169-04a19ca082ee`，healthy、restart count `0`。

维护者手工切换 Sub2API 时只使用 `ghcr.io/hxly520/sub2api:0.1.169-f79803bb73d6`，继续加载当前 root-only `bridge-secrets.env` 的 `POINTS_SYSTEM_ENABLED=false` 与 `POINTS_SYSTEM_PREVIEW_USER_IDS=1`，禁止使用旧的 `bridge-secrets.ready.env`。切换后先验收用户 1 菜单、右侧嵌入页与主题同步，再验收非名单菜单/路由/ticket 三层拒绝和管理员新标签；调试通过前不得改为全站 enabled。

### 11.8 维护者完成 Sub2API 切换与下一积分候选

- `2026-08-01 01:04 CST` 只读检查确认，维护者已完成 Sub2API 手工切换。当前容器完整 ID 为 `63d320fbf6ca2e091b349eddb45b9b70f5d3f45d73da8f8660dfe3473a665033`，镜像 `ghcr.io/hxly520/sub2api:0.1.169-f79803bb73d6`，OCI revision `f79803bb73d659e36627d6f716aab065ff4d56a6`，启动时间 `2026-07-31T15:47:53.260825915Z`，healthy、restart count `0`。积分容器仍为 `05f43434fc204efef8f2294b1837d3beb2cbb285aca58208b0f2a9d47b2dc065`、同 revision、healthy、restart count `0`。该检查没有修改服务器。
- `2026-08-01 00:05 CST` 的自动调度已成功结算业务日期 `2026-07-31`：来源用户 12、来源行 30,078、变更用户 12、消费差量 `498488911` microUSD、积分差量 `498483` hundredths。随后只读计数为 29 账户、328 快照、328 修订、322 账本、`needs_review=0`、签到/签到尝试/余额发放/发放尝试均为 0；用户 1 总积分 `9283.00`、当日业务快照积分 `1768.06`。该调度不是历史基线重跑。
- 下一积分独立候选把浏览器身份改为 `login_email`，个人消费积分的“发放时间”按业务日期次日 `00:05` 投影，修复刷新/签到异步状态，并把四张概览卡收紧为等高布局。个人积分记录和签到奖励记录分别默认每页 10 条，使用独立前后翻页。
- 该候选依据 `design-taste-frontend` 的重设计审计做定向演进，不改变信息架构或权限边界：浅色模式复用 Sub2API 的中性浅色与 teal 主色，深色模式复用 `dark-950/dark-800/dark-700` 层级；表头、表格行、悬停、分页、状态标签及 Canvas 曲线完整随父主题切换。父主题一旦通过严格 Origin/source 校验到达，后续 `/api/v1/me` 刷新不得用旧启动主题覆盖。
- `2026-08-01 01:25 CST` 曾把旧的一步收敛草案临时替换为 `ROLLBACK` 做生产预演。事务 `1947043` 验证升级前 `email=false/username=true`、草案临时态 `email=true/username=false` 后完整回滚；该记录只证明当时生产 ACL 没有变化，不能作为两阶段模板已执行的证据。该历史 `id/username/deleted_at` 状态随后由第 11.9 节的正式阶段 A 替代。
- 候选要求的数据库备份、阶段 A 和积分服务独立切换已按第 11.9 节完成；真实用户/管理员票据验收与阶段 B 尚未完成。阶段 B 后回滚仍须先恢复双读再切旧镜像。Sub2API 容器完整 ID、启动时间、镜像和 restart count 必须保持上述值，签到及全站开放继续关闭。

### 11.9 Taste 积分候选构建与独立上线

- `2026-08-01` 的最终源码节点为 `7e50f9aa9f395f00f235365376d33f859ae7d16f` 和无障碍布局修复 `e39c78bf8f6c00230d2756493b9c951a2c39d4fa`。后端全量 test/vet/build、积分 test/vet/build 与 integration tag 编译、前端 ESLint/typecheck/全量 Vitest/production build、三个积分脚本语法和浏览器多视口 QA 均通过。桌面/移动四卡高度差为 `0`，`390px` 无横向溢出，浅/深 hover 对比度分别约 `16.26:1`、`9.88:1`。
- GitHub Actions `30662285938` 与 `30662288244` 均绑定完整 revision `e39c78bf8f6c00230d2756493b9c951a2c39d4fa`，但在执行任何 step 前被账户付款/支出限额拒绝。随后按受控本机构建回退生成 `linux/amd64` 二进制层，复用未变化的既有 Alpine 运行时层，逐层回读远端二进制 SHA256、所有权、权限、入口、架构与 OCI revision；生产服务器只执行鉴权拉取、`docker save` 和积分服务更新，没有编译或构建镜像。

| 制品 | 标签 | registry digest | image ID | 服务器 archive SHA256 | bytes |
| --- | --- | --- | --- | --- | ---: |
| Sub2API 手工候选 | `ghcr.io/hxly520/sub2api:0.1.169-e39c78bf8f6c` | `sha256:344bf36a7cb21f7f0935f7cc5803cf2a2b83273054a8c07fd2dae0148203d910` | `sha256:2e58e6984e58c0c416695c899fc76fd31ef5165d62b46ff22bf6335240249be3` | `5eed097d2c65535747f85eed4fc0b3de105248da0c35ea8d0efd45c4868dcc61` | 360,636,928 |
| 积分服务 | `ghcr.io/hxly520/sub2api-points:0.1.169-e39c78bf8f6c`、`sha-e39c78bf8f6c` | `sha256:502abb9dbffa5237b388f70208ec0e72550b126baa398921aad9c4884048d2eb` | `sha256:d34ad6906ba39d904301c5cd6f6194cea06b5c9356cfe2547a447d91eb85d223` | `8c9218ef1270d37a2a48f9d5a83882dcda32beec8cd5959e680240ec7d7cce13` | 71,173,120 |

- Sub2API archive 位于 `/home/api/sub2api-deploy/image-archives/sub2api-0.1.169-e39c78bf8f6c-linux-amd64.tar`，只供维护者手工切换；运行中的 Sub2API 仍为容器 `63d320fbf6ca...`、镜像 `0.1.169-f79803bb73d6`、healthy、restart count `0`。积分 archive 位于 `/home/api/sub2api-points/releases/sub2api-points-0.1.169-e39c78bf8f6c-linux-amd64.tar`。两者均为 `0600 root:root`，临时 registry 凭据和 `.partial` 文件均已清理。
- 切换前全库 custom backup 为 `/home/api/sub2api-points/backups/sub2api-before-points-e39c78bf8f6c-20260801-043917.dump`，SHA256 `099a567598f64b07b25678e9fccb43f941f3daf41191beaddf73369f1fbc4574`，大小 `98,757,221` 字节，catalog `1,290` 行。阶段 A 事务 `1960217` 后 allowlist 精确为 `id/email/username/deleted_at`，整表和敏感列读取仍被拒绝。
- 积分 Compose 回滚点为 `/home/api/sub2api-points/backups/compose.pre-e39c78bf8f6c-20260801-045914.yml`。仅执行 `docker compose up -d --no-deps points-system` 后，新容器为 `11b1f9820fa7...`，healthy、restart count `0`；本地 health 为 `200`，本地/公网根和公网 health 仍为 `404`。启动对账 run `8b8a2d7b-17c9-4e9a-8074-105307203f5d` 读取 `30,078` 行、`12` 个来源用户，`changed_users=0`；用户 `187`、积分账户 `29`、快照 `328`、账本 `322` 均与切换前一致。
- 浏览器没有可复用登录态，`/points` 被正确重定向到登录页，因此本轮没有伪造真实用户或管理员票据。阶段 B 明确保留为待办：维护者需登录后验证用户 1 嵌入页 `login_email`、浅/深主题、两表分页、刷新恢复、管理员用户明细和发放记录，并验证非预览用户拒绝；全部通过后才能执行 `shared-database-users-email-finalize.sql.example` 撤销 `username`。签到和全站开放继续关闭。
- `2026-08-01 05:20 CST` 延迟只读复查确认积分容器仍为 `11b1f9820fa7...`、revision `e39c78bf8f6c...`、healthy、restart count `0`；Sub2API 仍为 `63d320fbf6ca...`、revision `f79803bb73d6...`、healthy、restart count `0`。积分启动后无 ERROR/WARN，`04:59` 启动对账仍为 `changed_users=0`；187 用户、29 账户、328 快照、328 修订和 322 账本未变化，签到及余额发放表仍为空。Sub2API 在 `05:12:43` 有一条上游 WebSocket `close 1006 (abnormal closure): unexpected EOF` 告警；上游已开始写流，访问日志最终为 `200` 且未写兜底错误，能解释无标准错误码的会话突然中断，不属于积分服务故障。该复查只执行容器、HTTP、日志和只读事务查询。

### 11.10 中间 Taste 状态与响应式节点（已并入 11.11）

- 中间源码节点在 `e39c78bf8f6c` 之上按 Taste 参数 `VARIANCE 4 / MOTION 3 / DENSITY 6` 收口视觉和完整状态循环：移动 Canvas 使用真实宽度并按约 `88px` 间距动态减少日期；表格 hover 降为轻量提示，签到赠送表缩小独立最小宽度；管理员待处理卡使用 amber 告警层级，策略阶梯改为表单内分隔行。该节点继续使用现有 CSS token、Lucide sprite 和原生 Canvas，没有引入第二套组件运行时。
- 用户首轮必需请求失败时显示持久错误面板与重试，不再把默认零值当作真实数据；管理员策略台等首轮汇总完成后才揭示。管理员新标签的 `about:blank` 等待页按 Sub2API 浅深主题渲染，带 `color-scheme`、主题色、语言、无障碍忙碌状态和 reduced-motion 降级，同时保持 opener/referrer 隔离。
- 历史积分账本在业务日期早于首个策略生效日时，改用账本固定的 `policy_version` 回退刷新分钟，确保发放时间仍为业务日期次日 `00:05`，不修改账本或重跑历史作业。用户明细失效游标自动回第一页；管理员首轮失败保留原地重试。签到待确认意图使用服务端业务日期、登录邮箱、签到前次数和原幂等键绑定到当前浏览器会话，POST 成功后也要等权威次数增加再清理；行为夹具已验证连续重试与页面重载均复用同一幂等键。
- 该中间节点已用真实 Sub2API production build 与积分 iframe 双服务夹具完成桌面浅色、桌面深色、`390px` 嵌入和持久错误态截图；父页与 iframe 均无横向溢出，嵌入页隐藏自身 topbar，上传 Logo 与父主题保持权威。全部改动随后并入第 11.11 节的 `e8d73f3e6655` 并重新构建；任何较早的 `94b417597017` 制品均不是当前生产制品。

### 11.11 Taste 数字工作区增强（已构建、积分已独立上线）

- `2026-08-01` 在 `94b417597017831c651cfda342afcec4a7d0291b` 之上把积分视觉基线调整为 `VARIANCE 6 / MOTION 4 / DENSITY 5`。积分 iframe 内新增低对比精密网格、主积分焦点、真实同步状态、统一 8px 面板尺度和紧凑结算说明；管理员策略台使用同一视觉语言。改动只作用于积分子页面，Sub2API 左侧导航和 Header 继续完全由父页控制，浅色/深色主题通过原有严格 Origin/source 消息同步。
- 本轮继续使用现有 CSS token、许可 Lucide sprite 和原生 Canvas，没有新增组件运行时或前端依赖。悬停位移最多 2px，`prefers-reduced-motion` 下全部取消；独立用户页 sticky 顶栏、嵌入隐藏顶栏、父页 Logo 和角色权限边界均保留。
- 浏览器夹具确认桌面浅色、桌面深色、`390px` 和持久错误态均无横向溢出，四卡高度差为 `0`，Canvas 非空，深色表格和悬停文字可读。积分服务全量 test/vet/build、前端全量 Vitest、ESLint、typecheck 和 production build 均通过；新增行为测试实际执行用户脚本，确认签到资料连续失败时仍只重放原幂等键。
- 完整源码 revision 为 `e8d73f3e665596fc0d9e185d8ce706c45d04438a`。本地 `linux/amd64` 制品通过 OCI 架构、label、healthcheck、entrypoint、UID/GID、权限、payload SHA256 和 `crane validate`；registry digest、服务器加载后的 image ID 与传输 archive SHA256 分开记录如下，不能互相替代：

| 制品 | 标签 | registry digest | image ID | archive SHA256 | bytes |
| --- | --- | --- | --- | --- | ---: |
| Sub2API 手工候选 | `ghcr.io/hxly520/sub2api:0.1.169-e8d73f3e6655` | `sha256:235add58b97e2f3c6bf06a65be86018dee4fac13769c280f1a508096f3bac78b` | `sha256:d5b2fa2b40d60550d6747a837eb517d691278ffc756c479db998edd0ccda3a66` | `c614b3d2eaca6963410ec9419188e6855fb2be58a860f56ab0d37b5666111346` | 208,714,240 |
| 积分服务 | `ghcr.io/hxly520/sub2api-points:0.1.169-e8d73f3e6655`、`sha-e8d73f3e6655` | `sha256:4b18317d48783df966cf570e7bf7aa59b8261561d280de161ace137b41509746` | `sha256:0b8a4732a17bce4ce9ff513f10c0599e736dd8ba69f16d4438f178cf8d96e9a9` | `2947440cfdf4dedd6bea09341969e99d9dfd49ef54c2739269e2831813a66925` | 47,331,840 |

- 两个 `0600 root:root` archive 已上传至 `/home/api/releases/` 并在服务器复核 SHA256 后执行 `docker load`；服务器没有编译或构建。Sub2API 新镜像只完成导入，运行容器继续是 `63d320fbf6ca...` / `f79803bb73d6`，启动时间和 restart count 均未变化，仍由维护者手工切换。
- 积分切换前全库 custom backup 为 `/home/api/backups/points-release-e8d73f3e6655-20260801-081507/sub2api-full.dump`，SHA256 `843ff7734791e92517c9bb02b56eb7b033c1e0662b50936d0cf44ea4d4c76df7`，大小 `99,384,093` 字节，`pg_restore -l` 共 1,291 行；同目录保留原 Compose、切换前后计数、容器身份和启动日志。服务器未登录 GHCR，故 Compose 以 commit 标签、`pull_policy: never` 和加载后 image ID 三重约束本地制品，未把 registry digest 冒充为 daemon `RepoDigest`。
- `2026-08-01 08:17 CST` 仅执行 `docker compose up -d --no-deps --pull never points-system`。新积分容器 `1166d2ff140c...` 使用预期 image ID，healthy、restart count `0`；启动对账 run `0144d6a3-3158-4e73-ad94-e03de54dcb9a` 读取 30,078 行、12 个来源用户，`changed_users=0`，启动日志无 error/fatal。29 账户、328 快照、328 修订、322 账本、187 个 Sub2API 用户均未变化，`needs_review=0`，签到、签到尝试、余额发放和发放尝试仍为 0，policy v3 的 `10.00:1`、`00:05` 和签到关闭均未变化。
- 本地服务 health 为 `200`，本地/公网根与公网 health 为 `404`，未认证 `/app/` 为 `401`，`nginx -t` 与 Compose config 均通过。生产继续只开放用户 1 预览；本轮未伪造认证票据，真实用户 1 与管理员工作区验收及 ACL 阶段 B 仍由维护者登录后完成。

### 11.12 用户 1 签到实测、审计兼容与 Sub2API 手工候选

- `2026-08-02` 积分服务已独立运行 `ghcr.io/hxly520/sub2api-points:0.1.169-ca18cf77a86a` / `ca18cf77a86a921600e7324a75d09188e1e4fed7`。容器 `c7d819ea0ea9...` healthy、restart count `0`；registry digest、image ID、archive SHA256 分别为 `sha256:b9f9b0c4924d73fb84a8a52ff6551b08cad3fb03c3c2797b705e55553118a6d2`、`sha256:f77aabccae46c02549775b242ddfd42002d1aff2a114ef4c131837adf691c64d`、`34bc3495c1b5f1811af539f82fdb1e899055a171a665690bb140db3a5b679e40`。
- 积分中心门禁保持 `all` 且名单为空；签到独立门禁为 `preview` 且名单精确为用户 1。policy v4 自 `2026-08-02` 生效，使用 `consumer_only/yesterday`、每日 1 次、`1000-50000 PPM`（`0.1%-5%`）和三个 `100 U` 绝对安全上限。`2026-08-02 00:05 CST` 自动快照成功，用户 1 昨日消费 `86.890694 U`、昨日积分 `868.90`。
- 用户 1 实际签到的理论区间为 `0.08-4.34 U`，抽中 `35537 PPM`，最终赠送 `3.08 U`。交易 UUID `8e20f4f9-d3ab-4d16-95be-0b186c96da97` 为 `settled`，Sub2API credit 记录恰好 1 条，余额结果 `8993.5808432400`；同一幂等键重放仍返回同一 settled grant，未重复加款，新的幂等键被每日次数限制以 `409` 拒绝。
- 用户 2 的积分中心访问不受影响，但签到资料显示不可用，签到 POST 返回 `403 checkin_unavailable`；非用户 1 的签到、签到尝试和每日计数均为 0。这是当前验收基准：禁止为放开签到而把 `POINTS_USER_ACCESS_MODE` 收窄，也禁止为开放积分中心而清空独立签到预览门禁。
- 首次余额 credit 曾返回 `500`：当前生产 `f79803bb73d6` 在插入 `audit_logs` 时把 `request_body` 写为 `NULL`。积分服务保持原 UUID 重试，最终只结算一次；生产数据库临时安装 `public.points_credit_audit_request_body_compat()` 与同名触发器，只对 `action='points.balance_credit' AND request_body IS NULL` 写入兼容空值，不撤销 `request_body` 的非空约束，不影响其他审计 action。
- 修复提交 `1a4a690dd999b669e2ce09522854ea157d7af984` 已生成并加载 `ghcr.io/hxly520/sub2api:0.1.169-1a4a690dd999`。registry digest 为 `sha256:d9646464040e846999f960e3050646fcfe7cac38695834ba85df21385ae5c3ef`，image ID 为 `sha256:07303dd1787d08a3038ba347a3fdaf0f78296f5f7a01aaf67ccce31edcd4ab16`，archive SHA256 为 `302f996c047c09919e8af53455851f0e18d7fd53d9c06640f8b2e3de7398c477`，服务器文件为 `/home/api/sub2api-deploy/releases/sub2api-0.1.169-1a4a690dd999-linux-amd64.tar`。Compose 仍指向 `0.1.169-f79803bb73d6`，当前 Sub2API 容器 `dee0f8efd24d...` healthy、restart count `0`；只能由维护者手工切换。
- 维护者手工切换并确认新镜像 credit 审计、同 UUID 幂等重放和余额缓存均正常后，删除临时兼容对象：

```sql
DROP TRIGGER IF EXISTS points_credit_audit_request_body_compat ON public.audit_logs;
DROP FUNCTION IF EXISTS public.points_credit_audit_request_body_compat();
```

- 本次变更前的恢复点为 `/home/api/backups/points-checkin-user1-20260801-232458`；全库 dump SHA256 `2bfca513c1d7f0be7db1aaecf31f5e3004a43eb8c150d1d5e7a2dce4e5e9df98`，catalog `1,291` 行。它同时保存积分 Compose/env 和两服务切换前 inspect；恢复演练仍不得直接在生产数据库执行。

### 11.13 最终消费阶梯积分镜像独立切换

- `2026-08-02 09:19 CST`，服务器从本地构建归档独立切换积分服务到 `ghcr.io/hxly520/sub2api-points:0.1.169-1d8d50522429`。运行容器为 `1e5ed38b81da...`，image ID `sha256:5d4edb7822499e7c2953f7aa1f4889d88fbd9ac630fce69742d0a4f694e192dd`，OCI revision `1d8d50522429b5d943766ad1d1b4a14b82e31d80`，linux/amd64、healthy、restart count `0`。服务器归档为 `/home/api/sub2api-points/releases/sub2api-points-0.1.169-1d8d50522429-linux-amd64.tar`，SHA256 `e33e80c5b28307120881ccf269ebd7b5cae46c447173cc136182643ef56d960b`；未在生产机编译或构建。
- 切换前完整备份为 `/home/api/backups/points-spend-tiers-20260802-091604`，目录权限 `0700`、文件权限 `0600`。全库 custom dump 为 `103,217,039` 字节，SHA256 `dae794a05a1d43bd13e2c9baab55969b35d4fb7f08420899d9cddb8ad0634e24`，catalog `1,294` 行；目录同时保存积分 Compose/env、Sub2API Compose/env 和两服务 inspect。积分 Compose 回滚点为 `/home/api/sub2api-points/backups/compose.pre-1d8d50522429-20260802-091919.yml`。
- 唯一执行的容器替换为 `docker compose up -d --no-deps --pull never points-system`。切换前后 Sub2API 始终是容器 `dee0f8efd24d...`、image ID `sha256:87856a8c2d35bbe1b035b2244e43dabfd385313515cb22c72853d302ee6eef97`、启动时间 `2026-08-01T14:11:17.205206857Z`、restart count `0`；PostgreSQL 与 Redis 身份也完全未变。Nginx active，生图工作台继续运行原 `infinite-canvas` 容器。
- 新容器启动时应用 `migrations/004_checkin_spend_tiers_and_optional_caps.sql`；积分 schema 仍为 21 张表，迁移数从 3 变为 4。启动对账业务日期 `2026-08-01`，读取 11 个来源用户、14,319 条来源记录，`changed_users=0`、消费差量和积分差量均为 0。切换后仍为 29 个积分账户、339 条快照、333 条账本、1 条签到和 1 条余额发放；除追加策略与管理员审计外没有改写业务积分或重复发放。启动后无 WARN/ERROR，积分本地 health 返回 `200`，公网根与 health 继续 `404`，无会话 `/app/` 继续 `401`。
- policy v5 通过一次性管理员票据、CSRF 与 `POST /api/v1/admin/policies` 正式保存，`policy.create` 审计 actor 为用户 1，生效日为 `2026-08-03`。策略为 `enabled=true`、`consumer_only/yesterday`、`checkin_tier_basis=spend`、最低昨日消费 `1 U`、每日 1 次、比例 `10.00:1`、刷新 `00:05`、三个金额 cap 为 `NULL`；四档依次为 `[1,10) U -> 1%-5%`、`[10,50) U -> 2%-5%`、`[50,100) U -> 3%-5%`、`[100,+∞) U -> 4%-5%`。保存会话已注销，首次契约探测产生的临时管理员会话也已精确清理。
- `2026-08-02` 当前生效策略仍是 policy v4，用户 1 当天已完成签到，不进行第二次真实资金测试。待 `2026-08-03 00:05 CST` 自动快照完成后，才使用用户 1 验证 v5 的实际判档与金额；签到门禁继续为 `preview/1`，不得提前向其他用户开放。
- Sub2API 候选仍为服务器已加载但未切换的 `ghcr.io/hxly520/sub2api:0.1.169-1a4a690dd999`，归档 `/home/api/sub2api-deploy/releases/sub2api-0.1.169-1a4a690dd999-linux-amd64.tar`、SHA256 `302f996c047c09919e8af53455851f0e18d7fd53d9c06640f8b2e3de7398c477`、image ID `sha256:07303dd1787d08a3038ba347a3fdaf0f78296f5f7a01aaf67ccce31edcd4ab16`。Compose 仍指向 `f79803bb73d6`，必须由维护者手工修改标签并只启动 `sub2api` 服务；在候选 credit 验收完成前，精确兼容触发器继续保留。
