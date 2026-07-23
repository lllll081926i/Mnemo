import { getDriveProviderLabel, type DriveProvider } from '../utils/driveProvider'

export interface ShareHistoryItem {
  /** 本地记录 id */
  id: string
  provider: DriveProvider | string
  user_id: string
  /** 账号名（邮箱/昵称） */
  account: string
  share_id: string
  share_url: string
  pass_code: string
  /** 分享标题（文件名列表） */
  share_name: string
  file_count: number
  /** ISO 时间；空串表示永久有效 */
  expiration: string
  created_at: number
}

const STORAGE_KEY = 'mnemo:share:history'
const MAX_HISTORY = 500

const readAll = (): ShareHistoryItem[] => {
  try {
    if (typeof localStorage === 'undefined') return []
    const raw = JSON.parse(localStorage.getItem(STORAGE_KEY) || '[]')
    if (!Array.isArray(raw)) return []
    return raw.filter((item) => item && item.share_url)
  } catch {
    return []
  }
}

const writeAll = (list: ShareHistoryItem[]) => {
  try {
    if (typeof localStorage === 'undefined') return
    localStorage.setItem(STORAGE_KEY, JSON.stringify(list.slice(0, MAX_HISTORY)))
  } catch {
    // 存储不可用时历史记录仅保留在本次会话内
  }
}

export const listShareHistory = (): ShareHistoryItem[] => readAll().sort((a, b) => (b.created_at || 0) - (a.created_at || 0))

export const recordShareHistory = (item: Omit<ShareHistoryItem, 'id' | 'created_at'>): ShareHistoryItem => {
  const record: ShareHistoryItem = { ...item, id: `${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 8)}`, created_at: Date.now() }
  const list = readAll()
  // 同一分享链接只保留一条（重复创建时更新为最新）
  const next = [record, ...list.filter((entry) => entry.share_url !== record.share_url)]
  writeAll(next)
  return record
}

export const removeShareHistory = (id: string) => {
  writeAll(readAll().filter((item) => item.id !== id))
}

export const clearShareHistory = () => writeAll([])

export const isShareHistoryActive = (item: ShareHistoryItem): boolean => {
  if (!item.expiration) return true
  const expireAt = new Date(item.expiration).getTime()
  if (Number.isNaN(expireAt)) return true
  return expireAt > Date.now()
}

export const shareHistoryProviderLabel = (provider: string): string => {
  try {
    return getDriveProviderLabel(provider as DriveProvider)
  } catch {
    return provider
  }
}
