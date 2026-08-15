// api.js — thin wrapper over the generated Wails bindings + runtime events.
import * as App from '../wailsjs/go/app/App'
import { EventsOn } from '../wailsjs/runtime/runtime'

// re-export the raw binding surface (used by views directly)
export * from '../wailsjs/go/app/App'

export const api = App

export function onEvent(name, cb) {
  return EventsOn(name, cb)
}

export { EventsOn }

// ---------- helpers ----------

export function listProviders() { return App.ListProviders() }
export function listAccounts() { return App.ListAccounts() }
export function login(provider, config) { return App.ProviderLogin(provider, config) }
export function saveMounted(provider, conn) { return App.SaveMountedAccount(provider, conn) }
export function removeAccount(userId) { return App.RemoveAccount(userId) }

export function listDir(userId, driveId, dirId) { return App.ListDir(userId, driveId, dirId) }
export function search(userId, driveId, kw) { return App.SearchFiles(userId, driveId, kw) }
export function listTrash(userId, driveId) { return App.ListTrash(userId, driveId) }
export function mkdir(userId, driveId, parentId, name) { return App.Mkdir(userId, driveId, parentId, name) }
export function rename(userId, driveId, fileId, name) { return App.RenameFile(userId, driveId, fileId, name) }
export function trash(userId, driveId, ids) { return App.TrashFiles(userId, driveId, ids) }
export function remove(userId, driveId, ids) { return App.DeleteFiles(userId, driveId, ids) }
export function restore(userId, driveId, ids) { return App.RestoreFiles(userId, driveId, ids) }
export function move(userId, driveId, ids, toParent) { return App.MoveFiles(userId, driveId, ids, toParent) }
export function copy(userId, driveId, ids, toParent) { return App.CopyFiles(userId, driveId, ids, toParent) }
export function favorite(userId, driveId, fav, ids) { return App.FavoriteFiles(userId, driveId, fav, ids) }
export function download(userId, driveId, file) { return App.DownloadFile(userId, driveId, file) }
export function downloadUrl(name, url, headers) { return App.DownloadURL(name, url, headers) }
export function createShare(userId, driveId, params) { return App.CreateShare(userId, driveId, params) }
export function uploadFiles(userId, driveId, parentId, paths) { return App.UploadFiles(userId, driveId, parentId, paths) }
export function migrateFiles(srcUser, srcDrive, dstUser, dstDrive, dstParent, fileIDs, move) {
  return App.MigrateFiles(srcUser, srcDrive, dstUser, dstDrive, dstParent, fileIDs, move)
}

/** 从 user_id 解析 provider：普通盘 `pikpak_xxx`，挂载存储 `webdav:xxx` / `s3:xxx`。 */
export function providerOf(userId) {
  const uid = String(userId || '')
  const ci = uid.indexOf(':')
  if (ci > 0 && (uid.startsWith('webdav:') || uid.startsWith('s3:'))) return uid.slice(0, ci)
  const ui = uid.indexOf('_')
  return ui > 0 ? uid.slice(0, ui) : ''
}

export function accountName(acc) {
  if (!acc) return ''
  const t = acc.token || {}
  return t.nick_name || t.user_name || t.name || acc.user_id
}

export function accountDetail(acc, providers) {
  if (!acc) return ''
  const pid = providerOf(acc.user_id)
  const p = (providers || []).find((x) => x.ID === pid)
  const label = p ? p.Meta.label : pid
  const t = acc.token || {}
  const id = t.user_name || t.user_id || acc.user_id
  const short = id.length > 12 ? '••••' + id.slice(-6) : id
  return `${label} · ${short}`
}

export function providerIconUrl(metaOrIcon) {
  const icon = typeof metaOrIcon === 'string' ? metaOrIcon : (metaOrIcon && metaOrIcon.icon) || ''
  const file = icon.replace(/^drive-icons\//, '')
  if (!file) return ''
  return new URL(`./assets/drive-icons/${file}`, import.meta.url).href
}

/** 账号对应的 provider 能力集（Capabilities，字段小写）。 */
export function capsOf(account, providers) {
  if (!account) return {}
  const pid = providerOf(account.user_id)
  const p = (providers || []).find((x) => x.ID === pid)
  return (p && p.Capabilities) || (p && p.capabilities) || {}
}

export function providerMetaOf(account, providers) {
  if (!account) return {}
  const pid = providerOf(account.user_id)
  const p = (providers || []).find((x) => x.ID === pid)
  return (p && p.Meta) || {}
}

export function formatBytes(n) {
  if (!n || n <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']
  let v = n, i = 0
  while (v >= 1024 && i < units.length - 1) { v /= 1024; i++ }
  return i === 0 ? `${n} B` : `${v.toFixed(1)} ${units[i]}`
}

export function formatSpeed(bps) {
  return formatBytes(bps) + '/s'
}

export function formatTime(ts) {
  if (!ts) return ''
  const d = new Date(ts * 1000)
  const p = (n) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`
}

/** 旧版 filetime 双行显示：{ date: '2024-01-02', clock: '15:04' } */
export function formatTimeParts(ts) {
  if (!ts) return { date: '', clock: '' }
  const d = new Date(ts * 1000)
  const p = (n) => String(n).padStart(2, '0')
  return { date: `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`, clock: `${p(d.getHours())}:${p(d.getMinutes())}` }
}

export function extOf(name) {
  const i = String(name || '').lastIndexOf('.')
  return i > 0 ? name.slice(i + 1).toLowerCase() : ''
}

/** 文件类型图标名（供 UiIcon 组件使用）。规范：UI 禁用 Emoji。 */
export function iconOf(file) {
  if (file.isDir) return 'folder'
  const cat = file.category || ''
  if (cat === 'video') return 'video'
  if (cat === 'audio') return 'audio'
  if (cat === 'image') return 'image'
  if (cat === 'archive') return 'archive'
  if (cat === 'doc' || cat === 'text') return 'doc'
  return 'file'
}

const PREVIEW_TEXT_EXTS = new Set(['txt', 'md', 'json', 'js', 'ts', 'vue', 'go', 'py', 'java', 'c', 'cpp', 'h', 'css', 'html', 'xml', 'yaml', 'yml', 'ini', 'log', 'sh', 'bat', 'srt', 'vtt', 'ass', 'ssa'])

/** 文件打开方式判定：video/audio → 播放，image/text/pdf → 预览，其余 → 下载。 */
export function openKindOf(file) {
  if (file.isDir) return 'dir'
  const cat = file.category || ''
  const ext = extOf(file.name)
  if (cat === 'video') return 'video'
  if (cat === 'audio') return 'audio'
  if (cat === 'image') return 'image'
  if (cat === 'text' || PREVIEW_TEXT_EXTS.has(ext)) return 'text'
  if (ext === 'pdf') return 'pdf'
  return 'download'
}

/** 复制文本到剪贴板（WebView2 兼容降级）。 */
export async function copyText(text) {
  try {
    await navigator.clipboard.writeText(text)
    return true
  } catch {
    const ta = document.createElement('textarea')
    ta.value = text
    ta.style.position = 'fixed'
    ta.style.opacity = '0'
    document.body.appendChild(ta)
    ta.select()
    let ok = false
    try { ok = document.execCommand('copy') } catch { ok = false }
    document.body.removeChild(ta)
    return ok
  }
}

// 打开外部浏览器（后端包装，避免直接依赖 runtime）
export function OpenBrowser(url) { return App.OpenBrowser(url) }
