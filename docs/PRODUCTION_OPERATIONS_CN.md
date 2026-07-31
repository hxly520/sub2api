# 生产运维、只读盘点与版本交接

本文档记录私有 Sub2API 的生产拓扑、只读盘点基线、镜像发布边界和版本交接流程。它只保存脱敏事实，不保存密码、API Key、数据库连接、证书私钥、Worker Secret、完整环境变量或原始日志。

当前生产事实见 [2026-07-31 积分激活与后续候选交接](PRODUCTION_DEPLOYMENT_20260731_CN.md)。[2026-07-30 v0.1.168 候选与积分系统生产记录](PRODUCTION_DEPLOYMENT_20260730_CN.md) 只保留首次部署证据；后续排障不得把其中的旧积分镜像、disabled policy 或迁移数量当成现状。

## 1. 权威来源

- 私有仓库：`hxly520/sub2api`。
- 当前仓库候选代码基线：官方 Sub2API Release `v0.1.169`（commit `26d894ef4f50645a4bf1030e378ac892f17d0223`）通过 merge `3da18b9dd2d0ecc890a5605a4d1cf97093a8659e` 与私有兼容层汇合；关键功能节点为媒体核销 `9f1b6bae`、积分 `e4179147`、同库隔离 `55ac503b`、公开首页 `7e598fbb`、历史积分与容量精确重试 `d6b367f31`、管理员用户积分明细 `28e760bc8`、嵌入式积分大屏 `874255bcd` 和升级前安全/终态收口 `1e33e7f7a`；`backend/cmd/server/VERSION=0.1.169`。生产运行版本与源码候选必须按下一条分别判断。
- 当前生产 Sub2API 为 `ghcr.io/hxly520/sub2api:0.1.169-04a19ca082ee`，OCI revision `04a19ca082ee43853573795d1385727bd38f20e9`，容器 `c37a7a014997...`，运行态 healthy、restart count `0`。后续 Sub2API 镜像仍须先构建上传，再由维护者手工切换，自动化不得替换运行中的 Sub2API。
- 当前生产积分服务为 `ghcr.io/hxly520/sub2api-points:0.1.169-04a19ca082ee`，同一 OCI revision，容器 `e92b5ddc872b...`，运行态 healthy、restart count `0`。积分镜像可在备份 `points` schema 后独立更新，但必须保证 Sub2API 容器 ID、启动时间和镜像引用不变。
- 生图工作台唯一源码：[`hxly520/infinite-canvas`](https://github.com/hxly520/infinite-canvas) 的 `main`；它独立于Sub2API版本发布。
- 独立积分系统源码：本仓库 `points-system/`。它复用现有 PostgreSQL 实例和 `sub2api` 数据库中的独立 `points` schema，不再部署第二个 PostgreSQL；实际容器、schema、域名和 Nginx 状态以本文件后续发布记录为准。
- 生产主机：`107.172.147.76`，SSH登录用户为 `root`；认证信息由仓库外的密码管理系统保存。
- 生产部署根：`/home/api/sub2api-deploy`。
- 生产变更原则：镜像必须在 CI 或受控本机构建环境完成并验证，服务器只负责拉取或 `docker load`；不在生产机编译源码、前端、二进制或镜像。

正式维护不得使用 shallow、partial clone 或 sparse checkout。升级前必须确认完整历史、完整对象和官方 tag均已获取。

## 2. 2026-07-26 历史只读生产快照

盘点时间：`2026-07-26 20:41 CST`。本节只用于还原升级前历史状态，不代表当前生产；盘点期间未安装软件、写文件、重启、reload或修改配置。

| 项目 | 已确认状态 |
| --- | --- |
| 操作系统 | Ubuntu 24.04 LTS，Linux `6.8.0-136-generic` |
| 主机资源 | 2 vCPU，1.9 GiB内存，1 GiB Swap；根盘34 GiB，剩余约26 GiB |
| Docker | Docker `26.1.4`，Compose `v2.27.1` |
| Sub2API | `gptcodex/sub2api:0.1.164-08fbef836b9c`，OCI revision完整匹配 `08fbef836`，healthy，restart/OOM均为0 |
| PostgreSQL | `postgres:17.8-alpine`，healthy，数据库约935 MB |
| Redis | `redis:8-alpine`，服务版本8.8.0，healthy，约3,079个键、266个连接、2.92 MB数据 |
| 画布工作台 | `ghcr.io/hxly520/infinite-canvas:sha-24f7826`，revision `24f7826cc9b93a64b1a0d9d80a9de825e32f52cd`，首页HTTP 200 |
| 积分与签到 | 该历史时点没有对应容器、端口、数据库或 Nginx 入口；当前状态见 2.2 节 |
| Nginx | `1.20.1`，手工维护二进制，`nginx -T`验证成功，systemd restart为0 |
| 本地端口 | Sub2API `127.0.0.1:8080`；画布工作台 `127.0.0.1:15731` |
| 对外端口 | `22/80`监听IPv4和IPv6；`443`仅IPv4 |
| 镜像回滚 | 当前daemon没有上一版Sub2API镜像；所有镜像均无 `RepoDigests`，尚无可验证的本机不可变回滚点 |

该历史时点的镜像版本、OCI revision 和仓库私有 v164 应用代码完全一致。`08fbef836` 仍是升级差异审计与二开回归的历史基线，但当前生产版本必须以 2.2 节的镜像、revision 和容器运行态为准，不能只凭前端版本文字判断。

### 2.1 2026-07-29 媒体异常冻结核销

- 核销前发现24条历史异常冻结，共 `2.42 U`。逐条关联媒体任务和成功结果后，22条无出图证据的冻结退款 `2.32 U`；2条存在成功出图证据的冻结按原报价结算 `0.10 U`，对应 hold ID `298`、`323`。
- 核销在单一数据库事务中执行，完成后旧异常冻结为0；相关余额缓存随后失效。生产 Sub2API 镜像、容器、Nginx和画布工作台均未变更。
- 审计记录为 `audit_logs.id=3339`，request ID `hold-reconcile-20260729`。操作前回滚快照位于 `/home/api/sub2api-deploy/backups/media-hold-reconcile-before-20260729-195009.jsonl`，SHA256 为 `57770ff7ca39ba929a66efd8dce7c180babf887768dc0d39852055dd3b327fdd`。
- 私有提交 `9f1b6bae` 增加明确失败即时退款、未知终态保留冻结、成功费用按报价封顶及全站到期冻结后台核销；该功能继续包含在当前生产 `v0.1.169-04a19ca082ee` 基线中，后续升级仍须持续检查后台核销审计和异常冻结聚合。

### 2.2 2026-07-30 历史运行基线

- Sub2API：`ghcr.io/hxly520/sub2api:0.1.168-339422728b2c`，OCI revision `339422728b2ceb87b4a81bb08229d370c4ca589d`，image ID `sha256:a8abdd92bb9f59082d9bbecbdf4812b8f1a441aed4f189bc674104f07074625b`；容器 healthy，restart count `0`。
- 积分服务：`ghcr.io/hxly520/sub2api-points:0.1.168-c0fe91506bca`，OCI revision `c0fe91506bca60dfcc96b6d868b48b30d2ca86f0`，image ID `sha256:a6a899da83a5e82eccb9bd8c2473b72c274d32d6c78e8d50774654f21dfd166b`；容器 healthy，restart count `0`。registry digest 为 `sha256:8a9b7f51ce454450fc797aeeb7bfea008351cdba354327ae6cf40d3ddbdb4148`；服务器通过 SHA256 为 `d8bed76bd257e4ecb3e72dddb5e26c11147274738a3e7e316e2015c85568ef7d` 的本机构建归档执行 `docker load`，没有在生产机编译或构建。
- 两个服务复用 PostgreSQL `17.8` 的同一个 `sub2api` 数据库。积分写表位于独立 `points` schema，共 19 张表、2 条积分迁移；`points_app` 写角色连接上限为 8，`points_usage_reader` 只读角色连接上限为 4 且只允许读取 `usage_logs` 指定列。
- Sub2API `public.schema_migrations` 当前共 250 条记录，私有迁移 `192_media_balance_hold_reconciliation_index_notx.sql` 和 `193_points_balance_credit_ledger.sql` 均已应用；积分迁移只记录在 `points.points_schema_migrations`，两套迁移表不得混用。
- 首次积分部署前备份位于 `/home/api/sub2api-deploy/backups/sub2api-before-points-20260730-033634.dump`，SHA256 为 `b48e6a8d8bcb2d50f12e301da16ece87174f94edf535702aa12e575cbed3c293`。积分服务的启动、建 schema 和 Nginx 平滑 reload 均未停止 Sub2API 业务。
- Sub2API 加固镜像已由维护者切换上线；积分中文双工作区镜像也已独立切换，过程中 Sub2API 容器 ID、启动时间和镜像引用完全未变。积分切换前备份位于 `/home/api/sub2api-deploy/backups/points-ui-c0fe91506bca-20260730-190702/`，其中 `points-schema.dump` SHA256 为 `b9b0e477267b6710a8a3df8cb311f5f5707c9f1779cf265a47623e2ce98b53dd`。
- exact-root 公开首页由 Nginx 读取宿主 `/home/api/sub2api-deploy/public/index.html`，不属于 Sub2API 镜像层。`2026-07-30 16:19 CST` 已原子更新为仓库新版，宿主与 Nginx 响应 SHA256 均为 `86eb43d94050780fd9dc81da6e189c469f709bffa11c63eed195cdc065d229e5`；旧文件备份为同目录 `index.html.bak-20260730081909`，未重启 Nginx 或 Sub2API。

### 2.3 2026-07-31 当前运行基线

- `2026-07-31 22:05 CST` 只读核对：Sub2API 容器为 `c37a7a014997...`，运行 `0.1.169-04a19ca082ee` / `04a19ca082ee43853573795d1385727bd38f20e9`，image ID `sha256:b2fcce5b...`，healthy、restart count `0`；Compose 已引用同一不可变标签。
- 同次核对的积分容器为 `e92b5ddc872b...`，运行 `0.1.169-04a19ca082ee` / 同一 OCI revision，image ID `sha256:568c1af8...`，healthy、restart count `0`；Compose 已引用同一不可变标签。`points` schema 的 21 张表和 3 条迁移为此前已核验基线，普通更新不得重跑历史作业。
- policy v3 已启用，比例 `10.00:1`，刷新 `00:05`，签到关闭。全历史作业 `5174eef7-5f0a-4a17-b4f1-f50840940f64` 已成功；共有 29 个积分账户、316 条每日快照、311 条积分账本，`needs_review=0`，签到与余额发放记录均为 0。用户 ID 1 的核对值为总积分 `7514.94`、昨日积分 `938.07`。
- 上述历史作业是当前 schema 的一次性基线，后续积分镜像更新不得重新执行 `activate`、`plan`、`apply` 或 `resume`。日常积分只由 `00:05` 调度和滚动差量对账维护。
- 分阶段开放保持 `points_system.enabled=false`，并仅设置 `points_system.preview_user_ids: [1]`。`2026-07-31 22:06 CST` 已在不重启容器的前提下把 `POINTS_SYSTEM_PREVIEW_USER_IDS=1` 原子写入 root-only `bridge-secrets.env`，备份位于 `/home/api/sub2api-deploy/backups/points-preview-user1-20260731T220613+0800`，`docker compose config -q` 通过；该值只会在维护者手工切换包含预览能力的新候选时加载。用户 ID 1 届时可看到菜单、访问 `/points` 并获取 user ticket，其他用户即使手工访问 URL 也由服务端拒绝；签到仍由 policy 独立保持关闭。
- 4 个遗留 `points_shared_*` 测试 schema 已清理。清理前备份位于 `/home/api/sub2api-deploy/backups/points-test-schema-cleanup-20260731-090001`，dump SHA256 为 `ecfc41fb6d3fbd332b3b0b86f9f8707257a81d4c266d9c6a7d6290c9ce661c29`，catalog 659 行；清理后测试 schema 为 0，正式 `points` 计数未变化。

### 2.4 2026-07-31 同步生图冻结核销与参数验证

- 发现 10 条超过 30 分钟、无成功 usage/任务证据的同步图片 `dispatched` 冻结，共 `1.02 U`；用户 1 为 `0.20 U`，用户 160 为 `0.82 U`。root-only 操作前备份位于 `/home/api/sub2api-deploy/backups/media-hold-no-output-refund-20260731-092046`，custom dump SHA256 为 `77c60a5611000d4c3ae945f0ce71f85e99005393e1416a843a1ae5c98a9706b7`。
- 固定 ID、总额、用户数和成功证据断言均通过后，在单一事务中把 10 条全部退款并标为 `released`；审计为 `audit_logs.id=3814`、request ID `hold-refund-no-output-20260731`。完成后 active hold、active 金额和非零冻结用户均为 0，Sub2API 未重启或替换。
- 使用工作台精确自动质量参数的主分组请求在 99 秒后成功生成 1 图，hold `408` 已 `captured` `0.10 U`，usage `219522` 只记一次；生成后 active hold 和非零冻结用户仍为 0。参考图 SHA256 为 `bc8a8bcabcbdc33429d035e9be90d61bef539fbc03811b8bc53f575a93f5b6c6`。
- 当前 `v0.1.169-04a19ca082ee` 已把同步 `/v1/images/generations` 未知终态冻结窗口从通用 24 小时缩短为 30 分钟；明确失败仍即时退款，异步图片/视频仍使用 24 小时。后续升级不得回退该分流规则。

## 3. 域名和进程边界

活跃域名包括：

- `52token.org`
- `api.52token.org`
- `image.52token.org`
- `video.52token.org`
- `points.52token.org`

活动业务域名只允许上述 `52token.org` 系列。`gpt-codex.top` 及其子域名已经淘汰，不是生产入口、回滚入口或仓库外依赖；相关字符串只可能保留在历史备份或审计记录中，不得重新写入活动 Nginx、积分父站 Origin、前端配置或发布说明。

主要链路：

```text
Cloudflare -> Nginx :443 -> 127.0.0.1:8080  Sub2API
                         -> 127.0.0.1:15731  独立画布工作台
                         -> 127.0.0.1:8090   独立积分服务
Cloudflare Worker -> 加密媒体URL -> 上游媒体源
```

- `api.52token.org` 是Sub2API完整API入口。
- `image.52token.org` 只把允许的图片、Gemini、视频和任务路径转到Sub2API，其余页面转到独立画布工作台。
- `video.52token.org` 源站对媒体路径固定返回404，必须由Cloudflare Worker截获。
- `points.52token.org` 只开放一次性 ticket、积分页面资源和积分 API；根路径与公网健康路径返回404，无积分会话时页面和静态资源返回401。普通用户菜单、`/points` 和 user ticket 由 Sub2API 服务端按用户授权：`points_system.enabled=true` 时对全部有效用户开放；全局关闭时只允许 `points_system.preview_user_ids` 中的用户。积分服务还按当前生效 policy 对用户 ticket、页面、资源和 API 二次拦截；预览白名单不能绕过桥接完整性、policy、角色、ticket 或 session 校验。
- `15731` 工作台源码不属于本仓库，唯一源码来源是 [`hxly520/infinite-canvas`](https://github.com/hxly520/infinite-canvas)。生产实际运行的是该仓库构建镜像内的 `/usr/share/nginx/html`。
- `basketikun/infinite-canvas` 仅是工作台的上游参考与合并来源，不是52Token生产构建源；Sub2API仓库也不得复制一份工作台源码形成第三个来源。
- 宿主 `/home/api/sub2api-deploy/images` 只是未挂载、无 `.git` 的旧 `v0.5.0` 页面快照，已退出构建、发布、运行和回滚链路；Nginx不得以该目录作为页面fallback。
- 2026-07-26盘点时工作台镜像固定在 `sha-24f7826` / `24f7826cc9b93a64b1a0d9d80a9de825e32f52cd`。当日仓库 `main` 为 `3949dc8bd65aee4c6e2f0e92414b5dc830ea23f5`，仅比生产revision多一个文档提交；生产仍以镜像OCI revision和digest为准，不能以滚动 `main` 或 `latest` 推断。
- 工作台仓库当前 `VERSION=v0.5.0`、`web/package.json=0.1.0`、README徽章为 `v0.2.0`，且没有Git tag。这些页面/文件版本文字不一致，不能作为生产版本证据。
- 工作台默认API Base为 `https://image.52token.org`，默认只启用Image2和Gemini图片模型。文本、视频和音频模型默认空；相关能力需要明确配置其他API Base。
- 仓库内 `/batch-image` 是Sub2API自带批量生图页，与15731画布工作台不是同一产品。
- 积分服务已使用 `127.0.0.1:8090`，复用现有 `sub2api` 数据库的独立 `points` schema 和最小权限角色，由 Nginx 精确反代；没有新建 PostgreSQL 容器。Sub2API 菜单必须先生成一次性签名启动票据。积分域名根路径不能作为公开首页，未持有积分会话时 `/app/` 和 `/admin/` 均拒绝访问。
- 系统设置内的“积分系统”标签和管理员路由 `/admin/settings/points` 不受 `points_system.enabled` 或预览白名单影响，必须始终对已认证管理员可见，用于查看桥接状态并经 step-up 启动积分策略台。管理员状态接口只能返回 enabled/configured/active、URL、Key ID、TTL 等非敏感元数据，严禁回传 launch/credit 密钥原值。
- 用户入口仍在 Sub2API `/points` 的右侧内容区嵌入受票据保护的积分 iframe，左侧导航、Header、主题状态和上传 Logo 始终由 Sub2API 提供。下一候选增加来源严格校验的实时主题同步，子页跟随 Sub2API 明暗切换但不能反向改写官方导航。管理员在 `/admin/settings/points` 点击“打开积分配置”后，以新浏览器标签页打开受一次性管理员票据保护的独立策略台；原 Sub2API 设置页保持原位，页面底部不得再追加管理员 iframe。新标签页必须隔离 opener/referrer，并对浏览器拦截新窗口给出明确失败状态。本轮新增行为在新候选实际手工切换前不得写成已上线。
- Nginx 仅对含一次性 ticket 的 `/launch` 关闭 access log，防止查询串落盘；`/app/`、`/admin/`、静态资源、API 以及所有拒绝、越权和限流请求都必须保留访问日志。`api.52token.org` 对 `/api/internal/points/credits` 的公网 `POST/OPTIONS` 精确返回 `404`，积分容器只通过 Docker 网络直连该接口。

## 4. 四种图片模式

| 模式 | 路由/触发方式 | 状态与结果存储 | 升级前检查 |
| --- | --- | --- | --- |
| 同步图片 | `/v1/images/generations`、`/v1/images/edits` | 当前请求；可选JSON空白保活 | 检查长请求、Cloudflare超时和真实终态 |
| 私有provider异步 | 请求体 `async:true` | PostgreSQL `media_generation_tasks`、`media_balance_holds` | 检查在途任务、冻结余额、固定账号和终态lease |
| 官方后缀异步 | `/v1/images/*/async`、`/v1/images/tasks/{id}` | Redis `image_task:*`，结果转存S3/R2 | 容器重启不能恢复处理中goroutine；升级前必须清空或等待在途任务 |
| 内置批量图片 | `/v1/images/batches*`、前端 `/batch-image` | PostgreSQL `batch_image_*`；Redis队列、锁和重试；Gemini Files或Vertex GCS | 同时确认 `batch_image.enabled=true` 与 `queue_enabled=true`，检查ready/delayed/active/inflight |

图片本体和用户浏览器IndexedDB内容不在生产数据库盘点范围。四种模式共用部分路由、账号选择和计费代码，但恢复语义不同，官方升级时禁止互相覆盖。`infinite-canvas` 的画布、素材、生成记录和API Key主要保存在浏览器本地；容器没有数据卷不代表工作台升级没有本地数据兼容风险。

## 5. 数据与迁移基线

- 当前 Sub2API `public.schema_migrations` 共记录 250 条迁移；`2026-07-26` 的 243 条仅是升级前历史快照。
- 历史私有提交 `08fbef836` 的 239 个 SQL 文件与当时数据库 checksum 全部一致，无缺失、无不匹配；数据库另保留4个历史迁移记录：`020_widen_accounts_type.sql`、`021_add_accounts_strip_reasoning_effort.sql`、`159_openai_first_response_settings.sql`、`160_media_generation_tasks.sql`。
- 私有迁移 `173_media_generation_tasks.sql` 至 `179_media_balance_hold_dispatch_state.sql` 已进入生产，禁止改名、删除或修改内容。
- 私有迁移 `192_media_balance_hold_reconciliation_index_notx.sql` 已进入生产并提供全站到期冻结扫描索引；继续按 `_notx` 语义执行，禁止修改 checksum。
- 私有迁移 `193_points_balance_credit_ledger.sql` 已进入生产，为积分服务向 Sub2API 幂等发放余额。积分服务自己的 3 条迁移位于同一数据库的独立 `points` schema，共创建 21 张表，并记录在 `points_schema_migrations`；不得写入或混入 Sub2API `public.schema_migrations`。第三条迁移保存一次性历史作业及逐日游标，当前生产作业已成功且不得重跑。
- PostgreSQL实际 `max_connections=100`；当前应用连接池配置为 `max_open=256`、`max_idle=128`，存在连接上限不匹配。
- 升级前必须备份数据库并记录备份校验、恢复命令和保存位置。积分首次部署备份及 SHA256 已记录在 2.2 节；这不替代后续版本升级的独立新备份，也不代表已建立定时备份或完成隔离恢复演练。

## 6. 已识别但未整改的风险

下表来自 `2026-07-26` 历史只读盘点。本节保留当时风险证据，不代表所有状态至今未变；执行整改或升级前必须重新只读核验。

| 风险 | 当前事实 | 影响 |
| --- | --- | --- |
| Redis持久化参数未生效 | Compose的 `sh -c` 命令被换行拆断，实际子进程只有 `redis-server`；AOF关闭，只使用默认RDB | 容器/主机故障时可能丢失最近缓存、队列和异步任务状态 |
| Redis主机参数 | `vm.overcommit_memory=0`，Redis持续提示后台保存风险 | fork或后台保存可能失败 |
| Redis认证 | 未启用认证，只依赖Docker内网；healthcheck产生空密码AUTH警告 | 网络边界变化时风险扩大，日志有持续噪声 |
| 数据库连接池 | 应用256个最大连接，PostgreSQL只允许100 | 高并发时可能耗尽数据库连接 |
| 环境变量未落地 | `.env` 有106项，Compose只引用47项；63项不会进入容器 | 日志、H2C、调度、Dashboard和PG调优等设置可能与文件表象不一致 |
| Docker日志 | `json-file`无轮转；Sub2API stdout约115 MB且仅约4.6小时 | 长期运行会持续占用磁盘 |
| 镜像回滚 | 无RepoDigest，daemon无旧Sub2API镜像 | 无法仅靠当前主机快速、可验证回滚 |
| 资源限制 | 全部容器无CPU、内存、PID限制，也未收缩capability | 单服务异常可能影响整机 |
| 源站防护 | UFW关闭、iptables `INPUT ACCEPT`；源站锁依赖Nginx，80默认server未套Cloudflare判断 | 配置遗漏可能绕过Cloudflare边界 |
| TLS私钥权限 | 2026-07-26 历史盘点发现 5 个证书私钥为 `0644`；其中包含已淘汰域名的历史文件，不代表活动域名状态 | 非特权本地用户可读历史私钥；活动 `52token.org` 系列证书仍须独立核对并收紧权限 |
| SSH | 允许root密码登录 | 凭据泄露影响面大 |
| URL安全 | URL allowlist/SSRF校验关闭，并允许HTTP与私网host | 管理配置错误时可能访问不应访问的地址 |
| 备份 | 未发现应用、PostgreSQL或部署目录定时备份；现有backups主要是Nginx/配置快照 | 数据恢复点不明确 |
| 工作台运行保障 | 无Docker healthcheck、资源限制和日志轮转 | 页面HTTP 200之外缺少持续健康信号 |

整改必须另开变更任务，先备份、演练和验证回滚；不得在只读盘点任务中顺手修改生产。

## 7. 日志与健康解释

- `/health` 只证明HTTP进程响应，不证明PostgreSQL、Redis、队列和媒体计费健康。
- `2026-07-26` 盘点时 Sub2API 近6小时约38,545行日志，其中671条WARN、86条ERROR；主要为上游403/502/503、内容风险拒绝、客户端取消和正确执行的自动重放抑制。这些计数是历史窗口，不可用于判断当前版本错误率。
- 同一历史窗口内 Nginx 最近5,000次请求中4,840次为200、46次为502。启动初期有应用尚未就绪导致的源站拒绝，之后未发现本地服务崩溃。
- 同一历史窗口内工作台近6小时日志无warning/error。
- 当前积分 Nginx 只对 `/launch` 关闭 access log；其他路径必须保留日志。内部 credit 公网拒绝、应用 fail-close 限流和积分容器日志应按同一时间窗关联，但不得记录 HMAC、ticket、cookie 或密钥原值。
- 排障时应按同一时间窗关联Nginx 499/502/504/524、Sub2API request ID、账号/代理脱敏标识和上游终态；不得把原始prompt、认证头或媒体URL写入工单。
- `2026-07-30` 只读检查了当前容器 stdout、最近 7 天持久化轮转日志及 29,610 条 `ops_error_logs`，均未捕获 `Selected model is at capacity. Please try a different model.`，因此生产证据不能确认该错误对应的 HTTP 状态码、上游错误码或固定响应结构。兼容逻辑不得猜测状态码，只按精确消息识别 HTTP JSON、HTTP 200 `response.failed` 和 SSE `error`；先换其他账号，无其他候选时允许在同一账号使用剩余预算，总计最多两次重放，并且不得给账号降权、冷却或 runtime block。结构化重试日志应核对 `failure_reason=openai_model_at_capacity`、`retry_attempt`、`retry_max`、`backoff_ms`、账号和路由，不得记录请求正文。
- `2026-07-31` 对用户 1 的中断窗口做了只读关联：当日 26 条 Nginx `502` 均能关联真实上游 `502/503`；5 条 `499` 表示客户端先关闭连接，其中 4 条同时出现上游 `524`，另 1 条上游首响应等待约 `125.766s` 后由客户端取消。Nginx API 代理超时为 `1800s`，相关请求仅数秒至约 125 秒，Nginx error log 没有对应 proxy timeout，故不能归因 Nginx 主动截断。10:47 附近另有客户端断开后上游仍完成并落账的样本，此类优先检查 Codex Desktop、本地网络或任务取消。
- Responses 流一旦已经发送响应头，后续失败的 HTTP 状态仍会保留 `200`；客户端已断开时也没有通道再收到错误事件。因此排障不能把 `200` 或“没有错误码”直接当成上游成功。应检查是否收到 `response.completed/done/failed/incomplete/cancelled` 正式终态、是否只有 `[DONE]`、usage/output token 是否异常为零，以及 `openai.responses_client_disconnected` 的 `request_id`、`client_request_id`、`user_id`、`api_key_id`、`group_id`、`account_id`、`model`、`upstream_request_id` 和 `stage`。旧版本对“仅 `[DONE]`”会错误成功并静默 EOF；候选版本已改为缺终态时安全换号或向仍连接的客户端补 `response.failed`。

## 8. 只读盘点清单

每次升级前至少记录：

1. 主机时间、uptime、CPU、内存、磁盘和监听端口。
2. Docker/Compose版本、容器镜像tag、image ID、RepoDigest、OCI revision、创建时间、health、restart和OOM。
3. Compose路径、项目名、端口、挂载、网络及环境变量名称；敏感值只记录“已配置/未配置”。
4. `nginx -t`、生效server/upstream、Cloudflare来源判断、证书SAN和到期时间；不复制私钥。
5. PostgreSQL readiness、数据库大小、迁移总数、checksum差异和任务状态聚合；不读取prompt、URL或凭据字段。
6. Redis PING、持久化模式、内存、eviction、keyspace和队列长度；不转储任务内容。
7. 四种图片模式的在途任务、失败、冻结余额、后台核销最近运行结果、worker队列和对象存储可用性；分别统计 active、capture_pending、captured、released，禁止只核对用户余额总数。
8. 15731工作台的 `infinite-canvas` 镜像tag、digest、OCI revision、首页状态、API Base、模型开关和近时段错误；确认旧 `images` 目录未挂载且未被Nginx引用。
9. 当前镜像与上一稳定镜像的tag、digest和数据库兼容边界。

原始盘点输出只能保存到仓库外受控位置；Git文档只保留本文件这类脱敏结论。

## 9. 版本升级与镜像发布

1. 从完整克隆的当前私有 `main` 创建 `codex/upgrade-vX.Y.Z-*`。
2. 获取并审阅目标官方Release tag；禁止直接跟随滚动官方 `main` 或部署官方 `latest`。
3. 解决协议、调度、四种图片模式、媒体计费、Nginx/Worker和支付冲突，并运行 `docs/PRIVATE_CUSTOMIZATION_CN.md` 中的完整门禁。
4. 通过 CI 或受控本机构建环境构建私有镜像。版本取自 `backend/cmd/server/VERSION`，镜像标签必须包含版本和 commit；发布摘要必须记录 manifest digest、image ID 和传输归档 SHA256。
5. Sub2API 新镜像只上传、导入或缓存，不立即替换运行容器；积分镜像可在 `points` schema 备份和旧镜像记录完成后独立更新。
6. 升级窗口前确认没有不可恢复的 `image_task:*` 在途任务，批量队列可恢复，私有媒体任务没有异常冻结余额。
7. 只替换镜像引用，不同时调整账号、价格、Redis、Nginx或数据库参数。
8. 上线后核对OCI revision、VERSION、health、DB/Redis、迁移、关键路由、任务终态和日志。

`2026-07-30` GitHub Actions 因账户计费或支出限额在 runner 分配前终止，job 未执行任何 step；当日改由受控本机生成标准 Docker archive，服务器只执行 `docker load`。Sub2API 随后曾切换为 `v0.1.168-339422728b2c`，积分服务也经历过 `v0.1.168-28e760bc8c6d`，这些只属于历史发布链。registry digest、image ID 与 archive SHA256 必须分别记录，禁止互相冒充；当前两项服务均为 `v0.1.169-04a19ca082ee`，精确状态以本章 2.3 节及链接的 `2026-07-31` 记录为准。

当前通用部署文档包含官方 `weishaw/sub2api:latest` 示例，只适用于官方默认部署。私有生产严禁照搬该镜像引用。

### 9.1 独立生图工作台发布

1. 从完整克隆的 [`hxly520/infinite-canvas`](https://github.com/hxly520/infinite-canvas) `main` 创建变更分支；不从服务器旧 `images` 快照反向生成源码。
2. 在该仓库完成构建和测试，由 `.github/workflows/docker-image.yml` 发布 `ghcr.io/hxly520/infinite-canvas:sha-<commit>`。
3. 记录完整commit、镜像tag和registry digest；生产Compose固定使用SHA tag或digest，禁止使用 `latest`。
4. 只替换15731工作台镜像，不同时变更Sub2API、Nginx、Cloudflare或API Base。
5. 联合回归 `/v1/images/*`、`/v1beta/models/*`、视频任务和媒体代理路径，并验证首页、静态资源、Image2/Gemini请求、参考图、IndexedDB与浏览器本地素材。

### 9.2 独立积分系统发布与安全边界

积分系统已经完成镜像导入、同库隔离、Nginx 接入、中文双工作区、管理员用户明细和一次性历史回算；积分与 Sub2API 当前均为 `v0.1.169-04a19ca082ee`。以下步骤同时是后续重部署和版本合并的强制边界，任何自动化都不得替换或重启 Sub2API。

1. 优先由 GitHub 分别构建 Sub2API 与 `points-system` 的 commit 不可变镜像，记录两者 tag、OCI revision 和 registry digest；生产机不编译。CI runner 受计费门禁时可使用受控本机构建和标准 archive，但仍须把镜像推送 GHCR 并分别记录 registry digest、archive SHA256 和服务器加载后的 image ID。
   第 2-4 步只适用于空白新环境或经审计的灾难恢复。当前生产角色、schema 和密钥均已存在，普通积分镜像更新不得重跑 bootstrap 或重新生成密钥。
2. 先对现有 `sub2api` 数据库执行一致性备份并记录 SHA256/恢复命令；由 PostgreSQL bootstrap superuser 使用全新的角色名，在同一数据库创建独立 `points` schema 和 `points_app` 最小权限角色，写池默认最多 8 条连接。角色或 schema 已存在时脚本必须停止并转人工 ACL 审计，不得覆盖。不得创建第二个 PostgreSQL 容器。
3. 由同一 bootstrap superuser 为 `POINTS_USAGE_DATABASE_URL` 创建全新的只读账号，只授予 Sub2API `usage_logs` 所需列的 `SELECT`，并强制只读事务和最多 4 条连接；不得复用 Sub2API 写账号或自动修改共享 PUBLIC ACL。
4. 生成独立 Base64 32 字节以上的 session、launch、credit 和内部集成密钥。生产 `points.env`、bridge env 和 psql 变量文件必须为 `root:root 0600`；Sub2API 与积分服务只共享对应公约中的同一解码后字节。状态或配置 API 只能返回是否已配置、Key ID 等非敏感元数据，不得回传密钥原值；文档、Compose、shell history 和日志也不能出现真实值。
5. 运行积分迁移和只读消费查询自检后启动积分容器，再启用 Nginx 精确反代。可信代理 CIDR 必须与实际容器/loopback 网络一致；根路径返回 404。只有 `/launch` 关闭 access log，其余路径必须保留访问证据；此阶段不得修改、替换或重启现有 Sub2API 容器。
6. 预览期保持 Sub2API `points_system.enabled=false`、`points_system.preview_user_ids: [1]`，同时配置积分服务 `POINTS_USER_ACCESS_MODE=preview`、`POINTS_USER_PREVIEW_IDS=1`。管理员设置导航 `/admin/settings/points` 必须可见并允许检查桥接状态；用户 ID 1 可看到用户菜单、访问 `/points` 并申请 user ticket，其他用户在菜单、前端路由、Sub2API launch、积分 ticket 交换和每次既有 user session 请求中均被拒绝。白名单只保存在两端服务端配置中，对浏览器只返回当前用户专属的 `points_system_access` 布尔值，不得返回完整 ID 列表。全量验收完成后再由维护者在同一维护变更中把 Sub2API `enabled` 明确改为 `true`、积分服务 mode 改为 `all` 并清空积分预览名单，不能把扩展预览白名单误当成全站开关。
   候选切换前必须在 root-only `points.env` 配置 `POINTS_EMBED_PARENT_ORIGIN=https://实际Sub2API域名`（精确 Origin、无路径和尾斜杠），并确认用户 iframe 所需的 Sub2API CSP `frame-src` 包含积分 Origin、积分响应 `frame-ancestors` 只包含该父 Origin、没有冲突的 `X-Frame-Options`。管理员点击“打开积分配置”必须通过 step-up 生成一次性管理员票据并在带 `noopener,noreferrer` 的新标签页打开；原设置页保持不变，禁止重新在底部嵌入管理员 iframe。
7. 验证用户/管理员角色隔离、票据一次性、CSRF、昨日消费快照、两位小数比例、最低昨日消费门槛、并发签到、三层金额上限和余额交易幂等后再开放用户 bridge。policy enabled 不等于签到 enabled，任何配置或镜像切换都不得隐式开启签到。
8. 当前 policy v3 固定为比例 `10.00:1`、刷新 `00:05`、签到关闭。以后启用签到必须追加最早次日生效的新策略，并由管理员明确保存签到模式、最低消费、阶梯和所有金额安全上限。
9. 余额发放超时后只重试同一交易 UUID；未知 credit 结果禁止直接冲正，必须确认 settled 后再发起关联 debit。确定性 4xx 进入永久失败终态，由管理员检查审计后显式重试；禁止删除幂等账本后重新发放。
10. Sub2API 余额缓存必须使用 Redis 用户代次保护数据库回源：失效和扣减原子推进代次，旧读取只能在代次未变化时回填。credit 已在数据库提交但余额缓存同步失败时必须返回可重试 `503`，积分发件箱继续使用原 UUID，禁止把该交易提前标为 settled。
11. `/api/internal/points/credits` 必须在应用层使用 Redis fail-close 限流，默认每分钟 120 次；限流存储异常时拒绝请求。公网 Nginx 对该精确路径的 `POST/OPTIONS` 返回 `404`，不能依赖 HMAC 作为唯一网络边界。
12. 积分系统不提供手工余额赠送或手工快照刷新 API/页面。管理员用户明细是只读聚合；发放任务只查询和操作 `checkin` 类型记录。历史基线只能在空白新 schema 首次激活时由专用运维命令执行，当前生产成功作业不得重跑。
13. 回滚入口时先关闭 Sub2API points 开关，不删除 `points` schema、快照、签到、事务发件箱或 Sub2API `points_balance_credits`。积分容器可独立停止，不能为此停止或回滚 Sub2API。
14. 同一发布记录必须包含 Sub2API/积分/工作台提交与镜像 tag/digest、数据库备份、Sub2API 与积分迁移表、Nginx 配置版本和 Cloudflare Worker 版本。

首次发布的命令记录必须能按以下模板复现，实际执行时把占位符替换为只读盘点结果。数据库密码通过权限为 `0600` 的 psql 变量文件拼接到标准输入，不能出现在命令参数、Git 或 shell history；积分运行环境文件同样必须为 `0600`：

```bash
# 备份、摘要和归档目录可读性检查；此过程不停止 PostgreSQL 或 Sub2API。
umask 077
docker exec POSTGRES_CONTAINER pg_dump -U POSTGRES_SUPERUSER -d sub2api -Fc --no-owner > BACKUP.dump
sha256sum BACKUP.dump > BACKUP.dump.sha256
pg_restore --list BACKUP.dump > BACKUP.catalog.txt

# 两个变量文件由密钥生成工具写入，内容为 psql \set 指令，权限必须为 0600。
# 模板要求全新角色并在单一事务中执行；任何 ACL 前置检查失败都会整体回滚。
cat POINTS_APP_PSQL_VARS points-system/deploy/shared-database-bootstrap.sql.example | \
  docker exec -i POSTGRES_CONTAINER psql -X -v ON_ERROR_STOP=1 -U POSTGRES_SUPERUSER -d sub2api
cat POINTS_READER_PSQL_VARS points-system/deploy/usage-reader.sql.example | \
  docker exec -i POSTGRES_CONTAINER psql -X -v ON_ERROR_STOP=1 -U POSTGRES_SUPERUSER -d sub2api

# --env-file 用于 Compose 插值；显式导出同一路径后，service.env_file 才读取该运行时文件。
export POINTS_ENV_FILE=/absolute/path/points.env
docker compose --env-file "$POINTS_ENV_FILE" -f points-system/compose.example.yml config --quiet
docker compose --env-file "$POINTS_ENV_FILE" -f points-system/compose.example.yml pull
docker compose --env-file "$POINTS_ENV_FILE" -f points-system/compose.example.yml up -d --no-deps points-system

# 等待数据库迁移和服务健康完成，并断言域名根路径仍被拒绝。
healthy=false
for attempt in $(seq 1 30); do
  if curl --fail --silent http://127.0.0.1:POINTS_HOST_PORT/healthz >/dev/null; then healthy=true; break; fi
  sleep 2
done
test "$healthy" = true
test "$(curl --silent --output /dev/null --write-out '%{http_code}' http://127.0.0.1:POINTS_HOST_PORT/)" = 404
```

新环境首启固定 `POINTS_USAGE_RECONCILE_DAYS=1`。自动调度在策略未启用时不访问 `usage_logs`，只幂等写入零值成功就绪标记；仍须在启动前只读记录昨日行数、表/索引大小、活动连接、长事务、磁盘余量，并在低峰审阅聚合查询 `EXPLAIN`。只有确认资源余量后才可把回算窗口恢复为默认 7 天。当前生产已完成历史基线，普通镜像更新只运行嵌入迁移和日常调度，禁止重跑历史命令。`pg_restore --list` 只证明归档目录可读，不等于可恢复性验证；完整 `pg_restore --clean --if-exists --create` 必须在服务器外或隔离演练实例执行，禁止直接在生产库试恢复。

### 9.3 exact-root 公开首页发布

未登录 Vue `/home` 随 Sub2API 候选镜像发布，但域名根路径 `/` 与 `/index.html` 由宿主 `/home/api/sub2api-deploy/public/index.html` 提供，二者必须分别处理：

1. 先运行前端静态页面契约测试，确认页面无脚本、无表单、无外域资源，且不出现具体国外模型或商业中转宣传名称。
2. 记录仓库 `deploy/public-landing/index.html` 的 SHA256，在宿主同目录备份当前文件。
3. 以同目录临时文件写入并执行原子 rename，保留原属主和权限；不 reload Nginx，不重启 Sub2API。
4. 分别核对仓库文件、宿主文件、`https://52token.org/` 与 `/index.html` 响应体 SHA256。任一不一致立即恢复备份。
5. 新页面通过同源 `GET /api/v1/settings/logo` 使用后台上传 Logo；该接口只在后续 Sub2API 候选手工切换后可用，切换前静态页必须能回退 `/logo.svg`。

## 10. 回滚边界

- 回滚必须使用事先记录的不可变镜像digest，不能只依赖可变tag。
- 回滚应用前确认新迁移是否向后兼容；forward-only迁移不能靠切回旧镜像自动撤销。
- 数据库恢复属于单独操作，必须先验证备份可恢复性。
- Redis状态不作为唯一业务事实源，但后缀异步图片和队列状态会受Redis持久化影响。
- Cloudflare Worker、Nginx和15731 `infinite-canvas` 工作台各有独立版本与回滚物，Sub2API镜像回滚不会同步回滚它们；旧服务器 `images` 快照不是工作台回滚物。

## 11. 仓库外依赖

以下状态无法仅从服务器源码仓库确认，交接时必须另行提供只读访问或证据：

- Cloudflare Worker代码版本、Routes、KV/R2绑定、WAF和Cache Rules。
- Docker registry中的不可变digest和历史镜像保留策略。
- `hxly520/infinite-canvas` 的分支保护、CI结果和镜像digest；Sub2API仓库只记录跨仓库契约，不复制工作台源码。
- 服务器外快照、数据库备份及恢复演练记录。
- 活动 Nginx、Cloudflare、积分父站 Origin 和前端配置是否始终只引用 `52token.org` 系列；历史备份中的淘汰域名记录不得恢复到活动链路。
- 63个未被Compose引用的环境变量哪些仍是期望配置。

## 12. 凭据规则

- SSH密码、私钥、API Key、数据库密码、支付密钥、Worker Secret和证书私钥永不进入Git。
- 积分 session、launch、credit、bridge 和数据库角色密钥只保存在仓库外 `root:root 0600` 文件中；所有配置/状态 API 只返回是否配置和 Key ID 等元数据，禁止回传原值。
- 文档可记录主机、端口、镜像、commit、非敏感路径和“密钥已配置”状态。
- 任何包含原始环境变量、日志、数据库行或 `nginx -T` 全量输出的文件都不得提交。
- 凭据发生轮换时，只更新外部密码管理系统；本文档不记录旧值或新值。
