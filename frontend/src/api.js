// api.js — thin wrapper over the generated Wails bindings + runtime events.
import * as App from '../wailsjs/go/app/App'
import { EventsOn } from '../wailsjs/runtime/runtime'

// re-export the raw binding surface (used by views directly)
export * from '../wailsjs/go/app/App'

export const api = App

export function onEvent(name, cb) {
  EventsOn(name, cb)
}

export { EventsOn }

// helpers for common tasks
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

export function formatBytes(n) {
  if (!n || n <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']
  let v = n, i = 0
  while (v >= 1024 && i < units.length - 1) { v /= 1024; i++ }
  return i === 0 ? `${n} B` : `${v.toFixed(1)} ${units[i]}`
}

export function formatTime(ts) {
  if (!ts) return ''
  const d = new Date(ts * 1000)
  const p = (n) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`
}

export function extOf(name) {
  const i = String(name || '').lastIndexOf('.')
  return i > 0 ? name.slice(i + 1).toLowerCase() : ''
}

export function iconOf(file) {
  if (file.isDir) return '📁'
  const cat = file.category || ''
  if (cat === 'video') return '🎬'
  if (cat === 'audio') return '🎵'
  if (cat === 'image') return '🖼️'
  if (cat === 'archive') return '🗜️'
  if (cat === 'doc' || cat === 'text') return '📄'
  return '📎'
}