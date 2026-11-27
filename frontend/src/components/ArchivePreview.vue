<script setup>
import { computed, onBeforeUnmount, onMounted, ref, shallowRef } from 'vue'
import { PinFileSnapshot, PreviewURL, formatBytes, formatTime, iconOf } from '../api'
import { createPreviewLease } from '../player/preview-lease'
import { buildArchiveTree, extractZipEntry, readZipIndex } from '../archive-zip'
import UiIcon from './UiIcon.vue'

const props = defineProps({
  account: { type: Object, required: true },
  file: { type: Object, required: true },
})
const emit = defineEmits(['toast'])

const loading = ref(true)
const error = ref('')
const keyword = ref('')
const currentPath = ref([])
const extractingPath = ref('')
const truncated = ref(false)
const totalFiles = ref(0)
const totalDirs = ref(0)
const archiveRoot = shallowRef(null)
const allEntries = shallowRef([])
let sourceUrl = ''
let fullBuffer = null
let lease = null
let disposed = false
const controller = new AbortController()

// 虚拟滚动：大压缩包万级条目也能保持 60fps
const listEl = ref(null)
const ROW_HEIGHT = 40
const OVERSCAN = 8
const scrollTop = ref(0)
const viewportHeight = ref(480)
let listObserver = null
let scrollFrame = 0

function onListScroll() {
  if (!listEl.value) return
  if (typeof requestAnimationFrame !== 'function') {
    scrollTop.value = listEl.value.scrollTop
    return
  }
  if (scrollFrame) return
  scrollFrame = requestAnimationFrame(() => {
    scrollFrame = 0
    if (listEl.value) scrollTop.value = listEl.value.scrollTop
  })
}

const currentDir = computed(() => {
  let node = archiveRoot.value
  if (!node) return null
  for (const part of currentPath.value) {
    node = node.childMap?.get(part)
    if (!node) return archiveRoot.value
  }
  return node
})

const items = computed(() => {
  const kw = keyword.value.trim().toLowerCase()
  const list = kw
    ? allEntries.value.filter(entry => !entry.isDir && entry.path.toLowerCase().includes(kw))
    : (currentDir.value?.children || [])
  return [...list].sort((a, b) => {
    if (a.isDir !== b.isDir) return a.isDir ? -1 : 1
    return a.name.localeCompare(b.name, 'zh-CN', { numeric: true, sensitivity: 'base' })
  })
})

const visibleRange = computed(() => {
  const total = items.value.length
  const start = Math.max(0, Math.floor(scrollTop.value / ROW_HEIGHT) - OVERSCAN)
  const count = Math.ceil(viewportHeight.value / ROW_HEIGHT) + OVERSCAN * 2 + 2
  const end = Math.min(total, start + count)
  return { start, end }
})

const visibleItems = computed(() => items.value.slice(visibleRange.value.start, visibleRange.value.end))
const topSpacer = computed(() => visibleRange.value.start * ROW_HEIGHT)
const bottomSpacer = computed(() => Math.max(0, (items.value.length - visibleRange.value.end) * ROW_HEIGHT))

async function loadArchive() {
  lease?.dispose()
  const nextLease = createPreviewLease()
  lease = nextLease
  loading.value = true
  error.value = ''
  fullBuffer = null
  try {
    await PinFileSnapshot(props.account.user_id, props.account.drive_id, props.file)
    if (disposed) return
    const url = await PreviewURL(props.account.user_id, props.account.drive_id, props.file.file_id)
    nextLease.replace(url)
    if (disposed) return
    sourceUrl = url
    const index = await readZipIndex(url, props.file.size, controller.signal)
    if (disposed) return
    fullBuffer = index.fullBuffer
    truncated.value = index.truncated
    allEntries.value = index.entries
    const built = buildArchiveTree(index.entries)
    archiveRoot.value = built.root
    totalFiles.value = built.totalFiles
    totalDirs.value = built.totalDirs
  } catch (cause) {
    if (!disposed) error.value = String(cause?.message || cause)
  } finally {
    if (!disposed) loading.value = false
  }
}

function enterDir(node) {
  if (!node.isDir) return
  keyword.value = ''
  currentPath.value = node.path.split('/').filter(Boolean)
  scrollTop.value = 0
  if (listEl.value) listEl.value.scrollTop = 0
}

function jumpCrumb(index) {
  keyword.value = ''
  currentPath.value = index < 0 ? [] : currentPath.value.slice(0, index + 1)
  scrollTop.value = 0
  if (listEl.value) listEl.value.scrollTop = 0
}

async function downloadEntry(entry) {
  if (entry.isDir || extractingPath.value) return
  extractingPath.value = entry.path
  try {
    const blob = await extractZipEntry(sourceUrl, entry, fullBuffer, controller.signal)
    if (disposed) return
    const objectUrl = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = objectUrl
    link.download = entry.name
    document.body.appendChild(link)
    link.click()
    link.remove()
    setTimeout(() => URL.revokeObjectURL(objectUrl), 1500)
    emit('toast', `已提取：${entry.name}`, 'success')
  } catch (cause) {
    if (!disposed) emit('toast', String(cause?.message || cause), 'warn')
  } finally {
    if (extractingPath.value === entry.path) extractingPath.value = ''
  }
}

onMounted(() => {
  void loadArchive()
  if (listEl.value && typeof ResizeObserver !== 'undefined') {
    listObserver = new ResizeObserver(() => {
      if (listEl.value?.clientHeight) viewportHeight.value = listEl.value.clientHeight
    })
    listObserver.observe(listEl.value)
  }
})

onBeforeUnmount(() => {
  disposed = true
  controller.abort()
  if (scrollFrame) cancelAnimationFrame(scrollFrame)
  listObserver?.disconnect()
  lease?.dispose()
  fullBuffer = null
})
</script>

<template>
  <div class="archive-viewer">
    <div class="archive-toolbar">
      <div class="archive-summary">
        <strong>ZIP 预览</strong>
        <span v-if="!loading && !error">{{ totalFiles.toLocaleString() }} 个文件 · {{ totalDirs.toLocaleString() }} 个目录 · {{ formatBytes(file.size) }}</span>
        <span v-if="truncated" class="archive-note">（条目过多，仅展示前 50,000 项）</span>
      </div>
      <div class="archive-search-wrap">
        <UiIcon name="search" :size="13" />
        <input v-model="keyword" class="input archive-search" aria-label="搜索压缩包内文件" placeholder="搜索压缩包内文件…" />
      </div>
    </div>

    <nav v-if="!loading && !error && !keyword.trim()" class="pathbar" aria-label="压缩包路径">
      <button type="button" class="crumb" @click="jumpCrumb(-1)">根目录</button>
      <template v-for="(part, idx) in currentPath" :key="part + idx">
        <span class="crumb-sep" aria-hidden="true">/</span>
        <button type="button" class="crumb" @click="jumpCrumb(idx)">{{ part }}</button>
      </template>
    </nav>

    <div v-if="loading" class="archive-state" role="status">
      <span class="spin"></span>
      <span>正在按需读取压缩包目录（无需下载整包）…</span>
    </div>
    <div v-else-if="error" class="archive-state" role="alert">
      <UiIcon name="warning" :size="20" />
      <span>{{ error }}</span>
      <button type="button" class="btn sm" @click="loadArchive">重试</button>
    </div>
    <div v-else ref="listEl" class="archive-list" @scroll.passive="onListScroll">
      <div v-if="topSpacer" aria-hidden="true" :style="{ height: topSpacer + 'px' }"></div>
      <div
        v-for="entry in visibleItems"
        :key="entry.path"
        class="archive-row"
        :class="{ dir: entry.isDir }"
        @click="entry.isDir ? enterDir(entry) : null"
      >
        <span class="archive-ico" :class="'ft-' + iconOf(entry)">
          <UiIcon :name="iconOf(entry)" :size="17" />
        </span>
        <div class="archive-name-col">
          <span class="archive-name" :title="entry.path">{{ keyword.trim() ? entry.path : entry.name }}</span>
          <span v-if="entry.encrypted" class="badge warn">已加密</span>
        </div>
        <span class="archive-meta">{{ entry.isDir ? '目录' : formatBytes(entry.size) }}</span>
        <span class="archive-time">{{ entry.modifiedAt ? formatTime(entry.modifiedAt) : '' }}</span>
        <div class="archive-actions">
          <button
            v-if="!entry.isDir"
            type="button"
            class="btn sm"
            :disabled="entry.encrypted || !!extractingPath"
            @click.stop="downloadEntry(entry)"
          >{{ extractingPath === entry.path ? '提取中…' : '提取' }}</button>
          <UiIcon v-else name="chevron-right" :size="14" class="archive-chevron" />
        </div>
      </div>
      <div v-if="bottomSpacer" aria-hidden="true" :style="{ height: bottomSpacer + 'px' }"></div>
      <div v-if="!items.length" class="archive-state">
        <span>{{ keyword.trim() ? '压缩包内未找到匹配项' : '空目录' }}</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.archive-viewer { flex: 1; min-height: 0; display: flex; flex-direction: column; background: var(--bg-base); }
.archive-toolbar {
  min-height: 48px; padding: 7px 16px;
  display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap;
  border-bottom: 1px solid var(--border-light); background: var(--bg-surface);
}
.archive-summary { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; font-size: 12px; color: var(--text-secondary); }
.archive-summary strong { color: var(--text-primary); font-weight: 600; }
.archive-note { color: var(--color-warning); }
.archive-search-wrap {
  display: inline-flex; align-items: center; gap: 6px;
  padding: 0 8px; height: 30px; width: min(260px, 100%);
  border: 1px solid var(--control-border); border-radius: var(--radius-sm);
  background: var(--control-bg); color: var(--text-tertiary);
}
.archive-search-wrap:focus-within { border-color: var(--border-focus); box-shadow: var(--ring-focus); }
.archive-search { flex: 1; min-width: 0; height: 100%; border: 0; background: transparent; padding: 0; font-size: 12.5px; box-shadow: none; }
.archive-search:focus { box-shadow: none; }
.archive-state { flex: 1; display: flex; align-items: center; justify-content: center; gap: 10px; color: var(--text-secondary); font-size: 13px; padding: 24px; text-align: center; }
.archive-list { flex: 1; min-height: 0; overflow-y: auto; padding: 6px 10px 14px; scrollbar-gutter: stable; }
.archive-row {
  display: flex; align-items: center; gap: 10px; height: 40px; padding: 0 10px;
  border-radius: var(--radius-sm); font-size: 13px; color: var(--text-primary);
  transition: background-color var(--motion-fast) var(--motion-ease);
}
.archive-row.dir { cursor: pointer; }
.archive-row:hover { background: var(--bg-hover); }
.archive-ico { width: 20px; display: inline-flex; justify-content: center; flex-shrink: 0; }
.archive-name-col { flex: 1; min-width: 0; display: flex; align-items: center; gap: 6px; }
.archive-name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.archive-meta { width: 84px; text-align: right; font-size: 12px; color: var(--text-secondary); font-variant-numeric: tabular-nums; flex-shrink: 0; }
.archive-time { width: 130px; text-align: right; font-size: 12px; color: var(--text-tertiary); font-variant-numeric: tabular-nums; flex-shrink: 0; }
.archive-actions { width: 68px; display: flex; justify-content: flex-end; flex-shrink: 0; }
.archive-chevron { color: var(--text-tertiary); }
@media (max-width: 680px) { .archive-time { display: none; } }
</style>
