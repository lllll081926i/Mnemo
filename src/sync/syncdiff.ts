/**
 * 同步差异计算（纯函数，不触碰 fs / 网络，方便单测）。
 *
 * 语义（按与用户确认的默认建议）：
 * - direction=upload：本地为准。本地新增/变化 → 上传；本地删除 → 默认不动云端（墓碑标记，不复活）；
 *   开启删除传播才删云端（受阈值保护）。
 * - direction=download：云端为准。云端新增/变化 → 下载；云端删除 → 默认不动本地；
 *   开启删除传播才删本地（受阈值保护）。本地删除 → 重新下载（镜像语义）。
 * - direction=both：增量合并。只在一侧出现 → 同步到另一侧；两侧都有但只有一侧变化 → 变化侧覆盖另一侧；
 *   两侧都变化 → 冲突，保留两份（旧版本改名加“冲突”后缀，原路径保留本地版本）；
 *   一侧删除 → 默认墓碑标记不复活，开启删除传播才删除另一侧（受阈值保护）。
 * - “变化”判定：本地用 size+mtimeMs 对比快照；云端 mtime 不可靠（多为上传时间），只用 size 对比。
 * - 首次同步两侧已存在同名文件：size 相同 → 直接登记（adopt）不传输；size 不同 → 按冲突处理。
 */
import { LocalFileEntry, RemoteFileEntry, SyncAction, SyncDirection, SyncFileEntry, baseName, conflictFileName } from './syncmodel'

export interface DiffOptions {
  direction: SyncDirection
  deletePropagation: boolean
  deleteThreshold: number
  now?: Date
}

export interface DiffResult {
  actions: SyncAction[]
  warnings: string[]
}

/** mtime 允许 2s 误差（FAT/部分网盘时间精度低） */
const MTIME_TOLERANCE_MS = 2000

const localChanged = (local: LocalFileEntry, snap: SyncFileEntry | undefined): boolean => {
  if (!snap) return true
  if (local.size !== snap.size) return true
  return Math.abs(local.mtimeMs - snap.mtimeMs) > MTIME_TOLERANCE_MS
}

const remoteChanged = (remote: RemoteFileEntry, snap: SyncFileEntry | undefined): boolean => {
  if (!snap || !snap.remote_file_id) return true
  return remote.size !== snap.remote_size
}

export function computeSyncPlan(
  local: Map<string, LocalFileEntry>,
  remote: Map<string, RemoteFileEntry>,
  snapshot: Map<string, SyncFileEntry>,
  options: DiffOptions
): DiffResult {
  const { direction, deletePropagation, deleteThreshold } = options
  const now = options.now || new Date()
  const actions: SyncAction[] = []
  const warnings: string[] = []
  const canUpload = direction === 'upload' || direction === 'both'
  const canDownload = direction === 'download' || direction === 'both'
  const relpaths = new Set<string>([...local.keys(), ...remote.keys(), ...snapshot.keys()])

  for (const relpath of relpaths) {
    const localItem = local.get(relpath)
    const remoteItem = remote.get(relpath)
    const snap = snapshot.get(relpath)

    if (localItem && remoteItem) {
      // 两侧都存在
      if (!snap) {
        if (localItem.size === remoteItem.size) {
          actions.push({ type: 'adopt', relpath })
        } else if (direction === 'upload') {
          actions.push({ type: 'upload', relpath, size: localItem.size })
        } else if (direction === 'download') {
          actions.push({ type: 'download', relpath, size: remoteItem.size })
        } else {
          actions.push({ type: 'conflict', relpath, size: localItem.size, conflictName: conflictFileName(baseName(relpath), '云端', now) })
        }
        continue
      }
      const lChanged = localChanged(localItem, snap)
      const rChanged = remoteChanged(remoteItem, snap)
      if (!lChanged && !rChanged) continue
      if (lChanged && !rChanged) {
        if (canUpload) actions.push({ type: 'upload', relpath, size: localItem.size })
      } else if (!lChanged && rChanged) {
        if (canDownload) actions.push({ type: 'download', relpath, size: remoteItem.size })
      } else {
        // 两侧都变了
        if (direction === 'upload') actions.push({ type: 'upload', relpath, size: localItem.size })
        else if (direction === 'download') actions.push({ type: 'download', relpath, size: remoteItem.size })
        else actions.push({ type: 'conflict', relpath, size: localItem.size, conflictName: conflictFileName(baseName(relpath), '云端', now) })
      }
      continue
    }

    if (localItem && !remoteItem) {
      // 只在本地存在
      if (!snap) {
        if (canUpload) actions.push({ type: 'upload', relpath, size: localItem.size })
        continue
      }
      // 云端此前被标记删除：本地没变就不复活；本地有变化则按变化重新上传
      if (snap.tombstone === 'remote_deleted') {
        if (localChanged(localItem, snap) && canUpload) actions.push({ type: 'upload', relpath, size: localItem.size })
        continue
      }
      // 上一轮云端还在，这一轮云端没了
      if (direction === 'upload') {
        actions.push({ type: 'upload', relpath, size: localItem.size }) // 上传模式镜像本地：云端被删就补传
      } else if (deletePropagation && (direction === 'both' || direction === 'download')) {
        actions.push({ type: 'delete_local', relpath })
      }
      // 其余情况：引擎写回 tombstone=remote_deleted，不动作
      continue
    }

    if (!localItem && remoteItem) {
      // 只在云端存在
      if (!snap) {
        if (canDownload) actions.push({ type: 'download', relpath, size: remoteItem.size })
        continue
      }
      // 本地此前被标记删除：云端没变就不复活
      if (snap.tombstone === 'local_deleted') continue
      // 上一轮本地还在，这一轮本地没了
      if (direction === 'download') {
        actions.push({ type: 'download', relpath, size: remoteItem.size }) // 下载模式镜像云端：本地被删就补下
      } else if (deletePropagation && (direction === 'both' || direction === 'upload')) {
        actions.push({ type: 'delete_remote', relpath })
      }
      // 其余情况：引擎写回 tombstone=local_deleted，不动作
      continue
    }
    // 两侧都不存在：由引擎清理快照条目
  }

  // 删除阈值保护：单轮删除超阈值则全部放弃删除并告警
  const deleteCount = actions.filter((a) => a.type === 'delete_remote' || a.type === 'delete_local').length
  if (deleteCount > 0 && deleteThreshold > 0 && deleteCount > deleteThreshold) {
    const kept = actions.filter((a) => a.type !== 'delete_remote' && a.type !== 'delete_local')
    warnings.push(`本轮共有 ${deleteCount} 个文件待删除，超过阈值 ${deleteThreshold}，已取消全部删除操作（请确认是否为误操作）`)
    return { actions: kept, warnings }
  }

  return { actions, warnings }
}
