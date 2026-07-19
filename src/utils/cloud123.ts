import type { ITokenInfo } from '../user/userstore'
import message from './message'
import { CLOUD123_APP_ID, CLOUD123_APP_SECRET } from '../secrets.generated'
import { buildDriveProviderUserId } from './driveProvider'

export { CLOUD123_APP_ID, CLOUD123_APP_SECRET }

const CLOUD123_AUTH_URL = 'https://yun.123pan.com/auth'
const CLOUD123_ACCESS_TOKEN_URL = 'https://open-api.123pan.com/api/v1/oauth2/access_token'
const CLOUD123_USER_INFO_URL = 'https://open-api.123pan.com/api/v1/user/info'

const CLOUD123_REDIRECT_URL = 'xbyboxplayer-oauth://callback'

const CLOUD123_SCOPE = 'user:base,file:all:read,file:all:write'

const readStoredCredential = (key: string) => {
  try {
    return typeof localStorage === 'undefined' ? '' : localStorage.getItem(key) || ''
  } catch {
    return ''
  }
}

export const resolveCloud123OAuthCredentials = (clientId = '', clientSecret = '') => ({
  clientId: clientId.trim() || readStoredCredential('cloud123_app_id').trim() || CLOUD123_APP_ID.trim(),
  clientSecret: clientSecret.trim() || readStoredCredential('cloud123_app_secret').trim() || CLOUD123_APP_SECRET.trim()
})

const base64UrlDecode = (input: string): string => {
  const pad = '='.repeat((4 - (input.length % 4)) % 4)
  const base64 = (input + pad).replace(/-/g, '+').replace(/_/g, '/')
  try {
    return decodeURIComponent(
      atob(base64)
        .split('')
        .map((c) => '%' + c.charCodeAt(0).toString(16).padStart(2, '0'))
        .join('')
    )
  } catch {
    return ''
  }
}

const parseJwtPayload = (token: string): Record<string, any> | null => {
  const parts = token.split('.')
  if (parts.length < 2) return null
  try {
    const json = base64UrlDecode(parts[1])
    return JSON.parse(json)
  } catch {
    return null
  }
}

const hashString = (value: string): string => {
  let hash = 0
  for (let i = 0; i < value.length; i++) {
    hash = (hash << 5) - hash + value.charCodeAt(i)
    hash |= 0
  }
  return Math.abs(hash).toString(36)
}

export const buildCloud123AuthUrl = (clientId = '', state = `mnemo_${Date.now()}`) => {
  const credentials = resolveCloud123OAuthCredentials(clientId)
  if (!credentials.clientId) throw new Error('请填写 123 网盘 App ID')
  const params = new URLSearchParams({
    client_id: credentials.clientId,
    redirect_uri: CLOUD123_REDIRECT_URL,
    scope: CLOUD123_SCOPE,
    response_type: 'code',
    state
  })
  return `${CLOUD123_AUTH_URL}?${params.toString()}`
}

const normalizeToken = (data: any, clientId: string): ITokenInfo | null => {
  if (!data?.access_token) return null
  const payload = parseJwtPayload(data.access_token) || {}
  const userId = payload.user_id || payload.id || payload.uid || hashString(data.refresh_token || data.access_token)
  const expireTime = new Date(Date.now() + (data.expires_in || 0) * 1000).toISOString()

  return {
    tokenfrom: 'cloud123',
    access_token: data.access_token,
    refresh_token: data.refresh_token || '',
    session_expires_in: 0,
    open_api_token_type: '',
    open_api_access_token: '',
    open_api_refresh_token: '',
    open_api_expires_in: 0,
    signature: '',
    device_id: clientId,
    expires_in: data.expires_in || 0,
    token_type: data.token_type || 'Bearer',
    user_id: buildDriveProviderUserId('cloud123', userId),
    user_name: '123网盘',
    avatar: '',
    nick_name: '123网盘',
    default_drive_id: '',
    default_sbox_drive_id: '',
    resource_drive_id: '',
    backup_drive_id: '',
    sbox_drive_id: '',
    role: '',
    status: '',
    expire_time: expireTime,
    state: '',
    pin_setup: false,
    is_first_login: false,
    need_rp_verify: false,
    name: '123网盘',
    spu_id: '',
    is_expires: false,
    used_size: 0,
    total_size: 0,
    free_size: 0,
    space_expire: false,
    spaceinfo: '',
    vipname: '',
    vipIcon: '',
    vipexpire: '',
    pic_drive_id: '',
    signInfo: { signMon: -1, signDay: -1 }
  }
}

const fetchCloud123UserInfo = async (accessToken: string) => {
  const response = await fetch(CLOUD123_USER_INFO_URL, {
    headers: {
      Authorization: `Bearer ${accessToken}`,
      Platform: 'open_platform'
    }
  })
  if (!response.ok) return null
  const data = await response.json()
  return data?.data || null
}

const applyCloud123Account = async (token: ITokenInfo) => {
  try {
    const userInfo = await fetchCloud123UserInfo(token.access_token)
    if (!userInfo) return
    const uid = userInfo.uid ?? userInfo.userId
    if (uid) token.user_id = buildDriveProviderUserId('cloud123', uid)
    token.user_name = userInfo.nickname || token.user_name
    token.nick_name = userInfo.nickname || token.nick_name
    token.avatar = userInfo.headImage || token.avatar
    const totalSize = userInfo.spacePermanent
    const usedSize = userInfo.spaceUsed
    if (typeof totalSize === 'number') token.total_size = totalSize
    if (typeof usedSize === 'number') token.used_size = usedSize
    if (typeof totalSize === 'number' && typeof usedSize === 'number') token.spaceinfo = `${(usedSize / 1024 ** 3).toFixed(2)} GB / ${(totalSize / 1024 ** 3).toFixed(2)} GB`
    const vipCurrent = Array.isArray(userInfo.vipInfo) ? userInfo.vipInfo[0] : undefined
    if (vipCurrent?.vipLabel) token.vipname = vipCurrent.vipLabel
    if (vipCurrent?.endTime) token.vipexpire = vipCurrent.endTime
  } catch {
    // Account metadata is optional after a successful OAuth token exchange.
  }
}

const requestCloud123Token = async (body: URLSearchParams, fallback: string) => {
  const response = await fetch(CLOUD123_ACCESS_TOKEN_URL, {
    method: 'POST',
    headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
    body
  })
  const data = await response.json().catch(() => undefined)
  if (!response.ok || (data?.code && Number(data.code) !== 0) || data?.error) {
    message.error(data?.message || data?.error_description || data?.error || fallback)
    return null
  }
  return data?.data || data
}

export const exchangeCloud123CodeForToken = async (code: string, clientId = '', clientSecret = ''): Promise<ITokenInfo | null> => {
  const credentials = resolveCloud123OAuthCredentials(clientId, clientSecret)
  if (!credentials.clientId || !credentials.clientSecret) {
    message.error('请填写 123 网盘 App ID 和 App Secret')
    return null
  }
  const body = new URLSearchParams({
    client_id: credentials.clientId,
    client_secret: credentials.clientSecret,
    code,
    grant_type: 'authorization_code',
    redirect_uri: CLOUD123_REDIRECT_URL
  })

  const data = await requestCloud123Token(body, '获取 123 网盘 access_token 失败')
  const token = normalizeToken(data, credentials.clientId)
  if (token) {
    await applyCloud123Account(token)
    const { default: UserDAL } = await import('../user/userdal')
    UserDAL.SaveUserToken(token)
  }
  return token
}

export const refreshCloud123AccessToken = async (refreshToken: string, clientId = '', clientSecret = ''): Promise<ITokenInfo | null> => {
  const credentials = resolveCloud123OAuthCredentials(clientId, clientSecret)
  if (!credentials.clientId || !credentials.clientSecret) return null
  const body = new URLSearchParams({
    client_id: credentials.clientId,
    client_secret: credentials.clientSecret,
    refresh_token: refreshToken,
    grant_type: 'refresh_token'
  })
  const data = await requestCloud123Token(body, '刷新 123 网盘 Token 失败')
  const token = normalizeToken(data, credentials.clientId)
  if (token) await applyCloud123Account(token)
  return token
}
