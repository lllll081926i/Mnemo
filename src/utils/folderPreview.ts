import { IAliGetFileModel } from '../aliapi/alimodels'
import AliDirFileList from '../aliapi/dirfilelist'
import {
  isAliyunUser,
  isPikPakUser
} from '../aliapi/utils'
import DebugLog from './debuglog'

export interface FolderPreviewParams {
  user_id: string
  drive_id: string
  file_id: string
  name?: string
  path?: string
}

const CACHE_TTL_MS = 5 * 60 * 1000
const MAX_PREVIEW_FILES = 12

interface CacheEntry {
  ts: number
  items: IAliGetFileModel[]
  promise?: Promise<IAliGetFileModel[]>
}

const previewCache = new Map<string, CacheEntry>()

const cacheKey = (p: FolderPreviewParams) => `${p.user_id}|${p.drive_id}|${p.file_id}`

const tryDynamicImport = async <T>(loader: () => Promise<T>): Promise<T | null> => {
  try {
    return await loader()
  } catch (e) {
    DebugLog.mSaveWarning('folderPreview dynamic import failed: ' + (e as Error).message)
    return null
  }
}

async function fetchFolderItemsRaw(p: FolderPreviewParams): Promise<IAliGetFileModel[]> {
  const { user_id, drive_id, file_id, name, path } = p
  if (!user_id || !drive_id || !file_id) return []

  try {
    if (isPikPakUser(user_id) || drive_id === 'pikpak') {
      const mod = await tryDynamicImport(() => import('../pikpak/dirfilelist'))
      if (!mod) return []
      const parentId = file_id === 'pikpak_root' ? '' : file_id
      const resp = await mod.apiPikPakFileList(user_id, parentId, MAX_PREVIEW_FILES)
      const items = resp?.items || []
      return items.map((item: any) => {
        const mapped = mod.mapPikPakFileToAliModel(item, drive_id, parentId)
        ;(mapped as any).user_id = user_id
        return mapped
      })
    }
    if (isAliyunUser(user_id)) {
      const result = await AliDirFileList.ApiDirFileList(
        user_id,
        drive_id,
        file_id,
        name || '',
        'name asc',
        '',
        undefined,
        false
      )
      return (result?.items || []).slice(0, MAX_PREVIEW_FILES)
    }
  } catch (e) {
    DebugLog.mSaveWarning('folderPreview fetch failed: ' + (e as Error).message)
  }
  return []
}

export async function fetchFolderPreview(p: FolderPreviewParams): Promise<IAliGetFileModel[]> {
  const key = cacheKey(p)
  const now = Date.now()
  const cached = previewCache.get(key)
  if (cached) {
    if (cached.promise) return cached.promise
    if (now - cached.ts < CACHE_TTL_MS) return cached.items
  }
  const promise = fetchFolderItemsRaw(p).then((items) => {
    previewCache.set(key, { ts: Date.now(), items })
    return items
  }).catch((e) => {
    previewCache.delete(key)
    DebugLog.mSaveWarning('folderPreview promise rejected: ' + (e as Error).message)
    return [] as IAliGetFileModel[]
  })
  previewCache.set(key, { ts: now, items: [], promise })
  return promise
}

export function clearFolderPreviewCache(): void {
  previewCache.clear()
}

export const FOLDER_PREVIEW_MAX = MAX_PREVIEW_FILES
