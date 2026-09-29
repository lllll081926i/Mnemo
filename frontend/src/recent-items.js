import { ref } from 'vue'

const KEY = 'mnemo.recentItems.v1'
const ENABLED_KEY = 'mnemo.recentItems.enabled'
const MAX_AGE = 30 * 24 * 60 * 60 * 1000
const MAX_ITEMS = 200

function valid(item) {
  return item && typeof item.userId === 'string' && typeof item.driveId === 'string' && typeof item.fileId === 'string' && typeof item.name === 'string' && typeof item.at === 'number' && item.at > Date.now() - MAX_AGE
}

function normalize(item) {
  return {
    userId: item.userId, driveId: item.driveId, fileId: item.fileId,
    dirId: item.dirId || '', name: item.name, isDir: !!item.isDir,
    path: Array.isArray(item.path) ? item.path.filter(part => part && typeof part.id === 'string' && typeof part.name === 'string').map(part => ({ id: part.id, name: part.name })) : [],
    at: item.at,
  }
}

function readRecentItems() {
  try {
    const saved = JSON.parse(localStorage.getItem(KEY) || '[]')
    return Array.isArray(saved) ? saved.filter(valid).slice(0, MAX_ITEMS).map(normalize) : []
  } catch { return [] }
}

function readEnabled() {
  try { return localStorage.getItem(ENABLED_KEY) !== 'false' } catch { return true }
}

export const recentEnabled = ref(readEnabled())
export const recentItems = ref(readRecentItems())

export function recordRecentItem(item) {
  if (!recentEnabled.value || !item?.userId || !item?.driveId || !item?.fileId || !item?.name) return
  const next = normalize({ ...item, at: Date.now() })
  recentItems.value = [next, ...recentItems.value.filter(entry => !(entry.userId === next.userId && entry.driveId === next.driveId && entry.fileId === next.fileId))]
    .filter(valid).slice(0, MAX_ITEMS)
  try { localStorage.setItem(KEY, JSON.stringify(recentItems.value)) } catch { /* 本地存储不可用时仅保留本次会话 */ }
}

export function clearRecentItems() {
  recentItems.value = []
  try { localStorage.removeItem(KEY) } catch { /* 无法写入时仍清除本次会话 */ }
}

export function setRecentEnabled(enabled) {
  recentEnabled.value = !!enabled
  try { localStorage.setItem(ENABLED_KEY, String(recentEnabled.value)) } catch { /* 无法写入时仅本次会话生效 */ }
}
