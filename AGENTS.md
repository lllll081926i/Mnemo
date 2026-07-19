# AGENTS.md — Mnemo

## Product

**Mnemo** is a free multi-cloud desktop file manager (Electron + Vue 3).  
Mythology: Mnemosyne — memory. Product focus: remember and manage files across cloud drives.

## Quick reference

```bash
npm install          # npm, use npm (not pnpm/yarn)
npm run dev
npm run build        # version bump → vue-tsc → vite build
npm run test         # focused Vitest subset
npm run secrets:generate
```

## Package manager / Node

- Lockfile: `package-lock.json` only  
- `engines.node >= 22.12.0`

## Build order

`npm run build` = **version.mjs bump → typecheck → vite bundle** (clears `dist/`; build also clears `release/`).  
`npm run build:electron` runs build then electron-builder. Product name: **Mnemo**, appId `com.mnemo.app`.

## Secrets

Real keys live in `.env.local` / GitHub Secrets → `scripts/generate-secrets.mjs` → `src/secrets.generated.ts` (gitignored).

## Architecture

| Directory | Purpose |
|---|---|
| `electron/main/` | Main process |
| `electron/preload/` | Preload |
| `src/` | Vue renderer + providers |
| `shared/` | Shared code |
| `scripts/` | Build / secrets |
| `static/engine/` | aria2 / mpv binaries |

Providers under `src/`: `aliapi/`, `cloudbaidu/`, `cloud123/`, `cloud115/`, `pikpak/`, `onedrive/`, `box/`, `dropbox/`, `quark/`, `cloud139/`, `cloud189/`, `guangya/`.

Aliases: `@shared/*`, `@main/*`.

## Out of scope (removed)

Media library UI, media-server clients, music, books/Reedy, clouddrive-cli/MCP, local BT seeding/UPnP tracker sync, WebDAV **server**, App Pro/paywall, rss toolbox.

Keep: cloud file manager, HTTP Aria download, provider cloud offline, video/file preview, slim share.

## Testing

Vitest Node env; explicit dirs in `vitest.config.ts`. Default `npm test` is a small aria/utils subset.

## Provider checklist

When adding a provider, implement in order (do not ship list-only):

1. Account/auth (`auth.ts`, secrets placeholders, userstore/userdal, login UI)  
2. Detection (`isXxxUser`, drive model)  
3. List/detail (`dirfilelist.ts` → `IAliGetFileModel`)  
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
