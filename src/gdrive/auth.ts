import type { ITokenInfo } from '../user/userstore'
import { GOOGLE_DRIVE_CLIENT_ID as CONFIGURED_GOOGLE_DRIVE_CLIENT_ID, GOOGLE_DRIVE_CLIENT_SECRET as CONFIGURED_GOOGLE_DRIVE_CLIENT_SECRET } from '../secrets.generated'
import { humanSize } from '../utils/format'
import { buildDriveProviderDriveId, buildDriveProviderUserId } from '../utils/driveProvider'
import { createProviderToken } from '../utils/providerToken'

const BUILTIN_GOOGLE_DRIVE_CLIENT_ID = '202264815644.apps.googleusercontent.com'
const BUILTIN_GOOGLE_DRIVE_CLIENT_SECRET = 'X4Z3ca8xfWDb1Voo-F9a7ZxJ'
export const GOOGLE_DRIVE_CLIENT_ID = CONFIGURED_GOOGLE_DRIVE_CLIENT_ID.trim() || BUILTIN_GOOGLE_DRIVE_CLIENT_ID
export const GOOGLE_DRIVE_CLIENT_SECRET = CONFIGURED_GOOGLE_DRIVE_CLIENT_SECRET.trim() || BUILTIN_GOOGLE_DRIVE_CLIENT_SECRET

const AUTH_URL = 'https://accounts.google.com/o/oauth2/v2/auth'
const TOKEN_URL = 'https://oauth2.googleapis.com/token'
const ABOUT_URL = 'https://www.googleapis.com/drive/v3/about?fields=user,storageQuota'
export const GOOGLE_DRIVE_SCOPE = 'https://www.googleapis.com/auth/drive'

const base64UrlEncode = (bytes: Uint8Array) => {
  let binary = ''
  bytes.forEach((byte) => {
    binary += String.fromCharCode(byte)
  })
  return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/g, '')
}

export const createGoogleDrivePkceVerifier = () => {
  const bytes = new Uint8Array(48)
  crypto.getRandomValues(bytes)
  return base64UrlEncode(bytes)
}

export const buildGoogleDriveAuthUrl = async (clientId: string, verifier: string, state: string, redirectUri: string) => {
  const digest = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(verifier))
  const params = new URLSearchParams({
    client_id: clientId.trim(),
    redirect_uri: redirectUri,
    response_type: 'code',
    scope: GOOGLE_DRIVE_SCOPE,
    access_type: 'offline',
    prompt: 'consent select_account',
    include_granted_scopes: 'true',
    code_challenge: base64UrlEncode(new Uint8Array(digest)),
    code_challenge_method: 'S256',
    state
  })
  return `${AUTH_URL}?${params.toString()}`
}

const tokenRequest = async (body: URLSearchParams) => {
  const response = await fetch(TOKEN_URL, { method: 'POST', headers: { 'Content-Type': 'application/x-www-form-urlencoded' }, body })
  const payload = await response.json().catch(() => ({}))
  if (!response.ok || payload?.error) throw new Error(payload?.error_description || payload?.error || 'Google Drive Token 请求失败')
  return payload
}

const applyGoogleDriveAccount = async (token: ITokenInfo) => {
  const response = await fetch(ABOUT_URL, { headers: { Authorization: `Bearer ${token.access_token}` } })
  const payload = await response.json().catch(() => ({}))
  if (!response.ok || payload?.error) throw new Error(payload?.error?.message || 'Google Drive 账号信息获取失败')
  const accountId = payload.user?.permissionId || payload.user?.emailAddress
  if (!accountId) throw new Error('Google Drive 账号标识缺失')
  const name = payload.user?.displayName || payload.user?.emailAddress || 'Google Drive'
  const used = Number(payload.storageQuota?.usage || 0)
  const total = Number(payload.storageQuota?.limit || 0)
  token.user_id = buildDriveProviderUserId('gdrive', accountId)
  token.default_drive_id = buildDriveProviderDriveId('gdrive', accountId)
  token.provider_account_id = accountId
  token.provider_root_id = 'root'
  token.user_name = payload.user?.emailAddress || name
  token.nick_name = name
  token.name = name
  token.avatar = payload.user?.photoLink || ''
  token.used_size = used
  token.total_size = total
  token.free_size = total > used ? total - used : 0
  token.spaceinfo = total ? `${humanSize(used)} / ${humanSize(total)}` : humanSize(used)
}

export const exchangeGoogleDriveCodeForToken = async (code: string, clientId: string, clientSecret: string, verifier: string, redirectUri: string) => {
  const body = new URLSearchParams({ client_id: clientId.trim(), code, code_verifier: verifier, grant_type: 'authorization_code', redirect_uri: redirectUri })
  if (clientSecret.trim()) body.set('client_secret', clientSecret.trim())
  const payload = await tokenRequest(body)
  const token = createProviderToken('gdrive', {
    access_token: payload.access_token || '',
    refresh_token: payload.refresh_token || '',
    token_type: payload.token_type || 'Bearer',
    expires_in: Number(payload.expires_in || 3600),
    expire_time: new Date(Date.now() + Number(payload.expires_in || 3600) * 1000).toISOString(),
    device_id: clientId.trim()
  })
  if (!token.access_token) throw new Error('Google Drive access_token 缺失')
  await applyGoogleDriveAccount(token)
  return token
}

export const refreshGoogleDriveAccessToken = async (token: ITokenInfo) => {
  const clientId = token.device_id || GOOGLE_DRIVE_CLIENT_ID
  if (!clientId || !token.refresh_token) return null
  const body = new URLSearchParams({ client_id: clientId, refresh_token: token.refresh_token, grant_type: 'refresh_token' })
  if (GOOGLE_DRIVE_CLIENT_SECRET.trim()) body.set('client_secret', GOOGLE_DRIVE_CLIENT_SECRET.trim())
  const payload = await tokenRequest(body)
  token.access_token = payload.access_token || token.access_token
  token.expires_in = Number(payload.expires_in || token.expires_in || 3600)
  token.expire_time = new Date(Date.now() + token.expires_in * 1000).toISOString()
  token.tokenfrom = 'gdrive'
  await applyGoogleDriveAccount(token)
  return token
}

export const applyGoogleDriveQuota = async (token: ITokenInfo) => {
  await applyGoogleDriveAccount(token)
  return true
}
