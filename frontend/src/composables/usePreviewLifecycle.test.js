import { ref } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import { usePreviewShortcuts } from './usePreviewLifecycle'

function shortcuts(kind = 'image') {
  const calls = {
    switchImage: vi.fn(),
    zoomByFactor: vi.fn(),
    resetImageTransform: vi.fn(),
    rotateBy: vi.fn(),
    toggleAudioPlay: vi.fn(),
    audioSeekBy: vi.fn(),
    applyAudioVolume: vi.fn(),
    toggleAudioMute: vi.fn(),
    toggleSearch: vi.fn(),
  }
  const state = {
    textMode: ref('preview'),
    kind: ref(kind),
    audioVolume: ref(100),
    audioLoop: ref(false),
  }
  return {
    calls,
    state,
    onKey: usePreviewShortcuts({ ...calls, ...state }),
  }
}

describe('usePreviewShortcuts', () => {
  it('编辑模式不处理预览区快捷键', () => {
    const { calls, state, onKey } = shortcuts('image')
    state.textMode.value = 'edit'
    onKey({ key: 'ArrowRight' })
    expect(calls.switchImage).not.toHaveBeenCalled()
  })

  it('保持图片导航和旋转快捷键', () => {
    const { calls, onKey } = shortcuts('image')
    onKey({ key: 'ArrowLeft' })
    onKey({ key: 'R' })
    expect(calls.switchImage).toHaveBeenCalledWith(-1)
    expect(calls.rotateBy).toHaveBeenCalledWith(90)
  })

  it('保持音频音量边界和文本搜索快捷键', () => {
    const audio = shortcuts('audio')
    const preventDefault = vi.fn()
    audio.state.audioVolume.value = 198
    audio.onKey({ key: 'ArrowUp', preventDefault })
    expect(audio.state.audioVolume.value).toBe(200)
    expect(audio.calls.applyAudioVolume).toHaveBeenCalledTimes(1)
    expect(preventDefault).toHaveBeenCalledTimes(1)

    const text = shortcuts('text')
    text.onKey({ key: 'f', code: 'KeyF', ctrlKey: true, preventDefault })
    expect(text.calls.toggleSearch).toHaveBeenCalledTimes(1)
  })
})
