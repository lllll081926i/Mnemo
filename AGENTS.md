# AGENTS.md — Mnemo

## Product

**Mnemo** is a free multi-cloud desktop file manager (Electron + Vue 3).  
Mythology: Mnemosyne — memory.

**Active providers** (login + `driveProvider`):  
`pikpak` · `onedrive` · `dropbox` · `gdrive` · `gofile` · `webdav` · `s3`

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

Providers under `src/`: `pikpak/`, `onedrive/`, `dropbox/`, `gdrive/`, `gofile/`.  
Mounted storage: `utils/webdavClient.ts`, `utils/s3Client.ts`.  
`src/aliapi/` may still hold shared file models / legacy helpers — **not** an active login product surface.

Aliases: `@shared/*`, `@main/*`.

## Out of scope (removed from product)

Media library UI, media-server clients, music library product, books/Reedy/AI, clouddrive-cli/MCP, local BT seeding, WebDAV **server**, App Pro/paywall, RSS toolbox.  
Login removed for Aliyun / Quark / 139 / 189 / Guangya / Nextcloud (do not re-add menus without full AGENTS checklist).

Keep: multi-cloud file manager, HTTP Aria download, PikPak cloud offline, video/file preview, slim share.

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

Recommended layout: `src/<provider>/{auth,dirfilelist,filecmd,search?,share?,upload?,…}.ts`.

## Formatting

Single quotes, no semicolons, printWidth 260, no trailing commas, LF.
