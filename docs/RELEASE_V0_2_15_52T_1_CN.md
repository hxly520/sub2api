# v0.2.15-52t.1 兼容升级交接

本轮只升级 Sub2API，沿用 GitHub Actions/full GoReleaser/GHCR 发布。服务器只缓存已验证镜像，生产容器由维护者手动替换。积分、生图工作台、数据库、Redis、Nginx 和宿主首页/帮助静态资产不替换。

## 源码与功能

- 私有起点 `27f5e0c2ffa6ff50c37e9fb2d3bb6cc849aeaf1f`，生产 `0.2.11-52t.3`。
- 官方 `v0.2.15` peeled commit `f2669c8cf62555cd92389b3f55920e9e6e7c6ff2`；merge-base `96f4c115c9749078f90cbf210a01d39baf3f53b6`。
- 候选 `codex/upgrade-v0.2.15-compat` / `0.2.15-52t.1`。官方 Tag 中 VERSION 滞后为 `0.2.14`，私有版本与自己的 Tag 对齐；Go `1.27.2`。
- 保留积分签名幂等桥接；提链独立余额、原生分组/专属倍率、欠费恢复、退款与公共门户；既有媒体冻结释放核销和账号绑定；TTFT 开关与成功 attempt 样本；Codex 单值 48 KiB 保护；KeyingPay V2；现有首页/帮助/Logo/导航。
- 网关、协议、重试、缓存、调度和通用计费采用官方实现，Seedance 维持官方后扣费。

## 数据库与回滚

既有迁移全部未修改。新增迁移按完整文件名共存，checksum 按 UTF-8 `strings.TrimSpace` 内容计算：

| 文件 | SHA256 |
| --- | --- |
| `241_add_payment_order_bonus_amount.sql` | `18b6a524a9873d3e4a45e6f9bd03b6e69f4384b701825c659817e017f2e8244f` |
| `241_add_typesafe_platform.sql` | `b6559525bf8d0b5d7c617e7415944f0d1fae8c408e9131ace84139795f66dcd2` |
| `242_drop_platform_check_constraints.sql` | `a7725f31f637208d200e0f37d5270e9f263743d99fa9e4f1bb9a7387e8be5a61` |

新增订单赠送额列默认零，不回填订单或改余额；平台约束改为应用层注册清单校验。迁移为 forward-only，人工切换前备份数据库和 Compose/env、保留旧不可变镜像。旧镜像可忽略新列，但不支持新平台/配置；回滚前应停止使用新平台与充值优惠配置并核对在途订单。回滚镜像不等于回滚数据库。

## 验证与产物

| 检查 | 结果 |
| --- | --- |
| 合并 | 5 处冲突已处理，保留发布流程与两组资金集成测试；补齐可盈路由测试的新 Redis 参数 |
| 前端 | lint、Vue 类型检查、生产 build 通过；377 文件 / 2925 项 Vitest 通过 |
| 积分服务 | Go test / vet / build 通过；仅构建依赖安全补丁更新，业务源码及生产独立容器不变 |
| 后端 | 全量编译、vet、unit/default、二开重点回归通过；隔离 PostgreSQL/Redis 迁移和资金并发回归通过 |
| 部署契约 | 额度卡 Nginx 静态契约通过；Compose 网关环境与运行资源契约通过 |
| GitHub / Release / GHCR | PR #16 合入；Release run `38060183370` 成功；精确 Tag/双架构 revision 核验通过 |
| 服务器候选缓存 | 2026-10-10 22:59 北京时间已拉取并核验；版本与 pg_dump 冒烟通过，5 个运行容器前后完全不变 |
| 生产切换 | 维护者手动执行，本任务不执行 |

长上下文计费保留官方预设、阈值、账号/分组开关和渠道显式区间，不叠加重复倍率。新增回归证明可盈优惠订单按实付校验、含赠金一次入账；删除/退款提链的晚到结算不能扣创建者余额或残留 dedup/ledger。

`kimi-k3` 等缺失 usage 的零 token 事件不视为本轮自动解决，不估计扣费或追扣历史。既有非本轮新增边界：无 RefundID 的可盈待确认退款查询会使用到账口径构造金额关联，正常带 RefundID 路径不受影响；后续支付专项应单独评估。

## CI 构建依赖与本地证据

- PR [#16](https://github.com/hxly520/sub2api/pull/16)，已合入；PR CI [38059295184](https://github.com/hxly520/sub2api/actions/runs/38059295184) 与安全 [38059295135](https://github.com/hxly520/sub2api/actions/runs/38059295135) 全绿，精确 Tag Release [38060183370](https://github.com/hxly520/sub2api/actions/runs/38060183370) 重新执行门禁并全部通过。
- 首轮 points-security run `38059057233` 发现旧 Go 1.27.0 标准库及 x/text 0.39.0 已知漏洞。仅积分构建元数据升级为 Go 1.27.2、x/text 0.42.0 及其 x/sync 0.23.0 依赖；不改变积分业务代码，不发布或替换积分镜像。更新后本地 test/vet/build 通过。
- 本地首轮 backup 测试缺少 Windows PATH 中的 sh，补齐 Git sh 路径后仓库套件通过；私有可盈路由测试已适配官方新 Redis 参数。官方内存增长测试首轮在混合负载中超出阈值，未修改断言，service 完整重跑通过。
- 本地日志留在工作区外 `output/upgrade-plans/v215-*`；源码仓库只保留脱敏摘要与 CI 链接。

## 最终发布与服务器证据

- Release：[`v0.2.15-52t.1`](https://github.com/hxly520/sub2api/releases/tag/v0.2.15-52t.1)。因私有版本含 `-52t.1`，GitHub 显示 prerelease；这不表示使用官方未发布提交，目标固定为官方稳定 v0.2.15。
- 源码、annotated Tag peeled commit 与双架构 OCI revision：`8afa8d339eec95607ccca76386813e1874b53159`。后续主线文档提交不重新构建镜像，不改变该不可变发布。
- 镜像：`ghcr.io/hxly520/sub2api:0.2.15-52t.1`；manifest `sha256:bd08b5ba2f5c6ca527fff280c4b3b27bb93d4415a1f24f70a96172628f1ca948`。
- amd64 manifest `sha256:bdedf27afe2ea9be6789f5e2d6a0a62b05331117e443cafd3005d4f22d68b148`；arm64 manifest `sha256:5879b521b8da64a0e690a314daf48684037b8f86633b20205e4866d11fd8bedf`。
- 服务器 amd64 image ID `sha256:2f65c62b25a968962085826d44dfe7f5f410e32febf09e1e461654be363313fa`，创建时间 `2026-10-10T14:57:06.376720263Z`；容器内 `-version` 确认为 `0.2.15-52t.1`、commit 与上面相同，`pg_dump (PostgreSQL) 18.6` 运行正常。
- full GoReleaser `v2.17.1`、`SIMPLE_RELEASE=false`；包含 5 平台归档与 checksums。linux/amd64 归档 SHA256 `5433824ff33e3e179c18333362685373d8b308848e373d1f1e11fdb9d2621db1`，本地回读与 checksums 匹配。
- 服务器仅执行 pull 和无网络/无挂载的版本冒烟，不连接业务数据库。拉取前后 Sub2API、积分、工作台、PostgreSQL、Redis 的 ID/镜像/启动时间/状态/重启数严格相等；Sub2API 仍 `0.2.11-52t.3`、healthy、restart `0`。
- `/usr/local/bin/docker-compose` 已核验为 Compose `v2.27.1` 的兼容命令，现有服务名为 `sub2api`；未修改 Compose。

```yaml
image: ghcr.io/hxly520/sub2api:0.2.15-52t.1
```

## 人工切换

镜像已缓存。将 `/home/api/sub2api-deploy/docker-compose.yml` 的 Sub2API image 改为上述精确镜像，备份完成后执行：

```bash
cd /home/api/sub2api-deploy
docker-compose config --quiet
docker-compose up -d --no-deps sub2api
```

只更新 `sub2api` 服务。切换后检查健康、登录、普通 Key、长上下文计费、可盈订单、额度卡、积分桥接与媒体/视频；不得同时重建其他服务。
