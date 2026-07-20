import { describe, expect, it, vi } from 'vitest'
import { buildOneDriveAuthUrl, exchangeOneDriveCodeForToken, ONEDRIVE_CLIENT_ID, ONEDRIVE_CLIENT_SECRET, ONEDRIVE_SCOPE, refreshOneDriveAccessToken, resolveOneDriveCredentials } from '../auth'

describe('OneDrive auth helpers', () => {
  it('ships an in-app OAuth client id when no release override is configured', () => {
    expect(ONEDRIVE_CLIENT_ID.trim()).not.toBe('')
    expect(ONEDRIVE_CLIENT_SECRET.trim()).not.toBe('')
  })

  it('pairs the bundled secret only with its matching client id', () => {
    expect(resolveOneDriveCredentials()).toEqual({ clientId: ONEDRIVE_CLIENT_ID, clientSecret: ONEDRIVE_CLIENT_SECRET })
    expect(resolveOneDriveCredentials('custom-client-id')).toEqual({ clientId: 'custom-client-id', clientSecret: '' })
    expect(resolveOneDriveCredentials('custom-client-id', 'custom-secret')).toEqual({ clientId: 'custom-client-id', clientSecret: 'custom-secret' })
  })

  it('builds Microsoft identity v2 authorize URL with PKCE and Graph scopes', async () => {
    const redirectUri = 'http://127.0.0.1:53682/oauth/callback'
    const url = await buildOneDriveAuthUrl('client-id', 'verifier', 'state-1', redirectUri)
    const parsed = new URL(url)

    expect(parsed.origin + parsed.pathname).toBe('https://login.microsoftonline.com/common/oauth2/v2.0/authorize')
    expect(parsed.searchParams.get('client_id')).toBe('client-id')
    expect(parsed.searchParams.get('response_type')).toBe('code')
    expect(parsed.searchParams.get('redirect_uri')).toBe(redirectUri)
    expect(parsed.searchParams.get('response_mode')).toBe('query')
    expect(parsed.searchParams.get('prompt')).toBe('select_account')
    expect(parsed.searchParams.get('code_challenge_method')).toBe('S256')
    expect(parsed.searchParams.get('code_challenge')).toBeTruthy()
    expect(parsed.searchParams.get('state')).toBe('state-1')
    expect(parsed.searchParams.get('scope')).toBe(ONEDRIVE_SCOPE)
  })

  it('does not send client_secret for public-client PKCE token requests', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({})
    })
    vi.stubGlobal('fetch', fetchMock)

    await exchangeOneDriveCodeForToken('auth-code', 'client-id', 'verifier', 'http://127.0.0.1:53682/oauth/callback')
    await refreshOneDriveAccessToken({
      device_id: 'client-id',
      refresh_token: 'refresh-token'
    } as any)

    const bodies = fetchMock.mock.calls.map(([, init]) => init?.body as URLSearchParams)
    expect(bodies).toHaveLength(2)
    expect(bodies[0].get('grant_type')).toBe('authorization_code')
    expect(bodies[0].has('client_secret')).toBe(false)
    expect(bodies[1].get('grant_type')).toBe('refresh_token')
    expect(bodies[1].has('client_secret')).toBe(false)
  })
})
