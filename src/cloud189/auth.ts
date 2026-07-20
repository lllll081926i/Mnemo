import { HmacSHA1 } from 'crypto-js'
import type { ITokenInfo } from '../user/userstore'
const CLOUD189_APP_ID = ''

export { CLOUD189_APP_ID }

export const CLOUD189_WEB_URL = 'https://cloud.189.cn'
export const CLOUD189_AUTH_URL = 'https://open.e.189.cn'
export const CLOUD189_API_URL = 'https://api.cloud.189.cn'
export const CLOUD189_CLIENT_TYPE = '10020'
export const CLOUD189_VERSION = '6.2'
export const CLOUD189_PC = 'TELEPC'
export const CLOUD189_CHANNEL_ID = 'web_cloud.189.cn'
export const CLOUD189_LOGIN_REDIRECT_URL = 'https://cloud.189.cn/main.action'
export const CLOUD189_USER_AGENT = 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36'

export interface Cloud189QrState {
  appId: string
  clientType: string
  returnUrl: string
  loginUrl: string
  lt: string
  reqId: string
  paramId: string
  qrUrl: string
  encryuuid: string
  createdAt: number
}

const hashString = (value: string): string => {
  let hash = 0
  for (let i = 0; i < value.length; i++) {
    hash = (hash << 5) - hash + value.charCodeAt(i)
    hash |= 0
  }
  return Math.abs(hash).toString(36)
}

export const cloud189ClientSuffix = () => ({
  clientType: CLOUD189_PC,
  version: CLOUD189_VERSION,
  channelId: CLOUD189_CHANNEL_ID,
  rand: `${Math.floor(Math.random() * 100000)}_${Math.floor(Math.random() * 10000000000)}`
})

const httpDate = () => new Date().toUTCString()

const requestPath = (fullUrl: string) => {
  try {
    return new URL(fullUrl).pathname || '/'
  } catch {
    return '/'
  }
}

export const cloud189SignatureHeaders = (sessionKey: string, sessionSecret: string, method: string, fullUrl: string, params = '') => {
  const date = httpDate()
  let text = `SessionKey=${sessionKey}&Operate=${method.toUpperCase()}&RequestURI=${requestPath(fullUrl)}&Date=${date}`
  if (params) text += `&params=${params}`
  return {
    Date: date,
    SessionKey: sessionKey,
    'X-Request-ID': crypto.randomUUID(),
    Signature: HmacSHA1(text, sessionSecret).toString().toUpperCase()
  }
}

export const buildCloud189LoginStartUrl = () => {
  const params = new URLSearchParams({ redirectURL: CLOUD189_LOGIN_REDIRECT_URL })
  return `${CLOUD189_WEB_URL}/api/portal/loginUrl.action?${params.toString()}`
}

export const parseCloud189LoginContext = (url: string) => {
  const parsed = new URL(url)
  const appId = parsed.searchParams.get('appId') || ''
  const lt = parsed.searchParams.get('lt') || ''
  const reqId = parsed.searchParams.get('reqId') || ''
  if (!appId || !lt || !reqId) throw new Error('解析天翼登录参数失败')
  return { appId, lt, reqId, loginUrl: parsed.toString() }
}

const cloud189LoginHeaders = (context: { lt: string; reqId: string; loginUrl: string }) => ({
  Accept: 'application/json;charset=UTF-8',
  'Content-Type': 'application/x-www-form-urlencoded',
  'User-Agent': CLOUD189_USER_AGENT,
  Referer: context.loginUrl,
  Origin: CLOUD189_AUTH_URL,
  Reqid: context.reqId,
  lt: context.lt
})

const cloud189Json = async (resp: Response, fallback: string) => {
  const data = await resp.json().catch(() => undefined)
  if (!resp.ok) throw new Error(`${fallback} HTTP ${resp.status}`)
  if (data?.result !== undefined && String(data.result) !== '0') throw new Error(data?.msg || data?.message || fallback)
  return data
}

export const buildCloud189QrPollForm = (state: Cloud189QrState, now = new Date()) => {
  const date = `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}-${String(now.getDate()).padStart(2, '0')}${String(now.getHours()).padStart(2, '0')}:${String(now.getMinutes()).padStart(2, '0')}:${String(now.getSeconds()).padStart(2, '0')}.${String(now.getMilliseconds()).padStart(3, '0')}`
  return new URLSearchParams({
    appId: state.appId,
    clientType: state.clientType,
    returnUrl: state.returnUrl,
    paramId: state.paramId,
    uuid: state.qrUrl,
    encryuuid: state.encryuuid,
    date,
    timeStamp: String(now.getTime()),
    cb_SaveName: '0',
    isOauth2: 'true',
    state: ''
  })
}

export const requestCloud189QrCode = async (): Promise<Cloud189QrState> => {
  const loginResp = await fetch(buildCloud189LoginStartUrl(), {
    credentials: 'include',
    headers: { 'User-Agent': CLOUD189_USER_AGENT, Referer: CLOUD189_WEB_URL }
  })
  if (!loginResp.ok) throw new Error(`初始化天翼登录参数失败 HTTP ${loginResp.status}`)
  const context = parseCloud189LoginContext(loginResp.url)
  const headers = cloud189LoginHeaders(context)
  const appConfResp = await fetch(`${CLOUD189_AUTH_URL}/api/logbox/oauth2/appConf.do`, {
    method: 'POST',
    credentials: 'include',
    headers,
    body: new URLSearchParams({ version: '2.0', appKey: context.appId })
  })
  const appConf = await cloud189Json(appConfResp, '获取天翼登录配置失败')
  const config = appConf?.data
  if (!config?.paramId || !config?.returnUrl) throw new Error('天翼登录配置不完整')
  const uuidResp = await fetch(`${CLOUD189_AUTH_URL}/api/logbox/oauth2/getUUID.do`, {
    method: 'POST',
    credentials: 'include',
    headers,
    body: new URLSearchParams({ appId: context.appId })
  })
  const data = await cloud189Json(uuidResp, '获取天翼云盘二维码失败')
  if (!data?.uuid || !data?.encryuuid) throw new Error('天翼云盘二维码数据不完整')
  return {
    ...context,
    clientType: String(config.clientType || '1'),
    returnUrl: String(config.returnUrl),
    paramId: String(config.paramId),
    qrUrl: String(data.uuid),
    encryuuid: String(data.encryuuid),
    createdAt: Date.now()
  }
}

export const pollCloud189QrLogin = async (state: Cloud189QrState): Promise<{ status: 'waiting' | 'expired' | 'failed' | 'success'; message: string; token?: ITokenInfo }> => {
  if (Date.now() - state.createdAt > 300000) return { status: 'expired', message: '二维码已过期，请重新获取' }
  const resp = await fetch(`${CLOUD189_AUTH_URL}/api/logbox/oauth2/qrcodeLoginState.do`, {
    method: 'POST',
    credentials: 'include',
    headers: cloud189LoginHeaders(state),
    body: buildCloud189QrPollForm(state)
  })
  const data = await resp.json().catch(() => undefined)
  if (!resp.ok) return { status: 'failed', message: `获取扫码状态失败 HTTP ${resp.status}` }
  const status = Number(data?.status)
  if (status === -106 || status === -11002) return { status: 'waiting', message: '请扫码并在手机上确认登录' }
  if (status === -11001) return { status: 'expired', message: '二维码已过期，请重新获取' }
  if (status !== 0 || !data?.redirectUrl) return { status: 'failed', message: data?.msg || data?.message || `扫码登录失败，状态码 ${data?.status}` }
  const token = await finalizeCloud189Login(data.redirectUrl, state.appId)
  return token ? { status: 'success', message: '登录成功', token } : { status: 'failed', message: '换取天翼云盘登录态失败' }
}

export const finalizeCloud189Login = async (redirectUrl: string, appId = CLOUD189_APP_ID): Promise<ITokenInfo | null> => {
  const params = new URLSearchParams({ ...cloud189ClientSuffix(), redirectURL: redirectUrl })
  const resp = await fetch(`${CLOUD189_API_URL}/getSessionForPC.action?${params.toString()}`, {
    method: 'POST',
    headers: { Accept: 'application/json;charset=UTF-8', 'User-Agent': CLOUD189_USER_AGENT }
  })
  const data = await resp.json().catch(() => undefined)
  if (!resp.ok || Number(data?.res_code || 0) !== 0 || !data?.refreshToken) return null
  return normalizeCloud189Token(data, appId)
}

export const normalizeCloud189Token = (data: any, appId = CLOUD189_APP_ID): ITokenInfo => {
  const id = data.userId || data.loginName || hashString(data.refreshToken || data.accessToken || data.sessionKey || '')
  return {
    tokenfrom: '189',
    access_token: data.accessToken || data.sessionKey || '',
    refresh_token: data.refreshToken || '',
    session_expires_in: 0,
    open_api_token_type: '',
    open_api_access_token: data.sessionKey || '',
    open_api_refresh_token: data.sessionSecret || '',
    open_api_expires_in: 0,
    signature: data.sessionSecret || '',
    device_id: appId,
    expires_in: Number(data.expiresIn || data.expires_in || 0),
    token_type: 'Bearer',
    user_id: `cloud189_${id}`,
    user_name: data.loginName || data.userName || '天翼云盘',
    avatar: '',
    nick_name: data.nickName || data.loginName || '天翼云盘',
    default_drive_id: 'cloud189',
    default_sbox_drive_id: '',
    resource_drive_id: '',
    backup_drive_id: '',
    sbox_drive_id: '',
    role: '',
    status: '',
    expire_time: data.expiresIn ? new Date(Date.now() + Number(data.expiresIn) * 1000).toISOString() : '',
    state: '',
    pin_setup: false,
    is_first_login: false,
    need_rp_verify: false,
    name: data.nickName || data.loginName || '天翼云盘',
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

export const refreshCloud189Token = async (token: ITokenInfo): Promise<ITokenInfo | null> => {
  if (!token.refresh_token) return null
  const appId = token.device_id || CLOUD189_APP_ID
  if (!appId) return null
  const form = new URLSearchParams({
    clientId: appId,
    refreshToken: token.refresh_token,
    grantType: 'refresh_token',
    format: 'json'
  })
  const resp = await fetch(`${CLOUD189_AUTH_URL}/api/oauth2/refreshToken.do`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/x-www-form-urlencoded', 'User-Agent': CLOUD189_USER_AGENT },
    body: form
  })
  const data = await resp.json().catch(() => undefined)
  if (!resp.ok || !data?.accessToken) return null
  const params = new URLSearchParams({ ...cloud189ClientSuffix(), appId, accessToken: data.accessToken })
  const sessionResp = await fetch(`${CLOUD189_API_URL}/getSessionForPC.action?${params.toString()}`, {
    headers: { Accept: 'application/json;charset=UTF-8', 'User-Agent': CLOUD189_USER_AGENT }
  })
  const session = await sessionResp.json().catch(() => undefined)
  if (!sessionResp.ok || Number(session?.res_code || 0) !== 0) return null
  return normalizeCloud189Token({ ...data, ...session, refreshToken: data.refreshToken || token.refresh_token }, appId)
}
