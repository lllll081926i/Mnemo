import { defineComponent, ref } from 'vue'
import { mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { extensionOf, normalizeStreamType, normalizeSubtitles } from './usePlaybackEngine'

const { savePlayCursor } = vi.hoisted(() => ({ savePlayCursor: vi.fn(() => Promise.resolve()) }))
vi.mock('../api', () => ({ savePlayCursor }))

import { usePlaybackCursor } from './usePlaybackCursor'

afterEach(() => {
  vi.clearAllMocks()
  vi.useRealTimers()
})

describe('播放器 composables', () => {
  it('从服务声明或文件扩展名稳定识别播放引擎', () => {
    expect(normalizeStreamType({ stream_type: 'm3u8' }, 'video.bin')).toBe('hls')
    expect(normalizeStreamType({ stream_type: 'MPD' }, 'video.bin')).toBe('dash')
    expect(normalizeStreamType({}, 'movie.MOV')).toBe('mp4')
    expect(normalizeStreamType({}, 'movie.webm')).toBe('webm')
    expect(extensionOf('archive')).toBe('')
  })

  it('清理无效字幕地址并保留原有默认标签语义', () => {
    expect(normalizeSubtitles([
      { url: ' /subtitle/zh.vtt ', language: 'zh-CN' },
      { url: '', language: 'en' },
      { url: '/subtitle/default.vtt' },
    ])).toEqual([
      { url: '/subtitle/zh.vtt', language: 'zh-CN', label: 'zh-CN' },
      { url: '/subtitle/default.vtt', language: 'und', label: '字幕 3' },
    ])
  })

  it('定时保存进度，播放结束后只清零且卸载时停止定时器', async () => {
    vi.useFakeTimers()
    const videoEl = ref({ currentTime: 42 })
    let cursor
    const wrapper = mount(defineComponent({
      setup() {
        cursor = usePlaybackCursor({
          account: () => ({ user_id: 'u1', drive_id: 'd1' }),
          currentFileId: () => 'file-1',
          videoEl,
        })
        return () => null
      },
    }))

    cursor.start()
    await vi.advanceTimersByTimeAsync(5000)
    expect(savePlayCursor).toHaveBeenLastCalledWith('u1', 'd1', 'file-1', 42)

    cursor.markEnded()
    cursor.save()
    expect(savePlayCursor).toHaveBeenLastCalledWith('u1', 'd1', 'file-1', 0)

    wrapper.unmount()
    const callsAfterUnmount = savePlayCursor.mock.calls.length
    await vi.advanceTimersByTimeAsync(10000)
    expect(savePlayCursor).toHaveBeenCalledTimes(callsAfterUnmount)
  })
})
