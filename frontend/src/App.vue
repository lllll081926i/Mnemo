<script setup>
import { ref, onMounted, computed } from 'vue'
import { listAccounts, listProviders, removeAccount, onEvent } from './api'
import PanView from './views/PanView.vue'
import TransferView from './views/TransferView.vue'
import ShareView from './views/ShareView.vue'
import SyncView from './views/SyncView.vue'
import SettingsView from './views/SettingsView.vue'
import LoginModal from './components/LoginModal.vue'

const tab = ref('pan')
const accounts = ref([])
const providers = ref([])
const current = ref(null)
const showLogin = ref(false)
const toasts = ref([])
const ballSpeed = ref(0)
const ballActive = ref(false)

const tabs = [
  { key: 'pan', label: '网盘', icon: '🗂️' },
  { key: 'transfer', label: '传输', icon: '⚡' },
  { key: 'share', label: '分享', icon: '🔗' },
  { key: 'sync', label: '同步', icon: '🔄' },
  { key: 'settings', label: '设置', icon: '⚙️' },
]

const providerMeta = computed(() => {
  const m = {}
  for (const p of providers.value) m[p.ID] = p.Meta
  return m
})

function refresh() {
  listAccounts().then((list) => {
    accounts.value = list || []
    if (!current.value && accounts.value.length) current.value = accounts.value[0]
  })
}

function select(acc) { current.value = acc }

async function remove(userId) {
  await removeAccount(userId)
  if (current.value && current.value.UserID === userId) current.value = null
  refresh()
}

function toast(msg, type = '') {
  const id = Date.now()
  toasts.value.push({ id, msg, type })
  setTimeout(() => { toasts.value = toasts.value.filter((t) => t.id !== id) }, 3000)
}

onMounted(() => {
  listProviders().then((p) => { providers.value = p || [] })
  refresh()
  onEvent('account:changed', refresh)
  onEvent('app:ready', () => refresh())
  onEvent('transfer:event', (ev) => {
    if (ev && ev.Kind === 'download' && ev.Task && ev.Task.Status === 'downloading') {
      ballSpeed.value = ev.Task.Speed || 0
      ballActive.value = true
    } else if (ev && ev.Kind === 'download') {
      // keep ball while any task is active
    }
  })
})

defineExpose({ toast })
</script>

<template>
  <div class="app-shell">
    <aside class="account-rail">
      <div class="brand">
        <img src="./assets/icon.svg" alt="" />
        <span>Mnemo</span>
      </div>
      <div
        v-for="acc in accounts"
        :key="acc.UserID"
        class="acc-item"
        :class="{ active: current && current.UserID === acc.UserID }"
        @click="select(acc)"
      >
        <img class="acc-icon" :src="providerIcon(acc.UserID)" alt="" />
        <span class="acc-name">{{ accName(acc) }}</span>
        <span class="remove" @click.stop="remove(acc.UserID)">✕</span>
      </div>
      <button class="btn primary rail-add" @click="showLogin = true">+ 添加账号</button>
      <nav class="nav-tabs">
        <div
          v-for="t in tabs"
          :key="t.key"
          class="nav-tab"
          :class="{ active: tab === t.key }"
          @click="tab = t.key"
        >
          <span>{{ t.icon }}</span><span>{{ t.label }}</span>
        </div>
      </nav>
    </aside>

    <main class="main-area">
      <PanView v-if="tab === 'pan'" :account="current" :provider-meta="providerMeta" @toast="toast" />
      <TransferView v-else-if="tab === 'transfer'" @toast="toast" />
      <ShareView v-else-if="tab === 'share'" :account="current" @toast="toast" />
      <SyncView v-else-if="tab === 'sync'" :account="current" @toast="toast" />
      <SettingsView v-else :account="current" @toast="toast" />
    </main>

    <LoginModal v-if="showLogin" :providers="providers" @close="showLogin = false" @toast="toast" />

    <div class="toast-wrap">
      <div v-for="t in toasts" :key="t.id" class="toast" :class="t.type">{{ t.msg }}</div>
    </div>

    <div v-if="ballActive" class="transfer-ball" title="打开传输页" @click="tab = 'transfer'">
      ⚡ {{ fmtSpeed(ballSpeed) }}
    </div>
  </div>
</template>

<script>
export default {
  methods: {
    fmtSpeed(bps) {
      if (!bps || bps <= 0) return '0 B/s'
      const units = ['B', 'KB', 'MB', 'GB']
      let v = bps, i = 0
      while (v >= 1024 && i < units.length - 1) { v /= 1024; i++ }
      return (i === 0 ? v : v.toFixed(1)) + ' ' + units[i] + '/s'
    },
    providerIcon(userId) {
      const p = (this.providers || []).find((x) => userId.startsWith(x.ID + '_') || userId.startsWith(x.ID + ':'))
      return p ? new URL('./assets/drive-icons/' + p.Meta.Icon.replace('drive-icons/', ''), import.meta.url).href : ''
    },
    accName(acc) {
      if (acc.Token && acc.Token.NickName) return acc.Token.NickName
      return acc.UserID
    },
  },
}
</script>

<style>
.transfer-ball {
  position: fixed; bottom: 24px; right: 24px; z-index: 250;
  background: var(--color-primary); color: #fff; border-radius: 999px;
  padding: 10px 16px; font-size: 13px; font-weight: 600;
  box-shadow: var(--shadow-modal); cursor: pointer; user-select: none;
}
.transfer-ball:hover { background: var(--color-primary-hover); }
</style>