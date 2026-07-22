import { MD5 } from 'crypto-js'
import type { ITokenInfo } from '../user/userstore'
import { humanSize } from '../utils/format'

const PIKPAK_API_HOST = 'https://api-drive.mypikpak.com'
const PIKPAK_USER_HOST = 'https://user.mypikpak.com'
export const PIKPAK_PROTOCOL_CLIENT_ID = 'YUMx5nI8ZU8Ap8pm'
export const PIKPAK_PROTOCOL_CLIENT_VERSION = '2.0.0'
export const PIKPAK_PROTOCOL_PACKAGE_NAME = 'mypikpak.com'
const DEFAULT_USER_AGENT = 'Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:129.0) Gecko/20100101 Firefox/129.0'

const CAPTCHA_SALTS = [
  'C9qPpZLN8ucRTaTiUMWYS9cQvWOE',
  '+r6CQVxjzJV6LCV',
  'F',
  'pFJRC',
  '9WXYIDGrwTCz2OiVlgZa90qpECPD6olt',
  '/750aCr4lm/Sly/c',
  'RB+DT/gZCrbV',
  '',
  'CyLsf7hdkIRxRm215hl',
  '7xHvLi2tOYP0Y92b',
  'ZGTXXxu8E/MIWaEDB+Sm/',
  '1UI3',
  'E7fP5Pfijd+7K+t6Tg/NhuLq0eEUVChpJSkrKxpO',
  'ihtqpG6FMt65+Xk+tWUH2',
  'NhXXU9rg4XXdzo7u5o'
]

type PikPakAuthResp = {
  access_token: string
  refresh_token: string
  expires_in?: number
  token_type?: string
  sub?: string
}

const PIKPAK_DEVICE_STORAGE_PREFIX = 'mnemo:pikpak:device:'
const volatileDeviceIds = new Map<string, string>()

export const createPikPakDeviceId = (): string => {
  const bytes = new Uint8Array(16)
  if (globalThis.crypto?.getRandomValues) {
    globalThis.crypto.getRandomValues(bytes)
  } else {
    for (let index = 0; index < bytes.length; index += 1) bytes[index] = Math.floor(Math.random() * 256)
  }
  bytes[6] = (bytes[6] & 0x0f) | 0x40
  bytes[8] = (bytes[8] & 0x3f) | 0x80
  return Array.from(bytes, value => value.toString(16).padStart(2, '0')).join('')
}

const getDeviceStorage = (): Pick<Storage, 'getItem' | 'setItem'> | undefined => {
  try {
    if (typeof localStorage === 'undefined' || typeof localStorage.getItem !== 'function' || typeof localStorage.setItem !== 'function') return undefined
    return localStorage
  } catch {
    return undefined
  }
}

export const getOrCreatePikPakDeviceId = (username: string): string => {
  const accountKey = MD5(username.trim().toLowerCase()).toString()
  const storageKey = `${PIKPAK_DEVICE_STORAGE_PREFIX}${accountKey}`
  const storage = getDeviceStorage()
  const stored = storage?.getItem(storageKey) || volatileDeviceIds.get(accountKey) || ''
  if (/^[a-f0-9]{32}$/i.test(stored)) return stored.toLowerCase()
  const deviceId = createPikPakDeviceId()
  volatileDeviceIds.set(accountKey, deviceId)
  try {
    storage?.setItem(storageKey, deviceId)
  } catch {
    // In-memory identity remains stable when storage is unavailable.
  }
  return deviceId
}

export const getPikPakAccountId = (accessToken: string, username: string): string => {
  try {
    const encodedPayload = accessToken.split('.')[1]
    if (encodedPayload) {
      const normalized = encodedPayload.replace(/-/g, '+').replace(/_/g, '/').padEnd(Math.ceil(encodedPayload.length / 4) * 4, '=')
      const payload = JSON.parse(atob(normalized))
      if (payload?.sub) return String(payload.sub)
    }
  } catch {
    // A token without a JWT payload still has a stable account fallback below.
  }
  return username.trim().toLowerCase()
}

export const captchaSign = (deviceId: string, timestamp: string): string => {
  let sign = PIKPAK_PROTOCOL_CLIENT_ID + PIKPAK_PROTOCOL_CLIENT_VERSION + PIKPAK_PROTOCOL_PACKAGE_NAME + deviceId + timestamp
  for (const salt of CAPTCHA_SALTS) {
    sign = MD5(sign + salt).toString()
  }
  return `1.${sign}`
}

export type PikPakCaptchaMeta = {
  captcha_sign: string
  client_version: string
  package_name: string
  timestamp: string
  username?: string
  email?: string
  phone_number?: string
  user_id?: string
}

const buildHeaders = (deviceId?: string, accessToken?: string): HeadersInit => {
  const headers: Record<string, string> = {
    'Content-Type': 'application/json; charset=utf-8',
    'User-Agent': DEFAULT_USER_AGENT,
    Referer: 'https://mypikpak.com/',
    'X-Client-Id': PIKPAK_PROTOCOL_CLIENT_ID,
    'X-Client-Version': PIKPAK_PROTOCOL_CLIENT_VERSION
  }
  if (deviceId) headers['X-Device-Id'] = deviceId
  if (accessToken) headers.Authorization = `Bearer ${accessToken}`
  return headers
}

const parsePikPakError = (data: any, fallback: string) => {
  if (!data) return fallback
  if (data.error === 'invalid_account_or_password') return 'PikPak 账号或密码错误'
  if (data.error === 'captcha_invalid' || data.error === 'captcha_required') return 'PikPak 验证失败，请重试登录'
  return data.error_description || data.message || data.error || fallback
}

class PikPakApiError extends Error {
  reason: string
  code: number

  constructor(data: any, fallback: string, status: number) {
    super(parsePikPakError(data, fallback))
    this.name = 'PikPakApiError'
    this.reason = String(data?.error || data?.reason || '')
    this.code = Number(data?.error_code ?? data?.code ?? status)
  }
}

const pikpakJson = async <T>(url: string, init: RequestInit, fallback: string): Promise<T> => {
  const resp = await fetch(url, init)
  const data = await resp.json().catch(() => undefined)
  if (!resp.ok || data?.error) {
    throw new PikPakApiError(data, fallback, resp.status)
  }
  return data as T
}

/** Build captcha_sign + meta for /v1/shield/captcha/init (login & API). */
export const buildPikPakCaptchaMeta = (deviceId: string, extra: Partial<PikPakCaptchaMeta> = {}): PikPakCaptchaMeta => {
  const timestamp = String(extra.timestamp || Date.now())
  const { captcha_sign: _ignored, timestamp: _t, client_version: _cv, package_name: _pn, ...rest } = extra
  return {
    captcha_sign: captchaSign(deviceId, timestamp),
    client_version: PIKPAK_PROTOCOL_CLIENT_VERSION,
    package_name: PIKPAK_PROTOCOL_PACKAGE_NAME,
    timestamp,
    ...rest
  }
}

export type PikPakCaptchaInitResult = {
  captchaToken: string
  /** When set, open this URL for slider / image captcha before using captchaToken. */
  challengeUrl?: string
  expiresIn?: number
}

export const initPikPakCaptcha = async (opts: {
  deviceId: string
  action: string
  accessToken?: string
  meta?: Partial<PikPakCaptchaMeta>
  previousToken?: string
}): Promise<PikPakCaptchaInitResult> => {
  const deviceId = /^[a-f0-9]{32}$/i.test(opts.deviceId) ? opts.deviceId.toLowerCase() : createPikPakDeviceId()
  const meta = opts.action === 'POST:/v1/auth/signin' ? { username: String(opts.meta?.username || '').trim() } : buildPikPakCaptchaMeta(deviceId, opts.meta || {})
  const body: Record<string, unknown> = {
    client_id: PIKPAK_PROTOCOL_CLIENT_ID,
    action: opts.action,
    device_id: deviceId,
    meta
  }
  if (opts.previousToken) body.captcha_token = opts.previousToken
  const captcha = await pikpakJson<{ captcha_token?: string; url?: string; expires_in?: number }>(`${PIKPAK_USER_HOST}/v1/shield/captcha/init`, {
    method: 'POST',
    headers: buildHeaders(deviceId, opts.accessToken),
    body: JSON.stringify(body)
  }, '获取 PikPak 验证信息失败')
  if (!captcha.captcha_token) throw new Error('获取 PikPak 验证信息失败')
  return {
    captchaToken: captcha.captcha_token,
    challengeUrl: captcha.url || undefined,
    expiresIn: Number(captcha.expires_in || 0) || undefined
  }
}

/** Resolve captcha_token only. PikPak's URL is informational; rclone uses the token directly. */
export const initPikPakCaptchaToken = async (opts: {
  deviceId: string
  action: string
  accessToken?: string
  meta?: Partial<PikPakCaptchaMeta>
}): Promise<string> => {
  const result = await initPikPakCaptcha(opts)
  return result.captchaToken
}

export const buildPikPakLoginCaptchaMeta = (username: string): Partial<PikPakCaptchaMeta> => {
  return { username: username.trim() }
}

const signInPikPak = async (username: string, password: string, captchaToken: string, deviceId: string): Promise<ITokenInfo> => {
  const normalizedUsername = username.trim()
  if (!normalizedUsername || !password) throw new Error('请输入 PikPak 账号和密码')
  if (!captchaToken) throw new Error('请先完成 PikPak 验证')
  const auth = await pikpakJson<PikPakAuthResp>(`${PIKPAK_USER_HOST}/v1/auth/signin`, {
    method: 'POST',
    headers: {
      ...buildHeaders(deviceId),
      'X-Captcha-Token': captchaToken
    },
    body: JSON.stringify({
      client_id: PIKPAK_PROTOCOL_CLIENT_ID,
      password,
      username: normalizedUsername
    })
  }, 'PikPak 登录失败')

  const token = emptyToken()
  token.access_token = auth.access_token
  token.refresh_token = auth.refresh_token
  token.expires_in = Number(auth.expires_in || 7200)
  token.token_type = auth.token_type || 'Bearer'
  const accountId = auth.sub || getPikPakAccountId(auth.access_token, normalizedUsername)
  token.user_id = `pikpak_${accountId}`
  token.user_name = normalizedUsername
  token.nick_name = normalizedUsername
  token.name = normalizedUsername
  token.device_id = deviceId
  token.expire_time = new Date(Date.now() + token.expires_in * 1000).toISOString()
  return token
}

const emptyToken = (): ITokenInfo => ({
  tokenfrom: 'pikpak',
  access_token: '',
  refresh_token: '',
  session_expires_in: 0,
  open_api_token_type: '',
  open_api_access_token: '',
  open_api_refresh_token: '',
  open_api_expires_in: 0,
  expires_in: 0,
  token_type: '',
  user_id: '',
  user_name: '',
  avatar: '',
  nick_name: '',
  default_drive_id: 'pikpak',
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
  name: '',
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
  device_id: '',
  signature: '',
  signInfo: {
    signMon: -1,
    signDay: -1
  }
})

export const loginPikPak = async (username: string, password: string, deviceId?: string): Promise<ITokenInfo> => {
  const normalizedUsername = username.trim()
  if (!normalizedUsername || !password) throw new Error('请输入 PikPak 账号和密码')
  const resolvedDeviceId = /^[a-f0-9]{32}$/i.test(deviceId || '') ? String(deviceId).toLowerCase() : getOrCreatePikPakDeviceId(normalizedUsername)
  const requestCaptcha = () => initPikPakCaptcha({
    deviceId: resolvedDeviceId,
    action: 'POST:/v1/auth/signin',
    meta: buildPikPakLoginCaptchaMeta(normalizedUsername)
  })
  let captcha = await requestCaptcha()
  try {
    return await signInPikPak(normalizedUsername, password, captcha.captchaToken, resolvedDeviceId)
  } catch (error) {
    if (!(error instanceof PikPakApiError) || error.reason !== 'captcha_invalid' || error.code !== 4002) throw error
  }
  captcha = await requestCaptcha()
  return signInPikPak(normalizedUsername, password, captcha.captchaToken, resolvedDeviceId)
}

export const refreshPikPakAccessToken = async (token: ITokenInfo): Promise<ITokenInfo | null> => {
  if (!token.refresh_token) return null
  const auth = await pikpakJson<PikPakAuthResp>(`${PIKPAK_USER_HOST}/v1/auth/token`, {
    method: 'POST',
    headers: buildHeaders(token.device_id),
    body: JSON.stringify({
      client_id: PIKPAK_PROTOCOL_CLIENT_ID,
      refresh_token: token.refresh_token,
      grant_type: 'refresh_token'
    })
  }, '刷新 PikPak Token 失败')
  token.access_token = auth.access_token
  token.refresh_token = auth.refresh_token || token.refresh_token
  token.expires_in = Number(auth.expires_in || token.expires_in || 7200)
  token.token_type = auth.token_type || token.token_type || 'Bearer'
  token.user_id = token.user_id || `pikpak_${auth.sub || ''}`
  token.expire_time = new Date(Date.now() + token.expires_in * 1000).toISOString()
  token.tokenfrom = 'pikpak'
  token.default_drive_id = token.default_drive_id || 'pikpak'
  return token
}

export const apiPikPakAbout = async (token: ITokenInfo): Promise<any | null> => {
  if (!token.access_token) return null
  return pikpakJson<any>(`${PIKPAK_API_HOST}/drive/v1/about`, {
    method: 'GET',
    headers: buildHeaders(token.device_id, token.access_token)
  }, '获取 PikPak 空间信息失败').catch(() => null)
}

export const applyPikPakQuota = async (token: ITokenInfo): Promise<boolean> => {
  const about = await apiPikPakAbout(token)
  const quota = about?.quota || {}
  const total = Number(quota.limit || 0)
  const used = Number(quota.usage || 0)
  if (Number.isFinite(total) && total > 0) token.total_size = total
  if (Number.isFinite(used) && used >= 0) token.used_size = used
  if (token.total_size || token.used_size) {
    token.spaceinfo = `${humanSize(token.used_size)} / ${humanSize(token.total_size)}`
  }
  return true
}

export const pikpakAuthHeaders = (token: ITokenInfo): HeadersInit => buildHeaders(token.device_id, token.access_token)
