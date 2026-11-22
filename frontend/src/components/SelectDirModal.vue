<script setup>
// 选择网盘目录：在指定账号内浏览文件夹树并选中目标目录。
import { ref, shallowRef, computed, watch, nextTick, onBeforeUnmount, useId } from 'vue'
import { listDir, providerMetaOf, providerIconUrl, accountName, mkdir } from '../api'
import Modal from './Modal.vue'
import UiIcon from './UiIcon.vue'
import { sortedDirectories } from '../file-order'

const newFolderName = ref('')
const creatingFolder = ref(false)
async function createFolder() {
  const name = newFolderName.value.trim()
  if (!name || creatingFolder.value) return
  creatingFolder.value = true
  const account = props.account
  const version = accountVersion
  try {
    const parentId = crumbs.value[crumbs.value.length - 1].id
    const result = await mkdir(account.user_id, account.drive_id, parentId, name)
    if (version !== accountVersion) return
    if (result?.error) throw new Error(result.error)
    newFolderName.value = ''
    if (crumbs.value[crumbs.value.length - 1].id === parentId) await load(parentId)
    if (version !== accountVersion) return
    emit('toast', '文件夹已创建', 'success')
  } catch (e) { if (version === accountVersion) emit('toast', String(e), 'error') }
  finally { if (version === accountVersion) creatingFolder.value = false }
}

const props = defineProps({
  title: { type: String, default: '选择目录' },
  account: { type: Object, required: true },
  providers: { type: Array, default: () => [] },
})
const emit = defineEmits(['close', 'select', 'toast'])

const rootMeta = computed(() => providerMetaOf(props.account, props.providers))
const crumbs = ref([])
const dirs = shallowRef([])
const loading = ref(false)
const error = ref('')
const directoryList = ref(null)
const optionPrefix = `dir-option-${useId()}`
const ROW_HEIGHT = 32
const WINDOW_ROWS = 18
const scrollRow = ref(0)
const activeIndex = ref(0)
const windowStart = computed(() => Math.max(0, Math.min(scrollRow.value - 4, dirs.value.length - WINDOW_ROWS)))
const windowEnd = computed(() => Math.min(windowStart.value + WINDOW_ROWS, dirs.value.length))
const visibleDirs = computed(() => dirs.value.slice(windowStart.value, windowEnd.value))
const activeOption = computed(() => activeIndex.value >= windowStart.value && activeIndex.value < windowEnd.value ? `${optionPrefix}-${activeIndex.value}` : undefined)

let loadSeq = 0
let accountVersion = 0
let retryAnchor = ''
async function load(id, anchor = '') {
  const seq = ++loadSeq
  retryAnchor = anchor
  loading.value = true
  error.value = ''
  dirs.value = []
  activeIndex.value = 0
  scrollRow.value = 0
  if (directoryList.value) directoryList.value.scrollTop = 0
  try {
    const list = await listDir(props.account.user_id, props.account.drive_id, id)
    if (seq !== loadSeq) return // 过期响应丢弃
    dirs.value = sortedDirectories(list || [])
    if (anchor) {
      const index = dirs.value.findIndex(directory => directory.file_id === anchor)
      if (index >= 0) {
        activeIndex.value = index
        await nextTick()
        if (seq !== loadSeq) return
        revealActiveDirectory()
      }
    }
  } catch (e) {
    if (seq !== loadSeq) return
    error.value = String(e)
    dirs.value = []
  }
  if (seq === loadSeq) loading.value = false
}

function enter(d) {
  if (loading.value) return
  crumbs.value.push({ id: d.file_id, name: d.name })
  load(d.file_id)
}

function goUp() {
  if (crumbs.value.length > 1) jump(crumbs.value.length - 2)
}

function jump(index) {
  directoryList.value?.focus({ preventScroll: true })
  const anchor = crumbs.value[index + 1]?.id
  crumbs.value = crumbs.value.slice(0, index + 1)
  load(crumbs.value[index].id, anchor)
}

function onDirectoryKey(event) {
  if (event.target !== directoryList.value || event.isComposing || event.altKey || event.ctrlKey || event.metaKey) return
  if (event.key === 'ArrowLeft' && crumbs.value.length > 1) {
    event.preventDefault()
    goUp()
    return
  }
  const list = directoryList.value
  if (!list || loading.value || !dirs.value.length) return
  if (['Enter', 'ArrowRight', ' '].includes(event.key)) {
    event.preventDefault()
    enter(dirs.value[activeIndex.value])
    return
  }
  const rowsPerPage = Math.max(1, Math.floor(list.clientHeight / ROW_HEIGHT))
  const offsets = { ArrowDown: 1, ArrowUp: -1, PageDown: rowsPerPage, PageUp: -rowsPerPage }
  const last = dirs.value.length - 1
  if (event.key === 'Home') activeIndex.value = 0
  else if (event.key === 'End') activeIndex.value = last
  else if (Object.hasOwn(offsets, event.key)) activeIndex.value = Math.max(0, Math.min(last, activeIndex.value + offsets[event.key]))
  else return
  event.preventDefault()
  revealActiveDirectory()
}

function revealActiveDirectory() {
  const list = directoryList.value
  if (!list) return
  const top = activeIndex.value * ROW_HEIGHT
  if (top < list.scrollTop) list.scrollTop = top
  else if (top + ROW_HEIGHT > list.scrollTop + list.clientHeight - 12) list.scrollTop = top + ROW_HEIGHT - list.clientHeight + 12
  scrollRow.value = Math.floor(list.scrollTop / ROW_HEIGHT)
}

watch([() => props.account.user_id, () => props.account.drive_id, () => rootMeta.value.rootKey], () => {
  ++accountVersion
  creatingFolder.value = false
  newFolderName.value = ''
  const rootKey = rootMeta.value.rootKey || 'root'
  crumbs.value = [{ id: rootKey, name: rootMeta.value.rootTitle || '根目录' }]
  load(rootKey)
}, { immediate: true, flush: 'sync' })
onBeforeUnmount(() => { ++loadSeq; ++accountVersion })
</script>

<template>
  <Modal :title="title" width="480px" @close="emit('close')">
    <div class="account-inline" style="margin-bottom:12px"><img :src="providerIconUrl(providerMetaOf(account, providers))" alt="" />{{ accountName(account) }}</div>
    <div class="pathbar" style="border:none;padding:0 0 8px">
      <template v-for="(c, i) in crumbs" :key="c.id + i">
        <span v-if="i" class="crumb-sep">/</span>
        <button v-if="i < crumbs.length - 1" type="button" class="crumb directory-crumb" @click="jump(i)">{{ c.name }}</button>
        <span v-else class="crumb" aria-current="location">{{ c.name }}</span>
      </template>
    </div>
    <button v-if="crumbs.length > 1" class="tree-node directory-up" type="button" @click="goUp">
      <UiIcon name="up-level" :size="15" /><span class="tn-label">返回上级</span>
    </button>
    <div ref="directoryList" class="directory-list" :role="dirs.length ? 'listbox' : 'region'" aria-label="子文件夹" :aria-busy="loading" :aria-activedescendant="activeOption" tabindex="0" @keydown="onDirectoryKey" @scroll.passive="scrollRow = Math.floor($event.currentTarget.scrollTop / ROW_HEIGHT)">
      <div v-if="loading && !dirs.length" class="skeleton-list">
        <div v-for="i in 4" :key="i" class="skeleton-row" style="height:34px">
          <div class="skeleton skeleton-icon" style="width:16px;height:16px"></div>
          <div class="skeleton skeleton-name" :style="{ width: (40 + (i * 13) % 40) + '%' }"></div>
        </div>
      </div>
      <div v-else-if="error" class="empty" style="padding:40px">
        <span>{{ error }}</span>
        <button class="btn sm" @click="load(crumbs[crumbs.length - 1].id, retryAnchor)">重试</button>
      </div>
      <div v-else-if="!dirs.length" class="empty" style="padding:40px">暂无子文件夹</div>
      <div v-if="windowStart" aria-hidden="true" :style="{ height: windowStart * ROW_HEIGHT + 'px' }"></div>
      <button v-for="(d, offset) in visibleDirs" :id="`${optionPrefix}-${windowStart + offset}`" :key="d.file_id" class="tree-node directory-option" type="button" role="option" tabindex="-1" :style="{ height: ROW_HEIGHT + 'px' }" :aria-selected="activeIndex === windowStart + offset" :aria-posinset="windowStart + offset + 1" :aria-setsize="dirs.length" @mousedown.prevent @click="directoryList?.focus({ preventScroll: true }); enter(d)">
        <UiIcon name="folder" :size="15" /><span class="tn-label">{{ d.name }}</span>
        <span style="margin-left:auto;color:var(--text-tertiary)"><UiIcon name="chevron-right" :size="13" /></span>
      </button>
      <div v-if="windowEnd < dirs.length" aria-hidden="true" :style="{ height: (dirs.length - windowEnd) * ROW_HEIGHT + 'px' }"></div>
    </div>
    <template #actions>
      <div class="new-folder-row">
        <input v-model="newFolderName" class="input" placeholder="新建文件夹..." aria-label="新建文件夹名称" @keydown.enter="createFolder" :disabled="creatingFolder" />
        <button class="btn" :disabled="!newFolderName.trim() || creatingFolder" @click="createFolder"><UiIcon name="plus" :size="12" />新建</button>
      </div>
      <span style="flex:1"></span>
      <button class="btn" @click="emit('close')">取消</button>
      <button class="btn primary" :disabled="loading || !!error" @click="emit('select', { id: crumbs[crumbs.length - 1].id, name: crumbs[crumbs.length - 1].name })">选择当前目录</button>
    </template>
  </Modal>
</template>

<style scoped>
.directory-list { min-height: 180px; max-height: 300px; overflow-y: auto; overflow-anchor: none; border: 1px solid var(--border-lighter); border-radius: var(--radius-md); padding: 6px; }
.directory-list .directory-option { margin-bottom: 0; }
.directory-list:focus-visible .directory-option[aria-selected="true"] { background: var(--bg-hover); box-shadow: inset 0 0 0 1px var(--border-focus); }
.directory-up { color: var(--text-tertiary); }
.directory-crumb { border: 0; background: transparent; font: inherit; }
.new-folder-row { display: flex; gap: 6px; align-items: center; }
.new-folder-row .input { width: 160px; height: 28px; font-size: 12.5px; padding: 0 8px; }
.new-folder-row .btn { display: inline-flex; align-items: center; gap: 3px; white-space: nowrap; }
</style>
