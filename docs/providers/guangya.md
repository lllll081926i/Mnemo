# 光鸭云盘（guangya）功能详情

> 调研范围：`internal/drive/providers/guangya/`（guangya.go ~693 行 + upload.go ~312 行）
> 对照旧版：`../Mnemo/src/guangya/` + `src/drive/providers/guangya.ts`
> 整体完成度：✅ ~98%（无明显差距）

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
| 短信验证码 | ✅ | `guangya.go:583-625` loginBySms 三步：SendSms → verify → signin | 无 |
| refresh_token | ✅ | `guangya.go:543-560` loginByRefreshToken | 无 |
| SendGuangyaSms | ✅ | `guangya.go:555-580` + `:688-692` Driver.SendSms 包装 | 无 |

---

## 2. 文件列表（guangya.go）

✅ `guangya.go:155-188` client.List POST /userres/v1/file/get_file_list page+pageSize=100 最多 200 页。对齐。

---

## 3. 下载（guangya.go）

✅ `guangya.go:201-212` DownloadInfo → `:385-397` GetDownloadURL proxy。对齐。

---

## 4. 视频预览（guangya.go）

✅ `guangya.go:399-410` GetVideoPreview 复用 GetDownloadURL origin。

---

## 5. 上传（upload.go）

| 子功能 | 状态 | Go 证据 | 差距 |
|--------|:----:|---------|------|
| OSS multipart | ✅ | `upload.go:96-165` get_res_center_token → S3 multipart queueSize=2 → waitUploadTask | 无 |
| 秒传 | ✅ | code=156 命中 | 无 |
| directPut 回退 | ✅ | 无 | 无 |

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

✅ `guangya.go:490-496` SpaceInfo /userres/v1/user/space 获取 usedSize/totalSize。对齐。

---

## 9. ProvideHashes（guangya.go）

✅ `guangya.go:43` SetHashes(["md5"], nil)。mapFile `f.ContentHash = it.MD5`。ResolveTransferHash 支持 md5（`:470-481`）。

---

## 差距清单

无明显差距。guangya 是最忠实的移植之一，上传（OSS S3 multipart + 秒传 + directPut 回退 + waitUploadTask 轮询）与旧版完全对齐。
