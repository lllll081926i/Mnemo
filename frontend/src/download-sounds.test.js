import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { DOWNLOAD_SOUNDS, createDownloadSoundPlayer, downloadSoundOf } from './download-sounds'
import { getPrefs, setPref } from './appearance'

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
})
afterEach(() => vi.unstubAllGlobals())

describe('下载完成音效', () => {
  it('只提供两个新音效，无效选择回退到第一个', () => {
    expect(DOWNLOAD_SOUNDS.map(sound => sound.value)).toEqual(['sound-1', 'sound-2'])
    expect(DOWNLOAD_SOUNDS.map(sound => sound.url)).toEqual([
      expect.stringContaining('download-complete-1.mp3'),
      expect.stringContaining('download-complete-2.mp3'),
    ])
    expect(downloadSoundOf('invalid')).toBe(DOWNLOAD_SOUNDS[0])
  })

  it('切换音效停止旧音频，重复播放同一音效从头开始', async () => {
    const player = createDownloadSoundPlayer()
    expect(Audio).not.toHaveBeenCalled()
    expect(await player.play('sound-1')).toBe(true)
    const first = Audio.mock.instances[0]
    first.currentTime = 3
    await player.play('sound-1')
    expect(Audio).toHaveBeenCalledOnce()
    expect(first.currentTime).toBe(0)
    expect(first.play).toHaveBeenCalledTimes(2)
    await player.play('sound-2')
    expect(first.pause).toHaveBeenCalledOnce()
    expect(Audio).toHaveBeenLastCalledWith(DOWNLOAD_SOUNDS[1].url)
    player.stop()
    expect(Audio.mock.instances[1].pause).toHaveBeenCalledOnce()
  })

  it('浏览器拒绝播放时不会产生未处理异常，之后可以重试', async () => {
    const player = createDownloadSoundPlayer()
    await player.play('sound-1')
    Audio.mock.instances[0].play.mockRejectedValueOnce(new Error('播放被阻止'))
    expect(await player.play('sound-1')).toBe(false)
    expect(await player.play('sound-1')).toBe(true)
  })

  it('旧偏好默认第一段音效，新选择持久化，无效值安全回退', () => {
    rawPrefs = JSON.stringify({ downloadSound: false })
    expect(getPrefs()).toMatchObject({ downloadSound: false, downloadSoundType: 'sound-1' })
    setPref('downloadSoundType', 'sound-2')
    expect(JSON.parse(rawPrefs).downloadSoundType).toBe('sound-2')
    expect(getPrefs().downloadSoundType).toBe('sound-2')
    rawPrefs = JSON.stringify({ downloadSoundType: 'download_finished.mp3' })
    expect(getPrefs().downloadSoundType).toBe('sound-1')
  })
})
