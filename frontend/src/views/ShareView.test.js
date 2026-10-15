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
  listShareImportReceipts: vi.fn().mockResolvedValue([]),
  refreshShareImportReceipt: vi.fn(),
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
  it('未知转存回执不提示成功，也不丢弃可核对记录', async () => {
    api.saveImportedShare.mockResolvedValueOnce({ id: 'receipt', accountId: 'pikpak:test', driveId: 'drive', updatedAt: 1, items: [{ sourceId: 'file', status: 'unknown' }], historySaved: true, message: '转存结果未确认' })
    wrapper = mount(ShareView, { props: { accounts: [{ user_id: 'pikpak:test', drive_id: 'drive' }] }, global: { stubs: { UiIcon: true } } })
    await flushPromises()
    wrapper.vm.openImport()
    wrapper.vm.importSession = { provider: 'pikpak', files: [{ fileId: 'file' }] }
    wrapper.vm.importSelected = ['file']
    await wrapper.vm.saveImport()
    expect(wrapper.emitted('toast').at(-1)).toEqual(['转存结果未确认', 'warn'])
    expect(wrapper.vm.lastImportReceipt.items[0].status).toBe('unknown')
    expect(wrapper.vm.importOpen).toBe(false)
  })
  it('转存无回执时不误报全部成功，保留选择', async () => {
    api.saveImportedShare.mockResolvedValueOnce([])
    wrapper = mount(ShareView, { props: { accounts: [{ user_id: 'pikpak:test', drive_id: 'drive' }] }, global: { stubs: { UiIcon: true } } })
    await flushPromises()
    wrapper.vm.openImport()
    wrapper.vm.importSession = { provider: 'pikpak', files: [{ fileId: 'file' }] }
    wrapper.vm.importSelected = ['file']
    await wrapper.vm.saveImport()
    expect(wrapper.emitted('toast').at(-1)).toEqual([expect.stringContaining('结果不完整'), 'error'])
    expect(wrapper.vm.importOpen).toBe(true)
    expect(wrapper.vm.importSelected).toEqual(['file'])
  })

  it('请求被接收只提示已提交，不承诺远端落盘完成', async () => {
    api.saveImportedShare.mockResolvedValueOnce({ id: 'receipt', accountId: 'pikpak:test', driveId: 'drive', items: [{ sourceId: 'file', status: 'submitted' }], historySaved: true })
    wrapper = mount(ShareView, { props: { accounts: [{ user_id: 'pikpak:test', drive_id: 'drive' }] }, global: { stubs: { UiIcon: true } } })
    await flushPromises()
    wrapper.vm.openImport()
    wrapper.vm.importSession = { provider: 'pikpak', files: [{ fileId: 'file' }] }
    wrapper.vm.importSelected = ['file']
    await wrapper.vm.saveImport()
    expect(wrapper.emitted('toast').at(-1)).toEqual([expect.stringContaining('已提交转存'), 'success'])
    expect(wrapper.vm.importOpen).toBe(false)
  })

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
