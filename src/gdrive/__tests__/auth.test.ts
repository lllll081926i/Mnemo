import { describe, expect, it, vi } from 'vitest'
import { buildGoogleDriveAuthUrl, createGoogleDrivePkceVerifier, GOOGLE_DRIVE_CLIENT_ID, GOOGLE_DRIVE_CLIENT_SECRET, GOOGLE_DRIVE_SCOPE } from '../auth'

describe('Google Drive OAuth', () => {
  it('ships internal desktop OAuth credentials when no release override is configured', () => {
    expect(GOOGLE_DRIVE_CLIENT_ID.trim()).not.toBe('')
    expect(GOOGLE_DRIVE_CLIENT_SECRET.trim()).not.toBe('')
  })

  it('builds a PKCE authorization URL for the in-app callback session', async () => {
    vi.stubGlobal('crypto', globalThis.crypto)
    const verifier = createGoogleDrivePkceVerifier()
    const redirectUri = 'http://127.0.0.1:53682/oauth/callback'
    const url = new URL(await buildGoogleDriveAuthUrl('client-id', verifier, 'state-a', redirectUri))

    expect(url.origin + url.pathname).toBe('https://accounts.google.com/o/oauth2/v2/auth')
    expect(url.searchParams.get('redirect_uri')).toBe(redirectUri)
    expect(url.searchParams.get('state')).toBe('state-a')
    expect(url.searchParams.get('scope')).toBe(GOOGLE_DRIVE_SCOPE)
    expect(url.searchParams.get('code_challenge_method')).toBe('S256')
    expect(verifier).toMatch(/^[A-Za-z0-9_-]{43,128}$/)
  })
})
