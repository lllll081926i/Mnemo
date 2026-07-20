import path from 'path'
import { useSettingStore } from '../store'
import DebugLog from '../utils/debuglog'
import { GetExpiresTime, HanToPin } from '../utils/utils'
import AliHttp from './alihttp'
import { IAliFileItem, IAliGetDirModel, IAliGetFileModel, IAliGetForderSizeModel } from './alimodels'
import AliDirFileList from './dirfilelist'
import { ICompilationList, IDownloadUrl, IOfficePreViewUrl, IVideoPreviewUrl, IVideoXBTUrl } from './models'
import { DecodeEncName, GetDriveType, isAliyunUser, isCloud139User, isCloud189User, isGuangyaUser, isPikPakUser, isQuarkUser } from './utils'
import { getProxyUrl, getRawUrl } from '../utils/proxyhelper'
import { apiPikPakDownloadInfo, apiPikPakFileDetail, mapPikPakFileToAliModel } from '../pikpak/dirfilelist'
import { apiQuarkDownloadUrl, apiQuarkFileDetail, mapQuarkFileToAliModel } from '../quark/dirfilelist'
import { apiCloud139DownloadInfo, apiCloud139FileDetail, mapCloud139FileToAliModel } from '../cloud139/dirfilelist'
import { apiCloud189DownloadInfo, apiCloud189FileDetail, mapCloud189FileToAliModel } from '../cloud189/dirfilelist'
import { apiGuangyaDownloadInfo, apiGuangyaFileDetail, mapGuangyaFileToAliModel } from '../guangya/dirfilelist'
import TreeStore from '../store/treestore'
import UserDAL from '../user/userdal'
import { ITokenInfo } from '../user/userstore'
import { getWebDavConnection, getWebDavConnectionId, getWebDavDownloadUrl, getWebDavRequestHeaders, isWebDavDrive } from '../utils/webdavClient'
import { getS3Connection, getS3ConnectionId, getS3DownloadUrl, getS3ObjectInfo, isS3Drive } from '../utils/s3Client'
import { getAlipanVideoPromotionReason } from '../utils/alipanPromotionRules'
import { canUseAliyunPreviewApi, getDriveProviderLabel, resolveDriveProvider } from '../utils/driveProvider'
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
    if (provider === 'quark') {
      if (file_id === 'quark_root' || file_id === '0') {
        return {
          drive_id,
          file_id: 'quark_root',
          parent_file_id: '',
          name: '网盘文件',
          type: 'folder',
          isDir: true
        }
      }
      const detail = await apiQuarkFileDetail(user_id, file_id)
      if (!detail) return undefined
      const mapped = mapQuarkFileToAliModel(detail, drive_id, detail.pdir_fid || 'quark_root') as any
      mapped.type = mapped.isDir ? 'folder' : 'file'
      return mapped
    }
    if (provider === '139') {
      if (file_id === 'cloud139_root' || file_id === '/' || file_id === '0') {
        return { drive_id, file_id: 'cloud139_root', parent_file_id: '', name: '网盘文件', type: 'folder', isDir: true }
      }
      const detail = await apiCloud139FileDetail(user_id, file_id)
      if (!detail) return undefined
      const mapped = mapCloud139FileToAliModel(detail, drive_id, detail.parentFileId || detail.parentCatalogId || 'cloud139_root') as any
      mapped.type = mapped.isDir ? 'folder' : 'file'
      return mapped
    }
    if (provider === '189') {
      if (file_id === 'cloud189_root' || file_id === '-11' || file_id === '0') {
        return { drive_id, file_id: 'cloud189_root', parent_file_id: '', name: '网盘文件', type: 'folder', isDir: true }
      }
      const detail = await apiCloud189FileDetail(user_id, file_id)
      if (!detail) return undefined
      const mapped = mapCloud189FileToAliModel(detail, drive_id, detail.parentId || detail.parentFolderId || 'cloud189_root') as any
      mapped.type = mapped.isDir ? 'folder' : 'file'
      return mapped
    }
    if (provider === 'guangya') {
      if (file_id === 'guangya_root' || file_id === '0' || file_id === '/') {
        return { drive_id, file_id: 'guangya_root', parent_file_id: '', name: '网盘文件', type: 'folder', isDir: true }
      }
      const detail = await apiGuangyaFileDetail(user_id, file_id)
      if (!detail) return undefined
      const mapped = mapGuangyaFileToAliModel(detail, drive_id, detail.parentId || detail.parentFileId || 'guangya_root') as any
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
    if (provider !== 'aliyun') return undefined
    let url = ''
    let postData = {}
    if (!ispic) {
      url = 'adrive/v1.0/openFile/get'
      postData = {
        drive_id: drive_id,
        file_id: file_id,
        image_thumbnail_width: 100,
        video_thumbnail_width: 100,
        video_thumbnail_time: 120000
      }
    } else {
      url = 'v2/file/get'
      postData = {
        drive_id: drive_id,
        file_id: file_id,
        url_expire_sec: 14400,
        office_thumbnail_process: 'image/resize,w_400/format,jpeg',
        image_thumbnail_process: 'image/resize,w_400/format,jpeg',
        image_url_process: 'image/resize,w_1920/format,jpeg',
        video_thumbnail_process: 'video/snapshot,t_106000,f_jpg,ar_auto,m_fast,w_400'
      }
    }
    const resp = await AliHttp.Post(url, postData, user_id, '')

    if (AliHttp.IsSuccess(resp.code)) {
      let fileInfo = resp.body as IAliFileItem
      if (fileInfo.name.toLowerCase() === 'default') {
        fileInfo.name = '备份盘'
      } else if (fileInfo.name.toLowerCase() === 'resource') {
        fileInfo.name = '资源盘'
      } else if (fileInfo.name.toLowerCase() === 'alibum') {
        fileInfo.name = '相册'
      } else {
        fileInfo.name = DecodeEncName(user_id, fileInfo).name
      }
      return fileInfo
    } else if (AliHttp.HttpCodeBreak(resp.code)) {
      return (resp.body.message || resp.body) as string
    } else if (!AliHttp.HttpCodeBreak(resp.code)) {
      DebugLog.mSaveWarning('ApiFileInfo err=' + file_id + ' ' + (resp.code || ''), resp.body)
    }
    return '网络错误'
  }

  static async ApiFileInfoByPath(user_id: string, drive_id: string, file_path: string): Promise<IAliFileItem | undefined> {
    if (!user_id || !drive_id || !file_path) return undefined
    if (!file_path.startsWith('/')) file_path = '/' + file_path
    const url = 'v2/file/get_by_path'
    const postData = {
      drive_id: drive_id,
      file_path: file_path,
      url_expire_sec: 14400,
      office_thumbnail_process: 'image/resize,w_400/format,jpeg',
      image_thumbnail_process: 'image/resize,w_400/format,jpeg',
      image_url_process: 'image/resize,w_1920/format,jpeg',
      video_thumbnail_process: 'video/snapshot,t_106000,f_jpg,ar_auto,m_fast,w_400'
    }
    const resp = await AliHttp.Post(url, postData, user_id, '')

    if (AliHttp.IsSuccess(resp.code)) {
      let fileInfo = resp.body as IAliFileItem
      fileInfo.name = DecodeEncName(user_id, fileInfo).name
      return fileInfo
    } else if (!AliHttp.HttpCodeBreak(resp.code)) {
      DebugLog.mSaveWarning('ApiFileInfoByPath err=' + file_path + ' ' + (resp.code || ''), resp.body)
    }
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
    if (provider === 'quark') {
      const info = await apiQuarkDownloadUrl(user_id, file_id)
      if (info.error) return info.error
      return {
        drive_id,
        file_id,
        expire_time: GetExpiresTime(info.url),
        url: info.url,
        size: Number(info.size || 0)
      }
    }
    if (provider === '139') {
      const info = await apiCloud139DownloadInfo(user_id, file_id)
      if (info.error) return info.error
      return { drive_id, file_id, expire_time: GetExpiresTime(info.url), url: info.url, size: Number(info.size || 0) }
    }
    if (provider === '189') {
      const info = await apiCloud189DownloadInfo(user_id, file_id)
      if (info.error) return info.error
      return { drive_id, file_id, expire_time: GetExpiresTime(info.url), url: info.url, size: Number(info.size || 0) }
    }
    if (provider === 'guangya') {
      const info = await apiGuangyaDownloadInfo(user_id, file_id)
      if (info.error) return info.error
      return { drive_id, file_id, expire_time: GetExpiresTime(info.url), url: info.url, size: Number(info.size || 0) }
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
    if (provider !== 'aliyun') return `${getDriveProviderLabel(provider)} 暂不支持下载`
    const data: IDownloadUrl = {
      drive_id: drive_id,
      file_id: file_id,
      expire_time: 0,
      url: '',
      size: 0
    }
    let url = ''
    // 处理OpenApi无法访问相册
    let isPic = GetDriveType(user_id, drive_id).name === 'pic'
    if (!isPic) {
      url = 'adrive/v1.0/openFile/getDownloadUrl'
    } else {
      url = 'v2/file/get_download_url'
    }
    const postData: any = {
      drive_id: drive_id,
      file_id: file_id,
      expire_sec: expire_sec
    }
    if (isPic) {
      delete postData.expire_sec
    }
    const resp = await AliHttp.Post(url, postData, user_id, '')
    if (AliHttp.IsSuccess(resp.code)) {
      data.url = resp.body.cdn_url || resp.body.url
      data.size = resp.body.size
      data.expire_time = GetExpiresTime(data.url)
      return data
    } else if (resp.body.code == 'NotFound.FileId') {
      return '文件已从网盘中彻底删除'
    } else if (resp.body.code == 'ForbiddenFileInTheRecycleBin') {
      return '文件已放入回收站'
    } else if (AliHttp.HttpCodeBreak(resp.code)) {
      return (resp.body.message || resp.body) as string
    } else if (resp.body.code) {
      return resp.body.code as string
    } else if (!AliHttp.HttpCodeBreak(resp.code)) {
      DebugLog.mSaveWarning('ApiFileDownloadUrl err=' + file_id + ' ' + (resp.code || ''), resp.body)
    }
    return '网络错误'
  }

  static async ApiVideoPreviewUrl(user_id: string, drive_id: string, file_id: string, promotionSkuCode = ''): Promise<IVideoPreviewUrl | string> {
    if (!drive_id || !file_id) return '参数错误'
    const provider = await resolveFileProvider(user_id, drive_id)
    const detectVideoType = (url: string, fallback = '') => {
      const lower = String(url || '')
        .split('?')[0]
        .split('#')[0]
        .toLowerCase()
      if (lower.endsWith('.m3u8')) return 'm3u8'
      if (lower.endsWith('.mpd')) return 'mpd'
      if (lower.endsWith('.ts')) return 'ts'
      return fallback
    }
  if (provider === 'webdav' || provider === 's3') {
      return '暂无转码信息'
    }
    if (!user_id || !drive_id || !file_id) return '参数错误'
    if (provider === 'pikpak') {
      return '暂无转码信息'
    }
    if (provider === 'quark') {
      return '暂无转码信息'
    }
    if (provider === 'guangya') {
      return '暂无转码信息'
    }
    if (provider === '139') {
      return '暂无转码信息'
    }
    if (provider === '189') {
      return '暂无转码信息'
    }
    if (!canUseAliyunPreviewApi(provider)) return `${getDriveProviderLabel(provider)} 暂无转码信息`
    let url = ''
    let need_open_api = true
    if (need_open_api) {
      url = 'adrive/v1.0/openFile/getVideoPreviewPlayInfo'
    } else {
      url = 'v2/file/get_video_preview_play_info'
    }
    const postData = {
      drive_id: drive_id,
      file_id: file_id,
      category: 'live_transcoding',
      template_id: '',
      get_subtitle_info: true,
      url_expire_sec: 14400
    }
    const resp = await AliHttp.Post(url, postData, user_id, '')

    if (resp.body.code == 'VideoPreviewWaitAndRetry') {
      return '视频正在转码中，稍后重试'
    }
    if (resp.body.code == 'ExceedCapacityForbidden') {
      return '容量超限限制播放，需要扩容或者删除不必要的文件释放空间'
    }
    const data: IVideoPreviewUrl = {
      drive_id: drive_id,
      file_id: file_id,
      size: 0,
      expire_time: 0,
      width: 0,
      height: 0,
      duration: 0,
      qualities: [],
      subtitles: []
    }
    if (AliHttp.IsSuccess(resp.code)) {
      const subtitle = resp.body.video_preview_play_info?.live_transcoding_subtitle_task_list || []
      for (let i = 0, maxi = subtitle.length; i < maxi; i++) {
        if (subtitle[i].status == 'finished') {
          data.subtitles.push({ language: subtitle[i].language, url: subtitle[i].url })
        }
      }
      const taskList = resp.body.video_preview_play_info?.live_transcoding_task_list || []
      const promotionReason = getAlipanVideoPromotionReason(resp.body)
      const qualityMap: any = {
        LD: { label: '低清', value: '480p' },
        SD: { label: '标清', value: '540P' },
        HD: { label: '高清', value: '720P' },
        FHD: { label: '全高清', value: '1080p' },
        QHD: { label: '超高清', value: '2560p' }
      }
      for (let i = 0, maxi = taskList.length; i < maxi; i++) {
        if (!taskList[i].url) {
          continue
        }
        let templateId = taskList[i].template_id
        if (templateId && taskList[i].status == 'finished') {
          let quality = qualityMap[templateId]
          data.qualities.push({
            html: quality.label + ' ' + quality.value,
            quality: templateId,
            height: taskList[i].template_height || 0,
            width: taskList[i].template_width || 0,
            label: quality.label,
            value: quality.value,
            url: taskList[i].url,
            type: detectVideoType(taskList[i].url, 'm3u8')
          })
        }
      }
      data.qualities = data.qualities.sort((a, b) => b.width - a.width)
      data.duration = Math.floor(resp.body.video_preview_play_info?.meta?.duration || 0)
      data.width = resp.body.video_preview_play_info?.meta?.width || 0
      data.height = resp.body.video_preview_play_info?.meta?.height || 0
      if (data.qualities.length === 0) return promotionReason || '暂无转码信息'
      data.expire_time = GetExpiresTime(data.qualities[0].url)
      return data
    } else if (!AliHttp.HttpCodeBreak(resp.code)) {
      DebugLog.mSaveWarning('ApiVideoPreviewUrl err=' + file_id + ' ' + (resp.code || ''), resp.body)
    }
    return '网络错误'
  }

  static async ApiListByFileInfo(user_id: string, drive_id: string, file_id: string, limit?: number): Promise<ICompilationList[] | undefined> {
    if (!user_id || !drive_id || !file_id) return undefined
    const url = 'adrive/v2/video/compilation/listByFileInfo'
    const postData = { drive_id: drive_id, file_id: file_id, limit: limit || 100 }
    const resp = await AliHttp.Post(url, postData, user_id, '')
    const data: ICompilationList[] = []

    if (AliHttp.IsSuccess(resp.code)) {
      const items = resp.body.items || []
      for (const item of items) {
        data.push({
          name: item.name,
          type: item.type,
          width: item.video_media_metadata?.width || 0,
          height: item.video_media_metadata?.height || 0,
          duration: Math.floor(item?.duration || 0),
          category: item.category,
          drive_id: item.drive_id,
          file_id: item.file_id,
          file_extension: item.file_extension,
          url: item.url,
          expire_time: GetExpiresTime(item.url),
          play_cursor: Math.floor(item?.play_cursor || 0),
          compilation_id: item.compilation_id
        })
      }
      return data
    } else if (!AliHttp.HttpCodeBreak(resp.code)) {
      DebugLog.mSaveWarning('ApiListByFileInfo err=' + file_id + ' ' + (resp.code || ''), resp.body)
    }
  }

  static async ApiAudioPreviewUrl(user_id: string, drive_id: string, file_id: string): Promise<IDownloadUrl | string> {
    if (!user_id || !drive_id || !file_id) return '参数错误'
    const provider = await resolveFileProvider(user_id, drive_id)
    if (!canUseAliyunPreviewApi(provider)) return AliFile.ApiFileDownloadUrl(user_id, drive_id, file_id, 14400)

    const url = 'v2/file/get_audio_play_info'

    const postData = { drive_id: drive_id, file_id: file_id, url_expire_sec: 14400 }
    const resp = await AliHttp.Post(url, postData, user_id, '')

    if (resp.body.code == 'AudioPreviewWaitAndRetry') {
      return '音频正在转码中，稍后重试'
    }

    const data: IDownloadUrl = {
      drive_id: drive_id,
      file_id: file_id,
      expire_time: 0,
      url: '',
      size: 0
    }
    if (AliHttp.IsSuccess(resp.code)) {
      const template_list = resp.body.template_list || []
      if (!data.url) {
        for (let i = 0, maxi = template_list.length; i < maxi; i++) {
          if (template_list[i].template_id && template_list[i].template_id == 'HQ' && template_list[i].status == 'finished') {
            data.url = template_list[i].url
          }
        }
      }
      if (!data.url) {
        for (let i = 0, maxi = template_list.length; i < maxi; i++) {
          if (template_list[i].template_id && template_list[i].template_id == 'LQ' && template_list[i].status == 'finished') {
            data.url = template_list[i].url
          }
        }
      }

      return data
    } else if (!AliHttp.HttpCodeBreak(resp.code)) {
      DebugLog.mSaveWarning('ApiAudioPreviewUrl err=' + file_id + ' ' + (resp.code || ''), resp.body)
    }
    return '网络错误'
  }

  static async ApiOfficePreViewUrl(user_id: string, drive_id: string, file_id: string): Promise<IOfficePreViewUrl | undefined> {
    if (!user_id || !drive_id || !file_id) return undefined
    const provider = await resolveFileProvider(user_id, drive_id)
    if (!canUseAliyunPreviewApi(provider)) return undefined
    const url = 'v2/file/get_office_preview_url'
    const postData = { drive_id: drive_id, file_id: file_id, url_expire_sec: 14400 }
    const resp = await AliHttp.Post(url, postData, user_id, '')
    const data: IOfficePreViewUrl = {
      drive_id: drive_id,
      file_id: file_id,
      access_token: '',
      preview_url: ''
    }
    if (AliHttp.IsSuccess(resp.code)) {
      data.access_token = resp.body.access_token
      data.preview_url = resp.body.preview_url
      return data
    } else if (!AliHttp.HttpCodeBreak(resp.code)) {
      DebugLog.mSaveWarning('ApiOfficePreViewUrl err=' + file_id + ' ' + (resp.code || ''), resp.body)
    }
    return undefined
  }

  static async ApiGetFile(user_id: string, drive_id: string, file_id: string): Promise<IAliGetFileModel | undefined> {
    if (!user_id || !drive_id || !file_id) return undefined
    const provider = await resolveFileProvider(user_id, drive_id)
    if (provider === 'onedrive' || provider === 'dropbox' || provider === 'gdrive' || provider === 'gofile' || provider === 'webdav' || provider === 's3') {
      const info = await AliFile.ApiFileInfo(user_id, drive_id, file_id)
      return typeof info === 'object' ? info as IAliGetFileModel : undefined
    }
    if (isPikPakUser(user_id) || drive_id === 'pikpak') {
      const detail = await apiPikPakFileDetail(user_id, file_id)
      if (!detail) return undefined
      return mapPikPakFileToAliModel(detail, drive_id, detail.parent_id || 'pikpak_root')
    }
    if (isQuarkUser(user_id) || drive_id === 'quark') {
      const detail = await apiQuarkFileDetail(user_id, file_id)
      if (!detail) return undefined
      return mapQuarkFileToAliModel(detail, drive_id, detail.pdir_fid || 'quark_root')
    }
    if (isCloud139User(user_id) || drive_id === 'cloud139') {
      const detail = await apiCloud139FileDetail(user_id, file_id)
      if (!detail) return undefined
      return mapCloud139FileToAliModel(detail, drive_id, detail.parentFileId || detail.parentCatalogId || 'cloud139_root')
    }
    if (isCloud189User(user_id) || drive_id === 'cloud189') {
      const detail = await apiCloud189FileDetail(user_id, file_id)
      if (!detail) return undefined
      return mapCloud189FileToAliModel(detail, drive_id, detail.parentId || detail.parentFolderId || 'cloud189_root')
    }
    if (isGuangyaUser(user_id) || drive_id === 'guangya') {
      const detail = await apiGuangyaFileDetail(user_id, file_id)
      if (!detail) return undefined
      return mapGuangyaFileToAliModel(detail, drive_id, detail.parentId || detail.parentFileId || 'guangya_root')
    }
    const url = 'v2/file/get'
    const postData = {
      drive_id: drive_id,
      file_id: file_id,
      url_expire_sec: 14400,
      office_thumbnail_process: 'image/resize,w_400/format,jpeg',
      image_thumbnail_process: 'image/resize,w_400/format,jpeg',
      image_url_process: 'image/resize,w_1920/format,jpeg',
      video_thumbnail_process: 'video/snapshot,t_106000,f_jpg,ar_auto,m_fast,w_400'
    }
    const resp = await AliHttp.Post(url, postData, user_id, '')

    if (AliHttp.IsSuccess(resp.code)) {
      return AliDirFileList.getFileInfo(user_id, resp.body as IAliFileItem, '')
    } else if (!AliHttp.HttpCodeBreak(resp.code)) {
      DebugLog.mSaveWarning('ApiGetFile err=' + file_id + ' ' + (resp.code || ''), resp.body)
    }
    return undefined
  }

  static async ApiFileGetPath(user_id: string, drive_id: string, file_id: string): Promise<IAliGetDirModel[]> {
    if (!user_id || !drive_id || !file_id) return []
    const provider = await resolveFileProvider(user_id, drive_id)
    if (provider !== 'aliyun') return TreeStore.GetDirPath(drive_id, file_id) as IAliGetDirModel[]
    if (
      isPikPakUser(user_id) ||
      drive_id === 'pikpak' ||
      isQuarkUser(user_id) ||
      drive_id === 'quark' ||
      isCloud139User(user_id) ||
      drive_id === 'cloud139' ||
      isCloud189User(user_id) ||
      drive_id === 'cloud189' ||
      isGuangyaUser(user_id) ||
      drive_id === 'guangya'
    ) {
      return TreeStore.GetDirPath(drive_id, file_id) as IAliGetDirModel[]
    }
    const url = 'adrive/v1/file/get_path'
    const postData = {
      drive_id: drive_id,
      file_id: file_id
    }
    const resp = await AliHttp.Post(url, postData, user_id, '')
    const driveType = GetDriveType(user_id, drive_id)
    let items = resp.body.items
    if (AliHttp.IsSuccess(resp.code) && items && items.length > 0) {
      const list: IAliGetDirModel[] = []
      list.push({
        __v_skip: true,
        drive_id: drive_id,
        album_id: '',
        file_id: driveType.key,
        parent_file_id: '',
        name: driveType.title,
        namesearch: HanToPin(driveType.title),
        size: 0,
        time: 0,
        description: ''
      } as IAliGetDirModel)
      for (let i = items.length - 1; i >= 0; i--) {
        const item = items[i]
        if (item.name === 'Default' || item.name === 'resource' || item.name === 'alibum') {
          continue
        }
        list.push({
          __v_skip: true,
          drive_id: item.drive_id,
          album_id: '',
          file_id: item.file_id,
          parent_file_id: item.parent_file_id || '',
          name: DecodeEncName(user_id, item).name,
          namesearch: HanToPin(item.name),
          size: item.size || 0,
          time: new Date(item.updated_at).getTime(),
          description: item.description || ''
        } as IAliGetDirModel)
      }
      return list
    } else if (!AliHttp.HttpCodeBreak(resp.code)) {
      DebugLog.mSaveWarning('ApiFileGetPath err=' + file_id + ' ' + (resp.code || ''), resp.body)
    }
    return []
  }

  static async ApiFileGetPathString(user_id: string, drive_id: string, file_id: string, dirsplit: string): Promise<string> {
    if (!user_id || !drive_id || !file_id) return ''
    const provider = await resolveFileProvider(user_id, drive_id)
    if (provider !== 'aliyun') {
      const pathList = TreeStore.GetDirPath(drive_id, file_id)
      return pathList.map((item) => item.name).filter(Boolean).join(dirsplit)
    }
    if (
      isPikPakUser(user_id) ||
      drive_id === 'pikpak' ||
      isGuangyaUser(user_id) ||
      drive_id === 'guangya' ||
      isCloud139User(user_id) ||
      drive_id === 'cloud139' ||
      isCloud189User(user_id) ||
      drive_id === 'cloud189'
    ) {
      const pathList = TreeStore.GetDirPath(drive_id, file_id)
      const pathNames = pathList.map((item) => item.name).filter((name) => name)
      return pathNames.join(dirsplit)
    }
    if (file_id.includes('root')) {
      if (file_id.startsWith('backup')) {
        return '备份盘'
      } else if (file_id.startsWith('resource')) {
        return '资源盘'
      } else if (file_id.startsWith('pic')) {
        return '相册'
      }
    }
    const url = 'adrive/v1/file/get_path'
    const postData = {
      drive_id: drive_id,
      file_id: file_id
    }
    const resp = await AliHttp.Post(url, postData, user_id, '')
    if (AliHttp.IsSuccess(resp.code) && resp.body.items && resp.body.items.length > 0) {
      const driveType = GetDriveType(user_id, drive_id)
      const list: string[] = [driveType.title]
      for (let i = resp.body.items.length - 1; i >= 0; i--) {
        const item = resp.body.items[i]
        list.push(DecodeEncName(user_id, item).name)
      }
      return list.join(dirsplit)
    } else if (!AliHttp.HttpCodeBreak(resp.code)) {
      DebugLog.mSaveWarning('ApiFileGetPathString err=' + file_id + ' ' + (resp.code || ''), resp.body)
    }
    return ''
  }

  static async ApiFileGetFolderSize(user_id: string, drive_id: string, file_id: string): Promise<IAliGetForderSizeModel | undefined> {
    if (!user_id || !drive_id || !file_id) return undefined
    const provider = await resolveFileProvider(user_id, drive_id)
    if (!canUseAliyunPreviewApi(provider)) {
      return { size: 0, folder_count: 0, file_count: 0, reach_limit: undefined }
    }
    const url = 'adrive/v1/file/get_folder_size_info'

    const postData = {
      drive_id: drive_id,
      file_id: file_id
    }
    const resp = await AliHttp.Post(url, postData, user_id, '')

    if (AliHttp.IsSuccess(resp.code)) {
      return resp.body as IAliGetForderSizeModel
    } else if (!AliHttp.HttpCodeBreak(resp.code)) {
      DebugLog.mSaveWarning('ApiFileGetFolderSize err=' + file_id + ' ' + (resp.code || ''), resp.body)
    }
    return { size: 0, folder_count: 0, file_count: 0, reach_limit: false }
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

  static async ApiBiXueTuBatch(user_id: string, drive_id: string, file_id: string, duration: number, imageCount: number, imageWidth: number): Promise<IVideoXBTUrl[]> {
    if (!user_id || !drive_id || !file_id) return []
    if (duration <= 0) return []
    const batchList: string[] = []
    let mtime = 0
    let subtime = Math.floor(duration / (imageCount + 2))
    if (subtime < 1) subtime = 1

    const imgList: IVideoXBTUrl[] = []
    for (let i = 0; i < imageCount; i++) {
      mtime += subtime
      if (mtime > duration) break
      const postData = {
        body: {
          drive_id: drive_id,
          file_id: file_id,
          url_expire_sec: 14400,
          video_thumbnail_process: 'video/snapshot,t_' + mtime.toString() + '000,f_jpg,ar_auto,m_fast,w_' + imageWidth.toString()
        },
        headers: { 'Content-Type': 'application/json' },
        id: (i.toString() + file_id).substr(0, file_id.length),
        method: 'POST',
        url: '/file/get'
      }
      batchList.push(JSON.stringify(postData))

      const time =
        Math.floor(mtime / 3600)
          .toString()
          .padStart(2, '0') +
        ':' +
        Math.floor((mtime % 3600) / 60)
          .toString()
          .padStart(2, '0') +
        ':' +
        Math.floor(mtime % 60)
          .toString()
          .padStart(2, '0')
      imgList.push({ time, url: '' } as IVideoXBTUrl)
    }

    let postData = '{"requests":['
    let add = 0
    for (let i = 0, maxi = batchList.length; i < maxi; i++) {
      if (add > 0) postData = postData + ','
      add++
      postData = postData + batchList[i]
    }
    postData += '],"resource":"file"}'

    const url = 'adrive/v4/batch'
    const resp = await AliHttp.Post(url, postData, user_id, '')
    if (AliHttp.IsSuccess(resp.code)) {
      const responses = resp.body.responses
      for (let i = 0, maxi = responses.length; i < maxi; i++) {
        const status = responses[i].status as number
        if (status >= 200 && status <= 205) {
          imgList[i].url = responses[i].body?.thumbnail || ''
        } else {
          console.log(responses[i])
        }
      }
    } else if (!AliHttp.HttpCodeBreak(resp.code)) {
      DebugLog.mSaveWarning('ApiBiXueTuBatch err=' + file_id + ' ' + (resp.code || ''), resp.body)
    }
    return imgList
  }

  static async ApiUpdateVideoTime(user_id: string, drive_id: string, file_id: string, play_cursor: number): Promise<IAliFileItem | undefined> {
    if (!useSettingStore().uiAutoPlaycursorVideo) return
    if (!user_id || !drive_id || !file_id) return undefined
    if (isWebDavDrive(drive_id) || isS3Drive(drive_id)) return undefined
    if (isPikPakUser(user_id) || drive_id === 'pikpak') return undefined
    if (!isAliyunUser(user_id)) return undefined
    let url = ''
    let need_open_api = true
    if (need_open_api) {
      url = 'adrive/v1.0/openFile/video/updateRecord'
    } else {
      url = 'adrive/v2/video/update'
    }
    const postVideoData = {
      drive_id: drive_id,
      file_id: file_id,
      play_cursor: Math.trunc(play_cursor).toString()
    }
    const respvideo = await AliHttp.Post(url, postVideoData, user_id, '')
    if (AliHttp.IsSuccess(respvideo.code)) {
      return respvideo.body as IAliFileItem
    } else {
      DebugLog.mSaveWarning('ApiUpdateVideoTime err=' + file_id + ' ' + (respvideo.code || ''), respvideo.body)
    }
    return undefined
  }
}
