import type { IAliGetFileModel } from '../aliapi/alimodels'
import getFileIcon from '../aliapi/fileicon'
import { humanDateTimeDateStr, humanSize } from '../utils/format'
import { HanToPin } from '../utils/utils'
import message from '../utils/message'
import { pikpakApiFetch } from './auth'

export type PikPakMediaItem = {
  media_id?: string
  media_name?: string
  resolution_name?: string
  category?: string
  is_origin?: boolean
  is_visible?: boolean
  is_default?: boolean
  need_more_quota?: boolean
  vip_types?: unknown[]
  link?: { url?: string }
  video?: { height?: number | string; width?: number | string }
}

export type PikPakFileItem = {
  id: string
  parent_id?: string
  kind?: string
  name: string
  size?: string | number
  modified_time?: string
  created_time?: string
  thumbnail_link?: string
  web_content_link?: string
  medias?: PikPakMediaItem[]
  links?: { 'application/octet-stream'?: { url?: string } }
}

export type PikPakDownloadInfo = {
  item: PikPakFileItem | null
  streamUrl: string
  downloadUrl: string
  error: string
}

export type PikPakVideoQuality = {
  html: string
  quality: string
  height: number
  width: number
  label: string
  value: string
  url: string
  type?: string
}

type PikPakFileListResp = {
  files?: PikPakFileItem[]
  next_page_token?: string
}

const API_URL = 'https://api-drive.mypikpak.com/drive/v1/files'

const isPikPakDir = (item: PikPakFileItem) => (item.kind || '').includes('folder')

const getPikPakStreamUrl = (item: PikPakFileItem) => {
  return item.medias?.find(media => media?.link?.url)?.link?.url || ''
}

// 原始媒体流（progressive，可拖进度）——比 web_content_link 更适合在线播放
const getPikPakOriginMediaUrl = (item: PikPakFileItem) => {
  const medias = Array.isArray(item.medias) ? item.medias : []
  const origin = medias.find(media => media?.is_origin && media?.link?.url)
  return origin?.link?.url || ''
}

const getPikPakWebContentUrl = (item: PikPakFileItem) => item.links?.['application/octet-stream']?.url || item.web_content_link || ''

const encodeDescription = (item: PikPakFileItem) => {
  const downloadUrl = getPikPakStreamUrl(item) || getPikPakWebContentUrl(item)
  return downloadUrl ? `pikpak_download:${encodeURIComponent(downloadUrl)}` : ''
}

const parsePikPakError = (data: any, fallback: string) => {
  return data?.error_description || data?.message || data?.error || fallback
}

const getPikPakToken = async (user_id: string) => {
  const { default: UserDAL } = await import('../user/userdal')
  let token = UserDAL.GetUserToken(user_id)
  if (!token?.access_token) {
    const dbToken = await UserDAL.GetUserTokenFromDB(user_id)
    if (dbToken) token = dbToken
  }
  return token
}

const PIKPAK_API_HOST = 'https://api-drive.mypikpak.com'
const pikpakVipCache = new Map<string, { isVip: boolean; expiresAt: number }>()

/** 查询并缓存 PikPak 账号会员状态（10 分钟），会员才能原画云播，非会员最高 720p 转码。 */
export const isPikPakVipAccount = async (user_id: string): Promise<boolean> => {
  const cached = pikpakVipCache.get(user_id)
  if (cached && cached.expiresAt > Date.now()) return cached.isVip
  let isVip = false
  const token = await getPikPakToken(user_id)
  if (token?.access_token) {
    try {
      const resp = await pikpakApiFetch(token, 'GET:/drive/v1/privilege/vip', `${PIKPAK_API_HOST}/drive/v1/privilege/vip`)
      const data = await resp.json().catch(() => undefined)
      const info = data?.data || {}
      const statusOk = String(info.status || '').toLowerCase() === 'ok'
      const typeVip = !!info.type && String(info.type).toLowerCase() !== 'novip'
      const expireOk = info.expire ? new Date(info.expire).getTime() > Date.now() : false
      isVip = statusOk && (typeVip || expireOk)
    } catch {
      isVip = false
    }
  }
  pikpakVipCache.set(user_id, { isVip, expiresAt: Date.now() + 10 * 60 * 1000 })
  return isVip
}

const detectPikPakStreamType = (url: string) => {
  const pathname = String(url || '').split('?')[0].split('#')[0].toLowerCase()
  if (pathname.endsWith('.m3u8')) return 'm3u8'
  if (pathname.endsWith('.mpd')) return 'mpd'
  if (pathname.endsWith('.ts')) return 'ts'
  return ''
}

const pikPakQualityTier = (height: number): { quality: string; html: string } => {
  if (height >= 2000) return { quality: 'QHD', html: '2560p' }
  if (height >= 1000) return { quality: 'FHD', html: '1080P' }
  if (height >= 700) return { quality: 'HD', html: '720P' }
  if (height >= 500) return { quality: 'SD', html: '540P' }
  return { quality: 'LD', html: '480P' }
}

/** 把 PikPak 的 medias 转码流映射成清晰度列表；会员含原画，非会员只保留 ≤720p 的转码。 */
export const buildPikPakVideoQualities = (item: PikPakFileItem, isVip: boolean): PikPakVideoQuality[] => {
  const tiers = new Map<string, PikPakVideoQuality>()
  for (const media of Array.isArray(item.medias) ? item.medias : []) {
    const url = media?.link?.url || ''
    if (!url || media.is_visible === false) continue
    if (!isVip && (media.is_origin || media.need_more_quota)) continue
    const height = Number(media.video?.height || 0) || 0
    const width = Number(media.video?.width || 0) || 0
    if (!isVip && height > 720) continue
    const tier = pikPakQualityTier(height)
    const existing = tiers.get(tier.quality)
    if (!existing || height > existing.height) {
      tiers.set(tier.quality, { html: tier.html, quality: tier.quality, height, width, label: tier.html, value: tier.quality, url, type: detectPikPakStreamType(url) })
    }
  }
  const qualities = ['QHD', 'FHD', 'HD', 'SD', 'LD'].map((key) => tiers.get(key)).filter((item): item is PikPakVideoQuality => !!item)
  if (isVip) {
    const originUrl = getPikPakOriginMediaUrl(item) || getPikPakWebContentUrl(item)
    if (originUrl) qualities.unshift({ html: '原画', quality: 'Origin', height: 0, width: 0, label: '原画', value: 'Origin', url: originUrl, type: detectPikPakStreamType(originUrl) })
  }
  return qualities
}

export const apiPikPakFileList = async (
  user_id: string,
  parentId: string,
  limit = 100,
  pageToken = '',
  trashed = false
): Promise<{ items: PikPakFileItem[]; nextPageToken: string }> => {
  const token = await getPikPakToken(user_id)
  if (!token?.access_token) {
    message.error('未登录 PikPak')
    return { items: [], nextPageToken: '' }
  }
  const params = new URLSearchParams()
  params.set('thumbnail_size', 'SIZE_MEDIUM')
  params.set('limit', String(limit))
  params.set('with_audit', 'true')
  if (trashed) {
    // 回收站：parent_id 传 * 且不带 phase 过滤（回收站条目不一定处于 COMPLETE 阶段）
    params.set('parent_id', '*')
    params.set('filters', JSON.stringify({ trashed: { eq: true } }))
  } else {
    params.set('filters', JSON.stringify({
      trashed: { eq: false },
      phase: { eq: 'PHASE_TYPE_COMPLETE' }
    }))
    if (parentId && parentId !== 'pikpak_root') params.set('parent_id', parentId)
  }
  if (pageToken) params.set('page_token', pageToken)
  const resp = await pikpakApiFetch(token, 'GET:/drive/v1/files', `${API_URL}?${params.toString()}`)
  const data = await resp.json().catch(() => undefined) as PikPakFileListResp | any
  if (!resp.ok || data?.error) {
    message.error(data?.error_description || data?.message || '获取 PikPak 文件列表失败')
    return { items: [], nextPageToken: '' }
  }
  return {
    items: Array.isArray(data?.files) ? data.files : [],
    nextPageToken: data?.next_page_token || ''
  }
}

export const apiPikPakFileDetail = async (user_id: string, fileId: string): Promise<PikPakFileItem | null> => {
  const token = await getPikPakToken(user_id)
  if (!token?.access_token) return null
  const resp = await pikpakApiFetch(token, `GET:/drive/v1/files/${fileId}`, `${API_URL}/${fileId}?thumbnail_size=SIZE_LARGE`)
  const data = await resp.json().catch(() => undefined)
  if (!resp.ok || data?.error) return null
  return data as PikPakFileItem
}

export const apiPikPakDownloadInfo = async (user_id: string, fileId: string): Promise<PikPakDownloadInfo> => {
  const result: PikPakDownloadInfo = { item: null, streamUrl: '', downloadUrl: '', error: '获取 PikPak 下载地址失败' }
  const token = await getPikPakToken(user_id)
  if (!token?.access_token) {
    result.error = '请先登录 PikPak'
    return result
  }
  try {
    const resp = await pikpakApiFetch(token, `GET:/drive/v1/files/${fileId}`, `${API_URL}/${fileId}?thumbnail_size=SIZE_LARGE`)
    const data = await resp.json().catch(() => undefined)
    if (!resp.ok || data?.error) {
      result.error = parsePikPakError(data, result.error)
      return result
    }
    const item = data as PikPakFileItem
    result.item = item
    result.streamUrl = getPikPakStreamUrl(item)
    result.downloadUrl = getPikPakWebContentUrl(item) || getPikPakOriginMediaUrl(item)
    result.error = result.streamUrl || result.downloadUrl ? '' : '获取 PikPak 下载地址失败'
  } catch (err: any) {
    result.error = err?.message || result.error
  }
  return result
}

export const mapPikPakFileToAliModel = (item: PikPakFileItem, drive_id: string, parentId: string): IAliGetFileModel => {
  const isDir = isPikPakDir(item)
  const name = item.name || ''
  const ext = isDir ? '' : (name.split('.').pop() || '')
  const time = new Date(item.modified_time || item.created_time || '').getTime() || 0
  const timeStr = time ? humanDateTimeDateStr(new Date(time).toISOString()) : ''
  const size = Number(item.size || 0)
  let category = ''
  let icon = 'iconfile-folder'
  if (!isDir) {
    const iconInfo = getFileIcon('', ext, ext, '', size)
    category = iconInfo[0]
    icon = iconInfo[1]
  }
  return {
    __v_skip: true,
    drive_id,
    file_id: String(item.id || ''),
    parent_file_id: item.parent_id || parentId,
    name,
    namesearch: HanToPin(name),
    ext,
    mime_type: '',
    mime_extension: ext,
    category,
    icon,
    file_count: 0,
    size,
    sizeStr: humanSize(size),
    time,
    timeStr,
    starred: false,
    isDir,
    thumbnail: isDir ? '' : item.thumbnail_link || '',
    description: encodeDescription(item)
  }
}
