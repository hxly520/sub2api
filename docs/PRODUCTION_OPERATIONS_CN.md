# 生产运维、只读盘点与版本交接

本文档记录私有 Sub2API 的生产拓扑、只读盘点基线、镜像发布边界和版本交接流程。它只保存脱敏事实，不保存密码、API Key、数据库连接、证书私钥、Worker Secret、完整环境变量或原始日志。

最新生产事实以本文件第 0 节及 [2026-07-31 积分激活与后续候选交接](PRODUCTION_DEPLOYMENT_20260731_CN.md) 第 0、11.14、11.15、11.16 节为准。[2026-07-30 v0.1.168 候选与积分系统生产记录](PRODUCTION_DEPLOYMENT_20260730_CN.md) 只保留首次部署证据；后续排障不得把其中的旧积分镜像、disabled policy 或迁移数量当成现状。

## 0. 2026-08-02 当前生产事实

- Sub2API 运行 `ghcr.io/hxly520/sub2api:0.1.169-1a4a690dd999`，revision `1a4a690dd999b669e2ce09522854ea157d7af984`，容器 `69a710a1ad0c...`，healthy、restart count `0`。points credit/reversal 的非空审计正文已真实验收，兼容触发器与函数已经删除。
- 积分服务运行 `ghcr.io/hxly520/sub2api-points:0.1.169-b64a0110ab2c`，revision `b64a0110ab2cb0fcf247b94be8f743ac770e8475`，容器 `85b668577d27...`，healthy、restart count `0`。registry digest、image ID、archive SHA256 分别为 `sha256:37949edae511fdd80533d4028dab137e44df4acd0a5797549cf432c25eaaafd2`、`sha256:f0d76d2b57d44eb4b4967e84b5bd55ff92290c2b364aa0a051f83ea0a1de8deb`、`592bfe5bbeff6127332c081b776613c9cf9670af3043b208d13d11c787293e26`。该镜像继承 `fc7ea1fe59c0` 的冲正净额修复并增加双精确父 Origin。
- 积分中心与签到门禁均为全体模式：`POINTS_USER_ACCESS_MODE=all`、`POINTS_USER_PREVIEW_IDS=`、`POINTS_CHECKIN_ACCESS_MODE=all`、`POINTS_CHECKIN_PREVIEW_IDS=`。当前 policy v7 于 `2026-08-02` 生效，按昨日原始成功余额消费使用 `1%-5% / 2%-5% / 3%-5% / 4%-5%` 四档，最低消费 `1 U`、每日一次、三个金额 cap 为 `NULL`；原 v5 保持不可变并于 `2026-08-03` 接管。
- 用户 1 原 `3.08 U` 已通过 reversal `12c061b4-380c-5119-8aec-26a500ef6590` 真正扣回并标记 `reversed`；新 grant `7d779e12-d5dd-4f09-a944-ff0eac93cf18` 按 `[50,100) U` 的 `3%-5%` 档实发 `3.11 U` 且为 `settled`。真实用户接口的今日赠送和累计赠送均为净 `3.11 U`，旧记录仅作为已冲正审计保留。
- `fc7ea1fe59c0` 把展示净额与资金预留分开：展示只统计已到账且未冲正 grant，限次和安全 cap 仍统计已接受签到的预留金额。达标非用户 1 已验收为可签到，未达 `1 U` 用户不可签到；`401`、CSRF `403`、每日次数和幂等边界继续生效。
- 最新恢复点为 `/home/api/backups/points-dual-origin-20260802-123509`；dump `104,197,299` 字节、SHA256 `6b0263241fc0d2f7596b0c98a7d1f7966de40d7c928523f5589dd313099e1e18`、catalog `1,294` 行。目录同时保存切换前 Compose/env、四容器 inspect、资金基线和最终验收；回滚时恢复其中 Compose/env 并只 force-recreate `points-system`。
- 最终 nonterminal/failed grant 为 0；Sub2API、PostgreSQL、Redis 的容器 ID和启动时间未因积分发布变化，Nginx 为 active。
- 当前 `https://api.52token.org/points` 与 `https://52token.org/points` 都是有效父页面。生产以 `POINTS_EMBED_PARENT_ORIGIN=https://api.52token.org` 保留 Logo 主来源，并以 `POINTS_EMBED_PARENT_ORIGINS=https://52token.org` 增加第二精确父 Origin；CSP 和 ready/theme 消息只使用合并后的有限列表，不允许通配符。只配置其中一个会使另一个父站超时显示“积分中心暂时未就绪”。
- 用户 1 已在 `390x844` 移动视口从根域完成真实 iframe 验收：无未就绪遮罩、资源和用户 API 全部成功、`clientWidth=scrollWidth=367` 且无控制台 WARN/ERROR。两个父站 `/points` 均为 `200`，积分 CSP 同时精确允许两个 Origin；后续任一父域、Nginx 或前端入口变更都必须重复桌面和移动验收。

### 0.1 2026-08-02 服务器空间只读盘点

`2026-08-02 13:32 CST` 对生产主机执行只读空间、Compose 引用、运行容器、镜像、日志和备份盘点；未删除文件、未执行 Docker prune、未修改数据库或配置，也未重启任何服务。

- 根文件系统为 34 GiB，已用约 11 GiB（33%），可用约 22 GiB，当前不存在容量告警。
- `/home/api` 共约 4.25 GiB：`sub2api-deploy` 约 2.82 GiB、`sub2api-points` 约 627 MiB、顶层 `backups` 约 588 MiB、顶层 `releases` 约 244 MiB。
- `sub2api-deploy` 的主要有效数据为 PostgreSQL 约 1.50 GiB、Sub2API `data` 约 98 MiB 和 Redis 数据约 432 KiB；这三处均被运行容器 bind mount，禁止直接删除。当前 Sub2API 主日志约 75 MiB，历史压缩应用日志约 23 MiB。
- Docker 数据根约 1.44 GiB。共有 17 个镜像、5 个运行镜像、无 dangling 镜像、无本地 volume、build cache 为 0；Docker 报告 923.7 MB 未使用镜像空间。当前运行引用仍为 Sub2API `1a4a690dd999`、积分 `b64a0110ab2c`、`infinite-canvas` `sha-24f7826`、PostgreSQL 17.8 和 Redis 8。
- Docker `json-file` 日志合计约 18.8 MiB；只有积分容器设置 `10m x 3` 轮转，Sub2API、PostgreSQL、Redis 和工作台仍未设置 Docker 日志上限。宿主 journal 约 409.6 MiB，Nginx 日志约 13.3 MiB且已有轮转。

清理必须按以下边界执行，不能把“占空间”等同于“无用”：

| 分类 | 内容 | 预计释放 | 条件 |
| --- | --- | ---: | --- |
| 可直接清理 | 无配置/容器引用的旧 `/home/api/releases` 两个 `e8d73f3e6655` 归档、`sub2api-cachecompat` 的 v0.1.139/v0.1.142 遗留、积分 `incoming/28e760bc8c6d`、`image-reference-tests`、空 `ui-reference-20260731` 和 `.incomplete` 目录 | 约 403 MiB | 删除前再次核对路径和当前引用；这些文件不属于当前或指定回滚版本 |
| 保留当前及一个回滚版本后可清理 | 旧 Sub2API/积分镜像 tar 约 977 MiB；早期数据库备份与旧运行基底约 778 MiB | 约 1.72 GiB | 至少保留下述四个镜像归档和两个完整恢复点；旧数据库备份应先转存到服务器外并校验 SHA256/目录清单 |
| Docker 显式清理 | 除当前与下述回滚镜像外的已停止版本镜像 | 最多约 923.7 MB | 按精确 tag/image ID 删除；禁止使用 `docker image prune -a` 或 `docker system prune -a`，它们会同时删除未运行的回滚镜像 |
| 条件清理 | `/home/api/sub2api-deploy/images` 旧工作台源码约 3 MiB | 约 3 MiB | 活动 Nginx 配置仍保留该 `root` 指令；先备份并移除残留指令、`nginx -t` 和平滑 reload 后再删。真实工作台仍由 15731 `infinite-canvas` 容器提供 |
| 可选系统清理 | APT 包归档约 250 MiB；journal 超出最终保留上限的历史段；约 23 MiB 压缩应用旧日志 | 取决于保留策略 | 不属于 Sub2API 发布物；先确定审计和故障排查保留周期，不得截断活动日志 |

执行任何发布物或备份清理时必须保留：

1. 当前 Sub2API 归档 `/home/api/sub2api-deploy/releases/sub2api-0.1.169-1a4a690dd999-linux-amd64.tar`，回滚归档 `/home/api/sub2api-deploy/image-archives/sub2api-0.1.169-f79803bb73d6-linux-amd64.tar`。
2. 当前积分归档 `/home/api/sub2api-points/releases/sub2api-points-0.1.169-b64a0110ab2c-linux-amd64.tar`，回滚归档 `/home/api/sub2api-points/releases/sub2api-points-0.1.169-fc7ea1fe59c0-linux-amd64.tar`。
3. 最新完整恢复点 `/home/api/backups/points-dual-origin-20260802-123509` 和前一恢复点 `/home/api/backups/points-all-checkin-20260802-103531`。在服务器外备份尚未验证前，不应删除其他数据库 dump。
4. 运行中的 `postgres_data`、`redis_data`、Sub2API `data`、当前 Compose/env、Nginx 配置和证书。任何情况下都不得手工删除 `/var/lib/docker/overlay2` 或容器日志路径。

上述文件层清理上限约 2.11 GiB；再显式清理废弃 Docker 镜像后总计约 3 GiB。由于当前磁盘仍有约 22 GiB 可用，本次只记录清单，实际删除应作为单独变更执行并在前后复核 `df`、运行容器 ID、健康状态和回滚文件 SHA256。

### 0.2 2026-08-02 残留备份清理执行

维护者随后明确授权只删除上述残留备份和发布归档，不允许清理 Docker 镜像、日志、数据库/Redis/Sub2API 运行数据、Nginx 配置或旧 `images` 目录。执行前后均按精确路径和保留白名单校验，未使用通配符或任何 prune 命令。

- 删除门禁精确命中 37 个旧归档/备份路径，删除前占用 `2,220,195,840` 字节（约 2.07 GiB）。净文件系统占用从 `11,121,172,480` 降至 `8,901,169,152` 字节，可用空间从 `22,875,725,824` 增至 `25,095,729,152` 字节，根盘使用率由 33% 降至 27%；净释放 `2,220,003,328` 字节，差量与操作期间活动日志/数据库写入有关。
- 当前 Sub2API `1a4a690dd999`、回滚 `f79803bb73d6`、当前积分 `b64a0110ab2c`、回滚 `fc7ea1fe59c0` 四个归档在删除前后均通过已记录 SHA256 校验。
- 只保留 `/home/api/backups/points-dual-origin-20260802-123509` 与 `/home/api/backups/points-all-checkin-20260802-103531` 两个完整恢复点；两份 dump 在删除前后通过 SHA256，删除前另通过 `pg_restore -l` 目录可读性检查。
- Docker 仍为 17 个镜像、5 个活动镜像、0 本地 volume、0 build cache；未删除任何 daemon 镜像，923.7 MB 的未使用镜像空间仍保留。
- Sub2API、积分服务、PostgreSQL、Redis 和 `infinite-canvas` 容器 ID、镜像和启动时间未变化，restart count 均为 0。Sub2API `/health`、积分 `/healthz` 和工作台根路径均返回 200，PostgreSQL `pg_isready`、Redis PING 和 `nginx -t` 通过；Nginx PID 仍为 `1814246`，未 reload。
- `postgres_data`、`redis_data`、Sub2API `data`、旧 `images`、`image-reference-tests` 和 `ui-reference-20260731` 均确认仍在原路径。空的发布/传入目录继续保留，未扩大删除范围。

### 0.3 2026-08-07 提链/额度卡开发候选边界

- 提链/额度卡中心已形成默认关闭的开发候选镜像，镜像已载入生产服务器缓存并启用专用 Nginx 入口；运行中的 Sub2API 尚未切换候选容器，迁移仍未执行。生产第 0 节所列 Sub2API、积分服务、容器和迁移事实除本节明确记录的 Nginx 平滑 reload 外不因该候选改变。
- 产品与资金契约见 [`LINK_CARDS_CN.md`](LINK_CARDS_CN.md)。候选使用同一 Sub2API 进程和数据库，复用真实 `users.balance`、`groups`、`api_keys`、网关定价与 `usage_logs`；公共页面计划使用 `https://key.52token.org`，API Base 计划使用 `https://api.52token.org/v1`。
- 候选默认 `link_cards_enabled=false`、开发模式开启且名单仅用户 ID `1`；管理员控制台受管理员认证保护。该默认值只是开发门禁，不是生产已验证状态，也不得据此推断用户 `1` 当前生产可访问。
- `194_link_cards.sql` 仍是未应用候选迁移。当前生产 `public.schema_migrations` 仍以第 5 节已核验基线为准；在共享或生产数据库首次应用 `194` 前，必须完成数据库备份、隔离迁移、标准 Key 零回归、文本后扣超额完整入账、欠费禁用、媒体并发额度预留、退款在途收口和旧镜像回滚审查。
- `2026-08-07` 本地已完成后端 `-tags=unit ./internal/...` 全量测试（并补齐既有 points `/auth/me` 契约字段）、提链定向 Go vet、Ent/Wire 重新生成、迁移测试、前端完整 Vitest、Vue TypeScript 检查和前端生产构建；覆盖原生分组交集、专属倍率覆盖、专属分组运行时撤权、发行/当前倍率换算、欠费充值自动恢复和普通 Key 隔离。`0.06x`、`0.07x`、`0.08x`、`0.09x` 的双向换算矩阵固定向上取一位小数，内部结算倍率不低于当前原生倍率。该记录不代表迁移或生产验收完成。
- 私有 `main` 提交 `71dff449192f982008bc333940636827a9f88786` 已由 Cachecompat Image [run 31174082161](https://github.com/hxly520/sub2api/actions/runs/31174082161) 构建、容器内 `-version` 冒烟并发布。不可变镜像为 `ghcr.io/hxly520/sub2api:0.1.169-71dff449192f`，registry digest 为 `sha256:9790db6362f924bbc55770e6f8b2f51b15559ad176708a589e0ad500503e5551`，OCI 创建时间为 `2026-08-07T11:32:42.3618001Z`；`latest` 与 DockerHub 均未发布。工作流同时刷新了可变标签 `0.1.169`，部署和回滚只能使用前述 commit 不可变标签或 digest。
- 本地回读归档位于仓库外 `C:\Users\Mr.O\Documents\sub2api\_build\image-71dff449192f\sub2api-0.1.169-71dff449192f-linux-amd64.tar`，大小 `42,720,256` bytes，SHA256 为 `1be64c1bd42b294cf690c2c5d9038a72c9d3ee0bee628316e3e370a5e2ecbeb4`。同一归档已上传为 `/home/api/sub2api-deploy/image-archives/sub2api-0.1.169-71dff449192f-linux-amd64.tar`，远端 SHA256 完全一致、权限 `0600 root:root`；`docker load` 后候选 image ID 为 `sha256:5e449fa9c3927f2f9f3d22c678d117285c7fc873db1e68670f89cd84c0050217`，OCI revision 与创建时间分别为完整 `71dff449192f982008bc333940636827a9f88786` 和 `2026-08-07T11:32:42.3618001Z`，远程 `-version` 冒烟通过。
- `key.52token.org` 的 Cloudflare DNS/HTTPS 映射已经生效。服务器备份位于 `/home/api/sub2api-deploy/backups/link-card-nginx-20260807-203936`；`/etc/nginx/conf.d/key.52token.org.conf` 与 `/etc/nginx/snippets/sub2api-key-proxy.inc` 的 SHA256 分别为 `4f1b063b932f6c8c5ee0d8661c2ee09b3bc30e451c0405ec8d0d46ba258e816a`、`ccccec93f3dc82d4386a321fc3f6077c30d3cd2a6355d965d743859e20931f32`，与仓库模板一致。`nginx -t`、平滑 reload、源站和 CF 外部检查通过：`/` 返回 `302 /card`、`/card=200`、`/health=404`、未知路径 `404`；Nginx 仍为 PID `1814246`、restart `0`。
- Nginx 与镜像缓存操作前后，运行中的 Sub2API 始终为容器 `69a710a1ad0c`、镜像 `ghcr.io/hxly520/sub2api:0.1.169-1a4a690dd999`、healthy；Compose 未修改，未执行 `up`。数据库只读核对确认 `public.schema_migrations` 中 `194_link_cards.sql` 计数为 `0`，`public.api_keys.key_type` 列计数为 `0`，因此候选迁移尚未提前应用。
- `2026-08-07` 需求再次确认文本请求完全沿用 Sub2API 后扣：准入时可用额度必须严格大于零，提链 `quota=0` 不代表不限额；最后一笔和已经准入的并发请求允许形成欠费。候选按完整实际费用增加 `quota_used`，结算后标记 `depleted` 并拒绝后续请求，禁止把费用截断成剩余额度。欠费充值只有覆盖已用额度和在途预留且仍有正余额后才恢复；媒体与批量生图仍保持请求前预留。该行为仍需在维护者手工切换候选后用用户 `1` 做最小金额逐笔对账。
- 最终代码提交 `daa7cb3fb4601a9f5d6bbf38b7c6f225c3c98676` 进一步固定提链零额度失败关闭和欠费卡不能通过管理员解冻恢复。GitHub CI [run 31190916794](https://github.com/hxly520/sub2api/actions/runs/31190916794) 全绿，包含后端 unit、golangci-lint、前端、积分 PostgreSQL 事务测试及 Sub2API 完整 PostgreSQL/Redis integration；并发用例验证两笔已准入请求均按完整实际费用入账、欠费状态生效且重复结算幂等。
- Cachecompat Image [run 31192125427](https://github.com/hxly520/sub2api/actions/runs/31192125427) 已完成构建、容器内 `-version` 冒烟和 GHCR 发布；DockerHub 步骤按未配置状态跳过，`latest` 未发布。不可变镜像为 `ghcr.io/hxly520/sub2api:0.1.169-daa7cb3fb460`，registry digest 为 `sha256:5d0a2b7f1ccf2d46fc22b96d7d4a25dd450b3db8c5a98f21d44df40ea26a0626`，OCI 创建时间为 `2026-08-07T15:24:09.72663139Z`。
- 本地回读归档为仓库外 `C:\Users\Mr.O\Documents\sub2api\_build\image-daa7cb3fb460\sub2api-0.1.169-daa7cb3fb460-linux-amd64.tar`，大小 `42,721,280` bytes，SHA256 `67c01a16d37be0c0d17403138dafe6eaa02efc4fb788c19e6add6afdf955b13e`，并通过 `crane validate`。同一归档已上传至 `/home/api/sub2api-deploy/image-archives/sub2api-0.1.169-daa7cb3fb460-linux-amd64.tar`，远端大小和 SHA256 一致、权限 `0600 root:root`；`docker load` 后 image ID 为 `sha256:740d156c1740a4965641cd38ec3ecfefa0d189bc7aea7184b9263509658c2edf`，OCI revision 与容器内 `-version` 均为完整 `daa7cb3fb4601a9f5d6bbf38b7c6f225c3c98676`。
- 镜像载入后未执行 Compose、未启动候选、未触发迁移。运行中的 Sub2API 仍为容器 `69a710a1ad0c...`、镜像 `0.1.169-1a4a690dd999`、healthy、restart count `0`、原启动时间不变；积分、PostgreSQL、Redis 和工作台容器也未变化。数据库只读复核仍为 `194_link_cards.sql=0`、`api_keys.key_type=0`；`key.52token.org` 为 `/ -> 302 /card`、`/card=200`、`/health=404`、未知路径 `404`。
- 自动 PostgreSQL 资金竞态测试已经完成；在用户 `1` 最小金额逐笔对账、非名单服务端拒绝和三端桌面/移动真实验收前，仍不得应用生产迁移、切换生产容器或开启全体功能。
- 候选构建仍按现有边界只把 Sub2API 镜像上传、导入或缓存到服务器，由维护者手工切换；自动化不得替换运行容器、执行生产迁移、修改 Nginx 或开启 `link_cards_enabled`。

### 0.4 2026-08-08 提链候选与用户 1 真实验收

- 当前运行 Sub2API 为 `ghcr.io/hxly520/sub2api:0.1.169-daa7cb3fb460`，容器 `157eaffa7973`，healthy，restart `0`；PostgreSQL、Redis、积分服务和生图工作台均未因本次操作重启或替换。
- UI 修复提交 `b3e230220a9dd023d133b4184a0c0a164ea95d51` 已通过 GitHub CI；候选镜像 `ghcr.io/hxly520/sub2api:0.1.169-b3e230220a9d`、GHCR digest `sha256:39baee21d5cfb259d5d903f5a8d54678b58a87345798539bb8a0246681110f81` 已由 GitHub 构建并只载入服务器缓存。归档路径、SHA256 与验收证据见 [`LINK_CARDS_ACCEPTANCE_20260808_CN.md`](LINK_CARDS_ACCEPTANCE_20260808_CN.md)。
- 用户 1 使用 `0.08x` 分组 ID `9` 完成提链 Key 创建、幂等重放、公共激活、`/v1/models` `200` 鉴权冒烟、充值、冻结/解冻、管理员退款和未激活用户退款。提链账本净 `creator_balance_delta=0`，活动提链 Key 为 `0`；临时分组授权已撤销，标准 Key 未受影响。
- `link_cards_enabled=false`、开发模式和名单 `[1]` 保持不变。候选镜像不得由自动化替换运行容器；维护者手动切换前继续保留全局关闭和审计记录。

## 1. 权威来源

- 私有仓库：`hxly520/sub2api`。
- 当前仓库基线为官方 Sub2API Release `v0.1.169` 与私有兼容层；当前新增关键节点为 credit 审计修复 `1a4a690dd`、最终消费阶梯 `1d8d50522` 和冲正净额展示 `fc7ea1fe5`。`backend/cmd/server/VERSION=0.1.169`，后续官方升级仍须逐项保留二开契约。
- 当前生产 Sub2API、积分服务及其容器身份以第 0 节为准。自动化不得替换 Sub2API；积分服务允许在备份、不可变镜像、仅单服务 Compose 和完整验收边界内独立发布。
- `2026-08-01 22:34 CST` 仅更新积分服务镜像和运行门禁：本地归档 SHA256 为 `bbf7d051b2295f230e65d80b77d5ecaf7dac0a049a576fa78e04eb586583ce1f`，服务器 `docker load` 后运行 `bee059a1cec5`，启动健康、restart `0`、无启动错误；Sub2API 容器未更换版本。`POINTS_SYSTEM_ENABLED=true`、`POINTS_USER_ACCESS_MODE=all`，两份 preview list 为空；当前签名 user-access 对用户 1、2、11、174、187 均返回 `200/allowed=true`。
- 生图工作台唯一源码：[`hxly520/infinite-canvas`](https://github.com/hxly520/infinite-canvas) 的 `main`；它独立于Sub2API版本发布。
- 独立积分系统源码：本仓库 `points-system/`。它复用现有 PostgreSQL 实例和 `sub2api` 数据库中的独立 `points` schema，不再部署第二个 PostgreSQL；实际容器、schema、域名和 Nginx 状态以本文件后续发布记录为准。
- 提链/额度卡中心维护契约：[`LINK_CARDS_CN.md`](LINK_CARDS_CN.md)。它是 Sub2API 内置开发候选，不是独立数据库或第二套网关；是否已部署只能由第 0 节后续明确生产记录确认。
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
- 私有提交 `9f1b6bae` 增加明确失败即时退款、未知终态保留冻结、成功费用按报价封顶及全站到期冻结后台核销；该功能继续包含在当前生产 `v0.1.169-1a4a690dd999` 基线中，后续升级仍须持续检查后台核销审计和异常冻结聚合。

### 2.2 2026-07-30 历史运行基线

- Sub2API：`ghcr.io/hxly520/sub2api:0.1.168-339422728b2c`，OCI revision `339422728b2ceb87b4a81bb08229d370c4ca589d`，image ID `sha256:a8abdd92bb9f59082d9bbecbdf4812b8f1a441aed4f189bc674104f07074625b`；容器 healthy，restart count `0`。
- 积分服务：`ghcr.io/hxly520/sub2api-points:0.1.168-c0fe91506bca`，OCI revision `c0fe91506bca60dfcc96b6d868b48b30d2ca86f0`，image ID `sha256:a6a899da83a5e82eccb9bd8c2473b72c274d32d6c78e8d50774654f21dfd166b`；容器 healthy，restart count `0`。registry digest 为 `sha256:8a9b7f51ce454450fc797aeeb7bfea008351cdba354327ae6cf40d3ddbdb4148`；服务器通过 SHA256 为 `d8bed76bd257e4ecb3e72dddb5e26c11147274738a3e7e316e2015c85568ef7d` 的本机构建归档执行 `docker load`，没有在生产机编译或构建。
- 两个服务复用 PostgreSQL `17.8` 的同一个 `sub2api` 数据库。积分写表位于独立 `points` schema，共 19 张表、2 条积分迁移；`points_app` 写角色连接上限为 8，`points_usage_reader` 只读角色连接上限为 4 且只允许读取 `usage_logs` 指定列。
- Sub2API `public.schema_migrations` 当前共 250 条记录，私有迁移 `192_media_balance_hold_reconciliation_index_notx.sql` 和 `193_points_balance_credit_ledger.sql` 均已应用；积分迁移只记录在 `points.points_schema_migrations`，两套迁移表不得混用。
- 首次积分部署前备份位于 `/home/api/sub2api-deploy/backups/sub2api-before-points-20260730-033634.dump`，SHA256 为 `b48e6a8d8bcb2d50f12e301da16ece87174f94edf535702aa12e575cbed3c293`。积分服务的启动、建 schema 和 Nginx 平滑 reload 均未停止 Sub2API 业务。
- Sub2API 加固镜像已由维护者切换上线；积分中文双工作区镜像也已独立切换，过程中 Sub2API 容器 ID、启动时间和镜像引用完全未变。积分切换前备份位于 `/home/api/sub2api-deploy/backups/points-ui-c0fe91506bca-20260730-190702/`，其中 `points-schema.dump` SHA256 为 `b9b0e477267b6710a8a3df8cb311f5f5707c9f1779cf265a47623e2ce98b53dd`。
- exact-root 公开首页由 Nginx 读取宿主 `/home/api/sub2api-deploy/public/index.html`，不属于 Sub2API 镜像层。`2026-07-30 16:19 CST` 已原子更新为仓库新版，宿主与 Nginx 响应 SHA256 均为 `86eb43d94050780fd9dc81da6e189c469f709bffa11c63eed195cdc065d229e5`；旧文件备份为同目录 `index.html.bak-20260730081909`，未重启 Nginx 或 Sub2API。

### 2.3 2026-07-31 至 2026-08-01 运行演进

- `2026-07-31 22:05 CST` 只读核对：Sub2API 容器为 `c37a7a014997...`，运行 `0.1.169-04a19ca082ee` / `04a19ca082ee43853573795d1385727bd38f20e9`，image ID `sha256:b2fcce5b...`，healthy、restart count `0`；Compose 已引用同一不可变标签。
- 同次核对的积分容器为 `e92b5ddc872b...`，运行 `0.1.169-04a19ca082ee` / 同一 OCI revision，image ID `sha256:568c1af8...`，healthy、restart count `0`；Compose 已引用同一不可变标签。`points` schema 的 21 张表和 3 条迁移为此前已核验基线，普通更新不得重跑历史作业。
- `2026-07-31 22:05 CST` 同次核对时，policy v3 已启用，比例 `10.00:1`，刷新 `00:05`，签到关闭。全历史作业 `5174eef7-5f0a-4a17-b4f1-f50840940f64` 已成功；当时共有 29 个积分账户、316 条每日快照、311 条积分账本，`needs_review=0`，签到与余额发放记录均为 0。用户 ID 1 当时的核对值为总积分 `7514.94`、昨日积分 `938.07`。
- 上述历史作业是当前 schema 的一次性基线，后续积分镜像更新不得重新执行 `activate`、`plan`、`apply` 或 `resume`。日常积分只由 `00:05` 调度和滚动差量对账维护。
- 分阶段开放保持 `points_system.enabled=false`，并仅设置 `points_system.preview_user_ids: [1]`。`2026-07-31 22:06 CST` 已在不重启容器的前提下把 `POINTS_SYSTEM_PREVIEW_USER_IDS=1` 原子写入 root-only `bridge-secrets.env`，备份位于 `/home/api/sub2api-deploy/backups/points-preview-user1-20260731T220613+0800`，`docker compose config -q` 通过；该值当时要等维护者手工切换包含预览能力的新候选后加载。`2026-08-01 01:04 CST` 的后续核对已确认切换完成，用户 ID 1 可看到菜单、访问 `/points` 并获取 user ticket，其他用户即使手工访问 URL 也由服务端拒绝；签到仍由 policy 独立保持关闭。
- `2026-07-31 23:27 CST` 积分服务已独立切换为 `f79803bb73d6`，root-only `points.env` 固定 `POINTS_USER_ACCESS_MODE=preview`、`POINTS_USER_PREVIEW_IDS=1`。真实签名票据验证用户 2 在积分端为 `403`，用户 1 嵌入工作区、历史用户名字段、每日积分、管理员工作区和注销均正常，全部测试 session 已清零。该记录只证明旧镜像运行态；下一候选必须改验 `login_email`，切换前先把 `points_app` 扩展为 `id/email/username/deleted_at` 双读兼容态，验收后再收敛到 `id/email/deleted_at`。Sub2API 当时仍为原容器和 `04a19ca082ee`。
- `2026-08-01 01:04 CST` 再次只读核对确认，维护者已把 Sub2API 手工切换为 `f79803bb73d6`：容器 `63d320fbf6ca...`，启动时间 `2026-07-31T15:47:53.260825915Z`，healthy、restart count `0`。积分服务仍为容器 `05f43434fc20...`、同 revision、healthy、restart count `0`；该核对没有修改配置、数据库、容器或镜像。此后所有“Sub2API 仍为 04a19ca”文字只表示对应时间点的历史事实，不再表示现状。
- `2026-08-01 04:59 CST` 积分服务独立切换为 `e39c78bf8f6c`：切换前全库 backup SHA256 为 `099a567598f64b07b25678e9fccb43f941f3daf41191beaddf73369f1fbc4574`，ACL 阶段 A 事务为 `1960217`，Compose 回滚点为 `backups/compose.pre-e39c78bf8f6c-20260801-045914.yml`。新容器 `11b1f9820fa7...` healthy、restart count `0`；启动对账 `changed_users=0`，29 账户、328 快照、322 账本和 187 个 Sub2API 用户均未变化。Sub2API 仍为原容器 `63d320fbf6ca...` 与 `f79803bb73d6`，没有重建或重启。
- `2026-08-01 08:17 CST` Taste 数字工作区积分镜像独立切换为 `e8d73f3e6655`：切换前全库 backup 位于 `/home/api/backups/points-release-e8d73f3e6655-20260801-081507/sub2api-full.dump`，SHA256 为 `843ff7734791e92517c9bb02b56eb7b033c1e0662b50936d0cf44ea4d4c76df7`，`pg_restore -l` 共 1,291 行；同目录保留 Compose、切换前后计数、容器身份和启动日志。新积分容器 `1166d2ff140c...` healthy、restart count `0`，启动对账 run `0144d6a3-3158-4e73-ad94-e03de54dcb9a` 读取 30,078 行和 12 个来源用户，`changed_users=0`。29 账户、328 快照/修订、322 账本、187 个 Sub2API 用户以及空签到/发放表均未变化；Sub2API 仍为原容器 `63d320fbf6ca...`，没有重建或重启。
- `2026-08-01 00:05 CST` 积分自动调度成功结算 `2026-07-31`，来源用户 12、来源行 30,078、变更用户 12；只读复核后共有 29 账户、328 快照、328 修订、322 账本，`needs_review=0`，签到/签到尝试/余额发放/发放尝试均为 0。该正常调度没有新增或重跑历史基线作业。
- 4 个遗留 `points_shared_*` 测试 schema 已清理。清理前备份位于 `/home/api/sub2api-deploy/backups/points-test-schema-cleanup-20260731-090001`，dump SHA256 为 `ecfc41fb6d3fbd332b3b0b86f9f8707257a81d4c266d9c6a7d6290c9ce661c29`，catalog 659 行；清理后测试 schema 为 0，正式 `points` 计数未变化。

### 2.4 2026-07-31 同步生图冻结核销与参数验证

- 发现 10 条超过 30 分钟、无成功 usage/任务证据的同步图片 `dispatched` 冻结，共 `1.02 U`；用户 1 为 `0.20 U`，用户 160 为 `0.82 U`。root-only 操作前备份位于 `/home/api/sub2api-deploy/backups/media-hold-no-output-refund-20260731-092046`，custom dump SHA256 为 `77c60a5611000d4c3ae945f0ce71f85e99005393e1416a843a1ae5c98a9706b7`。
- 固定 ID、总额、用户数和成功证据断言均通过后，在单一事务中把 10 条全部退款并标为 `released`；审计为 `audit_logs.id=3814`、request ID `hold-refund-no-output-20260731`。完成后 active hold、active 金额和非零冻结用户均为 0，Sub2API 未重启或替换。
- 使用工作台精确自动质量参数的主分组请求在 99 秒后成功生成 1 图，hold `408` 已 `captured` `0.10 U`，usage `219522` 只记一次；生成后 active hold 和非零冻结用户仍为 0。参考图 SHA256 为 `bc8a8bcabcbdc33429d035e9be90d61bef539fbc03811b8bc53f575a93f5b6c6`。
- 当前 `v0.1.169-1a4a690dd999` 继续把同步 `/v1/images/generations` 未知终态冻结窗口从通用 24 小时缩短为 30 分钟；明确失败仍即时退款，异步图片/视频仍使用 24 小时。后续升级不得回退该分流规则。

## 3. 域名和进程边界

活跃域名包括：

- `52token.org`
- `api.52token.org`
- `image.52token.org`
- `video.52token.org`
- `points.52token.org`

活动业务域名的目标清单只允许上述 `52token.org` 系列。`gpt-codex.top` 及其子域名已经淘汰，不应作为生产入口、回滚入口或仓库外依赖。`2026-08-01 08:29 CST` 只读执行 `nginx -T` 时仍发现 `/etc/nginx/conf.d/sub2api.conf`、`sub2api-api.conf`、`sub2api-images.conf` 加载了旧域名 server block；本轮按只读边界未修改或 reload Nginx，已作为配置漂移记录。下一次 Nginx 维护必须先备份并移除这些旧 block 和对应证书引用，通过 `nginx -t` 后再平滑 reload；在此之前不得宣称旧域名已清理，也不得把它写入新的积分 Origin、前端配置或发布说明。

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
- 用户入口仍在 Sub2API `/points` 的右侧内容区嵌入受票据保护的积分 iframe，左侧导航、Header、主题状态和上传 Logo 始终由 Sub2API 提供。已上线的积分 `e8d73f3e6655` 使用来源严格校验的实时主题同步，子页跟随 Sub2API 明暗切换但不能反向改写官方导航；父主题跨资料刷新持续生效，并用 Sub2API 的浅色中性层级、深色 `dark-950/dark-800/dark-700` 层级和 teal 主色覆盖表格、悬停、分页、状态标签及 Canvas。Taste 基线为 `VARIANCE 6 / MOTION 4 / DENSITY 5`，主积分焦点、低对比精密网格、真实同步状态、8 px 面板尺度和 reduced-motion 只作用于积分内容区；四张概览卡保持紧凑等高，两张记录表默认各 10 条并独立翻页。管理员在 `/admin/settings/points` 点击“打开积分配置”后，以新浏览器标签页打开受一次性管理员票据保护的独立策略台；原 Sub2API 设置页保持原位，页面底部不得再追加管理员 iframe。新标签页必须隔离 opener/referrer，并对浏览器拦截新窗口给出明确失败状态。
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
- 私有迁移 `193_points_balance_credit_ledger.sql` 已进入生产，为积分服务向 Sub2API 幂等发放余额。积分服务自己的 4 条迁移位于同一数据库的独立 `points` schema，共 21 张表，并记录在 `points_schema_migrations`；不得写入或混入 Sub2API `public.schema_migrations`。第三条迁移保存一次性历史作业及逐日游标，当前生产作业已成功且不得重跑；第四条迁移增加原始消费阶梯与可空金额上限，不增加新表。
- 私有迁移 `194_link_cards.sql` 仅存在于 `2026-08-07` 开发候选，尚未进入生产。它计划扩展 `api_keys` 并创建提链分组授权、永久幂等和不可修改资金流水表；首次在任一共享环境应用后即纳入 checksum 不可变清单，只能用后续新迁移修正。生产盘点发现 `194` 记录前不得宣称提链数据表已上线。
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
10. 若候选包含提链功能，记录功能开关、开发名单是否仅用户 `1`、标准/提链 Key 数量、各状态和资金流水聚合；不读取或保存完整 Key。核对创建者余额差量、提链储备/已用/退款守恒、在途额度和重复 request/operation 数为零异常。

原始盘点输出只能保存到仓库外受控位置；Git文档只保留本文件这类脱敏结论。

## 9. 版本升级与镜像发布

1. 从完整克隆的当前私有 `main` 创建 `codex/upgrade-vX.Y.Z-*`。
2. 获取并审阅目标官方Release tag；禁止直接跟随滚动官方 `main` 或部署官方 `latest`。
3. 解决协议、调度、四种图片模式、媒体计费、提链 Key 鉴权/计费、Nginx/Worker和支付冲突，并运行 `docs/PRIVATE_CUSTOMIZATION_CN.md` 与 `docs/LINK_CARDS_CN.md` 中的完整门禁。
4. 通过 CI 或受控本机构建环境构建私有镜像。版本取自 `backend/cmd/server/VERSION`，镜像标签必须包含版本和 commit；发布摘要必须记录 manifest digest、image ID 和传输归档 SHA256。
5. Sub2API 新镜像只上传、导入或缓存，不立即替换运行容器；积分镜像可在 `points` schema 备份和旧镜像记录完成后独立更新。
6. 升级窗口前确认没有不可恢复的 `image_task:*` 在途任务，批量队列可恢复，私有媒体任务没有异常冻结余额；若已启用提链，再确认在途额度为零或可安全恢复、资金账本守恒且旧镜像不会接管活动提链 Key。
7. 只替换镜像引用，不同时调整账号、价格、Redis、Nginx或数据库参数。
8. 上线后核对OCI revision、VERSION、health、DB/Redis、迁移、关键路由、任务终态和日志。

`2026-07-30` 与 `2026-08-01` 的 GitHub Actions 均因账户计费或支出限额在 runner 分配前终止，job 未执行任何 step；两次均改由受控本机生成标准 Docker archive，服务器只拉取/导入，不编译。Sub2API 曾切换为 `v0.1.168-339422728b2c`，积分服务也经历过 `v0.1.168-28e760bc8c6d`，这些只属于历史发布链。registry digest、image ID 与 archive SHA256 必须分别记录，禁止互相冒充；当前 Sub2API 为 `v0.1.169-1a4a690dd999`，当前积分服务为 `v0.1.169-b64a0110ab2c`，精确状态以本文件第 0 节及链接记录的第 0、11.14、11.15、11.16 节为准。

当前通用部署文档包含官方 `weishaw/sub2api:latest` 示例，只适用于官方默认部署。私有生产严禁照搬该镜像引用。

### 9.1 独立生图工作台发布

1. 从完整克隆的 [`hxly520/infinite-canvas`](https://github.com/hxly520/infinite-canvas) `main` 创建变更分支；不从服务器旧 `images` 快照反向生成源码。
2. 在该仓库完成构建和测试，由 `.github/workflows/docker-image.yml` 发布 `ghcr.io/hxly520/infinite-canvas:sha-<commit>`。
3. 记录完整commit、镜像tag和registry digest；生产Compose固定使用SHA tag或digest，禁止使用 `latest`。
4. 只替换15731工作台镜像，不同时变更Sub2API、Nginx、Cloudflare或API Base。
5. 联合回归 `/v1/images/*`、`/v1beta/models/*`、视频任务和媒体代理路径，并验证首页、静态资源、Image2/Gemini请求、参考图、IndexedDB与浏览器本地素材。

### 9.2 独立积分系统发布与安全边界

积分系统已经完成镜像导入、同库隔离、Nginx 接入、中文双工作区、管理员用户明细、一次性历史回算、全体积分中心/签到门禁、昨日原始消费四阶梯、冲正净额展示和双精确父 Origin；当前积分服务为 `v0.1.169-b64a0110ab2c`，Sub2API 为 `v0.1.169-1a4a690dd999`。以下步骤同时是后续重部署和版本合并的强制边界，任何自动化都不得替换或重启 Sub2API。

1. 优先由 GitHub 分别构建 Sub2API 与 `points-system` 的 commit 不可变镜像，记录两者 tag、OCI revision 和 registry digest；生产机不编译。CI runner 受计费门禁时可使用受控本机构建和标准 archive，但仍须把镜像推送 GHCR 并分别记录 registry digest、archive SHA256 和服务器加载后的 image ID。
   第 2-4 步只适用于空白新环境或经审计的灾难恢复。当前生产角色、schema 和密钥均已存在，普通积分镜像更新不得重跑 bootstrap 或重新生成密钥。
2. 先对现有 `sub2api` 数据库执行一致性备份并记录 SHA256/恢复命令；由 PostgreSQL bootstrap superuser 使用全新的角色名，在同一数据库创建独立 `points` schema 和 `points_app` 最小权限角色，写池默认最多 8 条连接。新装角色对 `public.users` 只能读取 `id/email/deleted_at`，其中 `email` 仅用于登录邮箱展示。角色或 schema 已存在时脚本必须停止并转人工 ACL 审计，不得覆盖。不得创建第二个 PostgreSQL 容器。
3. 由同一 bootstrap superuser 为 `POINTS_USAGE_DATABASE_URL` 创建全新的只读账号，只授予 Sub2API `usage_logs` 所需列的 `SELECT`，并强制只读事务和最多 4 条连接；不得复用 Sub2API 写账号或自动修改共享 PUBLIC ACL。
4. 生成独立 Base64 32 字节以上的 session、launch、credit 和内部集成密钥。生产 `points.env`、bridge env 和 psql 变量文件必须为 `root:root 0600`；Sub2API 与积分服务只共享对应公约中的同一解码后字节。状态或配置 API 只能返回是否已配置、Key ID 等非敏感元数据，不得回传密钥原值；文档、Compose、shell history 和日志也不能出现真实值。
   存量生产角色不得执行第 2 步或重跑历史 username 模板。登录邮箱候选切换前，先备份并记录积分账户/快照/账本与 Sub2API 用户计数，再执行阶段 A `shared-database-users-email-upgrade.sql.example`；它只授予 `email` 并保留 `username`，断言精确双读兼容态。新镜像真实验收后执行阶段 B `shared-database-users-email-finalize.sql.example` 撤销 `username`。两阶段均须保存审计输出、复核计数不变，并拒绝整表、其他列、写权限或 PUBLIC ACL。阶段 B 后回滚必须先执行 rollback-prepare，切回旧镜像验收后再执行 rollback-finalize。
5. 运行积分迁移和只读消费查询自检后启动积分容器，再启用 Nginx 精确反代。可信代理 CIDR 必须与实际容器/loopback 网络一致；根路径返回 404。只有 `/launch` 关闭 access log，其余路径必须保留访问证据；此阶段不得修改、替换或重启现有 Sub2API 容器。
6. 当前积分中心保持 Sub2API points 全局 enabled 与积分服务 `POINTS_USER_ACCESS_MODE=all`，签到门禁也为 `POINTS_CHECKIN_ACCESS_MODE=all`；两份 preview ID 均为空。关闭 policy `enabled` 时仍自动隐藏用户入口并拒绝用户 API；签到是否可用继续由 policy 签到开关、最低昨日消费、快照、阶梯和每日次数共同决定。完整门禁配置只保存在服务端，对浏览器只返回当前用户自己的布尔能力。
   候选切换前必须从所有实际 `/points` 地址栏 URL 收集 `scheme://host[:port]`。主值写入 `POINTS_EMBED_PARENT_ORIGIN`，其他父站以逗号分隔写入 `POINTS_EMBED_PARENT_ORIGINS`；每项必须是无路径、尾斜杠、凭证、查询、片段和通配符的精确 Origin。当前生产主值为 `https://api.52token.org`，附加值为 `https://52token.org`。必须确认用户 iframe 所需的 Sub2API CSP `frame-src` 包含积分 Origin、积分响应 `frame-ancestors` 和 `img-src` 精确包含全部父站、没有冲突的 `X-Frame-Options`，并通过真实用户桌面/移动浏览器确认收到 `sub2api:points-ready`。只检查 `/launch` 或 `/app/` 的 HTTP 状态不足以发现浏览器 CSP/消息 Origin 不匹配。管理员点击“打开积分配置”必须通过 step-up 生成一次性管理员票据并在带 `noopener,noreferrer` 的新标签页打开；原设置页保持不变，禁止重新在底部嵌入管理员 iframe。
7. 验证用户/管理员角色隔离、票据一次性、CSRF、昨日消费快照、两位小数比例、最低昨日消费门槛、并发签到、三层金额上限和余额交易幂等后再开放用户 bridge。用户页、管理员页、全站明细和签到发放任务必须显示 `login_email`，不得继续返回 `username` 或无必要的 `user_id`。policy enabled 不等于签到 enabled，任何配置或镜像切换都不得隐式开启签到。
8. 经本次明确授权，policy v7 作为 v5 的等值克隆于 `2026-08-02` 当日生效；原 v5 保持不可变并于 `2026-08-03` 接管。两者均为最低昨日消费 `1 U`、每日 1 次、四档 `1%-5% / 2%-5% / 3%-5% / 4%-5%` 和三个 `NULL` 金额 cap。该次同日启用有唯一管理员审计，不改变后续策略一律次日生效的常规规则。
   个人消费积分账本的“发放时间”字段为 `awarded_at`，按 `Asia/Shanghai` 业务时间展示：`business_date + 1` 发放自然日零点加该发放日实际生效策略的 `refresh_minute`，不得在策略切换日沿用消费日或账本绑定策略，当前默认即次日 `00:05`；非消费、旧版或找不到发放日策略的记录回退 `created_at`。展示投影不得修改不可变账本。
9. 余额发放超时后只重试同一交易 UUID；未知 credit 结果禁止直接冲正，必须确认 settled 后再发起关联 debit。确定性 4xx 进入永久失败终态，由管理员检查审计后显式重试；禁止删除幂等账本后重新发放。
   Sub2API `0.1.169-1a4a690dd999` 已运行并完成 credit/reversal 审计验收，临时兼容对象已删除。以下 SQL 只作为已执行的历史操作记录，不应在日常发布中重复运行：

   ```sql
   DROP TRIGGER IF EXISTS points_credit_audit_request_body_compat ON public.audit_logs;
   DROP FUNCTION IF EXISTS public.points_credit_audit_request_body_compat();
   ```
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

# 存量升级阶段 A：只授予 email 并保留 username；旧镜像继续可用。
cat POINTS_APP_PSQL_VARS points-system/deploy/shared-database-users-email-upgrade.sql.example | \
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

# 完成 login_email、角色隔离、用户 1 与非预览用户真实票据验收后执行阶段 B。
cat POINTS_APP_PSQL_VARS points-system/deploy/shared-database-users-email-finalize.sql.example | \
  docker exec -i POSTGRES_CONTAINER psql -X -v ON_ERROR_STOP=1 -U POSTGRES_SUPERUSER -d sub2api

# 若阶段 B 后需要回滚：先恢复 username 且保留 email，再切旧镜像并验收。
cat POINTS_APP_PSQL_VARS points-system/deploy/shared-database-users-email-rollback-prepare.sql.example | \
  docker exec -i POSTGRES_CONTAINER psql -X -v ON_ERROR_STOP=1 -U POSTGRES_SUPERUSER -d sub2api
# docker compose ... up -d --no-deps points-system  # 使用已记录的旧不可变镜像
# 旧镜像用户名读取验收通过后，才撤销 email 恢复历史最小权限。
cat POINTS_APP_PSQL_VARS points-system/deploy/shared-database-users-email-rollback-finalize.sql.example | \
  docker exec -i POSTGRES_CONTAINER psql -X -v ON_ERROR_STOP=1 -U POSTGRES_SUPERUSER -d sub2api
```

新环境首启固定 `POINTS_USAGE_RECONCILE_DAYS=1`。自动调度在策略未启用时不访问 `usage_logs`，只幂等写入零值成功就绪标记；仍须在启动前只读记录昨日行数、表/索引大小、活动连接、长事务、磁盘余量，并在低峰审阅聚合查询 `EXPLAIN`。只有确认资源余量后才可把回算窗口恢复为默认 7 天。当前生产已完成历史基线，普通镜像更新只运行嵌入迁移和日常调度，禁止重跑历史命令。`pg_restore --list` 只证明归档目录可读，不等于可恢复性验证；完整 `pg_restore --clean --if-exists --create` 必须在服务器外或隔离演练实例执行，禁止直接在生产库试恢复。

### 9.3 exact-root 公开首页发布

未登录 Vue `/home` 随 Sub2API 候选镜像发布，但域名根路径 `/` 与 `/index.html` 由宿主 `/home/api/sub2api-deploy/public/index.html` 提供，二者必须分别处理：

1. 先运行前端静态页面契约测试，确认页面无脚本、无表单、无外域资源，且不出现具体国外模型或商业中转宣传名称。
2. 记录仓库 `deploy/public-landing/index.html` 的 SHA256，在宿主同目录备份当前文件。
3. 以同目录临时文件写入并执行原子 rename，保留原属主和权限；不 reload Nginx，不重启 Sub2API。
4. 分别核对仓库文件、宿主文件、`https://52token.org/` 与 `/index.html` 响应体 SHA256。任一不一致立即恢复备份。
5. 新页面通过同源 `GET /api/v1/settings/logo` 使用后台上传 Logo；该接口已随生产 `f79803bb73d6` 上线。每次独立发布 exact-root 前仍须验证接口响应，并保留 `/logo.svg` 回退，避免后台未配置或异常 Logo 时出现空白品牌位。

## 10. 回滚边界

- 回滚必须使用事先记录的不可变镜像digest，不能只依赖可变tag。
- 回滚应用前确认新迁移是否向后兼容；forward-only迁移不能靠切回旧镜像自动撤销。
- 登录邮箱阶段 B 完成后，旧积分镜像回滚前必须先执行 ACL rollback-prepare；旧镜像验收前不得执行 rollback-finalize。阶段 A 或 rollback-prepare 的双读状态是限时兼容窗口，不是最终最小权限状态。
- 数据库恢复属于单独操作，必须先验证备份可恢复性。
- Redis状态不作为唯一业务事实源，但后缀异步图片和队列状态会受Redis持久化影响。
- Cloudflare Worker、Nginx和15731 `infinite-canvas` 工作台各有独立版本与回滚物，Sub2API镜像回滚不会同步回滚它们；旧服务器 `images` 快照不是工作台回滚物。
- 提链迁移是 forward-only。存在提链 Key 时，回滚到不识别 `key_type` 的旧 Sub2API 前必须先关闭入口、冻结并禁用全部提链 Key、失效鉴权缓存、等待在途请求归零并完成资金对账；不得删除幂等或资金流水表，也不得直接把储备余额加回创建者。

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
