import { gofileRequest } from './client'

export const buildGofileDirectLinkBody = (expiration = '') => {
  const expiresAt = expiration ? Math.floor(new Date(expiration).getTime() / 1000) : 0
  return Number.isFinite(expiresAt) && expiresAt > 0 ? { expireTime: expiresAt } : {}
}

export const apiGofileCreateDirectLink = async (userId: string, fileId: string, expiration = '') => {
  const response = await gofileRequest<{ directLink?: string }>(userId, `/contents/${encodeURIComponent(fileId)}/directlinks`, { method: 'POST', body: JSON.stringify(buildGofileDirectLinkBody(expiration)) })
  if (!response.data?.directLink) throw new Error('GoFile 直链创建失败')
  return response.data.directLink
}
