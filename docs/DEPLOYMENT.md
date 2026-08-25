# NAS 部署与回滚

## 固定路径

```text
/volume2/docker/menxun-unified/
├── releases/<commit>/   # Git 提交归档
├── current -> releases/<commit>
├── shared/.env
├── shared/data/
└── deploy-history.log
```

Compose 运行目录使用当前发布版本，但 `.env` 和 `data/` 始终来自 `shared/`。发布时不覆盖生产数据。

## 发布原则

1. 本地测试通过并提交。
2. 使用 `git archive` 打包指定提交。
3. 上传到新的 `releases/<commit>` 目录。
4. 将共享 `.env` 和 `data` 连接到发布目录。
5. 运行 `docker compose config` 验证。
6. 构建并启动，健康检查通过后更新 `current`。
7. 写入 `deploy-history.log`。

## 回滚

回滚只切换到上一个 release 并重新执行 Compose；不得删除 `shared/data`。数据库迁移默认只向前兼容，因此回滚前必须检查新迁移是否改变了旧版本无法识别的数据结构。

首次试部署不替换任何旧网站，也不复用旧站点端口。试用入口默认为 `2980`。
