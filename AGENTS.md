# 私有仓库接替入口

本文件供后续自动化维护者和 AI 接替时使用。它只记录源码边界与验证规则，不存储生产密码、完整 API Key、数据库连接串、HMAC secret、GHCR PAT、GitHub token 或请求数据。真实生产状态以脱敏运维记录和服务器只读核验为准。

> **候选移除范围**：提链/额度卡已从当前候选运行时代码、路由、页面和部署入口移除，生产尚未切换/重新核验。历史接口、账本及开放证据只用于审计。`194_link_cards.sql` 保留原文件和 checksum，当前迁移 runner 按完整文件名跳过。

## 当前已发布候选（2026-10-24，生产待人工切换）

- 官方基线 `v0.2.18`：annotated tag object `de86961b627e48db75d61fff8c05535d44a00cbe`，peeled commit `ac1b8f1271be0d674fa6f074403c1410f30a3a81`。官方 tag 内 VERSION 为 `0.2.17`；私有版本为 `0.2.18-52t.1`，Go `1.27.3`。
- 分支 `codex/upgrade-v0.2.18-no-linkcards`，私有起点为 `main` `6cd5468dff7b874647571b80f08002f51b8a9ea6`，merge-base 为官方 `v0.2.15` `f2669c8cf62555cd92389b3f55920e9e6e7c6ff2`。
- 用户确认 Sub2API 提链和独立额度卡中心全部停用；本候选移除运行时代码、路由、菜单、页面、独立部署模板及专用计费。历史数据仅审计，不能重新启用。
- 这是候选源码状态，不是生产已经切换或停用的证明。本轮测试、Release、镜像和服务器缓存状态见 [本轮交接](docs/RELEASE_V0_2_18_52T_1_CN.md)，无新证据的项目保持 `pending`。
- PR #17 已合入 main；私有 annotated Tag `v0.2.18-52t.1`、源码与双平台 OCI revision 均为 `d16c14b135fe8de25ce73850db51040c1cd8d679`。精确 Tag Release run `40056966532` 的质量、安全、真实 PostgreSQL 回归、full 五平台归档和双架构 GHCR 全部通过；实际 GoReleaser `v2.19.0`。
- 镜像 `ghcr.io/hxly520/sub2api:0.2.18-52t.1` 已核验并于 `2026-10-24 03:31 UTC` 缓存服务器，index 为 `sha256:5a3abb7da355d5e9df8bf203aca79e84c53e5d6215720034580f01160bc262fc`。缓存前后五容器身份/状态完全一致；生产仍为 `0.2.15-52t.1`，未切换或触发迁移。246 保留现有生图 true/false 的代码与隔离 PostgreSQL 回归已通过；生产切换后仍须核验。

## 开始前必读

1. `docs/PRIVATE_CUSTOMIZATION_CN.md`：二开功能清单、代码入口和官方合并规则。
2. `docs/PRIVATE_RELEASE_RUNBOOK_CN.md`：当前 v0.2.18 私有发布的测试、GitHub Actions、GHCR 和人工 Compose 流程。
3. `docs/OFFICIAL_COMPATIBILITY_HISTORY_CN.md`：官方版本差异及证据状态。
4. `docs/PRODUCTION_OPERATIONS_CN.md`：生产镜像、数据库、Redis、Nginx 和只读证据；不要以旧文档猜测运行态。
5. `docs/LINK_CARDS_ACCEPTANCE_20260808_CN.md`、`docs/MEDIA_API_CN.md` 和 `points-system/README.md`：额度卡历史验收（仅审计）、媒体和积分契约。

## 仓库与分支规则

- 二开远端为 `hxly520/sub2api`（可按发布需要设为私有或公开）；`upstream` 只用于读取官方 `Wei-Shaw/sub2api` Release 和提交。
- `main` 是私有维护主线。正式升级从当前主线创建 `codex/upgrade-vX.Y.Z-*` 分支，再合并指定官方 Release；不得从官方 tip 另起生产分支。
- 保留完整 Git 历史、merge-base、迁移文件名和 checksum。禁止浅克隆、按数字前缀覆盖同号迁移、重命名已应用迁移，或使用 `git reset --hard` 清掉未审查的工作树改动。
- 同类功能冲突时官方实现为主；私有代码只能在官方入口加入明确的产品契约和回归测试。

## 私有功能保留边界

- 同库 `points-system` 积分/签到桥接与管理员配置。
- 提链/额度卡功能已停用；其账本、预扣/后扣费、欠费恢复、退款和公共查询门户仅保留历史迁移与审计记录。
- 图片/视频任务、余额冻结/释放/核销及统一媒体 API。
- 首字 Token/TTFT 记录与调度画像；失败 attempt 不得污染样本，也不得新增请求重放。
- Codex `x-codex-turn-state` 单值 48 KiB 保护及 Nginx 响应头缓冲约束。
- 当前未登录首页、帮助页、导航和独立 `infinite-canvas` 工作台部署契约。

普通 API Key 页旧 CCSwitch 自动导入仍不属于当前保留范围；KeyingPay V2（可盈Pay）支付通道必须保留签名、金额核验、幂等回调、查询和退款适配。额度卡 CCSwitch 教程与提链 Key 均为历史内容，当前入口和鉴权路径已移除。当前升级以官方 `v0.2.18` 网关、协议、重试、缓存、计费和账号调度语义为准。官方 `235_group_model_allowlist.sql` 已属于历史基线，普通 Key 和媒体入口继续经过同一模型白名单。

## 不可破坏契约

- 媒体创建最多提交一次；明确失败、取消或空结果释放冻结余额，未知终态不擅自退款，成功只核销一次。
- Seedance 视频路径维持用户已确认的官方请求后计费，不接入私有媒体预冻结。
- 历史额度卡 Key 和旧认证快照必须失败关闭，不得按普通 Key 扣创建者余额；历史非用户资金来源媒体冻结也不得扣退普通余额。历史表和流水不删除、不自动退余额。
- 积分用户/管理员界面严格隔离，策略按约定的次日生效；服务端开关关闭时入口和 API 同时隐藏/拒绝。
- 官方请求失败、断流、超时或 5xx 不得被私有旁路泛化成重放；只允许官方明确的有界重试。
- 根路径首页和 `/help/` 帮助中心由记录中的静态资产发布；修改镜像不会自动更新宿主静态页。
- 生产服务器不编译 Go、前端或 Docker 镜像；镜像由 GitHub Actions/受控本机构建，服务器只拉取或导入已核验的 tag/digest。

## 版本与发布

当前版本身份见上文和本轮交接；旧 v0.2.15 发布/服务器缓存证据只属于历史，不代替本次验证。

官方相对 v0.2.15 新增 `242_group_i18n.sql`、`243_gemini_cached_contents.sql`、`244_gemini_cached_contents_channel_usage.sql`、`245_gemini_cached_contents_mutation_lease.sql`、`246_enable_gemini_image_generation_groups.sql`，两份 `242` 按完整文件名共存。迁移 checksum 是 UTF-8 内容经 `strings.TrimSpace` 后的 SHA256；旧迁移不修改、不改名。用户于 **2026-10-24 明确保留分组生图开关现状（true/false 均不变）**：246 原 SQL 及 checksum `87267be7ab657173768989e2b0b105c5569c5930a7f1ed124f4cf81e5198297c` 保持不变，runner 仅对该完整文件名和已审查 checksum 执行配置保留例外，不执行其开启开关的 SQL；首次处理在同一事务记录原 filename/checksum，已有迁移历史继续校验，未知 checksum 失败关闭。其他迁移正常执行，不泛化跳过同号文件。该兼容改动的最终精确提交及 Tag 必须另经 CI/隔离数据库验证，不继承此前代码全绿结论。未经隔离数据库验证不得发布；不把 `kimi-k3` 缺失 usage 自动视为修复，不估计扣费、不历史追扣。

沿用既有 full 发布目标和 GHCR，适配已审查的官方发布工作流结构，`SIMPLE_RELEASE=false`，发布策略为 `image-update-required`。本轮仅构建并缓存 Sub2API 候选镜像；生产由维护者手工仅切换 `sub2api` 服务，不替换积分、生图工作台等其他容器，不修改生产数据库、Redis、Nginx 或宿主首页/帮助静态资产。

私有发布 Tag 必须是 `vX.Y.Z-52t.N` 格式的 annotated Tag，且 Tag 树内 `backend/cmd/server/VERSION` 必须等于去掉前导 `v` 的 Tag 值。Release 工作流始终以该精确 Tag 调用 `backend-ci.yml` 与 `security-scan.yml`；校验、质量或安全门禁失败不得发布，成功后也不得由工作流单独改写或推进 `main`。

每次变更至少记录：官方 commit、私有分支/Tag、merge-base、迁移清单和 checksum、测试命令输出、workflow run、镜像 digest/OCI revision、回滚点和生产切换证据。没有命令输出或 CI 证据的项目保持 `pending`，不得写成已完成。

## 遇到不确定情况

涉及资金、数据库迁移、生产流量、Nginx/Cloudflare、媒体工作台或官方同类功能时，先停在方案和差异审查阶段，列出冲突与待确认项；不要猜测补齐，也不要自动替换生产容器、修改生产数据库或重启主服务。
