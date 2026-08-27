# 52Token 二开 Release 与双更新运行手册

本文说明 `hxly520/sub2api` 的两种版本更新方式。默认不直接操作生产；服务器上的 Sub2API 镜像由维护者在人工窗口切换。所有命令中的主机、项目名、路径、版本和凭据均为占位符。

## 1. 当前基线（2026-08-28）

- 52Token 二开仓库：`hxly520/sub2api`，长期公开以使用公共 Actions runner；生产配置、凭据和请求数据不得入库。工作分支 `codex/upgrade-v0.1.183-compat` 从二开起点 `ceb2326d740235852d9d81bbca6bee669a342130` 合入官方 `v0.1.183`，双父合并节点为 `e973f23ad474586cb607b8c6b4b6a1fa5c60c60c`，当前候选版本为 `0.1.183-52t.4`。发布提交与 annotated Tag 均为 `b21d92c5239a2aabd47d867e3b3bbb311d2b4272`；GitHub run `33105459243`、Release、manifest、GHCR 双架构镜像和服务器缓存均已完成，manifest digest 为 `sha256:02ae7c6248110ddb862358701fb912202da9429ec5a535a8918c1a9bf7bf95bf`。上一 `.2` 的提交、run、digest 和服务器缓存只作为历史证据保留。`v0.1.183-52t.3` 指向 `aa6a0d05e5a17e355723672a6cddfd5de2d0dc92`，GitHub run `33103107425` 因 8 个已脱离官方链路的旧二开未使用函数触发 lint 门禁而失败，未生成 Release 或 GHCR 镜像，禁止重跑或改写该 Tag。生产仍运行二开 `v0.1.176-52t.1`。候选源码、Tag、Release、GHCR 镜像、服务器缓存和生产切换必须分别记录，运行态只以维护者和 [`PRODUCTION_OPERATIONS_CN.md`](PRODUCTION_OPERATIONS_CN.md) 的证据为准。
- 官方 `v0.1.183` annotated tag object 为 `c21fd3382a1c39fe491a96ac6780bac927327ae4`，peeled commit 为 `e8cb019fabf8b55199436229044cbf9aa7a82564`，tag 树内 `VERSION=0.1.182`；当前私有源码 `VERSION=0.1.183-52t.4`。本轮官方网关、计费、重试和协议终态按官方实现优先恢复；保留私有积分、提链/额度卡、媒体冻结、生图/视频、长上下文计费、首页/帮助、私有更新源和精确容量兼容约束。客户端断开禁止重放，仅做有界 usage drain；OAuth 429 恢复窗口受请求级重试预算约束；Codex turn-state 上限 `48 KiB`，Nginx 响应头缓冲为 `128k`。
- 官方新增 `222-230` 迁移；两份 `225`、两份 `226` 与此前三份 `194` 均按完整文件名和 checksum 共存。候选跨越 forward-only 迁移、Ent/生成代码、前端和二进制，发布策略固定为 `image-update-required`，后台热更新不得安装。
- 私有版本 Tag 采用 `vX.Y.Z-52t.N`；同一官方基线内的 `N` 单调递增。Tag 必须是 annotated tag，不得在尚未合并官方版本时提前占用它的版本号。

## 2. 发布前门禁

### 2.1 代码与兼容审查

1. 从完整私有 `main` 建立 `codex/upgrade-vX.Y.Z-*`，记录官方 Release commit、merge-base 和旧生产 revision。
2. 阅读 [`PRIVATE_CUSTOMIZATION_CN.md`](PRIVATE_CUSTOMIZATION_CN.md)、[`OFFICIAL_COMPATIBILITY_HISTORY_CN.md`](OFFICIAL_COMPATIBILITY_HISTORY_CN.md) 和额度卡/积分文档；逐项标记官方新增路由、计费、迁移、配置、前端和容器差异。
3. 官方同类能力取官方实现，私有功能保留为兼容层；生成代码、Ent schema、Wire 和迁移按源码意图重新生成，不手工拼接。
4. 运行后端/积分/前端全量门禁和 `git diff --check`。未通过的测试不能进入 Tag。
5. 私有 Release 工作流会再次调用完整 CI；只有脚本、后端单元/集成、前端、积分、视频边缘 Worker 和 `golangci-lint` 全部成功，才允许构建二进制与镜像。不得绕过该门禁手工补发同一失败 Tag。

`v0.1.183-52t.4` 还必须定向验证以下矩阵后才能创建 Tag：API Key `v21` 快照 JSON/L1/L2 往返；GPT-5.6 在账号 `openai_long_context_billing_enabled=true`、渠道无显式区间价、未缓存输入加 cache write/read 严格大于 `272000` 时，输入及两类缓存 `2x`、输出 `1.5x`、分组倍率只乘一次且日志为 `long_context_billing_applied=true`；该规则须覆盖 HTTP Responses、WebSocket HTTP bridge 和 WebSocket v2。其余门禁包括长上下文分组/账号 OR 开关、渠道显式区间不重复计费、Fast/Flex 实际 service tier、分时时段和工作日、提链资金守恒与欠费准入、媒体冻结释放/核销、统一视频路由、精确容量拒绝有界重试、客户端断开零重放、OAuth 429 请求级预算、turn-state 48 KiB/Nginx 128k、积分桥接、首页/帮助和用户/管理员导航。定向测试不能替代后端、积分和前端全量门禁。

### 2.2 Tag 与分类

```bash
git fetch --tags --force upstream
git switch -c codex/upgrade-vX.Y.Z-compat origin/main
# 完成审查、测试和文档后：
git tag -a vX.Y.Z-52t.N -m "private release X.Y.Z-52t.N\n\n[image-update-required]"
git push origin codex/upgrade-vX.Y.Z-compat
git push origin vX.Y.Z-52t.N
```

不要把 `[migration-hot-update-safe]` 当作自动批准。只有迁移经过向后兼容审查、旧二进制仍能读取新结构、回退边界已记录时，才把标记放入 Tag message。分类器也会把上一私有 Tag 不是当前 Tag 祖先的分叉发布强制标为 Compose。

候选 Tag 必须从当前默认分支后继历史创建，或已经被默认分支以同一版本包含。Release 在质量门禁前校验双向祖先关系，并要求 Tag 树中的 `backend/cmd/server/VERSION` 与 Tag 完全一致；不允许在构建时临时改版本，也不会在发布后自动推进默认分支。`main` 始终代表维护者已经确认切换的生产版本，候选只通过兼容分支、不可变 Tag、Release 和镜像保存；维护者完成生产切换与冒烟后，再把 `main` 正常快进到该 Tag。默认分支与 Tag 分叉或旧 Tag 重跑会造成降级时，发布失败关闭且不强推。

本地可复核：

```bash
bash tools/release/classify-update.sh vX.Y.Z-52t.N > update-manifest.json
jq . update-manifest.json
bash tools/release/test-classify-update.sh
```

Manifest 的 `policy` 只有三种值：`hot-update-safe`、`image-update-recommended`、`image-update-required`。它必须随 Release 作为 `update-manifest.json` 上传；缺少 manifest 或 checksum 时，生产 updater fail-closed，不安装缓存资产。

本轮 `v0.1.183-52t.4` 必须使用上例的 `[image-update-required]` 标记，分类结果必须为 `image-update-required`。即使二进制、前端和所有测试通过，也不能改用后台热更新，因为官方 `222-230` 迁移及容器构建基线必须随不可变镜像交付。

## 3. GitHub Actions 发布

工作流 `.github/workflows/release.yml` 只接受 `vX.Y.Z-52t.N` 二开 Tag。它在同一 Tag checkout，先复用 `backend-ci.yml` 与 `security-scan.yml` 完成质量和安全门禁，再构建嵌入前端的 Linux/Windows/macOS 二进制和 Linux 多架构 GHCR 镜像，上传 `checksums.txt`、`update-manifest.json`、`public-landing-index.html`、`public-help-index.html` 及两份静态页各自的独立 SHA256，并保留完整 commit/revision。独立 CI/安全扫描的 `push` 只接受 `main`，因此候选分支或二开 Tag 不会额外启动重复流水线；Tag push 只触发一次 Release，普通 PR 仍执行两套独立门禁。失败时优先对同一不可变 Tag 的 run 执行 `Re-run`。由于 Actions UI 的手工触发默认读取默认分支上的工作流定义，本轮 `main` 尚未快进时不得从 UI 直接 dispatch；确需手工触发必须使用 `gh workflow run release.yml --ref vX.Y.Z-52t.N -f tag=vX.Y.Z-52t.N`，确保工作流定义、`checkout_ref`、门禁和构建均来自同一 Tag。发布不使用 `latest` 作为回滚点。

Go 构建基线同时约束 `backend/go.mod` 与 `points-system/go.mod`，当前均为 `1.27.0`。四个 Go Dockerfile 必须一并审查：仓库根 `Dockerfile`、`deploy/Dockerfile`、`backend/Dockerfile` 使用仓库根上下文，`points-system/Dockerfile` 由 `points-image.yml` 以 `points-system/` 为独立上下文构建。改变 Go 版本或复制路径时，不能只验证主镜像而漏掉积分镜像。

GoReleaser 固定为工作流中记录的精确版本，不能改回浮动 `~> v2`。它会同时推送精确版本、主/次版本和 `latest` 别名；生产和回滚只能使用精确私有 Tag 或 digest，不能把可变别名当作不可变产物。升级发布工具前必须在同一版本运行配置检查和隔离镜像构建；当前 `dockers`/`docker_manifests` 仍是已验证的多架构发布路径，GoReleaser 对它们的淘汰提示是已知待办，不得在没有 GHCR 双架构验证时直接迁移到 `dockers_v2`。

推荐人工批准 Tag 发布；不要让每个 `main` 提交自动创建 Release。GoReleaser 创建 Release 后才上传更新 manifest 和两份静态页；若后续上传失败，Release 可能暂时缺少这些资产，updater 会按 fail-closed 处理，需在发布日志中补传并复核完整性。网络或 runner 失败时，先修复 CI，不要在生产服务器编译。

`2026-08-27` 已完成全历史凭据审计并按维护者授权将仓库长期设为公开，Release 使用公共 runner，不再依赖私有仓库 Actions 分钟。禁止仅为一次构建反复切换可见性；若公开 runner 未分配，先记录 run ID 和状态，再对同一不可变 Tag 重跑。确需本机构建时，按本手册同等执行全量门禁、标准镜像归档、revision/SHA256/digest 回读和服务器仅导入边界，不能把“工作流未启动”写成 CI 通过。

发布后只读核对：

```bash
gh release view vX.Y.Z-52t.N --repo hxly520/sub2api
gh release download vX.Y.Z-52t.N --repo hxly520/sub2api --pattern 'checksums.txt' --pattern 'update-manifest.json' --pattern 'public-landing-index.html*' --pattern 'public-help-index.html*'
sha256sum checksums.txt update-manifest.json
sha256sum --check public-landing-index.html.sha256
sha256sum --check public-help-index.html.sha256
jq -e --arg v 'X.Y.Z-52t.N' '.schema_version == 1 and .version == $v and (.policy != "")' update-manifest.json
docker buildx imagetools inspect ghcr.io/hxly520/sub2api:X.Y.Z-52t.N
```

公开 Release/asset 的读取不强制配置 `UPDATE_GITHUB_TOKEN`；需要提高 GitHub API 限额时可使用只读 token。GHCR 包若保持私有，服务器 Docker 登录只授予 `read:packages`。两类 Token 都不得写入 Compose、日志、命令参数或本仓库。

## 4. 方式 A：后台二进制热更新

仅限 Linux，且仅当 manifest 为 `hot-update-safe` 或维护者明确接受 `image-update-recommended` 时使用。容器内二进制必须位于可写的 `/app/sub2api`，且进程以能够在同目录创建 `.backup`、临时目录和 `.update-pending.json` 的运行身份执行；首个私有 Release 必须先通过 Compose 建立这个基线。Windows/macOS Release 仍可下载或手工部署，但后台必须显示为非一键热更新，不能伪装成可自动重启。

后台流程为：

1. `/admin/system/check-updates` 强制从私有仓库读取 Release、manifest 和 checksum；仓库、通道、系统/架构纳入 Redis 缓存命名空间。
2. 下载 URL 限定 GitHub API/Release 域名，私有 asset 使用 Authorization；重定向跨域时不会转发 Authorization。
3. 校验 asset 大小、SHA256、归档路径，以及新二进制 `--version` 中的版本号与完整 revision 是否分别匹配 Tag 和 manifest，再原子替换当前二进制并保留 `.backup`。
4. 更新接口经管理员 step-up 2FA；服务发送中断信号走现有优雅关闭流程。Supervisor/Compose 允许短暂重启，不应并行启动第二个实例。
5. 新进程首次启动写入待确认标记，最多 90 秒轮询本地 `/health`。健康后删除标记；失败则重启，下一次启动自动把 `.backup` 恢复并退出，交给 supervisor 启动旧版本。

热更新只替换二进制及其内嵌前端，不更新 Docker 基础层、`deploy/docker-entrypoint.sh`、Compose、Nginx、宿主 exact-root 页面、积分镜像或独立工作台。上述任一项变化都按 Compose/人工运维处理。

未登录首页必须随同一私有 Tag 验收，但宿主 exact-root 仍单独发布。先从该 Tag 下载并校验 `public-landing-index.html`，备份宿主 `/home/api/sub2api-deploy/public/index.html`，再在同目录原子替换；最后核对 Release 资产、本地文件、宿主文件和线上 `/`、`/index.html` 的 SHA256 一致。此步骤不重启 Sub2API，也不能由后台二进制热更新代替。

帮助中心同样是宿主静态资产。先校验 `public-help-index.html.sha256`，备份宿主 `/home/api/sub2api-deploy/help/index.html`，再原子替换并核对线上 `/help/` 的 SHA256、标题、目录锚点、桌面和移动端布局。帮助页必须保持无 JavaScript、无表单、无密钥输入面，不得因为切换 Sub2API 镜像而误判为已更新。

热更新写入的是当前容器可写层，不会创建新镜像，也不会刷新镜像 OCI 创建时间或 digest。`docker restart` 会保留该层，但 `docker compose up --force-recreate`、删除容器或迁移主机会重新从 Compose 所指镜像创建，届时会回到旧镜像内的二进制。热更新验收通过后，维护者应把 `SUB2API_IMAGE`/Compose 引用预先钉到同一私有 Release tag 或 digest，并只运行 `docker compose config --quiet` 验证；不要在非切换窗口执行 `up`。这样后续计划内重建不会静默降回旧版本。

## 5. 方式 B：Docker Compose + GHCR

用于首个私有基线、`image-update-required`、数据库/容器/入口/Nginx/积分/外部静态资源变化，或热更新回滚风险无法证明时。服务器不构建镜像：

```bash
cd /srv/SUB2API_DIR
cp deploy/docker-compose.yml BACKUP_DIR/docker-compose.before-X.yml
cp .env BACKUP_DIR/env.before-X   # 权限和敏感值只保留在 root-only 目录
docker login ghcr.io               # 使用 GHCR read:packages 凭据
docker compose --env-file .env -f deploy/docker-compose.yml config --quiet
docker compose --env-file .env -f deploy/docker-compose.yml pull sub2api
docker image inspect ghcr.io/hxly520/sub2api:X.Y.Z-52t.N --format '{{.Id}}'
docker compose --env-file .env -f deploy/docker-compose.yml up -d --no-deps sub2api
```

实际切换前记录旧容器 ID、启动时间、health、restart count、镜像 tag/digest、数据库备份和迁移计数。切换后只核对目标服务 `/health`、日志、关键 API、Redis/PostgreSQL 连接和余额/任务状态；不要顺带改价格、账号、Nginx 或其他容器。若使用 digest，Compose 应固定 `ghcr.io/hxly520/sub2api@sha256:DIGEST`，禁止 `latest`。

对于独立 points 或 `infinite-canvas`，使用各自 Compose 服务和不可变镜像，先完成对应 schema/浏览器本地数据备份；更新它们不能重启 Sub2API。

`v0.1.183-52t.4` 的自动化边界止于发布并核验 `ghcr.io/hxly520/sub2api:0.1.183-52t.4` 的不可变 digest。GitHub run `33105459243` 已通过，Release 和服务器缓存已完成；manifest digest 为 `sha256:02ae7c6248110ddb862358701fb912202da9429ec5a535a8918c1a9bf7bf95bf`，服务器 amd64 image ID 为 `sha256:3a7d99a2655717de1fdc4952c2d78ec25bdb02f543820ce9a6b6e47f353f7518`，OCI revision 为 `b21d92c5239a2aabd47d867e3b3bbb311d2b4272`。未执行 `docker compose up`、替换容器、触发生产迁移或调整业务配置。维护者在人工窗口完成数据库备份、旧镜像回滚点和 migration filename/checksum 核对后，才执行单服务 Compose 切换。

## 6. 回退

### 6.1 热更新失败自动回退

不要删除 `.backup` 或 `.update-pending.json`。先读取服务日志确认 `/health` 超时和恢复动作；supervisor 应继续拉起恢复后的旧二进制。确认旧版本 `--version`、健康、数据库迁移兼容性和余额/任务状态后，才清理残留备份。若自动守卫报“backup unavailable”或重复启动失败，停止自动重试并人工处理。

### 6.2 Compose 回退

```bash
cd /srv/SUB2API_DIR
docker compose --env-file .env -f BACKUP_DIR/docker-compose.before-X.yml config --quiet
docker compose --env-file .env -f BACKUP_DIR/docker-compose.before-X.yml pull sub2api
docker compose --env-file .env -f BACKUP_DIR/docker-compose.before-X.yml up -d --no-deps sub2api
```

回退必须使用事先记录的不可变 digest。数据库迁移通常是 forward-only，切回旧镜像不会撤销 schema；若新迁移不向后兼容，先按独立数据库恢复演练处理，不能只改镜像。存在提链 Key 时先关闭入口、冻结/禁用提链 Key、等待在途请求归零并完成资金对账，禁止让不识别 `key_type` 的旧镜像接管活动 Key。

### 6.3 在线版本回退限制

后台列出的旧 Release 只代表存在二进制 asset，不代表迁移可回退。只有已审查为 `hot-update-safe` 且数据库向后兼容的版本才可使用在线二进制回退；`image-update-required`、含未回退迁移、容器/入口/Nginx/积分变化的版本必须通过 Compose 和人工数据方案回退。维护者应在使用在线回退前核对该目标 Release 的 manifest；若 API 未返回 manifest，按 Compose 处理。

## 7. 证据与交接

每次发布记录以下脱敏字段：私有 Tag、完整 source commit、manifest policy/reasons、二进制 checksum、GHCR tag/digest、服务器 archive SHA256、OCI revision、旧/新容器 ID 和启动时间、迁移计数、health 和回滚点。不要记录 token 原值、Key、余额明细或请求正文。发布完成后同步更新 [`OFFICIAL_COMPATIBILITY_HISTORY_CN.md`](OFFICIAL_COMPATIBILITY_HISTORY_CN.md) 和 [`PRIVATE_CUSTOMIZATION_CN.md`](PRIVATE_CUSTOMIZATION_CN.md)。
