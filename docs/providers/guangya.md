# 光鸭云盘（guangya）功能详情

> 调研范围：`internal/drive/providers/guangya/`（guangya.go ~693 行 + upload.go ~312 行）
> 当前证据：本仓库 `internal/drive/providers/guangya`、统一能力注册表和本地自动化测试；本次未读取父目录旧项目。
> 状态：✅ 统一驱动已注册；可靠性以自动验证范围和已知限制为准，见 [Provider 状态总表](../PROVIDER_STATUS.md)。

---

## 能力声明（guangya.go:36-43）

```
copy, permanentDelete: true
SetHashes(["md5"], nil)
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

❌ 无（设计）。`guangya.go:466-468` Trash 直接转发 Delete。

---

## 8. RefreshAccount（guangya.go）

✅ 登录完成后和 `RefreshAccount` 均调用 SpaceInfo `/userres/v1/user/space` 获取 usedSize/totalSize；业务请求收到 401/403 时会先刷新会话再重试。

---

## 9. ProvideHashes（guangya.go）

✅ `guangya.go:42-43, 243-279` SetHashes(["md5"], nil)，详情兼容旧版 `data.fileInfo` 与列表形态并读取 MD5。`ResolveTransferHash` 支持 md5（`:500-513`）。

---

## 差距清单

无明显差距。guangya 是最忠实的移植之一，登录（短信 captcha、refresh_token、请求级续期）和上传（OSS S3 multipart + 秒传 + directPut 回退 + waitUploadTask 轮询）均与旧版关键链路对齐。
