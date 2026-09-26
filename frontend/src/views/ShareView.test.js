import { mount, flushPromises } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'

const api = vi.hoisted(() => ({
  ListShareHistory: vi.fn().mockResolvedValue([]),
  OpenBrowser: vi.fn(),
  onEvent: vi.fn(() => () => {}),
  providerOf: vi.fn(userId => String(userId).split(':')[0]),
  accountName: vi.fn(account => account?.user_id || ''),
  providerIconUrl: vi.fn(() => ''),
  providerMetaOf: vi.fn(() => ({ label: '网盘' })),
  capsOf: vi.fn(() => ({ importShare: true })),
  formatTime: vi.fn(() => ''),
  copyText: vi.fn(),
  importShare: vi.fn(),
  saveImportedShare: vi.fn(),
  cancelShare: vi.fn(),
}))

vi.mock('../api', () => api)

import ShareView from './ShareView.vue'

let wrapper
afterEach(() => {
  wrapper?.unmount()
  wrapper = null
  vi.clearAllMocks()
})

describe('分享页面交互', () => {
  it('刷新分享历史失败时保留旧记录并允许重试', async () => {
    api.ListShareHistory.mockResolvedValueOnce([{ account_id: 'pikpak:test', provider: 'pikpak', share_id: 'one', share_name: '已有分享', share_url: 'https://example.test/share' }])
      .mockRejectedValueOnce(new Error('网络异常')).mockResolvedValueOnce([])
    wrapper = mount(ShareView, { props: { accounts: [{ user_id: 'pikpak:test', drive_id: 'drive' }] }, global: { stubs: { UiIcon: true } } })
    await flushPromises()
    await wrapper.vm.refresh()
    await flushPromises()

    expect(wrapper.get('.share-record-name').text()).toBe('已有分享')
    expect(wrapper.get('[role="alert"]').text()).toContain('已保留上次加载的记录')
    await wrapper.get('[role="alert"] button').trigger('click')
    await flushPromises()
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
  })

  it('微调同一分享文案时保留手动填写的提取码，换链接时清除', async () => {
    wrapper = mount(ShareView, { props: { accounts: [{ user_id: 'pikpak:test', drive_id: 'drive' }] }, global: { stubs: { UiIcon: true } } })
    await flushPromises()
    wrapper.vm.openImport()
    wrapper.vm.importForm.url = 'https://mypikpak.com/s/first'
    await nextTick()
    wrapper.vm.importForm.password = 'wxyz'
    wrapper.vm.importForm.url = 'https://mypikpak.com/s/first 附注'
    await nextTick()
    expect(wrapper.vm.importForm.password).toBe('wxyz')
    wrapper.vm.importForm.url = 'https://mypikpak.com/s/second'
    await nextTick()
    expect(wrapper.vm.importForm.password).toBe('')
  })

  it('不同账号的同名分享标识互不占用复制和取消状态', () => {
    wrapper = mount(ShareView, { global: { stubs: { UiIcon: true } } })
    expect(wrapper.vm.itemKey({ account_id: 'first', share_id: 'same' }, 0)).not.toBe(
      wrapper.vm.itemKey({ account_id: 'second', share_id: 'same' }, 0),
    )
  })
})
