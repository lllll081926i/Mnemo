<script setup>
import { onBeforeUnmount, onMounted, ref } from 'vue'

const props = defineProps({ bytes: Object, file: Object })
const stage = ref(null)
const host = ref(null)
const loading = ref(true)
const error = ref('')
const slideCount = ref(0)
const currentSlide = ref(0)
const zoom = ref(100)
const slideDraft = ref('')
const controller = new AbortController()
let viewer = null
let disposed = false
let zoomFrame = null
let zoomRunning = false
let anchor = null
let pendingSlide = null

function capturePosition() {
  const viewport = stage.value
  const first = host.value?.firstElementChild
  const second = first?.nextElementSibling
  if (!viewport || !first || viewport.scrollTop <= 1) return null
  const origin = first.getBoundingClientRect().top
  const center = viewport.getBoundingClientRect().top + viewport.clientHeight / 2
  const pitch = second ? second.getBoundingClientRect().top - origin : first.offsetHeight
  if (pitch <= 0) return null
  const index = Math.max(0, Math.min(slideCount.value - 1, Math.floor((center - origin) / pitch)))
  return { index, progress: (center - origin - index * pitch) / pitch }
}

function restorePosition() {
  const position = anchor
  anchor = null
  if (disposed || !stage.value) return
  if (pendingSlide !== null) {
    const index = pendingSlide
    pendingSlide = null
    scrollToSlide(index)
    return
  }
  if (!position) return
  const item = host.value?.children[position.index]
  if (!item) return
  const top = item.getBoundingClientRect().top
  const next = item.nextElementSibling
  const previous = item.previousElementSibling
  const pitch = next ? next.getBoundingClientRect().top - top : previous ? top - previous.getBoundingClientRect().top : item.offsetHeight
  stage.value.scrollTop += top + pitch * position.progress - stage.value.getBoundingClientRect().top - stage.value.clientHeight / 2
}

async function openPresentation() {
  try {
    const { PptxViewer, RECOMMENDED_ZIP_LIMITS } = await import('@aiden0z/pptx-renderer')
    if (disposed || !host.value) return
    viewer = new PptxViewer(host.value, {
      fitMode: 'contain',
      zoomPercent: zoom.value,
      zipLimits: RECOMMENDED_ZIP_LIMITS,
      lazyMedia: true,
      lazySlides: true,
      scrollContainer: stage.value,
      pdfjs: false,
      onSlideChange: index => { currentSlide.value = index },
      onRenderStart: () => { anchor = capturePosition() },
      onRenderComplete: restorePosition,
    })
    await viewer.open(props.bytes, { renderMode: 'list', listOptions: { windowed: true, initialSlides: 2, overscanViewport: 1.5 }, signal: controller.signal, lazyMedia: true, lazySlides: true })
    if (disposed) return
    slideCount.value = viewer.slideCount
    currentSlide.value = viewer.currentSlideIndex
    if (!slideCount.value) throw new Error('文件中没有可显示的幻灯片')
  } catch (cause) {
    if (!disposed) error.value = String(cause?.message || cause)
  } finally {
    if (!disposed) loading.value = false
  }
}

function scheduleZoom() {
  if (disposed || error.value || zoomRunning || zoomFrame !== null) return
  zoomFrame = requestAnimationFrame(applyZoom)
}

async function applyZoom() {
  zoomFrame = null
  if (disposed || !viewer || viewer.zoomPercent === zoom.value) return
  zoomRunning = true
  try {
    await viewer.setZoom(zoom.value)
  } catch (cause) {
    if (!disposed) error.value = String(cause?.message || cause)
  } finally {
    zoomRunning = false
    if (!disposed && viewer.zoomPercent !== zoom.value) scheduleZoom()
  }
}

function changeZoom(value) {
  const next = Math.max(50, Math.min(200, value))
  if (zoom.value === next) return
  zoom.value = next
  scheduleZoom()
}

function scrollToSlide(index) {
  const item = host.value?.children[index]
  if (!item || !stage.value) return
  const offset = item.getBoundingClientRect().top - stage.value.getBoundingClientRect().top
  stage.value.scrollTop += offset - 16
  currentSlide.value = index
}

function jumpToSlide() {
  const number = Number(slideDraft.value)
  if (!slideDraft.value.trim() || !Number.isInteger(number)) return
  const index = Math.max(0, Math.min(slideCount.value - 1, number - 1))
  slideDraft.value = String(index + 1)
  if (viewer?.isRendering) pendingSlide = index
  else scrollToSlide(index)
}

onMounted(openPresentation)
onBeforeUnmount(() => {
  disposed = true
  controller.abort()
  if (zoomFrame !== null) cancelAnimationFrame(zoomFrame)
  viewer?.destroy()
})
</script>

<template>
  <div class="pptx-viewer">
    <div class="pptx-toolbar"><strong>演示文稿预览</strong>
      <div v-if="slideCount && !error" class="pptx-controls">
        <form v-if="slideCount > 1" class="pptx-jump" @submit.prevent="jumpToSlide">
          <input v-model="slideDraft" inputmode="numeric" aria-label="跳转到幻灯片页码" placeholder="页码" />
          <button class="btn sm" type="submit">跳转</button>
        </form>
        <span>{{ currentSlide + 1 }} / {{ slideCount }}</span>
        <span class="pptx-divider"></span>
        <button class="btn sm" type="button" aria-label="缩小幻灯片" :disabled="zoom <= 50" @click="changeZoom(zoom - 10)">−</button>
        <span>{{ zoom }}%</span>
        <button class="btn sm" type="button" aria-label="放大幻灯片" :disabled="zoom >= 200" @click="changeZoom(zoom + 10)">＋</button>
        <button class="btn sm" type="button" :aria-pressed="zoom === 100" @click="changeZoom(100)">适合宽度</button>
      </div>
    </div>
    <Transition name="pptx-loading">
      <div v-if="loading" class="pptx-state" role="status"><span class="spin"></span>正在解析演示文稿…</div>
    </Transition>
    <div v-if="error" class="pptx-state" role="alert">{{ error }}</div>
    <div ref="stage" class="pptx-stage" role="region" aria-label="幻灯片内容" tabindex="0" :inert="loading || !!error"><div ref="host" class="pptx-host"></div></div>
  </div>
</template>

<style scoped>
.pptx-viewer { position: relative; flex: 1; min-height: 0; display: flex; flex-direction: column; }
.pptx-toolbar { position: relative; z-index: 3; min-height: 48px; padding: 7px 16px; display: flex; align-items: center; justify-content: space-between; gap: 12px; border-bottom: 1px solid var(--border-light); background: var(--bg-surface); }
.pptx-toolbar strong { font-size: 12px; color: var(--text-secondary); white-space: nowrap; }
.pptx-controls { display: flex; align-items: center; gap: 8px; font-size: 12px; color: var(--text-secondary); font-variant-numeric: tabular-nums; }
.pptx-jump { display: flex; align-items: center; gap: 4px; }
.pptx-jump input { width: 54px; padding: 4px 6px; border: 1px solid var(--border-light); border-radius: 6px; background: var(--bg-surface); color: var(--text-primary); }
.pptx-divider { width: 1px; height: 20px; background: var(--border-light); }
.pptx-state { position: absolute; z-index: 2; inset: 48px 0 0; display: flex; align-items: center; justify-content: center; gap: 10px; color: var(--text-secondary); background: var(--bg-base); font-size: 13px; }
.pptx-loading-leave-active { transition: opacity var(--motion-fast) var(--motion-ease); pointer-events: none; }
.pptx-loading-leave-to { opacity: 0; }
.pptx-stage { flex: 1; min-height: 0; overflow: auto; overflow-anchor: none; scrollbar-gutter: stable; padding: 24px; background: var(--bg-base); }
.pptx-stage:focus-visible { outline: 2px solid var(--border-focus); outline-offset: -2px; }
.pptx-host { width: 100%; margin: 0 auto; }
@media (max-width: 600px) { .pptx-toolbar, .pptx-controls { flex-wrap: wrap; } }
@media (prefers-reduced-motion: reduce) { .pptx-loading-leave-active { transition: none; } }
</style>
