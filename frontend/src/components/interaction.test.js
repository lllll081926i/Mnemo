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
  providerIconUrl: vi.fn(() => ''),
  OpenBrowser: vi.fn(),
  onEvent: vi.fn(() => () => {}),
  ClosePikPakCaptcha: vi.fn(),
  refreshAccount: vi.fn(),
  refreshAccountNow: vi.fn(),
  accountName: vi.fn((account) => account?.user_id || ''),
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

import LoginModal from './LoginModal.vue'
import AccountAvatar from './AccountAvatar.vue'

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

  it('账号容量在启动同步，并支持右上角手动同步', async () => {
    vi.useFakeTimers()
    api.refreshAccountNow.mockResolvedValue({ user_id: 'quota-dedupe', token: {}, usage: { size: 100, used: 20 } })
    const account = { user_id: 'quota-dedupe', token: {}, usage: null }
    const wrapper = mountAttached(AccountAvatar, { props: { account, providers: [] }, global: { stubs: { UiIcon: true } } })

    await vi.runAllTicks()
    expect(api.refreshAccountNow).toHaveBeenCalledTimes(1)
    await wrapper.get('.acc-ava').trigger('mouseenter')
    await vi.advanceTimersByTimeAsync(120)
    const refreshButton = document.querySelector('.ap-refresh')
    expect(refreshButton).not.toBeNull()
    refreshButton.click()
    await vi.runAllTicks()
    expect(api.refreshAccountNow).toHaveBeenCalledTimes(2)
  })
})
