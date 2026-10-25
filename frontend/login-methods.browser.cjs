const { chromium } = require(process.env.PLAYWRIGHT_MODULE || 'playwright')
const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const output = process.env.QA_OUTPUT || path.join(require('node:os').tmpdir(), 'mnemo-login-layout-qa')
fs.mkdirSync(output, { recursive: true })

async function assertCloseAtCorner(page, selector) {
  const geometry = await page.locator(selector).evaluate(panel => {
    const close = panel.querySelector('.modal-window-close, .qo-close, .pv-window-close, .pp-win-close')
    const bounds = panel.getBoundingClientRect()
    const button = close.getBoundingClientRect()
    return { top: button.top - bounds.top, right: bounds.right - button.right, width: button.width, height: button.height }
  })
  assert(Math.abs(geometry.top) < 1 && Math.abs(geometry.right) < 1 && geometry.width > 0 && geometry.height > 20, `${selector} 关闭按钮没有紧贴右上角：${JSON.stringify(geometry)}`)
}

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
        for (const id of ['pikpak', 'onedrive', 'dropbox', 'pan123', 'lanzou', 'ilanzou', 'yike', 'aliopen', 'webdav', 's3']) {
          providers.push({ ID: id, Meta: { label: id, icon: `drive-icons/${id}.svg` }, Login: { type: ['onedrive', 'dropbox', 'aliopen'].includes(id) ? 'oauth' : 'form', fields: [{ key: 'username', label: '账号', type: 'text' }, { key: 'password', label: '密码', type: 'password' }] } })
        }
        const listeners = {}
        window.runtime = new Proxy({ EventsOnMultiple: (name, handler) => { (listeners[name] ||= []).push(handler); return () => {} }, Environment: async () => ({ platform: 'windows' }), WindowGetSize: async () => ({ w: 1440, h: 1000 }) }, { get: (target, key) => target[key] || (() => {}) })
        const api = {
          ListProviders: async () => providers,
          ListAccounts: async () => [],
          GetSettings: async () => ({ theme, autoUpdate: false, defaultTab: 'pan' }),
          ListFavorites: async () => [],
          ListDownloads: async () => [],
          ListUploads: async () => [],
          SendThunderSMS: async config => {
            window.__smsConfig = config
            if (window.__requireVerification && !config.captcha_verified) throw new Error('thunder_verification_required\nurl=https://captcha.xunlei.com/verify\nattempt=qa-attempt\nsession=qa-session')
            return { login_attempt: 'qa-sms-attempt', device_id: config.device_id }
          },
          ShowThunderVerification: async () => false,
          CancelThunderLogin: async () => {},
        }
        window.go = { app: { App: new Proxy(api, { get: (target, key) => (...args) => target[key] ? target[key](...args) : Promise.resolve(null) }) } }
      }, theme)
      await page.route('https://captcha.xunlei.com/**', route => route.fulfill({ contentType: 'text/html', body: '<html><body>本地安全验证布局测试</body></html>' }))
      await page.goto(process.env.QA_URL || 'http://127.0.0.1:5173')
      await page.locator('.rail-add').click()
      await page.locator('.login-method-switch').waitFor()
      assert(await page.locator('#login-field-username').evaluate(element => element === document.activeElement), '登录页应聚焦账号输入框')
      const selectedProvider = page.locator('.lp-item[aria-selected="true"]')
      await selectedProvider.focus()
      await selectedProvider.press('End')
      assert(await page.locator('.lp-item[aria-selected="true"]').getAttribute('data-provider-id') === 's3', '网盘选择不支持 End')
      assert(!await page.locator('#mounted-region').isVisible(), '连接高级设置应默认收起')
      await page.locator('.login-advanced summary').click()
      assert(await page.locator('#mounted-region').isVisible(), '高级连接字段无法展开')
      await page.locator('#mounted-region').fill('ap-east-1')
      await page.locator('.login-advanced summary').click()
      await page.locator('.login-advanced summary').focus()
      await page.keyboard.press('Tab')
      assert(await page.getByRole('button', { name: '取消', exact: true }).evaluate(element => element === document.activeElement), '折叠字段不应进入 Tab 顺序')
      assert(await page.locator('#mounted-region').inputValue() === 'ap-east-1', '收起高级设置不应清除配置')
      await page.locator('.lp-item[aria-selected="true"]').press('Home')
      assert(await page.locator('.lp-item[aria-selected="true"]').getAttribute('data-provider-id') === 'thunder', '网盘选择不支持 Home')
      for (const width of [1440, 720, 600]) {
        await page.setViewportSize({ width, height: 1000 })
        await assertCloseAtCorner(page, '.login-modal')
        assert.equal(await page.locator('.login-provider-list').evaluate(element => getComputedStyle(element).scrollbarWidth), 'none', '左侧服务列表仍显示滚动条')
        for (const provider of ['thunder', 'pan139', 'pan189']) {
          await page.locator(`.lp-item[data-provider-id="${provider}"]`).click()
          const measurements = await page.locator('.login-method-switch').evaluate(element => {
            const parent = element.parentElement
            const styles = getComputedStyle(parent)
            return { control: element.getBoundingClientRect().width, content: parent.clientWidth - parseFloat(styles.paddingLeft) - parseFloat(styles.paddingRight), buttons: [...element.querySelectorAll('button')].map(button => button.getBoundingClientRect().width) }
          })
          assert(Math.abs(measurements.control - measurements.content) < 1, '登录切换没有占满右侧内容宽度')
          assert(Math.abs(measurements.buttons[0] - measurements.buttons[1]) < 1, '两个登录方式宽度没有平分')
          const alignment = await page.locator('.login-form').evaluate(form => {
            const mode = form.querySelector('.login-method-switch').getBoundingClientRect()
            const input = form.querySelector('.login-field input').getBoundingClientRect()
            return { left: mode.left - input.left, width: mode.width - input.width }
          })
          assert(Math.abs(alignment.left) < 1 && Math.abs(alignment.width) < 1, '登录切换与输入框宽度或左边界不一致')
          const phone = page.locator('.login-field').filter({ has: page.locator('label', { hasText: /^手机号$/ }) }).locator('input')
          const password = page.locator('.login-field').filter({ has: page.locator('label', { hasText: /^密码$/ }) }).locator('input')
          const code = page.locator('.login-field').filter({ has: page.locator('label', { hasText: /^短信验证码$/ }) }).locator('input')
          await phone.fill('13800138000')
          await password.fill('qa-password')
          await page.locator('.login-method-switch button').nth(1).click()
          assert(!await password.isVisible() && await code.isVisible(), '切换短信后表单不正确')
          const sms = await page.locator('.login-sms-field').evaluate(field => {
            const input = field.querySelector('input').getBoundingClientRect()
            const button = field.querySelector('button').getBoundingClientRect()
            return button.left > input.left && button.right <= input.right && button.top >= input.top && button.bottom <= input.bottom
          })
          assert(sms, '获取验证码按钮没有嵌入短信输入框')
          assert(await phone.inputValue() === '13800138000', '切换丢失手机号')
          await page.locator('.login-method-switch button').nth(0).click()
          assert(await password.isVisible() && await password.inputValue() === '', '切回密码后没有清理旧凭据')
        }
      }
      await page.locator('.lp-item[data-provider-id="guangya"]').click()
      assert(await page.locator('.login-method-switch').count() === 0, '光鸭单一短信登录显示了虚假切换')
      await page.setViewportSize({ width: 1440, height: 800 })
      await page.locator('.login-provider-list').hover()
      await page.mouse.wheel(0, 400)
      await page.waitForTimeout(200)
      assert(await page.locator('.login-provider-list').evaluate(element => element.scrollTop > 0), '隐藏滚动条后服务列表无法滚动')
      await page.locator('.lp-item[data-provider-id="thunder"]').click()
      await page.locator('.login-method-switch button').nth(1).click()
      await page.locator('.login-field').filter({ has: page.locator('label', { hasText: /^手机号$/ }) }).locator('input').fill('13800138000')
      await page.evaluate(() => { window.__requireVerification = true })
      await page.getByRole('button', { name: '获取验证码', exact: true }).click()
      await page.locator('.login-verification').waitFor()
      const verificationCenter = await page.locator('.login-verification').evaluate(element => {
        const bounds = element.getBoundingClientRect()
        return { x: (bounds.left + bounds.right - window.innerWidth) / 2, y: (bounds.top + bounds.bottom - window.innerHeight) / 2 }
      })
      assert(Math.abs(verificationCenter.x) < 1 && Math.abs(verificationCenter.y) < 1, '安全验证弹窗没有居中显示')
      await assertCloseAtCorner(page, '.login-verification')
      await page.screenshot({ path: path.join(output, `verification-${theme}.png`) })
      await page.keyboard.press('Escape')
      await page.locator('.login-verification').waitFor({ state: 'detached' })
      assert(await page.locator('.login-modal').isVisible(), '关闭验证同时关闭了登录页')
      await page.screenshot({ path: path.join(output, `login-${theme}.png`) })
      await page.getByRole('button', { name: '关闭添加网盘', exact: true }).click()
      await page.keyboard.press('Control+p')
      await page.locator('.qo-panel').waitFor()
      await assertCloseAtCorner(page, '.qo-panel')
      await page.getByRole('button', { name: '关闭快捷命令面板', exact: true }).click()
      assert.deepEqual(errors, [], '登录切换产生浏览器运行错误')
      console.log(`${theme}：桌面和窄窗口的表单对齐、验证码按钮、隐藏滚动条及滚动、居中安全验证、二级关闭按钮与 Esc 隔离通过；截图 ${output}`)
      await page.close()
    }
  } finally {
    await browser.close()
  }
}

main().catch(error => { console.error(error); process.exitCode = 1 })
