# 52Token 二开版本发布与升级运行手册

> 本手册只描述当前源码和可复核的发布动作，不记录生产密钥、数据库连接串、GHCR PAT 或用户数据。生产 Sub2API 容器始终由维护者手工切换；自动化只负责检查、构建和发布候选产物。

> **候选源码边界（2026-10-24）**：提链/额度卡已从当前候选的运行时代码、路由、前端页面和部署入口移除；本次未切换生产容器，尚未核验生产运行态是否停用。下文旧版本的相关接口、配置、账本、验收和回滚步骤仅为历史或审计资料，不是当前候选的部署、启用或回滚指令。`194_link_cards.sql` 原文件、原 checksum 和既有迁移历史保持不变，当前候选迁移 runner 按完整文件名跳过它；不得借升级重新启用旧入口或把历史提链 Key 当作普通 Key 使用。

### 当前候选总览（2026-10-24）

- 当前工作树版本为 `0.2.18-52t.1`，官方升级目标为 `v0.2.18`；后端 `go.mod` 使用 Go `1.27.3`。当前测试、发布、镜像与缓存状态以 [v0.2.18 候选交接](RELEASE_V0_2_18_52T_1_CN.md) 的逐项证据为准，不沿用下方 v0.2.15 的历史全绿结论。
- 当前候选完全移除提链/额度卡产品运行时，只保留原 `194_link_cards.sql`、checksum、迁移历史和审计数据；历史 Key/媒体冻结分类须失败关闭，禁止回落普通余额。
- 用户于 2026-10-24 确认保留分组生图开关现状（true/false 均不变）。官方 `246_enable_gemini_image_generation_groups.sql` 文件和原 checksum 不变；runner 对该精确 filename/hash 不执行开启开关的 SQL，首次仅在同一事务记录原 filename/checksum，已有记录继续核验，未知 hash 失败关闭；其他迁移照常。新增兼容改动的最终精确提交/Tag CI 与隔离 PostgreSQL 验证待完成，不继承此前提交的全绿证据。
- 本次不切换或重启生产，不修改生产 PostgreSQL、Redis、Nginx 或其他容器。2026-10-24 已重新只读核验生产为 `0.2.15-52t.1`，healthy、restart 0；旧配置仍启用提链，尚未切换本候选，具体见本轮交接。


历史 v0.2.15 发布证据（不代替 2026-10-24 当前工作树验收）：前端 377 文件 / 2925 项 Vitest、lint/typecheck/build，后端编译/vet/unit/default 与私有重点回归，积分 test/vet/build 已通过。PR #16 已合入 main；本地与 CI/隔离 PostgreSQL 门禁、安全扫描和 Release 均通过。v0.2.15-52t.1 已发布并缓存服务器，生产仍为 0.2.11-52t.3，等待维护者人工切换。Release run 38060183370；镜像源码/Tag/OCI revision 8afa8d339eec95607ccca76386813e1874b53159；manifest sha256:bd08b5ba2f5c6ca527fff280c4b3b27bb93d4415a1f24f70a96172628f1ca948。为修复 CI 已识别的旧依赖漏洞，仅积分构建元数据更新为 Go 1.27.2、x/text 0.42.0 及其 x/sync 0.23.0；积分业务源码和生产容器不变，不发布积分镜像。最终构建/缓存证据见 `docs/RELEASE_V0_2_15_52T_1_CN.md`。

## 0. v0.2.15 已发布候选的历史记录（当时待人工生产切换）

本节仅记录旧候选与当时产物，不构成当前工作树已完成测试、已发布或已部署的证据。历史候选当时包含提链产品；当前候选已移除，仅保留审计隔离，不得重复执行旧版本 Tag 或启用指令。

| 项目 | 状态 |
| --- | --- |
| 官方基线 | 当前升级目标 `v0.2.15`；peeled commit `f2669c8cf62555cd92389b3f55920e9e6e7c6ff2`；官方 tag 内 `VERSION=0.2.14`，私有候选主动设为 `0.2.15-52t.1` |
| 私有候选 | `v0.2.15-52t.1`；分支 `codex/upgrade-v0.2.15-compat`，起点 `27f5e0c2ffa6ff50c37e9fb2d3bb6cc849aeaf1f`，merge-base 为官方 v0.2.11 `96f4c115c9749078f90cbf210a01d39baf3f53b6`；PR #16 已合入 main，源码/Tag/OCI revision `8afa8d339eec95607ccca76386813e1874b53159` |
| 运行基线 | `2026-10-10` 只读服务器核验为 `ghcr.io/hxly520/sub2api:0.2.11-52t.3`、healthy、restart count `0`；本轮未改容器或其他服务 |
| 工具链与构建 | Go `1.27.2`；沿用精确 annotated Tag、完整 GoReleaser/GHCR 发布流程，`SIMPLE_RELEASE=false` |
| 发布策略 | `image-update-required`：数据库迁移、Ent/生成代码、前端和容器基线必须随镜像交付 |
| 生产动作 | 维护者备份数据库和 Compose 回滚点后，手工 `docker compose pull`/`up`；本手册不执行切换 |
| 验证与产物 | 本地/CI/隔离 PostgreSQL、安全扫描、Release 与服务器缓存均已完成；Release run `38060183370`，manifest `sha256:bd08b5ba2f5c6ca527fff280c4b3b27bb93d4415a1f24f70a96172628f1ca948`；尚未生产切换 |

### 历史升级候选：v0.2.15-52t.1（已发布并缓存）

- 官方 Release 固定为 `v0.2.15`，peeled commit `f2669c8cf62555cd92389b3f55920e9e6e7c6ff2`。tag 内 `backend/cmd/server/VERSION=0.2.14`，私有候选主动设置 `0.2.15-52t.1`；构建和检查使用 Go `1.27.2`。
- 从私有 `main` `27f5e0c2ffa6ff50c37e9fb2d3bb6cc849aeaf1f` 合入；merge-base 为官方 v0.2.11 `96f4c115c9749078f90cbf210a01d39baf3f53b6`。分支 `codex/upgrade-v0.2.15-compat` 保留双亲 merge 谱系；兼容审查、测试与 PR #16 已完成，已合入 main。
- 当时保留积分/签到同库桥接、提链/额度卡账本及既有图片/视频冻结释放核销、统一媒体接口、TTFT、Codex 48 KiB 头部保护及 KeyingPay V2；官方网关、协议、重试、缓存、调度和通用计费采用 v0.2.15。Seedance 按用户确认继续官方后扣费。
- 新增 `241_add_payment_order_bonus_amount.sql`、`241_add_typesafe_platform.sql`、`242_drop_platform_check_constraints.sql`；双 `241` 必须按完整文件名独立执行。旧私有迁移原名和 checksum 不变。checksum 以 UTF-8 内容经 `strings.TrimSpace` 后计算 SHA256；已核验的 10 份私有迁移及 238b/239/240 共 13 份与生产记录匹配，不代表新增迁移已执行。
- 重点回归充值实付/赠送/折扣/到账/退款与 KeyingPay 幂等，普通 Key 删除后的官方余额结算与当时提链在途退款/结算（仅历史测试范围，不是当前失败关闭验证证据），WS 后续轮次价格刷新、图像输入费用及 Claude 计费。`kimi-k3` 零 usage 是单独待确认事项，本次升级不授权估计扣费或历史追扣。
- 本地测试、隔离数据库迁移/事务回归、PR、CI、安全扫描、Release、GHCR 与服务器候选缓存全部完成，详细证据见 `RELEASE_V0_2_15_52T_1_CN.md`。沿用 full GoReleaser/GHCR，不改用新的官方矩阵发布方式。
- 只读生产镜像为 `0.2.11-52t.3`，manifest digest `sha256:100f8eb357fb2bddba42fb7d3f6fa1e7cc50160180bc5b92f93006b852928d83`，image ID `sha256:41bf83907718fe708d41578fc632c54fb56f373e6262b2ad99ccc2ba4dd5d906`。只允许缓存核验后的候选；生产 Sub2API 由维护者手工切换，积分/生图工作台等其他容器及宿主首页/帮助静态资产不变。

### 上一生产基线：v0.2.11-52t.3

- `2026-10-10` 只读确认生产为 `ghcr.io/hxly520/sub2api:0.2.11-52t.3`，healthy、restart count `0`，启动时间 `2026-09-30T14:37:29.03656502Z`。
- 对应本轮私有起点 `27f5e0c2ffa6ff50c37e9fb2d3bb6cc849aeaf1f`。既有 Release 的完整构建证据应从精确 Tag/Actions 查询，不使用下述 `.1` 早期候选阶段证据代替 `.3`。

### 历史候选记录：v0.2.11-52t.1（当时未发布）

本段保留 v0.2.11 首轮候选的当时状态；其 pending 与生产版本描述不代表当前状态。现行候选和生产基线见上文。

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

当前升级的官方网关、协议转换、账号调度、重试、缓存、计费和错误处理以已批准的精确官方 Release 为准；本文件第 0 节的 `v0.2.15` 仅为历史。只在官方入口保留下列现行产品契约与历史数据隔离边界：

- 同库积分/签到系统；
- 历史提链数据只审计保留，旧 Key 在认证/缓存/结算链路失败关闭，禁止回落普通余额；
- 图片/视频任务的冻结、释放和核销；
- 统一媒体 API、视频模型兼容；
- 首字 Token/TTFT 记录、Codex `x-codex-turn-state` 48 KiB 保护；
- 当前未登录首页、帮助页和独立 `points-system`/`infinite-canvas` 部署契约。

普通 API Key 页的旧 CCSwitch 自动导入仍为官方实现；KeyingPay V2（可盈Pay）仅作为支付 provider 最小适配恢复，额度卡页面及其 CCSwitch 教程已从当前候选移除，仅在历史审计资料中保留。发生重复实现时先保留官方路径，再以最小适配补丁满足上述契约。

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

重点回归：GPT-5.6 长上下文双倍计费、媒体一次提交与失败释放、历史提链 Key 失败关闭与迁移审计、视频统一接口、TTFT 开关和失败 attempt 隔离、Codex 头部上限，以及首页/帮助页静态资源。

当前候选另须完成隔离 PostgreSQL 迁移/事务回归、KeyingPay 充值赠送与退款金额核验，以及历史提链 Key 不进入普通认证/缓存/余额结算的隔离验证。`194_link_cards.sql` 原文件、checksum 和已应用历史保持不变；当前 runner 必须精确跳过该文件，官方同号迁移照常处理。正式计费只接受真实 usage，不以“请求成功”推断缺失的 token 数。

迁移 246 配置保留须单独回归：已有开启和关闭值均不变、首次仅记录原 filename/hash、同事务失败无半记录、重复启动校验幂等、已有历史校验、未知 hash 拒绝，以及相邻官方迁移仍正常应用。246 原 checksum 为 `87267be7ab657173768989e2b0b105c5569c5930a7f1ed124f4cf81e5198297c`；不得通过改写原 SQL、宽泛跳过数字前缀或生产直接回写开关来实现。最终门禁须绑定包含该改动的精确提交/Tag。

## 3. 生成候选 Tag 与 GitHub Actions 镜像

1. 在升级分支确认工作树干净、官方 commit 和 `backend/cmd/server/VERSION` 已记录。版本文件必须已经等于 Tag 去掉前导 `v` 后的完整值；工作流不会代替源码改写版本。
2. 提交候选变更并创建 annotated tag；Tag 名称只允许 `vX.Y.Z-52t.N`。以下仅展示历史 v0.2.15 的命令结构，执行时必须替换为当前已核验候选分支、版本和 Tag，不得重建或覆盖已发布 Tag：

```bash
git add -A
git diff --cached --check
git commit -m "merge: official v0.2.15 with 52Token compatibility"
git push origin codex/upgrade-v0.2.15-compat
# PR/审查后把候选合入私有 main，再在该 main commit 上创建 annotated Tag
git tag -a v0.2.15-52t.1 -m "private compatibility release v0.2.15-52t.1 [image-update-required]"
git push origin main
git push origin v0.2.15-52t.1
```

3. `.github/workflows/release.yml` 只接受现存的私有 Tag。它先确认名称格式、annotated Tag、精确 checkout commit、Tag 树内 `VERSION` 和默认分支祖先关系，再以同一个 `refs/tags/<tag>` 调用 `backend-ci.yml` 与 `security-scan.yml`。任何校验、质量或安全门禁失败都会阻止 GoReleaser 和镜像发布。
4. 质量门禁通过后才从同一 Tag 构建前端，再由 GoReleaser 构建 Linux 多架构 GHCR 镜像、各平台二进制和 checksums。当前工作流实际使用 `~> v2` 范围，不能将其描述成固定 `v2.17.1`；发布时记录实际工具版本。正式候选应保持 `SIMPLE_RELEASE=false`，镜像仓库按当前 GitHub 仓库 owner 发布，即 `ghcr.io/hxly520/sub2api:<tag-without-v>`。
5. 发布流程不提交代码、不推进默认分支，也不单独同步 `VERSION`；候选源码何时合并到 `main` 由维护者按正常 Git 审查流程决定。
6. 记录 workflow run URL、Tag commit、`backend/cmd/server/VERSION`、GHCR 多架构 manifest digest 和 OCI revision。不要把 `latest` 当作候选或回滚标识。
7. 只有源码门禁、Actions、Release 资产和 digest 全部复核后，才把文档中的 pending 改为完成。Release 成功也不代表生产已切换。

## 4. 维护者手工 Compose 切换

服务器不构建源码或镜像，发布工作流也不连接生产服务器。只有维护者在备份和回滚点确认后的维护窗口手工执行（示例，先替换为已核验的 tag/digest）：

```bash
cd /home/api/sub2api-deploy
docker compose config --quiet
# 备份数据库、.env、当前镜像 ID 和回滚 tag
docker compose pull sub2api
docker compose up -d --no-deps sub2api
```

切换后检查 `/health`、登录、原有 API、积分、历史提链 Key 失败关闭、媒体/视频和首页/帮助页；对比分组生图开关 true/false 均未改变，并核验 246 的原 filename/checksum 记录；确认无误再清理旧镜像。积分服务和 `infinite-canvas` 容器不随 Sub2API 镜像自动替换。

## 5. 回滚

优先使用上一个已核验的不可变 tag/digest，恢复 Compose 后执行 `docker compose up -d --no-deps sub2api`，再按同一健康检查矩阵验收。数据库迁移为 forward-only 时，不得把“回滚镜像”当作“回滚数据库”；先按迁移文档制定数据方案。回滚目标也必须隔离历史提链 Key，不能重新开放旧入口或让普通 Key 计费接管旧 Key；不删除历史幂等/资金流水。

## 6. 后续版本接替规则

每次官方升级都从当前私有主线创建 `codex/upgrade-vX.Y.Z-*` 分支，先 fetch 官方 Release，再按本手册的保留清单逐项审查。禁止用官方 `main/latest` 覆盖私有主线，禁止按迁移数字前缀覆盖同号文件，禁止在未完成门禁时上传或切换生产镜像。
