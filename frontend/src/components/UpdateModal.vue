<script setup>
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import Modal from './Modal.vue'
import UiIcon from './UiIcon.vue'
import ConfirmModal from './ConfirmModal.vue'
import appIcon from '../assets/logo/icon.svg'
import packageInfo from '../../package.json'
import { CheckUpdate, GetUpdateStatus, DownloadUpdate, CancelUpdate, ApplyUpdate, RevealInFolder, OpenBrowser, getSettings, onEvent } from '../api'

const props = defineProps({ initialInfo: { type: Object, default: null } })
const emit = defineEmits(['close'])
const state = ref(props.initialInfo?.available ? 'available' : 'checking')
const info = ref(props.initialInfo ? { ...props.initialInfo } : null)
const progress = ref({ downloaded: 0, total: 0 })
const updatePath = ref('')
const errorMsg = ref('')
const confirmBeforeInstall = ref(true)
const installConfirmOpen = ref(false)
const canceling = ref(false)
const canInstall = computed(() => info.value?.canInstall === true)
const busy = computed(() => ['checking', 'downloading', 'verifying', 'applying'].includes(state.value))
const presentation = computed(() => ({
  checking: { icon: 'refresh', title: '正在检查更新…', description: '正在获取最新正式版信息。' },
  idle: { icon: 'check', title: '已是最新正式版', description: '当前版本可以继续使用。' },
  available: { icon: 'download', title: '发现新版本', description: '新版本已准备好，可下载更新包。' },
  canceled: { icon: 'download', title: '下载已取消', description: '可以重新下载，也可以稍后更新。' },
  downloading: { icon: 'download', title: '正在下载更新…', description: '关闭此窗口后，下载仍会继续。' },
  verifying: { icon: 'check', title: '正在校验更新包…', description: '正在核对下载文件的 SHA-256。' },
  done: { icon: 'check', title: '下载完成，校验通过', description: canInstall.value ? '安装更新将退出并重启应用。' : '退出当前应用后，使用更新包手动安装。' },
  applying: { icon: 'refresh', title: '正在启动安装程序…', description: '请在系统提示中允许授权。' },
  error: { icon: 'warning', title: '更新未完成', description: '请重试，或前往发布页查看更新。' },
}[state.value] || { icon: 'warning', title: '更新状态暂不可用', description: '请重新检查更新。' }))
let alive = true, sequence = 0, revision = -1, offState

function pct() {
  const total = progress.value.total || 0
  return total > 0 ? Math.min(100, Math.round(progress.value.downloaded / total * 100)) : 0
}
function fmtBytes(n) {
  let value = Math.max(0, Number(n) || 0)
  const units = ['B', 'KB', 'MB', 'GB']
  let i = 0
  while (value >= 1024 && i < units.length - 1) { value /= 1024; i++ }
  return (i ? value.toFixed(1) : String(Math.round(value))) + ' ' + units[i]
}
function receive(status) {
  if (!alive || !status || Number(status.revision) < revision) return false
  revision = Number(status.revision) || 0
  state.value = status.phase || 'idle'
  if (status.info) info.value = status.info
  progress.value = { downloaded: status.downloaded || 0, total: status.total || status.info?.size || 0 }
  updatePath.value = status.path || ''
  errorMsg.value = status.error || ''
  canceling.value = false
  return true
}
async function check() {
  if (['downloading', 'verifying', 'applying'].includes(state.value)) return
  const seq = ++sequence
  state.value = 'checking'; errorMsg.value = ''
  try {
    const result = await CheckUpdate()
    if (!alive || seq !== sequence) return
    info.value = result
    state.value = result?.available ? 'available' : 'idle'
    updatePath.value = ''
  } catch (e) {
    if (!alive || seq !== sequence) return
    errorMsg.value = String(e); state.value = 'error'
  }
}
async function startDownload() {
  if (busy.value) return
  state.value = 'downloading'; errorMsg.value = ''; updatePath.value = ''
  progress.value = { downloaded: 0, total: info.value?.size || 0 }
  try {
    await DownloadUpdate(info.value?.url || '')
    if (alive) receive(await GetUpdateStatus())
  } catch (e) {
    if (alive) { errorMsg.value = String(e); state.value = 'error' }
  }
}
async function cancelDownload() {
  if (canceling.value) return
  canceling.value = true
  try {
    await CancelUpdate()
    if (alive) receive(await GetUpdateStatus())
  } catch (e) {
    if (alive) { errorMsg.value = String(e); canceling.value = false }
  }
}
async function install() {
  if (busy.value || !canInstall.value) return
  if (confirmBeforeInstall.value) { installConfirmOpen.value = true; return }
  await applyUpdate()
}
async function applyUpdate() {
  installConfirmOpen.value = false
  if (busy.value || !updatePath.value) return
  state.value = 'applying'; errorMsg.value = ''
  try { await ApplyUpdate(updatePath.value) }
  catch (e) {
    if (!alive) return
    try { receive(await GetUpdateStatus()) }
    catch { updatePath.value = '' }
    if (alive) { errorMsg.value = String(e); state.value = 'error' }
  }
}
async function openFolder() {
  if (!updatePath.value) return
  try { await RevealInFolder(updatePath.value) }
  catch (e) { errorMsg.value = String(e) }
}
async function openRelease() {
  if (!info.value?.releaseUrl) return
  try { await OpenBrowser(info.value.releaseUrl) }
  catch (e) { errorMsg.value = String(e) }
}

onMounted(async () => {
  offState = onEvent('update:state', status => {
    if (receive(status)) ++sequence
  })
  getSettings().then(s => { if (alive) confirmBeforeInstall.value = s?.confirmUpdate !== false }).catch(() => {})
  try {
    const status = await GetUpdateStatus()
    if (!alive) return
    if (!receive(status)) return
    if (['downloading', 'verifying', 'done', 'applying', 'canceled', 'error'].includes(state.value)) return
    if (props.initialInfo?.available) { info.value = props.initialInfo; state.value = 'available'; return }
    await check()
  } catch { if (alive) await check() }
})
onBeforeUnmount(() => { alive = false; ++sequence; offState?.() })
</script>

<template>
  <Modal title="更新" width="480px" dialog-class="update-dialog" @close="emit('close')">
    <div class="upd-body">
      <div class="upd-product">
        <img :src="appIcon" alt="" draggable="false" />
        <div><strong>Mnemo</strong><p class="upd-meta upd-current">当前版本 {{ info?.currentVersion || packageInfo.version }}</p></div>
        <span class="upd-channel">正式版</span>
      </div>
      <Transition name="update-state" mode="out-in">
        <section :key="state" class="upd-state" :class="'upd-state-' + state" aria-live="polite" :aria-busy="busy">
          <div class="upd-status-heading">
            <span class="upd-status-icon"><span v-if="busy" class="spin"></span><UiIcon v-else :name="presentation.icon" :size="21" /></span>
            <div><h4 class="upd-title">{{ presentation.title }}</h4><p class="upd-meta">{{ presentation.description }}</p></div>
          </div>
          <div v-if="info?.version && state !== 'idle' && state !== 'checking'" class="upd-version">
            <span>新版本 <strong>{{ info.version }}</strong></span><span v-if="info?.size">{{ fmtBytes(info.size) }}</span>
          </div>
          <div v-if="state === 'downloading' || state === 'verifying'" class="upd-download">
            <div class="upd-download-meta"><span>{{ fmtBytes(progress.downloaded) }} / {{ progress.total > 0 ? fmtBytes(progress.total) : '大小未知' }}</span><strong v-if="progress.total > 0">{{ pct() }}%</strong></div>
            <div class="upd-progress" role="progressbar" aria-label="更新下载进度" :aria-valuenow="progress.total > 0 ? pct() : undefined" aria-valuemin="0" aria-valuemax="100"><div class="upd-bar" :class="{ indeterminate: progress.total <= 0 }" :style="{ width: progress.total > 0 ? pct() + '%' : '35%' }"></div></div>
          </div>
          <p v-if="state === 'done' && !updatePath" class="upd-meta">安装包路径不可用，请重新下载或检查更新。</p>
          <p v-if="(state === 'available' || state === 'canceled') && !canInstall" class="upd-meta">下载完成后请手动安装更新包。</p>
          <div v-if="state !== 'checking' && state !== 'applying'" class="upd-actions">
            <template v-if="state === 'idle'"><button class="btn" @click="check">重新检查</button><button class="btn primary" @click="emit('close')">完成</button></template>
            <template v-else-if="state === 'available' || state === 'canceled'"><button class="btn" @click="emit('close')">稍后</button><button class="btn primary" @click="startDownload">{{ state === 'canceled' ? '重新下载' : '下载更新' }}</button></template>
            <button v-else-if="state === 'downloading' || state === 'verifying'" class="btn" :disabled="canceling" @click="cancelDownload">{{ canceling ? '取消中…' : '取消下载' }}</button>
            <template v-else-if="state === 'done'"><button class="btn" :disabled="!updatePath" @click="openFolder">打开目录</button><button v-if="!updatePath" class="btn" @click="info?.available ? startDownload() : check()">{{ info?.available ? '重新下载' : '重新检查' }}</button><button v-if="canInstall" class="btn primary" :disabled="!updatePath" @click="install">安装并重启</button></template>
            <template v-else-if="state === 'error'"><button class="btn" @click="check">重新检查</button><button v-if="info?.available" class="btn" @click="startDownload">重新下载</button><button v-if="canInstall && updatePath" class="btn primary" @click="install">重试安装</button></template>
            <button v-else class="btn" @click="check">重新检查</button>
          </div>
        </section>
      </Transition>
      <p v-if="errorMsg" class="upd-err" role="alert">{{ errorMsg }}</p>
      <details v-if="info?.notes" class="upd-notes" open><summary>更新内容</summary><pre>{{ info.notes }}</pre></details>
      <div class="upd-footer">
        <details class="upd-source" aria-label="更新来源与校验范围"><summary>更新来源与校验</summary><p>GitHub · lllll081926i/Mnemo</p><p>下载后核对 SHA-256，不代表已验证发布者签名。</p></details>
        <button v-if="info?.releaseUrl" class="upd-release" @click="openRelease">查看发布页<UiIcon name="external" :size="12" /></button>
      </div>
    </div>
  </Modal>
  <ConfirmModal v-if="installConfirmOpen" title="安装更新" message="安装更新将退出并重启应用，传输任务将会中断。是否继续？" ok-text="安装并重启" @ok="applyUpdate" @cancel="installConfirmOpen = false" />
</template>

<style scoped>
.upd-body { display: grid; gap: 16px; padding: 0 0 4px; }
.upd-product { display: flex; align-items: center; gap: 10px; padding: 0 2px; }
.upd-product img { width: 36px; height: 36px; flex: none; }
.upd-product strong { color: var(--text-primary); font-size: 15px; font-weight: 650; }
.upd-channel { margin-left: auto; padding: 3px 8px; border: 1px solid var(--border-light); border-radius: var(--radius-full); color: var(--text-secondary); font-size: 11px; }
.upd-state { display: grid; gap: 16px; padding: 18px; border: 1px solid var(--border-light); border-radius: var(--radius-lg); background: var(--bg-base); }
.upd-status-heading { display: flex; align-items: center; gap: 12px; }
.upd-status-heading > div { min-width: 0; }
.upd-status-icon { display: flex; align-items: center; justify-content: center; flex: none; width: 40px; height: 40px; border-radius: var(--radius-md); background: var(--bg-active); color: var(--color-primary); }
.upd-state-idle .upd-status-icon, .upd-state-done .upd-status-icon { color: var(--color-success); background: color-mix(in srgb, var(--color-success) 10%, var(--bg-base)); }
.upd-state-error .upd-status-icon { color: var(--color-error); background: color-mix(in srgb, var(--color-error) 10%, var(--bg-base)); }
.upd-title { font-size: 15px; line-height: 1.5; color: var(--text-primary); margin: 0 0 3px; font-weight: 650; }
.upd-meta { font-size: 12px; color: var(--text-secondary); margin: 0; line-height: 1.7; }
.upd-current { margin-top: 2px; }
.upd-version, .upd-download-meta { display: flex; align-items: center; justify-content: space-between; gap: 12px; font-size: 12px; color: var(--text-secondary); font-variant-numeric: tabular-nums; }
.upd-version { padding: 9px 12px; background: var(--bg-subtle); border-radius: var(--radius-sm); }
.upd-version strong { margin-left: 5px; color: var(--text-primary); font-weight: 600; }
.upd-download { display: grid; gap: 8px; }
.upd-download-meta strong { font-weight: 600; color: var(--text-primary); }
.upd-err { font-size: 12px; color: var(--color-error); padding: 10px 12px; border-radius: var(--radius-sm); background: color-mix(in srgb, var(--color-error) 7%, var(--bg-base)); line-height: 1.7; overflow-wrap: anywhere; }
.upd-progress { width: 100%; height: 5px; background: var(--border-light); border-radius: var(--radius-full); overflow: hidden; }
.upd-bar { height: 100%; background: var(--color-primary); transition: width var(--motion-normal) var(--motion-ease); }
.upd-bar.indeterminate { animation: update-progress 1.4s ease-in-out infinite alternate; }
@keyframes update-progress { to { transform: translateX(185%); } }
.upd-actions { display: flex; flex-wrap: wrap; gap: 8px; justify-content: flex-end; }
.upd-actions .btn { min-height: 30px; padding: 5px 12px; font-size: 12px; border-radius: var(--radius-sm); }
.upd-notes { min-width: 0; }
.upd-notes summary { cursor: pointer; font-size: 12px; font-weight: 600; color: var(--text-primary); }
.upd-notes pre { white-space: pre-wrap; overflow-wrap: anywhere; max-height: 180px; overflow-y: auto; margin-top: 8px; padding-right: 4px; font: inherit; font-size: 12px; line-height: 1.8; color: var(--text-secondary); }
.upd-footer { display: flex; align-items: flex-start; flex-wrap: wrap; justify-content: space-between; gap: 10px; color: var(--text-tertiary); font-size: 11px; line-height: 1.7; }
.upd-source { flex: 1 1 180px; }
.upd-source summary { cursor: pointer; }
.upd-source p { margin-top: 5px; overflow-wrap: anywhere; }
.upd-release { display: inline-flex; align-items: center; gap: 4px; flex: none; padding: 0; border: 0; background: transparent; color: var(--text-secondary); font: inherit; cursor: pointer; }
.upd-release:hover { color: var(--text-primary); text-decoration: underline; }
.update-state-enter-active, .update-state-leave-active { transition: opacity var(--motion-fast) var(--motion-ease), transform var(--motion-normal) var(--motion-glide); }
.update-state-enter-from { opacity: 0; transform: translateY(5px); }
.update-state-leave-to { opacity: 0; transform: translateY(-3px); }
@media (prefers-reduced-motion: reduce) {
  .update-state-enter-active, .update-state-leave-active, .upd-bar { transition-duration: 1ms; }
}
</style>
