import { mount, flushPromises } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'

const api = vi.hoisted(() => ({
  listShareImportReceipts: vi.fn().mockResolvedValue([]),
  refreshShareImportReceipt: vi.fn(),
  accountName: vi.fn(account => account?.user_id || ''),
  onEvent: vi.fn(() => () => {}),
  copyText: vi.fn().mockResolvedValue(true),
}))
vi.mock('../api', () => api)
import ShareImportHistory from './ShareImportHistory.vue'

const account = { user_id: 'pikpak:one', drive_id: 'space' }
const receipt = { id: 'receipt', accountId: account.user_id, driveId: 'space', parentId: 'folder', updatedAt: 1, historySaved: true, items: [{ sourceId: 'file', status: 'submitted', taskIds: ['task'] }] }
let wrapper
afterEach(() => { wrapper?.unmount(); vi.clearAllMocks() })

describe('转存回执', () => {
  it('只查询原任务，确认后更新完成统计并能打开原目标', async () => {
    api.listShareImportReceipts.mockResolvedValueOnce([receipt])
    api.refreshShareImportReceipt.mockResolvedValueOnce({ ...receipt, updatedAt: 2, items: [{ ...receipt.items[0], status: 'completed' }] })
    wrapper = mount(ShareImportHistory, { props: { accounts: [account], latest: receipt } })
    await flushPromises()
    expect(wrapper.text()).toContain('已受理 1')
    await wrapper.findAll('button').find(button => button.text() === '核对状态').trigger('click')
    await flushPromises()
    expect(api.refreshShareImportReceipt).toHaveBeenCalledWith(account.user_id, 'space', 'receipt')
    expect(wrapper.text()).toContain('已完成 1')
    await wrapper.findAll('button').find(button => button.text() === '打开目录').trigger('click')
    expect(wrapper.emitted('navigate')[0][0]).toMatchObject({ userId: account.user_id, dirId: 'folder' })
  })

  it('刷新失败保留旧回执，账号移除后不显示其他作用域', async () => {
    api.listShareImportReceipts.mockResolvedValueOnce([receipt]).mockRejectedValueOnce(new Error('断网'))
    wrapper = mount(ShareImportHistory, { props: { accounts: [account] } })
    await flushPromises()
    await wrapper.vm.load()
    expect(wrapper.text()).toContain('已受理 1')
    expect(wrapper.get('[role="alert"]').text()).toContain('已保留原记录')
    await wrapper.setProps({ accounts: [] })
    await flushPromises()
    expect(wrapper.find('.share-record').exists()).toBe(false)
  })

  it('最终保存失败仍保留内存回执，允许复制服务端标识', async () => {
    const latest = { ...receipt, historySaved: false, updatedAt: 2 }
    wrapper = mount(ShareImportHistory, { props: { accounts: [account], latest } })
    await flushPromises()
    expect(wrapper.text()).toContain('最终回执未保存')
    await wrapper.findAll('button').find(button => button.text() === '复制回执').trigger('click')
    await flushPromises()
    expect(JSON.parse(api.copyText.mock.calls.at(-1)[0]).items[0].taskIds).toEqual(['task'])
  })
})
