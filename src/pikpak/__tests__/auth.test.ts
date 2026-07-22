import { beforeEach, describe, expect, it, vi } from 'vitest'
import { buildPikPakCaptchaMeta, buildPikPakLoginCaptchaMeta, captchaSign, createPikPakDeviceId, getOrCreatePikPakDeviceId, loginPikPak, loginPikPakWithCaptcha, PIKPAK_PROTOCOL_CLIENT_ID, PIKPAK_PROTOCOL_CLIENT_VERSION, PIKPAK_PROTOCOL_PACKAGE_NAME } from '../auth'
import { MD5 } from 'crypto-js'

describe('PikPak captcha', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
  })

  it('generates random UUID-shaped device ids without binding them to credentials', () => {
    const first = createPikPakDeviceId()
    const second = createPikPakDeviceId()
    expect(first).toMatch(/^[a-f0-9]{32}$/)
    expect(second).toMatch(/^[a-f0-9]{32}$/)
    expect(first).not.toBe(second)
  })

  it('persists one separate device identity per account', () => {
    expect(getOrCreatePikPakDeviceId('User@Example.com')).toBe(getOrCreatePikPakDeviceId('user@example.com'))
    expect(getOrCreatePikPakDeviceId('first@example.com')).not.toBe(getOrCreatePikPakDeviceId('second@example.com'))
  })

  it('matches the MD5 salt chain used by web protocol captcha_sign', () => {
    const deviceId = 'deadbeefdeadbeefdeadbeefdeadbeef'
    const timestamp = '1710000000000'
    const salts = [
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
    let expected = PIKPAK_PROTOCOL_CLIENT_ID + PIKPAK_PROTOCOL_CLIENT_VERSION + PIKPAK_PROTOCOL_PACKAGE_NAME + deviceId + timestamp
    for (const salt of salts) expected = MD5(expected + salt).toString()
    expect(captchaSign(deviceId, timestamp)).toBe(`1.${expected}`)
  })

  it('includes captcha_sign and client meta for captcha/init', () => {
    const deviceId = '0123456789abcdef0123456789abcdef'
    const meta = buildPikPakCaptchaMeta(deviceId, { email: 'demo@example.com', timestamp: '1710000000000' })
    expect(meta.email).toBe('demo@example.com')
    expect(meta.client_version).toBe(PIKPAK_PROTOCOL_CLIENT_VERSION)
    expect(meta.package_name).toBe(PIKPAK_PROTOCOL_PACKAGE_NAME)
    expect(meta.timestamp).toBe('1710000000000')
    expect(meta.captcha_sign).toBe(captchaSign(deviceId, '1710000000000'))
    expect(meta.captcha_sign.startsWith('1.')).toBe(true)
  })

  it('uses username for login captcha meta exactly like rclone', () => {
    expect(buildPikPakLoginCaptchaMeta('demo@example.com')).toEqual({ username: 'demo@example.com' })
    expect(buildPikPakLoginCaptchaMeta('+8613812345678')).toEqual({ username: '+8613812345678' })
    expect(buildPikPakLoginCaptchaMeta(' demo_user ')).toEqual({ username: 'demo_user' })
  })

  it('logs in with the captcha token and without a client secret', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({ captcha_token: 'captcha-1', expires_in: 300 }), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ access_token: 'access', refresh_token: 'refresh', expires_in: 7200, token_type: 'Bearer', sub: 'account' }), { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)

    const token = await loginPikPak('demo@example.com', 'secret', '0123456789abcdef0123456789abcdef')

    const captchaRequest = JSON.parse(String(fetchMock.mock.calls[0][1]?.body))
    expect(captchaRequest).toEqual({
      action: 'POST:/v1/auth/signin',
      client_id: PIKPAK_PROTOCOL_CLIENT_ID,
      device_id: '0123456789abcdef0123456789abcdef',
      meta: { username: 'demo@example.com' }
    })
    const loginRequest = JSON.parse(String(fetchMock.mock.calls[1][1]?.body))
    expect(loginRequest).toEqual({ client_id: PIKPAK_PROTOCOL_CLIENT_ID, password: 'secret', username: 'demo@example.com' })
    expect((fetchMock.mock.calls[1][1]?.headers as Record<string, string>)['X-Captcha-Token']).toBe('captcha-1')
    expect(token.device_id).toBe('0123456789abcdef0123456789abcdef')
  })

  it('does not submit signin before a visual challenge is completed', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(new Response(JSON.stringify({ captcha_token: 'captcha-visual', url: 'https://captcha.example/challenge', expires_in: 300 }), { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)

    await expect(loginPikPak('demo@example.com', 'secret', '0123456789abcdef0123456789abcdef')).rejects.toMatchObject({
      code: 'PikPak_CAPTCHA_REQUIRED',
      deviceId: '0123456789abcdef0123456789abcdef',
      captchaToken: 'captcha-visual',
      challengeUrl: 'https://captcha.example/challenge'
    })
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it('uses the same device id and captcha token after the challenge is completed', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(new Response(JSON.stringify({ access_token: 'access', refresh_token: 'refresh', sub: 'account' }), { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)

    await loginPikPakWithCaptcha('demo@example.com', 'secret', 'captcha-visual', '0123456789abcdef0123456789abcdef')

    const request = fetchMock.mock.calls[0][1] as RequestInit
    expect((request.headers as Record<string, string>)['X-Device-Id']).toBe('0123456789abcdef0123456789abcdef')
    expect((request.headers as Record<string, string>)['X-Captcha-Token']).toBe('captcha-visual')
  })

  it('invalidates captcha_invalid code 4002 and retries login once', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({ captcha_token: 'captcha-1', expires_in: 300 }), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ error: 'captcha_invalid', error_code: 4002 }), { status: 400 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ captcha_token: 'captcha-2', expires_in: 300 }), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ access_token: 'access', refresh_token: 'refresh', sub: 'account' }), { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)

    await expect(loginPikPak('demo', 'secret', '0123456789abcdef0123456789abcdef')).resolves.toMatchObject({ access_token: 'access' })
    expect(fetchMock).toHaveBeenCalledTimes(4)
    expect((fetchMock.mock.calls[3][1]?.headers as Record<string, string>)['X-Captcha-Token']).toBe('captcha-2')
  })

  it('stops after a captcha_required response and exposes the fresh challenge', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({ captcha_token: 'captcha-1', expires_in: 300 }), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ error: 'captcha_required', error_code: 4001 }), { status: 400 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ captcha_token: 'captcha-2', url: 'https://captcha.example/challenge-2', expires_in: 300 }), { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)

    await expect(loginPikPak('demo', 'secret', '0123456789abcdef0123456789abcdef')).rejects.toMatchObject({
      code: 'PikPak_CAPTCHA_REQUIRED',
      captchaToken: 'captcha-2',
      challengeUrl: 'https://captcha.example/challenge-2'
    })
    expect(fetchMock).toHaveBeenCalledTimes(3)
  })
})
