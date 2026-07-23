import { describe, expect, it } from 'vitest'
import { DROPBOX_APP_KEY, DROPBOX_APP_SECRET, buildDropboxAuthUrl, createDropboxPkceVerifier, resolveDropboxCredentials } from '../auth'

describe('Dropbox auth helpers', () => {
  it('builds a PKCE authorization url using the app key and verifier challenge', async () => {
    const verifier = 'abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-._~'
    const redirectUri = 'http://127.0.0.1:53682/oauth/callback'
    const url = await buildDropboxAuthUrl('app-key', verifier, 'state-1', redirectUri)
    const parsed = new URL(url)

    expect(parsed.origin + parsed.pathname).toBe('https://www.dropbox.com/oauth2/authorize')
    expect(parsed.searchParams.get('client_id')).toBe('app-key')
    expect(parsed.searchParams.get('response_type')).toBe('code')
    expect(parsed.searchParams.get('token_access_type')).toBe('offline')
    // 不传 scope（令牌获得应用已授权的全部权限，避免 invalid_scope 拒登），也不强制重复授权
    expect(parsed.searchParams.has('force_reapprove')).toBe(false)
    expect(parsed.searchParams.has('scope')).toBe(false)
    expect(parsed.searchParams.get('code_challenge_method')).toBe('S256')
    expect(parsed.searchParams.get('code_challenge')).toBeTruthy()
    expect(parsed.searchParams.get('state')).toBe('state-1')
    expect(parsed.searchParams.get('redirect_uri')).toBe(redirectUri)
  })

  it('creates a verifier accepted by Dropbox PKCE', () => {
    const verifier = createDropboxPkceVerifier()

    expect(verifier).toMatch(/^[A-Za-z0-9._~-]{43,128}$/)
  })

  it('exports Dropbox app credentials as cleanable constants', () => {
    expect(DROPBOX_APP_KEY.trim()).not.toBe('')
    expect(DROPBOX_APP_SECRET.trim()).not.toBe('')
  })

  it('pairs the bundled secret only with its matching app key', () => {
    expect(resolveDropboxCredentials()).toEqual({ appKey: DROPBOX_APP_KEY, appSecret: DROPBOX_APP_SECRET })
    expect(resolveDropboxCredentials('custom-key')).toEqual({ appKey: 'custom-key', appSecret: '' })
    expect(resolveDropboxCredentials('custom-key', 'custom-secret')).toEqual({ appKey: 'custom-key', appSecret: 'custom-secret' })
  })

  it('always uses PKCE parameters even with the built-in confidential client', async () => {
    const url = new URL(await buildDropboxAuthUrl(DROPBOX_APP_KEY, 'test-verifier-value-01234567890123456789012', 'state-2', 'http://localhost:53682/'))
    expect(url.searchParams.has('code_challenge')).toBe(true)
    expect(url.searchParams.get('code_challenge_method')).toBe('S256')
  })
})
