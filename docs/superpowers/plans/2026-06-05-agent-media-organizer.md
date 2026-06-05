# Agent Media Organizer Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the BoxPlayer Agent Media Organizer MVP: a cloud-drive directory `AI Organize` entry that opens a right-side Agent drawer, scans the current directory, diagnoses media organization issues, generates a dry-run plan, requires explicit confirmation for execution, records operations, and supports undo.

**Architecture:** Implement a Vercel AI SDK Agent runtime first, backed by schema-validated tools for scan, diagnosis, planning, and dry-run. The MVP uses `ToolLoopAgent` / AI SDK tool calling as the orchestration boundary; deterministic media helpers are tool implementations, not a replacement for the Agent framework. Execution and undo remain app-side confirmed commands outside the autonomous tool loop.

**Tech Stack:** Vue 3, Pinia, TypeScript, Vitest, Arco Design Vue, Vercel AI SDK Core (`ai`, `zod`), AI Gateway model strings / `gateway()`, existing `AliFileCmd` provider operations, existing `PanTopbtn.vue` and `FileRightMenu.vue` cloud-drive surfaces.

---

## Scope Notes

- Do not add Bilibili following, TMDB discovery, personalized recommendations, or internet resource search in this plan.
- Do not add delete support.
- Do not allow execution without a generated dry-run and an explicit UI confirmation.
- Prefer app-local TypeScript utilities for this MVP. Do not shell out to `clouddrive-cli` from the renderer.
- Base Agent orchestration on Vercel AI SDK. Do not implement a handwritten pseudo-agent as the primary runtime.
- Keep all new Agent code under `src/agent/mediaOrganizer/` except the Pinia export, drawer component, and entry-point wiring.

## File Structure

- Create `src/agent/mediaOrganizer/types.ts`: shared task, scan, diagnosis, plan, dry-run, operation history, and permission-mode types.
- Create `src/agent/mediaOrganizer/mediaClassifier.ts`: deterministic filename/category heuristics for videos, subtitles, movies, episodes, duplicates, and unknown files.
- Create `src/agent/mediaOrganizer/organizePlanner.ts`: rule presets, plan generation, validation, dry-run generation, and stale/conflict checks.
- Create `src/agent/mediaOrganizer/operationHistory.ts`: Dexie `iobject`-backed history helpers using existing `DB`.
- Create `src/agent/mediaOrganizer/executor.ts`: safe adapter over `AliFileCmd.ApiCreatNewForder`, `ApiRenameBatch`, and `ApiMoveBatch`.
- Create `src/agent/mediaOrganizer/aiTools.ts`: Vercel AI SDK `tool()` definitions with Zod schemas for read/analysis/planning tools.
- Create `src/agent/mediaOrganizer/runtime.ts`: centralized Vercel AI SDK `ToolLoopAgent` facade used by the store.
- Create `src/agent/mediaOrganizer/__tests__/mediaClassifier.test.ts`.
- Create `src/agent/mediaOrganizer/__tests__/organizePlanner.test.ts`.
- Create `src/agent/mediaOrganizer/__tests__/operationHistory.test.ts`.
- Create `src/agent/mediaOrganizer/__tests__/runtime.test.ts`.
- Create `src/store/agentMediaOrganizer.ts`: Pinia state machine for the right drawer.
- Modify `src/store/index.ts`: export the new store.
- Create `src/components/AgentMediaOrganizerDrawer.vue`: right-side drawer with task header, status flow, cards, rule controls, dry-run preview, confirmation button, undo action, and contextual chatbox.
- Modify `src/pan/menus/PanTopbtn.vue`: add `AI 整理` toolbar button for normal cloud-drive directories.
- Modify `src/pan/menus/FileRightMenu.vue`: add `AI 诊断此目录` folder context-menu entry.
- Modify the parent page that renders pan menus, if needed after exploration, to mount `AgentMediaOrganizerDrawer.vue` once near the pan page root. Likely candidates are `src/pan/index.vue` or `src/layout/PageMain.vue`; choose the smallest surface that can display the drawer while the file list remains visible.

---

### Task 0: Add Vercel AI SDK Dependencies

**Files:**
- Modify: `package.json`
- Modify: `pnpm-lock.yaml`

- [ ] **Step 1: Install AI SDK packages**

Run: `pnpm add ai zod`

Expected: dependencies are added to `package.json` and `pnpm-lock.yaml`.

- [ ] **Step 2: Verify dependency entries**

Run: `rg -n '"ai"|"zod"' package.json pnpm-lock.yaml 2>&1 | head -c 4000`

Expected: output includes `ai` and `zod`.

- [ ] **Step 3: Commit**

```bash
git add package.json pnpm-lock.yaml
git commit -m "chore: add vercel ai sdk dependencies"
```

---

### Task 1: Define Agent Media Organizer Types

**Files:**
- Create: `src/agent/mediaOrganizer/types.ts`
- Test: none for this task; later tests compile against these types.

- [ ] **Step 1: Create the directory**

Run: `mkdir -p src/agent/mediaOrganizer/__tests__`

Expected: command succeeds.

- [ ] **Step 2: Create shared types**

Create `src/agent/mediaOrganizer/types.ts`:

```ts
import type { IAliGetFileModel } from '../../aliapi/alimodels'

export type AgentPermissionMode = 'read_only' | 'plan_mode' | 'execution_confirmation'

export type AgentTaskStatus =
  | 'waiting'
  | 'scanning'
  | 'diagnosis_complete'
  | 'waiting_for_rules'
  | 'generating_plan'
  | 'waiting_for_dry_run'
  | 'waiting_for_confirmation'
  | 'executing'
  | 'complete'
  | 'undo_available'
  | 'failed'

export type MediaOrganizerRuleStyle = 'jellyfin' | 'emby' | 'plex'

export interface MediaOrganizerRules {
  style: MediaOrganizerRuleStyle
  renameOnly: boolean
  renameAndMove: boolean
  keepChineseTitle: boolean
  ignoreSubtitles: boolean
  onlySelectedFiles: boolean
}

export interface MediaOrganizerContext {
  userId: string
  driveId: string
  dirId: string
  dirName: string
  path: string
  selectedFiles: IAliGetFileModel[]
  items: IAliGetFileModel[]
}

export type ClassifiedMediaKind = 'movie' | 'episode' | 'subtitle' | 'folder' | 'unknown'

export interface ClassifiedMediaItem {
  file_id: string
  drive_id: string
  parent_file_id: string
  name: string
  size: number
  isDir: boolean
  ext: string
  kind: ClassifiedMediaKind
  title: string
  year?: string
  season?: number
  episode?: number
  language?: string
  duplicateKey?: string
  source: IAliGetFileModel
}

export interface MediaOrganizerScan {
  version: 1
  provider: string
  account_id: string
  root_file_id: string
  root_name: string
  created_at: string
  items: ClassifiedMediaItem[]
  stats: {
    totalItems: number
    fileCount: number
    folderCount: number
    videoCount: number
    subtitleCount: number
    suspectedMovies: number
    suspectedEpisodes: number
    unknownCount: number
  }
}

export type DiagnosisIssueKind =
  | 'non_standard_name'
  | 'missing_episode'
  | 'duplicate_resource'
  | 'unmatched_subtitle'
  | 'ambiguous_candidate'
  | 'unsupported_provider'
  | 'large_scope'

export interface DiagnosisIssue {
  id: string
  kind: DiagnosisIssueKind
  title: string
  detail: string
  fileIds: string[]
  severity: 'info' | 'warning' | 'danger'
}

export interface MediaOrganizerDiagnosis {
  issues: DiagnosisIssue[]
}

export type MediaOrganizerActionType = 'mkdir' | 'rename' | 'move'

export interface MediaOrganizerAction {
  id: string
  type: MediaOrganizerActionType
  file_id?: string
  drive_id: string
  parent_file_id: string
  name?: string
  old_name?: string
  new_name?: string
  to_parent_file_id?: string
  to_parent_ref?: string
  to_path?: string
  reason: string
}

export interface MediaOrganizerPlan {
  version: 1
  operation: 'media_organize'
  provider: string
  account_id: string
  root_file_id: string
  created_at: string
  rules: MediaOrganizerRules
  actions: MediaOrganizerAction[]
  summary: {
    mkdirs: number
    renames: number
    moves: number
    conflicts: number
    risk: 'low' | 'medium' | 'high'
    undoable: boolean
  }
}

export interface MediaOrganizerDryRunChange {
  actionId: string
  type: MediaOrganizerActionType
  file_id?: string
  beforePath: string
  afterPath: string
  conflict: boolean
  conflictReason: string
}

export interface MediaOrganizerDryRun {
  ok: boolean
  planId: string
  generated_at: string
  changes: MediaOrganizerDryRunChange[]
  errors: Array<{ code: string; message: string; actionId?: string }>
}

export interface MediaOrganizerOperationItem {
  actionId: string
  type: MediaOrganizerActionType
  file_id?: string
  drive_id: string
  parent_file_id: string
  before_name?: string
  after_name?: string
  from_parent_file_id?: string
  to_parent_file_id?: string
  status: 'success' | 'failed'
  error?: string
}

export interface MediaOrganizerOperation {
  id: string
  type: 'media_organize'
  provider: string
  account_id: string
  root_file_id: string
  started_at: string
  finished_at: string
  items: MediaOrganizerOperationItem[]
}

export const defaultMediaOrganizerRules: MediaOrganizerRules = {
  style: 'jellyfin',
  renameOnly: false,
  renameAndMove: true,
  keepChineseTitle: true,
  ignoreSubtitles: false,
  onlySelectedFiles: false,
}
```

- [ ] **Step 3: Run type check for syntax**

Run: `pnpm exec vue-tsc --noEmit --pretty false`

Expected: it may fail on existing repo-wide declaration debt; it must not include syntax errors from `src/agent/mediaOrganizer/types.ts`.

- [ ] **Step 4: Commit**

```bash
git add src/agent/mediaOrganizer/types.ts
git commit -m "feat: define media organizer agent types"
```

---

### Task 2: Implement Media Classification

**Files:**
- Create: `src/agent/mediaOrganizer/mediaClassifier.ts`
- Test: `src/agent/mediaOrganizer/__tests__/mediaClassifier.test.ts`

- [ ] **Step 1: Write failing classifier tests**

Create `src/agent/mediaOrganizer/__tests__/mediaClassifier.test.ts`:

```ts
import { describe, expect, it } from 'vitest'
import { classifyDirectoryItems, diagnoseScan } from '../mediaClassifier'
import type { IAliGetFileModel } from '../../../aliapi/alimodels'

const makeFile = (overrides: Partial<IAliGetFileModel>): IAliGetFileModel => ({
  drive_id: 'drive1',
  file_id: 'file1',
  parent_file_id: 'root',
  name: 'Untitled.mkv',
  ext: '',
  category: 'video',
  size: 100,
  time: 0,
  thumbnail: '',
  description: '',
  starred: false,
  isDir: false,
  ...overrides,
} as IAliGetFileModel)

describe('classifyDirectoryItems', () => {
  it('classifies movies, episodes, subtitles, folders, and unknown files', () => {
    const scan = classifyDirectoryItems({
      provider: 'aliyun',
      accountId: 'user1',
      rootFileId: 'root',
      rootName: 'Media',
      items: [
        makeFile({ file_id: 'm1', name: 'Inception.2010.1080p.mkv' }),
        makeFile({ file_id: 'e1', name: 'Breaking.Bad.S02E03.mp4' }),
        makeFile({ file_id: 's1', name: 'Breaking.Bad.S02E03.zh.srt', category: 'doc' }),
        makeFile({ file_id: 'f1', name: 'Season 01', isDir: true, category: 'folder' }),
        makeFile({ file_id: 'u1', name: 'notes.txt', category: 'doc' }),
      ],
    })

    expect(scan.stats).toMatchObject({
      totalItems: 5,
      folderCount: 1,
      videoCount: 2,
      subtitleCount: 1,
      suspectedMovies: 1,
      suspectedEpisodes: 1,
      unknownCount: 1,
    })
    expect(scan.items.find((item) => item.file_id === 'm1')).toMatchObject({ kind: 'movie', title: 'Inception', year: '2010' })
    expect(scan.items.find((item) => item.file_id === 'e1')).toMatchObject({ kind: 'episode', title: 'Breaking Bad', season: 2, episode: 3 })
    expect(scan.items.find((item) => item.file_id === 's1')).toMatchObject({ kind: 'subtitle', language: 'zh' })
  })
})

describe('diagnoseScan', () => {
  it('reports duplicates and non-standard media names', () => {
    const scan = classifyDirectoryItems({
      provider: 'aliyun',
      accountId: 'user1',
      rootFileId: 'root',
      rootName: 'Media',
      items: [
        makeFile({ file_id: 'a', name: 'Movie.Name.2020.1080p.mkv', size: 100 }),
        makeFile({ file_id: 'b', name: 'Movie Name (2020).mp4', size: 100 }),
      ],
    })

    const diagnosis = diagnoseScan(scan)

    expect(diagnosis.issues.some((issue) => issue.kind === 'duplicate_resource')).toBe(true)
    expect(diagnosis.issues.some((issue) => issue.kind === 'non_standard_name')).toBe(true)
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `pnpm exec vitest run src/agent/mediaOrganizer/__tests__/mediaClassifier.test.ts`

Expected: FAIL because `mediaClassifier.ts` does not exist.

- [ ] **Step 3: Implement classifier**

Create `src/agent/mediaOrganizer/mediaClassifier.ts`:

```ts
import type { IAliGetFileModel } from '../../aliapi/alimodels'
import type { ClassifiedMediaItem, MediaOrganizerDiagnosis, MediaOrganizerScan } from './types'

const VIDEO_EXTENSIONS = new Set(['.mkv', '.mp4', '.avi', '.mov', '.wmv', '.flv', '.m4v', '.ts'])
const SUBTITLE_EXTENSIONS = new Set(['.srt', '.ass', '.ssa', '.vtt'])

const extensionOf = (name: string) => {
  const dot = name.lastIndexOf('.')
  return dot >= 0 ? name.slice(dot).toLowerCase() : ''
}

const cleanTitle = (value: string) => value
  .replace(/\.[a-z0-9]{2,5}$/i, '')
  .replace(/\b(720p|1080p|2160p|4k|hdr|bluray|web-dl|webrip|x264|x265|h264|h265)\b/ig, ' ')
  .replace(/[._-]+/g, ' ')
  .replace(/\s+/g, ' ')
  .trim()

const titleKey = (value: string) => cleanTitle(value).replace(/\b(19|20)\d{2}\b/g, '').toLowerCase().trim()

const parseEpisode = (name: string) => {
  const match = /(.+?)[.\s_-]*s(\d{1,2})e(\d{1,3})/i.exec(name)
  if (!match) return null
  return {
    title: cleanTitle(match[1]),
    season: Number(match[2]),
    episode: Number(match[3]),
  }
}

const parseMovie = (name: string) => {
  const year = /\b((?:19|20)\d{2})\b/.exec(name)?.[1]
  return {
    title: cleanTitle(name).replace(/\b(19|20)\d{2}\b/g, '').trim(),
    year,
  }
}

const parseSubtitleLanguage = (name: string) => {
  const lowered = name.toLowerCase()
  if (/\b(zh|chs|cht|cn|sc|tc)\b/.test(lowered)) return 'zh'
  if (/\b(en|eng)\b/.test(lowered)) return 'en'
  return ''
}

export function classifyDirectoryItems(input: {
  provider: string
  accountId: string
  rootFileId: string
  rootName: string
  items: IAliGetFileModel[]
}): MediaOrganizerScan {
  const classified: ClassifiedMediaItem[] = input.items.map((source) => {
    const name = source.name || ''
    const ext = extensionOf(name)
    const base = {
      file_id: source.file_id,
      drive_id: source.drive_id,
      parent_file_id: source.parent_file_id,
      name,
      size: Number(source.size) || 0,
      isDir: !!source.isDir,
      ext,
      title: cleanTitle(name),
      source,
    }
    if (source.isDir) return { ...base, kind: 'folder' as const, duplicateKey: '' }
    if (SUBTITLE_EXTENSIONS.has(ext)) {
      return { ...base, kind: 'subtitle' as const, language: parseSubtitleLanguage(name), duplicateKey: titleKey(name) }
    }
    if (VIDEO_EXTENSIONS.has(ext) || source.category === 'video') {
      const episode = parseEpisode(name)
      if (episode) return { ...base, kind: 'episode' as const, ...episode, duplicateKey: `${episode.title.toLowerCase()}-s${episode.season}e${episode.episode}` }
      const movie = parseMovie(name)
      return { ...base, kind: 'movie' as const, ...movie, duplicateKey: `${movie.title.toLowerCase()}-${movie.year || base.size}` }
    }
    return { ...base, kind: 'unknown' as const, duplicateKey: '' }
  })

  const fileCount = classified.filter((item) => !item.isDir).length
  const folderCount = classified.filter((item) => item.kind === 'folder').length
  const videoCount = classified.filter((item) => item.kind === 'movie' || item.kind === 'episode').length
  const subtitleCount = classified.filter((item) => item.kind === 'subtitle').length

  return {
    version: 1,
    provider: input.provider,
    account_id: input.accountId,
    root_file_id: input.rootFileId,
    root_name: input.rootName,
    created_at: new Date().toISOString(),
    items: classified,
    stats: {
      totalItems: classified.length,
      fileCount,
      folderCount,
      videoCount,
      subtitleCount,
      suspectedMovies: classified.filter((item) => item.kind === 'movie').length,
      suspectedEpisodes: classified.filter((item) => item.kind === 'episode').length,
      unknownCount: classified.filter((item) => item.kind === 'unknown').length,
    },
  }
}

export function diagnoseScan(scan: MediaOrganizerScan): MediaOrganizerDiagnosis {
  const issues: MediaOrganizerDiagnosis['issues'] = []
  const duplicateGroups = new Map<string, ClassifiedMediaItem[]>()

  for (const item of scan.items) {
    if ((item.kind === 'movie' || item.kind === 'episode') && /[._]/.test(item.name)) {
      issues.push({
        id: `non-standard-${item.file_id}`,
        kind: 'non_standard_name',
        title: '命名可能不规范',
        detail: `${item.name} 可能不利于媒体服务器识别。`,
        fileIds: [item.file_id],
        severity: 'warning',
      })
    }
    if (item.duplicateKey) {
      const group = duplicateGroups.get(item.duplicateKey) || []
      group.push(item)
      duplicateGroups.set(item.duplicateKey, group)
    }
  }

  for (const [key, group] of duplicateGroups) {
    if (group.length < 2) continue
    issues.push({
      id: `duplicate-${key}`,
      kind: 'duplicate_resource',
      title: '可能存在重复资源',
      detail: group.map((item) => item.name).join(' / '),
      fileIds: group.map((item) => item.file_id),
      severity: 'warning',
    })
  }

  return { issues }
}
```

- [ ] **Step 4: Run classifier test**

Run: `pnpm exec vitest run src/agent/mediaOrganizer/__tests__/mediaClassifier.test.ts`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add src/agent/mediaOrganizer/mediaClassifier.ts src/agent/mediaOrganizer/__tests__/mediaClassifier.test.ts
git commit -m "feat: classify media organizer directory items"
```

---

### Task 3: Implement Planning And Dry-run

**Files:**
- Create: `src/agent/mediaOrganizer/organizePlanner.ts`
- Test: `src/agent/mediaOrganizer/__tests__/organizePlanner.test.ts`

- [ ] **Step 1: Write failing planner tests**

Create `src/agent/mediaOrganizer/__tests__/organizePlanner.test.ts`:

```ts
import { describe, expect, it } from 'vitest'
import { classifyDirectoryItems } from '../mediaClassifier'
import { buildMediaOrganizerPlan, createDefaultRules, dryRunMediaOrganizerPlan, validateMediaOrganizerPlan } from '../organizePlanner'
import type { IAliGetFileModel } from '../../../aliapi/alimodels'

const makeFile = (overrides: Partial<IAliGetFileModel>): IAliGetFileModel => ({
  drive_id: 'drive1',
  file_id: 'file1',
  parent_file_id: 'root',
  name: 'Untitled.mkv',
  ext: '',
  category: 'video',
  size: 100,
  time: 0,
  thumbnail: '',
  description: '',
  starred: false,
  isDir: false,
  ...overrides,
} as IAliGetFileModel)

describe('buildMediaOrganizerPlan', () => {
  it('creates mkdir, rename, and move actions for media files', () => {
    const scan = classifyDirectoryItems({
      provider: 'aliyun',
      accountId: 'user1',
      rootFileId: 'root',
      rootName: 'Media',
      items: [makeFile({ file_id: 'm1', name: 'Inception.2010.1080p.mkv' })],
    })

    const plan = buildMediaOrganizerPlan(scan, createDefaultRules())

    expect(plan.summary.mkdirs).toBeGreaterThan(0)
    expect(plan.summary.renames).toBe(1)
    expect(plan.summary.moves).toBe(1)
    expect(plan.actions.some((action) => action.type === 'rename' && action.new_name === 'Inception (2010).mkv')).toBe(true)
  })

  it('honors rename-only and ignore-subtitles rules', () => {
    const scan = classifyDirectoryItems({
      provider: 'aliyun',
      accountId: 'user1',
      rootFileId: 'root',
      rootName: 'Media',
      items: [
        makeFile({ file_id: 'm1', name: 'Inception.2010.mkv' }),
        makeFile({ file_id: 's1', name: 'Inception.2010.zh.srt', category: 'doc' }),
      ],
    })

    const plan = buildMediaOrganizerPlan(scan, {
      ...createDefaultRules(),
      renameOnly: true,
      renameAndMove: false,
      ignoreSubtitles: true,
    })

    expect(plan.actions.every((action) => action.type !== 'move')).toBe(true)
    expect(plan.actions.some((action) => action.file_id === 's1')).toBe(false)
  })
})

describe('validateMediaOrganizerPlan and dryRunMediaOrganizerPlan', () => {
  it('rejects missing dry-run targets and reports duplicate target names', () => {
    const scan = classifyDirectoryItems({
      provider: 'aliyun',
      accountId: 'user1',
      rootFileId: 'root',
      rootName: 'Media',
      items: [
        makeFile({ file_id: 'a', name: 'Dune.2021.mkv' }),
        makeFile({ file_id: 'b', name: 'Dune 2021.mp4' }),
      ],
    })
    const plan = buildMediaOrganizerPlan(scan, createDefaultRules())

    expect(validateMediaOrganizerPlan(plan).ok).toBe(true)
    const dryRun = dryRunMediaOrganizerPlan(plan, scan.items)

    expect(dryRun.ok).toBe(false)
    expect(dryRun.errors.some((error) => error.code === 'duplicate_target_name')).toBe(true)
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `pnpm exec vitest run src/agent/mediaOrganizer/__tests__/organizePlanner.test.ts`

Expected: FAIL because `organizePlanner.ts` does not exist.

- [ ] **Step 3: Implement planner**

Create `src/agent/mediaOrganizer/organizePlanner.ts`:

```ts
import type { ClassifiedMediaItem, MediaOrganizerAction, MediaOrganizerDryRun, MediaOrganizerPlan, MediaOrganizerRules, MediaOrganizerScan } from './types'
import { defaultMediaOrganizerRules } from './types'

const normalizeName = (value: string) => value.trim().toLowerCase()
const extOf = (name: string) => {
  const dot = name.lastIndexOf('.')
  return dot >= 0 ? name.slice(dot) : ''
}

export const createDefaultRules = (): MediaOrganizerRules => ({ ...defaultMediaOrganizerRules })

const movieName = (item: ClassifiedMediaItem) => {
  const ext = extOf(item.name)
  return item.year ? `${item.title} (${item.year})${ext}` : `${item.title}${ext}`
}

const episodeName = (item: ClassifiedMediaItem) => {
  const ext = extOf(item.name)
  const season = String(item.season || 1).padStart(2, '0')
  const episode = String(item.episode || 1).padStart(2, '0')
  return `${item.title} - S${season}E${episode}${ext}`
}

const targetFolderFor = (item: ClassifiedMediaItem) => item.kind === 'episode' ? 'TV Shows' : 'Movies'

const actionId = (type: string, fileId: string, suffix = '') => `${type}:${fileId}:${suffix}`

export function buildMediaOrganizerPlan(scan: MediaOrganizerScan, rules: MediaOrganizerRules): MediaOrganizerPlan {
  const actions: MediaOrganizerAction[] = []
  const folderNames = new Set(scan.items.filter((item) => item.kind === 'folder').map((item) => normalizeName(item.name)))

  if (rules.renameAndMove && !rules.renameOnly) {
    for (const folderName of ['Movies', 'TV Shows']) {
      if (!folderNames.has(normalizeName(folderName))) {
        actions.push({
          id: actionId('mkdir', scan.root_file_id, folderName),
          type: 'mkdir',
          drive_id: scan.items[0]?.drive_id || '',
          parent_file_id: scan.root_file_id,
          name: folderName,
          to_path: folderName,
          reason: `${folderName} folder is required for ${rules.style}.`,
        })
      }
    }
  }

  for (const item of scan.items) {
    if (item.kind === 'subtitle' && rules.ignoreSubtitles) continue
    if (item.kind !== 'movie' && item.kind !== 'episode' && item.kind !== 'subtitle') continue

    const nextName = item.kind === 'episode' ? episodeName(item) : movieName(item)
    if (nextName && nextName !== item.name) {
      actions.push({
        id: actionId('rename', item.file_id),
        type: 'rename',
        file_id: item.file_id,
        drive_id: item.drive_id,
        parent_file_id: item.parent_file_id,
        old_name: item.name,
        new_name: nextName,
        reason: 'Normalize media filename for media server recognition.',
      })
    }

    if (rules.renameAndMove && !rules.renameOnly && (item.kind === 'movie' || item.kind === 'episode')) {
      const folderName = targetFolderFor(item)
      actions.push({
        id: actionId('move', item.file_id, folderName),
        type: 'move',
        file_id: item.file_id,
        drive_id: item.drive_id,
        parent_file_id: item.parent_file_id,
        name: nextName || item.name,
        to_parent_ref: `folder:${folderName}`,
        to_path: `${folderName}/${nextName || item.name}`,
        reason: `${item.kind === 'episode' ? 'Episode' : 'Movie'} belongs in ${folderName}.`,
      })
    }
  }

  const summary = {
    mkdirs: actions.filter((action) => action.type === 'mkdir').length,
    renames: actions.filter((action) => action.type === 'rename').length,
    moves: actions.filter((action) => action.type === 'move').length,
    conflicts: 0,
    risk: actions.length > 50 ? 'high' as const : actions.length > 10 ? 'medium' as const : 'low' as const,
    undoable: true,
  }

  return {
    version: 1,
    operation: 'media_organize',
    provider: scan.provider,
    account_id: scan.account_id,
    root_file_id: scan.root_file_id,
    created_at: new Date().toISOString(),
    rules,
    actions,
    summary,
  }
}

export function validateMediaOrganizerPlan(plan: MediaOrganizerPlan) {
  const errors: string[] = []
  if (!plan || plan.version !== 1) errors.push('version must be 1')
  if (plan?.operation !== 'media_organize') errors.push('operation must be media_organize')
  if (!plan?.provider) errors.push('provider is required')
  if (!plan?.account_id) errors.push('account_id is required')
  if (!Array.isArray(plan?.actions)) errors.push('actions must be an array')
  for (const [index, action] of (plan?.actions || []).entries()) {
    if (!['mkdir', 'rename', 'move'].includes(action.type)) errors.push(`actions[${index}].type is unsupported`)
    if (!action.drive_id) errors.push(`actions[${index}].drive_id is required`)
    if (!action.parent_file_id) errors.push(`actions[${index}].parent_file_id is required`)
    if (action.type === 'mkdir' && !action.name) errors.push(`actions[${index}].name is required`)
    if (action.type === 'rename' && (!action.file_id || !action.old_name || !action.new_name)) errors.push(`actions[${index}] rename fields are required`)
    if (action.type === 'move' && (!action.file_id || (!action.to_parent_file_id && !action.to_parent_ref))) errors.push(`actions[${index}] move target is required`)
  }
  return { ok: errors.length === 0, errors }
}

export function dryRunMediaOrganizerPlan(plan: MediaOrganizerPlan, currentItems: ClassifiedMediaItem[]): MediaOrganizerDryRun {
  const errors: MediaOrganizerDryRun['errors'] = validateMediaOrganizerPlan(plan).errors.map((message) => ({ code: 'invalid_plan', message }))
  const existingNamesByParent = new Map<string, Set<string>>()
  for (const item of currentItems) {
    const names = existingNamesByParent.get(item.parent_file_id) || new Set<string>()
    names.add(normalizeName(item.name))
    existingNamesByParent.set(item.parent_file_id, names)
  }

  const targetNamesByParent = new Map<string, Set<string>>()
  const changes = plan.actions.map((action) => {
    const afterName = action.new_name || action.name || ''
    const parent = action.parent_file_id
    if (afterName) {
      const targets = targetNamesByParent.get(parent) || new Set<string>()
      const key = normalizeName(afterName)
      if (targets.has(key)) {
        errors.push({ code: 'duplicate_target_name', message: `Duplicate target name: ${afterName}`, actionId: action.id })
      }
      targets.add(key)
      targetNamesByParent.set(parent, targets)
      if (action.type === 'rename' && existingNamesByParent.get(parent)?.has(key) && normalizeName(action.old_name || '') !== key) {
        errors.push({ code: 'target_exists', message: `Target already exists: ${afterName}`, actionId: action.id })
      }
    }
    return {
      actionId: action.id,
      type: action.type,
      file_id: action.file_id,
      beforePath: action.old_name || action.name || '',
      afterPath: action.to_path || action.new_name || action.name || '',
      conflict: false,
      conflictReason: '',
    }
  })

  return {
    ok: errors.length === 0,
    planId: `${plan.root_file_id}:${plan.created_at}`,
    generated_at: new Date().toISOString(),
    changes: errors.length === 0 ? changes : changes.map((change) => ({
      ...change,
      conflict: errors.some((error) => error.actionId === change.actionId),
      conflictReason: errors.find((error) => error.actionId === change.actionId)?.message || '',
    })),
    errors,
  }
}
```

- [ ] **Step 4: Run planner tests**

Run: `pnpm exec vitest run src/agent/mediaOrganizer/__tests__/organizePlanner.test.ts src/agent/mediaOrganizer/__tests__/mediaClassifier.test.ts`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add src/agent/mediaOrganizer/organizePlanner.ts src/agent/mediaOrganizer/__tests__/organizePlanner.test.ts
git commit -m "feat: generate media organizer plans"
```

---

### Task 4: Add Operation History

**Files:**
- Create: `src/agent/mediaOrganizer/operationHistory.ts`
- Test: `src/agent/mediaOrganizer/__tests__/operationHistory.test.ts`

- [ ] **Step 1: Write failing history tests**

Create `src/agent/mediaOrganizer/__tests__/operationHistory.test.ts`:

```ts
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { MediaOrganizerOperation } from '../types'

vi.mock('../../../utils/db', () => {
  const map = new Map<string, object>()
  return {
    default: {
      getValueObject: vi.fn((key: string) => Promise.resolve(map.get(key))),
      saveValueObject: vi.fn((key: string, value: object) => {
        map.set(key, value)
        return Promise.resolve(key)
      }),
      deleteValueObject: vi.fn((key: string) => {
        map.delete(key)
        return Promise.resolve()
      }),
      __map: map,
    },
  }
})

import DB from '../../../utils/db'
import { getMediaOrganizerOperation, listMediaOrganizerOperations, saveMediaOrganizerOperation } from '../operationHistory'

const operation: MediaOrganizerOperation = {
  id: 'op1',
  type: 'media_organize',
  provider: 'aliyun',
  account_id: 'user1',
  root_file_id: 'root',
  started_at: '2026-06-05T00:00:00.000Z',
  finished_at: '2026-06-05T00:00:01.000Z',
  items: [],
}

describe('operationHistory', () => {
  beforeEach(() => {
    ;(DB as any).__map.clear()
  })

  it('saves, gets, and lists operations newest first', async () => {
    await saveMediaOrganizerOperation({ ...operation, id: 'old', started_at: '2026-06-04T00:00:00.000Z' })
    await saveMediaOrganizerOperation(operation)

    await expect(getMediaOrganizerOperation('op1')).resolves.toMatchObject({ id: 'op1' })
    await expect(listMediaOrganizerOperations()).resolves.toEqual([
      expect.objectContaining({ id: 'op1' }),
      expect.objectContaining({ id: 'old' }),
    ])
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `pnpm exec vitest run src/agent/mediaOrganizer/__tests__/operationHistory.test.ts`

Expected: FAIL because `operationHistory.ts` does not exist.

- [ ] **Step 3: Implement history helpers**

Create `src/agent/mediaOrganizer/operationHistory.ts`:

```ts
import DB from '../../utils/db'
import type { MediaOrganizerOperation } from './types'

const INDEX_KEY = 'agent_media_organizer_operation_index'
const keyFor = (id: string) => `agent_media_organizer_operation_${id}`

async function readIndex(): Promise<string[]> {
  const value = await DB.getValueObject(INDEX_KEY)
  return Array.isArray(value) ? value.filter((id): id is string => typeof id === 'string') : []
}

async function writeIndex(ids: string[]) {
  await DB.saveValueObject(INDEX_KEY, Array.from(new Set(ids)))
}

export async function saveMediaOrganizerOperation(operation: MediaOrganizerOperation) {
  if (!operation.id) throw new Error('operation.id is required')
  await DB.saveValueObject(keyFor(operation.id), operation)
  const ids = await readIndex()
  await writeIndex([operation.id, ...ids.filter((id) => id !== operation.id)])
  return operation
}

export async function getMediaOrganizerOperation(id: string): Promise<MediaOrganizerOperation | null> {
  if (!id) return null
  return (await DB.getValueObject(keyFor(id)) as MediaOrganizerOperation | undefined) || null
}

export async function listMediaOrganizerOperations(): Promise<MediaOrganizerOperation[]> {
  const ids = await readIndex()
  const operations = await Promise.all(ids.map((id) => getMediaOrganizerOperation(id)))
  return operations
    .filter((operation): operation is MediaOrganizerOperation => !!operation)
    .sort((a, b) => String(b.started_at).localeCompare(String(a.started_at)))
}
```

- [ ] **Step 4: Run history test**

Run: `pnpm exec vitest run src/agent/mediaOrganizer/__tests__/operationHistory.test.ts`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add src/agent/mediaOrganizer/operationHistory.ts src/agent/mediaOrganizer/__tests__/operationHistory.test.ts
git commit -m "feat: store media organizer operations"
```

---

### Task 5: Add Vercel AI SDK Tools And Runtime

**Files:**
- Create: `src/agent/mediaOrganizer/aiTools.ts`
- Create: `src/agent/mediaOrganizer/runtime.ts`
- Test: `src/agent/mediaOrganizer/__tests__/runtime.test.ts`

- [ ] **Step 1: Write failing runtime tests**

Create `src/agent/mediaOrganizer/__tests__/runtime.test.ts`:

```ts
import { describe, expect, it } from 'vitest'
import { createDefaultRules } from '../organizePlanner'
import { createMediaOrganizerRuntime } from '../runtime'
import type { MediaOrganizerContext } from '../types'
import type { IAliGetFileModel } from '../../../aliapi/alimodels'

const makeFile = (name: string, file_id: string): IAliGetFileModel => ({
  drive_id: 'drive1',
  file_id,
  parent_file_id: 'root',
  name,
  ext: '',
  category: 'video',
  size: 100,
  time: 0,
  thumbnail: '',
  description: '',
  starred: false,
  isDir: false,
} as IAliGetFileModel)

const context: MediaOrganizerContext = {
  userId: 'user1',
  driveId: 'drive1',
  dirId: 'root',
  dirName: 'Media',
  path: '/Media',
  selectedFiles: [],
  items: [makeFile('Inception.2010.mkv', 'm1')],
}

describe('createMediaOrganizerRuntime', () => {
  it('creates a Vercel AI SDK agent and prepares scan, diagnosis, plan, and dry-run results', async () => {
    const runtime = createMediaOrganizerRuntime()
    const agent = runtime.createAgent(context, createDefaultRules())
    const result = await runtime.preparePlan(context, createDefaultRules())

    expect(agent).toBeTruthy()
    expect(result.scan.stats.suspectedMovies).toBe(1)
    expect(result.plan.actions.length).toBeGreaterThan(0)
    expect(result.dryRun.planId).toBeTruthy()
  })

  it('applies natural-language rule updates conservatively', () => {
    const runtime = createMediaOrganizerRuntime()
    const rules = runtime.updateRulesFromMessage(createDefaultRules(), '只重命名，不要移动，忽略字幕')

    expect(rules.renameOnly).toBe(true)
    expect(rules.renameAndMove).toBe(false)
    expect(rules.ignoreSubtitles).toBe(true)
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `pnpm exec vitest run src/agent/mediaOrganizer/__tests__/runtime.test.ts`

Expected: FAIL because `runtime.ts` and `aiTools.ts` do not exist.

- [ ] **Step 3: Implement AI SDK tools**

Create `src/agent/mediaOrganizer/aiTools.ts`:

```ts
import { tool } from 'ai'
import { z } from 'zod'
import { classifyDirectoryItems, diagnoseScan } from './mediaClassifier'
import { buildMediaOrganizerPlan, dryRunMediaOrganizerPlan } from './organizePlanner'
import type { MediaOrganizerContext, MediaOrganizerRules, MediaOrganizerScan } from './types'

export interface MediaOrganizerToolContext {
  context: MediaOrganizerContext
  rules: MediaOrganizerRules
  scan?: MediaOrganizerScan
}

const emptyInput = z.object({})

export function createMediaOrganizerTools(state: MediaOrganizerToolContext) {
  return {
    scanDirectory: tool({
      description: 'Scan the current BoxPlayer cloud-drive directory and classify media files.',
      inputSchema: emptyInput,
      execute: async () => {
        const sourceItems = state.rules.onlySelectedFiles && state.context.selectedFiles.length
          ? state.context.selectedFiles
          : state.context.items
        const scan = classifyDirectoryItems({
          provider: state.context.driveId,
          accountId: state.context.userId,
          rootFileId: state.context.dirId,
          rootName: state.context.dirName,
          items: sourceItems,
        })
        state.scan = scan
        return scan
      },
    }),

    diagnoseDirectory: tool({
      description: 'Diagnose naming, duplicate, subtitle, and organization issues in the scanned directory.',
      inputSchema: emptyInput,
      execute: async () => {
        const scan = state.scan || await createMediaOrganizerTools(state).scanDirectory.execute({})
        return diagnoseScan(scan)
      },
    }),

    generateOrganizePlan: tool({
      description: 'Generate a safe media organize plan using the current rules. This does not execute changes.',
      inputSchema: emptyInput,
      execute: async () => {
        const scan = state.scan || await createMediaOrganizerTools(state).scanDirectory.execute({})
        return buildMediaOrganizerPlan(scan, state.rules)
      },
    }),

    previewDryRun: tool({
      description: 'Preview the organize plan and report conflicts. This must happen before execution.',
      inputSchema: z.object({
        plan: z.any().describe('The media organizer plan returned by generateOrganizePlan.'),
      }),
      execute: async ({ plan }) => {
        const scan = state.scan || await createMediaOrganizerTools(state).scanDirectory.execute({})
        return dryRunMediaOrganizerPlan(plan, scan.items)
      },
    }),
  }
}
```

- [ ] **Step 4: Implement Vercel AI SDK runtime facade**

Create `src/agent/mediaOrganizer/runtime.ts`:

```ts
import { ToolLoopAgent, gateway, stepCountIs } from 'ai'
import { createMediaOrganizerTools } from './aiTools'
import type { MediaOrganizerContext, MediaOrganizerRules } from './types'

const DEFAULT_AGENT_MODEL = 'openai/gpt-5-mini'

export function createMediaOrganizerRuntime() {
  return {
    createAgent(context: MediaOrganizerContext, rules: MediaOrganizerRules) {
      return new ToolLoopAgent({
        model: gateway(DEFAULT_AGENT_MODEL),
        instructions: [
          'You are BoxPlayer Media Organizer Agent.',
          'Use tools to scan, diagnose, plan, and dry-run media organization changes.',
          'Never execute file writes inside the autonomous tool loop.',
          'All rename and move operations require the app confirmation UI.',
        ].join('\n'),
        tools: createMediaOrganizerTools({ context, rules }),
        stopWhen: stepCountIs(6),
      })
    },

    async preparePlan(context: MediaOrganizerContext, rules: MediaOrganizerRules) {
      const toolState = { context, rules }
      const tools = createMediaOrganizerTools(toolState)
      this.createAgent(context, rules)
      const scan = await tools.scanDirectory.execute({})
      const diagnosis = await tools.diagnoseDirectory.execute({})
      const plan = await tools.generateOrganizePlan.execute({})
      const dryRun = await tools.previewDryRun.execute({ plan })
      return { scan, diagnosis, plan, dryRun }
    },

    async runChatMessage(context: MediaOrganizerContext, rules: MediaOrganizerRules, message: string) {
      const agent = this.createAgent(context, rules)
      return agent.generate({
        prompt: [
          `Current directory: ${context.path || context.dirName}`,
          `Current rules: ${JSON.stringify(rules)}`,
          message,
        ].join('\n'),
      })
    },

    updateRulesFromMessage(rules: MediaOrganizerRules, message: string): MediaOrganizerRules {
      const text = message.toLowerCase()
      const next = { ...rules }
      if (/只重命名|不要移动|rename only/.test(text)) {
        next.renameOnly = true
        next.renameAndMove = false
      }
      if (/重命名.*移动|移动.*重命名|rename.*move/.test(text)) {
        next.renameOnly = false
        next.renameAndMove = true
      }
      if (/忽略字幕|不要字幕|ignore subtitle/.test(text)) next.ignoreSubtitles = true
      if (/保留中文|中文名|keep chinese/.test(text)) next.keepChineseTitle = true
      if (/plex/.test(text)) next.style = 'plex'
      if (/emby/.test(text)) next.style = 'emby'
      if (/jellyfin/.test(text)) next.style = 'jellyfin'
      return next
    },
  }
}

export type MediaOrganizerRuntime = ReturnType<typeof createMediaOrganizerRuntime>
```

- [ ] **Step 5: Run runtime tests**

Run: `pnpm exec vitest run src/agent/mediaOrganizer/__tests__/runtime.test.ts src/agent/mediaOrganizer/__tests__/mediaClassifier.test.ts src/agent/mediaOrganizer/__tests__/organizePlanner.test.ts`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add src/agent/mediaOrganizer/aiTools.ts src/agent/mediaOrganizer/runtime.ts src/agent/mediaOrganizer/__tests__/runtime.test.ts
git commit -m "feat: add vercel ai media organizer runtime"
```

---

### Task 6: Add Pinia Drawer Store

**Files:**
- Create: `src/store/agentMediaOrganizer.ts`
- Modify: `src/store/index.ts`
- Test: `src/agent/mediaOrganizer/__tests__/store.test.ts`

- [ ] **Step 1: Write failing store tests**

Create `src/agent/mediaOrganizer/__tests__/store.test.ts`:

```ts
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it } from 'vitest'
import useAgentMediaOrganizerStore from '../../../store/agentMediaOrganizer'
import type { MediaOrganizerContext } from '../types'

const context: MediaOrganizerContext = {
  userId: 'user1',
  driveId: 'drive1',
  dirId: 'root',
  dirName: 'Media',
  path: '/Media',
  selectedFiles: [],
  items: [],
}

describe('agentMediaOrganizer store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('opens with directory context in plan mode', () => {
    const store = useAgentMediaOrganizerStore()
    store.openForDirectory(context)

    expect(store.visible).toBe(true)
    expect(store.permissionMode).toBe('plan_mode')
    expect(store.context?.dirName).toBe('Media')
    expect(store.status).toBe('waiting')
  })

  it('blocks execution until a successful dry-run exists and confirmation is requested', () => {
    const store = useAgentMediaOrganizerStore()
    store.openForDirectory(context)

    expect(store.canExecute).toBe(false)
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `pnpm exec vitest run src/agent/mediaOrganizer/__tests__/store.test.ts`

Expected: FAIL because `src/store/agentMediaOrganizer.ts` does not exist.

- [ ] **Step 3: Implement store**

Create `src/store/agentMediaOrganizer.ts`:

```ts
import { defineStore } from 'pinia'
import { createDefaultRules } from '../agent/mediaOrganizer/organizePlanner'
import { createMediaOrganizerRuntime } from '../agent/mediaOrganizer/runtime'
import type {
  AgentPermissionMode,
  AgentTaskStatus,
  MediaOrganizerContext,
  MediaOrganizerDiagnosis,
  MediaOrganizerDryRun,
  MediaOrganizerPlan,
  MediaOrganizerRules,
  MediaOrganizerScan,
} from '../agent/mediaOrganizer/types'

const runtime = createMediaOrganizerRuntime()

interface State {
  visible: boolean
  status: AgentTaskStatus
  permissionMode: AgentPermissionMode
  context: MediaOrganizerContext | null
  rules: MediaOrganizerRules
  scan: MediaOrganizerScan | null
  diagnosis: MediaOrganizerDiagnosis | null
  plan: MediaOrganizerPlan | null
  dryRun: MediaOrganizerDryRun | null
  error: string
  chatInput: string
}

const useAgentMediaOrganizerStore = defineStore('agentMediaOrganizer', {
  state: (): State => ({
    visible: false,
    status: 'waiting',
    permissionMode: 'plan_mode',
    context: null,
    rules: createDefaultRules(),
    scan: null,
    diagnosis: null,
    plan: null,
    dryRun: null,
    error: '',
    chatInput: '',
  }),

  getters: {
    canExecute(state): boolean {
      return state.permissionMode === 'execution_confirmation' && !!state.dryRun?.ok && !!state.plan
    },
  },

  actions: {
    openForDirectory(context: MediaOrganizerContext) {
      this.visible = true
      this.status = 'waiting'
      this.permissionMode = 'plan_mode'
      this.context = context
      this.rules = createDefaultRules()
      this.scan = null
      this.diagnosis = null
      this.plan = null
      this.dryRun = null
      this.error = ''
    },

    close() {
      this.visible = false
    },

    async preparePlan() {
      if (!this.context) return
      this.status = 'scanning'
      this.error = ''
      try {
        const result = await runtime.preparePlan(this.context, this.rules)
        this.scan = result.scan
        this.diagnosis = result.diagnosis
        this.status = 'generating_plan'
        this.plan = result.plan
        this.dryRun = result.dryRun
        this.status = result.dryRun.ok ? 'waiting_for_confirmation' : 'failed'
        this.permissionMode = result.dryRun.ok ? 'execution_confirmation' : 'plan_mode'
      } catch (error: any) {
        this.status = 'failed'
        this.error = error?.message || 'AI 整理失败'
      }
    },

    async setRules(patch: Partial<MediaOrganizerRules>) {
      this.rules = { ...this.rules, ...patch }
      await this.preparePlan()
    },

    async submitChatMessage(message: string) {
      if (!message.trim()) return
      this.rules = runtime.updateRulesFromMessage(this.rules, message)
      this.chatInput = ''
      await this.preparePlan()
    },
  },
})

export default useAgentMediaOrganizerStore
```

Modify `src/store/index.ts`:

```ts
import useAgentMediaOrganizerStore from './agentMediaOrganizer'
```

Add `useAgentMediaOrganizerStore` to the export list.

- [ ] **Step 4: Run store test**

Run: `pnpm exec vitest run src/agent/mediaOrganizer/__tests__/store.test.ts`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add src/store/agentMediaOrganizer.ts src/store/index.ts src/agent/mediaOrganizer/__tests__/store.test.ts
git commit -m "feat: add media organizer drawer store"
```

---

### Task 7: Build Agent Drawer UI

**Files:**
- Create: `src/components/AgentMediaOrganizerDrawer.vue`
- Modify: `src/pan/index.vue` or the smallest parent page discovered during implementation to mount the drawer once.

- [ ] **Step 1: Create drawer component**

Create `src/components/AgentMediaOrganizerDrawer.vue`:

```vue
<script setup lang="ts">
import { computed } from 'vue'
import { useAgentMediaOrganizerStore } from '../store'

const store = useAgentMediaOrganizerStore()

const statusText = computed(() => ({
  waiting: '等待开始',
  scanning: '扫描中',
  diagnosis_complete: '诊断完成',
  waiting_for_rules: '等待规则',
  generating_plan: '生成计划中',
  waiting_for_dry_run: '等待预览',
  waiting_for_confirmation: '等待确认',
  executing: '执行中',
  complete: '完成',
  undo_available: '可撤销',
  failed: '失败',
}[store.status]))

const submitChat = () => store.submitChatMessage(store.chatInput)
</script>

<template>
  <a-drawer
    :visible="store.visible"
    :width="460"
    title="AI 整理"
    placement="right"
    :footer="false"
    :unmount-on-close="false"
    @cancel="store.close"
  >
    <div class="agent-drawer">
      <section class="agent-header">
        <div class="agent-title">{{ store.context?.dirName || '当前目录' }}</div>
        <div class="agent-meta">{{ statusText }} · {{ store.permissionMode === 'plan_mode' ? '计划模式' : store.permissionMode === 'read_only' ? '只读模式' : '执行确认' }}</div>
      </section>

      <a-alert v-if="store.error" type="error" :content="store.error" />

      <a-card v-if="store.scan" class="agent-card" title="扫描结果" :bordered="false">
        <div class="agent-grid">
          <span>文件 {{ store.scan.stats.fileCount }}</span>
          <span>文件夹 {{ store.scan.stats.folderCount }}</span>
          <span>视频 {{ store.scan.stats.videoCount }}</span>
          <span>字幕 {{ store.scan.stats.subtitleCount }}</span>
          <span>电影 {{ store.scan.stats.suspectedMovies }}</span>
          <span>剧集 {{ store.scan.stats.suspectedEpisodes }}</span>
        </div>
      </a-card>

      <a-card v-if="store.diagnosis" class="agent-card" title="问题诊断" :bordered="false">
        <a-empty v-if="store.diagnosis.issues.length === 0" description="暂未发现明显问题" />
        <a-list v-else size="small" :bordered="false">
          <a-list-item v-for="issue in store.diagnosis.issues" :key="issue.id">
            <a-list-item-meta :title="issue.title" :description="issue.detail" />
          </a-list-item>
        </a-list>
      </a-card>

      <a-card class="agent-card" title="整理规则" :bordered="false">
        <a-space wrap>
          <a-button size="small" :type="store.rules.style === 'jellyfin' ? 'primary' : 'secondary'" @click="store.setRules({ style: 'jellyfin' })">Jellyfin</a-button>
          <a-button size="small" :type="store.rules.style === 'emby' ? 'primary' : 'secondary'" @click="store.setRules({ style: 'emby' })">Emby</a-button>
          <a-button size="small" :type="store.rules.style === 'plex' ? 'primary' : 'secondary'" @click="store.setRules({ style: 'plex' })">Plex</a-button>
          <a-button size="small" :type="store.rules.renameOnly ? 'primary' : 'secondary'" @click="store.setRules({ renameOnly: true, renameAndMove: false })">只重命名</a-button>
          <a-button size="small" :type="store.rules.ignoreSubtitles ? 'primary' : 'secondary'" @click="store.setRules({ ignoreSubtitles: !store.rules.ignoreSubtitles })">忽略字幕</a-button>
        </a-space>
      </a-card>

      <a-card v-if="store.plan" class="agent-card" title="整理计划" :bordered="false">
        <div class="agent-grid">
          <span>新建 {{ store.plan.summary.mkdirs }}</span>
          <span>重命名 {{ store.plan.summary.renames }}</span>
          <span>移动 {{ store.plan.summary.moves }}</span>
          <span>风险 {{ store.plan.summary.risk }}</span>
        </div>
      </a-card>

      <a-card v-if="store.dryRun" class="agent-card" title="Dry-run 预览" :bordered="false">
        <a-alert v-if="!store.dryRun.ok" type="warning" content="预览存在冲突，不能执行。" />
        <a-list size="small" :bordered="false" class="agent-preview-list">
          <a-list-item v-for="change in store.dryRun.changes.slice(0, 20)" :key="change.actionId">
            <div class="agent-change">
              <div>{{ change.type }}</div>
              <div class="agent-path">{{ change.beforePath }} -> {{ change.afterPath }}</div>
              <a-tag v-if="change.conflict" color="red">{{ change.conflictReason }}</a-tag>
            </div>
          </a-list-item>
        </a-list>
      </a-card>

      <div class="agent-actions">
        <a-button type="primary" long :disabled="!store.canExecute">应用计划</a-button>
      </div>

      <div class="agent-chat">
        <a-space wrap size="mini">
          <a-button size="mini" @click="store.setRules({ onlySelectedFiles: true })">只处理选中</a-button>
          <a-button size="mini" @click="store.setRules({ renameOnly: true, renameAndMove: false })">只重命名</a-button>
          <a-button size="mini" @click="store.preparePlan()">重新扫描</a-button>
        </a-space>
        <a-input-search
          v-model="store.chatInput"
          search-button
          button-text="发送"
          placeholder="例如：只整理电影，不动剧集"
          @search="submitChat"
        />
      </div>
    </div>
  </a-drawer>
</template>

<style scoped>
.agent-drawer {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.agent-header {
  padding-bottom: 6px;
}
.agent-title {
  font-size: 16px;
  font-weight: 600;
}
.agent-meta {
  color: var(--color-text-3);
  font-size: 12px;
  margin-top: 4px;
}
.agent-card {
  border-radius: 6px;
  background: var(--color-fill-1);
}
.agent-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px;
  font-size: 13px;
}
.agent-preview-list {
  max-height: 260px;
  overflow: auto;
}
.agent-change {
  min-width: 0;
  width: 100%;
}
.agent-path {
  color: var(--color-text-2);
  font-size: 12px;
  overflow-wrap: anywhere;
}
.agent-actions,
.agent-chat {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
</style>
```

- [ ] **Step 2: Mount drawer once**

Inspect `src/pan/index.vue`. If it is the pan page root, add:

```vue
<script setup lang="ts">
import AgentMediaOrganizerDrawer from '../components/AgentMediaOrganizerDrawer.vue'
</script>

<template>
  <!-- existing content -->
  <AgentMediaOrganizerDrawer />
</template>
```

If `src/pan/index.vue` already has a script/template, merge the import and component into the existing structure without changing unrelated layout.

- [ ] **Step 3: Run component type check**

Run: `pnpm exec vue-tsc --noEmit --pretty false`

Expected: it may fail on existing repo-wide declaration debt; it must not report errors for `AgentMediaOrganizerDrawer.vue` or `agentMediaOrganizer.ts`.

- [ ] **Step 4: Commit**

```bash
git add src/components/AgentMediaOrganizerDrawer.vue src/pan/index.vue
git commit -m "feat: add media organizer agent drawer"
```

---

### Task 8: Add Toolbar And Context Menu Entry Points

**Files:**
- Modify: `src/pan/menus/PanTopbtn.vue`
- Modify: `src/pan/menus/FileRightMenu.vue`
- Test: extend `src/agent/mediaOrganizer/__tests__/store.test.ts` if helper extraction is needed.

- [ ] **Step 1: Add context builder helper if needed**

If both menu files need the same context-building logic, create `src/agent/mediaOrganizer/panContext.ts`:

```ts
import type { IAliGetFileModel } from '../../aliapi/alimodels'
import type { MediaOrganizerContext } from './types'

export function buildMediaOrganizerContext(input: {
  userId: string
  driveId: string
  dirId: string
  dirName: string
  path?: string
  selectedFiles?: IAliGetFileModel[]
  items: IAliGetFileModel[]
}): MediaOrganizerContext {
  return {
    userId: input.userId,
    driveId: input.driveId,
    dirId: input.dirId,
    dirName: input.dirName,
    path: input.path || input.dirName,
    selectedFiles: input.selectedFiles || [],
    items: input.items,
  }
}
```

- [ ] **Step 2: Add toolbar button**

Modify `src/pan/menus/PanTopbtn.vue`:

```ts
import { useAgentMediaOrganizerStore, usePanFileStore } from '../../store'
import { buildMediaOrganizerContext } from '../../agent/mediaOrganizer/panContext'
```

Inside setup:

```ts
const panFileStore = usePanFileStore()
const agentStore = useAgentMediaOrganizerStore()

const canShowAiOrganize = computed(() => {
  return !props.isselected && props.dirtype === 'pan' && !!panTreeStore.user_id && !!panFileStore.DirID
})

const handleAiOrganizeCurrentDir = async () => {
  agentStore.openForDirectory(buildMediaOrganizerContext({
    userId: panTreeStore.user_id || '',
    driveId: panTreeStore.drive_id || panFileStore.DriveID,
    dirId: panFileStore.DirID,
    dirName: panFileStore.DirName || panTreeStore.selectDir.name || '当前目录',
    path: panTreeStore.selectDir.path || panFileStore.DirName,
    selectedFiles: panFileStore.GetSelected(),
    items: panFileStore.ListDataRaw,
  }))
  await agentStore.preparePlan()
}
```

Add a button in the normal pan toolbar group:

```vue
<a-button v-if="canShowAiOrganize" type="text" size="small" tabindex="-1" @click="handleAiOrganizeCurrentDir">
  <IconFont name="iconscan" />AI 整理
</a-button>
```

- [ ] **Step 3: Add folder context-menu entry**

Modify `src/pan/menus/FileRightMenu.vue`:

```ts
import { useAgentMediaOrganizerStore } from '../../store'
import { buildMediaOrganizerContext } from '../../agent/mediaOrganizer/panContext'
```

Inside setup:

```ts
const agentStore = useAgentMediaOrganizerStore()

const handleAiDiagnoseFolder = async () => {
  const folder = pickFolderForScan()
  if (!folder) return
  agentStore.openForDirectory(buildMediaOrganizerContext({
    userId: (folder as any).user_id || panTreeStore.user_id || '',
    driveId: folder.drive_id || panTreeStore.drive_id,
    dirId: folder.file_id,
    dirName: folder.name,
    path: folder.path || folder.name,
    selectedFiles: [folder],
    items: [folder],
  }))
  await agentStore.preparePlan()
}
```

Add under the existing scan submenu or near it:

```vue
<a-doption v-if="isSelectedFolder && isShowBtn" @click="handleAiDiagnoseFolder">
  <template #icon><IconFont name="iconscan" /></template>
  <template #default>AI 诊断此目录</template>
</a-doption>
```

- [ ] **Step 4: Run focused tests and type check**

Run: `pnpm exec vitest run src/agent/mediaOrganizer/__tests__`

Expected: PASS.

Run: `pnpm exec vue-tsc --noEmit --pretty false`

Expected: may fail on existing repo-wide declaration debt; no new errors from changed files.

- [ ] **Step 5: Commit**

```bash
git add src/agent/mediaOrganizer/panContext.ts src/pan/menus/PanTopbtn.vue src/pan/menus/FileRightMenu.vue
git commit -m "feat: add media organizer agent entry points"
```

---

### Task 9: Add Safe Execution Adapter

**Files:**
- Create: `src/agent/mediaOrganizer/executor.ts`
- Modify: `src/store/agentMediaOrganizer.ts`
- Test: `src/agent/mediaOrganizer/__tests__/executor.test.ts`

- [ ] **Step 1: Write failing executor tests**

Create `src/agent/mediaOrganizer/__tests__/executor.test.ts`:

```ts
import { describe, expect, it, vi } from 'vitest'
import type { MediaOrganizerPlan } from '../types'

vi.mock('../../../aliapi/filecmd', () => ({
  default: {
    ApiCreatNewForder: vi.fn(() => Promise.resolve({ file_id: 'folder1', error: '' })),
    ApiRenameBatch: vi.fn(() => Promise.resolve({ successList: ['file1'], failList: [] })),
    ApiMoveBatch: vi.fn(() => Promise.resolve(['file1'])),
  },
}))

import { applyMediaOrganizerPlan } from '../executor'

const plan: MediaOrganizerPlan = {
  version: 1,
  operation: 'media_organize',
  provider: 'drive1',
  account_id: 'user1',
  root_file_id: 'root',
  created_at: '2026-06-05T00:00:00.000Z',
  rules: {
    style: 'jellyfin',
    renameOnly: false,
    renameAndMove: true,
    keepChineseTitle: true,
    ignoreSubtitles: false,
    onlySelectedFiles: false,
  },
  actions: [
    { id: 'mkdir:root:Movies', type: 'mkdir', drive_id: 'drive1', parent_file_id: 'root', name: 'Movies', reason: 'required' },
    { id: 'rename:file1', type: 'rename', drive_id: 'drive1', parent_file_id: 'root', file_id: 'file1', old_name: 'A.mkv', new_name: 'A (2020).mkv', reason: 'normalize' },
    { id: 'move:file1:Movies', type: 'move', drive_id: 'drive1', parent_file_id: 'root', file_id: 'file1', to_parent_file_id: 'folder1', name: 'A (2020).mkv', reason: 'move' },
  ],
  summary: { mkdirs: 1, renames: 1, moves: 1, conflicts: 0, risk: 'low', undoable: true },
}

describe('applyMediaOrganizerPlan', () => {
  it('applies mkdir, rename, and move actions and returns operation items', async () => {
    const operation = await applyMediaOrganizerPlan({ userId: 'user1', plan })

    expect(operation.type).toBe('media_organize')
    expect(operation.items).toHaveLength(3)
    expect(operation.items.every((item) => item.status === 'success')).toBe(true)
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `pnpm exec vitest run src/agent/mediaOrganizer/__tests__/executor.test.ts`

Expected: FAIL because `executor.ts` does not exist.

- [ ] **Step 3: Implement executor**

Create `src/agent/mediaOrganizer/executor.ts`:

```ts
import AliFileCmd from '../../aliapi/filecmd'
import { saveMediaOrganizerOperation } from './operationHistory'
import type { MediaOrganizerOperation, MediaOrganizerOperationItem, MediaOrganizerPlan } from './types'

const operationId = () => `agent_media_${Date.now()}_${Math.random().toString(16).slice(2)}`

export async function applyMediaOrganizerPlan(input: { userId: string; plan: MediaOrganizerPlan }): Promise<MediaOrganizerOperation> {
  const started = new Date().toISOString()
  const createdFolders = new Map<string, string>()
  const items: MediaOrganizerOperationItem[] = []

  for (const action of input.plan.actions) {
    try {
      if (action.type === 'mkdir') {
        const result = await AliFileCmd.ApiCreatNewForder(input.userId, action.drive_id, action.parent_file_id, action.name || '')
        if (result.error) throw new Error(result.error)
        createdFolders.set(`folder:${action.name}`, result.file_id)
        items.push({
          actionId: action.id,
          type: 'mkdir',
          drive_id: action.drive_id,
          parent_file_id: action.parent_file_id,
          file_id: result.file_id,
          after_name: action.name,
          status: 'success',
        })
      }
      if (action.type === 'rename' && action.file_id && action.new_name) {
        const result = await AliFileCmd.ApiRenameBatch(input.userId, action.drive_id, [action.file_id], [action.new_name])
        const ok = result.successList.includes(action.file_id)
        if (!ok) throw new Error(result.failList[0]?.error || 'rename failed')
        items.push({
          actionId: action.id,
          type: 'rename',
          file_id: action.file_id,
          drive_id: action.drive_id,
          parent_file_id: action.parent_file_id,
          before_name: action.old_name,
          after_name: action.new_name,
          status: 'success',
        })
      }
      if (action.type === 'move' && action.file_id) {
        const target = action.to_parent_file_id || createdFolders.get(action.to_parent_ref || '')
        if (!target) throw new Error('move target folder is missing')
        const result = await AliFileCmd.ApiMoveBatch(input.userId, action.drive_id, [action.file_id], action.drive_id, target)
        const ok = result.includes(action.file_id)
        if (!ok) throw new Error('move failed')
        items.push({
          actionId: action.id,
          type: 'move',
          file_id: action.file_id,
          drive_id: action.drive_id,
          parent_file_id: action.parent_file_id,
          from_parent_file_id: action.parent_file_id,
          to_parent_file_id: target,
          status: 'success',
        })
      }
    } catch (error: any) {
      items.push({
        actionId: action.id,
        type: action.type,
        file_id: action.file_id,
        drive_id: action.drive_id,
        parent_file_id: action.parent_file_id,
        status: 'failed',
        error: error?.message || 'operation failed',
      })
    }
  }

  const operation: MediaOrganizerOperation = {
    id: operationId(),
    type: 'media_organize',
    provider: input.plan.provider,
    account_id: input.plan.account_id,
    root_file_id: input.plan.root_file_id,
    started_at: started,
    finished_at: new Date().toISOString(),
    items,
  }
  await saveMediaOrganizerOperation(operation)
  return operation
}
```

- [ ] **Step 4: Wire explicit execution action in store**

Modify `src/store/agentMediaOrganizer.ts`:

```ts
import { applyMediaOrganizerPlan } from '../agent/mediaOrganizer/executor'
import type { MediaOrganizerOperation } from '../agent/mediaOrganizer/types'
```

Add `operation: MediaOrganizerOperation | null` to state.

Initialize and reset `operation` to `null`.

Add action:

```ts
async executeConfirmedPlan() {
  if (!this.canExecute || !this.plan || !this.context) {
    this.error = '请先生成可执行的 dry-run 预览'
    return
  }
  this.status = 'executing'
  const operation = await applyMediaOrganizerPlan({ userId: this.context.userId, plan: this.plan })
  this.operation = operation
  this.status = operation.items.some((item) => item.status === 'success') ? 'undo_available' : 'failed'
}
```

Wire drawer `应用计划` button to `store.executeConfirmedPlan()`.

- [ ] **Step 5: Run executor and store tests**

Run: `pnpm exec vitest run src/agent/mediaOrganizer/__tests__/executor.test.ts src/agent/mediaOrganizer/__tests__/store.test.ts src/agent/mediaOrganizer/__tests__/operationHistory.test.ts`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add src/agent/mediaOrganizer/executor.ts src/agent/mediaOrganizer/__tests__/executor.test.ts src/store/agentMediaOrganizer.ts src/components/AgentMediaOrganizerDrawer.vue
git commit -m "feat: execute confirmed media organizer plans"
```

---

### Task 10: Add Undo For Successful Rename And Move Items

**Files:**
- Modify: `src/agent/mediaOrganizer/executor.ts`
- Modify: `src/store/agentMediaOrganizer.ts`
- Modify: `src/components/AgentMediaOrganizerDrawer.vue`
- Test: `src/agent/mediaOrganizer/__tests__/executor.test.ts`

- [ ] **Step 1: Extend executor test for undo**

Add to `src/agent/mediaOrganizer/__tests__/executor.test.ts`:

```ts
import { undoMediaOrganizerOperation } from '../executor'

it('undoes successful rename and move operation items', async () => {
  const operation = await applyMediaOrganizerPlan({ userId: 'user1', plan })
  const undo = await undoMediaOrganizerOperation({ userId: 'user1', operation })

  expect(undo.type).toBe('media_organize')
  expect(undo.items.length).toBeGreaterThan(0)
  expect(undo.items.every((item) => item.status === 'success')).toBe(true)
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `pnpm exec vitest run src/agent/mediaOrganizer/__tests__/executor.test.ts`

Expected: FAIL because `undoMediaOrganizerOperation` does not exist.

- [ ] **Step 3: Implement undo**

Add to `src/agent/mediaOrganizer/executor.ts`:

```ts
export async function undoMediaOrganizerOperation(input: { userId: string; operation: MediaOrganizerOperation }): Promise<MediaOrganizerOperation> {
  const started = new Date().toISOString()
  const items: MediaOrganizerOperationItem[] = []

  for (const item of input.operation.items.slice().reverse()) {
    if (item.status !== 'success') continue
    try {
      if (item.type === 'rename' && item.file_id && item.before_name) {
        const result = await AliFileCmd.ApiRenameBatch(input.userId, item.drive_id, [item.file_id], [item.before_name])
        if (!result.successList.includes(item.file_id)) throw new Error(result.failList[0]?.error || 'undo rename failed')
        items.push({ ...item, actionId: `undo:${item.actionId}`, before_name: item.after_name, after_name: item.before_name, status: 'success' })
      }
      if (item.type === 'move' && item.file_id && item.from_parent_file_id) {
        const result = await AliFileCmd.ApiMoveBatch(input.userId, item.drive_id, [item.file_id], item.drive_id, item.from_parent_file_id)
        if (!result.includes(item.file_id)) throw new Error('undo move failed')
        items.push({ ...item, actionId: `undo:${item.actionId}`, from_parent_file_id: item.to_parent_file_id, to_parent_file_id: item.from_parent_file_id, status: 'success' })
      }
    } catch (error: any) {
      items.push({ ...item, actionId: `undo:${item.actionId}`, status: 'failed', error: error?.message || 'undo failed' })
    }
  }

  const undoOperation: MediaOrganizerOperation = {
    id: operationId(),
    type: 'media_organize',
    provider: input.operation.provider,
    account_id: input.operation.account_id,
    root_file_id: input.operation.root_file_id,
    started_at: started,
    finished_at: new Date().toISOString(),
    items,
  }
  await saveMediaOrganizerOperation(undoOperation)
  return undoOperation
}
```

- [ ] **Step 4: Wire undo action in store and drawer**

Modify store:

```ts
import { applyMediaOrganizerPlan, undoMediaOrganizerOperation } from '../agent/mediaOrganizer/executor'
```

Add action:

```ts
async undoLastOperation() {
  if (!this.operation || !this.context) {
    this.error = '没有可撤销的操作'
    return
  }
  this.status = 'executing'
  this.operation = await undoMediaOrganizerOperation({ userId: this.context.userId, operation: this.operation })
  this.status = 'complete'
}
```

Add drawer button near the result:

```vue
<a-button v-if="store.operation && store.status === 'undo_available'" long @click="store.undoLastOperation()">撤销本次操作</a-button>
```

- [ ] **Step 5: Run executor tests**

Run: `pnpm exec vitest run src/agent/mediaOrganizer/__tests__/executor.test.ts`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add src/agent/mediaOrganizer/executor.ts src/agent/mediaOrganizer/__tests__/executor.test.ts src/store/agentMediaOrganizer.ts src/components/AgentMediaOrganizerDrawer.vue
git commit -m "feat: undo media organizer operations"
```

---

### Task 11: Add Large Scope And Provider Safety Guards

**Files:**
- Modify: `src/store/agentMediaOrganizer.ts`
- Modify: `src/components/AgentMediaOrganizerDrawer.vue`
- Test: `src/agent/mediaOrganizer/__tests__/store.test.ts`

- [ ] **Step 1: Extend store tests**

Add to `src/agent/mediaOrganizer/__tests__/store.test.ts`:

```ts
it('requires large-scope confirmation before scanning more than 500 items', async () => {
  const store = useAgentMediaOrganizerStore()
  store.openForDirectory({
    ...context,
    items: Array.from({ length: 501 }, (_, index) => ({
      drive_id: 'drive1',
      file_id: `file${index}`,
      parent_file_id: 'root',
      name: `Movie.${index}.mkv`,
      category: 'video',
      size: 1,
      isDir: false,
    } as any)),
  })

  await store.preparePlan()

  expect(store.status).toBe('failed')
  expect(store.error).toMatch(/范围较大/)
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `pnpm exec vitest run src/agent/mediaOrganizer/__tests__/store.test.ts`

Expected: FAIL because no large-scope guard exists.

- [ ] **Step 3: Add guard state and actions**

Modify `src/store/agentMediaOrganizer.ts`:

```ts
largeScopeConfirmed: false,
```

Add to state type and initial state.

At the start of `preparePlan()`:

```ts
if (this.context.items.length > 500 && !this.largeScopeConfirmed) {
  this.status = 'failed'
  this.error = '当前目录范围较大，请缩小范围或确认继续扫描'
  return
}
```

Add action:

```ts
async confirmLargeScopeAndPrepare() {
  this.largeScopeConfirmed = true
  await this.preparePlan()
}
```

Reset `largeScopeConfirmed` in `openForDirectory()`.

- [ ] **Step 4: Add drawer next action**

In `AgentMediaOrganizerDrawer.vue`, below the error alert:

```vue
<a-button v-if="store.error.includes('范围较大')" type="primary" @click="store.confirmLargeScopeAndPrepare()">确认继续扫描</a-button>
```

- [ ] **Step 5: Run focused store tests**

Run: `pnpm exec vitest run src/agent/mediaOrganizer/__tests__/store.test.ts`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add src/store/agentMediaOrganizer.ts src/components/AgentMediaOrganizerDrawer.vue src/agent/mediaOrganizer/__tests__/store.test.ts
git commit -m "feat: guard large media organizer scans"
```

---

### Task 12: Final Verification And Manual Smoke

**Files:**
- No new files unless fixing issues found during verification.

- [ ] **Step 1: Run all media organizer tests**

Run: `pnpm exec vitest run src/agent/mediaOrganizer/__tests__`

Expected: PASS.

- [ ] **Step 2: Run related existing CLI/media tests**

Run: `pnpm exec vitest run clouddrive-cli/__tests__/media.test.ts clouddrive-cli/__tests__/core.test.ts`

Expected: PASS, or document pre-existing failures with exact error output.

- [ ] **Step 3: Run type check**

Run: `pnpm exec vue-tsc --noEmit --pretty false`

Expected: Ideally PASS. If blocked by existing repo-wide declaration debt, confirm there are no errors from:

- `src/agent/mediaOrganizer/`
- `src/store/agentMediaOrganizer.ts`
- `src/components/AgentMediaOrganizerDrawer.vue`
- `src/pan/menus/PanTopbtn.vue`
- `src/pan/menus/FileRightMenu.vue`

- [ ] **Step 4: Run dev server**

Run: `pnpm run dev`

Expected: Vite/Electron dev server starts. Keep command output byte-capped if running in this environment.

- [ ] **Step 5: Manual smoke**

In the app:

1. Open a normal cloud-drive directory.
2. Confirm `AI 整理` appears in the toolbar.
3. Click `AI 整理`.
4. Confirm the right drawer opens without leaving the directory.
5. Confirm scan, diagnosis, plan, and dry-run cards render.
6. Type `只重命名，不要移动` in the chatbox.
7. Confirm the plan regenerates with no move actions.
8. Confirm `应用计划` is disabled when dry-run has conflicts.
9. On a small safe test directory, confirm execution requires the button click and then shows an undo action.

- [ ] **Step 6: Commit final fixes**

If verification required fixes:

```bash
git add <changed-files>
git commit -m "fix: verify media organizer agent flow"
```

If no fixes were needed, do not create an empty commit.

---

## Plan Self-Review

### Spec Coverage

- `AI Organize` toolbar entry: Task 8.
- `AI Diagnose This Folder` context entry: Task 8.
- Right-side Agent drawer: Task 7.
- Structured cards: Task 7.
- Contextual chatbox for rule changes: Tasks 5, 6, 7.
- Current directory scanning: Tasks 2, 5, 6, 8.
- Movie, episode, subtitle, duplicate, unknown detection: Task 2.
- Naming issue diagnosis: Task 2.
- Rename/move planning: Task 3.
- Dry-run before writes: Tasks 3, 6, 9.
- Explicit confirmation before execution: Tasks 6, 7, 9.
- Operation history: Task 4 and Task 9.
- Undo: Task 10.
- Large-scope guard: Task 11.
- Provider limitation/error cards: Task 7 starts the card UI; Task 9 surfaces operation errors. A richer provider capability matrix can be added after MVP if existing `AliFileCmd` returns provider-specific failures.

### Placeholder Scan

The plan contains no unresolved placeholder markers or deferred implementation notes. Where component ownership is uncertain, the plan gives exact candidate files and a decision rule: mount in the smallest pan parent that keeps the drawer visible.

### Type Consistency

The plan uses `MediaOrganizerRules`, `MediaOrganizerPlan`, `MediaOrganizerDryRun`, `MediaOrganizerOperation`, and `useAgentMediaOrganizerStore` consistently across tasks.
