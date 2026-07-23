/** 同步功能的数据模型与纯工具函数 */

export type SyncDirection = 'upload' | 'download' | 'both'

export interface SyncTask {
  id: string
  name: string
  /** 绑定的网盘账号（换账号后任务置灰不可用） */
  user_id: string
  drive_id: string
  /** 网盘侧同步根目录 */
  remote_dir_id: string
  remote_dir_name: string
  /** 本地侧同步根目录（绝对路径） */
  local_path: string
  direction: SyncDirection
  /** 定时同步间隔（分钟），0 = 不定时 */
  interval_min: number
  /** 应用启动时自动同步一次 */
  sync_on_launch: boolean
  /** 删除传播：开启后一边删除的文件另一边也删除（默认关闭，永不删除） */
  delete_propagation: boolean
  /** 单轮删除数量超过该阈值时放弃删除并告警（防误清空） */
  delete_threshold: number
  enabled: boolean
  created_at: number
  last_sync_at: number
  last_sync_status: 'ok' | 'error' | 'partial' | ''
  last_sync_message: string
}

/** 本地文件快照项 */
export interface LocalFileEntry {
  relpath: string
  size: number
  mtimeMs: number
}

/** 网盘文件快照项 */
export interface RemoteFileEntry {
  relpath: string
  file_id: string
  name: string
  size: number
  time: number
  isDir: boolean
}

/** 上一轮同步完成后落库的快照 */
export interface SyncFileEntry {
  size: number
  mtimeMs: number
  remote_file_id: string
  remote_size: number
  remote_time: number
  /** 删除传播关闭时，标记哪一侧被用户主动删除过（防止下轮同步把文件“复活”） */
  tombstone?: 'local_deleted' | 'remote_deleted'
}

export type SyncAction =
  | { type: 'upload', relpath: string, size: number }
  | { type: 'download', relpath: string, size: number }
  | { type: 'delete_remote', relpath: string }
  | { type: 'delete_local', relpath: string }
  | { type: 'conflict', relpath: string, size: number, conflictName: string }
  | { type: 'adopt', relpath: string }

export const SYNC_DIRECTION_LABEL: Record<SyncDirection, string> = {
  upload: '仅上传到网盘',
  download: '仅下载到本地',
  both: '双向同步'
}

export const newSyncTaskId = () => `sync_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 8)}`

export const defaultSyncTask = (): Omit<SyncTask, 'id' | 'user_id' | 'drive_id' | 'remote_dir_id' | 'remote_dir_name' | 'local_path' | 'name'> => ({
  direction: 'both',
  interval_min: 0,
  sync_on_launch: false,
  delete_propagation: false,
  delete_threshold: 20,
  enabled: true,
  created_at: Date.now(),
  last_sync_at: 0,
  last_sync_status: '',
  last_sync_message: ''
})

/** 同步黑名单：系统/临时文件永远不参与同步 */
const EXCLUDE_EXACT = new Set(['desktop.ini', 'thumbs.db', '.ds_store', 'icon\r'])
const EXCLUDE_EXT = /\.(tmp|temp|part|crdownload|download|aria2|bak|syncpart|~)$/i

export const isSyncExcludedName = (name: string): boolean => {
  if (!name) return true
  const lower = name.toLowerCase()
  if (EXCLUDE_EXACT.has(lower)) return true
  if (name.startsWith('~$')) return true
  if (EXCLUDE_EXT.test(lower)) return true
  return false
}

/** 冲突时保留两份：为“旧版本”生成带后缀的文件名 */
export const conflictFileName = (name: string, sideTag: string, now: Date): string => {
  const pad = (n: number) => String(n).padStart(2, '0')
  const stamp = `${now.getFullYear()}${pad(now.getMonth() + 1)}${pad(now.getDate())}-${pad(now.getHours())}${pad(now.getMinutes())}`
  const suffix = ` (冲突-${sideTag} ${stamp})`
  const dot = name.lastIndexOf('.')
  if (dot > 0) return `${name.slice(0, dot)}${suffix}${name.slice(dot)}`
  return `${name}${suffix}`
}

export const joinRelPath = (...parts: string[]) => parts.filter(Boolean).join('/').replace(/\/+/g, '/')

export const parentRelPath = (relpath: string) => {
  const idx = relpath.lastIndexOf('/')
  return idx < 0 ? '' : relpath.slice(0, idx)
}

export const baseName = (relpath: string) => {
  const idx = relpath.lastIndexOf('/')
  return idx < 0 ? relpath : relpath.slice(idx + 1)
}
