# 私有仓库接替入口

本文件供后续自动化维护者和 AI 接替时使用。它只记录源码边界与验证规则，不存储生产密码、完整 API Key、数据库连接串、HMAC secret、GHCR PAT、GitHub token 或请求数据。真实生产状态以脱敏运维记录和服务器只读核验为准。

## 开始前必读

1. `docs/PRIVATE_CUSTOMIZATION_CN.md`：二开功能清单、代码入口和官方合并规则。
2. `docs/PRIVATE_RELEASE_RUNBOOK_CN.md`：v0.2.1 候选的测试、GitHub Actions、GHCR 和人工 Compose 流程。
3. `docs/OFFICIAL_COMPATIBILITY_HISTORY_CN.md`：官方版本差异及证据状态。
4. `docs/PRODUCTION_OPERATIONS_CN.md`：生产镜像、数据库、Redis、Nginx 和只读证据；不要以旧文档猜测运行态。
5. `docs/LINK_CARDS_CN.md`、`docs/MEDIA_API_CN.md` 和 `points-system/README.md`：额度卡、媒体和积分契约。

## 仓库与分支规则

- 二开远端为 `hxly520/sub2api`（可按发布需要设为私有或公开）；`upstream` 只用于读取官方 `Wei-Shaw/sub2api` Release 和提交。
- `main` 是私有维护主线。正式升级从当前主线创建 `codex/upgrade-vX.Y.Z-*` 分支，再合并指定官方 Release；不得从官方 tip 另起生产分支。
- 保留完整 Git 历史、merge-base、迁移文件名和 checksum。禁止浅克隆、按数字前缀覆盖同号迁移、重命名已应用迁移，或使用 `git reset --hard` 清掉未审查的工作树改动。
- 同类功能冲突时官方实现为主；私有代码只能在官方入口加入明确的产品契约和回归测试。

## 当前 v0.2.1 保留模块

- 同库 `points-system` 积分/签到桥接与管理员配置。
- 提链/额度卡账本、预扣/后扣费、欠费恢复、退款和公共查询门户。
- 图片/视频任务、余额冻结/释放/核销及统一媒体 API。
- 首字 Token/TTFT 记录与调度画像；失败 attempt 不得污染样本，也不得新增请求重放。
- Codex `x-codex-turn-state` 单值 48 KiB 保护及 Nginx 响应头缓冲约束。
- 当前未登录首页、帮助页、导航和独立 `infinite-canvas` 工作台部署契约。

普通 API Key 页旧 CCSwitch 自动导入仍不属于当前保留范围；KeyingPay V2（可盈Pay）支付通道已按支付模块最小适配恢复，额度卡页面的 CCSwitch 只读接入教程仍保留。网关、协议、重试、缓存、计费和账号调度必须保持官方 v0.2.1 语义。

## 不可破坏契约

- 媒体创建最多提交一次；明确失败、取消或空结果释放冻结余额，未知终态不擅自退款，成功只核销一次。
- 额度卡和余额变更必须经服务端鉴权、幂等键、事务和审计；客户端参数不是金额事实来源。
- 积分用户/管理员界面严格隔离，策略按约定的次日生效；服务端开关关闭时入口和 API 同时隐藏/拒绝。
- 官方请求失败、断流、超时或 5xx 不得被私有旁路泛化成重放；只允许官方明确的有界重试。
- 根路径首页和 `/help/` 帮助中心由记录中的静态资产发布；修改镜像不会自动更新宿主静态页。
- 生产服务器不编译 Go、前端或 Docker 镜像；镜像由 GitHub Actions/受控本机构建，服务器只拉取或导入已核验的 tag/digest。

## 版本与发布

当前候选基线：官方 `v0.2.1` commit `ab99d56e9626e6cd731592dae8553c9758a0efa2`，私有候选 `v0.2.1-52t.3`，生产基线 `v0.1.183-52t.4`。本候选包含数据库迁移、Ent/生成代码、后端、前端和容器变化，发布策略为 `image-update-required`：先跑全量门禁，再由 GitHub Actions 构建 `ghcr.io/hxly520/sub2api:<version>`，最后由维护者备份并手工 Compose 切换。官方后台在线更新器只识别官方 Release，不可安装本私有候选。

私有发布 Tag 必须是 `vX.Y.Z-52t.N` 格式的 annotated Tag，且 Tag 树内 `backend/cmd/server/VERSION` 必须等于去掉前导 `v` 的 Tag 值。Release 工作流始终以该精确 Tag 调用 `backend-ci.yml` 与 `security-scan.yml`；校验、质量或安全门禁失败不得发布，成功后也不得由工作流单独改写或推进 `main`。

每次变更至少记录：官方 commit、私有分支/Tag、merge-base、迁移清单和 checksum、测试命令输出、workflow run、镜像 digest/OCI revision、回滚点和生产切换证据。没有命令输出或 CI 证据的项目保持 `pending`，不得写成已完成。

## 遇到不确定情况

涉及资金、数据库迁移、生产流量、Nginx/Cloudflare、媒体工作台或官方同类功能时，先停在方案和差异审查阶段，列出冲突与待确认项；不要猜测补齐，也不要自动替换生产容器、修改生产数据库或重启主服务。
