import type { IAliGetFileModel } from '../aliapi/alimodels'
import getFileIcon from '../aliapi/fileicon'
import { humanDateTimeDateStr, humanSize } from '../utils/format'
import { resolveFileExt } from '../utils/filetype'
import { GetExpiresTime, HanToPin } from '../utils/utils'
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
  priority?: number
  link?: { url?: string; token?: string; expire?: string | number; type?: string }
  video?: { height?: number | string; width?: number | string; video_type?: string }
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
  /** PikPak 权威扩展名（如 ".MP4"），文件名异常/过长时依然可靠 */
  file_extension?: string
  /** PikPak 权威 MIME（如 "video/mp4"），用于无后缀文件的类型识别 */
  mime_type?: string
  medias?: PikPakMediaItem[]
  links?: { 'application/octet-stream'?: { url?: string; token?: string; expire?: string | number; type?: string } }
}

export type PikPakDownloadInfo = {
  item: PikPakFileItem | null
  streamUrl: string
  downloadUrl: string
  /** 下载链接过期时间（毫秒）；URL 无 expire 参数时回退到 JSON link.expire（对齐 rclone Link.Valid()）。 */
  expireTime: number
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
  /** PikPak 的临时媒体链接需要经过本地 Range 代理，避免长视频直连失败。 */
  forceProxy?: boolean
  /** 链接过期时间（毫秒）；URL 无 expire 参数时回退到 JSON link.expire（对齐 rclone Link.Valid()）。 */
  expireTime?: number
}

type PikPakFileListResp = {
  files?: PikPakFileItem[]
  next_page_token?: string
}

const API_URL = 'https://api-drive.mypikpak.com/drive/v1/files'

const isPikPakDir = (item: PikPakFileItem) => (item.kind || '').includes('folder')

const isUsablePikPakUrl = (url: string) => /^https?:\/\//i.test(String(url || ''))

const getPikPakFid = (url: string) => {
  try {
    return new URL(url).searchParams.get('fid') || ''
  } catch {
    return ''
  }
}

// rclone 的 Link.Valid() 在 URL 没有 expire 参数时回退到 JSON 的 link.expire 字段（RFC3339）。
// PikPak 转码/原画链接有时不带 expire 查询参数，此时只能靠这个字段判定过期。
const parseRfc3339Expire = (expire: string | number | undefined): number => {
  if (!expire) return 0
  const t = new Date(expire).getTime()
  return Number.isFinite(t) && t > 0 ? t : 0
}

// 链接过期时间（毫秒）：优先读 URL 的 expire 查询参数，回退到 JSON 的 link.expire（RFC3339）
const getPikPakLinkExpireTime = (url: string, expire: string | number | undefined): number => GetExpiresTime(url) || parseRfc3339Expire(expire)

const getPikPakOriginalLink = (item: PikPakFileItem) => item.links?.['application/octet-stream']?.url || item.web_content_link || ''

const getPikPakStreamUrl = (item: PikPakFileItem) => {
  const mediaUrl = item.medias?.find(media => isUsablePikPakUrl(media?.link?.url || ''))?.link?.url || ''
  return mediaUrl || getPikPakOriginalLink(item)
}

// 原始媒体流（progressive，可拖进度）。只接受与原始文件 fid 一致的媒体链接；
// rclone 的 PikPak backend 也用这个校验规避 API 偶发返回的失效媒体链接。
const getPikPakOriginMediaUrl = (item: PikPakFileItem) => {
  const medias = Array.isArray(item.medias) ? item.medias : []
  const originalUrl = getPikPakOriginalLink(item)
  const originalFid = getPikPakFid(originalUrl)
  const origin = medias.find((media) => {
    const url = media?.link?.url || ''
    if (!media?.is_origin || !isUsablePikPakUrl(url)) return false
    return !originalFid || getPikPakFid(url) === originalFid
  })
  return origin?.link?.url || originalUrl
}

const getPikPakWebContentUrl = (item: PikPakFileItem) => getPikPakOriginalLink(item)

// 为实际选中的下载链接找到对应的 JSON expire 字段（原画取 links，转码取匹配 medias），
// 再结合 URL 的 expire 参数得出过期时间，供代理缓存判定刷新（对齐 rclone Link.Valid()）。
const getPikPakDownloadExpireTime = (item: PikPakFileItem | null, url: string): number => {
  if (!item || !url) return 0
  let jsonExpire: string | number | undefined
  if (url === getPikPakOriginalLink(item)) {
    jsonExpire = item.links?.['application/octet-stream']?.expire
  } else {
    jsonExpire = (Array.isArray(item.medias) ? item.medias : []).find((media) => media?.link?.url === url)?.link?.expire
  }
  return getPikPakLinkExpireTime(url, jsonExpire)
}

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

const detectPikPakStreamType = (url: string, hint = '') => {
  const normalizedHint = String(hint || '').toLowerCase()
  if (normalizedHint.includes('mpegurl') || normalizedHint.includes('hls') || normalizedHint === 'm3u8') return 'm3u8'
  if (normalizedHint.includes('dash') || normalizedHint === 'mpd') return 'mpd'
  // PikPak 转码流的 video_type 是 "mpegts"（见 rclone backend/pikpak/api 注释）
  if (normalizedHint.includes('mpegts') || normalizedHint.includes('mp2t') || normalizedHint === 'ts') return 'ts'
  const pathname = String(url || '').split('?')[0].split('#')[0].toLowerCase()
  if (pathname.endsWith('.m3u8')) return 'm3u8'
  if (pathname.endsWith('.mpd')) return 'mpd'
  if (pathname.endsWith('.ts')) return 'ts'
  return ''
}

const parsePikPakResolutionHeight = (media: PikPakMediaItem) => {
  const direct = Number(media.video?.height || 0) || 0
  if (direct > 0) return direct
  const label = `${media.resolution_name || ''} ${media.media_name || ''}`.toLowerCase()
  const pixelHeight = label.match(/(?:^|[^0-9])([0-9]{3,4})\s*[pP](?:\b|$)/i)
  if (pixelHeight) return Number(pixelHeight[1]) || 0
  const kHeight = label.match(/(?:^|[^0-9])([248])\s*k(?:\b|$)/i)
  if (kHeight) return Number(kHeight[1]) * 512
  return 0
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
  // rclone 的 setMetaData 只对 fid 与原始文件一致的媒体链接采信；转码档位同样套用这个校验，
  // 规避 API 偶发返回的失效/陈旧转码链接（原始 fid 未知时不校验，保持兼容）。
  const originalFid = getPikPakFid(getPikPakOriginalLink(item))
  for (const media of Array.isArray(item.medias) ? item.medias : []) {
    const url = media?.link?.url || ''
    if (!url || media.is_visible === false) continue
    if (media.is_origin || media.category === 'category_origin') continue
    if (!isVip && media.need_more_quota) continue
    if (originalFid && getPikPakFid(url) !== originalFid) continue
    const height = parsePikPakResolutionHeight(media)
    const width = Number(media.video?.width || 0) || 0
    if (!isVip && height > 720) continue
    const tier = pikPakQualityTier(height)
    const existing = tiers.get(tier.quality)
    const priority = Number(media.priority || 0) || 0
    const existingPriority = Number((existing as any)?.priority || 0) || 0
    if (!existing || height > existing.height || (height === existing.height && priority > existingPriority)) {
      tiers.set(tier.quality, { html: tier.html, quality: tier.quality, height, width, label: tier.html, value: tier.quality, url, type: detectPikPakStreamType(url, media.link?.type || media.video?.video_type), forceProxy: true, expireTime: getPikPakLinkExpireTime(url, media.link?.expire) })
    }
  }
  const qualities = ['QHD', 'FHD', 'HD', 'SD', 'LD'].map((key) => tiers.get(key)).filter((item): item is PikPakVideoQuality => !!item)
  if (isVip) {
    const originUrl = getPikPakOriginMediaUrl(item) || getPikPakWebContentUrl(item)
    if (originUrl) qualities.unshift({ html: '原画', quality: 'Origin', height: 0, width: 0, label: '原画', value: 'Origin', url: originUrl, type: detectPikPakStreamType(originUrl), forceProxy: true, expireTime: getPikPakLinkExpireTime(originUrl, item.links?.['application/octet-stream']?.expire) })
  } else {
    // 非会员也把原画放在最后兼底：长视频可能没有低码率转码，或转码流（MPEG-TS）播放失败时，
    // 还能像 rclone 一样播原始文件链接（经本地代理）。
    const originUrl = getPikPakOriginMediaUrl(item) || getPikPakWebContentUrl(item)
    if (originUrl) qualities.push({ html: '原画', quality: 'Origin', height: 0, width: 0, label: '原画', value: 'Origin', url: originUrl, type: detectPikPakStreamType(originUrl), forceProxy: true, expireTime: getPikPakLinkExpireTime(originUrl, item.links?.['application/octet-stream']?.expire) })
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

// 文件详情里是否已经有可用的媒体/下载链接。刚离线或转码中的文件首次返回可能两者都空。
const hasUsablePikPakMediaOrLink = (item: PikPakFileItem | null): boolean => {
  if (!item) return false
  if (isPikPakDir(item)) return true
  if (Array.isArray(item.medias) && item.medias.some((media) => isUsablePikPakUrl(media?.link?.url || ''))) return true
  return isUsablePikPakUrl(getPikPakOriginalLink(item))
}

// rclone 的 getFile 在链接还没就绪时会 sleep 后重试；这里对刚离线/转码的长视频做同样的重试。
const fetchPikPakDetailWithRetry = async (fetchFn: () => Promise<PikPakFileItem | null>, maxRetries = 2, delayMs = 3000): Promise<PikPakFileItem | null> => {
  let data: PikPakFileItem | null = null
  for (let attempt = 0; attempt <= maxRetries; attempt++) {
    data = await fetchFn()
    if (hasUsablePikPakMediaOrLink(data)) return data
    if (attempt < maxRetries) await new Promise((resolve) => setTimeout(resolve, delayMs))
  }
  return data
}

export const apiPikPakFileDetail = async (user_id: string, fileId: string): Promise<PikPakFileItem | null> => {
  const token = await getPikPakToken(user_id)
  if (!token?.access_token) return null
  return fetchPikPakDetailWithRetry(async () => {
    const resp = await pikpakApiFetch(token, `GET:/drive/v1/files/${fileId}`, `${API_URL}/${fileId}?thumbnail_size=SIZE_LARGE`)
    const data = await resp.json().catch(() => undefined)
    if (!resp.ok || data?.error) return null
    return data as PikPakFileItem
  })
}

export const apiPikPakDownloadInfo = async (user_id: string, fileId: string): Promise<PikPakDownloadInfo> => {
  const result: PikPakDownloadInfo = { item: null, streamUrl: '', downloadUrl: '', expireTime: 0, error: '获取 PikPak 下载地址失败' }
  const token = await getPikPakToken(user_id)
  if (!token?.access_token) {
    result.error = '请先登录 PikPak'
    return result
  }
  try {
    // 刚离线/转码中的文件首次返回可能还没有可用链接，像 rclone 的 getFile 那样稍等重试
    const item = await fetchPikPakDetailWithRetry(async () => {
      const resp = await pikpakApiFetch(token, `GET:/drive/v1/files/${fileId}`, `${API_URL}/${fileId}?thumbnail_size=SIZE_LARGE`)
      const data = await resp.json().catch(() => undefined)
      if (!resp.ok || data?.error) {
        result.error = parsePikPakError(data, result.error)
        return null
      }
      return data as PikPakFileItem
    })
    if (!item) return result
    result.item = item
    result.streamUrl = getPikPakStreamUrl(item)
    result.downloadUrl = getPikPakWebContentUrl(item) || getPikPakOriginMediaUrl(item)
    result.expireTime = getPikPakDownloadExpireTime(item, result.downloadUrl || result.streamUrl)
    result.error = result.streamUrl || result.downloadUrl ? '' : '获取 PikPak 下载地址失败'
  } catch (err: any) {
    result.error = err?.message || result.error
  }
  return result
}

export const mapPikPakFileToAliModel = (item: PikPakFileItem, drive_id: string, parentId: string): IAliGetFileModel => {
  const isDir = isPikPakDir(item)
  const name = item.name || ''
  const mimeType = item.mime_type || ''
  // 优先用 PikPak 返回的 file_extension / mime_type，文件名无后缀或过长时也能正确识别类型
  const ext = isDir ? '' : resolveFileExt(name, item.file_extension, mimeType)
  const time = new Date(item.modified_time || item.created_time || '').getTime() || 0
  const timeStr = time ? humanDateTimeDateStr(new Date(time).toISOString()) : ''
  const size = Number(item.size || 0)
  let category = ''
  let icon = 'iconfile-folder'
  if (!isDir) {
    const iconInfo = getFileIcon('', ext, ext, mimeType, size)
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
    mime_type: mimeType,
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
