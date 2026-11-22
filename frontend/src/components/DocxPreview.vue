<script setup>
import { nextTick, onBeforeUnmount, onMounted, ref } from 'vue'

const props = defineProps({ bytes: Object, file: Object })
const content = ref(null)
const stage = ref(null)
const loading = ref(true)
const error = ref('')
const pages = ref(0)
const compatibilityMode = ref(false)
const zoom = ref(100)
const fitToWidth = ref(true)
const pageDraft = ref('')
let pageElements = []
let disposed = false
let stageObserver = null
let fitFrame = null
let zoomRevision = 0
let anchor = null

function fitOnResize() {
  if (!fitToWidth.value || loading.value || fitFrame !== null) return
  fitFrame = requestAnimationFrame(() => {
    fitFrame = null
    if (fitToWidth.value && !loading.value) fitWidth()
  })
}

async function renderDocument() {
  loading.value = true
  error.value = ''
  compatibilityMode.value = false
  pages.value = 0
  pageElements = []
  content.value?.replaceChildren()
  try {
    const { renderAsync, defaultOptions } = await import('docx-preview')
    if (disposed || !content.value) return
    const options = {
      className: 'docx',
      inWrapper: true,
      breakPages: true,
      renderHeaders: true,
      renderFooters: true,
      renderFootnotes: true,
      renderEndnotes: true,
      renderAltChunks: true,
      useBase64URL: true,
      experimental: true,
      h: element => {
        const node = defaultOptions.h(element)
        if (node instanceof HTMLIFrameElement) node.setAttribute('sandbox', '')
        return node
      },
    }
    try {
      await renderAsync(props.bytes, content.value, content.value, options)
      if (!content.value.querySelector('section.docx')) throw new Error('未能解析出可显示的文档内容')
    } catch {
      if (disposed || !content.value) return
      content.value.replaceChildren()
      await renderAsync(props.bytes, content.value, content.value, { ...options, experimental: false, renderAltChunks: false })
      if (!content.value.querySelector('section.docx')) throw new Error('未能解析出可显示的文档内容')
      compatibilityMode.value = true
    }
    if (disposed) return
    pageElements = Array.from(content.value.querySelectorAll('section.docx'))
    pages.value = pageElements.length
    fitWidth(false)
    loading.value = false
  } catch (cause) {
    if (!disposed) { error.value = String(cause?.message || cause); loading.value = false }
  }
}

async function setZoom(value, preservePosition = true) {
  const viewport = stage.value
  if (!viewport || !pages.value || value === zoom.value) return
  const revision = ++zoomRevision
  if (preservePosition && !anchor) {
    const offset = viewport.scrollTop > 1 ? viewport.clientHeight / 2 : 0
    anchor = { documentY: (viewport.scrollTop + offset) / zoom.value, offset }
  }
  zoom.value = value
  await nextTick()
  if (disposed || revision !== zoomRevision) return
  if (anchor) viewport.scrollTop = anchor.documentY * value - anchor.offset
  anchor = null
}

function changeZoom(step) {
  const value = Math.max(25, Math.min(200, zoom.value + step))
  if (value === zoom.value) return
  fitToWidth.value = false
  void setZoom(value)
}

function fitWidth(preservePosition = true) {
  fitToWidth.value = true
  const pageWidth = pageElements[0]?.offsetWidth
  if (pageWidth && stage.value) void setZoom(Math.max(25, Math.min(200, Math.floor((stage.value.clientWidth - 48) / pageWidth * 100))), preservePosition)
}

function jumpToPage() {
  if (!stage.value || !pageDraft.value.trim() || !pages.value) return
  const requested = Number(pageDraft.value)
  if (!Number.isInteger(requested)) return
  const target = Math.min(pages.value, Math.max(1, requested))
  const top = pageElements[target - 1].getBoundingClientRect().top - stage.value.getBoundingClientRect().top
  stage.value.scrollTop += top - 12
  pageDraft.value = String(target)
}

onMounted(() => {
  if (stage.value && typeof ResizeObserver !== 'undefined') {
    stageObserver = new ResizeObserver(fitOnResize)
    stageObserver.observe(stage.value)
  } else window.addEventListener('resize', fitOnResize)
  void renderDocument()
})
onBeforeUnmount(() => {
  disposed = true
  stageObserver?.disconnect()
  window.removeEventListener('resize', fitOnResize)
  if (fitFrame !== null) cancelAnimationFrame(fitFrame)
  pageElements = []
  content.value?.replaceChildren()
})
</script>

<template>
  <div class="document-viewer">
    <div class="document-toolbar"><strong>Word 预览 <span v-if="pages">· {{ pages }} 页</span><span v-if="compatibilityMode"> · 兼容模式</span></strong>
      <div v-if="pages" class="document-controls">
        <form v-if="pages > 1" class="docx-jump" @submit.prevent="jumpToPage">
          <input v-model="pageDraft" inputmode="numeric" aria-label="跳转到 Word 页码" placeholder="页码" />
          <button class="btn sm" type="submit">跳转</button>
        </form>
        <button class="btn sm" type="button" aria-label="缩小 Word" :disabled="zoom <= 25" @click="changeZoom(-10)">−</button>
        <span>{{ zoom }}%</span>
        <button class="btn sm" type="button" aria-label="放大 Word" :disabled="zoom >= 200" @click="changeZoom(10)">＋</button>
        <button class="btn sm" type="button" :aria-pressed="fitToWidth" @click="fitWidth()">适合宽度</button>
      </div>
    </div>
    <Transition name="docx-loading">
      <div v-if="loading" class="document-state" role="status"><span class="spin"></span>正在排版文档…</div>
    </Transition>
    <div v-if="error" class="document-state" role="alert"><span>{{ error }}</span><button class="btn sm" type="button" @click="renderDocument">重试</button></div>
    <div ref="stage" class="docx-stage" role="region" aria-label="Word 文档内容" tabindex="0" :inert="loading || !!error"><div ref="content" class="docx-content" :style="{ zoom: zoom / 100 }"></div></div>
  </div>
</template>

<style scoped>
.document-viewer { position: relative; flex: 1; min-height: 0; display: flex; flex-direction: column; }
.document-toolbar { position: relative; z-index: 3; min-height: 48px; padding: 7px 16px; display: flex; align-items: center; justify-content: space-between; gap: 12px; border-bottom: 1px solid var(--border-light); background: var(--bg-surface); }
.document-toolbar strong { font-size: 12px; color: var(--text-secondary); }
.document-controls { display: flex; align-items: center; gap: 8px; font-size: 12px; color: var(--text-secondary); font-variant-numeric: tabular-nums; }
.docx-jump { display: flex; align-items: center; gap: 4px; }
.docx-jump input { width: 54px; padding: 4px 6px; border: 1px solid var(--border-light); border-radius: 6px; background: var(--bg-surface); color: var(--text-primary); }
.document-state { position: absolute; inset: 48px 0 0; z-index: 2; display: flex; align-items: center; justify-content: center; gap: 10px; background: var(--bg-base); color: var(--text-secondary); font-size: 13px; }
.docx-loading-leave-active { transition: opacity var(--motion-fast) var(--motion-ease); pointer-events: none; }
.docx-loading-leave-to { opacity: 0; }
.docx-stage { flex: 1; min-height: 0; overflow: auto; overflow-anchor: none; background: var(--bg-base); }
.docx-stage:focus-visible { outline: 2px solid var(--border-focus); outline-offset: -2px; }
.docx-content { width: max-content; min-width: 100%; }
.docx-content :deep(.docx-wrapper) { background: transparent; padding: 24px 0; }
.docx-content :deep(section.docx) { margin: 0 auto 24px; box-shadow: var(--shadow-md); }
@media (max-width: 600px) { .document-toolbar { flex-wrap: wrap; } .document-controls { flex-wrap: wrap; } }
@media (prefers-reduced-motion: reduce) { .docx-loading-leave-active { transition: none; } }
</style>
