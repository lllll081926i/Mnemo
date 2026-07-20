import { afterEach, describe, expect, it, vi } from 'vitest'
import { isOAuthAuthorizationUrl, OAuthCallbackServer } from '../oauthServer'

const servers: OAuthCallbackServer[] = []

afterEach(async () => {
  await Promise.all(servers.splice(0).map((server) => server.close()))
})

describe('OAuthCallbackServer', () => {
  it('routes concurrent provider callbacks by state', async () => {
    const server = new OAuthCallbackServer(0)
    servers.push(server)
    const firstTarget = { send: vi.fn(), isDestroyed: () => false }
    const secondTarget = { send: vi.fn(), isDestroyed: () => false }
    const first = await server.begin('onedrive', firstTarget)
    const second = await server.begin('gdrive', secondTarget)

    expect(new URL(first.redirectUri).hostname).toBe('localhost')
    expect(new URL(first.redirectUri).pathname).toBe('/')
    expect(new URL(second.redirectUri).hostname).toBe('127.0.0.1')
    expect(new URL(second.redirectUri).pathname).toBe('/')

    const secondResponse = await fetch(`${second.redirectUri}?state=${second.state}&code=google-code`)
    const firstResponse = await fetch(`${first.redirectUri}?state=${first.state}&code=microsoft-code`)

    expect(secondResponse.status).toBe(200)
    expect(firstResponse.status).toBe(200)
    expect(firstTarget.send).toHaveBeenCalledWith('OAuth:callback', expect.objectContaining({ provider: 'onedrive', state: first.state, code: 'microsoft-code' }))
    expect(secondTarget.send).toHaveBeenCalledWith('OAuth:callback', expect.objectContaining({ provider: 'gdrive', state: second.state, code: 'google-code' }))
  })

  it('rejects callbacks with unknown state without notifying a renderer', async () => {
    const server = new OAuthCallbackServer(0)
    servers.push(server)
    const target = { send: vi.fn(), isDestroyed: () => false }
    const session = await server.begin('dropbox', target)

    const response = await fetch(`${session.redirectUri}?state=wrong-state&code=code`)

    expect(response.status).toBe(400)
    expect(target.send).not.toHaveBeenCalled()
  })

  it('allows only registered providers and session owners to cancel', async () => {
    const server = new OAuthCallbackServer(0)
    servers.push(server)
    const target = { send: vi.fn(), isDestroyed: () => false }
    const otherTarget = { send: vi.fn(), isDestroyed: () => false }

    await expect(server.begin('mega', target)).rejects.toThrow('不支持的 OAuth 提供方')
    const session = await server.begin('onedrive', target)
    expect(server.cancel(session.state, otherTarget)).toBe(false)
    expect(server.cancel(session.state, target)).toBe(true)
  })

  it('accepts only the provider authorize endpoint bound to the current session', () => {
    const redirectUri = 'http://127.0.0.1:53682/oauth/callback'
    const valid = `https://login.microsoftonline.com/common/oauth2/v2.0/authorize?state=session-state&redirect_uri=${encodeURIComponent(redirectUri)}`
    expect(isOAuthAuthorizationUrl('onedrive', valid, 'session-state', redirectUri)).toBe(true)
    expect(isOAuthAuthorizationUrl('onedrive', valid, 'other-state', redirectUri)).toBe(false)
    expect(isOAuthAuthorizationUrl('dropbox', valid, 'session-state', redirectUri)).toBe(false)
    expect(isOAuthAuthorizationUrl('gdrive', 'https://example.com/o/oauth2/v2/auth?state=session-state', 'session-state', redirectUri)).toBe(false)
  })
})
