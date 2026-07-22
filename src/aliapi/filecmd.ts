import DebugLog from '../utils/debuglog'
import { IAliGetFileModel } from './alimodels'
import { isPikPakUser } from './utils'
import { IDownloadUrl } from './models'
import AliFile from './file'
import message from '../utils/message'
import { copyWebDavPath, createWebDavDirectory, deleteWebDavPath, getWebDavConnection, getWebDavConnectionId, isWebDavDrive, moveWebDavPath, normalizeWebDavPath, renameWebDavPath, statWebDavPath } from '../utils/webdavClient'
import { copyS3Path, createS3Directory, deleteS3Path, getS3Connection, getS3ConnectionId, getS3ObjectInfo, isS3Drive, moveS3Path, normalizeS3RelativePath, renameS3Path } from '../utils/s3Client'
import { apiPikPakCopyBatch, apiPikPakMkdir, apiPikPakMoveBatch, apiPikPakRename, apiPikPakTrashBatch, apiPikPakTrashDelete, apiPikPakTrashRestore } from '../pikpak/filecmd'
import { isDriveProviderRootId, resolveDriveProvider } from '../utils/driveProvider'
import { apiOneDriveCopyBatch, apiOneDriveDeleteBatch, apiOneDriveMkdir, apiOneDriveMoveBatch, apiOneDriveRename } from '../onedrive/filecmd'
import { apiDropboxCopyBatch, apiDropboxDeleteBatch, apiDropboxMkdir, apiDropboxMoveBatch, apiDropboxRename } from '../dropbox/filecmd'
import { apiGoogleDriveCopyBatch, apiGoogleDriveDeleteBatch, apiGoogleDriveMkdir, apiGoogleDriveMoveBatch, apiGoogleDriveRename, apiGoogleDriveRestoreBatch, apiGoogleDriveTrashBatch } from '../gdrive/filecmd'
import { apiGofileCopyBatch, apiGofileDeleteBatch, apiGofileMkdir, apiGofileMoveBatch, apiGofileRename } from '../gofile/filecmd'

export default class AliFileCmd {
  static async ApiCreatNewForder(user_id: string, drive_id: string, parent_file_id: string, creatDirName: string, encType: string = '', check_name_mode: string = 'refuse'): Promise<{ file_id: string; error: string }> {
    const result = { file_id: '', error: '新建文件夹失败' }
    if (!user_id || !drive_id || !parent_file_id) return result
    if (isWebDavDrive(drive_id)) {
      const connectionId = getWebDavConnectionId(drive_id)
      const connection = getWebDavConnection(connectionId)
      if (!connection) return result
      const parentPath = isDriveProviderRootId('webdav', parent_file_id) ? '/' : parent_file_id
      const targetPath = normalizeWebDavPath(`${parentPath}/${creatDirName}`)
      try {
        await createWebDavDirectory(connection, targetPath)
        return { file_id: targetPath, error: '' }
      } catch (error: any) {
        return { file_id: '', error: error?.message || result.error }
      }
    }
    if (isS3Drive(drive_id)) {
      const connection = getS3Connection(getS3ConnectionId(drive_id))
      if (!connection) return result
      const parentPath = isDriveProviderRootId('s3', parent_file_id) ? '/' : parent_file_id
      const targetPath = normalizeS3RelativePath(`${parentPath}/${creatDirName}`)
      try {
        await createS3Directory(connection, targetPath)
        return { file_id: targetPath, error: '' }
      } catch (error: any) {
        return { file_id: '', error: error?.message || result.error }
      }
    }
    if (isPikPakUser(user_id) || drive_id === 'pikpak') {
      return apiPikPakMkdir(user_id, isDriveProviderRootId('pikpak', parent_file_id) ? 'pikpak_root' : parent_file_id, creatDirName)
    }
    const provider = resolveDriveProvider({ userId: user_id, driveId: drive_id })
    if (provider === 'onedrive') return apiOneDriveMkdir(user_id, parent_file_id, creatDirName)
    if (provider === 'dropbox') return apiDropboxMkdir(user_id, parent_file_id, creatDirName)
    if (provider === 'gdrive') return apiGoogleDriveMkdir(user_id, parent_file_id, creatDirName)
    if (provider === 'gofile') return apiGofileMkdir(user_id, parent_file_id, creatDirName)
    return { file_id: '', error: '当前网盘不支持新建文件夹' }
  }

  static async ApiTrashBatch(user_id: string, drive_id: string, file_idList: string[]): Promise<string[]> {
    if (isWebDavDrive(drive_id) || isS3Drive(drive_id)) {
      return []
    }
    if (isPikPakUser(user_id) || drive_id === 'pikpak') {
      return apiPikPakTrashBatch(user_id, file_idList)
    }
    const provider = resolveDriveProvider({ userId: user_id, driveId: drive_id })
    if (provider === 'onedrive') return apiOneDriveDeleteBatch(user_id, file_idList)
    if (provider === 'dropbox') return apiDropboxDeleteBatch(user_id, file_idList)
    if (provider === 'gdrive') return apiGoogleDriveTrashBatch(user_id, file_idList)
    if (provider === 'gofile') return []
    return []
  }

  static async ApiDeleteBatch(user_id: string, drive_id: string, file_idList: string[]): Promise<string[]> {
    if (isWebDavDrive(drive_id)) {
      const connection = getWebDavConnection(getWebDavConnectionId(drive_id))
      if (!connection) {
        message.error('WebDAV 连接不存在，请重新连接')
        return []
      }
      const successList: string[] = []
      for (const file_id of file_idList) {
        try {
          await deleteWebDavPath(connection, file_id)
          successList.push(file_id)
        } catch (error: any) {
          console.error('WebDAV 删除失败:', error)
        }
      }
      return successList
    }
    if (isS3Drive(drive_id)) {
      const connection = getS3Connection(getS3ConnectionId(drive_id))
      if (!connection) {
        message.error('S3 连接不存在，请重新连接')
        return []
      }
      const successList: string[] = []
      for (const file_id of file_idList) {
        try {
          await deleteS3Path(connection, file_id)
          successList.push(file_id)
        } catch (error) {
          console.error('S3 删除失败:', error)
        }
      }
      return successList
    }
    if (isPikPakUser(user_id) || drive_id === 'pikpak') {
      return apiPikPakTrashDelete(user_id, file_idList)
    }
    const provider = resolveDriveProvider({ userId: user_id, driveId: drive_id })
    if (provider === 'gdrive') return apiGoogleDriveDeleteBatch(user_id, file_idList)
    if (provider === 'gofile') return apiGofileDeleteBatch(user_id, file_idList)
    if (provider === 'onedrive' || provider === 'dropbox') return []
    return []
  }

  static async ApiRenameBatch(
    user_id: string,
    drive_id: string,
    file_idList: string[],
    names: string[]
  ): Promise<
    {
      file_id: string
      parent_file_id: string
      name: string
      isDir: boolean
    }[]
  > {
    if (isWebDavDrive(drive_id)) {
      const connection = getWebDavConnection(getWebDavConnectionId(drive_id))
      if (!connection) return []
      const successList: { file_id: string; parent_file_id: string; name: string; isDir: boolean }[] = []
      for (let i = 0, maxi = file_idList.length; i < maxi; i++) {
        const file_id = file_idList[i]
        const name = names[i] || ''
        if (!file_id || !name) continue
        try {
          const info = await statWebDavPath(connection, file_id)
          const targetPath = await renameWebDavPath(connection, file_id, name)
          successList.push({ file_id: targetPath, parent_file_id: normalizeWebDavPath(targetPath.split('/').slice(0, -1).join('/')), name, isDir: info.type === 'directory' })
        } catch (error) {
          console.error('WebDAV 重命名失败:', error)
        }
      }
      return successList
    }
    if (isS3Drive(drive_id)) {
      const connection = getS3Connection(getS3ConnectionId(drive_id))
      if (!connection) return []
      const successList: { file_id: string; parent_file_id: string; name: string; isDir: boolean }[] = []
      for (let i = 0, maxi = file_idList.length; i < maxi; i++) {
        const file_id = file_idList[i]
        const name = names[i] || ''
        if (!file_id || !name) continue
        try {
          const info = await getS3ObjectInfo(connection, file_id)
          const targetPath = await renameS3Path(connection, file_id, name)
          successList.push({ file_id: targetPath, parent_file_id: normalizeS3RelativePath(targetPath.split('/').slice(0, -1).join('/')), name, isDir: info.isDir })
        } catch (error) {
          console.error('S3 重命名失败:', error)
        }
      }
      return successList
    }
    if (isPikPakUser(user_id) || drive_id === 'pikpak') {
      const successList: { file_id: string; parent_file_id: string; name: string; isDir: boolean }[] = []
      for (let i = 0, maxi = file_idList.length; i < maxi; i++) {
        const file_id = file_idList[i]
        const name = names[i] || ''
        if (!file_id || !name) continue
        const resp = await apiPikPakRename(user_id, file_id, name)
        if (resp.success) {
          successList.push({ file_id, name: resp.name, parent_file_id: resp.parent_file_id, isDir: resp.isDir })
        }
      }
      return successList
    }
    const provider = resolveDriveProvider({ userId: user_id, driveId: drive_id })
    if (provider === 'onedrive' || provider === 'dropbox' || provider === 'gdrive' || provider === 'gofile') {
      const successList: { file_id: string; parent_file_id: string; name: string; isDir: boolean }[] = []
      for (let i = 0; i < file_idList.length; i++) {
        const file_id = file_idList[i]
        const name = names[i] || ''
        if (!file_id || !name) continue
        try {
          if (provider === 'dropbox') {
            const result = await apiDropboxRename(user_id, file_id, name)
            if (result.success) successList.push({ file_id: result.file_id, parent_file_id: result.parent_file_id, name: result.name, isDir: result.isDir })
            continue
          }
          if (provider === 'onedrive') {
            const result = await apiOneDriveRename(user_id, file_id, name)
            if (result.error) continue
          } else if (provider === 'gdrive') {
            await apiGoogleDriveRename(user_id, file_id, name)
          } else {
            await apiGofileRename(user_id, file_id, name)
          }
          const detail = await AliFile.ApiGetFile(user_id, drive_id, file_id)
          if (detail) successList.push({ file_id: detail.file_id || file_id, parent_file_id: detail.parent_file_id || '', name: detail.name || name, isDir: !!detail.isDir })
        } catch (error) {
          DebugLog.mSaveWarning(`${provider} rename failed ${file_id}`, error)
        }
      }
      return successList
    }
    return []
  }

  static async ApiFavorBatch(_user_id: string, _drive_id: string, _isfavor: boolean, _ismessage: boolean, _file_idList: string[]): Promise<string[]> {
    return []
  }

  static async ApiTrashCleanBatch(user_id: string, drive_id: string, ismessage: boolean, file_idList: string[]): Promise<string[]> {
    if (isPikPakUser(user_id) || drive_id === 'pikpak') {
      return apiPikPakTrashDelete(user_id, file_idList)
    }
    if (resolveDriveProvider({ userId: user_id, driveId: drive_id }) === 'gdrive') return apiGoogleDriveDeleteBatch(user_id, file_idList)
    return []
  }

  static async ApiTrashRestoreBatch(user_id: string, drive_id: string, ismessage: boolean, file_idList: string[]): Promise<string[]> {
    if (isPikPakUser(user_id) || drive_id === 'pikpak') {
      return apiPikPakTrashRestore(user_id, file_idList)
    }
    if (resolveDriveProvider({ userId: user_id, driveId: drive_id }) === 'gdrive') return apiGoogleDriveRestoreBatch(user_id, file_idList)
    return []
  }

  static async ApiFileHistoryBatch(user_id: string, drive_id: string, file_idList: string[]) {
    let allTask = []
    const loadingKey = 'filehistorybatch' + Date.now().toString()
    message.loading('清除历史 执行中...', 60, loadingKey)
    for (const file_id of file_idList) {
      allTask.push(AliFile.ApiUpdateVideoTime(user_id, drive_id, file_id, 0))
      if (allTask.length >= 3) {
        await Promise.all(allTask).catch()
        allTask = []
      }
    }
    if (allTask.length > 0) {
      await Promise.all(allTask).catch()
    }
    message.success('播放记录已清除', 1, loadingKey)
  }

  static async ApiFileColorBatch(_user_id: string, drive_id: string, _description: string, _color: string, _file_idList: string[]) {
    if (isWebDavDrive(drive_id) || isS3Drive(drive_id)) return
  }

  static async ApiMoveBatch(user_id: string, drive_id: string, file_idList: string[], to_drive_id: string, to_parent_file_id: string, to_parent_description: string = ''): Promise<string[]> {
    if (isWebDavDrive(drive_id)) {
      if (drive_id !== to_drive_id) {
        message.error('WebDAV 暂不支持跨来源移动')
        return []
      }
      const connection = getWebDavConnection(getWebDavConnectionId(drive_id))
      if (!connection) return []
      const targetParent = isDriveProviderRootId('webdav', to_parent_file_id) ? '/' : normalizeWebDavPath(to_parent_file_id)
      const successList: string[] = []
      for (const file_id of file_idList) {
        try {
          const targetPath = normalizeWebDavPath(`${targetParent}/${file_id.split('/').pop() || ''}`)
          await moveWebDavPath(connection, file_id, targetPath)
          successList.push(file_id)
        } catch (error) {
          console.error('WebDAV 移动失败:', error)
        }
      }
      return successList
    }
    if (isS3Drive(drive_id)) {
      if (drive_id !== to_drive_id) {
        message.error('S3 暂不支持跨来源移动')
        return []
      }
      const connection = getS3Connection(getS3ConnectionId(drive_id))
      if (!connection) return []
      const targetParent = isDriveProviderRootId('s3', to_parent_file_id) ? '/' : normalizeS3RelativePath(to_parent_file_id)
      const successList: string[] = []
      for (const file_id of file_idList) {
        try {
          await moveS3Path(connection, file_id, normalizeS3RelativePath(`${targetParent}/${file_id.split('/').pop() || ''}`))
          successList.push(file_id)
        } catch (error) {
          console.error('S3 移动失败:', error)
        }
      }
      return successList
    }
    if (isPikPakUser(user_id) || drive_id === 'pikpak') {
      return apiPikPakMoveBatch(user_id, file_idList, isDriveProviderRootId('pikpak', to_parent_file_id) ? 'pikpak_root' : to_parent_file_id)
    }
    const provider = resolveDriveProvider({ userId: user_id, driveId: drive_id })
    if (provider === 'onedrive' || provider === 'dropbox' || provider === 'gdrive' || provider === 'gofile') {
      if (drive_id !== to_drive_id) {
        message.error('暂不支持跨网盘移动')
        return []
      }
      if (provider === 'onedrive') return apiOneDriveMoveBatch(user_id, to_parent_file_id, file_idList)
      if (provider === 'dropbox') return apiDropboxMoveBatch(user_id, file_idList, to_parent_file_id, to_parent_description)
      if (provider === 'gdrive') return apiGoogleDriveMoveBatch(user_id, file_idList, to_parent_file_id)
      return apiGofileMoveBatch(user_id, file_idList, to_parent_file_id)
    }
    return []
  }

  static async ApiCopyBatch(user_id: string, drive_id: string, file_idList: string[], to_drive_id: string, to_parent_file_id: string, to_parent_description: string = ''): Promise<string[]> {
    if (isWebDavDrive(drive_id)) {
      if (drive_id !== to_drive_id) {
        message.error('WebDAV 暂不支持跨来源复制')
        return []
      }
      const connection = getWebDavConnection(getWebDavConnectionId(drive_id))
      if (!connection) return []
      const targetParent = isDriveProviderRootId('webdav', to_parent_file_id) ? '/' : normalizeWebDavPath(to_parent_file_id)
      const successList: string[] = []
      for (const file_id of file_idList) {
        try {
          const targetPath = normalizeWebDavPath(`${targetParent}/${file_id.split('/').pop() || ''}`)
          await copyWebDavPath(connection, file_id, targetPath)
          successList.push(file_id)
        } catch (error) {
          console.error('WebDAV 复制失败:', error)
        }
      }
      return successList
    }
    if (isS3Drive(drive_id)) {
      if (drive_id !== to_drive_id) {
        message.error('S3 暂不支持跨来源复制')
        return []
      }
      const connection = getS3Connection(getS3ConnectionId(drive_id))
      if (!connection) return []
      const targetParent = isDriveProviderRootId('s3', to_parent_file_id) ? '/' : normalizeS3RelativePath(to_parent_file_id)
      const successList: string[] = []
      for (const file_id of file_idList) {
        try {
          await copyS3Path(connection, file_id, normalizeS3RelativePath(`${targetParent}/${file_id.split('/').pop() || ''}`))
          successList.push(file_id)
        } catch (error) {
          console.error('S3 复制失败:', error)
        }
      }
      return successList
    }
    if (isPikPakUser(user_id) || drive_id === 'pikpak') {
      return apiPikPakCopyBatch(user_id, file_idList, isDriveProviderRootId('pikpak', to_parent_file_id) ? 'pikpak_root' : to_parent_file_id)
    }
    const provider = resolveDriveProvider({ userId: user_id, driveId: drive_id })
    if (provider === 'onedrive' || provider === 'dropbox' || provider === 'gdrive' || provider === 'gofile') {
      if (drive_id !== to_drive_id) {
        message.error('暂不支持跨网盘复制')
        return []
      }
      if (provider === 'onedrive') {
        const fileList: { file_id: string; name: string }[] = []
        for (const file_id of file_idList) {
          const detail = await AliFile.ApiGetFile(user_id, drive_id, file_id)
          if (detail) fileList.push({ file_id, name: detail.name })
        }
        return apiOneDriveCopyBatch(user_id, to_parent_file_id, fileList)
      }
      if (provider === 'dropbox') return apiDropboxCopyBatch(user_id, file_idList, to_parent_file_id, to_parent_description)
      if (provider === 'gdrive') return apiGoogleDriveCopyBatch(user_id, file_idList, to_parent_file_id)
      return apiGofileCopyBatch(user_id, file_idList, to_parent_file_id)
    }
    return []
  }

  static async ApiGetFileBatch(user_id: string, drive_id: string, file_idList: string[]): Promise<IAliGetFileModel[]> {
    const files = await Promise.all(file_idList.map((file_id) => AliFile.ApiGetFile(user_id, drive_id, file_id)))
    return files.filter((file): file is IAliGetFileModel => !!file)
  }

  static async ApiGetFileDownloadUrlBatch(user_id: string, drive_id: string, file_idList: string[]): Promise<IDownloadUrl[]> {
    const successList: IDownloadUrl[] = []
    for (const file_id of file_idList) {
      const data = await AliFile.ApiFileDownloadUrl(user_id, drive_id, file_id, 14400)
      if (typeof data !== 'string') successList.push(data)
    }
    return successList
  }
}
