/** 同步引擎：执行单任务同步 + 全局调度（启动同步 / 定时同步） */
import fsPromises from 'fs/promises'
import path from 'path'
import { computeSyncPlan } from './syncdiff'
import { loadSyncSnapshot, loadSyncTasks, saveSyncSnapshot, saveSyncTasks, scanLocalFolder } from './syncdal'
import { LocalFileEntry, RemoteFileEntry, SyncFileEntry, SyncTask, joinRelPath, parentRelPath } from './syncmodel'
import { createSyncRemoteAdapter, SyncRemoteAdapter } from './syncremote'
import useSyncStore from './syncstore'
import message from '../utils/message'

const runningTaskIds = new Set<string>()

const log = (taskId: string, level: 'info' | 'success' | 'warning' | 'error', text: string) => useSyncStore().mLog(taskId, level, text)

/** 应用快照：写入/更新某文件的同步记录 */
const writeSnapshotEntry = (snapshot: Map<string, SyncFileEntry>, relpath: string, local: LocalFileEntry | undefined, remote: { file_id: string, size: number, time: number }, clearTombstone = true) => {
  const existing = snapshot.get(relpath)
  snapshot.set(relpath, {
    size: local ? local.size : existing?.size || 0,
    mtimeMs: local ? local.mtimeMs : existing?.mtimeMs || 0,
    remote_file_id: remote.file_id,
    remote_size: remote.size,
    remote_time: remote.time,
    ...(clearTombstone ? {} : existing?.tombstone ? { tombstone: existing.tombstone } : {})
  })
}

const statLocal = async (localRoot: string, relpath: string): Promise<LocalFileEntry | undefined> => {
  try {
    const stat = await fsPromises.stat(path.join(localRoot, ...relpath.split('/')))
    return { relpath, size: stat.size, mtimeMs: Math.floor(stat.mtimeMs) }
  } catch {
    return undefined
  }
}

export const runSyncTask = async (task: SyncTask, manual = false): Promise<boolean> => {
  if (runningTaskIds.has(task.id)) {
    if (manual) message.info('该任务正在同步中')
    return false
  }
  if (!task.enabled) {
    if (manual) message.info('任务已停用，请先启用')
    return false
  }
  const syncStore = useSyncStore()
  runningTaskIds.add(task.id)
  syncStore.mSaveRunState(task.id, { running: true, phase: '准备中', currentFile: '', doneCount: 0, totalCount: 0, transferred: 0, transferredTotal: 0 })
  log(task.id, 'info', manual ? '手动触发同步' : '自动同步开始')

  let status: SyncTask['last_sync_status'] = 'ok'
  const errors: string[] = []
  let adapter: SyncRemoteAdapter | undefined

  try {
    // 1. 校验本地目录
    try {
      const stat = await fsPromises.stat(task.local_path)
      if (!stat.isDirectory()) throw new Error('not a directory')
    } catch {
      throw new Error(`本地文件夹不存在：${task.local_path}`)
    }

    adapter = createSyncRemoteAdapter(task)

    // 2. 扫描两侧 + 读取快照
    syncStore.mSaveRunState(task.id, { phase: '扫描本地文件夹' })
    const localFiles = await scanLocalFolder(task.local_path)
    syncStore.mSaveRunState(task.id, { phase: '获取网盘文件列表' })
    const remoteList = await adapter.listRecursive()
    const remoteFiles = new Map<string, RemoteFileEntry>(remoteList.map((item) => [item.relpath, item]))
    const snapshot = await loadSyncSnapshot(task.id)

    // 3. 计算同步计划
    const plan = computeSyncPlan(localFiles, remoteFiles, snapshot, {
      direction: task.direction,
      deletePropagation: task.delete_propagation,
      deleteThreshold: task.delete_threshold
    })
    for (const warning of plan.warnings) log(task.id, 'warning', warning)
    const totalCount = plan.actions.filter((a) => a.type !== 'adopt').length
    syncStore.mSaveRunState(task.id, { phase: '同步中', totalCount, doneCount: 0 })
    log(task.id, 'info', `本地 ${localFiles.size} 个文件，网盘 ${remoteFiles.size} 个文件，待处理 ${totalCount} 项`)

    // 4. 逐个执行
    let doneCount = 0
    let transferredTotal = 0
    for (const action of plan.actions) {
      const relpath = action.relpath
      try {
        if (action.type === 'adopt') {
          const local = localFiles.get(relpath)
          const remote = remoteFiles.get(relpath)
          if (local && remote) writeSnapshotEntry(snapshot, relpath, local, remote)
          continue
        }

        syncStore.mSaveRunState(task.id, { currentFile: relpath })

        if (action.type === 'upload') {
          const localAbs = path.join(task.local_path, ...relpath.split('/'))
          const result = await adapter.uploadFile(relpath, localAbs, action.size, (transferred) => {
            transferredTotal += transferred
            syncStore.mSaveRunState(task.id, { transferred: transferredTotal })
          })
          const local = await statLocal(task.local_path, relpath)
          writeSnapshotEntry(snapshot, relpath, local, { file_id: result.file_id, size: result.size, time: result.time })
          log(task.id, 'success', `已上传 ${relpath}`)
        } else if (action.type === 'download') {
          const remote = remoteFiles.get(relpath)
          if (!remote) throw new Error('网盘文件不存在')
          const localAbs = path.join(task.local_path, ...relpath.split('/'))
          await adapter.downloadFile(remote, localAbs, (transferred) => {
            transferredTotal += transferred
            syncStore.mSaveRunState(task.id, { transferred: transferredTotal })
          })
          const local = await statLocal(task.local_path, relpath)
          writeSnapshotEntry(snapshot, relpath, local, remote)
          log(task.id, 'success', `已下载 ${relpath}`)
        } else if (action.type === 'conflict') {
          // 保留两份：云端旧版本下载为“冲突”副本，原路径用本地版本覆盖云端
          const remote = remoteFiles.get(relpath)
          if (!remote) throw new Error('网盘文件不存在')
          const conflictRel = joinRelPath(parentRelPath(relpath), action.conflictName)
          const conflictAbs = path.join(task.local_path, ...conflictRel.split('/'))
          await adapter.downloadFile(remote, conflictAbs, () => {})
          const localAbs = path.join(task.local_path, ...relpath.split('/'))
          const result = await adapter.uploadFile(relpath, localAbs, action.size, () => {})
          const local = await statLocal(task.local_path, relpath)
          writeSnapshotEntry(snapshot, relpath, local, { file_id: result.file_id, size: result.size, time: result.time })
          log(task.id, 'warning', `冲突已保留两份：${relpath}（云端旧版本保存为 ${action.conflictName}）`)
        } else if (action.type === 'delete_remote') {
          const remote = remoteFiles.get(relpath)
          if (remote) await adapter.deleteFile(remote)
          snapshot.delete(relpath)
          log(task.id, 'warning', `已删除网盘文件 ${relpath}`)
        } else if (action.type === 'delete_local') {
          await fsPromises.rm(path.join(task.local_path, ...relpath.split('/')), { force: true })
          snapshot.delete(relpath)
          log(task.id, 'warning', `已删除本地文件 ${relpath}`)
        }
      } catch (error: any) {
        status = 'partial'
        const text = `${relpath}: ${error?.message || error}`
        errors.push(text)
        log(task.id, 'error', `失败 ${text}`)
      } finally {
        if (action.type !== 'adopt') {
          doneCount++
          syncStore.mSaveRunState(task.id, { doneCount })
        }
      }
    }

    // 5. 处理墓碑（删除传播关闭时标记，防止复活）与失效快照
    const relpaths = new Set<string>([...localFiles.keys(), ...remoteFiles.keys(), ...snapshot.keys()])
    for (const relpath of relpaths) {
      const hasLocal = localFiles.has(relpath)
      const hasRemote = remoteFiles.has(relpath)
      const entry = snapshot.get(relpath)
      if (!hasLocal && !hasRemote) {
        snapshot.delete(relpath)
        continue
      }
      if (hasLocal && hasRemote) {
        if (entry?.tombstone) writeSnapshotEntry(snapshot, relpath, localFiles.get(relpath), remoteFiles.get(relpath)!)
        continue
      }
      if (!entry) continue
      if (!hasLocal && hasRemote && !entry.tombstone && task.direction !== 'download') {
        snapshot.set(relpath, { ...entry, tombstone: 'local_deleted' })
      } else if (hasLocal && !hasRemote && !entry.tombstone && task.direction !== 'upload') {
        snapshot.set(relpath, { ...entry, tombstone: 'remote_deleted' })
      }
    }

    await saveSyncSnapshot(task.id, snapshot)

    if (status === 'ok') log(task.id, 'success', `同步完成，共处理 ${totalCount} 项`)
    else log(task.id, 'warning', `同步完成但有 ${errors.length} 项失败，下轮将自动重试`)

    syncStore.mUpdateTask(task.id, {
      last_sync_at: Date.now(),
      last_sync_status: status,
      last_sync_message: status === 'ok' ? `成功，处理 ${totalCount} 项` : `${errors.length} 项失败：${errors[0] || ''}`
    })
    await persistTasks()
    return status === 'ok'
  } catch (error: any) {
    const text = error?.message || String(error)
    log(task.id, 'error', `同步失败：${text}`)
    syncStore.mUpdateTask(task.id, { last_sync_at: Date.now(), last_sync_status: 'error', last_sync_message: text })
    await persistTasks()
    if (manual) message.error(`同步失败：${text}`)
    return false
  } finally {
    runningTaskIds.delete(task.id)
    syncStore.mSaveRunState(task.id, { running: false, phase: '', currentFile: '' })
  }
}

export const persistTasks = async () => {
  await saveSyncTasks(useSyncStore().tasks)
}

export const refreshSyncTasks = async () => {
  const tasks = await loadSyncTasks()
  useSyncStore().mSaveTasks(tasks)
}

/** 全局调度：每 30s 检查一次到期任务；应用启动时执行标记了“启动时同步”的任务 */
let schedulerTimer: ReturnType<typeof setInterval> | undefined
let launchSyncDone = false

export const startSyncScheduler = () => {
  if (schedulerTimer) return
  void refreshSyncTasks().then(() => {
    if (launchSyncDone) return
    launchSyncDone = true
    const tasks = useSyncStore().tasks.filter((task) => task.enabled && task.sync_on_launch)
    for (const task of tasks) void runSyncTask(task)
  })
  schedulerTimer = setInterval(() => {
    const now = Date.now()
    for (const task of useSyncStore().tasks) {
      if (!task.enabled || !task.interval_min || task.interval_min <= 0) continue
      if (runningTaskIds.has(task.id)) continue
      const due = (task.last_sync_at || 0) + task.interval_min * 60 * 1000
      if (now >= due) void runSyncTask(task)
    }
  }, 30 * 1000)
}
