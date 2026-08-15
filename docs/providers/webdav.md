# WebDAV 功能详情

> 调研范围：`internal/drive/providers/webdav/webdav.go` + `internal/provider/webdav/client.go`
> 对照旧版：`../Mnemo/src/utils/webdavClient.ts`
> 整体完成度：⚠️ ~75%

---

## 能力声明（webdav.go:20-35）

```
mountedStorage, permanentDelete: true
recycleBin, trashView: false
UploadMode: direct
```

---

## 1. 连接配置（webdav.go / account.go）

| 子功能 | 状态 | Go 证据 | 差距 |
|--------|:----:|---------|------|
| URL + 账密 | ✅ | `webdav.go:43-49` clientOf 从 c.Token.Conn 取 Endpoint+Username+Password | 无 |
| 连接持久化 | ✅ | account.go Token.Conn *ConnConfig 持久化 | 无（服务端存储） |
| **rootPath 前缀** | ❌ | ConnConfig 无 RootPath 字段 | 🟡 旧版 WebDavConnectionConfig.rootPath + joinDavPath |

---

## 2. 文件列表（webdav.go / client.go）

✅ `webdav.go:65-78` client.List → PROPFIND Depth:1 XML 解析。Go 版自研 PROPFIND。

---

## 3. 下载（webdav.go）

✅ `webdav.go:98-114` GetDownloadURL Stat + client.DownloadURL（Endpoint+href 直链）redirect + Basic Auth。

> 🟡 旧版对路径段 encodeURIComponent，Go 版直接拼接不编码。

---

## 4. 上传（webdav.go / client.go）

| 子功能 | 状态 | Go 证据 | 差距 |
|--------|:----:|---------|------|
| direct PUT | ✅ | `webdav.go:196-208` client.Put HTTP PUT Content-Length | 无 |
| **冲突策略** | ❌ | 无 resolveProviderPathConflictAction | 🔴 直接覆盖同名文件 |
| **进度回调** | ❌ | TODO 未接线 | 🟡 旧版有 |
| **递归目录上传** | ❌ | 仅单文件 | 🟡 旧版 uploadWebDavEntry 递归 |

---

## 5. 文件操作（webdav.go / client.go）

| 操作 | 状态 | Go 证据 | 差距 |
|------|:----:|---------|------|
| Mkdir | ✅ | `webdav.go:116-121` client.Mkcol HTTP MKCOL | 🟡 无 recursive（WebDAV 协议限制） |
| Rename | ✅ | `webdav.go:123-133` Stat + client.Move | 无 |
| Move | ✅ | `webdav.go:154-169` 遍历 client.Move | 🟡 无自嵌套防护 |
| Copy | ✅ | `webdav.go:171-186` 遍历 client.Copy | 🟡 无自嵌套防护 |
| Delete | ✅ | 直接 Delete（永久） | 无 |

---

## 6. 回收站 / 分享 / 搜索

❌ 均无（设计）。永久删除。

---

## 差距清单

1. 🔴 **上传冲突策略完全缺失**：直接覆盖同名文件，无 refuse/overwrite/rename
2. 🟡 **rootPath 前缀拼接缺失**
3. 🟡 上传无进度回调、无递归目录上传
4. 🟡 Move/Copy 无自嵌套防护
5. 🟡 URL 路径段不编码
6. 🟡 无递归列出方法（旧版 listWebDavRecursive 供同步引擎用）
