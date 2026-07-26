# 52Token Cloudflare、Nginx 与媒体边缘配置

本文是 `52token.org` 这一套域名的最终操作说明，目标是隐藏源站、保留现有
Sub2API/工作台路径，并把图片和视频的完成地址限制在自有域名。本文不包含任何
SSH 密钥、API Key、支付密钥或 Worker Secret。

画布工作台唯一源码来源为 [`hxly520/infinite-canvas`](https://github.com/hxly520/infinite-canvas)，
通过固定SHA镜像运行在 `127.0.0.1:15731`。服务器旧 `images` 目录不参与构建、
运行、Nginx fallback或回滚；Sub2API仓库内 `/batch-image` 也不是该画布工作台。

## 1. 最终域名职责

| 域名 | Cloudflare | 只承担的职责 |
| --- | --- | --- |
| `52token.org` | 橙云 | 公开首页、帮助页和既有兼容入口 |
| `api.52token.org` | 橙云/SaaS CNAME | OpenAI、Anthropic、Gemini、Codex、OpenClaw、支付回调和长流式文本 |
| `image.52token.org` | 橙云 | 画布工作台、图片创建/编辑、视频创建和任务查询 |
| `video.52token.org` | 橙云 + 两条精确 Worker Route | 已完成图片/视频的签名下载和 Range 请求 |
| `long.52token.org` | 退役 | 不再提供入口，不做 301/302 |

`image` 不负责最终媒体下载，`video` 不负责工作台、创建 API、任务 API 或平台
页面。用户看到的完成地址只能是 `video.52token.org`；上游地址、供应商任务 ID
和认证头只保存在服务端受控字段。

## 2. 上线前准备

1. 在 GitHub、本机或安全备份中保存当前 DNS、Nginx、Compose 和 Cloudflare
   Rules 的导出；不要把导出文件提交到仓库。
2. 确认源站证书覆盖 `52token.org`、`api.52token.org`、`image.52token.org`
   和 `video.52token.org`。仓库示例默认使用 `/etc/nginx/cert/52token.org.pem`
   与对应 key；实际路径以服务器证书文件为准。
3. 将 `deploy/nginx/cloudflare-real-ip.conf.example` 复制为 Nginx 的
   `conf.d/00-cloudflare-real-ip.conf`，并在变更前对照 Cloudflare 官方 IP 列表
   更新网段：[Cloudflare IP ranges](https://www.cloudflare.com/ips/)。
4. 将 `deploy/nginx/api.52token.org.conf.example`、
   `deploy/nginx/image.52token.org.conf.example` 和现有
   `deploy/nginx/video.52token.org.conf.example` 按现网证书路径放入 `conf.d`。
   `52token-public-root.inc.example`、`52token-public-help.inc.example` 和
   `52token-static-security-headers.inc.example` 分别复制到服务器 snippets
   目录；如果已有同名 include，以当前已审查版本为准，不重复定义 location。
5. 用 `deploy/nginx/000-default-deny.conf.example` 替换发行版默认虚拟主机的
   catch-all 规则；不要直接添加第二个 `default_server`，否则 `nginx -t` 会失败。

示例文件中的 `if (...) { return 444; }` 只做源站入口拒绝，属于 Nginx 官方允许
的安全用法。它依赖 `cloudflare-real-ip` include 记录的原始 TCP peer；不要把
用户 IP 放入 `set_real_ip_from`。

## 3. Cloudflare DNS

在 Cloudflare DNS 中：

1. 保留 `52token.org` 和 `api.52token.org` 的现有解析与代理状态。
2. 将 `image.52token.org` 指向当前源站，打开 **Proxied/橙云**。
3. 将 `video.52token.org` 指向当前源站，打开 **Proxied/橙云**。
4. 删除 `long.52token.org` 的 DNS 记录。不要创建重定向；部分 SDK 会在
   301/302 后丢失 POST body 或 Authorization。若需要回滚，恢复原记录而不是
   临时改成重定向。
5. 记录并核对 DNS-only 历史记录。橙云只隐藏当前 DNS，不会从历史 DNS、日志或
   第三方缓存中抹掉旧源 IP；源站入口锁定后再按维护窗口轮换源 IP。

## 4. Worker 精确路由

Worker 使用 `deploy/video-edge-worker/`，不要配置 Custom Domain 或接管整个
`video.52token.org`。`wrangler.toml` 只保留：

```toml
routes = [
  { pattern = "video.52token.org/v1/video-content/*", zone_name = "52token.org" },
  { pattern = "video.52token.org/v1/image-content/*", zone_name = "52token.org" }
]
```

在 Worker Settings 中添加生产 Secret（值只在 Cloudflare 控制台输入），名称与
代码约定一致。Sub2API 的 `gateway.video_proxy.encryption_key` 必须与 Worker
使用的 64 位十六进制密钥一致，但不要把值写进 Git、命令历史或工单：

```yaml
gateway:
  video_proxy:
    mode: edge
    edge_base_url: https://video.52token.org
    encryption_key: <只在服务器生产配置填写>
    token_ttl_seconds: 3600
```

首次部署保持 `ALLOWED_MEDIA_HOSTS` 为空，确认真实图片/CDN 主机后再填写精确
allowlist。Worker 必须：

- 只接受 `/v1/video-content/*` 与 `/v1/image-content/*`；
- 支持 `GET`、`HEAD`、`OPTIONS`、Range 和有限内部跳转；
- 不返回上游 `Location`、Server、供应商请求 ID 或上游 URL；
- 不限制生成媒体大小，只做 URL 代理和流式转发；
- 根路径、生成 API、任务 API、后台路径统一返回 404。

## 5. 两条 WAF Custom Rules

Cloudflare 自定义规则只保留下面两条，按顺序排列。先删除或停用旧的重复放行、
旧 `long` 规则和重复中国大陆规则，避免规则数量超过套餐的 5 条上限。

### 5.1 规则一：必要 API/媒体/支付 Skip

动作选择 **Skip**，只勾选 **All remaining custom rules**。不要跳过 Managed
Rules、DDoS、Rate Limiting 或 Bot 规则；若某个具体托管规则误拦，再按 Security
Events 做窄范围例外。

```text
(
  (
    http.host in {"52token.org" "api.52token.org"}
    and (
      starts_with(http.request.uri.path, "/v1/")
      or starts_with(http.request.uri.path, "/v1beta/")
      or starts_with(http.request.uri.path, "/backend-api/codex/")
      or starts_with(http.request.uri.path, "/antigravity/")
      or http.request.uri.path in {
        "/responses" "/chat/completions" "/embeddings" "/models"
        "/images/generations" "/images/edits" "/videos"
        "/videos/generations" "/video/generations"
        "/contents/generations/tasks"
      }
      or starts_with(http.request.uri.path, "/responses/")
      or starts_with(http.request.uri.path, "/images/generations/")
      or starts_with(http.request.uri.path, "/images/edits/")
      or starts_with(http.request.uri.path, "/videos/")
      or starts_with(http.request.uri.path, "/video/generations/")
      or starts_with(http.request.uri.path, "/contents/generations/tasks/")
    )
    and (
      any(http.request.headers["authorization"][*] ne "")
      or any(http.request.headers["x-api-key"][*] ne "")
      or any(http.request.headers["x-goog-api-key"][*] ne "")
      or (
        http.request.method eq "OPTIONS"
        and http.request.headers["origin"][0] eq "https://image.52token.org"
      )
      or (
        any(http.request.uri.args["key"][*] ne "")
        and (
          starts_with(http.request.uri.path, "/v1beta/")
          or starts_with(http.request.uri.path, "/antigravity/v1beta/")
        )
      )
    )
  )
  or (
    http.host eq "image.52token.org"
    and (
      http.request.uri.path in {
        "/v1/models" "/v1/images/generations" "/v1/images/edits"
        "/v1beta/models" "/v1/videos" "/videos"
        "/v1/video/generations" "/video/generations"
        "/v1/contents/generations/tasks" "/contents/generations/tasks"
      }
      or starts_with(http.request.uri.path, "/v1/images/generations/")
      or starts_with(http.request.uri.path, "/v1/images/edits/")
      or starts_with(http.request.uri.path, "/v1beta/models/")
      or starts_with(http.request.uri.path, "/v1/videos/")
      or starts_with(http.request.uri.path, "/videos/")
      or starts_with(http.request.uri.path, "/v1/video/generations/")
      or starts_with(http.request.uri.path, "/video/generations/")
      or starts_with(http.request.uri.path, "/v1/contents/generations/tasks/")
      or starts_with(http.request.uri.path, "/contents/generations/tasks/")
    )
    and (
      any(http.request.headers["authorization"][*] ne "")
      or any(http.request.headers["x-api-key"][*] ne "")
      or any(http.request.headers["x-goog-api-key"][*] ne "")
      or (
        any(http.request.uri.args["key"][*] ne "")
        and starts_with(http.request.uri.path, "/v1beta/")
      )
      or (
        http.request.method eq "OPTIONS"
        and http.request.headers["origin"][0] eq "https://image.52token.org"
      )
    )
  )
  or (
    http.host in {"52token.org" "api.52token.org"}
    and starts_with(http.request.uri.path, "/api/v1/payment/webhook/")
    and http.request.method in {"GET" "POST"}
  )
  or (
    http.host eq "video.52token.org"
    and http.request.method in {"GET" "HEAD" "OPTIONS"}
    and (
      starts_with(http.request.uri.path, "/v1/video-content/")
      or starts_with(http.request.uri.path, "/v1/image-content/")
    )
  )
)
```

### 5.2 规则二：阻止中国大陆非必要访问

动作选择 **Block**，紧跟规则一之后：

```text
ip.src.country eq "CN"
and http.host in {"52token.org" "api.52token.org" "video.52token.org"}
```

这条规则会阻止中国大陆访问主站页面、API 非必要路径和 `video` 非媒体路径；
规则一已经先跳过必要的带鉴权 API、工作台预检、支付回调和签名媒体 URL。
`image.52token.org` 默认不加入国家阻断，以保留用户在中国大陆打开工作台的现有
行为。如果明确不需要大陆工作台，再把 `image.52token.org` 加入集合；此时图片/视频
API 仍可由规则一放行，但工作台页面会被阻断，二者不能同时要求。

## 6. Cache Rules（不占 Custom Rules 配额）

新增一条 Cache Rule，动作选择 **Bypass cache**。动态 API、支付回调和加密媒体
地址不能被共享缓存：

```text
(
  http.host in {"52token.org" "api.52token.org"}
  and (
    starts_with(http.request.uri.path, "/v1/")
    or starts_with(http.request.uri.path, "/v1beta/")
    or starts_with(http.request.uri.path, "/backend-api/codex/")
    or starts_with(http.request.uri.path, "/antigravity/")
    or starts_with(http.request.uri.path, "/responses")
    or starts_with(http.request.uri.path, "/chat/completions")
    or starts_with(http.request.uri.path, "/api/v1/payment/webhook/")
  )
)
or (
  http.host eq "image.52token.org"
  and (
    starts_with(http.request.uri.path, "/v1/")
    or starts_with(http.request.uri.path, "/v1beta/")
    or starts_with(http.request.uri.path, "/videos")
    or starts_with(http.request.uri.path, "/video/generations")
    or starts_with(http.request.uri.path, "/contents/generations/tasks")
  )
)
or (
  http.host eq "video.52token.org"
  and (
    starts_with(http.request.uri.path, "/v1/image-content/")
    or starts_with(http.request.uri.path, "/v1/video-content/")
  )
)
```

Worker 代码同时使用 `cacheEverything: false`。不要为媒体签名 URL 设置 Edge
Cache TTL，也不要对 POST/轮询响应使用 Cache Rules。

## 7. API 长请求与图片保活

普通 Cloudflare 套餐不能保证连续 120 秒没有任何源站响应字节的非流式请求；Nginx
设置 1800 秒不能绕过 Cloudflare 的 524 限制。统一使用：

- Responses/Chat：`stream=true`，依赖 SSE 注释心跳；
- Gemini 工作台：`streamGenerateContent?alt=sse`，忽略 `:` 心跳并聚合图片；
- 视频：创建快速返回任务 ID，客户端轮询同一任务，不把长等待保持为一个创建连接；
- OpenAI Images 同步 JSON：先用无计费慢请求验证，再在 Sub2API 生产配置启用：

```yaml
gateway:
  image_nonstream_keepalive_interval: 10
```

该配置只发送保活空白，不创建任务、不改变 usage 或计费。出现晚到上游错误时，
响应可能已经是 HTTP 200，客户端必须检查 JSON `error` 字段，不能只看状态码。

## 8. 源站隔离与 Compose

按以下顺序操作，任何一步失败先停止，不要通过重新创建媒体任务来验证：

1. 先部署 Nginx 示例，执行 `nginx -t`，再 `systemctl reload nginx`。
2. 在云安全组/主机防火墙中只允许 Cloudflare 官方 IPv4/IPv6 网段访问 80/443；
   SSH 只允许管理入口。不要用 Nginx `allow` 替代网络层封锁。
3. 将 Sub2API Compose 的端口从：

   ```yaml
   - "8080:8080"
   ```

   改成：

   ```yaml
   - "127.0.0.1:8080:8080"
   ```

   如果 Compose 使用 `network_mode` 或宿主健康检查方式不同，按实际文件等价修改，
   不要同时改镜像、数据库或网关配置。改后确认 `curl http://127.0.0.1:8080/health`
   仍返回 200，而公网 `:8080` 无法连接。
4. 在锁定防火墙前，从外部复测 `api` SaaS CNAME 的回源链路，确认不是只允许了
   普通 Zone 的 Cloudflare 网段而漏掉 SaaS 代理路径。
5. 确认 `image` 和 `video` 的 DNS 变成橙云后，再删除 `long` DNS 和对应 Nginx
   server；不添加重定向。观察一段访问日志后按需要轮换旧源 IP。

## 9. 验收与回滚

无计费探针：

```text
GET https://image.52token.org/
POST https://image.52token.org/v1/images/generations  (无 Authorization，应为应用层 401)
POST https://image.52token.org/v1/videos                 (无 Authorization，应为应用层 401)
GET https://video.52token.org/
POST https://video.52token.org/v1/videos                 (应为 404)
GET https://video.52token.org/v1/video-content/invalid   (通用 JSON 404)
OPTIONS https://video.52token.org/v1/image-content/invalid (204)
```

真实回归只使用用户明确选择的最小任务：

1. 图片成功响应只出现 `video.52token.org/v1/image-content/`，或按请求返回
   `b64_json`；工作台刷新后仍能从本地 Blob 预览。
2. 视频创建只发生一次；轮询恢复仍使用同一平台任务 ID，成功只写一条 usage，
   失败/取消/过期不扣费；下载支持 200/206，响应无 `Location` 和上游 host。
3. 浏览器 Network、DOM、IndexedDB 和错误提示中不出现上游地址、签名或供应商任务 ID。
4. `api + stream=true` 的长文本持续收到心跳；不要用连续 130 秒无字节的非流式请求
   作为普通套餐验收标准。

若出现 524、CORS、工作台空白、媒体地址泄露、重复计费或源站无法健康检查：

- 先暂停真实媒体创建；
- 恢复上一份 DNS/WAF/Cache/Nginx/Compose 备份；
- 保持旧不可变 Sub2API 镜像不变；
- 查清边缘、源站和应用日志后再分阶段重新启用。
