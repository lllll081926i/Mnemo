<script setup>
import { ref, onMounted, onBeforeUnmount } from 'vue'
import Modal from './Modal.vue'
import UiIcon from './UiIcon.vue'
import ConfirmModal from './ConfirmModal.vue'
import { CheckUpdate, DownloadUpdate, ApplyUpdate, getSettings, onEvent } from '../api'

const props = defineProps({
  // App.vue 的后台静默检查会把已确认的版本传进来，避免弹窗再次请求
  // GitHub；从设置页手动打开时为空，才执行一次主动检查。
  initialInfo: { type: Object, default: null },
})
const emit = defineEmits(['close'])

const state = ref('idle') // idle | checking | available | downloading | done | applying | error
const info = ref(props.initialInfo ? { ...props.initialInfo } : null)
const progress = ref({ downloaded: 0, total: 0 })
const updatePath = ref('')
const errorMsg = ref('')
const confirmBeforeInstall = ref(true)
const installConfirmOpen = ref(false)
const installAfterDownload = ref(false)
let offProgress, offDone, offApplying, offError

function pct() {
  const t = progress.value.total || 0
  if (!t) return 0
  return Math.min(100, Math.round((progress.value.downloaded / t) * 100))
}

function fmtBytes(n) {
  const value = Number(n) || 0
  if (value < 1024) return `${Math.round(value)} B`
  const units = ['KB', 'MB', 'GB', 'TB']
  let scaled = value / 1024
  let unit = 0
  while (scaled >= 1024 && unit < units.length - 1) {
    scaled /= 1024
    unit++
  }
  return `${scaled.toFixed(scaled >= 100 ? 1 : 2)} ${units[unit]}`
}

async function check() {
  state.value = 'checking'
  errorMsg.value = ''
  try {
    const r = await CheckUpdate()
    if (r && r.available) {
      info.value = r
      state.value = 'available'
    } else {
      state.value = 'idle'
      emit('close')
    }
  } catch (e) {
    errorMsg.value = String(e)
    state.value = 'error'
  }
}

async function startDownload(installAfter = false) {
  installAfterDownload.value = installAfter
  state.value = 'downloading'
  progress.value = { downloaded: 0, total: info.value?.size || 0 }
  try {
    await DownloadUpdate(info.value?.url || '')
  } catch (e) {
    errorMsg.value = String(e)
    state.value = 'error'
  }
}

onMounted(() => {
  offProgress = onEvent('update:progress', (p) => {
    if (p && p.error) { errorMsg.value = p.error; state.value = 'error'; return }
    progress.value = { ...progress.value, ...(p || {}) }
  })
  offDone = onEvent('update:done', (payload) => {
    updatePath.value = payload?.path || ''
    const actualSize = Number(payload?.size) || Number(progress.value.total) || Number(info.value?.size) || 0
    progress.value = { ...progress.value, downloaded: actualSize, total: actualSize, size: actualSize }
    state.value = 'done'
    if (installAfterDownload.value) void install()
  })
  offApplying = onEvent('update:applying', () => { state.value = 'applying' })
  offError = onEvent('update:error', (e) => { errorMsg.value = e?.error || '更新失败'; state.value = 'error' })
  getSettings().then((s) => { confirmBeforeInstall.value = s?.confirmUpdate !== false }).catch(() => {})
  if (info.value && info.value.available !== false) {
    state.value = 'available'
  } else {
    void check()
  }
})

onBeforeUnmount(() => { offProgress && offProgress(); offDone && offDone(); offApplying && offApplying(); offError && offError() })

async function install() {
  if (confirmBeforeInstall.value) {
    installConfirmOpen.value = true
    return
  }
  await applyUpdate()
}

async function applyUpdate() {
  installConfirmOpen.value = false
  const path = updatePath.value || progress.value.path || ''
  if (!path) {
    errorMsg.value = '更新文件路径不可用，请重新下载'
    state.value = 'error'
    return
  }
  try {
    state.value = 'applying'
    await ApplyUpdate(path)
  } catch (e) {
    errorMsg.value = String(e)
      state.value = 'error'
  }
}

function closeLater() { emit('close') }
</script>

<template>
  <Modal title="检查更新" width="420px" @close="emit('close')">
    <div class="upd-body">
      <!-- checking -->
      <div v-if="state === 'checking'" class="upd-state">
        <span class="spin"></span><span>正在检查更新…</span>
      </div>

      <!-- available -->
      <div v-else-if="state === 'available'" class="upd-state">
        <UiIcon name="download" :size="32" />
        <p class="upd-title">发现新版本 <b>{{ info.version }}</b></p>
        <p v-if="info.size" class="upd-meta">安装包大小：{{ fmtBytes(info.size) }}</p>
        <div class="upd-actions">
          <button class="btn primary" @click="startDownload(true)">下载并安装</button>
          <button class="btn" @click="closeLater">稍后</button>
        </div>
      </div>

      <!-- downloading -->
      <div v-else-if="state === 'downloading'" class="upd-state">
        <p class="upd-title">正在下载更新…</p>
        <div class="upd-progress">
          <div class="upd-bar" :style="{ width: pct() + '%' }"></div>
        </div>
        <p class="upd-meta">{{ fmtBytes(progress.downloaded) }} / {{ fmtBytes(progress.total) }} · {{ pct() }}%</p>
      </div>

      <!-- done -->
      <div v-else-if="state === 'done'" class="upd-state">
        <UiIcon name="check" :size="32" />
        <p class="upd-title">下载完成</p>
        <p v-if="progress.size" class="upd-meta">实际文件大小：{{ fmtBytes(progress.size) }}</p>
        <div class="upd-actions">
          <button class="btn primary" @click="install">安装并重启</button>
          <button class="btn" @click="closeLater">稍后</button>
        </div>
      </div>

      <div v-else-if="state === 'applying'" class="upd-state">
        <span class="spin"></span><span>正在安装更新，应用即将重启…</span>
      </div>

      <!-- error -->
      <div v-else-if="state === 'error'" class="upd-state">
        <UiIcon name="warning" :size="32" />
        <p class="upd-title">更新失败</p>
        <p class="upd-err">{{ errorMsg }}</p>
        <button class="btn" @click="check">重试</button>
      </div>
    </div>
  </Modal>
  <ConfirmModal
    v-if="installConfirmOpen"
    title="确认安装更新"
    message="安装更新会关闭并重启 Mnemo，未完成的操作请先确认已保存。"
    ok-text="安装并重启"
    @ok="applyUpdate"
    @cancel="installConfirmOpen = false"
  />
</template>

<style scoped>
.upd-body { padding: 8px 0; }
.upd-state { display: flex; flex-direction: column; align-items: center; gap: 12px; padding: 16px 0; text-align: center; }
.upd-title { font-size: var(--fs-body); color: var(--text-primary); margin: 0; }
.upd-title b { color: var(--color-primary); }
.upd-meta { font-size: var(--fs-aux); color: var(--text-tertiary); margin: 0; }
.upd-err { font-size: var(--fs-aux); color: var(--color-danger); margin: 0; word-break: break-all; }
.upd-progress { width: 100%; height: 6px; background: var(--bg-subtle); border-radius: var(--radius-full); overflow: hidden; }
.upd-bar { height: 100%; background: var(--color-primary); border-radius: var(--radius-full); transition: width 150ms linear; }
.upd-actions { display: flex; gap: 8px; justify-content: center; }
.btn { min-width: 120px; }
</style>
