<p align="center">
  <img src="screenshot/icon.svg" alt="Mnemo" width="96">
</p>

<p align="center">
  <br> <a href="./README.md">中文</a> | English
</p>

<p align="center">
  <em><strong>Mnemo</strong> — remember every cloud.</em>
</p>

<p align="center">
  Free · open-source · multi-cloud manager + fast transfer + online preview/playback
</p>

<p align="center">
  <img alt="License" src="https://img.shields.io/badge/license-GPL--3.0-blue?style=flat-square" />
  <img alt="Windows" src="https://img.shields.io/badge/-Windows-blue?style=flat-square&logo=windows&logoColor=white" />
  <img alt="macOS" src="https://img.shields.io/badge/-macOS-black?style=flat-square&logo=apple&logoColor=white" />
  <img alt="Linux" src="https://img.shields.io/badge/-Linux-yellow?style=flat-square&logo=linux&logoColor=white" />
</p>

---

## Name

**Mnemo** is from **Mnemosyne (Μνημοσύνη)** — Greek goddess of memory, mother of the Muses. One place to remember, browse, transfer, and open files across cloud drives.

---

## Features

### Supported cloud drives

| Drive | Notes |
|---|---|
| **Aliyun Drive** | Full list / upload-download / share / preview |
| **Baidu Netdisk** | Browse, transfer, file ops |
| **123 Pan** | Browse, upload, share, cloud offline, video |
| **115** | Browse, transfer, share, cloud offline, auth headers for playback |
| **PikPak** | Login, list, share, cloud offline |
| **Quark** | Login, browse, upload/download, rename/move, share, search |
| **China Mobile 139** | Login, browse, upload/download, rename/move |
| **China Telecom 189** | Login, browse, upload/download, rename/move |
| **OneDrive** | OAuth, list, search, share, upload, versions |
| **Dropbox** | OAuth, list, search, share, upload, thumbnails |
| **Box** | OAuth, list, search, share, upload, versions |
| **Guangya** | List, upload, share, instant upload, cloud offline, search |

Menus follow real provider APIs; unsupported actions are hidden or reported clearly.

- Multiple accounts at once
- Unified file model across providers

### File management
- Folder tree + file list
- Sort by name / size / time; folder size display
- Batch rename / move / copy / delete
- Search, properties, large directory listing
- Quick preview (images, video sprites, documents)

### Transfer
- Aria2 multi-connection HTTP(S) download from cloud direct links
- Upload files and folders
- Task center: downloading / completed / uploading / uploaded
- Remote Aria (download to VPS/NAS)
- Speed limits, connection count, resume (as configured)
- Completion notifications; prevent sleep while transferring (engine support)

### Cloud offline
- Where supported (e.g. 115 / 123 / PikPak / Guangya), submit magnet/URL to **provider offline download** so files land **on the cloud**
- Separate from local Aria HTTP downloads

### Preview & playback
- Online video (built-in player + MPV)
- Multi-audio / external subtitles, speed, quality streams when available
- Same-folder playlist, progress memory
- Auth headers via local proxy / player args when needed
- Image, PDF, Office, text/code, EPUB as **file preview**

### Share
- Create share links
- Import others’ links
- My shares + import history (provider-dependent)

### Settings
- UI, accounts, player, download/upload, proxy, remote Aria, security, logs

---

## Supported platforms

| OS | Arch | Artifacts (electron-builder) |
|---|---|---|
| **Windows** | x64 | NSIS installer (`.exe`) |
| **macOS** | Intel x64, Apple Silicon arm64 | `.dmg` / `.zip` (signing & notarization optional) |
| **Linux** | x64, arm64 | `.AppImage` / `.deb` / `.pacman` |

- Desktop app on **Electron** for everyday PC/laptop and common Linux distros.
- Platform-matched **Aria2** (and related engine resources) ship in the package; MPV assets follow platform resource dirs.
- Build: `npm run build:windows` / `build:mac` / `build:linux` / `build:all`.

---

## Main tabs

| Tab | Role |
|---|---|
| **Cloud** | Accounts, tree, file ops |
| **Transfer** | Upload / download jobs |
| **Share** | Shares & link import |
| **Settings** | Preferences |

---

## Develop

```bash
npm install
npm run dev
npm run build
npm run build:electron
npm run test
```

- Node **≥ 22.12**
- **npm** + `package-lock.json` (not pnpm/yarn)
- Secrets: `.env.local` → `npm run secrets:generate`

Stack: **Electron 40 · Vue 3 · Vite · Pinia · TypeScript · npm**. Provider checklist: [AGENTS.md](./AGENTS.md).

---

## Disclaimer

Personal use on your own cloud data. Follow provider ToS and local law. GPL-3.0 — [LICENSE](./LICENSE).

## Contributing

[CONTRIBUTION.md](./CONTRIBUTION.md) · [AGENTS.md](./AGENTS.md).
