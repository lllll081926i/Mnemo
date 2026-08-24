# 优享版蓝奏云（ilanzou）功能详情

> 调研范围：`internal/drive/providers/ilanzou/`（12 文件）
> 当前证据：本仓库 `internal/drive/providers/ilanzou`、统一能力注册表和本地自动化测试；本次未读取父目录旧项目。
> 状态：✅ 统一驱动已注册；可靠性以自动验证范围和已知限制为准，见 [Provider 状态总表](../PROVIDER_STATUS.md)。

---

## 能力声明（ilanzou.go:18-23）

```
permanentDelete: true
search, createShare, copy, recycleBin, trashView: false
SetHashes(["md5"], ["md5"])
```

---

## 1. 登录（auth.go / client.go）

| 子功能 | 状态 | Go 证据 | 差距 |
|--------|:----:|---------|------|
| 账密登录 | ✅ | `auth.go:10-25` → `client.go:290-340` ilanzouLogin /unproved/login → /proved/user/account/map | 无 |
| AES-128-ECB 签名 | ✅ | `crypto.go:12-28` aesEncryptToHex + `client.go:143-155` signParams | 无 |
| UUID 生成 | ✅ | `crypto.go:38-44` newDeviceUuid v4 + `client.go:257-281` fetchILanzouUuid 服务器 getUuid | 无 |
| RefreshAccount | ✅ | `client.go` account/map 校验、过期自动重登并更新容量 | 无（比旧版更完整） |
| 请求级自动重登 | ✅ | `client.go:86-130` request code -1/-2 或 token 空时重登重放 | 无 |
| 风控限速 | ✅ | `client.go:22-38` throttle 260ms | 无 |
| 自动重登持久化 | ✅ | `client.go` 重登成功后更新当前 `TokenInfo`，由 facade 统一落库 | 已覆盖请求级回归测试 |

---

## 2. 文件列表（ilanzou.go / dirfilelist.go）

✅ `ilanzou.go:54-67` List；`dirfilelist.go:9-41` /record/file/list 分页 offset+limit=60。对齐。

---

## 3. 下载（ilanzou.go / download.go / client.go）

| 子功能 | 状态 | Go 证据 | 差距 |
|--------|:----:|---------|------|
| GetDownloadURL | ✅ | `ilanzou.go:97-112` proxy；过期 401/403 自动重登后重试 | 无 |
| Referer | ✅ | `download.go:51` | 无 |
| Concurrency:1 | ✅ | `ilanzou.go:110` | 无 |
| 签名 URL 构建 | ✅ | `client.go:343-363` buildILanzouDownloadUrl AES 加密 downloadId+auth | 无 |
| 3xx Location 跟随 | ✅ | `download.go:53-56` manualClient 不跟随取 Location | 无 |
| 列表大小回填 | ✅ | `ilanzou.go` 优先使用已缓存的文件大小 | 无 |

---

## 4. 视频预览（ilanzou.go）

✅ `ilanzou.go:115-126` 复用 GetDownloadURL origin 原画并标记 `ForceProxy`；应用层统一交给本地网页播放会话代理。

---

## 5. 上传（upload.go）

| 子功能 | 状态 | Go 证据 | 差距 |
|--------|:----:|---------|------|
| MD5 计算 | ✅ | `upload.go:19-30` fileMD5 | 无 |
| getUpToken 秒传 | ✅ | `upload.go:62-81` POST /7n/getUpToken MD5 命中返回 fileId | 无 |
| 整包上传 ≤8MB | ✅ | `upload.go:93-119` multipart → upload.qiniup.com | 无 |
| 分片上传 >8MB | ✅ | `upload.go:121-161` initUpload → 逐片 PUT → complete（七牛分片） | 无 |
| 上传确认轮询 | ✅ | `upload.go` POST /7n/results 最多 10 次每秒，支持上下文取消并回写实际文件大小/文件 ID | 无 |

> ilanzou 是蓝奏系中唯一支持秒传 + 分片上传的。

---

## 6. 容量

✅ 登录和手动/启动刷新复用一次 `/proved/user/account/map` 请求，按 `totalSize + vipSize + rewardSize` 计算总量、按 `usedSize` 计算已用量（接口单位 KiB）。不增加独立轮询请求；优享版不使用普通蓝奏的格式或单文件大小拦截。

---

## 7. 文件操作（ilanzou.go / filecmd.go）

| 操作 | 状态 | Go 证据 | 差距 |
|------|:----:|---------|------|
| Mkdir | ✅ | `ilanzou.go:129-131` → `filecmd.go:9-27` /file/folder/save | 无 |
| Rename | ✅ | `ilanzou.go:133-145` → `filecmd.go:32-41` /file/edit 或 /file/folder/edit，kind fallback | 无 |
| Move | ✅ | `ilanzou.go:125-160` → `filecmd.go:55-79` /file/folder/move CSV，kind fallback | 无（支持文件夹） |
| Copy | ❌(设计) | `ilanzou.go:170-172` 返回空 | 无 |
| Delete | ✅ | `ilanzou.go:103-124` → `filecmd.go:44-53` /file/delete CSV 批量 + fallback | 无 |

---

## 8. 回收站 / 分享 / 搜索

❌ 均无（设计）。ilanzou `createShare:false`。

---

## 9. ProvideHashes / RapidUploadHashes

✅ **已声明**。

`ilanzou.go:26` SetHashes(["md5"], ["md5"]) — ProvideHashes 和 RapidUploadHashes 均已声明。

`upload.go` 的 `UploadOneFile` 与 `RapidUploadByHash` 都通过 `/7n/getUpToken` 判定 MD5 秒传；目标端命中时可直接创建远端文件，未命中则安全回退到常规迁移上传。

源端 `ResolveTransferHash` 优先读取统一文件缓存/列表映射中的合法 MD5。该方法仍保留 `allowStream=true` 时完整读取下载源计算 MD5 的能力，供明确接受额外读取成本的调用方使用；跨盘迁移引擎不会启用这一模式，只使用已有元数据/缓存指纹：

- 缓存中已有合法 MD5 时，迁移先尝试目标端秒传，不额外下载源文件。
- 缓存缺失、哈希无效或为空时，迁移跳过秒传探测，直接进入后续流式上传或临时文件 spool 回退，避免“先完整下载算 MD5、未命中后再次下载”的双次读取。
- 目标端拒绝已有 MD5 或秒传未命中时，迁移不会把任务误判为成功，而是安全回退到常规传输路径。

---

## 差距清单

1. ✅ **跨盘秒传已闭环**：`RapidUploadByHash` 命中 `/7n/getUpToken`；迁移源端只使用缓存/元数据中的 MD5，缓存缺失时直接进入常规传输，避免为秒传探测产生额外完整下载。`ResolveTransferHash` 的流式算 MD5 能力仍保留，但迁移引擎不启用。
2. ✅ **请求级重登会持久化新 session**：普通列表/文件操作遇到过期 token 时，重登后的 `appToken/uuid` 不再只在局部返回值中丢失。
3. ✅ **下载/预览会话续期**：下载重定向遇到 401/403 会按当前账号账密重登并重试，视频质量固定为原画代理源。
4. 其余功能与旧版对齐，无重大缺失
