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
import UiIcon from './components/UiIcon.vue'
import Modal from './components/Modal.vue'
import LoginModal from './components/LoginModal.vue'
import QuickOpen from './components/QuickOpen.vue'
import ConfirmModal from './components/ConfirmModal.vue'
import UpdateModal from './components/UpdateModal.vue'
import { CheckUpdate } from './api'

const tab = ref('pan')
const tabOrder = ['pan', 'transfer', 'sync', 'share', 'settings']
const prevTabIdx = ref(0)
const pageTrans = ref('page-slide-left')
function switchTab(key) {
  if (key === tab.value) return
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
const showTransferBall = computed(() => !!ball.value && getPrefs().transferBall !== false)
const ballPos = ref(null) // { x, y } 拖动后的固定位置
const isBallDragging = ref(false)

function onBallPointerDown(e) {
  const el = e.currentTarget
  const rect = el.getBoundingClientRect()
  const startX = e.clientX, startY = e.clientY
  const offX = startX - rect.left, offY = startY - rect.top
  let moved = false
  isBallDragging.value = true
  const onMove = (ev) => {
    const dx = ev.clientX - startX, dy = ev.clientY - startY
    if (!moved && Math.hypot(dx, dy) < 4) return
    moved = true
    const x = Math.min(Math.max(ev.clientX - offX, 4), window.innerWidth - rect.width - 4)
    const y = Math.min(Math.max(ev.clientY - offY, 52), window.innerHeight - rect.height - 4)
    ballPos.value = { x, y }
  }
  const cleanup = () => {
    isBallDragging.value = false
    window.removeEventListener('pointermove', onMove)
    window.removeEventListener('pointerup', onUp)
    window.removeEventListener('pointercancel', onCancel)
  }
  const onUp = () => {
    cleanup()
    if (!moved) switchTab('transfer')
  }
  const onCancel = cleanup
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
  const item = { id, msg: String(msg ?? ''), type: normalizedType }
  toasts.value.push(item)
  const lifetime = normalizedType === 'error' ? 6500 : 3600
  setTimeout(() => dismissToast(id), lifetime)
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
  const mq = window.matchMedia('(prefers-color-scheme: dark)')
  const onScheme = () => applyAppearance(curTheme.value)
  mq.addEventListener('change', onScheme)
  const offFns = [
    onEvent('account:changed', refresh),
    onEvent('app:ready', refresh),
    onEvent('share:history-error', (ev) => {
      toast(`分享已创建，但本地历史保存失败：${ev?.error || '未知错误'}`, 'warn')
    }),
    onEvent('transfer:event', (ev) => {
      if (!ev || !ev.task) return
      const t = ev.task
      if (ev.kind === 'download' && (t.status === 'downloading' || t.status === 'queued')) {
        ball.value = { down: t.speed || 0, up: (ball.value && ball.value.up) || 0 }
      } else if (ev.kind === 'upload' && (t.status === 'uploading' || t.status === 'queued')) {
        ball.value = { down: (ball.value && ball.value.down) || 0, up: t.speed || 0 }
      } else if (t.status === 'completed' || t.status === 'failed' || t.status === 'canceled') {
        // 并发传输时仅清零当前方向速度，保留对侧；两方向均无活跃才隐藏
        const cur = ball.value
        if (!cur) return
        const down = ev.kind === 'download' ? 0 : cur.down
        const up = ev.kind === 'upload' ? 0 : cur.up
        ball.value = (down || up) ? { down, up } : null
      }
    }),
  ]
  nextTick(updateGlider)
  window.addEventListener('resize', updateGlider)
  cleanupFns = () => {
    window.removeEventListener('resize', updateGlider)
    window.removeEventListener('keydown', onKey)
    mq.removeEventListener('change', onScheme)
    offFns.forEach((fn) => { try { fn && fn() } catch { /* noop */ } })
  }
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
            <PanView v-if="tab === 'pan'" ref="panView" :key="'pan'" :account="current" :accounts="accounts" :providers="providers" @toast="toast" @go="onPanGo" />
            <TransferView v-else-if="tab === 'transfer'" :key="'transfer'" :accounts="accounts" :providers="providers" @toast="toast" />
            <SyncView v-else-if="tab === 'sync'" :key="'sync'" :account="current" :accounts="accounts" :providers="providers" @toast="toast" />
            <ShareView v-else-if="tab === 'share'" :key="'share'" :accounts="accounts" :providers="providers" @toast="toast" />
            <SettingsView v-else :key="'settings'" @toast="toast" @theme="applyTheme" @update="showUpdate = true" @clear-cache="clearPanCache" />
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
        <span class="t-icon"><UiIcon :name="t.type === 'success' ? 'check' : (t.type === 'error' || t.type === 'warn' ? 'warning' : 'info')" :size="16" /></span>
        <span class="t-message">{{ t.msg }}</span>
        <button class="toast-close" type="button" title="关闭通知" aria-label="关闭通知" @click="dismissToast(t.id)"><UiIcon name="close" :size="14" /></button>
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
        <span v-if="ball && ball.down">↓ {{ formatSpeed(ball.down) }}</span>
        <span v-if="ball && ball.up">↑ {{ formatSpeed(ball.up) }}</span>
      </div>
    </transition>

    <ConfirmModal v-if="confirmDialog" :title="confirmDialog.title" :message="confirmDialog.message" :okText="confirmDialog.okText" :danger="confirmDialog.danger" @ok="handleConfirmOk" @cancel="closeConfirm" />
    <UpdateModal v-if="showUpdate" @close="showUpdate = false" />
  </div>
</template>
