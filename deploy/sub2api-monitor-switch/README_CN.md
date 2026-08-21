# 渠道监控定时开关

`sub2api-monitor-switch.sh` 控制的是 `settings.channel_monitor_enabled`，即
Sub2API 的渠道监控/探测任务。它不会修改 `channels.status`，也不会关闭模型
请求入口。

## 生产路径

服务器部署路径：

```text
/home/api/sub2api-deploy/sub2api-monitor-switch/sub2api-monitor-switch.sh
```

脚本通过 `sub2api-postgres` 容器内的 `psql` 更新同一份 Sub2API 数据库，使用
固定的 `Asia/Shanghai` 主机时区。日志写入：

```text
/var/log/sub2api-monitor-switch.log
```

## Cron

root crontab 中保留以下两条任务，脚本自身带并发锁，重复安装不会产生重复执行：

```cron
0 0 * * * /home/api/sub2api-deploy/sub2api-monitor-switch/sub2api-monitor-switch.sh off >>/var/log/sub2api-monitor-switch.log 2>&1
0 8 * * * /home/api/sub2api-deploy/sub2api-monitor-switch/sub2api-monitor-switch.sh on >>/var/log/sub2api-monitor-switch.log 2>&1
```

手工检查：

```sh
/home/api/sub2api-deploy/sub2api-monitor-switch/sub2api-monitor-switch.sh status
crontab -l
tail -n 50 /var/log/sub2api-monitor-switch.log
```

如需停止定时任务，删除带有 `sub2api-monitor-switch.sh` 的两条 root cron，
不要删除脚本本身。

## 与“关闭所有渠道”不同

如果需求是每天 00:00 将所有渠道的 `channels.status` 改为 `disabled`，并在
08:00 恢复请求能力，那是另一项高影响操作，需要单独设计状态快照和缓存失效
流程；不能把本脚本当作全站请求开关。
