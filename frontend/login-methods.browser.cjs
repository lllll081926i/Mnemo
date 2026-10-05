const { chromium } = require(process.env.PLAYWRIGHT_MODULE || 'playwright')
const assert = require('node:assert/strict')

async function main() {
  const browser = await chromium.launch({ channel: 'msedge', headless: true })
  try {
    for (const theme of ['light', 'dark']) {
      const page = await browser.newPage({ viewport: { width: 1440, height: 1000 } })
      const errors = []
      page.on('pageerror', error => errors.push(error.message))
      await page.addInitScript(theme => {
        localStorage.setItem('login_provider', 'thunder')
        const fields = [
          { key: 'login_mode', type: 'select', label: '登录方式', required: true, options: [{ value: 'password', label: '密码登录' }, { value: 'sms', label: '短信验证码登录' }] },
          { key: 'username', type: 'text', label: '手机号', required: true },
          { key: 'password', type: 'password', label: '密码', required: true },
          { key: 'sms_code', type: 'text', label: '短信验证码' },
        ]
        const providers = ['thunder', 'pan139', 'pan189'].map(id => ({ ID: id, Meta: { label: id, icon: `drive-icons/${id}.svg` }, Login: { fields } }))
        providers.push({ ID: 'guangya', Meta: { label: '光鸭', icon: 'drive-icons/guangya.svg' }, Login: { fields: [{ key: 'phone', label: '手机号', type: 'text' }, { key: 'sms_code', label: '短信验证码', type: 'text' }] } })
        const listeners = {}
        window.runtime = new Proxy({ EventsOnMultiple: (name, handler) => { (listeners[name] ||= []).push(handler); return () => {} }, Environment: async () => ({ platform: 'windows' }), WindowGetSize: async () => ({ w: 1440, h: 1000 }) }, { get: (target, key) => target[key] || (() => {}) })
        const api = {
          ListProviders: async () => providers,
          ListAccounts: async () => [],
          GetSettings: async () => ({ theme, autoUpdate: false, defaultTab: 'pan' }),
          ListFavorites: async () => [],
          ListDownloads: async () => [],
          ListUploads: async () => [],
          SendThunderSMS: async config => { window.__smsConfig = config; return { login_attempt: 'qa-sms-attempt', device_id: config.device_id } },
          CancelThunderLogin: async () => {},
        }
        window.go = { app: { App: new Proxy(api, { get: (target, key) => (...args) => target[key] ? target[key](...args) : Promise.resolve(null) }) } }
      }, theme)
      await page.goto(process.env.QA_URL || 'http://127.0.0.1:5173')
      await page.locator('.rail-add').click()
      await page.locator('.login-method-switch').waitFor()
      for (const width of [1440, 720]) {
        await page.setViewportSize({ width, height: 1000 })
        for (const provider of ['thunder', 'pan139', 'pan189']) {
          await page.locator(`.lp-item[data-provider-id="${provider}"]`).click()
          const measurements = await page.locator('.login-method-switch').evaluate(element => {
            const parent = element.parentElement
            const styles = getComputedStyle(parent)
            return { control: element.getBoundingClientRect().width, content: parent.clientWidth - parseFloat(styles.paddingLeft) - parseFloat(styles.paddingRight), buttons: [...element.querySelectorAll('button')].map(button => button.getBoundingClientRect().width) }
          })
          assert(Math.abs(measurements.control - measurements.content) < 1, '登录切换没有占满右侧内容宽度')
          assert(Math.abs(measurements.buttons[0] - measurements.buttons[1]) < 1, '两个登录方式宽度没有平分')
          const phone = page.locator('.login-field').filter({ has: page.locator('label', { hasText: /^手机号$/ }) }).locator('input')
          const password = page.locator('.login-field').filter({ has: page.locator('label', { hasText: /^密码$/ }) }).locator('input')
          const code = page.locator('.login-field').filter({ has: page.locator('label', { hasText: /^短信验证码$/ }) }).locator('input')
          await phone.fill('13800138000')
          await password.fill('qa-password')
          await page.locator('.login-method-switch button').nth(1).click()
          assert(!await password.isVisible() && await code.isVisible(), '切换短信后表单不正确')
          assert(await phone.inputValue() === '13800138000', '切换丢失手机号')
          await page.locator('.login-method-switch button').nth(0).click()
          assert(await password.isVisible() && await password.inputValue() === '', '切回密码后没有清理旧凭据')
        }
      }
      await page.locator('.lp-item[data-provider-id="guangya"]').click()
      assert(await page.locator('.login-method-switch').count() === 0, '光鸭单一短信登录显示了虚假切换')
      assert.deepEqual(errors, [], '登录切换产生浏览器运行错误')
      console.log(`${theme}：迅雷、移动、天翼在桌面和窄窗口均为全宽等分；切换与凭据清理通过`)
      await page.close()
    }
  } finally {
    await browser.close()
  }
}

main().catch(error => { console.error(error); process.exitCode = 1 })
