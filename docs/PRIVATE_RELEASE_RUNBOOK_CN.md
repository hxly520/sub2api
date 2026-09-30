# 52Token 二开版本发布与升级运行手册

> 本手册只描述当前源码和可复核的发布动作，不记录生产密钥、数据库连接串、GHCR PAT 或用户数据。生产 Sub2API 容器始终由维护者手工切换；自动化只负责检查、构建和发布候选产物。

## 0. 当前候选

| 项目 | 状态 |
| --- | --- |
| 官方基线 | 当前升级目标 `v0.2.11`；peeled commit `96f4c115c9749078f90cbf210a01d39baf3f53b6`；官方 tag 内 `VERSION` 与 tag 名不一致，私有候选按 tag 设为 `0.2.11-52t.1` |
| 私有候选 | `v0.2.11-52t.1`；分支 `codex/upgrade-v0.2.11-compat`，起点 `3c990be5c8fe5b2105a7abfeed8a6f27679227a4`，merge-base `86f93c28ee34cc74b629dafb748bd5ac5ca8c5ea`；源码、PR、Release 与镜像待验证 |
| 运行基线 | 只读服务器核验为 `ghcr.io/hxly520/sub2api:0.2.5-52t.1`、healthy、restart count `0`；本轮未改容器或其他服务 |
| 发布策略 | `image-update-required`：数据库迁移、Ent/生成代码、前端和容器基线必须随镜像交付 |
| 生产动作 | 维护者备份数据库和 Compose 回滚点后，手工 `docker compose pull`/`up`；本手册不执行切换 |
| 验证与产物 | 本地/CI 门禁、PR、安全扫描、Release、GHCR 与服务器候选缓存均为 `pending`；不得把源码合并或构建过程写成生产上线 |

### 当前升级候选：v0.2.11-52t.1（未发布）

- 官方 Release 固定为 `v0.2.11`，peeled commit `96f4c115c9749078f90cbf210a01d39baf3f53b6`。官方 tag 树内的 `backend/cmd/server/VERSION` 仍为 `0.2.10`；私有候选版本文件改为 `0.2.11-52t.1`，以与候选 Tag 保持一致。
- 私有起点为 `3c990be5c8fe5b2105a7abfeed8a6f27679227a4`，merge-base 为官方 `v0.2.5` peeled commit `86f93c28ee34cc74b629dafb748bd5ac5ca8c5ea`。当前分支保留双亲 merge 谱系，未压平官方历史。
- 官方网关、协议转换、计费、重试、缓存、账号调度、管理功能与前端按 `v0.2.11` 为准；仅在官方入口保留本手册列明的积分/签到、额度卡账本、媒体冻结/释放/核销、统一媒体与视频、TTFT、Codex 48 KiB 防护、KeyingPay V2、当前首页/帮助页及 Logo 等私有产品契约。
- 私有 `points-system/`、额度卡实现与迁移、KeyingPay provider、首页和帮助页静态资源均保留。官方新增迁移为 `238b_content_moderation_engine_meta.sql`、`239_channel_reasoning_effort_multipliers.sql`、`240_affiliate_ledger_operation_id.sql`；私有迁移不重命名、不覆盖，按完整文件名和 checksum 执行。
- 官方新增 Seedance 路径维持官方请求后计费，不接入私有媒体预冻结。现有图片及视频任务仍保留既有余额冻结、明确失败释放与成功核销语义；未知终态不擅自退款。
- 后端核心兼容适配、全量门禁和 Codex 48 KiB 边界回归尚未完成时，不创建 Release 或推送 GHCR 镜像。发布策略为 `image-update-required`；只发布 Sub2API 镜像，生产切换由维护者手动执行。
- SSH 只读核验的运行版本为 `0.2.5-52t.1`，服务 healthy、重启数为 0；当前候选尚未上传服务器或替换容器。积分、生图工作台、额度卡独立服务、数据库、Redis 与 Nginx 不在本轮部署范围。

### 上一私有发布：v0.2.5-52t.1

- PR [`#13`](https://github.com/hxly520/sub2api/pull/13) 已合入 `main`；merge commit、annotated Tag peeled commit 和 amd64/arm64 OCI revision 均为 `58d2b2f85f34fc11c103437b21945dae248ffe2a`。
- Release workflow [`35237673572`](https://github.com/hxly520/sub2api/actions/runs/35237673572) 成功；Release [`v0.2.5-52t.1`](https://github.com/hxly520/sub2api/releases/tag/v0.2.5-52t.1) 包含五个平台归档和 `checksums.txt`，归档 digest 与校验文件一致。
- 镜像为 `ghcr.io/hxly520/sub2api:0.2.5-52t.1`；多架构 manifest 为 `sha256:c6c6f534fa1aa6d2e396961b0351e3ee95c2bc549fb2da5b3b878653498d1f26`，amd64 为 `sha256:10502066ddb8880fde154cd026e265800c2207aafe3af19c5281609fab05ab1d`，arm64 为 `sha256:5487351ede4bc029d7e6efc2dc3f044d228b425252728f91a3014e2b2c968b05`。
- `2026-09-17` 服务器只执行该不可变 Tag 的 `docker pull`。缓存的 amd64 image ID 为 `sha256:d674f0ceca50015dd69787312bc01664696e1a2f786b107dd18301884af6cb9b`，RepoDigest 与上述多架构 manifest 一致，创建时间为 `2026-09-17T15:26:39.538188747Z`。
- 拉取前后生产 Sub2API 容器均为 `c3458bdc8ce1...`，镜像仍为 `0.2.1-52t.3`，旧 image ID `sha256:4da29aaf61487f94747ab2865ab5aaf2c23ead8ae0a28daf6764456510af12b6`，启动时间 `2026-09-07T23:35:28.690785751Z`，状态 healthy、restart count `0`；积分、生图工作台、PostgreSQL 和 Redis 容器也未改变。
- 本版本包含 forward-only 迁移与前后端跨度，继续分类为 `image-update-required`。当前代码采用官方在线更新器且本 Release 不提供私有 `update-manifest.json`，因此不得通过后台二进制热更新安装；数据库备份、人工 Compose 切换和生产冒烟仍由维护者完成。


### 上一私有发布：KeyingPay V2 v0.2.1-52t.3

- Main release commit: `15f24da8b0ed607a864dd4cc81f24bc2ee15b4d8`.
- Scope: restore KeyingPay V2 provider, admin configuration, signed callbacks, query, refund/refund-query, close, and frontend entry only; points, link cards, media, gateway, billing, retry, and home modules are unchanged.
- PR CI: https://github.com/hxly520/sub2api/actions/runs/34147361923 (success).
- PR security scan: https://github.com/hxly520/sub2api/actions/runs/34147361869 (success).
- Release workflow: https://github.com/hxly520/sub2api/actions/runs/34148161221 (success).
- GHCR: `ghcr.io/hxly520/sub2api:0.2.1-52t.3`; manifest `sha256:f27bbe666ae8cb582adc6755c27b29ce2d4ba95dc0156546788b1d236ce90ff0`; amd64 `sha256:6b229be0feb37e25df1f9fbcdb94b00ededc7a8456e92e16eb1b51c1868ab736`; arm64 `sha256:0771d5278face508e2e1ee37f73604cbec10bff36422721c97832ed53c0a8924`.
- Production: server, database, and running containers remain unchanged; the maintainer performs the manual image switch.

## 1. 保留范围与冲突规则

当前升级的官方网关、协议转换、账号调度、重试、缓存、计费和错误处理以 `v0.2.11` 为准。只在官方入口接回以下产品契约：

- 同库积分/签到系统；
- 提链/额度卡账本、预扣/后扣费、欠费恢复和退款边界；
- 图片/视频任务的冻结、释放和核销；
- 统一媒体 API、视频模型兼容；
- 首字 Token/TTFT 记录、Codex `x-codex-turn-state` 48 KiB 保护；
- 当前未登录首页、帮助页和独立 `points-system`/`infinite-canvas` 部署契约。

普通 API Key 页的旧 CCSwitch 自动导入仍为官方实现；KeyingPay V2（可盈Pay）仅作为支付 provider 最小适配恢复，额度卡页面的 CCSwitch 只读教程仍保留。发生重复实现时先保留官方路径，再以最小适配补丁满足上述契约。

## 2. 发布前门禁

在仓库根目录执行：

```powershell
Push-Location backend
go test ./... -run '^$' -count=1
go test ./internal/service -count=1
go test ./internal/handler ./internal/repository ./internal/server/... -count=1
go test -tags=unit ./internal/service ./internal/server/... -count=1
go vet ./...
Pop-Location

pnpm --dir frontend lint:check
pnpm --dir frontend typecheck
pnpm --dir frontend test:run
pnpm --dir frontend build

Push-Location points-system
go test ./... -count=1
go vet ./...
go build ./cmd/server
Pop-Location

git diff --check
```

重点回归：GPT-5.6 长上下文双倍计费、媒体一次提交与失败释放、额度卡账本/并发/欠费恢复、视频统一接口、TTFT 开关和失败 attempt 隔离、Codex 头部上限，以及首页/帮助页静态资源。

## 3. 生成候选 Tag 与 GitHub Actions 镜像

1. 在升级分支确认工作树干净、官方 commit 和 `backend/cmd/server/VERSION` 已记录。版本文件必须已经等于 Tag 去掉前导 `v` 后的完整值；工作流不会代替源码改写版本。
2. 提交候选变更并创建 annotated tag；Tag 名称只允许 `vX.Y.Z-52t.N`，例如：

```bash
git add -A
git diff --cached --check
git commit -m "merge: official v0.2.5 with 52Token compatibility"
git push origin codex/upgrade-v0.2.5-compat
# PR/审查后把候选合入私有 main，再在该 main commit 上创建 annotated Tag
git tag -a v0.2.5-52t.1 -m "private compatibility release v0.2.5-52t.1 [image-update-required]"
git push origin main
git push origin v0.2.5-52t.1
```

3. `.github/workflows/release.yml` 只接受现存的私有 Tag。它先确认名称格式、annotated Tag、精确 checkout commit、Tag 树内 `VERSION` 和默认分支祖先关系，再以同一个 `refs/tags/<tag>` 调用 `backend-ci.yml` 与 `security-scan.yml`。任何校验、质量或安全门禁失败都会阻止 GoReleaser 和镜像发布。
4. 质量门禁通过后才从同一 Tag 构建前端，并由固定版本 GoReleaser `v2.17.1` 构建 Linux 多架构 GHCR 镜像、各平台二进制和 checksums。正式候选应保持 `SIMPLE_RELEASE=false`，镜像仓库按当前 GitHub 仓库 owner 发布，即 `ghcr.io/hxly520/sub2api:<tag-without-v>`。
5. 发布流程不提交代码、不推进默认分支，也不单独同步 `VERSION`；候选源码何时合并到 `main` 由维护者按正常 Git 审查流程决定。
6. 记录 workflow run URL、Tag commit、`backend/cmd/server/VERSION`、GHCR 多架构 manifest digest 和 OCI revision。不要把 `latest` 当作候选或回滚标识。
7. 只有源码门禁、Actions、Release 资产和 digest 全部复核后，才把文档中的 pending 改为完成。Release 成功也不代表生产已切换。

## 4. 维护者手工 Compose 切换

服务器不构建源码或镜像，发布工作流也不连接生产服务器。只有维护者在备份和回滚点确认后的维护窗口手工执行（示例，先替换为已核验的 tag/digest）：

```bash
cd /path/to/sub2api
git diff -- deploy/docker-compose.yml
docker compose config --quiet
# 备份数据库、.env、当前镜像 ID 和回滚 tag
docker compose pull sub2api
docker compose up -d --no-deps sub2api
```

切换后检查 `/health`、登录、原有 API、积分、额度卡、媒体/视频和首页/帮助页；确认无误再清理旧镜像。积分服务和 `infinite-canvas` 容器不随 Sub2API 镜像自动替换。

## 5. 回滚

优先使用上一个已核验的不可变 tag/digest，恢复 Compose 后执行 `docker compose up -d --no-deps sub2api`，再按同一健康检查矩阵验收。数据库迁移为 forward-only 时，不得把“回滚镜像”当作“回滚数据库”；先按迁移文档制定数据方案。

## 6. 后续版本接替规则

每次官方升级都从当前私有主线创建 `codex/upgrade-vX.Y.Z-*` 分支，先 fetch 官方 Release，再按本手册的保留清单逐项审查。禁止用官方 `main/latest` 覆盖私有主线，禁止按迁移数字前缀覆盖同号文件，禁止在未完成门禁时上传或切换生产镜像。
