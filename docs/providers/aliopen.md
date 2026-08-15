# 阿里云盘（aliopen）功能详情

> 调研范围：`internal/drive/providers/aliopen/aliopen.go`（单文件，~810 行）
> 对照旧版：`../Mnemo/src/aliopen/`（10 文件）+ `src/drive/providers/aliopen.ts`
> 整体完成度：✅ ~90%

---

## 能力声明（aliopen.go:39-53）

```
search, createShare, shareExpiration, sharePassword, shareHistory, importShare: true
copy, recycleBin, permanentDelete: true
trashView, trashRestore: false
SetHashes(["sha1"], ["sha1"])
```

---

## 1. 登录（aliopen.go）

| 子功能 | 状态 | Go 证据 | 差距 |
|--------|:----:|---------|------|
| refresh_token 粘贴 | ✅ | `aliopen.go:756-809` authRefreshToken | 无 |
| client_id/client_secret | ✅ | LoginConfig `aliopen.go:34-37`，client_id 可选走 AList 公共换票 | 无 |
| token 刷新 | ✅ | `aliopen.go:193-222` client.refresh | ⚠️ 无 429 限流惩罚、无 401 自动重试、无并发限速 |
| ensureDrive | ✅ | `aliopen.go:224-238` getDriveInfo | 无 |
| **限速/重试** | ❌ | apiPost 单次请求 | 🟡 旧版 concurrency:2 + 220ms + 429 penalize + 401 重试 |

---

## 2. 文件列表（双盘 b:/r:）

| 子功能 | 状态 | Go 证据 | 差距 |
|--------|:----:|---------|------|
| 双盘作用域 | ✅ | `aliopen.go:109-143` parseRef/wrapRef，b:/r: 前缀 | 无 |
| 根目录虚拟盘入口 | ✅ | `aliopen.go:654-672` 备份盘+资源盘 | 无 |
| ListPage | ✅ | `aliopen.go:269-285` `/adrive/v1.0/openFile/list` limit=200 | ⚠️ 未传排序参数、无游标重复检测 |

---

## 3. 下载（aliopen.go）

| 子功能 | 状态 | Go 证据 | 差距 |
|--------|:----:|---------|------|
| GetDownloadURL | ✅ | `aliopen.go:309-324` getDownloadUrl | 无 |
| DownloadMode | redirect | `aliopen.go:702-713` | 无 |
| **`.livp` 回退** | ❌ | 无 | 🟡 旧版 `.livp` 回退 streamsUrl.jpeg/mov |
| **expire_sec** | ❌ | 未传 | 🟡 旧版传 14400(4h) |

---

## 4. 视频预览（aliopen.go）

⚠️ `aliopen.go:702-712` 直接调用 GetDownloadURL 返回单一 origin 原画，无转码清晰度。
> Go 和 TS 均未实现真正的转码视频预览（阿里有 `/adrive/v1.0/openFile/getVideoPreviewPlayInfo` 接口但未调用）。

---

## 5. 上传（aliopen.go）

| 子功能 | 状态 | Go 证据 | 差距 |
|--------|:----:|---------|------|
| SHA1 秒传 | ✅ | `aliopen.go:438-468` RapidUpload | 无 |
| CreateUploadFile | ✅ | `aliopen.go:483-505` `/adrive/v1.0/openFile/create` | 🔴 未传 pre_hash/content_hash/proof_code/local_modified_at |
| 分片上传 | ✅ | 固定 partSize=10MiB | 🟡 旧版 calPartSize 8 档(20MB-5GB) |
| **CompleteUpload** | ✅ | `aliopen.go:533-540` 传真实 uploadID (`aliopen.go:912`) | 无 |
| **断点续传** | ✅ | `aliopen.go:873-914` UploadSession（LoadUploadSession + SaveUploadSession + ClearUploadSession） | 无 |
| **pre_hash 探测** | ❌ | 无 | 🟡 旧版前 1KB SHA1 探测提升秒传命中率 |
| **proof_code** | ❌ | 无 | 🟡 旧版 calProofCode |
| **分片地址过期刷新** | ❌ | 无 | 🟡 旧版 50min 刷新 |
| **同名冲突** | ❌ | 固定 check_name_mode:ignore | 🟡 旧版 refuse/ignore/overwrite |

---

## 6. 秒传（aliopen.go）

| 子功能 | 状态 | Go 证据 | 差距 |
|--------|:----:|---------|------|
| RapidUploadByHash | ✅ | `aliopen.go:792-803` 校验 sha1 | 无 |
| ResolveTransferHash | ✅ | `aliopen.go:805-815` Detail 读 content_hash | 🟡 无 SHA1 格式校验 |

---

## 7. 文件操作（aliopen.go）

| 操作 | 状态 | Go 证据 | 差距 |
|------|:----:|---------|------|
| Mkdir | ✅ | `aliopen.go:326-341` create type=folder refuse | 无 |
| Rename | ✅ | `aliopen.go:343-350` update | 无 |
| Move | ✅ | `aliopen.go:367-374` move | 🟡 跨盘 Move 静默跳过（旧版明确报错） |
| Copy | ✅ | `aliopen.go:376-384` copy 含 to_drive_id | 无 |

---

## 8. 回收站（aliopen.go）

| 操作 | 状态 | Go 证据 | 差距 |
|------|:----:|---------|------|
| Trash | ✅ | `aliopen.go:352-361` recyclebin/trash | 无 |
| Delete | ✅ | `aliopen.go:363-372` delete | 无 |
| Restore | ❌ | `aliopen.go:774-775` NotSupported | 设计（旧版也无） |

---

## 9. 分享（aliopen.go）

| 子功能 | 状态 | Go 证据 | 差距 |
|--------|:----:|---------|------|
| CreateShare | ✅ | `aliopen.go:386-416` createShareLink | 无 |
| **分享导入 importShare** | ✅ | `aliopen.go:952-1027` ImportShareSession（getShareToken → listByShare → SaveShare `aliopen.go:1028-1055`） | 无 |

---

## 10. 搜索（aliopen.go）

✅ `aliopen.go:303-316` `/adrive/v1.0/openFile/search` 双盘并行，资源盘失败静默降级。对齐。

---

## 11. RefreshAccount（aliopen.go）

✅ `aliopen.go:817-834` refresh + GetSpaceInfo。对齐。

---

## P0 差距清单

1. ✅ **CompleteUpload 传真实 upload_id**（`aliopen.go:912`）：已修复
2. ✅ **分享导入已实现**（`aliopen.go:952-1055` ImportShareSession + SaveShare）
3. ✅ **上传断点续传已实现**（`aliopen.go:873-914` UploadSession）
4. 🟡 上传无 pre_hash/proof_code：秒传命中率降低
5. 🟡 无 401 自动重试、无并发限速
6. 🟡 上传分片大小固定 10MB（大文件可能超 10000 片限制）
7. 🟡 `.livp` 下载无 streamsUrl 回退
8. 🟡 跨盘 Move 静默跳过无错误反馈
