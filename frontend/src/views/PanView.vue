<script setup>
import { ref, shallowRef, computed, watch, onMounted, onBeforeUnmount, onActivated, onDeactivated, nextTick, defineAsyncComponent } from 'vue'
import {
  listDir, listDirSilently, listTrash, search, mkdir, rename, trash, remove, restore,
  move, copy, createShare, uploadFiles, validateUploadFiles, migrateFiles, PreviewMigration, download, downloadFolder,
  AddFavorite, RemoveFavorite, ListFavorites, ListMigrateJobs, OfflineDownload, PickDirectory, PickFiles,
  formatBytes, formatTime, formatTimeParts, iconOf, extOf, openKindOf, copyText,
  capsOf, providerMetaOf, providerOf, accountName, providerIconUrl, GetDirectoryCache, SaveDirectoryCache, DeleteDirectoryCache, OpenBrowser, onEvent,
} from '../api'
import ContextMenu from '../components/ContextMenu.vue'
import DropdownBtn from '../components/DropdownBtn.vue'
import Modal from '../components/Modal.vue'
import SelectDirModal from '../components/SelectDirModal.vue'
import RenameMultiModal from '../components/RenameMultiModal.vue'
import ConfirmModal from '../components/ConfirmModal.vue'
import UiIcon from '../components/UiIcon.vue'
import UiSelect from '../components/UiSelect.vue'
import TreeNode from '../components/TreeNode.vue'
import FileColumns from '../components/FileColumns.vue'
import { compareFileNames, sortedDirectories } from '../file-order'
import DragDropZone from '../components/DragDropZone.vue'
import SharePrivacyNotice from '../components/SharePrivacyNotice.vue'
const DeletionHistory = defineAsyncComponent(() => import('../components/DeletionHistory.vue'))
const showDeletionHistory = ref(false)
async function openTrashWebsite() {
  try { await window.go.app.App.OpenTrashWebsite(uid.value, did.value) }
  catch (failure) { emit('toast', String(failure), 'error') }
}
import { getPrefs, setPref } from '../appearance'
import { createNavigationHistory } from '../navigation'
import { directoryCacheKey, recordAccountHealth } from '../workspace'
import { uniqueFileSelection, transferFiles, transferProblem, sameSpace } from '../file-transfer'
import { fileClipboard, clipboardBusy, setFileClipboard, clearFileClipboard, retainClipboardFiles, clipboardPasteSource, rememberClipboardCopy, clipboardMigrationEvent } from '../file-clipboard'
import { recordRecentItem } from '../recent-items'

const props = defineProps({
  account: Object,
  keyboardActive: { type: Boolean, default: true },
  locationKey: { type: String, default: '' },
  cloudDragEnabled: { type: Boolean, default: false },
  cloudDragActive: { type: Boolean, default: false },
  accounts: { type: Array, default: () => [] },
  providers: { type: Array, default: () => [] },
})
const emit = defineEmits(['toast', 'go', 'cloud-drag-start', 'cloud-drag-end', 'cloud-drop'])

// ---------- 状态 ----------
const mode = ref('list') // list | trash | search | favorite
const dirId = ref('root')
const pathStack = ref([]) // [{id,name}]
const files = shallowRef([])
const selected = ref([]) // file 对象数组
const focusId = ref('')  // 键盘焦点行
const selectionAnchorId = ref('')
const loading = ref(false)
const error = ref('')
const staleError = ref('')
const favoriteError = ref('')
const keyword = ref('')       // 全盘搜索关键词
const initialPrefs = getPrefs()
const sortKey = ref(['name', 'time', 'size'].includes(initialPrefs.defaultSortKey) ? initialPrefs.defaultSortKey : 'name') // name | time | size
const sortAsc = ref(initialPrefs.defaultSortAsc !== false)
const sortLabel = computed(() => ({ name: '名称', time: '修改时间', size: '大小' }[sortKey.value] + '·' + (sortAsc.value ? '升' : '降')))
const sortMenuItems = computed(() => [
  { header: '排序方式' },
  { icon: sortKey.value === 'name' ? 'check' : '', label: '名称', action: 'key name' },
  { icon: sortKey.value === 'size' ? 'check' : '', label: '大小', action: 'key size' },
  { icon: sortKey.value === 'time' ? 'check' : '', label: '修改时间', action: 'key time' },
  { sep: true },
  { header: '方向' },
  { icon: sortAsc.value ? 'check' : '', label: '升序', action: 'dir asc' },
  { icon: !sortAsc.value ? 'check' : '', label: '降序', action: 'dir desc' },
])
function onSortPick(action) {
  const [kind, v] = action.split(' ')
  if (kind === 'key') sortKey.value = v
  else if (kind === 'dir') sortAsc.value = v === 'asc'
}

// 区间选择模式：开启后点两个行选定区间
const rangIsSelecting = ref(false)
const rangAnchor = ref('')
const viewMode = ref(initialPrefs.viewMode || 'list')  // list | grid | columns
const sideWidth = ref(initialPrefs.sideWidth || 220) // 侧边栏宽度
const isSideResizing = ref(false)
let sideResizing = null
function sideDown(e) {
  if (e.button !== 0) return
  e.preventDefault()
  sideResizing = { x: e.clientX, w: sideWidth.value }
  isSideResizing.value = true
  document.body.style.cursor = 'col-resize'
  document.body.style.userSelect = 'none'
}
function sideMove(e) {
  if (!sideResizing) return
  const w = Math.max(180, Math.min(420, sideResizing.w + (e.clientX - sideResizing.x)))
  sideWidth.value = w
}
function sideUp() {
  if (!sideResizing) return
  sideResizing = null
  isSideResizing.value = false
  document.body.style.cursor = ''
  document.body.style.userSelect = ''
  setPref('sideWidth', sideWidth.value)
}
const menu = ref(null)
const favorites = ref([])
const favExpanded = ref(false)

// 目录树
const tree = ref({})          // id -> 子目录数组
const treeNames = ref({})     // id -> name
const treeParents = ref({})   // id -> parent id
const expanded = ref({})
const treeSelected = ref('root')
const columnSnapshots = shallowRef({})
const columnBrowser = ref(null)
const columnErrors = ref({})
let columnLoadSeq = 0

// 弹窗
const modal = ref(null) // mkdir | rename | renamemulti | share | upload | offline | migrate | movedir | copydir | preview | detail | download
const modalBusy = ref(false)
const modalFile = ref(null)
// confirm dialog state: { message, okText, danger, onOk }
const confirmDialog = ref(null)
function askConfirm(message, onOk, opts) {
  confirmDialog.value = { message, onOk, okText: opts?.okText || '确定', danger: opts?.danger || false, title: opts?.title || '确认操作' }
}
function closeConfirm() { confirmDialog.value = null }
function handleConfirmOk() {
  if (!confirmDialog.value) return
  const cb = confirmDialog.value.onOk
  closeConfirm()
  if (typeof cb === 'function') cb()
}
const inputText = ref('')
const shareForm = ref({ name: '', expiration: '', password: '' })
const migrateTarget = ref('') // 目标账号 user_id
const migrateDir = ref('root')
const migrateDirName = ref('根目录')
const migrateDirPick = ref(false)
const migrationPreview = ref(null), migrationChecking = ref(false)
async function checkMigration() {
  if (!migrateTargetAcc.value || migrationChecking.value) return
  const target = migrateTargetAcc.value, source = props.account, parent = migrateDir.value
  migrationChecking.value = true
  try {
    const result = await PreviewMigration(source.user_id, source.drive_id, target.user_id, target.drive_id, parent, (Array.isArray(modalFile.value) ? modalFile.value : [modalFile.value]).map(f => f.file_id))
    if (props.account?.user_id === source.user_id && migrateTarget.value === target.user_id && migrateDir.value === parent) migrationPreview.value = result
  } catch(e) { emit('toast', String(e), 'error') }
  finally { migrationChecking.value = false }
}
watch([migrateTarget, migrateDir, modalFile], () => { migrationPreview.value = null })

const uid = computed(() => (props.account ? props.account.user_id : ''))
const did = computed(() => (props.account ? props.account.drive_id : ''))
const cutClipboardIDs = computed(() => {
  const source = fileClipboard.value
  if (!source?.moving || source.account.user_id !== uid.value || source.account.drive_id !== did.value) return new Set()
  return new Set(source.files.map(file => file.file_id))
})
watch(() => props.accounts.map(account => account.user_id + '\0' + account.drive_id), accounts => {
  const source = fileClipboard.value?.account
  if (source && !accounts.includes(source.user_id + '\0' + source.drive_id)) fileClipboard.value = null
})
const caps = computed(() => capsOf(props.account, props.providers))
const uploadConflictPolicies = computed(() => Array.isArray(caps.value.uploadConflictPolicies) ? caps.value.uploadConflictPolicies : ['overwrite', 'rename', 'skip'])
const meta = computed(() => providerMetaOf(props.account, props.providers))
const defaultShareExpirationOptions = [
  { value: '', label: '永久有效' },
  { value: '1', label: '1 天' },
  { value: '7', label: '7 天' },
  { value: '30', label: '30 天' },
]
const shareExpirationOptions = computed(() => {
  const values = Array.isArray(caps.value.shareExpirationOptions)
    ? caps.value.shareExpirationOptions.map((v) => Number(v)).filter((v) => Number.isFinite(v) && v >= 0)
    : []
  if (!values.length) return defaultShareExpirationOptions
  return values.map((days) => days === 0
    ? { value: '', label: '永久有效' }
    : { value: String(days), label: days === 365 ? '1 年' : `${days} 天` })
})
function defaultShareExpiration() {
  return shareExpirationOptions.value[0]?.value ?? ''
}
function canReceiveMigration(account) {
  const targetCaps = capsOf(account, props.providers)
  return !!(targetCaps.upload || (Array.isArray(targetCaps.rapidUploadHashes) && targetCaps.rapidUploadHashes.length))
}
const migrateAccounts = computed(() => props.accounts.filter((a) => a.user_id !== uid.value && canReceiveMigration(a)))
const canMigrate = computed(() => !!caps.value.download && migrateAccounts.value.length > 0)

const rootKey = computed(() => meta.value.rootKey || 'root')
const rootTitle = computed(() => meta.value.rootTitle || '全部文件')

// ---------- 数据加载 ----------
// 目录缓存：按 provider、账号、存储、模式、目录和关键词隔离。
const dirCache = new Map() // key -> { files: File[], at: number }
const DIR_CACHE_MAX = 200
const DIR_CACHE_TTL_MS = 10 * 60 * 1000
const TREE_PREFETCH_LIMIT = 6
const TREE_PREFETCH_CONCURRENCY = 2
const TREE_PREFETCH_DELAY_MS = 180
const directoryRequests = new Map()
let prefetchTimer = null, prefetchQueue = [], prefetchActive = 0, prefetchReady = false
let viewDisposed = false
const cacheWrites = new Map()
function cacheKeyPart(value) { return encodeURIComponent(String(value ?? '')) }
function validDirectory(list) {
  if (!Array.isArray(list)) return false
  const ids = new Set()
  return list.every(file => {
    const id = file?.file_id
    if (typeof id !== 'string' || !id.trim() || ids.has(id)) return false
    ids.add(id)
    return true
  })
}
function dirCacheKey(uidV, didV, modeV, idV, kwV) {
  return directoryCacheKey(uidV, didV, modeV, idV, kwV)
}
function cacheDir(key, list) {
  if (!validDirectory(list)) return
  dirCache.set(key, { files: list || [], at: Date.now() })
  if (dirCache.size > DIR_CACHE_MAX) { const first = dirCache.keys().next().value; dirCache.delete(first) }
}
function getCachedDir(key) {
  const cached = dirCache.get(key)
  if (!cached) return null
  if (!validDirectory(cached.files) || Date.now() - cached.at > DIR_CACHE_TTL_MS) {
    dirCache.delete(key)
    return null
  }
  return cached
}
function queueCacheWrite(key, action) {
  const previous = cacheWrites.get(key) || Promise.resolve()
  const next = previous.catch(() => {}).then(action)
  cacheWrites.set(key, next)
  next.finally(() => {
    if (cacheWrites.get(key) === next) cacheWrites.delete(key)
  }).catch(() => {})
  return next
}
function persistDir(key, list, epoch = cacheEpoch) {
  return queueCacheWrite(key, () => {
    if (epoch !== cacheEpoch) return
    return SaveDirectoryCache(key, list || [])
  })
}
function isPersistableMode(modeV) { return modeV === 'list' }
function invalidateDirCache(uidV, didV, modeV, idV) {
  if (uidV === uid.value && didV === did.value && modeV === 'list') {
    const snapshots = { ...columnSnapshots.value }
    delete snapshots[idV]
    columnSnapshots.value = snapshots
  }
  if (uidV === uid.value && didV === did.value && modeV === mode.value && idV === dirId.value) {
    cacheEpoch++
    cancelDirectoryPrefetch()
  }
  // 变更后清掉该目录缓存，避免后台刷新前闪现旧数据
  const prefix = dirCacheKey(uidV, didV, modeV, idV, '')
  for (const k of dirCache.keys()) if (k.startsWith(prefix)) dirCache.delete(k)
  const key = dirCacheKey(uidV, didV, modeV, idV, '')
  queueCacheWrite(key, () => DeleteDirectoryCache(key)).catch(() => {})
}
let loadSeq = 0
let cacheEpoch = 0
let displayedContentKey = ''

// ---------- 滚动位置记忆 ----------
// 每个视图（模式+目录+关键词）记住自己的 scrollTop；返回时恢复，进入新目录归零。
const listEl = ref(null)
const scrollMemory = new Map()
let scrollSaveFrame = 0
let pendingScrollSeq = 0
const VIRTUAL_THRESHOLD = 200
const LIST_ROW_PITCH = 48
const VIRTUAL_OVERSCAN = 10
const virtualScrollTop = ref(0)
const virtualViewportHeight = ref(0)
const gridColumnCount = ref(1)
const gridRowPitch = ref(166)
let listResizeObserver = null

function currentViewKey() { return dirCacheKey(uid.value, did.value, mode.value, dirId.value, keyword.value) }

function updateVirtualMetrics(el = listEl.value) {
  if (!el) return
  virtualViewportHeight.value = el.clientHeight || 0
  if (!el.classList.contains('gridlist')) return
  const template = getComputedStyle(el).gridTemplateColumns || ''
  const tracks = template.match(/(?:^|\s)[\d.]+px(?=\s|$)/g)
  gridColumnCount.value = Math.max(1, tracks?.length || Math.floor(Math.max(0, el.clientWidth - 24) / 140) || 1)
  const card = el.querySelector('.griditem')
  if (card) {
    const gap = Number.parseFloat(getComputedStyle(el).rowGap || '8') || 8
    gridRowPitch.value = Math.max(1, card.getBoundingClientRect().height + gap)
  }
}

function onListScroll(e) {
  const el = e?.currentTarget || listEl.value
  if (el) virtualScrollTop.value = el.scrollTop
  if (scrollSaveFrame) return
  scrollSaveFrame = requestAnimationFrame(() => {
    scrollSaveFrame = 0
    if (listEl.value) scrollMemory.set(currentViewKey(), listEl.value.scrollTop)
  })
}

watch([loading, files], () => {
  if (loading.value || !pendingScrollSeq) return
  const seq = pendingScrollSeq
  nextTick(() => {
    if (seq !== loadSeq) return // 期间又跳转了，丢弃
    pendingScrollSeq = 0
    if (listEl.value) {
      const scrollTop = scrollMemory.get(currentViewKey()) || 0
      listEl.value.scrollTop = scrollTop
      virtualScrollTop.value = scrollTop
      updateVirtualMetrics()
    }
  })
})

async function load(id, options = {}) {
  if (!props.account) return
  cancelDirectoryPrefetch()
  const seq = ++loadSeq
  pendingScrollSeq = seq
  const epoch = cacheEpoch
  const snapUid = uid.value, snapDid = did.value, snapMode = mode.value, snapKw = keyword.value
  const ckey = dirCacheKey(snapUid, snapDid, snapMode, id, snapKw)
  if (displayedContentKey !== ckey) {
    selected.value = []
    focusId.value = ''
    selectionAnchorId.value = ''
  }
  // 快路径：有缓存先立即展示，后台静默刷新
  const cached = getCachedDir(ckey)
  let displayedCache = Boolean(cached)
  let networkDone = false
  if (cached) {
    files.value = cached.files
    displayedContentKey = ckey
    loading.value = false
    if (snapMode === 'list') updateTreeSnapshot(id, cached.files, snapUid, snapDid)
  }
  else {
    if (displayedContentKey !== ckey) {
      files.value = []
      displayedContentKey = ckey
    }
    loading.value = true
  }
  error.value = ''
  staleError.value = ''
  if (!cached && isPersistableMode(snapMode)) {
    // Persistent cache is a second fast path for a newly mounted page. It is
    // intentionally account/provider keyed and is ignored once fresh data
    // has arrived from the provider.
    GetDirectoryCache(ckey).then((list) => {
      if (seq !== loadSeq || epoch !== cacheEpoch || snapMode !== mode.value || networkDone || !validDirectory(list)) return
      displayedCache = true
      files.value = list
      displayedContentKey = ckey
      cacheDir(ckey, list)
      if (snapMode === 'list') updateTreeSnapshot(id, list, snapUid, snapDid)
      loading.value = false
    }).catch(() => {})
  }
  try {
    let list
    if (snapMode === 'trash') list = (await listTrash(snapUid, snapDid)) || []
    else if (snapMode === 'search') list = snapKw ? (await search(snapUid, snapDid, snapKw.trim())) || [] : []
    else list = await requestDirectory(snapUid, snapDid, id, epoch, options)
    networkDone = true
    // 时序保护：过期响应（账号/目录已切换或有更新请求）直接丢弃
    if (seq !== loadSeq || epoch !== cacheEpoch || snapMode !== mode.value || snapUid !== uid.value || snapDid !== did.value) return
    recordAccountHealth(snapUid)
    if (!validDirectory(list)) throw new Error('目录数据异常，请刷新重试')
    files.value = list
    displayedContentKey = ckey
    // 清理当前目录已不存在的缩略图错误标记，避免瞬时失败被永久记住
    const validIds = new Set(list.map((f) => f.file_id))
    const nextErr = {}
    for (const k in thumbErrors.value) if (validIds.has(k)) nextErr[k] = thumbErrors.value[k]
    thumbErrors.value = nextErr
    cacheDir(ckey, list)
    if (epoch === cacheEpoch && isPersistableMode(snapMode)) persistDir(ckey, list, epoch).catch(() => {})
    if (snapMode === 'list') {
      updateTreeSnapshot(id, list, snapUid, snapDid)
      scheduleDirectoryPrefetch(list)
    }
  } catch (e) {
    networkDone = true
    if (seq !== loadSeq || epoch !== cacheEpoch || snapMode !== mode.value || snapUid !== uid.value || snapDid !== did.value) return
    recordAccountHealth(snapUid, e)
    if (!displayedCache) {
      error.value = String(e)
      files.value = []
    } else staleError.value = String(e)
  }
  if (seq === loadSeq) loading.value = false
}

function updateTreeSnapshot(id, list, snapUid = uid.value, snapDid = did.value) {
  if (snapUid !== uid.value || snapDid !== did.value) return
  tree.value[id] = sortedDirectories(list || [])
  const snapshots = { ...columnSnapshots.value, [id]: list }
  columnSnapshots.value = Object.fromEntries(Object.entries(snapshots).slice(-DIR_CACHE_MAX))
  for (const f of tree.value[id]) {
    treeNames.value[f.file_id] = f.name
    treeParents.value[f.file_id] = id
  }
}

const STARTUP_DIRECTORY_RETRY_DELAY_MS = 1000

function requestDirectory(user, drive, id, epoch = cacheEpoch, options = {}) {
  const silent = options.silent === true
  // 静默启动读取与用户手动读取不共享失败请求：后者必须继续走统一错误提示。
  const key = `${silent ? 'silent' : 'interactive'}|${epoch}|${dirCacheKey(user, drive, 'list', id, '')}`
  if (directoryRequests.has(key)) return directoryRequests.get(key)
  const request = silent ? listDirSilently : listDir
  const pending = Promise.resolve().then(async () => {
    // 某些网盘在应用刚启动时仍在刷新令牌。仅自动读取短暂重试一次，既避免
    // 误报，也不会把真实的长期故障隐藏成无限请求。
    for (let attempt = 0; ; attempt++) {
      try {
        const result = await request(user, drive, id)
        const list = result || []
        if (!validDirectory(list)) throw new Error('目录数据异常，请刷新重试')
        return list
      } catch (cause) {
        if (!silent || attempt > 0 || epoch !== cacheEpoch || user !== uid.value || drive !== did.value) throw cause
        await new Promise(resolve => setTimeout(resolve, STARTUP_DIRECTORY_RETRY_DELAY_MS))
      }
    }
  })
  directoryRequests.set(key, pending)
  pending.finally(() => { if (directoryRequests.get(key) === pending) directoryRequests.delete(key) }).catch(() => {})
  return pending
}

function cancelDirectoryPrefetch() {
  clearTimeout(prefetchTimer)
  prefetchQueue = []
  prefetchReady = false
}

function scheduleDirectoryPrefetch(list) {
  cancelDirectoryPrefetch()
  if (viewDisposed || mode.value !== 'list') return
  const seen = new Set()
  const queue = []
  for (const file of list) {
    if (!file.isDir || !file.file_id || seen.has(file.file_id)) continue
    seen.add(file.file_id)
    if (getCachedDir(dirCacheKey(uid.value, did.value, 'list', file.file_id, ''))) continue
    queue.push({ id: file.file_id, user: uid.value, drive: did.value, epoch: cacheEpoch })
    if (queue.length === TREE_PREFETCH_LIMIT) break
  }
  prefetchQueue = queue
  if (!queue.length) return
  prefetchTimer = setTimeout(() => { prefetchReady = true; drainDirectoryPrefetch() }, TREE_PREFETCH_DELAY_MS)
}

function drainDirectoryPrefetch() {
  if (!prefetchReady || viewDisposed) return
  while (prefetchActive < TREE_PREFETCH_CONCURRENCY && prefetchQueue.length) {
    const item = prefetchQueue.shift()
    if (item.user !== uid.value || item.drive !== did.value || item.epoch !== cacheEpoch) continue
    prefetchActive++
    // A one-level warm-up only: results do not enqueue grandchildren.
    listDirectorySnapshot(item.id, { silent: true }).catch(() => {}).finally(() => {
      prefetchActive--
      drainDirectoryPrefetch()
    })
  }
}

async function listDirectorySnapshot(id, options = {}) {
  const snapUid = uid.value, snapDid = did.value
  const epoch = cacheEpoch
  const key = dirCacheKey(snapUid, snapDid, 'list', id, '')
  const inMemory = getCachedDir(key)
  if (inMemory) {
    updateTreeSnapshot(id, inMemory.files, snapUid, snapDid)
    return inMemory.files
  }
  const persisted = await GetDirectoryCache(key).catch(() => null)
  if (epoch !== cacheEpoch || snapUid !== uid.value || snapDid !== did.value) return []
  if (validDirectory(persisted)) {
    cacheDir(key, persisted)
    updateTreeSnapshot(id, persisted, snapUid, snapDid)
    return persisted
  }
  const list = await requestDirectory(snapUid, snapDid, id, epoch, options)
  if (epoch !== cacheEpoch || snapUid !== uid.value || snapDid !== did.value) return []
  cacheDir(key, list)
  if (epoch === cacheEpoch) persistDir(key, list, epoch).catch(() => {})
  updateTreeSnapshot(id, list, snapUid, snapDid)
  return list
}

let favoriteLoadSeq = 0
const favoritesLoaded = ref(false)
async function loadFavorites() {
  const seq = ++favoriteLoadSeq
  if (!props.account) { favorites.value = []; favoriteError.value = ''; return }
  const snapUid = uid.value, snapDid = did.value
  try {
    const list = (await ListFavorites(snapUid, snapDid)) || []
    if (!viewDisposed && seq === favoriteLoadSeq && snapUid === uid.value && snapDid === did.value) {
      favorites.value = list
      favoritesLoaded.value = true
      favoriteError.value = ''
    }
  } catch (e) {
    if (!viewDisposed && seq === favoriteLoadSeq && snapUid === uid.value && snapDid === did.value) favoriteError.value = String(e && e.message ? e.message : e)
  }
}

const displayFiles = computed(() => {
  const arr = [...files.value]
  const key = sortKey.value
  arr.sort((a, b) => {
    if (a.isDir !== b.isDir) return a.isDir ? -1 : 1
    let r = 0
    if (key === 'name') r = compareFileNames(a, b)
    else if (key === 'time') r = (a.time || 0) - (b.time || 0)
    else r = (a.size || 0) - (b.size || 0)
    return (sortAsc.value ? r : -r) || compareFileNames(a, b)
  })
  return arr
})

// 名称高亮：搜索关键词命中部分拆段（模板用 <mark> 渲染，无关键词时零开销）
const hlKeyword = computed(() => (mode.value === 'search' ? keyword.value.trim() : ''))
const thumbErrors = ref({})
// ref({}) 的属性赋值不是响应式的，必须整体替换才能让行模型重算
function markThumbError(id) { thumbErrors.value = { ...thumbErrors.value, [id]: true } }

function namePartsOf(name, kw) {
  if (!kw) return null
  const str = String(name || '')
  const i = str.toLowerCase().indexOf(kw.toLowerCase())
  if (i < 0) return null
  return [
    { text: str.slice(0, i), hit: false },
    { text: str.slice(i, i + kw.length), hit: true },
    { text: str.slice(i + kw.length), hit: false },
  ]
}

// 只格式化视口内的文件；大目录的其余文件不参与每次行模型计算。
function renderRows(list) {
  const kw = hlKeyword.value
  const errs = thumbErrors.value
  return list.map((f) => ({
    f,
    icon: iconOf(f),
    parts: kw ? namePartsOf(f.name, kw) : null,
    sizeText: f.isDir ? (f.file_count != null ? f.file_count + ' 项' : '-') : formatBytes(f.size),
    timeParts: formatTimeParts(f.time),
    thumb: !f.isDir && f.thumbnail && !errs[f.file_id] ? f.thumbnail : '',
  }))
}

const crumbs = computed(() => [{ id: rootKey.value, name: rootTitle.value }, ...pathStack.value])
const browserColumns = computed(() => {
  if (mode.value !== 'list') return [{ id: mode.value, name: modeTitle.value, files: listShown.value }]
  return crumbs.value.map((crumb, index) => ({ ...crumb,
    files: index === crumbs.value.length - 1 ? listShown.value : [...(columnSnapshots.value[crumb.id] || [])].sort((a, b) => Number(b.isDir) - Number(a.isDir) || compareFileNames(a, b)),
    activeId: crumbs.value[index + 1]?.id || '',
    loading: index < crumbs.value.length - 1 && !columnSnapshots.value[crumb.id] && !columnErrors.value[crumb.id],
    error: columnErrors.value[crumb.id] || '',
  }))
})
async function loadColumnAncestors() {
  const seq = ++columnLoadSeq
  if (viewMode.value !== 'columns' || mode.value !== 'list') return
  const account = uid.value, drive = did.value
  const ancestors = crumbs.value.slice(0, -1)
  for (const ancestor of ancestors) {
    if (seq !== columnLoadSeq || account !== uid.value || drive !== did.value) return
    if (columnSnapshots.value[ancestor.id]) continue
    try { await listDirectorySnapshot(ancestor.id) }
    catch (cause) {
      if (seq === columnLoadSeq && account === uid.value && drive === did.value) columnErrors.value = { ...columnErrors.value, [ancestor.id]: String(cause?.message || cause) }
    }
  }
}
watch([viewMode, () => JSON.stringify(pathStack.value), mode], loadColumnAncestors, { flush: 'post' })
async function retryColumn(id) {
  columnErrors.value = { ...columnErrors.value, [id]: '' }
  await loadColumnAncestors()
}
async function selectColumnFile(index, file, event, open = false, navigate = true) {
  const ancestor = mode.value === 'list' && index < crumbs.value.length - 1
  if (ancestor) {
    const parent = crumbs.value[index]
    pathStack.value = pathStack.value.slice(0, index)
    dirId.value = parent.id
    treeSelected.value = parent.id
    clearSelection()
    const user = uid.value, drive = did.value
    const pending = load(parent.id), seq = loadSeq
    await pending
    if (seq !== loadSeq || user !== uid.value || drive !== did.value || dirId.value !== parent.id || mode.value !== 'list' || error.value) return
    file = files.value.find(entry => entry.file_id === file.file_id)
    if (!file) return
  }
  toggleSel(file, event)
  if (open || (navigate && file.isDir && !event?.ctrlKey && !event?.metaKey && !event?.shiftKey)) onRowOpen(file)
}
async function contextColumnFile(index, event, file) {
  const user = uid.value, drive = did.value
  if (index < browserColumns.value.length - 1) await selectColumnFile(index, file, event, false, false)
  if (user === uid.value && drive === did.value && listShown.value.some(entry => entry.file_id === file.file_id)) onCtx(event, file)
}

const modeTitle = computed(() => ({
  trash: '回收站',
  favorite: '收藏',
  search: `搜索“${keyword.value}”`,
}[mode.value] || ''))

// ---------- 导航 ----------
function openDir(file) {
  pathStack.value.push({ id: file.file_id, name: file.name })
  dirId.value = file.file_id
  treeSelected.value = file.file_id
  mode.value = 'list'
  selected.value = []
  load(file.file_id).then(() => {
    if (!error.value && uid.value === props.account?.user_id && dirId.value === file.file_id) {
      recordRecentItem({ userId: uid.value, driveId: did.value, fileId: file.file_id, dirId: file.file_id, name: file.name, isDir: true, path: pathStack.value })
    }
  })
}

function goCrumb(i) {
  if (i === crumbs.value.length - 1) return
  pathStack.value = pathStack.value.slice(0, i)
  dirId.value = crumbs.value[i].id
  treeSelected.value = dirId.value
  mode.value = 'list'
  selected.value = []
  load(dirId.value)
}

function goUp() {
  if (!pathStack.value.length) return
  pathStack.value.pop()
  selected.value = []
  dirId.value = pathStack.value.length ? pathStack.value[pathStack.value.length - 1].id : rootKey.value
  treeSelected.value = dirId.value
  load(dirId.value)
}

function goHome(options = {}) {
  mode.value = 'list'
  pathStack.value = []
  dirId.value = rootKey.value
  treeSelected.value = dirId.value
  keyword.value = ''
  selected.value = []
  load(dirId.value, options)
}

const locationHistory = createNavigationHistory()
let historyAccount = ''
let restoringLocation = false
function locationSnapshot() {
  return { mode: mode.value, dirId: dirId.value, path: pathStack.value, keyword: mode.value === 'search' ? keyword.value : '', selected: treeSelected.value }
}
watch(() => [uid.value, did.value, mode.value, dirId.value, JSON.stringify(pathStack.value)], () => {
  const account = `${uid.value}/${did.value}`
  if (account !== historyAccount) {
    historyAccount = account
    locationHistory.reset(locationSnapshot())
  } else if (!restoringLocation) locationHistory.record(locationSnapshot())
}, { flush: 'post', immediate: true })
function navigateHistory(direction) {
  if (!props.account || restoringLocation) return false
  const destination = locationHistory.move(direction)
  if (!destination) return false
  restoringLocation = true
  mode.value = destination.mode
  dirId.value = destination.dirId
  pathStack.value = destination.path
  keyword.value = destination.keyword
  treeSelected.value = destination.selected
  selected.value = []
  focusId.value = ''
  menu.value = null
  suppressHoverPreview()
  refresh()
  nextTick(() => { restoringLocation = false })
  return true
}

function refresh() {
  if (mode.value === 'list') return load(dirId.value).then(loadColumnAncestors)
  if (mode.value === 'favorite') return loadFavorites()
  return load(null)
}

function showTrash() {
  mode.value = 'trash'
  selected.value = []
  treeSelected.value = 'trash'
  load()
}

function showFavorites() {
  loadSeq++
  cancelDirectoryPrefetch()
  mode.value = 'favorite'
  loading.value = false
  error.value = ''
  staleError.value = ''
  selected.value = []
  treeSelected.value = 'favorite'
  loadFavorites()
}

function favoriteFile(f) {
  return {
    size: 0, time: f.added, category: '', ...f.file,
    drive_id: f.drive_id, file_id: f.file_id, name: f.name, isDir: f.isDir, starred: true,
  }
}
const favoriteFiles = computed(() => favorites.value.map(favoriteFile))

// 搜索：Enter/按钮触发；Esc 清空并返回目录
function enterSearch() {
  if (!caps.value.search) return
  if (mode.value === 'search') {
    nextTick(() => document.getElementById('pan-search-' + props.locationKey)?.focus())
    return
  }
  loadSeq++
  cancelDirectoryPrefetch()
  mode.value = 'search'
  loading.value = false
  error.value = ''
  staleError.value = ''
  selected.value = []
  treeSelected.value = ''
  files.value = []
  nextTick(() => document.getElementById('pan-search-' + props.locationKey)?.focus())
}
function goSearch() {
  if (!keyword.value.trim()) return
  mode.value = 'search'
  selected.value = []
  treeSelected.value = ''
  load()
}
function onSearchKey(e) {
  if (e.key === 'Escape') { keyword.value = ''; if (mode.value === 'search') goHome(); e.target.blur() }
}

// ---------- 目录树 ----------
function treeChildren(id) { return tree.value[id] || [] }

// ---------- 位置持久化：切走再切回/重启后恢复到之前的目录 ----------
let locSaveTimer = null
function persistLocation() {
  const a = props.account
  if (!a || mode.value !== 'list') return
  clearTimeout(locSaveTimer)
  const location = {
    dirId: dirId.value,
    pathStack: [...pathStack.value],
    treeSelected: treeSelected.value,
    expanded: Object.keys(expanded.value).filter((key) => expanded.value[key]),
  }
  locSaveTimer = setTimeout(() => {
    const all = { ...(getPrefs().panLocations || {}) }
    all[a.user_id + props.locationKey] = location
    // 最多保留最近 20 个账号的位置
    const keys = Object.keys(all)
    if (keys.length > 20) for (const k of keys.slice(0, keys.length - 20)) delete all[k]
    setPref('panLocations', all)
  }, 400)
}

watch(() => [dirId.value, treeSelected.value, JSON.stringify(pathStack.value), JSON.stringify(expanded.value)], persistLocation)

async function toggleTree(idOrNode, name) {
  suppressHoverPreview()
  const id = typeof idOrNode === 'object' ? idOrNode.file_id : idOrNode
  expanded.value[id] = !expanded.value[id]
  const snapUid = uid.value, snapDid = did.value
  const epoch = cacheEpoch
  if (expanded.value[id] && !tree.value[id] && props.account) {
    try {
      const list = await listDirectorySnapshot(id)
      if (epoch !== cacheEpoch || snapUid !== uid.value || snapDid !== did.value) return
      updateTreeSnapshot(id, list, snapUid, snapDid)
      scheduleDirectoryPrefetch(list)
    } catch (e) {
      if (epoch === cacheEpoch && snapUid === uid.value && snapDid === did.value) {
        expanded.value[id] = false
        emit('toast', `目录树加载失败：${String(e && e.message ? e.message : e)}`, 'error')
      }
    }
  }
}

// 幂等展开（加载子目录），用于根节点默认展开
async function expandTree(id, name, options = {}) {
  if (expanded.value[id] && tree.value[id]) return
  expanded.value[id] = true
  const snapUid = uid.value, snapDid = did.value
  const epoch = cacheEpoch
  if (!tree.value[id] && props.account) {
    try {
      const list = await listDirectorySnapshot(id, options)
      if (epoch !== cacheEpoch || snapUid !== uid.value || snapDid !== did.value) return
      updateTreeSnapshot(id, list, snapUid, snapDid)
    } catch (e) {
      if (epoch === cacheEpoch && snapUid === uid.value && snapDid === did.value) {
        expanded.value[id] = false
        if (!options.silent) emit('toast', `目录树加载失败：${String(e && e.message ? e.message : e)}`, 'error')
      }
    }
  }
}

function selectTreeNode(idOrNode, name) {
  suppressHoverPreview()
  const id = typeof idOrNode === 'object' ? idOrNode.file_id : idOrNode
  if (typeof idOrNode === 'object') name = idOrNode.name
  treeSelected.value = id
  mode.value = 'list'
  selected.value = []
  if (id === rootKey.value) { pathStack.value = [] }
  else {
    const chain = []
    const seen = new Set()
    let current = id
    while (current && current !== rootKey.value && !seen.has(current)) {
      seen.add(current)
      chain.unshift({ id: current, name: treeNames.value[current] || (current === id ? name : current) })
      current = treeParents.value[current] || ''
    }
    pathStack.value = chain.length ? chain : [{ id, name }]
  }
  dirId.value = id
  load(id)
  // 双击跳转后展开该节点（加载子目录），便于在树中定位
  if (!expanded.value[id]) toggleTree(id, name)
}

// ---------- 选中 ----------
const selSet = computed(() => new Set(selected.value.map((s) => s.file_id)))
function isSel(f) { return selSet.value.has(f.file_id) }

// 在 listShown 中选中 [fromId, toId] 区间（含端点）
function selectRange(fromId, toId) {
  const list = listShown.value
  const a = list.findIndex((x) => x.file_id === fromId)
  const b = list.findIndex((x) => x.file_id === toId)
  if (a < 0 || b < 0) return
  const [lo, hi] = a < b ? [a, b] : [b, a]
  selected.value = list.slice(lo, hi + 1)
}

function toggleSel(f, e) {
  // 区间选择模式：第一次点定起点，第二次点选中区间并退出
  if (rangIsSelecting.value) {
    if (!rangAnchor.value) { rangAnchor.value = f.file_id; focusId.value = f.file_id; selectionAnchorId.value = f.file_id; return }
    selectRange(rangAnchor.value, f.file_id)
    rangIsSelecting.value = false
    rangAnchor.value = ''
    focusId.value = f.file_id
    return
  }
  // Shift+点击：以上次焦点为锚点选区间（标准桌面行为）
  const anchor = selectionAnchorId.value || focusId.value
  if (e && e.shiftKey && anchor && anchor !== f.file_id && listShown.value.some((item) => item.file_id === anchor)) {
    selectRange(anchor, f.file_id)
    focusId.value = f.file_id
    return
  }
  focusId.value = f.file_id
  selectionAnchorId.value = f.file_id
  if (e && (e.ctrlKey || e.metaKey)) {
    const i = selected.value.findIndex((s) => s.file_id === f.file_id)
    if (i >= 0) selected.value.splice(i, 1)
    else selected.value.push(f)
  } else {
    selected.value = [f]
  }
}

function clearSelection() {
  selected.value = []
  rangIsSelecting.value = false
  rangAnchor.value = ''
  selectionAnchorId.value = ''
}

function onListBackgroundClick(event) {
  if (!event.target.closest('.fileitem, .griditem')) clearSelection()
}

function toggleRangSelect() {
  rangIsSelecting.value = !rangIsSelecting.value
  rangAnchor.value = ''
}

const listShown = computed(() => mode.value === 'favorite' ? favoriteFiles.value : displayFiles.value)
watch(listShown, (list) => {
  const visible = new Map(list.map((file) => [file.file_id, file]))
  const next = selected.value.map((file) => visible.get(file.file_id)).filter(Boolean)
  if (next.length !== selected.value.length || next.some((file, index) => file !== selected.value[index])) selected.value = next
})

// 大目录只保留视口附近的行/卡片，数据排序、选择和事件仍使用完整 listShown。
// 仅改变渲染数量，不改变现有项目的 CSS 或尺寸。
const listVirtualized = computed(() => viewMode.value === 'list' && listShown.value.length > VIRTUAL_THRESHOLD)
const gridVirtualized = computed(() => viewMode.value === 'grid' && listShown.value.length > VIRTUAL_THRESHOLD)
function virtualRange(total, pitch) {
  if (!total) return { start: 0, end: 0 }
  const viewport = Math.max(virtualViewportHeight.value, pitch * 8)
  const start = Math.max(0, Math.floor(Math.max(0, virtualScrollTop.value) / pitch) - VIRTUAL_OVERSCAN)
  const end = Math.min(total, Math.max(start + 1, Math.ceil((virtualScrollTop.value + viewport) / pitch) + VIRTUAL_OVERSCAN))
  return { start, end }
}
const listWindow = computed(() => listVirtualized.value ? virtualRange(listShown.value.length, LIST_ROW_PITCH) : { start: 0, end: listShown.value.length })
const gridWindow = computed(() => {
  const total = listShown.value.length
  const cols = Math.max(1, gridColumnCount.value)
  if (!gridVirtualized.value) return { start: 0, end: total }
  const rows = virtualRange(Math.ceil(total / cols), gridRowPitch.value)
  return { start: rows.start * cols, end: Math.min(total, rows.end * cols) }
})
const listRenderRows = computed(() => renderRows(listShown.value.slice(listWindow.value.start, listWindow.value.end)))
const gridRenderRows = computed(() => renderRows(listShown.value.slice(gridWindow.value.start, gridWindow.value.end)))
const listVirtualTop = computed(() => listWindow.value.start * LIST_ROW_PITCH)
const listVirtualBottom = computed(() => Math.max(0, (listShown.value.length - listWindow.value.end) * LIST_ROW_PITCH))
const gridVirtualTop = computed(() => Math.floor(gridWindow.value.start / Math.max(1, gridColumnCount.value)) * gridRowPitch.value)
const gridVirtualBottom = computed(() => {
  const cols = Math.max(1, gridColumnCount.value)
  const totalRows = Math.ceil(listShown.value.length / cols)
  const renderedRows = Math.ceil((gridWindow.value.end - gridWindow.value.start) / cols)
  return Math.max(0, (totalRows - Math.floor(gridWindow.value.start / cols) - renderedRows) * gridRowPitch.value)
})
function revealRow(index) {
  const el = listEl.value
  if (!el || index < 0) return
  const isGrid = viewMode.value === 'grid'
  const virtualized = isGrid ? gridVirtualized.value : listVirtualized.value
  if (virtualized) {
    const row = isGrid ? Math.floor(index / Math.max(1, gridColumnCount.value)) : index
    const top = row * (isGrid ? gridRowPitch.value : LIST_ROW_PITCH)
    const height = isGrid ? gridRowPitch.value : LIST_ROW_PITCH
    if (top < el.scrollTop) el.scrollTop = top
    else if (top + height > el.scrollTop + el.clientHeight) el.scrollTop = Math.max(0, top + height - el.clientHeight)
    virtualScrollTop.value = el.scrollTop
  }
  nextTick(() => el.querySelector('.fileitem.focus, .griditem.focus')?.scrollIntoView({ block: 'nearest' }))
}
const allSelected = computed(() => listShown.value.length > 0 && selected.value.length === listShown.value.length)

function selectAll() { selected.value = [...listShown.value] }
function invertSel() { selected.value = listShown.value.filter((f) => !isSel(f)) }

const selectionMenuItems = computed(() => [
  { icon: 'check', label: '全选', action: 'all', disabled: allSelected.value },
  { icon: 'refresh', label: '反向选择', action: 'invert' },
  { icon: 'list', label: rangIsSelecting.value ? '退出区间选择' : '区间选择', action: 'range', disabled: !listShown.value.length },
  selected.value.length && { sep: true },
  selected.value.length && { icon: 'close', label: '取消选择', action: 'clear' },
].filter(Boolean))
function onSelectionPick(action) {
  if (action === 'all') selectAll()
  else if (action === 'clear') clearSelection()
  else if (action === 'range') toggleRangSelect()
  else if (action === 'invert') invertSel()
}

const selectedMoreItems = computed(() => [
  caps.value.download && selected.value.some(file => file.isDir) && { icon: 'download', label: '下载（跳过同名文件夹）', action: 'download-skip' },
  { icon: 'star', label: selected.value.every(isFav) ? '移出收藏' : '加入收藏', action: 'favorite' },
  mode.value === 'list' && { icon: 'copy', label: '复制到剪贴板', action: 'clipboard-copy' },
  mode.value === 'list' && { icon: 'move', label: '剪切到剪贴板', action: 'clipboard-cut' },
  mode.value === 'list' && fileClipboard.value && { icon: 'copy', label: '粘贴到当前目录', action: 'clipboard-paste' },
  mode.value === 'list' && fileClipboard.value && { icon: 'close', label: '清空文件剪贴板', action: 'clipboard-clear' },
  caps.value.move && { icon: 'move', label: '移动', action: 'move' },
  caps.value.copy && { icon: 'copy', label: '复制', action: 'copy' },
].filter(Boolean))
function onSelectedMorePick(action) {
  if (action === 'favorite') toggleFav(selected.value[0])
  else if (action === 'download-skip') doDownload(null, true)
  else if (action === 'clipboard-copy') copyToFileClipboard(false)
  else if (action === 'clipboard-cut') copyToFileClipboard(true)
  else if (action === 'clipboard-paste') pasteFiles()
  else if (action === 'clipboard-clear') clearFileClipboard()
  else if (action === 'move') { modalFile.value = [...selected.value]; modal.value = 'movedir' }
  else if (action === 'copy') { modalFile.value = [...selected.value]; modal.value = 'copydir' }
}

// ---------- 打开 ----------
async function openFile(file) {
  if (file.isDir) { openDir(file); return }
  if (mode.value === 'trash') { emit('toast', '回收站中的文件无法打开', 'error'); return }
  const kind = openKindOf(file, caps.value)
  if (kind === 'download') {
    askConfirm(`“${file.name}”暂不支持在线预览，是否下载到本地查看？`, () => doDownload([file]), { okText: '下载', title: '暂不支持预览' })
  } else {
    try {
      const accountId = uid.value, driveId = did.value, parentId = file.parent_file_id || dirId.value
      const openedPath = mode.value === 'list' && parentId === dirId.value
        ? pathStack.value.map(part => ({ ...part })) : [{ id: parentId, name: '所在目录' }]
      const prefs = getPrefs()
      await window.go.app.App.OpenPreviewWindow({
        account: { user_id: uid.value, drive_id: did.value },
        file, files: listShown.value, capabilities: caps.value, kind,
        preferences: { ...Object.fromEntries(['defaultVolume', 'defaultSpeed', 'seekStep', 'autoCloseOnEnd', 'autoLoadSubtitles', 'oledBackground', 'themeStyle'].map(key => [key, prefs[key]])), theme: document.documentElement.classList.contains('dark') ? 'dark' : 'light' },
      })
      recordRecentItem({ userId: accountId, driveId, fileId: file.file_id, dirId: parentId, name: file.name, isDir: false, path: openedPath })
    } catch (error) { emit('toast', `打开预览窗口失败：${String(error)}`, 'error') }
  }
}

// ---------- 操作 ----------
function selIds() { return selected.value.map((f) => f.file_id) }

let running = false
async function run(fn, okMsg) {
  if (running) return false
  running = true
  const snapUid = uid.value, snapDid = did.value, snapMode = mode.value, snapDir = dirId.value
  try {
    await fn()
    if (okMsg) emit('toast', okMsg, 'success')
    invalidateDirCache(snapUid, snapDid, snapMode, snapDir)
    if (snapUid === uid.value && snapDid === did.value) {
      const sameView = snapMode === mode.value && snapDir === dirId.value
      if (sameView) refresh()
      if (!sameView || snapMode !== 'favorite') loadFavorites()
    }
    return true
  } catch (e) {
    emit('toast', String(e), 'error')
    return false
  } finally {
    running = false
  }
}

// 下载：直接入队，停留当前页 + toast，不弹确认框、不跳转传输页
async function doDownload(list, skipExisting = false) {
  const targets = uniqueFileSelection(list || selected.value)
  if (!targets.length) { emit('toast', '请选择要下载的文件或文件夹', 'error'); return }
  if (running) return
  running = true
  const snapUid = uid.value, snapDid = did.value
  let completed = 0
  let skipped = 0
  let firstError = null
  try {
    for (const file of targets) {
      try {
        if (file.isDir && skipExisting) {
          if (await downloadFolder(snapUid, snapDid, file, true)) skipped++
          else completed++
        } else {
          await download(snapUid, snapDid, file)
          completed++
        }
      } catch (error) { firstError ||= error }
    }
    if (firstError) emit('toast', `已加入下载队列 ${completed}/${targets.length} 项${skipped ? `，跳过 ${skipped} 项` : ''}；失败：${String(firstError)}`, 'error')
    else emit('toast', `已加入下载队列 ${completed} 项${skipped ? `，跳过同名文件夹 ${skipped} 项` : ''}`, 'success')
  } finally {
    running = false
  }
}

function targets(file) {
  return file && isSel(file) && selected.value.length > 1 ? selected.value : [file]
}

// ---------- 收藏（后端按网盘能力选择云端或本地） ----------
const favoriteIDs = computed(() => new Set(favorites.value.map(file => file.file_id)))
function isFav(file) { return !!file && (favoriteIDs.value.has(file.file_id) || ((!favoritesLoaded.value || favoriteError.value) && !!file.starred)) }
function removingFavorites(file) { return targets(file).filter(Boolean).every(isFav) }

async function toggleFav(file) {
  const list = [...targets(file)].filter(Boolean)
  if (running || !list.length) return
  const snapUid = uid.value, snapDid = did.value
  const removing = list.every(isFav)
  let completed = 0
  running = true
  favoriteLoadSeq++
  try {
    for (const f of list) {
      if (removing) await RemoveFavorite(snapUid, snapDid, f.file_id)
      else await AddFavorite(snapUid, snapDid, { file_id: f.file_id, name: f.name, isDir: !!f.isDir, user_id: snapUid, drive_id: snapDid, file: f })
      completed++
    }
    emit('toast', removing ? '已移出收藏' : '已加入收藏', 'success')
  } catch (e) {
    emit('toast', `${completed ? `已完成 ${completed}/${list.length} 项，` : ''}${String(e)}`, 'error')
  } finally {
    if (!viewDisposed && snapUid === uid.value && snapDid === did.value) await loadFavorites()
    running = false
  }
}

// ---------- 右键菜单 ----------
function onCtx(e, file) {
  // 标准文件管理器行为：右键未选中项时先将其变为唯一选中并聚焦，
  // 避免菜单作用于一个没有任何高亮的文件（ targets() 据此取操作对象）
  if (file && file.file_id && !isSel(file)) {
    selected.value = [file]
    focusId.value = file.file_id
    selectionAnchorId.value = file.file_id
  }
  if (mode.value === 'trash') {
    menu.value = {
      x: e.clientX, y: e.clientY, file,
      items: [
        caps.value.trashRestore ? { icon: 'restore', label: '还原', action: 'restore' } : null,
        caps.value.permanentDelete ? { icon: 'x-circle', label: '彻底删除', danger: true, action: 'delete' } : null,
      ].filter(Boolean),
    }
    return
  }
  const list = [
    caps.value.download && { icon: 'download', label: '下载', action: 'download' },
    caps.value.download && targets(file).some(item => item.isDir) && { icon: 'download', label: '下载（跳过同名文件夹）', action: 'download-skip' },
    caps.value.createShare && { icon: 'share', label: '分享', action: 'share' },
    { icon: 'star', label: removingFavorites(file) ? '移出收藏' : '加入收藏', action: 'fav' },
    { sep: true },
    mode.value === 'list' && { icon: 'copy', label: '复制到剪贴板', action: 'clipboard-copy' },
    mode.value === 'list' && { icon: 'move', label: '剪切到剪贴板', action: 'clipboard-cut' },
    mode.value === 'list' && fileClipboard.value && { icon: 'copy', label: '粘贴到当前目录', action: 'clipboard-paste' },
    caps.value.move && { icon: 'move', label: '移动到…', action: 'move' },
    caps.value.copy && { icon: 'copy', label: '复制到…', action: 'copy' },
    canMigrate.value && { icon: 'migrate', label: '迁移到其他网盘…', action: 'migrate' },
    caps.value.rename && selected.value.length === 1 && { icon: 'pencil', label: '重命名', action: 'rename' },
    caps.value.rename && selected.value.length > 1 && { icon: 'pencil', label: `批量重命名 (${selected.value.length})`, action: 'renamemulti' },
    { sep: true },
    caps.value.recycleBin && mode.value !== 'trash' && { icon: 'trash', label: '移入回收站', danger: true, action: 'trash' },
    caps.value.permanentDelete && { icon: 'x-circle', label: '彻底删除', danger: true, action: 'delete' },
    { sep: true },
    { icon: 'copy', label: '复制文件名', action: 'copyname' },
    { icon: 'info', label: '属性', action: 'detail' },
  ].filter(Boolean)
  menu.value = { x: e.clientX, y: e.clientY, file, items: list }
}

function onMenuSelect(action) {
  const file = menu.value.file
  const list = targets(file)
  const ids = list.map((f) => f.file_id)
  menu.value = null
  switch (action) {
    case 'treeopen': selectTreeNode(file.file_id, file.name); break
    case 'treerefresh': {
      delete tree.value[file.file_id]
      expanded.value[file.file_id] = false
      if (treeSelected.value === file.file_id || dirId.value === file.file_id) load(file.file_id)
      break
    }
    case 'download': doDownload(list); break
    case 'download-skip': doDownload(list, true); break
    case 'clipboard-copy': copyToFileClipboard(false, list); break
    case 'clipboard-cut': copyToFileClipboard(true, list); break
    case 'clipboard-paste': pasteFiles(); break
    case 'open': openFile(file); break
    case 'share': openShareDialog(list); break
    case 'fav': toggleFav(file); break
    case 'rename':
      if (list.length !== 1) { emit('toast', '单次只能重命名单个文件', 'error'); return }
      modalFile.value = file
      inputText.value = file.name
      modal.value = 'rename'
      break
    case 'renamemulti':
      modalFile.value = [...selected.value]
      modal.value = 'renamemulti'
      break
    case 'move': modalFile.value = list; modal.value = 'movedir'; break
    case 'copy': modalFile.value = list; modal.value = 'copydir'; break
    case 'migrate':
      if (!canMigrate.value) { emit('toast', '暂无支持接收迁移的目标网盘账号', 'error'); return }
      modalFile.value = list; migrateTarget.value = ''; migrateDir.value = 'root'; migrateDirName.value = '根目录'; migrateDirPick.value = false; modal.value = 'migrate'; break
    case 'trash': run(() => trash(uid.value, did.value, ids), '已移入回收站'); break
    case 'delete':
      askConfirm(`确定彻底删除选中的 ${list.length} 项？此操作不可撤销。`, () => run(() => remove(uid.value, did.value, ids), '已彻底删除'), { danger: true, title: '彻底删除' })
      break
    case 'restore': run(() => restore(uid.value, did.value, ids), '已还原'); break
    case 'copyname': copyText(file.name).then(() => emit('toast', '已复制文件名', 'success')); break
    case 'detail': modalFile.value = file; modal.value = 'detail'; break
  }
}

// ---------- 弹窗动作 ----------
async function doMkdir() {
  const name = inputText.value.trim()
  if (!name || modalBusy.value) return
  modalBusy.value = true
  try {
    if (await run(async () => {
      const r = await mkdir(uid.value, did.value, dirId.value, name)
      if (r && r.error) throw new Error(r.error)
    }, '文件夹已创建')) {
      modal.value = null
      inputText.value = ''
    }
  } finally { modalBusy.value = false }
}

async function doRename() {
  const name = inputText.value.trim()
  if (!name || !modalFile.value || modalBusy.value) return
  const file = modalFile.value
  modalBusy.value = true
  try {
    if (await run(() => rename(uid.value, did.value, file.file_id, name), '已重命名')) {
      modal.value = null
      inputText.value = ''
    }
  } finally { modalBusy.value = false }
}

// 有效期选项（天数）转为绝对时间字符串，与后端 parseFlexibleTime 契约一致
function shareExpireAt(days) {
  const d = Number(days)
  if (!days || !Number.isFinite(d) || d <= 0) return undefined
  const t = new Date(Date.now() + d * 86400000)
  const p = (n) => String(n).padStart(2, '0')
  return `${t.getFullYear()}-${p(t.getMonth() + 1)}-${p(t.getDate())} ${p(t.getHours())}:${p(t.getMinutes())}:${p(t.getSeconds())}`
}

async function doShare() {
  const list = modalFile.value
  if (!list || modalBusy.value) return
  modalBusy.value = true
  try {
    if (await run(async () => {
      const item = await createShare(uid.value, did.value, {
        fileIds: list.map((f) => f.file_id),
        fileRefs: list.map((f) => ({ id: f.file_id, isDir: !!f.isDir })),
        shareName: shareForm.value.name,
        expiration: shareExpireAt(shareForm.value.expiration),
        password: shareForm.value.password || undefined,
      })
      const url = item && (item.share_url || item.share_msg || item.full_share_msg)
      const password = item && (item.share_pwd || shareForm.value.password)
      if (url) {
        const text = password ? `${url}\n提取码: ${password}` : url
        const copied = await copyText(text)
        emit('toast', copied ? '分享链接已复制到剪贴板' : '分享已创建，复制链接失败', copied ? 'success' : 'warn')
      } else {
        emit('toast', '分享已创建，未获取到链接', 'warn')
      }
    }, null)) {
      modal.value = null
    }
  } finally { modalBusy.value = false }
}

// 上传：原生文件/目录选择器（选择文件夹时后端递归入队），停留当前页 + toast
const uploadPickModal = ref(false) // 上传前的小弹窗：选文件还是文件夹
const conflictModal = ref(null) // { names, onPolicy }
function uploadDestination() { return { user: uid.value, drive: did.value, dir: dirId.value } }
function uploadDestinationMatches(destination) {
  return props.account && mode.value === 'list' && destination.user === uid.value && destination.drive === did.value && destination.dir === dirId.value
}

function copyToFileClipboard(moving, list = selected.value) {
  if (mode.value !== 'list' || !setFileClipboard({ ...snapshot(), label: accountName(props.account), files: list }, moving)) {
    emit('toast', '请选择文件；正在迁移时无法复制', 'warn')
    return
  }
  emit('toast', `已${moving ? '剪切' : '复制'} ${fileClipboard.value.files.length} 项，可切换目录或账号后粘贴`, 'success')
}

function pasteFiles(moveOverride = false) {
  const source = fileClipboard.value
  if (!source || clipboardBusy.value) return
  if (source.jobId) { emit('toast', '跨盘迁移已提交，请在传输页查看或继续任务', 'warn'); return }
  const moving = source.moving || moveOverride
  const target = snapshot()
  const pending = moving ? source : clipboardPasteSource(source, target)
  if (!pending.files.length) { emit('toast', '所选文件已在当前目录中', 'info'); return }
  const problem = transferProblem(pending, target, props.providers, moving)
  if (problem) { emit('toast', problem, 'warn'); return }
  const label = `${source.label || source.account.user_id} / ${pending.files.length} 项`
  const destination = `${accountName(props.account)} / ${crumbs.value.map(part => part.name).join(' / ')}`
  const conflict = sameSpace(source, target) ? '同名文件将由网盘直接处理或覆盖。' : '跨盘同名文件将自动重命名保留。'
  askConfirm(`${moving ? '移动' : '复制'} ${label} 到 ${destination}？${conflict}`, async () => {
    if (clipboardBusy.value || fileClipboard.value !== source) return
    clipboardBusy.value = true
    try {
      const result = await transferFiles(pending, target, props.providers, moving)
      if (result.queued) {
        fileClipboard.value = { ...source, moving, jobId: result.job?.id, jobStatus: 'running' }
        try {
          const jobs = await ListMigrateJobs()
          clipboardMigrationEvent(jobs?.find(job => job.id === result.job?.id))
        } catch { }
        emit('toast', '已提交跨盘迁移，可在传输页查看进度', 'success', { tab: 'transfer' })
      } else {
        if (moving) retainClipboardFiles(source, result.completed)
        else if (sameSpace(source, target)) rememberClipboardCopy(source, target, result.completed, true)
        emit('toast', `已${moving ? '移动' : '复制'}到目标目录`, 'success')
      }
      invalidateDirCache(source.account.user_id, source.account.drive_id, 'list', source.dirId)
      invalidateDirCache(target.account.user_id, target.account.drive_id, 'list', target.dirId)
      if (target.account.user_id === uid.value && target.dirId === dirId.value) refresh()
    } catch (error) {
      if (error.completedIDs?.length) {
        if (moving) retainClipboardFiles(source, error.completedIDs)
        else if (sameSpace(source, target)) rememberClipboardCopy(source, target, error.completedIDs)
      }
      if (error.completedIDs?.length) {
        invalidateDirCache(source.account.user_id, source.account.drive_id, 'list', source.dirId)
        invalidateDirCache(target.account.user_id, target.account.drive_id, 'list', target.dirId)
        if (target.account.user_id === uid.value && target.dirId === dirId.value) refresh()
      }
      emit('toast', error.completedIDs?.length ? `已完成 ${error.completedIDs.length} 项，剩余项目可在此目录重试：${String(error)}` : String(error), 'error')
    } finally { clipboardBusy.value = false }
  }, { title: '粘贴文件', okText: moving ? '移动' : '复制' })
}
function ensureUploadDestination(destination) {
  if (uploadDestinationMatches(destination)) return true
  emit('toast', '上传目标已切换，请重新选择文件', 'warn')
  return false
}
watch(() => [uid.value, did.value, dirId.value, mode.value], () => {
  if (conflictModal.value) conflictModal.value.onPolicy(null)
  uploadPickModal.value = false
})

async function checkUploadConflict(paths, destination) {
  const existing = new Set()
  try {
    const res = await listDir(destination.user, destination.drive, destination.dir)
    const remoteFiles = Array.isArray(res) ? res : res?.files
    if (!Array.isArray(remoteFiles)) throw new Error('目录数据格式不正确')
    for (const f of remoteFiles) existing.add(f.name)
  } catch (error) {
    if (uploadDestinationMatches(destination)) emit('toast', `无法检查同名文件，上传已取消：${String(error)}`, 'error')
    return null
  }
  if (!uploadDestinationMatches(destination)) return null
  const clashes = []
  for (const p of paths) {
    const name = p.split(/[/\\]/).pop()
    if (existing.has(name)) clashes.push(name)
  }
  if (clashes.length === 0) return uploadConflictPolicies.value.includes('overwrite') ? 'overwrite' : uploadConflictPolicies.value[0] || 'rename'
  return new Promise((resolve) => {
    conflictModal.value = {
      names: clashes,
      onPolicy: (policy) => { conflictModal.value = null; resolve(policy) },
    }
  })
}

async function validateUploadSelection(paths, destination) {
  try {
    await validateUploadFiles(destination.user, destination.drive, paths)
    return true
  } catch (e) {
    if (!uploadDestinationMatches(destination)) return ensureUploadDestination(destination)
    emit('toast', String(e), 'error')
    return false
  }
}

async function enqueueUpload(paths, destination, message) {
  if (!paths?.length || !ensureUploadDestination(destination)) return
  if (!await validateUploadSelection(paths, destination) || !ensureUploadDestination(destination)) return
  const policy = await checkUploadConflict(paths, destination)
  if (!ensureUploadDestination(destination) || !policy) return
  await run(() => uploadFiles(destination.user, destination.drive, destination.dir, policy, paths), message)
}

async function pickUploadFiles() {
  const destination = uploadDestination()
  let paths
  try { paths = await PickFiles('选择要上传的文件') } catch { return }
  if (paths?.length) await enqueueUpload(paths, destination, `已加入上传队列（${paths.length} 项）`)
}
async function pickUploadFolder() {
  const destination = uploadDestination()
  let dir
  try { dir = await PickDirectory('选择要上传的文件夹', '') } catch { return }
  if (dir) await enqueueUpload([dir], destination, '文件夹已加入上传队列')
}

async function doOffline() {
  const url = inputText.value.trim()
  if (!url || modalBusy.value) return
  modalBusy.value = true
  try {
    if (await run(() => OfflineDownload(uid.value, did.value, url, ''), '已提交云离线任务')) {
      modal.value = null
      inputText.value = ''
    }
  } finally { modalBusy.value = false }
}

async function onDirPicked(target) {
  const which = modal.value
  const list = modalFile.value
  modal.value = null
  if (!list) return
  const ids = list.map((f) => f.file_id)
  if (which === 'movedir') await run(() => move(uid.value, did.value, ids, target.id), '已移动')
  else if (which === 'copydir') await run(() => copy(uid.value, did.value, ids, target.id), '已复制')
}

const migrateTargetAcc = computed(() => props.accounts.find((a) => a.user_id === migrateTarget.value) || null)
watch(migrateTargetAcc, (account, previous) => {
  if (account?.user_id === previous?.user_id && account?.drive_id === previous?.drive_id) return
  const meta = providerMetaOf(account, props.providers)
  migrateDir.value = meta.rootKey || 'root'
  migrateDirName.value = meta.rootTitle || '根目录'
  migrateDirPick.value = false
})

async function doMigrate() {
  const targetAcc = props.accounts.find((a) => a.user_id === migrateTarget.value)
  if (!targetAcc) { emit('toast', '请选择目标账号', 'error'); return }
  if (!canReceiveMigration(targetAcc)) { emit('toast', '目标网盘不支持上传，无法迁移', 'error'); return }
  const list = modalFile.value
  if (!list || modalBusy.value) return
  modalBusy.value = true
  try {
    if (await run(
      () => migrateFiles(uid.value, did.value, targetAcc.user_id, targetAcc.drive_id, migrateDir.value || 'root', list.map((f) => f.file_id), false),
      '迁移任务已创建'
    )) {
      modal.value = null
    }
  } finally { modalBusy.value = false }
}

// ---------- 工具条分享 ----------
function openShareDialog(list) {
  if (!caps.value.combinedShare && list.length > 1) {
    emit('toast', '此网盘不支持批量分享，请单选文件', 'warn')
    return
  }
  shareForm.value = { name: list.length === 1 ? list[0].name : `${list.length} 个文件`, expiration: defaultShareExpiration(), password: '' }
  modalFile.value = [...list]
  modal.value = 'share'
}

function openShareModal() { openShareDialog(selected.value) }

function confirmDeleteSelected() {
  if (!selected.value.length) return
  const ids = selIds(), snapUid = uid.value, snapDid = did.value
  askConfirm(`确定彻底删除选中的 ${ids.length} 项？此操作不可撤销。`, () => run(() => remove(snapUid, snapDid, ids), '已彻底删除'), { danger: true, title: '彻底删除' })
}

// ---------- 收藏打开 ----------
function openFav(f) {
  if (f.file) f = favoriteFile(f)
  if (f.isDir) {
    mode.value = 'list'
    pathStack.value = [{ id: f.file_id, name: f.name }]
    dirId.value = f.file_id
    treeSelected.value = f.file_id
    selected.value = []
    load(f.file_id).then(() => {
      if (!error.value && dirId.value === f.file_id) recordRecentItem({ userId: uid.value, driveId: did.value, fileId: f.file_id, dirId: f.file_id, name: f.name, isDir: true, path: pathStack.value })
    })
  } else {
    openFile(f)
  }
}

function onRowOpen(f) {
  if (mode.value === 'favorite') openFav(f)
  else openFile(f)
}

// ---------- 目录树右键菜单 ----------
function onTreeCtx(e, node) {
  suppressHoverPreview()
  const file = { file_id: node.file_id || node.id, name: node.name, isDir: true }
  const isRoot = file.file_id === rootKey.value
  const items = [
    { icon: 'folder', label: '打开', action: 'treeopen' },
    { icon: 'refresh', label: '刷新', action: 'treerefresh' },
    { sep: true },
    caps.value.createShare && { icon: 'share', label: '分享', action: 'share' },
    { icon: 'star', label: '加入收藏', action: 'fav' },
    { sep: true },
    !isRoot && caps.value.rename && { icon: 'pencil', label: '重命名', action: 'rename' },
    !isRoot && caps.value.recycleBin && { icon: 'trash', label: '移入回收站', danger: true, action: 'trash' },
    { icon: 'info', label: '属性', action: 'detail' },
  ].filter(Boolean)
  menu.value = { x: e.clientX, y: e.clientY, file, items }
}

// ---------- 文件夹悬停预览 ----------
const hoverPreview = ref(null) // { id, name, x, y, items, loading }
let hoverTimer = null
let hoverSuppressUntil = 0 // 点击交互后的抑制期：展开/收起动画期间不弹预览

function suppressHoverPreview(ms = 700) {
  hoverSuppressUntil = Date.now() + ms
  clearTimeout(hoverTimer)
  hoverPreview.value = null
}

function onTreeEnter(e, node) {
  if (!getPrefs().hoverPreview) return
  if (Date.now() < hoverSuppressUntil) return
  clearTimeout(hoverTimer)
  const id = node.file_id || node.id
  const targetEl = e.currentTarget
  if (!targetEl) return
  const rect = targetEl.getBoundingClientRect()
  hoverTimer = setTimeout(async () => {
    const x = Math.min(rect.right + 8, window.innerWidth - 290)
    const y = Math.min(rect.top, window.innerHeight - 320)
    hoverPreview.value = { id, name: node.name, x, y, items: [], loading: true }
    try {
      const list = await listDir(uid.value, did.value, id)
      if (hoverPreview.value && hoverPreview.value.id === id) {
        hoverPreview.value.items = (list || []).slice(0, 8)
        hoverPreview.value.loading = false
      }
    } catch {
      if (hoverPreview.value && hoverPreview.value.id === id) { hoverPreview.value.items = []; hoverPreview.value.loading = false }
    }
  }, 450)
}

function onTreeLeave() {
  clearTimeout(hoverTimer)
  hoverTimer = setTimeout(() => { hoverPreview.value = null }, 200)
}

function onPreviewEnter() { clearTimeout(hoverTimer) }
function onPreviewLeave() { hoverPreview.value = null }

function openPreviewItem(f) {
  hoverPreview.value = null
  openFile(f)
}

// ---------- 快捷键 ----------
function onKey(e) {
  if (!props.keyboardActive || !pageActive || e.defaultPrevented || document.querySelector('[role="dialog"], .ctx-menu, .player-panel')) return
  const tag = (e.target.tagName || '').toLowerCase()
  if (tag === 'input' || tag === 'textarea' || tag === 'select' || tag === 'button' || e.target.isContentEditable) return
  if (!props.account || modal.value) return
  if (e.code === 'Escape' && (selected.value.length || rangIsSelecting.value)) { clearSelection(); e.preventDefault() }
  else if ((e.ctrlKey || e.metaKey) && e.code === 'KeyA') { selectAll(); e.preventDefault() }
  else if ((e.ctrlKey || e.metaKey) && e.code === 'KeyC' && mode.value === 'list' && selected.value.length) { copyToFileClipboard(false); e.preventDefault() }
  else if ((e.ctrlKey || e.metaKey) && e.code === 'KeyX' && mode.value === 'list' && selected.value.length) { copyToFileClipboard(true); e.preventDefault() }
  else if ((e.ctrlKey || e.metaKey) && e.code === 'KeyV' && mode.value === 'list' && fileClipboard.value) { pasteFiles(e.metaKey && e.altKey); e.preventDefault() }
  else if (e.code === 'F5') { refresh(); e.preventDefault() }
  else if (e.code === 'Backspace' && mode.value === 'list' && pathStack.value.length) { goUp(); e.preventDefault() }
  else if ((e.ctrlKey || e.metaKey) && !e.shiftKey && e.code === 'KeyF' && caps.value.search) { enterSearch(); e.preventDefault() }
  else if (e.ctrlKey && e.shiftKey && e.code === 'KeyN' && caps.value.createFolder) { inputText.value = ''; modal.value = 'mkdir'; e.preventDefault() }
  else if (e.ctrlKey && e.code === 'KeyU' && caps.value.upload && mode.value === 'list') { pickUploadFiles(); e.preventDefault() }
  else if (e.ctrlKey && e.code === 'KeyH') { goHome(); e.preventDefault() }
  else if (e.code === 'F2' && selected.value.length === 1 && caps.value.rename) {
    modalFile.value = selected.value[0]; inputText.value = selected.value[0].name; modal.value = 'rename'; e.preventDefault()
  }
  else if (e.code === 'Delete' && selected.value.length && caps.value.recycleBin && mode.value !== 'trash') {
    run(() => trash(uid.value, did.value, selIds()), '已移入回收站'); e.preventDefault()
  }
  else if (e.ctrlKey && e.shiftKey && e.code === 'KeyS' && selected.value.length && caps.value.createShare) { openShareModal(); e.preventDefault() }
  else if (e.code === 'Enter') {
    if (selected.value.length === 1) {
      onRowOpen(selected.value[0])
      e.preventDefault()
    } else if (focusId.value) {
      const f = listShown.value.find((x) => x.file_id === focusId.value)
      if (f) { onRowOpen(f); e.preventDefault() }
    }
  }
  else if (e.code === 'ArrowDown' || e.code === 'ArrowUp') {
    const list = listShown.value
    if (!list.length) return
    // move focus only (don't auto-select); Space toggles selection, Enter opens
    const cur = focusId.value ? list.findIndex((f) => f.file_id === focusId.value) : -1
    const start = cur >= 0 ? cur : (selected.value.length ? list.findIndex((f) => f.file_id === selected.value[selected.value.length - 1].file_id) : 0)
    const next = e.code === 'ArrowDown' ? Math.min(list.length - 1, start + 1) : Math.max(0, start - 1)
    focusId.value = list[next].file_id
    revealRow(next)
    e.preventDefault()
  }
  else if (e.code === 'Space' && focusId.value) {
    const f = listShown.value.find((x) => x.file_id === focusId.value)
    if (f) { toggleSel(f); e.preventDefault() }
  }
}

watch(() => [props.account?.user_id || '', props.account?.drive_id || '', rootKey.value], ([nextUid, nextDid]) => {
  const a = props.account
  ++columnLoadSeq
  columnSnapshots.value = {}
  columnErrors.value = {}
  showDeletionHistory.value = false
  confirmDialog.value = null
  menu.value = null
  modal.value = null
  cancelDirectoryPrefetch()
  cacheEpoch++
  loadSeq++
  files.value = []
  displayedContentKey = ''
  focusId.value = ''
  selectionAnchorId.value = ''
  selected.value = []
  error.value = ''
  staleError.value = ''
  favorites.value = []
  favoritesLoaded.value = false
  favoriteLoadSeq++
  favoriteError.value = ''
  if (!a) { loading.value = false; return }
  tree.value = {}
  expanded.value = {}
  treeParents.value = {}
  treeNames.value = {}
  thumbErrors.value = {}
  // 恢复该账号上次浏览位置；没有记录时回根目录
  const saved = (getPrefs().panLocations || {})[a.user_id + props.locationKey]
  if (saved && saved.dirId && saved.dirId !== rootKey.value) {
    mode.value = 'list'
    keyword.value = ''
    selected.value = []
    dirId.value = saved.dirId
    pathStack.value = Array.isArray(saved.pathStack) ? saved.pathStack : []
    treeSelected.value = saved.treeSelected || ''
    expanded.value = {}
    for (const id of saved.expanded || []) expanded.value[id] = true
    expanded.value[rootKey.value] = true
    // 预载展开节点的子目录，让树直接呈现上次的展开形态
    for (const id of Object.keys(expanded.value).slice(0, TREE_PREFETCH_LIMIT)) if (!tree.value[id]) expandTree(id, '', { silent: true })
    load(saved.dirId, { silent: true })
  } else {
    goHome({ silent: true })
    expanded.value[rootKey.value] = true
  }
  loadFavorites()
}, { immediate: true })

async function onDropUpload(paths) {
  if (!props.account || mode.value !== 'list' || !caps.value.upload) return
  if (paths?.length) await enqueueUpload(paths, uploadDestination(), `已加入上传队列（${paths.length} 项）`)
}

function openMkdirModal() {
  inputText.value = ''
  modal.value = 'mkdir'
}

function openUploadModal() {
  uploadPickModal.value = true
}

function snapshot() {
  return { account: props.account, dirId: dirId.value, path: crumbs.value, files: [...selected.value], mode: mode.value }
}
function startCloudDrag(event, file) {
  if (!props.cloudDragEnabled || mode.value !== 'list' || !file.file_id) { event.preventDefault(); return }
  if (!isSel(file)) selected.value = [file]
  emit('cloud-drag-start', snapshot(), event)
}
function dropCloudFiles(event, file = null) {
  if (!props.cloudDragActive) return
  event.preventDefault()
  event.stopPropagation()
  emit('cloud-drop', file?.isDir ? file : null)
}

async function invalidateDirectories(user, drive, ids = []) {
  const prefix = [providerOf(user), user, drive].map(cacheKeyPart).join('|') + '|'
  for (const key of dirCache.keys()) if (key.startsWith(prefix)) dirCache.delete(key)
  const active = user === uid.value && drive === did.value
  if (active) {
    cacheEpoch++
    columnLoadSeq++
    columnSnapshots.value = {}
    columnErrors.value = {}
    cancelDirectoryPrefetch()
    tree.value = {}
    treeParents.value = {}
    treeNames.value = {}
  }
  const directories = new Set([...ids, ...(active ? [dirId.value] : [])])
  await Promise.all([...directories].filter(Boolean).map(id => {
    const key = dirCacheKey(user, drive, 'list', id, '')
    return queueCacheWrite(key, () => DeleteDirectoryCache(key)).catch(() => {})
  }))
  if (active && user === uid.value && drive === did.value) return refresh()
}

defineExpose({
  snapshot,
  invalidateDirectories,
  navigate: async location => {
    mode.value = 'list'; selected.value = []; keyword.value = ''
    dirId.value = location.dirId || rootKey.value
    pathStack.value = Array.isArray(location.path) ? location.path.map(part => ({ id: part.id, name: part.name })) : dirId.value === rootKey.value ? [] : [{ id: dirId.value, name: location.name || '目录' }]
    await load(dirId.value)
    if (error.value) throw new Error(error.value)
    if (location.fileId && !location.isDir) {
      const file = files.value.find(entry => entry.file_id === location.fileId)
      if (!file) throw new Error('文件已移动或删除，无法打开')
      await openFile(file)
    } else if (location.file && !location.file.isDir) selected.value = files.value.filter(f => f.file_id === location.file.file_id)
  },
  navigateHistory,
  refresh,
  openMkdirModal,
  openUploadModal,
  clearCache: () => {
    cancelDirectoryPrefetch()
    cacheEpoch++
    loadSeq++
    columnLoadSeq++
    columnSnapshots.value = {}
    columnErrors.value = {}
    dirCache.clear()
    tree.value = {}
    expanded.value = {}
    treeParents.value = {}
    if (props.account) return refresh()
  },
})

watch(listEl, (el) => {
  listResizeObserver?.disconnect()
  listResizeObserver = null
  if (!el) return
  nextTick(() => {
    if (listEl.value !== el) return
    updateVirtualMetrics(el)
    if (typeof ResizeObserver === 'undefined') return
    listResizeObserver = new ResizeObserver(() => updateVirtualMetrics(el))
    listResizeObserver.observe(el)
  })
}, { flush: 'post' })
watch([listShown, viewMode], () => nextTick(() => updateVirtualMetrics()), { flush: 'post' })

let pageActive = false
let pageWasDeactivated = false
let stopPreviewSaved
let stopFavoritesChanged
let stopClipboardMigration
function activatePage() {
  if (pageActive) return
  pageActive = true
  window.addEventListener('keydown', onKey)
  window.addEventListener('mousemove', sideMove)
  window.addEventListener('mouseup', sideUp)
  if (pageWasDeactivated && props.account) {
    pageWasDeactivated = false
    load(dirId.value)
    loadFavorites()
  }
}
function deactivatePage() {
  cancelDirectoryPrefetch()
  pageWasDeactivated = true
  pageActive = false
  window.removeEventListener('keydown', onKey)
  window.removeEventListener('mousemove', sideMove)
  window.removeEventListener('mouseup', sideUp)
  sideUp()
  clearTimeout(hoverTimer)
  hoverPreview.value = null
  menu.value = null
}
onActivated(activatePage)
onDeactivated(deactivatePage)
onMounted(() => {
  stopClipboardMigration = onEvent('migrate:progress', clipboardMigrationEvent)
  stopFavoritesChanged = onEvent('favorites:changed', (event) => {
    if (!running && event?.user_id === uid.value && event?.drive_id === did.value) loadFavorites()
  })
  stopPreviewSaved = onEvent('preview:saved', (user, drive) => {
    if (user === uid.value && drive === did.value) refresh()
  })
  activatePage()
})
onBeforeUnmount(() => {
  viewDisposed = true
  ++columnLoadSeq
  cacheEpoch++
  cancelDirectoryPrefetch()
  clearTimeout(locSaveTimer)
  stopPreviewSaved?.()
  stopFavoritesChanged?.()
  stopClipboardMigration?.()
  deactivatePage()
  clearTimeout(hoverTimer)
  listResizeObserver?.disconnect()
  if (scrollSaveFrame) cancelAnimationFrame(scrollSaveFrame)
})
</script>

<template>
  <div class="page">
    <!-- 无账号空态 -->
    <div v-if="!account" class="workspace-empty-state" style="flex:1">
      <UiIcon name="drive" :size="40" style="opacity:.4" />
      <span class="wes-title">登录后查看文件</span>
      <button class="btn primary sm" @click="emit('go', 'login')">登录网盘账号</button>
    </div>

    <template v-else>
      <div class="pan-split">
        <!-- 左侧：快捷入口 + 收藏 + 目录树 -->
        <div class="pan-left" :style="{ width: sideWidth + 'px' }">
          <div class="tree-node" :class="{ active: mode === 'favorite' }" @click="showFavorites">
            <span class="tn-arrow" :class="{ open: favExpanded }" @click.stop="favExpanded = !favExpanded"><UiIcon v-if="favorites.length" name="chevron-right" :size="12" /></span>
            <UiIcon name="star" :size="14" /><span class="tn-label">收藏</span>
            <span class="tn-count" v-if="favorites.length">{{ favorites.length }}</span>
          </div>
          <div v-if="favExpanded" class="tree-children">
            <div
              v-for="f in favorites" :key="f.file_id"
              class="tree-node"
              :title="`${f.name} · ${f.source === 'cloud' ? '云端收藏' : '本地收藏'}`"
              @click="openFav(f)"
            >
              <span class="tn-arrow"></span><UiIcon :name="f.isDir ? 'folder' : iconOf(f)" :size="14" :class="f.isDir ? '' : 'ft-' + iconOf(f)" /><span class="tn-label">{{ f.name }}</span>
            </div>
            <div v-if="favoriteError" class="tree-load-error" role="alert">
              <span :title="favoriteError">收藏加载失败：{{ favoriteError }}</span>
              <button type="button" @click.stop="loadFavorites">重试</button>
            </div>
            <div v-else-if="!favorites.length" class="tree-empty">暂无收藏</div>
          </div>
          <div
            v-if="caps.trashView"
            class="tree-node"
            :class="{ active: mode === 'trash' }"
            @click="showTrash"
          ><span class="tn-arrow"></span><UiIcon name="trash" :size="14" /><span class="tn-label">回收站</span></div>
          <button
            v-else-if="caps.trashWebUrl"
            type="button"
            class="tree-node"
            title="此网盘不支持客户端回收站，前往官网管理"
            @click="openTrashWebsite"
          ><span class="tn-arrow"></span><UiIcon name="trash" :size="14" /><span class="tn-label">官网回收站</span></button>
          <button v-if="caps.localDeletionHistory" type="button" class="tree-node" @click="showDeletionHistory = true"><span class="tn-arrow"></span><UiIcon name="restore" :size="14" /><span class="tn-label">本机删除记录</span></button>
          <div
            class="tree-node"
            :class="{ active: mode === 'list' && treeSelected === rootKey }"
            @click="goHome"
            @contextmenu.prevent="onTreeCtx($event, { id: rootKey, name: rootTitle })"
          >
            <span class="tn-arrow" :class="{ open: expanded[rootKey] }" @click.stop="toggleTree(rootKey, rootTitle)"><UiIcon name="chevron-right" :size="12" /></span>
            <UiIcon name="cloud" :size="14" /><span class="tn-label">{{ rootTitle }}</span>
          </div>
          <div v-if="expanded[rootKey]" class="tree-children">
            <TreeNode
              v-for="d in treeChildren(rootKey)" :key="d.file_id"
              :node="d"
              :tree="tree"
              :expanded="expanded"
              :selected-id="mode === 'list' ? treeSelected : ''"
              @toggle="toggleTree"
              @select="selectTreeNode"
              @ctx="onTreeCtx"
              @enter="onTreeEnter"
              @leave="onTreeLeave"
            />
          </div>

        </div>
        <!-- 侧边栏拖拽手柄 -->
        <div class="pan-resizer" :class="{ resizing: isSideResizing }" @mousedown="sideDown"></div>

        <!-- 右侧文件区（支持桌面文件拖拽上传） -->
        <DragDropZone class="pan-right" @drop-files="onDropUpload" @drop="dropCloudFiles($event)">
          <!-- 面包屑路径条（置顶、矮） -->
          <div class="pathbar">
            <template v-if="mode === 'list'">
              <template v-for="(c, i) in crumbs" :key="c.id + i">
                <span v-if="i" class="crumb-sep">/</span>
                <span class="crumb" @click="goCrumb(i)">{{ c.name }}</span>
              </template>
            </template>
            <template v-else>
              <span class="crumb" @click="goHome">{{ rootTitle }}</span><span class="crumb-sep">/</span><span class="crumb">{{ modeTitle }}</span>
            </template>
          </div>

          <!-- 未选中时管理目录，选中后只显示选择操作。 -->
          <div class="toppanbtns pan-file-toolbar" :class="{ 'has-selection': selected.length }">
            <div class="toppanbtn" v-if="!selected.length && mode === 'list'">
              <button v-if="caps.createFolder" class="tbtn sm" title="新建文件夹 (Ctrl+Shift+N)" @click="inputText = ''; modal = 'mkdir'"><UiIcon name="plus" :size="14" />新建</button>
              <button v-if="caps.upload" class="tbtn sm" title="上传 (Ctrl+U)" @click="uploadPickModal = true"><UiIcon name="upload" :size="14" />上传</button>
              <button v-if="fileClipboard" class="tbtn sm" :disabled="clipboardBusy || !!fileClipboard.jobId" title="粘贴到当前目录 (Ctrl/⌘+V)" @click="pasteFiles"><UiIcon name="copy" :size="14" />粘贴 {{ fileClipboard.files.length }}</button>
              <button v-if="fileClipboard" class="tbtn icon" :disabled="clipboardBusy" title="清空剪贴板" @click="clearFileClipboard"><UiIcon name="close" :size="14" /></button>
            </div>
            <div v-if="!selected.length" class="toppanbtn">
              <button class="tbtn icon" :disabled="loading" title="刷新 (F5)" @click="refresh"><UiIcon name="refresh" :size="16" /></button>
              <button v-if="caps.search && mode !== 'search'" class="tbtn icon" title="全盘搜索 (Ctrl/⌘+F)" @click="enterSearch"><UiIcon name="search" :size="16" /></button>
              <button v-if="caps.offlineDownload && mode === 'list'" class="tbtn icon" title="云离线下载" @click="inputText = ''; modal = 'offline'"><UiIcon name="cloud-down" :size="16" /></button>
              <span v-if="mode === 'search' && caps.search" class="search-quick-wrap">
                <span class="sq-icon"><UiIcon name="search" :size="13" /></span>
                <input
                  :id="'pan-search-' + locationKey"
                  class="search-quick"
                  style="width:220px"
                  v-model="keyword"
                  placeholder="搜索文件 (Enter)"
                  @keydown.enter="goSearch"
                  @keydown="onSearchKey"
                />
                <button v-if="keyword" class="sq-clear" title="清空搜索" @click="keyword = ''; if (mode === 'search') goHome()"><UiIcon name="close" :size="11" /></button>
              </span>
            </div>
            <div v-if="selected.length || listShown.length" class="toppanbtn sel-mgmt">
              <DropdownBtn :label="selected.length ? `已选 ${selected.length} 项` : rangIsSelecting ? '区间选择中' : '选择'" icon="check" :items="selectionMenuItems" :class="{ 'selection-active': selected.length || rangIsSelecting }" title="选择" @select="onSelectionPick" />
            </div>
            <div class="toppanbtn" v-if="selected.length && mode !== 'trash'">
              <button v-if="caps.download" class="tbtn" @click="doDownload()"><UiIcon name="download" :size="15" />下载</button>
              <button v-if="caps.createShare" class="tbtn" title="分享 (Ctrl+Shift+S)" @click="openShareModal"><UiIcon name="share" :size="15" />分享</button>
              <button v-if="caps.recycleBin" class="tbtn danger" title="删除 (Delete)" @click="run(() => trash(uid, did, selIds()), '已移入回收站')"><UiIcon name="trash" :size="15" />删除</button>
              <DropdownBtn label="更多" icon="more-horizontal" :items="selectedMoreItems" title="更多" @select="onSelectedMorePick" />
            </div>
            <div class="toppanbtn" v-if="mode === 'trash' && selected.length">
              <button v-if="caps.trashRestore" class="tbtn" @click="run(() => restore(uid, did, selIds()), '已还原')"><UiIcon name="restore" :size="15" />还原</button>
              <button v-if="caps.permanentDelete" class="tbtn danger" @click="confirmDeleteSelected"><UiIcon name="x-circle" :size="15" />彻底删除</button>
            </div>
            <div v-if="!selected.length" class="toolbar-spacer"></div>
            <div v-if="!selected.length" class="toppanbtn">
              <DropdownBtn :label="sortLabel" :icon="sortAsc ? 'sort-asc' : 'sort-desc'" :items="sortMenuItems" title="排序方式" @select="onSortPick" />
              <div class="view-switch">
                <button class="vs-btn" :class="{ on: viewMode === 'list' }" title="列表视图" @click="viewMode = 'list'; setPref('viewMode', 'list')"><UiIcon name="list" :size="15" /></button>
                <button class="vs-btn" :class="{ on: viewMode === 'grid' }" title="网格视图" @click="viewMode = 'grid'; setPref('viewMode', 'grid')"><UiIcon name="grid" :size="15" /></button>
                <button class="vs-btn" :class="{ on: viewMode === 'columns' }" title="分栏视图" aria-label="分栏视图" @click="viewMode = 'columns'; setPref('viewMode', 'columns')"><UiIcon name="browse-columns" :size="15" /></button>
              </div>
            </div>
          </div>

          <div v-if="staleError" class="directory-stale" role="status" :title="staleError">
            <UiIcon name="warning" :size="14" />
            <span>刷新失败，当前显示为本地缓存</span>
            <button type="button" :disabled="loading" @click="refresh">重试</button>
          </div>

          <!-- 骨架屏加载状态 -->
          <div v-if="loading && !listShown.length" :class="viewMode === 'list' ? 'skeleton-list' : 'skeleton-grid'">
            <template v-if="viewMode === 'list'">
              <div v-for="i in 8" :key="i" class="skeleton-row">
                <div class="skeleton skeleton-icon"></div>
                <div class="skeleton skeleton-name" :style="{ width: (40 + (i * 7) % 35) + '%' }"></div>
                <div class="skeleton skeleton-size"></div>
                <div class="skeleton skeleton-date"></div>
              </div>
            </template>
            <template v-else>
              <div v-for="i in 10" :key="i" class="skeleton-card">
                <div class="skeleton skeleton-card-icon"></div>
                <div class="skeleton skeleton-card-text"></div>
                <div class="skeleton skeleton-card-sub"></div>
              </div>
            </template>
          </div>
          <div v-else-if="error" class="empty"><span class="empty-icon"><UiIcon name="warning" :size="30" /></span><span>{{ error }}</span><button class="btn sm" @click="refresh">重试</button><button v-if="mode === 'trash' && caps.trashWebUrl" class="btn sm" @click="openTrashWebsite">官网回收站</button></div>
          <div v-else-if="mode === 'favorite' && favoriteError && !favorites.length" class="empty"><span class="empty-icon"><UiIcon name="warning" :size="30" /></span><span>收藏加载失败：{{ favoriteError }}</span><button class="btn sm" @click="loadFavorites">重试</button></div>

          <FileColumns v-else-if="viewMode === 'columns'" ref="columnBrowser" :columns="browserColumns" :selected-ids="selSet" :focus-id="focusId" :drag-enabled="cloudDragEnabled && mode === 'list'" @select="selectColumnFile" @open="(index, file, event) => selectColumnFile(index, file, event, true)" @context="contextColumnFile" @clear="clearSelection" @retry="retryColumn" @drag-start="startCloudDrag" @drag-end="emit('cloud-drag-end')" @drop="dropCloudFiles" />

          <!-- 列表视图 -->
          <div v-else-if="viewMode === 'list'" ref="listEl" :key="'list:' + currentViewKey()" class="file-list" @click="onListBackgroundClick" @scroll.passive="onListScroll">
            <div v-if="listVirtualized" aria-hidden="true" :style="{ height: listVirtualTop + 'px' }"></div>
            <div
              v-for="r in listRenderRows"
              :key="r.f.file_id"
              class="fileitem"
              :draggable="cloudDragEnabled && mode === 'list'"
              @dragstart="startCloudDrag($event, r.f)"
              @dragend="emit('cloud-drag-end')"
              @drop="dropCloudFiles($event, r.f)"
              :class="{ selected: isSel(r.f), focus: focusId === r.f.file_id, 'anchor-node': rangIsSelecting && rangAnchor === r.f.file_id, 'clipboard-cut': cutClipboardIDs.has(r.f.file_id) }"
              @click="toggleSel(r.f, $event)"
              @dblclick="onRowOpen(r.f)"
              @contextmenu.prevent="onCtx($event, r.f)"
            >
              <div class="rangselect">
                <button class="btn-check" :class="{ on: isSel(r.f) }" tabindex="-1" @click.stop="toggleSel(r.f, { ctrlKey: true })">
                  <UiIcon v-if="isSel(r.f)" name="check" :size="11" />
                </button>
              </div>
              <div class="fileicon" :class="!r.thumb ? 'ft-' + r.icon : ''">
                <img v-if="r.thumb" :src="r.thumb" loading="lazy" :alt="r.f.name" @error="markThumbError(r.f.file_id)" />
                <UiIcon v-else :name="r.icon" :size="20" />
              </div>
              <div class="filename">
                <div :title="r.f.name">
                  <template v-if="r.parts">
                    <template v-for="(p, i) in r.parts" :key="i"><mark v-if="p.hit" class="hl">{{ p.text }}</mark><template v-else>{{ p.text }}</template></template>
                  </template>
                  <template v-else>{{ r.f.name }}</template>
                </div>
              </div>
              <span v-if="isFav(r.f)" class="fstar-mark"><UiIcon name="star" :size="13" /></span>
              <div class="filesize">{{ r.sizeText }}</div>
              <div class="filetime">
                <span class="filedate">{{ r.timeParts.date }}</span>
                <span class="fileclock">{{ r.timeParts.clock }}</span>
              </div>
            </div>
            <div v-if="listVirtualized" aria-hidden="true" :style="{ height: listVirtualBottom + 'px' }"></div>
            <div v-if="!listShown.length" class="workspace-empty-state">
              <UiIcon :name="mode === 'trash' ? 'trash' : mode === 'search' ? 'search' : mode === 'favorite' ? 'star' : 'folder'" :size="36" style="opacity:.4" />
              <span class="wes-title">{{ mode === 'trash' ? '回收站为空' : mode === 'search' ? '没有找到匹配文件' : mode === 'favorite' ? '暂无收藏' : '空目录' }}</span>
            </div>
          </div>

          <!-- 网格视图（旧版 griditem） -->
          <div v-else ref="listEl" :key="'grid:' + currentViewKey()" class="file-list gridlist" @click="onListBackgroundClick" @scroll.passive="onListScroll">
            <div v-if="gridVirtualized" aria-hidden="true" :style="{ gridColumn: '1 / -1', height: gridVirtualTop + 'px' }"></div>
            <div
              v-for="r in gridRenderRows"
              :key="r.f.file_id"
              class="griditem"
              :draggable="cloudDragEnabled && mode === 'list'"
              @dragstart="startCloudDrag($event, r.f)"
              @dragend="emit('cloud-drag-end')"
              @drop="dropCloudFiles($event, r.f)"
              :class="{ selected: isSel(r.f), focus: focusId === r.f.file_id, 'anchor-node': rangIsSelecting && rangAnchor === r.f.file_id, 'clipboard-cut': cutClipboardIDs.has(r.f.file_id) }"
              @click="toggleSel(r.f, $event)"
              @dblclick="onRowOpen(r.f)"
              @contextmenu.prevent="onCtx($event, r.f)"
            >
              <span class="gsel">
                <button class="btn-check" :class="{ on: isSel(r.f) }" tabindex="-1" @click.stop="toggleSel(r.f, { ctrlKey: true })">
                  <UiIcon v-if="isSel(r.f)" name="check" :size="11" />
                </button>
              </span>
              <span v-if="isFav(r.f)" class="gstar"><UiIcon name="star" :size="12" /></span>
              <div class="gridicon" :class="!r.thumb ? 'ft-' + r.icon : ''">
                <img v-if="r.thumb" :src="r.thumb" loading="lazy" alt="" @error="markThumbError(r.f.file_id)" />
                <UiIcon v-else :name="r.icon" :size="32" />
              </div>
              <div class="gridname" :title="r.f.name">
                <template v-if="r.parts">
                  <template v-for="(p, i) in r.parts" :key="i"><mark v-if="p.hit" class="hl">{{ p.text }}</mark><template v-else>{{ p.text }}</template></template>
                </template>
                <template v-else>{{ r.f.name }}</template>
              </div>
              <div class="gridinfo">{{ r.f.isDir ? '文件夹' : r.sizeText }}</div>
            </div>
            <div v-if="gridVirtualized" aria-hidden="true" :style="{ gridColumn: '1 / -1', height: gridVirtualBottom + 'px' }"></div>
            <div v-if="!listShown.length" class="workspace-empty-state" style="grid-column:1/-1">
              <UiIcon name="folder" :size="36" style="opacity:.4" />
              <span class="wes-title">{{ mode === 'trash' ? '回收站为空' : mode === 'search' ? '没有找到匹配文件' : mode === 'favorite' ? '暂无收藏' : '空目录' }}</span>
            </div>
          </div>
        </DragDropZone>
      </div>
    </template>

    <!-- 右键菜单 -->
    <ContextMenu v-if="menu" :x="menu.x" :y="menu.y" :items="menu.items" @close="menu = null" @select="onMenuSelect" />

    <!-- 文件夹悬停预览 -->
    <teleport to="body">
      <div
        v-if="hoverPreview"
        class="folder-preview"
        :style="{ left: hoverPreview.x + 'px', top: hoverPreview.y + 'px' }"
        @mouseenter="onPreviewEnter"
        @mouseleave="onPreviewLeave"
      >
        <div class="fp-head"><UiIcon name="folder" :size="13" /> {{ hoverPreview.name }}</div>
        <div v-if="hoverPreview.loading" class="fp-empty"><span class="spin"></span></div>
        <div v-else-if="!hoverPreview.items.length" class="fp-empty">空文件夹</div>
        <div
          v-else
          v-for="f in hoverPreview.items"
          :key="f.file_id"
          class="fp-item"
          @click="openPreviewItem(f)"
        >
          <UiIcon :name="iconOf(f)" :size="13" :class="'ft-' + iconOf(f)" />
          <span class="fp-name">{{ f.name }}</span>
          <span class="fp-size">{{ f.isDir ? '' : formatBytes(f.size) }}</span>
        </div>
      </div>
    </teleport>

    <!-- 上传前选择：文件 / 文件夹 -->
    <Modal v-if="uploadPickModal" title="上传" width="360px" @close="uploadPickModal = false">
      <div class="uppick">
        <button class="uppick-card" @click="uploadPickModal = false; pickUploadFiles()">
          <UiIcon name="file" :size="24" /><span class="uppick-name">上传文件</span><span class="uppick-desc">支持单选或多选文件</span>
        </button>
        <button class="uppick-card" @click="uploadPickModal = false; pickUploadFolder()">
          <UiIcon name="folder" :size="24" /><span class="uppick-name">上传文件夹</span><span class="uppick-desc">上传完整文件夹内容</span>
        </button>
      </div>
    </Modal>

    <!-- 新建文件夹 -->
    <Modal v-if="modal === 'mkdir'" title="新建文件夹" @close="modal = null">
      <div class="field">
        <label>文件夹名称</label>
        <input class="input" v-model="inputText" placeholder="输入文件夹名称" @keyup.enter="doMkdir" autofocus :disabled="modalBusy" />
      </div>
      <template #actions>
        <button class="btn" :disabled="modalBusy" @click="modal = null">取消</button>
        <button class="btn primary" :disabled="!inputText.trim() || modalBusy" @click="doMkdir">
          <span v-if="modalBusy" class="spin spin-on-primary"></span>
          {{ modalBusy ? '创建中…' : '创建' }}
        </button>
      </template>
    </Modal>

    <!-- 重命名 -->
    <Modal v-if="modal === 'rename'" title="重命名" @close="modal = null">
      <div class="field">
        <label>新名称</label>
        <input class="input" v-model="inputText" placeholder="输入新名称" @keyup.enter="doRename" autofocus :disabled="modalBusy" />
      </div>
      <template #actions>
        <button class="btn" :disabled="modalBusy" @click="modal = null">取消</button>
        <button class="btn primary" :disabled="!inputText.trim() || modalBusy" @click="doRename">
          <span v-if="modalBusy" class="spin spin-on-primary"></span>
          {{ modalBusy ? '保存中…' : '确定' }}
        </button>
      </template>
    </Modal>

    <!-- 创建分享 -->
    <Modal v-if="modal === 'share'" title="创建分享" @close="modal = null">
      <SharePrivacyNotice :caps="caps" />
      <div class="field">
        <label>分享名称</label>
        <input class="input" v-model="shareForm.name" :disabled="modalBusy" />
      </div>
      <div class="field" v-if="caps.shareExpiration">
        <label>有效期（可选）</label>
        <UiSelect
          v-model="shareForm.expiration"
          block
          :disabled="modalBusy"
          :options="shareExpirationOptions"
        />
      </div>
      <div class="field" v-if="caps.sharePassword">
        <label>提取码（可选）</label>
        <input class="input" v-model="shareForm.password" placeholder="留空自动生成或公开分享" :disabled="modalBusy" />
      </div>
      <template #actions>
        <button class="btn" :disabled="modalBusy" @click="modal = null">取消</button>
        <button class="btn primary" :disabled="modalBusy" @click="doShare">
          <span v-if="modalBusy" class="spin spin-on-primary"></span>
          {{ modalBusy ? '创建中…' : '创建并复制链接' }}
        </button>
      </template>
    </Modal>

    <!-- 云离线下载 -->
    <Modal v-if="modal === 'offline'" title="云离线下载" @close="modal = null">
      <div class="field">
        <label>离线下载链接（支持磁力链接、HTTP、ed2k）</label>
        <textarea class="textarea" v-model="inputText" placeholder="magnet:?xt=urn:btih:…" rows="3" :disabled="modalBusy"></textarea>
      </div>
      <template #actions>
        <button class="btn" :disabled="modalBusy" @click="modal = null">取消</button>
        <button class="btn primary" :disabled="!inputText.trim() || modalBusy" @click="doOffline">
          <span v-if="modalBusy" class="spin spin-on-primary"></span>
          {{ modalBusy ? '提交中…' : '提交' }}
        </button>
      </template>
    </Modal>

    <!-- 跨盘迁移 -->
    <Modal v-if="modal === 'migrate'" title="迁移到其他网盘" @close="modal = null">
      <p v-if="migrationPreview" class="hint">{{ migrationPreview.files }} 个文件 · {{ formatBytes(migrationPreview.bytes) }}<template v-if="migrationPreview.conflicts.length"> · {{ migrationPreview.conflicts.length }} 个同名项目（如 {{ migrationPreview.conflicts.slice(0,3).join('、') }} 等，同名将自动重命名）</template></p>
      <p v-if="migrationPreview?.transferMode === 'spool'" class="hint">经本机临时中转：最大文件 {{ formatBytes(migrationPreview.tempBytes) }}，临时磁盘剩余 {{ formatBytes(migrationPreview.tempAvailable) }}。</p>
      <p v-for="warning in migrationPreview?.warnings || []" :key="warning" role="alert">{{ warning }}</p>
      <div class="field">
        <label>来源账号</label>
        <div class="account-inline"><img :src="providerIconUrl(providerMetaOf(account, providers))" alt="" />{{ providerMetaOf(account, providers).label }} · {{ accountName(account) }}</div>
      </div>
      <div class="field">
        <label>目标账号</label>
        <UiSelect
          v-model="migrateTarget"
          block
          :disabled="modalBusy"
          placeholder="选择目标账号"
          :options="migrateAccounts.map((a) => ({ value: a.user_id, label: `${providerMetaOf(a, providers).label} · ${accountName(a)}`, img: providerIconUrl(providerMetaOf(a, providers)) }))"
        />
        <div class="hint" v-if="!migrateAccounts.length">暂无其他可用账号，请先添加目标网盘账号</div>
      </div>
      <div class="field" v-if="migrateTarget">
        <label>目标目录</label>
        <div class="panel-row" style="display:flex;gap:6px;align-items:center">
          <input class="input" style="flex:1" :value="migrateDirName" readonly placeholder="根目录" />
          <button class="btn sm" :disabled="modalBusy" @click="migrateDirPick = true">选择目录</button>
        </div>
      </div>
      <template #actions>
        <button class="btn" :disabled="modalBusy" @click="modal = null">取消</button>
        <button class="btn" :disabled="!migrateTarget || modalBusy || migrationChecking" @click="checkMigration">{{ migrationChecking ? '检查中…' : '检查' }}</button>
        <button class="btn primary" :disabled="!migrateTarget || modalBusy || migrationChecking" @click="doMigrate">
          <span v-if="modalBusy" class="spin spin-on-primary"></span>
          {{ modalBusy ? '创建任务中…' : '开始迁移' }}
        </button>
      </template>
    </Modal>

    <!-- 移动到 / 复制到 -->
    <SelectDirModal
      v-if="modal === 'movedir' || modal === 'copydir'"
      :title="modal === 'movedir' ? '移动到' : '复制到'"
      :account="account"
      :providers="providers"
      @close="modal = null"
      @select="onDirPicked"
      @toast="(m, t) => emit('toast', m, t)"
    />

    <!-- 迁移目标目录（浏览目标账号） -->
    <SelectDirModal
      v-if="migrateDirPick && migrateTargetAcc"
      title="选择目标目录"
      :account="migrateTargetAcc"
      :providers="providers"
      @close="migrateDirPick = false"
      @select="(d) => { migrateDir = d.id; migrateDirName = d.name; migrateDirPick = false }"
      @toast="(m, t) => emit('toast', m, t)"
    />

    <!-- 批量重命名 -->
    <RenameMultiModal
      v-if="modal === 'renamemulti'"
      :account="account"
      :files="modalFile"
      @close="modal = null"
      @done="refresh"
      @toast="(m, t) => emit('toast', m, t)"
    />

    <!-- 属性 -->
    <Modal v-if="modal === 'detail'" title="属性" @close="modal = null">
      <div class="kv-row"><span class="kv-label">名称</span><span style="user-select:text">{{ modalFile.name }}</span></div>
      <div class="kv-row"><span class="kv-label">类型</span><span>{{ modalFile.isDir ? '文件夹' : (modalFile.category || extOf(modalFile.name) || '文件') }}</span></div>
      <div class="kv-row" v-if="!modalFile.isDir"><span class="kv-label">大小</span><span>{{ formatBytes(modalFile.size) }}</span></div>
      <div class="kv-row"><span class="kv-label">修改时间</span><span>{{ formatTime(modalFile.time) }}</span></div>
      <div class="kv-row"><span class="kv-label">文件 ID</span><span style="user-select:text;font-size: 12px">{{ modalFile.file_id }}</span></div>
      <template #actions>
        <button class="btn primary" @click="modal = null">关闭</button>
      </template>
    </Modal>
    <DeletionHistory v-if="showDeletionHistory && props.account" :key="uid + ':' + did" :account="props.account" @close="showDeletionHistory = false" @toast="(...args) => emit('toast', ...args)" />
    <ConfirmModal v-if="confirmDialog" :title="confirmDialog.title" :message="confirmDialog.message" :okText="confirmDialog.okText" :danger="confirmDialog.danger" @ok="handleConfirmOk" @cancel="closeConfirm" />

    <!-- 上传同名文件冲突选择 -->
    <Modal v-if="conflictModal" title="同名文件已存在" @close="conflictModal.onPolicy(null)">
      <div class="conflict-body">
        <p class="conflict-msg">目标目录已存在同名文件：</p>
        <div class="conflict-files">
          <span v-for="n in conflictModal.names.slice(0, 5)" :key="n" class="conflict-file">{{ n }}</span>
          <span v-if="conflictModal.names.length > 5" class="conflict-more">等 {{ conflictModal.names.length }} 个文件</span>
        </div>
        <p class="conflict-q">请选择处理方式：</p>
        <div class="conflict-actions">
          <button class="btn" @click="conflictModal.onPolicy(null)">取消上传</button>
          <button v-if="uploadConflictPolicies.includes('overwrite')" class="btn primary" @click="conflictModal.onPolicy('overwrite')">覆盖</button>
          <button v-if="uploadConflictPolicies.includes('rename')" class="btn" @click="conflictModal.onPolicy('rename')">自动重命名</button>
          <button v-if="uploadConflictPolicies.includes('skip')" class="btn ghost" @click="conflictModal.onPolicy('skip')">跳过</button>
        </div>
      </div>
    </Modal>
  </div>
</template>

<style scoped>
.tree-load-error {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 6px;
  padding: 5px 8px 5px 28px;
  color: var(--color-error);
  font-size: 11px;
}
.tree-load-error button {
  padding: 1px 5px;
  border: 1px solid currentColor;
  border-radius: 4px;
  color: inherit;
  background: transparent;
  font: inherit;
  cursor: pointer;
}
mark.hl {
  background: color-mix(in srgb, var(--color-warning) 35%, transparent);
  color: inherit;
  border-radius: 2px;
  padding: 0 1px;
}
.conflict-body { padding: 8px 0; }
.conflict-msg { color: var(--text-secondary); margin-bottom: 8px; }
.conflict-files { display: flex; flex-wrap: wrap; gap: 6px; margin-bottom: 12px; }
.conflict-file { background: var(--bg-subtle); padding: 2px 8px; border-radius: var(--radius-sm); font-size: var(--fs-aux); color: var(--text-primary); }
.conflict-more { font-size: var(--fs-aux); color: var(--text-tertiary); align-self: center; }
.conflict-q { color: var(--text-primary); margin-bottom: 12px; }
.conflict-actions { display: flex; gap: 8px; justify-content: flex-end; }
.fileitem.clipboard-cut, .griditem.clipboard-cut { opacity: .72; transition: opacity var(--motion-fast) var(--motion-ease); }
</style>
