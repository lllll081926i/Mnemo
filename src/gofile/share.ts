import { gofileRequest } from './client'

export const apiGofileCreateDirectLink = async (userId: string, fileId: string) => {
  const response = await gofileRequest<{ directLink?: string }>(userId, `/contents/${encodeURIComponent(fileId)}/directlinks`, { method: 'POST', body: '{}' })
  if (!response.data?.directLink) throw new Error('GoFile 直链创建失败')
  return response.data.directLink
}
