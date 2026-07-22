import path from 'path'
import DebugLog from '../utils/debuglog'
import { GetExpiresTime } from '../utils/utils'
import AliHttp from './alihttp'
import { IAliFileItem, IAliGetDirModel, IAliGetFileModel, IAliGetForderSizeModel } from './alimodels'
import { ICompilationList, IDownloadUrl, IOfficePreViewUrl, IVideoPreviewUrl, IVideoXBTUrl } from './models'
import { isPikPakUser } from './utils'
import { getProxyUrl, getRawUrl } from '../utils/proxyhelper'
import { apiPikPakDownloadInfo, apiPikPakFileDetail, mapPikPakFileToAliModel } from '../pikpak/dirfilelist'
import TreeStore from '../store/treestore'
import UserDAL from '../user/userdal'
import { ITokenInfo } from '../user/userstore'
import { getWebDavConnection, getWebDavConnectionId, getWebDavDownloadUrl, getWebDavRequestHeaders, isWebDavDrive } from '../utils/webdavClient'
import { getS3Connection, getS3ConnectionId, getS3DownloadUrl, getS3ObjectInfo, isS3Drive } from '../utils/s3Client'
import { getDriveProviderLabel, isDriveProviderRootId, resolveDriveProvider } from '../utils/driveProvider'
import { apiOneDriveFileDetail, getOneDriveDownloadUrl, mapOneDriveItemToAliModel } from '../onedrive/dirfilelist'
import { apiDropboxFileDetail, apiDropboxTemporaryLink, mapDropboxFileToAliModel, resolveDropboxParentIdFromPath } from '../dropbox/dirfilelist'
import { apiGoogleDriveFileDetail, mapGoogleDriveItemToAliModel } from '../gdrive/dirfilelist'
import { apiGoogleDriveDownloadInfo } from '../gdrive/download'
import { apiGofileFileDetail, mapGofileItemToAliModel } from '../gofile/dirfilelist'

const resolveFileProvider = async (user_id: string, drive_id: string) => {
  let token: ITokenInfo | undefined = UserDAL.GetUserToken(user_id)
  if (!token && user_id) token = await UserDAL.GetUserTokenFromDB(user_id)
  return resolveDriveProvider({ tokenfrom: token?.tokenfrom, userId: user_id, driveId: drive_id })
}

export default class AliFile {
  static async ApiFileInfo(user_id: string, drive_id: string, file_id: string, ispic: boolean = false): Promise<any | undefined> {
    if (!drive_id || !file_id) return undefined
    const provider = await resolveFileProvider(user_id, drive_id)
  if (provider === 'webdav') {
      const connection = getWebDavConnection(getWebDavConnectionId(drive_id))
      const normalizedPath = file_id === 'root' ? '/' : file_id
      if (normalizedPath === '/') {
        return {
          drive_id,
          file_id: '/',
          parent_file_id: '',
          name: connection?.name || 'WebDAV',
          type: 'folder',
          isDir: true
        }
      }
      return {
        drive_id,
        file_id: normalizedPath,
        parent_file_id: path.posix.dirname(normalizedPath) || '/',
        name: path.posix.basename(normalizedPath) || connection?.name || 'WebDAV',
        type: 'folder',
        isDir: true
      }
    }
    if (provider === 's3') {
      const connection = getS3Connection(getS3ConnectionId(drive_id))
      const normalizedPath = file_id === 'root' ? '/' : file_id
      if (normalizedPath === '/') return { drive_id, file_id: '/', parent_file_id: '', name: connection?.name || 'S3', type: 'folder', isDir: true }
      if (!connection) return undefined
      const info = await getS3ObjectInfo(connection, normalizedPath)
      return {
        drive_id,
        file_id: normalizedPath,
        parent_file_id: path.posix.dirname(normalizedPath) || '/',
        name: path.posix.basename(normalizedPath) || connection.name,
        type: info.isDir ? 'folder' : 'file',
        size: info.size,
        isDir: info.isDir
      }
    }
    if (!user_id || !drive_id || !file_id) return undefined
    if (provider === 'pikpak') {
      if (file_id === 'pikpak_root') {
        return {
          drive_id,
          file_id,
          parent_file_id: '',
          name: '网盘文件',
          type: 'folder',
          isDir: true
        }
      }
      const detail = await apiPikPakFileDetail(user_id, file_id)
      if (!detail) return undefined
      const mapped = mapPikPakFileToAliModel(detail, drive_id, detail.parent_id || 'pikpak_root') as any
      mapped.type = mapped.isDir ? 'folder' : 'file'
      return mapped
    }
    if (provider === 'onedrive') {
      if (file_id === 'onedrive_root') return { drive_id, file_id, parent_file_id: '', name: 'OneDrive', type: 'folder', isDir: true }
      const detail = await apiOneDriveFileDetail(user_id, file_id)
      if (!detail) return undefined
      const mapped = mapOneDriveItemToAliModel(detail, drive_id, detail.parentReference?.id || 'onedrive_root') as any
      mapped.type = mapped.isDir ? 'folder' : 'file'
      return mapped
    }
    if (provider === 'dropbox') {
      if (file_id === 'dropbox_root') return { drive_id, file_id, parent_file_id: '', name: 'Dropbox', type: 'folder', isDir: true }
      const detail = await apiDropboxFileDetail(user_id, file_id)
      if (!detail) return undefined
      const mapped = mapDropboxFileToAliModel(detail, drive_id, resolveDropboxParentIdFromPath(detail.path_lower || detail.path_display)) as any
      mapped.type = mapped.isDir ? 'folder' : 'file'
      return mapped
    }
    if (provider === 'gdrive') {
      if (file_id === 'gdrive_root') return { drive_id, file_id, parent_file_id: '', name: 'Google Drive', type: 'folder', isDir: true }
      const detail = await apiGoogleDriveFileDetail(user_id, file_id)
      const mapped = mapGoogleDriveItemToAliModel(detail, drive_id, detail.parents?.[0] || 'gdrive_root') as any
      mapped.type = mapped.isDir ? 'folder' : 'file'
      return mapped
    }
    if (provider === 'gofile') {
      if (file_id === 'gofile_root') return { drive_id, file_id, parent_file_id: '', name: 'GoFile', type: 'folder', isDir: true }
      const detail = await apiGofileFileDetail(user_id, file_id)
      if (!detail) return undefined
      const mapped = mapGofileItemToAliModel(detail, drive_id, detail.parentFolder || 'gofile_root') as any
      mapped.type = mapped.isDir ? 'folder' : 'file'
      return mapped
    }
    return undefined
  }

  static async ApiFileInfoByPath(_user_id: string, _drive_id: string, _file_path: string): Promise<IAliFileItem | undefined> {
    return undefined
  }

  static async ApiFileDownloadUrl(user_id: string, drive_id: string, file_id: string, expire_sec: number): Promise<IDownloadUrl | string> {
    if (!drive_id || !file_id) return '参数错误'
    const provider = await resolveFileProvider(user_id, drive_id)
  if (provider === 'webdav') {
      const connectionId = getWebDavConnectionId(drive_id)
      const connection = getWebDavConnection(connectionId)
      if (!connection) return 'WebDAV 连接不存在，请重新连接'
      const url = await getWebDavDownloadUrl(connection, file_id)
      return {
        drive_id,
        file_id,
        expire_time: 0,
        url,
        size: 0,
        headers: getWebDavRequestHeaders(connection)
      }
    }
    if (provider === 's3') {
      const connection = getS3Connection(getS3ConnectionId(drive_id))
      if (!connection) return 'S3 连接不存在，请重新连接'
      const info = await getS3ObjectInfo(connection, file_id)
      if (info.isDir) return '文件夹不能直接下载'
      const url = await getS3DownloadUrl(connection, file_id, Math.max(60, expire_sec || 14400))
      return { drive_id, file_id, expire_time: Date.now() + Math.max(60, expire_sec || 14400) * 1000, url, size: info.size }
    }
    if (!user_id || !drive_id || !file_id) return '参数错误'
    if (provider === 'pikpak') {
      const info = await apiPikPakDownloadInfo(user_id, file_id)
      if (info.error) return info.error
      const detail = info.item
      const url = info.streamUrl || info.downloadUrl
      if (!url) return '获取下载地址失败'
      return {
        drive_id: drive_id,
        file_id: file_id,
        expire_time: GetExpiresTime(url),
        url,
        size: Number(detail?.size || 0)
      }
    }
    if (provider === 'onedrive') {
      const item = await apiOneDriveFileDetail(user_id, file_id)
      if (!item || item.folder) return item?.folder ? '文件夹不能直接下载' : '获取 OneDrive 文件信息失败'
      const url = getOneDriveDownloadUrl(item)
      if (!url) return '获取 OneDrive 下载地址失败'
      return { drive_id, file_id, expire_time: GetExpiresTime(url), url, size: Number(item.size || 0) }
    }
    if (provider === 'dropbox') {
      const info = await apiDropboxTemporaryLink(user_id, file_id)
      if (info.error || !info.url) return info.error || '获取 Dropbox 下载地址失败'
      return { drive_id, file_id, expire_time: GetExpiresTime(info.url), url: info.url, size: Number(info.metadata?.size || 0) }
    }
    if (provider === 'gdrive') {
      const info = await apiGoogleDriveDownloadInfo(user_id, file_id)
      if (info.error || !info.url) return info.error || '获取 Google Drive 下载地址失败'
      return { drive_id, file_id, expire_time: 0, url: info.url, size: info.size, headers: info.headers }
    }
    if (provider === 'gofile') {
      const item = await apiGofileFileDetail(user_id, file_id)
      if (!item || item.type === 'folder') return item?.type === 'folder' ? '文件夹不能直接下载' : '获取 GoFile 文件信息失败'
      if (!item.link) return '获取 GoFile 下载地址失败'
      return { drive_id, file_id, expire_time: 0, url: item.link, size: Number(item.size || 0) }
    }
    return `${getDriveProviderLabel(provider)} 暂不支持下载`
  }

  static async ApiVideoPreviewUrl(user_id: string, drive_id: string, file_id: string, _promotionSkuCode = ''): Promise<IVideoPreviewUrl | string> {
    if (!drive_id || !file_id) return '参数错误'
    const provider = await resolveFileProvider(user_id, drive_id)
    return `${getDriveProviderLabel(provider)} 暂无转码信息`
  }

  static async ApiListByFileInfo(_user_id: string, _drive_id: string, _file_id: string, _limit?: number): Promise<ICompilationList[] | undefined> {
    return undefined
  }

  static async ApiAudioPreviewUrl(user_id: string, drive_id: string, file_id: string): Promise<IDownloadUrl | string> {
    if (!user_id || !drive_id || !file_id) return '参数错误'
    return AliFile.ApiFileDownloadUrl(user_id, drive_id, file_id, 14400)
  }

  static async ApiOfficePreViewUrl(_user_id: string, _drive_id: string, _file_id: string): Promise<IOfficePreViewUrl | undefined> {
    return undefined
  }

  static async ApiGetFile(user_id: string, drive_id: string, file_id: string): Promise<IAliGetFileModel | undefined> {
    if (!user_id || !drive_id || !file_id) return undefined
    const provider = await resolveFileProvider(user_id, drive_id)
    if (provider === 'pikpak' || isPikPakUser(user_id) || drive_id === 'pikpak') {
      const detail = await apiPikPakFileDetail(user_id, file_id)
      if (!detail) return undefined
      return mapPikPakFileToAliModel(detail, drive_id, detail.parent_id || 'pikpak_root')
    }
    const info = await AliFile.ApiFileInfo(user_id, drive_id, file_id)
    return typeof info === 'object' ? info as IAliGetFileModel : undefined
  }

  static async ApiFileGetPath(user_id: string, drive_id: string, file_id: string): Promise<IAliGetDirModel[]> {
    if (!user_id || !drive_id || !file_id) return []
    const cached = TreeStore.GetDirPath(drive_id, file_id) as IAliGetDirModel[]
    if (cached.length > 0) return cached
    const result: IAliGetDirModel[] = []
    const visited = new Set<string>()
    let currentId = file_id
    for (let depth = 0; currentId && depth < 100; depth++) {
      if (visited.has(currentId)) break
      visited.add(currentId)
      const detail = await AliFile.ApiFileInfo(user_id, drive_id, currentId)
      if (!detail || !detail.file_id) break
      result.unshift(detail as IAliGetDirModel)
      if (isDriveProviderRootId({ userId: user_id, driveId: drive_id }, detail.file_id)) break
      currentId = detail.parent_file_id || ''
    }
    return result
  }

  static async ApiFileGetPathString(user_id: string, drive_id: string, file_id: string, dirsplit: string): Promise<string> {
    if (!user_id || !drive_id || !file_id) return ''
    const pathList = TreeStore.GetDirPath(drive_id, file_id)
    return pathList.map((item) => item.name).filter(Boolean).join(dirsplit)
  }

  static async ApiFileGetFolderSize(user_id: string, drive_id: string, file_id: string): Promise<IAliGetForderSizeModel | undefined> {
    if (!user_id || !drive_id || !file_id) return undefined
    return { size: 0, folder_count: 0, file_count: 0, reach_limit: undefined }
  }

  static async ApiFileDownText(user_id: string, drive_id: string, file_id: string, filesize: number, maxsize: number, encType: string = '', password: string = ''): Promise<string> {
    if (!user_id || !drive_id || !file_id) return ''
    const downUrl = await getRawUrl(user_id, drive_id, file_id, encType, password)
    if (typeof downUrl == 'string') return downUrl
    // 原始文件大小
    if (filesize === -1) filesize = downUrl.size
    if (maxsize === -1) maxsize = downUrl.size
    const proxyUrl = getProxyUrl({
      user_id,
      drive_id,
      file_id,
      file_size: downUrl.size,
      encType,
      password,
      quality: 'Origin',
      proxy_kind: 'subtitle',
      proxy_url: downUrl.url,
      proxy_headers: downUrl.headers ? JSON.stringify(downUrl.headers) : undefined
    })
    const resp = await AliHttp.GetString(proxyUrl, '', filesize, maxsize)
    if (AliHttp.IsSuccess(resp.code)) {
      if (typeof resp.body == 'string') return resp.body
      return JSON.stringify(resp.body, undefined, 2)
    } else if (!AliHttp.HttpCodeBreak(resp.code)) {
      DebugLog.mSaveWarning('ApiFileDownText err=' + file_id + ' ' + (resp.code || ''), resp.body)
    }
    return ''
  }

  static async ApiBiXueTuBatch(_user_id: string, _drive_id: string, _file_id: string, _duration: number, _imageCount: number, _imageWidth: number): Promise<IVideoXBTUrl[]> {
    return []
  }

  static async ApiUpdateVideoTime(_user_id: string, _drive_id: string, _file_id: string, _play_cursor: number): Promise<IAliFileItem | undefined> {
    return undefined
  }
}
