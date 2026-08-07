# 提链额度卡验收记录（2026-08-08）

## 代码与制品

- UI 修复提交：`b3e230220a9dd023d133b4184a0c0a164ea95d51`。
- 修复内容：用户提链中心恢复标准 `AppLayout`，保留 Sub2API 左侧导航和顶部信息；回归测试锁定 `hideSidebar=false`。
- GitHub Actions Cachecompat Image：run `31201789117`，成功，`publish_latest=false`。
- 候选镜像：`ghcr.io/hxly520/sub2api:0.1.169-b3e230220a9d`。
- GHCR digest：`sha256:39baee21d5cfb259d5d903f5a8d54678b58a87345798539bb8a0246681110f81`。
- 服务器归档：`/home/api/sub2api-deploy/image-archives/sub2api-0.1.169-b3e230220a9d-linux-amd64.tar`，权限 `0600 root:root`。
- 归档 SHA256：`b3fe97a57a68d970e6d9c4c219b557c4b9ea3748aa55a858d829ad38420a98ad`。
- 候选 Image ID：`sha256:66992bb6c2e2d5a3bc99e83527da0ab8f132cad15e86ce68d33961dc8b8390a7`。

服务器只执行了归档校验和 `docker load`，没有执行 Compose、迁移、重启或替换运行容器。当前 Sub2API 仍为 `ghcr.io/hxly520/sub2api:0.1.169-daa7cb3fb460`，容器 `157eaffa7973`，healthy，restart `0`。

## 生产基线

- 用户 1：管理员、active；验收前后冻结余额为 `0`。
- 0.08x 分组：ID `9`，名称 `Plus 0.08x`，native `rate_multiplier=0.0800`，active、standard、非专属。
- 验收开始时提链 Key、授权、操作和账本均为 `0`；现有标准 Key 未被修改。
- 全局配置保持 `link_cards_enabled=false`、开发模式开启、开发名单仅 `[1]`、并发 `5`、RPM `0`、批量上限 `100`。

## 用户 1 真实验收

测试在服务端通过用户 1 管理员会话执行，金额均为单张 `1 U`，完整 Key 只在内存中使用并以 SHA256 前缀记录：

1. 临时授权分组 `9`，创建单张卡 A；返回 `pending_activation`，`1 U` 扣款，0.08x 转换为 `12.5` 1x quota。
2. 使用相同幂等键重放创建，返回同一卡且没有二次扣款或多发卡。
3. 通过 `POST /api/v1/public/link-cards/activate` 激活，公共 `/me` 与 `/usage` 均成功，未泄露完整 Key。
4. 使用该 Key 请求本机 `/v1/models` 返回 `200`，说明 Key 鉴权、分组绑定和网关入口可用；该接口未产生计费 usage。
5. 充值 `1 U` 后卡额度由 `12.5` 增至 `25`，余额再次准确扣减；冻结和解冻均按预期切换状态。
6. 管理员退款卡 A，退回 `2 U`，状态变为 `refunded`，创建者余额恢复。
7. 创建卡 B 后保持未激活，由创建者退款 `1 U`；重放退款幂等键不重复入账，旧 Key 激活返回 `404`。

验收结束时，提链资金流水净 `creator_balance_delta=0`，`issue=2`、`recharge=1`、`refund=2`；活动提链 Key 为 `0`。两张测试卡及不可变操作/账本行保留作审计证据，不执行物理删除。临时分组授权已撤销，授权表恢复为空。

验收窗口内用户 1 仍有其他正常业务请求，脚本结束后的余额相对脚本基线出现约 `0.026 U` 的独立消费波动；该差额不来自提链账本，不能用作提链退款误差。

## 后续切换边界

- 候选镜像已上传并缓存，但不得自动替换 Sub2API 容器；由维护者手工修改 Compose 标签并执行只针对 `sub2api` 的切换。
- 切换前继续保持全局关闭和用户 1 灰度；未得到单独确认前不得开放全体用户、恢复临时分组授权或删除验收审计行。
- 切换候选后必须重复标准 Key、移动端/桌面用户布局、公共额度卡页面和 `/v1/models` 鉴权冒烟。
