import { MD5 } from 'crypto-js'
import type { ITokenInfo } from '../user/userstore'
import { humanSize } from '../utils/format'

const PIKPAK_API_HOST = 'https://api-drive.mypikpak.com'
const PIKPAK_USER_HOST = 'https://user.mypikpak.com'
export const PIKPAK_PROTOCOL_CLIENT_ID = 'YUMx5nI8ZU8Ap8pm'
/** Web client secret used by official web / open protocol reverse-engineering (AList/OpenList). */
export const PIKPAK_PROTOCOL_CLIENT_SECRET = 'dbw2OtmVEeuUvIptb1Coyg'
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

export const createPikPakDeviceId = (username: string): string => MD5(`mnemo-pikpak:${username.trim().toLowerCase()}`).toString()

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

const pikpakJson = async <T>(url: string, init: RequestInit, fallback: string): Promise<T> => {
  const resp = await fetch(url, init)
  const data = await resp.json().catch(() => undefined)
  if (!resp.ok || data?.error) {
    throw new Error(parsePikPakError(data, fallback))
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

export const initPikPakCaptchaToken = async (opts: {
  deviceId: string
  action: string
  accessToken?: string
  meta?: Partial<PikPakCaptchaMeta>
}): Promise<string> => {
  const deviceId = opts.deviceId || createPikPakDeviceId(opts.meta?.username || opts.meta?.email || opts.meta?.phone_number || 'mnemo')
  const meta = buildPikPakCaptchaMeta(deviceId, opts.meta || {})
  const captcha = await pikpakJson<{ captcha_token?: string; url?: string }>(`${PIKPAK_USER_HOST}/v1/shield/captcha/init`, {
    method: 'POST',
    headers: buildHeaders(deviceId, opts.accessToken),
    body: JSON.stringify({
      client_id: PIKPAK_PROTOCOL_CLIENT_ID,
      action: opts.action,
      device_id: deviceId,
      captcha_token: '',
      meta
    })
  }, '获取 PikPak 验证信息失败')
  if (captcha.url) throw new Error('PikPak 需要额外图形验证，请稍后重试或在网页端完成验证后再登录')
  if (!captcha.captcha_token) throw new Error('获取 PikPak 验证信息失败')
  return captcha.captcha_token
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

const isEmailUsername = (value: string) => value.includes('@')
const isPhoneUsername = (value: string) => /^\+?\d{6,}$/.test(value.replace(/[\s-]/g, ''))

export const loginPikPak = async (username: string, password: string): Promise<ITokenInfo> => {
  const normalizedUsername = username.trim()
  if (!normalizedUsername || !password) throw new Error('请输入 PikPak 账号和密码')
  const deviceId = createPikPakDeviceId(normalizedUsername)
  const loginUrl = `${PIKPAK_USER_HOST}/v1/auth/signin`
  const captchaMeta: Partial<PikPakCaptchaMeta> = {}
  if (isEmailUsername(normalizedUsername)) captchaMeta.email = normalizedUsername
  else if (isPhoneUsername(normalizedUsername)) captchaMeta.phone_number = normalizedUsername.replace(/[\s-]/g, '')
  else captchaMeta.username = normalizedUsername

  const captchaToken = await initPikPakCaptchaToken({
    deviceId,
    action: 'POST:/v1/auth/signin',
    meta: captchaMeta
  })

  const auth = await pikpakJson<PikPakAuthResp>(loginUrl, {
    method: 'POST',
    headers: {
      ...buildHeaders(deviceId),
      'X-Captcha-Token': captchaToken
    },
    body: JSON.stringify({
      client_id: PIKPAK_PROTOCOL_CLIENT_ID,
      client_secret: PIKPAK_PROTOCOL_CLIENT_SECRET,
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

export const refreshPikPakAccessToken = async (token: ITokenInfo): Promise<ITokenInfo | null> => {
  if (!token.refresh_token) return null
  const auth = await pikpakJson<PikPakAuthResp>(`${PIKPAK_USER_HOST}/v1/auth/token`, {
    method: 'POST',
    headers: buildHeaders(token.device_id),
    body: JSON.stringify({
      client_id: PIKPAK_PROTOCOL_CLIENT_ID,
      client_secret: PIKPAK_PROTOCOL_CLIENT_SECRET,
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
