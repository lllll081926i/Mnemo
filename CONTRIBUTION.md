# Mnemo 开发说明

**作者 / 维护**：lllll081926i  
仓库：https://github.com/lllll081926i/Mnemo

技术栈：TypeScript · Vue 3 · Vite · Electron · npm  
产品版本：`0.1.1-preview.x`

## 1. 获取源码

```bash
git clone https://github.com/lllll081926i/Mnemo.git
cd Mnemo
```

## 2. 安装依赖

```bash
npm install
```

（可选，国内镜像）

```bash
npm config set registry https://registry.npmmirror.com
```

## 3. 环境与密钥

以 `.env.example` 为模板创建 `.env.local`。当前构建/登录相关示例：

```dotenv
ONEDRIVE_CLIENT_ID=
ONEDRIVE_CLIENT_SECRET=
DROPBOX_APP_KEY=
DROPBOX_APP_SECRET=
GOOGLE_DRIVE_CLIENT_ID=
GOOGLE_DRIVE_CLIENT_SECRET=
SUBTITLE_SEARCH_API_KEY=
SUBTITLE_DOWNLOAD_API_KEY=
APPLE_ID=
APPLE_PASSWORD=
APPLE_TEAM_ID=
```

生成渲染进程密钥模块（不进 Git）：

```bash
npm run secrets:generate
```

产出：`src/secrets.generated.ts`。

**在役登录提供方**：PikPak、OneDrive、Dropbox、Google Drive、GoFile、WebDAV、S3。

## 4. 开发运行

```bash
npm run dev
```

## 5. 类型检查、测试与打包

```bash
npm run typecheck
npm run test
npm run build
npm run build:electron
npm run build:windows
npm run build:mac
npm run build:linux
npm run build:all
```

macOS 签名（可选）：配置 Apple 相关 env 后 `npm run build:mac:signed`。

## 6. 目录结构（简要）

```
electron/          # 主进程、预加载、Aria2（懒启动）、MPV、OAuth
worker.html        # 上传工作窗轻量入口
src/
  pikpak/ onedrive/ dropbox/ gdrive/ gofile/
  pan/ down/ share/ layout/ user/ setting/
  utils/           # driveProvider、WebDAV / S3 等
  aliapi/          # 历史统一文件模型 / 共享类型（非登录入口）
shared/
scripts/
static/engine/
```

新增网盘：[AGENTS.md](./AGENTS.md)。界面：[DESIGN.md](./DESIGN.md)。审查：[docs/PROJECT_REVIEW.md](./docs/PROJECT_REVIEW.md)。

## 7. 常见问题

| 问题 | 处理 |
|------|------|
| OAuth / 配置错误 | 检查 `.env.local`，重新 `npm run secrets:generate` |
| 仓库无真实密钥 | 设计如此 |
| 打包后登录失败 | 构建环境需注入密钥并生成 `secrets.generated.ts` |
| 如何加新网盘 | 按 AGENTS.md 清单实现 |

## 8. 致谢

- [rclone](https://rclone.org/)  
- [小白羊云盘](https://github.com/gaozhangmin/aliyunpan)

## 9. 反馈

[Issues](https://github.com/lllll081926i/Mnemo/issues)
