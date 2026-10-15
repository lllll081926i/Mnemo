import { flushPromises, mount } from '@vue/test-utils'
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
})
function openPlayer(files = []) {
  wrapper = mount(PlayerPanel, {
    attachTo: document.body,
    props: { account: { user_id: 'account', drive_id: 'drive' }, file: { file_id: 'video', name: 'video.mp4' }, files },
  })
}
const released = () => fetch.mock.calls.filter(([, options]) => options?.method === 'DELETE').map(([url]) => url)

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
