# CLAUDE.md

Guidance for Claude Code when working in this repository.

## Product

**Mnemo** — free multi-cloud desktop file manager (Electron + Vue 3).  
Named after Mnemosyne, Greek goddess of memory.

Focus: multi-account cloud drives, file tree, upload/download (Aria2 HTTP), cloud offline where supported, preview/playback (incl. MPV), slim share.  
Removed: media library, media servers, music, books, AI/CLI, local BT seeding, WebDAV server, paid features.

## Quick commands

```bash
npm install          # npm — use npm (not pnpm/yarn)
npm run dev              # hot-reload Electron (Vite)
npm run build        # version bump → vue-tsc → vite build
npm run build:electron
npm run test         # focused Vitest (aria2 + related)
npm run build:mac
npm run build:linux
npm run build:windows
npm run secrets:generate   # from .env.local → src/secrets.generated.ts
```

## Architecture

```
electron/main/     Main process (window, IPC, Aria2, MPV, protocol)
electron/preload/  Preload bridge
src/               Vue 3 renderer
  aliapi/          Aliyun + shared file models / routing hub
  cloud*/pikpak/…  Other providers
  pan/             File manager UI
  down/            Download/upload tasks
  layout/          Shell + preview pages (video, pdf, image…)
  store/           Pinia
  user/            Cloud account login UI
shared/            Shared constants / utils
static/engine/     Platform aria2 / mpv binaries
```

Aliases: `@shared/*` → `shared/*`, `@main/*` → `electron/main/*`

## Key patterns

- **npm** (`package-lock.json`)
- **Node ≥ 22.12**
- Formatting: single quotes, no semicolons, printWidth 260, no trailing commas
- Secrets never committed; use generated `src/secrets.generated.ts`
- Vitest: explicit include paths in `vitest.config.ts`
- New provider: follow **AGENTS.md** 15-step checklist
