# 光鸭云盘（guangya）功能详情

> 调研范围：`internal/drive/providers/guangya/`（guangya.go ~693 行 + upload.go ~312 行）
> 当前证据：本仓库 `internal/drive/providers/guangya`、统一能力注册表、本地自动化测试，以及 `D:/Code/Mnemo/Example/LitePan/drivers/Guangya` 的参考实现。
> 状态：✅ 统一驱动已注册；可靠性以自动验证范围和已知限制为准，见 [Provider 状态总表](../PROVIDER_STATUS.md)。

---

## 能力声明（guangya.go:36-43）

```
copy, permanentDelete, createShare, shareExpiration, sharePassword, combinedShare, shareHistory: true
SetHashes(["md5"], ["md5"])
UploadConflictPolicies: ["rename"]
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

✅ `guangya.go:267-288` DownloadInfo → `:398-415` GetDownloadURL proxy；使用旧版稳定的 `/nd.bizuserres.s/v1/get_res_download_url`，优先解析 `data.signedURL`，兼容 `downloadUrl`。

---

## 4. 视频预览（guangya.go）

✅ `guangya.go:422-433` GetVideoPreview 复用 GetDownloadURL origin，视频统一交给网页播放器会话代理。

---

## 5. 上传（upload.go）

| 子功能 | 状态 | Go 证据 | 差距 |
|--------|:----:|---------|------|
| OSS multipart | ✅ | `upload.go:144-196` get_res_center_token → S3 multipart queueSize=2 → waitUploadTask | 无 |
| 普通上传内置秒传 | ✅ | `get_res_center_token` 返回 code=156 时等待任务完成 | 无 |
| 跨盘 MD5 秒传 | ✅ | `RapidUploadByHash`：创建资源任务 → `/userres/v1/check_can_flash_upload` 提交 taskId+md5 → 等待文件 ID | 明确未命中才回退普通上传 |
| 未命中任务复用 | ✅ | 秒传探测未命中时把完整资源中心 token 保存到统一上传会话；普通上传复用同一 taskId/OSS 凭据 | 避免再创建第二个远端任务 |
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

❌ 无（设计）。`guangya.go:466-468` Trash 直接转发 Delete。

---

## 8. 分享（share.go）

✅ `POST /nd.bizuserres.s/v1/share_file`；支持多文件/文件夹、永久或 1/7/30 天、可选自定义提取码。响应必须返回分享链接，业务失败会保留服务端错误。

---

## 9. RefreshAccount（guangya.go）

✅ 登录和手动容量刷新调用 `/assets/v1/get_assets`，读取 `data.usedSpaceSize` / `data.totalSpaceSize`；兼容数字和字符串字节数，并携带 `Did` / `Dt` 设备头。业务请求收到 401/403 时仍会刷新会话再重试。

---

## 10. ProvideHashes（guangya.go）

✅ `SetHashes(["md5"], ["md5"])`。详情兼容 `data.fileInfo` 与列表形态并读取 MD5，`ResolveTransferHash` 可补取源文件 MD5；目标通过明确的 `check_can_flash_upload` 接口实现秒传。

限制：当前服务端/参考实现仅可靠支持 `rename` 冲突策略，因此能力表只向迁移引擎声明 `rename`，不伪装支持覆盖或跳过。

---

## 差距清单

登录、容量、请求级续期、MD5 源指纹、目标秒传和普通上传链路均已接入。无法可靠提供 MD5 的源文件会跳过秒传，直接进入后续迁移策略。
