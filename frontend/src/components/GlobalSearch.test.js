import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'

const api = vi.hoisted(() => ({
  search: vi.fn(),
  capsOf: vi.fn(() => ({ search: true })),
  accountName: vi.fn(account => account?.user_id || ''),
  providerMetaOf: vi.fn(() => ({ label: '网盘' })),
  providerIconUrl: vi.fn(() => ''),
}))

vi.mock('../api', () => api)

import GlobalSearch from './GlobalSearch.vue'

const account = { user_id: 'webdav:test', drive_id: 'drive' }
let wrapper
afterEach(() => {
  wrapper?.unmount()
  wrapper = null
  vi.unstubAllGlobals()
  vi.clearAllMocks()
})

describe('全盘搜索', () => {
  it('接收顶部搜索框的文字并自动搜索本地缓存', async () => {
    const searchCache = vi.fn().mockResolvedValue([])
    vi.stubGlobal('go', { app: { App: { SearchCachedFiles: searchCache } } })
    wrapper = mount(GlobalSearch, { props: { accounts: [account], providers: [], initialQuery: '设计资料' }, global: { stubs: { UiIcon: true } } })
    await flushPromises()
    expect(wrapper.vm.keyword).toBe('设计资料')
    expect(searchCache).toHaveBeenCalledWith('设计资料')
    expect(api.search).not.toHaveBeenCalled()
  })
  it('在线结果不截断，并按批次显示', async () => {
    vi.stubGlobal('go', { app: { App: { SearchCachedFiles: vi.fn().mockResolvedValue([]) } } })
    api.search.mockResolvedValue(Array.from({ length: 601 }, (_, index) => ({ file_id: `file-${index}`, name: `文件-${index}` })))
    wrapper = mount(GlobalSearch, { props: { accounts: [account], providers: [] }, global: { stubs: { UiIcon: true } } })
    wrapper.vm.keyword = '文件'
    wrapper.vm.remote = true
    await wrapper.vm.run()
    await flushPromises()
    expect(wrapper.vm.results).toHaveLength(601)
    expect(document.body.querySelectorAll('.workspace-result')).toHaveLength(200)
    expect(document.body.textContent).toContain('显示更多')
    expect(wrapper.vm.errors).toEqual([])
  })

  it('停止在线搜索时取消后端请求', async () => {
    let finishOnline
    const cancel = vi.fn().mockResolvedValue()
    vi.stubGlobal('go', { app: { App: { SearchCachedFiles: vi.fn().mockResolvedValue([]), CancelSearch: cancel } } })
    api.search.mockImplementationOnce(() => new Promise(resolve => { finishOnline = resolve }))
    wrapper = mount(GlobalSearch, { props: { accounts: [account], providers: [] }, global: { stubs: { UiIcon: true } } })
    wrapper.vm.keyword = '文件'
    wrapper.vm.remote = true
    const running = wrapper.vm.run()
    await flushPromises()
    wrapper.vm.stop()
    expect(cancel).toHaveBeenCalledWith(api.search.mock.calls.at(-1)[3])
    finishOnline([{ file_id: 'late', name: '迟到文件' }])
    await running
    expect(wrapper.vm.results).toEqual([])
  })

  it('缓存未返回时先展示在线结果，缓存晚到也不覆盖在线版本', async () => {
    let finishCache
    vi.stubGlobal('go', { app: { App: { SearchCachedFiles: vi.fn(() => new Promise(resolve => { finishCache = resolve })) } } })
    api.search.mockResolvedValue([{ file_id: 'same', name: '在线版本', parent_file_id: 'root' }])
    wrapper = mount(GlobalSearch, { props: { accounts: [account], providers: [] }, global: { stubs: { UiIcon: true } } })
    wrapper.vm.keyword = '文档'
    wrapper.vm.remote = true

    const running = wrapper.vm.run()
    await flushPromises()
    expect(wrapper.vm.results.map(result => result.file.name)).toEqual(['在线版本'])
    finishCache([
      { userId: account.user_id, driveId: account.drive_id, file: { file_id: 'same', name: '过期缓存' } },
      { userId: account.user_id, driveId: account.drive_id, file: { file_id: 'other', name: '缓存独有' } },
    ])
    await running
    expect(wrapper.vm.results.map(result => result.file.name)).toEqual(['缓存独有', '在线版本'])
  })

  it('缓存搜索失败时仍完成在线搜索', async () => {
    vi.stubGlobal('go', { app: { App: { SearchCachedFiles: vi.fn().mockRejectedValue(new Error('缓存损坏')) } } })
    api.search.mockResolvedValue([{ file_id: 'online', name: '在线文件' }])
    wrapper = mount(GlobalSearch, { props: { accounts: [account], providers: [] }, global: { stubs: { UiIcon: true } } })
    wrapper.vm.keyword = '文件'
    wrapper.vm.remote = true

    await wrapper.vm.run()
    expect(wrapper.vm.results.map(result => result.file.name)).toEqual(['在线文件'])
    expect(wrapper.vm.errors).toContain('缓存搜索失败，请重试')
  })

  it('停止搜索后忽略迟到结果并显示已停止', async () => {
    let finishCache
    vi.stubGlobal('go', { app: { App: { SearchCachedFiles: vi.fn(() => new Promise(resolve => { finishCache = resolve })) } } })
    wrapper = mount(GlobalSearch, { props: { accounts: [account], providers: [] }, global: { stubs: { UiIcon: true } } })
    wrapper.vm.keyword = '文件'
    const running = wrapper.vm.run()
    await flushPromises()

    wrapper.vm.stop()
    finishCache([{ userId: account.user_id, file: { file_id: 'late', name: '迟到结果' } }])
    await running
    await flushPromises()
    expect(wrapper.vm.results).toEqual([])
    expect(document.body.textContent).toContain('搜索已停止，可重新搜索')
  })
})
