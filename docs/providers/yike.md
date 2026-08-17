# 一刻相册（yike）功能详情

> 调研范围：`internal/drive/providers/yike/`（yike.go ~693 行 + upload.go ~259 行）
> 对照旧版：`../Mnemo/src/yike/` + `src/drive/providers/yike.ts`
> 整体完成度：✅ ~90%

---

## 能力声明（yike.go:38-48）

```
createFolder, photoAlbum, permanentDelete: true
createDateFolder: false
不声明 ProvideHashes/RapidUploadHashes  // 按需求不参与跨盘秒传
```

---

## 1. 登录（yike.go）

| 子功能 | 状态 | Go 证据 | 差距 |
|--------|:----:|---------|------|
| BDUSS/Cookie | ✅ | `yike.go:631-656` authLogin normalizeCookie 自动补 BDUSS= | 无 |
| getuinfo + getBdstoken | ✅ | 登录后调用；getuinfo 必需，bdstoken 获取失败可继续登录 | 无 |

---

## 2. 文件列表（yike.go）

| 子功能 | 状态 | Go 证据 | 差距 |
|--------|:----:|---------|------|
| 相册列表 album | ✅ | `yike.go:174-200` listRoot album 部分 GET albumAPI/list cursor 分页 limit=100 | 无 |
| 文件列表 | ✅ | `yike.go:202-228` listRoot loose files GET fileV1/list cursor | 无 |
| 相册内文件 | ✅ | `yike.go:230-258` listAlbum albumAPI/listfile cursor limit=1000 | 无 |
| **decryptYikeMd5** | ✅ | `yike.go:676-725` decryptYikeMd5（纯 hex 返回；否则 XOR position + 8-char block 重排） | 无 |
| **albumDirId** | ❌ | 无此概念 | 🟡 改用 album: 前缀 ID（`yike.go:610-617`），功能等价 |

---

## 3. 下载（yike.go）

✅ `yike.go:400-418` DownloadInfo POST fileV2/download 获取 dlink，proxy 模式。对齐。

---

## 4. 上传（upload.go）

| 子功能 | 状态 | Go 证据 | 差距 |
|--------|:----:|---------|------|
| UploadOneFile | ✅ | `upload.go:42-258` precreate → superfile2 分片 → create → addToAlbum | 无 |
| 分片 4MB | ✅ | `upload.go:23` | 无 |
| content-md5 + slice-md5 + block_list | ✅ | 秒传支持 | 无 |

---

## 5. 文件操作（yike.go）

| 操作 | 状态 | Go 证据 | 差距 |
|------|:----:|---------|------|
| Mkdir（建相册） | ✅ | `yike.go:260-273` CreateAlbum 仅根目录可建 | 无 |
| Rename（仅相册） | ✅ | `yike.go:602-610` RenameAlbum album: 前缀 | 无 |
| Move | ❌(设计) | `yike.go:612-614` NotSupported | 无 |
| Copy | ❌(设计) | `yike.go:616-618` NotSupported | 无 |

---

## 6. 回收站

❌ 无（设计）。`yike.go:620-622` Trash 直接转发 Delete，permanentDelete:true。

---

## 7. photoAlbum 相册模式

✅ `yike.go:41` photoAlbum:true。相册列表/浏览/创建/改名完整。

---

## 8. RefreshAccount（yike.go）

✅ `yike.go:624-630` getuinfo 刷新。⚠️ **未获取配额**（一刻相册 API 无配额接口）。

---

## 9. ProvideHashes

✅ `mapFile` 仍会通过 `decryptYikeMd5` 解析文件 MD5 并填入 `ContentHash`；能力位按需求不声明，因此不参与跨盘秒传路由。

---

## 差距清单

1. ✅ **decryptYikeMd5 已实现**（`yike.go:676-725`）：纯 hex 直接返回，非 hex 执行 XOR+重排解密
2. 🟡 RefreshAccount 无配额（一刻 API 限制）
3. albumDirId 改为 album: 前缀方案（功能等价，实现不同）
4. Move/Copy 不支持——与旧版设计一致，非差距
