<script setup>
// 高级预览弹窗：
// 1. 图片画廊（缩放/拖拽平移/90度旋转/翻页/胶卷条）
// 2. 文本与代码专业预览/编辑器（大屏自适应/最大化全屏/滚动同步/防折行排布/Markdown精美排版/Ctrl+S云端回传/状态栏）
// 3. 音频播放；PDF 等非白名单格式由文件页提示下载，不会进入此弹窗
import { ref, computed, watch, nextTick, onMounted, onBeforeUnmount } from 'vue'
import { PreviewURL, PinFileSnapshot, openKindOf, formatBytes, formatTime, saveCloudText, copyText, iconOf } from '../api'
import { WindowMinimise, WindowToggleMaximise, WindowIsMaximised } from '../../wailsjs/runtime/runtime'
import Modal from './Modal.vue'
import ConfirmModal from './ConfirmModal.vue'
import UiIcon from './UiIcon.vue'

const props = defineProps({
  account: { type: Object, required: true },
  file: { type: Object, required: true },
  fileList: { type: Array, default: () => [] },
})
const emit = defineEmits(['close', 'toast', 'saved'])

const activeFile = ref(props.file)
const kind = computed(() => openKindOf(activeFile.value))

const url = ref('')
const text = ref('')
const editContent = ref('')
const saving = ref(false)
const loading = ref(true)
const error = ref('')
const winMax = ref(false) // 应用窗口最大化状态
function winMinimise() { try { WindowMinimise() } catch { /* browser preview */ } }
function winToggleMax() {
  try {
    WindowToggleMaximise()
    WindowIsMaximised().then((v) => { winMax.value = !!v }).catch(() => {})
  } catch { /* browser preview */ }
}

// ---------- 图片画廊状态 ----------
const imageList = computed(() => {
  const list = props.fileList && props.fileList.length ? props.fileList : [props.file]
  return list.filter((f) => !f.isDir && openKindOf(f) === 'image')
})

const currentImageIdx = computed(() => {
  const id = activeFile.value.file_id
  const idx = imageList.value.findIndex((f) => f.file_id === id)
  return idx >= 0 ? idx : 0
})

// ---------- 图片查看器（沉浸式重写） ----------
// 图层转场模型：切图时旧层冻结变换退出、新层解码完成后才淡入，切换无白屏闪烁
const stageEl = ref(null)
const layers = ref([]) // [{ key, url, file, w, h, leaving, frozen }]
const natural = ref({ w: 0, h: 0 })
const fitScale = ref(1) // 适配视口的基准缩放（不超过原始尺寸）
const zoom = ref(1) // 1 = 适配窗口
const rotation = ref(0)
const pos = ref({ x: 0, y: 0 })
const isDragging = ref(false)
const uiHidden = ref(false) // 光标闲置后隐藏控件
const showFilm = ref(false)
const imgSwitching = ref(false)
let dragStart = { x: 0, y: 0, posX: 0, posY: 0 }
let activeDragCleanup = null
let idleTimer = null
let imgSeq = 0
let stageRO = null

const liveTransform = computed(
  () => `translate(${pos.value.x}px, ${pos.value.y}px) scale(${(fitScale.value * zoom.value).toFixed(4)}) rotate(${rotation.value}deg)`
)

function computeFit() {
  const el = stageEl.value, n = natural.value
  if (!el || !n.w || !n.h) { fitScale.value = 1; return }
  fitScale.value = Math.min(1, (el.clientWidth - 24) / n.w, (el.clientHeight - 24) / n.h)
}

function resetImageTransform() {
  zoom.value = 1
  rotation.value = 0
  pos.value = { x: 0, y: 0 }
}

function stagePoint(e) {
  const r = stageEl.value.getBoundingClientRect()
  return { x: e.clientX - r.left - r.width / 2, y: e.clientY - r.top - r.height / 2 }
}

// 缩放（origin 存在且未旋转时锚定光标位置）
function zoomTo(next, origin) {
  const old = zoom.value
  const nz = Math.min(10, Math.max(0.1, Number(next.toFixed(3))))
  if (nz === old) return
  if (origin && rotation.value % 360 === 0) {
    const k = nz / old
    pos.value = { x: origin.x - (origin.x - pos.value.x) * k, y: origin.y - (origin.y - pos.value.y) * k }
  }
  zoom.value = nz
}
function zoomByFactor(f) { zoomTo(zoom.value * f) }
function rotateBy(deg) { rotation.value = (rotation.value + deg + 360) % 360 }

function onWheel(e) {
  if (kind.value !== 'image') return
  e.preventDefault()
  pokeUI()
  zoomTo(zoom.value * (e.deltaY < 0 ? 1.18 : 1 / 1.18), stagePoint(e))
}

// 双击：适配 ↔ 2x（锚定点击点）
function onDblClick(e) {
  if (kind.value !== 'image') return
  if (e.target.closest('.pv-ctl, .pv-edge, .pv-filmstrip')) return
  if (Math.abs(zoom.value - 1) > 0.01 || pos.value.x || pos.value.y) resetImageTransform()
  else zoomTo(2, stagePoint(e))
}

function onPointerDown(e) {
  if (kind.value !== 'image' || e.button !== 0) return
  if (e.target.closest('.pv-ctl, .pv-edge, .pv-filmstrip')) return
  isDragging.value = true
  dragStart = { x: e.clientX, y: e.clientY, posX: pos.value.x, posY: pos.value.y }
  const onMove = (ev) => {
    if (!isDragging.value) return
    pos.value = { x: dragStart.posX + (ev.clientX - dragStart.x), y: dragStart.posY + (ev.clientY - dragStart.y) }
  }
  const onUp = () => {
    isDragging.value = false
    window.removeEventListener('pointermove', onMove)
    window.removeEventListener('pointerup', onUp)
    activeDragCleanup = null
  }
  window.addEventListener('pointermove', onMove)
  window.addEventListener('pointerup', onUp)
  activeDragCleanup = onUp
}

// 控件自动隐藏（类播放器）：光标活动 2.4s 后隐藏
function pokeUI() {
  uiHidden.value = false
  clearTimeout(idleTimer)
  idleTimer = setTimeout(() => { if (!isDragging.value) uiHidden.value = true }, 2400)
}

function switchImage(step) {
  const total = imageList.value.length
  if (total <= 1) return
  pokeUI()
  const nextIdx = (currentImageIdx.value + step + total) % total
  activeFile.value = imageList.value[nextIdx]
}

function selectImage(img) {
  if (activeFile.value.file_id === img.file_id) return
  activeFile.value = img
}

function loadImageBitmap(url) {
  return new Promise((resolve, reject) => {
    const im = new Image()
    im.onload = () => resolve(im)
    im.onerror = () => reject(new Error('图片解码失败'))
    im.src = url
  })
}

async function loadImage() {
  const seq = ++imgSeq
  const file = activeFile.value
  imgSwitching.value = true
  error.value = ''
  try {
    await PinFileSnapshot(props.account.user_id, props.account.drive_id, file)
    const previewUrl = await PreviewURL(props.account.user_id, props.account.drive_id, file.file_id)
    const im = await loadImageBitmap(previewUrl) // 等像素就绪再换层，避免白屏
    if (seq !== imgSeq) return
    const frozen = liveTransform.value
    layers.value.forEach((l) => { l.leaving = true; l.frozen = frozen })
    natural.value = { w: im.naturalWidth, h: im.naturalHeight }
    computeFit()
    resetImageTransform()
    layers.value.push({ key: `${file.file_id}-${seq}`, url: previewUrl, file, w: im.naturalWidth, h: im.naturalHeight })
    setTimeout(() => { layers.value = layers.value.filter((l) => !l.leaving) }, 300)
    preloadNeighbors()
  } catch (e) {
    if (seq !== imgSeq) return
    // 首次加载失败才全屏报错；切换失败保留当前图
    if (!layers.value.length) error.value = String(e && e.message ? e.message : e)
  } finally {
    if (seq === imgSeq) { imgSwitching.value = false; loading.value = false }
  }
}

// 预载相邻两张，切换基本零等待
async function preloadNeighbors() {
  const list = imageList.value, idx = currentImageIdx.value
  for (const step of [1, -1]) {
    const f = list[(idx + step + list.length) % list.length]
    if (!f || f.file_id === activeFile.value.file_id) continue
    try {
      await PinFileSnapshot(props.account.user_id, props.account.drive_id, f)
      const u = await PreviewURL(props.account.user_id, props.account.drive_id, f.file_id)
      const im = new Image()
      im.src = u
    } catch { /* 预载失败静默 */ }
  }
}

// ---------- 文本与代码专业预览/编辑状态 ----------
const isMarkdownFile = computed(() => {
  const name = (activeFile.value.name || '').toLowerCase()
  return name.endsWith('.md') || name.endsWith('.markdown')
})

// 文本模式：'preview' (代码/文本预览) | 'markdown' (文档渲染) | 'edit' (在线编辑)
const textMode = ref('preview')
const fontSize = ref(13.5)
const wordWrap = ref(false) // 默认不自动换行，保持宽敞横向滚动，避免长行频繁被折断
const showLineNumbers = ref(true)
const encoding = ref('UTF-8')
const copiedFull = ref(false)
const showSearch = ref(false)
const searchKw = ref('')
const searchInputEl = ref(null)

// 光标位置
const cursorPos = ref({ line: 1, col: 1 })
const gutterEl = ref(null)
const viewEl = ref(null)
const editorEl = ref(null)
const confirmLeaveDialog = ref(false)

const currentText = computed(() => (textMode.value === 'edit' ? editContent.value : text.value))
const textLines = computed(() => currentText.value.split('\n'))
const isModified = computed(() => textMode.value === 'edit' && editContent.value !== text.value)

// 语言标签识别
const langMeta = computed(() => {
  const name = (activeFile.value.name || '').toLowerCase()
  const ext = name.includes('.') ? name.split('.').pop() : ''
  const map = {
    js: 'JavaScript', mjs: 'JavaScript', cjs: 'JavaScript',
    ts: 'TypeScript', tsx: 'TypeScript React', jsx: 'React JSX',
    vue: 'Vue Component',
    json: 'JSON', json5: 'JSON5', jsonc: 'JSON with Comments',
    html: 'HTML', htm: 'HTML',
    css: 'CSS', scss: 'SCSS', sass: 'SASS', less: 'LESS',
    go: 'Go',
    py: 'Python', pyw: 'Python',
    rs: 'Rust',
    java: 'Java', kt: 'Kotlin',
    c: 'C', cpp: 'C++', cc: 'C++', h: 'C/C++ Header', hpp: 'C++ Header',
    cs: 'C#',
    php: 'PHP',
    rb: 'Ruby',
    sh: 'Shell Script', bash: 'Bash Script', zsh: 'Zsh Script', ps1: 'PowerShell', bat: 'Batch', cmd: 'Batch',
    sql: 'SQL Database',
    yaml: 'YAML', yml: 'YAML',
    xml: 'XML', svg: 'SVG Image/XML',
    toml: 'TOML', ini: 'INI Config', conf: 'Config', env: 'Environment',
    md: 'Markdown', markdown: 'Markdown',
    txt: 'Plain Text', log: 'Log File',
  }
  return map[ext] || (ext ? ext.toUpperCase() : 'Plain Text')
})

// 滚动同步（行号与内容区 100% 像素级对齐）
function onContentScroll(e) {
  if (gutterEl.value) {
    gutterEl.value.scrollTop = e.target.scrollTop
  }
}

// 统计字符数/字数
const charCount = computed(() => currentText.value.length)
const lineEnding = computed(() => (currentText.value.includes('\r\n') ? 'CRLF' : 'LF'))

// 搜索匹配拆分
function searchParts(line) {
  const kw = searchKw.value.trim()
  if (!kw) return null
  const str = String(line || '')
  const i = str.toLowerCase().indexOf(kw.toLowerCase())
  if (i < 0) return null
  return [
    { text: str.slice(0, i), hit: false },
    { text: str.slice(i, i + kw.length), hit: true },
    { text: str.slice(i + kw.length), hit: false },
  ]
}

const matchCount = computed(() => {
  const kw = searchKw.value.trim()
  if (!kw) return 0
  let count = 0
  const kwLower = kw.toLowerCase()
  for (const line of textLines.value) {
    let p = 0
    const lower = line.toLowerCase()
    while ((p = lower.indexOf(kwLower, p)) !== -1) {
      count++
      p += kwLower.length
    }
  }
  return count
})

function toggleSearch() {
  showSearch.value = !showSearch.value
  if (showSearch.value) {
    nextTick(() => searchInputEl.value?.focus())
  } else {
    searchKw.value = ''
  }
}

// 复制全部文本
async function copyAllText() {
  const ok = await copyText(currentText.value)
  if (ok) {
    copiedFull.value = true
    emit('toast', '已复制全部文本内容', 'success')
    setTimeout(() => { copiedFull.value = false }, 1800)
  } else {
    emit('toast', '复制失败', 'error')
  }
}

// 编辑器光标与按键事件
function updateCursorPos(e) {
  const el = e.target
  if (!el || typeof el.selectionStart !== 'number') return
  const val = el.value.slice(0, el.selectionStart)
  const lines = val.split('\n')
  cursorPos.value = {
    line: lines.length,
    col: lines[lines.length - 1].length + 1,
  }
}

function onEditorKeyDown(e) {
  if (e.key === 'Tab') {
    e.preventDefault()
    const el = e.target
    const start = el.selectionStart
    const end = el.selectionEnd
    const val = editContent.value
    editContent.value = val.substring(0, start) + '  ' + val.substring(end)
    nextTick(() => {
      el.selectionStart = el.selectionEnd = start + 2
      updateCursorPos(e)
    })
  } else if ((e.ctrlKey || e.metaKey) && (e.key === 's' || e.code === 'KeyS')) {
    e.preventDefault()
    doSaveText()
  } else if ((e.ctrlKey || e.metaKey) && (e.key === 'f' || e.code === 'KeyF')) {
    e.preventDefault()
    toggleSearch()
  } else {
    nextTick(() => updateCursorPos(e))
  }
}

// 保存修改回传云端
async function doSaveText() {
  if (saving.value || !isModified.value) return
  saving.value = true
  try {
    const parentId = activeFile.value.parent_file_id || 'root'
    await saveCloudText(
      props.account.user_id,
      props.account.drive_id,
      parentId,
      activeFile.value.name,
      editContent.value
    )
    text.value = editContent.value
    emit('toast', '保存成功，已上传到网盘', 'success')
    emit('saved')
  } catch (e) {
    emit('toast', '保存失败: ' + String(e), 'error')
  } finally {
    saving.value = false
  }
}

// 关闭前未保存检查
function handleCloseRequest() {
  if (isModified.value) {
    confirmLeaveDialog.value = true
  } else {
    emit('close')
  }
}

// ---------- 轻量安全 Markdown 解析器 ----------
function escapeHtml(str) {
  return String(str || '')
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;')
}

function sanitizeUrl(rawUrl) {
  const trimmed = String(rawUrl || '').trim()
  if (/^(https?:\/\/|mailto:|\/|\.\/|#)/i.test(trimmed)) {
    return escapeHtml(trimmed)
  }
  return '#'
}

function renderMarkdown(src) {
  if (!src) return ''

  // 代码块提取 (```lang ... ```)
  const codeBlocks = []
  let md = src.replace(/```([a-zA-Z0-9_-]*)\n([\s\S]*?)```/g, (_, lang, code) => {
    const idx = codeBlocks.length
    const safeLang = escapeHtml(lang || 'code')
    const safeCode = escapeHtml(code.trim())
    codeBlocks.push(
      `<div class="md-code-block"><div class="md-code-header"><span class="md-code-lang">${safeLang}</span></div><pre><code>${safeCode}</code></pre></div>`
    )
    return `<!--CODEBLOCK_${idx}-->`
  })

  // 转义 HTML 字符
  md = escapeHtml(md)

  // 行内代码
  md = md.replace(/`([^`]+)`/g, '<code class="md-inline-code">$1</code>')

  // 标题
  md = md.replace(/^###### (.*$)/gim, '<h6 class="md-h6">$1</h6>')
  md = md.replace(/^##### (.*$)/gim, '<h5 class="md-h5">$1</h5>')
  md = md.replace(/^#### (.*$)/gim, '<h4 class="md-h4">$1</h4>')
  md = md.replace(/^### (.*$)/gim, '<h3 class="md-h3">$1</h3>')
  md = md.replace(/^## (.*$)/gim, '<h2 class="md-h2">$1</h2>')
  md = md.replace(/^# (.*$)/gim, '<h1 class="md-h1">$1</h1>')

  // 分割线
  md = md.replace(/^---$/gim, '<hr class="md-hr" />')

  // 引用块 (转义后为 &gt;)
  md = md.replace(/^&gt;\s?(.*$)/gim, '<blockquote class="md-quote">$1</blockquote>')

  // 格式
  md = md.replace(/\*\*\*(.*?)\*\*\*/g, '<strong><em>$1</em></strong>')
  md = md.replace(/\*\*(.*?)\*\*/g, '<strong>$1</strong>')
  md = md.replace(/\*(.*?)\*/g, '<em>$1</em>')
  md = md.replace(/~~(.*?)~~/g, '<del>$1</del>')

  // 任务列表
  md = md.replace(/^- \[x\] (.*$)/gim, '<li class="md-task-item"><span class="md-task-box checked">✓</span> <span>$1</span></li>')
  md = md.replace(/^- \[ \] (.*$)/gim, '<li class="md-task-item"><span class="md-task-box"></span> <span>$1</span></li>')

  // 列表
  md = md.replace(/^[-\*] (.*$)/gim, '<li class="md-list-item">$1</li>')

  // 链接（严格白名单与 URL 属性清洗）
  md = md.replace(/\[([^\]]+)\]\(([^)]+)\)/g, (_, label, link) => {
    const href = sanitizeUrl(link)
    return `<a href="${href}" target="_blank" rel="noopener noreferrer" class="md-link">${label}</a>`
  })

  // 段落
  md = md.replace(/\n\n/g, '<div class="md-gap"></div>')
  md = md.replace(/\n/g, '<br />')

  // 还原代码块
  codeBlocks.forEach((block, idx) => {
    md = md.replace(`<!--CODEBLOCK_${idx}-->`, block)
  })

  return md
}

const renderedMarkdown = computed(() => renderMarkdown(text.value))

// ---------- 加载核心逻辑 ----------
let loadSeq = 0
async function loadPreview() {
  if (kind.value === 'image') return loadImage()
  const seq = ++loadSeq
  loading.value = true
  error.value = ''
  url.value = ''
  try {
    if (!['image', 'text', 'audio'].includes(kind.value)) {
      throw new Error(kind.value === 'pdf' ? 'PDF 暂不支持在线预览，请下载后查看' : '此文件格式不支持在线预览，请下载后查看')
    }
    await PinFileSnapshot(
      props.account.user_id,
      props.account.drive_id,
      activeFile.value
    )
    const previewUrl = await PreviewURL(
      props.account.user_id,
      props.account.drive_id,
      activeFile.value.file_id
    )
    if (seq !== loadSeq) return
    url.value = previewUrl
    if (kind.value === 'text') {
      const resp = await fetch(previewUrl)
      if (!resp.ok) throw new Error(`HTTP ${resp.status} 加载失败`)
      const buf = await resp.arrayBuffer()
      if (buf.byteLength > 4 * 1024 * 1024) throw new Error('文本文件超过 4MB，不支持在线预览，请下载后查看')
      const decoded = decodeText(buf)
      text.value = decoded.text
      encoding.value = decoded.encoding
      editContent.value = text.value
      textMode.value = isMarkdownFile.value ? 'markdown' : 'preview'
    }
  } catch (e) {
    if (seq !== loadSeq) return
    error.value = String(e && e.message ? e.message : e)
  } finally {
    if (seq === loadSeq) loading.value = false
  }
}

watch(() => props.file, (f) => { if (f) activeFile.value = f })
watch(() => activeFile.value.file_id, loadPreview)

function onKey(e) {
  if (textMode.value === 'edit') return
  if (kind.value === 'image') {
    if (e.key === 'ArrowLeft') switchImage(-1)
    else if (e.key === 'ArrowRight') switchImage(1)
    else if (e.key === '+' || e.key === '=') zoomByFactor(1.25)
    else if (e.key === '-' || e.key === '_') zoomByFactor(1 / 1.25)
    else if (e.key === '0') resetImageTransform()
    else if (e.key === 'r' || e.key === 'R') rotateBy(90)
  } else if (kind.value === 'text') {
    if ((e.ctrlKey || e.metaKey) && (e.key === 'f' || e.code === 'KeyF')) {
      e.preventDefault()
      toggleSearch()
    }
  }
}

onMounted(() => {
  loadPreview()
  pokeUI()
  try { WindowIsMaximised().then((v) => { winMax.value = !!v }).catch(() => {}) } catch { /* browser preview */ }
  window.addEventListener('keydown', onKey)
  if (typeof ResizeObserver !== 'undefined') stageRO = new ResizeObserver(() => computeFit())
})
watch(stageEl, (el) => {
  stageRO?.disconnect()
  if (el && stageRO) { stageRO.observe(el); nextTick(computeFit) }
})
onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKey)
  clearTimeout(idleTimer)
  stageRO?.disconnect()
  if (activeDragCleanup) activeDragCleanup()
})

// 文本编码探测
function decodeText(buf) {
  const u8 = new Uint8Array(buf)
  if (u8.length >= 3 && u8[0] === 0xef && u8[1] === 0xbb && u8[2] === 0xbf) {
    return { text: new TextDecoder('utf-8').decode(u8.subarray(3)), encoding: 'UTF-8 (BOM)' }
  }
  const utf8 = new TextDecoder('utf-8', { fatal: false }).decode(u8)
  let replacements = 0
  for (let i = 0; i < utf8.length; i++) {
    if (utf8.charCodeAt(i) === 0xfffd) replacements++
  }
  if (replacements > utf8.length / 100) {
    try {
      return { text: new TextDecoder('gbk').decode(u8), encoding: 'GBK' }
    } catch {
      // 回退
    }
  }
  return { text: utf8, encoding: 'UTF-8' }
}
</script>

<template>
  <Modal
    :dialog-class="'preview-modal' + (kind === 'image' ? ' immersive' : '')"
    width=""
    @close="handleCloseRequest"
    body-class="preview-body"
  >
    <!-- 自定义高级弹窗头部（图片预览为沉浸式浮层，不使用实体头栏） -->
    <template #head>
      <div v-if="kind !== 'image'" class="pv-head-custom">
        <div class="pv-head-icon" :class="'ft-' + iconOf(activeFile)">
          <UiIcon :name="iconOf(activeFile)" :size="20" />
        </div>
        <div class="pv-head-meta">
          <div class="pv-head-title" :title="activeFile.name">
            <span>{{ activeFile.name }}</span>
            <span v-if="isModified" class="pv-head-unsaved" title="有未保存的修改">*</span>
          </div>
          <div class="pv-head-sub">
            <span>{{ formatBytes(activeFile.size) }}</span>
            <span class="pv-head-sep">·</span>
            <span>修改时间: {{ formatTime(activeFile.time) }}</span>
            <template v-if="kind === 'text'">
              <span class="pv-head-sep">·</span>
              <span class="pv-head-pill">{{ langMeta }}</span>
              <span class="pv-head-pill">{{ encoding }}</span>
            </template>
          </div>
        </div>
      </div>
    </template>

    <!-- 头部右侧窗口控制（最小化 / 最大化还原） -->
    <template #head-extra>
      <template v-if="kind !== 'image'">
        <button class="icon-btn" style="width:28px;height:28px" title="最小化" @click="winMinimise">
          <UiIcon name="minimize" :size="13" />
        </button>
        <button class="icon-btn" style="width:28px;height:28px" :title="winMax ? '还原窗口' : '最大化窗口'" @click="winToggleMax">
          <UiIcon :name="winMax ? 'restore' : 'maximize'" :size="13" />
        </button>
      </template>
    </template>

    <!-- 顶部悬浮工具条（仅文本） -->
    <div v-if="kind === 'text'" class="pv-toolbar">
      <!-- 文本与代码专业工具 -->
      <template>
        <!-- 模式切换分段按钮 -->
        <div class="pv-mode-seg">
          <button
            v-if="isMarkdownFile"
            class="pv-seg-btn"
            :class="{ active: textMode === 'markdown' }"
            @click="textMode = 'markdown'"
          >
            <UiIcon name="info" :size="12" /> Markdown 渲染
          </button>
          <button
            class="pv-seg-btn"
            :class="{ active: textMode === 'preview' }"
            @click="textMode = 'preview'"
          >
            <UiIcon name="play" :size="12" /> 代码预览
          </button>
          <button
            class="pv-seg-btn"
            :class="{ active: textMode === 'edit' }"
            @click="textMode = 'edit'"
          >
            <UiIcon name="pencil" :size="12" /> 在线编辑
          </button>
        </div>

        <span class="pv-sep"></span>

        <!-- 复制全文 -->
        <button class="tbtn xs" :title="'复制全部文本内容'" @click="copyAllText">
          <UiIcon v-if="copiedFull" name="check" :size="12" class="icon-check-pop" />
          <UiIcon v-else name="copy" :size="12" />
          <span>{{ copiedFull ? '已复制' : '复制全文' }}</span>
        </button>

        <!-- 自动换行 -->
        <button
          v-if="textMode !== 'markdown'"
          class="tbtn xs"
          :class="{ active: wordWrap }"
          :title="wordWrap ? '当前：自动折行（点击切换为单行连续横向滚动）' : '当前：单行连续横向滚动（点击切换为自动折行）'"
          @click="wordWrap = !wordWrap"
        >
          <UiIcon name="list" :size="12" />
          <span>自动换行: {{ wordWrap ? '已开启' : '已关闭' }}</span>
        </button>

        <!-- 字号调节 -->
        <div v-if="textMode !== 'markdown'" class="pv-font-size-group">
          <button class="tbtn xs icon" title="减小字号" :disabled="fontSize <= 11" @click="fontSize = Math.max(11, fontSize - 1)">A-</button>
          <span class="pv-fs-val">{{ fontSize }}px</span>
          <button class="tbtn xs icon" title="加大字号" :disabled="fontSize >= 18" @click="fontSize = Math.min(18, fontSize + 1)">A+</button>
        </div>

        <!-- 查找搜索按钮 -->
        <button
          v-if="textMode !== 'markdown'"
          class="tbtn xs"
          :class="{ active: showSearch }"
          title="文本查找 (Ctrl+F)"
          @click="toggleSearch"
        >
          <UiIcon name="search" :size="12" /> 查找
        </button>

        <!-- 保存按钮 (编辑模式下) -->
        <div v-if="textMode === 'edit'" class="pv-edit-actions">
          <span v-if="isModified" class="pv-modified-hint"><span class="pulse-dot"></span>已修改</span>
          <button class="btn primary sm" :disabled="!isModified || saving" title="保存修改并上传 (Ctrl+S)" @click="doSaveText">
            <span v-if="saving" class="spin spin-on-primary"></span>
            <UiIcon v-else name="check" :size="13" />
            <span>{{ saving ? '保存上传中…' : '保存修改 (Ctrl+S)' }}</span>
          </button>
        </div>
      </template>
    </div>

    <!-- 文本查找浮层 -->
    <div v-if="kind === 'text' && showSearch && textMode !== 'markdown'" class="pv-search-bar">
      <UiIcon name="search" :size="14" style="color:var(--color-primary)" />
      <input
        ref="searchInputEl"
        v-model="searchKw"
        class="pv-search-input"
        placeholder="在文本中查找… (Esc 关闭)"
        @keydown.esc.stop="toggleSearch"
      />
      <span class="pv-search-count">{{ matchCount ? `${matchCount} 个匹配` : (searchKw ? '无匹配' : '') }}</span>
      <button class="btn-circle sm" title="关闭查找" @click="toggleSearch"><UiIcon name="close" :size="12" /></button>
    </div>

    <!-- 主展示区 -->
    <div v-if="loading" class="empty pv-center"><span class="spin"></span><span>加载内容中…</span></div>
    <div v-else-if="error" class="empty pv-center">
      <span class="empty-icon"><UiIcon name="warning" :size="32" /></span>
      <span>{{ error }}</span>
      <button class="btn sm" style="margin-top:8px" @click="loadPreview">重试加载</button>
    </div>

    <template v-else>
      <!-- 1. 图片画廊（沉浸式舞台） -->
      <div
        v-if="kind === 'image'"
        ref="stageEl"
        class="pv-stage"
        :class="{ 'ui-hidden': uiHidden, grabbing: isDragging }"
        @wheel="onWheel"
        @pointerdown="onPointerDown"
        @pointermove="pokeUI"
        @dblclick="onDblClick"
      >
        <div
          v-for="layer in layers"
          :key="layer.key"
          class="pv-img-layer"
          :class="{ leaving: layer.leaving }"
        >
          <img
            :src="layer.url"
            :alt="layer.file.name"
            draggable="false"
            :style="{ transform: layer.frozen || liveTransform, width: layer.w + 'px', height: layer.h + 'px' }"
          />
        </div>

        <div v-if="imgSwitching && layers.length" class="pv-stage-busy"><span class="spin"></span></div>

        <template v-if="imageList.length > 1">
          <button class="pv-edge left" title="上一张 (←)" @click.stop="switchImage(-1)"><UiIcon name="back" :size="17" /></button>
          <button class="pv-edge right" title="下一张 (→)" @click.stop="switchImage(1)"><UiIcon name="forward" :size="17" /></button>
        </template>

        <!-- 顶部浮动栏：渐变遮罩，无实体条（播放器语言） -->
        <div class="pv-topbar" @pointerdown.stop @dblclick.stop>
          <div class="pv-topbar-meta">
            <UiIcon :name="iconOf(activeFile)" :size="15" />
            <span class="pv-topbar-name">{{ activeFile.name }}</span>
            <span class="pv-topbar-sub">{{ formatBytes(activeFile.size) }}<template v-if="imageList.length > 1"> · {{ currentImageIdx + 1 }} / {{ imageList.length }}</template></span>
          </div>
          <div class="pv-topbar-actions">
            <button class="pv-ctl-btn" title="最小化" @click="winMinimise"><UiIcon name="minimize" :size="14" /></button>
            <button class="pv-ctl-btn" :title="winMax ? '还原窗口' : '最大化窗口'" @click="winToggleMax"><UiIcon :name="winMax ? 'restore' : 'maximize'" :size="14" /></button>
            <button class="pv-ctl-btn" title="关闭 (Esc)" @click="handleCloseRequest"><UiIcon name="close" :size="15" /></button>
          </div>
        </div>

        <!-- 胶卷缩略图条（控制条切换） -->
        <div v-if="showFilm && imageList.length > 1" class="pv-filmstrip" @pointerdown.stop @dblclick.stop>
          <div
            v-for="img in imageList"
            :key="img.file_id"
            class="pv-film-thumb"
            :class="{ active: img.file_id === activeFile.file_id }"
            :title="img.name"
            @click="selectImage(img)"
          >
            <img v-if="img.thumbnail" :src="img.thumbnail" alt="" draggable="false" />
            <UiIcon v-else name="image" :size="16" />
          </div>
        </div>

        <!-- 浮动控制条 -->
        <div class="pv-ctl" @pointerdown.stop @dblclick.stop>
          <button class="pv-ctl-btn" :disabled="imageList.length <= 1" title="上一张 (←)" @click="switchImage(-1)"><UiIcon name="back" :size="15" /></button>
          <span class="pv-ctl-counter">{{ currentImageIdx + 1 }} / {{ imageList.length }}</span>
          <button class="pv-ctl-btn" :disabled="imageList.length <= 1" title="下一张 (→)" @click="switchImage(1)"><UiIcon name="forward" :size="15" /></button>
          <span class="pv-ctl-sep"></span>
          <button class="pv-ctl-btn pv-ctl-text" title="缩小 (-)" @click="zoomByFactor(1 / 1.25)">−</button>
          <span class="pv-ctl-zoom" title="点击复原（适配窗口）" @click="resetImageTransform">{{ Math.round(fitScale * zoom * 100) }}%</span>
          <button class="pv-ctl-btn" title="放大 (+)" @click="zoomByFactor(1.25)"><UiIcon name="plus" :size="13" /></button>
          <button class="pv-ctl-btn" title="适配窗口 (0)" @click="resetImageTransform"><UiIcon name="size" :size="14" /></button>
          <button class="pv-ctl-btn" title="顺时针旋转 90° (R)" @click="rotateBy(90)"><UiIcon name="refresh" :size="13" /></button>
          <span class="pv-ctl-sep"></span>
          <button class="pv-ctl-btn" :class="{ active: showFilm }" :disabled="imageList.length <= 1" title="缩略图" @click="showFilm = !showFilm"><UiIcon name="grid" :size="14" /></button>
        </div>
      </div>

      <!-- 2. 音频播放 -->
      <div v-else-if="kind === 'audio'" class="pv-audio-container">
        <div class="pv-audio-disc">
          <UiIcon name="audio" :size="48" style="color:var(--color-primary)" />
        </div>
        <div class="pv-audio-name">{{ activeFile.name }}</div>
        <audio :src="url" controls autoplay class="pv-audio-player"></audio>
      </div>

      <!-- 4. 文本/代码/Markdown 专业展示区 -->
      <div v-else class="pv-text-container">
        <!-- Markdown 渲染视图 -->
        <div
          v-if="textMode === 'markdown'"
          class="pv-markdown-view"
          v-html="renderedMarkdown"
        ></div>

        <!-- 代码预览与编辑器双轨容器 -->
        <div v-else class="pv-code-box">
          <!-- 智能同步行号槽 -->
          <div v-if="showLineNumbers" ref="gutterEl" class="pv-gutter" :style="{ fontSize: fontSize + 'px' }">
            <div v-for="n in textLines.length" :key="n" class="pv-gutter-line">{{ n }}</div>
          </div>

          <!-- 在线编辑 textarea -->
          <textarea
            v-if="textMode === 'edit'"
            ref="editorEl"
            v-model="editContent"
            class="pv-editor-textarea"
            :class="{ wrap: wordWrap }"
            :style="{ fontSize: fontSize + 'px' }"
            spellcheck="false"
            @scroll="onContentScroll"
            @click="updateCursorPos"
            @keyup="updateCursorPos"
            @keydown="onEditorKeyDown"
          ></textarea>

          <!-- 查看模式 pre 渲染行 -->
          <div
            v-else
            ref="viewEl"
            class="pv-code-view"
            :class="{ wrap: wordWrap }"
            :style="{ fontSize: fontSize + 'px' }"
            @scroll="onContentScroll"
          >
            <div
              v-for="(line, idx) in textLines"
              :key="idx"
              class="pv-code-line"
            >
              <template v-if="searchKw && searchParts(line)">
                <template v-for="(part, pIdx) in searchParts(line)" :key="pIdx">
                  <mark v-if="part.hit" class="pv-search-hit">{{ part.text }}</mark>
                  <template v-else>{{ part.text }}</template>
                </template>
              </template>
              <template v-else>{{ line || ' ' }}</template>
            </div>
          </div>
        </div>

        <!-- 文本视图底部状态栏 (类似 VS Code / Sublime 的专业质感) -->
        <footer class="pv-statusbar">
          <div class="pv-sb-section">
            <span class="pv-sb-item"><UiIcon name="list" :size="12" />{{ textLines.length }} 行</span>
            <span class="pv-sb-item">{{ charCount }} 字符</span>
            <span class="pv-sb-item">{{ formatBytes(activeFile.size) }}</span>
            <span v-if="textMode === 'edit'" class="pv-sb-item pv-sb-pos">Ln {{ cursorPos.line }}, Col {{ cursorPos.col }}</span>
          </div>
          <div class="pv-sb-spacer"></div>
          <div class="pv-sb-section">
            <span class="pv-sb-item">{{ encoding }}</span>
            <span class="pv-sb-item">{{ lineEnding }}</span>
            <span class="pv-sb-item">{{ wordWrap ? '自动换行' : '不换行' }}</span>
            <span class="pv-sb-item pv-sb-lang">{{ langMeta }}</span>
          </div>
        </footer>
      </div>
    </template>

    <!-- 未保存修改确认弹窗 -->
    <ConfirmModal
      v-if="confirmLeaveDialog"
      title="存在未保存的修改"
      message="文本内容已发生变更，未保存离开将丢失修改。确定要关闭吗？"
      ok-text="放弃修改并关闭"
      :danger="true"
      @ok="confirmLeaveDialog = false; emit('close')"
      @cancel="confirmLeaveDialog = false"
    />
  </Modal>
</template>

<style scoped>
/* 弹窗头部自适应高级排布 */
.pv-head-custom {
  display: flex;
  align-items: center;
  gap: 12px;
  min-width: 0;
  flex: 1;
}
.pv-head-icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 34px;
  height: 34px;
  border-radius: var(--radius-sm);
  background: var(--bg-subtle);
  flex-shrink: 0;
}
.pv-head-meta {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
  flex: 1;
}
.pv-head-title {
  font-size: 15px;
  font-weight: 700;
  color: var(--text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  display: flex;
  align-items: center;
  gap: 4px;
}
.pv-head-unsaved {
  color: var(--color-warning);
  font-weight: 800;
  font-size: 16px;
}
.pv-head-sub {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--text-tertiary);
  white-space: nowrap;
}
.pv-head-sep { opacity: 0.6; }
.pv-head-pill {
  padding: 0 6px;
  border-radius: 4px;
  background: var(--bg-subtle);
  color: var(--text-secondary);
  font-size: 11px;
}

.pv-toolbar {
  display: flex;
  align-items: center;
  gap: 6px;
  min-height: 46px;
  padding: 7px 14px;
  background: var(--bg-surface);
  border-bottom: 1px solid var(--border-light);
  flex-shrink: 0;
  flex-wrap: nowrap;
  overflow-x: auto;
  scrollbar-width: thin;
}
.pv-toolbar > * { flex: 0 0 auto; }
.pv-sep { width: 1px; height: 18px; background: var(--border-light); margin: 0 4px; }
.pv-center { flex: 1; display: flex; flex-direction: column; align-items: center; justify-content: center; }
.pv-edit-actions { display: flex; align-items: center; gap: 8px; margin-left: auto; }

/* 模式分段选择器 */
.pv-mode-seg {
  display: inline-flex;
  background: var(--bg-subtle);
  border-radius: var(--radius-sm);
  padding: 2.5px;
  gap: 3px;
}
.pv-seg-btn {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 4px 11px;
  font-size: 12.5px;
  color: var(--text-secondary);
  border: none;
  background: transparent;
  border-radius: var(--radius-xs);
  cursor: pointer;
  transition: all var(--motion-fast) var(--motion-ease);
}
.pv-seg-btn:hover { color: var(--text-primary); }
.pv-seg-btn.active {
  background: var(--bg-surface);
  color: var(--color-primary);
  font-weight: 600;
  box-shadow: var(--shadow-sm);
}

.pv-font-size-group {
  display: inline-flex;
  align-items: center;
  gap: 2px;
  background: var(--bg-subtle);
  border-radius: var(--radius-sm);
  padding: 1px 4px;
}
.pv-fs-val { font-size: 11.5px; font-variant-numeric: tabular-nums; color: var(--text-secondary); padding: 0 3px; }

.pv-modified-hint {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  font-size: 12px;
  color: var(--color-warning);
  font-weight: 500;
}
.pulse-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--color-warning);
  animation: pulse 1.4s ease infinite;
}

/* 文本查找栏 */
.pv-search-bar {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 7px 18px;
  background: var(--bg-elevated);
  border-bottom: 1px solid var(--border-light);
  animation: modal-fade-enter 160ms var(--motion-ease);
}
.pv-search-input {
  flex: 1;
  max-width: 360px;
  height: 28px;
  border: 1px solid var(--control-border);
  border-radius: var(--radius-sm);
  padding: 0 10px;
  font-size: 13px;
  background: var(--control-bg);
  color: var(--text-primary);
  outline: none;
}
.pv-search-input:focus { border-color: var(--border-focus); box-shadow: var(--ring-focus); }
.pv-search-count { font-size: 12.5px; color: var(--text-tertiary); }
.pv-search-hit {
  background: color-mix(in srgb, var(--color-warning) 50%, transparent);
  color: inherit;
  border-radius: 2px;
  padding: 0 1px;
}

/* 文本与代码主区容器 */
.pv-text-container {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  gap: 0;
  padding: 12px 14px 14px;
  background: var(--bg-base);
}

.pv-code-box {
  flex: 1;
  min-height: 0;
  display: flex;
  background: var(--bg-surface);
  font-family: 'JetBrains Mono', 'Fira Code', 'Cascadia Code', 'SF Mono', Consolas, monospace;
  line-height: 24px;
  overflow: hidden;
  position: relative;
  border: 1px solid var(--border-light);
  border-bottom: 0;
  border-radius: var(--radius-sm) var(--radius-sm) 0 0;
}

/* 行号栏 */
.pv-gutter {
  width: 56px;
  flex-shrink: 0;
  background: var(--bg-subtle);
  border-right: 1px solid var(--border-light);
  padding: 10px 0;
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  padding-right: 12px;
  color: var(--text-tertiary);
  user-select: none;
  overflow: hidden;
  font-variant-numeric: tabular-nums;
}
.pv-gutter-line {
  height: 24px;
  line-height: 24px;
}

/* 查看与编辑内容区 */
.pv-code-view, .pv-editor-textarea {
  flex: 1;
  min-width: 0;
  height: 100%;
  padding: 10px 16px;
  margin: 0;
  border: none;
  background: transparent;
  color: var(--text-primary);
  font-family: inherit;
  line-height: 24px;
  outline: none;
  resize: none;
  overflow: auto;
  scrollbar-gutter: stable;
  user-select: text;
  white-space: pre;
  tab-size: 2;
}
.pv-code-view.wrap, .pv-editor-textarea.wrap {
  white-space: pre-wrap;
  word-break: break-all;
}
.pv-code-line {
  height: 24px;
  line-height: 24px;
}
.pv-code-view.wrap .pv-code-line {
  height: auto;
  min-height: 24px;
}

/* Markdown 文档渲染视图 */
.pv-markdown-view {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 24px 42px 48px;
  background: var(--bg-surface);
  color: var(--text-primary);
  font-size: 15px;
  line-height: 1.8;
  user-select: text;
  max-width: 920px;
  margin: 0 auto;
  width: 100%;
  border: 1px solid var(--border-light);
  border-radius: var(--radius-sm);
}
:deep(.md-h1) { font-size: 26px; font-weight: 750; margin: 18px 0 12px; border-bottom: 1px solid var(--border-light); padding-bottom: 8px; color: var(--text-primary); }
:deep(.md-h2) { font-size: 21px; font-weight: 700; margin: 20px 0 10px; border-bottom: 1px solid var(--border-lighter); padding-bottom: 6px; color: var(--text-primary); }
:deep(.md-h3) { font-size: 18px; font-weight: 650; margin: 16px 0 8px; color: var(--text-primary); }
:deep(.md-h4) { font-size: 16px; font-weight: 650; margin: 14px 0 6px; color: var(--text-primary); }
:deep(.md-h5), :deep(.md-h6) { font-size: 14.5px; font-weight: 600; margin: 12px 0 4px; color: var(--text-secondary); }
:deep(.md-inline-code) {
  padding: 2px 6px;
  border-radius: 4px;
  font-size: 13.5px;
  font-family: 'JetBrains Mono', 'Cascadia Code', monospace;
  background: var(--bg-subtle);
  color: var(--color-primary);
  border: 1px solid var(--border-lighter);
}
:deep(.md-code-block) {
  margin: 14px 0;
  border-radius: var(--radius-md);
  border: 1px solid var(--border-light);
  overflow: hidden;
  background: var(--bg-base);
}
:deep(.md-code-header) {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 5px 14px;
  background: var(--bg-subtle);
  border-bottom: 1px solid var(--border-lighter);
  font-size: 12px;
  color: var(--text-tertiary);
  font-weight: 600;
  text-transform: uppercase;
}
:deep(.md-code-block pre) {
  padding: 14px 16px;
  margin: 0;
  overflow-x: auto;
  font-family: 'JetBrains Mono', 'Cascadia Code', monospace;
  font-size: 13.5px;
  line-height: 1.65;
}
:deep(.md-quote) {
  border-left: 3.5px solid var(--color-primary);
  margin: 12px 0;
  padding: 6px 16px;
  color: var(--text-secondary);
  background: color-mix(in srgb, var(--color-primary) 6%, transparent);
  border-radius: 0 var(--radius-xs) var(--radius-xs) 0;
}
:deep(.md-link) { color: var(--color-primary); text-decoration: none; font-weight: 500; }
:deep(.md-link:hover) { text-decoration: underline; }
:deep(.md-hr) { border: none; border-top: 1px solid var(--border-light); margin: 24px 0; }
:deep(.md-list-item) { margin-left: 22px; list-style: disc; margin-bottom: 5px; }
:deep(.md-task-item) { list-style: none; display: flex; align-items: center; gap: 8px; margin-bottom: 5px; }
:deep(.md-task-box) { width: 15px; height: 15px; border: 1.5px solid var(--control-border); border-radius: 3px; display: inline-flex; align-items: center; justify-content: center; font-size: 11px; }
:deep(.md-task-box.checked) { background: var(--color-primary); border-color: var(--color-primary); color: #fff; }
:deep(.md-gap) { height: 12px; }

/* 底部状态栏 (Status bar) */
.pv-statusbar {
  display: flex;
  align-items: center;
  height: 28px;
  padding: 0 14px;
  background: var(--bg-subtle);
  border: 1px solid var(--border-light);
  border-top: 0;
  border-radius: 0 0 var(--radius-sm) var(--radius-sm);
  font-size: 12px;
  color: var(--text-tertiary);
  user-select: none;
  font-variant-numeric: tabular-nums;
  flex-shrink: 0;
}
.pv-sb-section { display: flex; align-items: center; gap: 14px; }
.pv-sb-item { display: inline-flex; align-items: center; gap: 4px; }
.pv-sb-pos { color: var(--text-secondary); font-weight: 500; }
.pv-sb-spacer { flex: 1; }
.pv-sb-lang { color: var(--color-primary); font-weight: 600; }

@media (max-width: 720px) {
  .pv-head-sub { overflow: hidden; text-overflow: ellipsis; }
  .pv-toolbar { padding: 6px 10px; }
  .pv-search-bar { padding: 7px 10px; }
  .pv-text-container { padding: 8px; }
  .pv-code-view, .pv-editor-textarea { padding: 10px 12px; }
  .pv-markdown-view { padding: 20px 22px 36px; }
  .pv-statusbar { padding: 0 10px; }
  .pv-sb-section { gap: 8px; }
}

/* 图片查看器：沉浸式黑场舞台 */
.pv-stage {
  flex: 1; min-height: 0; position: relative; overflow: hidden;
  background: #050507;
  user-select: none; touch-action: none; cursor: grab;
}
.pv-stage.grabbing { cursor: grabbing; }
.pv-img-layer {
  position: absolute; inset: 0;
  display: flex; align-items: center; justify-content: center;
  pointer-events: none;
}
.pv-img-layer:not(.leaving) { animation: pv-layer-in 260ms var(--motion-ease) both; }
.pv-img-layer.leaving { animation: pv-layer-out 240ms ease both; }
@keyframes pv-layer-in { from { opacity: 0; transform: scale(1.015); } }
@keyframes pv-layer-out { to { opacity: 0; transform: scale(.99); } }
.pv-img-layer img {
  display: block; max-width: none; max-height: none;
  transform-origin: center center;
  transition: transform 200ms var(--motion-ease);
  box-shadow: 0 10px 44px rgba(0, 0, 0, .55);
  border-radius: 2px;
}
.pv-stage.grabbing .pv-img-layer img { transition: none; }
.pv-stage-busy { position: absolute; top: 12px; right: 14px; color: rgba(255,255,255,.7); }

/* 顶部浮动栏：渐变遮罩，无实体条（播放器语言） */
.pv-topbar {
  position: absolute; top: 0; left: 0; right: 0; z-index: 3;
  display: flex; align-items: flex-start; justify-content: space-between; gap: 12px;
  padding: 12px 14px 32px;
  background: linear-gradient(rgba(0, 0, 0, .55), transparent);
  transition: opacity .25s ease, transform .25s ease;
}
.pv-topbar-meta { display: flex; align-items: center; gap: 8px; min-width: 0; color: rgba(255, 255, 255, .92); padding-top: 2px; }
.pv-topbar-name {
  font-size: 13.5px; font-weight: 600;
  overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
  text-shadow: 0 1px 4px rgba(0, 0, 0, .7);
}
.pv-topbar-sub { font-size: 11.5px; color: rgba(255, 255, 255, .62); flex: none; }
.pv-topbar-actions { display: flex; gap: 4px; flex: none; }

/* 左右翻页箭头 */
.pv-edge {
  position: absolute; top: 50%; transform: translateY(-50%);
  width: 38px; height: 62px;
  display: flex; align-items: center; justify-content: center;
  color: rgba(255, 255, 255, .95);
  background: rgba(14, 14, 18, .72);
  backdrop-filter: blur(12px); -webkit-backdrop-filter: blur(12px);
  border: 1px solid rgba(255, 255, 255, .16); border-radius: 10px;
  cursor: pointer; opacity: .3;
  transition: opacity .2s ease, background .18s ease, transform .18s var(--motion-spring);
}
.pv-edge.left { left: 14px; }
.pv-edge.right { right: 14px; }
.pv-edge:hover { opacity: 1; background: rgba(22, 22, 28, .9); }
.pv-edge:active { transform: translateY(-50%) scale(.93); }

/* 底部控制条：渐变遮罩条（无实体药丸），控件直接浮在渐变上 */
.pv-ctl {
  position: absolute; left: 0; right: 0; bottom: 0;
  display: flex; align-items: center; justify-content: center; gap: 5px;
  padding: 28px 16px 12px;
  background: linear-gradient(transparent, rgba(0, 0, 0, .62));
  transition: opacity .25s ease, transform .25s ease;
}
.pv-ctl-btn {
  width: 28px; height: 28px; flex: none;
  display: flex; align-items: center; justify-content: center;
  border-radius: 8px; color: rgba(255, 255, 255, .95); cursor: pointer;
  transition: background .16s ease, transform .18s var(--motion-spring), color .16s;
}
.pv-ctl-btn:hover:not(:disabled) { background: rgba(255, 255, 255, .16); }
.pv-ctl-btn:active:not(:disabled) { transform: scale(.88); }
.pv-ctl-btn:disabled { opacity: .32; cursor: default; }
.pv-ctl-btn.active { color: var(--color-primary); background: color-mix(in srgb, var(--color-primary) 20%, transparent); }
.pv-ctl-text { font-size: 17px; line-height: 1; padding-bottom: 2px; }
.pv-ctl-counter, .pv-ctl-zoom { font-size: 12px; color: rgba(255, 255, 255, .92); font-variant-numeric: tabular-nums; white-space: nowrap; }
.pv-ctl-zoom { cursor: pointer; min-width: 40px; text-align: center; border-radius: 6px; padding: 3px 4px; }
.pv-ctl-zoom:hover { background: rgba(255, 255, 255, .14); }
.pv-ctl-sep { width: 1px; height: 16px; background: rgba(255, 255, 255, .2); flex: none; }

/* 胶卷缩略图条（默认隐藏，控制条切换） */
.pv-filmstrip {
  position: absolute; left: 50%; bottom: 62px; transform: translateX(-50%);
  display: flex; gap: 6px; max-width: min(78%, 560px); overflow-x: auto;
  padding: 7px; border-radius: 12px;
  background: rgba(12, 12, 16, .92);
  backdrop-filter: blur(20px); -webkit-backdrop-filter: blur(20px);
  border: 1px solid rgba(255, 255, 255, .16);
  scrollbar-width: none;
  transition: opacity .25s ease;
}
.pv-filmstrip::-webkit-scrollbar { display: none; }
.pv-film-thumb {
  width: 56px; height: 40px; flex: none;
  border-radius: 7px; overflow: hidden; cursor: pointer;
  border: 1.5px solid transparent;
  display: flex; align-items: center; justify-content: center;
  background: rgba(255, 255, 255, .06); color: rgba(255, 255, 255, .5);
  transition: transform .18s var(--motion-spring), border-color .15s ease;
}
.pv-film-thumb:hover { transform: translateY(-2px); }
.pv-film-thumb.active { border-color: var(--color-primary); }
.pv-film-thumb img { width: 100%; height: 100%; object-fit: cover; }

/* 控件自动隐藏 */
.pv-stage.ui-hidden { cursor: none; }
.pv-stage.ui-hidden .pv-ctl,
.pv-stage.ui-hidden .pv-edge,
.pv-stage.ui-hidden .pv-topbar,
.pv-stage.ui-hidden .pv-filmstrip { opacity: 0; pointer-events: none; }
.pv-stage.ui-hidden .pv-ctl { transform: translateY(8px); }
.pv-stage.ui-hidden .pv-topbar { transform: translateY(-8px); }

/* 沉浸式：隐藏弹窗实体头栏 */
.modal.preview-modal.immersive .modal-head { display: none; }

/* PDF & Audio */
.pv-audio-container { flex: 1; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 18px; padding: 36px; }
.pv-audio-disc { width: 104px; height: 104px; border-radius: 50%; background: var(--bg-subtle); border: 2px solid var(--border-light); display: flex; align-items: center; justify-content: center; box-shadow: var(--shadow-md); }
.pv-audio-name { font-size: 16px; font-weight: 600; color: var(--text-primary); text-align: center; max-width: 520px; }
.pv-audio-player { width: min(480px, 90%); }
</style>
