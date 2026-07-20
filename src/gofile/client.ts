import type { ITokenInfo } from '../user/userstore'

const GOFILE_API = 'https://api.gofile.io'

export interface GofileResponse<T> {
  status?: string
  data?: T
  metadata?: {
    totalCount?: number
    totalPages?: number
    page?: number
    pageSize?: number
    hasNextPage?: boolean
  }
}

export interface GofileItem {
  id: string
  parentFolder?: string
  type: 'file' | 'folder'
  name: string
  size?: number
  createTime?: number
  modTime?: number
  link?: string
  md5?: string
  mimetype?: string
  childrenCount?: number
  children?: Record<string, GofileItem>
  directLinks?: Record<string, { directLink?: string; expireTime?: number }>
}

export const getGofileUserToken = async (userId: string): Promise<ITokenInfo | null> => {
  const { default: UserDAL } = await import('../user/userdal')
  let token = UserDAL.GetUserToken(userId)
  if (!token?.access_token) token = (await UserDAL.GetUserTokenFromDB(userId)) || token
  return token?.tokenfrom === 'gofile' && token.access_token ? token : null
}

export const gofileRequestWithToken = async <T>(accessToken: string, path: string, init: RequestInit = {}): Promise<GofileResponse<T>> => {
  const headers = new Headers(init.headers || {})
  headers.set('Authorization', `Bearer ${accessToken}`)
  if (init.body && !(init.body instanceof FormData) && !headers.has('Content-Type')) headers.set('Content-Type', 'application/json')
  const response = await fetch(path.startsWith('https://') ? path : `${GOFILE_API}${path}`, { ...init, headers })
  const payload = await response.json().catch(() => ({})) as GofileResponse<T>
  if (!response.ok || (payload.status && payload.status !== 'ok')) throw new Error(payload.status || `GoFile 请求失败 (${response.status})`)
  return payload
}

export const gofileRequest = async <T>(userId: string, path: string, init: RequestInit = {}) => {
  const token = await getGofileUserToken(userId)
  if (!token) throw new Error('未登录 GoFile')
  return gofileRequestWithToken<T>(token.access_token, path, init)
}

export const getGofileRootId = async (userId: string) => {
  const token = await getGofileUserToken(userId)
  if (!token?.provider_root_id) throw new Error('GoFile 根目录信息缺失，请重新登录')
  return token.provider_root_id
}
