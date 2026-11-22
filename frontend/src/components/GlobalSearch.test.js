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
  it('在线结果不截断，键盘可以直接跳到未渲染的末条结果', async () => {
    vi.stubGlobal('go', { app: { App: { SearchCachedFiles: vi.fn().mockResolvedValue([]) } } })
    api.search.mockResolvedValue(Array.from({ length: 601 }, (_, index) => ({ file_id: `file-${index}`, name: `文件-${index}` })))
    wrapper = mount(GlobalSearch, { props: { accounts: [account], providers: [] }, global: { stubs: { UiIcon: true } } })
    wrapper.vm.keyword = '文件'
    wrapper.vm.remote = true
    await wrapper.vm.run()
    await flushPromises()
    expect(wrapper.vm.results).toHaveLength(601)
    expect(document.body.querySelectorAll('.workspace-result').length).toBeLessThan(20)
    expect(document.body.textContent).toContain('601 项结果')
    document.body.querySelector('.workspace-result').dispatchEvent(new KeyboardEvent('keydown', { key: 'End', bubbles: true, cancelable: true }))
    await flushPromises()
    expect(document.activeElement.getAttribute('aria-posinset')).toBe('601')
    expect(document.body.querySelectorAll('.workspace-result').length).toBeLessThan(20)
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

  it('同一空间的重复结果只显示一次，旧空间的结果不可打开', async () => {
    const file = { file_id: 'same', name: '当前空间文件', parent_file_id: 'root' }
    vi.stubGlobal('go', { app: { App: { SearchCachedFiles: vi.fn().mockResolvedValue([
      { userId: account.user_id, driveId: 'old-drive', file: { ...file, name: '旧空间文件' } },
      { userId: account.user_id, driveId: account.drive_id, file },
    ]) } } })
    api.search.mockResolvedValue([file, file])
    wrapper = mount(GlobalSearch, { props: { accounts: [account], providers: [] }, global: { stubs: { UiIcon: true } } })
    wrapper.vm.keyword = '文件'
    wrapper.vm.remote = true
    await wrapper.vm.run()
    await flushPromises()
    const matches = document.body.querySelectorAll('.workspace-result')
    expect(matches).toHaveLength(1)
    expect(matches[0].textContent).toContain('当前空间文件')
    matches[0].click()
    expect(wrapper.emitted('navigate')[0][0]).toMatchObject({ userId: account.user_id, driveId: account.drive_id, dirId: 'root' })
    await wrapper.setProps({ accounts: [{ ...account, drive_id: 'new-drive' }] })
    expect(document.body.querySelectorAll('.workspace-result')).toHaveLength(0)
  })

  it('可直接提交新关键词，取消旧在线请求并忽略迟到结果', async () => {
    let finishOldOnline
    const cancelSearch = vi.fn().mockResolvedValue(undefined)
    const cache = vi.fn().mockResolvedValue([])
    vi.stubGlobal('go', { app: { App: { SearchCachedFiles: cache, CancelSearch: cancelSearch } } })
    api.search.mockImplementationOnce(() => new Promise(resolve => { finishOldOnline = resolve }))
      .mockResolvedValueOnce([{ file_id: 'new', name: '新结果' }])
    wrapper = mount(GlobalSearch, { props: { accounts: [account], providers: [] }, global: { stubs: { UiIcon: true } } })
    const form = document.body.querySelector('.global-search-form')
    const submit = () => form.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    wrapper.vm.keyword = '旧关键词'
    wrapper.vm.remote = true
    submit()
    submit()
    await flushPromises()
    expect(cache).toHaveBeenCalledOnce()
    expect(api.search).toHaveBeenCalledOnce()
    expect(form.querySelector('button').disabled).toBe(true)
    wrapper.vm.keyword = '新关键词'
    await wrapper.vm.$nextTick()
    expect(form.querySelector('button').disabled).toBe(false)
    submit()
    await flushPromises()
    expect(cancelSearch).toHaveBeenCalledExactlyOnceWith(api.search.mock.calls[0][3])
    expect(cache).toHaveBeenLastCalledWith('新关键词')
    finishOldOnline([{ file_id: 'old', name: '迟到结果' }])
    await flushPromises()
    expect(wrapper.vm.results.map(result => result.file.name)).toEqual(['新结果'])
    expect(wrapper.vm.busy).toBe(false)
    wrapper.vm.keyword = '未提交关键词'
    await wrapper.vm.$nextTick()
    expect(document.body.textContent).toContain('当前显示「新关键词」的缓存和在线结果')
  })

  it('移除正在搜索的网盘后立即让出名额，忽略迟到的结果与错误', async () => {
    const accounts = [account, { ...account, drive_id: 'second' }, { ...account, drive_id: 'waiting' }]
    let finishRemoved
    let finishSecond
    const cancelSearch = vi.fn().mockResolvedValue(undefined)
    vi.stubGlobal('go', { app: { App: { SearchCachedFiles: vi.fn().mockResolvedValue([]), CancelSearch: cancelSearch } } })
    api.search.mockImplementation((_user, drive) => {
      if (drive === account.drive_id) return new Promise((_resolve, reject) => { finishRemoved = reject })
      if (drive === 'second') return new Promise(resolve => { finishSecond = resolve })
      return Promise.resolve([{ file_id: 'waiting-file', name: '等待查询的文件' }])
    })
    wrapper = mount(GlobalSearch, { props: { accounts, providers: [] }, global: { stubs: { UiIcon: true } } })
    wrapper.vm.keyword = '文件'
    wrapper.vm.remote = true
    const running = wrapper.vm.run()
    await flushPromises()
    expect(api.search).toHaveBeenCalledTimes(2)
    await wrapper.setProps({ accounts: accounts.slice(1) })
    await flushPromises()
    expect(cancelSearch).toHaveBeenCalledExactlyOnceWith(api.search.mock.calls[0][3])
    expect(api.search.mock.calls.map(([, drive]) => drive)).toEqual(['drive', 'second', 'waiting'])
    expect(wrapper.vm.results.map(result => result.file.file_id)).toEqual(['waiting-file'])
    finishSecond([{ file_id: 'second-file', name: '第二个文件' }])
    await running
    expect(wrapper.vm.busy).toBe(false)
    finishRemoved(new Error('网盘已移除'))
    await flushPromises()
    expect(wrapper.vm.errors).toEqual([])
    expect(wrapper.vm.accountStates.map(state => state.driveId)).toEqual(['second', 'waiting'])
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

  it('结果支持键盘导航：输入框 ↓ 进入列表，↑↓/Home/End 移动，↑ 越界回输入框', async () => {
    vi.stubGlobal('go', { app: { App: { SearchCachedFiles: vi.fn().mockResolvedValue([
      { userId: account.user_id, driveId: account.drive_id, file: { file_id: 'a', name: '甲文件' } },
      { userId: account.user_id, driveId: account.drive_id, file: { file_id: 'b', name: '乙文件' } },
    ]) } } })
    wrapper = mount(GlobalSearch, { attachTo: document.body, props: { accounts: [account], providers: [], initialQuery: '文件' }, global: { stubs: { UiIcon: true } } })
    await flushPromises()
    const input = document.body.querySelector('.global-search-form .input')
    const results = () => Array.from(document.body.querySelectorAll('.global-search-results .workspace-result'))
    expect(results()).toHaveLength(2)

    input.focus()
    input.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown', bubbles: true, cancelable: true }))
    await flushPromises()
    expect(document.activeElement).toBe(results()[0])

    const press = (el, key) => el.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true }))
    press(results()[0], 'ArrowDown')
    await flushPromises()
    expect(document.activeElement).toBe(results()[1])

    press(results()[1], 'End')
    await flushPromises()
    expect(document.activeElement).toBe(results()[1])

    press(results()[1], 'Home')
    await flushPromises()
    expect(document.activeElement).toBe(results()[0])

    press(results()[0], 'ArrowUp')
    await flushPromises()
    expect(document.activeElement).toBe(input)
  })
})
