<script setup>
import { onBeforeUnmount, onMounted, ref } from 'vue'
import pdfWorkerURL from 'pdfjs-dist/build/pdf.worker.min.mjs?url'
import 'pdfjs-dist/web/pdf_viewer.css'

const props = defineProps({ url: String, file: Object })
const stage = ref(null)
const content = ref(null)
const loading = ref(true)
const error = ref('')
const pages = ref(0)
const pageNumber = ref(1)
const pageInput = ref('')
const editingPage = ref(false)
const zoom = ref(1)
const fitWidth = ref(true)
const passwordRequired = ref(false)
const password = ref('')
let passwordUpdater = null
let loadingTask = null
let viewer = null
let viewerController = null
let openSequence = 0
let disposed = false
let stageObserver = null
let fitFrame = null
let fittedWidth = null

function cancelFit() {
  if (fitFrame !== null) cancelAnimationFrame(fitFrame)
  fitFrame = null
}

function applyFit() {
  const width = stage.value?.clientWidth
  if (!fitWidth.value || !pages.value || !viewer || width === undefined || fittedWidth === width) return
  fittedWidth = width
  viewer.currentScaleValue = 'page-width'
}

function fitOnResize() {
  if (!fitWidth.value || !pages.value || !viewer || fitFrame !== null) return
  fitFrame = requestAnimationFrame(() => {
    fitFrame = null
    applyFit()
  })
}

function fitPageWidth() {
  cancelFit()
  fitWidth.value = true
  fittedWidth = null
  applyFit()
}

function releasePDF() {
  cancelFit()
  fittedWidth = null
  viewer?.setDocument(null)
  viewerController?.abort()
  viewer = null
  passwordUpdater = null
  const previousTask = loadingTask
  loadingTask = null
  return previousTask?.destroy().catch(() => {})
}

async function openPDF() {
  const sequence = ++openSequence
  loading.value = true
  error.value = ''
  pages.value = 0
  pageNumber.value = 1
  passwordRequired.value = false
  try {
    await releasePDF()
    const { getDocument, GlobalWorkerOptions, PasswordResponses } = await import('pdfjs-dist')
    const { EventBus, PDFViewer, ScrollMode } = await import('pdfjs-dist/web/pdf_viewer.mjs')
    if (disposed || sequence !== openSequence) return
    GlobalWorkerOptions.workerSrc = pdfWorkerURL
    viewerController = new AbortController()
    const eventBus = new EventBus()
    viewer = new PDFViewer({ container: stage.value, viewer: content.value, eventBus,
      abortSignal: viewerController.signal, maxCanvasPixels: 32_000_000,
      annotationMode: 0, enableAutoLinking: false })
    const activeViewer = viewer
    const isCurrent = () => !disposed && sequence === openSequence
    eventBus.on('pagesinit', () => {
      if (!isCurrent()) return
      activeViewer.scrollMode = ScrollMode.VERTICAL
      if (fitWidth.value) applyFit()
      else activeViewer.currentScaleValue = zoom.value
      loading.value = false
    })
    eventBus.on('pagechanging', event => { if (isCurrent()) pageNumber.value = event.pageNumber })
    eventBus.on('scalechanging', event => { if (isCurrent()) zoom.value = event.scale })
    eventBus.on('pagerendered', event => {
      if (isCurrent() && event.error && event.error.name !== 'RenderingCancelledException') {
        error.value = String(event.error.message || event.error)
        loading.value = false
      }
    })
    loadingTask = getDocument({ url: props.url, rangeChunkSize: 256 * 1024, disableAutoFetch: true, disableStream: true, useSystemFonts: true })
    loadingTask.onPassword = (update, reason) => {
      if (!isCurrent()) return
      passwordUpdater = update
      passwordRequired.value = true
      password.value = ''
      error.value = reason === PasswordResponses.INCORRECT_PASSWORD ? '密码不正确，请重试' : ''
      loading.value = false
    }
    const document = await loadingTask.promise
    if (!isCurrent()) return
    pages.value = document.numPages
    if (!pages.value) throw new Error('PDF 没有可显示的页面')
    passwordRequired.value = false
    activeViewer.setDocument(document)
    await activeViewer.pagesPromise
  } catch (cause) {
    if (!disposed && sequence === openSequence) {
      if (cause?.name !== 'AbortException') error.value = String(cause?.message || cause)
      loading.value = false
    }
  }
}

function submitPassword() {
  if (!password.value || !passwordUpdater) return
  passwordRequired.value = false
  loading.value = true
  error.value = ''
  passwordUpdater(password.value)
}

function changeZoom(step) {
  if (!viewer || !pages.value) return
  cancelFit()
  fitWidth.value = false
  viewer.currentScale = Math.max(.25, Math.min(3, viewer.currentScale + step))
}

function submitPage() {
  const requested = Number(pageInput.value)
  if (!viewer || !pages.value || !Number.isInteger(requested) || !pageInput.value.trim()) return
  const target = Math.max(1, Math.min(pages.value, requested))
  viewer.currentPageNumber = target
  pageInput.value = String(target)
}

onMounted(() => {
  if (stage.value && typeof ResizeObserver !== 'undefined') {
    stageObserver = new ResizeObserver(fitOnResize)
    stageObserver.observe(stage.value)
  } else window.addEventListener('resize', fitOnResize)
  void openPDF()
})
onBeforeUnmount(() => {
  disposed = true
  stageObserver?.disconnect()
  window.removeEventListener('resize', fitOnResize)
  ++openSequence
  void releasePDF()
})
</script>

<template>
  <div class="document-viewer">
    <div class="document-toolbar">
      <strong>PDF 预览</strong>
      <div v-if="pages" class="document-controls">
        <form class="pdf-page-jump" @submit.prevent="submitPage">
          <input class="input" aria-label="跳转到 PDF 页码" title="输入页码后按 Enter 跳转" inputmode="numeric" :value="editingPage ? pageInput : pageNumber" @focus="editingPage = true; pageInput = String(pageNumber); $event.target.select()" @input="pageInput = $event.target.value" @blur="editingPage = false" />
          <span>/ {{ pages }}</span>
        </form>
        <span class="document-divider"></span>
        <button class="btn sm" aria-label="缩小 PDF" @click="changeZoom(-.25)">−</button>
        <button class="btn sm" :aria-pressed="fitWidth" @click="fitPageWidth">适合宽度</button>
        <button class="btn sm" aria-label="放大 PDF" @click="changeZoom(.25)">＋</button>
      </div>
    </div>
    <div class="document-body">
      <div ref="stage" class="pdf-stage" aria-label="PDF 连续页面"><div ref="content" class="pdfViewer"></div></div>
      <div v-if="passwordRequired" class="document-state">
        <form @submit.prevent="submitPassword"><label>请输入 PDF 密码</label><input v-model="password" class="input" type="password" autofocus /><button class="btn primary sm" type="submit">解锁</button></form>
        <span v-if="error">{{ error }}</span>
      </div>
      <div v-else-if="loading" class="document-state"><span class="spin"></span>正在解析 PDF…</div>
      <div v-else-if="error" class="document-state" role="alert"><span>{{ error }}</span><button class="btn sm" type="button" @click="openPDF">重试</button></div>
    </div>
  </div>
</template>

<style scoped>
.document-viewer { flex: 1; min-height: 0; display: flex; flex-direction: column; }
.document-toolbar { min-height: 48px; padding: 7px 16px; display: flex; align-items: center; justify-content: space-between; gap: 12px; border-bottom: 1px solid var(--border-light); background: var(--bg-surface); }
.document-toolbar strong { font-size: 12px; color: var(--text-secondary); white-space: nowrap; }
.document-controls { display: flex; align-items: center; justify-content: flex-end; gap: 6px; flex-wrap: wrap; font-size: 12px; color: var(--text-secondary); font-variant-numeric: tabular-nums; }
.document-divider { width: 1px; height: 20px; margin: 0 4px; background: var(--border-light); }
.pdf-page-jump { display: flex; align-items: center; gap: 5px; }
.pdf-page-jump input { width: 54px; height: 26px; padding: 0 5px; text-align: center; font-variant-numeric: tabular-nums; }
.document-controls [aria-pressed=true] { background: var(--bg-active); }
.document-body { position: relative; flex: 1; min-height: 0; }
.document-state { position: absolute; inset: 0; z-index: 2; display: flex; align-items: center; justify-content: center; flex-direction: column; gap: 10px; color: var(--text-secondary); background: var(--bg-base); font-size: 13px; }
.document-state form { display: flex; align-items: center; gap: 8px; }
.pdf-stage { position: absolute; inset: 0; overflow: auto; padding: 16px 0; background: var(--bg-base); }
.pdfViewer :deep(.page) { box-shadow: var(--shadow-md); }
@media (max-width: 600px) { .document-toolbar { flex-wrap: wrap; } .document-controls { justify-content: flex-start; } }
</style>
