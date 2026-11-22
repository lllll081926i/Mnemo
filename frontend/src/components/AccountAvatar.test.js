import { flushPromises, mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'

const api = vi.hoisted(() => ({
  refreshAccountSilently: vi.fn(),
  refreshAccountNow: vi.fn(),
  accountName: vi.fn(account => account?.user_id || ''),
  providerIconUrl: vi.fn(() => ''),
  providerMetaOf: vi.fn(() => ({ label: '网盘' })),
  formatBytes: vi.fn(value => `${value} B`),
}))
const health = vi.hoisted(() => ({ recordAccountHealth: vi.fn() }))

vi.mock('../api', () => api)
vi.mock('../workspace', () => health)

import AccountAvatar from './AccountAvatar.vue'

let wrapper
afterEach(() => {
  wrapper?.unmount()
  wrapper = null
  document.body.replaceChildren()
  vi.clearAllMocks()
})

describe('账号容量刷新', () => {
  it('手动刷新等待静默请求后再强制请求，显示最新容量', async () => {
    let finishSilent
    const account = { user_id: 'quota:manual', token: {}, usage: null }
    api.refreshAccountSilently.mockImplementation(() => new Promise(resolve => { finishSilent = resolve }))
    api.refreshAccountNow.mockResolvedValue({ ...account, usage: { size: 100, used: 40 } })
    wrapper = mount(AccountAvatar, { props: { account }, global: { stubs: { UiIcon: true } } })
    const manual = wrapper.vm.manualRefresh()
    expect(api.refreshAccountNow).not.toHaveBeenCalled()

    finishSilent({ ...account, usage: { size: 100, used: 20 } })
    await manual
    expect(api.refreshAccountNow).toHaveBeenCalledOnce()
    expect(wrapper.vm.used).toBe(40)
    expect(health.recordAccountHealth).toHaveBeenCalledWith(account.user_id)
  })

  it('强制刷新失败时在容量弹窗显示提示', async () => {
    const account = { user_id: 'quota:failure', token: {}, usage: null }
    api.refreshAccountSilently.mockResolvedValue(account)
    api.refreshAccountNow.mockRejectedValue(new Error('网络失败'))
    wrapper = mount(AccountAvatar, { props: { account }, global: { stubs: { UiIcon: true } } })
    await flushPromises()
    wrapper.vm.show = true
    await wrapper.vm.manualRefresh()
    await flushPromises()
    expect(document.body.querySelector('[role="alert"]')?.textContent).toContain('刷新失败')
  })
})

describe('容量弹窗键盘访问', () => {
  it('焦点进入头像可打开弹窗，Escape 关闭并收回焦点', async () => {
    vi.useFakeTimers()
    try {
      const account = { user_id: 'quota:keyboard', token: {}, usage: null }
      api.refreshAccountSilently.mockResolvedValue(account)
      wrapper = mount(AccountAvatar, { attachTo: document.body, props: { account }, global: { stubs: { UiIcon: true } } })
      const root = wrapper.get('.acc-ava')
      expect(root.attributes('tabindex')).toBe('0')
      root.element.focus()
      await vi.advanceTimersByTimeAsync(200)
      expect(wrapper.vm.show).toBe(true)
      expect(document.body.querySelector('.acc-pop')).toBeTruthy()

      window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true }))
      await nextTick()
      expect(wrapper.vm.show).toBe(false)
      expect(document.activeElement).toBe(root.element)
    } finally {
      vi.useRealTimers()
    }
  })
})
