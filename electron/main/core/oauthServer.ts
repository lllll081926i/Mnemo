import { randomBytes } from 'node:crypto'
import { createServer, type IncomingMessage, type Server, type ServerResponse } from 'node:http'

export type OAuthProvider = 'onedrive' | 'dropbox' | 'gdrive'

export interface OAuthCallbackPayload {
  provider: OAuthProvider
  state: string
  code: string
  error: string
  errorDescription: string
}

export interface OAuthCallbackTarget {
  send(channel: string, payload: OAuthCallbackPayload): void
  isDestroyed?(): boolean
}

interface OAuthSession {
  provider: OAuthProvider
  target: OAuthCallbackTarget
  expiresAt: number
}

const CALLBACK_PATH = '/oauth/callback'
const CALLBACK_CHANNEL = 'OAuth:callback'
const SESSION_TTL_MS = 10 * 60 * 1000
const supportedProviders = new Set<OAuthProvider>(['onedrive', 'dropbox', 'gdrive'])

const authorizationEndpoints: Record<OAuthProvider, { origin: string; pathname: string }> = {
  onedrive: { origin: 'https://login.microsoftonline.com', pathname: '/common/oauth2/v2.0/authorize' },
  dropbox: { origin: 'https://www.dropbox.com', pathname: '/oauth2/authorize' },
  gdrive: { origin: 'https://accounts.google.com', pathname: '/o/oauth2/v2/auth' }
}

export const isOAuthAuthorizationUrl = (provider: OAuthProvider, value: string, state: string, redirectUri: string) => {
  try {
    const url = new URL(value)
    const endpoint = authorizationEndpoints[provider]
    return url.origin === endpoint.origin && url.pathname === endpoint.pathname && url.searchParams.get('state') === state && url.searchParams.get('redirect_uri') === redirectUri
  } catch {
    return false
  }
}

const responseHtml = (success: boolean) => `<!doctype html><html><head><meta charset="utf-8"><meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'"><title>Mnemo</title><style>body{font-family:system-ui,sans-serif;margin:0;display:grid;place-items:center;min-height:100vh;background:#f7f8fa;color:#1d2129}main{max-width:420px;padding:32px;text-align:center}h1{font-size:20px;margin:0 0 12px}p{margin:0;color:#4e5969}</style></head><body><main><h1>${success ? '登录授权已完成' : '登录授权失败'}</h1><p>${success ? '现在可以关闭此窗口并返回 Mnemo。' : '请返回 Mnemo 后重新发起登录。'}</p></main></body></html>`

export class OAuthCallbackServer {
  private server: Server | null = null
  private listenPromise: Promise<void> | null = null
  private readonly sessions = new Map<string, OAuthSession>()

  constructor(private readonly port = 53682, private readonly host = '127.0.0.1') {}

  async begin(provider: string, target: OAuthCallbackTarget) {
    if (!supportedProviders.has(provider as OAuthProvider)) throw new Error('不支持的 OAuth 提供方')
    await this.ensureListening()
    this.pruneExpiredSessions()
    const state = randomBytes(24).toString('base64url')
    this.sessions.set(state, { provider: provider as OAuthProvider, target, expiresAt: Date.now() + SESSION_TTL_MS })
    return { state, redirectUri: this.getRedirectUri() }
  }

  cancel(state: string, target?: OAuthCallbackTarget) {
    const session = this.sessions.get(state)
    if (!session || (target && session.target !== target)) return false
    this.sessions.delete(state)
    return true
  }

  async close() {
    this.sessions.clear()
    const server = this.server
    this.server = null
    this.listenPromise = null
    if (!server?.listening) return
    await new Promise<void>((resolve) => server.close(() => resolve()))
  }

  private async ensureListening() {
    if (this.server?.listening) return
    if (this.listenPromise) return this.listenPromise
    this.server = createServer((request, response) => this.handleRequest(request, response))
    this.listenPromise = new Promise<void>((resolve, reject) => {
      const server = this.server!
      const onError = (error: Error) => {
        this.server = null
        this.listenPromise = null
        reject(new Error(`OAuth 回调服务启动失败: ${error.message}`))
      }
      server.once('error', onError)
      server.listen(this.port, this.host, () => {
        server.off('error', onError)
        resolve()
      })
    })
    return this.listenPromise
  }

  private getRedirectUri() {
    const address = this.server?.address()
    const port = typeof address === 'object' && address ? address.port : this.port
    return `http://${this.host}:${port}${CALLBACK_PATH}`
  }

  private pruneExpiredSessions() {
    const now = Date.now()
    for (const [state, session] of this.sessions) {
      if (session.expiresAt <= now || session.target.isDestroyed?.()) this.sessions.delete(state)
    }
  }

  private handleRequest(request: IncomingMessage, response: ServerResponse) {
    const requestUrl = new URL(request.url || '/', this.getRedirectUri())
    if (request.method !== 'GET' || requestUrl.pathname !== CALLBACK_PATH) {
      this.sendResponse(response, 404, false)
      return
    }

    this.pruneExpiredSessions()
    const state = requestUrl.searchParams.get('state') || ''
    const session = this.sessions.get(state)
    if (!session) {
      this.sendResponse(response, 400, false)
      return
    }
    this.sessions.delete(state)

    const payload: OAuthCallbackPayload = {
      provider: session.provider,
      state,
      code: requestUrl.searchParams.get('code') || '',
      error: requestUrl.searchParams.get('error') || '',
      errorDescription: requestUrl.searchParams.get('error_description') || ''
    }
    const success = !!payload.code && !payload.error
    if (!session.target.isDestroyed?.()) session.target.send(CALLBACK_CHANNEL, payload)
    this.sendResponse(response, success ? 200 : 400, success)
  }

  private sendResponse(response: ServerResponse, statusCode: number, success: boolean) {
    response.writeHead(statusCode, {
      'Content-Type': 'text/html; charset=utf-8',
      'Cache-Control': 'no-store',
      'X-Content-Type-Options': 'nosniff',
      'Referrer-Policy': 'no-referrer'
    })
    response.end(responseHtml(success))
  }
}

export const oauthCallbackServer = new OAuthCallbackServer()
