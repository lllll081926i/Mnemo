import type { ITokenInfo } from '../user/userstore'
import { DRIVE115_APP_ID, DRIVE115_APP_SECRET } from '../secrets.generated'

export { DRIVE115_APP_ID, DRIVE115_APP_SECRET }

const DRIVE115_AUTH_DEVICE_URL = 'https://passportapi.115.com/open/authDeviceCode'
const DRIVE115_QR_STATUS_URL = 'https://qrcodeapi.115.com/get/status/'
const DRIVE115_TOKEN_URL = 'https://passportapi.115.com/open/deviceCodeToToken'
const DRIVE115_REFRESH_URL = 'https://passportapi.115.com/open/refreshToken'

const readStoredCredential = (key: string) => {
  try {
    return typeof localStorage === 'undefined' ? '' : localStorage.getItem(key) || ''
  } catch {
    return ''
  }
}

export const resolve115Credentials = (clientId = '', clientSecret = '') => ({
  clientId: clientId.trim() || readStoredCredential('drive115_client_id').trim() || DRIVE115_APP_ID.trim(),
  clientSecret: clientSecret.trim() || readStoredCredential('drive115_client_secret').trim() || DRIVE115_APP_SECRET.trim()
})

export const isDrive115ApiSuccess = (data: any): boolean => {
  if (!data || data.status === false || data.state === false || data.error) return false
  if (data.errno !== undefined && data.errno !== null && Number(data.errno) !== 0) return false
  if (data.code !== undefined && data.code !== null && data.code !== '' && Number(data.code) !== 0) return false
  return true
}

export const unwrapDrive115List = <T = any>(data: any): T[] => {
  const payload = data?.data
  if (Array.isArray(payload)) return payload as T[]
  if (Array.isArray(payload?.data)) return payload.data as T[]
  if (Array.isArray(payload?.list)) return payload.list as T[]
  if (Array.isArray(data?.list)) return data.list as T[]
  return []
}

const base64UrlEncode = (input: ArrayBuffer): string => {
  const bytes = new Uint8Array(input)
  let binary = ''
  for (let i = 0; i < bytes.length; i++) {
    binary += String.fromCharCode(bytes[i])
  }
  const base64 = btoa(binary)
  return base64.replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/g, '')
}

const randomString = (length: number) => {
  const chars = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-._~'
  const array = new Uint8Array(length)
  crypto.getRandomValues(array)
  let output = ''
  for (let i = 0; i < array.length; i++) {
    output += chars[array[i] % chars.length]
  }
  return output
}

export const generatePkce = async () => {
  const codeVerifier = randomString(64)
  const digest = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(codeVerifier))
  const codeChallenge = base64UrlEncode(digest)
  return { codeVerifier, codeChallenge }
}

export const requestDeviceCode = async (clientId: string, codeChallenge: string, method = 'sha256') => {
  const effectiveClientId = resolve115Credentials(clientId).clientId
  const body = new URLSearchParams({
    client_id: effectiveClientId,
    code_challenge: codeChallenge,
    code_challenge_method: method
  })
  const resp = await fetch(DRIVE115_AUTH_DEVICE_URL, {
    method: 'POST',
    headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
    body
  })
  if (!resp.ok) return { error: '获取二维码失败' }
  const data = await resp.json()
  if (!isDrive115ApiSuccess(data)) return { error: data?.message || '获取二维码失败' }
  if (!data?.data?.uid) return { error: data?.message || '获取二维码失败' }
  return {
    uid: data.data.uid as string,
    time: data.data.time as string,
    sign: data.data.sign as string,
    qrcode: data.data.qrcode as string,
    error: ''
  }
}

export const pollDeviceStatus = async (uid: string, time: string, sign: string) => {
  const params = new URLSearchParams({ uid, time, sign })
  const resp = await fetch(`${DRIVE115_QR_STATUS_URL}?${params.toString()}`)
  if (!resp.ok) return { error: '获取扫码状态失败' }
  const data = await resp.json()
  if (!isDrive115ApiSuccess(data)) {
    return { error: data?.message || '获取扫码状态失败' }
  }
  return {
    state: data?.state ?? 0,
    status: data?.data?.status ?? 0,
    msg: data?.data?.msg || data?.message || '',
    error: ''
  }
}

export const exchangeDeviceCode = async (uid: string, codeVerifier: string) => {
  const body = new URLSearchParams({ uid, code_verifier: codeVerifier })
  const resp = await fetch(DRIVE115_TOKEN_URL, {
    method: 'POST',
    headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
    body
  })
  if (!resp.ok) return { error: '获取 access_token 失败' }
  const data = await resp.json()
  if (!isDrive115ApiSuccess(data)) {
    return { error: data?.message || '获取 access_token 失败' }
  }
  return { data: data?.data || data, error: '' }
}

export const refresh115AccessToken = async (refreshToken: string, clientId = '', clientSecret = '') => {
  const credentials = resolve115Credentials(clientId, clientSecret)
  const body = new URLSearchParams({ refresh_token: refreshToken })
  if (credentials.clientId) body.set('client_id', credentials.clientId)
  if (credentials.clientSecret) body.set('client_secret', credentials.clientSecret)
  const resp = await fetch(DRIVE115_REFRESH_URL, {
    method: 'POST',
    headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
    body
  })
  if (!resp.ok) return { error: '刷新 access_token 失败' }
  const data = await resp.json()
  if (!isDrive115ApiSuccess(data)) {
    return { error: data?.message || '刷新 access_token 失败' }
  }
  const payload = data?.data || data || {}
  if (!payload?.access_token) return { error: '刷新 access_token 失败' }
  return {
    access_token: payload.access_token as string,
    refresh_token: payload.refresh_token as string,
    expires_in: payload.expires_in as number,
    token_type: payload.token_type || 'Bearer',
    error: ''
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

export const build115UserId = (refreshToken?: string, accessToken?: string) => {
  const base = refreshToken || accessToken || ''
  if (!base) return ''
  return `115_${hashString(base)}`
}

export const normalize115Token = (data: any, clientId = ''): ITokenInfo | null => {
  if (!data?.access_token) return null
  const expireTime = new Date(Date.now() + (data.expires_in || 0) * 1000).toISOString()
  const userId = build115UserId(data.refresh_token, data.access_token)
  return {
    tokenfrom: '115',
    access_token: data.access_token,
    refresh_token: data.refresh_token || '',
    session_expires_in: 0,
    open_api_token_type: '',
    open_api_access_token: '',
    open_api_refresh_token: '',
    open_api_expires_in: 0,
    signature: '',
    device_id: resolve115Credentials(clientId).clientId,
    expires_in: data.expires_in || 0,
    token_type: data.token_type || 'Bearer',
    user_id: userId,
    user_name: '115网盘',
    avatar: '',
    nick_name: '115网盘',
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
    name: '115网盘',
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
