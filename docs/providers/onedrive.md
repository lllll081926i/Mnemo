# OneDrive 功能详情

> 调研范围：`internal/drive/providers/onedrive/`（onedrive.go, auth.go, graph.go, upload.go）
> 对照旧版：`../Mnemo/src/onedrive/` + `src/drive/providers/onedrive.ts`
> 整体完成度：✅ ~90%

---

## 能力声明（onedrive.go:18-22）

```
search, createShare, shareExpiration, sharePassword, shareHistory: true
SetHashes(["sha1", "quickxorhash"], nil)
```
> trashView 未设；ProvideHashes 已声明

---

## 1. 登录（auth.go）

| 子功能 | 状态 | Go 证据 | 差距 |
|--------|:----:|---------|------|
| OAuth PKCE S256 | ✅ | `auth.go` verifier+challenge | 包含 `prompt=select_account`，便于多账号登录 |
| 本地回调 | ✅ | `auth.go` 127.0.0.1:0 + state/来源校验 + 10min 超时 | Go 版内置回调服务器 |
| Token 交换 | ✅ | `auth.go` POST msTokenURL | 按实际凭据传递 `client_secret`，同时发送 `code_verifier` |
| client_id / secret | ✅ | 请求配置、release secrets 或内置 rclone 凭据 | 自定义 client_id 只在同时提供 secret 时使用 secret |
| **Token 刷新** | ✅ | `onedrive.go` 的 `RefreshAccount` 调 `refreshOneDriveToken` | 缺省 `expires_in` 时保留旧过期时间 |
| **账号信息/配额** | ✅ | `onedrive.go:386-414` 获取 /me 和 /me/drive 配额 | 无 |

默认使用与旧版及 `Example/rclone` 一致的 OneDrive OAuth `client_id/client_secret`；发行版可通过 `secrets.json` 覆盖 client_id。登录后账号 ID 来自 Graph `/me`，默认 drive ID 使用 `onedrive:<drive-id>` 命名空间，避免多账号串用。

---

## 2. 文件列表（graph.go / onedrive.go）

| 子功能 | 状态 | Go 证据 | 差距 |
|--------|:----:|---------|------|
| List / ListPaged | ✅ | `graph.go:282-314` + `onedrive.go:56-76` | 无 |
| $select 字段 | ✅ | `graph.go:270` downloadUrl/hashes/thumbnails | 无 |
| $expand=thumbnails | ✅ | `graph.go:272` | 无 |
| 分页防环 | ✅ | `graph.go:306-310` seen map | 无 |
| 可信 URL 校验 | ✅ | `graph.go:50-58` trustedGraphURL 含 sharepoint/1drv | Go 版更严格 |

---

## 3. 下载（onedrive.go）

| 子功能 | 状态 | Go 证据 | 差距 |
|--------|:----:|---------|------|
| GetDownloadURL | ✅ | `onedrive.go:96-113` cl.Detail + DownloadURL | 🟡 多一次 Detail 调用（旧版从列表 item 取） |
| URL 来源 | ✅ | `graph.go:404-412` @microsoft.graph.downloadUrl → @content.downloadUrl → /content | Go 版多了 fallback |
| Authorization | ✅ | `onedrive.go:108-110` Bearer | 无 |
| 文件夹拒绝 | ✅ | `onedrive.go:102-104` | Go 版更健壮 |

---

## 4. 视频预览（onedrive.go）

✅ `onedrive.go:115-127` 复用 GetDownloadURL origin。Go 新增（旧版无独立模块）。

---

## 5. 上传（upload.go）

| 子功能 | 状态 | Go 证据 | 差距 |
|--------|:----:|---------|------|
| 小文件 PUT ≤4MB | ✅ | `upload.go:43-56` rawPut /content | 无 |
| 大文件 session | ✅ | `upload.go:59-110` CreateUploadSession + 10MB chunked PUT + Content-Range | 无 |
| **断点续传** | ✅ | `upload.go:65-128` UploadSessionKey + LoadUploadSession + SaveUploadSession + querySessionPosition 恢复 | 无 |
| **416 处理** | ✅ | `upload.go:67-72` querySessionPosition 查询实际位置跳过已传 | 无 |
| **会话取消** | ✅ | `upload.go:128` ClearUploadSession | 无 |
| 冲突策略 | ⚠️ | 硬编码 rename | 🟡 旧版 fail/replace/rename |
| **ignore 模式** | ❌ | 无 | 🟡 旧版先删同名再 overwrite |

---

## 6. 文件操作（graph.go / onedrive.go）

| 操作 | 状态 | Go 证据 | 差距 |
|------|:----:|---------|------|
| Mkdir | ✅ | `graph.go:249-261` /children conflictBehavior:rename | 无 |
| Rename | ✅ | `graph.go:264-272` PATCH name | 无 |
| Move | ✅ | `graph.go:281-284` PATCH parentReference | 无 |
| Copy | ✅ | `graph.go:287-340` /copy respond-async + monitor 轮询 10min | 无 |
| Copy 状态分类 | ✅ | `graph.go:342-375` 303/completed/failed | 🟡 旧版状态更细 |

---

## 7. 回收站

| 操作 | 状态 | 说明 |
|------|:----:|------|
| Trash | ✅ | `graph.go:275` DELETE（永久删除） |
| Restore | ❌ | `onedrive.go:162-164` NotSupported |
| TrashView | ❌ | 未设 |

---

## 8. 分享（graph.go / onedrive.go）

| 子功能 | 状态 | Go 证据 | 差距 |
|--------|:----:|---------|------|
| CreateShare | ✅ | `graph.go:378-402` /createLink type:view scope:anonymous | 无 |
| 过期时间 | ✅ | `graph.go:384-386` expirationDateTime | 无 |
| 密码 | ✅ | `graph.go:381-383` | 无 |

---

## 9. 搜索（graph.go）

| 子功能 | 状态 | Go 证据 | 差距 |
|--------|:----:|---------|------|
| Search | ✅ | `graph.go:317-325` /me/drive/root/search(q=...) | 无 |
| **搜索分页** | ✅ | `graph.go:238-261` 跟随 @odata.nextLink 循环 | 无 |
| **缩略图** | ❌ | URL 无 $expand=thumbnails | 🟡 旧版有 |
| **过滤器** | ❌ | 无 | 🟡 旧版 filterOneDriveSearchResults |
| **引号转义** | ✅ | OData 单引号加倍 + `url.PathEscape` | 无 |

---

## 10. RefreshAccount

✅ `onedrive.go:361-414` 调 refreshOneDriveToken 刷新 token，再获取 /me 和 /me/drive 配额。对齐。

---

## 11. ProvideHashes

✅ `onedrive.go:30` SetHashes(["sha1", "quickxorhash"], nil)。`graph.go:460-472` mapItem 提取 sha1Hash/quickXorHash → ContentHash。

---

## 12. 版本历史（revisions）

❌ 完全缺失。旧版有 apiOneDriveListVersions / apiOneDriveRestoreVersion。

---

## P0 差距清单

1. ✅ **Token 刷新已实现**（`onedrive.go` 的 `RefreshAccount`），刷新响应缺少 `expires_in` 时保留旧值
2. ✅ **账号信息/配额已实现**（`onedrive.go:386-414`）
3. ✅ **上传断点续传已实现**（`upload.go:65-128` UploadSession + querySessionPosition + ClearUploadSession）
4. ✅ **ProvideHashes 已声明**（`onedrive.go:30` SetHashes）
5. ✅ **搜索分页已实现**（`graph.go:238-261` 跟随 nextLink）
6. 🟡 上传冲突策略单一（仅 rename）
7. 🟡 搜索无缩略图；关键词的 OData 单引号和 URL 路径转义已修复
8. 🟡 版本历史完全缺失
9. ✅ **OAuth 默认凭据与 client_secret**（`auth.go`）：沿用旧版/rclone，登录与刷新均使用同一组凭据；登录会显示账号选择。
