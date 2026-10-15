# 全审计整改：第一阶段

## 本阶段交付

补齐 OneDrive 分片异常对账、S3/WebDAV 条件写入、跨业务目标锁、目录创建并发、下载可选摘要校验、分享身份与回执字段、离线完整列表对账、迁移错误回传、缓存完整性与搜索索引、可取消在线搜索、旧预览路由停用、关键 JSON 刷盘。

所有状态以 `AUDIT_IMPLEMENTATION_STATUS_2026-10-10.md` 为准。本阶段不是全部 65 项完成。后续继续处理分享转存最终回执、OneDrive 删除墓碑及条件能力、交互与工程项。

## 验证

- `go test ./...`、`go vet ./...`、`go build ./...` 通过。
- 前端 24 个文件、246 项测试通过；生产构建通过，保留既有大分块提示。
- 新增目标锁串行/取消、S3/WebDAV 条件失败、OneDrive 最终响应丢失、Graph 凭据受众隔离、缓存完整性/索引失效、离线缺项隔离、持久写失败、预览旧路由、单流/分段三类摘要回归。
- 迁移来源变化的旧断言改为要求非空错误，同时继续检查来源内容保留；不是删掉安全验收。

## 协议和验收边界

- [Microsoft Graph 分片上传](https://learn.microsoft.com/en-us/graph/api/driveitem-createuploadsession?view=graph-rest-1.0)：会话查询与顺序范围恢复；不把分片接收当作整个文件提交。
- [Microsoft Graph 异步复制](https://learn.microsoft.com/en-us/graph/api/driveitem-copy?view=graph-rest-1.0)：监控 URL 可能属于 SharePoint；Graph 分页只能使用 Graph 域名。非 Graph monitor 不转发 Graph Bearer，权限不符返回错误，不猜测跨受众认证。
- [AWS 条件写入](https://docs.aws.amazon.com/AmazonS3/latest/userguide/conditional-writes.html)、[HTTP 条件请求](https://www.rfc-editor.org/rfc/rfc9110.html)：服务端拒绝条件时停止，不重试无条件写入。第三方兼容实例是否执行条件必须实测。
- 文件哈希仅比较服务端明确提供的完整摘要，不将 multipart ETag、QuickXorHash 或 Dropbox 复合摘要解释为 MD5/SHA。
- 自动化测试不证明所有真实套餐、企业租户、系统钥匙环、磁盘断电或签名发布环境已验收。
