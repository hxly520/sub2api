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
| 积分服务 | Go test / vet / build 通过，源码及生产独立容器不变 |
| 后端 | 全量编译、vet 通过；unit/default、隔离 PostgreSQL 测试 pending |
| 部署契约 | 额度卡 Nginx 静态契约通过；其他门禁 pending |
| GitHub / Release / GHCR | pending |
| 服务器候选缓存 | pending |
| 生产切换 | 维护者手动执行，本任务不执行 |

长上下文计费保留官方预设、阈值、账号/分组开关和渠道显式区间，不叠加重复倍率。新增回归证明可盈优惠订单按实付校验、含赠金一次入账；删除/退款提链的晚到结算不能扣创建者余额或残留 dedup/ledger。

`kimi-k3` 等缺失 usage 的零 token 事件不视为本轮自动解决，不估计扣费或追扣历史。既有非本轮新增边界：无 RefundID 的可盈待确认退款查询会使用到账口径构造金额关联，正常带 RefundID 路径不受影响；后续支付专项应单独评估。

## 人工切换

待上表补齐成功镜像 digest 和服务器缓存证据后，将 `/home/api/sub2api-deploy/docker-compose.yml` 的 Sub2API image 改为精确候选，备份完成后执行：

```bash
cd /home/api/sub2api-deploy
docker compose config --quiet
docker compose up -d --no-deps sub2api
```

只更新 `sub2api` 服务。切换后检查健康、登录、普通 Key、长上下文计费、可盈订单、额度卡、积分桥接与媒体/视频；不得同时重建其他服务。
