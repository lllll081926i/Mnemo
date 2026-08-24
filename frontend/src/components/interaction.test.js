import { mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import Modal from './Modal.vue'
import UiSelect from './UiSelect.vue'

const api = vi.hoisted(() => ({
  login: vi.fn(),
  saveMounted: vi.fn(),
  validateMountedWrite: vi.fn(),
  SendGuangyaSms: vi.fn(),
  SendPan139SMS: vi.fn(),
  SendPan189SMS: vi.fn(),
  providerIconUrl: vi.fn(() => ''),
  OpenBrowser: vi.fn(),
  onEvent: vi.fn(() => () => {}),
  ClosePikPakCaptcha: vi.fn(),
  refreshAccount: vi.fn(),
  refreshAccountNow: vi.fn(),
  listDir: vi.fn(),
  listTrash: vi.fn(),
  search: vi.fn(),
  mkdir: vi.fn(),
  rename: vi.fn(),
  trash: vi.fn(),
  remove: vi.fn(),
  restore: vi.fn(),
  move: vi.fn(),
  copy: vi.fn(),
  favorite: vi.fn(),
  createShare: vi.fn(),
  uploadFiles: vi.fn(),
  validateUploadFiles: vi.fn(),
  migrateFiles: vi.fn(),
  download: vi.fn(),
  AddFavorite: vi.fn(),
  RemoveFavorite: vi.fn(),
  ListFavorites: vi.fn(),
  OfflineDownload: vi.fn(),
  PickDirectory: vi.fn(),
  PickFiles: vi.fn(),
  formatTime: vi.fn(() => ''),
  formatTimeParts: vi.fn(() => ({ date: '', clock: '' })),
  iconOf: vi.fn(() => ''),
  extOf: vi.fn(() => ''),
  openKindOf: vi.fn(() => ''),
  copyText: vi.fn(),
  capsOf: vi.fn(() => ({})),
  GetDirectoryCache: vi.fn(),
  SaveDirectoryCache: vi.fn(),
  DeleteDirectoryCache: vi.fn(),
  ListDirPage: vi.fn(),
  onFileChange: vi.fn(() => () => {}),
  notifyFileChange: vi.fn(),
  accountName: vi.fn((account) => account?.user_id || ''),
  providerOf: vi.fn((userId) => String(userId || '').split(/[_:]/, 1)[0]),
  providerMetaOf: vi.fn(() => ({ key: 'webdav', label: 'WebDAV' })),
  formatBytes: vi.fn((value) => `${value} B`),
}))

vi.mock('../api', () => api)
vi.mock('../logger', () => ({
  debug: vi.fn(),
  info: vi.fn(),
  warn: vi.fn(),
  error: vi.fn(),
  errorText: vi.fn((value) => String(value)),
  configKeys: vi.fn(() => []),
}))
vi.mock('../appearance', () => ({
  getPrefs: vi.fn(() => ({ accountOrder: [] })),
  setPref: vi.fn(),
  accountOrderKey: vi.fn((account) => account?.user_id || ''),
  orderAccounts: vi.fn((accounts) => [...(accounts || [])]),
}))

import LoginModal from './LoginModal.vue'
import AccountAvatar from './AccountAvatar.vue'
import AccountRail from './AccountRail.vue'
import PanView from '../views/PanView.vue'
import { warn as logWarn } from '../logger'

const storage = new Map()
Object.defineProperty(globalThis, 'localStorage', {
  configurable: true,
  value: {
    getItem: (key) => storage.has(key) ? storage.get(key) : null,
    setItem: (key, value) => storage.set(String(key), String(value)),
    removeItem: (key) => storage.delete(String(key)),
    clear: () => storage.clear(),
  },
})

const wrappers = []

function mountAttached(component, options = {}) {
  const wrapper = mount(component, { attachTo: document.body, ...options })
  wrappers.push(wrapper)
  return wrapper
}

async function setDomInput(input, value) {
  input.value = value
  input.dispatchEvent(new Event('input', { bubbles: true }))
  await nextTick()
}

afterEach(async () => {
  for (const wrapper of wrappers.splice(0)) wrapper.unmount()
  await nextTick()
  document.body.replaceChildren()
  localStorage.clear()
  vi.useRealTimers()
  vi.clearAllMocks()
})

describe('关键交互组件', () => {
  it('账号列右键“自定义”会把对应账号传给父组件', async () => {
    const account = { user_id: 'dropbox_account-1', drive_id: 'root', token: { user_name: 'Dropbox' } }
    const wrapper = mountAttached(AccountRail, {
      props: {
        accounts: [account],
        providers: [{ ID: 'dropbox', Meta: { label: 'Dropbox', icon: 'drive-icons/dropbox.svg' } }],
        current: account,
      },
      global: { stubs: { UiIcon: true } },
    })

    await wrapper.get('.rail-item').trigger('contextmenu', { clientX: 80, clientY: 96 })
    await nextTick()

    const entries = [...document.body.querySelectorAll('.ctx-item')]
    expect(entries.map((entry) => entry.textContent.trim())).toEqual(['账号信息', '自定义', '移除账号'])
    await entries[1].click()
    await nextTick()

    expect(wrapper.emitted('rename')).toEqual([[account]])
  })

  it('侧栏切换账号后文件页自动加载新账号目录，无需手动刷新', async () => {
    const first = { user_id: 'pikpak_first', drive_id: 'pikpak:first', token: { user_name: 'first' } }
    const second = { user_id: 'dropbox_second', drive_id: 'dropbox:second', token: { user_name: 'second' } }
    api.GetDirectoryCache.mockResolvedValue(null)
    api.ListFavorites.mockResolvedValue([])
    api.ListDirPage.mockImplementation(async (userId, driveId, dirId) => ({
      items: [{ file_id: `${userId}-${dirId}`, drive_id: driveId, name: `文件-${userId}`, isDir: false, size: 1 }],
      nextMarker: '',
    }))

    const wrapper = mountAttached(PanView, {
      props: { account: first, accounts: [first, second], providers: [] },
      shallow: true,
    })
    await vi.waitFor(() => {
      expect(api.ListDirPage).toHaveBeenCalledWith(first.user_id, first.drive_id, 'root', '')
    })

    await wrapper.setProps({ account: second, accounts: [first, second] })
    await vi.waitFor(() => {
      expect(api.ListDirPage).toHaveBeenCalledWith(second.user_id, second.drive_id, 'root', '')
    })
  })

  it('弹窗声明对话框语义，并由 Escape 请求关闭和恢复焦点', async () => {
    const opener = document.createElement('button')
    document.body.appendChild(opener)
    opener.focus()

    const wrapper = mountAttached(Modal, {
      props: { title: '删除文件' },
      slots: { default: '<button type="button">确认</button>' },
    })
    await nextTick()

    const dialog = document.querySelector('[role="dialog"]')
    expect(dialog).not.toBeNull()
    expect(dialog.getAttribute('aria-modal')).toBe('true')
    expect(dialog.getAttribute('aria-label')).toBe('删除文件')

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    expect(wrapper.emitted('close')).toHaveLength(1)

    wrapper.unmount()
    wrappers.splice(wrappers.indexOf(wrapper), 1)
    await nextTick()
    expect(document.activeElement).toBe(opener)
  })

  it('普通弹窗统一提供经典三键，沉浸式弹窗不渲染实体标题栏', async () => {
    const standard = mountAttached(Modal, { props: { title: '窗口控制' } })
    await nextTick()

    const controls = [...document.querySelectorAll('.modal-window-btn')]
    expect(controls).toHaveLength(3)
    expect(controls.map((button) => button.getAttribute('aria-label'))).toEqual(['最小化窗口', '最大化窗口', '关闭对话框'])

    standard.unmount()
    wrappers.splice(wrappers.indexOf(standard), 1)
    await nextTick()

    mountAttached(Modal, { props: { title: '图片预览', hideHead: true } })
    await nextTick()
    expect(document.querySelector('.modal-head')).toBeNull()
    expect(document.querySelectorAll('.modal-window-btn')).toHaveLength(0)
  })

  it('下拉框以键盘跳过禁用项并提交当前可选项', async () => {
    const wrapper = mountAttached(UiSelect, {
      props: {
        modelValue: 'alpha',
        placeholder: '选择供应商',
        options: [
          { value: 'alpha', label: 'Alpha' },
          { value: 'blocked', label: '不可用', disabled: true },
          { value: 'beta', label: 'Beta' },
        ],
      },
    })
    const trigger = wrapper.get('[role="combobox"]')

    await trigger.trigger('keydown', { key: 'ArrowDown' })
    await nextTick()
    expect(trigger.attributes('aria-expanded')).toBe('true')
    expect(document.querySelector('[role="listbox"]')).not.toBeNull()

    await trigger.trigger('keydown', { key: 'Enter' })
    expect(wrapper.emitted('update:modelValue')).toEqual([['beta']])
    expect(wrapper.emitted('change')).toEqual([['beta']])
    expect(trigger.attributes('aria-expanded')).toBe('false')
  })

  it('WebDAV 预设会同时填写连接名称和地址，且密码只有一个自定义切换按钮', async () => {
    localStorage.setItem('login_provider', 'webdav')
    const wrapper = mountAttached(LoginModal, {
      props: {
        providers: [
          { ID: 'webdav', Meta: { label: 'WebDAV' }, Login: { fields: [] } },
        ],
      },
      global: { stubs: { UiIcon: true } },
    })
    await nextTick()
    const preset = wrapper.findAllComponents(UiSelect)[0]
    await preset.vm.$emit('update:modelValue', 'jianguoyun')
    await preset.vm.$emit('change', 'jianguoyun')
    await nextTick()

    const inputs = [...document.body.querySelectorAll('input')]
    expect(inputs.find((input) => input.getAttribute('placeholder') === '我的 WebDAV / S3').value).toBe('坚果云')
    expect(inputs.find((input) => input.getAttribute('placeholder') === 'https://dav.example.com').value).toBe('https://dav.jianguoyun.com/dav/')
    expect(document.body.querySelectorAll('.password-toggle')).toHaveLength(1)
    expect(document.body.querySelector('[role="switch"]').getAttribute('aria-checked')).toBe('false')
  })

  it('普通蓝奏和天翼云使用内置选择项及安全默认值', async () => {
    localStorage.setItem('login_provider', 'lanzou')
    const lanzou = mountAttached(LoginModal, {
      props: {
        providers: [{
          ID: 'lanzou', Meta: { label: '蓝奏云' }, Login: { fields: [
            { key: 'username', type: 'text', label: '账号', required: true },
            { key: 'password', type: 'password', label: '密码', required: true },
            { key: 'upload_tier', type: 'select', label: '会员等级', options: [
              { value: 'v0', label: 'V0（100 MB）' },
              { value: 'v3', label: 'V3（550 MB）' },
            ] },
          ] },
        }],
      },
      global: { stubs: { UiIcon: true } },
    })
    await nextTick()
    const tier = lanzou.findComponent(UiSelect)
    expect(tier.props('modelValue')).toBe('v0')
    expect(tier.props('options')).toContainEqual({ value: 'v3', label: 'V3（550 MB）' })

    lanzou.unmount()
    wrappers.splice(wrappers.indexOf(lanzou), 1)
    localStorage.setItem('login_provider', 'pan189')
    const pan189 = mountAttached(LoginModal, {
      props: {
        providers: [{
          ID: 'pan189', Meta: { label: '天翼云盘' }, Login: { fields: [
            { key: 'username', type: 'text', label: '账号', required: true },
            { key: 'password', type: 'password', label: '密码', required: true },
            { key: 'cloud_type', type: 'select', label: '云空间', options: [
              { value: 'personal', label: '个人云' },
              { value: 'family', label: '家庭云' },
            ] },
          ] },
        }],
      },
      global: { stubs: { UiIcon: true } },
    })
    await nextTick()
    const cloudType = pan189.findComponent(UiSelect)
    expect(cloudType.props('modelValue')).toBe('personal')
    expect(cloudType.props('options')).toContainEqual({ value: 'family', label: '家庭云' })
  })

  it('139 账密触发安全校验后复用登录会话完成短信验证', async () => {
    localStorage.setItem('login_provider', 'pan139')
    api.login
      .mockRejectedValueOnce(new Error('pan139_sms_required\n139 登录需要短信安全校验'))
      .mockResolvedValueOnce(undefined)
    api.SendPan139SMS.mockResolvedValue(undefined)
    const wrapper = mountAttached(LoginModal, {
      props: {
        providers: [{
          ID: 'pan139', Meta: { label: '139 云盘' }, Login: { fields: [
            { key: 'login_mode', type: 'select', label: '登录方式', required: true, options: [] },
            { key: 'username', type: 'text', label: '手机号/账号', required: true },
            { key: 'password', type: 'password', label: '密码', required: false },
            { key: 'sms_code', type: 'text', label: '短信验证码', required: false },
          ] },
        }],
      },
      global: { stubs: { UiIcon: true } },
    })
    await nextTick()

    expect(document.body.querySelector('.lf-title')?.textContent).toBe('移动云盘')
    const inputForLabel = (label) => [...document.body.querySelectorAll('.login-field')]
      .find((field) => field.querySelector('label')?.textContent === label)
      ?.querySelector('input')
    await setDomInput(inputForLabel('手机号/账号'), '13800138000')
    await setDomInput(inputForLabel('密码'), 'password')
    document.body.querySelector('form').dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    await Promise.resolve()
    await nextTick()

    expect(api.login).toHaveBeenCalledTimes(1)
    expect(document.body.querySelector('.form-error')?.textContent).toContain('请获取并填写短信验证码')
    const smsField = [...document.body.querySelectorAll('.login-field')]
      .find((field) => field.querySelector('label')?.textContent === '短信验证码')
    expect(smsField.style.display).not.toBe('none')
    expect([...document.body.querySelectorAll('.login-field')]
      .find((field) => field.querySelector('label')?.textContent === '密码').style.display).toBe('none')

    const sendButton = [...smsField.querySelectorAll('button')].find((button) => button.textContent.includes('获取验证码'))
    await sendButton.click()
    await Promise.resolve()
    expect(api.SendPan139SMS).toHaveBeenCalledWith('13800138000')

    await setDomInput(inputForLabel('短信验证码'), '123456')
    document.body.querySelector('form').dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    await Promise.resolve()
    await nextTick()
    expect(api.login).toHaveBeenCalledTimes(2)
    expect(api.login.mock.calls[1][0]).toBe('pan139')
    expect(api.login.mock.calls[1][1]).toMatchObject({ login_mode: 'sms', username: '13800138000', sms_code: '123456' })
  })

  it('天翼云盘短信失败只在表单展示，重试后可用验证码登录', async () => {
    localStorage.setItem('login_provider', 'pan189')
    api.SendPan189SMS
      .mockRejectedValueOnce(new Error('短信发送失败'))
      .mockResolvedValueOnce(undefined)
    api.login.mockResolvedValue(undefined)
    const wrapper = mountAttached(LoginModal, {
      props: {
        providers: [{
          ID: 'pan189', Meta: { label: '189 云盘' }, Login: { fields: [
            { key: 'login_mode', type: 'select', label: '登录方式', required: true, options: [
              { value: 'password', label: '账号密码' },
              { value: 'sms', label: '短信验证码' },
            ] },
            { key: 'username', type: 'text', label: '手机号/邮箱', required: true },
            { key: 'password', type: 'password', label: '密码', required: false },
            { key: 'sms_code', type: 'text', label: '短信验证码', required: false },
            { key: 'cloud_type', type: 'select', label: '云空间', required: false, options: [] },
            { key: 'validate_code', type: 'text', label: '图形验证码', required: false },
          ] },
        }],
      },
      global: { stubs: { UiIcon: true } },
    })
    await nextTick()

    expect(document.body.querySelector('.lf-title')?.textContent).toBe('天翼云盘')
    const loginMode = wrapper.findAllComponents(UiSelect)
      .find((select) => select.props('options')?.some((option) => option.value === 'sms'))
    loginMode.vm.$emit('update:modelValue', 'sms')
    await nextTick()
    expect(document.body.querySelector('.lf-sub')?.textContent).toBe('短信验证码登录')

    const fieldForLabel = (label) => [...document.body.querySelectorAll('.login-field')]
      .find((field) => field.querySelector('label')?.textContent === label)
    const inputForLabel = (label) => fieldForLabel(label)?.querySelector('input')
    expect(fieldForLabel('密码').style.display).toBe('none')
    expect(fieldForLabel('短信验证码').style.display).not.toBe('none')

    await setDomInput(inputForLabel('手机号/邮箱'), '18900000000')
    const sendButton = [...fieldForLabel('短信验证码').querySelectorAll('button')]
      .find((button) => button.textContent.includes('获取验证码'))
    await sendButton.click()
    await Promise.resolve()
    await nextTick()
    expect(document.body.querySelector('.form-error')?.textContent).toContain('短信发送失败')
    expect(logWarn).not.toHaveBeenCalled()

    await sendButton.click()
    await Promise.resolve()
    expect(api.SendPan189SMS).toHaveBeenCalledTimes(2)
    expect(api.SendPan189SMS).toHaveBeenLastCalledWith('18900000000')

    await setDomInput(inputForLabel('短信验证码'), '654321')
    document.body.querySelector('form').dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    await Promise.resolve()
    await nextTick()
    expect(api.login).toHaveBeenCalledWith('pan189', expect.objectContaining({
      login_mode: 'sms', username: '18900000000', sms_code: '654321', cloud_type: 'personal',
    }))
  })

  it('S3 默认不做写入验证和内网预览授权，保存时只提交一次连接配置', async () => {
    api.saveMounted.mockResolvedValue(undefined)
    localStorage.setItem('login_provider', 's3')
    const wrapper = mountAttached(LoginModal, {
      props: {
        providers: [
          { ID: 'webdav', Meta: { label: 'WebDAV' }, Login: { fields: [] } },
          { ID: 's3', Meta: { label: 'S3' }, Login: { fields: [] } },
        ],
      },
      global: { stubs: { UiIcon: true } },
    })
    await nextTick()

    const values = { endpoint: 'https://s3.example.test', username: 'access-key', password: 'secret-key', bucket: 'mnemo' }
    const inputs = [...document.body.querySelectorAll('input')]
    await setDomInput(inputs.find((input) => input.placeholder === 's3.us-east-1.amazonaws.com (可选，默认 AWS)'), values.endpoint)
    const inputForLabel = (label) => [...document.body.querySelectorAll('.login-field')]
      .find((field) => field.querySelector('label')?.textContent.startsWith(label))
      ?.querySelector('input')
    await setDomInput(inputForLabel('Access Key ID'), values.username)
    await setDomInput(inputForLabel('Secret Access Key'), values.password)
    await setDomInput(inputForLabel('Bucket'), values.bucket)

    const switches = [...document.body.querySelectorAll('[role="switch"]')]
    expect(switches).toHaveLength(3)
    expect(switches[0].getAttribute('aria-checked')).toBe('false')
    expect(switches[1].getAttribute('aria-checked')).toBe('true')
    expect(switches[2].getAttribute('aria-checked')).toBe('false')
    document.body.querySelector('form').dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    await nextTick()

    expect(api.validateMountedWrite).not.toHaveBeenCalled()
    expect(api.saveMounted).toHaveBeenCalledTimes(1)
    expect(api.saveMounted.mock.calls[0][0]).toBe('s3')
    expect(api.saveMounted.mock.calls[0][1]).toMatchObject({ name: 'S3', endpoint: values.endpoint, username: values.username, password: values.password, bucket: values.bucket, allowPrivateNetwork: false })
    expect(api.saveMounted.mock.calls[0][1]).not.toHaveProperty('verifyWrite')
  })

  it('登录请求期间锁定服务商，避免异步结果串到另一个网盘', async () => {
    localStorage.setItem('login_provider', 'dropbox')
    let resolveLogin
    api.login.mockImplementation(() => new Promise((resolve) => { resolveLogin = resolve }))
    const wrapper = mountAttached(LoginModal, {
      props: {
        providers: [
          { ID: 'dropbox', Meta: { label: 'Dropbox' }, Login: { fields: [{ key: 'oauth', type: 'oauth', label: '浏览器授权' }] } },
          { ID: 'pikpak', Meta: { label: 'PikPak' }, Login: { fields: [{ key: 'username', type: 'text', label: '账号', required: true }] } },
        ],
      },
      global: { stubs: { UiIcon: true } },
    })
    await nextTick()
    document.body.querySelector('form').dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    await nextTick()

    expect(api.login).toHaveBeenCalledWith('dropbox', {})
    const providerButtons = [...document.body.querySelectorAll('.lp-item')]
    const pikpakButton = providerButtons.find((button) => button.textContent.includes('PikPak'))
    expect(pikpakButton.disabled).toBe(true)
    expect(api.login).toHaveBeenCalledTimes(1)
    expect(providerButtons.find((button) => button.classList.contains('active')).textContent).toContain('Dropbox')

    resolveLogin()
    await Promise.resolve()
    await nextTick()
  })

  it('登录失败由后端操作边界统一记录，表单只展示错误而不重复告警', async () => {
    localStorage.setItem('login_provider', 'pikpak')
    api.login.mockRejectedValueOnce(new Error('PikPak risk control'))
    const wrapper = mountAttached(LoginModal, {
      props: {
        providers: [{
          ID: 'pikpak',
          Meta: { label: 'PikPak' },
          Login: { fields: [
            { key: 'username', type: 'text', label: '账号', required: true },
            { key: 'password', type: 'password', label: '密码', required: true },
          ] },
        }],
      },
      global: { stubs: { UiIcon: true } },
    })
    await nextTick()

    const fieldInput = (label) => [...document.body.querySelectorAll('.login-field')]
      .find((field) => field.querySelector('label')?.textContent.startsWith(label))
      ?.querySelector('input')
    await setDomInput(fieldInput('账号'), 'first@example.com')
    await setDomInput(fieldInput('密码'), 'secret')
    document.body.querySelector('form').dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))

    await vi.waitFor(() => expect(document.body.textContent).toContain('PikPak 暂时限制了登录请求'))
    expect(logWarn).not.toHaveBeenCalled()
    expect(wrapper.exists()).toBe(true)
  })

  it('PikPak 验证内嵌在登录页，并把最终 token 原样续接登录', async () => {
    localStorage.setItem('login_provider', 'pikpak')
    let onCaptchaCompleted
    api.onEvent.mockImplementation((event, listener) => {
      if (event === 'pikpak:captcha:completed') onCaptchaCompleted = listener
      return () => {}
    })
    api.ClosePikPakCaptcha.mockResolvedValue(undefined)
    api.login
      .mockRejectedValueOnce(new Error('pikpak: captcha_required\nurl=https://captcha.example/challenge\ntoken=initial-token\nsession=session-1\ncallback=http://127.0.0.1:4567/callback/session-1'))
      .mockResolvedValueOnce(undefined)

    const wrapper = mountAttached(LoginModal, {
      props: {
        providers: [{
          ID: 'pikpak',
          Meta: { label: 'PikPak' },
          Login: { fields: [
            { key: 'username', type: 'text', label: '账号', required: true },
            { key: 'password', type: 'password', label: '密码', required: true },
          ] },
        }],
      },
      global: { stubs: { UiIcon: true } },
    })
    await nextTick()

    const fieldInput = (label) => [...document.body.querySelectorAll('.login-field')]
      .find((field) => field.querySelector('label')?.textContent.startsWith(label))
      ?.querySelector('input')
    await setDomInput(fieldInput('账号'), 'first@example.com')
    await setDomInput(fieldInput('密码'), 'secret')
    document.body.querySelector('form').dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    await vi.waitFor(() => {
      expect(api.login).toHaveBeenCalledTimes(1)
      const frame = document.querySelector('iframe.captcha-frame')
      expect(frame).not.toBeNull()
      expect(frame.getAttribute('src')).toContain('https://captcha.example/challenge')
      expect(frame.getAttribute('src')).toContain('redirect_uri=')
      expect(document.body.textContent).toContain('请在下方完成安全验证')
    })
    expect(onCaptchaCompleted).toEqual(expect.any(Function))

    onCaptchaCompleted({ session_id: 'session-1', captcha_token: 'verified-token' })
    await vi.waitFor(() => expect(api.login).toHaveBeenCalledTimes(2))
    expect(api.login.mock.calls[1][0]).toBe('pikpak')
    expect(api.login.mock.calls[1][1]).toMatchObject({
      username: 'first@example.com',
      password: 'secret',
      captcha_token: 'verified-token',
      captcha_verified: 'true',
    })
    expect(api.login.mock.calls[1][1]).not.toHaveProperty('captcha_requires_confirmation')
    await vi.waitFor(() => expect(wrapper.emitted('close')).toHaveLength(1))
  })

  it('PikPak 登录仅短暂禁用按钮，超过一小时的重试提示不在本地锁定', async () => {
    localStorage.setItem('login_provider', 'pikpak')
    api.login.mockRejectedValueOnce(new Error('PikPak login requests are rate limited; retry after 8 seconds'))
    const wrapper = mountAttached(LoginModal, {
      props: {
        providers: [{
          ID: 'pikpak',
          Meta: { label: 'PikPak' },
          Login: { fields: [
            { key: 'username', type: 'text', label: '账号', required: true },
            { key: 'password', type: 'password', label: '密码', required: true },
          ] },
        }],
      },
      global: { stubs: { UiIcon: true } },
    })
    await nextTick()
    const fieldInput = (label) => [...document.body.querySelectorAll('.login-field')]
      .find((field) => field.querySelector('label')?.textContent.startsWith(label))
      ?.querySelector('input')
    await setDomInput(fieldInput('账号'), 'first@example.com')
    await setDomInput(fieldInput('密码'), 'secret')
    const form = document.body.querySelector('form')
    const loginButton = () => document.body.querySelector('.login-actions .primary')

    form.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    await vi.waitFor(() => {
      expect(loginButton().disabled).toBe(true)
      expect(document.body.textContent).toContain('等待 8 秒后再试')
    })

    // 关闭第一轮弹窗会清理短计时器；下一轮用长 Retry-After 验证不会
    // 把按钮变为不可用，避免让真实计时器拖慢测试。
    wrapper.unmount()
    wrappers.splice(wrappers.indexOf(wrapper), 1)
    await nextTick()

    api.login.mockRejectedValueOnce(new Error('PikPak login requests are rate limited; retry after 3600 seconds'))
    const longWaitWrapper = mountAttached(LoginModal, {
      props: {
        providers: [{
          ID: 'pikpak',
          Meta: { label: 'PikPak' },
          Login: { fields: [
            { key: 'username', type: 'text', label: '账号', required: true },
            { key: 'password', type: 'password', label: '密码', required: true },
          ] },
        }],
      },
      global: { stubs: { UiIcon: true } },
    })
    await nextTick()
    await setDomInput(fieldInput('账号'), 'first@example.com')
    await setDomInput(fieldInput('密码'), 'secret')
    document.body.querySelector('form').dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    await vi.waitFor(() => {
      expect(document.body.querySelector('.login-actions .primary').disabled).toBe(false)
      expect(document.body.textContent).toContain('请稍后重试')
    })
    expect(longWaitWrapper.exists()).toBe(true)
  })

  it('账号容量在启动同步，并支持右上角手动同步', async () => {
    vi.useFakeTimers()
    api.refreshAccount.mockResolvedValue({ user_id: 'quota-dedupe', token: {}, usage: { size: 100, used: 20 } })
    api.refreshAccountNow.mockResolvedValue({ user_id: 'quota-dedupe', token: {}, usage: { size: 100, used: 20 } })
    const account = { user_id: 'quota-dedupe', token: {}, usage: null }
    const wrapper = mountAttached(AccountAvatar, { props: { account, providers: [] }, global: { stubs: { UiIcon: true } } })

    await vi.runAllTicks()
    expect(api.refreshAccount).toHaveBeenCalledTimes(1)
    expect(api.refreshAccountNow).not.toHaveBeenCalled()
    await wrapper.get('.acc-ava').trigger('mouseenter')
    await vi.advanceTimersByTimeAsync(120)
    const refreshButton = document.querySelector('.ap-refresh')
    expect(refreshButton).not.toBeNull()
    refreshButton.click()
    await vi.runAllTicks()
    expect(api.refreshAccountNow).toHaveBeenCalledTimes(1)
  })
})
