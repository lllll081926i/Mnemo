/**
 * 统一的文件扩展名解析。
 *
 * 背景：各网盘此前用 `name.split('.').pop()` 取扩展名，有两个致命问题：
 * 1. 文件名里没有 `.` 时，整个文件名会被当成扩展名（超长垃圾 ext），导致
 *    getFileIcon 识别失败 → 文件类型显示为“其他” → 无法预览/播放。
 * 2. 文件名过长或尾部包含特殊字符时，取到的“后缀”并不是真实扩展名。
 *
 * 这里优先使用网盘 API 返回的权威字段（如 PikPak 的 file_extension），
 * 其次从文件名末段提取并做合法性校验，最后用 mime_type 兜底，
 * 保证“拿不到后缀”时依然能识别出视频/音频/图片等类型。
 */

/** 扩展名里不应该出现的字符（出现即判定为不是真后缀） */
const IMPLAUSIBLE_EXT_CHARS = /[\s/\\?%#&@!$'"()*,;:=+[\]{}<>|]/

/** 常见扩展名最长不超过 12（如 tar.gz 取 gz、webm、jpeg 等） */
const MAX_EXT_LENGTH = 12

/** 规范化一个扩展名字符串；不合法时返回 '' */
export const normalizeFileExt = (value: string): string => {
  const ext = String(value || '').trim().replace(/^\.+/, '').toLowerCase()
  if (!ext || ext.length > MAX_EXT_LENGTH) return ''
  if (IMPLAUSIBLE_EXT_CHARS.test(ext)) return ''
  return ext
}

/** 从文件名提取扩展名；没有合法后缀时返回 ''（绝不返回整段文件名） */
export const extFromFileName = (name: string): string => {
  const base = String(name || '').trim()
  const dot = base.lastIndexOf('.')
  if (dot <= 0 || dot >= base.length - 1) return ''
  return normalizeFileExt(base.slice(dot + 1))
}

/** mime_type → 扩展名兜底映射（覆盖常见的可预览类型） */
const MIME_EXT_MAP: Record<string, string> = {
  'video/mp4': 'mp4',
  'video/x-matroska': 'mkv',
  'video/webm': 'webm',
  'video/x-msvideo': 'avi',
  'video/quicktime': 'mov',
  'video/x-ms-wmv': 'wmv',
  'video/x-flv': 'flv',
  'video/mpeg': 'mpg',
  'video/mp2t': 'ts',
  'video/3gpp': '3gp',
  'video/3gpp2': '3g2',
  'video/x-m4v': 'm4v',
  'video/x-ms-asf': 'asf',
  'video/vnd.rn-realvideo': 'rmvb',
  'application/vnd.apple.mpegurl': 'm3u8',
  'application/x-mpegurl': 'm3u8',
  'application/dash+xml': 'mpd',
  'audio/mpeg': 'mp3',
  'audio/flac': 'flac',
  'audio/x-flac': 'flac',
  'audio/wav': 'wav',
  'audio/x-wav': 'wav',
  'audio/ape': 'ape',
  'audio/ogg': 'ogg',
  'audio/aac': 'aac',
  'audio/mp4': 'm4a',
  'audio/x-m4a': 'm4a',
  'audio/x-ms-wma': 'wma',
  'image/jpeg': 'jpg',
  'image/png': 'png',
  'image/gif': 'gif',
  'image/bmp': 'bmp',
  'image/webp': 'webp',
  'image/svg+xml': 'svg',
  'image/x-icon': 'ico',
  'image/heic': 'heic',
  'image/heif': 'heif',
  'application/pdf': 'pdf'
}

/** 从 mime_type 推断扩展名（识别不出时返回 ''） */
export const extFromMimeType = (mimeType: string): string => {
  const mime = String(mimeType || '').split(';')[0].trim().toLowerCase()
  return MIME_EXT_MAP[mime] || ''
}

/**
 * 解析文件的真实扩展名：
 * 1. 网盘 API 权威字段（file_extension）
 * 2. 文件名末段（经过合法性校验）
 * 3. mime_type 兜底
 */
export const resolveFileExt = (name: string, fileExtension: string = '', mimeType: string = ''): string => {
  return normalizeFileExt(fileExtension) || extFromFileName(name) || extFromMimeType(mimeType)
}
