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
  it('分类跳转保持目标高亮，手动滚动后跟随实际位置', async () => {
    api.GetSettings.mockResolvedValue({})
    wrapper = mount(SettingsView, { global: { stubs: { UiIcon: true } } })
    await flushPromises()
    const body = wrapper.get('.settings-body')
    const tabs = wrapper.findAll('.sn-item')
    const current = () => tabs.findIndex(tab => tab.attributes('aria-current') === 'location')
    Object.defineProperties(body.element, { scrollHeight: { value: 2400 }, clientHeight: { value: 600 }, scrollTo: { value: vi.fn() } })
    wrapper.findAll('.settings-group').forEach((section, index) => Object.defineProperty(section.element, 'offsetTop', { value: 40 + index * 400 }))
    try {
      vi.useFakeTimers({ toFake: ['requestAnimationFrame', 'cancelAnimationFrame'] })
      await tabs[5].trigger('click')
      expect(body.element.scrollTo).toHaveBeenLastCalledWith({ top: 1800, behavior: 'smooth' })
      body.element.scrollTop = 540
      await body.trigger('scroll')
      vi.advanceTimersToNextFrame()
      expect(current()).toBe(5)
      await tabs[3].trigger('click')
      expect(current()).toBe(3)
      await body.trigger('wheel')
      expect(body.element.scrollTo).toHaveBeenLastCalledWith({ top: 540, behavior: 'instant' })
      vi.advanceTimersToNextFrame()
      await wrapper.vm.$nextTick()
      expect(current()).toBe(1)
      body.element.scrollTop = 1800
      await body.trigger('scroll')
      vi.advanceTimersToNextFrame()
      await wrapper.vm.$nextTick()
      expect(current()).toBe(5)
    } finally {
      vi.useRealTimers()
    }
  })

  it('设置开关、输入框与选择器均提供可访问名称', async () => {
    api.GetSettings.mockResolvedValue({})
    api.GetLogPath.mockResolvedValue('D:/logs/app.log')
    wrapper = mount(SettingsView, { global: { stubs: { UiIcon: true } } })
    await flushPromises()
    for (const control of wrapper.findAll('input, [role="switch"], [role="combobox"], [role="tablist"]')) {
      const label = control.attributes('aria-label')
      const labelledBy = control.attributes('aria-labelledby')
      expect(Boolean(label || labelledBy), control.html()).toBe(true)
      if (labelledBy) expect(wrapper.find(`[id="${labelledBy}"]`).exists()).toBe(true)
      if (['switch', 'combobox'].includes(control.attributes('role'))) expect(control.element.tagName).toBe('BUTTON')
    }
  })
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

  it('下载目录查询未返回时仍可保存其他设置，并忽略旧目录响应', async () => {
    api.GetSettings.mockResolvedValue({ downloadDir: 'D:/旧目录' })
    api.SaveSettings.mockResolvedValue(undefined)
    let finishOldDirectory
    api.GetDownloadDirectory.mockResolvedValueOnce('D:/旧目录')
      .mockImplementationOnce(() => new Promise(resolve => { finishOldDirectory = resolve }))
      .mockResolvedValueOnce('D:/新目录')
    wrapper = mount(SettingsView, { global: { stubs: { UiIcon: true } } })
    await flushPromises()
    const input = wrapper.get('[aria-label="下载保存目录"]')
    await input.setValue('D:/中间目录')
    await input.trigger('keydown.enter')
    await input.trigger('blur')
    await flushPromises()
    expect(api.SaveSettings).toHaveBeenCalledTimes(1)
    await input.setValue('D:/新目录')
    await input.trigger('blur')
    wrapper.vm.toggle('autoUpdate')
    await flushPromises()
    expect(api.SaveSettings).toHaveBeenLastCalledWith(expect.objectContaining({ downloadDir: 'D:/新目录', autoUpdate: wrapper.vm.settings.autoUpdate }))
    finishOldDirectory('D:/过期目录')
    await flushPromises()
    expect(input.element.value).toBe('D:/新目录')
    expect(input.attributes('placeholder')).toBe('D:/新目录')
  })

  it('下载目录选择器等待期间只打开一次，取消后可重新打开', async () => {
    let closePicker
    api.GetSettings.mockResolvedValue({})
    api.PickDirectory.mockImplementationOnce(() => new Promise(resolve => { closePicker = resolve })).mockResolvedValueOnce('')
    wrapper = mount(SettingsView, { global: { stubs: { UiIcon: true } } })
    await flushPromises()
    const picker = wrapper.findAll('button').find(button => button.text() === '选择')
    await picker.trigger('click')
    expect(picker.attributes('aria-busy')).toBe('true')
    await picker.trigger('click')
    expect(api.PickDirectory).toHaveBeenCalledOnce()
    closePicker('')
    await flushPromises()
    expect(picker.attributes('aria-busy')).toBe('false')
    await picker.trigger('click')
    expect(api.PickDirectory).toHaveBeenCalledTimes(2)
    expect(api.SaveSettings).not.toHaveBeenCalled()
  })

  it('目录保存失败时保留输入，并在行内显示仍生效的目录', async () => {
    api.GetSettings.mockResolvedValue({ downloadDir: 'D:/原目录' })
    api.GetDownloadDirectory.mockResolvedValueOnce('D:/原目录')
    api.SaveSettings.mockRejectedValueOnce(new Error('目录不可写'))
    wrapper = mount(SettingsView, { global: { stubs: { UiIcon: true } } })
    await flushPromises()
    const input = wrapper.get('[aria-label="下载保存目录"]')
    await input.setValue('D:/无效目录')
    await input.trigger('blur')
    await flushPromises()
    expect(input.element.value).toBe('D:/无效目录')
    expect(wrapper.get('[role="alert"]').text()).toContain('目录不可写。仍使用：D:/原目录')
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

  it('滑块拖动实时显示音量，只在确认时持久化', async () => {
    await prepare()
    const volume = wrapper.get('[aria-label="默认播放音量"]')
    for (const value of [120, 130, 140]) {
      volume.element.value = value
      await volume.trigger('input')
    }
    expect(volume.attributes('aria-valuetext')).toBe('140%')
    expect(localStorage.setItem).not.toHaveBeenCalled()
    await volume.trigger('change')
    await volume.trigger('blur')
    expect(localStorage.setItem).toHaveBeenCalledOnce()
    expect(JSON.parse(rawPrefs).defaultVolume).toBe(140)
  })

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
