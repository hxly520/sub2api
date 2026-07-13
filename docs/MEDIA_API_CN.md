# 图片与视频 API 契约

本文档记录私有分支对外提供的媒体接口及维护约束。部署域名、公开模型和价格由运营配置决定；示例使用占位地址和占位密钥，不包含任何账号或供应商信息。

## 1. 通用约定

- 鉴权：`Authorization: Bearer YOUR_API_KEY`。
- JSON 请求：`Content-Type: application/json`。
- 参考文件上传：使用 `multipart/form-data`，不要手工填写 boundary。
- 媒体创建是非幂等业务。客户端应为异步创建传稳定且唯一的 `Idempotency-Key`，网络不明时先用原任务或原幂等键恢复，不能直接创建新任务。
- 任务查询和内容下载可以重复，不会重复计费。
- 对外任务 ID 是平台公开 ID；客户端不得依赖内部任务 ID格式。
- 最终媒体 URL 可能是同域相对路径，也可能是平台媒体下载域名。客户端应按返回值访问，不拼接供应商地址。

## 2. OpenAI Images

### 2.1 图片生成

```http
POST /v1/images/generations
```

常用 JSON 字段：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `model` | string | 必填，使用当前分组可用渠道中显示的图片模型 |
| `prompt` | string | 必填，图片描述 |
| `n` | integer | 生成数量；实际范围受模型和渠道限制 |
| `size` | string | OpenAI 风格尺寸，例如 `1024x1024`；是否生效由模型决定 |
| `quality` | string | 质量档位；按模型支持情况传递 |
| `image_size` | string | 固定清晰度模型使用的档位，例如 `1K`、`2K`、`4K` |
| `aspect_ratio` | string | 固定清晰度模型的比例，例如 `1:1`、`16:9`、`9:16` |
| `response_format` | string | `url` 或 `b64_json` |
| `output_format` | string | 常用 `png`、`jpeg` 或 `webp`，以模型支持为准 |
| `output_compression` | integer | 压缩质量，只有支持的输出格式/模型会生效 |
| `async` | boolean | 显式异步模式；默认不传即同步 |

同步示例：

```bash
curl https://image.example.com/v1/images/generations \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "YOUR_IMAGE_MODEL",
    "prompt": "白底产品图，柔和棚拍光",
    "size": "1024x1024",
    "response_format": "b64_json"
  }'
```

显式异步示例：

```bash
curl https://image.example.com/v1/images/generations \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: image-job-20260713-001" \
  -d '{
    "model": "YOUR_IMAGE_MODEL",
    "prompt": "雨夜霓虹街道",
    "image_size": "4K",
    "aspect_ratio": "16:9",
    "response_format": "url",
    "async": true
  }'
```

如果创建响应已包含图片，直接使用 `data[].url` 或 `data[].b64_json`。如果返回任务 ID，使用相同 API Key 查询：

```http
GET /v1/images/generations/{task_id}
```

### 2.2 图片编辑

```http
POST /v1/images/edits
```

使用 `multipart/form-data`：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `model` | string | 必填 |
| `prompt` | string | 必填，说明如何修改图片 |
| `image` | file，可重复 | 一张或多张参考图 |
| `mask` | file | 可选蒙版；透明/有效区域语义由模型决定 |
| `n`、`size`、`quality` | mixed | 与图片生成相同 |
| `image_size`、`aspect_ratio` | string | 固定清晰度模型可用 |
| `response_format`、`output_format` | string | 返回和输出格式 |
| `async` | boolean | 显式异步；默认同步 |

```bash
curl https://image.example.com/v1/images/edits \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -F "model=YOUR_IMAGE_MODEL" \
  -F "prompt=把背景改成纯白色，保留主体" \
  -F "image=@input.png" \
  -F "mask=@mask.png" \
  -F "response_format=b64_json"
```

异步编辑任务查询：

```http
GET /v1/images/edits/{task_id}
```

同步失败不会自动切成异步重新创建。客户端若需要异步，必须从第一次创建时就显式传 `async: true` 或 multipart 字段 `async=true`。

## 3. Gemini Native 图片生成

```http
POST /v1beta/models/{model}:generateContent
```

可使用 `x-goog-api-key: YOUR_API_KEY`。请求正文至少包含 `contents`，生图时在 `generationConfig` 中请求 IMAGE 模态；参考图可放在 `parts[].inlineData` 或兼容的文件引用中。

```bash
curl https://image.example.com/v1beta/models/YOUR_GEMINI_IMAGE_MODEL:generateContent \
  -H "x-goog-api-key: YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "contents": [{
      "role": "user",
      "parts": [{"text": "生成一张 16:9 的城市夜景"}]
    }],
    "generationConfig": {
      "responseModalities": ["TEXT", "IMAGE"],
      "imageConfig": {
        "imageSize": "4K",
        "aspectRatio": "16:9"
      }
    }
  }'
```

图片通常位于 `candidates[].content.parts[].inlineData`；客户端也应兼容 `inline_data` 和文件 URL 返回。模型是否支持 `imageSize`、`aspectRatio`、参考图和 4K 由当前渠道能力决定。

## 4. 统一视频任务 API

### 4.1 创建

```http
POST /v1/videos
```

```bash
curl https://image.example.com/v1/videos \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: video-job-20260713-001" \
  -d '{
    "model": "seedance-2.0-mini",
    "prompt": "雨夜霓虹街道，镜头缓慢推进，电影感光影",
    "aspect_ratio": "16:9",
    "duration": 8,
    "resolution": "720p"
  }'
```

创建响应中的 `id`、`task_id` 或 `request_id` 会统一改写为平台公开任务 ID。客户端保存该 ID，后续只查询该任务。

### 4.2 查询

```http
GET /v1/videos/{task_id}
```

```bash
curl https://image.example.com/v1/videos/VIDEO_TASK_ID \
  -H "Authorization: Bearer YOUR_API_KEY"
```

常见状态：`creating`、`queued`、`pending`、`running`、`completed`、`failed`、`cancelled`、`expired`。未完成时建议每 5-10 秒查询一次；视频可能运行数分钟，客户端不应因单次查询超时重新创建。

### 4.3 下载

```http
GET /v1/videos/{task_id}/content
```

```bash
curl -L https://image.example.com/v1/videos/VIDEO_TASK_ID/content \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -o result.mp4
```

终态响应中的 `url`、`video_url`、`result_url`、`content_url` 等字段会统一指向平台控制的任务内容地址。内容下载可能临时跳转到平台媒体代理域名；客户端应允许 `-L`/跟随 307，并保留 Range 下载能力。

## 5. 视频通用字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `model` | string | 必填；以当前分组可用渠道页面显示的模型名为准 |
| `prompt` | string | 受支持的视频模型必填 |
| `duration` / `seconds` | integer | 时长秒数；优先使用 `duration` |
| `aspect_ratio` | string | 画面比例 |
| `resolution` / `resolution_name` | string | 清晰度；固定分辨率模型由模型后缀决定 |
| `audio` | boolean | 是否生成音频，仅对支持的模型生效；参考音频请使用 `reference_audios` |
| `image_url` | string | 单张参考图 |
| `image_urls` | string[] | 多张参考图 |
| `reference_image_urls` | string[] | 附加参考图，适用于支持多参考图的模型 |
| `first_image_url`、`last_image_url` | string | 首尾帧模式；必须按模型规则成对传递 |
| `video_url` | string | 单个参考视频 |
| `reference_videos` | string[] | 多个参考视频 |
| `reference_audios` | string[] | 多个参考音频 |
| `reference_mode` | string | `frame` 或 `image`，只在对应模型需要时传 |

直接 API 调用时，参考图片、视频和音频建议使用公网可访问的 HTTPS URL。本地文件路径、`blob:` URL 和只在本机可访问的地址不能被服务端读取。工作台会在浏览器侧处理本地素材并按模型适配请求格式。

## 6. 当前公开视频模型限制

可用模型以用户可用渠道页面为准。当前共公开 9 个视频模型，独立的固定 480p 后缀模型已下架，不应再写入模型清单。标准按次模型仍可按表中限制传递 `resolution=480p`，这与独立固定分辨率模型不是同一概念。模型映射可以改变内部模型名，但不能绕过这些公开校验。

| 模型 | 计费/分辨率 | 时长 | 比例 | 参考素材 | 其他限制 |
| --- | --- | --- | --- | --- | --- |
| `seedance-2.0` | 按次；`480p`/`720p` | 4-15 秒 | `16:9`、`9:16`、`1:1`、`21:9`、`3:4`、`4:3` | 最多 4 图、3 视频、1 音频 | prompt 最多 5000 字；视频/音频必须同时至少有 1 张图；首尾帧必须成对且不能混用其他素材 |
| `seedance-2.0-fast` | 按次；`480p`/`720p` | 4-15 秒 | 同上 | 最多 4 图、3 视频、1 音频 | 同 Seedance 标准规则 |
| `seedance-2.0-mini` | 按次；`480p`/`720p` | 4-15 秒 | 同上 | 最多 4 图、3 视频、1 音频 | 同 Seedance 标准规则 |
| `seedance-2.0-fast-720p` | 按秒；固定 `720p` | 必填，4-15 秒 | 同上 | 最多 9 图、3 视频、3 音频 | 同固定分辨率规则 |
| `seedance-2.0-mini-720p` | 按秒；固定 `720p` | 必填，4-15 秒 | 同上 | 最多 9 图、3 视频、3 音频 | 同固定分辨率规则 |
| `seedance-2.0-720p` | 按秒；固定 `720p` | 必填，4-15 秒 | 同上 | 最多 9 图、3 视频、3 音频 | 同固定分辨率规则 |
| `seedance-2.0-1080p` | 按秒；固定 `1080p` | 必填，4-15 秒 | 同上 | 最多 9 图、3 视频、3 音频 | 同固定分辨率规则 |
| `seedance-2.0-4k` | 按秒；固定 `4k` | 必填，4-15 秒 | 同上 | 最多 9 图、3 视频、3 音频 | 同固定分辨率规则；生成时间和文件体积通常更大 |
| `grok-video` | 按次；`480p`/`720p` | `4`、`6`、`8`、`10`、`12`、`15` 秒 | `1:1`、`16:9`、`9:16`、`4:3`、`3:4`、`3:2`、`2:3` | 最多 7 图和 1 个参考视频，可同时使用；不支持音频 | prompt 最多 4096 字；不支持首尾帧；多图请求最长按 10 秒处理 |

字段示例：

```json
{
  "model": "seedance-2.0-fast-720p",
  "prompt": "人物沿海边奔跑，镜头平稳跟随",
  "duration": 8,
  "aspect_ratio": "16:9",
  "image_url": "https://media.example.com/reference-1.jpg",
  "reference_image_urls": [
    "https://media.example.com/reference-2.jpg"
  ],
  "reference_videos": [
    "https://media.example.com/reference.mp4"
  ]
}
```

首尾帧示例：

```json
{
  "model": "seedance-2.0",
  "prompt": "从白天平滑过渡到夜晚",
  "duration": 8,
  "resolution": "720p",
  "aspect_ratio": "16:9",
  "first_image_url": "https://media.example.com/first.jpg",
  "last_image_url": "https://media.example.com/last.jpg"
}
```

## 7. 可配置兼容模型

代码还保留 Omni Fast、Omni V2V、Sora 和 Veo 协议适配，只有管理员在渠道定价和模型映射中启用后才对用户可见：

| 模型族 | 时长/分辨率 | 参考素材限制 |
| --- | --- | --- |
| Omni Fast | 10 秒、720p、`16:9`/`9:16` | 最多 5 图；不支持参考视频或音频；多图 API 使用 multipart `input_reference` |
| Omni V2V | 10 秒、720p、`16:9`/`9:16` | 必须且只能 1 个参考视频，不超过 5MB；不支持图片或音频 |
| Sora | 4/8/12 秒，`16:9`/`9:16`，兼容 `1280x720`、`720x1280`、`1024x1024` | 最多 1 张首帧图；`reference_mode=frame`；prompt 最多 1200 字；不支持视频或音频 |
| Veo 标准 | 4/6/8 秒、720p/1080p、`16:9`/`9:16` | 最多 2 图，`reference_mode=frame`；prompt 最多 1200 字；不支持视频或音频 |
| Veo `ref` | 4/6/8 秒、720p/1080p、`16:9`/`9:16` | 最多 3 图，`reference_mode=image`；其他限制同 Veo |

未出现在可用渠道页面的模型不能仅凭本表调用。公开能力始终由用户分组权限、渠道定价和账号模型映射共同决定。

## 8. 计费与错误处理

- 图片/视频创建失败、取消或过期不记录成功 usage。
- 异步任务在终态成功时按创建时保存的价格快照结算，并以任务 ID 幂等，只记录一次。
- 按秒视频使用请求或最终返回的有效时长；按次视频每个成功任务计一次。
- 查询、继续查询、下载、Range 分段下载不计为新生成任务。
- 客户端收到 5xx、连接中断或超时时，不能据此认定创建未受理。优先使用原 `Idempotency-Key` 或已返回的公开任务 ID恢复。
- 如果任务已明确 `failed`，需要生成新视频时创建新的幂等键；不要继续轮询失败任务。

## 9. 维护回归

每次修改媒体适配至少验证：

1. JSON 和 multipart 请求解析。
2. 公开模型名、渠道映射模型和账号映射模型三层一致性。
3. 创建只发送一次，任务查询固定回原账号和创建端点。
4. 公开任务 ID不包含供应商任务 ID。
5. 所有 URL 字段只返回平台任务内容地址或媒体代理地址。
6. 同步/异步图片、视频长轮询、重复查询、失败不扣费和成功一次计费。
7. `GET /content` 的重定向、Range、HEAD 和大文件流式下载。
