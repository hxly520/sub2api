# 生产运维、只读盘点与版本交接

本文档记录私有 Sub2API 的生产拓扑、只读盘点基线、镜像发布边界和版本交接流程。它只保存脱敏事实，不保存密码、API Key、数据库连接、证书私钥、Worker Secret、完整环境变量或原始日志。

## 1. 权威来源

- 私有仓库：`hxly520/sub2api`。
- 当前私有主线：`main`；生产应用代码基线为 `08fbef836b9c89c043d4269623b0e73e4aa674b6`，文档和发布流程在该提交之后继续维护。
- 官方基线：Sub2API Release `v0.1.164`，commit `cd8bb98c44303b2c8f04c0da340447c992f0cb7d`。
- 当前应用版本：`backend/cmd/server/VERSION=0.1.164`。
- 生图工作台唯一源码：[`hxly520/infinite-canvas`](https://github.com/hxly520/infinite-canvas) 的 `main`；它独立于Sub2API版本发布。
- 生产主机：`107.172.147.76`。SSH用户名和认证信息由仓库外的密码管理系统保存。
- 生产部署根：`/home/api/sub2api-deploy`。
- 生产变更原则：服务器只拉取或导入已在CI构建并验证的私有镜像；不在生产机编译源码或镜像。

正式维护不得使用 shallow、partial clone 或 sparse checkout。升级前必须确认完整历史、完整对象和官方 tag均已获取。

## 2. 2026-07-26 只读生产快照

盘点时间：`2026-07-26 20:41 CST`。盘点期间未安装软件、写文件、重启、reload或修改配置。

| 项目 | 已确认状态 |
| --- | --- |
| 操作系统 | Ubuntu 24.04 LTS，Linux `6.8.0-136-generic` |
| 主机资源 | 2 vCPU，1.9 GiB内存，1 GiB Swap；根盘34 GiB，剩余约26 GiB |
| Docker | Docker `26.1.4`，Compose `v2.27.1` |
| Sub2API | `gptcodex/sub2api:0.1.164-08fbef836b9c`，OCI revision完整匹配 `08fbef836`，healthy，restart/OOM均为0 |
| PostgreSQL | `postgres:17.8-alpine`，healthy，数据库约935 MB |
| Redis | `redis:8-alpine`，服务版本8.8.0，healthy，约3,079个键、266个连接、2.92 MB数据 |
| 画布工作台 | `ghcr.io/hxly520/infinite-canvas:sha-24f7826`，revision `24f7826cc9b93a64b1a0d9d80a9de825e32f52cd`，首页HTTP 200 |
| Nginx | `1.20.1`，手工维护二进制，`nginx -T`验证成功，systemd restart为0 |
| 本地端口 | Sub2API `127.0.0.1:8080`；画布工作台 `127.0.0.1:15731` |
| 对外端口 | `22/80`监听IPv4和IPv6；`443`仅IPv4 |
| 镜像回滚 | 当前daemon没有上一版Sub2API镜像；所有镜像均无 `RepoDigests`，尚无可验证的本机不可变回滚点 |

现网镜像版本、OCI revision和仓库私有v164应用代码完全一致，因此更新后的 `main` 必须保留 `08fbef836` 的完整应用树和二开行为，并在其后维护文档与发布流程。实际生产状态不能只凭前端版本文字判断。

## 3. 域名和进程边界

活跃域名包括：

- `52token.org`
- `api.52token.org`
- `image.52token.org`
- `video.52token.org`
- `gpt-codex.top`
- `api.gpt-codex.top`
- `image.gpt-codex.top`

主要链路：

```text
Cloudflare -> Nginx :443 -> 127.0.0.1:8080  Sub2API
                         -> 127.0.0.1:15731  独立画布工作台
Cloudflare Worker -> 加密媒体URL -> 上游媒体源
```

- `api.52token.org` 是Sub2API完整API入口。
- `image.52token.org` 只把允许的图片、Gemini、视频和任务路径转到Sub2API，其余页面转到独立画布工作台。
- `video.52token.org` 源站对媒体路径固定返回404，必须由Cloudflare Worker截获。
- `15731` 工作台源码不属于本仓库，唯一源码来源是 [`hxly520/infinite-canvas`](https://github.com/hxly520/infinite-canvas)。生产实际运行的是该仓库构建镜像内的 `/usr/share/nginx/html`。
- `basketikun/infinite-canvas` 仅是工作台的上游参考与合并来源，不是52Token生产构建源；Sub2API仓库也不得复制一份工作台源码形成第三个来源。
- 宿主 `/home/api/sub2api-deploy/images` 只是未挂载、无 `.git` 的旧 `v0.5.0` 页面快照，已退出构建、发布、运行和回滚链路；Nginx不得以该目录作为页面fallback。
- 2026-07-26盘点时工作台镜像固定在 `sha-24f7826` / `24f7826cc9b93a64b1a0d9d80a9de825e32f52cd`。当日仓库 `main` 为 `3949dc8bd65aee4c6e2f0e92414b5dc830ea23f5`，仅比生产revision多一个文档提交；生产仍以镜像OCI revision和digest为准，不能以滚动 `main` 或 `latest` 推断。
- 工作台仓库当前 `VERSION=v0.5.0`、`web/package.json=0.1.0`、README徽章为 `v0.2.0`，且没有Git tag。这些页面/文件版本文字不一致，不能作为生产版本证据。
- 工作台默认API Base为 `https://image.52token.org`，默认只启用Image2和Gemini图片模型。文本、视频和音频模型默认空；相关能力需要明确配置其他API Base。
- 仓库内 `/batch-image` 是Sub2API自带批量生图页，与15731画布工作台不是同一产品。

## 4. 四种图片模式

| 模式 | 路由/触发方式 | 状态与结果存储 | 升级前检查 |
| --- | --- | --- | --- |
| 同步图片 | `/v1/images/generations`、`/edits` | 当前请求；可选JSON空白保活 | 检查长请求、Cloudflare超时和真实终态 |
| 私有provider异步 | 请求体 `async:true` | PostgreSQL `media_generation_tasks`、`media_balance_holds` | 检查在途任务、冻结余额、固定账号和终态lease |
| 官方后缀异步 | `/v1/images/*/async`、`/v1/images/tasks/{id}` | Redis `image_task:*`，结果转存S3/R2 | 容器重启不能恢复处理中goroutine；升级前必须清空或等待在途任务 |
| 内置批量图片 | `/v1/images/batches*`、前端 `/batch-image` | PostgreSQL `batch_image_*`；Redis队列、锁和重试；Gemini Files或Vertex GCS | 同时确认 `batch_image.enabled=true` 与 `queue_enabled=true`，检查ready/delayed/active/inflight |

图片本体和用户浏览器IndexedDB内容不在生产数据库盘点范围。四种模式共用部分路由、账号选择和计费代码，但恢复语义不同，官方升级时禁止互相覆盖。`infinite-canvas` 的画布、素材、生成记录和API Key主要保存在浏览器本地；容器没有数据卷不代表工作台升级没有本地数据兼容风险。

## 5. 数据与迁移基线

- 现网共记录243条迁移。
- 私有提交 `08fbef836` 的239个SQL文件与数据库checksum全部一致，无缺失、无不匹配。
- 数据库另保留4个历史迁移记录：`020_widen_accounts_type.sql`、`021_add_accounts_strip_reasoning_effort.sql`、`159_openai_first_response_settings.sql`、`160_media_generation_tasks.sql`。
- 私有迁移 `173_media_generation_tasks.sql` 至 `179_media_balance_hold_dispatch_state.sql` 已进入生产，禁止改名、删除或修改内容。
- PostgreSQL实际 `max_connections=100`；当前应用连接池配置为 `max_open=256`、`max_idle=128`，存在连接上限不匹配。
- 升级前必须备份数据库并记录备份校验、恢复命令和保存位置。当前服务器未发现数据库定时备份任务，仓库外快照状态待确认。

## 6. 已识别但未整改的风险

下表来自只读盘点。本次只记录，不代表已经修复。

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
| TLS私钥权限 | 5个私钥为 `0644`，包含正在使用的 `gpt-codex.top` 与 `image.gpt-codex.top` | 非特权本地用户可读私钥 |
| SSH | 允许root密码登录 | 凭据泄露影响面大 |
| URL安全 | URL allowlist/SSRF校验关闭，并允许HTTP与私网host | 管理配置错误时可能访问不应访问的地址 |
| 备份 | 未发现应用、PostgreSQL或部署目录定时备份；现有backups主要是Nginx/配置快照 | 数据恢复点不明确 |
| 工作台运行保障 | 无Docker healthcheck、资源限制和日志轮转 | 页面HTTP 200之外缺少持续健康信号 |

整改必须另开变更任务，先备份、演练和验证回滚；不得在只读盘点任务中顺手修改生产。

## 7. 日志与健康解释

- `/health` 只证明HTTP进程响应，不证明PostgreSQL、Redis、队列和媒体计费健康。
- 盘点时Sub2API近6小时约38,545行日志，其中671条WARN、86条ERROR；主要为上游403/502/503、内容风险拒绝、客户端取消和正确执行的自动重放抑制。
- Nginx最近5,000次请求中4,840次为200、46次为502。启动初期有应用尚未就绪导致的源站拒绝，之后未发现本地服务崩溃。
- 工作台近6小时日志无warning/error。
- 排障时应按同一时间窗关联Nginx 499/502/504/524、Sub2API request ID、账号/代理脱敏标识和上游终态；不得把原始prompt、认证头或媒体URL写入工单。

## 8. 只读盘点清单

每次升级前至少记录：

1. 主机时间、uptime、CPU、内存、磁盘和监听端口。
2. Docker/Compose版本、容器镜像tag、image ID、RepoDigest、OCI revision、创建时间、health、restart和OOM。
3. Compose路径、项目名、端口、挂载、网络及环境变量名称；敏感值只记录“已配置/未配置”。
4. `nginx -t`、生效server/upstream、Cloudflare来源判断、证书SAN和到期时间；不复制私钥。
5. PostgreSQL readiness、数据库大小、迁移总数、checksum差异和任务状态聚合；不读取prompt、URL或凭据字段。
6. Redis PING、持久化模式、内存、eviction、keyspace和队列长度；不转储任务内容。
7. 四种图片模式的在途任务、失败、冻结余额、worker队列和对象存储可用性。
8. 15731工作台的 `infinite-canvas` 镜像tag、digest、OCI revision、首页状态、API Base、模型开关和近时段错误；确认旧 `images` 目录未挂载且未被Nginx引用。
9. 当前镜像与上一稳定镜像的tag、digest和数据库兼容边界。

原始盘点输出只能保存到仓库外受控位置；Git文档只保留本文件这类脱敏结论。

## 9. 版本升级与镜像发布

1. 从完整克隆的当前私有 `main` 创建 `codex/upgrade-vX.Y.Z-*`。
2. 获取并审阅目标官方Release tag；禁止直接跟随滚动官方 `main` 或部署官方 `latest`。
3. 解决协议、调度、四种图片模式、媒体计费、Nginx/Worker和支付冲突，并运行 `docs/PRIVATE_CUSTOMIZATION_CN.md` 中的完整门禁。
4. 通过CI构建私有镜像。版本取自 `backend/cmd/server/VERSION`，镜像标签必须包含版本和commit；发布摘要必须记录registry digest。
5. 上传/拉取新镜像但不立即替换运行容器；先记录旧镜像和数据库备份点。
6. 升级窗口前确认没有不可恢复的 `image_task:*` 在途任务，批量队列可恢复，私有媒体任务没有异常冻结余额。
7. 只替换镜像引用，不同时调整账号、价格、Redis、Nginx或数据库参数。
8. 上线后核对OCI revision、VERSION、health、DB/Redis、迁移、关键路由、任务终态和日志。

当前通用部署文档包含官方 `weishaw/sub2api:latest` 示例，只适用于官方默认部署。私有生产严禁照搬该镜像引用。

### 9.1 独立生图工作台发布

1. 从完整克隆的 [`hxly520/infinite-canvas`](https://github.com/hxly520/infinite-canvas) `main` 创建变更分支；不从服务器旧 `images` 快照反向生成源码。
2. 在该仓库完成构建和测试，由 `.github/workflows/docker-image.yml` 发布 `ghcr.io/hxly520/infinite-canvas:sha-<commit>`。
3. 记录完整commit、镜像tag和registry digest；生产Compose固定使用SHA tag或digest，禁止使用 `latest`。
4. 只替换15731工作台镜像，不同时变更Sub2API、Nginx、Cloudflare或API Base。
5. 联合回归 `/v1/images/*`、`/v1beta/models/*`、视频任务和媒体代理路径，并验证首页、静态资源、Image2/Gemini请求、参考图、IndexedDB与浏览器本地素材。
6. 同一发布记录必须包含Sub2API提交、工作台提交、两者镜像tag/digest、Nginx配置版本和Cloudflare Worker版本，再保留上一工作台digest作为独立回滚点。

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
- 旧 `gpt-codex.top` 域名是否继续承担业务。
- 63个未被Compose引用的环境变量哪些仍是期望配置。

## 12. 凭据规则

- SSH密码、私钥、API Key、数据库密码、支付密钥、Worker Secret和证书私钥永不进入Git。
- 文档可记录主机、端口、镜像、commit、非敏感路径和“密钥已配置”状态。
- 任何包含原始环境变量、日志、数据库行或 `nginx -T` 全量输出的文件都不得提交。
- 凭据发生轮换时，只更新外部密码管理系统；本文档不记录旧值或新值。
