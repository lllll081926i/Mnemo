# 蓝奏云（lanzou）功能详情

> 调研范围：`internal/drive/providers/lanzou/`（12 文件，含 acwscv2 反爬）
> 对照旧版：`../Mnemo/src/lanzou/` + `src/drive/providers/lanzou.ts`
> 整体完成度：✅ ~98%（源码级忠实移植）

---

## 能力声明（lanzou.go:14-21）

```
createShare, sharePassword, shareHistory: true
move, permanentDelete: true
search, copy, recycleBin, trashView: false
```

---

## 1. 登录（auth.go / client.go）

| 子功能 | 状态 | Go 证据 | 差距 |
|--------|:----:|---------|------|
| Cookie 登录 | ✅ | `auth.go:34-43` loginLanzouWithCookie | 无 |
| 账密登录 | ✅ | `auth.go:46-60` → `client.go:177-201` lanzouAccountLogin mlogin.php task=3 | 无 |
| acwscv2 反爬 | ✅ | `acwscv2.go:30-33` solveAcwScV2 + `client.go:49-68` fetchText 3 次重试 | 无（算法源码级移植） |
| RefreshAccount | ✅ | `lanzou.go:226-260` 校验 cookie + 失败时账密重登 | 无 |

---

## 2. 文件列表（lanzou.go / dirfilelist.go）

✅ `lanzou.go:49-63` List；`dirfilelist.go:9-26` task=47 文件夹 + task=5 文件翻页。对齐。

---

## 3. 下载（lanzou.go / download.go）

| 子功能 | 状态 | Go 证据 | 差距 |
|--------|:----:|---------|------|
| GetDownloadURL | ✅ | `lanzou.go:96-112` proxy | 无 |
| Referer | ✅ | `download.go:155-159` | 无 |
| Concurrency:1 | ✅ | `lanzou.go:110` | 无 |
| 分享页解析链 | ✅ | `download.go:37-120` lanzouResolveShareDownload 密码分享 + iframe + ajaxm.php + dom/file 跳转 + verification ajax.php | 无（最复杂链路完整移植） |
| 风控限速 | ✅ | `client.go:22-38` throttle 300ms | 无 |

---

## 4. 视频预览（lanzou.go）

✅ `lanzou.go:115-126` 复用 GetDownloadURL origin 原画。

---

## 5. 上传（upload.go）

| 子功能 | 状态 | Go 证据 | 差距 |
|--------|:----:|---------|------|
| 整包上传 | ✅ | `upload.go:27-95` html5up.php ≤200MB | 无 |
| 分片上传 | ❌ | 接口限制 | 无（设计） |
| 秒传 | ❌ | 接口限制 | 无（设计） |

---

## 6. 文件操作（lanzou.go / filecmd.go）

| 操作 | 状态 | Go 证据 | 差距 |
|------|:----:|---------|------|
| Mkdir | ✅ | `lanzou.go:152-154` → `filecmd.go:9-23` task=2 | 无 |
| Rename | ✅ | `lanzou.go:156-158` → `filecmd.go:26-35` task=46 文件 | 无 |
| Move | ✅(仅文件) | `lanzou.go:180-194` → `filecmd.go:38-46` task=20；文件夹报错"蓝奏暂不支持移动文件夹" | 无（AList 对齐） |
| Copy | ❌(设计) | `lanzou.go:197-199` 返回空 | 无 |
| Delete | ✅ | `lanzou.go:164-178` → `filecmd.go:49-52` task=6 文件 / task=3 文件夹 | 无 |

---

## 7. 回收站

❌ 无（设计如此，删除即永久）。`lanzou.go:159` Trash 返回空数组。

---

## 8. 分享（share.go）

| 子功能 | 状态 | Go 证据 | 差距 |
|--------|:----:|---------|------|
| CreateShare | ✅ | `share.go:14-49` fileShare task=22/18 取 f_id+pwd → 构造 ShareURL | 无 |
| 密码 | ✅ | API 返回的 pwd（用户不可自定义） | 无 |

> 限单文件/文件夹（fileIds.length != 1 报错），与旧版一致。

---

## 9. 搜索 / ProvideHashes

❌ 均无（设计）。`lanzou.go:12-21` NewCapabilities 第三参数 nil。

---

## 差距清单

无功能差距。lanzou 是最忠实的源码级移植之一，包括 acwscv2 反爬算法和 5 段式分享页下载解析链。
