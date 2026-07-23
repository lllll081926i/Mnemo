import type { ITokenInfo } from '../user/userstore'
import { humanSize } from '../utils/format'
import message from '../utils/message'
import { DROPBOX_APP_KEY as CONFIGURED_DROPBOX_APP_KEY, DROPBOX_APP_SECRET as CONFIGURED_DROPBOX_APP_SECRET } from '../secrets.generated'
import { buildDriveProviderDriveId, buildDriveProviderUserId } from '../utils/driveProvider'

const BUILTIN_DROPBOX_APP_KEY = '5jcck7diasz0rqy'
const BUILTIN_DROPBOX_APP_SECRET = '1n9m04y2zx7bf26'
export const DROPBOX_APP_KEY = CONFIGURED_DROPBOX_APP_KEY.trim() || BUILTIN_DROPBOX_APP_KEY
export const DROPBOX_APP_SECRET = CONFIGURED_DROPBOX_APP_KEY.trim() ? CONFIGURED_DROPBOX_APP_SECRET.trim() : BUILTIN_DROPBOX_APP_SECRET

const DROPBOX_AUTH_URL = 'https://www.dropbox.com/oauth2/authorize'
const DROPBOX_TOKEN_URL = 'https://api.dropboxapi.com/oauth2/token'
const DROPBOX_ACCOUNT_URL = 'https://api.dropboxapi.com/2/users/get_current_account'
const DROPBOX_SPACE_URL = 'https://api.dropboxapi.com/2/users/get_space_usage'

export const resolveDropboxCredentials = (appKey = '', appSecret = '') => {
  const effectiveAppKey = appKey.trim() || DROPBOX_APP_KEY
  return {
    appKey: effectiveAppKey,
    appSecret: appSecret.trim() || (effectiveAppKey === DROPBOX_APP_KEY ? DROPBOX_APP_SECRET : '')
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

const base64UrlEncode = (bytes: Uint8Array): string => {
  let binary = ''
  bytes.forEach((byte) => {
    binary += String.fromCharCode(byte)
  })
  return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/g, '')
}

const sha256 = async (value: string): Promise<Uint8Array> => {
  const bytes = new TextEncoder().encode(value)
  const digest = await crypto.subtle.digest('SHA-256', bytes)
  return new Uint8Array(digest)
}

export const createDropboxPkceVerifier = (): string => {
  const bytes = new Uint8Array(48)
  crypto.getRandomValues(bytes)
  return base64UrlEncode(bytes)
}

export const buildDropboxAuthUrl = async (appKey: string, verifier: string, state: string, redirectUri: string): Promise<string> => {
  const credentials = resolveDropboxCredentials(appKey)
  // 与 rclone 授权方式保持一致：不手动指定 scope，令牌会获得该应用在 Dropbox 后台已授权的全部权限。
  // 手动传 scope 时，只要有一个未在该应用后台启用就会被 Dropbox 以 invalid_scope 直接拒绝登录。
  // 也不使用 force_reapprove，避免每次登录都被强制要求重新授权。
  const params = new URLSearchParams({
    client_id: credentials.appKey,
    response_type: 'code',
    redirect_uri: redirectUri,
    token_access_type: 'offline',
    state
  })
  // 始终启用 PKCE：即便使用内置 client_secret，也叠加 S256 code_challenge，
  // 避免授权码在回调链路中被截获后直接换取令牌（rclone 的 Dropbox 后端同样恒用 PKCE）。
  params.set('code_challenge', base64UrlEncode(await sha256(verifier)))
  params.set('code_challenge_method', 'S256')
  return `${DROPBOX_AUTH_URL}?${params.toString()}`
}

const emptyToken = (): ITokenInfo => ({
  tokenfrom: 'dropbox',
  access_token: '',
  refresh_token: '',
  session_expires_in: 0,
  open_api_token_type: '',
  open_api_access_token: '',
  open_api_refresh_token: '',
  open_api_expires_in: 0,
  signature: '',
  device_id: '',
  expires_in: 0,
  token_type: 'Bearer',
  user_id: '',
  user_name: 'Dropbox',
  avatar: '',
  nick_name: 'Dropbox',
  default_drive_id: 'dropbox',
  default_sbox_drive_id: '',
  resource_drive_id: '',
  backup_drive_id: '',
  sbox_drive_id: '',
  role: '',
  status: '',
  expire_time: '',
  state: '',
  pin_setup: false,
  is_first_login: false,
  need_rp_verify: false,
  name: 'Dropbox',
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
})

const dropboxJson = async <T>(url: string, init: RequestInit, fallback: string): Promise<T | null> => {
  const resp = await fetch(url, init)
  const data = await resp.json().catch(() => undefined)
  if (!resp.ok || data?.error) {
    message.error(data?.error_description || data?.error_summary || fallback)
    return null
  }
  return data as T
}

const applyDropboxAccount = async (token: ITokenInfo) => {
  const account = await dropboxJson<any>(
    DROPBOX_ACCOUNT_URL,
    {
      method: 'POST',
      headers: {
        Authorization: `Bearer ${token.access_token}`,
        'Content-Type': 'application/json'
      },
      body: 'null'
    },
    '获取 Dropbox 账号信息失败'
  )
  if (account) {
    const accountId = account.account_id || hashString(token.refresh_token || token.access_token)
    token.user_id = buildDriveProviderUserId('dropbox', accountId)
    token.default_drive_id = buildDriveProviderDriveId('dropbox', accountId)
    token.user_name = account.name?.display_name || account.email || token.user_name
    token.nick_name = account.name?.display_name || token.nick_name
    token.name = token.user_name
    token.avatar = account.profile_photo_url || token.avatar
  }

  const space = await dropboxJson<any>(
    DROPBOX_SPACE_URL,
    {
      method: 'POST',
      headers: {
        Authorization: `Bearer ${token.access_token}`,
        'Content-Type': 'application/json'
      },
      body: 'null'
    },
    '获取 Dropbox 空间信息失败'
  )
  if (space) {
    const used = Number(space.used || 0)
    const allocated = Number(space.allocation?.allocated || 0)
    token.used_size = used
    token.total_size = allocated
    token.free_size = allocated > used ? allocated - used : 0
    if (allocated || used) token.spaceinfo = `${humanSize(used)} / ${humanSize(allocated)}`
  }
}

const normalizeDropboxToken = (data: any, appKey: string): ITokenInfo | null => {
  if (!data?.access_token) return null
  const token = emptyToken()
  token.access_token = data.access_token
  token.refresh_token = data.refresh_token || ''
  token.expires_in = Number(data.expires_in || 14400)
  token.token_type = data.token_type || 'Bearer'
  const accountId = data.account_id || hashString(data.refresh_token || data.access_token)
  token.user_id = buildDriveProviderUserId('dropbox', accountId)
  token.default_drive_id = buildDriveProviderDriveId('dropbox', accountId)
  token.device_id = appKey.trim()
  token.expire_time = new Date(Date.now() + token.expires_in * 1000).toISOString()
  return token
}

export const exchangeDropboxCodeForToken = async (code: string, appKey: string, verifier: string, redirectUri: string): Promise<ITokenInfo | null> => {
  const credentials = resolveDropboxCredentials(appKey)
  const body = new URLSearchParams({
    code,
    grant_type: 'authorization_code',
    client_id: credentials.appKey,
    redirect_uri: redirectUri
  })
  // 授权 URL 恒带 PKCE challenge，换取令牌时就必须带上 code_verifier；有 secret 时一并附上。
  body.set('code_verifier', verifier)
  if (credentials.appSecret) body.set('client_secret', credentials.appSecret)
  const data = await dropboxJson<any>(
    DROPBOX_TOKEN_URL,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      body
    },
    '获取 Dropbox access_token 失败'
  )
  const token = normalizeDropboxToken(data, credentials.appKey)
  if (token) {
    await applyDropboxAccount(token)
    const { default: UserDAL } = await import('../user/userdal')
    UserDAL.SaveUserToken(token)
  }
  return token
}

const refreshPromises = new Map<string, Promise<ITokenInfo | null>>()

export const refreshDropboxAccessToken = async (token: ITokenInfo): Promise<ITokenInfo | null> => {
  const flightKey = token.user_id || token.refresh_token
  const inflight = refreshPromises.get(flightKey)
  if (inflight) return inflight
  const promise = (async (): Promise<ITokenInfo | null> => {
    try {
      const credentials = resolveDropboxCredentials(token.device_id)
      const appKey = credentials.appKey
      if (!appKey || !token.refresh_token) return null
      const body = new URLSearchParams({
        grant_type: 'refresh_token',
        refresh_token: token.refresh_token,
        client_id: appKey
      })
      if (credentials.appSecret) body.set('client_secret', credentials.appSecret)
      const data = await dropboxJson<any>(
        DROPBOX_TOKEN_URL,
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
          body
        },
        '刷新 Dropbox Token 失败'
      )
      if (!data?.access_token) return null
      token.access_token = data.access_token
      token.expires_in = Number(data.expires_in || token.expires_in || 14400)
      token.token_type = data.token_type || token.token_type || 'Bearer'
      token.expire_time = new Date(Date.now() + token.expires_in * 1000).toISOString()
      token.tokenfrom = 'dropbox'
      token.default_drive_id = token.default_drive_id || buildDriveProviderDriveId('dropbox', token.user_id)
      await applyDropboxAccount(token)
      return token
    } finally {
      refreshPromises.delete(flightKey)
    }
  })()
  refreshPromises.set(flightKey, promise)
  return promise
}

export const applyDropboxQuota = async (token: ITokenInfo): Promise<boolean> => {
  if (!token.access_token) return false
  await applyDropboxAccount(token)
  return true
}
