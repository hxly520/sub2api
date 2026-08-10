# 官方版本兼容合并台账

本文只记录可由 Git 历史、官方 Release tag、测试结果和脱敏部署证据复核的事实。`pending` 不得写成已完成；未完成版本不得创建生产候选或在线热更新 Release。

## 1. 版本总表

| 官方基线 | 官方 Release commit | 私有状态 | 私有兼容节点/说明 |
| --- | --- | --- | --- |
| `v0.1.164` | `cd8bb98c44303b2c8f04c0da340447c992f0cb7d` | completed (historical) | 私有 `d5bea143b` 合并；保留聚合分组、媒体冻结、积分和首页谱系。 |
| `v0.1.168` | `99c8e4bf7564823bafbab369acab6539e734c1bb` | completed (historical) | 私有 `d30c42da` 合并；后续由 `9f1b6bae`、`e4179147`、`55ac503b`、`7e598fbb` 补齐媒体、积分同库和首页。 |
| `v0.1.169` | `26d894ef4f50645a4bf1030e378ac892f17d0223` | completed (historical) | 私有 `3da18b9dd` 合并；保留官方安全修复、定价资源和调度语义，积分/额度卡独立兼容。 |
| `v0.1.172` | `155c494964c3ea6ecc31f52679525c1034bf0f16` | completed | 私有 `d6cfece20` 合并，`62d636672` 合入 tag 后官方热修复；当前源码候选 `0.1.172`。 |
| `v0.1.173` | `29009f0b2ea14edf3b11ae2564fb617ff91a03b4` | **pending** | 已读取，尚未合并。涉及 Grok/xAI、被动渠道监控 V2、音频/搜索计费、邮箱域名限量和迁移 `194-220`，必须逐项审查后再建 `codex/upgrade-v0.1.173-*`。 |

官方 `v0.1.173` tag 的源码 `VERSION` 仍为 `0.1.172`；官方 `upstream/main` 后续提交 `48eb3766d` 才同步为 `0.1.173`。发布审计不得只看 tag 名称，必须同时记录 `VERSION`、完整 commit 和构建产物版本。

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

## 3. v0.1.173 待合并审查表

在创建私有兼容分支前，逐项填写状态和测试证据：

| 范围 | 官方变化 | 私有风险 | 处理状态 |
| --- | --- | --- | --- |
| Grok/xAI | OAuth/SSO、媒体、Voice、Realtime、搜索、模型映射和团队/模型配额 | 可能触及私有账号调度、模型可见性、协议路由和计费 | pending |
| 渠道监控 V2 | 被动流量聚合、V1/V2 开关、滚动表和权限迁移 | 不得向上游发送探活；不能泄露私有账号、Key 或请求正文 | pending |
| 计费 | Gemini 原生图片张数、Grok 视频/搜索/音频、上游响应模型 | 必须保留私有请求级报价快照、缓存/Token 明细、失败退款和额度卡倍率 | pending |
| 迁移 | 官方新增 `194-206`、`217-220` 等迁移 | 私有 `194_link_cards.sql` 同号冲突；需完整文件名、checksum、前向/回退审计 | pending |
| 账号/用户 | 邮箱域名注册限量、声明列、OAuth 和凭证清理 | 积分列级 ACL、登录邮箱展示、额度卡创建者权限不得扩大 | pending |
| 前端/管理 | 监控 V2、Grok 管理、模型价格矩阵和文案 | 登录 Dashboard、积分管理员页、提链控制台和首页主题不能被覆盖 | pending |
| 发布 | 官方 `VERSION`、Release asset、Compose security/env 变化 | 私有 Tag、manifest、GHCR、Nginx/points/infinite-canvas 需分层发布 | pending |

### v0.1.173 合并门禁

1. 从私有 `main` 创建完整 clone 的兼容分支，记录 `git merge-base`、官方 tag commit 和数据库备份点。
2. 先阅读官方所有迁移、路由、协议、计费和配置差异，再按功能块合并；不要直接复制官方 `main` 或覆盖私有目录。
3. 对私有 `backend/migrations/194_link_cards.sql` 做同号冲突审查；新官方迁移必须保留完整文件名和唯一 checksum，必要时把本次发布强制标为 `image-update-required`。
4. 运行私有功能矩阵：余额/缓存代次、媒体冻结与退款、容量错误有限重试、积分刷新/签到幂等、额度卡创建/充值/退回/欠费恢复、用户/管理员 ACL、移动端嵌入、公开首页和 `infinite-canvas` 跨服务边界。
5. 只有测试、迁移兼容、镜像构建和文档全部通过后，才创建 `v0.1.173-52t.N` annotated Tag。包含容器、Nginx、points、外部静态资源或未审迁移时，必须先 Compose；不得在线热更新。

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
