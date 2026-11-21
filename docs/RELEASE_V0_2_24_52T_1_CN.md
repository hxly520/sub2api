# Sub2API v0.2.24-52t.1 升级交接

## 版本与范围

| 项目 | 记录 |
| --- | --- |
| 官方 Release | [v0.2.24](https://github.com/Wei-Shaw/sub2api/releases/tag/v0.2.24)，发布于 2026-11-20 17:19:50（北京时间）；非草稿、非预发布 |
| 官方身份 | annotated Tag object `7515db6dc56c2a6120dfb7abed70ef089ebc1457`；peeled commit `5c2982a81b46e2142d0143d2f83c1ce9c4da1d24`；官方 Tag 树内 `VERSION=0.2.23` |
| 私有起点 | `main` `572cb8e6c4a1b00e82f02d0299a5d7cd602f3b80`；共同基线 `v0.2.18` `ac1b8f1271be0d674fa6f074403c1410f30a3a81` |
| 私有目标 | `codex/upgrade-v0.2.24-52t`；`backend/cmd/server/VERSION=0.2.24-52t.1`；最终 annotated Tag `v0.2.24-52t.1` |
| 构建与切换 | 后端 Go `1.27.4`，前端 Node `24` / pnpm `9`；`image-update-required`，精确 Tag full 发布；维护者手工只切换 Compose 的 `sub2api` 服务 |

- **退役**：提链中心、管理控制台、额度卡公共入口和专用计费均保持移除。历史 `194_link_cards.sql`、已应用记录、账本和旧 Key 仅审计，不删历史数据、不自动退款、不重新开放产品。旧 Key 和非普通用户资金媒体冻结失败关闭；普通 Key 删除前的官方在途后扣费照常。认证快照升级为 `v29`，旧快照回源标准 Key 查询，不需要清空 Redis；官方新批量扣费路径也必须执行相同资金隔离。
- **保留**：同库积分/签到及次日生效策略、现有媒体及视频接入、失败释放和成功一次核销、统一备用生图与视频 API、工作台服务接入、真实 TTFT、Codex `x-codex-turn-state` 单值 48 KiB 保护、KeyingPay V2、现有未登录首页、帮助页、Logo 与导航。Seedance 维持官方后扣费，不接入私有媒体预冻结。
- **官方为主**：官方网关协议、调度/重试、缓存与普通计费优先，仅接回上文列明的私有边界。关注新版分组图片按张计费（默认关闭）、OpenAI 缓存写入周期与计价、用量批处理/日汇总，以及新增分组和渠道 Ultra Fast 定价字段。不得借升级启用新的生产功能开关；`kimi-k3` 缺失 usage 不估算、不历史追扣。
- **排除**：不替换或重启积分、生图工作台、独立额度卡等容器，不修改生产 PostgreSQL、Redis、Nginx、Cloudflare 或宿主首页/帮助静态资产。独立额度卡即使发现仍在运行，也只记录状态；本轮不操作该容器或改代理配置。

## 新增迁移

下表 checksum 为 UTF-8 内容经 Go `strings.TrimSpace` 后的 SHA256。双 `249`、双 `251` 按完整文件名处理，不修改或覆盖既有迁移。`250_openai_cache_write_ttl.sql` 的原始空白行与官方 Tag blob 一致；不要为了消除 `git diff --check` 的 EOF 提示改写已核验的上游迁移文件。

| 文件 | 用途 | SHA256 |
| --- | --- | --- |
| `247_scoped_admin_api_keys.sql` | 官方管理 Key 按权限范围存储 | `03445774e2dfad32da9e23279193d1bca02953896bc3d3b8a87ba0b36b6ed577` |
| `248_group_image_count_billing.sql` | 分组图片按实际张数计费开关，默认 false | `a872dde339b19f311feb5fa6fd85c8ff7b11d69794c35a18e2086d9846b3499c` |
| `249_add_usage_logs_api_key_cost_index_notx.sql` | 以 `CREATE INDEX CONCURRENTLY` 为 API Key 用量汇总建索引，非事务迁移 | `94f124c3e50405f7b75746885111a4f662c331985a697de46fe946a17ff1f7d9` |
| `249_mcp_oauth.sql` | MCP OAuth 授权、凭据及相关索引 | `751a1dcee6108afcecbd580b6e3140cca3480853fecd2174861b02f3a71e54a1` |
| `250_openai_cache_write_ttl.sql` | OpenAI 30m/12h 缓存写入 usage 及可选 12h 价格 | `532551ce7aee86b2ee0a78d397d422424ceed4051e31d5f542277c80c21b8b98` |
| `251_group_force_openai_ultrafast.sql` | 分组强制 Ultra Fast 开关，默认 false | `d78e605d4fc2038c57f620dff398022823a850ea8994b129b50774ac49e20af1` |
| `251_group_usage_rollup_nonblocking_build.sql` | 分组用量汇总发布水位、重建与失效触发器 | `5b6e9b57f9b354a96c6870185c034db8adc7def944efcddf654e39cd97770718` |
| `252_channel_pricing_ultrafast_multiplier.sql` | 渠道 Ultra Fast 可选正倍率，NULL 使用官方默认 | `458752b3523254ee60ab4a821261ab64df9216a6f208c1960e7b816f8c7cfd1d` |

继承两个例外：runner 按完整文件名跳过历史审计迁移 `194_link_cards.sql`；`246_enable_gemini_image_generation_groups.sql` 保持原 SQL 和已审查 checksum `87267be7ab657173768989e2b0b105c5569c5930a7f1ed124f4cf81e5198297c`，不执行开启生图开关的 SQL，首次同事务写入原身份，已记录继续核验，未知 checksum 失败关闭。现有开启及关闭值都不改。

## 门禁与交付状态

- 本地前端完整生产构建与全量 Vitest（398 文件，3,285 项）、积分 Go 单测及迁移单测已在候选验证期间通过；精确最终提交的完整门禁以 CI 和 Release 证据为准。
- 重点门禁：发布分支完整后端/前端/Wire；安全扫描；隔离 Linux PostgreSQL 验证 194/246、双编号迁移、并发索引与汇总重建、旧 Key/历史资金隔离、使用账单和迁移后回滚兼容。验证 KeyingPay 实付/赠送/余额到账/退款与幂等、GPT-5.6 长上下文双倍计费、媒体提交一次/失败释放/成功核销、Seedance 后扣费、TTFT 与响应头上限。
- 最终候选提交/PR、CI/安全扫描与 Linux 集成、精确 Tag Release、GHCR index/平台 digest 与 OCI revision、服务器缓存、人工切换和新版本生产验收：**待核验**。门禁未通过时不创建生产发布或缓存候选镜像。
- SSH 预检在握手阶段超时，当前镜像、在途任务和数据库迁移状态未重新核实。旧 `0.2.15-52t.1` 运行证据和 `0.2.18-52t.1` 缓存证据仅代表 [2026-10-24 的历史快照](RELEASE_V0_2_18_52T_1_CN.md)，不用于推断本轮生产现状。本轮未替换运行容器，也未执行生产迁移。

## 人工切换与回滚

1. GitHub Actions 从同一精确 annotated Tag 通过质量和安全门禁，沿用现有 full 构建和发布流程。核验五平台归档、checksums、GHCR 双架构 index 和平台 digest；确认 OCI revision 等于 Tag peeled commit。交付固定引用 `ghcr.io/hxly520/sub2api@sha256:<已核验的索引摘要>`，不交付移动标签作为部署依据。
2. SSH 恢复后只读核验当前生产镜像和服务、Compose 路径、磁盘余量、待执行迁移与在途媒体；只缓存已核验的 Sub2API 镜像并比对拉取前后容器身份、健康和重启计数。不执行 Compose 切换，不运行生产迁移。
3. 维护者在维护窗口备份生产数据库、Compose/.env 和当前不可变镜像引用，确认回滚条件，再按 [发布运行手册](PRIVATE_RELEASE_RUNBOOK_CN.md) 手工仅切换 `sub2api`。检查健康、登录、正常流式响应和真实 usage、长上下文计费、可盈支付、积分、主图/备用图/视频以及旧 Key/路由拒绝；核对配置开关不变。
4. 本次包含 forward-only 迁移。镜像回滚不会撤销数据库迁移；旧运行镜像与迁移后 schema 未在隔离环境验证之前不预设兼容。回退目标也必须继续隔离历史提链 Key 和入口，不以恢复旧额度卡业务作为回滚方案。
