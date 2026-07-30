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

### 2.1 管理员积分入口与余额缓存加固候选

用户确认生产页面缺少积分配置后，仓库 `main` 于 `2026-07-30` 快进到提交 `339422728b2ceb87b4a81bb08229d370c4ca589d`。该提交新增始终对管理员可见的“系统设置 > 积分系统”标签和 `/admin/settings/points` 入口、管理员 disabled 状态下的策略台启动、TOTP step-up、余额缓存用户代次/CAS、credit 缓存同步重试、内部端点 fail-close 限流及对应测试。普通用户 `/points` 仍受 `POINTS_SYSTEM_ENABLED` 控制。

GitHub Actions 已在新提交上触发，但仍在 runner 分配前被账户计费或支出限额终止，两个 job 均无执行 step：

- Sub2API：`https://github.com/hxly520/sub2api/actions/runs/30507215061`
- 积分服务：`https://github.com/hxly520/sub2api/actions/runs/30507215626`

随后按既有受控本机构建边界完成 Linux/amd64 候选。服务器只通过 `docker save` 流式导出当前运行时基底；源码、前端、Go 编译、镜像层组装、OCI 标签和归档校验均在本机完成。制品通过 GHCR Registry v2 上传并回读清单、config 与 labels，再由服务器使用临时 `DOCKER_CONFIG` 拉入缓存；临时登录目录在命令结束时删除。

| 镜像 | 不可变 tag | registry digest | image ID | archive SHA256 | bytes |
| --- | --- | --- | --- | --- | ---: |
| Sub2API 加固候选 | `ghcr.io/hxly520/sub2api:0.1.168-339422728b2c` | `sha256:d50f01b1344763616e8198a23107e5f37d815460a5feae902c5bf447cf069f99` | `sha256:a8abdd92bb9f59082d9bbecbdf4812b8f1a441aed4f189bc674104f07074625b` | `544b4a91fcd3e9c0cfd700ffd915956103b39a546a4c724f8ef4b4b3b9ec117c` | 87,545,344 |
| 积分服务加固候选 | `ghcr.io/hxly520/sub2api-points:0.1.168-339422728b2c` | `sha256:bd04ab7ddf53c33625faef35d34ac1e379a9bf09e2de94ab92711680429ac09d` | `sha256:40c35815f6cbc575d871452fbc858f9ab93e647e8ad0df65fc3c1de7aa1046f3` | `168a1ec1bff3e908f611da0eb3bebc30846fccd213f7dec8ca0ecc8f977722bf` | 12,605,440 |

Sub2API 候选的 `-version` 输出为 `0.1.168`、完整 revision `339422728...` 和 release build；积分候选以非 root `65532:65532` 运行，容器内二进制 SHA256 为 `a0ead5e3721e13e59f365f423b82d4a15d94d85e0231c946097eb036fa7fec5e`，与本地交叉编译产物一致。生产 Compose 和两个运行容器仍引用 `0.1.168-2ad2815e`；两者均 healthy、restart count `0`，本次仅增加服务器缓存镜像，没有替换或重启服务。

### 2.2 中文双工作区与关闭态二次校验发布

仓库 `main` 于 `2026-07-30` 更新到 `c0fe91506bca60dfcc96b6d868b48b30d2ca86f0`。积分用户端与管理员端改为独立中文 HTML/JS：用户端只显示总积分、昨日积分、昨日消费、今日/累计签到赠送、7/30/90 日曲线和个人记录；管理员端提供策略、手工赠送、快照与任务管理。普通用户不能下载管理员脚本或调用管理员 API。积分服务还在用户 ticket、页面、静态资源和 API 四层校验当前生效 policy，disabled 状态下旧会话和手工 URL 也不能开放用户功能。

| 制品 | 不可变 tag | registry digest | image ID | archive SHA256 | bytes |
| --- | --- | --- | --- | --- | ---: |
| 积分中文双工作区 | `ghcr.io/hxly520/sub2api-points:0.1.168-c0fe91506bca` | `sha256:8a9b7f51ce454450fc797aeeb7bfea008351cdba354327ae6cf40d3ddbdb4148` | `sha256:a6a899da83a5e82eccb9bd8c2473b72c274d32d6c78e8d50774654f21dfd166b` | `d8bed76bd257e4ecb3e72dddb5e26c11147274738a3e7e316e2015c85568ef7d` | 12,584,448 |

本地使用 Go `1.26.5`、`CGO_ENABLED=0` 交叉编译，归档内二进制 SHA256 为 `9a2af6a95e025bca9c59fae064bb8e55148ed171b43b46c16948b77eaaff9f47`，与本地产物一致；运行用户 `65532:65532`、入口、healthcheck、两层镜像结构和 OCI labels 均已重新读取验证。两个 GHCR tag `0.1.168-c0fe91506bca`、`sha-c0fe91506bca` 回读为同一 digest。服务器未登录私有 GHCR，因此使用 `0600 root:root` 归档 `/home/api/sub2api-points/releases/sub2api-points-0.1.168-c0fe91506bca.tar` 执行 `docker load`，没有在服务器构建。

切换前备份位于 `/home/api/sub2api-deploy/backups/points-ui-c0fe91506bca-20260730-190702/`：原 Compose SHA256 为 `76f254f9c18fabbb34cb831a152e128231ead1f07be08befe8869893b72bba2c`，`points` schema custom dump SHA256 为 `b9b0e477267b6710a8a3df8cb311f5f5707c9f1779cf265a47623e2ce98b53dd`。只执行 `docker compose up -d --no-deps points-system`；Sub2API 容器 ID、启动时间和镜像引用前后完全一致。

未登录公开首页没有随 Sub2API 镜像变化，是因为 Nginx exact root 读取宿主文件。`2026-07-30 16:19 CST` 已把仓库 `deploy/public-landing/index.html` 原子发布到 `/home/api/sub2api-deploy/public/index.html`，宿主文件与 Nginx 响应 SHA256 均为 `86eb43d94050780fd9dc81da6e189c469f709bffa11c63eed195cdc065d229e5`；旧文件备份为 `index.html.bak-20260730081909`。该操作未重启 Nginx 或 Sub2API。

## 3. 生产 Sub2API 手工切换与后续边界

候选载入阶段没有替换生产 Sub2API；维护者先在 `2026-07-30 07:36 CST` 切换首个 `2ad2815e` 镜像，后于 `15:10 CST` 手工切换到当前 `339422728` 加固镜像。当前只读复核结果：

- 容器 ID：`bfd162bb93806652614c2a59528dfef1d11788c6114308790fa04d6956039d87`。
- 运行镜像 ID：`sha256:a8abdd92bb9f59082d9bbecbdf4812b8f1a441aed4f189bc674104f07074625b`。
- 运行 tag：`ghcr.io/hxly520/sub2api:0.1.168-339422728b2c`，OCI revision `339422728b2ceb87b4a81bb08229d370c4ca589d`，registry digest `sha256:d50f01b1344763616e8198a23107e5f37d815460a5feae902c5bf447cf069f99`。
- 启动时间：`2026-07-30 15:10:23 CST`；当前 healthy、restart count `0`。
- `points_balance_credits` 已随 Sub2API 迁移创建且当前为 0 行。bridge 配置已注入但保持 `POINTS_SYSTEM_ENABLED=false`，公开设置为 `points_system_enabled=false`，因此普通用户菜单和余额发放桥接均不开放。
- `2026-07-30 19:07 CST` 切换积分镜像时，本容器 ID、启动时间和镜像引用前后完全一致；自动化仍不得替换或重启 Sub2API。

服务器安装的是 Docker Compose v2.27.1 插件，并保留 `/usr/local/bin/docker-compose` 兼容入口。`2026-07-30 08:06 CST` 已先备份原 Compose 到 `backups/docker-compose.before-points-bridge-20260730-080637.yml`（原 SHA256 `d6c6ee9c4e2b186bc130b7df31c11da4d2b9f7c8d7d06a6e7e5841719c6be301`），再只加入 root-only `/home/api/sub2api-points/bridge-secrets.env` 的 `env_file` 引用；新 SHA256 为 `0826fc9b699949eb02aac13b19b7be592c21e420c425116ac97516245314f8de`，权限 `0600 root:root`。当时只运行 `docker compose config -q`，没有执行 `up`；维护者后续切换 `339422728` 时该配置已加载，密钥文件至今保持 `POINTS_SYSTEM_ENABLED=false`。

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
- Sub2API bridge 密钥记录：`/home/api/sub2api-points/bridge-secrets.env`，`0600 root:root`；当前 Sub2API 已加载该文件但保持 `POINTS_SYSTEM_ENABLED=false`。密钥不得由 API/UI 回显，也不得复制到 Git、工单、聊天或日志。
- 容器：`sub2api-points`，容器 ID `0a1b7d0b5fbfa8a76ecf6543f8ea0327d5a4a6add274c193451fffab892f9a07`。
- 镜像：`ghcr.io/hxly520/sub2api-points:0.1.168-c0fe91506bca`；image ID `sha256:a6a899da83a5e82eccb9bd8c2473b72c274d32d6c78e8d50774654f21dfd166b`；OCI revision `c0fe91506bca60dfcc96b6d868b48b30d2ca86f0`。
- 网络：`sub2api-deploy_sub2api-network`；主机只绑定 `127.0.0.1:8090`。
- 安全边界：非 root、read-only rootfs、`cap_drop: ALL`、`no-new-privileges`、PID limit 128、日志轮转、无 host bind mount。
- 当前 `healthy`、restart count `0`；本地 `/healthz=200`，公网根路径与 `/healthz=404`，无会话 `/app/`、`/admin/` 和 `/assets/app.css` 均为 `401`。
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

积分服务当前仍为默认 disabled policy，签到关闭，公开设置 `points_system_enabled=false`。已完成新版本管理员只读冒烟：管理员 ticket 交换 `303`、中文管理页和管理员脚本 `200`、身份为 admin、policy 1 只读且 disabled、CSRF 注销 `200`、注销后 `401`；用户 ticket 在交换阶段返回 `403 points_disabled`。冒烟前后 policy/account/ledger/checkin/grant 计数完全一致，没有创建策略、签到、积分账本或余额任务。

后续必须按顺序完成：

1. 继续保持 Sub2API bridge 与 policy 1 双重 disabled；管理员可调试，普通用户入口、ticket、页面、资源和 API 均不得开放。
2. 管理员后续创建的完整 policy 必须最早次日生效；初始 disabled policy 不得原地修改。确认比例、刷新分钟、签到模式、最低消费、阶梯和三层金额上限后，先完成安全验收。
3. 开启前用明确批准的最小金额验证一次 credit、同 UUID 重试、缓存同步、余额账本和 reversal；不得直接改用户余额。验证后保留审计并使净余额回到原值。
4. 安全验收通过后才可同时开启生效 policy 与 Sub2API bridge；只开其中一层不算正式开放。
5. 继续观察数据库连接、调度零值标记、outbox、容器 restart 和 Nginx 4xx/5xx，再决定是否把 reconcile window 从 1 恢复到 7。

积分服务回滚只停止 `/home/api/sub2api-points/compose.yml` 中的容器；不得删除 `points` schema、迁移、快照、审计、outbox、角色或首次部署备份。Sub2API 回滚继续由维护者在人工维护窗口使用已记录的上一稳定镜像处理，自动化不得替换或重启运行容器。
