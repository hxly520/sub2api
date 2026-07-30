# 2026-07-30 v0.1.168 候选与积分系统生产记录

本文记录 `2026-07-30 CST` 的实际部署事实，用于后续排障、官方版本兼容合并和人工切换。文档不保存密码、数据库连接串、HMAC secret、会话密钥或证书私钥。

## 1. 代码与构建边界

- 私有候选提交：`2ad2815e075aadf0553be9913518af35d8b0c7b3`。
- 官方基线：Sub2API Release `v0.1.168`，官方 commit `99c8e4bf7564823bafbab369acab6539e734c1bb`。
- GitHub Actions 因账户计费或支出限额在 runner 分配前终止；既有候选 SHA 的完整 CI 和安全扫描已经成功，最终两个 Linux/amd64 镜像改由本机构建。新 push 的 job 均无执行 step，不能解释为代码测试失败。
- 本机构建使用 Go `1.26.5`、`CGO_ENABLED=0`、`GOOS=linux`、`GOARCH=amd64`；前端使用 pnpm `9.15.9` frozen install 和生产构建。
- 本机没有 Docker daemon。标准 Docker archive 由 `go-containerregistry` 组装并在写出后重新读取验证；服务器没有编译源码、Go 二进制、前端或 Docker 镜像，只执行了运行时基底导出、文件校验、`docker load` 和一次性冒烟容器。

前端构建固定在同一 SHA 的独立 worktree，产物共 174 个文件、`5,531,450` bytes；排序文件清单聚合 SHA256 为 `efe52df0ce966d7549462855c3ea3fddd185eb007769ca237cae03c893b737b6`。Sub2API 二进制内嵌该前端，报告版本 `0.1.168`、commit `2ad2815e...` 和 release build type。

## 2. 最终镜像与归档

| 镜像 | Docker tag | image ID | manifest digest | archive SHA256 | bytes |
| --- | --- | --- | --- | --- | ---: |
| Sub2API 候选 | `ghcr.io/hxly520/sub2api:0.1.168-2ad2815e` | `sha256:3629271e24d230e14911ad136403f096716630c3f60196861e4c1b69b6e9845a` | `sha256:21cba2a077814777016fe2603bf012038009763ce4b2ebcbbd5371541201932a` | `6c1e3696139c0a67094ba619d6b611647ed0c626d9d522f00328c6ef221f00df` | 47,627,776 |
| 积分服务 | `ghcr.io/hxly520/sub2api-points:0.1.168-2ad2815e` | `sha256:b35da824c9dc7c194ca26dc216035b55326e9fc0a47d89dc5939bc7e6bcdc23e` | `sha256:59a145e5b6081986c51277c6234f4c10bb962ad5a5dc9784501712dcabcac7cb` | `fc3662e1e5457ba7377a185bc6a384741990404de3153d864adc734fea45590b` | 7,801,344 |

服务器归档：

- `/home/api/sub2api-deploy/image-archives/sub2api-0.1.168-2ad2815e-linux-amd64.tar`
- `/home/api/sub2api-deploy/image-archives/sub2api-points-0.1.168-2ad2815e-linux-amd64.tar`
- 两个文件均为 `0600 root:root`，服务器 SHA256 与本地一致。
- 为绕开本机到 Docker Hub 的网络阻断，服务器仅执行 `docker save` 导出既有运行时基底，再由本机下载组装；压缩基底保存在 `/home/api/sub2api-deploy/backups/runtime-base-v0.1.164-4736d53e.tar.gz`，大小 `45,185,166` bytes，SHA256 为 `12b4c84cb0515312d68a4a29aaf8c135d0df878a0931fa6ace8700bc5d850c4d`，权限 `0600 root:root`。该操作不是镜像构建，也没有触碰运行容器。

Sub2API 归档使用生产 `v0.1.164` 镜像的 Alpine、运行库、PostgreSQL 客户端和 UID/GID 1000 基底，去掉旧 binary/resources/data/entrypoint 五层后追加新应用层。最终为 8 层，`layers`、`diff_ids` 和非空 History 数量一致。积分归档是单层 scratch 静态镜像，内含 CA bundle、Go zoneinfo 和专用 HTTP healthcheck，运行用户为数字 UID/GID `65532:65532`。

镜像载入和冒烟结果：

- 两个 archive 均由 Docker `26.1.4` 成功 `docker load`。
- Sub2API 一次性容器输出 `Sub2API 0.1.168` 和完整候选 commit；默认 entrypoint 成功降权至 UID/GID 1000；`pg_dump`、`psql`、CA、时区和 resources 可用。
- 积分镜像成功加载 `Asia/Shanghai`；只读根文件系统、无 capability、专用 healthcheck 均已在正式容器验证。

## 3. 生产 Sub2API 手工切换与后续边界

候选载入阶段没有替换生产 Sub2API；维护者随后自行在 `2026-07-30 07:36 CST` 完成手工切换。后续只读复核结果：

- 容器 ID：`0e037ca9275d5d176020dbef93ad4f0cb2da7f171df3314e7dc46179d288c038`。
- 运行镜像 ID：`sha256:3629271e24d230e14911ad136403f096716630c3f60196861e4c1b69b6e9845a`。
- 运行 tag：`ghcr.io/hxly520/sub2api:0.1.168-2ad2815e`，OCI revision `2ad2815e075aadf0553be9913518af35d8b0c7b3`。
- 启动时间：`2026-07-30 07:36:06 CST`；当前 healthy、restart count `0`。
- `points_balance_credits` 已随 Sub2API 迁移创建且当前为 0 行。运行容器未注入 `POINTS_SYSTEM_*`，公开设置仍为 `points_system_enabled=false`，因此不会开放用户菜单或接收余额发放。
- 本轮没有重建、重启或替换该 Sub2API 容器。后续包含管理员 disabled 配置入口、缓存代次和内部端点限流的新镜像仍只由维护者手工切换。

服务器安装的是 Docker Compose v2.27.1 插件，并保留 `/usr/local/bin/docker-compose` 兼容入口。`2026-07-30 08:06 CST` 已先备份原 Compose 到 `backups/docker-compose.before-points-bridge-20260730-080637.yml`（原 SHA256 `d6c6ee9c4e2b186bc130b7df31c11da4d2b9f7c8d7d06a6e7e5841719c6be301`），再只加入 root-only `/home/api/sub2api-points/bridge-secrets.env` 的 `env_file` 引用；新 SHA256 为 `0826fc9b699949eb02aac13b19b7be592c21e420c425116ac97516245314f8de`，权限 `0600 root:root`。`docker compose config -q` 已通过，但没有执行 `up`，运行容器 ID、启动时间、health 和 restart count 均未变化；该配置只会在维护者下一次手工重建时生效，密钥文件保持 `POINTS_SYSTEM_ENABLED=false`。

## 4. 备份与同库隔离

首次积分部署前已完成一致性 PostgreSQL 备份，期间没有重启 Sub2API 或 PostgreSQL：

- 文件：`/home/api/sub2api-deploy/backups/sub2api-before-points-20260730-033634.dump`
- 大小：`83,133,518` bytes
- SHA256：`b48e6a8d8bcb2d50f12e301da16ece87174f94edf535702aa12e575cbed3c293`
- catalog：同路径 `.catalog.txt`，共 1,095 行。

积分服务复用现有 PostgreSQL 17.8 实例和 `sub2api` 数据库，没有创建第二个数据库容器：

- 写角色 `points_app`：非 superuser/createdb/createrole/replication/bypassrls，连接上限 8，只拥有独立 `points` schema。
- 只读角色 `points_usage_reader`：同样无高权限，连接上限 4，强制 `default_transaction_read_only=on`，只拥有 `public.usage_logs` 的 `id`、`user_id`、`billing_type`、`actual_cost`、`created_at` 列级 SELECT。
- 两个角色均无数据库 CREATE、`public` schema CREATE、`public.users` SELECT 或其他 Sub2API 表权限。
- `points` schema 共 19 张表；迁移记录为 `migrations/001_init.sql` 和 `migrations/002_balance_grant_outbox.sql`，不写入 Sub2API 的 public migration 表。
- 初始 policy version 1 为 disabled，签到关闭，basis 为 yesterday，默认刷新分钟为 `5`（00:05）。首次启动只写入前一自然日的零值 succeeded 标记，`source_rows/source_users/changed_users` 均为 0。

## 5. 积分服务运行态

- 部署目录：`/home/api/sub2api-points`。
- Compose：`/home/api/sub2api-points/compose.yml`。
- 敏感运行环境：`/home/api/sub2api-points/points.env`，`0600 root:root`。
- Sub2API bridge 密钥记录：`/home/api/sub2api-points/bridge-secrets.env`，`0600 root:root`；Compose 已引用该文件，但当前保持 `POINTS_SYSTEM_ENABLED=false`，只在维护者后续手工重建 Sub2API 时加载。密钥不得由 API/UI 回显，也不得复制到 Git、工单、聊天或日志。
- 容器：`sub2api-points`，容器 ID `adf14ad1806db4c5cec04d54b1ab4ec429e1565b553e40c2d0198cce854e421f`。
- 镜像 ID：`sha256:b35da824c9dc7c194ca26dc216035b55326e9fc0a47d89dc5939bc7e6bcdc23e`。
- 网络：`sub2api-deploy_sub2api-network`；主机只绑定 `127.0.0.1:8090`。
- 安全边界：非 root、read-only rootfs、`cap_drop: ALL`、`no-new-privileges`、PID limit 128、日志轮转、无 host bind mount。
- 当前 `healthy`、restart count `0`；本地验证为 `/healthz=200`、根路径 `404`、无会话 `/app/=401`、伪造 launch ticket `401`。
- 首启固定 `POINTS_USAGE_RECONCILE_DAYS=1`。确认多日运行资源稳定后，才可按运维窗口恢复默认 7 天。

## 6. Nginx、DNS 与公开边界

- Nginx 配置：`/etc/nginx/conf.d/sub2api-52token-points.conf`；部署副本：`/home/api/sub2api-points/nginx-points.conf`；仓库灾备模板：[`deploy/nginx/points.52token.org.conf.example`](../deploy/nginx/points.52token.org.conf.example)。
- 配置通过两次 `nginx -t` 并平滑 reload；现有 Sub2API、工作台和其他域名未重启。
- 证书：Cloudflare Origin Certificate，SAN 覆盖 `*.52token.org` 和 `52token.org`，有效期至 2041-07-02。
- `points.52token.org` 已启用 Cloudflare 代理 A/AAAA 记录。公网验证：根路径和 `/healthz` 为 404；无会话 `/app/` 与无效 `/launch` 为 401；安全响应头存在。
- `2026-07-30 08:07 CST` 完成首轮反代加固：`api.52token.org` 对 `/api/internal/points/credits` 的 `POST/OPTIONS` 均精确返回 `404`，只允许积分容器通过 Docker 网络直连 Sub2API。`09:02 CST` 又将 HTTP 跳转层从全站关闭日志收窄为仅 `/launch` 关闭；HTTPS 同样只对含票据的 `/launch` 关闭 access log，其余拒绝、越权和限流请求均保留日志。现网 API/积分配置 SHA256 分别为 `b0b47cef7d29b3d944be9f49d680d2c9d8251612206d8d80378ea49405a36ac6`、`3cd74472001fa3f650b52d610e1cd66f90f44d2ad812b26e10a4c0cbaedd6595`。
- 首轮加固前配置保存在 `/home/api/sub2api-deploy/backups/nginx-points-hardening-20260730-080737/`，HTTP 日志收窄前配置保存在 `/home/api/sub2api-deploy/backups/nginx-points-http-log-20260730-090234/`。两次 `nginx -t` 和平滑 reload 均成功；最终 Nginx 为 active、restart count `0`，所有业务容器仍 healthy、restart count `0`，未发生替换或重启。
- Nginx 只允许 `/launch`、`/app/`、`/admin/`、`/assets/` 和 `/api/v1/`；健康路径不公开。直接知道域名不能建立会话，只有 Sub2API 签发的一次性 HMAC ticket 才能进入。

## 7. 当前验证与待办

积分服务当前仍为默认 disabled policy，`points_accounts`、快照、签到、余额发件箱和 Sub2API credit 表均无业务记录，不存在误发金额。已使用服务器内现有密钥完成一次无金额变更的管理员链路冒烟：用户 ID `1` 首次 ticket 交换后进入 `/admin/`，重放返回 `401`，策略只读返回 policy 1，CSRF 注销为 `200`，注销后再次访问为 `401`。测试没有创建策略、签到或余额记录。

后续必须按顺序完成：

1. 等待包含本轮管理员 disabled 配置入口、余额缓存代次、缓存同步重试和内部端点限流的 Sub2API 镜像完成构建；只上传服务器，由维护者手工替换。
2. 手工重建后确认 root-only bridge `env_file` 已注入且仍为 disabled；管理员 `/admin/settings/points` 应可检查桥接并进入策略控制台，普通用户 `/points` 必须继续隐藏。
3. 在管理设置中创建次日生效的完整 policy；初始 disabled policy 不得原地修改。确认比例、刷新分钟、签到模式、最低消费、阶梯和三层金额上限后，再把 bridge 开关改为 true 并由维护者手工重建 Sub2API。
4. 开启前用明确批准的最小金额验证一次 credit、同 UUID 重试、缓存同步、余额账本和 reversal；不得直接改用户余额。验证后保留审计并使净余额回到原值。
5. 继续观察数据库连接、调度零值标记、outbox、容器 restart 和 Nginx 4xx/5xx，再决定是否把 reconcile window 从 1 恢复到 7。

积分服务回滚只停止 `/home/api/sub2api-points/compose.yml` 中的容器；不得删除 `points` schema、迁移、快照、审计、outbox、角色或首次部署备份。Sub2API 回滚继续由维护者在人工维护窗口使用已记录的上一稳定镜像处理，自动化不得替换或重启运行容器。
