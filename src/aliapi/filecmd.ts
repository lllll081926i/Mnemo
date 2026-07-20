import DebugLog from '../utils/debuglog'
import AliHttp from './alihttp'
import { IAliFileItem, IAliGetFileModel } from './alimodels'
import AliDirFileList from './dirfilelist'
import { ApiBatch, ApiBatchMaker, ApiBatchMaker2, ApiBatchSuccess, EncodeEncName, isPikPakUser } from './utils'
import { IDownloadUrl } from './models'
import AliFile from './file'
import message from '../utils/message'
import usePanFileStore from '../pan/panfilestore'
import { copyWebDavPath, createWebDavDirectory, deleteWebDavPath, getWebDavConnection, getWebDavConnectionId, isWebDavDrive, moveWebDavPath, normalizeWebDavPath, renameWebDavPath, statWebDavPath } from '../utils/webdavClient'
import { copyS3Path, createS3Directory, deleteS3Path, getS3Connection, getS3ConnectionId, getS3ObjectInfo, isS3Drive, moveS3Path, normalizeS3RelativePath, renameS3Path } from '../utils/s3Client'
import { apiPikPakCopyBatch, apiPikPakMkdir, apiPikPakMoveBatch, apiPikPakRename, apiPikPakTrashBatch, apiPikPakTrashDelete, apiPikPakTrashRestore } from '../pikpak/filecmd'
import { resolveDriveProvider } from '../utils/driveProvider'
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
      const parentPath = parent_file_id.includes('root') ? '/' : parent_file_id
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
      const parentPath = parent_file_id.includes('root') ? '/' : parent_file_id
      const targetPath = normalizeS3RelativePath(`${parentPath}/${creatDirName}`)
      try {
        await createS3Directory(connection, targetPath)
        return { file_id: targetPath, error: '' }
      } catch (error: any) {
        return { file_id: '', error: error?.message || result.error }
      }
    }
    if (isPikPakUser(user_id) || drive_id === 'pikpak') {
      return apiPikPakMkdir(user_id, parent_file_id.includes('root') ? 'pikpak_root' : parent_file_id, creatDirName)
    }
    const provider = resolveDriveProvider({ userId: user_id, driveId: drive_id })
    if (provider === 'onedrive') return apiOneDriveMkdir(user_id, parent_file_id, creatDirName)
    if (provider === 'dropbox') return apiDropboxMkdir(user_id, parent_file_id, creatDirName)
    if (provider === 'gdrive') return apiGoogleDriveMkdir(user_id, parent_file_id, creatDirName)
    if (provider === 'gofile') return apiGofileMkdir(user_id, parent_file_id, creatDirName)
    if (parent_file_id.includes('root')) parent_file_id = 'root'
    const url = 'adrive/v2/file/createWithFolders'
    const name = EncodeEncName(user_id, creatDirName, true, encType)
    const postData = JSON.stringify({
      drive_id: drive_id,
      parent_file_id: parent_file_id,
      name: name,
      check_name_mode: check_name_mode,
      type: 'folder',
      description: encType
    })
    const resp = await AliHttp.Post(url, postData, user_id, '')
    if (AliHttp.IsSuccess(resp.code)) {
      const file_id = resp.body.file_id as string | undefined
      if (file_id) return { file_id, error: '' }
    } else if (!AliHttp.HttpCodeBreak(resp.code)) {
      DebugLog.mSaveWarning('ApiCreatNewForder err=' + parent_file_id + ' ' + (resp.code || ''), resp.body)
    }
    if (resp.body?.code == 'QuotaExhausted.Drive') return { file_id: '', error: '网盘空间已满,无法创建' }
    if (resp.body?.code) return { file_id: '', error: resp.body?.code }
    return result
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
    const batchList = ApiBatchMaker('/recyclebin/trash', file_idList, (file_id: string) => {
      return { drive_id: drive_id, file_id: file_id }
    })
    return ApiBatchSuccess('放入回收站', batchList, user_id, '')
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
    const batchList = ApiBatchMaker('/file/delete', file_idList, (file_id: string) => {
      return { drive_id: drive_id, file_id: file_id }
    })
    return ApiBatchSuccess('彻底删除', batchList, user_id, '')
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
    const batchList = ApiBatchMaker2('/file/update', file_idList, names, (file_id: string, name: string) => {
      return { drive_id: drive_id, file_id: file_id, name: name, check_name_mode: 'refuse' }
    })

    if (batchList.length == 0) return Promise.resolve([])
    const successList: { file_id: string; parent_file_id: string; name: string; isDir: boolean }[] = []
    const result = await ApiBatch(file_idList.length <= 1 ? '' : '批量重命名', batchList, user_id, '')
    result.reslut.map((t) =>
      successList.push({
        file_id: t.file_id!,
        name: t.name!,
        parent_file_id: t.parent_file_id!,
        isDir: t.type === 'folder'
      })
    )
    return successList
  }

  static async ApiFavorBatch(user_id: string, drive_id: string, isfavor: boolean, ismessage: boolean, file_idList: string[]): Promise<string[]> {
    const batchList = ApiBatchMaker('/file/update', file_idList, (file_id: string) => {
      return { drive_id: drive_id, file_id: file_id, custom_index_key: isfavor ? 'starred_yes' : '', starred: isfavor }
    })
    return ApiBatchSuccess(ismessage ? (isfavor ? '收藏文件' : '取消收藏') : '', batchList, user_id, '')
  }

  static async ApiTrashCleanBatch(user_id: string, drive_id: string, ismessage: boolean, file_idList: string[]): Promise<string[]> {
    if (isPikPakUser(user_id) || drive_id === 'pikpak') {
      return apiPikPakTrashDelete(user_id, file_idList)
    }
    if (resolveDriveProvider({ userId: user_id, driveId: drive_id }) === 'gdrive') return apiGoogleDriveDeleteBatch(user_id, file_idList)
    const batchList = ApiBatchMaker('/file/delete', file_idList, (file_id: string) => {
      return { drive_id: drive_id, file_id: file_id }
    })
    return ApiBatchSuccess(ismessage ? '从回收站删除' : '', batchList, user_id, '')
  }

  static async ApiTrashRestoreBatch(user_id: string, drive_id: string, ismessage: boolean, file_idList: string[]): Promise<string[]> {
    if (isPikPakUser(user_id) || drive_id === 'pikpak') {
      return apiPikPakTrashRestore(user_id, file_idList)
    }
    if (resolveDriveProvider({ userId: user_id, driveId: drive_id }) === 'gdrive') return apiGoogleDriveRestoreBatch(user_id, file_idList)
    const batchList = ApiBatchMaker('/recyclebin/restore', file_idList, (file_id: string) => {
      return { drive_id: drive_id, file_id: file_id }
    })
    return ApiBatchSuccess(ismessage ? '从回收站还原' : '', batchList, user_id, '')
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
    message.success('成功执行 清除历史', 1, loadingKey)
  }

  static async ApiFileColorBatch(user_id: string, drive_id: string, description: string, color: string, file_idList: string[]) {
    if (isWebDavDrive(drive_id) || isS3Drive(drive_id)) return
    if (isPikPakUser(user_id) || drive_id === 'pikpak') return
    // 防止加密标记清空
    let parts = description.split(',') || []
    let encryptPart = parts.find((part: any) => part.includes('mnemoEncrypt')) || ''
    let colorPart = parts.find((part: any) => /c.{6}$/.test(part)) || ''
    if (color) {
      if (color.includes('mnemoEncrypt')) {
        encryptPart = color
      } else if (color === 'notEncrypt') {
        encryptPart = ''
      } else {
        colorPart = color
      }
    }
    color = color ? [encryptPart, colorPart].filter(Boolean).join(',') : encryptPart
    let batchList = ApiBatchMaker('/file/update', file_idList, (file_id: string) => {
      return { drive_id: drive_id, file_id: file_id, description: color }
    })
    let title = ''
    if (color == '' || color == 'notEncrypt') {
      title = '清除标记'
    } else if (color.includes('ce74c3c')) {
      title = ''
    } else if (color.includes('mnemoEncrypt')) {
      title = '标记加密'
    }
    let successList = await ApiBatchSuccess(title, batchList, user_id, '')
    usePanFileStore().mColorFiles(color, successList)
  }

  static async ApiMoveBatch(user_id: string, drive_id: string, file_idList: string[], to_drive_id: string, to_parent_file_id: string, to_parent_description: string = ''): Promise<string[]> {
    if (isWebDavDrive(drive_id)) {
      if (drive_id !== to_drive_id) {
        message.error('WebDAV 暂不支持跨来源移动')
        return []
      }
      const connection = getWebDavConnection(getWebDavConnectionId(drive_id))
      if (!connection) return []
      const targetParent = to_parent_file_id.includes('root') ? '/' : normalizeWebDavPath(to_parent_file_id)
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
      const targetParent = to_parent_file_id.includes('root') ? '/' : normalizeS3RelativePath(to_parent_file_id)
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
      return apiPikPakMoveBatch(user_id, file_idList, to_parent_file_id.includes('root') ? 'pikpak_root' : to_parent_file_id)
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
    if (to_parent_file_id.includes('root')) to_parent_file_id = 'root'
    const batchList = ApiBatchMaker('/file/move', file_idList, (file_id: string) => {
      if (drive_id == to_drive_id)
        return {
          drive_id: drive_id,
          file_id: file_id,
          to_parent_file_id: to_parent_file_id,
          auto_rename: true
        }
      else
        return {
          drive_id: drive_id,
          file_id: file_id,
          to_drive_id: to_drive_id,
          to_parent_file_id: to_parent_file_id,
          auto_rename: true
        }
    })
    return ApiBatchSuccess(file_idList.length <= 1 ? '移动' : '批量移动', batchList, user_id, '')
  }

  static async ApiCopyBatch(user_id: string, drive_id: string, file_idList: string[], to_drive_id: string, to_parent_file_id: string, to_parent_description: string = ''): Promise<string[]> {
    if (isWebDavDrive(drive_id)) {
      if (drive_id !== to_drive_id) {
        message.error('WebDAV 暂不支持跨来源复制')
        return []
      }
      const connection = getWebDavConnection(getWebDavConnectionId(drive_id))
      if (!connection) return []
      const targetParent = to_parent_file_id.includes('root') ? '/' : normalizeWebDavPath(to_parent_file_id)
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
      const targetParent = to_parent_file_id.includes('root') ? '/' : normalizeS3RelativePath(to_parent_file_id)
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
      return apiPikPakCopyBatch(user_id, file_idList, to_parent_file_id.includes('root') ? 'pikpak_root' : to_parent_file_id)
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
    if (to_parent_file_id.includes('root')) to_parent_file_id = 'root'
    const batchList = ApiBatchMaker('/file/copy', file_idList, (file_id: string) => {
      if (drive_id == to_drive_id)
        return {
          drive_id: drive_id,
          file_id: file_id,
          to_parent_file_id: to_parent_file_id,
          auto_rename: true
        }
      else
        return {
          drive_id: drive_id,
          file_id: file_id,
          to_drive_id: to_drive_id,
          to_parent_file_id: to_parent_file_id,
          auto_rename: true
        }
    })
    return ApiBatchSuccess(file_idList.length <= 1 ? '复制' : '批量复制', batchList, user_id, '')
  }

  static async ApiGetFileBatch(user_id: string, drive_id: string, file_idList: string[]): Promise<IAliGetFileModel[]> {
    const provider = resolveDriveProvider({ userId: user_id, driveId: drive_id })
    if (provider !== 'aliyun') {
      const files = await Promise.all(file_idList.map((file_id) => AliFile.ApiGetFile(user_id, drive_id, file_id)))
      return files.filter((file): file is IAliGetFileModel => !!file)
    }
    const batchList = ApiBatchMaker('/file/get', file_idList, (file_id: string) => {
      return {
        drive_id: drive_id,
        file_id: file_id,
        url_expire_sec: 14400,
        office_thumbnail_process: 'image/resize,w_400/format,jpeg',
        image_thumbnail_process: 'image/resize,w_400/format,jpeg',
        image_url_process: 'image/resize,w_1920/format,jpeg',
        video_thumbnail_process: 'video/snapshot,t_106000,f_jpg,ar_auto,m_fast,w_400'
      }
    })
    const successList: IAliGetFileModel[] = []
    const result = await ApiBatch('', batchList, user_id, '')
    result.reslut.map((t) => {
      if (t.body) successList.push(AliDirFileList.getFileInfo(user_id, t.body as IAliFileItem, 'download_url'))
      return true
    })
    return successList
  }

  static async ApiGetFileDownloadUrlBatch(user_id: string, drive_id: string, file_idList: string[]): Promise<IDownloadUrl[]> {
    const batchList = ApiBatchMaker('/file/get_download_url', file_idList, (file_id: string) => {
      return {
        drive_id: drive_id,
        file_id: file_id,
        expire_sec: 14400
      }
    })
    const successList: IDownloadUrl[] = []
    const result = await ApiBatch('', batchList, user_id, '')
    result.reslut.map((t) => {
      if (t.body) successList.push(t.body as IDownloadUrl)
      return true
    })
    return successList
  }
}
