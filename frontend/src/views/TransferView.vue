<script setup>
import { ref, computed, watch, onMounted, onActivated, onDeactivated, onBeforeUnmount } from 'vue'
import {
  ListDownloads, ListUploads,
  PauseDownload, ResumeDownload, CancelDownload, ClearDownloads,
  RemoveDownload, PrioritizeDownload, OpenFile,
  CancelUpload, ClearUploads, DownloadURL, ResumeUpload, ResumeUploadReviewed,
  ListOfflineTasks, refreshOfflineTasks, OfflineDownload, DeleteOfflineTask,
  ListMigrateJobs, CancelMigrate, ResumeMigrate, DeleteMigrateJob, ClearMigrateJobs,
  EventsOn, RevealInFolder,
  verifyDownload, accountName, providerIconUrl, providerMetaOf, capsOf,
  formatBytes, formatSpeed, formatTime, iconOf, copyText
} from '../api'
import { getPrefs } from '../appearance'
import { createDownloadSoundPlayer } from '../download-sounds'
import Modal from '../components/Modal.vue'
import ConfirmModal from '../components/ConfirmModal.vue'
import ContextMenu from '../components/ContextMenu.vue'
import UiIcon from '../components/UiIcon.vue'
import UiSelect from '../components/UiSelect.vue'
import TaskIssue from '../components/TaskIssue.vue'
import TransferUploadRow from '../components/TransferUploadRow.vue'
import TransferDownloadRow from '../components/TransferDownloadRow.vue'
import TransferFolderDetails from '../components/TransferFolderDetails.vue'
import TransferMigrateRow from '../components/TransferMigrateRow.vue'
import TransferMigrateDetails from '../components/TransferMigrateDetails.vue'
import { downProgress, downTaskStatus } from '../download-display'
import { taskErrorText, notifyTransferFailure, notifyMigrateFailure } from '../task-notices'
import { mergeLiveTasks } from '../transfer-refresh'

const props = defineProps({
  accounts: { type: Array, default: () => [] },
  providers: { type: Array, default: () => [] }
})
const emit = defineEmits(['toast', 'navigate', 'repair'])
const detailID = ref('')
const detailJob = computed(() => migrateByID.value.get(detailID.value))

// ---------- 菜单 / 账号筛选 ----------
const menu = ref('downloading')
const filterUser = ref('')

// 支持云离线的账号
const hasOffline = computed(() => offlineAccounts.value.length > 0)
const offlineAccounts = computed(() => props.accounts.filter((a) => capsOf(a, props.providers).offlineDownload))

const HISTORY_PAGE_SIZE = 120
const doneDownloadLimit = ref(HISTORY_PAGE_SIZE)
const doneUploadLimit = ref(HISTORY_PAGE_SIZE)

const menus = computed(() => {
  const list = [
    { key: 'downloading', icon: 'download', label: '正在下载', cnt: activeDownloads.value.length },
    { key: 'downloaded', icon: 'check', label: '已下载', cnt: doneDownloads.value.length },
    { key: 'uploading', icon: 'upload', label: '正在上传', cnt: activeUploads.value.length },
    { key: 'uploaded', icon: 'check', label: '已上传', cnt: doneUploads.value.length }
  ]
  if (hasOffline.value) list.push({ key: 'offline', icon: 'cloud-down', label: '云离线', cnt: offlineTasks.value.length })
  list.push({ key: 'migrate', icon: 'migrate', label: '迁移', cnt: visibleMigrateJobs.value.length })
  return list
})

watch(hasOffline, (v) => { if (!v && menu.value === 'offline') menu.value = 'downloading' })
watch(menu, () => {
  selectedIds.value = new Set()
  ctx.value.show = false
  taskFilterRaw.value = ''
  taskFilter.value = ''
  doneDownloadLimit.value = HISTORY_PAGE_SIZE
  doneUploadLimit.value = HISTORY_PAGE_SIZE
})
watch(filterUser, () => {
  selectedIds.value = new Set()
  doneDownloadLimit.value = HISTORY_PAGE_SIZE
  doneUploadLimit.value = HISTORY_PAGE_SIZE
})

function accIcon(acc) { return providerIconUrl(providerMetaOf(acc, props.providers)) }
function accLabel(acc) { return accountName(acc) }

// ---------- 下载 / 上传数据 ----------
const downloads = ref([])
const uploads = ref([])
const downloadByID = computed(() => new Map(downloads.value.map(task => [task.id, task])))
const uploadByID = computed(() => new Map(uploads.value.map(task => [task.UploadID, task])))
const hasWorkingTransfers = computed(() =>
  downloads.value.some(task => task.status === 'downloading' || task.status === 'queued') ||
  uploads.value.some(({ Upload }) => Upload?.DownState === 'uploading' || Upload?.DownState === 'queued'),
)
const loading = ref(true)
const refreshError = ref('')
let refreshSeq = 0
let pendingDownloadEvents = null
let pendingUploadEvents = null
let progressFrame = null
const queuedProgress = new Map()

// 下载完成提示音
const finishSoundPlayer = createDownloadSoundPlayer()
function playFinishSound() {
  const prefs = getPrefs()
  if (prefs.downloadSound) void finishSoundPlayer.play(prefs.downloadSoundType)
}

async function refresh() {
  if (!viewActive) return
  if (refreshInFlight) {
    refreshQueued = true
    return
  }
  refreshInFlight = true
  const seq = ++refreshSeq
  const downloadEvents = new Map()
  const uploadEvents = new Map()
  pendingDownloadEvents = downloadEvents
  pendingUploadEvents = uploadEvents
  try {
    const [d, u] = await Promise.all([ListDownloads(), ListUploads()])
    if (seq !== refreshSeq) return
    flushProgress()
    downloads.value = mergeLiveTasks(d, downloadEvents, (task) => task.id, downloads.value)
    uploads.value = mergeLiveTasks(u, uploadEvents, (task) => task.UploadID, uploads.value)
    if (selectedIds.value.size) {
      const activeIds = new Set(downloads.value.filter((task) => task.status !== 'completed').map((task) => task.id))
      const next = new Set([...selectedIds.value].filter((id) => activeIds.has(id)))
      if (next.size !== selectedIds.value.size) selectedIds.value = next
    }
    refreshError.value = ''
  } catch (e) {
    if (seq === refreshSeq) refreshError.value = String(e && e.message ? e.message : e)
  } finally {
    if (seq === refreshSeq) {
      pendingDownloadEvents = null
      pendingUploadEvents = null
      loading.value = false
      refreshInFlight = false
      if (refreshQueued) {
        refreshQueued = false
        scheduleRefresh(0)
      }
    }
  }
}

let refreshInFlight = false
let refreshQueued = false

function onTransferEvent(ev) {
  if (!viewActive) return
  if (!ev || !ev.task) { scheduleRefresh(); return }
  notifyTransferFailure(ev)
  const { kind, task } = ev
  const key = `${kind}:${task.id}`
  const previous = queuedProgress.get(key)
  const upload = kind === 'upload' ? uploadByID.value.get(task.id)?.Upload : null
  const liveProgress = !task.issue && !task.error && (
    (kind === 'download' && task.status === 'downloading' && downloadByID.value.get(task.id)?.status === 'downloading') ||
    (kind === 'upload' && task.status === 'uploading' && upload?.IsDowning && upload.DownState === 'uploading')
  )
  if (liveProgress) {
    let next = task
    if (previous && kind === 'download') next = { ...previous.task, ...task }
    if (previous && kind === 'upload') {
      next = { ...task }
      for (const field of ['downloaded', 'progress']) {
        if (!Number.isFinite(next[field])) next[field] = previous.task[field]
      }
    }
    queuedProgress.set(key, { kind, task: next })
    if (progressFrame === null) progressFrame = requestAnimationFrame(flushProgress)
    return
  }
  if (previous) {
    queuedProgress.delete(key)
    applyTransferEvent(previous)
  }
  applyTransferEvent(ev)
}

function flushProgress() {
  if (progressFrame !== null) cancelAnimationFrame(progressFrame)
  progressFrame = null
  for (const event of queuedProgress.values()) applyTransferEvent(event)
  queuedProgress.clear()
}

function applyTransferEvent(ev) {
  const t = ev.task
  if (ev.kind === 'download') {
    const list = downloads.value
    const existing = downloadByID.value.get(t.id)
    if (t.status === 'removed') {
      pendingDownloadEvents?.set(t.id, null)
      if (existing) list.splice(list.indexOf(existing), 1)
      if (selectedIds.value.has(t.id)) {
        const s = new Set(selectedIds.value)
        s.delete(t.id)
        selectedIds.value = s
      }
      return
    }
    if (existing) {
      const prev = existing
      if (prev.status !== 'completed' && t.status === 'completed') {
        if (!t.groupId) playFinishSound()
        // 完成后清理该项已选状态
        if (selectedIds.value.has(t.id)) {
          const s = new Set(selectedIds.value)
          s.delete(t.id)
          selectedIds.value = s
        }
      }
      Object.assign(existing, t, { issue: t.issue || null })
    } else {
      downloads.value.unshift(t)
    }
    pendingDownloadEvents?.set(t.id, existing || t)
  } else if (ev.kind === 'upload') {
    const job = uploadByID.value.get(t.id)
    if (!job) {
      // The enqueue event can arrive before the first list refresh. Fetch once
      // for the new task, then keep subsequent progress event-driven.
      scheduleRefresh(150)
      return
    }
    const wasCompleted = job.Upload?.IsCompleted === true
    const next = {
      DownSize: Number.isFinite(t.downloaded) ? t.downloaded : (job.Upload?.DownSize || 0),
      DownSpeed: Number.isFinite(t.speed) ? t.speed : (job.Upload?.DownSpeed || 0),
      DownSpeedStr: t.speed ? formatSpeed(t.speed) : '',
      DownProcess: Number.isFinite(t.progress) ? t.progress : (job.Upload?.DownProcess || 0),
      stage: t.stage || '',
      stageProgress: Number.isFinite(t.stageProgress) ? t.stageProgress : 0,
      DownState: t.status === 'completed' ? 'completed' : (t.status === 'failed' ? 'failed' : (t.status === 'paused' ? 'stopped' : t.status)),
      IsCompleted: t.status === 'completed',
      IsFailed: t.status === 'failed',
      IsStop: t.status === 'paused' || t.status === 'canceled',
      IsDowning: t.status === 'uploading' || t.status === 'queued',
      failedMessage: t.error || '',
      issue: t.issue || null,
    }
    if (job.Upload) Object.assign(job.Upload, next)
    else job.Upload = next
    pendingUploadEvents?.set(t.id, job)
    if (!wasCompleted && t.status === 'completed') scheduleRefresh(0)
  }
}

let refreshTimer = null
let refreshDue = 0
function scheduleRefresh(delay = 400) {
  if (!viewActive) return
  const due = Date.now() + Math.max(0, delay)
  if (refreshTimer && refreshDue <= due) return
  clearTimeout(refreshTimer)
  refreshDue = due
  refreshTimer = setTimeout(() => {
    refreshTimer = null
    refreshDue = 0
    void refresh()
  }, Math.max(0, due - Date.now()))
}

// 快速筛选（任务名/链接，120ms 防抖）
const taskFilterRaw = ref('')
const taskFilter = ref('')
let taskFilterTimer = null
watch(taskFilterRaw, (v) => {
  clearTimeout(taskFilterTimer)
  taskFilterTimer = setTimeout(() => {
    taskFilter.value = v
    doneDownloadLimit.value = HISTORY_PAGE_SIZE
    doneUploadLimit.value = HISTORY_PAGE_SIZE
  }, 120)
})
const byKw = (list, nameFn) => {
  const kw = taskFilter.value.trim().toLowerCase()
  if (!kw) return list
  return list.filter((t) => (nameFn(t) || '').toLowerCase().includes(kw) || String(t.url || '').toLowerCase().includes(kw))
}
const byAccount = (list) => filterUser.value ? list.filter((t) => t.user_id === filterUser.value) : list
const byUploadAccount = (list) => filterUser.value ? list.filter((t) => t.UserID === filterUser.value || (t.Info && t.Info.user_id === filterUser.value)) : list
const visibleDownloads = computed(() => {
  const groups = new Set(downloads.value.filter(task => task.isDir).map(task => task.id))
  return downloads.value.filter(task => !task.groupId || !groups.has(task.groupId))
})
const downloadStatusBuckets = computed(() => {
  const active = []
  const done = []
  for (const task of visibleDownloads.value) {
    if (task.status === 'completed') done.push(task)
    else active.push(task)
  }
  return { active, done }
})
const activeDownloads = computed(() => byKw(byAccount(downloadStatusBuckets.value.active), (t) => t.name))
const doneDownloads = computed(() => byKw(byAccount(downloadStatusBuckets.value.done), (t) => t.name))
const renderedDoneDownloads = computed(() => doneDownloads.value.slice(0, doneDownloadLimit.value))
const groupChildrenByID = computed(() => {
  const groups = new Map()
  for (const task of downloads.value) {
    if (!task.groupId) continue
    if (!groups.has(task.groupId)) groups.set(task.groupId, [])
    groups.get(task.groupId).push(task)
  }
  return groups
})
const groupChildren = id => groupChildrenByID.value.get(id) || []
const groupChildLimits = ref({})
const failedGroupOnly = ref({})
const openFolderIds = ref(new Set())
function setFolderOpen(id, open) {
  if (open) openFolderIds.value.add(id)
  else openFolderIds.value.delete(id)
}
function toggleFailedGroup(id) { failedGroupOnly.value = { ...failedGroupOnly.value, [id]: !failedGroupOnly.value[id] }; groupChildLimits.value = { ...groupChildLimits.value, [id]: 100 } }
async function retryGroupChild(parent, child) {
  if (!['failed', 'paused'].includes(child.status)) return
  if (['paused', 'canceled'].includes(parent.status)) { emit('toast', '请先继续文件夹下载任务', 'warn'); return }
  await resumeTask(child)
}
function showMoreGroupChildren(id) { groupChildLimits.value = { ...groupChildLimits.value, [id]: (groupChildLimits.value[id] || 100) + 100 } }
const uploadStatusBuckets = computed(() => {
  const active = []
  const done = []
  for (const task of uploads.value) {
    if (!task.Upload) continue
    if (task.Upload.IsCompleted) done.push(task)
    else active.push(task)
  }
  return { active, done }
})
const activeUploads = computed(() => byKw(byUploadAccount(uploadStatusBuckets.value.active), upName))
const doneUploads = computed(() => byKw(byUploadAccount(uploadStatusBuckets.value.done), upName))
const renderedDoneUploads = computed(() => doneUploads.value.slice(0, doneUploadLimit.value))

// 全选（正在下载列表）
const allActiveSelected = computed(() => activeDownloads.value.length > 0 && activeDownloads.value.every((t) => selectedIds.value.has(t.id)))
function toggleSelectAllActive() {
  selectedIds.value = allActiveSelected.value ? new Set() : new Set(activeDownloads.value.map((t) => t.id))
}


const upName = (t) => (t.Info && t.Info.name) || ((t.Info && t.Info.localFilePath) || '').split(/[\\/]/).pop() || t.UploadID
const upSize = (t) => (t.Info && (t.Info.sizeStr || formatBytes(t.Info.size))) || ''

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
const resumableDownloads = computed(() => activeDownloads.value.filter((task) => task.status === 'paused' || task.status === 'failed'))
const pausableDownloads = computed(() => activeDownloads.value.filter((task) => task.status === 'downloading' || task.status === 'queued'))
const resumableSelected = computed(() => selectedTasks.value.filter((task) => task.status === 'paused' || task.status === 'failed'))
const pausableSelected = computed(() => selectedTasks.value.filter((task) => task.status === 'downloading' || task.status === 'queued'))
const cancelableSelected = computed(() => selectedTasks.value.filter((task) => task.status !== 'canceled'))

// ---------- 核心操作（全部配备即时乐观响应与回滚） ----------

// 暂停
async function pauseTask(t) {
  const orig = t.status
  t.status = 'paused'
  try {
    await PauseDownload(t.id)
    await refresh()
    emit('toast', '已暂停', 'warn')
  } catch (e) {
    t.status = orig
    await refresh()
    emit('toast', '操作失败: ' + String(e), 'error')
  }
}

// 继续/重试
async function resumeTask(t) {
  const orig = t.status
  t.status = 'queued'
  try {
    await ResumeDownload(t.id)
    await refresh()
    emit('toast', '已恢复下载', 'success')
  } catch (e) {
    t.status = orig
    await refresh()
    emit('toast', '操作失败: ' + String(e), 'error')
  }
}

// 取消下载
async function cancelTask(t) {
  const orig = t.status
  t.status = 'canceled'
  try {
    await CancelDownload(t.id)
    await refresh()
    emit('toast', '已取消下载', 'warn')
  } catch (e) {
    t.status = orig
    await refresh()
    emit('toast', '操作失败: ' + String(e), 'error')
  }
}

// 优先下载
function prioritizeTask(t) {
  askConfirm('优先下载此任务将暂停其他任务，是否继续？', async () => {
    const origMap = new Map(activeDownloads.value.map((x) => [x.id, x.status]))
    activeDownloads.value.forEach((x) => { if (x.id !== t.id && x.status === 'downloading') x.status = 'paused' })
    t.status = 'downloading'
    try {
      await PrioritizeDownload(t.id)
      await refresh()
      emit('toast', '已优先下载该任务（其他已暂停）', 'success')
    } catch (e) {
      activeDownloads.value.forEach((x) => { if (origMap.has(x.id)) x.status = origMap.get(x.id) })
      await refresh()
      emit('toast', '操作失败: ' + String(e), 'error')
    }
  }, { okText: '优先下载', title: '优先下载' })
}

// 删除单条下载记录 (立即乐观移除)
async function removeTask(t) {
  const id = t.id
  const origList = [...downloads.value]
  downloads.value = downloads.value.filter((x) => x.id !== id)
  const s = new Set(selectedIds.value)
  s.delete(id)
  selectedIds.value = s
  try {
    await RemoveDownload(id)
    await refresh()
    emit('toast', '已删除记录', 'success')
  } catch (e) {
    downloads.value = origList
    await refresh()
    emit('toast', '删除失败: ' + String(e), 'error')
  }
}

// 打开文件 / 定位目录
async function openFile(task) {
  if (!task.localPath) return
  try { await OpenFile(task.localPath) }
  catch (error) { emit('toast', '打开文件失败：' + String(error?.message || error), 'error') }
}
async function reveal(task) {
  if (!task.localPath) return
  try { await RevealInFolder(task.localPath) }
  catch (error) { emit('toast', '定位文件失败：' + String(error?.message || error), 'error') }
}

// 全部开始 / 全部暂停
async function runDownloadBatch(tasks, action, optimisticStatus, successMessage, failureAction, successType) {
  if (!tasks.length) return
  const originals = new Map(tasks.map((task) => [task.id, task.status]))
  tasks.forEach((task) => { task.status = optimisticStatus })
  const results = await Promise.allSettled(tasks.map((task) => Promise.resolve().then(() => action(task.id))))
  const failed = results.filter((result) => result.status === 'rejected').length
  results.forEach((result, index) => {
    if (result.status === 'rejected') tasks[index].status = originals.get(tasks[index].id)
  })
  await refresh()
  if (failed) emit('toast', `${failed} 个任务${failureAction}失败，已完成 ${tasks.length - failed}/${tasks.length}`, 'error')
  else emit('toast', successMessage, successType)
}

function startAll() { return runDownloadBatch([...resumableDownloads.value], ResumeDownload, 'queued', '已开始全部下载', '恢复', 'success') }

function pauseAll() { return runDownloadBatch([...pausableDownloads.value], PauseDownload, 'paused', '已暂停全部下载', '暂停', 'warn') }

// 批量操作 (继续/暂停/取消/删除)
function batchResume() {
  const tasks = [...resumableSelected.value]
  return runDownloadBatch(tasks, ResumeDownload, 'queued', `已恢复 ${tasks.length} 个任务`, '恢复', 'success')
}

function batchPause() {
  const tasks = [...pausableSelected.value]
  return runDownloadBatch(tasks, PauseDownload, 'paused', `已暂停 ${tasks.length} 个任务`, '暂停', 'warn')
}

function batchCancel() {
  const tasks = [...cancelableSelected.value]
  return runDownloadBatch(tasks, CancelDownload, 'canceled', `已取消 ${tasks.length} 个任务`, '取消', 'warn')
}

function batchRemove() {
  const tasks = [...selectedTasks.value]
  const ids = new Set(tasks.map((t) => t.id))
  const origList = [...downloads.value]
  downloads.value = downloads.value.filter((t) => !ids.has(t.id))
  selectedIds.value = new Set()
  let failed = 0
  Promise.all(Array.from(ids).map((id) => RemoveDownload(id).catch(() => { failed++ })))
    .then(async () => {
      if (failed) {
        downloads.value = origList
        await refresh()
        emit('toast', `${failed} 项记录删除失败`, 'error')
      } else {
        await refresh()
        emit('toast', `已删除 ${ids.size} 项记录`, 'success')
      }
    })
}

// 清除所有已完成下载
async function clearAllDoneDownloads() {
  downloads.value = downloads.value.filter((t) => t.status !== 'completed')
  try {
    await ClearDownloads()
    await refresh()
    emit('toast', '已清除已完成记录', 'success')
  } catch (e) {
    await refresh()
    emit('toast', '清除失败: ' + String(e), 'error')
  }
}

// 清除所有已上传记录
async function clearAllDoneUploads() {
  uploads.value = uploads.value.filter((t) => !t.Upload || !t.Upload.IsCompleted)
  try {
    await ClearUploads()
    await refresh()
    emit('toast', '已清除已上传记录', 'success')
  } catch (e) {
    await refresh()
    emit('toast', '清除失败: ' + String(e), 'error')
  }
}

// 取消/重试上传
async function cancelUploadTask(t) {
  const id = t.UploadID
  const origList = [...uploads.value]
  uploads.value = uploads.value.filter((x) => x.UploadID !== id)
  try {
    await CancelUpload(id)
    await refresh()
    emit('toast', '已取消上传', 'warn')
  } catch (e) {
    uploads.value = origList
    await refresh()
    emit('toast', '取消失败: ' + String(e), 'error')
  }
}

async function resumeUploadTask(t) {
  try {
    await ResumeUpload(t.UploadID)
    emit('toast', uploadNeedsReview(t) ? '已复核上传结果' : '已恢复上传', 'success')
    await refresh()
  } catch (e) {
    await refresh()
    emit('toast', String(e), 'error')
  }
}

function uploadNeedsReview(task) {
  return ['pending', 'unknown'].includes(task.Upload?.attempt?.state) || task.Upload?.issue?.code === 'UPLOAD_RESULT_UNKNOWN'
}

function reviewAndResumeUpload(task) {
  askConfirm('网盘可能已经接收文件。请先打开目标目录核对；仅在确认仍需上传时继续。继续可能产生重复文件，或按原冲突策略覆盖同名文件。', async () => {
    try {
      await ResumeUploadReviewed(task.UploadID)
      emit('toast', '已按核对结果恢复上传', 'success')
      await refresh()
    } catch (error) {
      await refresh()
      emit('toast', String(error), 'error')
    }
  }, { title: '核对后重试上传', okText: '确认仍需上传', danger: true })
}

const verifyingDownloadIds = ref(new Set())
const downloadVerifications = ref({})
async function verifyDownloadTask(t) {
  if (!t?.id || t.isDir || verifyingDownloadIds.value.has(t.id)) return
  const next = new Set(verifyingDownloadIds.value)
  next.add(t.id)
  verifyingDownloadIds.value = next
  try {
    const res = await verifyDownload(t.id)
    if (res) {
      downloadVerifications.value = { ...downloadVerifications.value, [t.id]: res }
      const tone = res.status === 'hash' ? 'success' : res.status === 'size' ? 'info' : 'warn'
      emit('toast', res.detail, tone)
    }
  } catch (e) {
    emit('toast', '校验失败：' + String(e?.message || e), 'error')
  } finally {
    const done = new Set(verifyingDownloadIds.value)
    done.delete(t.id)
    verifyingDownloadIds.value = done
  }
}

const copiedLinkMap = ref({})
async function copyLink(t) {
  if (!t.url) return
  try {
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
  } catch (error) {
    emit('toast', '复制失败：' + String(error?.message || error), 'error')
  }
}

// ---------- 右键菜单 ----------
const ctx = ref({ show: false, x: 0, y: 0, task: null })
const confirmDialog = ref(null)
function askConfirm(message, onOk, opts) {
  confirmDialog.value = { message, onOk, okText: opts?.okText || '确定', danger: opts?.danger || false, title: opts?.title || '确认操作' }
}
function closeConfirm() { confirmDialog.value = null }
function handleConfirmOk() {
  if (!confirmDialog.value) return
  const cb = confirmDialog.value.onOk
  closeConfirm()
  cb && cb()
}

function onCtx(e, t) {
  ctx.value = { show: true, x: e.clientX, y: e.clientY, task: t }
}
const ctxItems = computed(() => {
  const t = ctx.value.task
  if (!t) return []
  const targets = selectedIds.value.has(t.id) && selectedTasks.value.length > 1 ? selectedTasks.value : [t]
  const items = []
  if (t.status === 'completed') {
    if (t.localPath) items.push({ icon: 'play', label: '打开文件', action: 'open' })
    if (t.localPath) items.push({ icon: 'folder', label: '打开所在目录', action: 'reveal' })
    items.push({ icon: 'link', label: '复制链接', disabled: !t.url, action: 'copy' })
    items.push({ sep: true })
    items.push({ icon: 'trash', label: '删除记录', danger: true, action: 'remove' })
    return items
  }
  const resumable = targets.filter((task) => task.status === 'paused' || task.status === 'failed').length
  const pausable = targets.filter((task) => task.status === 'downloading' || task.status === 'queued').length
  const cancelable = targets.filter((task) => task.status !== 'canceled').length
  if (resumable) items.push({ icon: 'play', label: resumable > 1 ? `继续 (${resumable})` : '继续', action: 'resume' })
  if (pausable) items.push({ icon: 'pause', label: pausable > 1 ? `暂停 (${pausable})` : '暂停', action: 'pause' })
  if (targets.some((x) => x.status !== 'completed'))
    items.push({ icon: 'priority', label: '优先下载', action: 'prioritize' })
  if (cancelable) items.push({ icon: 'x-circle', label: cancelable > 1 ? `取消 (${cancelable})` : '取消', danger: true, action: 'cancel' })
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
  if (action === 'resume') {
    const applicable = targets.filter((task) => task.status === 'paused' || task.status === 'failed')
    runDownloadBatch(applicable, ResumeDownload, 'queued', `已恢复 ${applicable.length} 个任务`, '恢复', 'success')
  } else if (action === 'pause') {
    const applicable = targets.filter((task) => task.status === 'downloading' || task.status === 'queued')
    runDownloadBatch(applicable, PauseDownload, 'paused', `已暂停 ${applicable.length} 个任务`, '暂停', 'warn')
  }
  else if (action === 'prioritize') prioritizeTask(t)
  else if (action === 'cancel') {
    const applicable = targets.filter((task) => task.status !== 'canceled' && task.status !== 'completed')
    runDownloadBatch(applicable, CancelDownload, 'canceled', `已取消 ${applicable.length} 个任务`, '取消', 'warn')
  }
  else if (action === 'remove') removeTask(t)
  else if (action === 'open') openFile(t)
  else if (action === 'reveal') reveal(t)
  else if (action === 'copy') copyLink(t)
  else if (action === 'detail') detailTask.value = t
}

// ---------- 新建下载 ----------
const dlModal = ref(false)
const dlLinks = ref('')
const dlName = ref('')
const dlAdvanced = ref(false)
const dlHeaders = ref({ ua: '', referer: '', cookie: '' })
const downloadingUrls = ref(false)
const dlSubmitted = ref(0)
const dlSubmitError = ref('')
function openDlModal() {
  if (downloadingUrls.value) return
  if (!dlSubmitError.value) {
    dlLinks.value = ''; dlName.value = ''; dlAdvanced.value = false
    dlHeaders.value = { ua: '', referer: '', cookie: '' }
  }
  dlModal.value = true
}
function closeDlModal() {
  dlModal.value = false
  if (!downloadingUrls.value) dlSubmitError.value = ''
}
function isHttpUrl(value) {
  try {
    const url = new URL(value)
    return ['http:', 'https:'].includes(url.protocol) && !!url.hostname
  } catch { return false }
}
const dlUrlList = computed(() => dlLinks.value.split('\n').map((s) => s.trim()).filter(isHttpUrl))
const dlInvalidCount = computed(() => dlLinks.value.split('\n').map((s) => s.trim()).filter((s) => s && !isHttpUrl(s)).length)
async function submitDownload() {
  if (downloadingUrls.value) return
  const urls = dlUrlList.value
  if (!urls.length || dlInvalidCount.value) { emit('toast', '链接列表中包含无效链接，请检查', 'error'); return }
  downloadingUrls.value = true
  dlSubmitted.value = 0
  dlSubmitError.value = ''
  const name = urls.length === 1 ? dlName.value.trim() : ''
  const headers = {}
  if (dlHeaders.value.ua.trim()) headers['User-Agent'] = dlHeaders.value.ua.trim()
  if (dlHeaders.value.referer.trim()) headers['Referer'] = dlHeaders.value.referer.trim()
  if (dlHeaders.value.cookie.trim()) headers['Cookie'] = dlHeaders.value.cookie.trim()
  const failed = []
  let firstError = null
  try {
    for (const url of urls) {
      try { await DownloadURL(name, url, headers) } catch (error) { failed.push(url); firstError ||= error }
      dlSubmitted.value++
    }
    if (failed.length) {
      dlLinks.value = failed.join('\n')
      dlName.value = name
      dlSubmitError.value = `已添加 ${urls.length - failed.length}/${urls.length} 个下载任务；失败：${String(firstError)}`
      if (!dlModal.value) emit('toast', `${dlSubmitError.value}。可从「新建下载」重试失败链接。`, 'error')
    } else {
      dlModal.value = false
      emit('toast', `已添加 ${urls.length} 个下载任务`, 'success')
    }
    refresh()
  } finally {
    downloadingUrls.value = false
  }
}

// ---------- 云离线 ----------
const offlineUser = ref('')
const offlineTasks = ref([])
const offlineLoading = ref(false)
const offlineError = ref('')
let offlineRefreshSeq = 0
let offlineRequest = null
let offlineHasSnapshot = false
const offModal = ref(false)
const offLinks = ref('')
let offlineTimer = null

watch(offlineAccounts, (list) => {
  if (!list.some((account) => account.user_id === offlineUser.value)) offlineUser.value = list[0]?.user_id || ''
}, { immediate: true })

watch(() => props.accounts.map(account => account.user_id), (ids) => {
  if (filterUser.value && !ids.includes(filterUser.value)) filterUser.value = ''
})

const offlineDriveId = computed(() => {
  const acc = offlineAccounts.value.find((a) => a.user_id === offlineUser.value)
  return acc ? acc.drive_id : ''
})

function invalidateOfflineRefresh() {
  offlineRefreshSeq++
  offlineRequest = null
  offlineLoading.value = false
}

function refreshOffline() {
  const userID = offlineUser.value
  const driveID = offlineDriveId.value
  if (!viewActive || menu.value !== 'offline' || !userID) return
  if (offlineRequest) return offlineRequest
  const seq = ++offlineRefreshSeq
  const isCurrent = () => seq === offlineRefreshSeq && userID === offlineUser.value && driveID === offlineDriveId.value && menu.value === 'offline'
  offlineLoading.value = true
  offlineRequest = Promise.resolve().then(async () => {
    if (!isCurrent()) return
    try {
      if (!offlineHasSnapshot) {
        const cached = (await ListOfflineTasks(userID)) || []
        if (!isCurrent()) return
        offlineTasks.value = cached.filter(task => task.drive_id === driveID)
        offlineHasSnapshot = true
      }
      const remote = (await refreshOfflineTasks(userID, driveID)) || []
      if (!isCurrent()) return
      offlineTasks.value = remote
      offlineError.value = ''
    } catch (error) {
      if (isCurrent()) offlineError.value = String(error?.message || error)
    } finally {
      if (isCurrent()) {
        offlineRequest = null
        offlineLoading.value = false
      }
    }
  })
  return offlineRequest
}
watch([menu, offlineUser, offlineDriveId], ([tab, user, drive], [, previousUser, previousDrive]) => {
  invalidateOfflineRefresh()
  if (user !== previousUser || drive !== previousDrive) {
    offlineHasSnapshot = false
    offlineTasks.value = []
    offlineError.value = ''
    offModal.value = false
  }
  if (tab === 'offline') void refreshOffline()
})

function isOfflineUrl(value) { return isHttpUrl(value) || /^magnet:\?.+/i.test(value) || /^ed2k:\/\/.+/i.test(value) }
const offUrlList = computed(() => offLinks.value.split('\n').map((s) => s.trim()).filter(isOfflineUrl))
const offInvalidCount = computed(() => offLinks.value.split('\n').map((s) => s.trim()).filter((s) => s && !isOfflineUrl(s)).length)
const submittingOffline = ref(false)
const offlineSubmitted = ref(0)
const offlineRetries = ref(new Map())
const offlineScope = computed(() => JSON.stringify([offlineUser.value, offlineDriveId.value]))
const offlineSubmitError = computed(() => offlineRetries.value.get(offlineScope.value)?.message || '')

function openOfflineModal() {
  if (submittingOffline.value || !offlineUser.value || !offlineDriveId.value) return
  offLinks.value = offlineRetries.value.get(offlineScope.value)?.links || ''
  offModal.value = true
}

function closeOfflineModal() {
  offModal.value = false
  if (!submittingOffline.value) offlineRetries.value.delete(offlineScope.value)
}

async function submitOffline() {
  if (submittingOffline.value) return
  const urls = offUrlList.value
  if (!urls.length || offInvalidCount.value) { emit('toast', '链接列表中包含无效链接，请检查', 'error'); return }
  const userID = offlineUser.value, driveID = offlineDriveId.value
  if (!userID || !driveID) { emit('toast', '当前账号不支持离线下载', 'error'); return }
  const scope = offlineScope.value
  const isCurrentScope = () => offlineScope.value === scope
  submittingOffline.value = true
  offlineSubmitted.value = 0
  offlineRetries.value.delete(scope)
  const failed = []
  let firstError = null
  try {
    for (const url of urls) {
      try { await OfflineDownload(userID, driveID, url, '') }
      catch (error) { failed.push(url); firstError ||= error }
      offlineSubmitted.value++
    }
    if (failed.length) {
      const links = failed.join('\n')
      const message = `已提交 ${urls.length - failed.length}/${urls.length} 个离线任务；失败：${String(firstError)}`
      offlineRetries.value.set(scope, { links, message })
      if (viewActive && offModal.value && isCurrentScope()) offLinks.value = links
      else emit('toast', `${message}。请回到原账号重试失败链接。`, 'error')
    } else {
      if (isCurrentScope()) {
        offLinks.value = ''
        offModal.value = false
      }
      emit('toast', `已提交 ${urls.length} 个离线任务`, 'success')
    }
  } finally {
    submittingOffline.value = false
  }
  if (isCurrentScope()) {
    await offlineRequest
    if (isCurrentScope()) await refreshOffline()
  }
}
const offProgress = (t) => Math.max(0, Math.min(100, Math.round(t.progress || 0)))
const offStatusText = (status) => ({ queued: '排队中', running: '进行中', completed: '已完成', failed: '失败', canceled: '已取消', partial: '部分完成', unknown: '状态未知', notFoundOnServer: '网盘未返回', PHASE_TYPE_PENDING: '排队中', PHASE_TYPE_RUNNING: '进行中', PHASE_TYPE_COMPLETE: '已完成', PHASE_TYPE_ERROR: '失败' }[status] || status || '状态未知')
const offProgressClass = (status) => ['completed', 'PHASE_TYPE_COMPLETE'].includes(status) ? 'succeed' : ['failed', 'PHASE_TYPE_ERROR'].includes(status) ? 'error' : ['partial', 'canceled'].includes(status) ? 'warning' : 'active'

async function delOfflineTask(t) {
  const userID = offlineUser.value, driveID = offlineDriveId.value, taskID = t.task_id || t.id
  askConfirm(`确定删除离线任务「${t.file_name || t.url || t.task_id}」并同步删除网盘文件？`, async () => {
    if (offlineUser.value !== userID || offlineDriveId.value !== driveID) { emit('toast', '账号已切换，请重新选择任务', 'warn'); return }
    await offlineRequest
    if (offlineUser.value !== userID || offlineDriveId.value !== driveID) { emit('toast', '账号已切换，请重新选择任务', 'warn'); return }
    // 乐观剔除
    offlineTasks.value = offlineTasks.value.filter((x) => x.task_id !== taskID && x.id !== taskID)
    try {
      await DeleteOfflineTask(userID, driveID, taskID, true)
      await refreshOffline()
      emit('toast', '已删除', 'success')
    } catch (e) { await refreshOffline(); emit('toast', String(e), 'error') }
  }, { danger: true, title: '删除离线任务' })
}

// ---------- 迁移 ----------
const migrateJobs = ref([])
const migrateByID = computed(() => new Map(migrateJobs.value.map(job => [job.id, job])))
const visibleMigrateJobs = computed(() => filterUser.value ? migrateJobs.value.filter((job) => job.srcUser === filterUser.value || job.dstUser === filterUser.value) : migrateJobs.value)
const migrateLoading = ref(false)
let migrateRefreshSeq = 0
let pendingMigrateEvents = null
let migrateFrame = null
const queuedMigrateJobs = new Map()
function onMigrate(job) {
  if (!viewActive || !job?.id) return
  notifyMigrateFailure(job)
  const current = migrateByID.value.get(job.id)
  if (current?.status === 'running' && job.status === 'running' && current.stage === job.stage && !job.issue) {
    queuedMigrateJobs.set(job.id, job)
    if (migrateFrame === null) migrateFrame = requestAnimationFrame(flushMigrateProgress)
    return
  }
  queuedMigrateJobs.delete(job.id)
  applyMigrateSnapshot(job)
}
function applyMigrateSnapshot(job) {
  const current = migrateByID.value.get(job.id)
  if (current) {
    for (const key of Object.keys(current)) {
      if (!Object.hasOwn(job, key)) delete current[key]
    }
    Object.assign(current, job)
  } else migrateJobs.value.unshift(job)
  pendingMigrateEvents?.set(job.id, current || job)
}
function flushMigrateProgress() {
  if (migrateFrame !== null) cancelAnimationFrame(migrateFrame)
  migrateFrame = null
  for (const job of queuedMigrateJobs.values()) applyMigrateSnapshot(job)
  queuedMigrateJobs.clear()
}
async function refreshMigrateJobs({ background = false } = {}) {
  if (!viewActive) return
  if (background && pendingMigrateEvents) return
  const seq = ++migrateRefreshSeq
  const migrateEvents = new Map()
  pendingMigrateEvents = migrateEvents
  flushMigrateProgress()
  if (!background) migrateLoading.value = true
  try {
    const list = await ListMigrateJobs()
    if (seq === migrateRefreshSeq) {
      flushMigrateProgress()
      migrateJobs.value = mergeLiveTasks(list, migrateEvents, (job) => job.id, migrateJobs.value)
    }
  } catch (e) {
    if (seq === migrateRefreshSeq && !migrateJobs.value.length) emit('toast', '迁移任务加载失败: ' + String(e), 'error')
  } finally {
    if (pendingMigrateEvents === migrateEvents) pendingMigrateEvents = null
    if (seq === migrateRefreshSeq) migrateLoading.value = false
  }
}
async function cancelMigrateJob(job) {
  if (!job || !job.id || !['pending', 'running'].includes(job.status)) return
  try {
    await CancelMigrate(job.id)
    job.message = '正在取消…'
    await refreshMigrateJobs()
    setTimeout(refreshMigrateJobs, 150)
    emit('toast', '已取消迁移', 'warn')
  } catch (e) {
    emit('toast', '取消迁移失败: ' + String(e), 'error')
  }
}
async function resumeMigrateJob(job) {
  if (!job || !job.id || !['partial', 'failed', 'canceled'].includes(job.status)) return
  try {
    const updated = await ResumeMigrate(job.id)
    if (updated) onMigrate(updated)
    emit('toast', '已恢复迁移，将跳过已完成项', 'success')
  } catch (e) {
    await refreshMigrateJobs()
    emit('toast', '恢复迁移失败: ' + String(e), 'error')
  }
}
async function removeMigrateJob(job) {
  if (!job || !job.id || ['pending', 'running'].includes(job.status)) return
  const original = migrateJobs.value
  migrateJobs.value = original.filter((x) => x.id !== job.id)
  try {
    await DeleteMigrateJob(job.id)
    await refreshMigrateJobs()
    emit('toast', '已删除迁移记录', 'success')
  } catch (e) {
    migrateJobs.value = original
    await refreshMigrateJobs()
    emit('toast', '删除迁移记录失败: ' + String(e), 'error')
  }
}
async function clearMigrateHistory() {
  const original = migrateJobs.value
  migrateJobs.value = original.filter((x) => ['pending', 'running'].includes(x.status))
  try {
    await ClearMigrateJobs()
    await refreshMigrateJobs()
    emit('toast', '已清除迁移记录', 'success')
  } catch (e) {
    migrateJobs.value = original
    await refreshMigrateJobs()
    emit('toast', '清除迁移记录失败: ' + String(e), 'error')
  }
}
const migName = (uid) => {
  const acc = props.accounts.find((a) => a.user_id === uid)
  return acc ? accountName(acc) : uid
}
const migIcon = (uid) => providerIconUrl(providerMetaOf(props.accounts.find(a => a.user_id === uid) || { user_id: uid }, props.providers))
const migStatusText = (s) => ({ pending: '等待中', running: '迁移中', completed: '已完成', partial: '部分完成', failed: '失败', canceled: '已取消' }[s] || s)

// ---------- 生命周期 ----------
let pollTimer = null
let pollEpoch = 0
let offFns = []
let viewActive = false

function bindEvents() {
  if (offFns.length) return
  const off1 = EventsOn('transfer:event', onTransferEvent)
  const off2 = EventsOn('migrate:progress', onMigrate)
  if (typeof off1 === 'function') offFns.push(off1)
  if (typeof off2 === 'function') offFns.push(off2)
}

function unbindEvents() {
  offFns.forEach((fn) => { try { fn() } catch { /* 忽略 */ } })
  offFns = []
}

function stopPolling() {
  pollEpoch++
  clearTimeout(pollTimer)
  clearTimeout(offlineTimer)
  pollTimer = null
  offlineTimer = null
}

function startPolling() {
  stopPolling()
  const epoch = pollEpoch
  const pollTransfers = async () => {
    if (!viewActive || epoch !== pollEpoch) return
    if (!document.hidden) {
      const transfers = hasWorkingTransfers.value ? refresh() : null
      const migrations = menu.value === 'migrate' ? refreshMigrateJobs({ background: true }) : null
      await Promise.all([transfers, migrations])
    }
    if (viewActive && epoch === pollEpoch) pollTimer = setTimeout(pollTransfers, hasWorkingTransfers.value || menu.value === 'migrate' ? 5000 : 15000)
  }
  const pollOffline = async () => {
    if (!viewActive || epoch !== pollEpoch) return
    if (!document.hidden && menu.value === 'offline') await refreshOffline()
    if (viewActive && epoch === pollEpoch) offlineTimer = setTimeout(pollOffline, menu.value === 'offline' ? 8000 : 20000)
  }
  pollTimer = setTimeout(pollTransfers, 5000)
  offlineTimer = setTimeout(pollOffline, 8000)
}

function activateView() {
  if (viewActive) return
  viewActive = true
  bindEvents()
  refresh()
  refreshMigrateJobs()
  if (menu.value === 'offline') void refreshOffline()
  startPolling()
}

function deactivateView() {
  if (!viewActive) return
  viewActive = false
  invalidateOfflineRefresh()
  offModal.value = false
  detailID.value = ''
  if (progressFrame !== null) cancelAnimationFrame(progressFrame)
  progressFrame = null
  queuedProgress.clear()
  if (migrateFrame !== null) cancelAnimationFrame(migrateFrame)
  migrateFrame = null
  queuedMigrateJobs.clear()
  migrateRefreshSeq++
  pendingMigrateEvents = null
  migrateLoading.value = false
  stopPolling()
  clearTimeout(refreshTimer)
  refreshTimer = null
  refreshDue = 0
  refreshQueued = false
  refreshSeq++
  refreshInFlight = false
  pendingDownloadEvents = null
  pendingUploadEvents = null
  unbindEvents()
}

onMounted(activateView)
onActivated(activateView)
onDeactivated(deactivateView)
onBeforeUnmount(() => {
  finishSoundPlayer.stop()
  deactivateView()
  clearTimeout(taskFilterTimer)
})
</script>

<template>
  <div class="page">
    <TransferMigrateDetails
      v-if="detailJob"
      :key="detailID"
      :job="detailJob"
      :source-name="migName(detailJob.srcUser)"
      :source-icon="migIcon(detailJob.srcUser)"
      :target-name="migName(detailJob.dstUser)"
      :target-icon="migIcon(detailJob.dstUser)"
      :status-label="migStatusText"
      @close="detailID = ''"
      @toast="(message, type) => emit('toast', message, type)"
      @navigate="emit('navigate', $event)"
      @resume="resumeMigrateJob"
    />
    <div class="down-layout">
      <!-- 左侧边栏 -->
      <aside class="down-side" aria-label="传输分类与账号筛选">
        <div class="transfer-side-title">传输中心</div>
        <div class="transfer-menu" role="tablist" aria-label="传输分类">
        <button
          v-for="m in menus" :key="m.key"
          type="button"
          class="down-menu-item" :class="{ active: menu === m.key }"
          role="tab"
          :aria-selected="menu === m.key"
          @click="menu = m.key"
        >
          <span style="width:15px;display:inline-flex"><UiIcon :name="m.icon" :size="15" /></span><span>{{ m.label }}</span>
          <span v-if="m.cnt" class="cnt">{{ m.cnt }}</span>
        </button>
        </div>

        <button type="button" class="down-filter" :class="{ active: !filterUser }" :aria-pressed="!filterUser" @click="filterUser = ''">
          <span style="width:15px;display:inline-flex"><UiIcon name="globe" :size="15" /></span>
          <span class="df-name">全部账号</span>
        </button>
        <button
          v-for="acc in accounts" :key="acc.user_id"
          type="button"
          class="down-filter" :class="{ active: filterUser === acc.user_id }"
          :aria-pressed="filterUser === acc.user_id"
          @click="filterUser = acc.user_id"
        >
          <img v-if="accIcon(acc)" :src="accIcon(acc)" alt="" />
          <span v-else style="width:15px;display:inline-flex"><UiIcon name="drive" :size="15" /></span>
          <span class="df-name">{{ accLabel(acc) }}</span>
        </button>
      </aside>

      <!-- 右侧内容区 -->
      <section :key="menu" class="down-content content-reveal">
        <div v-if="refreshError" class="transfer-error" role="alert">
          <UiIcon name="alert" :size="15" />
          <span>传输列表刷新失败：{{ refreshError }}</span>
          <button type="button" class="transfer-error-retry" @click="refresh">重试</button>
        </div>
        <!-- 正在下载 -->
        <template v-if="menu === 'downloading'">
          <div class="toppanbtns">
            <div class="toppanbtn">
              <button class="tbtn primary" :disabled="downloadingUrls" @click="openDlModal"><UiIcon name="plus" :size="14" />新建下载</button>
            </div>
            <div class="toppanbtn" v-if="!selectedIds.size">
              <button class="tbtn" :disabled="!resumableDownloads.length" @click="startAll"><UiIcon name="play" :size="14" />开始全部</button>
              <button class="tbtn" :disabled="!pausableDownloads.length" @click="pauseAll"><UiIcon name="pause" :size="14" />暂停全部</button>
              <button class="tbtn danger" :disabled="!doneDownloads.length" @click="askConfirm('确定清除已完成的下载记录？', clearAllDoneDownloads, { danger: true, title: '清除已完成' })"><UiIcon name="trash" :size="14" />清空已完成</button>
            </div>
            <div class="toppanbtn" v-else>
              <button class="tbtn" :disabled="!resumableSelected.length" @click="batchResume"><UiIcon name="play" :size="14" />继续 ({{ resumableSelected.length }})</button>
              <button class="tbtn" :disabled="!pausableSelected.length" @click="batchPause"><UiIcon name="pause" :size="14" />暂停 ({{ pausableSelected.length }})</button>
              <button class="tbtn danger" :disabled="!cancelableSelected.length" @click="batchCancel"><UiIcon name="x-circle" :size="14" />取消 ({{ cancelableSelected.length }})</button>
              <button class="tbtn danger" @click="batchRemove"><UiIcon name="trash" :size="14" />删除 ({{ selectedTasks.length }})</button>
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
                <UiIcon :name="allActiveSelected ? 'minus' : 'check'" :size="15" />
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
            <div v-if="loading && !activeDownloads.length" class="skeleton-pulse">
              <div v-for="i in 5" :key="i" class="skeleton-row">
                <div class="skeleton skeleton-icon"></div>
                <div class="skeleton skeleton-name" :style="{ width: (45 + (i * 9) % 30) + '%' }"></div>
                <div class="skeleton skeleton-size"></div>
                <div class="skeleton skeleton-date"></div>
              </div>
            </div>
            <template v-else>
              <TransferDownloadRow
                v-for="task in activeDownloads"
                :key="task.id"
                :task="task"
                :selected="selectedIds.has(task.id)"
                @select="onItemClick($event, task.id)"
                @toggle="toggleSelect(task.id)"
                @context="onCtx($event, task)"
                @prioritize="prioritizeTask"
                @pause="pauseTask"
                @resume="resumeTask"
                @reveal="reveal"
                @cancel="cancelTask"
                @remove="removeTask"
              >
                <TransferFolderDetails
                  v-if="task.isDir && groupChildren(task.id).length"
                  :children="groupChildren(task.id)"
                  :open="openFolderIds.has(task.id)"
                  :failed-only="failedGroupOnly[task.id]"
                  :limit="groupChildLimits[task.id] || 100"
                  track-progress
                  @toggle="setFolderOpen(task.id, $event)"
                  @filter="toggleFailedGroup(task.id)"
                  @more="showMoreGroupChildren(task.id)"
                  @retry="retryGroupChild(task, $event)"
                />
              </TransferDownloadRow>
              <div v-if="!activeDownloads.length" class="workspace-empty-state">
                <UiIcon name="download" :size="36" />
                <span class="wes-title">暂无正在下载的任务</span>
                <span class="wes-desc">在网盘中选中文件后点击「下载」，任务会显示在这里。</span>
              </div>
            </template>
          </div>
        </template>

        <!-- 已下载 -->
        <template v-else-if="menu === 'downloaded'">
          <div class="toppanbtns">
            <div class="toppanbtn">
              <button class="tbtn danger" :disabled="!doneDownloads.length" @click="askConfirm('确定清除所有已下载记录？', clearAllDoneDownloads, { danger: true, title: '清除记录' })"><UiIcon name="trash" :size="14" />清空已下载</button>
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
              <div v-for="t in renderedDoneDownloads" :key="t.id" class="taskrow" @contextmenu.prevent="onCtx($event, t)">
                <div class="rangselect"></div>
                <div class="fileicon" :class="'ft-' + iconOf({ name: t.name })"><UiIcon :name="iconOf({ name: t.name })" :size="20" /></div>
                <div class="filename">
                  <div :title="t.localPath || ''">{{ t.name || t.url || t.id }}</div>
                  <div class="fsub">{{ t.isDir ? `${t.completedFiles || 0} 个文件 · ${t.localPath || ''}` : t.localPath || '' }}<template v-if="t.updated"> · {{ formatTime(t.updated) }}</template></div>
                  <TransferFolderDetails
                    v-if="t.isDir && groupChildren(t.id).length"
                    :children="groupChildren(t.id)"
                    :open="openFolderIds.has(t.id)"
                    :failed-only="failedGroupOnly[t.id]"
                    :limit="groupChildLimits[t.id] || 100"
                    @toggle="setFolderOpen(t.id, $event)"
                    @filter="toggleFailedGroup(t.id)"
                    @more="showMoreGroupChildren(t.id)"
                    @retry="retryGroupChild(t, $event)"
                  />
                </div>
                <div class="filesize">{{ formatBytes(t.size) }}</div>
                <div class="downprogress">
                  <div class="transfering-state">
                    <p class="text-state">
                      <span>{{ downloadVerifications[t.id] ? (downloadVerifications[t.id].status === 'hash' ? '哈希已校验' : downloadVerifications[t.id].status === 'size' ? '大小一致' : '校验异常') : '已完成' }}</span>
                      <span>100%</span>
                    </p>
                    <div class="progress-total"><div class="progress-current" :class="downloadVerifications[t.id]?.status === 'mismatch' || downloadVerifications[t.id]?.status === 'missing' ? 'warning' : 'succeed'"></div></div>
                  </div>
                </div>
                <div class="downspeed"></div>
                <div class="tactions">
                  <button v-if="t.localPath && !t.isDir" class="btn-circle" :disabled="verifyingDownloadIds.has(t.id)" :title="downloadVerifications[t.id]?.detail || '校验本地文件完整性'" @click="verifyDownloadTask(t)">
                    <span v-if="verifyingDownloadIds.has(t.id)" class="spin"></span>
                    <UiIcon v-else name="check" :size="14" />
                  </button>
                  <button v-if="t.localPath && !t.isDir" class="btn-circle" title="打开文件" @click="openFile(t)"><UiIcon name="play" :size="14" /></button>
                  <button v-if="t.localPath" class="btn-circle" title="打开所在目录" @click="reveal(t)"><UiIcon name="folder" :size="14" /></button>
                  <button v-if="t.url" class="btn-circle" title="复制链接" @click="copyLink(t)">
                    <UiIcon v-if="copiedLinkMap[t.id]" name="check" :size="14" class="icon-check-pop" />
                    <UiIcon v-else name="link" :size="14" />
                  </button>
                  <button class="btn-circle danger" title="删除记录" @click="removeTask(t)"><UiIcon name="trash" :size="14" /></button>
                </div>
              </div>
            <div v-if="doneDownloads.length > renderedDoneDownloads.length" class="history-load-more">
              <button type="button" class="btn sm" @click="doneDownloadLimit += HISTORY_PAGE_SIZE">加载更多（已显示 {{ renderedDoneDownloads.length }} / {{ doneDownloads.length }}）</button>
            </div>
            <div v-if="!doneDownloads.length" class="workspace-empty-state">
              <UiIcon name="check" :size="36" />
              <span class="wes-title">暂无已下载记录</span>
              <span class="wes-desc">下载完成后会保留记录，可随时打开所在目录。</span>
            </div>
          </div>
        </template>

        <!-- 正在上传 -->
        <template v-else-if="menu === 'uploading'">
          <div class="toppanbtns">
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
            <TransferUploadRow
              v-for="task in activeUploads"
              :key="task.UploadID"
              :task="task"
              :name="upName(task)"
              :icon="iconOf({ name: upName(task) })"
              :size="upSize(task)"
              :needs-review="uploadNeedsReview(task)"
              @navigate="emit('navigate', $event)"
              @resume="resumeUploadTask"
              @review="reviewAndResumeUpload"
              @cancel="cancelUploadTask"
            />
            <div v-if="!activeUploads.length" class="workspace-empty-state">
              <UiIcon name="upload" :size="36" />
              <span class="wes-title">暂无正在上传的任务</span>
              <span class="wes-desc">在网盘工具栏点击「上传」选择本地文件或文件夹。</span>
            </div>
          </div>
        </template>

        <!-- 已上传 -->
        <template v-else-if="menu === 'uploaded'">
          <div class="toppanbtns">
            <div class="toppanbtn">
              <button class="tbtn danger" :disabled="!doneUploads.length" @click="askConfirm('确定清除所有已上传记录？', clearAllDoneUploads, { danger: true, title: '清除记录' })"><UiIcon name="trash" :size="14" />清空已上传</button>
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
              <div v-for="t in renderedDoneUploads" :key="t.UploadID" class="taskrow">
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
                    <div class="progress-total"><div class="progress-current succeed"></div></div>
                  </div>
                </div>
                <div class="downspeed"></div>
                <div class="tactions">
                  <button class="btn-circle danger" title="删除记录" @click="cancelUploadTask(t)"><UiIcon name="trash" :size="14" /></button>
                </div>
              </div>
            <div v-if="doneUploads.length > renderedDoneUploads.length" class="history-load-more">
              <button type="button" class="btn sm" @click="doneUploadLimit += HISTORY_PAGE_SIZE">加载更多（已显示 {{ renderedDoneUploads.length }} / {{ doneUploads.length }}）</button>
            </div>
            <div v-if="!doneUploads.length" class="workspace-empty-state">
              <UiIcon name="check" :size="36" />
              <span class="wes-title">暂无已上传记录</span>
              <span class="wes-desc">上传完成后会保留记录，失败任务可在此重试。</span>
            </div>
          </div>
        </template>

        <!-- 云离线 -->
        <template v-else-if="menu === 'offline'">
          <div class="toppanbtns">
            <div class="toppanbtn">
              <UiSelect
                v-model="offlineUser"
                aria-label="云离线账号"
                style="width:200px"
                :options="offlineAccounts.map((acc) => ({ value: acc.user_id, label: accLabel(acc) }))"
              />
              <button class="tbtn primary" :disabled="!offlineUser || !offlineDriveId || submittingOffline" @click="openOfflineModal"><UiIcon name="plus" :size="14" />新建离线任务</button>
            </div>
            <div class="toolbar-spacer"></div>
            <div class="toppanbtn">
              <button class="tbtn offline-refresh" :aria-busy="offlineLoading" @click="refreshOffline">
                <span class="offline-refresh-icon" aria-hidden="true"><span v-if="offlineLoading" class="spin"></span><UiIcon v-else name="refresh" :size="14" /></span>刷新
              </button>
            </div>
          </div>
          <div v-if="offlineError" class="transfer-error" role="alert">
            <UiIcon name="alert" :size="15" />
            <span>云离线任务刷新失败：{{ offlineError }}</span>
            <button type="button" class="transfer-error-retry" @click="refreshOffline">重试</button>
          </div>
          <div class="file-list">
              <div v-for="t in offlineTasks" :key="t.id || t.task_id" class="taskrow">
                <div class="rangselect"></div>
                <div class="fileicon" :class="'ft-' + iconOf({ name: t.file_name })"><UiIcon :name="iconOf({ name: t.file_name })" :size="20" /></div>
                <div class="filename">
                  <div>{{ t.file_name || t.url }}</div>
                  <div class="fsub">{{ t.url }}<template v-if="t.created"> · {{ formatTime(t.created) }}</template></div>
                </div>
                <div class="filesize">{{ t.file_size ? formatBytes(t.file_size) : '' }}</div>
                <div class="downprogress">
                  <div class="transfering-state">
                    <p class="text-state"><span :key="t.status" class="content-reveal">{{ offStatusText(t.status) }}</span><span>{{ offProgress(t) }}%</span></p>
                    <div class="progress-total" role="progressbar" :aria-label="`${t.file_name || '离线任务'}：${offStatusText(t.status)}`" :aria-valuenow="offProgress(t)">
                      <div :class="['progress-current', offProgressClass(t.status)]" :style="{ '--progress': offProgress(t) / 100 }"></div>
                    </div>
                    <TaskIssue :issue="t.issue" :reason="t.message" action="云离线下载" :user-id="t.user_id" />
                  </div>
                </div>
                <div class="downspeed"></div>
                <div class="tactions">
                  <button class="btn-circle danger" title="删除离线任务" @click="delOfflineTask(t)"><UiIcon name="trash" :size="14" /></button>
                </div>
              </div>
            <div v-if="!offlineTasks.length && !offlineLoading" class="workspace-empty-state">
              <UiIcon name="cloud-down" :size="36" />
              <span class="wes-title">暂无离线任务</span>
              <span class="wes-desc">选中文件后使用「离线下载」，网盘云端会代为拉取资源。</span>
            </div>
            <div v-if="offlineLoading && !offlineTasks.length" class="empty"><span class="spin"></span><span>加载中…</span></div>
          </div>
        </template>

        <!-- 迁移 -->
        <template v-else-if="menu === 'migrate'">
          <div class="toppanbtns">
            <div class="toppanbtn">
              <button class="tbtn danger" :disabled="!migrateJobs.some((j) => ['completed', 'partial', 'failed', 'canceled'].includes(j.status))" @click="askConfirm('确定清除已结束的迁移记录？', clearMigrateHistory, { danger: true, title: '清除迁移记录' })"><UiIcon name="trash" :size="14" />清除历史</button>
            </div>
            <div class="toolbar-spacer"></div>
            <div v-if="migrateLoading" class="toppanbtn"><span class="spin"></span></div>
          </div>
          <div class="file-list">
            <TransferMigrateRow
              v-for="job in visibleMigrateJobs"
              :key="job.id"
              :job="job"
              :source-name="migName(job.srcUser)"
              :source-icon="migIcon(job.srcUser)"
              :target-name="migName(job.dstUser)"
              :target-icon="migIcon(job.dstUser)"
              :status-text="migStatusText(job.status)"
              @details="detailID = $event"
              @repair="emit('repair', $event)"
              @cancel="cancelMigrateJob"
              @resume="resumeMigrateJob"
              @remove="removeMigrateJob"
            />
            <div v-if="!visibleMigrateJobs.length" class="workspace-empty-state">
              <UiIcon name="migrate" :size="36" />
              <span class="wes-title">{{ migrateJobs.length ? '该账号暂无迁移任务' : '暂无迁移任务' }}</span>
              <span class="wes-desc">在网盘中选中文件后使用「迁移」，可在不同网盘间搬移文件。</span>
            </div>
          </div>
        </template>
      </section>
    </div>
    <!-- 新建下载弹窗 -->
    <Modal v-if="dlModal" title="新建下载" width="480px" @close="closeDlModal">
      <div class="field">
        <label>下载链接（支持 HTTP/HTTPS，每行一个）</label>
        <textarea v-model="dlLinks" class="textarea" rows="5" :readonly="downloadingUrls" placeholder="https://example.com/file.zip"></textarea>
        <p v-if="dlInvalidCount" class="hint" role="alert">有 {{ dlInvalidCount }} 行链接无效，请检查后提交。</p>
        <p v-if="dlSubmitError" class="hint" role="alert">{{ dlSubmitError }}。下方操作只会提交保留的链接。</p>
      </div>
      <div class="field">
        <label>文件名（可选，仅单个链接时生效）</label>
        <input v-model="dlName" class="input" placeholder="留空则自动命名" :readonly="downloadingUrls" :disabled="dlUrlList.length !== 1" />
      </div>
      <div class="field">
        <label style="cursor:pointer;user-select:none" @click="dlAdvanced = !dlAdvanced">
          <UiIcon :name="dlAdvanced ? 'chevron-down' : 'chevron-right'" :size="12" /> 高级选项
        </label>
        <template v-if="dlAdvanced">
          <input v-model="dlHeaders.ua" class="input" :readonly="downloadingUrls" style="width:100%;margin-bottom:8px" placeholder="User-Agent（可选）" />
          <input v-model="dlHeaders.referer" class="input" :readonly="downloadingUrls" style="width:100%;margin-bottom:8px" placeholder="Referer（可选）" />
          <textarea v-model="dlHeaders.cookie" class="textarea" :readonly="downloadingUrls" rows="2" placeholder="Cookie（可选）" style="width:100%"></textarea>
        </template>
      </div>
      <template #actions>
        <button class="btn" @click="closeDlModal">{{ downloadingUrls ? '转到后台' : '取消' }}</button>
        <button class="btn primary" :disabled="!dlUrlList.length || !!dlInvalidCount" :aria-busy="downloadingUrls" :aria-disabled="downloadingUrls" @click="submitDownload">
          {{ downloadingUrls ? `正在添加 ${dlSubmitted}/${dlUrlList.length}…` : dlSubmitError ? '重试失败链接' : dlUrlList.length > 1 ? `开始下载 (${dlUrlList.length})` : '开始下载' }}
        </button>
      </template>
    </Modal>

    <!-- 新建离线任务弹窗 -->
    <Modal v-if="offModal" title="新建离线任务" width="480px" @close="closeOfflineModal">
      <div class="field">
        <label>离线链接（支持磁力链接、HTTP、ed2k，每行一个）</label>
        <textarea v-model="offLinks" class="textarea" rows="6" :readonly="submittingOffline" placeholder="magnet:?xt=urn:btih:…"></textarea>
        <p v-if="offInvalidCount" class="hint" role="alert">有 {{ offInvalidCount }} 行链接无效，请检查后提交。</p>
        <p v-if="offlineSubmitError" class="hint" role="alert">{{ offlineSubmitError }}。重试仅提交保留的链接。</p>
      </div>
      <template #actions>
        <button class="btn" @click="closeOfflineModal">{{ submittingOffline ? '转到后台' : '取消' }}</button>
        <button class="btn primary" :disabled="!offUrlList.length || !!offInvalidCount" :aria-busy="submittingOffline" :aria-disabled="submittingOffline" @click="submitOffline">
          {{ submittingOffline ? `正在提交 ${offlineSubmitted}/${offUrlList.length}…` : offlineSubmitError ? '重试失败链接' : offUrlList.length > 1 ? `提交 (${offUrlList.length})` : '提交' }}
        </button>
      </template>
    </Modal>

    <!-- 右键菜单 -->
    <ContextMenu
      v-if="ctx.show"
      :x="ctx.x" :y="ctx.y" :items="ctxItems"
      @close="ctx.show = false"
      @select="onCtxSelect"
    />

    <!-- 任务详情弹窗 -->
    <Modal v-if="detailTask" title="任务详情" width="440px" @close="detailTask = null">
      <div class="td-row"><span class="td-label">文件名</span><span class="td-val">{{ detailTask.name }}</span></div>
      <div class="td-row"><span class="td-label">任务 ID</span><span class="td-val">{{ detailTask.id }}</span></div>
      <div class="td-row"><span class="td-label">保存路径</span><span class="td-val">{{ detailTask.localPath || '-' }}</span></div>
      <div class="td-row"><span class="td-label">大小</span><span class="td-val">{{ formatBytes(detailTask.downloaded) }} / {{ formatBytes(detailTask.size) }}（{{ downProgress(detailTask) }}%）</span></div>
      <div class="td-row"><span class="td-label">状态</span><span class="td-val">{{ statusText(detailTask.status) }}</span></div>
      <div class="td-row" v-if="detailTask.speed"><span class="td-label">速度</span><span class="td-val">{{ formatSpeed(detailTask.speed) }}</span></div>
      <div class="td-row" v-if="detailTask.url"><span class="td-label">来源链接</span><span class="td-val td-url">{{ detailTask.url }}</span></div>
      <div class="td-row" v-if="detailTask.error"><span class="td-label">失败原因</span><span class="td-val error">{{ taskErrorText(detailTask.error, '下载文件') }}</span></div>
      <div class="td-row" v-if="detailTask.error"><span class="td-label">错误详情</span><span class="td-val">{{ detailTask.error }}</span></div>
      <TaskIssue :issue="detailTask.issue" :reason="detailTask.error" action="下载文件" :user-id="detailTask.user_id" />
      <div class="td-row"><span class="td-label">创建时间</span><span class="td-val">{{ formatTime(detailTask.created) }}</span></div>
      <template #actions>
        <button class="btn primary" @click="detailTask = null">关闭</button>
      </template>
    </Modal>

    <ConfirmModal v-if="confirmDialog" :title="confirmDialog.title" :message="confirmDialog.message" :okText="confirmDialog.okText" :danger="confirmDialog.danger" @ok="handleConfirmOk" @cancel="closeConfirm" />
  </div>
</template>

<style scoped>
.offline-refresh-icon { display:inline-grid; place-items:center; width:14px; height:14px; }
.taskrow:has(.task-notice) { height: auto; min-height: 68px; padding-block: 10px; }
.transfer-error {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 0 0 10px;
  padding: 8px 10px;
  border: 1px solid color-mix(in srgb, var(--color-error) 35%, transparent);
  border-radius: 8px;
  color: var(--color-error);
  background: color-mix(in srgb, var(--color-error) 8%, transparent);
  font-size: 12px;
}
.transfer-error span { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.transfer-error-retry {
  flex-shrink: 0;
  margin-left: auto;
  padding: 2px 8px;
  border: 1px solid currentColor;
  border-radius: 5px;
  color: inherit;
  background: transparent;
  font: inherit;
  cursor: pointer;
}
.transfer-error-retry:hover { background: color-mix(in srgb, currentColor 10%, transparent); }
.field { margin-bottom: 14px; }
.td-row { display: flex; align-items: baseline; gap: 12px; padding: 6px 0; font-size: 13.5px; border-bottom: 1px solid var(--border-lighter); }
.td-row:last-child { border-bottom: none; }
.td-label { width: 72px; flex-shrink: 0; color: var(--text-tertiary); font-size: 12.5px; font-weight: 600; }
.td-val { flex: 1; min-width: 0; color: var(--text-primary); word-break: break-all; user-select: text; }
.td-val.error { color: var(--color-error); }
.td-url { font-size: 12px; color: var(--text-link); }
.history-load-more { display: flex; justify-content: center; padding: 14px 0 8px; }
</style>
