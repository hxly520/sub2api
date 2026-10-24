# v0.2.18-52t.1 兼容升级与额度卡退役交接

本轮仅交付 Sub2API 候选镜像，生产容器由维护者手工替换。积分、生图工作台、PostgreSQL、Redis、Nginx 与宿主静态页面不由本任务替换、重启或修改。

**最终状态（2026-10-24 03:31 UTC）**：PR #17 已合入 main，`v0.2.18-52t.1` 已发布并缓存服务器。精确 Tag 的质量、安全、真实 PostgreSQL、五平台构建与双架构镜像回读均通过；生产仍为 `0.2.15-52t.1`，五个现有容器未替换或重启。生产切换及业务验收仍待维护者人工执行。

## 1. 精确源码与范围

| 项目 | 记录 |
| --- | --- |
| 官方稳定 Release | `v0.2.18`，2026-10-23 发布；2026-10-24 通过 GitHub API 重新核验 latest |
| 官方 annotated tag object | `de86961b627e48db75d61fff8c05535d44a00cbe` |
| 官方 peeled commit | `ac1b8f1271be0d674fa6f074403c1410f30a3a81` |
| 官方 Tag 树内 VERSION | `0.2.17`，私有版本主动对齐自己的 Tag |
| 私有起点 | `main` `6cd5468dff7b874647571b80f08002f51b8a9ea6` |
| merge-base | 官方 v0.2.15 `f2669c8cf62555cd92389b3f55920e9e6e7c6ff2` |
| 候选 | `codex/upgrade-v0.2.18-no-linkcards` / `0.2.18-52t.1` |
| 工具链 | Sub2API 与独立积分构建工具链 Go `1.27.3`；积分业务及模块依赖不变，不发布/替换积分镜像 |
| 发布策略 | `image-update-required`；精确 annotated Tag、同 Tag CI/安全门禁、5 平台归档与双架构 GHCR |

**保留**：同库积分/签到及签名幂等桥接、媒体余额冻结/失败释放/成功核销、统一媒体接口与视频、真实首字 Token/TTFT 优化、Codex `x-codex-turn-state` 单值 48 KiB 保护、KeyingPay V2、现有首页/帮助/Logo/导航。Seedance 继续官方请求后计费；不新增私有预冻结。

**全部退役**：Sub2API 提链中心、管理控制台、公共额度卡页、API、激活/会话、专属 Key、倍率换算、充值/退款/账本写入和独立额度卡部署模板。不再发布、启用或恢复额度卡产品；历史数据不删除、不自动退款。

**官方优先**：网关、协议、重试、缓存、通用计费和账号调度基于 v0.2.18，包括官方 Gemini 显式缓存、流式中断 usage 收口、Vertex 配额、分组国际化、调度快照和并发修复。不把缺失 usage 估算为计费，不历史追扣。

## 2. 退役资金隔离

- 历史 `194_link_cards.sql` 原文件、checksum `7a40799ddd3379acda1a3f704f110d81278a8d38705cd965325880996a8d23b4` 和数据库历史保持不变；当前 runner 按完整文件名跳过，fresh install 不创建退役表/配置。
- 历史非 standard Key 在加载和资金写入口失败关闭，包括软删除后的晚到结算；不能按普通 Key 扣创建者余额。认证快照由 v24 切换至 v25 使旧快照失效并自然回源，不清空 Redis。
- 新库没有 `key_type` / `funding_source` 列时仍支持普通 Key 与用户余额媒体冻结；已有库的历史独立资金 hold 不参加普通余额释放/核销/清理。分类读不新增迁移、不改历史行。
- 帮助页保留原主题、新手及积分等非额度卡章节，只删除退役产品的导航、卡片、教程和分销内容。首页保持不变。

## 3. 新增官方迁移

checksum 均为 UTF-8 内容经 `strings.TrimSpace` 后的 SHA256；已有同号文件不覆盖、改名或合并。

| 文件 | SHA256 |
| --- | --- |
| `242_group_i18n.sql` | `e81c6818cc806bc54365c993f4cec1634f16a311451faf5cb1a875a1358774cd` |
| `243_gemini_cached_contents.sql` | `2d345cff5c64b01e705f04ec972e5d5ec3d618fe71dc7d45bc80fdd38c6a89d5` |
| `244_gemini_cached_contents_channel_usage.sql` | `9072cb37752a23c3a29cc0330a69837f09c5102375ee053d7bccf14634d3da29` |
| `245_gemini_cached_contents_mutation_lease.sql` | `75434a7beeb8aad09b3339708ed520604d153e99c7fe680a376e8c958aadb1e8` |
| `246_enable_gemini_image_generation_groups.sql` | `87267be7ab657173768989e2b0b105c5569c5930a7f1ed124f4cf81e5198297c` |

官方迁移 246 原行为会开启已有 Gemini/Antigravity 分组生图，composite 不自动开启；只读核验生产有 1 个 Gemini 分组关闭、1 个开启。用户于 **2026-10-24 明确选择“保留现有状态”**，因此所有已有开关 true/false 均保持不变。

兼容方式：不修改官方 246 SQL 文件或上述 checksum；runner 只识别该完整 filename 与已审查 checksum，不执行其中的配置重写，首次处理仅在同一事务向 `schema_migrations` 记录原 filename/checksum。已记录的迁移仍核验原 checksum；该文件出现未知 checksum 时失败关闭，不回退执行 SQL。其他迁移（包括同号文件）照常处理。此为精确配置保留例外，不是人工修改生产数据库、回写开关或忽略全部迁移。最终兼容提交与正式 Tag 的隔离 PostgreSQL、质量及安全门禁均已通过，见下方精确证据；本任务没有执行生产迁移。迁移记录表示该私有版本已完成配置保留处理，不表示执行过官方 UPDATE。

## 4. 当前验证状态

| 门禁 | 状态及证据 |
| --- | --- |
| 后端 default 全量 | `go test -p 2 ./... -count=1`通过；最终退役/普通Key/媒体/迁移/长上下文/支付/积分/TTFT/头部重点unit回归与vet通过 |
| 后端 unit / vet / embed build | 本地 vet/embed build/版本冒烟通过；Windows 全进程内存阈值波动保留记录；代码提交 70f6aaf5e 的 Linux CI 完整 unit 通过，未放宽断言 |
| 前端 | lint:check、typecheck/生产 build 通过；完整Vitest最终382文件/2971项通过；补3项取消竞态回归，KeyUsageView测试卸载计时器清理修复后无uncaught |
| 独立积分 | test/vet 通过，`go build ./cmd/server` 通过；业务源码和运行容器未改 |
| 媒体退役屏障 | default/unit 定向通过；本机无 Docker 不计实测；代码提交 70f6aaf5e 的 CI 完整 PostgreSQL integration 通过，包含新旧可选字段与历史 Key/资金来源屏障 |
| 帮助页 | 静态契约 25/25、定向 ESLint 通过；9 个非额度卡章节与原 main 一致 |
| Release helper | 10 项通过、Windows POSIX 权限检查 1 项跳过；bash 语法检查通过 |
| 迁移 246 配置保留 | 本地 repository/migrations default、unit、vet 与独立审查通过；新增 PG 测试编译成功；最终兼容提交及精确 Tag 的真实 PostgreSQL integration 均通过，原 SQL/checksum 不变 |
| GitHub CI / 安全 / Release | 最终兼容提交 1f2b4158b：CI 40056021267、安全 40056021225 success；正式 Tag Release 40056966532 的 18/18 jobs success，含同 Tag 质量、安全与 full 构建 |
| 镜像 digest / OCI revision / 服务器缓存 | 双架构 manifest/config 身份和五平台资产校验通过；服务器缓存及隔离版本冒烟通过，五个现有容器前后身份完全一致，见第 6 节 |
| 生产切换 | 仅维护者人工执行；本任务不执行 |

Windows模型目录测试曾因同一测试连续调用时间戳重复而命中上一场景缓存；仅给fixture增加递增后缀隔离，定向重复50次与随后整套该项通过。未修改网关生产实现。内存阈值测试整套曾测得8,801,344 bytes高于8,388,608阈值，独立10次均通过；保留本地波动记录，最终代码提交的 Linux CI 完整 unit 已通过。

本地脱敏日志位于仓库外 `output/upgrade-plans/v218-*`。旧 v0.2.15 全绿证据不代替本轮门禁。

首轮 PR17 安全扫描 `40053058230` 的 backend/frontend 通过，points-security 检出 Go 标准库 `GO-2026-6809`（`crypto/x509@go1.27.2`，修复版 `go1.27.3`）。仅将积分 `go.mod` 与 Dockerfile 构建工具链同步至 1.27.3，不改业务、不更换依赖、不触发积分镜像发布或生产替换；修复后的本地与 CI 结果另行记录，旧失败不覆盖。

工具链补丁后本地明确输出 `go version go1.27.3 windows/amd64`，积分 `go test ./... -count=1`、`go vet ./...`、`go build ./cmd/server` 均通过。首轮 CI `40053058304` 最终 success；正式代码验证以补丁提交 `70f6aaf5e144286374a9b580928e394f1d1e6580` 的 CI `40053477408` 和安全扫描 `40053477431` 为准，两者全部 success。CI 包含完整后端 unit/integration、前端、积分 PostgreSQL 事务/并发、视频 worker、部署脚本及 golangci-lint；repository integration 28.033 秒，service unit 209.538 秒。后续纯文档提交不自动继承为新的精确 HEAD/Tag 门禁证据；正式发布仍须在最终 Tag 上重跑质量与安全门禁。

首轮安全补丁阶段日志留存于仓库外 `output/upgrade-plans/v218-ci-securityfix-success.log` 和 `v218-securityfix-success.log`。该阶段当时尚未发布；后续最终提交、Tag、镜像和服务器缓存已完成，以下最终证据覆盖前述阶段性 pending，不覆盖生产切换边界。

## 5. 生产只读核验（2026-10-24）

- SSH 密码认证正常；只执行 Docker 身份、磁盘与数据库只读事务检查，没有更改配置、服务或数据。
- Sub2API 当前 `ghcr.io/hxly520/sub2api:0.2.15-52t.1`，容器 `675cf9c85293...`，2026-10-17 启动，healthy、restart 0。
- 工作台仍 `infinite-canvas:v0.16.0-52t.1`；积分仍 `sub2api-points:0.1.169-b64a0110ab2c`；PostgreSQL 17.8 与 Redis 8 未变。
- 历史提链 Key 共 17：1 个 depleted/quota_exhausted、16 个 refunded/disabled，没有 active Key；在途储备合计为 0，未发现独立额度卡资金源媒体 hold。只保留聚合证据，不读取完整 Key。
- 旧生产配置仍为 `link_cards_enabled=true` / `development_mode=false`，因此**候选移除不等于现行生产已停用**；切换到候选才会移除应用入口和鉴权。未修改该设置或 Nginx。
- `194_link_cards.sql` 生产 checksum 与仓库一致；本轮 242_group_i18n 至 246 五项未应用。磁盘约 34 GiB、已用 18 GiB、可用 14 GiB。

## 6. 发布与人工切换

### 6.1 已发布身份及门禁

- PR [#17](https://github.com/hxly520/sub2api/pull/17) 已合入 main。发布源码、Tag peeled commit 和双平台 OCI revision：`d16c14b135fe8de25ce73850db51040c1cd8d679`。后续纯文档提交不是重新构建的镜像源码。
- annotated Tag：`v0.2.18-52t.1`；Tag object：`7b92303550f0b645a545f5a2e0a7771fe1396cdd`；Tag 内 VERSION：`0.2.18-52t.1`。
- 最终兼容提交 `1f2b4158bc3c7874b860d47f4cc18ccf3d10b400` 的 CI `40056021267` 和安全 `40056021225` 全通过；PR merge 与该提交的树完全一致。repository integration 实测 33.656 秒。
- 正式 [Release run 40056966532](https://github.com/hxly520/sub2api/actions/runs/40056966532) 18/18 jobs success；同 Tag 的后端 unit/integration、前端、积分 PostgreSQL 事务/并发、视频 worker、部署脚本、lint 和三安全扫描均通过。repository integration 实测 34.763 秒；非 verbose 输出未列每个用例名称，不额外声称逐项日志存在。
- 实际 GoReleaser `v2.19.0`；`SIMPLE_RELEASE=false`，`DRY_RUN=false`。Release [v0.2.18-52t.1](https://github.com/hxly520/sub2api/releases/tag/v0.2.18-52t.1) 包含 darwin amd64/arm64、linux amd64/arm64、windows amd64 五份归档及 checksums。五项 API digest 与 checksums 一致；linux/amd64 归档完整下载核验，39,568,567 bytes，SHA256 `afed7b85dc88ba6a9166c03d4648c859ad78d5927106b0bc7c961a0d513fd3b7`。其余四项未重复完整下载，不计完整字节回读。

| 镜像身份 | 值 |
| --- | --- |
| Tag | `ghcr.io/hxly520/sub2api:0.2.18-52t.1` |
| 多架构 index | `sha256:5a3abb7da355d5e9df8bf203aca79e84c53e5d6215720034580f01160bc262fc` |
| amd64 manifest | `sha256:7b826f9984457a788d1b6d90ea754dd52e7a6e2cfe6ef82fd89ec19a98791310` |
| amd64 config/image ID | `sha256:349f72bd5cf69d4f139412ac95cdc662fd7cb93cc52d571bf428800bee402101` |
| arm64 manifest | `sha256:59b97e120726bc3fbd4613147ebc86ea1381f153e3104d2558a2758526b834f5` |
| arm64 config/image ID | `sha256:ac1b531f4500f808ba8c0b211a2295e0cfa481f793f351bef0ea8616bad02e4b` |

两平台原始 manifest/config 字节 hash、size、os/arch 及 OCI version/source/revision 已严格核验。

### 6.2 服务器仅缓存证据

`2026-10-24 03:30:30–03:31:06 UTC`，只 pull 上述 Sub2API Tag，校验缓存 image ID、RepoDigest、OCI labels 与发布证据一致。amd64 创建时间 `2026-10-24T03:25:14.787783153Z`。覆盖 entrypoint、无网络、只读、无挂载的一次性 `-version` 输出正确版本/完整 commit；`pg_dump --version` 为 PostgreSQL `18.6`。没有启动默认业务或生产迁移。

| 原容器 | ID 前缀 | 运行镜像 | 前后状态 |
| --- | --- | --- | --- |
| sub2api | `675cf9c85293` | `0.2.15-52t.1` | healthy，restart 0 |
| sub2api-points | `85b668577d27` | `0.1.169-b64a0110ab2c` | healthy，restart 1，既有值未变 |
| 52token-image-workbench | `3a5ba2c521d3` | `v0.16.0-52t.1` | running，无 healthcheck，restart 0 |
| sub2api-postgres | `e31c7e773771` | `17.8-alpine` | healthy，restart 0 |
| sub2api-redis | `2a25f3796ce7` | `8-alpine` | healthy，restart 0 |

五容器的完整 ID、image ID/配置镜像、StartedAt、状态/健康和重启数严格相等。未执行 Compose、容器替换/重启、数据库或 Redis 写入、Nginx/Cloudflare/静态文件修改。首次检查因工作台没有 Health 字段在 pull 前停止，修正仓库外检查模板后完成；失败预检和最终成功证据均保留，没有隐去失败。

仓库外证据：`output/upgrade-plans/v218-ci-preserve-success.log`、`v218-release-full.log`、`v218-release-verification-summary.json`、`v218-release-artifacts/release-artifact-evidence.json`、`v218-server-cache-precheck-failure.json`、`v218-server-cache-evidence.json`。这些本地证据路径不是生产部署路径。

### 6.3 维护者人工切换（本任务未执行）

在 `/home/api/sub2api-deploy/docker-compose.yml` 仅修改 `sub2api.image` 为上述精确 Tag（也可使用已核验 index digest）。完成数据库和配置备份、回滚点记录后：

```bash
cd /home/api/sub2api-deploy
docker-compose config --quiet
docker-compose up -d --no-deps sub2api
docker ps --filter name=^/sub2api$
```

维护者切换前备份 PostgreSQL、Compose/env，保留旧镜像精确 Tag；仅更新 Compose 的 `sub2api` image，然后单服务 `up -d --no-deps sub2api`。自动化不执行该步骤，不替换积分/工作台等容器。

切换后验证健康、登录、普通 Key、流式与 usage、长上下文计费、可盈支付、积分桥接、媒体/视频以及退役 Key 失败关闭；对比切换前后的各分组生图开关，确认 true/false 均未变化，并核验 246 以原 filename/checksum 记录。不能通过旧镜像撤回数据库 forward-only 迁移；回滚前确认新缓存数据和新配置兼容。

宿主 `/help/` 仍由 Nginx 静态目录提供，镜像切换不会自动同步。仓库帮助页已删除额度卡内容，SHA256 为 `a2eb9ade4ad512c38f412b2a2bad565c7cce5d5d606ba4f3b065edcaa5a33fb5`；维护者如需线上同步，另行备份并原子替换 `/home/api/sub2api-deploy/help/index.html` 后核验。本任务不修改该宿主文件，当前首页也不重新部署。
