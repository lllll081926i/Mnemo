<p align="center">
  <img src="screenshot/icon.svg" alt="Mnemo" width="96">
</p>

<p align="center">
  <br> 中文 | <a href="./README.en.md">English</a>
</p>

<p align="center">
  <em><strong>Mnemo</strong> — 记住你的每一个网盘。</em>
</p>

<p align="center">
  免费 · 开源 · 多网盘统一管理 + 高速传输 + 在线预览播放
</p>

<p align="center">
  <img src="public/images/drive-icons/pikpak.svg" width="36" height="36" alt="PikPak" title="PikPak">
  <img src="public/images/drive-icons/onedrive.svg" width="36" height="36" alt="OneDrive" title="OneDrive">
  <img src="public/images/drive-icons/dropbox.svg" width="36" height="36" alt="Dropbox" title="Dropbox">
  <img src="public/images/drive-icons/gdrive.svg" width="36" height="36" alt="Google Drive" title="Google Drive">
  <img src="public/images/drive-icons/gofile.svg" width="36" height="36" alt="GoFile" title="GoFile">
  <img src="public/images/drive-icons/webdav.svg" width="36" height="36" alt="WebDAV" title="WebDAV">
  <img src="public/images/drive-icons/s3.svg" width="36" height="36" alt="S3" title="S3">
</p>

<p align="center">
  <img alt="License" src="https://img.shields.io/badge/license-GPL--3.0-blue?style=flat-square" />
  <img alt="Version" src="https://img.shields.io/badge/version-0.1.1--preview.1-orange?style=flat-square" />
  <img alt="TypeScript" src="https://img.shields.io/badge/-TypeScript-blue?style=flat-square&logo=typescript&logoColor=white" />
  <img alt="Vue" src="https://img.shields.io/badge/Vue.js-35495E?style=flat-square&logo=vuedotjs&logoColor=white" />
  <img alt="Electron" src="https://img.shields.io/badge/Electron-47848F?style=flat-square&logo=electron&logoColor=white" />
  <img alt="Windows" src="https://img.shields.io/badge/-Windows-blue?style=flat-square&logo=windows&logoColor=white" />
  <img alt="macOS" src="https://img.shields.io/badge/-macOS-black?style=flat-square&logo=apple&logoColor=white" />
  <img alt="Linux" src="https://img.shields.io/badge/-Linux-yellow?style=flat-square&logo=linux&logoColor=white" />
</p>

[![](https://img.shields.io/badge/-名字-blue)](#名字) [![](https://img.shields.io/badge/-功能-blue)](#功能) [![](https://img.shields.io/badge/-界面-blue)](#界面) [![](https://img.shields.io/badge/-安装-blue)](#安装与开发) [![](https://img.shields.io/badge/-架构-blue)](#架构) [![](https://img.shields.io/badge/-免责声明-blue)](#免责声明)

---

## 名字

**Mnemo** 取自希腊神话中的 **Mnemosyne（Μνημοσύνη）**——记忆女神，缪斯之母。

她守护记忆，也守护一切被讲述、被保存的事物。Mnemo 想做的也是这件事：把散落在各家网盘与对象存储里的文件，收进同一处记忆，随时可找、可传、可看。

---

# 功能

## 🌟 支持的网盘（在役）

能力由 `src/utils/driveProvider.ts` 统一声明，登录入口见 `src/user/UserLogin.vue`。菜单按真实 API 裁剪；**不支持的操作会隐藏或明确提示**。

当前在役提供方（默认登录 **PikPak**）：

| 网盘 | 登录方式 | 能力概览 |
|---|---|---|
| **PikPak** | 账号登录 | 列表、上传下载、分享、回收站视图、**云离线** |
| **OneDrive** | 应用内 OAuth（PKCE） | 列表、上传下载、搜索、创建分享、基础文件操作 |
| **Dropbox** | 应用内 OAuth（PKCE） | 列表、上传下载、搜索、创建分享、修订 / 缩略图等 |
| **Google Drive** | 应用内 OAuth（PKCE） | 列表、上传下载、搜索、创建分享、回收站相关（深度仍在补齐） |
| **GoFile** | API Token | 列表、上传下载、直链式分享、永久删除（无回收站） |
| **WebDAV** | 地址 + 账号密码 | 挂载存储、浏览、直传上传、基础文件管理与传输 |
| **S3** | Endpoint + 密钥 | 兼容 S3 的对象存储挂载、浏览、直传上传、基础管理 |

> **说明**
>
> - 产品版本：**0.1.1-preview.x（预览）**。以客户端实际菜单为准。
> - 多账号可同时登录并切换；侧边栏（如回收站、搜索）按能力位生成。
> - OneDrive / Dropbox / Google Drive 需在对应开发者控制台配置 OAuth 客户端，经 `npm run secrets:generate` 注入本地密钥。
> - 阿里云盘、夸克、139、189、光鸭等**已从登录入口与能力表移除**，不再作为产品能力宣传。

1. **多平台接入**：上表内置，统一账号与文件模型  
2. **多账号管理**：同时登录多个账号，快速切换  
3. **按盘裁剪能力**：列表、搜索、上传下载、重命名、移动、复制、删除、回收站、分享等按服务商能力展示  

## 📁 文件管理

4. **文件夹树 + 文件列表**  
5. **排序**：文件名 / 体积 / 时间  
6. **文件夹体积**（视盘与实现）  
7. **批量操作**：重命名、移动、复制、删除（视盘能力）  
8. **新建与属性**  
9. **搜索**（OneDrive / Dropbox / Google Drive 等开启搜索能力位）  
10. **大目录列表**、快速预览入口  

## ⚡ 高速传输

11. **Aria2c 多线程下载**：网盘直链分片下载到本地  
12. **懒启动引擎**：无任务时不常驻；首次下载等场景再拉起 Aria2  
13. **上传**：文件 / 文件夹；云盘队列上传或 WebDAV / S3 直传  
14. **任务中心**：下载中 / 已下载 / 上传中 / 已上传  
15. **轻量上传工作窗**：独立 `worker.html` 入口，减轻主窗负担  
16. **远程 Aria**、限速、断点续传、完成通知、传输防休眠（视配置与平台）  

## 🧲 网盘云离线

17. **PikPak 云离线**：磁力 / 链接提交到网盘服务器离线下载，文件落在云端  
18. 与本机 Aria HTTP 下载分离，互不混淆  

## 🎥 在线预览与播放

19. 视频：内置 Artplayer（HLS / DASH 等视源）+ MPV（视平台资源）  
20. 多音轨 / 字幕、倍速、同目录列表、进度记忆  
21. 需鉴权源：本地代理 / Header 传递  
22. 图片、PDF、Office、文本 / 代码、本地音频等文件预览（非独立媒体库产品）  

## 🔗 分享

23. **创建分享**（PikPak / OD / Dropbox / GDrive / GoFile 等按能力开启）  
24. 导入、我的分享、历史等：视提供方支持程度  

## ⚙️ 设置与体验

25. 主题、默认页签、关闭行为  
26. 账户、播放器、下载 / 上传、代理、日志  
27. 确认式自动更新（渠道以 GitHub Release 为准）  

## 🖥️ 支持的平台

| 系统 | 架构 | 安装包 |
|---|---|---|
| **Windows** | x64 | NSIS（`.exe`） |
| **macOS** | x64、arm64 | `.dmg` / `.zip` |
| **Linux** | x64、arm64 | `.AppImage` / `.deb` / `.pacman` |

构建：`npm run build:windows` / `build:mac` / `build:linux` / `build:all`。  
Windows 云端预览：推送 `v*` tag 或手动运行 `.github/workflows/release.yml`。

## 🗂️ 主界面

| Tab | 内容 |
|---|---|
| **网盘** | 多账号、目录树、文件列表 |
| **传输** | 上传 / 下载任务 |
| **分享** | 分享与导入（按能力） |
| **设置** | 外观、账号、播放、传输、代理、日志 |

---

# 界面

<img src="screenshot/drive_home.png" width="720" alt="网盘首页">

*多账号、文件夹树与文件列表统一管理。*

---

# 安装与开发

## 环境

- **Node.js ≥ 22.12**
- **npm**（使用 `package-lock.json`）

## 命令

```bash
npm install
npm run dev
npm run build
npm run build:electron
npm run test
npm run typecheck
```

密钥：复制 `.env.example` → `.env.local`，填写后：

```bash
npm run secrets:generate
```

生成 `src/secrets.generated.ts`（gitignore）。当前示例主要包括：

- OneDrive / Dropbox / Google Drive OAuth  
- 字幕相关 key（可选）  
- Apple 签名（macOS 发布，可选）

---

# 架构

```
electron/main/     主进程：窗口、IPC、Aria2（懒启动）、MPV、OAuth 回调、更新
electron/preload/  预加载桥
worker.html        上传工作窗轻量入口
src/
  pikpak/ onedrive/ dropbox/ gdrive/ gofile/
  pan/ down/ share/ layout/ user/ setting/
  utils/           driveProvider、WebDAV / S3、代理等
  aliapi/          历史统一文件模型与部分共享类型（非产品登录入口）
shared/
static/engine/     各平台 aria2 等资源
```

技术栈：**Electron 40 · Vue 3 · Vite · Pinia · TypeScript · npm**

- 接入清单：[AGENTS.md](./AGENTS.md)  
- 界面规范：[DESIGN.md](./DESIGN.md)  
- 工程审查：[docs/PROJECT_REVIEW.md](./docs/PROJECT_REVIEW.md)  
- 本地搭建：[CONTRIBUTION.md](./CONTRIBUTION.md)

---

# 免责声明

- 仅供学习与管理**自有**网盘 / 存储数据。  
- 请遵守各服务条款与当地法律。  
- API 变更、限流、封号等风险由使用者自担。  
- 协议：[LICENSE](./LICENSE)（GPL-3.0）。

---

# 致谢

- [rclone](https://rclone.org/) — 多云存储同步与访问的实践参考。  
- [小白羊云盘](https://github.com/gaozhangmin/aliyunpan) — 阿里云盘桌面端开源实现，文件管理与传输等方向上的公开思路与社区积累。

# 开发与反馈

见 [CONTRIBUTION.md](./CONTRIBUTION.md)、[AGENTS.md](./AGENTS.md)。  
Issues：https://github.com/lllll081926i/Mnemo/issues
