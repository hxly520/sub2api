# 提链与额度卡中心维护契约

本文记录私有 Sub2API 的“提链/额度卡中心”产品、数据、计费、安全和升级契约，供后续官方版本兼容合并与生产交接使用。本文不保存真实 API Key、用户余额、数据库连接、会话令牌或服务器凭据。

> 历史生产状态（2026-08-08）：维护者当时手工运行的是 `ghcr.io/hxly520/sub2api:0.1.169-b3e230220a9d`，`194_link_cards.sql` 已应用且用户 1 的 0.08x 资金验收完成；这段记录只描述历史运行态。

> 当前源码候选状态：私有分支 `codex/final-v0.1.172-compat` 的代码提交为 `0948f0191c18045d8d04ccbf275ac4688d2c39af`，已兼容官方 `v0.1.172` 与 `upstream/main=cc67b1aca`，`backend/cmd/server/VERSION=0.1.172`。额度卡公共会话 404 修复、Redis fail-close 激活保护、原生使用记录复刻、刷新并发收口和首行悬停视口保护已纳入候选；候选镜像尚未自动替换 Sub2API 生产容器，必须由维护者手工切换。

## 1. 产品边界

提链 Key 是存放在 Sub2API `api_keys` 表中的预充值 API Key。创建者仍是已注册用户，但下游使用者不需要注册 Sub2API 账号：创建者先从自己的 Sub2API 余额中划出金额创建 Key，下游在额度卡中心用完整 Key 激活后，直接调用现有 Sub2API 网关。

本功能只增加预充值 Key 的发行、激活、充值、退款、状态管理和查询视图，不复制模型、渠道、分组、价格、账号调度或 usage 计费实现。所有请求继续经过原有 `/v1` 网关、真实分组、模型能力、账号选择、协议转换、内容风控、价格和使用记录链路。

提链 Key 不使用订阅、不设置到期时间、也不设置额外总消耗上限；余额用尽后进入 `depleted`。默认单张并发为 `5`，RPM 默认不额外限制，管理员可以实时冻结或调整单张 Key 的限制。

三个界面边界如下：

| 角色 | 前端路由 | 能力与限制 |
| --- | --- | --- |
| 注册用户 | `/link-cards` | 选择管理员已授权分组，单张或批量创建；查看本人完整 Key、状态、额度和使用记录；给本人 Key 充值；仅对未激活且零使用的 Key 申请退款 |
| 管理员 | `/admin/link-cards` | 独立总览、请求记录、分组授权和密钥操作；配置功能开关与运行限制；查询全部创建者和全部提链 Key；执行充值、退款、冻结、解冻、撤销及限额调整 |
| 下游额度卡用户 | `https://key.52token.org/card` | 默认只显示完整 Key 输入与激活；激活成功后显示 1x 额度、请求记录和接入教程，不提供充值、退款、编辑、删除或创建者资料 |

公共页面别名为 `/link-card` 与 `/quota-card`。`key.52token.org` 根路径应转到 `/card`；注册用户和管理员页面继续使用 Sub2API 原生布局、浅深主题和上传 Logo，只有公共额度卡页使用独立的简约科技主题。

管理员总览至少显示 Key 总量、活动 Key、实际储备和累计消耗；请求页提供与 Sub2API 使用记录一致的搜索和字段。三端品牌位都读取 `GET /api/v1/settings/logo` 的后台上传 Logo并保留 `/logo.svg` 回退，不得硬编码另一套 Logo。

所有 Key 列表和使用记录默认每页 `10` 条。注册用户页和管理员页必须支持完整 Key 搜索与分页；公共页只显示当前会话绑定 Key 的记录。

## 2. 灰度与功能开关

候选默认设置必须保持：

- `link_cards_enabled=false`：全体用户功能关闭。
- `link_cards_development_mode=true`。
- `link_cards_development_user_ids=[1]`，兼容单值 `link_cards_rollout_user_id=1`。
- 管理员控制台不受普通用户开关限制，但仍必须经过管理员认证和合规门禁。
- 开关关闭时，只有开发名单中的用户可以看到菜单、通过前端路由守卫并调用用户或公共接口；其余用户的菜单、直接路由和手工 API 请求都必须在服务端失败关闭。
- 未经真实验收不得把 `link_cards_enabled` 改为 `true`，不得扩大开发名单，也不得把名单下发给浏览器。

功能开关不是安全边界的替代品。每个读取和写入接口都必须重新检查当前用户或 Key 创建者是否仍在允许范围；公共激活也必须在修改 Key 状态前完成同一检查。

## 3. 路由与代码入口

### 3.1 浏览器 API

注册用户路由前缀：`/api/v1/link-cards`

- `GET /access`
- `GET /settings`
- `GET /groups`
- `GET /cards`
- `POST /cards`
- `POST /cards/:id/recharge`
- `POST /cards/:id/refund`
- `GET /usage`

管理员路由前缀：`/api/v1/admin/link-cards`

- `GET|PUT /settings`
- `GET|POST|PUT|DELETE /groups`
- `GET /cards`
- `GET /usage`
- `POST /cards/:id/actions`

公共路由前缀：`/api/v1/public/link-cards`

- `POST /activate`
- `GET /me`
- `GET /usage`

公共安全约束：激活接口按可信客户端 IP 共享 Redis 限制每分钟 30 次，只有明确的未知 Key 错误计入连续失败；第 10 次连续失败锁定 5 分钟并返回 `Retry-After`。成功激活清零错误计数，Redis 故障时 fail-close 返回 `503`。公共接口不带 Sub2API 登录 `Authorization`，短期会话过期的 401 由额度卡页面回到 Key 输入页，不跳转不存在的 `/login`。

公共激活只接受完整 Key。激活成功响应仍只返回脱敏 Key和短期会话令牌；后续资料和记录请求使用 `X-Link-Card-Session`，不得在 URL、查询参数或日志中传递完整 Key。为了支持用户在详情页恢复并复制当前提链 Key，只有通过有效短期会话访问 `GET /me` 时，响应顶层才返回该卡完整 Key；该响应必须强制 `Cache-Control: private, no-store`，页面只显示脱敏值，复制按钮读取完整值。公共资料仍不得返回创建者 ID、邮箱、发行倍率、实际预充值金额或内部资金字段。

### 3.2 主要代码入口

- 数据模型：`backend/ent/schema/api_key.go`、`link_card_group_authorization.go`、`link_card_operation.go`、`link_card_ledger.go`。
- 迁移：`backend/migrations/194_link_cards.sql`。
- 业务与资金事务：`backend/internal/service/link_card.go`、`backend/internal/repository/link_card_repo.go`。
- 网关鉴权与计费：`backend/internal/server/middleware/api_key_auth.go`、`backend/internal/service/gateway_usage_billing.go`、`backend/internal/repository/usage_billing_repo.go`、`backend/internal/service/openai_gateway_usage.go`。
- 路由与 Handler：`backend/internal/server/routes/link_cards.go`、`routes/user.go`、`routes/admin.go`、`backend/internal/handler/link_card_handler.go`、`handler/admin/link_card_handler.go`。
- 前端：`frontend/src/api/linkCards.ts`、`views/user/LinkCardsView.vue`、`views/admin/LinkCardsConsoleView.vue`、`views/public/LinkCardPortalView.vue`、`components/link-cards/`。

升级时必须同时保留 `frontend/src/api/client.ts` 的公共会话拦截规则、`backend/internal/server/middleware/link_card_activation_limiter.go` 的 Redis 防爆破中间件和 `deploy/nginx/key.52token.org.conf.example` 的 `/login -> /card` 防御性重定向；删除任一项都会复现额度卡 404 或降低激活安全性。

普通 `/api/v1/keys` 页面、查询和更新必须固定只处理 `key_type=standard`。提链 Key 只能通过本节专用接口管理，官方升级不得因 API Key 仓库或 Ent 查询重构而重新混入原 Key 列表。

## 4. 数据与计费口径

### 4.1 真实数据复用

| 事实 | 权威来源 | 约束 |
| --- | --- | --- |
| 创建者可用余额 | `users.balance` | 客户端余额、总扣款和返回值均不可信；只在数据库事务内条件扣减 |
| 可发行分组 | `groups` + `link_card_group_authorizations` + 原生用户分组权限 | 只允许管理员明确授权、活动、非订阅且倍率大于 0 的真实分组；专属分组还必须存在 `user_allowed_groups` 授权 |
| Key | `api_keys`，`key_type=link` | 与标准 Key 共用网关鉴权和 usage 关联，但管理查询必须隔离 |
| 模型与价格 | 现有分组、渠道定价、模型价格和网关计费 | 不为额度卡维护第二套模型表或价格表 |
| 请求记录 | `usage_logs` | Token、缓存创建/读取、图片 Token、费用、类型、流式、延迟和首 Token 等字段沿用 Sub2API 口径 |
| 资金审计 | `link_card_operations` + `link_card_ledger` | 操作幂等记录永久保留；资金流水只追加，不允许更新或删除 |

额度费用必须按数据库 `decimal` 精确值对账。公共/注册用户视图先将原生 `actual_cost` 除以发行倍率得到 1x 口径，缓存读取、缓存写入、输入、输出和图像费用明细全部沿用 `usage_logs`；原生 Sub2API 使用记录当前主金额显示保留 6 位小数，不能把该显示值再次乘倍率反算真实费用。例如原始 `actual_cost=0.0005845600`、发行倍率 `0.08` 时，额度卡正确显示 `0.007307`；原生页显示 `0.000585` 只是六位小数四舍五入，不构成漏算缓存或资金差额。费用和 Token 明细悬停面板必须通过 `Teleport` 放到 `body`，按面板实际尺寸在视口上下左右边界内钳制，窄屏允许换行和纵向滚动，首行不得被上方容器裁剪。

提链管理控制台只维护允许发行的 `group_id` 白名单，不提供独立倍率、专属倍率或专属分组编辑器。注册用户可见分组必须是“提链白名单”和“Sub2API 原生用户可用分组”的实时交集：普通分组按原生公共权限显示，专属分组还必须由原生 `user_allowed_groups` 授权。显示和创建使用的有效倍率也只由 Sub2API 原生解析器给出，用户专属倍率覆盖分组默认倍率。例如控制台授权默认 `0.08x` 分组后，存在该分组 `0.07x` 专属倍率的用户显示并按 `0.07x` 发行；未配置该专属倍率的其他用户按原生 `0.08x` 发行（若另有原生专属倍率则以其为准）。控制台授权 `0.05x` 专属分组后，也只有同时获得该原生专属分组授权的用户可见、可创建。前端列表仅用于展示，创建事务必须重新解析权限和倍率，不能信任浏览器提交值。

### 4.2 发行换算快照与动态请求倍率

创建时从服务端重新锁定分组与用户权限，并按“用户专属倍率覆盖分组默认倍率”得到发行换算倍率 `r_issue`，保存为 Key 的发行倍率快照。单张实际预充值金额为 `A` 时，对外 1x 可用额度为：

```text
issued_quota_1x = A / r_issue
```

例如 `A=10 U`、`r_issue=0.10`，则 1x 发行额度为 `100 U`。分组或用户专属倍率后续变化不回写、不重算已发行额度；充值继续使用该 Key 的发行换算快照，禁止让客户端提交或覆盖倍率。

每次请求仍由 Sub2API 原生分组逻辑实时解析当前有效倍率 `r_current`，包括分组默认倍率、创建者专属倍率及原生图片/视频独立倍率。对卡片 1x 额度的等效扣减倍率为：

```text
quota_rate_1x = ceil((r_current / r_issue) * 10) / 10
```

换算结果固定向上取一位小数，不能用普通四舍五入向下少扣成本。例如发行 `0.07x`、当前有效倍率 `0.08x` 时，`0.08 / 0.07 = 1.142857...`，卡片倍率取 `1.2x`，内部真实扣费倍率为 `1.2 * 0.07 = 0.084x`。发行 `0.10x`、当前 `0.15x` 可整除时仍为 `1.5x`。倍率变化不会瞬间重算已发行额度；只影响后续请求。使用记录继续提供 Sub2API 原生 Token/费用明细；注册用户、公共额度卡和提链 Key 的 `/v1/sub2api/billing` 只返回换算后的 1x 口径，不得到创建者的专属倍率、发行倍率或实际资金字段。管理员审计保留实际结算倍率与真实资金费用。创建者在发行和充值时已经扣款，网关调用不得再次扣创建者 `users.balance`。

### 4.3 生命周期与权限

```text
pending_activation -> active -> depleted --足额充值且可用额度>0--> active
                          |  \
                          |   -> frozen -> active
                          -> refunded / revoked
```

- 新 Key 固定为 `pending_activation` 且 API Key 状态禁用，不能直接调用网关。
- 公共额度卡中心首次提交完整 Key 后，服务端在事务内锁定 Key 并切换为 `active`；重复输入已激活 Key只建立新的短期查询会话，不重复扣款或创建 Key。
- 只有创建者可充值本人 Key；管理员可按权限给任意 Key 充值。充值金额先从创建者余额扣减，再增加该 Key 储备，两个动作必须同事务提交。
- `depleted` 对外统一显示“欠费”且不能调用网关；创建者充值后，只有在覆盖历史欠费和在途预留后仍有严格大于零的可用额度时，才在同一事务把卡片与底层 API Key 自动恢复为 `active`。充值事务响应直接返回恢复后的状态，前端不得提供或要求再次点击“启用”。提交后立即失效余额与鉴权缓存，不提供额外手动启用步骤。自动恢复只处理“额度耗尽”这一阻断原因；管理员冻结、退款、撤销、创建者禁用或原生分组权限失效时不得借充值自动解锁。即使状态恢复，网关每次请求仍重新检查原生用户/分组权限，状态变更不能绕过该复核。
- 注册用户只能退款本人 `pending_activation` 且实际使用额为零的 Key。退款返还全部剩余实际预充值金额并销毁原 Key 凭据。
- 管理员可以退款已激活或已使用 Key，但只能返还未消费、未退款且未被在途请求占用的剩余实际金额；退款前必须冻结 Key、失效鉴权缓存并等待或收口在途请求。
- `freeze` 只暂停调用，不返还资金；`unfreeze` 不能绕过零额度或欠费状态，必须先充值至可用额度严格大于零；`revoke/delete` 销毁凭据但不等于退款。任何返款都必须经过明确 `refund` 资金流水。

## 5. 资金安全与幂等

所有创建、充值、退款和管理员状态操作必须携带 `Idempotency-Key`。服务端只保存其 SHA-256，唯一范围至少包含操作类型和操作者；相同幂等键与相同请求应返回原结果，相同幂等键与不同请求必须冲突。请求指纹必须使用稳定结构化编码，禁止使用无分隔字符串拼接或指针地址。

批量创建的权威总扣款为 `单张金额 x 数量`。服务端验证数量上限、金额、授权分组和倍率后，在一个数据库事务中完成：

1. 永久占用幂等操作记录。
2. 条件扣减创建者余额，余额不足时零张生成。
3. 创建全部 Key，并为每张写入倍率快照和发行流水。
4. 保存可重放的完整结果并提交。

任一 Key 或流水失败必须整体回滚，不允许部分成功。金额使用数据库 `NUMERIC` 与 Go decimal；客户端传入的用户 ID、创建者、余额、倍率、总金额、已用额度和可退款金额一律不能作为资金事实。

网关结算必须用 `request_id + api_key_id` 幂等，同一请求只增加一次 usage 和一次提链消费流水。提链专用计费仓库异常时必须失败关闭，禁止回退为创建者余额扣款或只写 usage 不扣储备。

以下是生产发布阻断项，必须在代码和 PostgreSQL 并发测试中得到证据：

- 文本请求沿用 Sub2API 原生后扣时序：请求准入时可用额度必须严格大于零；提链 Key 的 `quota=0` 不继承普通 Key 的“不限额”语义。已经准入的最后一笔或并发在途请求允许在成功后把额度扣成负数，但必须按实际费用完整增加 `quota_used`，不得截断为剩余额度或形成免费差额。结算后立即进入 `depleted`、失效鉴权缓存并拒绝后续请求。
- 图片、视频和批量生图继续在调用上游前按每张 Key 原子预留预计费用，实际结算不得超过预留；媒体失败、取消和未知终态按现有冻结核销规则释放或收口，不能改扣创建者余额。
- 欠费 Key 充值后只有在 `充值后总额度 - 已用额度 - 在途预留 > 0` 时才自动恢复 `active`；不足以覆盖欠款的充值仍保持 `depleted`。
- 并发和 RPM 必须按提链 Key 独立计算，不能按创建者用户聚合，否则同一创建者的多张卡会互相占用或绕过限制。
- 退款、冻结、撤销和额度耗尽必须使该 Key 的鉴权缓存立即失效；公共会话每次查询还要重新检查 Key 状态。
- 管理员退款必须确认在途预留归零后计算可退款额；旧请求不得在退款提交后继续追加消费。

## 6. 公共域名与教程

- 额度卡中心：`https://key.52token.org`。
- API Base：`https://api.52token.org/v1`。
- Codex：流式 `POST https://api.52token.org/v1/responses`。
- Claude 协议：流式 `POST https://api.52token.org/v1/messages`。
- OpenAI 兼容：流式 `POST https://api.52token.org/v1/chat/completions`。

教程必须独立标注并允许复制只到 `/v1` 为止的 API Base，例如 `https://api.52token.org/v1`。前端对服务端返回值去除首尾空白和尾部 `/`，并保证 `/v1` 恰好追加一次；输入是域名根地址、尾部斜杠或已经带 `/v1`/`/v1/` 时，都不得生成重复路径。

三种协议页都提供流式请求示例和可复制配置文件：Codex 使用 `~/.codex/config.toml`、`~/.codex/auth.json` 与 `/responses`；Claude 使用 `~/.claude/settings.json` 与 `/messages`；OpenAI 兼容使用 `.env` 与 `/chat/completions`。所有请求都必须显式启用流式格式。示例中的密钥和模型固定使用 `CARD_KEY`、`MODEL` 占位符，绝不把当前真实 Key 插入请求或配置片段。

正式额度详情页继续保留“额度摘要 -> 使用记录 -> 接入教程”的既有布局，不新增独立大型 Key 面板。使用记录标题下方的“脱敏 Key · 分组名称”后只增加小型复制图标；点击后从有效短期 `/me` 会话响应的 `profile.key` 复制完整 Key，并通过页面顶部绿色 Toast 提示成功。公共页面正文与代码片段始终只显示脱敏 Key；剪贴板拒绝或写入失败只显示错误，不清除当前会话、不返回激活页，也不得降级为把完整 Key 渲染到 DOM。

CCSwitch 区域是只读配置指引，不是自动导入或一键写入：只展示客户端类型、只到 `/v1` 的端点、`CARD_KEY` 和 `MODEL`，由用户自行选择对应协议。

公共激活和查询路由必须设置独立 IP 限流、请求体上限和访问日志脱敏。日志不得包含完整 Key、`X-Link-Card-Session`、Authorization、请求正文或响应正文。会话默认 `3600` 秒，允许管理员在 `300-86400` 秒范围调整；页面关闭或会话过期不影响 Key 本身的激活状态。

Nginx/Cloudflare 只允许 `52token.org` 系列活动域名。上线前必须验证 `key.52token.org` 的证书、CSP、CORS、API 反代、真实客户端 IP 和移动端，不得恢复淘汰域名或使用通配父 Origin。

权威 Nginx 模板为 `deploy/nginx/key.52token.org.conf.example` 和 `deploy/nginx/sub2api-key-proxy.inc.example`。额度卡不是第二个后端容器：两个模板把最小公共路由集合反代到现有 `127.0.0.1:8080`，根路径跳转 `/card`，其余未知页面/API 返回 `404`。激活接口与会话查询关闭 origin access log，并叠加独立 IP 限流；模板只能在备份现状、确认 CF 真实 IP、执行 `nginx -t` 后原子安装并平滑 reload。

## 7. 设置键

| 设置键 | 候选默认值 | 含义 |
| --- | --- | --- |
| `link_cards_enabled` | `false` | 全体注册用户开关 |
| `link_cards_development_mode` | `true` | 开发灰度门禁 |
| `link_cards_development_user_ids` | `[1]` | 开发名单；只保存在服务端 |
| `link_cards_rollout_user_id` | `1` | 旧单值灰度兼容键 |
| `link_cards_default_concurrency` | `5` | 每张新 Key 的默认并发 |
| `link_cards_default_rpm_limit` | `0` | 每张新 Key 的 RPM；`0` 表示不额外限制 |
| `link_cards_max_batch_size` | `100` | 单次批量创建数量上限 |
| `link_cards_minimum_deposit` | 空 | 可选单张最低预充值金额 |
| `link_cards_public_portal_url` | `https://key.52token.org` | 公共额度卡中心地址 |
| `link_cards_api_base_url` | `https://api.52token.org/v1` | 教程和资料返回的 API Base |
| `link_cards_public_session_ttl_seconds` | `3600` | 公共查询会话有效期 |

管理员修改设置必须由后端校验范围并写入现有 `settings` 表。浏览器只可获取当前用户必要的公开设置；开发名单、内部倍率、资金流水和安全配置不得出现在普通用户或公共响应中。

功能开关、开发名单、已发行 Key 的冻结/限额操作实时生效。默认并发/RPM、批量上限和最低充值设置只约束后续创建。移除提链分组授权时必须立即阻止新创建并冻结该分组下尚未退款或撤销的提链 Key；撤销用户的原生专属分组权限后，鉴权也必须实时拒绝该用户既有提链 Key。上述权限变化都不得改写 Key 的绑定分组或发行换算快照。

## 8. 迁移 194

`backend/migrations/194_link_cards.sql` 是已进入生产的 forward-only 私有迁移，当前结构包括：

- 给 `api_keys` 增加 `key_type`、状态、倍率快照、预充值/退款、并发/RPM 和激活/撤销字段；已有行回填并保持 `standard`。
- 创建管理员分组授权表 `link_card_group_authorizations`。
- 创建永久幂等表 `link_card_operations`。
- 创建不可修改的资金流水表 `link_card_ledger` 及 usage/request 唯一约束。
- 写入关闭状态和用户 `1` 开发灰度的默认设置。

生产记录中的 checksum 为 `7a40799ddd3379acda1a3f704f110d81278a8d38705cd965325880996a8d23b4`。该文件已经应用，禁止改名、重编号、删除或修改 checksum；后续在途额度、退款收口或其他资金字段只能新增迁移，禁止原地改写 `194`。

迁移前必须做一致性数据库备份、记录 SHA256 和恢复命令，在隔离数据库验证从当前生产迁移总数升级、重复启动、标准 Key 回填、约束、索引、触发器、Ent schema 和旧版本只读兼容。不得在生产数据库试恢复。

## 9. 发布前验证

至少完成以下证据后才允许构建候选镜像：

1. Ent/Wire 生成文件无漂移，后端全量 test、vet、build 与前端 lint、typecheck、全量 Vitest、production build 通过。
2. 迁移 `194` 在当前生产结构副本执行成功；标准 Key 数量、状态、余额、usage 和原页面查询前后一致。
3. 功能关闭时用户 `1` 可见、其他普通用户菜单隐藏且所有用户/公共接口均服务端拒绝；管理员入口仍可用。
4. 用开发用户 `1` 的真实分组和余额做最小金额测试前先记录余额、Key、usage 和资金流水基线；创建、批量创建、激活、调用、充值、用户退款和管理员退款逐笔对账。
5. 批量余额不足、并发双击、网络超时和同/异请求幂等键重放不重复扣款、不部分发卡。
6. 每张 Key 独立并发/RPM、文本后扣末笔完整入账、并发晚到结算、欠费禁用、充值不足仍欠费、充值覆盖欠款后自动恢复并可直接发起下一次请求、管理员冻结不被充值绕过、重复充值幂等、媒体额度预留、缓存失效、退款等待在途和失败回滚通过 PostgreSQL 竞态测试。
7. Responses、Chat Completions、Messages、Gemini、图片、视频和其他已授权模型路径沿用原定价与 usage；提链调用不再扣创建者余额，标准 Key 与订阅计费不受影响。
8. 注册用户、管理员和公共使用记录都包含 Sub2API 同口径的 Token、缓存、费用、类型、流式、延迟与首 Token 字段；公共费用为 1x 口径且不泄露创建者或内部倍率。
9. 普通分组默认倍率、同组用户专属倍率和专属分组分别验证；用户可见项必须等于提链授权与原生权限交集，创建事务重算结果与界面一致，其他用户不能看到或提交用户 `1` 的专属分组。
10. API Base 分别使用域名根地址、尾部 `/`、`/v1` 和 `/v1/` 验证，最终端点都只能含一个 `/v1`；三种协议路径、流式参数、配置文件名及关键字段与 CCSwitch 四项只读值正确。
11. 真实完整 Key 不出现在可见正文、DOM 文本或请求/配置示例中；显式复制按钮只从有效短期会话复制完全一致的 Key，剪贴板失败后会话和详情页保持不变。
12. 桌面浅色、桌面深色和 `390px` 移动端完成截图及交互验收；每页 10 条、100 条以上 Key 搜索分页、6-8 个授权分组、激活前后页面切换、长配置片段和三种教程无溢出。
13. 未执行生产数据库迁移、未修改 Nginx、未切换容器前，不得把候选行为写成生产事实。

## 10. 官方升级兼容清单

后续合并官方版本时逐项检查：

1. `api_keys` Ent schema、生成代码和仓库查询继续保留 `key_type` 与提链字段；标准 Key 管理仍过滤 `standard`。
2. API Key 鉴权允许已激活提链 Key 在创建者余额为零时调用，但继续检查创建者状态、Key 状态、绑定分组、模型权限、IP、并发与 RPM。
3. 分组管理、原生用户可用分组、用户专属倍率、模型价格和 usage 字段变更同步进入提链授权交集、发行快照与记录投影；提链控制台继续只授权分组，不复制权限、倍率或定价代码。
4. 所有协议的成功、失败、断流、重试和客户端断开路径只结算一次；官方新增端点默认不得绕过提链计费。
5. 图片/视频冻结逻辑不得冻结创建者余额；提链媒体请求必须使用提链自己的预留与结算，并保持媒体不安全重放边界。
6. `usage_billing_dedup`、usage 归档、请求 ID 或计费事务重构时保留提链流水唯一性和失败关闭。
7. 用户余额缓存、API Key 鉴权缓存或并发实现变更时，重新验证创建/充值/退款缓存失效与每 Key 独立限制。
8. 路由、中间件和前端导航合并时保留三种角色隔离、管理员开关、用户 `1` 默认灰度、公共短期会话和每页 10 条。
9. 公共教程重构时保留 `/v1` 恰好一次归一化、三种流式协议、四类配置文件、CCSwitch 只读指引、占位符安全、完整 Key 显式复制和剪贴板失败不清会话。
10. 迁移 runner 继续识别并校验 `194_link_cards.sql`；已应用后只追加新迁移。
11. 运行本文第 9 节完整门禁，并把新官方 release、私有 commit、迁移数量、镜像 digest 和回滚点写入生产交接文档。

## 11. 部署与回滚原则

- Sub2API 候选镜像只在 GitHub Actions 或受控本机生成，标签包含 VERSION 与 commit，并记录 registry digest、image ID 和 archive SHA256；服务器不编译或构建。
- 按现有约定只把 Sub2API 镜像上传/导入服务器，由维护者手工切换。功能镜像、迁移和 Nginx 不得在未确认窗口中自动替换生产服务。
- 首次切换必须保持全局开关关闭和开发名单仅用户 `1`。先验证标准业务，再验证用户 `1`，最后才由管理员显式决定是否全体开放。
- 切换前备份数据库、Compose、环境变量名称清单和 Nginx；记录旧镜像不可变 digest、迁移总数、标准/提链 Key 数量、用户余额合计和提链流水聚合。
- 回滚应用不会自动撤销 forward-only 迁移。若已存在提链 Key，切旧镜像前必须关闭入口、冻结并禁用全部提链 Key、失效鉴权缓存、等待在途请求归零并保存资金对账；旧版本可能不认识 `key_type`，不得让活动提链 Key暴露给旧鉴权或标准 Key 页面。
- 回滚不得删除 `link_card_operations`、`link_card_ledger` 或已发行 Key 的审计记录，也不得把储备余额直接加回创建者。任何退款或修复都必须使用可审计、幂等的资金事务。
- 数据库恢复是独立灾难恢复操作，只能使用已验证可恢复的备份；不得为了应用回滚直接在生产删除迁移记录、表、列或触发器。
