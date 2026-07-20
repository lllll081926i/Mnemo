import { googleDriveRequest } from './client'

export const apiGoogleDriveCreateShare = async (userId: string, fileId: string) => {
  await googleDriveRequest(userId, `/files/${encodeURIComponent(fileId)}/permissions?supportsAllDrives=true`, { method: 'POST', body: JSON.stringify({ type: 'anyone', role: 'reader' }) })
  const item = await googleDriveRequest<{ webViewLink?: string }>(userId, `/files/${encodeURIComponent(fileId)}?fields=webViewLink&supportsAllDrives=true`)
  if (!item.webViewLink) throw new Error('Google Drive 分享链接获取失败')
  return item.webViewLink
}
