# 私有 Release 与双更新运行手册

本文说明 `hxly520/sub2api` 的两种版本更新方式。默认不直接操作生产；服务器上的 Sub2API 镜像由维护者在人工窗口切换。所有命令中的主机、项目名、路径、版本和凭据均为占位符。

## 1. 当前基线（2026-08-11）

- 私有仓库：`hxly520/sub2api`，`main` 当前版本为 `0.1.172-52t.1`；首个私有 Release `v0.1.172-52t.1` 已建立 GHCR、二进制 updater 和宿主静态页面资产基线。
- 官方 `v0.1.172` 已完成私有兼容合并；官方 `v0.1.173`（tag commit `29009f0b2ea14edf3b11ae2564fb617ff91a03b4`）已读取但尚未合并到私有主线。它包含 Grok/xAI、被动渠道监控、音频/搜索计费和迁移 `194-220` 等大范围变化，不能直接热更新或直接覆盖私有代码。
- 私有版本 Tag 采用 `vX.Y.Z-52t.N`；同一官方基线内的 `N` 单调递增。Tag 必须是 annotated tag，不得在尚未合并官方版本时提前占用它的版本号。

## 2. 发布前门禁

### 2.1 代码与兼容审查

1. 从完整私有 `main` 建立 `codex/upgrade-vX.Y.Z-*`，记录官方 Release commit、merge-base 和旧生产 revision。
2. 阅读 [`PRIVATE_CUSTOMIZATION_CN.md`](PRIVATE_CUSTOMIZATION_CN.md)、[`OFFICIAL_COMPATIBILITY_HISTORY_CN.md`](OFFICIAL_COMPATIBILITY_HISTORY_CN.md) 和额度卡/积分文档；逐项标记官方新增路由、计费、迁移、配置、前端和容器差异。
3. 官方同类能力取官方实现，私有功能保留为兼容层；生成代码、Ent schema、Wire 和迁移按源码意图重新生成，不手工拼接。
4. 运行后端/积分/前端全量门禁和 `git diff --check`。未通过的测试不能进入 Tag。

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

本地可复核：

```bash
bash tools/release/classify-update.sh vX.Y.Z-52t.N > update-manifest.json
jq . update-manifest.json
bash tools/release/test-classify-update.sh
```

Manifest 的 `policy` 只有三种值：`hot-update-safe`、`image-update-recommended`、`image-update-required`。它必须随 Release 作为 `update-manifest.json` 上传；缺少 manifest 或 checksum 时，生产 updater fail-closed，不安装缓存资产。

## 3. GitHub Actions 发布

工作流 `.github/workflows/release.yml` 只接受私有 Tag。它在同一 Tag checkout，构建嵌入前端的 Linux/Windows/macOS 二进制和 Linux 多架构 GHCR 镜像，上传 `checksums.txt`、`update-manifest.json`、`public-landing-index.html`、`public-help-index.html` 及两份静态页各自的独立 SHA256，并保留完整 commit/revision。发布不使用 `latest` 作为回滚点。

GoReleaser 固定为工作流中记录的精确版本，不能改回浮动 `~> v2`。升级发布工具前必须在同一版本运行配置检查和隔离镜像构建；当前 `dockers`/`docker_manifests` 仍是已验证的多架构发布路径，GoReleaser 对它们的淘汰提示是已知待办，不得在没有 GHCR 双架构验证时直接迁移到 `dockers_v2`。

推荐人工批准 Tag 发布；不要让每个 `main` 提交自动创建 Release。网络或 runner 失败时，先修复 CI，不要在生产服务器编译。

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

私有仓库 Release API 和 GHCR 拉取使用分离的只读凭据：`UPDATE_GITHUB_TOKEN` 只授予私有 Release/asset 读取；服务器 Docker 登录只授予 GHCR `read:packages`。Token 不写入 Compose、日志、命令参数或本仓库。

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
