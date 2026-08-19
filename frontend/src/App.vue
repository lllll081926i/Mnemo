<script setup>
import { ref, onMounted, computed, watch, nextTick, onBeforeUnmount } from 'vue'
import { listAccounts, listProviders, removeAccount, onEvent, GetSettings, SaveSettings, formatSpeed, providerOf, accountName } from './api'
import { applyAppearance, getPrefs, getLastDriveSelection, setLastDriveSelection, clearLastDriveSelection } from './appearance'
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
import { CheckUpdate } from './api'
import { debug, info, installGlobalErrorLogging } from './logger'

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
  if (tab.value === 'pan') listeners.go = onPanGo
  if (tab.value === 'settings') {
    listeners.theme = applyTheme
    listeners.update = () => { showUpdate.value = true }
    listeners.clearCache = clearPanCache
  }
  return listeners
})
function switchTab(key) {
	if (key === tab.value) return
	info('navigation', 'page switch requested', { from: tab.value, to: key })
  const ni = tabOrder.indexOf(key)
  pageTrans.value = ni >= tabOrder.indexOf(tab.value) ? 'page-slide-left' : 'page-slide-right'
  prevTabIdx.value = tabOrder.indexOf(tab.value)
  tab.value = key
}
const accounts = ref([])
const providers = ref([])
const current = ref(null)
const showLogin = ref(false)
const showQuickOpen = ref(false)
const showUpdate = ref(false)
const infoAcc = ref(null)
const curTheme = ref('system')
const isDark = ref(false)
applyAppearance('system') // 防启动闪白，随后以设置为准
isDark.value = document.documentElement.classList.contains('dark')

function quickToggleTheme() {
  applyTheme(isDark.value ? 'light' : 'dark')
  saveThemePref()
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
const ball = ref(null) // { down, up }
const prefsRevision = ref(0)
const showTransferBall = computed(() => {
  // Preferences live in localStorage, so a small revision ref bridges changes
  // made from SettingsView into this long-lived root component.
  prefsRevision.value
  return !!ball.value && getPrefs().transferBall !== false
})
const ballPos = ref(null) // { x, y } 拖动后的固定位置
const isBallDragging = ref(false)
const transferActivities = new Map()
let ballUpdateTimer = null

function onPrefsChanged() {
  prefsRevision.value += 1
}

function scheduleTransferBallUpdate() {
  if (ballUpdateTimer) return
  ballUpdateTimer = setTimeout(() => {
    ballUpdateTimer = null
    let down = 0
    let up = 0
    let hasDown = false
    let hasUp = false
    transferActivities.forEach((activity) => {
      if (activity.kind === 'download') {
        hasDown = true
        down += activity.speed
      } else if (activity.kind === 'upload') {
        hasUp = true
        up += activity.speed
      }
    })
    const next = hasDown || hasUp ? { down, up } : null
    if (!next || !ball.value || next.down !== ball.value.down || next.up !== ball.value.up) ball.value = next
  }, 300)
}

function updateTransferBall(ev) {
  if (!ev || !ev.task || !ev.task.id) return
  const task = ev.task
  const active = task.status === 'downloading' || task.status === 'uploading' || task.status === 'queued'
  const key = `${ev.kind}:${task.id}`
  if (active) transferActivities.set(key, { kind: ev.kind, speed: Number(task.speed) || 0 })
  else transferActivities.delete(key)
  scheduleTransferBallUpdate()
}

function onBallPointerDown(e) {
  if (e.button !== undefined && e.button !== 0) return
  e.preventDefault()
  const el = e.currentTarget
  const rect = el.getBoundingClientRect()
  const startX = e.clientX, startY = e.clientY
  const offX = startX - rect.left, offY = startY - rect.top
  let moved = false
  let frame = 0
  let pending = null
  let current = { x: rect.left, y: rect.top }
  isBallDragging.value = true

  const applyPending = () => {
    frame = 0
    if (!pending) return
    current = pending
    pending = null
    el.style.transform = `translate3d(${current.x - rect.left}px, ${current.y - rect.top}px, 0)`
  }

  const onMove = (ev) => {
    const dx = ev.clientX - startX, dy = ev.clientY - startY
    if (!moved && Math.hypot(dx, dy) < 4) return
    if (!moved) {
      // Anchor the element once, then move it on the compositor instead of
      // updating the Vue tree for every pointer event.
      el.style.left = `${rect.left}px`
      el.style.top = `${rect.top}px`
      el.style.right = 'auto'
      el.style.bottom = 'auto'
    }
    moved = true
    const maxX = Math.max(4, window.innerWidth - rect.width - 4)
    const maxY = Math.max(52, window.innerHeight - rect.height - 4)
    pending = {
      x: Math.min(Math.max(ev.clientX - offX, 4), maxX),
      y: Math.min(Math.max(ev.clientY - offY, 52), maxY),
    }
    // Pointer events can arrive faster than Vue needs to render. Coalesce them
    // into one update per frame so dragging does not cause a render storm.
    if (!frame) frame = requestAnimationFrame(applyPending)
  }
  const cleanup = () => {
    if (frame) cancelAnimationFrame(frame)
    frame = 0
    if (pending) {
      current = pending
      pending = null
      el.style.transform = `translate3d(${current.x - rect.left}px, ${current.y - rect.top}px, 0)`
    }
    isBallDragging.value = false
    window.removeEventListener('pointermove', onMove)
    window.removeEventListener('pointerup', onUp)
    window.removeEventListener('pointercancel', onCancel)
    try { el.releasePointerCapture?.(e.pointerId) } catch { /* pointer already released */ }
  }
  const onUp = () => {
    cleanup()
    if (!moved) {
      switchTab('transfer')
      return
    }
    ballPos.value = { x: Math.round(current.x), y: Math.round(current.y) }
    nextTick(() => {
      if (!isBallDragging.value && el.isConnected) el.style.transform = ''
    })
  }
  const onCancel = () => {
    cleanup()
    if (moved) {
      ballPos.value = { x: Math.round(current.x), y: Math.round(current.y) }
      nextTick(() => {
        if (!isBallDragging.value && el.isConnected) el.style.transform = ''
      })
    }
  }
  try { el.setPointerCapture?.(e.pointerId) } catch { /* unsupported by older WebView */ }
  window.addEventListener('pointermove', onMove)
  window.addEventListener('pointerup', onUp)
  window.addEventListener('pointercancel', onCancel)
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
function refresh() {
  const my = ++refreshEpoch
  listAccounts().then((list) => {
    if (my !== refreshEpoch) return
    accounts.value = list || []
    const available = accounts.value
    if (current.value) {
      const found = available.find((a) => a.user_id === current.value.user_id)
      current.value = found || available[0] || null
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
  }).catch(() => {})
}

function select(acc) {
  if (!acc) return
  current.value = acc
  setLastDriveSelection(acc.user_id, acc.drive_id)
}

function onPanGo(target) {
  if (target === 'login') showLogin.value = true
  else switchTab(target)
}

function clearPanCache() {
  panView.value?.clearCache?.()
}

function providerLabel(acc) {
  const p = providers.value.find((x) => x.ID === providerOf(acc.user_id))
  return p ? p.Meta.label : providerOf(acc.user_id)
}

function remove(acc) {
  askConfirm(`移除账号「${(acc.token && (acc.token.nick_name || acc.token.user_name)) || acc.user_id}」？本地记录将被删除。`, async () => {
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

function onQuickAction(action) {
  if (action === 'toggle-theme') quickToggleTheme()
  else if (action === 'refresh') {
    switchTab('pan')
    panView.value?.refresh()
  } else if (action === 'mkdir') {
    switchTab('pan')
    panView.value?.openMkdirModal()
  } else if (action === 'upload') {
    switchTab('pan')
    panView.value?.openUploadModal()
  }
}

onMounted(async () => {
	info('app', 'frontend mounted')
	const removeGlobalErrorLogging = installGlobalErrorLogging()
	listProviders().then((p) => { providers.value = p || [] }).catch(() => {})
  refresh()
  try {
    const s = await GetSettings()
    // 颜色模式默认跟随系统；顶栏可手动切换（不出现在设置页）
    if (s) applyTheme(s.theme || 'system')
  } catch { /* 默认跟随系统 */ }
  // 启动后延迟检查更新（静默，仅发现有新版时弹窗）
  setTimeout(() => {
    CheckUpdate().then((r) => { if (r && r.available) showUpdate.value = true }).catch(() => {})
  }, 3000)
  window.addEventListener('keydown', onKey)
  window.addEventListener('mnemo:prefs-changed', onPrefsChanged)
  const mq = window.matchMedia('(prefers-color-scheme: dark)')
  const onScheme = () => applyAppearance(curTheme.value)
  mq.addEventListener('change', onScheme)
  const offFns = [
    onEvent('account:changed', refresh),
    onEvent('app:ready', refresh),
    onEvent('share:history-error', (ev) => {
      toast(`分享已创建，但本地历史保存失败：${ev?.error || '未知错误'}`, 'warn')
    }),
    onEvent('transfer:event', updateTransferBall),
  ]
  nextTick(updateGlider)
  window.addEventListener('resize', updateGlider)
	cleanupFns = () => {
		removeGlobalErrorLogging()
    window.removeEventListener('resize', updateGlider)
    window.removeEventListener('keydown', onKey)
    window.removeEventListener('mnemo:prefs-changed', onPrefsChanged)
    mq.removeEventListener('change', onScheme)
    clearTimeout(ballUpdateTimer)
    ballUpdateTimer = null
    transferActivities.clear()
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
      />
      <main class="page-host">
        <transition :name="pageTrans" mode="out-in">
          <KeepAlive>
            <component
              :is="pageComponent"
              :key="tab"
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
      <div class="kv-row"><span class="kv-label">网盘</span><span>{{ providerLabel(infoAcc) }}</span></div>
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

    <transition-group name="toast-list" tag="div" class="toast-wrap" role="status" aria-live="polite">
      <div v-for="t in toasts" :key="t.id" class="toast" :class="t.type" role="alert">
        <span class="t-icon" aria-hidden="true"><UiIcon :name="t.type === 'success' ? 'check' : (t.type === 'error' || t.type === 'warn' ? 'warning' : 'info')" :size="17" /></span>
        <div class="t-content">
          <div class="t-head"><span class="t-label">{{ t.label }}</span><span class="t-dot">·</span><span class="t-context">Mnemo</span></div>
          <div class="t-message">{{ t.msg }}</div>
        </div>
        <button class="toast-close" type="button" title="关闭通知" aria-label="关闭通知" @click="dismissToast(t.id)"><UiIcon name="close" :size="14" /></button>
        <span class="t-timebar" aria-hidden="true"></span>
      </div>
    </transition-group>

    <transition name="popover-zoom">
      <div
        v-if="showTransferBall"
        class="transfer-ball"
        :class="{ dragging: isBallDragging }"
        :style="ballPos ? { left: ballPos.x + 'px', top: ballPos.y + 'px', right: 'auto', bottom: 'auto' } : {}"
        title="传输中，点击打开传输页 (Alt+2)，可拖动"
        @pointerdown="onBallPointerDown"
      >
        <span class="pulse"></span>
        <span class="transfer-stat" :class="{ idle: !(ball && ball.down) }">
          <UiIcon name="download" :size="13" />
          <span>{{ ball && ball.down ? formatSpeed(ball.down) : '—' }}</span>
        </span>
        <span class="transfer-stat" :class="{ idle: !(ball && ball.up) }">
          <UiIcon name="upload" :size="13" />
          <span>{{ ball && ball.up ? formatSpeed(ball.up) : '—' }}</span>
        </span>
      </div>
    </transition>

    <ConfirmModal v-if="confirmDialog" :title="confirmDialog.title" :message="confirmDialog.message" :okText="confirmDialog.okText" :danger="confirmDialog.danger" @ok="handleConfirmOk" @cancel="closeConfirm" />
    <UpdateModal v-if="showUpdate" @close="showUpdate = false" />
  </div>
</template>
