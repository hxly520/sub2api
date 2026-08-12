# 官方版本兼容合并台账

本文只记录可由 Git 历史、官方 Release tag、测试结果和脱敏部署证据复核的事实。`pending` 不得写成已完成；未完成版本不得创建生产候选或在线热更新 Release。

## 1. 版本总表

| 官方基线 | 官方 Release commit | 私有状态 | 私有兼容节点/说明 |
| --- | --- | --- | --- |
| `v0.1.164` | `cd8bb98c44303b2c8f04c0da340447c992f0cb7d` | completed (historical) | 私有 `d5bea143b` 合并；保留聚合分组、媒体冻结、积分和首页谱系。 |
| `v0.1.168` | `99c8e4bf7564823bafbab369acab6539e734c1bb` | completed (historical) | 私有 `d30c42da` 合并；后续由 `9f1b6bae`、`e4179147`、`55ac503b`、`7e598fbb` 补齐媒体、积分同库和首页。 |
| `v0.1.169` | `26d894ef4f50645a4bf1030e378ac892f17d0223` | completed (historical) | 私有 `3da18b9dd` 合并；保留官方安全修复、定价资源和调度语义，积分/额度卡独立兼容。 |
| `v0.1.172` | `155c494964c3ea6ecc31f52679525c1034bf0f16` | completed (historical) | 私有 `d6cfece20` 合并，`62d636672` 合入 tag 后官方热修复。 |
| `v0.1.173` | `29009f0b2ea14edf3b11ae2564fb617ff91a03b4` | superseded by candidate | 其 Grok/xAI、渠道监控 V2、计费、注册限制与迁移变化已由 `v0.1.175` 兼容候选整体吸收。 |
| `v0.1.175` | `93c32fa1a2450351561abc46156d2e28cb5f74ca` | compatibility verified; release pending | annotated tag object `b898c60c422d1de059968c56aca22f6643f1fed4`；分支 `codex/upgrade-v0.1.175-compat` 已通过本地全量门禁，尚未创建最终私有提交、Tag、镜像或生产发布。 |

官方 `v0.1.173` tag 的源码 `VERSION` 仍为 `0.1.172`，官方 `v0.1.175` tag 树内的 `VERSION` 仍为 `0.1.173`。发布审计不得只看 tag 名称，必须同时记录 annotated tag object、peeled commit、私有 `VERSION`、manifest source commit 和构建产物 revision。

## 2. 已完成合并结论

### v0.1.164

- 私有合并保留官方聚合分组、Ollama Cloud 用量、支付入口和安全修复。
- 私有媒体任务遵守“创建最多一次、失败/空结果退款、未知终态不擅自退款”；该约束在后续版本继续继承。
- 旧 v164 生产节点只作为历史回滚谱系，不能作为当前版本判断依据。

### v0.1.168

- 官方 Passkey、模型广场、Kimi K3、声明列更新和协议/安全修复按官方实现为主。
- 私有媒体核销、积分同库隔离、视频统一接口、协议终态、可见渠道、公开首页和余额缓存保护保留。
- 迁移、积分 schema、独立工作台和宿主 Nginx 不得因镜像更新被覆盖或重跑。

### v0.1.169

- 官方上游 URL 路径校验、`no-new-privileges`、价格 fallback、调度熔断和 GPT-5.6 价格更新保留。
- 私有积分策略、签到金额安全上限、邮箱列级 ACL、中文双工作区、登录邮箱展示、容量错误精确重试和额度卡原子计费保留。
- 官方同类行为冲突时以官方结构为主；私有测试必须移植到官方入口，而不是并行维护两套实现。

### v0.1.172

- `d6cfece20` 的第二父节点是官方 `v0.1.172`；`62d636672` 继续合入 tag 后 OAuth/路由提示等官方修复。
- 私有最终节点 `0948f0191c18045d8d04ccbf275ac4688d2c39af` 及其后文档/额度卡修复保留：媒体冻结回收、Redis fail-close、跨协议正式终态、提链 Key 资金与并发门禁、积分同库和公开首页。
- 迁移 runner 按完整文件名和 checksum 识别；私有 `194_link_cards.sql` 与官方同号迁移不得重命名、覆盖或按数字前缀猜测。
- 候选镜像和生产切换必须分别记录；自动化不得替换 Sub2API 容器。

### v0.1.175

- 本轮从私有起点 `689627207313a24cd5e445dd46cf3552fd3d9bc0` 合入官方 annotated tag `v0.1.175`，merge-base 为 `cc67b1aca1d3b590609abef2fcd3a6ca31c5c651`。Git 冲突已全部解决，并以保留官方父节点的双父 merge commit 收口；发布身份以最终私有 Tag 指向的完整 commit、manifest 和 OCI revision 为准。
- 官方 Grok/xAI、渠道监控 V2、响应模型计费、Gemini 图片统计、音频/搜索/视频价格、注册/凭证安全和 Codex 指纹能力保留；私有积分、提链、媒体冻结、容量精确重试、首页/帮助和私有更新源均保留。
- OpenAI 首输出状态机同时满足三条约束：空完成响应 failover；metadata-only EOF 不写下游并返回 `UpstreamFailoverError`；reasoning/item 结构进度解除超时但不提前记录 TTFT。旧身份测试夹具已补真实 output/usage，生产空响应保护没有放宽。
- 三份 `194` 迁移按完整文件名共存：`194_add_usage_log_upstream_response_model.sql`、`194_channel_monitor_v2.sql`、`194_link_cards.sql`。官方 `195-206`、`217-220` 也原名保留，发布策略固定为 `image-update-required`。
- 验证证据：后端 `go test ./... -count=1`、编译门禁与 `go vet ./...` 通过；前端 ESLint、typecheck、全量 Vitest 与生产 build 通过；`git diff --check` 通过。尚未构建镜像或验证数据库迁移后的生产冒烟，因此状态是“兼容已验证、发布待完成”，不是已上线。

## 3. v0.1.175 兼容矩阵与发布缺口

在创建私有兼容分支前，逐项填写状态和测试证据：

| 范围 | 官方变化 | 私有风险 | 处理状态 |
| --- | --- | --- | --- |
| Grok/xAI | OAuth/SSO、媒体、Voice、Realtime、搜索、模型映射和团队/模型配额 | 触及私有账号调度、协议路由和媒体冻结 | completed in candidate；全量测试通过 |
| 渠道监控 V2 | 被动流量聚合、V1/V2 开关、滚动表和权限迁移 | 不得主动探活或泄露私有请求内容 | completed in candidate；Wire 与路由测试通过 |
| 计费 | Gemini 原生图片、Grok 视频/搜索/音频、上游响应模型 | 保留价格快照、失败退款和额度卡倍率 | completed in candidate；定向计费与全量测试通过 |
| 迁移 | 官方 `194-206`、`217-220` | 私有 `194_link_cards.sql` 同号 | completed in source；按完整 filename/checksum 共存，部署必须 Compose |
| 账号/用户 | 邮箱域名限量、声明列、OAuth、凭证清理 | 积分 ACL 与提链创建者权限不得扩大 | completed in candidate；后端与前端门禁通过 |
| 前端/管理 | 监控 V2、Grok 管理、模型价格矩阵 | 积分、提链、首页和帮助不得覆盖 | completed in candidate；lint/typecheck/Vitest/build 通过 |
| 发布 | Tag、manifest、镜像、数据库迁移与人工切换 | 不能把本地候选误当生产 | pending：尚未提交、打 Tag、构建镜像或部署 |

### v0.1.175 发布门禁

1. 完成最终 merge commit 后重新执行 `git diff --check`、后端全量测试/vet 和前端 lint/typecheck/Vitest/build，记录最终 commit。
2. 创建私有 annotated Tag 与 `update-manifest.json`，确认 Tag `^{commit}`、manifest source commit 和二进制/OCI revision 一致。
3. 本轮强制标记 `image-update-required`；只能构建不可变镜像并由维护者使用 Compose 人工切换，后台热更新必须拒绝该候选。
4. 切换前备份数据库并核对所有 `194-220` 迁移的 filename/checksum；切换后验证积分、提链、媒体冻结、渠道监控、OpenAI/Grok/Gemini 请求和生产健康。
5. Grok pending billing 与媒体冻结均有 24 小时边界；发布观察期内必须持续检查未结算 claim、冻结余额与异步任务终态，异常时回滚镜像但不得回退已执行的 forward-only 迁移。

## 4. 记录模板

每次官方兼容合并追加一节，至少包含：

```text
官方 Release/tag：vX.Y.Z / OFFICIAL_COMMIT
私有分支与合并提交：BRANCH / PRIVATE_COMMIT
merge-base：MERGE_BASE
迁移：新增、冲突、checksum、是否 forward-only
官方优先冲突：文件/行为/测试证据
私有保留功能：代码入口与回归测试
发布策略：hot-update-safe | image-update-recommended | image-update-required
镜像与回滚：GHCR_TAG、DIGEST、BACKUP、数据库兼容边界
未解决问题：明确负责人和阻断条件
```

没有证据的项目保持 `pending`，不得以“看起来兼容”结案。
