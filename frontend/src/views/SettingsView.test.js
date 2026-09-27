import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'

const api = vi.hoisted(() => ({
  GetSettings: vi.fn(),
  SaveSettings: vi.fn(),
  GetDownloadDirectory: vi.fn().mockResolvedValue('D:/下载'),
  OpenDownloadDirectory: vi.fn(),
  ClearCache: vi.fn(),
  PickDirectory: vi.fn(),
  RevealInFolder: vi.fn(),
  GetLogPath: vi.fn(),
  ClearLogs: vi.fn(),
  ExportLogs: vi.fn(),
  ListFavorites: vi.fn(),
  RestoreFavorite: vi.fn(),
  listAccounts: vi.fn(),
  setAccountCustomMeta: vi.fn(),
}))

vi.mock('../api', () => api)

import SettingsView from './SettingsView.vue'

let wrapper
afterEach(() => {
  wrapper?.unmount()
  wrapper = null
  vi.clearAllMocks()
})

describe('设置页加载', () => {
  it('日志路径读取未完成时先显示核心设置', async () => {
    let finishLogPath
    api.GetSettings.mockResolvedValue({ proxy: 'http://localhost:7890' })
    api.GetLogPath.mockImplementation(() => new Promise(resolve => { finishLogPath = resolve }))
    wrapper = mount(SettingsView, { global: { stubs: { UiIcon: true } } })
    await flushPromises()

    expect(wrapper.vm.loaded).toBe(true)
    expect(wrapper.vm.loadingSettings).toBe(false)
    expect(wrapper.vm.settings.proxy).toBe('http://localhost:7890')
    finishLogPath('D:/logs/app.log')
    await flushPromises()
    expect(wrapper.vm.logPath).toBe('D:/logs/app.log')
  })
})
