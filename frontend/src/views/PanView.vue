<script setup>
import { ref, computed, watch, onMounted, onActivated, onDeactivated, onBeforeUnmount, nextTick } from 'vue'
import {
  listDir, listTrash, search, mkdir, rename, trash, remove, restore,
  move, copy, favorite, createShare, uploadFiles, validateUploadFiles, migrateFiles, download,
  AddFavorite, RemoveFavorite, ListFavorites, OfflineDownload, PickDirectory, PickFiles,
  formatBytes, formatTime, formatTimeParts, iconOf, extOf, openKindOf, copyText,
  capsOf, providerMetaOf, providerIconUrl, providerOf, accountName, GetDirectoryCache,
  onEvent, onFileChange, notifyFileChange,
} from '../api'
import { debug, info, warn, error as logError, errorText } from '../logger'
import ContextMenu from '../components/ContextMenu.vue'
import DropdownBtn from '../components/DropdownBtn.vue'
import Modal from '../components/Modal.vue'
import SelectDirModal from '../components/SelectDirModal.vue'
import PreviewModal from '../components/PreviewModal.vue'
import RenameMultiModal from '../components/RenameMultiModal.vue'
import ConfirmModal from '../components/ConfirmModal.vue'
import UiIcon from '../components/UiIcon.vue'
import UiSelect from '../components/UiSelect.vue'
import PlayerPanel from '../components/PlayerPanel.vue'
import TreeNode from '../components/TreeNode.vue'
import DragDropZone from '../components/DragDropZone.vue'
import { getPrefs, setPref, orderAccounts } from '../appearance'
import { useDirectoryCache } from '../composables/useDirectoryCache'
import { useFileSelection } from '../composables/useFileSelection'
import { useVirtualFileList } from '../composables/useVirtualFileList'

const props = defineProps({
  account: Object,
  accounts: { type: Array, default: () => [] },
  providers: { type: Array, default: () => [] },
})
const emit = defineEmits(['toast', 'go', 'ready'])

// ---------- 状态 ----------
const mode = ref('list') // list | trash | search | favorite
const dirId = ref('root')
const pathStack = ref([]) // [{id,name}]
const files = ref([])
const loading = ref(false)
const error = ref('')
const favoriteError = ref('')
const keyword = ref('')       // 全盘搜索关键词
const filterRaw = ref('')     // 当前目录快速筛选（输入原值）
const filter = ref('')        // 防抖后的筛选值
let filterTimer = null
watch(filterRaw, (v) => {
  clearTimeout(filterTimer)
  filterTimer = setTimeout(() => { filter.value = v }, 120)
})
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

const viewMode = ref(initialPrefs.viewMode || 'list')  // list | grid
const sideWidth = ref(initialPrefs.sideWidth || 220) // 侧边栏宽度
const isSideResizing = ref(false)
let sideResizing = null
function sideDown(e) {
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

const uid = computed(() => (props.account ? props.account.user_id : ''))
const did = computed(() => (props.account ? props.account.drive_id : ''))
const caps = computed(() => capsOf(props.account, props.providers))
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
const migrateAccounts = computed(() => orderAccounts(props.accounts).filter((a) => a.user_id !== uid.value && canReceiveMigration(a)))
const migrateAccountOptions = computed(() => migrateAccounts.value.map((account) => {
  const accountMeta = providerMetaOf(account, props.providers)
  const providerLabel = accountMeta.label || providerOf(account.user_id) || '网盘'
  return {
    value: account.user_id,
    label: `${providerLabel} · ${accountName(account) || account.user_id}`,
    img: providerIconUrl(accountMeta),
  }
}))
const canMigrate = computed(() => !!caps.value.download && migrateAccounts.value.length > 0)

const rootKey = computed(() => meta.value.rootKey || 'root')
const rootTitle = computed(() => meta.value.rootTitle || '全部文件')

// ---------- 数据加载 ----------
// 短时间重复打开同一目录直接命中内存；超过该窗口后仍先展示缓存，
// 再在后台按页校验，避免缓存永久遮蔽远端变化。
const DIR_CACHE_REVALIDATE_AFTER_MS = 20 * 1000
const {
  clear: clearDirectoryCache,
  currentEpoch: cacheEpochOf,
  directoryKey: dirCacheKey,
  get: getCachedDir,
  invalidateDirectory: invalidateDirCache,
  invalidateMode: invalidateModeCache,
  isCurrent: cacheIsCurrent,
  isDirty: isDirectoryCacheDirty,
  isPersistableMode,
  listProgressively: listDirectoryProgressively,
  modeKey: cacheModeKey,
  modeVersionOf: modeCacheVersionOf,
  persist: persistDir,
  put: cacheDir,
  versionOf: cacheVersionOf,
} = useDirectoryCache()
let loadSeq = 0

// 滚动位置也必须按账号隔离。多个网盘通常都以 root 作为目录 ID；若只用
// 目录和模式作 key，切换账号后会沿用上一个网盘的滚动位置，页面可能直接
// 停在空白区域，造成“没有切换”的错觉。
function currentViewKey() { return [uid.value, did.value, mode.value, dirId.value, keyword.value].join('|') }

async function load(id, options = {}) {
  if (!props.account) return
  const seq = ++loadSeq
  markVirtualLoad(seq)
  const epoch = cacheEpochOf()
  const snapUid = uid.value, snapDid = did.value, snapMode = mode.value, snapKw = keyword.value
  const ckey = dirCacheKey(snapUid, snapDid, snapMode, id, snapKw)
  const version = cacheVersionOf(ckey)
  const modeKey = cacheModeKey(snapUid, snapDid, snapMode)
  const modeVersion = modeCacheVersionOf(modeKey)
  // 目录页进入 KeepAlive 后不可见时不继续追加分页请求；已有的单次
  // RPC 无法取消，但返回后不会再拉下一页或覆盖当前缓存。
  const isCurrent = () => panViewActive && seq === loadSeq && cacheIsCurrent(ckey, epoch, version, modeKey, modeVersion)
  // 快路径：有缓存先立即展示，后台静默刷新
  const cached = getCachedDir(ckey)
  const cacheStillFresh = cached && Date.now() - cached.at < DIR_CACHE_REVALIDATE_AFTER_MS
  let displayedCache = Boolean(cached)
  let networkDone = false
  let freshDataArrived = false
  if (cached) {
    files.value = cached.files
    loading.value = false
    if (snapMode === 'list') updateTreeSnapshot(id, cached.files, snapUid, snapDid)
  }
  else loading.value = true
  error.value = ''
  // 缓存刚写入时不重复请求；超过短校验窗口后保持内容可见并后台渐进刷新。
  if (cached && !options.force && cacheStillFresh) return
  if (!cached && !isDirectoryCacheDirty(ckey) && isPersistableMode(snapMode)) {
    // Persistent cache is a second fast path for a newly mounted page. It is
    // intentionally account/provider keyed and is ignored once fresh data
    // has arrived from the provider.
    GetDirectoryCache(ckey).then((list) => {
      if (!isCurrent() || networkDone || freshDataArrived || !Array.isArray(list)) return
      displayedCache = true
      files.value = list
      cacheDir(ckey, list)
      if (snapMode === 'list') updateTreeSnapshot(id, list, snapUid, snapDid)
      loading.value = false
    }).catch(() => {})
  }
  try {
    debug('pan', '加载目录', { mode: snapMode, dir: id || 'root', cached: !!cached, force: !!options.force })
    let list
    if (snapMode === 'trash') list = (await listTrash(snapUid, snapDid)) || []
    else if (snapMode === 'search') list = snapKw ? (await search(snapUid, snapDid, snapKw.trim())) || [] : []
    else list = await listDirectoryProgressively(snapUid, snapDid, id, isCurrent, (partial) => {
      if (!isCurrent()) return
      freshDataArrived = true
      displayedCache = true
      const snapshot = [...partial]
      files.value = snapshot
      loading.value = false
      cacheDir(ckey, snapshot)
      updateTreeSnapshot(id, snapshot, snapUid, snapDid)
    })
    networkDone = true
    // 时序保护：过期响应（账号/目录已切换或有更新请求）直接丢弃
    if (!isCurrent()) return
    files.value = list
    // 清理当前目录已不存在的缩略图错误标记，避免瞬时失败被永久记住
    const validIds = new Set(list.map((f) => f.file_id))
    const nextErr = {}
    for (const k in thumbErrors.value) if (validIds.has(k)) nextErr[k] = thumbErrors.value[k]
    thumbErrors.value = nextErr
    cacheDir(ckey, list)
    if (isPersistableMode(snapMode)) persistDir(ckey, list, epoch, version)
    if (snapMode === 'list') updateTreeSnapshot(id, list, snapUid, snapDid)
  } catch (e) {
    networkDone = true
    if (!isCurrent()) return
    if (!displayedCache) {
      error.value = String(e)
      files.value = []
    }
    warn('pan', '加载目录失败', { mode: snapMode, dir: id || 'root', error: errorText(e) })
  }
  if (isCurrent()) loading.value = false
}

function updateTreeSnapshot(id, list, snapUid = uid.value, snapDid = did.value) {
  if (snapUid !== uid.value || snapDid !== did.value) return
  tree.value[id] = (list || []).filter((f) => f.isDir)
  for (const f of tree.value[id]) {
    treeNames.value[f.file_id] = f.name
    treeParents.value[f.file_id] = id
  }
}

async function listDirectorySnapshot(id) {
  const snapUid = uid.value, snapDid = did.value
  const epoch = cacheEpochOf()
  const key = dirCacheKey(snapUid, snapDid, 'list', id, '')
  const version = cacheVersionOf(key)
  const modeKey = cacheModeKey(snapUid, snapDid, 'list')
  const modeVersion = modeCacheVersionOf(modeKey)
  const inMemory = getCachedDir(key)
  if (inMemory) return inMemory.files
  const persisted = isDirectoryCacheDirty(key) ? null : await GetDirectoryCache(key).catch(() => null)
  if (!cacheIsCurrent(key, epoch, version, modeKey, modeVersion) || snapUid !== uid.value || snapDid !== did.value) return []
  if (Array.isArray(persisted)) {
    cacheDir(key, persisted)
    updateTreeSnapshot(id, persisted, snapUid, snapDid)
    return persisted
  }
  const list = (await listDir(snapUid, snapDid, id)) || []
  if (!cacheIsCurrent(key, epoch, version, modeKey, modeVersion) || snapUid !== uid.value || snapDid !== did.value) return []
  cacheDir(key, list)
  persistDir(key, list, epoch, version)
  updateTreeSnapshot(id, list, snapUid, snapDid)
  return list
}

async function loadFavorites() {
  if (!props.account) { favorites.value = []; favoriteError.value = ''; return }
  const snapUid = uid.value, snapDid = did.value
  try {
    const list = (await ListFavorites(snapUid, snapDid)) || []
    if (snapUid === uid.value && snapDid === did.value) {
      favorites.value = list
      favoriteError.value = ''
    }
  } catch (e) {
    if (snapUid === uid.value && snapDid === did.value) favoriteError.value = String(e && e.message ? e.message : e)
  }
}

const displayFiles = computed(() => {
  let list = files.value
  if (filter.value) {
    const kw = filter.value.toLowerCase()
    list = list.filter((f) => (f.name || '').toLowerCase().includes(kw))
  }
  const arr = [...list]
  const key = sortKey.value
  arr.sort((a, b) => {
    if (a.isDir !== b.isDir) return a.isDir ? -1 : 1
    let r = 0
    if (key === 'name') r = (a.name || '').localeCompare(b.name || '', 'zh-Hans-CN')
    else if (key === 'time') r = (a.time || 0) - (b.time || 0)
    else r = (a.size || 0) - (b.size || 0)
    return sortAsc.value ? r : -r
  })
  return arr
})

const favoriteFiles = computed(() =>
  favorites.value.map((file) => ({
    file_id: file.file_id,
    name: file.name,
    isDir: file.isDir,
    size: 0,
    time: file.added,
    category: '',
    starred: false,
  }))
)
const listShown = computed(() => (mode.value === 'favorite' ? favoriteFiles.value : displayFiles.value))

const {
  allSelected,
  focusId,
  invert: invertSel,
  isSelected: isSel,
  rangeAnchor: rangAnchor,
  rangeSelecting: rangIsSelecting,
  selectAll,
  selected,
  toggle: toggleSel,
  toggleRangeSelecting: toggleRangSelect,
  toggleSelectAll,
} = useFileSelection(listShown)

// 名称高亮：搜索/筛选关键词命中部分拆段（模板用 <mark> 渲染，无关键词时零开销）
const hlKeyword = computed(() => (mode.value === 'search' ? keyword.value.trim() : filter.value.trim()))
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

// 行展示模型：列表/排序/关键词/缩略图状态变化时统一预计算一次；
// 选中、焦点等高频交互只改 class，不再触发每行的字符串/时间格式化运算。
const rowsShown = computed(() => {
  const kw = hlKeyword.value
  const errs = thumbErrors.value
  return listShown.value.map((f) => ({
    f,
    icon: iconOf(f),
    parts: kw ? namePartsOf(f.name, kw) : null,
    sizeText: f.isDir ? (f.file_count != null ? f.file_count + ' 项' : '-') : formatBytes(f.size),
    timeParts: formatTimeParts(f.time),
    thumb: f.thumbnail && !errs[f.file_id] ? f.thumbnail : '',
  }))
})

const {
  cancelPendingRestore: cancelVirtualRestore,
  gridRenderRows,
  gridVirtualBottom,
  gridVirtualTop,
  gridVirtualized,
  listEl,
  listRenderRows,
  listVirtualBottom,
  listVirtualTop,
  listVirtualized,
  markLoad: markVirtualLoad,
  onScroll: onListScroll,
  resetScroll: resetVirtualScroll,
  reveal: revealRow,
} = useVirtualFileList({
  files,
  getLoadSequence: () => loadSeq,
  loading,
  rowsShown,
  viewKey: currentViewKey,
  viewMode,
  visibleFiles: listShown,
})

const crumbs = computed(() => [{ id: rootKey.value, name: rootTitle.value }, ...pathStack.value])

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
  load(file.file_id)
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

function goHome() {
  mode.value = 'list'
  pathStack.value = []
  dirId.value = rootKey.value
  treeSelected.value = dirId.value
  keyword.value = ''
  selected.value = []
  load(dirId.value)
}

function refresh() {
  if (mode.value === 'list') load(dirId.value, { force: true })
  else if (mode.value === 'favorite') loadFavorites()
  else load(null, { force: true })
}

let refreshTimer = 0
const lastAutoRefreshAt = new Map()
const pendingMigrationSources = new Map()
let panViewActive = false
let panViewWasInactive = false
let pendingVisibleRefresh = null
let accountFavoritesTimer = 0
let pendingAccountFavoritesLoad = false
const ACCOUNT_FAVORITES_LOAD_DELAY_MS = 220

// 收藏树不影响目录首屏。账号切换时把它放到首个目录请求之后，既避免
// 同一网盘同时起两条初始化请求，也让用户更快看到文件列表。
function scheduleAccountFavoritesLoad(delay = ACCOUNT_FAVORITES_LOAD_DELAY_MS) {
  if (accountFavoritesTimer) clearTimeout(accountFavoritesTimer)
  const expectedAccountKey = accountViewKey()
  accountFavoritesTimer = setTimeout(() => {
    accountFavoritesTimer = 0
    if (accountViewKey() !== expectedAccountKey) return
    if (!panViewActive) {
      pendingAccountFavoritesLoad = true
      return
    }
    pendingAccountFavoritesLoad = false
    loadFavorites()
  }, Math.max(0, delay))
}

function deferVisibleRefresh(delay = 250, minimumInterval = 0) {
  if (!pendingVisibleRefresh) {
    pendingVisibleRefresh = { delay, minimumInterval }
    return
  }
  // 多个后台动作合并成用户回到页面后的一次刷新；保留最短等待时间即可。
  pendingVisibleRefresh.delay = Math.min(pendingVisibleRefresh.delay, delay)
  pendingVisibleRefresh.minimumInterval = Math.max(pendingVisibleRefresh.minimumInterval, minimumInterval)
}

function flushDeferredVisibleRefresh() {
  if (!panViewActive || !pendingVisibleRefresh) return
  const pending = pendingVisibleRefresh
  pendingVisibleRefresh = null
  scheduleCurrentViewRefresh(pending.delay, pending.minimumInterval)
}

function currentRefreshSnapshot() {
  return {
    userId: uid.value,
    driveId: did.value,
    mode: mode.value,
    dirId: dirId.value,
    keyword: keyword.value,
  }
}

function refreshSnapshotKey(snapshot) {
  return [snapshot.userId, snapshot.driveId, snapshot.mode, snapshot.dirId, snapshot.keyword].join('|')
}

function isCurrentRefreshSnapshot(snapshot) {
  return snapshot.userId === uid.value && snapshot.driveId === did.value &&
    snapshot.mode === mode.value && snapshot.dirId === dirId.value && snapshot.keyword === keyword.value
}

function currentViewNeedsRevalidation() {
  if (!props.account || mode.value === 'favorite') return false
  const id = mode.value === 'list' ? dirId.value : null
  const key = dirCacheKey(uid.value, did.value, mode.value, id, keyword.value)
  const cached = getCachedDir(key)
  return !cached || Date.now() - cached.at >= DIR_CACHE_REVALIDATE_AFTER_MS
}

function scheduleCurrentViewRefresh(delay = 250, minimumInterval = 0) {
  if (!panViewActive) {
    deferVisibleRefresh(delay, minimumInterval)
    return
  }
  clearTimeout(refreshTimer)
  const snapshot = currentRefreshSnapshot()
  const viewKey = refreshSnapshotKey(snapshot)
  const lastAt = lastAutoRefreshAt.get(viewKey) || 0
  const wait = Math.max(delay, minimumInterval - (Date.now() - lastAt))
  refreshTimer = setTimeout(() => {
    refreshTimer = 0
    if (!panViewActive || !isCurrentRefreshSnapshot(snapshot)) return
    // 目录/搜索词组合的刷新节流记录只保留有限数量，避免长期使用后无界增长。
    lastAutoRefreshAt.delete(viewKey)
    lastAutoRefreshAt.set(viewKey, Date.now())
    if (lastAutoRefreshAt.size > 200) {
      lastAutoRefreshAt.delete(lastAutoRefreshAt.keys().next().value)
    }
    if (snapshot.mode === 'list') load(snapshot.dirId, { force: true })
    else if (snapshot.mode === 'favorite') loadFavorites()
    else load(null, { force: true })
  }, Math.max(0, wait))
}

function invalidateListDirectories(userID, driveID, ids) {
  for (const id of new Set((ids || []).filter(Boolean))) {
    invalidateDirCache(userID, driveID, 'list', id)
    if (userID === uid.value && driveID === did.value && tree.value[id]) {
      // 左侧树下次展开时会按新数据加载，避免一个隐藏节点持续保留旧快照。
      delete tree.value[id]
    }
  }
}

function fileParentDirectories(list, includeCurrentListDirectory = true) {
  const directories = (list || [])
    .map((file) => String(file?.parent_file_id || '').trim())
    .filter(Boolean)
  if (includeCurrentListDirectory && mode.value === 'list' && dirId.value) directories.push(dirId.value)
  return [...new Set(directories)]
}

function notifyCurrentFileChange(options = {}) {
  const sourceFiles = options.files || selected.value
  const directories = [
    ...fileParentDirectories(sourceFiles, options.includeCurrentListDirectory !== false),
    ...(options.directories || []),
    ...(options.invalidateDirectories || []),
  ]
  const refreshView = Boolean(options.refreshView)
  notifyFileChange({
    userId: uid.value,
    driveId: did.value,
    directories,
    refreshTrash: Boolean(options.refreshTrash || (refreshView && mode.value === 'trash')),
    refreshFavorites: Boolean(options.refreshFavorites || (refreshView && mode.value === 'favorite')),
    refreshSearch: Boolean(options.refreshSearch || (refreshView && mode.value === 'search')),
    delay: options.delay,
    minimumInterval: options.minimumInterval,
  })
}

function onFileMutation(change) {
  if (!change?.userId || !change?.driveId) return
  invalidateListDirectories(change.userId, change.driveId, change.directories)
  if (change.refreshTrash) invalidateModeCache(change.userId, change.driveId, 'trash')
  if (change.refreshSearch) invalidateModeCache(change.userId, change.driveId, 'search')
  if (change.userId !== uid.value || change.driveId !== did.value) return

  // 网盘页面隐藏在 KeepAlive 中时，仅记录受影响范围；不要在后台请求或重绘。
  if (change.refreshFavorites && mode.value !== 'favorite' && panViewActive) loadFavorites()
  const refreshCurrent =
    (mode.value === 'list' && change.directories.includes(dirId.value)) ||
    (mode.value === 'trash' && change.refreshTrash) ||
    (mode.value === 'favorite' && change.refreshFavorites) ||
    (mode.value === 'search' && change.refreshSearch)
  if (refreshCurrent) scheduleCurrentViewRefresh(change.delay ?? 250, change.minimumInterval ?? 0)
}

function onMigrateEvent(job) {
  const id = String(job?.id || '')
  const status = String(job.status || '')
  if (id && ['completed', 'partial'].includes(status) && job.move) {
    const source = pendingMigrationSources.get(id)
    pendingMigrationSources.delete(id)
    if (source) notifyFileChange({ ...source, refreshSearch: true, delay: 400 })
  }
}

function showTrash() {
  mode.value = 'trash'
  selected.value = []
  treeSelected.value = 'trash'
  load()
}

function showFavorites() {
  mode.value = 'favorite'
  selected.value = []
  treeSelected.value = 'favorite'
  loadFavorites()
}

// 搜索：Enter/按钮触发；Esc 清空并返回目录
function enterSearch() {
  if (!caps.value.search) return
  mode.value = 'search'
  selected.value = []
  treeSelected.value = ''
  files.value = []
  nextTick(() => document.getElementById('pan-search')?.focus())
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
  locSaveTimer = setTimeout(() => {
    const all = { ...(getPrefs().panLocations || {}) }
    all[a.user_id] = {
      dirId: dirId.value,
      pathStack: pathStack.value,
      treeSelected: treeSelected.value,
      expanded: Object.keys(expanded.value).filter((k) => expanded.value[k]),
    }
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
  if (expanded.value[id] && !tree.value[id] && props.account) {
    try {
      const list = await listDirectorySnapshot(id)
      if (snapUid !== uid.value || snapDid !== did.value) return
      updateTreeSnapshot(id, list, snapUid, snapDid)
    } catch (e) {
      if (snapUid === uid.value && snapDid === did.value) {
        expanded.value[id] = false
        emit('toast', `目录树加载失败：${String(e && e.message ? e.message : e)}`, 'error')
      }
    }
  }
}

// 幂等展开（加载子目录），用于根节点默认展开
async function expandTree(id, name) {
  if (expanded.value[id]) return
  expanded.value[id] = true
  const snapUid = uid.value, snapDid = did.value
  if (!tree.value[id] && props.account) {
    try {
      const list = await listDirectorySnapshot(id)
      if (snapUid !== uid.value || snapDid !== did.value) return
      updateTreeSnapshot(id, list, snapUid, snapDid)
    } catch (e) {
      if (snapUid === uid.value && snapDid === did.value) {
        expanded.value[id] = false
        emit('toast', `目录树加载失败：${String(e && e.message ? e.message : e)}`, 'error')
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

// ---------- 打开 ----------
async function openFile(file) {
  if (file.isDir) { openDir(file); return }
  if (mode.value === 'trash') { emit('toast', '回收站中的文件无法打开', 'error'); return }
  const kind = openKindOf(file)
  if (kind === 'video') {
    modalFile.value = file
    modal.value = 'player'
  } else if (kind === 'pdf') {
    askConfirm(`“${file.name}”暂不支持在线预览，需要下载后查看，是否下载到本地？`, () => doDownload([file]), { okText: '下载', title: 'PDF 暂不支持预览' })
  } else if (kind === 'download') {
    askConfirm(`“${file.name}”不支持在线预览，是否下载到本地？`, () => doDownload([file]), { okText: '下载', title: '无法预览' })
  } else {
    modalFile.value = file
    modal.value = 'preview'
  }
}

// ---------- 操作 ----------
function selIds() { return selected.value.map((f) => f.file_id) }

let running = false
async function run(fn, okMsg, options = {}) {
  if (running) return false
  running = true
  const action = okMsg || '执行操作'
  // 实际文件动作由后端统一记录开始/完成；界面这里只保留 debug 轨迹，
  // 避免同一操作同时出现两套 info 日志。
  debug('pan', '文件操作已提交', { action })
  try {
    await fn()
    if (okMsg) emit('toast', okMsg, 'success')
    // 仅把当前操作影响到的目录/视图标记为失效。订阅者会在可见目录
    // 上合并成一次后台刷新，其他目录保持本机缓存，直到用户真正进入。
    if (options.refreshView || options.invalidateDirectories?.length || options.refreshTrash || options.refreshFavorites || options.refreshSearch) {
      notifyCurrentFileChange({
        ...options,
        refreshTrash: options.refreshTrash || (options.refreshView && mode.value === 'trash'),
        refreshFavorites: options.refreshFavorites || (options.refreshView && mode.value === 'favorite'),
        refreshSearch: options.refreshSearch || (options.refreshView && mode.value === 'search'),
      })
    }
    debug('pan', '文件操作界面已更新', { action })
    return true
  } catch (e) {
    emit('toast', String(e), 'error')
    logError('pan', `${action}失败`, { error: errorText(e) })
    return false
  } finally {
    running = false
  }
}

// 下载：直接入队，停留当前页 + toast，不弹确认框、不跳转传输页
async function doDownload(list) {
  const targets = list || selected.value.filter((f) => !f.isDir)
  if (!targets.length) { emit('toast', '请选择要下载的文件', 'error'); return }
  if (running) return
  running = true
  const started = performance.now()
  const fields = { provider: providerOf(uid.value), count: targets.length }
  info('pan', '加入下载队列开始', fields)
  try {
    for (const f of targets) await download(uid.value, did.value, f)
    emit('toast', `已加入下载队列（${targets.length} 项）`, 'success')
    info('pan', '加入下载队列完成', { ...fields, duration_ms: Math.round(performance.now() - started) })
  } catch (e) {
    emit('toast', String(e), 'error')
    logError('pan', '加入下载队列失败', { ...fields, error: errorText(e), duration_ms: Math.round(performance.now() - started) })
  } finally {
    running = false
  }
}

function targets(file) {
  return file && isSel(file) && selected.value.length > 1 ? selected.value : [file]
}

// ---------- 收藏（本地收藏 + 云端收藏同步） ----------
function isFav(file) { return favorites.value.some((f) => f.file_id === file.file_id) }

async function toggleFav(file) {
  const list = targets(file)
  const removing = isFav(file)
  await run(async () => {
    if (removing) {
      for (const f of list) await RemoveFavorite(uid.value, did.value, f.file_id)
      if (caps.value.favorite) { try { await favorite(uid.value, did.value, false, list.map((f) => f.file_id)) } catch { /* 云端同步失败不阻塞 */ } }
    } else {
      for (const f of list) await AddFavorite(uid.value, did.value, { file_id: f.file_id, name: f.name, isDir: f.isDir, user_id: uid.value, drive_id: did.value, added: Math.floor(Date.now() / 1000) })
      if (caps.value.favorite) { try { await favorite(uid.value, did.value, true, list.map((f) => f.file_id)) } catch { /* 同上 */ } }
    }
  }, removing ? '已移出收藏' : '已加入收藏', { refreshFavorites: true })
}

// ---------- 右键菜单 ----------
function onCtx(e, file) {
  // 标准文件管理器行为：右键未选中项时先将其变为唯一选中并聚焦，
  // 避免菜单作用于一个没有任何高亮的文件（ targets() 据此取操作对象）
  if (file && file.file_id && !isSel(file)) {
    selected.value = [file]
    focusId.value = file.file_id
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
    caps.value.createShare && { icon: 'share', label: '分享', action: 'share' },
    { icon: 'star', label: isFav(file) ? '移出收藏' : '加入收藏', action: 'fav' },
    { sep: true },
    caps.value.move && { icon: 'move', label: '移动到…', action: 'move' },
    caps.value.copy && { icon: 'copy', label: '复制到…', action: 'copy' },
    canMigrate.value && { icon: 'migrate', label: '迁移到其他账号或网盘…', action: 'migrate' },
    caps.value.rename && { icon: 'pencil', label: '重命名', action: 'rename' },
    caps.value.rename && selected.value.length > 1 && { icon: 'pencil', label: `批量重命名 (${selected.value.length})`, action: 'renamemulti' },
    { sep: true },
    caps.value.recycleBin && mode.value !== 'trash' && { icon: 'trash', label: '放回收站', danger: true, action: 'trash' },
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
    case 'download': doDownload([file]); break
    case 'open': openFile(file); break
    case 'share': openShareDialog(list); break
    case 'fav': toggleFav(file); break
    case 'rename':
      if (list.length !== 1) { emit('toast', '重命名仅支持单个文件', 'error'); return }
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
      if (!canMigrate.value) { emit('toast', '没有支持接收迁移文件的目标账号', 'error'); return }
      modalFile.value = list; migrateTarget.value = ''; migrateDir.value = 'root'; migrateDirName.value = '根目录'; migrateDirPick.value = false; modal.value = 'migrate'; break
    case 'trash': run(() => trash(uid.value, did.value, ids), '已移入回收站', {
      refreshView: true, refreshFavorites: true, refreshTrash: true, refreshSearch: true, files: list,
    }); break
    case 'delete':
      askConfirm(`彻底删除 ${list.length} 项？删除后无法还原。`, () => run(() => remove(uid.value, did.value, ids), '已彻底删除', {
        refreshView: true, refreshFavorites: true, refreshTrash: true, refreshSearch: true, files: list,
      }), { danger: true, title: '彻底删除' })
      break
    case 'restore': run(() => restore(uid.value, did.value, ids), '已还原', {
      refreshView: true, refreshTrash: true, refreshSearch: true, files: list,
    }); break
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
    }, '文件夹已创建', { refreshView: true, refreshSearch: true, directories: [dirId.value] })) {
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
    if (await run(() => rename(uid.value, did.value, file.file_id, name), '已重命名', {
      refreshView: true, refreshSearch: true, files: [file],
    })) {
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
        emit('toast', copied ? '分享链接已复制到剪贴板' : '分享已创建，但复制到剪贴板失败', copied ? 'success' : 'warn')
      } else {
        emit('toast', '分享已创建，但服务端未返回可复制链接', 'warn')
      }
    }, null)) {
      modal.value = null
    }
  } finally { modalBusy.value = false }
}

// 上传：原生文件/目录选择器（选择文件夹时后端递归入队），停留当前页 + toast
const uploadPickModal = ref(false) // 上传前的小弹窗：选文件还是文件夹
const conflictModal = ref(null) // { names, onPolicy }

async function checkUploadConflict(paths) {
  const existing = new Set()
  for (const f of files.value) existing.add(f.name)
  const clashes = []
  for (const p of paths) {
    const name = p.split(/[/\\]/).pop()
    if (existing.has(name)) clashes.push(name)
  }
  if (clashes.length === 0) return 'overwrite'
  return new Promise((resolve) => {
    conflictModal.value = {
      names: clashes,
      onPolicy: (policy) => { conflictModal.value = null; resolve(policy) },
    }
  })
}

async function validateUploadSelection(paths) {
  try {
    await validateUploadFiles(uid.value, did.value, paths)
    return true
  } catch (e) {
    emit('toast', String(e), 'error')
    return false
  }
}

async function pickUploadFiles() {
  let paths
  try { paths = await PickFiles('选择要上传的文件') } catch { return }
  if (!paths || !paths.length) return
  if (!await validateUploadSelection(paths)) return
  const policy = await checkUploadConflict(paths)
  if (!policy) return
  await run(() => uploadFiles(uid.value, did.value, dirId.value, policy, paths), `已加入上传队列（${paths.length} 项）`)
}
async function pickUploadFolder() {
  let dir
  try { dir = await PickDirectory('选择要上传的文件夹', '') } catch { return }
  if (!dir) return
  if (!await validateUploadSelection([dir])) return
  const policy = await checkUploadConflict([dir])
  if (!policy) return
  await run(() => uploadFiles(uid.value, did.value, dirId.value, policy, [dir]), '已加入上传队列（文件夹）')
}

async function doOffline() {
  const url = inputText.value.trim()
  if (!url || modalBusy.value) return
  modalBusy.value = true
  const started = performance.now()
  const fields = { provider: providerOf(uid.value), count: 1 }
  info('pan', '创建云离线任务开始', fields)
  try {
    if (await run(() => OfflineDownload(uid.value, did.value, url, ''), '已提交云离线任务')) {
      // PikPak 未指定目录时会落到下载根目录；先让该目录在短延迟后校验，
      // 既能尽早显示新建任务文件，也不在当前子目录做无意义刷新。
      notifyFileChange({
        userId: uid.value,
        driveId: did.value,
        directories: [rootKey.value],
        refreshSearch: true,
        delay: 1500,
        minimumInterval: 5000,
      })
      info('pan', '创建云离线任务完成', { ...fields, duration_ms: Math.round(performance.now() - started) })
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
  if (which === 'movedir') await run(() => move(uid.value, did.value, ids, target.id), '已移动', {
    refreshView: true, invalidateDirectories: [target.id], refreshSearch: true, files: list,
  })
  else if (which === 'copydir') await run(() => copy(uid.value, did.value, ids, target.id), '已复制', {
    refreshView: mode.value === 'list' && target.id === dirId.value,
    invalidateDirectories: [target.id], refreshSearch: true, files: list,
  })
}

// 迁移目标必须从同一份已排序、已过滤的列表里解析，避免账号列表更新后
// 下拉显示的顺序和实际执行目标不一致。
const migrateTargetAcc = computed(() => migrateAccounts.value.find((a) => a.user_id === migrateTarget.value) || null)
const migrateTargetRoot = computed(() => providerMetaOf(migrateTargetAcc.value, props.providers).rootKey || 'root')

watch(migrateTarget, () => {
  const targetMeta = providerMetaOf(migrateTargetAcc.value, props.providers)
  migrateDir.value = targetMeta.rootKey || 'root'
  migrateDirName.value = targetMeta.rootTitle || '根目录'
})

async function doMigrate() {
  const targetAcc = migrateTargetAcc.value
  if (!targetAcc) { emit('toast', '请选择目标账号', 'error'); return }
  if (!canReceiveMigration(targetAcc)) { emit('toast', '目标账号不支持上传，无法迁移', 'error'); return }
  const list = modalFile.value
  if (!list || modalBusy.value) return
  modalBusy.value = true
  try {
    let createdJob = null
    if (await run(
      async () => { createdJob = await migrateFiles(uid.value, did.value, targetAcc.user_id, targetAcc.drive_id, migrateDir.value || migrateTargetRoot.value, list.map((f) => f.file_id), false) },
      '迁移任务已创建'
    )) {
      if (createdJob?.id && createdJob.move) {
        pendingMigrationSources.set(createdJob.id, {
          userId: uid.value,
          driveId: did.value,
          directories: fileParentDirectories(list),
        })
      }
      modal.value = null
    }
  } finally { modalBusy.value = false }
}

function onPreviewSaved() {
  notifyCurrentFileChange({ files: modalFile.value ? [modalFile.value] : [], refreshView: true, refreshSearch: true })
}

function onRenameMultiDone() {
  const renamed = Array.isArray(modalFile.value) ? modalFile.value : selected.value
  notifyCurrentFileChange({ files: renamed, refreshView: true, refreshSearch: true })
}

// ---------- 工具条分享 ----------
function openShareDialog(list) {
  if (!caps.value.combinedShare && list.length > 1) {
    emit('toast', '该网盘一次只能分享一个文件或文件夹', 'warn')
    return
  }
  shareForm.value = { name: list.length === 1 ? list[0].name : `${list.length} 个文件`, expiration: defaultShareExpiration(), password: '' }
  modalFile.value = [...list]
  modal.value = 'share'
}

function openShareModal() { openShareDialog(selected.value) }

function confirmDeleteSelected() {
  if (!selected.value.length) return
  askConfirm(`彻底删除 ${selected.value.length} 项？删除后无法还原。`, () => run(() => remove(uid.value, did.value, selIds()), '已彻底删除', {
    refreshView: true, refreshFavorites: true, refreshTrash: true, refreshSearch: true, files: selected.value,
  }), { danger: true, title: '彻底删除' })
}

// ---------- 收藏打开 ----------
function openFav(f) {
  if (f.isDir) {
    mode.value = 'list'
    pathStack.value = [{ id: f.file_id, name: f.name }]
    dirId.value = f.file_id
    treeSelected.value = f.file_id
    selected.value = []
    load(f.file_id)
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
    caps.value.download && { icon: 'download', label: '下载', action: 'download' },
    caps.value.createShare && { icon: 'share', label: '分享', action: 'share' },
    { icon: 'star', label: '加入收藏', action: 'fav' },
    { sep: true },
    !isRoot && caps.value.rename && { icon: 'pencil', label: '重命名', action: 'rename' },
    !isRoot && caps.value.recycleBin && { icon: 'trash', label: '放回收站', danger: true, action: 'trash' },
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
      const list = await listDirectorySnapshot(id)
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
  const tag = (e.target.tagName || '').toLowerCase()
  if (tag === 'input' || tag === 'textarea' || tag === 'select') return
  if (!props.account || modal.value) return
  if (e.ctrlKey && e.code === 'KeyA') { selectAll(); e.preventDefault() }
  else if (e.code === 'F5') { refresh(); e.preventDefault() }
  else if (e.code === 'Backspace' && mode.value === 'list' && pathStack.value.length) { goUp(); e.preventDefault() }
  else if (e.ctrlKey && e.shiftKey && e.code === 'KeyF') { enterSearch(); e.preventDefault() }
  else if (e.ctrlKey && e.code === 'KeyF') { document.getElementById('pan-filter')?.focus(); e.preventDefault() }
  else if (e.ctrlKey && e.shiftKey && e.code === 'KeyN' && caps.value.createFolder) { inputText.value = ''; modal.value = 'mkdir'; e.preventDefault() }
  else if (e.ctrlKey && e.code === 'KeyU' && caps.value.upload && mode.value === 'list') { pickUploadFiles(); e.preventDefault() }
  else if (e.ctrlKey && e.code === 'KeyH') { goHome(); e.preventDefault() }
  else if (e.code === 'F2' && selected.value.length === 1 && caps.value.rename) {
    modalFile.value = selected.value[0]; inputText.value = selected.value[0].name; modal.value = 'rename'; e.preventDefault()
  }
  else if (e.code === 'Delete' && selected.value.length && caps.value.recycleBin && mode.value !== 'trash') {
    run(() => trash(uid.value, did.value, selIds()), '已移入回收站', {
      refreshView: true, refreshFavorites: true, refreshTrash: true, refreshSearch: true, files: selected.value,
    }); e.preventDefault()
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

function accountViewKey(account = props.account) {
  return [account?.user_id || '', account?.drive_id || '', rootKey.value].join('\u0000')
}

let appliedAccountViewKey = ''
let pendingAccountViewSync = false

// 账号变更需要同时失效旧请求、清空旧账号的瞬时 UI 状态，并以新账号的
// 缓存为首屏。这一方法既由 prop watcher 调用，也暴露给父页面作为切换
// 兜底，避免 KeepAlive/过渡期间漏掉一次 prop watcher 后必须手动刷新。
function syncAccountView({ force = false } = {}) {
  const a = props.account
  const nextKey = accountViewKey(a)
  if (accountFavoritesTimer) clearTimeout(accountFavoritesTimer)
  accountFavoritesTimer = 0
  if (!a) {
    appliedAccountViewKey = ''
    pendingAccountViewSync = false
    pendingAccountFavoritesLoad = false
    loadSeq++
    cancelVirtualRestore()
    files.value = []
    favorites.value = []
    selected.value = []
    focusId.value = ''
    loading.value = false
    error.value = ''
    return false
  }
  if (!force && nextKey === appliedAccountViewKey) return false

  appliedAccountViewKey = nextKey
  // 让上一账号仍在进行中的分页/缓存读结果立即失效。
  loadSeq++
  cancelVirtualRestore()
  tree.value = {}
  expanded.value = {}
  treeParents.value = {}
  treeNames.value = {}
  thumbErrors.value = {}
  files.value = []
  favorites.value = []
  favoriteError.value = ''
  selected.value = []
  focusId.value = ''
  filterRaw.value = ''
  filter.value = ''
  resetVirtualScroll()
  error.value = ''

  // 页面在 KeepAlive 后台时只记录待同步状态；回到可见页面再读取缓存或
  // 远端，避免账号列表更新在后台额外制造网络请求。
  if (!panViewActive) {
    pendingAccountViewSync = true
    pendingAccountFavoritesLoad = true
    return true
  }
  pendingAccountViewSync = false

  // 恢复该账号上次浏览位置；没有记录时回根目录
  const saved = (getPrefs().panLocations || {})[a.user_id]
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
    for (const id of Object.keys(expanded.value)) if (!tree.value[id]) expandTree(id)
    load(saved.dirId)
  } else {
    goHome()
    expanded.value[rootKey.value] = true
  }
  scheduleAccountFavoritesLoad()
  return true
}

watch(() => accountViewKey(), () => { syncAccountView() })

async function onDropUpload(paths) {
  if (!props.account || mode.value !== 'list' || !caps.value.upload) return
  if (!paths || !paths.length || !(await validateUploadSelection(paths))) return
  const policy = await checkUploadConflict(paths)
  if (!policy) return
  await run(() => uploadFiles(uid.value, did.value, dirId.value, policy, paths), `已添加拖拽上传（${paths.length} 项）`)
}

function openMkdirModal() {
  inputText.value = ''
  modal.value = 'mkdir'
}

function openUploadModal() {
  uploadPickModal.value = true
}

defineExpose({
  refresh,
  syncAccountView,
  openMkdirModal,
  openUploadModal,
  clearCache: () => {
    loadSeq++
    clearDirectoryCache()
    tree.value = {}
    expanded.value = {}
    treeParents.value = {}
    if (props.account) load(dirId.value)
  },
})

let offMigrateEvent = null
let offFileMutation = null
onMounted(() => {
  panViewActive = true
  window.addEventListener('keydown', onKey)
  window.addEventListener('mousemove', sideMove)
  window.addEventListener('mouseup', sideUp)
  offMigrateEvent = onEvent('migrate:progress', onMigrateEvent)
  offFileMutation = onFileChange(onFileMutation)
  syncAccountView({ force: true })
  emit('ready')
})
onActivated(() => {
  panViewActive = true
  const returningFromBackground = panViewWasInactive
  panViewWasInactive = false
  const accountWasSynced = pendingAccountViewSync
    ? syncAccountView({ force: true })
    : syncAccountView()
  if (accountWasSynced) {
    // 新账号已由 syncAccountView 按缓存策略加载，不再叠加一次自动校验。
  } else {
    if (pendingAccountFavoritesLoad) scheduleAccountFavoritesLoad(0)
    if (pendingVisibleRefresh) flushDeferredVisibleRefresh()
    // 回到文件页时，短时间内继续复用本机缓存；超过校验窗口才后台拉取
    // 当前这一页，避免离开页面一段时间后仍停留在过期快照。
    else if (returningFromBackground && currentViewNeedsRevalidation()) scheduleCurrentViewRefresh(120)
  }
  emit('ready')
})
onDeactivated(() => {
  panViewActive = false
  panViewWasInactive = true
  if (accountFavoritesTimer) {
    clearTimeout(accountFavoritesTimer)
    accountFavoritesTimer = 0
    pendingAccountFavoritesLoad = true
  }
  if (refreshTimer) {
    clearTimeout(refreshTimer)
    refreshTimer = 0
    deferVisibleRefresh(120)
  }
})
onBeforeUnmount(() => {
  panViewActive = false
  window.removeEventListener('keydown', onKey)
  window.removeEventListener('mousemove', sideMove)
  window.removeEventListener('mouseup', sideUp)
  clearTimeout(filterTimer)
  clearTimeout(hoverTimer)
  clearTimeout(refreshTimer)
  clearTimeout(accountFavoritesTimer)
  offMigrateEvent?.()
  offFileMutation?.()
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
              :title="f.name"
              @click="openFav(f)"
            >
              <span class="tn-arrow"></span><UiIcon :name="f.isDir ? 'folder' : iconOf(f)" :size="14" :class="f.isDir ? '' : 'ft-' + iconOf(f)" /><span class="tn-label">{{ f.name }}</span>
            </div>
            <div v-if="favoriteError" class="tree-load-error" role="alert">
              <span>收藏加载失败</span>
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
        <DragDropZone class="pan-right" @drop-files="onDropUpload">
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

          <!-- 合并工具条：导航 + 新建/上传 + 选择管理 + 文件操作 + 排序/视图/筛选 -->
          <div class="toppanbtns">
            <div class="toppanbtn">
              <button class="tbtn icon" :disabled="mode !== 'list' || !pathStack.length" title="后退 (Backspace)" @click="goUp"><UiIcon name="back" :size="16" /></button>
              <button class="tbtn icon" :disabled="loading" title="刷新 (F5)" @click="refresh"><UiIcon name="refresh" :size="16" /></button>
              <button v-if="caps.search" class="tbtn icon" title="全盘搜索 (Ctrl+Shift+F)" @click="enterSearch"><UiIcon name="search" :size="16" /></button>
              <button v-if="caps.offlineDownload && mode === 'list'" class="tbtn icon" title="云离线下载" @click="inputText = ''; modal = 'offline'"><UiIcon name="cloud-down" :size="16" /></button>
            </div>
            <div class="toppanbtn" v-if="mode === 'list'">
              <button v-if="caps.createFolder" class="tbtn sm" title="新建文件夹 (Ctrl+Shift+N)" @click="inputText = ''; modal = 'mkdir'"><UiIcon name="plus" :size="14" />新建</button>
              <button v-if="caps.upload" class="tbtn sm" title="上传 (Ctrl+U)" @click="uploadPickModal = true"><UiIcon name="upload" :size="14" />上传</button>
            </div>
            <div class="toppanbtn sel-mgmt">
              <button class="btn-check" :class="{ on: allSelected }" title="全选 (Ctrl+A)" @click="toggleSelectAll">
                <UiIcon v-if="allSelected" name="check" :size="11" />
              </button>
              <span class="selectInfo">已选中 {{ selected.length }}/{{ listShown.length }} 个</span>
              <button
                class="tbtn xs"
                :class="{ danger: rangIsSelecting }"
                title="点击后依次点两个文件选中区间（或按住 Shift 点击）"
                @click="toggleRangSelect"
              >{{ rangIsSelecting ? '取消选择' : '区间选择' }}</button>
              <button
                v-if="!rangIsSelecting && selected.length && selected.length < listShown.length"
                class="tbtn xs"
                @click="invertSel"
              >反向选择</button>
              <button v-if="!rangIsSelecting && selected.length" class="tbtn xs" @click="selected = []">取消已选</button>
            </div>
            <div class="toppanbtn" v-if="selected.length">
              <button v-if="caps.download" class="tbtn" @click="doDownload()"><UiIcon name="download" :size="15" />下载</button>
              <button v-if="caps.createShare" class="tbtn" title="分享 (Ctrl+Shift+S)" @click="openShareModal"><UiIcon name="share" :size="15" />分享</button>
              <button class="tbtn" @click="toggleFav(selected[0])"><UiIcon name="star" :size="15" />{{ isFav(selected[0]) && selected.length === 1 ? '移出收藏' : '收藏' }}</button>
              <button v-if="caps.move" class="tbtn" @click="modalFile = [...selected]; modal = 'movedir'"><UiIcon name="move" :size="15" />移动</button>
              <button v-if="caps.copy" class="tbtn" @click="modalFile = [...selected]; modal = 'copydir'"><UiIcon name="copy" :size="15" />复制</button>
              <button v-if="caps.recycleBin && mode !== 'trash'" class="tbtn danger" title="删除 (Delete)" @click="run(() => trash(uid, did, selIds()), '已移入回收站', { refreshView: true, refreshFavorites: true, refreshTrash: true, refreshSearch: true, files: selected })"><UiIcon name="trash" :size="15" />删除</button>
            </div>
            <div class="toppanbtn" v-if="mode === 'trash' && selected.length">
              <button v-if="caps.trashRestore" class="tbtn" @click="run(() => restore(uid, did, selIds()), '已还原', { refreshView: true, refreshTrash: true, refreshSearch: true, files: selected })"><UiIcon name="restore" :size="15" />还原</button>
              <button v-if="caps.permanentDelete" class="tbtn danger" @click="confirmDeleteSelected"><UiIcon name="x-circle" :size="15" />彻底删除</button>
            </div>
            <div class="toolbar-spacer"></div>
            <div class="toppanbtn">
              <DropdownBtn :label="sortLabel" :icon="sortAsc ? 'sort-asc' : 'sort-desc'" :items="sortMenuItems" title="排序方式" @select="onSortPick" />
              <div class="view-switch">
                <button class="vs-btn" :class="{ on: viewMode === 'list' }" title="列表视图" @click="viewMode = 'list'; setPref('viewMode', 'list')"><UiIcon name="list" :size="15" /></button>
                <button class="vs-btn" :class="{ on: viewMode === 'grid' }" title="网格视图" @click="viewMode = 'grid'; setPref('viewMode', 'grid')"><UiIcon name="grid" :size="15" /></button>
              </div>
              <span v-if="mode === 'search' && caps.search" class="search-quick-wrap">
                <span class="sq-icon"><UiIcon name="search" :size="13" /></span>
                <input
                  id="pan-search"
                  class="search-quick"
                  style="width:220px"
                  v-model="keyword"
                  placeholder="输入关键字进行搜索 (Enter)"
                  @keydown.enter="goSearch"
                  @keydown="onSearchKey"
                />
                <button v-if="keyword" class="sq-clear" title="清空搜索" @click="keyword = ''; if (mode === 'search') goHome()"><UiIcon name="close" :size="11" /></button>
              </span>
              <span class="search-quick-wrap">
                <span class="sq-icon"><UiIcon name="search" :size="13" /></span>
                <input id="pan-filter" class="search-quick" v-model="filterRaw" placeholder="快速筛选 (Ctrl+F)" />
                <button v-if="filterRaw" class="sq-clear" title="清空筛选" @click="filterRaw = ''"><UiIcon name="close" :size="11" /></button>
              </span>
            </div>
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
          <div v-else-if="error" class="empty"><span class="empty-icon"><UiIcon name="warning" :size="30" /></span><span>{{ error }}</span><button class="btn sm" @click="refresh">重试</button></div>

          <!-- 列表视图（旧版 fileitem 行） -->
          <div v-else-if="viewMode === 'list'" ref="listEl" :key="currentViewKey()" class="file-list" @scroll.passive="onListScroll">
            <div v-if="listVirtualized" aria-hidden="true" :style="{ height: listVirtualTop + 'px' }"></div>
            <div
              v-for="r in listRenderRows"
              :key="r.f.file_id"
              class="fileitem"
              :class="{ selected: isSel(r.f), focus: focusId === r.f.file_id, 'anchor-node': rangIsSelecting && rangAnchor === r.f.file_id }"
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
              <span v-if="r.f.starred" class="fstar-mark"><UiIcon name="star" :size="13" /></span>
              <div class="filesize">{{ r.sizeText }}</div>
              <div class="filetime">
                <span class="filedate">{{ r.timeParts.date }}</span>
                <span class="fileclock">{{ r.timeParts.clock }}</span>
              </div>
            </div>
            <div v-if="listVirtualized" aria-hidden="true" :style="{ height: listVirtualBottom + 'px' }"></div>
            <div v-if="!listShown.length" class="workspace-empty-state">
              <UiIcon :name="mode === 'trash' ? 'trash' : mode === 'search' ? 'search' : mode === 'favorite' ? 'star' : 'folder'" :size="36" style="opacity:.4" />
              <span class="wes-title">{{ mode === 'trash' ? '回收站为空' : mode === 'search' ? '没有匹配的文件' : mode === 'favorite' ? '暂无收藏，右键文件加入收藏' : '空目录' }}</span>
            </div>
          </div>

          <!-- 网格视图（旧版 griditem） -->
          <div v-else ref="listEl" :key="currentViewKey()" class="file-list gridlist" @scroll.passive="onListScroll">
            <div v-if="gridVirtualized" aria-hidden="true" :style="{ gridColumn: '1 / -1', height: gridVirtualTop + 'px' }"></div>
            <div
              v-for="r in gridRenderRows"
              :key="r.f.file_id"
              class="griditem"
              :class="{ selected: isSel(r.f), focus: focusId === r.f.file_id, 'anchor-node': rangIsSelecting && rangAnchor === r.f.file_id }"
              @click="toggleSel(r.f, $event)"
              @dblclick="onRowOpen(r.f)"
              @contextmenu.prevent="onCtx($event, r.f)"
            >
              <span class="gsel">
                <button class="btn-check" :class="{ on: isSel(r.f) }" tabindex="-1" @click.stop="toggleSel(r.f, { ctrlKey: true })">
                  <UiIcon v-if="isSel(r.f)" name="check" :size="11" />
                </button>
              </span>
              <span v-if="r.f.starred" class="gstar"><UiIcon name="star" :size="12" /></span>
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
              <span class="wes-title">{{ mode === 'trash' ? '回收站为空' : mode === 'search' ? '没有匹配的文件' : mode === 'favorite' ? '暂无收藏' : '空目录' }}</span>
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
          <UiIcon name="file" :size="24" /><span class="uppick-name">上传文件</span><span class="uppick-desc">选择一个或多个文件</span>
        </button>
        <button class="uppick-card" @click="uploadPickModal = false; pickUploadFolder()">
          <UiIcon name="folder" :size="24" /><span class="uppick-name">上传文件夹</span><span class="uppick-desc">选择整个文件夹</span>
        </button>
      </div>
    </Modal>

    <!-- 新建文件夹 -->
    <Modal v-if="modal === 'mkdir'" title="新建文件夹" @close="modal = null">
      <div class="field">
        <label>文件夹名称</label>
        <input class="input" v-model="inputText" placeholder="输入名称" @keyup.enter="doMkdir" autofocus :disabled="modalBusy" />
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
        <input class="input" v-model="inputText" @keyup.enter="doRename" autofocus :disabled="modalBusy" />
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
        <input class="input" v-model="shareForm.password" placeholder="留空则自动生成或无提取码" :disabled="modalBusy" />
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
        <label>下载链接（磁力 / HTTP / ed2k）</label>
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

    <!-- 跨账号/跨盘迁移 -->
    <Modal v-if="modal === 'migrate'" title="迁移到其他账号或网盘" @close="modal = null">
      <div class="field">
        <label>目标账号</label>
        <UiSelect
          v-model="migrateTarget"
          block
          :disabled="modalBusy"
          placeholder="选择目标账号"
          :options="migrateAccountOptions"
        />
        <div class="hint" v-if="!migrateAccounts.length">没有其他可用账号，请先在左侧添加网盘账号</div>
        <div class="hint" v-else>可选择同一网盘的其他账号；源、目标支持相同指纹时会优先秒传，目标支持常规上传时未命中会自动继续迁移。</div>
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
        <button class="btn primary" :disabled="!migrateTarget || modalBusy" @click="doMigrate">
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
      title="选择迁移目标目录"
      :account="migrateTargetAcc"
      :providers="providers"
      @close="migrateDirPick = false"
      @select="(d) => { migrateDir = d.id; migrateDirName = d.name; migrateDirPick = false }"
      @toast="(m, t) => emit('toast', m, t)"
    />

    <!-- 预览（支持画廊/翻页/缩放/文本编辑保存） -->
    <PreviewModal
      v-if="modal === 'preview'"
      :account="account"
      :file="modalFile"
      :file-list="listShown"
      @close="modal = null"
      @toast="(m, t) => emit('toast', m, t)"
      @saved="onPreviewSaved"
    />
    <PlayerPanel
      v-if="modal === 'player'"
      :account="account"
      :file="modalFile"
      :files="listShown"
      @select-file="modalFile = $event"
      @close="modal = null"
      @toast="(m, t) => emit('toast', m, t)"
    />

    <!-- 批量重命名 -->
    <RenameMultiModal
      v-if="modal === 'renamemulti'"
      :account="account"
      :files="modalFile"
      @close="modal = null"
      @done="onRenameMultiDone"
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
          <button class="btn primary" @click="conflictModal.onPolicy('overwrite')">覆盖</button>
          <button class="btn" @click="conflictModal.onPolicy('rename')">保留两者（新增后缀）</button>
          <button class="btn ghost" @click="conflictModal.onPolicy('skip')">跳过</button>
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
</style>
