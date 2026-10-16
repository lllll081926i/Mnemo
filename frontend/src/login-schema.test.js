import { describe, expect, it } from 'vitest'
import { loginDefaults, loginFields, loginFieldRequired, loginFieldVisible, selectLoginMethod } from './login-schema'

describe('驱动登录声明', () => {
  it('字段用途、隐藏、模式和默认值不依赖凭据名称', () => {
    const fields = [
      { key: 'choice', purpose: 'mode', default: 'password' },
      { key: 'opaque', hidden: true },
      { key: 'credential', purpose: 'password', modes: ['password'], requiredModes: ['password'] },
      { key: 'verification', purpose: 'sms', modes: ['sms'], requiredModes: ['sms'] },
    ]
    expect(loginDefaults({ fields })).toEqual({ choice: 'password' })
    expect(loginFields({ fields }).map(field => field.key)).toEqual(['credential', 'verification'])
    expect(loginFieldVisible(fields[2], 'sms')).toBe(false)
    expect(loginFieldRequired(fields[3], 'sms')).toBe(true)
    expect(loginFieldRequired(fields[3], 'password')).toBe(false)
  })
  it('切换授权来源清除隐藏凭据、同意记录和验证会话', () => {
    const method = { id: 'relay', fieldKeys: ['refresh_token'] }
    expect(selectLoginMethod({ refresh_token: 'kept', client_secret: 'secret', relay_consent: 'true', captcha_token: 'stale' }, method)).toEqual({ refresh_token: 'kept', login_method: 'relay' })
  })
})
