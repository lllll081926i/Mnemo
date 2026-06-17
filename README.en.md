<p align="center">
  <img src="screenshot/icon.svg" alt="BoxPlayer">
</p>
<p align="center">
    <br> English | <a href="./README.md">中文</a>
</p>
<p align="center">
    <em>BoxPlayer - unified cloud drive management, smart media library, media servers, and high-speed downloads.</em>
</p>

<p align="center">
  🌐 Website: <a href="https://xbyvideohub.com/" target="_blank"><strong>xbyvideohub.com</strong></a>
</p>

<p align="center">
  <a href="LICENSE" target="_blank">
    <img alt="MIT License" src="https://img.shields.io/github/license/gaozhangmin/aliyunpan?style=flat-square&logoColor=white" />
  </a>

  <img alt="TypeScript" src="https://img.shields.io/badge/-TypeScript-blue?style=flat-square&logo=typescript&logoColor=white" />
  <img alt="VUE" src="https://img.shields.io/badge/Vue.js-35495E?style=flat-square&logo=vuedotjs&logoColor=white" />

  <a href="https://github.com/gaozhangmin/aliyunpan/releases" target="_blank">
    <img alt="macOS" src="https://img.shields.io/badge/-macOS-black?style=flat-square&logo=apple&logoColor=white" />
  </a>
  <a href="https://github.com/gaozhangmin/aliyunpan/releases" target="_blank">
    <img alt="Windows" src="https://img.shields.io/badge/-Windows-blue?style=flat-square&logo=windows&logoColor=white" />
  </a>
  <a href="https://github.com/gaozhangmin/aliyunpan/releases" target="_blank">
    <img alt="Linux" src="https://img.shields.io/badge/-Linux-yellow?style=flat-square&logo=linux&logoColor=white" />
  </a>

  <a href="https://github.com/gaozhangmin/aliyunpan/stargazers" target="_blank">
    <img alt="Star" src="https://img.shields.io/github/stars/gaozhangmin/aliyunpan?style=social" />
  </a>
  <a href="https://github.com/gaozhangmin/aliyunpan/releases/latest" target="_blank">
    <img alt="Downloads" src="https://img.shields.io/github/downloads/gaozhangmin/aliyunpan/total?style=social" />
  </a>
</p>

[![](https://img.shields.io/badge/-Features-blue)](#features-) [![](https://img.shields.io/badge/-%E2%9C%A8%20New%20Features-orange)](#new-features-) [![](https://img.shields.io/badge/-Screenshots-blue)](#screenshots-) [![](https://img.shields.io/badge/-Installation-blue)](#installation-) [![](https://img.shields.io/badge/-CLI-blue)](#clouddrive-cli-) [![](https://img.shields.io/badge/-WeChat-blue)](#wechat-official-account-) [![](https://img.shields.io/badge/-Community-blue)](#community-) [![](https://img.shields.io/badge/-Credits-blue)](#credits-) [![](https://img.shields.io/badge/-Disclaimer-blue)](#disclaimer-)

# Features [![](https://img.shields.io/badge/-Features-blue)](#features-)

## Media Servers
1. **Multiple server types**: connect to popular media servers such as Emby, Jellyfin, and Plex.<br>
2. **Custom server icons**: assign a custom icon to each media server so multiple instances are easy to tell apart.<br>
3. **Aggregated home page**: view Continue Watching, Recently Added, Movies, TV Shows, Anime, and more in one place.<br>
4. **Full library browsing**: browse by media type, including movies, TV series, anime, documentaries, poster wall, and list views.<br>
5. **Media server search**: search inside one server or across multiple servers at once.<br>
6. **Episode details**: show posters, ratings, summaries, episode lists, and playback progress.<br>

## Multiple Cloud Drives
7. **Multi-platform cloud drive support**: supports Aliyun Drive, Baidu Netdisk, 123Pan, 115 Drive, and other mainstream cloud drive services.<br>
8. **WebDAV connections**: connect to Quark Cloud Drive, Tianyi Cloud, and more through WebDAV.<br>
9. **Local folder import**: import local folders and scrape TMDB metadata.<br>
10. **Multi-account management**: sign in to and manage multiple cloud drive accounts at the same time.<br>

## Smart Media Library
11. **TMDB metadata scraping**: scan cloud drives and local files, then fetch movie and TV metadata from TMDB.<br>
12. **Media organization**: classify and organize media files into a complete personal media library.<br>
13. **Unified search**: search across cloud drives and local libraries to find media quickly.<br>

## Playback
14. **Online HD playback**: play original-quality videos in many formats directly from cloud drives.<br>
15. **Multi-audio tracks**: switch between audio tracks inside the player.<br>
16. **External subtitles**: load external subtitle files and switch between subtitle tracks.<br>
17. **Video stream and quality switching**: switch video streams and choose quality according to network conditions.<br>
18. **Playback speed control**: customize playback speed.<br>
19. **Playlist management**: create and manage playlists.<br>
20. **External players**: supports professional players such as MPV and IINA.<br>

## High-Speed Downloads
21. **Aria2c downloads**: built-in high-speed Aria2c engine with multi-threaded downloads.<br>
22. **Remote downloads**: use remote Aria2 to download files directly to a VPS or NAS.<br>

## File Management
23. **Folder tree view**: a dedicated folder tree for fast navigation and operations.<br>
24. **Smart sorting**: show folder sizes and sort folders and files together by name, size, or time.<br>
25. **Batch operations**: batch rename large numbers of files and deeply nested folders.<br>
26. **Quick preview**: copy files quickly, preview video sprite images, and delete files directly.<br>
27. **Large-scale file handling**: manage tens of thousands of folders and files, and list all files in a folder at once.<br>
28. **Bulk transfer**: upload or download massive numbers of files and folders in one operation.<br>

## Cross-Platform
29. **Platform support**: supports Windows 7-11, macOS, and Linux.<br>

<a href="#readme">
    <img src="https://img.shields.io/badge/-Back%20to%20top-orange.svg" alt="#" align="right">
</a>

# New Features [![](https://img.shields.io/badge/-%E2%9C%A8%20New%20Features-orange)](#new-features-)

> This release ships **45+ major upgrades**, covering AI Read-Aloud / AI Reading Assistant / AI Translation / multi-model chat / advanced music playback / Download infrastructure / 3 new cloud drives / AI media organizer agent. Every new capability is grouped by module below.

## Brand-new “Library” — AI-powered personal e-book reader

1. **AI Text-to-Speech (TTS)**: built-in **Azure Neural TTS** + Web Speech API dual engines, with dozens of natural voices (Xiaoxian / Xiaoxiao / Yunxi / Yunyang and more), reading any book aloud from the cursor with **continuous cross-chapter playback**, adjustable speed / voice / pitch — turn any e-book into a professional audiobook instantly. <br>
2. **AI Reading Assistant**: one-click access to 10+ leading models — **OpenAI / DeepSeek / Zhipu GLM / Qwen / Moonshot Kimi / SiliconFlow / Ollama (local) / OpenRouter / Vercel AI Gateway** — to **summarize chapters / answer questions / recommend similar books / hold multi-turn conversations**. Includes a **local chapter-vector RAG index** so “what does chapter 3 actually say?” gets a grounded answer. <br>
3. **AI Word Translation + Whole-Book Translation**: select to translate, with **AI translation (DeepL-grade quality) / Azure / Google Translate**, **bilingual side-by-side reading**, or **whole-book translation mode** — read any foreign-language book without friction. <br>
4. **Multi-format e-book reader**: supports **EPUB / PDF / TXT / MOBI / AZW / AZW3 / FB2 / DOCX / MD / HTML / CBZ / CBR / CB7 / CBT** — every major format. <br>
5. **Three pagination modes**: single / double-page / scroll, freely switchable. The new release fixes the “only partial text shown, large blank areas” rendering bug, restoring native pagination and container scrolling. <br>
6. **Cloud + local dual book sources**: every connected cloud drive auto-detects books; one-click local-folder import; auto-scrape covers / authors / publication dates / summaries. <br>
7. **Professional typography engine**: built-in **Heti / Hanzi Standard Format / Chinese Web Typography Reset / Tufte CSS / Typebase** academic-grade styles — better Chinese-English mixed typesetting than Kindle. <br>
8. **Bookshelf / favorites / tags / trash**: complete book lifecycle management with card / list / cover view modes. <br>
9. **Notes / highlights / bookmarks / selection menu**: customizable highlight colors, attached notes, in-document jumps, keyboard shortcuts. <br>
10. **Bulk annotation export**: one-click export of all highlights and notes to Markdown / JSON / CSV / TXT. <br>
11. **Reading statistics**: visualize daily reading time / page turns / completion rate. <br>
12. **OPDS subscriptions**: subscribe to any OPDS-compliant online library. <br>
13. **PDF full-text search + chapter navigation + dictionary lookup + reference search**. <br>

## Advanced Music Playback (lx-music-desktop port, no copyright risk)

14. **AudioContext audio engine**: 10-band EQ + reverb + panning + pitch shift (without speed change) + real-time spectrum visualization. <br>
15. **Word-by-word karaoke lyrics**: per-character highlight animations powered by Web Animations API, with translation / romaji dual-line support. <br>
16. **Floating desktop lyrics window**: independent transparent always-on-top window, draggable, scrolling with playback. <br>
17. **Multi-source lyrics / cover fallback**: when LRCLIB has no match, automatically fetches lyrics and covers from NetEase / KuGou / QQ Music / Kuwo / Migu open APIs (metadata only, no audio stream download). <br>
18. **Customizable theme system**: 12-color visual editor + 15 built-in preset themes, fully CSS-variable-driven. <br>

## Download Infrastructure

19. **Main-process aria2c hosting**: PID file + session resume + graceful shutdown + auto-reconnect on crash. <br>
20. **EngineClient real-time events**: aria2-lib low-latency event-driven, subscribes to `onDownloadStart/Complete/Error/Stop/BtComplete`, sub-100ms state feedback. <br>
21. **UPnP automatic port mapping**: BT downloads automatically open NAT ports for better seeding connectivity. <br>
22. **BT tracker auto-sync every 12h**: pulls the latest public trackers from [ngosang/trackerslist](https://github.com/ngosang/trackerslist) every 12 hours after launch. <br>
23. **Torrent file selector**: select specific files within a BT task to avoid downloading the whole torrent. <br>
24. **Task detail drawer**: GID / total size / progress / speed / seeders / connections / InfoHash / save path / file list — all visible at a glance. <br>
25. **Drag-and-drop add tasks**: drop URLs / magnet links / .torrent files from browser address bars / Finder / File Explorer to create tasks instantly. <br>
26. **Protocol associations**: `magnet://` / `mo://` automatically captured to open the download dialog (supports macOS `open-url` / `open-file`). <br>
27. **Download progress indicator**: macOS Dock / Windows taskbar shows real-time download progress ring. <br>
28. **Completion system notifications**: each completed file triggers a system notification; click to focus the main window. <br>
29. **Batch pause / resume / delete**: direct RPC, no polling delay. <br>
30. **Per-platform aria2 configurations**: 7 optimized `aria2.conf` profiles across darwin / linux / win32 × x64 / arm64 / armv7l / ia32. <br>
31. **Expanded settings**: upload speed cap, seed ratio, seed time, auto-resume unfinished tasks, browser extension RPC URL display, tracker editor (one URL per line) with instant sync. <br>
32. **Sleep prevention**: blocks system sleep while downloads are in progress. <br>

## New Cloud Drives

| Cloud Drive | App capabilities | clouddrive-cli `--provider` |
|---|---|---|
| **Quark Cloud Drive** | Login / browse / download / upload / rename / move / share / search | `quark` |
| **China Mobile Cloud (139)** | Login / browse / download / upload / rename / move | `cloud139` |
| **China Telecom Cloud (189)** | Login / browse / download / upload / rename / move | `cloud189` |

33. clouddrive-cli adds three new providers — `cloud139` / `cloud189` / `quark` — and extracts shared `ossUpload` / `uploadUtils` helpers for unified resumable uploads, chunked transfer, and progress callbacks. <br>
34. clouddrive-cli adds `commandManifest` and `mcpToolSchema` metadata so MCP clients (Claude Desktop, Cursor, etc.) can auto-discover commands, parameters, and examples. <br>

## AI Media Organizer Agent

35. **AgentMediaOrganizer drawer**: right-click any cloud-drive folder to launch “AI Organize”, letting the AI see the current directory and execute renames / moves / classifications according to your description, with multi-turn conversation. <br>
36. **Powered by Vercel AI SDK**: compatible with all OpenAI-protocol models (GPT / Claude / DeepSeek / Qwen / GLM / Moonshot / Ollama via Gateway, etc.). <br>
37. **Reversible operation log**: every AI action is recorded in `operationHistory`; one-click rollback to the pre-operation state. <br>
38. **Pan-context tool set**: built-in `walkDirectory` / `renamePlan` / `movePlan` / `mediaClassifier` and other safe tools the AI can call. <br>
39. **clouddrive-cli `organize` command**: exposes the same capabilities to AI terminals (Claude Code, Cursor, etc.) so AI agents can organize cloud drives remotely. <br>

## Settings and Infrastructure Refactor

40. **Unified AI / API key configuration page** (`SettingAPI.vue`): centrally manage OpenAI / DeepSeek / Azure TTS / Vercel AI Gateway / translation API keys, shared by the reader and organizer agents. <br>
41. **Advanced download settings panel** (`SettingDownloadAdvanced.vue`): aggregates aria2, seeding, tracker, and protocol-association advanced parameters. <br>
42. **`shared/` cross-cutting layer**: constants, UA, `configKeys`, `tracker`, and `rename` utilities reused by main process / renderer / CLI. <br>
43. **Protocol handler refactor**: unified `electron/main/core/protocol.ts` handles magnet / file / custom protocols, with unit-test coverage. <br>
44. **aria2 engine policy** (`aria2EnginePolicy.ts`): auto-selects the optimal aria2c binary and configuration based on platform and architecture. <br>

<a href="#readme">
    <img src="https://img.shields.io/badge/-Back%20to%20top-orange.svg" alt="#" align="right">
</a>

# Screenshots [![](https://img.shields.io/badge/-Screenshots-blue)](#screenshots-)

## Media Server Management
<img src="screenshot/截屏2026-04-24 23.03.06.png" width="380"> <img src="screenshot/截屏2026-04-24 23.04.22.png" width="380">

*Media server list and server card view, with custom icon support.*

## Media Server Home
<img src="screenshot/截屏2026-04-24 23.06.07.png" width="380"> <img src="screenshot/截屏2026-04-24 23.18.46.png" width="380">

*Continue Watching and categorized media library gallery view.*

## Episodes and Media Details
<img src="screenshot/截屏2026-04-24 23.06.22.png" width="380"> <img src="screenshot/截屏2026-04-24 23.28.52.png" width="380">

*Episode detail page and episode list.*

## Anime and Category Browsing
<img src="screenshot/截屏2026-04-24 23.19.14.png" width="380"> <img src="screenshot/截屏2026-04-24 23.19.41.png" width="380">

*Anime library and media server search.*

## Media Library and Aggregated Search
<img src="screenshot/截屏2026-04-24 23.19.53.png" width="380"> <img src="screenshot/截屏2026-04-24 23.20.36.png" width="380">

*Aggregated search and local media library list view.*

## File Management
<img src="screenshot/截屏2026-04-24 23.20.18.png" width="380"> <img src="images/folder-tree.png" width="380">

*Main file management view and folder tree.*

## Multiple Accounts
<img src="images/multi-account.png" width="380"> <img src="images/login-qr.png" width="380">

*Multi-cloud account management and QR code login.*

<a href="#readme">
<img src="https://img.shields.io/badge/-Back%20to%20top-orange.svg" alt="#" align="right">
</a>

# Installation [![](https://img.shields.io/badge/-Installation-blue)](#installation-)

## Package Guide

The `release` folder contains installers for each platform and CPU architecture. Use the keywords in each filename to pick the right package.

### Windows

| File name | Platform | Notes |
|--------|----------|------|
| `...-win.exe` | Windows universal | Detects system architecture automatically. Recommended for most users. |
| `...-win-x64.exe` | Windows x64 | For Intel / AMD 64-bit processors. |
| `...-win-ia32.exe` | Windows 32-bit | For 32-bit systems or older processors. |
| `...-win-arm64.exe` | Windows ARM64 | For ARM64 processors, such as Qualcomm Snapdragon X Elite. |
| `...-win-x64.zip` | Windows x64 portable | Extract and run, no installation required. |
| `...-win-ia32.zip` | Windows 32-bit portable | Extract and run, no installation required. |
| `...-win-arm64.zip` | Windows ARM64 portable | Extract and run, no installation required. |

**How to install:** double-click the `.exe` installer and follow the prompts. For the portable version, extract the `.zip` package and run `xbyboxplayer.exe`.

### macOS

| File name | Platform | Notes |
|--------|----------|------|
| `...-mac-x64.dmg` | macOS Intel | For Macs with Intel chips. |
| `...-mac-arm64.dmg` | macOS Apple Silicon | For Macs with M1 / M2 / M3 / M4 chips. |

**How to install:** double-click the `.dmg` file and drag the app to the `Applications` folder. If Apple Silicon shows a damaged-file warning after installation, run:

```sh
sudo xattr -d com.apple.quarantine /Applications/xbyboxplayer.app
```

### Linux

| File name | Platform | Notes |
|--------|----------|------|
| `...-linux-amd64.deb` | Debian / Ubuntu x64 | For Debian, Ubuntu, and similar distributions on 64-bit Intel/AMD. |
| `...-linux-arm64.deb` | Debian / Ubuntu ARM64 | For ARM64 Debian / Ubuntu. |
| `...-linux-armv7l.deb` | Debian / Ubuntu ARMv7 | For 32-bit ARM Debian / Ubuntu. |
| `...-linux-x86_64.AppImage` | Generic Linux x64 | Portable package for most 64-bit Linux distributions. |
| `...-linux-arm64.AppImage` | Generic Linux ARM64 | Portable package for ARM64 Linux. |
| `...-linux-armv7l.AppImage` | Generic Linux ARMv7 | Portable package for 32-bit ARM Linux. |
| `...-linux-x64.pacman` | Arch Linux / Manjaro x64 | For Arch Linux and derivatives on 64-bit systems. |
| `...-linux-aarch64.pacman` | Arch Linux ARM64 | For Arch Linux ARM64. |
| `...-linux-armv7l.pacman` | Arch Linux ARMv7 | For Arch Linux ARMv7. |
| `...-linux-x64.zip` / `arm64.zip` / `armv7l.zip` | Linux portable | Extract and run the executable inside. |

**How to install:**
- `.deb`: `sudo dpkg -i <file>.deb`
- `.AppImage`: `chmod +x <file>.AppImage && ./<file>.AppImage`
- `.pacman`: `sudo pacman -U <file>.pacman`
- `.zip`: extract and run the executable inside the folder

---

# clouddrive-cli [![](https://img.shields.io/badge/-CLI-blue)](#clouddrive-cli-)

`clouddrive-cli` is the terminal and AI Agent automation interface for BoxPlayer. It supports Aliyun Drive, OneDrive, Dropbox, Box, Baidu Netdisk, 115 Drive, and PikPak.

Key capabilities:

- List and recursively walk cloud drive files
- Generate media rename plans compatible with Jellyfin / Emby / Plex
- Dry-run validation before executing trackable, reversible batch renames
- Upload plans with dry-run and cloud drive organization plans
- Expose the same capabilities to MCP-compatible AI clients (Claude Desktop, Cursor, etc.) via `clouddrive-mcp`

**Install (with App):** open BoxPlayer → **Account Settings → Install CLI**

**Standalone install:**

```bash
npm install -g clouddrive-cli
```

Full documentation: [clouddrive-cli/README.en.md](./clouddrive-cli/README.en.md) | [中文](./clouddrive-cli/README.md)

<a href="#readme">
    <img src="https://img.shields.io/badge/-Back%20to%20top-orange.svg" alt="#" align="right">
</a>

---

# WeChat Official Account [![](https://img.shields.io/badge/-WeChat-blue)](#wechat-official-account-)

<p align="center">
  <img src="screenshot/qrcode_wechat.jpg" width="380">
</p>

<a href="#readme">
    <img src="https://img.shields.io/badge/-Back%20to%20top-orange.svg" alt="#" align="right">
</a>

# Community [![](https://img.shields.io/badge/-Community-blue)](#community-)

#### Telegram
[![Telegram-group](https://img.shields.io/badge/Telegram-Group-blue)](https://t.me/+wjdFeQ7ZNNE1NmM1)

# Credits [![](https://img.shields.io/badge/-Credits-blue)](#credits-)

This project continues development based on https://github.com/liupan1890/aliyunpan.

Thanks to [liupan1890](https://github.com/liupan1890).

<a href="#readme">
<img src="https://img.shields.io/badge/-Back%20to%20top-orange.svg" alt="#" align="right">
</a>

# Disclaimer [![](https://img.shields.io/badge/-Disclaimer-blue)](#disclaimer-)

1. This is a free and open-source project intended for sharing cloud-drive files, convenient downloads, and Electron learning. Please comply with all applicable laws and regulations and do not abuse it.

2. This program works through official SDKs and APIs and does not damage or bypass official interfaces.

3. This program only performs 302 redirects / traffic forwarding. It does not intercept, store, or tamper with user data.

4. Before using this program, you should understand and accept the related risks, including but not limited to account bans and download speed limits. These risks are unrelated to this program.

5. If there is any infringement, please contact me by email and I will handle it promptly.

<a href="#readme">
<img src="https://img.shields.io/badge/-Back%20to%20top-orange.svg" alt="#" align="right">
</a>
