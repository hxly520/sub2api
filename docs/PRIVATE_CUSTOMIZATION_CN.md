# 私有二开维护与官方升级指南

本文档是本私有分支的维护入口。它记录官方 Sub2API 之外的兼容扩展、代码入口、不可破坏约束和升级流程，供后续维护者在合并官方新版本时使用。

## 1. 维护原则

1. 官方能力是基线。OpenAI Responses/Chat Completions、Anthropic Messages、Gemini、Codex、Claude、账号池、订阅、计费和管理后台的原有行为必须继续可用。
2. 私有二开只增加兼容能力，不用供应商特例替换官方通用流程。
3. 账号、分组和渠道映射是运行时配置。模型价格、账号地址、认证信息和渠道模型不得硬编码到源码。
4. 文本请求和媒体请求的重试边界不同。媒体创建是非幂等操作，发送后不得因为超时、断流或 5xx 自动换号重放。
5. 用户响应不得泄露账号 Base URL、供应商任务 ID、媒体地址、认证头或原始错误体。
6. 镜像只在 CI 或受控本机构建环境生成；生产服务器只拉取或导入已验证的标准镜像归档，不在生产机编译 Go、前端、二进制或 Docker 镜像。

## 2. 上游基线与私有分层

- 官方仓库：`Wei-Shaw/sub2api`。
- 私有仓库：维护者自己的 fork；官方远端只用于获取基线，不直接向官方远端推送私有提交。
- 当前仓库候选基线：官方 Release `v0.1.169`（release commit `26d894ef4f50645a4bf1030e378ac892f17d0223`）及其后的私有兼容提交，`backend/cmd/server/VERSION=0.1.169`。本轮只合入该已发布 tag，不合入 tag 之后的官方 `main`。媒体冻结、积分同库、公开首页、管理员积分配置入口、余额缓存并发保护和跨协议终态校验均属于必须保留的二开。

积分控制台采用单策略编辑器。管理员保存“开放用户积分功能”及其他积分/签到配置时，后端只追加下一自然日版本；历史版本不可变，页面不提供历史版本列表，也不允许客户端提交自定义生效日期。该 `enabled` 开关只负责业务层用户积分中心可见性，Sub2API/积分服务自身的 all/preview 配置仍是独立部署门禁。后续官方升级合并必须保留 `POST /api/v1/internal/user-access`、Sub2API `/api/v1/points/access`、菜单/路由/launch/session 的 fail-closed 校验和管理员策略台可用性。
- 截至 `2026-08-01 22:34 CST`，生产 Sub2API 仍为维护者手工切换的 `0.1.169-f79803bb73d6` / revision `f79803bb73d659e36627d6f716aab065ff4d56a6`，容器 `63d320fbf6ca...`；积分服务已独立切换为 `0.1.169-bee059a1cec5` / revision `bee059a1cec5d0eb1a6d022d766670489dcf484d`，容器 `43bde62f3fa2...`。两者均 healthy、restart count `0`。后续仍必须以运行容器 OCI revision 为准，不能只看仓库 `main` 或服务器镜像缓存；自动化不得替换或重启 Sub2API，详见 [`PRODUCTION_DEPLOYMENT_20260731_CN.md`](PRODUCTION_DEPLOYMENT_20260731_CN.md)。
- 版本来源：以 `backend/cmd/server/VERSION`、Git commit 和不可变镜像标签三者共同确认，不能只看前端版本文字。
- 当前分支必须保留一个可定位的官方 merge-base。升级前先记录旧生产 commit、官方新 tip、数据库备份点和可回滚镜像。

私有主线谱系：

| 节点 | 定位 | 维护含义 |
| --- | --- | --- |
| `e4cea8f8` | 旧私有 `main` / v0.1.155 | 只保留历史，不再作为升级起点 |
| `7d93ca720` | v0.1.162 兼容线 | 历史中间基线；是否仍有可用镜像由镜像仓库单独确认 |
| `d5bea143b` | 合并官方 v0.1.164 | 第二父节点为官方 release `cd8bb98c` |
| `6986037ed` | v164 发布修复 | 清理安全扫描与 lint 问题 |
| `08fbef836` | 历史生产应用代码 v164 | 加入视频源站防直连；保留为升级前回滚谱系 |
| `d30c42da` | 合并官方 v0.1.168 | 第二父节点为官方 release `99c8e4bf`；保留完整私有二开历史 |
| `9f1b6bae` | 媒体冻结最终核销修复 | 明确失败/空结果即时退款，未知终态保留，全站到期原子核销，成功费用按报价封顶 |
| `e4179147` | 安全积分与签到系统 | Sub2API 桥接、独立积分服务、版本化策略、事务发件箱、签到金额上限及 PostgreSQL 并发测试 |
| `55ac503b` | 积分同库隔离部署 | 复用现有 `sub2api` 数据库的独立 `points` schema 和最小权限角色，不启动第二个 PostgreSQL |
| `7e598fbb` | 未登录公开首页 | 中性功能文案、响应式导航和无脚本静态首页；不改变登录后 Dashboard |
| `2ad2815e` | 首个 v0.1.168 生产镜像 | 由维护者手工切换；包含媒体核销、积分桥接和同库积分服务契约 |
| `339422728` | 积分管理入口与余额桥接加固 | 管理员入口不受用户开关限制；增加 step-up、缓存代次/CAS、credit 重试和 fail-close 限流；历史 v0.1.168 生产 revision |
| `c0fe91506` | 中文积分双工作区 | 用户/管理员页面与脚本分离、用户关闭态二次校验、7/30/90 日趋势和用户响应脱敏；历史积分生产节点 |
| `d6b367f31` | 历史积分激活与容量错误精确重试 | 增加一次性历史回算、启用策略门禁，以及仅针对精确容量拒绝文案的有界文本重试；当前 Sub2API 候选必须保留 |
| `28e760bc8` | 积分用户明细与签到发放任务收敛 | 管理员全站用户积分明细；删除积分侧手工赠送和快照刷新入口，发放任务只处理签到奖励；历史生产积分 revision |
| `874255bcd` | 嵌入式积分大屏优化 | 用户大屏、管理员紧凑工作区、中文结算状态和嵌入式布局优化；历史视觉基线，已由 `7e50f9aa9/e39c78bf8` 演进 |
| `1e33e7f7a` | v0.1.169 升级前二开收口 | 历史用户名积分展示与最小 ACL、软删除会话失效、双工作区/首页视觉、跨协议终态和客户端断连有限排空；当前展示契约已演进为登录邮箱 |
| `7e50f9aa9` | Taste 积分工作区与登录邮箱候选 | 用户/管理员主题融合、业务发放时间、HMAC 游标分页、请求超时、邮箱 ACL 四阶段迁移与完整回归 |
| `e39c78bf8` | 无障碍趋势表布局隔离 | 保留屏幕阅读器 table 语义，同时消除隐藏表造成的桌面空白尾部；上一积分生产 revision |
| `e8d73f3e6` | Taste 数字工作区增强 | 主积分焦点、精密网格、同步状态、8px 面板尺度和 reduced-motion；当前积分生产 revision |
| `3da18b9dd` | 合并官方 v0.1.169 | 双亲为私有收口 `1e33e7f7a` 与官方 release `26d894ef4`；冲突按官方能力优先并兼容保留私有调度、渠道可见性、媒体和积分行为 |
| `04a19ca08` | v0.1.169 发布链历史节点 | Gemini 非流式 SSE 聚合收到正式 `finishReason` 后立即完成；后续已由用户 1 预览兼容提交 `f79803bb7` 取代 |

完整提交历史和 tag 是正式合并的前置条件。浅克隆、`blob:none` 或 sparse checkout 只可用于阅读，不得用于正式版本合并和发布。

私有变更按职责分为四层：

| 层级 | 目标 | 约束 |
| --- | --- | --- |
| 协议兼容层 | 兼容 OpenAI、Anthropic、Gemini、国内 OpenAI-compatible 实现和第三方客户端 | 不按 User-Agent 或检测器名称伪造能力；工具调用、usage、SSE 和终止事件必须来自真实协议转换 |
| 调度与缓存层 | 提高会话缓存亲和、账号选择质量和首响应体验 | 分组和账号能力约束优先；失败切换不能产生额外计费请求；TTFT 不得伪造 |
| 媒体任务层 | 图片/视频创建、轮询、恢复、计费和下载代理 | 创建最多一次；查询可重复；成功只扣一次，失败/取消/过期不扣费；公开响应只使用平台任务 ID 和代理 URL |
| 产品扩展层 | 可用渠道、Q 群入口、KeyingPay V2、独立积分与签到服务等 | 不改变官方核心数据语义；后台配置为准；敏感配置不进入公开 API 和仓库 |

## 3. 二开功能清单与代码入口

### 3.1 缓存与协议兼容

| 功能 | 当前行为 | 主要代码入口 | 关键回归 |
| --- | --- | --- | --- |
| OpenAI-compatible 缓存锚点 | 保留客户端显式 `prompt_cache_key`；没有显式锚点时，按 API Key、模型、稳定会话信息和请求前缀派生短哈希 | `backend/internal/service/openai_compat_prompt_cache_key.go`、`openai_content_session_seed.go` | `openai_compat_prompt_cache_key_test.go`、Chat/Responses/Anthropic 转换测试 |
| 提示词归属 | OpenAI APIKey 账号可显式开启 `extra.openai_upstream_relay`。开启后不再由平台重复注入默认 Codex 基础提示词；客户端显式 `instructions` 原样保留，上游内部提示词由上游负责。默认关闭，维持官方兼容行为 | `backend/internal/service/account.go`、`openai_gateway_forward.go`、账号创建/编辑前端 | `account_openai_passthrough_test.go`、`openai_gateway_service_hotpath_test.go`、账号前端测试 |
| Chat/Responses/Anthropic 工具流 | 保留 function/custom/freeform 工具、工具顺序归一、call id、thinking 和 terminal 事件 | `backend/internal/pkg/apicompat/`、`backend/internal/service/openai_gateway_*` | `chatcompletions_responses_bridge_*`、`openai_gateway_*_test.go` |
| 跨协议流终态与断流保护 | Responses、原生 Chat、Anthropic 转 Chat、Gemini 转 Chat/Messages、Responses WS v2 和 WS-to-HTTP bridge 只有收到各自正式成功终态才可成功；EOF、读取错误、上游 SSE/WebSocket error 和缺终态不能静默返回成功 | `backend/internal/service/openai_gateway_response_handling.go`、`openai_gateway_chat_completions*.go`、`gateway_forward_as_chat_completions.go`、`gemini_*_compat_service.go`、`openai_ws_http_bridge.go`、`openai_ws_v2/` 及对应 handler | `openai_gateway_chat_completions*_test.go`、`gateway_forward_as_chat_completions_test.go`、`gemini_*_compat_service_test.go`、`openai_ws_*_test.go` |
| OpenAI-compatible 账号 | 支持精确 Chat Completions URL、Responses/Chat 模式和可配置认证头 | `backend/internal/service/openai_compatible_auth.go`、账号配置与各 OpenAI 转发服务 | 账号探测、模型同步、Chat/Responses/Embeddings/Images 回归 |
| Codex 模型清单 | 带 `client_version` 的请求优先使用同组 OAuth/Setup Token 获取 ChatGPT manifest；APIKey 账号按官方代理路径获取并复用缓存、ETag 与 singleflight。APIKey-only 分组全部失败时返回合法空远程清单，由 Codex 合并内置目录；OAuth 凭据损坏仍报错 | `backend/internal/service/openai_codex_models_service.go`、`handler/openai_codex_models_handler.go` | `openai_codex_models_service_test.go`、`openai_codex_models_handler_test.go`、`gateway_codex_models_test.go` |
| 错误脱敏 | 用户仅看到稳定错误结构，内部日志保留不含凭据的诊断字段 | `backend/internal/service/upstream_error_sanitize.go`、各 gateway handler | `upstream_error_sanitize_test.go`、协议错误回归 |

缓存命中由请求前缀稳定性、模型、账号和上游缓存共同决定。网关只能提高亲和性，不能承诺每次请求都命中。禁止为了提高命中率改写用户正文、跨用户共享缓存键或固定到已不可调度账号。

### 3.2 智能调度与首响应

| 功能 | 当前行为 | 主要代码入口 |
| --- | --- | --- |
| 会话粘性 | 同一会话优先保持可调度账号，减少缓存失效；轻微速度波动和瞬时并发不会触发改绑 | `backend/internal/service/openai_account_scheduler.go`、`repository/scheduler_cache.go` |
| 速度画像 | 按账号、模型族、入站端点和上游端点维护成功 TTFT/错误率画像，避免失败样本、Chat 和 Responses 互相污染 | `openai_account_scheduler.go`、`handler/openai_account_schedule_profile.go` |
| 快账号优先 | 新会话在分组、模型、传输协议、能力和并发限制内优先综合分最高账号；只有分差不超过 2% 时才在近似账号间分流 | `openai_account_scheduler.go` |
| 粘性性能迁移 | 当前与候选在同模型/端点/协议下各至少有 3 次成功 TTFT，且候选至少快 1000ms、当前至少慢 1.75 倍时才迁移；先占候选槽位，再用 Redis CAS 原子改绑，CAS 不可用或冲突时保持原账号 | `openai_account_scheduler.go`、`openai_sticky_compat.go`、`repository/gateway_cache.go` |
| 故障 pool 逃逸 | 5xx、524、流读取失败或失败终态只在绑定仍指向故障账号时原子解除当前会话粘性，并软降权 2 分钟；不全局封禁，单候选仍可回落 | `openai_account_scheduler.go`、`openai_sticky_compat.go`、`repository/gateway_cache.go`、`handler/openai_account_schedule_profile.go` |
| 首响应记录 | `first_token_ms` 从最终成功 attempt 发出上游请求起，记录收到 2xx 响应头的最早真实确认时间；调度、排队和失败 attempt 不计入 | `handler/openai_first_response_failover.go`、`service/openai_first_response_*`、`service/openai_first_token_timing.go` |
| 运行时短熔断 | 只隔离明确异常的账号/能力组合，不能因为用户取消或媒体长任务等待永久封禁账号 | `openai_account_runtime_block_fastpath.go` |

52Token 的固定运行策略是 `openai_advanced_scheduler_enabled=true`、
`openai_advanced_scheduler_sticky_weighted_enabled=false`。官方粘性加权模式继续保留
用于通用产品兼容，但本部署不把 previous/session affinity bonus 叠加进账号分数；
新会话直接按智能调度综合分选择，已有会话绑定只承担缓存亲和，并在上述充分 TTFT
证据成立时原子迁移到更快账号。未来官方升级不得把粘性加权默认打开。

不得将“还没收到首字”视为“请求没有被上游接受”。文本请求的错误切换策略必须区分确定拒绝与状态不明；媒体创建一律使用更保守的边界。高级调度开关关闭时，新会话回到官方兼容选择流程；独立的 `sticky_escape_enabled` 只允许上述有充分成功样本且可原子改绑的性能迁移，关闭该配置即可恢复硬粘性。

同一文本请求只有 `401/402/403/404/429` 这类明确拒绝才允许执行最多两次跨账号重放；首响应等待、transport 异常、一般 `5xx/524`、流中断和一般 `response.failed` 都可能已经触发上游计费，不得在同一请求中自动重放。唯一新增例外是上游错误消息去除首尾空白后精确等于 `Selected model is at capacity. Please try a different model.`：该消息被视为生成前的明确容量拒绝，不依赖 HTTP 状态码；HTTP 错误、HTTP 200 内的 `response.failed` 或 SSE `error` 只要尚未向客户端输出任何语义正文，即可复用原请求体执行最多两次重放，并分别等待 `100ms`、`200ms`。调度先选择其他账号；没有其他候选时可在同一账号上使用剩余预算，故单账号组最多为首次请求加两次重放。该请求级错误不得降低账号健康分、写入运行时 block 或触发账号冷却。近似文案、大小写变化、附加前后缀、媒体创建请求及已经输出正文的流均不适用。每次容量重试写入 `openai.model_capacity_retry_scheduled` 结构化日志，但不记录请求正文。pool 上游发生其他故障后，当前请求返回真实错误；下一次客户端重试会在绑定仍指向故障账号时原子解除旧粘性，并在 2 分钟软降权窗口内优先选择健康账号。并发请求已经写入的新绑定不会被旧失败请求删除。软降权只改变排序，不改变 `schedulable`，因此不会造成单账号分组无可用账号。

响应成功不能由 HTTP 200、EOF 或通用 framing 哨兵推断，必须满足当前协议的正式成功终态：Responses 为 `response.completed`/`response.done`；原生 Chat 非流式每个 choice 必须有 `finish_reason`，流式必须观察到 `finish_reason` 和 `[DONE]`；Anthropic 转 Chat 必须收到 `message_stop` 或带 `stop_reason` 的 `message_delta`；Gemini 转 Chat/Messages 与 Gemini native 流必须收到 `finishReason`；Responses WS v2 和 WS-to-HTTP bridge 必须收到正式 Responses 成功终态。`response.failed`、`response.incomplete`、`response.cancelled`/`response.canceled`、Anthropic/Gemini/OpenAI SSE `error` 和 WebSocket `error` 都是显式失败，不得改写为成功。

所有上述入口统一遵守以下断流边界：

1. 上游 EOF、scanner/WebSocket 读取错误或缺正式终态时，尚未向客户端输出语义正文的请求进入既有安全故障切换判断；已经输出部分正文时在仍可写的连接上发送当前下游协议的显式错误，禁止伪造 `[DONE]`、`finish_reason=stop`、`message_stop` 或其他成功终止事件。
2. 正式上游错误必须继续走原有 HTTP/错误事件分类、账号健康、粘性释放和故障切换规则；本修复只关闭静默成功路径，不把一般 5xx、读错误或部分输出重新定义为可安全重放。
3. 客户端断开后关闭所有重放窗口和下游写入，但服务端继续 drain 上游终态与 usage，供真实消费计费收口。结果必须标记 `ClientDisconnect`；后续 EOF、容量错误或读取错误不得因此触发第二次上游请求。断开日志记录请求、用户、分组、账号、模型、上游 request ID 和阶段，不记录正文、cookie 或凭据。
4. Responses 的 `response_id` 账号绑定继续保留 trace 值，并使用不继承客户端取消信号的上下文完成，避免连续会话因客户端先断开而丢失账号亲和。

TTFT 起点必须设置在最终选中账号实际发出上游请求之前。系统设置 `openai_first_response_enabled=true` 时，最终 attempt 收到上游 2xx 响应头即表示上游已经接受并正常开始响应，`first_token_ms` 在此处结束；关闭时恢复首个真实语义输出口径。4xx/5xx、transport error、调度排队和之前失败 attempt 不得留下快值。该值是首响应指标，不代表首段可见文本；`duration_ms` 仍保留从客户端请求进入到完成的完整耗时。禁止发送本地伪造 token、提前结束请求或修改计费记录来缩短指标。

`openai_upstream_relay` 与全局 TTFT 开关完全独立，只控制默认提示词归属。开启后可以避免与上游内部提示词重复、缩短请求前缀并减少输入计费；显式 `prompt_cache_key`、`session_id`、客户端提示词、工具转换、模型映射、usage 和计费不变。该账号首次切换后缓存前缀可能冷启动一次，后续会话粘性不受影响。

### 3.3 可用渠道与产品入口

- 用户可用渠道的模型来源是渠道定价配置与用户可访问分组，不是账号同步得到的全部模型。
- 后端聚合入口：`backend/internal/handler/available_channel_handler.go`、`backend/internal/service/channel_available.go`。
- 前端入口：`frontend/src/components/channels/AvailableChannel*.vue`、`SupportedModelChip.vue`、`TablePageLayout.vue`。
- 模型列表必须支持换行/展开/纵向滚动，桌面和移动端都不能依赖整页横向拖动查看完整模型。
- Q 群链接来自系统公开设置，入口位于 `frontend/src/components/layout/AppHeader.vue`；不得硬编码群号或把管理端未公开设置泄露给未登录用户。

### 3.4 图片生成

对外契约详见 [媒体 API 契约](MEDIA_API_CN.md)。主要代码入口：

- 路由与请求处理：`backend/internal/server/routes/gateway.go`、`handler/openai_images.go`、`handler/openai_image_tasks.go`。
- 请求解析、转发和响应兼容：`service/openai_images.go`、`service/openai_image_tasks.go`、`service/openai_image_delivery.go`。
- 任务持久化：`repository/media_generation_task_repo.go`、`service/media_generation_task.go`。
- 计费：`service/openai_gateway_usage.go`、`billing_service.go`、媒体定价快照。

同步和异步都是显式模式。同步失败不得自动改成异步再创建一次；异步创建必须使用幂等键，后续只查询已有任务。返回 URL 必须改写为平台代理地址或转换为可直接交付的图片数据。

Cloudflare 橙云兼容分两类：OpenAI Images 同步 JSON 可通过 `gateway.image_nonstream_keepalive_interval` 发送前导空白保活，默认 `0` 表示关闭，生产建议先以 `10` 秒完成模拟慢请求回归再开启；Gemini 工作台固定调用 `streamGenerateContent?alt=sse`，服务端和客户端都必须忽略 `:` SSE 注释心跳并聚合分块图片。第三方客户端若继续调用非流式 `generateContent`，不能承诺超过边缘无响应超时的长任务。

余额计费用户在调用上游前按创建时渠道价格快照冻结预计费用，原子地从可用余额转入冻结余额；并发创建不能透支。余额不足必须在上游请求前返回 `402`。成功 usage 与冻结结算在同一数据库事务中完成并退还差额；明确 `4xx`、结构化失败/取消和供应商明确失败立即释放，普通 `5xx`、超时、transport error 或无终态响应继续保留冻结等待核销，且不得重放媒体创建。后台核销服务启动后立即执行并每分钟扫描全站到期冻结：只有 `capture_pending` 或成功任务证据才能扣费，其余到期冻结退款；单用户失败不阻塞其他用户，提交后必须失效余额缓存。当前生产把同步 `/v1/images/generations` 未知终态窗口收敛为 30 分钟，异步图片、视频和其他媒体任务继续使用 24 小时。成功媒体最终计算费用高于创建时报价时按原冻结报价封顶，不能二次追扣。订阅分组和 simple mode 保持原有计费行为。

当前代码有四种图片调用模式，升级和排障时不得混为一套：

| 模式 | 公开入口 | 状态事实源 | 重启恢复与计费边界 |
| --- | --- | --- | --- |
| 同步图片 | `/v1/images/generations`、`/v1/images/edits` | 当前 HTTP 请求 | 不创建后台任务；可选 JSON 空白保活不能伪造结果 |
| 私有 provider-native 异步 | 请求体 `async:true`，随后查询原图片路径的公开任务 ID | PostgreSQL `media_generation_tasks` 与 `media_balance_holds` | 固定创建账号，可恢复轮询；发送后不重放，成功一次计费 |
| 官方后缀异步 | `/v1/images/generations/async`、`/edits/async`、`/v1/images/tasks/{id}` | Redis `image_task:*`，结果写 S3/R2 | 进程内 goroutine；容器重启不能恢复正在处理的请求，升级前必须确认无在途任务 |
| 内置批量图片 | `/v1/images/batches*`，前端 `/batch-image` | PostgreSQL `batch_image_*`，Redis只做队列/锁 | Gemini Files 或 Vertex GCS；成功图片结算，worker需同时开启 `enabled` 和 `queue_enabled` |

生产 `image` 域名上的画布工作台是独立服务，不属于本仓库，也不是内置 `/batch-image` 页面。其唯一源码来源是 [`hxly520/infinite-canvas`](https://github.com/hxly520/infinite-canvas)；服务器旧 `images` 目录不得再用于构建或页面回退。运行身份、构建版本、API Base 和日志边界见 [生产运维文档](PRODUCTION_OPERATIONS_CN.md)。

### 3.5 视频生成与媒体代理

- 统一公开接口：`POST /v1/videos`（兼容 `POST /v1/videos/generations`）、`GET /v1/videos/{task_id}`、`GET /v1/videos/{task_id}/content`。
- 官方 `v0.1.153` 新增 `POST /v1/videos/edits` 和 `POST /v1/videos/extensions`。当前只对支持该能力的视频分组开放，仍复用统一任务查询、余额预留、账号粘性、成功一次计费和媒体代理流程。
- 兼容路径只在入口解析，受支持的视频模型最终走统一任务流程。任务查询必须复用创建时保存的账号、端点和供应商任务 ID。
- 主要入口：`handler/openai_videos.go`、`service/openai_videos.go`、`service/video_billing_resolution.go`、`repository/media_generation_task_repo.go`。
- 用户仅看到本地公开任务 ID。供应商任务 ID、原始响应地址和账号 Base URL只保存在服务端受控字段。
- 完成后的图片/视频 URL 由 `gateway.video_proxy` 生成短期加密地址；Edge 模式实现位于 `deploy/video-edge-worker/`，Nginx 下载域名示例位于 `deploy/nginx/video.52token.org.conf.example`。
- Edge Worker 只允许 `/v1/video-content/*` 和 `/v1/image-content/*`，支持 `GET`、`HEAD`、Range 和有限跳转；其他路径必须拒绝。
- 52Token 的 `image` 橙云、`api` 长流式入口、`long` 退役、两条 WAF、Cache Bypass 和源站隔离步骤统一记录在 `deploy/CLOUDFLARE_52TOKEN.md`；不要再从历史聊天规则拼接生产表达式。

视频创建成功返回任务后，客户端应长轮询同一个任务。轮询中断只暂停查询，不得重新创建。只有终态成功且媒体结果可用时记录一次 usage；重复查询和重复下载不得再次计费。

### 3.6 独立积分与签到系统

- 源码边界：`points-system/` 是独立 Go 服务、独立 PostgreSQL schema 和独立镜像；Sub2API 只负责菜单入口、一次性启动票据和幂等余额发放桥接，不在浏览器计算积分、消费额或奖励金额。
- 产品口径以 `points-system/PRODUCT_REQUIREMENTS_CN.md` 为唯一事实源。积分是成功余额消费换算的只读统计值，不提供积分兑换余额或订阅结算；默认 `1 U = 10.00` 积分，比例支持两位小数并只影响生效后的新消费。
- 每日快照默认在 `Asia/Shanghai 00:05` 汇总上一自然日，可通过次日生效的版本化策略调整。生产事实源必须是只读查询 Sub2API `usage_logs` 中 `billing_type=0`、`actual_cost>0` 的成功余额消费，浏览器上报不能作为消费事实。
- 个人消费积分账本的“发放时间”字段为 `awarded_at`，按业务日期展示：`kind=usage_points` 且具备 `business_date` 时，取 `business_date + 1` 发放自然日零点加该发放日实际生效策略的 `refresh_minute`，时区固定 `Asia/Shanghai`；正常策略切换日不得沿用消费日或账本绑定策略的刷新时间，当前默认显示次日 `00:05`。只有发放日早于首个生效策略的历史回算行，才回退该账本固定 `policy_version` 的刷新分钟；非消费、旧版或同时缺少发放日/账本策略的记录回退 `created_at`。不得为展示时间改写不可变账本。
- 昨日汇总未成功完成时签到必须返回可重试的准备中状态，不得把缺失快照当作零积分命中首个阶梯，也不得占用当天签到次数。
- 签到总开关、每日次数、全体/仅消费用户、昨日/总积分依据均由管理员配置。仅消费用户模式必须锁定昨日积分；最低签到消费金额始终按昨日成功余额消费判断，`0 U` 表示关闭额外门槛。
- 每个积分阶梯只能选择固定余额区间或消费金额百分比区间。百分比基数使用对应统计周期的原始成功消费金额，不得用积分反推；所有金额向下量化到 `0.01 U`，随机值来自 CSPRNG，用户页面只显示实际中奖金额。
- 用户工作台同时显示总积分、昨日积分、今日签到所得和累计签到赠送金额。累计值只汇总已经成功发放且未冲正的签到余额，不能把 pending、永久失败或已冲正金额用于展示。7/30/90 日趋势按今天之前的连续完整自然日查询，缺失快照补零，日均固定除以自然日天数；个人积分与签到记录使用绑定用户的签名稳定游标分页，插入新记录不得在页间造成重复或遗漏。
- 用户工作区 `/app/` 与管理员工作区 `/admin/` 是两套中文页面和脚本。用户端只提供总积分、昨日积分、今日/累计签到赠送、7/30/90 日积分趋势和个人记录；管理员端提供当前策略配置、全站用户积分明细以及签到发放任务的查询、重试和审计冲正。管理员用户明细翻页在请求期间锁定按钮，成功后才提交 offset，失败时保留原页数据和页码；管理员发放记录使用会话绑定的签名稳定游标分页，不得只加载最新 100 条。发放概览使用服务端对全部 `checkin` 任务的状态聚合，冲正永久失败必须进入失败告警。积分系统不提供手工余额赠送或手工快照刷新入口，直接调余额继续使用 Sub2API 原有管理能力，每日快照只由调度器自动结算。普通用户必须同时被页面、管理员脚本和 `/api/v1/admin/*` 权限校验拦截，管理员也不得调用用户账户、积分、签到或个人赠送 API；除共用退出外均按精确角色返回 `403`。
- 用户和管理员积分工作区继续使用独立 HTML、脚本、路由和双向精确角色授权；视觉变量复用 Sub2API 的浅色中性层级、深色 `dark-950/dark-800/dark-700` 层级和 teal 主色。用户页采用四张紧凑等高指标卡、宽幅 7/30/90 日趋势和两张独立分页明细表（默认每页 10 条）；签到卡使用内容自适应网格放置按钮，禁止恢复绝对定位。管理员页采用同一设计语言下的紧凑运维工作区，视觉改版不得合并角色页面、引入第二套全局主题或放宽 API 权限。表格行、悬停、分页按钮、状态标签和 Canvas 曲线必须随主题切换保持可读。
- Taste 设计基线固定为 `DESIGN_VARIANCE=6`、`MOTION_INTENSITY=4`、`VISUAL_DENSITY=5`：这是嵌入 Sub2API 的运营与用户积分工作区，不按营销落地页处理。用户页用主积分焦点卡、低对比精密网格、真实同步状态和紧凑结算说明增强数字感；所有指标卡与工具面板沿用 8px 圆角，悬停最多上移 2px，并在 `prefers-reduced-motion` 下完全取消。继续复用现有 CSS token、受许可的 Lucide sprite 和原生 Canvas，不因装饰引入第二套组件运行时；若后续确需新组件，必须先证明现有体系无法满足功能、主题或无障碍要求，并保持全项目只使用一个设计体系。Canvas 绘制宽度必须等于真实 CSS 宽度，日期刻度按约 `88px` 可读间距动态收敛且最多 6 个；`390px` 视口应显示 3-4 个互不重叠日期。普通表格 hover 只使用轻量主色提示，不能模拟选中态；签到赠送四列表使用独立较小最小宽度。管理员待处理任务采用 amber 语义层级，奖励阶梯在策略表单内使用分隔行，禁止恢复卡片套卡片。
- 用户首轮资料、趋势或任一必需明细请求失败时，不得继续展示默认 `0` 和空表冒充真实业务状态；积分页必须显示持久错误面板与明确“重新加载”，同时仍通知父 iframe 揭示该可操作失败。用户明细游标失效时必须自动重置到第一页，禁止反复提交同一个失效游标。管理员策略台在策略、用户和发放汇总首轮加载完成前保持加载页，不得先显示“未启用/0/-”占位；首轮请求失败时保留管理应用并提供原地“重新加载”，重试不得重复绑定事件或新增重复阶梯。管理员新标签等待文档必须按打开瞬间的 Sub2API 浅深主题设置 `color-scheme`、`theme-color`、语言、`aria-busy` 和 reduced-motion 进度状态，同时保留 `opener=null` 与 `no-referrer`。
- 积分界面中所有可见身份统一显示 Sub2API 登录邮箱，包括用户页顶部身份、管理员页顶部身份、全站用户积分明细首列和签到余额发放任务的用户列。数值用户 ID 只保留在服务端账户关联、财务记录、审计和幂等键中；浏览器 API 需要标识身份时返回 `login_email`，不得额外暴露无展示或操作用途的 `user_id`。所有积分浏览器 API 使用 `20` 秒客户端超时并清理计时器；用户/管理员刷新按钮必须在异步成功、失败和超时后恢复。签到在网络结果不确定或 POST 已成功但资料确认尚未返回时，必须在当前浏览器会话持久化原幂等键、登录邮箱、业务日期和签到前次数；权威资料确认次数增加后才能清除，未增加时显示“确认签到结果”并只允许用同一幂等键重放，禁止生成第二个奖励请求或永久锁死按钮。
- 单次、单用户每日、全平台每日奖励上限均为管理员配置。签到资格读取、次数和金额占用、规则快照、审计记录及余额发放事务发件箱必须在同一串行化事务中完成。
- 积分业务写表复用现有 `sub2api` 数据库，但固定在独立 `points` schema，写池默认最多 8 条连接；消费读取使用另一只读账号和最多 4 条连接。启动时 `current_schema()` 不匹配必须失败，禁止回退到 `public` 建表。
- 新装时 `points-system/deploy/shared-database-bootstrap.sql.example` 对 `public.users` 只授予 `SELECT (id, email, deleted_at)`；不得为了显示登录邮箱扩大到整表 `SELECT`。存量环境不得重跑首次 bootstrap，必须先完成数据库备份和积分账户/快照/账本、Sub2API 用户计数，再以 root-only stdin 传入既有 `points_app_role`。阶段 A `shared-database-users-email-upgrade.sql.example` 只增加 `SELECT (email)` 并保留 `username`，断言兼容态精确为 `id/email/username/deleted_at`；切换并验收新积分镜像后，阶段 B `shared-database-users-email-finalize.sql.example` 才撤销 `username` 并断言最终 `id/email/deleted_at`。若需回滚，先运行 `shared-database-users-email-rollback-prepare.sql.example` 恢复双读，切回旧镜像验收后再运行 `shared-database-users-email-rollback-finalize.sql.example` 撤销 `email`。四个模板都不修改 PUBLIC ACL，不允许 `WITH GRANT OPTION`，角色、表、列、整表/额外列/写权限或前置状态任一不满足都整体回滚。历史 username 模板及事务 `1914532` 禁止改写或重跑。
- 业务拒绝按用户、自然日和拒绝原因收敛，恶意轮换幂等键不能无界增加财务表。余额 credit 的网络、超时或 5xx 结果属于未知状态，必须用原 UUID 重试确认到账后才能排队 debit；不得直接标为已冲正。
- Sub2API 与积分服务使用 Base64 编码的版本化 HMAC 密钥。启动票据为 `key_id.payload.signature` 三段格式并绑定 `aud=points-system`；余额请求签名绑定 key ID、方法、路径、时间戳、交易 UUID 和请求体摘要，同一交易只能入账一次。
- 用户入口：Sub2API `/points`；管理员入口：系统设置内的“积分系统”标签及独立路由 `/admin/settings/points`。这两个入口是内置菜单，不得再把积分域名配置成普通自定义菜单。用户页继续在 Sub2API 右侧内容区加载受票据保护的 iframe，左侧导航、Header、主题状态和上传的 `site_logo` 始终由 Sub2API 提供；父子页通过校验 `source` 与精确 Origin 的消息实时同步明暗主题，积分内页在 `ui_mode=embedded` 下隐藏重复品牌和退出入口，且不得给官方导航附加积分主题类。管理员页不再嵌入设置页底部：管理员经 step-up 点击“打开积分配置”后，以隔离 opener/referrer 的新浏览器标签页打开一次性票据保护的独立策略台，原 Sub2API 页面和状态保持不变；浏览器拦截新窗口时必须显示明确失败状态。管理员入口不依赖用户开关或预览白名单。积分域名根路径不提供工作台，直接访问 `/app/` 或 `/admin/` 没有积分会话时必须拒绝。
- 历史预览阶段采用双服务白名单：Sub2API `points_system.enabled=false` 且 `preview_user_ids: [1]`，积分服务 `POINTS_USER_ACCESS_MODE=preview` 且 `POINTS_USER_PREVIEW_IDS=1` 时，仅用户 ID 1 显示积分菜单、通过 `/points` 路由、获得 user ticket 并继续使用积分 user session；其余用户即使依赖陈旧公共设置、手工构造路由/launch 请求或持有收窄前旧 cookie，也必须被拒绝并清除积分 cookie。当前生产已切换为两端全体模式，用户可见性只由当前生效策略的 `enabled` 控制。两份完整白名单不得下发浏览器，认证接口只返回当前用户专属的 `points_system_access` 布尔值。
- 积分独立页和管理页的品牌位必须加载 `POINTS_EMBED_PARENT_ORIGIN/api/v1/settings/logo`，不得再显示“积”或“管”等文字占位。服务端只把精确父站 Origin 加入 CSP `img-src`；父站 Logo 请求失败时前端仅回退到积分镜像内、受积分会话保护的 `/assets/logo.svg`。该图片来源规则不改变 iframe Origin、启动票据、session 或角色权限。
- 普通用户开放必须同时满足 Sub2API 的部署授权与积分服务当前生效策略完整启用。部署授权为“全局 `enabled=true`”或“全局关闭且当前用户在 `preview_user_ids`”二选一；积分服务继续在 user ticket、页面、静态资源和 API 四层执行关闭态校验，旧会话与手工 URL 不能绕过。当前生产 policy v3 已启用、历史回算已成功且签到关闭，两端部署门禁为 `enabled=true`/`all` 且预览名单为空；关闭策略的 `enabled` 后，普通用户入口自动隐藏并拒绝。
- 全历史回算是当前 `points` schema 的一次性已完成操作，成功作业为 `5174eef7-5f0a-4a17-b4f1-f50840940f64`。后续不得再次创建或应用历史计划；日常只允许调度器按版本化策略自动对账，迟到消费在滚动窗口内按原日期固定比例修订。
- Sub2API 余额缓存使用 Redis 用户代次保护回源写入：余额失效或扣减必须原子推进代次，旧数据库读取只能在代次未变化时回填。积分 credit 已提交但缓存失效失败时返回可重试 `503`，积分发件箱用原 UUID 重试，不能把未完成缓存同步的交易标为 settled。
- `/api/internal/points/credits` 只允许容器网络访问，公网 Nginx 必须精确返回 `404`，应用内部再执行 fail-close 限流与 HMAC。积分 Nginx 仅对含票据的 `/launch` 关闭访问日志，其余拒绝、越权和限流请求必须保留边缘证据。
- 主要 Sub2API 入口：`backend/internal/service/points_bridge.go`、`repository/points_bridge_repo.go`、`handler/points_handler.go`、`server/routes/points.go`、`frontend/src/views/user/PointsPortalView.vue`。
- 主要积分服务入口：`points-system/internal/domain/`、`store/`、`worker/`、`httpapi/`、`sub2client/` 与 `internal/migrate/migrations/`。

后续官方升级不得把积分余额发放改成前端请求参数直写用户余额，不得恢复已删除的积分兑换流程，也不得用浮点数处理积分比例、消费金额、百分比或奖励金额。

### 3.7 支付扩展

- KeyingPay V2 provider：`backend/internal/payment/provider/keyingpay.go`。
- 工厂、路由、配置和前端入口：`payment/provider/factory.go`、`server/routes/payment.go`、`handler/payment_webhook_handler.go`、`frontend/src/components/payment/`。
- 部署说明：`deploy/KEYINGPAY_V2.md`。

支付回调必须验证签名并保持幂等。创建、回调、主动查单、退款、退款查询和关闭订单要复用 Sub2API 原有订单状态机，不能绕过平台订单表直接加余额。

### 3.8 公开页面、导入与边缘部署

- CC Switch API Key 导入兼容：`backend/internal/service/ccswitch_import.go`、`frontend/src/utils/ccswitchImport.ts`。
- 私有公开首页和帮助页：`deploy/public-landing/`、`deploy/public-help/`；公开内容必须经过本地净化，不得把内部 API 或管理入口暴露到静态域名。
- Vue `/home` 和 exact-root 静态首页只使用中性功能、稳定性和管理文案，不出现具体国外模型或商业中转宣传名称；本次仅修改未登录首页，登录后 Dashboard 不得随首页迭代改变。两套入口必须同步采用生图参考稿确定的冷灰/电蓝数据化风格、左文右图主视觉和一致的信息层级，不能只改 Vue 页面后把生产根路径遗留为旧版。
- 上传 Logo 的同源图片入口为 `GET /api/v1/settings/logo`：只接受后台 `site_logo` 中不超过 2 MiB 且真实文件签名一致的 PNG/JPEG/WebP/GIF Base64 Data URI，响应设置 `nosniff`；后台上传控件同步只接受这四类栅格格式。异常值、伪造类型、SVG、AVIF 或未配置时重定向到 `/logo.svg`。公开首页和积分嵌入页统一使用该上传 Logo，不复制品牌素材。
- 生产 Nginx 对 `/` 和 `/index.html` 使用 exact location，从宿主 `/home/api/sub2api-deploy/public/index.html` 提供公开首页；该文件不在 Sub2API 容器层中，单独切换镜像不会更新它。`frontend/src/views/HomeView.vue`、其主视觉资源和 `deploy/public-landing/index.html` 必须作为同一次首页改版审查；静态 exact-root 应保持单文件自包含或把新增资产纳入原子发布清单。每次首页发布必须先运行 `publicStaticPages.spec.ts`，备份宿主旧文件后原子替换，并核对本地文件、宿主文件和线上响应 SHA256 一致；不需要重启 Sub2API。
- Cloudflare/Nginx 边界：`deploy/CLOUDFLARE_52TOKEN.md`、`deploy/CLOUDFLARE_ABUSE_REMEDIATION.md`、`deploy/nginx/`。
- 图片/视频 Edge Worker：`deploy/video-edge-worker/`；源站 Nginx 对媒体域名返回 404，只有 Worker 精确接管加密内容路径。
- 二开镜像发布：`.github/workflows/cachecompat-image.yml`；版本必须来自源码 VERSION，镜像必须同时记录完整 commit 与 digest，默认不发布 `latest`。

## 4. 数据与配置约束

### 4.1 数据库

- 媒体任务、公开任务 ID、定价快照、计费终态和幂等信息由迁移维护。
- 私有已发布迁移必须全部保留：`173_media_generation_tasks.sql`、`174_media_generation_task_public_ids.sql`、`175_openai_first_response_settings.sql`、`176_media_generation_finalization_recovery.sql`、`177_media_generation_pricing_snapshot.sql`、`178_media_balance_holds.sql`、`179_media_balance_hold_dispatch_state.sql`。
- `192_media_balance_hold_reconciliation_index_notx.sql` 是 `v0.1.168` 新增的非事务并发索引迁移，服务于全站到期冻结扫描，当前已进入生产。后续发布必须验证迁移执行器继续按 `_notx` 语义运行，并确认旧 `173-179` 文件及 checksum 不变。
- `193_points_balance_credit_ledger.sql` 是 Sub2API 侧积分余额幂等入账账本，当前已进入生产；积分服务自己的迁移位于 `points-system/internal/migrate/migrations/`，在同一数据库的独立 `points` schema 使用独立迁移表和最小权限角色。两套迁移不得混放、重编号或绕过事务发件箱直接改余额。
- 积分角色读取 Sub2API 用户表必须保持列级 allowlist：内部关联用 `id`、界面登录邮箱用 `email`、过滤软删除用 `deleted_at`。阶段 A 允许短期精确双读 `id/email/username/deleted_at`，仅用于新旧积分镜像兼容切换；新镜像验收后必须由阶段 B 收敛为 `id/email/deleted_at`。任何新增展示字段都必须先经过数据最小化审查，禁止把用户表整表授权给积分角色。
- 官方后续存在相同数字前缀的其他迁移；runner按完整文件名排序并以完整文件名作为主键，因此可以共存。已经进入生产数据库的私有迁移禁止重命名、删除或修改 checksum。
- `178_media_balance_holds.sql` 创建原子媒体冻结记录；`179_media_balance_hold_dispatch_state.sql` 只扩展发送态过期索引。
- 修改 `backend/ent/schema/` 后必须重新生成 Ent 文件，并提交 schema、生成代码、SQL migration 和回归测试。
- 新迁移必须可重复启动，不能依赖某个生产账号、分组或固定主键。
- 升级前必须验证旧数据库迁移到新版本；回滚应用前确认新迁移是否向后兼容。

### 4.2 运行时配置

重要配置示例统一保存在 `deploy/config.example.yaml`：

- OpenAI 高级调度、首响应阈值、粘性逃逸和评分权重。
- 图片流超时、keepalive、图片并发和媒体任务设置。
- `gateway.video_proxy` 模式、公开地址、令牌时效和加密密钥。
- URL allowlist、CORS、请求体大小和错误切换边界。
- `points_system` 启用状态、仅服务端保存的 `preview_user_ids`、公开启动 URL、启动/余额 HMAC key ID 与 Base64 secret、票据 TTL 和时钟偏差；积分服务还必须配置一个无路径、无通配符的 `POINTS_EMBED_PARENT_ORIGIN`，只允许实际 Sub2API 浏览器 Origin 通过 CSP `frame-ancestors` 嵌入用户页。真实密钥和完整预览白名单只在部署环境保存。
- 积分服务的同库 `points` schema、最大 8 条写连接、Sub2API `usage_logs` 最多 4 条只读连接、可信 Nginx CIDR、公开 origin、会话密钥和服务间 HMAC keyring；消费数据账号只授予所需列的 `SELECT` 权限。

仓库只提交字段说明和占位值。真实密钥、账号地址、数据库连接、支付私钥、Worker Secret、SSH/GitHub 凭据不得提交。

## 5. 官方版本升级流程

### 5.0 v0.1.169 合并兼容结论

- 合并提交 `3da18b9dd2d0ecc890a5605a4d1cf97093a8659e` 只合入官方 `v0.1.169` Release commit `26d894ef4f50645a4bf1030e378ac892f17d0223`。冲突仅涉及可用渠道复合分组测试和 OpenAI 账号调度器：复合分组保留官方测试与私有可见模型/隐私边界；调度器采用官方带 context 的代理熔断 fail-open，同时保留私有模型级运行时阻断、sticky/画像选择和单候选降级逻辑。
- 官方新增 Gemini/xAI 上游路径片段校验、Responses 子路径守卫、代理流熔断事件折叠与无候选 fail-open、容器 `no-new-privileges`、运行时价格资源 fallback、邮件正文处理、订阅配额显示修正及安全审计更新。Compose 中的积分环境变量、非 root 运行、私有迁移和媒体配置均保留。
- 官方价格资源同步 GPT-5.6 Terra/Luna 并增加 GLM-5.2 fallback。渠道自定义价格仍优先于官方 fallback；价格变化只影响升级后新产生的成功消费及其积分换算，不回写历史消费、快照或积分账本。
- 合并审查补上两个边界：`PlatformComposite` 的 `SupportedModelsByGroup` 必须包含渠道全部已配置具体平台模型；Gemini Chat/Messages/native 流处理完带 `finishReason` 的 parts 与 usage 后立即完成，不等待上游 EOF。任一下游写失败都显式启动断连排空 guard，默认最多 5 秒后关闭上游 body，避免永久占用账号并发槽。
- 私有媒体冻结和核销、精确容量拒绝重试、Responses/Chat/Anthropic/Gemini/WS 正式终态、积分同库隔离、登录邮箱展示与软删除会话失效、管理员/用户双工作区、未登录首页和上传 Logo 均不得被官方更新覆盖。签到继续关闭，历史积分作业不得重跑。
- 合并后的后端 unit/default 全量测试、`go vet`、`go build`，积分服务 test/vet/build，前端 ESLint/typecheck/全量 Vitest/production build 均已通过。本轮 GitHub 镜像工作流因账户计费门禁未获得 runner，已按受控本机构建回退完成镜像层内容、revision、SHA256、GHCR digest、服务器 `docker load` 和隔离冒烟；后续发布仍优先使用 GitHub 工作流，runner 不可用时必须沿用同等校验的受控回退流程并记录不可变 digest。

### 5.0.1 v0.1.168 历史合并结论

- 合并提交 `d30c42da` 只合入官方 `v0.1.168` Release tree，不合入 tag 之后的未发布 `main`；其后私有提交 `9f1b6bae`、`e4179147`、`55ac503b`、`d83ea1bb`、`7e598fbb` 分别承载媒体核销、积分、同库部署、跨平台测试稳定性和未登录首页。官方新增 Passkey、模型广场、OpenAI Live、Kimi K3、账号/API Key 声明列更新和多项协议、计费及安全审计修复。
- 冲突处理以官方结构和行为为主，同时保留私有 Codex APIKey-only 合法空清单兜底、当前 TTFT 口径、协议/缓存兼容、媒体余额预留与核销、统一视频接口、平台代理 URL、KeyingPay V2、Q 群入口和可用渠道展示。
- 账号选择使用高级智能调度但关闭粘性加权。新会话按综合分选择；已有会话只有在同画像成功样本证明候选显著更快时才通过 Redis CAS 迁移。迁移不发送探测请求，不增加上游调用和计费。
- OpenAI Images JSON 保活和 Gemini SSE 心跳兼容继续保留；Gemini 工作台改为流式聚合，不依赖超过 120 秒的无字节非流式响应。
- 文本只有明确未受理的 `401/402/403/404/429` 可执行既有有界切号；5xx、超时、断流和一般失败终态不自动重放。精确容量拒绝文案是唯一额外例外，按 3.2 节的原文匹配、未输出正文、最多两次及 `100ms/200ms` 退避执行。图片和视频创建继续严格一次提交。
- Responses、原生 Chat、Anthropic 转 Chat、Gemini 转 Chat/Messages、Responses WS v2 和 WS-to-HTTP bridge 都不得把 HTTP 200、EOF 或 framing 哨兵当成成功终态；缺正式终态时按 3.2 节区分“未输出正文进入安全故障切换判断”“已输出正文发送协议显式错误”和“客户端已断开继续 drain usage、禁止重放”。升级合并必须保留三条路径及其测试。
- Composite 公开模型、路由模型和上游实际模型必须分离：用户计费与任务查询使用公开模型，账号选择使用路由模型，转发使用创建时保存的上游模型；图片异步和视频所有查询/内容路径都必须遵守这条边界。
- 当前生产 Sub2API 为 `0.1.169-f79803bb73d6`，积分服务为 `0.1.169-bee059a1cec5`；Sub2API 仍由维护者手工切换，自动化不得替换或重启。积分已执行阶段 A 并运行登录邮箱/Taste 候选，ACL 当前为 `id/email/username/deleted_at` 双读兼容态；阶段 B 必须等用户 1 与管理员真实票据验收后再收敛为 `id/email/deleted_at`。policy v3 仍为比例 `10.00:1`、刷新 `00:05`、签到关闭；历史作业仍为原成功记录。当前事实见 [`PRODUCTION_DEPLOYMENT_20260731_CN.md`](PRODUCTION_DEPLOYMENT_20260731_CN.md)，`2026-07-30` 文档只作为历史发布记录。
- 当前生产 Sub2API GHCR digest 为 `sha256:efeac9309f33a9ccd204932ed1144955641a78260982c3041842fb89c71caa49`；当前生产积分 GHCR digest 为 `sha256:4b18317d48783df966cf570e7bf7aa59b8261561d280de161ace137b41509746`。积分 loaded image ID 为 `sha256:0b8a4732a17bce4ce9ff513f10c0599e736dd8ba69f16d4438f178cf8d96e9a9`，服务器 archive SHA256 为 `2947440cfdf4dedd6bea09341969e99d9dfd49ef54c2739269e2831813a66925`。下一 Sub2API 手工候选 digest 为 `sha256:235add58b97e2f3c6bf06a65be86018dee4fac13769c280f1a508096f3bac78b`，但缓存或 archive 存在不等于生产已切换。image ID、传输 archive SHA256 和运行容器 revision 必须继续分别核对，不能把任一值当作另一值。
- 后端全量测试、vet、编译，积分服务单元/vet/integration-tag 编译，前端 lint/typecheck/全量测试/生产构建和 Compose 解析已通过。官方纳秒 TTL 测试已在 `d83ea1bb` 改为显式过期快照，Windows 连续 20 次验证通过；真实 PostgreSQL 事务和并发用例必须由 GitHub PostgreSQL 16 CI 最终确认。

### 5.1 升级前盘点

1. 记录生产镜像 tag、commit、digest、容器健康、数据库版本和上一稳定回滚镜像。
2. 确认当前私有分支工作树干净，列出所有 worktree 和未合并分支。
3. `git fetch` 官方与私有远端，记录官方新 tip 和 `backend/cmd/server/VERSION`。
4. 验证 `git rev-parse --is-shallow-repository` 为 `false`，`git rev-list --objects --all --missing=print` 无缺失对象，并完成 `git fsck --full`；浅克隆不得继续升级。
5. 使用 `git merge-base`、`git range-diff` 和 `git diff --stat` 区分官方变化与私有补丁；不要用文件覆盖方式升级。
6. 先阅读官方 migration、路由、协议桥、调度、计费和前端渠道展示变更，再决定 merge、rebase 或重新移植。

### 5.2 合并策略

1. 从当前权威私有 `main` 创建 `codex/upgrade-vX.Y.Z-*` 分支，再合并目标官方 Release tag；不得从官方 tip 新建生产候选后遗漏二开。
2. 保留当前私有行为和测试；只有官方已提供更合适的同类抽象时，才在同一功能组内重构协议/缓存、调度、媒体、支付、前端产品入口和部署文件。
3. 官方已实现同类功能时，以官方抽象为主，移植私有行为和测试，不保留两套并行实现。
4. 冲突解决后先运行对应小测试，再进入全量检查；不要一次解决全部冲突后才验证。
5. 迁移和生成文件发生冲突时，以 schema 和 migration 意图为依据重新生成，不能手工拼接 Ent 生成代码。

### 5.3 冲突高风险区

- `backend/internal/service/openai_gateway_*` 与 `backend/internal/pkg/apicompat/`：工具流、usage、缓存锚点和错误终态。
- `openai_account_scheduler.go` 与 first-response 文件：调度口径、重试边界、TTFT 记录和并发释放。
- `openai_images.go`、`openai_videos.go`、媒体任务仓库和 usage：任务幂等、一次计费、失败不扣费、公开 ID 和 URL 脱敏。
- `server/routes/gateway.go`：路径别名不能绕过鉴权或能力检查。
- `channel_available.go` 和可用渠道前端：模型来源必须仍为渠道定价。
- 支付 provider、webhook 和订单生命周期：签名、幂等、金额和状态转换。
- `points-system/`、Sub2API points bridge 和 `193_points_balance_credit_ledger.sql`：单位精度、次日策略、消费快照修订、最低昨日消费门槛、阶梯边界、金额上限、签到并发、管理员 disabled 配置入口、启动票据、缓存代次、余额幂等与失败终态。

## 6. 发布门禁

### 6.1 静态与单元测试

```bash
git diff --check

cd backend
go test ./internal/pkg/apicompat -count=1
go test ./internal/handler -count=1
go test ./internal/repository -count=1
go test -tags=unit ./internal/service -count=1
go test -tags=unit ./internal/server -count=1

cd ../frontend
pnpm install --frozen-lockfile
pnpm run lint:check
pnpm run typecheck
pnpm run test:run
pnpm run build

cd ../points-system
go test ./... -count=1
go vet ./...
go build ./cmd/server
```

实际命令以当前 `Makefile`、`package.json` 和 CI 为准。pnpm 版本应与 CI/Dockerfile 一致，不能因为本机新版 pnpm 改写 lockfile。

### 6.2 关键回归矩阵

| 范围 | 最低验证 |
| --- | --- |
| OpenAI | Responses、原生 Chat Completions、SSE、非流式、工具调用、缓存 usage、Embeddings、账号错误；逐项验证正式成功/失败终态、EOF、读取错误、SSE error、部分输出显式错误、客户端断开 drain usage 且禁止重放 |
| Anthropic | Messages、thinking、tool_use/tool_result、cache creation/read、Anthropic 转 Chat 的 `message_stop`/`stop_reason`、缺终态、SSE error、部分输出和客户端断开 |
| Gemini | native `v1beta`、兼容 Chat/Messages、模型列表、SSE 注释心跳、分块图片聚合、`finishReason`、缺终态、读取错误、部分输出和客户端断开 |
| WebSocket | Responses WS v2 与 WS-to-HTTP bridge 的正式成功/失败终态、上游 error、终态前 EOF、终态后正常关闭、客户端断开 drain usage 及零重放 |
| 调度 | 分组隔离、模型/端点/协议画像隔离、近似评分分流、粘性、显著快账号 CAS 迁移、CAS 冲突回退、明确拒绝切号、模糊失败不重放 |
| 图片 | 同步生成、同步编辑、手动异步、重复查询、失败不扣费、成功一次计费、代理 URL |
| 视频 | 每个公开模型的创建、查询、内容下载、长轮询恢复、公开 ID、失败不扣费、成功一次计费 |
| 前端 | 可用渠道长模型列表、移动端、Q 群入口、支付 provider 配置 |
| 积分与签到 | 两位小数比例、真实成功消费、00:05 动态刷新、业务发放时间、快照修订、总/昨日积分阶梯、最低昨日消费金额、固定/百分比奖励、每日次数、三层金额上限、并发签到、刷新与签到按钮恢复、签到未知结果幂等确认、四卡紧凑等高、真实同步状态、8px 面板尺度、reduced-motion、两表每页 10 条独立分页、父主题跨刷新权威、浅/深表格与悬停对比、Canvas 换色、disabled 管理员入口、HMAC 轮换、票据重放、公网内部接口拒绝、同库 schema 隔离、邮箱 ACL 阶段 A/B、反向回滚与失败事务 PostgreSQL CI、缓存旧值回填竞态、拒绝记录收敛、余额幂等、未知 credit 禁止直接冲正和永久失败人工重试 |
| 检测兼容 | 按标准协议真实测试，不识别检测器、不伪造模型身份或固定输出 |

收费媒体真实测试应使用最小数量并保存任务 ID、时间窗和 usage 对照。禁止为了“多试几次”自动循环创建付费媒体任务。

### 6.3 构建、上线与回滚

1. 先在 GitHub PR 完成 CI、安全扫描和人工 diff 审查。
2. 构建带 commit 的不可变镜像标签，并记录 digest；浮动版本标签不能作为唯一回滚点。
3. Sub2API 镜像按运维约定只上传/导入服务器，是否切换由维护窗口决定。
4. 积分镜像可在备份 `points` schema 并记录旧镜像后独立更新，但必须断言 Sub2API 容器 ID、启动时间和镜像引用未变；一次性历史回算不得随更新重跑。
5. 切换前备份 compose 和数据库；只修改目标服务的镜像 tag，不顺带改账号、渠道价格或网关配置。
6. 切换后检查版本、health、restart、migration、关键路由和错误日志。
7. 回滚时恢复旧不可变镜像；如果新迁移不向后兼容，按预先准备的数据回滚方案处理，不能只改镜像。

## 7. 不可破坏清单

- 原有 OpenAI、Claude、Gemini、Codex、OpenClaw 调用继续可用。
- Codex manifest 兼容只作用于带 `client_version` 的模型清单路径；普通 `/v1/models`、渠道定价模型列表、Responses/Chat 调度和计费不得复用该回退。
- 显式缓存键优先，自动缓存键不跨用户、不改正文、不影响图片意图。
- 调度不越过用户分组、模型限制、账号状态、传输协议和并发限制。
- 快账号迁移不得发送探测请求；候选槽位必须先获取，原子改绑失败后必须释放并继续使用原粘性账号。
- 全局首响应优化开启时，TTFT 从最终账号真实上游发送开始，以最终成功 attempt 的 2xx 响应头作为最早正常响应；关闭时使用首个真实语义输出。错误切号、调度排队和失败响应不计入，不能发送本地伪造 token 或篡改总耗时。
- 文本请求最多按明确安全条件故障转移；媒体创建发出后绝不跨账号自动重放。
- 流式 HTTP/WS 只有协议正式成功终态才能成功；EOF、读取错误、上游错误或缺终态必须显式失败。部分输出后不得合成成功终止事件；客户端断开后只允许有界 drain usage，禁止再写下游或重放上游请求。
- 图片/视频任务使用公开 ID；查询固定回创建账号；成功一次计费，失败不扣费。
- 用户响应和浏览器 Network 不出现供应商域名、账号 Base URL、供应商任务 ID或认证头。
- 可用渠道只展示渠道定价中且用户有权访问的模型，并在桌面/移动端完整可查看。
- 支付回调验签、订单幂等和金额校验不能被 WAF 放行规则或前端状态替代。
- 积分只统计成功余额消费且不可兑换；签到奖励通过余额发放事务发件箱入账。最低签到消费金额只读昨日成功消费，百分比奖励只读对应周期原始消费金额，浏览器参数不能改变用户、积分、消费基数、阶梯或奖励。
- 积分配置只追加版本并次日生效；仅消费用户必须锁定昨日积分。直接访问积分域名不能建立会话，余额发放必须同时通过 HMAC、时间窗、key ID、交易 UUID 和数据库幂等校验。
- 积分服务只能在现有 Sub2API 数据库的独立 schema 写入；首次迁移前必须备份数据库，启动或回滚积分容器不得停止、重启或替换运行中的 Sub2API 容器。
- 积分页面只显示登录邮箱，不显示数值用户 ID；浏览器 API 使用 `login_email`，数值 ID 只允许在服务端关联、财务审计和幂等处理中使用，不返回无必要的 `user_id`。
- 积分子页不得用固定浅色覆盖深色表格或悬停状态；严格校验后的父主题在后续资料刷新中保持权威，并触发 Canvas 重绘。四张概览卡在桌面和移动端不得互相错位或发生按钮遮挡，两张个人记录表默认每页 10 条且独立翻页。
- 积分用户首屏失败必须进入持久可重试状态，管理员首屏必须在真实汇总加载后才揭示；窄屏 Canvas 不得使用固定内部宽度造成日期挤压，普通 hover 不得使用近似选中态的高强度主色填充。
- 余额缓存旧回源不得覆盖积分 credit/reversal 后的新余额；缓存同步失败必须保持同一交易可重试。公网域名不得代理内部 credit 路径，`/launch` 之外的积分访问日志不得整体关闭。

## 8. 文档更新规则

新增或调整二开功能时，同一 PR 至少更新：

1. 本文档的功能清单、代码入口和不可破坏约束。
2. `docs/MEDIA_API_CN.md`、`docs/PRODUCTION_OPERATIONS_CN.md` 或对应部署文档中的对外契约与运行边界。
3. 配置示例和迁移说明。
4. 对应测试矩阵。

不得在文档中保存生产凭据、真实 API Key、支付密钥、Worker Secret、数据库连接串或 SSH/GitHub Token。
