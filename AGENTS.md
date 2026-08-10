# 私有仓库接手入口

本文件是自动化维护者和后续 AI 接手 `hxly520/sub2api` 时的第一份检查清单。它不是生产密码、部署凭据或运行时事实的存储位置。真实环境状态以脱敏后的运维记录和服务器只读盘点为准。

## 开始前必读

按以下顺序读取，读完后再修改代码：

1. [`docs/PRIVATE_CUSTOMIZATION_CN.md`](docs/PRIVATE_CUSTOMIZATION_CN.md)：二开功能清单、代码入口、不可破坏契约和官方合并规则。
2. [`docs/PRIVATE_RELEASE_RUNBOOK_CN.md`](docs/PRIVATE_RELEASE_RUNBOOK_CN.md)：私有 Release、在线热更新、Compose 更新和回退门禁。
3. [`docs/OFFICIAL_COMPATIBILITY_HISTORY_CN.md`](docs/OFFICIAL_COMPATIBILITY_HISTORY_CN.md)：官方版本差异与已完成/待处理状态。
4. [`docs/PRODUCTION_OPERATIONS_CN.md`](docs/PRODUCTION_OPERATIONS_CN.md)：当前生产镜像、数据库、Redis、Nginx、工作台和只读验收证据。
5. [`docs/LINK_CARDS_CN.md`](docs/LINK_CARDS_CN.md)：额度卡/提链的资金、权限、计费和迁移边界。

若文档与服务器事实冲突，先只读复核并更新事实记录；不能凭旧文档猜测生产状态。不得把服务器密码、完整 API Key、数据库连接串、HMAC secret、GHCR PAT、GitHub token 或证书私钥写入仓库、Issue、提交信息、测试 fixture 或日志。

## 仓库与分支规则

- 私有远端是 `hxly520/sub2api`；`upstream` 仅用于读取官方 `Wei-Shaw/sub2api` Release 和提交。
- `main` 是当前私有生产兼容主线。正式升级必须从完整的私有 `main` 创建 `codex/upgrade-vX.Y.Z-*` 分支，再按官方 Release tag 合并；不得从官方 tip 另起生产分支而遗漏二开。
- 保留完整 Git 历史、官方 merge-base、私有提交和迁移文件名。禁止浅克隆、文件覆盖式“升级”、重编号或重命名已有迁移。
- 官方同类功能以官方实现为主，私有行为以兼容补丁和回归测试保留。冲突处理后必须更新二开清单和兼容历史，不得只写一句“已合并”。
- 工作区可能已有用户修改。只改任务相关文件，不使用 `git reset --hard`、`git checkout --` 或清理命令抹掉未由当前任务创建的变更。

## 二开不变量

- 文本协议、媒体任务、计费、余额、缓存、积分同库、额度卡、公开首页和独立 `infinite-canvas` 工作台的契约见上述文档；官方更新不得让它们退化。
- 媒体创建最多提交一次；确定失败或空结果退款，状态未知不能擅自退款。文本容量拒绝只能按精确错误文案进行有限重试，不能把一般 5xx、超时或断流当作可重放。
- 额度卡和余额变更必须使用服务端鉴权、幂等键、事务和审计。客户端参数不是金额事实源；任何资金异常先停入口、对账和保留证据。
- 用户积分与管理员控制台必须严格隔离。关闭服务端开关时用户入口/API 同时隐藏和拒绝；策略修改按既定次日生效规则处理，不能由浏览器提交生效时间。
- 公开首页只面向未登录访客，登录后的 Dashboard 不随首页改版改变。根路径静态页面由 Nginx 宿主文件单独发布，不能误以为替换 Sub2API 镜像会更新它。
- 生产服务器不编译 Go、前端或 Docker 镜像。镜像由 GitHub Actions 或受控本机构建，服务器只拉取/导入经过记录的不可变 tag/digest；Sub2API 容器切换由维护者在窗口内手工执行。

## 发布选择

私有发布 Tag 固定为 `vX.Y.Z-52t.N`，并使用 annotated tag。发布工作流会生成二进制、`checksums.txt`、`update-manifest.json`、带独立 SHA256 的公开首页资产和 GHCR 镜像：

- `hot-update-safe`：后台可下载经 checksum、manifest、大小、版本号和完整 source commit/revision 校验的二进制，走短暂优雅重启；镜像不自动替换。
- `image-update-recommended`：允许热更新，但维护者应在窗口内同步 Compose 镜像。
- `image-update-required`：禁止后台热更新，必须先用 Compose/GHCR 更新；首个私有 Release、容器入口/Compose/Nginx/积分系统/独立静态资源变化、未审迁移和历史分叉都会落入此类。

分类由 [`tools/release/classify-update.sh`](tools/release/classify-update.sh) 生成，隔离回归由 [`tools/release/test-classify-update.sh`](tools/release/test-classify-update.sh) 覆盖。任何 Tag 只要修改数据库迁移，除非 Tag message 含 `[migration-hot-update-safe]` 且完成向后兼容审查，否则必须 Compose 更新。该标记不是自动批准，审查证据仍要写入兼容历史。

热更新只修改当前容器可写层；重新创建容器会恢复 Compose 镜像内的二进制。热更新验收后必须由维护者把 Compose 镜像引用钉到同一 Release tag/digest，并只做配置校验，等人工窗口再决定是否重建。

## 每次变更的最小交付

1. 说明官方基线、私有分支、merge-base、变更文件和冲突取舍。
2. 为共享行为增加或更新定向测试；运行 `git diff --check`、后端测试/`go vet`/编译、前端 lint/typecheck/test/build，以及 Compose YAML 解析。
3. 更新二开清单、兼容历史和发布/回滚记录；记录镜像 tag、完整 revision、registry digest、归档 SHA256、数据库迁移和回滚边界，但不记录敏感值。
4. 生产操作先给出只读证据和人工切换命令。除非维护者明确要求，自动化不替换 Sub2API 容器、不修改生产数据库、不重启主服务、不构建上传镜像。

## 遇到不确定性

涉及资金、数据库迁移、生产流量、Nginx/Cloudflare、跨仓库工作台或官方同类功能时，先停在方案和差异审查阶段，向维护者列出具体冲突与选项；不要用猜测补齐需求。完成一个子任务后把文件、测试命令、未解决风险和下一步人工动作写入交接消息。
