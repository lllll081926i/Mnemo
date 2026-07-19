import { describe, expect, it } from 'vitest'
import { buildCloud189LoginStartUrl, buildCloud189QrPollForm, parseCloud189LoginContext, type Cloud189QrState } from '../auth'

describe('Cloud189 login helpers', () => {
  it('starts from the current cloud login redirect endpoint', () => {
    const url = new URL(buildCloud189LoginStartUrl())
    expect(url.origin + url.pathname).toBe('https://cloud.189.cn/api/portal/loginUrl.action')
    expect(url.searchParams.get('redirectURL')).toBe('https://cloud.189.cn/main.action')
  })

  it('reads dynamic login parameters from the redirected page', () => {
    const context = parseCloud189LoginContext('https://open.e.189.cn/api/logbox/separate/web/index.html?appId=cloud&lt=login-token&reqId=request-id')
    expect(context.appId).toBe('cloud')
    expect(context.lt).toBe('login-token')
    expect(context.reqId).toBe('request-id')
  })

  it('uses appConf values when polling the QR login state', () => {
    const state: Cloud189QrState = {
      appId: 'cloud',
      clientType: '1',
      returnUrl: 'https://cloud.189.cn/callback',
      loginUrl: 'https://open.e.189.cn/login',
      lt: 'lt',
      reqId: 'req',
      paramId: 'param',
      qrUrl: 'https://open.e.189.cn/qr',
      encryuuid: 'encrypted',
      createdAt: 0
    }
    const form = buildCloud189QrPollForm(state, new Date('2026-07-19T06:00:00.000Z'))
    expect(form.get('appId')).toBe('cloud')
    expect(form.get('clientType')).toBe('1')
    expect(form.get('returnUrl')).toBe(state.returnUrl)
    expect(form.get('paramId')).toBe('param')
  })
})
