<script setup>
import { ref, onMounted, computed, watch, nextTick, onBeforeUnmount } from 'vue'
import { listAccounts, listProviders, removeAccount, onEvent, GetSettings, SaveSettings, formatSpeed, providerOf, accountName } from './api'
import { applyAppearance } from './appearance'
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
const ball = ref(null) // { down, up }
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
  const onUp = () => {
    isBallDragging.value = false
    window.removeEventListener('pointermove', onMove)
    window.removeEventListener('pointerup', onUp)
    if (!moved) switchTab('transfer')
  }
  window.addEventListener('pointermove', onMove)
  window.addEventListener('pointerup', onUp)
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

function refresh() {
  listAccounts().then((list) => {
    accounts.value = list || []
    if (current.value) {
      const found = accounts.value.find((a) => a.user_id === current.value.user_id)
      current.value = found || accounts.value[0] || null
    } else if (accounts.value.length) {
      current.value = accounts.value[0]
    }
  }).catch(() => {})
}

function select(acc) { current.value = acc }

function onPanGo(target) {
  if (target === 'login') showLogin.value = true
  else switchTab(target)
}

function providerLabel(acc) {
  const p = providers.value.find((x) => x.ID === providerOf(acc.user_id))
  return p ? p.Meta.label : providerOf(acc.user_id)
}

async function remove(acc) {
  if (!confirm(`移除账号「${(acc.token && (acc.token.nick_name || acc.token.user_name)) || acc.user_id}」？本地记录将被删除。`)) return
  try {
    await removeAccount(acc.user_id)
    if (current.value && current.value.user_id === acc.user_id) current.value = null
    refresh()
    toast('账号已移除', 'success')
  } catch (e) { toast(String(e), 'error') }
}

function toast(msg, type = '') {
  const id = Date.now() + Math.random()
  toasts.value.push({ id, msg, type })
  setTimeout(() => { toasts.value = toasts.value.filter((t) => t.id !== id) }, 3200)
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
  window.addEventListener('keydown', onKey)
  const mq = window.matchMedia('(prefers-color-scheme: dark)')
  const onScheme = () => applyAppearance(curTheme.value)
  mq.addEventListener('change', onScheme)
  const offFns = [
    onEvent('account:changed', refresh),
    onEvent('app:ready', refresh),
    onEvent('transfer:event', (ev) => {
      if (!ev || !ev.task) return
      const t = ev.task
      if (ev.kind === 'download' && (t.status === 'downloading' || t.status === 'queued')) {
        ball.value = { down: t.speed || 0, up: (ball.value && ball.value.up) || 0 }
      } else if (ev.kind === 'upload' && (t.status === 'uploading' || t.status === 'queued')) {
        ball.value = { down: (ball.value && ball.value.down) || 0, up: t.speed || 0 }
      } else if (t.status === 'completed' || t.status === 'failed' || t.status === 'canceled') {
        ball.value = null
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
          <PanView v-if="tab === 'pan'" ref="panView" :key="'pan'" :account="current" :accounts="accounts" :providers="providers" @toast="toast" @go="onPanGo" />
          <TransferView v-else-if="tab === 'transfer'" :key="'transfer'" :accounts="accounts" :providers="providers" @toast="toast" />
          <SyncView v-else-if="tab === 'sync'" :key="'sync'" :account="current" :accounts="accounts" :providers="providers" @toast="toast" />
          <ShareView v-else-if="tab === 'share'" :key="'share'" :accounts="accounts" :providers="providers" @toast="toast" />
          <SettingsView v-else :key="'settings'" @toast="toast" @theme="applyTheme" />
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

    <transition-group name="toast-list" tag="div" class="toast-wrap">
      <div v-for="t in toasts" :key="t.id" class="toast" :class="t.type">
        <span v-if="t.type" class="t-icon"><UiIcon :name="t.type === 'error' ? 'warning' : 'check'" :size="14" /></span>
        <span>{{ t.msg }}</span>
      </div>
    </transition-group>

    <div
      v-if="ball"
      class="transfer-ball"
      :class="{ dragging: isBallDragging }"
      :style="ballPos ? { left: ballPos.x + 'px', top: ballPos.y + 'px', right: 'auto', bottom: 'auto' } : {}"
      title="传输中，点击打开传输页 (Alt+2)，可拖动"
      @pointerdown="onBallPointerDown"
    >
      <span class="pulse"></span>
      <span v-if="ball.down">↓ {{ formatSpeed(ball.down) }}</span>
      <span v-if="ball.up">↑ {{ formatSpeed(ball.up) }}</span>
    </div>
  </div>
</template>
