/** 同步任务/快照的持久化，以及本地文件夹扫描 */
import fsPromises from 'fs/promises'
import path from 'path'
import DB from '../utils/db'
import { LocalFileEntry, SyncFileEntry, SyncTask, isSyncExcludedName, joinRelPath } from './syncmodel'

const TASKS_KEY = 'SyncTasks'
const snapshotKey = (taskId: string) => `SyncSnapshot_${taskId}`

export const loadSyncTasks = async (): Promise<SyncTask[]> => {
  const raw = (await DB.getValueObject(TASKS_KEY).catch(() => undefined)) as SyncTask[] | undefined
  if (!Array.isArray(raw)) return []
  return raw.filter((item) => item && item.id && item.local_path && item.remote_dir_id)
}

export const saveSyncTasks = async (tasks: SyncTask[]): Promise<void> => {
  await DB.saveValueObject(TASKS_KEY, JSON.parse(JSON.stringify(tasks)))
}

export const loadSyncSnapshot = async (taskId: string): Promise<Map<string, SyncFileEntry>> => {
  const raw = (await DB.getValueObject(snapshotKey(taskId)).catch(() => undefined)) as Record<string, SyncFileEntry> | undefined
  return new Map(Object.entries(raw || {}))
}

export const saveSyncSnapshot = async (taskId: string, snapshot: Map<string, SyncFileEntry>): Promise<void> => {
  await DB.saveValueObject(snapshotKey(taskId), JSON.parse(JSON.stringify(Object.fromEntries(snapshot))))
}

export const deleteSyncTaskData = async (taskId: string): Promise<void> => {
  await DB.deleteValueObject(snapshotKey(taskId)).catch(() => undefined)
}

/** 递归扫描本地文件夹（跳过黑名单与隐藏项），返回 相对路径 → 文件信息 */
export const scanLocalFolder = async (rootPath: string): Promise<Map<string, LocalFileEntry>> => {
  const result = new Map<string, LocalFileEntry>()
  const walk = async (absDir: string, relDir: string): Promise<void> => {
    let entries
    try {
      entries = await fsPromises.readdir(absDir, { withFileTypes: true })
    } catch {
      return
    }
    for (const entry of entries) {
      if (entry.name.startsWith('.') || isSyncExcludedName(entry.name)) continue
      const absPath = path.join(absDir, entry.name)
      const relpath = joinRelPath(relDir, entry.name)
      if (entry.isDirectory()) {
        await walk(absPath, relpath)
      } else if (entry.isFile()) {
        try {
          const stat = await fsPromises.stat(absPath)
          result.set(relpath, { relpath, size: stat.size, mtimeMs: Math.floor(stat.mtimeMs) })
        } catch {
          // 文件可能在扫描期间被移动/删除，跳过
        }
      }
    }
  }
  await walk(rootPath, '')
  return result
}

/** 任务冲突校验：同一账号下，本地目录或网盘目录不允许重叠（防止两个任务互相打架） */
export const validateSyncTaskOverlap = (tasks: SyncTask[], candidate: SyncTask): string => {
  const normLocal = (value: string) => value.replace(/\\/g, '/').replace(/\/+$/, '').toLowerCase()
  const candidateLocal = normLocal(candidate.local_path)
  for (const task of tasks) {
    if (task.id === candidate.id) continue
    if (task.user_id !== candidate.user_id) continue
    const taskLocal = normLocal(task.local_path)
    if (candidateLocal === taskLocal || candidateLocal.startsWith(`${taskLocal}/`) || taskLocal.startsWith(`${candidateLocal}/`)) {
      return `本地文件夹与任务“${task.name}”的本地文件夹重叠`
    }
    if (task.drive_id === candidate.drive_id) {
      const a = candidate.remote_dir_id.replace(/\/+$/, '')
      const b = task.remote_dir_id.replace(/\/+$/, '')
      if (a === b || a.startsWith(`${b}/`) || b.startsWith(`${a}/`)) {
        return `网盘文件夹与任务“${task.name}”的网盘文件夹重叠`
      }
    }
  }
  return ''
}
