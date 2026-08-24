import { nextTick, onBeforeUnmount, onMounted, watch } from 'vue'

export function usePreviewShortcuts({
  textMode,
  kind,
  switchImage,
  zoomByFactor,
  resetImageTransform,
  rotateBy,
  toggleAudioPlay,
  audioSeekBy,
  audioVolume,
  applyAudioVolume,
  toggleAudioMute,
  audioLoop,
  toggleSearch,
}) {
  return function onKey(event) {
    if (textMode.value === 'edit') return
    if (kind.value === 'image') {
      if (event.key === 'ArrowLeft') switchImage(-1)
      else if (event.key === 'ArrowRight') switchImage(1)
      else if (event.key === '+' || event.key === '=') zoomByFactor(1.25)
      else if (event.key === '-' || event.key === '_') zoomByFactor(1 / 1.25)
      else if (event.key === '0') resetImageTransform()
      else if (event.key === 'r' || event.key === 'R') rotateBy(90)
    } else if (kind.value === 'audio') {
      if (event.code === 'Space') {
        event.preventDefault()
        toggleAudioPlay()
      } else if (event.key === 'ArrowLeft') audioSeekBy(-10)
      else if (event.key === 'ArrowRight') audioSeekBy(10)
      else if (event.key === 'ArrowUp') {
        event.preventDefault()
        audioVolume.value = Math.min(200, audioVolume.value + 5)
        applyAudioVolume()
      } else if (event.key === 'ArrowDown') {
        event.preventDefault()
        audioVolume.value = Math.max(0, audioVolume.value - 5)
        applyAudioVolume()
      } else if (event.key === 'm' || event.key === 'M') toggleAudioMute()
      else if (event.key === 'l' || event.key === 'L') audioLoop.value = !audioLoop.value
    } else if (kind.value === 'text') {
      if ((event.ctrlKey || event.metaKey) && (event.key === 'f' || event.code === 'KeyF')) {
        event.preventDefault()
        toggleSearch()
      }
    }
  }
}

export function usePreviewLifecycle({
  loadPreview,
  pokeUI,
  syncWindowState,
  stageEl,
  computeFit,
  onKey,
  cleanup,
}) {
  let stageResizeObserver = null

  onMounted(() => {
    loadPreview()
    pokeUI()
    syncWindowState()
    window.addEventListener('keydown', onKey)
    if (typeof ResizeObserver !== 'undefined') {
      stageResizeObserver = new ResizeObserver(() => computeFit())
    }
  })

  watch(stageEl, (element) => {
    stageResizeObserver?.disconnect()
    if (element && stageResizeObserver) {
      stageResizeObserver.observe(element)
      nextTick(computeFit)
    }
  })

  onBeforeUnmount(() => {
    window.removeEventListener('keydown', onKey)
    stageResizeObserver?.disconnect()
    cleanup()
  })
}
