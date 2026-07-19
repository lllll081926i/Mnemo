import type { ITokenInfo } from '../user/userstore'
import message from './message'
import { BAIDU_APP_ID, BAIDU_APP_SECRET } from '../secrets.generated'
import { buildDriveProviderUserId } from './driveProvider'

const BAIDU_AUTH_URL = 'https://openapi.baidu.com/oauth/2.0/authorize'
const BAIDU_TOKEN_URL = 'https://openapi.baidu.com/oauth/2.0/token'
const BAIDU_USER_INFO_URL = 'https://pan.baidu.com/rest/2.0/xpan/nas'
const BAIDU_QUOTA_URL = 'https://pan.baidu.com/api/quota'

const BAIDU_REDIRECT_URL = 'xbyboxplayer-oauth://callback'
const BAIDU_SCOPE = 'basic,netdisk'

const readStoredCredential = (key: string) => {
  try {
    return typeof localStorage === 'undefined' ? '' : localStorage.getItem(key) || ''
  } catch {
    return ''
  }
}

export const resolveBaiduOAuthCredentials = (clientId = '', clientSecret = '') => ({
  clientId: clientId.trim() || readStoredCredential('baidu_app_id').trim() || BAIDU_APP_ID.trim(),
  clientSecret: clientSecret.trim() || readStoredCredential('baidu_app_secret').trim() || BAIDU_APP_SECRET.trim()
})

const hashString = (value: string): string => {
  let hash = 0
  for (let i = 0; i < value.length; i++) {
    hash = (hash << 5) - hash + value.charCodeAt(i)
    hash |= 0
  }
  return Math.abs(hash).toString(36)
}

const normalizeToken = (data: any, clientId: string): ITokenInfo | null => {
  if (!data?.access_token) return null
  const expiresIn = Number(data.expires_in || 0)
  const expireTime = new Date(Date.now() + expiresIn * 1000).toISOString()
  return {
    tokenfrom: 'baidu',
    access_token: data.access_token,
    refresh_token: data.refresh_token || '',
    session_expires_in: 0,
    open_api_token_type: '',
    open_api_access_token: '',
    open_api_refresh_token: '',
    open_api_expires_in: 0,
    signature: '',
    device_id: clientId,
    expires_in: expiresIn,
    token_type: data.token_type || 'Bearer',
    user_id: buildDriveProviderUserId('baidu', hashString(data.refresh_token || data.access_token)),
    user_name: '百度网盘',
    avatar: '',
    nick_name: '百度网盘',
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
    name: '百度网盘',
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

export const buildBaiduAuthUrl = (clientId = '', state = `boxplayer_${Date.now()}`) => {
  const credentials = resolveBaiduOAuthCredentials(clientId)
  if (!credentials.clientId) throw new Error('请填写百度网盘 App ID')
  const params = new URLSearchParams({
    response_type: 'code',
    client_id: credentials.clientId,
    redirect_uri: BAIDU_REDIRECT_URL,
    scope: BAIDU_SCOPE,
    state,
    qrcode: '1',
    force_login: '1'
  })
  return `${BAIDU_AUTH_URL}?${params.toString()}`
}

const fetchBaiduUserInfo = async (accessToken: string) => {
  const params = new URLSearchParams({
    method: 'uinfo',
    access_token: accessToken,
    vip_version: 'v2'
  })
  const url = `${BAIDU_USER_INFO_URL}?${params.toString()}`
  const resp = await fetch(url, {
    headers: {
      'User-Agent': 'pan.baidu.com'
    }
  })
  if (!resp.ok) return null
  const data = await resp.json()
  if (data?.errno !== 0) return null
  return data
}

const fetchBaiduQuota = async (accessToken: string) => {
  const params = new URLSearchParams({
    access_token: accessToken,
    checkfree: '1',
    checkexpire: '1'
  })
  const url = `${BAIDU_QUOTA_URL}?${params.toString()}`
  const resp = await fetch(url, {
    headers: {
      'User-Agent': 'pan.baidu.com'
    }
  })
  if (!resp.ok) return null
  const data = await resp.json()
  if (data?.errno !== 0) return null
  return data
}

const applyBaiduAccount = async (token: ITokenInfo) => {
  try {
    const info = await fetchBaiduUserInfo(token.access_token)
    if (info) {
      const uk = info.uk ?? info.user_id
      if (uk !== undefined && uk !== null && String(uk)) token.user_id = buildDriveProviderUserId('baidu', uk)
      token.user_name = info.netdisk_name || info.baidu_name || token.user_name
      token.nick_name = info.netdisk_name || info.baidu_name || token.nick_name
      token.avatar = info.avatar_url || token.avatar
      if (info.vip_type === 2) token.vipname = 'SVIP'
      if (info.vip_type === 1) token.vipname = 'VIP'
    }
    const quota = await fetchBaiduQuota(token.access_token)
    if (quota) {
      if (typeof quota.total === 'number') token.total_size = quota.total
      if (typeof quota.used === 'number') token.used_size = quota.used
      if (typeof quota.free === 'number') token.free_size = quota.free
      if (typeof quota.expire === 'boolean') token.space_expire = quota.expire
      if (typeof quota.total === 'number' && typeof quota.used === 'number') token.spaceinfo = `${(quota.used / 1024 ** 3).toFixed(2)} GB / ${(quota.total / 1024 ** 3).toFixed(2)} GB`
    }
  } catch {
    // Account metadata is optional after a successful OAuth token exchange.
  }
}

const requestBaiduToken = async (params: URLSearchParams, fallback: string) => {
  const resp = await fetch(`${BAIDU_TOKEN_URL}?${params.toString()}`, {
    headers: { 'User-Agent': 'pan.baidu.com' }
  })
  const data = await resp.json().catch(() => undefined)
  if (!resp.ok || data?.error) {
    message.error(data?.error_description || data?.error_msg || data?.error || fallback)
    return null
  }
  return data
}

export const exchangeBaiduCodeForToken = async (code: string, clientId = '', clientSecret = ''): Promise<ITokenInfo | null> => {
  const credentials = resolveBaiduOAuthCredentials(clientId, clientSecret)
  if (!credentials.clientId || !credentials.clientSecret) {
    message.error('请填写百度网盘 App ID 和 App Secret')
    return null
  }
  const params = new URLSearchParams({
    grant_type: 'authorization_code',
    code,
    client_id: credentials.clientId,
    client_secret: credentials.clientSecret,
    redirect_uri: BAIDU_REDIRECT_URL
  })
  const data = await requestBaiduToken(params, '获取百度网盘 access_token 失败')
  const token = normalizeToken(data, credentials.clientId)
  if (token) {
    await applyBaiduAccount(token)
    const { default: UserDAL } = await import('../user/userdal')
    UserDAL.SaveUserToken(token)
  }
  return token
}

export const refreshBaiduAccessToken = async (refreshToken: string, clientId = '', clientSecret = ''): Promise<ITokenInfo | null> => {
  if (!refreshToken) return null
  const credentials = resolveBaiduOAuthCredentials(clientId, clientSecret)
  if (!credentials.clientId || !credentials.clientSecret) return null
  const params = new URLSearchParams({
    grant_type: 'refresh_token',
    refresh_token: refreshToken,
    client_id: credentials.clientId,
    client_secret: credentials.clientSecret
  })
  const data = await requestBaiduToken(params, '刷新百度网盘 Token 失败')
  const token = normalizeToken(data, credentials.clientId)
  if (token) await applyBaiduAccount(token)
  return token
}
