# WebDAV 功能详情

> 调研范围：`internal/drive/providers/webdav/webdav.go` + `internal/provider/webdav/client.go`
> 对照旧版：`../Mnemo/src/utils/webdavClient.ts`
> 整体完成度：✅ ~95%

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
| **rootPath 前缀** | ✅ | `ConnConfig.RootPath` + client 的 endpointPath/逻辑路径转换；旧 `BasePath` 自动兼容 | 无 |

---

## 2. 文件列表（webdav.go / client.go）

✅ `webdav.go:65-78` client.List → PROPFIND Depth:1 XML 解析。Go 版自研 PROPFIND。

---

## 3. 下载（webdav.go）

✅ `webdav.go:98-114` GetDownloadURL Stat + client.DownloadURL（Endpoint+href 直链）redirect + Basic Auth。

> ✅ 请求 URL 按路径段编码，空格、Unicode、`#`、`?` 等文件名不会被误解析为 URL 控制符。

---

## 4. 上传（webdav.go / client.go）

| 子功能 | 状态 | Go 证据 | 差距 |
|--------|:----:|---------|------|
| direct PUT | ✅ | `webdav.go:196-208` client.Put HTTP PUT Content-Length | 无 |
| **冲突策略** | ✅ | `webdav.go:256` ResolveConflictPolicy refuse/rename/overwrite | 无 |
| **进度回调** | ✅ | `webdav.go:275` NewProgressReader 实时 DownSize/DownProcess | 无 |
| **递归目录上传** | ✅ | `internal/transfer/upload.go` 目录遍历 + `ensureRemoteParent` | 队列上传会先创建远端目录 |

---

## 5. 文件操作（webdav.go / client.go）

| 操作 | 状态 | Go 证据 | 差距 |
|------|:----:|---------|------|
| Mkdir | ✅ | `webdav.go:116-121` client.Mkcol HTTP MKCOL | 🟡 无 recursive（WebDAV 协议限制） |
| Rename | ✅ | `webdav.go:123-133` Stat + client.Move | 无 |
| Move | ✅ | `webdav.go` 遍历 client.Move | 自身/子目录目标在请求前拒绝 |
| Copy | ✅ | `webdav.go` 遍历 client.Copy | 自身/子目录目标在请求前拒绝 |
| Delete | ✅ | 直接 Delete（永久） | 无 |

---

## 6. 回收站 / 分享 / 搜索

❌ 均无（设计）。永久删除。

---

## 差距清单

1. ✅ **上传冲突策略已实现**（`webdav.go:256` ResolveConflictPolicy refuse/rename/overwrite）
2. ✅ **进度回调已实现**（`webdav.go:275` NewProgressReader + DownSize/DownProcess）
3. ✅ 上传支持递归目录（队列遍历并逐级创建远端目录）
4. ✅ Move/Copy 已增加自身/子目录防护
5. ✅ URL 路径段按 RFC 规则编码
6. 🟡 当前没有独立的 provider 级递归列出 API；同步引擎通过统一目录接口递归遍历
