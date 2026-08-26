# 备份、恢复与监控

## 定时任务

在 NAS 任务计划中以 root 运行：

```sh
0 3 * * * /volume2/docker/menxun-unified/current/scripts/backup.sh
15 3 * * * /volume2/docker/menxun-unified/current/scripts/healthcheck.sh
```

`backup.sh` 使用一致性快照导出 MySQL，保留 14 个日备份；每月 1 日另存月备份并保留约 3 个月。结果写入 `data/backups/status.json`。`.env` 不进入 Git，应通过群晖加密备份单独保存。

附件由群晖快照/Hyper Backup 对 `data/assets` 做增量保护。数据库、附件、配置必须同时纳入灾备，不能只备份 SQL。

## 恢复演练

```sh
/volume2/docker/menxun-unified/current/scripts/restore-drill.sh
```

脚本创建临时数据库、导入最新日备份、读取账号/小组/打卡数量后自动删除临时库，不覆盖生产库。每季度至少运行一次并保存输出。

## 告警条件

`healthcheck.sh` 在 API/数据库不可用、`/volume2` 剩余空间低于 15% 或最近成功备份超过 36 小时时返回非零。群晖任务计划应配置失败邮件通知。
