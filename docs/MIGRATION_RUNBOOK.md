# 旧站迁移操作手册

## 1. 只读盘点

Windows 工作区示例：

```powershell
.\scripts\inventory-legacy.ps1 `
  -SitePaths ..\LongWay,..\agape,..\zk,..\zwlingyi,..\zwmouss `
  -OutputPath .\migration-reports\legacy-inventory-local.json
```

扫描器只读取 `config.json`、打卡 JSON 和资料目录，输出成员、周任务、打卡日期范围、资源大小、SHA-256 与重复文件组，不写旧站目录。

## 2. 试迁移

1. 先在统一平台创建目标小组并导入 Excel 名单。
2. 在“数据工具”上传旧 `config.json` 与 `records.json` 预览。
3. 未匹配姓名、异常日期和未知任务必须由组长确认，不自动合并。
4. 迁移源记录使用 `source_site + source_record_key` 唯一键，重复运行不得重复写入。

## 3. 正式迁移

1. 先运行 `backup.sh`，确认 `status.json` 为成功。
2. 每次仅导入一个小组。
3. 核对成员、任务、打卡、得着、资源总数及最早/最晚日期。
4. 抽查典型成员月历、补卡、资料下载与跨组拒绝。
5. 记录 `migration_batches` 批次状态和失败原因。

## 4. 灰度与回滚

旧站保持可用。试点组连续使用 7 天并确认后，旧入口才可进入只读/跳转；严重故障时恢复旧入口。停容器后旧数据至少保留 30 天，严禁直接删除目录。
