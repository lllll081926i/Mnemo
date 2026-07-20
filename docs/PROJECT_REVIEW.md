# Mnemo 项目审查报告

> **更新**：2026-07-20（二次审查，与当前 `main` 工作区对齐）  
> **首轮审查**：同日早些时候（旧定位：国内多盘 + 阿里中枢）  
> **版本**：`package.json` → `0.1.1-preview.1`  
> **维护**：lllll081926i  

本文档描述**当前**产品与工程状态。实现细节以仓库代码为准。

---

## 目录

1. [产品定位（当前）](#1-产品定位当前)
2. [相对首轮审查的变化](#2-相对首轮审查的变化)
3. [在役网盘能力矩阵](#3-在役网盘能力矩阵)
4. [产品面进度](#4-产品面进度)
5. [工程与架构](#5-工程与架构)
6. [文档与发布](#6-文档与发布)
7. [完成度总览](#7-完成度总览)
8. [风险与技术债](#8-风险与技术债)
9. [建议优先级](#9-建议优先级)
10. [依赖与内存（摘要）](#10-依赖与内存摘要)
11. [一句话结论](#11-一句话结论)

---

## 1. 产品定位（当前）

**Mnemo** — 免费多网盘桌面文件管理器（Electron + Vue 3）。

| 保留 | 已移出 / 不再登录 |
|------|-------------------|
| 多网盘文件管理、Aria HTTP 下载（懒启动）、PikPak 云离线、预览/播放、轻量分享 | 媒体库、媒体服务器、图书/AI、本地 BT、WebDAV Server、付费/Pro、RSS 工具箱 |
| | **登录已移除**：阿里云盘、夸克、139、189、光鸭、Nextcloud |

**在役登录**（`UserLogin.vue`）：

```text
pikpak（默认） · onedrive · dropbox · gdrive · gofile · webdav · s3
```

技术栈：Electron 40 · Vue 3 · Vite · Pinia · TypeScript · npm · Node ≥ 22.12

---

## 2. 相对首轮审查的变化

| 领域 | 首轮（约上午） | 当前（二次） |
|------|----------------|--------------|
| 默认盘 / 中枢 | 阿里 + 国内多盘 | **PikPak + 国际 OAuth + 挂载存储** |
| 登录入口 | 含阿里/夸克/139/189/光鸭等 | **仅 7 家在役** |
| Aria | 启动即起 | **`ensureEngineReady` 懒启动** |
| 上传窗 | 完整 main 入口 × 隐藏窗 | **`worker.html` 轻入口** |
| 下载隐藏窗 | 存在 | **已去掉 createDownload（需回归下载路径）** |
| 后台节流 | 全局禁用 | **默认 throttling；仅 worker 关闭** |
| 旧盘分派 | 大量 if/provider 分支 | **一串 refactor 阻断已移除盘** |
| 密钥模板 | 含阿里/PikPak client 等 | **OD/DB/GD + 字幕 + Apple** |
| markdown-it | 建议删除 | **已卸载** |
| 预览发布 | 未打通 | 远端有 `v0.1.1-preview.1` Pre-release（可能落后本地 commit） |

本地相对 `origin/main` 曾 **ahead 十余 commit**（清理分派、停用登录、perf 等）——以 `git status` 为准推送。

---

## 3. 在役网盘能力矩阵

来源：`src/utils/driveProvider.ts` 的 `driveProviderMap` + `driveProviderCapabilities`。

| 提供方 | 能力位摘要 | 实现观感 |
|--------|------------|----------|
| **PikPak** | 标准文件 + 分享 + 回收站视图；**云离线**（DownDAL） | 当前默认盘，中等完整 |
| **OneDrive** | 标准 + 搜索 + 创建分享 | 中等 |
| **Dropbox** | 标准 + 搜索 + 创建分享 | 中等偏上 |
| **Google Drive** | 标准 + 搜索 + 创建分享 + 回收站位 | 仍偏薄 |
| **GoFile** | 标准改写：无回收站、永久删除 + 创建分享 | 薄 |
| **WebDAV** | 挂载 + direct 上传、永久删除 | 基础可用 |
| **S3** | 同上 | 基础可用 |

**标准文件能力基线**（多数盘）：download / upload(queue) / mkdir / rename / move / copy / recycleBin（GoFile/WebDAV/S3 会覆盖回收站等）。

`canUseAliyunPreviewApi`：**恒 false**。侧栏代码中仍可能残留 `provider === 'aliyun'` 分支（死代码）。

`src/aliapi/` 仍保留较多文件，作历史统一模型/共享逻辑，**不是登录产品面**。

---

## 4. 产品面进度

### 网盘 `pan/`

- 目录树、列表、能力裁剪菜单：主干在  
- 登录与账号切换已切到 7 家  
- 旧盘 UI 分派持续清理中  

### 传输 `down/` + Aria

- Aria 多线程下载、任务分区、远程 Aria 等仍在  
- **懒启动**已落地（`MotrixApplication.ensureEngineReady`）  
- PikPak 云离线任务仍在 DownDAL  
- 光鸭离线路径已移除  

### 上传 worker

- `worker.html` + `createElectronWindow(..., 'worker', ..., backgroundThrottling: false)`  
- 主窗默认允许后台节流  

### 分享 / 预览 / 设置

- 分享按能力位；国际盘多为「创建分享」级  
- 预览：Artplayer / MPV / PDF / Office / 图片等；延迟加载有 commit  
- 阿里专属设置页已移除  

### 登录

- 聚合侧栏选择提供方  
- OD/DB/GD：应用内 OAuth + 本机回调服务  
- WebDAV / S3：表单连接  
- GoFile：Token  
- PikPak：账号流程（构建不再强绑 client secret 一类依赖）  

---

## 5. 工程与架构

| 项 | 状态 |
|----|------|
| 包管理 | 仅 npm |
| 密钥 | secrets.generated 不进 Git |
| 测试 | Vitest 显式 include；Windows 注意 CRLF（layoutShellPort 已 normalize） |
| 格式 | single quotes、无分号、printWidth 260 |
| 发布 | electron-builder；Windows workflow `release.yml` |
| 空目录 | `aisearch` / `book-*` / `reedy` / `nextcloud` 等可能仍为空壳 |

接入清单：`AGENTS.md`（14 步）。

---

## 6. 文档与发布

| 文档 | 状态（本次更新后） |
|------|-------------------|
| README.md / README.en.md | 对齐 7 家在役盘 + 致谢 |
| CONTRIBUTION.md | 个人维护、当前 secrets、目录 |
| AGENTS.md | 在役列表 + 已移除登录说明 |
| docs/PROJECT_REVIEW.md | 本文件（二次审查） |
| docs/releases/v0.1.1-preview.1.md | 预览说明（若存在） |

**发布**：

- GitHub 上可能已有 `v0.1.1-preview.1` 安装包  
- 若本地仍有未推送 commit，云端包**不等于**最新源码  
- package 版本需与 tag 一致（如 `0.1.1-preview.1`），避免 electron-builder 发错 tag  

---

## 7. 完成度总览

（主观，新定位下）

```
产品定位 / 登录收敛       ████████████████░░░░  ~80%
文档与代码一致            ████████████████████  ~95%（本次文档更新后）
PikPak 主链路             ██████████████░░░░░░  ~70%
OneDrive / Dropbox        █████████████░░░░░░░  ~65–70%
Google Drive / GoFile     ████████░░░░░░░░░░░░  ~40–55%
WebDAV / S3               ████████████░░░░░░░░  ~60%
传输 Aria + 懒启动        ██████████████████░░  ~90%
上传 worker 轻入口        ████████████████░░░░  ~80%
预览播放                  ████████████████░░░░  ~80%
旧盘死代码清理            ██████░░░░░░░░░░░░░░  ~35–45%
真实账号联调              ████░░░░░░░░░░░░░░░░  ~20%
最新代码已发布            视 git push / 是否 retag
```

| 目标 | 就绪度 |
|------|--------|
| 新盘集可演示 | 约 **65–70%** |
| 稳定首发 | 约 **45–55%** |

---

## 8. 风险与技术债

1. **半删除**：`tokenfrom` 类型、`aliapi/` 大包、侧栏 aliyun 分支、空目录  
2. **下载路径**：去掉 download 隐藏窗后需回归任务恢复与并发  
3. **预览鉴权**：阿里预览 API 已关，统一走直链/代理是否全覆盖  
4. **薄盘**：GDrive / GoFile 深度不足  
5. **本地未推送**：发布与文档若不同步会误导用户  
6. **依赖体积**：Ant + 全量 Arco、dashjs 等仍偏重（非阻断）  

---

## 9. 建议优先级

### P0

1. 推送本地 commit；需要时打 `v0.1.1-preview.2`  
2. `npm test` + `typecheck` + Windows `build:test`  
3. PikPak 主路径真人账号冒烟（登录→列表→传→预览→分享/云离线）  

### P1

4. 收窄 `DriveProvider` / `tokenfrom` 类型；删空目录；去掉 aliyun 死分支  
5. 明确 `aliapi/` 定位（rename / 只保留模型）  
6. OD/Dropbox/GDrive 分享与下载联调  

### P2

7. GDrive / GoFile 加深  
8. WebDAV/S3 预览鉴权  
9. dashjs 动态 import、UI 库瘦身  

---

## 10. 依赖与内存（摘要）

详见首轮分析逻辑，**已落地部分**：

- Aria 懒启动  
- 上传 worker 轻入口  
- 默认 `backgroundThrottling: true`  
- 移除全局 `disable-renderer-backgrounding`  
- 卸载 `markdown-it`  

**仍建议**：

- dashjs / 双 UI（Arco 全量 + Ant 局部）按需化  
- 预览窗减配、避免每个预览复制完整应用栈（若仍 WebOpenWindow 全量 main）  

环境变量：AI / BoxPlayer / TMDB / Supabase 等应保持不进产品 secrets（以 `.env.example` 为准）。

---

## 11. 一句话结论

Mnemo 已从「阿里 + 国内全家桶」收敛为 **PikPak + 国际 OAuth + WebDAV/S3 的多云文件管理预览版**：懒启动 Aria、轻量上传 worker、停用盘登录清理已到位；**文档本次已对齐在役 7 盘**。剩余重点是 **推送与预览包一致、死代码收口、薄盘做实、真实账号联调**。

---

## 相关文档

| 文档 | 用途 |
|------|------|
| [README.md](../README.md) | 产品说明 |
| [AGENTS.md](../AGENTS.md) | 工程约束与 provider 清单 |
| [DESIGN.md](../DESIGN.md) | UI 规范 |
| [CONTRIBUTION.md](../CONTRIBUTION.md) | 本地开发 |
| [CLAUDE.md](../CLAUDE.md) | Claude Code 指引 |

### 修订记录

| 日期 | 说明 |
|------|------|
| 2026-07-20 | 首轮：进度 + 依赖/内存 |
| 2026-07-20 | 二次：登录收缩为 7 盘、perf 落地、文档全量对齐 |
