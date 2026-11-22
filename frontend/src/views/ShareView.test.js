import { mount, flushPromises } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { nextTick, h, KeepAlive, ref } from 'vue'

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
  api.onEvent.mockImplementation(() => () => {})
})

describe('分享页面交互', () => {
  it('缓存页面隐藏后停止监听，重新激活后恢复并丢弃旧响应', async () => {
    const off = vi.fn()
    api.onEvent.mockImplementation(() => off)
    let finishOld
    api.ListShareHistory.mockImplementationOnce(() => new Promise(resolve => { finishOld = resolve })).mockResolvedValue([])
    const visible = ref(true)
    wrapper = mount({ setup: () => () => h(KeepAlive, null, () => visible.value ? h(ShareView, { accounts: [{ user_id: 'webdav:test', drive_id: '' }] }) : h('div')) }, { global: { stubs: { UiIcon: true } } })
    await flushPromises()
    expect(api.onEvent).toHaveBeenCalledTimes(3)
    visible.value = false
    await nextTick()
    expect(off).toHaveBeenCalledTimes(3)
    expect(api.listShareImportReceipts).toHaveBeenCalledTimes(1)
    visible.value = true
    await flushPromises()
    expect(api.onEvent).toHaveBeenCalledTimes(6)
    expect(api.ListShareHistory).toHaveBeenCalledTimes(2)
    expect(api.listShareImportReceipts).toHaveBeenCalledTimes(2)
    finishOld([{ account_id: 'webdav:test', share_name: '旧响应', share_url: 'https://example.test/old' }])
    await flushPromises()
    expect(wrapper.text()).not.toContain('旧响应')
  })

  it('未知转存回执不提示成功，也不丢弃可核对记录', async () => {
    api.saveImportedShare.mockResolvedValueOnce({ id: 'receipt', accountId: 'pikpak:test', driveId: 'drive', updatedAt: 1, items: [{ sourceId: 'file', status: 'unknown' }], historySaved: true, message: '转存结果未确认' })
    wrapper = mount(ShareView, { props: { accounts: [{ user_id: 'pikpak:test', drive_id: 'drive' }] }, global: { stubs: { UiIcon: true } } })
    await flushPromises()
    wrapper.vm.openImport()
    wrapper.vm.importSession = { provider: 'pikpak', files: [{ fileId: 'file' }] }
    wrapper.vm.toggleImportFile(wrapper.vm.importSession.files[0])
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
    wrapper.vm.toggleImportFile(wrapper.vm.importSession.files[0])
    await wrapper.vm.saveImport()
    expect(wrapper.emitted('toast').at(-1)).toEqual([expect.stringContaining('结果不完整'), 'error'])
    expect(wrapper.vm.importOpen).toBe(true)
    expect([...wrapper.vm.importSelected]).toEqual(['file'])
  })

  it('请求被接收只提示已提交，不承诺远端落盘完成', async () => {
    api.saveImportedShare.mockResolvedValueOnce({ id: 'receipt', accountId: 'pikpak:test', driveId: 'drive', items: [{ sourceId: 'file', status: 'submitted' }], historySaved: true })
    wrapper = mount(ShareView, { props: { accounts: [{ user_id: 'pikpak:test', drive_id: 'drive' }] }, global: { stubs: { UiIcon: true } } })
    await flushPromises()
    wrapper.vm.openImport()
    wrapper.vm.importSession = { provider: 'pikpak', files: [{ fileId: 'file' }] }
    wrapper.vm.toggleImportFile(wrapper.vm.importSession.files[0])
    await wrapper.vm.saveImport()
    expect(wrapper.emitted('toast').at(-1)).toEqual([expect.stringContaining('已提交转存'), 'success'])
    expect(wrapper.vm.importOpen).toBe(false)
  })

  it('提交期间锁定选区，按提交顺序发送并按 ID 核对迟到回执', async () => {
    let resolveReceipt
    api.saveImportedShare.mockImplementationOnce(() => new Promise(resolve => { resolveReceipt = resolve }))
    wrapper = mount(ShareView, { props: { accounts: [{ user_id: 'pikpak:test', drive_id: 'drive' }] }, global: { stubs: { UiIcon: true } } })
    await flushPromises()
    const files = [{ fileId: 'first' }, { fileId: 'second' }, { fileId: 'third' }]
    wrapper.vm.openImport()
    wrapper.vm.importSession = { provider: 'pikpak', files }
    wrapper.vm.toggleAllImportFiles()
    wrapper.vm.toggleImportFile(files[1])
    wrapper.vm.toggleImportFile(files[1])
    const pending = wrapper.vm.saveImport()
    wrapper.vm.toggleImportFile(files[0])
    wrapper.vm.toggleAllImportFiles()
    expect([...wrapper.vm.importSelected]).toEqual(['first', 'third', 'second'])
    expect(api.saveImportedShare.mock.calls[0][3]).toEqual(['first', 'third', 'second'])
    await wrapper.setProps({ accounts: [{ user_id: 'pikpak:replacement', drive_id: 'replacement-drive' }] })
    await wrapper.vm.saveImport()
    expect(api.saveImportedShare).toHaveBeenCalledTimes(1)
    wrapper.vm.closeImport()
    expect(wrapper.vm.importOpen).toBe(true)
    resolveReceipt({ id: 'receipt', historySaved: true, items: ['second', 'first', 'third'].map(sourceId => ({ sourceId, status: 'submitted' })) })
    await pending
    expect(wrapper.emitted('toast').at(-1)).toEqual(['已提交转存 3 个项目，等待网盘处理', 'success'])
    expect(wrapper.vm.importedLocation.userId).toBe('pikpak:test')
    expect(wrapper.vm.importOpen).toBe(false)
  })

  it('通知集中到达时最多追加一次读取，并保留已加载的空状态', async () => {
    let reply
    let latest
    const record = { account_id: 'pikpak:test', share_id: 'first', share_name: '最新分享', share_url: 'https://example.test/share' }
    api.ListShareHistory.mockResolvedValueOnce([])
      .mockImplementationOnce(() => new Promise(resolve => { reply = resolve }))
      .mockImplementationOnce(() => new Promise(resolve => { latest = resolve }))
    wrapper = mount(ShareView, { global: { stubs: { UiIcon: true } } })
    await flushPromises()
    const notify = api.onEvent.mock.calls.find(([event]) => event === 'share:history-changed')[1]
    for (let count = 0; count < 30; count++) notify()
    await nextTick()
    expect(api.ListShareHistory).toHaveBeenCalledTimes(2)
    expect(wrapper.find('.share-skeleton').exists()).toBe(false)
    expect(wrapper.get('.share-empty').text()).toContain('还没有分享记录')
    expect(wrapper.get('.share-toolbar-actions [aria-busy="true"]').text()).toBe('刷新')
    reply([{ ...record, share_name: '过时分享' }])
    await flushPromises()
    expect(api.ListShareHistory).toHaveBeenCalledTimes(3)
    expect(wrapper.text()).not.toContain('过时分享')
    latest([record])
    await flushPromises()
    expect(wrapper.get('.share-record-name').text()).toBe('最新分享')
  })

  it('名称、链接和提取码搜索复用同一份索引', async () => {
    api.ListShareHistory.mockResolvedValueOnce([
      { account_id: 'pikpak:test', share_id: 'first', share_name: 'PDF 手册', share_url: 'https://example.test/Guide', share_pwd: 'ABC9' },
      { account_id: 'pikpak:test', share_id: 'second', share_name: '其他资料' },
    ])
    wrapper = mount(ShareView, { global: { stubs: { UiIcon: true } } })
    await flushPromises()
    for (const term of ['pdf', 'TEST/guide', 'abc9']) {
      wrapper.vm.kw = term
      await nextTick()
      expect(wrapper.findAll('.share-record-name').map(row => row.text())).toEqual(['PDF 手册'])
    }
    const index = wrapper.vm.searchIndex
    wrapper.vm.kw = '不匹配'
    await nextTick()
    expect(wrapper.vm.searchIndex).toBe(index)
    expect(wrapper.get('.share-empty').text()).toContain('没有匹配的分享记录')
  })

  it('手动刷新期间保留焦点且不重复请求', async () => {
    let reply
    api.ListShareHistory.mockResolvedValueOnce([])
      .mockImplementationOnce(() => new Promise(resolve => { reply = resolve }))
    wrapper = mount(ShareView, { attachTo: document.body, global: { stubs: { UiIcon: true } } })
    await flushPromises()
    const refresh = wrapper.get('.share-toolbar-actions [aria-busy]')
    refresh.element.focus()
    await refresh.trigger('click')
    expect(refresh.attributes('aria-busy')).toBe('true')
    expect(refresh.attributes('aria-disabled')).toBe('true')
    expect(refresh.attributes('disabled')).toBeUndefined()
    await refresh.trigger('click')
    reply([])
    await flushPromises()
    expect(api.ListShareHistory).toHaveBeenCalledTimes(2)
    expect(document.activeElement).toBe(refresh.element)
    expect(refresh.attributes('aria-busy')).toBe('false')
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
