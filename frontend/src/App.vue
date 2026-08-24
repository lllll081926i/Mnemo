<script setup>
import { ref, onMounted, computed, watch, nextTick, onBeforeUnmount } from 'vue'
import { listAccounts, listProviders, removeAccount, renameMountedAccount, onEvent, notifyFileChange, GetSettings, SaveSettings, providerOf, accountName, providerIconUrl, providerMetaOf, setAccountCustomMeta as setAccountCustomMetaBackend } from './api'
import { applyAppearance, getLastDriveSelection, setLastDriveSelection, clearLastDriveSelection, getAccountAlias, getAccountCustomIcon, setAccountCustomMeta, orderAccounts } from './appearance'
import PanView from './views/PanView.vue'
import TransferView from './views/TransferView.vue'
import ShareView from './views/ShareView.vue'
import SyncView from './views/SyncView.vue'
import SettingsView from './views/SettingsView.vue'
import AccountRail from './components/AccountRail.vue'
import AccountAvatar from './components/AccountAvatar.vue'
import UiIcon from './components/UiIcon.vue'
import Modal from './components/Modal.vue'
import LoginModal from './components/LoginModal.vue'
import QuickOpen from './components/QuickOpen.vue'
import ConfirmModal from './components/ConfirmModal.vue'
import UpdateModal from './components/UpdateModal.vue'
import ImageCropModal from './components/ImageCropModal.vue'
import { CheckUpdate } from './api'
import { debug, error, errorText, info, installGlobalErrorLogging, warn } from './logger'
import { WindowMinimise, WindowToggleMaximise, Quit } from '../wailsjs/runtime/runtime'
import appLogo from './assets/logo/icon.svg'

const tab = ref('pan')
const tabOrder = ['pan', 'transfer', 'sync', 'share', 'settings']
const prevTabIdx = ref(0)
const pageTrans = ref('page-slide-left')
const pageComponents = { pan: PanView, transfer: TransferView, sync: SyncView, share: ShareView, settings: SettingsView }
const pageComponent = computed(() => pageComponents[tab.value] || PanView)
const pageProps = computed(() => {
  if (tab.value === 'pan') return { account: current.value, accounts: accounts.value, providers: providers.value }
  if (tab.value === 'sync') return { account: current.value, accounts: accounts.value, providers: providers.value }
  if (tab.value === 'transfer' || tab.value === 'share') return { accounts: accounts.value, providers: providers.value }
  return {}
})
const pageListeners = computed(() => {
  const listeners = { toast }
  if (tab.value === 'pan') {
    listeners.go = onPanGo
    listeners.ready = runPendingPanAction
  }
  if (tab.value === 'settings') {
    listeners.theme = applyTheme
    listeners.update = () => { pendingUpdateInfo.value = null; showUpdate.value = true }
    listeners.clearCache = clearPanCache
  }
  return listeners
})
function switchTab(key) {
	if (key === tab.value) return
  const from = tab.value
	info('navigation', '页面切换开始', { from, to: key })
  const ni = tabOrder.indexOf(key)
  pageTrans.value = ni >= tabOrder.indexOf(tab.value) ? 'page-slide-left' : 'page-slide-right'
  prevTabIdx.value = tabOrder.indexOf(tab.value)
  tab.value = key
  nextTick(() => info('navigation', '页面切换完成', { from, to: key }))
}
const accounts = ref([])
const providers = ref([])
const current = ref(null)
const showLogin = ref(false)
const showQuickOpen = ref(false)
const showUpdate = ref(false)
const pendingUpdateInfo = ref(null)
const infoAcc = ref(null)
const renameAcc = ref(null)
const renameName = ref('')
const renameIcon = ref('')
const renameBusy = ref(false)
const showPresetIcons = ref(false)
const cropImageSrc = ref('')
const cropIsSvg = ref(false)
const fileInputRef = ref(null)
let pendingPanAction = ''
const completedUploadChanges = new Set()
const migrationChangeVersions = new Map()
const syncChangeVersions = new Map()

function rootDirectoryOf(userId, driveId) {
  const account = accounts.value.find((item) => item.user_id === userId && item.drive_id === driveId)
  return providerMetaOf(account, providers.value)?.rootKey || 'root'
}

function normalizeRootDirectory(userId, driveId, directory) {
  const value = String(directory || '').trim()
  const root = rootDirectoryOf(userId, driveId)
  // 同步/迁移后台任务历史上会使用通用 root，而文件页对 PikPak、WebDAV
  // 等使用各自的根 ID；统一后才能命中对应的局部缓存。
  return !value || value === 'root' || value === '/' ? root : value
}

function onGlobalTransferEvent(ev) {
  const task = ev?.task
  if (ev?.kind !== 'upload' || task?.status !== 'completed' || !task?.id) return
  if (completedUploadChanges.has(task.id)) return
  completedUploadChanges.add(task.id)
  if (completedUploadChanges.size > 1000) completedUploadChanges.delete(completedUploadChanges.values().next().value)
  if (!task.user_id || !task.drive_id || !task.parent_id) return
  notifyFileChange({
    userId: task.user_id,
    driveId: task.drive_id,
    directories: [normalizeRootDirectory(task.user_id, task.drive_id, task.parent_id)],
    refreshSearch: true,
    delay: 1200,
    minimumInterval: 5000,
  })
}

function onGlobalMigrateEvent(job) {
  const id = String(job?.id || '')
  const targetUser = String(job?.dstUser || job?.dst_user || '')
  const targetDrive = String(job?.dstDrive || job?.dst_drive || '')
  const targetParent = String(job?.dstParent || job?.dst_parent || '')
  if (!id || !targetUser || !targetDrive || !targetParent) return
  const status = String(job.status || '')
  const version = `${status}|${Number(job.processed || 0)}|${Number(job.failed || 0)}`
  if (migrationChangeVersions.get(id) === version) return
  migrationChangeVersions.set(id, version)
  if (migrationChangeVersions.size > 1000) migrationChangeVersions.delete(migrationChangeVersions.keys().next().value)
  const terminal = ['completed', 'partial'].includes(status)
  if (!terminal && Number(job.processed || 0) <= 0) return
  notifyFileChange({
    userId: targetUser,
    driveId: targetDrive,
    directories: [normalizeRootDirectory(targetUser, targetDrive, targetParent)],
    refreshSearch: terminal,
    delay: 1200,
    minimumInterval: 5000,
  })
}

function onGlobalSyncState(ev) {
  const id = String(ev?.id || '')
  const status = String(ev?.status || '')
  if (!id || ev?.running || status !== 'completed') return

  // pull 只会写入本机，不会改变云端目录；push/two-way 才需要失效网盘缓存。
  const direction = String(ev.direction || 'two-way').toLowerCase()
  if (direction === 'pull') return
  const userId = String(ev.user_id || ev.userId || '')
  const driveId = String(ev.drive_id || ev.driveId || '')
  if (!userId || !driveId) return

  const version = `${status}|${String(ev.startedAt || '')}`
  if (syncChangeVersions.get(id) === version) return
  syncChangeVersions.set(id, version)
  if (syncChangeVersions.size > 1000) syncChangeVersions.delete(syncChangeVersions.keys().next().value)

  const remoteDir = normalizeRootDirectory(userId, driveId, ev.remote_dir || ev.remoteDir)
  notifyFileChange({
    userId,
    driveId,
    directories: [remoteDir],
    refreshTrash: Boolean(ev.delete_propagation || ev.deletePropagation),
    refreshSearch: true,
    delay: 800,
    minimumInterval: 5000,
  })
}

const PRESET_ICONS = [
  { id: 'pikpak.svg', label: 'PikPak' },
  { id: 'aliopen.svg', label: '阿里云盘' },
  { id: 'onedrive.svg', label: 'OneDrive' },
  { id: 'dropbox.svg', label: 'Dropbox' },
  { id: 'pan123.svg', label: '123 云盘' },
  { id: 'pan189.svg', label: '天翼云盘' },
  { id: 'pan139.svg', label: '139 云盘' },
  { id: 'lanzou.svg', label: '蓝奏云' },
  { id: 'ilanzou.svg', label: '优享蓝奏' },
  { id: 'guangya.svg', label: '光鸭云' },
  { id: 'yike.svg', label: '一刻相册' },
  { id: 'jianguoyun.svg', label: '坚果云' },
  { id: 'infinitycloud.svg', label: 'InfiniCLOUD' },
  { id: 'nextcloud.svg', label: 'Nextcloud' },
  { id: 'owncloud.svg', label: 'ownCloud' },
  { id: 'seafile.svg', label: 'Seafile' },
  { id: 'openlist.svg', label: 'OpenList/AList' },
  { id: 'synology.svg', label: '群晖' },
  { id: 'koofr.svg', label: 'Koofr' },
  { id: 'yandex.svg', label: 'Yandex' },
  { id: 'pcloud-eu.svg', label: 'pCloud (EU)' },
  { id: 'pcloud-us.svg', label: 'pCloud (US)' },
  { id: 's3.svg', label: 'S3' },
  { id: 'webdav.svg', label: 'WebDAV' },
]
const windowMaximized = ref(false)
const curTheme = ref('system')
const isDark = ref(false)
applyAppearance('system') // 防启动闪白，随后以设置为准
isDark.value = document.documentElement.classList.contains('dark')

function closeUpdateModal() {
  showUpdate.value = false
  pendingUpdateInfo.value = null
}

function quickToggleTheme() {
  applyTheme(isDark.value ? 'light' : 'dark')
  saveThemePref()
}
function windowMinimise() {
  try { WindowMinimise() } catch { /* browser preview */ }
}
function windowToggleMaximise() {
  try {
    WindowToggleMaximise()
    windowMaximized.value = !windowMaximized.value
  } catch { /* browser preview */ }
}
function windowQuit() {
  // Wails routes Quit through OnBeforeClose. The backend is the single source
  // of truth for deciding whether to hide to tray or terminate the process.
  try {
    Quit()
  } catch (err) {
    error('window', 'window close request failed', { error: errorText(err) })
    window.close?.()
  }
}
async function saveThemePref() {
  try {
    const s = (await GetSettings()) || {}
    s.theme = curTheme.value
    await SaveSettings(s)
  } catch { /* 静默 */ }
}
const toasts = ref([])
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
const tabs = [
  { key: 'pan', label: '网盘' },
  { key: 'transfer', label: '传输' },
  { key: 'sync', label: '同步' },
  { key: 'share', label: '分享' },
]

const panView = ref(null)

// 顶栏滑动色块：跟随当前 tab 的位置与宽度
const tabStrip = ref(null)
const gliderStyle = ref({ opacity: 0 })
function updateGlider() {
  const strip = tabStrip.value
  if (!strip) return
  const el = strip.querySelector('.top-tab.active')
  if (!el) { gliderStyle.value = { opacity: 0 }; return }
  gliderStyle.value = {
    opacity: 1,
    transform: `translateX(${el.offsetLeft}px)`,
    width: el.offsetWidth + 'px',
  }
}
watch(tab, () => nextTick(updateGlider))

let refreshEpoch = 0
function applyGlobalAccountOrder() {
  const ordered = orderAccounts(accounts.value)
  const changed = ordered.length !== accounts.value.length || ordered.some((account, index) => account !== accounts.value[index])
  if (!changed) return
  accounts.value = ordered
  debug('account', '账号顺序已同步', { count: ordered.length })
}

function onPreferencesChanged(event) {
  if (event?.detail?.key === 'accountOrder') applyGlobalAccountOrder()
}

function refresh() {
  const my = ++refreshEpoch
  debug('account', '刷新账号列表')
  listAccounts().then((list) => {
    if (my !== refreshEpoch) return
    accounts.value = orderAccounts(list || [])
    const available = accounts.value
    if (current.value) {
      // user_id 是主键，但同时保留 drive_id 匹配，避免旧数据/挂载盘的
      // 同账号多存储记录在后台刷新时把用户刚选择的目标覆盖回去。
      const found = available.find((a) => a.user_id === current.value.user_id && a.drive_id === current.value.drive_id) ||
        available.find((a) => a.user_id === current.value.user_id)
      current.value = found ? { ...found } : (available[0] ? { ...available[0] } : null)
    } else if (available.length) {
      const saved = getLastDriveSelection()
      const preferred = saved
        ? (available.find((a) => a.user_id === saved.userId && (!saved.driveId || a.drive_id === saved.driveId)) ||
          available.find((a) => a.user_id === saved.userId))
        : null
      current.value = preferred || available[0]
    }
    if (current.value) {
      setLastDriveSelection(current.value.user_id, current.value.drive_id)
    }
    // 账号变更、后台刷新都会触发这里；只在 debug 级记录，避免轮询刷屏。
    debug('account', '账号列表已更新', { count: accounts.value.length })
  }).catch((e) => warn('account', '账号列表更新失败', { error: errorText(e) }))
}

function accountSelectionKey(acc) {
  return [acc?.user_id || '', acc?.drive_id || ''].join('\u0000')
}

function select(acc) {
  if (!acc) return
  const nextKey = accountSelectionKey(acc)
  // 使用独立对象而不是复用列表项，确保每次侧栏选择都会向 KeepAlive 中的
  // 文件页提交一次明确的响应式更新。
  current.value = { ...acc }
  setLastDriveSelection(acc.user_id, acc.drive_id)
  info('account', '网盘切换完成', { provider: providerOf(acc.user_id) })
  nextTick(() => {
    // watcher 是主路径；这里是页面过渡/KeepAlive 下的无副作用兜底。
    // syncAccountView 内部会按账号 key 去重，因此不会产生重复请求。
    if (accountSelectionKey(current.value) === nextKey) panView.value?.syncAccountView?.()
  })
}

function onPanGo(target) {
  if (target === 'login') showLogin.value = true
  else switchTab(target)
}

function runPendingPanAction() {
  const action = pendingPanAction
  if (!action) return
  const view = panView.value
  const handler = view && view[action]
  if (typeof handler !== 'function') return
  pendingPanAction = ''
  handler.call(view)
}

function queuePanAction(action) {
  pendingPanAction = action
  if (tab.value !== 'pan') switchTab('pan')
  // 已经挂载的文件页可在同一轮更新后立即执行；首次进入则由 PanView
  // 的 ready 事件兜底，避免 transition 的 out-in 阶段丢失快捷操作。
  nextTick(runPendingPanAction)
}

function clearPanCache() {
  queuePanAction('clearCache')
}

function providerLabel(acc) {
  const p = providers.value.find((x) => x.ID === providerOf(acc.user_id))
  return p ? p.Meta.label : providerOf(acc.user_id)
}

function remove(acc) {
  askConfirm(`移除账号「${accountName(acc) || acc.user_id}」？只删除账号凭据，下载任务、收藏和同步配置等本地记录会保留。`, async () => {
    try {
      await removeAccount(acc.user_id)
      if (current.value && current.value.user_id === acc.user_id) current.value = null
      const saved = getLastDriveSelection()
      if (saved && saved.userId === acc.user_id) clearLastDriveSelection()
      refresh()
      toast('账号已移除', 'success')
    } catch (e) { toast(String(e), 'error') }
  }, { danger: true, title: '移除账号' })
}

function openRename(acc) {
  if (!acc) return
  renameAcc.value = acc
  // 后端账户存储是跨重启的主来源；LocalStorage 作为旧数据和写入失败时的本机兜底。
  renameName.value = String(acc.custom_name || getAccountAlias(acc.user_id) || '')
  renameIcon.value = String(acc.custom_icon || getAccountCustomIcon(acc.user_id) || '')
  showPresetIcons.value = false
  info('account', '打开账号自定义', {
    provider: providerOf(acc.user_id),
    has_name: Boolean(renameName.value),
    has_icon: Boolean(renameIcon.value),
  })
}

function onIconFileSelected(e) {
  const file = e.target.files?.[0]
  if (!file) return
  const isSvg = file.type === 'image/svg+xml' || file.name.toLowerCase().endsWith('.svg')
  const reader = new FileReader()
  reader.onload = () => {
    cropIsSvg.value = isSvg
    cropImageSrc.value = String(reader.result || '')
    if (fileInputRef.value) fileInputRef.value.value = ''
  }
  reader.readAsDataURL(file)
}

function onCropConfirm(croppedDataUrl) {
  renameIcon.value = croppedDataUrl
  cropImageSrc.value = ''
}

async function saveRename() {
  if (!renameAcc.value) return
  renameBusy.value = true
  try {
    const uid = renameAcc.value.user_id
    const alias = renameName.value.trim()
    const icon = renameIcon.value
    // 1. 本地 LocalStorage 快速持久化 + 事件分发
    setAccountCustomMeta(uid, alias, icon)
    // 2. 后端持久化到 accounts.json（双向保证持久）
    try {
      await setAccountCustomMetaBackend(uid, alias, icon)
    } catch {
      // 本地昵称/图标不依赖云端，后端持久化失败时仍保留当前设备的设置。
      info('account', '账号自定义本机完成', {
        provider: providerOf(uid),
        has_name: !!alias,
        has_icon: !!icon,
        backend_saved: false,
      })
    }
    // 强制触发一次账号列表浅拷贝以便全局响应式刷新
    accounts.value = accounts.value.map((a) => (a.user_id === uid ? { ...a, custom_name: alias, custom_icon: icon } : a))
    if (current.value?.user_id === uid) current.value = { ...current.value, custom_name: alias, custom_icon: icon }
    // 后端成功时已经记录统一的开始/完成日志；仅本机兜底时在上方补充完成记录。
    renameAcc.value = null
    toast('账号设置已更新', 'success')
  } catch (e) {
    error('account', '账号自定义保存失败', { error: errorText(e) })
    toast(String(e), 'error')
  } finally {
    renameBusy.value = false
  }
}

function toast(msg, type = '') {
  const id = Date.now() + Math.random()
  const normalizedType = ['success', 'error', 'warn', 'info'].includes(type) ? type : 'info'
  const labels = { success: '已完成', error: '操作失败', warn: '需要注意', info: '提示' }
  const item = { id, msg: String(msg ?? ''), type: normalizedType, label: labels[normalizedType] }
  toasts.value.push(item)
  const lifetime = normalizedType === 'error' ? 6500 : 3600
  setTimeout(() => dismissToast(id), lifetime)
  return id
}

function dismissToast(id) {
  toasts.value = toasts.value.filter((t) => t.id !== id)
}

function applyTheme(theme) {
  curTheme.value = theme || 'system'
  applyAppearance(curTheme.value)
  isDark.value = document.documentElement.classList.contains('dark')
}

function onKey(e) {
  if ((e.ctrlKey || e.metaKey) && (e.key === 'p' || e.code === 'KeyP')) {
    e.preventDefault()
    showQuickOpen.value = !showQuickOpen.value
    return
  }
  if (!e.altKey) return
  const map = { Digit1: 'pan', Digit2: 'transfer', Digit3: 'sync', Digit4: 'share', Digit5: 'settings' }
  if (map[e.code]) { switchTab(map[e.code]); e.preventDefault() }
}

function preventNativeContextMenu(e) {
  // Keep the native menu for text editing, but never expose the browser menu in the app shell.
  const target = e.target
  if (target && target.closest && target.closest('input, textarea, select, [contenteditable="true"]')) return
  e.preventDefault()
}

function onQuickAction(action) {
  if (action === 'toggle-theme') quickToggleTheme()
  else if (action === 'refresh') queuePanAction('refresh')
  else if (action === 'mkdir') queuePanAction('openMkdirModal')
  else if (action === 'upload') queuePanAction('openUploadModal')
}

onMounted(async () => {
	info('app', '前端初始化完成')
	window.addEventListener('contextmenu', preventNativeContextMenu, true)
	window.addEventListener('mnemo:prefs-changed', onPreferencesChanged)
	const removeGlobalErrorLogging = installGlobalErrorLogging()
	listProviders().then((p) => { providers.value = p || [] }).catch(() => {})
	let autoUpdateEnabled = true
	try {
	  const s = await GetSettings()
	  // 颜色模式默认跟随系统；顶栏可手动切换（不出现在设置页）
	  if (s) {
		applyTheme(s.theme || 'system')
		if (pageComponents[s.defaultTab]) tab.value = s.defaultTab
		autoUpdateEnabled = s.autoUpdate !== false
	  }
	} catch { /* 默认跟随系统 */ }
	refresh()
	// 启动后延迟检查更新（静默，仅发现有新版时弹窗）
	if (autoUpdateEnabled) {
	  setTimeout(() => {
		CheckUpdate().then((r) => {
		  if (r && r.available) {
			pendingUpdateInfo.value = r
			showUpdate.value = true
		  }
		}).catch(() => {})
	  }, 3000)
	}
  window.addEventListener('keydown', onKey)
  const mq = window.matchMedia('(prefers-color-scheme: dark)')
  const onScheme = () => applyAppearance(curTheme.value)
  mq.addEventListener('change', onScheme)
  const offFns = [
    onEvent('account:changed', refresh),
    onEvent('app:ready', refresh),
    onEvent('share:history-error', (ev) => {
      toast(`分享已创建，但本地历史保存失败：${ev?.error || '未知错误'}`, 'warn')
    }),
    // 即使文件页尚未挂载，传输/迁移完成也应先记录受影响目录，
    // 等用户进入对应目录时再按缓存策略刷新。
    onEvent('transfer:event', onGlobalTransferEvent),
    onEvent('migrate:progress', onGlobalMigrateEvent),
    onEvent('sync:state', onGlobalSyncState),
    // 原生传输悬浮窗点击/菜单「显示主窗口」时跳到传输页
    onEvent('nav:tab', (key) => { if (typeof key === 'string' && tabOrder.includes(key)) switchTab(key) }),
  ]
	nextTick(updateGlider)
	info('navigation', '页面进入', { page: tab.value })
  window.addEventListener('resize', updateGlider)
	cleanupFns = () => {
		removeGlobalErrorLogging()
    window.removeEventListener('resize', updateGlider)
    window.removeEventListener('keydown', onKey)
    window.removeEventListener('contextmenu', preventNativeContextMenu, true)
		window.removeEventListener('mnemo:prefs-changed', onPreferencesChanged)
    mq.removeEventListener('change', onScheme)
    offFns.forEach((fn) => { try { fn && fn() } catch { /* noop */ } })
	}
	debug('app', 'frontend initialization tasks scheduled')
})
let cleanupFns = null
onBeforeUnmount(() => cleanupFns && cleanupFns())
</script>

<template>
  <div class="app-shell">
    <header class="topbar">
      <div class="app-brand"><img :src="appLogo" alt="" />Mnemo</div>
      <div ref="tabStrip" class="top-tabs">
        <span class="top-tab-glider" :style="gliderStyle"></span>
        <button
          v-for="t in tabs"
          :key="t.key"
          class="top-tab"
          :class="{ active: tab === t.key }"
          @click="switchTab(t.key)"
        >{{ t.label }}</button>
      </div>
      <div class="spacer"></div>
      <button class="icon-btn" title="快捷命令面板 (Ctrl+P)" @click="showQuickOpen = true"><UiIcon name="search" :size="16" /></button>
      <button class="icon-btn" :title="isDark ? '切换到浅色' : '切换到深色'" @click="quickToggleTheme"><UiIcon :name="isDark ? 'sun' : 'moon'" :size="17" /></button>
      <button class="icon-btn" :class="{ active: tab === 'settings' }" title="设置 (Alt+5)" @click="switchTab('settings')"><UiIcon name="settings" :size="17" /></button>
      <AccountAvatar v-if="current" class="topbar-account" :account="current" :providers="providers" />
      <div class="window-actions" aria-label="窗口控制">
        <button class="window-btn" type="button" title="最小化" @click="windowMinimise"><UiIcon name="window-minimize" :size="13" /></button>
        <button class="window-btn" type="button" :title="windowMaximized ? '还原窗口' : '最大化'" @click="windowToggleMaximise"><UiIcon :name="windowMaximized ? 'window-restore' : 'window-maximize'" :size="13" /></button>
        <button class="window-btn window-close" type="button" title="关闭窗口" @click="windowQuit"><UiIcon name="close" :size="14" /></button>
      </div>
    </header>

    <div class="app-body">
      <AccountRail
        v-if="tab === 'pan'"
        :accounts="accounts"
        :providers="providers"
        :current="current"
        @select="select"
        @add="showLogin = true"
        @remove="remove"
        @info="infoAcc = $event"
        @rename="openRename"
      />
      <main class="page-host">
        <transition :name="pageTrans" mode="out-in">
          <KeepAlive>
            <component
              :is="pageComponent"
              :key="tab"
              class="page-view"
              :ref="tab === 'pan' ? 'panView' : undefined"
              v-bind="pageProps"
              v-on="pageListeners"
            />
          </KeepAlive>
        </transition>
      </main>
    </div>

    <LoginModal v-if="showLogin" :providers="providers" @close="showLogin = false" @toast="toast" />

    <QuickOpen
      :show="showQuickOpen"
      :accounts="accounts"
      :providers="providers"
      :current-account="current"
      @close="showQuickOpen = false"
      @select-tab="switchTab"
      @select-account="select"
      @action="onQuickAction"
    />

    <Modal v-if="infoAcc" title="账号信息" width="420px" @close="infoAcc = null">
      <div class="kv-row"><span class="kv-label">账号</span><span style="user-select:text">{{ accountName(infoAcc) }}</span></div>
      <div class="kv-row"><span class="kv-label">网盘</span><span style="display:inline-flex;align-items:center;gap:5px"><img v-if="providerIconUrl(providerMetaOf(infoAcc, providers))" :src="providerIconUrl(providerMetaOf(infoAcc, providers))" alt="" style="width:15px;height:15px;object-fit:contain" />{{ providerLabel(infoAcc) }}</span></div>
      <div class="kv-row" v-if="infoAcc.usage && infoAcc.usage.size">
        <span class="kv-label">容量</span><span>{{ infoAcc.usage.usedStr }} / {{ infoAcc.usage.sizeStr }}</span>
      </div>
      <div class="kv-row" v-if="infoAcc.token && infoAcc.token.vipname">
        <span class="kv-label">会员</span><span class="badge primary">{{ infoAcc.token.vipname }}</span>
      </div>
      <div class="kv-row"><span class="kv-label">账号 ID</span><span style="user-select:text;font-size: 12px;color:var(--text-tertiary)">{{ infoAcc.user_id }}</span></div>
      <template #actions>
        <button class="btn primary" @click="infoAcc = null">关闭</button>
      </template>
    </Modal>

    <!-- 账号本地自定义昵称与图标弹窗 -->
    <Modal v-if="renameAcc" title="账号自定义" width="420px" @close="renameAcc = null">
      <div class="custom-acc-form">
        <div class="field">
          <label>自定义显示昵称</label>
          <input
            v-model="renameName"
            class="input"
            maxlength="40"
            placeholder="留空则使用默认账号名称"
            autofocus
            @keyup.enter="saveRename"
          />
        </div>

        <div class="field">
           <label>自定义图标</label>
          <div class="acc-icon-selector">
            <!-- 当前选中的预览图 -->
            <div class="acc-icon-preview">
              <img v-if="renameIcon" :src="providerIconUrl(renameIcon)" alt="" />
              <img v-else :src="providerIconUrl(providerMetaOf(renameAcc, providers))" alt="" />
            </div>

            <!-- 图标操作按钮组 -->
            <div class="acc-icon-actions">
              <input
                ref="fileInputRef"
                type="file"
                accept="image/*,.svg"
                style="display: none"
                @change="onIconFileSelected"
              />
              <button class="btn sm" type="button" @click="fileInputRef?.click()">
                <UiIcon name="camera" :size="13" />
                <span>自定义</span>
              </button>
              <button class="btn sm" type="button" @click="showPresetIcons = !showPresetIcons">
                <UiIcon name="grid" :size="13" />
                <span>{{ showPresetIcons ? '收起内置预设' : '内置预设' }}</span>
              </button>
              <button
                v-if="renameIcon"
                class="btn sm text"
                type="button"
                title="恢复为该网盘的默认图标"
                @click="renameIcon = ''"
              >
                恢复默认
              </button>
            </div>
          </div>

          <!-- 内置预设图标网格展开 -->
          <div v-if="showPresetIcons" class="preset-icons-grid">
            <button
              v-for="p in PRESET_ICONS"
              :key="p.id"
              type="button"
              class="preset-icon-chip"
              :class="{ active: renameIcon === p.id || (!renameIcon && providerMetaOf(renameAcc, providers).icon === 'drive-icons/' + p.id) }"
              :title="p.label"
              @click="renameIcon = p.id; showPresetIcons = false"
            >
              <img :src="providerIconUrl(p.id)" :alt="p.label" />
              <span>{{ p.label }}</span>
            </button>
          </div>
        </div>
      </div>

      <template #actions>
        <button class="btn" type="button" :disabled="renameBusy" @click="renameAcc = null">取消</button>
        <button class="btn primary" type="button" :disabled="renameBusy" @click="saveRename">
          <span v-if="renameBusy" class="spin spin-on-primary"></span>
          {{ renameBusy ? '保存中…' : '保存设置' }}
        </button>
      </template>
    </Modal>

    <!-- 图片/SVG 裁剪弹窗 -->
    <ImageCropModal
      v-if="cropImageSrc"
      :src="cropImageSrc"
      :is-svg="cropIsSvg"
      @confirm="onCropConfirm"
      @cancel="cropImageSrc = ''"
    />

    <!-- 全局正下方药丸长条通知 (Toast Pills) -->
    <transition-group name="toast-list" tag="div" class="toast-wrap" role="status" aria-live="polite">
      <div v-for="t in toasts" :key="t.id" class="toast" :class="t.type" role="alert">
        <span class="t-icon" aria-hidden="true">
          <UiIcon
            :name="t.type === 'success' ? 'check' : (t.type === 'error' ? 'close' : (t.type === 'warn' ? 'warning' : 'info'))"
            :size="13"
          />
        </span>
        <span class="t-message">{{ t.msg }}</span>
        <button class="toast-close" type="button" title="关闭通知" aria-label="关闭通知" @click="dismissToast(t.id)">
          <UiIcon name="close" :size="11" />
        </button>
      </div>
    </transition-group>

    <ConfirmModal v-if="confirmDialog" :title="confirmDialog.title" :message="confirmDialog.message" :okText="confirmDialog.okText" :danger="confirmDialog.danger" @ok="handleConfirmOk" @cancel="closeConfirm" />
    <UpdateModal v-if="showUpdate" :initial-info="pendingUpdateInfo" @close="closeUpdateModal" />
  </div>
</template>
