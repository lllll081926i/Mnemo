import { describe, expect, it, vi } from 'vitest'
import { loginGofile } from '../auth'
import { gofileRequestWithToken } from '../client'

describe('GoFile client', () => {
  it('authenticates API calls with a bearer token', async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, json: async () => ({ status: 'ok', data: { id: 'account-id' } }) })
    vi.stubGlobal('fetch', fetchMock)

    await gofileRequestWithToken('account-token', '/accounts/getid')

    const [, init] = fetchMock.mock.calls[0]
    expect(init.headers.get('Authorization')).toBe('Bearer account-token')
  })

  it('creates an account-scoped token from account metadata', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce({ ok: true, json: async () => ({ status: 'ok', data: { id: 'account-a' } }) })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ status: 'ok', data: { id: 'account-a', email: 'user@example.com', rootFolder: 'root-a', tier: 'premium', statsCurrent: { storage: 12 } } }) })
    vi.stubGlobal('fetch', fetchMock)

    const token = await loginGofile('account-token')

    expect(token.user_id).toBe('gofile_account-a')
    expect(token.default_drive_id).toBe('gofile:account-a')
    expect(token.provider_root_id).toBe('root-a')
    expect(token.access_token).toBe('account-token')
  })
})
