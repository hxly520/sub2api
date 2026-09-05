# 52Token 二开版本发布与升级运行手册

> 本手册只描述当前源码和可复核的发布动作，不记录生产密钥、数据库连接串、GHCR PAT 或用户数据。生产 Sub2API 容器始终由维护者手工切换；自动化只负责检查、构建和发布候选产物。

## 0. 当前候选

| 项目 | 状态 |
| --- | --- |
| 官方基线 | `v0.2.1`，commit `ab99d56e9626e6cd731592dae8553c9758a0efa2` |
| 私有候选 | `v0.2.1-52t.2`（最终 commit、Tag、Release、GHCR digest 在门禁完成前均为 pending） |
| 生产基线 | `v0.1.183-52t.4`；本轮不自动替换、重启或连接生产服务器 |
| 发布策略 | `image-update-required`：数据库迁移、Ent/生成代码、前端和容器基线必须随镜像交付 |
| 生产动作 | 维护者备份数据库和 Compose 回滚点后，手工 `docker compose pull`/`up`；本手册不执行切换 |

## 1. 保留范围与冲突规则

官方网关、协议转换、账号调度、重试、缓存、计费和错误处理以 `v0.2.1` 为准。只在官方入口接回以下产品契约：

- 同库积分/签到系统；
- 提链/额度卡账本、预扣/后扣费、欠费恢复和退款边界；
- 图片/视频任务的冻结、释放和核销；
- 统一媒体 API、视频模型兼容；
- 首字 Token/TTFT 记录、Codex `x-codex-turn-state` 48 KiB 保护；
- 当前未登录首页、帮助页和独立 `points-system`/`infinite-canvas` 部署契约。

普通 API Key 页的旧 CCSwitch 自动导入和 KeyingPay V2 已恢复为官方状态；额度卡页面的 CCSwitch 只读教程仍保留。发生重复实现时先保留官方路径，再以最小适配补丁满足上述契约。

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
git commit -m "chore(release): prepare v0.2.1-52t.2"
git tag -a v0.2.1-52t.2 -m "private compatibility release v0.2.1-52t.2"
git push origin codex/upgrade-v0.2.1-compat
git push origin v0.2.1-52t.2
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
