<script setup>
// 高级预览弹窗：
// 1. 图片画廊（缩放/拖拽平移/90度旋转/翻页/胶卷条）
// 2. 文本与代码（行号/编辑/一键保存回传网盘）
// 3. 音频播放与 PDF 嵌入
import { ref, computed, watch, onMounted, onBeforeUnmount } from 'vue'
import { PreviewURL, openKindOf, formatBytes, saveCloudText } from '../api'
import Modal from './Modal.vue'
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
const isEditing = ref(false)
const saving = ref(false)
const loading = ref(true)
const error = ref('')

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

// ---------- 代码/文本行号 ----------
const textLines = computed(() => {
  const raw = isEditing.value ? editContent.value : text.value
  return raw.split('\n')
})

const isModified = computed(() => isEditing.value && editContent.value !== text.value)

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
    isEditing.value = false
    emit('toast', '已保存修改并上传到网盘', 'success')
    emit('saved')
  } catch (e) {
    emit('toast', '保存失败: ' + String(e), 'error')
  } finally {
    saving.value = false
  }
}

// ---------- 加载核心逻辑 ----------
let loadSeq = 0
async function loadPreview() {
  const seq = ++loadSeq
  loading.value = true
  error.value = ''
  url.value = ''
  resetImageTransform()
  try {
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
      if (buf.byteLength > 4 * 1024 * 1024) throw new Error('文本文件超过 4MB，不支持在线预览')
      text.value = decodeText(buf)
      editContent.value = text.value
    }
  } catch (e) {
    if (seq !== loadSeq) return
    error.value = String(e && e.message ? e.message : e)
  } finally {
    if (seq === loadSeq) loading.value = false
  }
}

watch(() => activeFile.value.file_id, loadPreview)

function onKey(e) {
  if (isEditing.value) return
  if (kind.value === 'image') {
    if (e.key === 'ArrowLeft') switchImage(-1)
    else if (e.key === 'ArrowRight') switchImage(1)
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

// decodeText tries UTF-8 first; if it produces many replacement characters
// (U+FFFD), it falls back to GBK which is common for legacy Chinese .srt/.ass
// subtitle files. This avoids garbled text without requiring a full charset
// detection library.
function decodeText(buf) {
  const u8 = new Uint8Array(buf)
  // quick BOM check
  if (u8.length >= 3 && u8[0] === 0xef && u8[1] === 0xbb && u8[2] === 0xbf) {
    return new TextDecoder('utf-8').decode(u8.subarray(3))
  }
  const utf8 = new TextDecoder('utf-8', { fatal: false }).decode(u8)
  // count replacement chars; if more than 1% are U+FFFD, assume non-UTF-8
  let replacements = 0
  for (let i = 0; i < utf8.length; i++) {
    if (utf8.charCodeAt(i) === 0xfffd) replacements++
  }
  if (replacements > utf8.length / 100) {
    try {
      return new TextDecoder('gbk').decode(u8)
    } catch {
      // GBK not supported (rare), fall through to UTF-8 result
    }
  }
  return utf8
}
</script>

<template>
  <Modal
    :title="activeFile.name"
    width=""
    @close="emit('close')"
    body-class="preview-body"
    class="preview-modal-host"
  >
    <!-- 顶部悬浮工具条（图片/文本专属工具） -->
    <div class="pv-toolbar">
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

      <template v-else-if="kind === 'text'">
        <button class="tbtn xs" :class="{ active: isEditing }" @click="isEditing = !isEditing">
          <UiIcon :name="isEditing ? 'play' : 'pencil'" :size="12" />
          <span>{{ isEditing ? '切换为预览' : '编辑内容' }}</span>
        </button>
        <button v-if="isEditing" class="btn primary sm" :disabled="!isModified || saving" @click="doSaveText">
          <span v-if="saving" class="spin spin-on-primary"></span>
          <span>{{ saving ? '保存中…' : '保存回传网盘' }}</span>
        </button>
        <span class="pv-counter">{{ textLines.length }} 行 · {{ formatBytes(activeFile.size) }}</span>
      </template>
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

      <!-- 4. 文本/代码 -->
      <div v-else class="pv-code-box">
        <div class="pv-gutter">
          <span v-for="n in textLines.length" :key="n">{{ n }}</span>
        </div>
        <textarea
          v-if="isEditing"
          class="pv-editor-textarea"
          v-model="editContent"
          spellcheck="false"
        ></textarea>
        <pre v-else class="pv-code-view">{{ text }}</pre>
      </div>
    </template>
  </Modal>
</template>

<style scoped>
:deep(.modal) {
  width: 900px;
  max-width: 95vw;
  height: 84vh;
  display: flex;
  flex-direction: column;
}
.preview-body {
  position: relative;
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  padding: 0;
  background: var(--bg-base);
  border-radius: 0 0 var(--radius-xl) var(--radius-xl);
  overflow: hidden;
}
.pv-toolbar {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 16px;
  background: var(--bg-surface);
  border-bottom: 1px solid var(--border-light);
  flex-shrink: 0;
}
.pv-counter { font-size: 12.5px; color: var(--text-tertiary); font-variant-numeric: tabular-nums; }
.pv-sep { width: 1px; height: 16px; background: var(--border-light); margin: 0 4px; }
.pv-zoom-text { font-size: 12px; color: var(--text-secondary); min-width: 36px; text-align: center; font-variant-numeric: tabular-nums; }
.pv-center { flex: 1; display: flex; flex-direction: column; align-items: center; justify-content: center; }

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
  bottom: 12px;
  left: 50%;
  transform: translateX(-50%);
  display: flex;
  gap: 6px;
  padding: 6px 10px;
  background: color-mix(in srgb, var(--bg-elevated) 85%, transparent);
  backdrop-filter: blur(8px);
  border: 1px solid var(--border-light);
  border-radius: var(--radius-full);
  box-shadow: var(--shadow-md);
  max-width: 80%;
  overflow-x: auto;
  scrollbar-width: none;
}
.pv-filmstrip::-webkit-scrollbar { display: none; }
.pv-film-thumb {
  width: 36px;
  height: 36px;
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
.pv-pdf-frame { width: 100%; height: 100%; border: none; background: #525659; }
.pv-audio-container { flex: 1; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 16px; padding: 32px; }
.pv-audio-disc { width: 96px; height: 96px; border-radius: 50%; background: var(--bg-subtle); border: 2px solid var(--border-light); display: flex; align-items: center; justify-content: center; box-shadow: var(--shadow-md); }
.pv-audio-name { font-size: 15px; font-weight: 600; color: var(--text-primary); text-align: center; max-width: 480px; }
.pv-audio-player { width: min(440px, 90%); }

/* 代码/文本编辑 */
.pv-code-box {
  flex: 1;
  min-height: 0;
  display: flex;
  background: var(--bg-surface);
  font-family: 'Cascadia Code', Consolas, Monaco, monospace;
  font-size: 13px;
  line-height: 1.6;
  overflow: hidden;
}
.pv-gutter {
  width: 46px;
  background: var(--bg-subtle);
  border-right: 1px solid var(--border-light);
  padding: 12px 0;
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  padding-right: 8px;
  color: var(--text-tertiary);
  user-select: none;
  overflow: hidden;
}
.pv-code-view, .pv-editor-textarea {
  flex: 1;
  min-width: 0;
  height: 100%;
  padding: 12px 16px;
  margin: 0;
  border: none;
  background: transparent;
  color: var(--text-primary);
  font-family: inherit;
  font-size: inherit;
  line-height: inherit;
  outline: none;
  resize: none;
  overflow: auto;
  user-select: text;
  white-space: pre;
  tab-size: 2;
}
</style>
