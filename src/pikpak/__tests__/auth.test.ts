import { describe, expect, it } from 'vitest'
import { buildPikPakCaptchaMeta, buildPikPakLoginCaptchaMeta, captchaSign, createPikPakDeviceId, PIKPAK_PROTOCOL_CLIENT_ID, PIKPAK_PROTOCOL_CLIENT_VERSION, PIKPAK_PROTOCOL_PACKAGE_NAME } from '../auth'
import { MD5 } from 'crypto-js'

describe('PikPak captcha', () => {
  it('builds stable device ids from username', () => {
    expect(createPikPakDeviceId('User@Example.com')).toBe(createPikPakDeviceId('user@example.com'))
    expect(createPikPakDeviceId('a')).not.toBe(createPikPakDeviceId('b'))
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
    const deviceId = createPikPakDeviceId('demo@example.com')
    const meta = buildPikPakCaptchaMeta(deviceId, { email: 'demo@example.com', timestamp: '1710000000000' })
    expect(meta.email).toBe('demo@example.com')
    expect(meta.client_version).toBe(PIKPAK_PROTOCOL_CLIENT_VERSION)
    expect(meta.package_name).toBe(PIKPAK_PROTOCOL_PACKAGE_NAME)
    expect(meta.timestamp).toBe('1710000000000')
    expect(meta.captcha_sign).toBe(captchaSign(deviceId, '1710000000000'))
    expect(meta.captcha_sign.startsWith('1.')).toBe(true)
  })

  it('maps login account into captcha meta fields', () => {
    expect(buildPikPakLoginCaptchaMeta('demo@example.com')).toEqual({ email: 'demo@example.com' })
    expect(buildPikPakLoginCaptchaMeta('+8613812345678')).toEqual({ phone_number: '+8613812345678' })
    expect(buildPikPakLoginCaptchaMeta('demo_user')).toEqual({ username: 'demo_user' })
  })
})
