# 光鸭云离线接入规范

## 范围与协议依据

- 在现有光鸭插件实现 `drive.OfflineDriver`，同时开启 `offlineDownload`；复用传输页，不增加独立登录或下载器。
- 协议来源：2026-10-04 读取官网 `https://www.guangyapan.com/static/js/index.2795c2ad.js` 的云添加代码。
- 提交前调用 `POST /cloudcollection/v1/resolve_res`，参数为 `{url}`；使用返回的规范化 URL。含子文件的资源递归提取 `fileIndex`，排除 `excludeIndices`，提交全部可添加文件。
- 提交：`POST /cloudcollection/v1/create_task`，参数为 `{url,parentId,newName?,fileIndexes?}`，必须返回 `data.taskId`。
- 列表：`POST /cloudcollection/v1/list_task`，使用 `pageSize`、`status` 数组与 `cursor`；响应 `data.list`、`hasMore`、`cursor`。限制分页次数并拒绝循环游标。
- 状态：0 排队、1 进行中、2 完成、3 失败、4 取消、5 部分完成；仅状态 2 强制 100%，部分完成不得冒充全部完成。
- 删除：`POST /cloudcollection/v2/delete_task`，参数为 `{taskIds,deleteFile}`；若返回 `data.deleteFileTaskId`，查询现有文件任务接口确认删除结果。
- 首版支持统一入口的 HTTP/HTTPS、magnet、ed2k 链接；不增加种子文件上传、文件选择或重试任务界面。

## 失败处理与验收

- 业务非零错误码、缺失数据/任务 ID、无法添加的资源不能返回成功；429 不循环重试。
- 沿用光鸭请求鉴权与一次续期，刷新凭据必须保存；不改动其他文件功能。
- 传输页必须实际请求远端状态，不能只读本地创建记录；远端查询失败显示错误并保留本地缓存。
- 覆盖资源解析、子文件索引、分页、状态/百分比、删除、业务错误、取消、凭据续期持久化、应用层和前端能力入口。
- 自动化测试通过不等同真实账号验收；不生成验证版或安装包。
