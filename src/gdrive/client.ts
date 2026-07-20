import type { ITokenInfo } from '../user/userstore'

export const GOOGLE_DRIVE_API = 'https://www.googleapis.com/drive/v3'

export const getGoogleDriveToken = async (userId: string): Promise<ITokenInfo | null> => {
  const { default: UserDAL } = await import('../user/userdal')
  let token = UserDAL.GetUserToken(userId)
  if (!token?.access_token) token = (await UserDAL.GetUserTokenFromDB(userId)) || token
  return token?.tokenfrom === 'gdrive' && token.access_token ? token : null
}

export const googleDriveRequest = async <T>(userId: string, pathOrUrl: string, init: RequestInit = {}): Promise<T> => {
  const token = await getGoogleDriveToken(userId)
  if (!token) throw new Error('未登录 Google Drive')
  const headers = new Headers(init.headers || {})
  headers.set('Authorization', `Bearer ${token.access_token}`)
  if (init.body && !headers.has('Content-Type')) headers.set('Content-Type', 'application/json')
  const response = await fetch(pathOrUrl.startsWith('https://') ? pathOrUrl : `${GOOGLE_DRIVE_API}${pathOrUrl}`, { ...init, headers })
  if (response.status === 204) return undefined as T
  const payload = await response.json().catch(() => ({}))
  if (!response.ok || payload?.error) throw new Error(payload?.error?.message || `Google Drive 请求失败 (${response.status})`)
  return payload as T
}
