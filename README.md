# 统一门训打卡平台

一个网站承载多个门训小组。成员通过 Excel 名单提交注册申请，管理员审批后使用同一账号登录；登录后按所属小组自动分流。

## 当前试用版

- Excel 名单导入、预览和同步
- 名单匹配后提交注册申请
- 组长/小组管理员审批，审批通过才创建账号
- 单账号多小组数据模型与登录分流
- 今日打卡、历史记录、统计和资料库
- 简化的“三步发布本周任务”界面
- 超级管理员、小组组长、小组管理员、成员四级权限
- 旧网站 JSON 数据迁移能力

参考代码来自同级 `Discipleship` 目录，但本项目独立开发，不修改参考项目。

## Excel 名单格式

推荐使用一个工作表，第一行为以下表头：

| 小组编码 | 小组名称 | 成员姓名 | 是否组长 | 是否辅修 |
|---|---|---|---|---|
| truth-a | 真理 A 组 | 张三 | 是 | 否 |
| truth-a | 真理 A 组 | 李四 | 否 | 否 |

必填字段只有“小组名称”和“成员姓名”。建议填写稳定的小组编码；未填写时系统会根据小组名称生成稳定编码。“是否组长”和“是否辅修”可填“是/否”。旧版按工作表分组、红字标记组长的名单仍可兼容。

## 注册流程

1. 超级管理员导入 Excel 名单。
2. 成员选择小组并填写名单中的真实姓名。
3. 名单匹配后填写登录账号、密码和可选邮箱，提交申请。
4. 组长或小组管理员在“注册审批”中通过或拒绝。
5. 审批通过后成员才能登录。

平台不支持邀请码和名单外自由注册。

## 本地检查

```bash
cd backend
go test ./...

cd ../frontend
npm ci
npm run build
```

## Docker 启动

```bash
cp .env.example .env
# 修改 .env 中所有 CHANGE_ME 项
docker compose --env-file .env -f deploy/docker-compose.separated.yml up -d --build
```

默认入口：`http://NAS_IP:2980`

默认数据库端口只绑定本机：`127.0.0.1:3308`

运行数据统一保存在 `data/`：

```text
data/
├── mysql/
├── assets/
├── content/
├── roster/
├── migration-reports/
└── backups/
```

生产部署时建议将整个项目放在 `/volume2/docker/menxun-unified`，并把 `data/` 纳入备份。

## Git 管理

- `main` 只保存已验证、可部署版本。
- 开发统一使用 `codex/*`、`feature/*` 或 `fix/*` 分支。
- 禁止提交 `.env`、密码、私钥、生产数据和构建产物。
- 每次部署必须对应一个 Git 提交和带注释标签。
- NAS 只接收 `git archive` 生成的已提交文件，不直接上传未提交工作区。

完整规则见 [docs/GIT_WORKFLOW.md](docs/GIT_WORKFLOW.md) 和 [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md)。
