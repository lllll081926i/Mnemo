# AGENTS.md — Mnemo

## Product

**Mnemo** is a free multi-cloud desktop file manager (Electron + Vue 3).  
Mythology: Mnemosyne — memory.

**Active providers** (login + `driveProvider`):  
`pikpak` · `onedrive` · `dropbox` · `gdrive` · `gofile` · `pan123` · `lanzou` · `ilanzou` · `pan139` · `pan189` · `yike` · `aliopen` · `guangya` · `webdav` · `s3`

Default login provider: **PikPak**.

## Quick reference

```bash
npm install          # npm only (package-lock.json)
npm run dev
npm run build        # typecheck → vite build
npm run test
npm run secrets:generate
```

## Package manager / Node

- Lockfile: `package-lock.json` only  
- `engines.node >= 22.12.0`

## Build order

`npm run build` = **typecheck → vite bundle** (clears `dist/`; full electron build also clears `release/`).  
`npm run build:electron` runs build then electron-builder. Product name: **Mnemo**, appId `com.mnemo.app`.  
Version line: `0.1.1-preview.x` during preview.

## Secrets

Real keys: `.env.local` / GitHub Secrets → `scripts/generate-secrets.mjs` → `src/secrets.generated.ts` (gitignored).  
Typical: OneDrive / Dropbox / Google Drive OAuth, optional subtitle keys, Apple signing.

## Architecture

| Directory | Purpose |
|---|---|
| `electron/main/` | Main process (Aria lazy-start, OAuth callback, windows) |
| `electron/preload/` | Preload |
| `worker.html` + `src/workerpage/` | Light upload worker entry |
| `src/` | Vue renderer + providers |
| `shared/` | Shared code |
| `scripts/` | Build / secrets |
| `static/engine/` | aria2 / platform resources |

Providers under `src/`（各盘独立文件夹，对齐 AList driver）：`pikpak/` `onedrive/` `dropbox/` `gdrive/` `gofile/` `pan123/` `lanzou/` `ilanzou/` `pan139/` `pan189/` `yike/` `aliopen/` `guangya/`。  
Mounted storage: `utils/webdavClient.ts` `utils/s3Client.ts`。  
**Drive plugin layer:** `src/drive/` — `registry` + `ops` + `providers/*`；UI/传输只走 `drive/ops`，禁止中央 `if (provider === …)`。  
加盘：`src/<id>/` API → `src/drive/providers/<id>.ts` + `providers/index.ts` 注册。  
共用：`providerRateLimit`（防风控）、`providerUpload`、`driveSyncAdapter`、`proxyhelper` 头透传。  
**ProviderNet 中继**：小网盘 API 无 CORS 且需 Cookie/Referer 等禁用头，渲染进程 fetch 必挂。`public/global-shim.js` 将中继域名的 fetch 透明转发到主进程 `ProviderNet:request`（`ipcEvent.ts`，net.fetch + 域名白名单）；新加盘若 API 无 CORS，把根域名加进 `PROVIDER_NET_RELAY_ROOTS` 与 shim 的 `RELAY_ROOTS`。
登录优先账密/OAuth/refresh_token/SMS；`yike` 仅 BDUSS（AList 限制）；`photoAlbum` 瀑布流 UI。图标源：`logo/` → `public/images/drive-icons/`。

Aliases: `@shared/*`, `@main/*`.

## Out of scope (removed from product)

Media library UI, media-server clients, music library product, books/Reedy/AI, clouddrive-cli/MCP, local BT seeding, WebDAV **server**, App Pro/paywall, RSS toolbox.  
Login removed for Quark / Nextcloud（未做完整 checklist 勿加菜单）。`aliopen` / `guangya` / `pan139` / `pan189` / `yike` 已按 AList 重新接入。

Keep: multi-cloud file manager, HTTP Aria download, PikPak cloud offline, video/file preview, slim share, cross-drive migrate (`src/migrate/`, see `docs/MIGRATE_SPEC.md`).

## Testing

Vitest Node env; explicit dirs in `vitest.config.ts`. Prefer normalizing CRLF when asserting multi-line source strings on Windows.

## Provider checklist

When adding a provider, implement in order (do not ship list-only):

1. Account/auth (`auth.ts`, secrets placeholders, userstore/userdal, login UI)  
2. Detection (`tokenfrom`, drive model, `driveProvider` meta + capabilities)  
3. List/detail → shared file model  
4. Download/playback URLs (no wrong-provider APIs)  
5. Search if supported  
6. Thumbnails  
7. File ops (mkdir/rename/move/copy/trash/delete)  
8. Share if supported  
9. Upload  
10. Folder picker modals  
11. Menu capability boundaries  
12. Properties / recycle if any  
13. Tests + `vitest.config.ts`  
14. `npm run build`  

Recommended layout:
- API: `src/<provider>/{auth,dirfilelist,filecmd,search?,share?,upload?,…}.ts`
- Plugin adapter: `src/drive/providers/<provider>.ts` + `providers/index.ts` import（单文件宜 <200 行）
- Shared: `src/drive/ops.ts`、`dirQuery.ts`、`mountedUpload.ts`、`shareHelpers.ts`
- Local tags/favorites: `src/pan/quickFiles.ts`（非盘协议）
- Do **not** add central `if (provider === …)` in `fileapi/*` or pan list loaders

## Formatting

Single quotes, no semicolons, printWidth 260, no trailing commas, LF.

## 协作偏好（产品体验原则）

- **后台无感**：索引、缓存、同步等基础能力在后台静默建立与维护（及时增删、及时同步）；不向用户展示「系统做了什么」的说明性提示，仅真正需要用户决策/纠错时才提示
- **资源克制**：后台任务默认低占用——防抖写回、惰性加载、限量上限，不阻塞交互、不刷磁盘/网络
- **链路闭环**：功能必须打通到可用终点（入口→处理→结果），不留死路；能力缺失时静默降级（如本地索引兜底搜索），不堆解释文案
- **验证方式**：以静态分析 + 单测/typecheck 为准，不代用户启动或操作 App
