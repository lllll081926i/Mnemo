# OneDrive 功能详情

> 调研范围：`internal/drive/providers/onedrive/`（onedrive.go, auth.go, graph.go, upload.go）
> 对照旧版：`../Mnemo/src/onedrive/` + `src/drive/providers/onedrive.ts`
> 整体完成度：⚠️ ~70%（RefreshAccount 完全缺失是最大问题）

---

## 能力声明（onedrive.go:18-22）

```
createShare, shareExpiration, sharePassword: true
```
> trashView 未设；ProvideHashes 未声明（但实际映射了 hash）

---

## 1. 登录（auth.go）

| 子功能 | 状态 | Go 证据 | 差距 |
|--------|:----:|---------|------|
| OAuth PKCE S256 | ✅ | `auth.go:27-64` verifier+challenge | 无 |
| 本地回调 | ✅ | `auth.go:66-113` net.Listen 127.0.0.1:0 随机端口 + state 校验 + 10min 超时 | Go 版内置回调服务器 |
| Token 交换 | ✅ | `auth.go:134-166` POST msTokenURL | 🟡 缺 client_secret 传递 |
| client_id | ✅ | `auth.go:30-34` req.Config 或 onedrive_client_id | 🟡 无内置 fallback client_id |
| **Token 刷新** | ❌ | 无 RefreshAccount 重写，回退 BaseDriver 返回 nil | 🔴 旧版 refreshOneDriveAccessToken 完整逻辑未迁移 |
| **账号信息/配额** | ❌ | 不获取 /me 和 /me/drive 配额 | 🔴 旧版 applyOneDriveAccount |

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
| **断点续传** | ❌ | 无会话持久化 | 🔴 旧版 querySessionPosition + 416 恢复 + saveUploadSession |
| **416 处理** | ⚠️ | `upload.go:96-98` 遇 416 直接 return nil | 🟡 旧版查询实际位置跳过已传 |
| **会话取消** | ❌ | 无 | 🟡 旧版 cancelUploadSession DELETE |
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
| **搜索分页** | ❌ | 只取第一页 | 🟡 旧版跟 @odata.nextLink 循环 |
| **缩略图** | ❌ | URL 无 $expand=thumbnails | 🟡 旧版有 |
| **过滤器** | ❌ | 无 | 🟡 旧版 filterOneDriveSearchResults |
| **引号转义** | ⚠️ | url.QueryEscape | 🟡 旧版 OData `''` 转义（潜在 bug） |

---

## 10. RefreshAccount

❌ **完全缺失**。未重写 BaseDriver.RefreshAccount，返回 nil。

---

## 11. ProvideHashes

⚠️ `graph.go:460-472` mapItem 提取 sha1Hash/quickXorHash → ContentHash，但 Caps 未声明 ProvideHashes。

---

## 12. 版本历史（revisions）

❌ 完全缺失。旧版有 apiOneDriveListVersions / apiOneDriveRestoreVersion。

---

## P0 差距清单

1. 🔴 **Token 刷新完全缺失**：RefreshAccount 未实现，token 过期后无法续期
2. 🔴 **账号信息/配额缺失**：不获取用户信息和配额
3. 🔴 **上传断点续传缺失**：无会话持久化、无 416 位置查询、无会话取消
4. 🟡 上传冲突策略单一（仅 rename）
5. 🟡 搜索无分页、无缩略图、引号转义可能错误
6. 🟡 版本历史完全缺失
7. 🟡 client_secret 不传递、无内置 fallback client_id
8. 🟡 ProvideHashes 未在 Caps 声明
