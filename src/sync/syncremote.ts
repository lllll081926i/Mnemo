/** 同步引擎的网盘侧适配器：PikPak / WebDAV / S3 */
import fsPromises from 'fs/promises'
import { createWriteStream } from 'fs'
import path from 'path'
import { Readable } from 'stream'
import { pipeline } from 'stream/promises'
import { apiPikPakDownloadInfo, apiPikPakFileList } from '../pikpak/dirfilelist'
import { apiPikPakMkdir, apiPikPakTrashBatch } from '../pikpak/filecmd'
import PikPakUploadDisk from '../pikpak/upload'
import type { IUploadingUI } from '../utils/dbupload'
import { buildWebDavDownloadUrl, createWebDavDirectory, deleteWebDavPath, getWebDavConnection, getWebDavConnectionId, getWebDavRequestHeaders, isWebDavDrive, listWebDavRecursive, uploadWebDavFile } from '../utils/webdavClient'
import { deleteS3File, getS3Connection, getS3ConnectionId, getS3DownloadUrl, isS3Drive, listS3Recursive, uploadS3File } from '../utils/s3Client'
import { resolveDriveProvider } from '../utils/driveProvider'
import { RemoteFileEntry, SyncTask, baseName, joinRelPath, parentRelPath } from './syncmodel'

export interface SyncRemoteAdapter {
  /** 递归列出网盘同步根目录下的全部文件 */
  listRecursive(): Promise<RemoteFileEntry[]>
  /** 确保某个相对目录存在，返回其网盘 file_id（S3 返回相对路径本身） */
  ensureDir(relpath: string): Promise<string>
  uploadFile(relpath: string, localAbsPath: string, size: number, onProgress: (transferred: number) => void): Promise<{ file_id: string, size: number, time: number }>
  downloadFile(entry: RemoteFileEntry, localAbsPath: string, onProgress: (transferred: number) => void): Promise<void>
  deleteFile(entry: RemoteFileEntry): Promise<void>
}

/** 下载到临时文件再改名，避免中断留下半个文件被当成已同步 */
const downloadToFile = async (url: string, headers: Record<string, string>, localAbsPath: string, onProgress: (transferred: number) => void): Promise<void> => {
  const resp = await fetch(url, { headers })
  if (!resp.ok || !resp.body) throw new Error(`下载失败 HTTP ${resp.status}`)
  await fsPromises.mkdir(path.dirname(localAbsPath), { recursive: true })
  const tmpPath = `${localAbsPath}.syncpart`
  const reader = resp.body.getReader()
  const nodeStream = new Readable({
    async read() {
      try {
        const { done, value } = await reader.read()
        if (done) this.push(null)
        else {
          onProgress(value?.byteLength || 0)
          this.push(Buffer.from(value))
        }
      } catch (error) {
        this.destroy(error as Error)
      }
    }
  })
  await pipeline(nodeStream, createWriteStream(tmpPath))
  await fsPromises.rename(tmpPath, localAbsPath)
}

const pikpakAdapter = (task: SyncTask): SyncRemoteAdapter => {
  const { user_id, drive_id, remote_dir_id } = task

  const listDirAll = async (dirId: string): Promise<{ id: string, name: string, isDir: boolean, size: number, time: number }[]> => {
    const items: { id: string, name: string, isDir: boolean, size: number, time: number }[] = []
    let pageToken = ''
    for (let page = 0; page < 200; page++) {
      const { items: list, nextPageToken } = await apiPikPakFileList(user_id, dirId, 100, pageToken)
      for (const item of list) {
        items.push({
          id: String(item.id || ''),
          name: item.name || '',
          isDir: (item.kind || '').includes('folder'),
          size: Number(item.size || 0),
          time: new Date(item.modified_time || item.created_time || '').getTime() || 0
        })
      }
      if (!nextPageToken) break
      pageToken = nextPageToken
    }
    return items
  }

  return {
    async listRecursive() {
      const result: RemoteFileEntry[] = []
      const walk = async (dirId: string, relDir: string) => {
        const items = await listDirAll(dirId)
        for (const item of items) {
          const relpath = joinRelPath(relDir, item.name)
          if (item.isDir) await walk(item.id, relpath)
          else result.push({ relpath, file_id: item.id, name: item.name, size: item.size, time: item.time, isDir: false })
        }
      }
      await walk(remote_dir_id, '')
      return result
    },

    async ensureDir(relpath: string) {
      if (!relpath) return remote_dir_id
      let currentId = remote_dir_id
      for (const segment of relpath.split('/').filter(Boolean)) {
        const items = await listDirAll(currentId)
        const found = items.find((item) => item.isDir && item.name === segment)
        if (found) {
          currentId = found.id
        } else {
          const created = await apiPikPakMkdir(user_id, currentId, segment)
          if (!created.file_id) throw new Error(created.error || `创建网盘文件夹失败：${segment}`)
          currentId = created.file_id
        }
      }
      return currentId
    },

    async uploadFile(relpath, localAbsPath, size, onProgress) {
      const parentId = await this.ensureDir(parentRelPath(relpath))
      const taskId = Date.now()
      const fileui: IUploadingUI = {
        IsRunning: true,
        TaskID: taskId,
        UploadID: taskId + 1,
        user_id,
        parent_file_id: parentId,
        drive_id,
        check_name_mode: 'overwrite',
        localFilePath: path.dirname(localAbsPath),
        encType: '',
        File: {
          TaskID: taskId,
          UploadID: taskId + 1,
          partPath: path.basename(localAbsPath),
          name: baseName(relpath),
          size,
          sizeStr: '',
          mtime: 0,
          isDir: false,
          IsRoot: true,
          uploaded_is_rapid: false,
          uploaded_file_id: ''
        },
        Info: {
          UploadID: Date.now() + 1,
          uploadState: 'running',
          up_upload_id: '',
          up_file_id: '',
          uploadSize: 0,
          fileSize: size,
          Speed: 0,
          speedStr: '',
          Progress: 0,
          failedCode: 0,
          failedMessage: '',
          autoTryTime: 0,
          autoTryCount: 0
        }
      }
      const result = await PikPakUploadDisk.UploadOneFile(fileui)
      if (result !== 'success') throw new Error(result || '上传失败')
      onProgress(size)
      return { file_id: fileui.File.uploaded_file_id, size, time: Date.now() }
    },

    async downloadFile(entry, localAbsPath, onProgress) {
      const info = await apiPikPakDownloadInfo(user_id, entry.file_id)
      const url = info.streamUrl || info.downloadUrl
      if (!url) throw new Error(info.error || '获取 PikPak 下载地址失败')
      await downloadToFile(url, {}, localAbsPath, onProgress)
    },

    async deleteFile(entry) {
      const deleted = await apiPikPakTrashBatch(user_id, [entry.file_id])
      if (!deleted.includes(entry.file_id)) throw new Error('网盘文件删除失败')
    }
  }
}

const webdavAdapter = (task: SyncTask): SyncRemoteAdapter => {
  const connection = getWebDavConnection(getWebDavConnectionId(task.drive_id))
  if (!connection) throw new Error('WebDAV 连接不存在，请重新连接')
  const root = task.remote_dir_id === '/' ? '/' : task.remote_dir_id
  return {
    async listRecursive() {
      const items = await listWebDavRecursive(connection, root)
      const prefix = root === '/' ? '/' : `${root.replace(/\/+$/, '')}/`
      return items
        .map((item) => ({
          relpath: item.file_id.startsWith(prefix) ? item.file_id.slice(prefix.length) : item.file_id.replace(/^\//, ''),
          file_id: item.file_id,
          name: item.name,
          size: item.size,
          time: item.time,
          isDir: false
        }))
        .filter((item) => !!item.relpath)
    },

    async ensureDir(relpath: string) {
      const target = joinRelPath(root === '/' ? '' : root, relpath) || '/'
      await createWebDavDirectory(connection, target)
      return target
    },

    async uploadFile(relpath, localAbsPath, size, onProgress) {
      await this.ensureDir(parentRelPath(relpath))
      const target = joinRelPath(root === '/' ? '' : root, relpath)
      const result = await uploadWebDavFile(connection, target, localAbsPath, onProgress)
      return { file_id: target, size: result.size, time: result.time }
    },

    async downloadFile(entry, localAbsPath, onProgress) {
      const url = buildWebDavDownloadUrl(connection, entry.file_id)
      await downloadToFile(url, getWebDavRequestHeaders(connection), localAbsPath, onProgress)
    },

    async deleteFile(entry) {
      await deleteWebDavPath(connection, entry.file_id)
    }
  }
}

const s3Adapter = (task: SyncTask): SyncRemoteAdapter => {
  const connection = getS3Connection(getS3ConnectionId(task.drive_id))
  if (!connection) throw new Error('S3 连接不存在，请重新连接')
  const root = task.remote_dir_id === '/' ? '' : task.remote_dir_id.replace(/\/+$/, '')
  const joinKey = (relpath: string) => (root ? `${root}/${relpath}` : relpath)
  return {
    async listRecursive() {
      const items = await listS3Recursive(connection, root ? `${root}/` : '/')
      return items.map((item) => ({
        relpath: root ? item.file_id.slice(root.length + 1) : item.file_id,
        file_id: item.file_id,
        name: item.name,
        size: item.size,
        time: item.time,
        isDir: false
      })).filter((item) => !!item.relpath)
    },

    async ensureDir(relpath: string) {
      return joinKey(relpath) // S3 无真实目录，无需创建
    },

    async uploadFile(relpath, localAbsPath, _size, onProgress) {
      const target = joinKey(relpath)
      const result = await uploadS3File(connection, target, localAbsPath, onProgress)
      return { file_id: target, size: result.size, time: result.time }
    },

    async downloadFile(entry, localAbsPath, onProgress) {
      const url = await getS3DownloadUrl(connection, entry.file_id)
      await downloadToFile(url, {}, localAbsPath, onProgress)
    },

    async deleteFile(entry) {
      await deleteS3File(connection, entry.file_id)
    }
  }
}

export const createSyncRemoteAdapter = (task: SyncTask): SyncRemoteAdapter => {
  if (isWebDavDrive(task.drive_id)) return webdavAdapter(task)
  if (isS3Drive(task.drive_id)) return s3Adapter(task)
  if (resolveDriveProvider({ driveId: task.drive_id }) === 'pikpak' || task.drive_id === 'pikpak') return pikpakAdapter(task)
  throw new Error('该网盘暂不支持同步（目前支持 PikPak / WebDAV / S3）')
}
