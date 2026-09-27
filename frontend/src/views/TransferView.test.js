import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'

const api = vi.hoisted(() => ({
  ListDownloads: vi.fn().mockResolvedValue([]),
  ListUploads: vi.fn().mockResolvedValue([]),
  ListMigrateJobs: vi.fn().mockResolvedValue([]),
  EventsOn: vi.fn(() => () => {}),
  capsOf: vi.fn(() => ({})),
  accountName: vi.fn(() => ''),
  providerIconUrl: vi.fn(() => ''),
  providerMetaOf: vi.fn(() => ({})),
  formatBytes: vi.fn(value => `${value} B`),
  formatSpeed: vi.fn(value => `${value} B/s`),
  formatTime: vi.fn(() => ''),
  iconOf: vi.fn(() => 'file'),
}))

vi.mock('../api', () => api)

import TransferView from './TransferView.vue'

let wrapper
afterEach(() => {
  wrapper?.unmount()
  wrapper = null
  vi.useRealTimers()
  vi.clearAllMocks()
})

describe('传输页刷新调度', () => {
  it('高频事件不会把已经安排的首次刷新不断往后推', async () => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout', 'Date'] })
    wrapper = mount(TransferView, { props: { accounts: [], providers: [] }, global: { stubs: { UiIcon: true } } })
    await flushPromises()
    api.ListDownloads.mockClear()

    wrapper.vm.scheduleRefresh(150)
    await vi.advanceTimersByTimeAsync(100)
    wrapper.vm.scheduleRefresh(150)
    await vi.advanceTimersByTimeAsync(50)
    await flushPromises()

    expect(api.ListDownloads).toHaveBeenCalledOnce()
  })

  it('文件夹只占用一条下载行，子任务在行内展开', async () => {
    api.ListDownloads.mockResolvedValueOnce([
      { id: 'folder', isDir: true, name: '文件夹', status: 'downloading', stage: 'scanning', totalFiles: 1, completedFiles: 0, localPath: 'folder' },
      { id: 'file', groupId: 'folder', name: 'file.txt', status: 'queued', localPath: 'folder/file.txt' },
    ])
    wrapper = mount(TransferView, { props: { accounts: [], providers: [] }, global: { stubs: { UiIcon: true } } })
    await flushPromises()
    expect(wrapper.findAll('.taskrow')).toHaveLength(1)
    expect(wrapper.text()).toContain('扫描目录')
    expect(wrapper.text()).toContain('查看文件（1/1）')
  })
})
