import type { ITokenInfo } from '../user/userstore'

export const GOOGLE_DRIVE_API = 'https://www.googleapis.com/drive/v3'

const sleep = (ms: number) => new Promise<void>((resolve) => setTimeout(resolve, ms))

// 共享 OAuth 项目容易撞到 drive.googleapis.com 的每分钟配额，遇到 429/quota 错误退避重试
export const isGoogleDriveQuotaError = (status: number, payload: any): boolean => {
  if (status === 429 || status === 403) {
    const text = JSON.stringify(payload?.error || {})
    if (/quota|rateLimit|userRateLimit/i.test(text)) return true
    if (status === 429) return true
  }
  return false
}

const fetchWithQuotaRetry = async (url: string, init: RequestInit, attempts = 3): Promise<Response> => {
  let response = await fetch(url, init)
  for (let attempt = 1; attempt <= attempts; attempt++) {
    if (response.status !== 429 && response.status !== 403) break
    const payload = await response.clone().json().catch(() => ({}))
    if (!isGoogleDriveQuotaError(response.status, payload)) break
    await sleep(attempt * 1500)
    response = await fetch(url, init)
  }
  return response
}

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
  const response = await fetchWithQuotaRetry(pathOrUrl.startsWith('https://') ? pathOrUrl : `${GOOGLE_DRIVE_API}${pathOrUrl}`, { ...init, headers })
  if (response.status === 204) return undefined as T
  const payload = await response.json().catch(() => ({}))
  if (!response.ok || payload?.error) throw new Error(payload?.error?.message || `Google Drive 请求失败 (${response.status})`)
  return payload as T
}
