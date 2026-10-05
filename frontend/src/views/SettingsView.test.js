import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

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
import UiSelect from '../components/UiSelect.vue'
import { DOWNLOAD_SOUNDS } from '../download-sounds'

let wrapper
afterEach(() => {
  wrapper?.unmount()
  wrapper = null
  vi.clearAllMocks()
})

describe('设置页加载', () => {
  it('下载文件数与单文件连接数独立，并保存连接数上限', async () => {
    api.GetSettings.mockResolvedValue({ maxConcurrentDownloads: 1 })
    api.GetLogPath.mockResolvedValue('D:/logs/app.log')
    api.SaveSettings.mockResolvedValue(undefined)
    wrapper = mount(SettingsView, { global: { stubs: { UiIcon: true } } })
    await flushPromises()
    expect(wrapper.vm.settings.maxDownloadConnections).toBe(8)
    const row = wrapper.findAll('.sg-row').find(row => row.text().includes('单文件下载连接数'))
    await row.get('input').setValue(99)
    await row.get('input').trigger('blur')
    await flushPromises()
    expect(api.SaveSettings).toHaveBeenLastCalledWith(expect.objectContaining({ maxConcurrentDownloads: 1, maxDownloadConnections: 16 }))
  })
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

describe('下载完成提示音设置', () => {
  let rawPrefs
  beforeEach(() => {
    rawPrefs = '{}'
    vi.stubGlobal('localStorage', {
      getItem: vi.fn(() => rawPrefs),
      setItem: vi.fn((key, value) => { rawPrefs = value }),
    })
    vi.stubGlobal('Audio', vi.fn(function (url) {
      this.src = url
      this.currentTime = 0
      this.play = vi.fn().mockResolvedValue(undefined)
      this.pause = vi.fn()
    }))
    api.GetSettings.mockResolvedValue({})
    api.GetLogPath.mockResolvedValue('D:/logs/app.log')
  })
  afterEach(() => vi.unstubAllGlobals())

  async function prepare() {
    wrapper = mount(SettingsView, { global: { stubs: { UiIcon: true } } })
    await flushPromises()
    return wrapper.findAll('.sg-row').find(row => row.text().includes('下载完成提示音'))
  }

  it('仅提供两项切换，保存选择，试听播放当前音效', async () => {
    const row = await prepare()
    const select = row.getComponent(UiSelect)
    expect(select.props('options').map(option => option.label)).toEqual(['提示音 1', '提示音 2'])
    expect(select.props('modelValue')).toBe('sound-1')
    await row.get('.tbtn').trigger('click')
    const first = Audio.mock.instances[0]
    expect(Audio).toHaveBeenLastCalledWith(DOWNLOAD_SOUNDS[0].url)
    select.vm.$emit('update:modelValue', 'sound-2')
    await flushPromises()
    expect(first.pause).toHaveBeenCalledOnce()
    expect(JSON.parse(rawPrefs).downloadSoundType).toBe('sound-2')
    await row.get('.tbtn').trigger('click')
    expect(Audio).toHaveBeenLastCalledWith(DOWNLOAD_SOUNDS[1].url)
    expect(wrapper.emitted('toast')).toContainEqual(['正在试听提示音', 'success'])
    const second = Audio.mock.instances[1]
    await row.get('[role="switch"]').trigger('click')
    expect(second.pause).toHaveBeenCalledOnce()
    expect(row.find('.tbtn').exists()).toBe(false)
    expect(JSON.parse(rawPrefs)).toMatchObject({ downloadSound: false, downloadSoundType: 'sound-2' })
  })

  it('重新打开保留选择，卸载时停止试听', async () => {
    rawPrefs = JSON.stringify({ downloadSoundType: 'sound-2' })
    const row = await prepare()
    expect(row.getComponent(UiSelect).props('modelValue')).toBe('sound-2')
    await row.get('.tbtn').trigger('click')
    const audio = Audio.mock.instances[0]
    wrapper.unmount()
    wrapper = null
    expect(audio.pause).toHaveBeenCalledOnce()
  })
})
