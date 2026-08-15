<script setup>
import { ref, computed, watch, onMounted, onBeforeUnmount } from 'vue'
import {
  ListDownloads, ListUploads,
  PauseDownload, ResumeDownload, CancelDownload, ClearDownloads,
  RemoveDownload, PrioritizeDownload, OpenFile,
  CancelUpload, ClearUploads, DownloadURL,
  ListOfflineTasks, OfflineDownload,
  EventsOn, RevealInFolder,
  accountName, providerIconUrl, providerMetaOf, capsOf,
  formatBytes, formatSpeed, formatTime, iconOf, copyText
} from '../api'
import Modal from '../components/Modal.vue'
import ContextMenu from '../components/ContextMenu.vue'
import UiIcon from '../components/UiIcon.vue'
import UiSelect from '../components/UiSelect.vue'

const props = defineProps({
  accounts: { type: Array, default: () => [] },
  providers: { type: Array, default: () => [] }
})
const emit = defineEmits(['toast'])

// ---------- 菜单 / 账号筛选 ----------
const menu = ref('downloading')
const filterUser = ref('')

// 支持云离线的账号（按能力集判定，不再硬编码 pikpak）
const hasOffline = computed(() => offlineAccounts.value.length > 0)
const offlineAccounts = computed(() => props.accounts.filter((a) => capsOf(a, props.providers).offlineDownload))

const menus = computed(() => {
  const list = [
    { key: 'downloading', icon: 'download', label: '正在下载', cnt: activeDownloads.value.length },
    { key: 'downloaded', icon: 'check', label: '已下载', cnt: doneDownloads.value.length },
    { key: 'uploading', icon: 'upload', label: '正在上传', cnt: activeUploads.value.length },
    { key: 'uploaded', icon: 'check', label: '已上传', cnt: doneUploads.value.length }
  ]
  if (hasOffline.value) list.push({ key: 'offline', icon: 'cloud-down', label: '云离线', cnt: 0 })
  list.push({ key: 'migrate', icon: 'migrate', label: '迁移', cnt: migrateJobs.value.length })
  return list
})

watch(hasOffline, (v) => { if (!v && menu.value === 'offline') menu.value = 'downloading' })
watch(menu, () => { selectedIds.value = new Set(); ctx.show = false })

function accIcon(acc) { return providerIconUrl(providerMetaOf(acc, props.providers)) }
function accLabel(acc) { return accountName(acc) }

// ---------- 下载 / 上传数据 ----------
const downloads = ref([])
const uploads = ref([])
const loading = ref(true)

async function refresh() {
  try {
    const [d, u] = await Promise.all([ListDownloads(), ListUploads()])
    downloads.value = d || []
    uploads.value = u || []
  } catch { /* 静默 */ } finally { loading.value = false }
}

function onTransferEvent(ev) {
  if (!ev || !ev.task) { scheduleRefresh(); return }
  const t = ev.task
  if (ev.kind === 'download') {
    const list = downloads.value
    const idx = list.findIndex((x) => x.id === t.id)
    if (idx >= 0) {
      // 局部增量合并更新，避免大数组整体反序列化和重新挂载
      Object.assign(list[idx], t)
    } else {
      scheduleRefresh()
    }
  } else if (ev.kind === 'upload') {
    scheduleRefresh()
  }
}

let refreshTimer = null
function scheduleRefresh() {
  clearTimeout(refreshTimer)
  refreshTimer = setTimeout(refresh, 500)
}

// 快速筛选（任务名/链接，120ms 防抖）
const taskFilterRaw = ref('')
const taskFilter = ref('')
let taskFilterTimer = null
watch(taskFilterRaw, (v) => {
  clearTimeout(taskFilterTimer)
  taskFilterTimer = setTimeout(() => { taskFilter.value = v }, 120)
})
const byKw = (list, nameFn) => {
  const kw = taskFilter.value.trim().toLowerCase()
  if (!kw) return list
  return list.filter((t) => (nameFn(t) || '').toLowerCase().includes(kw) || String(t.url || '').toLowerCase().includes(kw))
}
const byAccount = (list) => filterUser.value ? list.filter((t) => t.user_id === filterUser.value) : list
const activeDownloads = computed(() => byKw(byAccount(downloads.value.filter((t) => t.status !== 'completed')), (t) => t.name))
const doneDownloads = computed(() => byKw(byAccount(downloads.value.filter((t) => t.status === 'completed')), (t) => t.name))
const activeUploads = computed(() => uploads.value.filter((t) => t.Upload && !t.Upload.IsCompleted))
const doneUploads = computed(() => uploads.value.filter((t) => t.Upload && t.Upload.IsCompleted))

// 全选（正在下载列表）
const allActiveSelected = computed(() => activeDownloads.value.length > 0 && activeDownloads.value.every((t) => selectedIds.value.has(t.id)))
function toggleSelectAllActive() {
  selectedIds.value = allActiveSelected.value ? new Set() : new Set(activeDownloads.value.map((t) => t.id))
}

const downStatusText = (s) => ({ queued: '排队中', downloading: '下载中', paused: '已暂停', completed: '已完成', failed: '失败', canceled: '已取消' }[s] || s)
const downStatusBadge = (s) => ({ downloading: 'primary', queued: 'primary', completed: 'success', failed: 'error', canceled: 'warn', paused: 'warn' }[s] || '')

function upStatus(t) {
  const u = t.Upload || {}
  if (u.IsCompleted) return '已完成'
  if (u.IsFailed) return '失败'
  if (u.IsStop) return '已停止'
  if (u.IsDowning) return '上传中'
  return '排队中'
}
const upStatusBadge = (t) => ({ 已完成: 'success', 失败: 'error', 已停止: 'warn' }[upStatus(t)] || 'primary')
const upName = (t) => (t.Info && t.Info.name) || ((t.Info && t.Info.localFilePath) || '').split(/[\\/]/).pop() || t.UploadID
const upSize = (t) => (t.Info && (t.Info.sizeStr || formatBytes(t.Info.size))) || ''
const upSpeed = (t) => (t.Upload && (t.Upload.DownSpeedStr || formatSpeed(t.Upload.DownSpeed || 0))) || ''
const upErr = (t) => (t.Upload && t.Upload.failedMessage) || ''

// ---------- 选择 / 批量 ----------
const selectedIds = ref(new Set())
function toggleSelect(id) {
  const s = new Set(selectedIds.value)
  s.has(id) ? s.delete(id) : s.add(id)
  selectedIds.value = s
}
function onItemClick(e, id) {
  if (e.ctrlKey || e.metaKey) toggleSelect(id)
  else selectedIds.value = new Set([id])
}
const selectedTasks = computed(() => activeDownloads.value.filter((t) => selectedIds.value.has(t.id)))

async function runAction(fn, okMsg) {
  try { await fn(); scheduleRefresh(); if (okMsg) emit('toast', okMsg, 'success') }
  catch (e) { emit('toast', String(e && e.message ? e.message : e), 'error') }
}

const pauseTask = (t) => runAction(() => PauseDownload(t.id))
const resumeTask = (t) => runAction(() => ResumeDownload(t.id))
const cancelTask = (t) => runAction(() => CancelDownload(t.id))
// 优先下载：暂停其他任务，把带宽让给该任务
const prioritizeTask = (t) => runAction(() => PrioritizeDownload(t.id), '已优先下载该任务（其他已暂停）')
// 删除记录：硬删除，立即从列表移除
const removeTask = (t) => runAction(() => RemoveDownload(t.id), '已删除')
// 用系统默认程序打开文件
const openFile = (t) => { if (t.localPath) OpenFile(t.localPath) }
// 在系统文件管理器中打开所在目录（选中文件）
const reveal = (t) => { if (t.localPath) RevealInFolder(t.localPath) }

let batchBusy = false
function startAll() { if (batchBusy) return; batchBusy = true; activeDownloads.value.filter((t) => t.status === 'paused' || t.status === 'failed').forEach((t) => ResumeDownload(t.id).catch(() => {})); scheduleRefresh(); setTimeout(() => { batchBusy = false }, 600) }
function pauseAll() { if (batchBusy) return; batchBusy = true; activeDownloads.value.filter((t) => t.status === 'downloading' || t.status === 'queued').forEach((t) => PauseDownload(t.id).catch(() => {})); scheduleRefresh(); setTimeout(() => { batchBusy = false }, 600) }
function batch(fn) { if (batchBusy) return; batchBusy = true; selectedTasks.value.forEach((t) => fn(t).catch(() => {})); selectedIds.value = new Set(); scheduleRefresh(); setTimeout(() => { batchBusy = false }, 600) }

const copiedLinkMap = ref({})
async function copyLink(t) {
  if (!t.url) return
  const ok = await copyText(t.url)
  if (ok && t.id) {
    copiedLinkMap.value[t.id] = true
    setTimeout(() => {
      const m = { ...copiedLinkMap.value }
      delete m[t.id]
      copiedLinkMap.value = m
    }, 1600)
  }
  emit('toast', ok ? '链接已复制' : '复制失败', ok ? 'success' : 'error')
}

// ---------- 右键菜单 ----------
const ctx = ref({ show: false, x: 0, y: 0, task: null })
function onCtx(e, t) {
  ctx.value = { show: true, x: e.clientX, y: e.clientY, task: t }
}
const ctxItems = computed(() => {
  const t = ctx.value.task
  if (!t) return []
  const targets = selectedIds.value.has(t.id) && selectedTasks.value.length > 1 ? selectedTasks.value : [t]
  const items = []
  if (t.status === 'completed') {
    // 已完成：打开文件 / 打开文件夹 / 复制链接 / 删除记录
    if (t.localPath) items.push({ icon: 'play', label: '打开文件', action: 'open' })
    if (t.localPath) items.push({ icon: 'folder', label: '打开所在目录', action: 'reveal' })
    items.push({ icon: 'link', label: '复制链接', disabled: !t.url, action: 'copy' })
    items.push({ sep: true })
    items.push({ icon: 'trash', label: '删除记录', danger: true, action: 'remove' })
    return items
  }
  // 进行中：继续 / 暂停 / 优先下载 / 取消
  if (targets.some((x) => x.status === 'paused' || x.status === 'failed'))
    items.push({ icon: 'play', label: targets.length > 1 ? `继续 (${targets.length})` : '继续', action: 'resume' })
  if (targets.some((x) => x.status === 'downloading' || x.status === 'queued'))
    items.push({ icon: 'pause', label: '暂停', action: 'pause' })
  if (targets.some((x) => x.status !== 'completed'))
    items.push({ icon: 'priority', label: '优先下载', action: 'prioritize' })
  items.push({ icon: 'x-circle', label: '取消', danger: true, action: 'cancel' })
  items.push({ sep: true })
  if (t.localPath) items.push({ icon: 'folder', label: '打开所在目录', action: 'reveal' })
  items.push({ icon: 'link', label: '复制链接', disabled: !t.url, action: 'copy' })
  items.push({ icon: 'info', label: '任务详情', action: 'detail' })
  return items
})

// ---------- 任务详情 ----------
const detailTask = ref(null)
const statusText = (s) => ({ queued: '排队中', downloading: '下载中', paused: '已暂停', completed: '已完成', failed: '失败', canceled: '已取消', uploading: '上传中' }[s] || s)

function onCtxSelect(action) {
  const t = ctx.value.task
  if (!t) return
  const targets = selectedIds.value.has(t.id) && selectedTasks.value.length > 1 ? selectedTasks.value : [t]
  if (action === 'resume') targets.filter((x) => x.status === 'paused' || x.status === 'failed').forEach((x) => ResumeDownload(x.id).catch(() => {}))
  else if (action === 'pause') targets.forEach((x) => PauseDownload(x.id).catch(() => {}))
  else if (action === 'prioritize') targets.forEach((x) => PrioritizeDownload(x.id).catch(() => {}))
  else if (action === 'cancel') targets.forEach((x) => CancelDownload(x.id).catch(() => {}))
  else if (action === 'remove') RemoveDownload(t.id).catch(() => {})
  else if (action === 'open') openFile(t)
  else if (action === 'reveal') reveal(t)
  else if (action === 'copy') copyLink(t)
  else if (action === 'detail') detailTask.value = t
  if (action !== 'copy' && action !== 'detail' && action !== 'open' && action !== 'reveal') scheduleRefresh()
}

// ---------- 新建下载 ----------
const dlModal = ref(false)
const dlLinks = ref('')
const dlName = ref('')
const dlAdvanced = ref(false)
const dlHeaders = ref({ ua: '', referer: '', cookie: '' })
function openDlModal() {
  dlLinks.value = ''; dlName.value = ''; dlAdvanced.value = false
  dlHeaders.value = { ua: '', referer: '', cookie: '' }
  dlModal.value = true
}
const dlUrlList = computed(() => dlLinks.value.split('\n').map((s) => s.trim()).filter((s) => /^https?:\/\//i.test(s)))
async function submitDownload() {
  const urls = dlUrlList.value
  if (!urls.length) { emit('toast', '请输入有效的 http/https 链接', 'error'); return }
  dlModal.value = false
  const headers = {}
  if (dlHeaders.value.ua.trim()) headers['User-Agent'] = dlHeaders.value.ua.trim()
  if (dlHeaders.value.referer.trim()) headers['Referer'] = dlHeaders.value.referer.trim()
  if (dlHeaders.value.cookie.trim()) headers['Cookie'] = dlHeaders.value.cookie.trim()
  for (const url of urls) {
    const name = urls.length === 1 ? dlName.value.trim() : ''
    try { await DownloadURL(name, url, headers) } catch (e) { emit('toast', String(e && e.message ? e.message : e), 'error') }
  }
  emit('toast', `已添加 ${urls.length} 个下载任务`, 'success')
  scheduleRefresh()
}

// ---------- 云离线 ----------
const offlineUser = ref('')
const offlineTasks = ref([])
const offlineLoading = ref(false)
const offModal = ref(false)
const offLinks = ref('')
let offlineTimer = null

watch(offlineAccounts, (list) => {
  if (!offlineUser.value && list.length) offlineUser.value = list[0].user_id
}, { immediate: true })

// 账号被移除后，筛选与离线选中项失效清理
watch(() => props.accounts, (list) => {
  if (filterUser.value && !list.some((a) => a.user_id === filterUser.value)) filterUser.value = ''
  if (offlineUser.value && !list.some((a) => a.user_id === offlineUser.value)) offlineUser.value = ''
}, { deep: true })

const offlineDriveId = computed(() => {
  const acc = offlineAccounts.value.find((a) => a.user_id === offlineUser.value)
  return acc ? acc.drive_id : ''
})

async function refreshOffline() {
  if (!offlineUser.value) { offlineTasks.value = []; return }
  offlineLoading.value = true
  try { offlineTasks.value = (await ListOfflineTasks(offlineUser.value)) || [] }
  catch { /* 静默 */ } finally { offlineLoading.value = false }
}
watch(offlineUser, refreshOffline)
watch(menu, (m) => { if (m === 'offline') refreshOffline() })

const offUrlList = computed(() => offLinks.value.split('\n').map((s) => s.trim()).filter((s) => /^https?:\/\//i.test(s) || /^magnet:/i.test(s) || /^ed2k:/i.test(s)))
async function submitOffline() {
  const urls = offUrlList.value
  if (!urls.length) { emit('toast', '请输入有效的链接', 'error'); return }
  offModal.value = false
  let ok = 0
  for (const url of urls) {
    try { await OfflineDownload(offlineUser.value, offlineDriveId.value, url, ''); ok++ }
    catch (e) { emit('toast', String(e && e.message ? e.message : e), 'error') }
  }
  if (ok) emit('toast', `已提交 ${ok} 个离线任务`, 'success')
  offLinks.value = ''
  refreshOffline()
}
const offProgress = (t) => Math.max(0, Math.min(100, Math.round(t.progress || 0)))

// ---------- 迁移 ----------
const migrateJobs = ref([])
function onMigrate(job) {
  if (!job || !job.id) return
  const i = migrateJobs.value.findIndex((j) => j.id === job.id)
  if (i >= 0) migrateJobs.value.splice(i, 1, job)
  else migrateJobs.value.unshift(job)
}
const migName = (uid) => {
  const acc = props.accounts.find((a) => a.user_id === uid)
  return acc ? accountName(acc) : uid
}
const migBadge = (s) => ({ completed: 'success', failed: 'error', running: 'primary' }[s] || 'warn')
const migStatusText = (s) => ({ pending: '等待中', running: '迁移中', completed: '已完成', failed: '失败', canceled: '已取消' }[s] || s)
const migProgress = (j) => j.total ? Math.round(((j.processed || 0) / j.total) * 100) : 0

// ---------- 生命周期 ----------
let pollTimer = null
const offFns = []
onMounted(() => {
  refresh()
  const off1 = EventsOn('transfer:event', onTransferEvent)
  const off2 = EventsOn('migrate:progress', onMigrate)
  if (typeof off1 === 'function') offFns.push(off1)
  if (typeof off2 === 'function') offFns.push(off2)
  pollTimer = setInterval(() => { if (!document.hidden) refresh() }, 2000)
  offlineTimer = setInterval(() => { if (!document.hidden && menu.value === 'offline') refreshOffline() }, 8000)
})
onBeforeUnmount(() => {
  clearTimeout(refreshTimer)
  clearInterval(pollTimer)
  clearInterval(offlineTimer)
  offFns.forEach((fn) => { try { fn() } catch { /* 忽略 */ } })
})
</script>

<template>
  <div class="page">
    <div class="down-layout">
      <!-- 左侧边栏 -->
      <aside class="down-side">
        <div
          v-for="m in menus" :key="m.key"
          class="down-menu-item" :class="{ active: menu === m.key }"
          @click="menu = m.key"
        >
          <span style="width:15px;display:inline-flex"><UiIcon :name="m.icon" :size="15" /></span><span>{{ m.label }}</span>
          <span v-if="m.cnt" class="cnt">{{ m.cnt }}</span>
        </div>

        <div class="side-label">账号筛选</div>
        <div class="down-filter" :class="{ active: !filterUser }" @click="filterUser = ''">
          <span style="width:15px;display:inline-flex"><UiIcon name="globe" :size="15" /></span>
          <span class="df-name">全部账号</span>
        </div>
        <div
          v-for="acc in accounts" :key="acc.user_id"
          class="down-filter" :class="{ active: filterUser === acc.user_id }"
          @click="filterUser = acc.user_id"
        >
          <img v-if="accIcon(acc)" :src="accIcon(acc)" alt="" />
          <span v-else style="width:15px;display:inline-flex"><UiIcon name="drive" :size="15" /></span>
          <span class="df-name">{{ accLabel(acc) }}</span>
        </div>
      </aside>

      <!-- 右侧内容区 -->
      <section class="down-content">
        <!-- 正在下载 -->
        <template v-if="menu === 'downloading'">
          <div class="toppanbtns">
            <div class="toppanbtn">
              <button class="tbtn primary" @click="openDlModal"><UiIcon name="plus" :size="14" />新建下载</button>
            </div>
            <div class="toppanbtn" v-if="!selectedIds.size">
              <button class="tbtn" @click="startAll"><UiIcon name="play" :size="14" />开始全部</button>
              <button class="tbtn" @click="pauseAll"><UiIcon name="pause" :size="14" />暂停全部</button>
              <button class="tbtn danger" @click="runAction(() => ClearDownloads(), '已清除')"><UiIcon name="trash" :size="14" />清除已完成</button>
            </div>
            <div class="toppanbtn" v-else>
              <button class="tbtn" @click="batch((t) => ResumeDownload(t.id))"><UiIcon name="play" :size="14" />继续</button>
              <button class="tbtn" @click="batch((t) => PauseDownload(t.id))"><UiIcon name="pause" :size="14" />暂停</button>
              <button class="tbtn danger" @click="batch((t) => CancelDownload(t.id))"><UiIcon name="x-circle" :size="14" />取消</button>
              <button class="tbtn" @click="selectedIds = new Set()">清除选择</button>
            </div>
            <div class="toolbar-spacer"></div>
            <div class="toppanbtn">
              <span class="search-quick-wrap">
                <span class="sq-icon"><UiIcon name="search" :size="13" /></span>
                <input class="search-quick" v-model="taskFilterRaw" placeholder="快速筛选" />
                <button v-if="taskFilterRaw" class="sq-clear" title="清空筛选" @click="taskFilterRaw = ''"><UiIcon name="close" :size="11" /></button>
              </span>
            </div>
          </div>

          <div class="toppanarea">
            <div class="sel-all-wrap">
              <button class="btn-circle" :class="{ on: allActiveSelected }" title="全选" @click="toggleSelectAllActive">
                <UiIcon :name="allActiveSelected ? 'x-circle' : 'check'" :size="15" />
              </button>
            </div>
            <div class="selectInfo">{{ selectedIds.size ? `已选 ${selectedIds.size} 项 / ` : '' }}共 {{ activeDownloads.length }} 项</div>
            <div class="toolbar-spacer"></div>
            <div class="cell" style="width:88px">大小</div>
            <div class="cell" style="width:212px">进度</div>
            <div class="cell" style="width:86px">速度</div>
            <div class="cell" style="width:80px">操作</div>
          </div>

          <div class="file-list">
            <template v-if="loading && !activeDownloads.length">
              <div v-for="i in 5" :key="i" class="skeleton-row">
                <div class="skeleton skeleton-icon"></div>
                <div class="skeleton skeleton-name" :style="{ width: (45 + (i * 9) % 30) + '%' }"></div>
                <div class="skeleton skeleton-size"></div>
                <div class="skeleton skeleton-date"></div>
              </div>
            </template>
            <template v-else>
              <div
                v-for="t in activeDownloads" :key="t.id"
                class="taskrow" :class="{ selected: selectedIds.has(t.id) }"
                @click="onItemClick($event, t.id)"
                @contextmenu.prevent="onCtx($event, t)"
              >
                <div class="rangselect">
                  <button class="btn-circle" :class="{ on: selectedIds.has(t.id) }" tabindex="-1" @click.stop="toggleSelect(t.id)">
                    <UiIcon :name="selectedIds.has(t.id) ? 'check' : 'plus'" :size="13" />
                  </button>
                </div>
                <div class="fileicon" :class="'ft-' + iconOf({ name: t.name })"><UiIcon :name="iconOf({ name: t.name })" :size="20" /></div>
                <div class="filename">
                  <div :title="t.localPath || t.url || ''">{{ t.name || t.url || t.id }}</div>
                  <div class="fsub">{{ t.localPath || t.url || '' }}</div>
                </div>
                <div class="filesize">{{ formatBytes(t.size) }}</div>
                <div class="downprogress">
                  <div class="transfering-state">
                    <p class="text-state"><span class="badge" :class="downStatusBadge(t.status)">{{ downStatusText(t.status) }}</span><span>{{ t.progress || 0 }}%</span></p>
                    <div class="progress-total">
                      <div :class="t.status === 'downloading' || t.status === 'queued' ? 'progress-current active' : t.status === 'completed' ? 'progress-current succeed' : t.status === 'failed' ? 'progress-current error' : 'progress-current'" :style="{ width: (t.progress || 0) + '%' }"></div>
                    </div>
                    <p v-if="t.status === 'failed' && t.error" class="text-error" :title="t.error">{{ t.error }}</p>
                  </div>
                </div>
                <div class="downspeed">{{ t.status === 'downloading' ? formatSpeed(t.speed || 0) : '' }}</div>
                <div class="tactions" @click.stop>
                  <button v-if="t.status !== 'completed'" class="btn-circle" title="优先下载（暂停其他任务）" @click="prioritizeTask(t)"><UiIcon name="priority" :size="14" /></button>
                  <button v-if="t.status === 'downloading' || t.status === 'queued'" class="btn-circle" title="暂停" @click="pauseTask(t)"><UiIcon name="pause" :size="14" /></button>
                  <button v-else-if="t.status === 'paused' || t.status === 'failed'" class="btn-circle" :title="t.status === 'failed' ? '重试' : '继续'" @click="resumeTask(t)"><UiIcon name="play" :size="14" /></button>
                  <button v-if="t.localPath" class="btn-circle" title="打开所在目录" @click="reveal(t)"><UiIcon name="folder" :size="14" /></button>
                  <button class="btn-circle" title="取消" style="color:var(--color-error)" @click="cancelTask(t)"><UiIcon name="x-circle" :size="14" /></button>
                </div>
              </div>
              <div v-if="!activeDownloads.length" class="workspace-empty-state">
                <UiIcon name="download" :size="36" style="opacity:.4" />
                <span class="wes-title">暂无下载任务</span>
              </div>
            </template>
          </div>
        </template>

        <!-- 已下载 -->
        <template v-else-if="menu === 'downloaded'">
          <div class="toppanbtns">
            <div class="toppanbtn">
              <button class="tbtn danger" @click="runAction(() => ClearDownloads(), '已清除')"><UiIcon name="trash" :size="14" />清除全部</button>
            </div>
            <div class="toolbar-spacer"></div>
            <div class="toppanbtn">
              <span class="search-quick-wrap">
                <span class="sq-icon"><UiIcon name="search" :size="13" /></span>
                <input class="search-quick" v-model="taskFilterRaw" placeholder="快速筛选" />
                <button v-if="taskFilterRaw" class="sq-clear" title="清空筛选" @click="taskFilterRaw = ''"><UiIcon name="close" :size="11" /></button>
              </span>
            </div>
          </div>
          <div class="file-list">
            <div v-for="t in doneDownloads" :key="t.id" class="taskrow" @contextmenu.prevent="onCtx($event, t)">
              <div class="rangselect"></div>
              <div class="fileicon" :class="'ft-' + iconOf({ name: t.name })"><UiIcon :name="iconOf({ name: t.name })" :size="20" /></div>
              <div class="filename">
                <div :title="t.localPath || ''">{{ t.name || t.url || t.id }}</div>
                <div class="fsub">{{ t.localPath || '' }}<template v-if="t.updated"> · {{ formatTime(t.updated) }}</template></div>
              </div>
              <div class="filesize">{{ formatBytes(t.size) }}</div>
              <div class="downprogress">
                <div class="transfering-state">
                  <p class="text-state"><span>已完成</span><span>100%</span></p>
                  <div class="progress-total"><div class="progress-current succeed" style="width:100%"></div></div>
                </div>
              </div>
              <div class="downspeed"></div>
              <div class="tactions">
                <button v-if="t.localPath" class="btn-circle" title="打开文件" @click="openFile(t)"><UiIcon name="play" :size="14" /></button>
                <button v-if="t.localPath" class="btn-circle" title="打开所在目录" @click="reveal(t)"><UiIcon name="folder" :size="14" /></button>
                <button v-if="t.url" class="btn-circle" title="复制链接" @click="copyLink(t)">
                  <UiIcon v-if="copiedLinkMap[t.id]" name="check" :size="14" class="icon-check-pop" />
                  <UiIcon v-else name="link" :size="14" />
                </button>
                <button class="btn-circle" title="删除记录" style="color:var(--color-error)" @click="removeTask(t)"><UiIcon name="trash" :size="14" /></button>
              </div>
            </div>
            <div v-if="!doneDownloads.length" class="workspace-empty-state">
              <UiIcon name="check" :size="36" style="opacity:.4" />
              <span class="wes-title">暂无已下载记录</span>
            </div>
          </div>
        </template>

        <!-- 正在上传 -->
        <template v-else-if="menu === 'uploading'">
          <div class="file-list">
            <div v-for="t in activeUploads" :key="t.UploadID" class="taskrow">
              <div class="rangselect"></div>
              <div class="fileicon" :class="'ft-' + iconOf({ name: upName(t) })"><UiIcon :name="iconOf({ name: upName(t) })" :size="20" /></div>
              <div class="filename">
                <div>{{ upName(t) }}</div>
                <div class="fsub">{{ t.Info && t.Info.path ? t.Info.path : (t.Info && t.Info.localFilePath) || '' }}</div>
              </div>
              <div class="filesize">{{ upSize(t) }}</div>
              <div class="downprogress">
                <div class="transfering-state">
                  <p class="text-state"><span class="badge" :class="upStatusBadge(t)">{{ upStatus(t) }}</span><span>{{ t.Upload.DownProcess || 0 }}%</span></p>
                  <div class="progress-total">
                    <div :class="t.Upload.IsDowning ? 'progress-current active' : t.Upload.IsFailed ? 'progress-current error' : 'progress-current'" :style="{ width: (t.Upload.DownProcess || 0) + '%' }"></div>
                  </div>
                  <p v-if="t.Upload.IsFailed && upErr(t)" class="text-error" :title="upErr(t)">{{ upErr(t) }}</p>
                </div>
              </div>
              <div class="downspeed">{{ t.Upload.IsDowning ? upSpeed(t) : '' }}</div>
              <div class="tactions">
                <button class="btn-circle" title="取消" style="color:var(--color-error)" @click="runAction(() => CancelUpload(t.UploadID))"><UiIcon name="x-circle" :size="14" /></button>
              </div>
            </div>
            <div v-if="!activeUploads.length" class="workspace-empty-state">
              <UiIcon name="upload" :size="36" style="opacity:.4" />
              <span class="wes-title">暂无上传任务</span>
            </div>
          </div>
        </template>

        <!-- 已上传 -->
        <template v-else-if="menu === 'uploaded'">
          <div class="toppanbtns">
            <div class="toppanbtn">
              <button class="tbtn danger" @click="runAction(() => ClearUploads(), '已清除')"><UiIcon name="trash" :size="14" />清除全部</button>
            </div>
            <div class="toolbar-spacer"></div>
            <div class="toppanbtn"><span class="panel-desc">{{ doneUploads.length }} 条记录</span></div>
          </div>
          <div class="file-list">
            <div v-for="t in doneUploads" :key="t.UploadID" class="taskrow">
              <div class="rangselect"></div>
              <div class="fileicon" :class="'ft-' + iconOf({ name: upName(t) })"><UiIcon :name="iconOf({ name: upName(t) })" :size="20" /></div>
              <div class="filename">
                <div>{{ upName(t) }}</div>
                <div class="fsub">{{ t.Info && t.Info.path ? t.Info.path : '' }}</div>
              </div>
              <div class="filesize">{{ upSize(t) }}</div>
              <div class="downprogress">
                <div class="transfering-state">
                  <p class="text-state"><span>已完成</span><span>100%</span></p>
                  <div class="progress-total"><div class="progress-current succeed" style="width:100%"></div></div>
                </div>
              </div>
              <div class="downspeed"></div>
              <div class="tactions">
                <button class="btn-circle" title="删除记录" style="color:var(--color-error)" @click="runAction(() => CancelUpload(t.UploadID))"><UiIcon name="trash" :size="14" /></button>
              </div>
            </div>
            <div v-if="!doneUploads.length" class="workspace-empty-state">
              <UiIcon name="check" :size="36" style="opacity:.4" />
              <span class="wes-title">暂无已上传记录</span>
            </div>
          </div>
        </template>

        <!-- 云离线 -->
        <template v-else-if="menu === 'offline'">
          <div class="toppanbtns">
            <div class="toppanbtn">
              <UiSelect
                v-model="offlineUser"
                style="width:200px"
                :options="offlineAccounts.map((acc) => ({ value: acc.user_id, label: accLabel(acc) }))"
              />
              <button class="tbtn primary" :disabled="!offlineUser" @click="offLinks = ''; offModal = true"><UiIcon name="plus" :size="14" />新建离线任务</button>
            </div>
            <div class="toolbar-spacer"></div>
            <div class="toppanbtn">
              <button class="tbtn" :disabled="offlineLoading" @click="refreshOffline">
                <span v-if="offlineLoading" class="spin"></span><template v-else><UiIcon name="refresh" :size="14" />刷新</template>
              </button>
            </div>
          </div>
          <div class="file-list">
            <div v-for="t in offlineTasks" :key="t.id" class="taskrow">
              <div class="rangselect"></div>
              <div class="fileicon" :class="'ft-' + iconOf({ name: t.file_name })"><UiIcon :name="iconOf({ name: t.file_name })" :size="20" /></div>
              <div class="filename">
                <div>{{ t.file_name || t.url }}</div>
                <div class="fsub">{{ t.url }}<template v-if="t.created"> · {{ formatTime(t.created) }}</template></div>
              </div>
              <div class="filesize"></div>
              <div class="downprogress">
                <div class="transfering-state">
                  <p class="text-state"><span>{{ t.status || '进行中' }}</span><span>{{ offProgress(t) }}%</span></p>
                  <div class="progress-total">
                    <div :class="offProgress(t) >= 100 ? 'progress-current succeed' : 'progress-current active'" :style="{ width: offProgress(t) + '%' }"></div>
                  </div>
                </div>
              </div>
              <div class="downspeed"></div>
              <div class="tactions"></div>
            </div>
            <div v-if="!offlineTasks.length && !offlineLoading" class="workspace-empty-state">
              <UiIcon name="cloud-down" :size="36" style="opacity:.4" />
              <span class="wes-title">暂无离线任务</span>
            </div>
            <div v-if="offlineLoading && !offlineTasks.length" class="empty"><span class="spin"></span><span>加载中…</span></div>
          </div>
        </template>

        <!-- 迁移 -->
        <template v-else-if="menu === 'migrate'">
          <div class="file-list">
            <div v-for="j in migrateJobs" :key="j.id" class="taskrow">
              <div class="rangselect"></div>
              <div class="fileicon"><UiIcon name="migrate" :size="20" style="color:var(--color-primary)" /></div>
              <div class="filename">
                <div>{{ migName(j.srcUser) }} → {{ migName(j.dstUser) }}</div>
                <div class="fsub">{{ (j.fileIDs || []).length }} 个文件<template v-if="j.failed"> · 失败 {{ j.failed }}</template></div>
              </div>
              <div class="filesize"></div>
              <div class="downprogress">
                <div class="transfering-state">
                  <p class="text-state"><span class="badge" :class="migBadge(j.status)">{{ migStatusText(j.status) }}</span><span>{{ migProgress(j) }}%</span></p>
                  <div class="progress-total">
                    <div :class="j.status === 'completed' ? 'progress-current succeed' : j.status === 'failed' ? 'progress-current error' : 'progress-current active'" :style="{ width: migProgress(j) + '%' }"></div>
                  </div>
                  <p v-if="j.message" class="text-error" :title="j.message">{{ j.message }}</p>
                </div>
              </div>
              <div class="downspeed">{{ (j.processed || 0) + '/' + (j.total || 0) }}</div>
              <div class="tactions"></div>
            </div>
            <div v-if="!migrateJobs.length" class="workspace-empty-state">
              <UiIcon name="migrate" :size="36" style="opacity:.4" />
              <span class="wes-title">暂无迁移任务</span>
              <span class="wes-desc">在网盘页选中文件后，右键选择「迁移到其他网盘」</span>
            </div>
          </div>
        </template>
      </section>
    </div>
    <!-- 新建下载弹窗 -->
    <Modal v-if="dlModal" title="新建下载" width="480px" @close="dlModal = false">
      <div class="field">
        <label>下载链接（每行一个 http/https）</label>
        <textarea v-model="dlLinks" class="textarea" rows="5" placeholder="https://example.com/file.zip"></textarea>
      </div>
      <div class="field">
        <label>文件名（可选，仅单个链接时生效）</label>
        <input v-model="dlName" class="input" placeholder="留空则自动命名" :disabled="dlUrlList.length !== 1" />
      </div>
      <div class="field">
        <label style="cursor:pointer;user-select:none" @click="dlAdvanced = !dlAdvanced">
          <UiIcon :name="dlAdvanced ? 'chevron-down' : 'chevron-right'" :size="12" /> 高级选项
        </label>
        <template v-if="dlAdvanced">
          <input v-model="dlHeaders.ua" class="input" style="width:100%;margin-bottom:8px" placeholder="User-Agent（可选）" />
          <input v-model="dlHeaders.referer" class="input" style="width:100%;margin-bottom:8px" placeholder="Referer（可选）" />
          <textarea v-model="dlHeaders.cookie" class="textarea" rows="2" placeholder="Cookie（可选）" style="width:100%"></textarea>
        </template>
      </div>
      <template #actions>
        <button class="btn" @click="dlModal = false">取消</button>
        <button class="btn primary" :disabled="!dlUrlList.length" @click="submitDownload">开始下载{{ dlUrlList.length > 1 ? ` (${dlUrlList.length})` : '' }}</button>
      </template>
    </Modal>

    <!-- 新建离线任务弹窗 -->
    <Modal v-if="offModal" title="新建离线任务" width="480px" @close="offModal = false">
      <div class="field">
        <label>链接（每行一个，支持 http/https/magnet/ed2k）</label>
        <textarea v-model="offLinks" class="textarea" rows="6" placeholder="magnet:?xt=urn:btih:…"></textarea>
      </div>
      <template #actions>
        <button class="btn" @click="offModal = false">取消</button>
        <button class="btn primary" :disabled="!offUrlList.length" @click="submitOffline">提交{{ offUrlList.length > 1 ? ` (${offUrlList.length})` : '' }}</button>
      </template>
    </Modal>

    <!-- 右键菜单 -->
    <ContextMenu
      v-if="ctx.show"
      :x="ctx.x" :y="ctx.y" :items="ctxItems"
      @close="ctx.show = false"
      @select="onCtxSelect"
    />

    <!-- 任务详情弹窗（复刻旧版 TaskDetailDrawer） -->
    <Modal v-if="detailTask" title="任务详情" width="440px" @close="detailTask = null">
      <div class="td-row"><span class="td-label">文件名</span><span class="td-val">{{ detailTask.name }}</span></div>
      <div class="td-row"><span class="td-label">任务 ID</span><span class="td-val">{{ detailTask.id }}</span></div>
      <div class="td-row"><span class="td-label">保存路径</span><span class="td-val">{{ detailTask.localPath || '-' }}</span></div>
      <div class="td-row"><span class="td-label">大小</span><span class="td-val">{{ formatBytes(detailTask.downloaded) }} / {{ formatBytes(detailTask.size) }}（{{ detailTask.progress || 0 }}%）</span></div>
      <div class="td-row"><span class="td-label">状态</span><span class="td-val">{{ statusText(detailTask.status) }}</span></div>
      <div class="td-row" v-if="detailTask.speed"><span class="td-label">速度</span><span class="td-val">{{ formatSpeed(detailTask.speed) }}</span></div>
      <div class="td-row" v-if="detailTask.url"><span class="td-label">来源链接</span><span class="td-val td-url">{{ detailTask.url }}</span></div>
      <div class="td-row" v-if="detailTask.error"><span class="td-label">失败原因</span><span class="td-val" style="color:var(--color-error)">{{ detailTask.error }}</span></div>
      <div class="td-row"><span class="td-label">创建时间</span><span class="td-val">{{ formatTime(detailTask.created) }}</span></div>
      <template #actions>
        <button class="btn primary" @click="detailTask = null">关闭</button>
      </template>
    </Modal>
  </div>
</template>

<style scoped>
.field { margin-bottom: 14px; }
.field label { display: block; font-size: 13px; color: var(--text-secondary); margin-bottom: 6px; }
.td-row { display: flex; gap: 12px; padding: 7px 0; border-bottom: 1px solid var(--border-lighter); font-size: 13.5px; }
.td-row:last-of-type { border-bottom: none; }
.td-label { width: 64px; flex-shrink: 0; color: var(--text-tertiary); }
.td-val { flex: 1; min-width: 0; user-select: text; word-break: break-all; }
.td-url { font-size: 12.5px; color: var(--text-secondary); }
</style>
