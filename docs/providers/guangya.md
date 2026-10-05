# 光鸭云盘（guangya）功能详情

> 调研范围：`internal/drive/providers/guangya/`（guangya.go ~693 行 + upload.go ~312 行）
> 当前证据：本仓库 `internal/drive/providers/guangya`、统一能力注册表、本地自动化测试与官网网页脚本；旧项目说明不作为现行接口依据。
> 状态：✅ 统一驱动已注册；可靠性以自动验证范围和已知限制为准，见 [Provider 状态总表](../PROVIDER_STATUS.md)。

---

## 能力声明（guangya.go:36-43）

```
copy, offlineDownload, recycleBin, trashView, trashRestore, createShare, shareExpiration, sharePassword, combinedShare, shareHistory: true
permanentDelete: false
SetHashes(["md5"], nil)
ShareExpirationOptions: [0, 1, 7, 30]
```

---

## 1. 登录（guangya.go）

| 子功能 | 状态 | Go 证据 | 差距 |
|--------|:----:|---------|------|
| 短信验证码 | ✅ | `guangya.go` 的 SendSms → verify → signin，含静默 captcha 初始化与失效重取 | 无 |
| refresh_token | ✅ | `guangya.go` 的 loginByRefreshToken | 无 |
| SendGuangyaSms | ✅ | Driver.SendSms 返回 verification/device/captcha token，前端会带回登录 | 无 |
| 请求级刷新 | ✅ | `guangya.go` client.post 遇 401/403 自动 refresh_token、重试一次并落库 | 无 |

---

## 2. 文件列表（guangya.go）

✅ `guangya.go:155-188` client.List POST /userres/v1/file/get_file_list page+pageSize=100 最多 200 页。对齐。

---

## 3. 下载（guangya.go）

2026-10-05 修复下载探测与分段响应 `Last-Modified` 不一致导致立即失败的问题。相同强 ETag 优先确认资源身份，HTTP 日期按时间值比较而不是原始文本比较。光鸭启用内部 `AllowLastModifiedFallback`：无法以强 ETag 确认且修改时间不一致时，停止所有分段、废弃旧分片断点，从头下载一条完整响应；保留 ETag 条件和总长度校验，不混合旧分片。正常响应仍遵从用户并发设置。普通下载与目录同步均消费该策略。已完成本地 HTTP 回归，真实 CDN 下载仍需账号验收。

下载器兼容 Range 探测返回 `200 + Content-Length: 1` 的响应：不把该 1 字节当作完整文件大小，有效 `Content-Range` 可用于取总大小，但只有 `206` 才启用分段。单流开始后以完整响应的 `Content-Length` 更新任务大小，缺少长度时回退已知文件大小或在完成后记录实际字节数。前后端百分比均限制在 0–100%，历史异常进度也不会撑破进度条。对应回归测试：`internal/e2e/dlengine_test.go`、`internal/transfer/dlengine/progress_test.go`、`frontend/src/views/TransferView.test.js`。

✅ `guangya.go:267-288` DownloadInfo → `:398-415` GetDownloadURL proxy；使用旧版稳定的 `/nd.bizuserres.s/v1/get_res_download_url`，优先解析 `data.signedURL`，兼容 `downloadUrl`。

---

## 4. 视频预览（guangya.go）

✅ `guangya.go:422-433` GetVideoPreview 复用 GetDownloadURL origin，视频统一交给网页播放器会话代理。

---

## 5. 上传（upload.go）

| 子功能 | 状态 | Go 证据 | 差距 |
|--------|:----:|---------|------|
| OSS multipart | ✅ | `upload.go:144-196` get_res_center_token → S3 multipart queueSize=2 → waitUploadTask | 无 |
| 秒传 | ✅ | code=156 命中 | 无 |
| directPut 回退 | ✅ | `upload.go:186-189,226-252`，直传后校验 waitUploadTask 错误 | 无 |
| 本地文件尺寸 | ✅ | `upload.go:141-150` 先 `Stat` 再申请上传凭证 | 避免 `fileSize: 0` 破坏秒传/上传任务 |

> 500ms 限速（`guangya.go:145`）与旧版一致。

---

## 6. 文件操作（guangya.go）

| 操作 | 状态 | Go 证据 |
|------|:----:|---------|
| Mkdir | ✅ | `guangya.go:214-225` /userres/v1/file/create_dir + `:419-425` |
| Rename | ✅ | `guangya.go:227-229` /userres/v1/file/rename + `:427-434` |
| Move | ✅ | `guangya.go:231-233` /userres/v1/file/move_file + `:436-449` |
| Copy | ✅ | `guangya.go:235-237` /userres/v1/file/copy_file + `:451-464` |

---

## 7. 回收站

光鸭删除任务将文件移入回收站；`guangya.go` 使用 `get_file_list` 的 `dirType=4` 分页列举。`trash.go` 通过网页当前使用的 `POST /userres/v1/file/recycle_file` 还原，发送 `{fileIds}` 并查询 `/userres/v1/get_task_status`；仅任务状态为成功时返回成功。来源为官网网页脚本 `https://www.guangyapan.com/static/js/index.2795c2ad.js`，已完成模拟请求测试，尚待真实账号验证。普通 `delete_file` 仅能确定移入回收站，彻底删除能力仍关闭。

---

## 8. 分享（share.go）

✅ `POST /nd.bizuserres.s/v1/share_file`；支持多文件/文件夹、永久或 1/7/30 天、可选自定义提取码。响应必须返回分享链接，业务失败会保留服务端错误。

---

## 9. RefreshAccount（guangya.go）

✅ 登录和手动容量刷新调用 `/assets/v1/get_assets`，读取 `data.usedSpaceSize` / `data.totalSpaceSize`；兼容数字和字符串字节数，并携带 `Did` / `Dt` 设备头。业务请求收到 401/403 时仍会刷新会话再重试。

---

## 10. ProvideHashes（guangya.go）

✅ `guangya.go:42-43, 243-279` SetHashes(["md5"], nil)，详情兼容旧版 `data.fileInfo` 与列表形态并读取 MD5。`ResolveTransferHash` 支持 md5（`:500-513`）。

---

## 差距清单

登录、容量、请求级续期和上传链路均已接入；云离线接口已接入并开启，真实账号仍待验收，不能把模拟测试视为线上成功。

## 11. 云离线（offline.go）

- 支持 HTTP/HTTPS、magnet、ed2k，通过统一“传输 → 云离线”入口提交，文件保存在网盘，不是本地下载。
- 先调用 `/cloudcollection/v1/resolve_res`，含子文件的资源递归提取可添加索引，排除服务端 `excludeIndices`；再用 `/cloudcollection/v1/create_task` 提交。
- `/cloudcollection/v1/list_task` 游标分页读取任务与进度；0 排队、1 进行中、2 完成、3 失败、4 取消、5 部分完成。部分完成不强制 100%，并保留服务端错误。
- `/cloudcollection/v2/delete_task` 区分清除任务与同时删除网盘文件；有 `deleteFileTaskId` 时查询文件任务并等待成功。
- 沿用现有光鸭账号鉴权，续期后的凭据保存到原账号。业务失败、缺失任务 ID、重复分页游标和 429 均不冒充成功。
- 协议来源：2026-10-04 读取官网 `https://www.guangyapan.com/static/js/index.2795c2ad.js`。首版不提供种子上传、子文件勾选或任务重试界面，详见 `docs/specs/GUANGYA_OFFLINE.md`。
