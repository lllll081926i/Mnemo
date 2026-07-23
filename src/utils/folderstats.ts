import { ref } from 'vue'
import { apiPikPakFileList } from '../pikpak/dirfilelist'
import { apiOneDriveFileList } from '../onedrive/dirfilelist'
import { apiGoogleDriveFileList } from '../gdrive/dirfilelist'
import { apiDropboxFileList } from '../dropbox/dirfilelist'
import { apiGofileFileList } from '../gofile/dirfilelist'
import { getWebDavConnection, getWebDavConnectionId, isWebDavDrive, listWebDavDirectory } from './webdavClient'
import { getS3Connection, getS3ConnectionId, isS3Drive, listS3Directory } from './s3Client'
import { resolveDriveProvider } from './driveProvider'
import UserDAL from '../user/userdal'

export interface FolderStatsResult {
  size: number
  folder_count: number
  file_count: number
  reach_limit?: boolean
}

interface FolderStatsCacheEntry extends FolderStatsResult {
  /** 直接子项 id+size 的签名：移动/删除/新增会变，重命名不会变 */
  signature: string
  updatedAt: number
}

const STORAGE_KEY = 'mnemo:folder-stats'
const MAX_CACHE_ENTRIES = 200
const MAX_FOLDERS = 100
const MAX_ITEMS = 2000
// 静默缓慢统计：每次列目录请求之间停顿一下，避免打爆网盘接口
const PACE_MS = 150

/** 完成时自增，UI 监听它来刷新显示 */
export const folderStatsVersion = ref(0)

const statsKey = (userId: string, driveId: string, fileId: string) => `${userId}|${driveId}|${fileId}`

const readCache = (): Record<string, FolderStatsCacheEntry> => {
  try {
    if (typeof localStorage === 'undefined') return {}
    const raw = JSON.parse(localStorage.getItem(STORAGE_KEY) || '{}')
    return raw && typeof raw === 'object' ? raw : {}
  } catch {
    return {}
  }
}

const writeCache = (cache: Record<string, FolderStatsCacheEntry>) => {
  try {
    if (typeof localStorage === 'undefined') return
    const entries = Object.entries(cache).sort((a, b) => (b[1].updatedAt || 0) - (a[1].updatedAt || 0)).slice(0, MAX_CACHE_ENTRIES)
    localStorage.setItem(STORAGE_KEY, JSON.stringify(Object.fromEntries(entries)))
  } catch {
    // 缓存不可用时只保留内存结果
  }
}

export const getCachedFolderStats = (userId: string, driveId: string, fileId: string): FolderStatsResult | undefined => {
  const entry = readCache()[statsKey(userId, driveId, fileId)]
  if (!entry) return undefined
  return { size: entry.size, folder_count: entry.folder_count, file_count: entry.file_count, reach_limit: entry.reach_limit }
}

const resolveProvider = async (userId: string, driveId: string) => {
  const token = UserDAL.GetUserToken(userId) || (await UserDAL.GetUserTokenFromDB(userId))
  return resolveDriveProvider({ tokenfrom: token?.tokenfrom, userId, driveId })
}

type ChildItem = { id: string; isDir: boolean; size: number }

const listChildren = async (provider: string, userId: string, driveId: string, folderId: string): Promise<ChildItem[]> => {
  if (provider === 'pikpak') {
    const out: ChildItem[] = []
    let pageToken = ''
    do {
      const page = await apiPikPakFileList(userId, folderId, 100, pageToken)
      out.push(...page.items.map((item) => ({ id: String(item.id || ''), isDir: (item.kind || '').includes('folder'), size: Number(item.size || 0) })))
      pageToken = page.nextPageToken
    } while (pageToken && out.length < 500)
    return out
  }
  if (provider === 'onedrive') {
    const list = await apiOneDriveFileList(userId, folderId)
    return list.map((item) => ({ id: item.id || '', isDir: !!item.folder, size: Number(item.size || 0) }))
  }
  if (provider === 'gdrive') {
    const list = await apiGoogleDriveFileList(userId, folderId)
    return list.map((item) => ({ id: item.id || '', isDir: item.mimeType === 'application/vnd.google-apps.folder', size: Number(item.size || 0) }))
  }
  if (provider === 'dropbox') {
    const list = await apiDropboxFileList(userId, folderId)
    return list.map((item) => ({ id: item.id || '', isDir: item['.tag'] === 'folder', size: Number((item as any).size || 0) }))
  }
  if (provider === 'gofile') {
    const list = await apiGofileFileList(userId, folderId)
    return list.map((item) => ({ id: item.id || '', isDir: item.type === 'folder', size: Number(item.size || 0) }))
  }
  if (isWebDavDrive(driveId)) {
    const connection = getWebDavConnection(getWebDavConnectionId(driveId))
    if (!connection) return []
    const list = await listWebDavDirectory(connection, folderId)
    return list.map((item) => ({ id: item.file_id, isDir: item.isDir, size: Number(item.size || 0) }))
  }
  if (isS3Drive(driveId)) {
    const connection = getS3Connection(getS3ConnectionId(driveId))
    if (!connection) return []
    const list = await listS3Directory(connection, folderId)
    return list.map((item) => ({ id: item.file_id, isDir: item.isDir, size: Number(item.size || 0) }))
  }
  return []
}

// 只依赖直接子项的 id 和大小：改名不影响签名，增/删/移动/大小变化都会改变签名
const directSignature = (children: ChildItem[]): string => {
  let hash = 0
  for (const child of children) {
    const text = `${child.id}:${child.isDir ? 1 : 0}:${child.size};`
    for (let i = 0; i < text.length; i++) hash = (hash * 31 + text.charCodeAt(i)) | 0
  }
  return `${children.length}:${hash}`
}

const sleep = (ms: number) => new Promise<void>((resolve) => setTimeout(resolve, ms))

const computeFolderStats = async (provider: string, userId: string, driveId: string, fileId: string, paced: boolean): Promise<FolderStatsResult & { signature: string }> => {
  const result: FolderStatsResult = { size: 0, folder_count: 0, file_count: 0, reach_limit: false }
  let signature = ''
  let visited = 0
  const queue: string[] = [fileId]
  while (queue.length) {
    if (result.folder_count >= MAX_FOLDERS || visited >= MAX_ITEMS) {
      result.reach_limit = true
      break
    }
    const folderId = queue.shift()!
    const children = await listChildren(provider, userId, driveId, folderId)
    if (folderId === fileId) signature = directSignature(children)
    for (const child of children) {
      visited++
      if (child.isDir) {
        result.folder_count++
        if (result.folder_count < MAX_FOLDERS) queue.push(child.id)
        else result.reach_limit = true
      } else {
        result.file_count++
        result.size += child.size
      }
      if (visited >= MAX_ITEMS) {
        result.reach_limit = true
        break
      }
    }
    if (paced && queue.length) await sleep(PACE_MS)
  }
  return { ...result, signature }
}

// 同一文件夹只跑一个后台统计任务
const running = new Map<string, Promise<void>>()

/**
 * 请求文件夹统计：先查缓存并核对直接子项签名；
 * 签名一致直接返回缓存，否则后台静默（缓慢）重算并写回缓存。
 */
export const requestFolderStats = async (userId: string, driveId: string, fileId: string): Promise<FolderStatsResult | undefined> => {
  const key = statsKey(userId, driveId, fileId)
  const inFlight = running.get(key)
  if (inFlight) return undefined
  const provider = await resolveProvider(userId, driveId)
  if (provider === 'unknown') return undefined
  const task = (async () => {
    try {
      const cache = readCache()
      const cached = cache[key]
      // 先拿直接子项算签名，判断是否真的需要全量递归
      const directChildren = await listChildren(provider, userId, driveId, fileId)
      const signature = directSignature(directChildren)
      if (cached && cached.signature === signature) {
        cached.updatedAt = Date.now()
        writeCache(cache)
        folderStatsVersion.value++
        return
      }
      const result = await computeFolderStats(provider, userId, driveId, fileId, true)
      cache[key] = { ...result, signature: result.signature || signature, updatedAt: Date.now() }
      writeCache(cache)
      folderStatsVersion.value++
    } catch {
      // 静默失败：下次打开属性时再重试
    } finally {
      running.delete(key)
    }
  })()
  running.set(key, task)
  return undefined
}

/** 立即全量统计（不同步限速），供需要等待结果的场景使用 */
export const computeFolderStatsNow = async (userId: string, driveId: string, fileId: string): Promise<FolderStatsResult | undefined> => {
  const provider = await resolveProvider(userId, driveId)
  if (provider === 'unknown') return undefined
  try {
    const result = await computeFolderStats(provider, userId, driveId, fileId, false)
    const cache = readCache()
    cache[statsKey(userId, driveId, fileId)] = { ...result, signature: result.signature, updatedAt: Date.now() }
    writeCache(cache)
    return result
  } catch {
    return undefined
  }
}
