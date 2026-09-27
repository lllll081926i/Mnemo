import { afterEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { useSeekDisplay } from './seek-display'

afterEach(() => vi.useRealTimers())

describe('播放进度拖动', () => {
  it('拖动和定位等待期间不被旧的播放时间拉回', () => {
    const position = ref(10)
    const seek = useSeekDisplay(position)
    seek.showPreview(70)
    seek.sync(11)
    expect(seek.display.value).toBe(70)
    seek.commit(70)
    seek.sync(12)
    expect(seek.display.value).toBe(70)
    seek.sync(69.8)
    expect(seek.display.value).toBe(69.8)
  })

  it('定位失败或超时后恢复真实位置', () => {
    vi.useFakeTimers()
    const position = ref(5)
    const seek = useSeekDisplay(position)
    seek.commit(80)
    seek.sync(6)
    expect(seek.display.value).toBe(80)
    vi.advanceTimersByTime(15000)
    expect(seek.display.value).toBe(6)
    seek.commit(30)
    seek.reset()
    expect(seek.display.value).toBe(6)
  })
})
