import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import vm from 'node:vm'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const bootstrap = readFileSync(resolve(process.cwd(), '../internal/captcha/thunder_bootstrap.js'), 'utf8')

describe('迅雷官方验证页初始化', () => {
  let sandbox
  let config

  beforeEach(() => {
    document.body.innerHTML = '<main id="captch-wrap" style="display:none"><input id="verification_code"></main>'
    document.getElementById('mnemo-verification-error')?.remove()
    config = null
    const sdk = { init: vi.fn(options => {
      config = options
      document.getElementById(options.IFRAME_BOX_ID).style.display = 'block'
      sdk.baseParams = { mobile: '138****8000', userID: '123' }
    }) }
    const page = { XlCaptcha: sdk }
    page.parent = page
    sandbox = {
      window: page,
      document,
      location: {
        protocol: 'https:', hostname: 'i.xunlei.com', pathname: '/xlcaptcha/vertifyPhone.html',
        search: '?appid=40&appName=ANDROID-test&deviceid=test-device&creditkey=review-key&mobile=138****8000&userID=123',
        replace: vi.fn(),
      },
      URL, URLSearchParams, setTimeout: vi.fn(),
    }
  })

  function initialize() {
    vm.runInNewContext(`(${bootstrap})('xlaccsdk01://xunlei.com/callback?state=harbor')`, sandbox)
  }

  it('调用官方 SDK，显示短信表单，防止重复绑定事件', () => {
    initialize()
    initialize()
    expect(sandbox.window.XlCaptcha.init).toHaveBeenCalledOnce()
    expect(config).toMatchObject({ IFRAME_BOX_ID: 'captch-wrap', PANEL_THEME: 'default', LANGUAGE: 'zh-CN', CLOSEABLE: false })
    expect(document.getElementById('captch-wrap').style.display).toBe('block')
    expect(sandbox.location.replace).not.toHaveBeenCalled()
  })

  it('只有官方验证成功才回传，并完整保留信任参数', () => {
    initialize()
    config.VERTIFY_SUCC_FUNC({ code: 100, creditkey: 'invalid' })
    expect(sandbox.location.replace).not.toHaveBeenCalled()
    config.VERTIFY_SUCC_FUNC({ code: '200', creditkey: 'verified+key/with==' })
    const callback = new URL(sandbox.location.replace.mock.calls[0][0])
    expect(callback.searchParams.get('state')).toBe('harbor')
    expect(callback.searchParams.get('creditkey')).toBe('verified+key/with==')
    expect(callback.searchParams.has('captcha_token')).toBe(false)
  })

  it('滑块 Token 与短信信任参数分开传递', () => {
    initialize()
    config.VERTIFY_SUCC_FUNC({ code: 200, captcha_token: 'verified-captcha-token' })
    const callback = new URL(sandbox.location.replace.mock.calls[0][0])
    expect(callback.searchParams.get('captcha_token')).toBe('verified-captcha-token')
    expect(callback.searchParams.has('creditkey')).toBe(false)
  })

  it.each(['iframe', 'provider', 'host', 'path', 'protocol'])('不在其他页面或内部图形验证 iframe 初始化：%s', change => {
    if (change === 'iframe') sandbox.window.parent = {}
    if (change === 'provider') sandbox.location.hostname = 'user.mypikpak.com'
    if (change === 'host') sandbox.location.hostname = 'xunlei.com.evil.test'
    if (change === 'path') sandbox.location.pathname = '/xlcaptcha/pic.html'
    if (change === 'protocol') sandbox.location.protocol = 'http:'
    initialize()
    expect(sandbox.window.XlCaptcha.init).not.toHaveBeenCalled()
    expect(sandbox.location.replace).not.toHaveBeenCalled()
  })

  it.each(['sdk', 'params', 'init', 'result', 'locale'])('加载或验证结果异常时显示提示，不冒充验证成功：%s', failure => {
    if (failure === 'sdk') sandbox.window.XlCaptcha = undefined
    if (failure === 'params') sandbox.location.search = '?appid=40'
    if (failure === 'init') sandbox.window.XlCaptcha.init.mockImplementation(() => { throw new Error('private-key') })
    initialize()
    if (failure === 'result') config.VERTIFY_SUCC_FUNC({ code: 200 })
    if (failure === 'locale') {
      sandbox.window.XlCaptcha.baseParams = undefined
      sandbox.setTimeout.mock.calls[0][0]()
    }
    expect(document.getElementById('mnemo-verification-error')?.textContent).toMatch(/迅雷/)
    expect(document.documentElement.textContent).not.toContain('private-key')
    expect(sandbox.location.replace).not.toHaveBeenCalled()
  })
})
