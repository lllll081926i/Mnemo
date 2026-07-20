# CLAUDE.md

Guidance for Claude Code when working in this repository.

## Product

**Mnemo** — free multi-cloud desktop file manager (Electron + Vue 3).  
Named after Mnemosyne, Greek goddess of memory.

**Active providers** (login): PikPak (default), OneDrive, Dropbox, Google Drive, GoFile, WebDAV, S3.

Focus: multi-account cloud drives, file tree, upload/download (Aria2 HTTP, lazy engine start), PikPak cloud offline, preview/playback (incl. MPV), slim share.

Removed from product: media library, media servers, music library product, books/AI, local BT, WebDAV server, paid features; **login removed** for Aliyun / Quark / 139 / 189 / Guangya / Nextcloud.

## Quick commands

```bash
npm install          # npm — use npm (not pnpm/yarn)
npm run dev              # hot-reload Electron (Vite)
npm run build        # typecheck → vite build
npm run build:electron
npm run test         # Vitest
npm run build:mac
npm run build:linux
npm run build:windows
npm run secrets:generate   # from .env.local → src/secrets.generated.ts
```

## Architecture

```
electron/main/     Main process (window, IPC, Aria2 lazy, MPV, OAuth, protocol)
electron/preload/  Preload bridge
worker.html        Light upload worker entry
src/               Vue 3 renderer
  pikpak/ onedrive/ dropbox/ gdrive/ gofile/
  pan/             File manager UI
  down/            Download/upload tasks
  layout/          Shell + preview pages
  store/           Pinia
  user/            Cloud account login UI
  utils/           driveProvider, WebDAV/S3 clients, …
  aliapi/          Legacy shared file models (not a login surface)
shared/            Shared constants / utils
static/engine/     Platform aria2 / mpv binaries
```

Aliases: `@shared/*` → `shared/*`, `@main/*` → `electron/main/*`

## Key patterns

- **npm** (`package-lock.json`)
- **Node ≥ 22.12**
- Formatting: single quotes, no semicolons, printWidth 260, no trailing commas
- Secrets never committed; use generated `src/secrets.generated.ts`
- Vitest: explicit include paths in `vitest.config.ts`; normalize CRLF for multi-line source asserts on Windows
- New provider: follow **AGENTS.md** checklist
- Docs: README / AGENTS / DESIGN / CONTRIBUTION / `docs/PROJECT_REVIEW.md`
