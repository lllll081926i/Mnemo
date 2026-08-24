import { onBeforeUnmount, ref } from 'vue'
import { parseSup, SupRenderer } from '../player/subtitles'

let jassubPromise = null
async function getJassub() {
  if (!jassubPromise) {
    // JASSUB 及其 worker/WASM/字体必须继续按需加载。
    jassubPromise = Promise.all([
      import('jassub'),
      import('jassub/dist/wasm/jassub-worker.js?url'),
      import('jassub/dist/wasm/jassub-worker.wasm?url'),
      import('jassub/dist/default.woff2?url'),
    ]).then(([module, worker, wasm, font]) => ({
      JASSUB: module.default,
      workerUrl: worker.default,
      wasmUrl: wasm.default,
      fontUrl: font.default,
    }))
  }
  return jassubPromise
}

export function useSubtitleRenderers({ currentSubtitle, emit, selectTextSubtitle, supCanvasEl, videoEl }) {
  const supTracks = ref([])
  const supActive = ref(false)
  const assTracks = ref([])
  const assActive = ref(false)
  let disposed = false
  let supRenderer = null
  let assRenderer = null
  let fetchController = null

  function cancelFetch() {
    const controller = fetchController
    fetchController = null
    if (controller) try { controller.abort() } catch {}
  }

  function stopSup() {
    supActive.value = false
    if (supRenderer) supRenderer.stop()
  }

  function destroyAss() {
    assActive.value = false
    if (assRenderer) {
      try { assRenderer.destroy() } catch {}
      assRenderer = null
    }
  }

  function computeSubtitleBox(video, canvas) {
    const width = Math.round(video.clientWidth)
    const height = Math.round(video.clientHeight)
    if (!width || !height) return null
    if (canvas.width !== width || canvas.height !== height) { canvas.width = width; canvas.height = height }
    const videoWidth = video.videoWidth || 16
    const videoHeight = video.videoHeight || 9
    const scale = Math.min(width / videoWidth, height / videoHeight)
    const displayWidth = videoWidth * scale
    const displayHeight = videoHeight * scale
    const displayX = (width - displayWidth) / 2
    const displayY = (height - displayHeight) / 2
    let boxWidth = displayWidth
    let boxHeight = displayWidth * 9 / 16
    if (boxHeight > displayHeight) { boxHeight = displayHeight; boxWidth = displayHeight * 16 / 9 }
    return {
      x: displayX + (displayWidth - boxWidth) / 2,
      y: displayY + (displayHeight - boxHeight) / 2,
      w: boxWidth,
      h: boxHeight,
    }
  }

  function renderSupFrame() {
    const video = videoEl.value
    const canvas = supCanvasEl.value
    if (!video || !canvas || !supRenderer) return
    supRenderer.renderAt(video.currentTime, computeSubtitleBox(video, canvas))
  }

  async function selectSup(index) {
    const track = supTracks.value[index]
    if (!track) return
    cancelFetch()
    const controller = new AbortController()
    fetchController = controller
    selectTextSubtitle(-1)
    destroyAss()
    try {
      const response = await fetch(track.url, { signal: controller.signal })
      if (!response.ok) throw new Error(`HTTP ${response.status}`)
      const sets = parseSup(await response.arrayBuffer())
      if (disposed || controller.signal.aborted) return
      if (!sets.length) throw new Error('no display sets')
      if (!supRenderer) supRenderer = new SupRenderer(supCanvasEl.value)
      supRenderer.load(sets)
      supActive.value = true
      currentSubtitle.value = 'sup:' + index
      renderSupFrame()
    } catch (error) {
      if (disposed || error?.name === 'AbortError') return
      emit('toast', 'SUP 字幕解析失败', 'error')
    } finally {
      if (fetchController === controller) fetchController = null
    }
  }

  async function selectAss(index) {
    const track = assTracks.value[index]
    if (!track) return
    cancelFetch()
    const controller = new AbortController()
    fetchController = controller
    selectTextSubtitle(-1)
    stopSup()
    try {
      let content = track.content
      if (!content) {
        const response = await fetch(track.url, { signal: controller.signal })
        if (!response.ok) throw new Error(`HTTP ${response.status}`)
        content = await response.text()
      }
      if (disposed || controller.signal.aborted) return
      const { JASSUB, workerUrl, wasmUrl, fontUrl } = await getJassub()
      if (disposed || controller.signal.aborted) return
      destroyAss()
      assRenderer = new JASSUB({ video: videoEl.value, subContent: content, workerUrl, wasmUrl, fonts: [fontUrl] })
      assActive.value = true
      currentSubtitle.value = 'ass:' + index
    } catch (error) {
      if (disposed || error?.name === 'AbortError') return
      emit('toast', 'ASS 字幕加载失败', 'error')
    } finally {
      if (fetchController === controller) fetchController = null
    }
  }

  function reset() {
    cancelFetch()
    stopSup()
    destroyAss()
    supTracks.value = []
    assTracks.value = []
  }

  function onResize() {
    if (supActive.value) renderSupFrame()
  }

  onBeforeUnmount(() => {
    disposed = true
    reset()
  })

  return {
    assActive,
    assTracks,
    cancelFetch,
    destroyAss,
    onResize,
    renderSupFrame,
    reset,
    selectAss,
    selectSup,
    stopSup,
    supActive,
    supTracks,
  }
}
