import { apiGoogleDriveFileDetail } from './dirfilelist'
import { getGoogleDriveToken, GOOGLE_DRIVE_API } from './client'

const exportMimeTypes: Record<string, string> = {
  'application/vnd.google-apps.document': 'application/pdf',
  'application/vnd.google-apps.spreadsheet': 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
  'application/vnd.google-apps.presentation': 'application/pdf',
  'application/vnd.google-apps.drawing': 'application/pdf'
}

export const apiGoogleDriveDownloadInfo = async (userId: string, fileId: string) => {
  const token = await getGoogleDriveToken(userId)
  if (!token) return { error: '未登录 Google Drive', url: '', size: 0, headers: {} as Record<string, string> }
  const item = await apiGoogleDriveFileDetail(userId, fileId)
  if (item.mimeType === 'application/vnd.google-apps.folder') return { error: '文件夹不能直接下载', url: '', size: 0, headers: {} }
  const exportMime = exportMimeTypes[item.mimeType]
  const url = exportMime
    ? `${GOOGLE_DRIVE_API}/files/${encodeURIComponent(fileId)}/export?mimeType=${encodeURIComponent(exportMime)}`
    : `${GOOGLE_DRIVE_API}/files/${encodeURIComponent(fileId)}?alt=media&supportsAllDrives=true`
  const headers: Record<string, string> = { Authorization: `Bearer ${token.access_token}` }
  return { error: '', url, size: Number(item.size || 0), headers }
}
