import { DOMWrapper, flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import PlayerPanel from './PlayerPanel.vue'

const api = vi.hoisted(() => ({
  playVideo: vi.fn(), playVideoQuality: vi.fn(), previewUrl: vi.fn(),
  pinFileSnapshot: vi.fn(), getPlayCursor: vi.fn(), savePlayCursor: vi.fn(),
  getSettings: vi.fn(), download: vi.fn(), openKindOf: vi.fn(() => 'video'),
}))
vi.mock('../api', () => api)

const target = name => `http://127.0.0.1:45678/stream/${name}?t=test-token`
const preview = name => ({ url: target(name), subtitles: [{ url: target(name + '-subtitle') }], stream_type: 'mp4', qualities: [] })
let wrapper
let fetch
beforeEach(() => {
  vi.resetAllMocks()
  vi.spyOn(HTMLMediaElement.prototype, 'load').mockImplementation(() => {})
  vi.spyOn(HTMLMediaElement.prototype, 'pause').mockImplementation(() => {})
  api.getSettings.mockResolvedValue({ playbackResume: false })
  api.getPlayCursor.mockResolvedValue(0)
  api.savePlayCursor.mockResolvedValue(undefined)
  api.pinFileSnapshot.mockResolvedValue(undefined)
  api.playVideo.mockResolvedValue(preview('initial'))
  fetch = vi.fn().mockResolvedValue({ ok: true })
  vi.stubGlobal('fetch', fetch)
})
afterEach(() => {
  wrapper?.unmount()
  wrapper = null
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
  vi.useRealTimers()
})
function openPlayer(files = []) {
  wrapper = mount(PlayerPanel, {
    attachTo: document.body,
    props: { account: { user_id: 'account', drive_id: 'drive' }, file: { file_id: 'video', name: 'video.mp4' }, files },
  })
}
const released = () => fetch.mock.calls.filter(([, options]) => options?.method === 'DELETE').map(([url]) => url)

describe('播放器高频交互', () => {
  it('每帧仅测量一次进度条并使用最后的坐标，移出后不显示迟到的提示', async () => {
    openPlayer()
    await flushPromises()
    wrapper.vm.duration = 120
    await wrapper.vm.$nextTick()
    const progress = new DOMWrapper(document.body).get('.pp-progress')
    const measure = vi.spyOn(progress.element, 'getBoundingClientRect').mockReturnValue({ left: 100, width: 200 })
    const frames = new Map()
    let frameID = 0
    vi.stubGlobal('requestAnimationFrame', callback => { frames.set(++frameID, callback); return frameID })
    vi.stubGlobal('cancelAnimationFrame', id => frames.delete(id))
    await progress.trigger('pointermove', { clientX: 110 })
    await progress.trigger('pointermove', { clientX: 250 })
    expect(frames.size).toBe(1)
    expect(measure).not.toHaveBeenCalled()
    const callback = frames.get(frameID)
    frames.delete(frameID)
    callback()
    await wrapper.vm.$nextTick()
    expect(measure).toHaveBeenCalledTimes(1)
    expect(progress.get('.pp-scrub').text()).toBe('01:30')
    await progress.trigger('pointermove', { clientX: 220 })
    await progress.trigger('pointerleave')
    expect(frames.size).toBe(0)
    expect(progress.find('.pp-scrub').exists()).toBe(false)
    await progress.trigger('pointermove', { clientX: 200 })
    wrapper.unmount()
    wrapper = null
    expect(frames.size).toBe(0)
  })

  it('控制栏以最后一次交互为起点计时', async () => {
    openPlayer()
    await flushPromises()
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout', 'performance'] })
    const player = new DOMWrapper(document.body).get('.player-panel')
    await player.trigger('mousemove')
    vi.advanceTimersByTime(2000)
    await player.trigger('mousemove')
    vi.advanceTimersByTime(2599)
    expect(wrapper.vm.showControls).toBe(true)
    vi.advanceTimersByTime(1)
    expect(wrapper.vm.showControls).toBe(false)
  })

  it('拖动期间保留控制栏，取消拖动不改变实际播放位置', async () => {
    openPlayer()
    await flushPromises()
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout', 'performance'] })
    const player = new DOMWrapper(document.body).get('.player-panel')
    const video = player.get('video')
    wrapper.vm.duration = 120
    video.element.currentTime = 12
    await video.trigger('timeupdate')
    await player.trigger('mousemove')
    await video.trigger('play')
    const slider = player.get('.pp-progress-input')
    await slider.trigger('pointerdown')
    slider.element.value = '80'
    await slider.trigger('input')
    vi.advanceTimersByTime(3000)
    expect(wrapper.vm.showControls).toBe(true)
    expect(slider.attributes('aria-valuetext')).toBe('01:20 / 02:00')
    await slider.trigger('pointercancel')
    expect(video.element.currentTime).toBe(12)
    expect(slider.attributes('aria-valuetext')).toBe('00:12 / 02:00')
    vi.advanceTimersByTime(2600)
    expect(wrapper.vm.showControls).toBe(false)
    await video.trigger('pause')
    expect(wrapper.vm.showControls).toBe(true)
  })
})

describe('播放器会话释放', () => {
  it('清晰度乱序回执释放旧源和过时源，关闭释放最终主源与字幕', async () => {
    const pending = []
    api.playVideoQuality.mockImplementation(() => new Promise(resolve => pending.push(resolve)))
    openPlayer()
    await flushPromises()
    const first = wrapper.vm.switchQuality('720p')
    const last = wrapper.vm.switchQuality('1080p')
    pending[1](preview('last'))
    await last
    pending[0](preview('stale'))
    await first
    expect(wrapper.vm.src).toBe(target('last'))
    expect(released()).toEqual([target('initial'), target('initial-subtitle'), target('stale'), target('stale-subtitle')])
    wrapper.unmount()
    wrapper = null
    expect(released().slice(-2)).toEqual([target('last'), target('last-subtitle')])
  })

  it('关闭后迟到的首次播放回执立即释放', async () => {
    let complete
    api.playVideo.mockImplementation(() => new Promise(resolve => { complete = resolve }))
    openPlayer()
    await flushPromises()
    expect(complete).toBeTypeOf('function')
    wrapper.unmount()
    wrapper = null
    complete(preview('late'))
    await flushPromises()
    expect(released()).toEqual([target('late'), target('late-subtitle')])
  })

  it('关闭期间仍在获取的同名字幕回执不会泄漏会话', async () => {
    let complete
    api.previewUrl.mockImplementation(() => new Promise(resolve => { complete = resolve }))
    openPlayer([{ file_id: 'subtitle', name: 'video.zh.srt' }])
    await flushPromises()
    expect(complete).toBeTypeOf('function')
    wrapper.unmount()
    wrapper = null
    complete(target('late-sibling'))
    await flushPromises()
    expect(released()).toEqual([target('initial'), target('initial-subtitle'), target('late-sibling')])
  })
})
