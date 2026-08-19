<script setup>
// 高级预览弹窗：
// 1. 图片画廊（缩放/拖拽平移/90度旋转/翻页/胶卷条）
// 2. 文本与代码专业预览/编辑器（大屏自适应/最大化全屏/滚动同步/防折行排布/Markdown精美排版/Ctrl+S云端回传/状态栏）
// 3. 音频播放与 PDF 嵌入
import { ref, computed, watch, nextTick, onMounted, onBeforeUnmount } from 'vue'
import { PreviewURL, PinFileSnapshot, openKindOf, formatBytes, formatTime, saveCloudText, copyText, iconOf } from '../api'
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
const isMaximized = ref(false) // 窗口最大化/全屏状态

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

const zoom = ref(1)
const rotation = ref(0)
const pos = ref({ x: 0, y: 0 })
const isDragging = ref(false)
let dragStart = { x: 0, y: 0, posX: 0, posY: 0 }

function resetImageTransform() {
  zoom.value = 1
  rotation.value = 0
  pos.value = { x: 0, y: 0 }
}

function zoomBy(delta) {
  zoom.value = Math.min(5, Math.max(0.2, Number((zoom.value + delta).toFixed(2))))
}

function rotateBy(deg) {
  rotation.value = (rotation.value + deg + 360) % 360
}

function onWheel(e) {
  if (kind.value !== 'image') return
  e.preventDefault()
  const delta = e.deltaY < 0 ? 0.15 : -0.15
  zoomBy(delta)
}

function onPointerDown(e) {
  if (kind.value !== 'image') return
  isDragging.value = true
  dragStart = { x: e.clientX, y: e.clientY, posX: pos.value.x, posY: pos.value.y }
  const onMove = (ev) => {
    if (!isDragging.value) return
    pos.value = {
      x: dragStart.posX + (ev.clientX - dragStart.x),
      y: dragStart.posY + (ev.clientY - dragStart.y),
    }
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
let activeDragCleanup = null

function switchImage(step) {
  const total = imageList.value.length
  if (total <= 1) return
  const nextIdx = (currentImageIdx.value + step + total) % total
  activeFile.value = imageList.value[nextIdx]
}

function selectImage(img) {
  if (activeFile.value.file_id === img.file_id) return
  activeFile.value = img
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
    emit('toast', '已保存修改并上传覆盖到网盘', 'success')
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
  const seq = ++loadSeq
  loading.value = true
  error.value = ''
  url.value = ''
  resetImageTransform()
  try {
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
  } else if (kind.value === 'text') {
    if ((e.ctrlKey || e.metaKey) && (e.key === 'f' || e.code === 'KeyF')) {
      e.preventDefault()
      toggleSearch()
    }
  }
}

onMounted(() => {
  loadPreview()
  window.addEventListener('keydown', onKey)
})
onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKey)
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
    :dialog-class="'preview-modal ' + (isMaximized ? 'is-maximized' : '')"
    width=""
    @close="handleCloseRequest"
    body-class="preview-body"
  >
    <!-- 自定义高级弹窗头部 -->
    <template #head>
      <div class="pv-head-custom">
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

    <!-- 头部右侧操作扩展（最大化/全屏按钮） -->
    <template #head-extra>
      <button
        class="icon-btn"
        style="width:28px;height:28px"
        :title="isMaximized ? '还原窗口' : '最大化窗口'"
        @click="isMaximized = !isMaximized"
      >
        <UiIcon :name="isMaximized ? 'minimize' : 'maximize'" :size="14" />
      </button>
    </template>

    <!-- 顶部悬浮工具条 -->
    <div class="pv-toolbar">
      <!-- 1. 图片画廊工具 -->
      <template v-if="kind === 'image'">
        <button class="tbtn xs" :disabled="imageList.length <= 1" title="上一张 (←)" @click="switchImage(-1)">
          <UiIcon name="back" :size="12" />上一张
        </button>
        <span class="pv-counter">{{ currentImageIdx + 1 }} / {{ imageList.length }}</span>
        <button class="tbtn xs" :disabled="imageList.length <= 1" title="下一张 (→)" @click="switchImage(1)">
          下一张<UiIcon name="forward" :size="12" />
        </button>
        <span class="pv-sep"></span>
        <button class="btn-circle sm" title="放大 (滚轮向上)" @click="zoomBy(0.2)"><UiIcon name="plus" :size="13" /></button>
        <span class="pv-zoom-text">{{ Math.round(zoom * 100) }}%</span>
        <button class="btn-circle sm" title="缩小 (滚轮向下)" @click="zoomBy(-0.2)"><UiIcon name="close" :size="11" /></button>
        <button class="btn-circle sm" title="顺时针旋转 90°" @click="rotateBy(90)"><UiIcon name="refresh" :size="13" /></button>
        <button class="tbtn xs" title="复原" @click="resetImageTransform">重置</button>
      </template>

      <!-- 2. 文本与代码专业工具 -->
      <template v-else-if="kind === 'text'">
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
      <!-- 1. 图片画廊 -->
      <div
        v-if="kind === 'image'"
        class="pv-image-viewport"
        @wheel="onWheel"
        @pointerdown="onPointerDown"
      >
        <img
          :src="url"
          :alt="activeFile.name"
          class="pv-image-element"
          :class="{ dragging: isDragging }"
          :style="{
            transform: `translate(${pos.x}px, ${pos.y}px) scale(${zoom}) rotate(${rotation}deg)`,
          }"
          draggable="false"
          @dblclick="resetImageTransform"
        />

        <!-- 底部胶卷缩略图条 -->
        <div v-if="imageList.length > 1" class="pv-filmstrip" @pointerdown.stop>
          <div
            v-for="img in imageList"
            :key="img.file_id"
            class="pv-film-thumb"
            :class="{ active: img.file_id === activeFile.file_id }"
            :title="img.name"
            @click="selectImage(img)"
          >
            <img v-if="img.thumbnail" :src="img.thumbnail" alt="" />
            <UiIcon v-else name="image" :size="18" />
          </div>
        </div>
      </div>

      <!-- 2. PDF 嵌入 -->
      <iframe v-else-if="kind === 'pdf'" :src="url" class="pv-pdf-frame" title="PDF 预览"></iframe>

      <!-- 3. 音频播放 -->
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
.pv-counter { font-size: 12.5px; color: var(--text-tertiary); font-variant-numeric: tabular-nums; }
.pv-sep { width: 1px; height: 18px; background: var(--border-light); margin: 0 4px; }
.pv-zoom-text { font-size: 12px; color: var(--text-secondary); min-width: 36px; text-align: center; font-variant-numeric: tabular-nums; }
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

/* 图片画廊 */
.pv-image-viewport {
  flex: 1;
  min-height: 0;
  position: relative;
  overflow: hidden;
  display: flex;
  align-items: center;
  justify-content: center;
  background: radial-gradient(circle, var(--bg-surface) 10%, var(--bg-base) 90%);
  user-select: none;
  touch-action: none;
  cursor: grab;
}
.pv-image-viewport:active { cursor: grabbing; }
.pv-image-element {
  max-width: 100%;
  max-height: 100%;
  object-fit: contain;
  border-radius: var(--radius-xs);
  transition: transform 60ms linear;
  box-shadow: var(--shadow-modal);
  pointer-events: none;
}
.pv-image-element.dragging { transition: none; }

/* 底部胶卷 */
.pv-filmstrip {
  position: absolute;
  bottom: 14px;
  left: 50%;
  transform: translateX(-50%);
  display: flex;
  gap: 8px;
  padding: 6px 12px;
  background: var(--bg-elevated);
  border: 1px solid var(--border-light);
  border-radius: var(--radius-full);
  box-shadow: var(--shadow-md);
  max-width: 80%;
  overflow-x: auto;
  scrollbar-width: none;
}
.pv-filmstrip::-webkit-scrollbar { display: none; }
.pv-film-thumb {
  width: 38px;
  height: 38px;
  border-radius: var(--radius-sm);
  overflow: hidden;
  cursor: pointer;
  border: 1.5px solid transparent;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--bg-subtle);
  flex-shrink: 0;
  transition: border-color var(--motion-fast) var(--motion-ease), transform var(--motion-fast) var(--motion-spring);
}
.pv-film-thumb:hover { transform: scale(1.1); }
.pv-film-thumb.active { border-color: var(--color-primary); box-shadow: 0 0 0 2px color-mix(in srgb, var(--color-primary) 30%, transparent); }
.pv-film-thumb img { width: 100%; height: 100%; object-fit: cover; }

/* PDF & Audio */
.pv-pdf-frame { width: 100%; height: 100%; border: none; background: var(--bg-base); }
.pv-audio-container { flex: 1; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 18px; padding: 36px; }
.pv-audio-disc { width: 104px; height: 104px; border-radius: 50%; background: var(--bg-subtle); border: 2px solid var(--border-light); display: flex; align-items: center; justify-content: center; box-shadow: var(--shadow-md); }
.pv-audio-name { font-size: 16px; font-weight: 600; color: var(--text-primary); text-align: center; max-width: 520px; }
.pv-audio-player { width: min(480px, 90%); }
</style>
