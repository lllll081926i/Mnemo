<script setup>
import { ref, computed, nextTick, watch, onMounted, onBeforeUnmount } from 'vue'
import PanView from './PanView.vue'
import UiSelect from '../components/UiSelect.vue'
import UiIcon from '../components/UiIcon.vue'
import Modal from '../components/Modal.vue'
import { accountName, providerMetaOf, providerIconUrl, onEvent } from '../api'
import { transferFiles, transferProblem } from '../file-transfer'
const props = defineProps({ account: Object, accounts: Array, providers: Array, dual: Boolean })
const emit = defineEmits(['toast', 'go'])
const dual = computed(() => props.dual)
const rightId = ref(''), focused = ref('left'), left = ref(null), right = ref(null), busy = ref(false)
const cloudDrag = ref(null), dragOver = ref('')
const pinnedTarget = ref(null), transferPlan = ref(null), lastTarget = ref(null)
function endDrag() { cloudDrag.value = null; dragOver.value = '' }
watch(dual, () => { focused.value = 'left'; transferPlan.value = null; endDrag() })
const rightAccount = computed(() => props.accounts.find(a => a.user_id === rightId.value) || null)
const options = computed(() => props.accounts.map(a => ({ value: a.user_id, label: accountName(a), img: providerIconUrl(providerMetaOf(a, props.providers)) })))
watch(() => props.accounts, list => { if (!list.some(a => a.user_id === rightId.value)) rightId.value = list.find(a => a.user_id !== props.account?.user_id)?.user_id || list[0]?.user_id || '' }, { immediate: true })
watch(() => [props.account?.user_id, props.account?.drive_id, rightAccount.value?.user_id, rightAccount.value?.drive_id], () => {
  endDrag()
  transferPlan.value = null
  if (pinnedTarget.value && (pinnedTarget.value.account.user_id !== rightAccount.value?.user_id || pinnedTarget.value.account.drive_id !== rightAccount.value?.drive_id)) pinnedTarget.value = null
})
function pane(side) { return (side === 'left' ? left : right).value }
function captureLocation(snapshot) {
  return { ...snapshot, account: { ...snapshot.account }, path: (snapshot.path || []).map(part => ({ ...part })), files: (snapshot.files || []).map(file => ({ ...file })) }
}
function locationLabel(location) {
  return `${accountName(location.account)} · /${(location.path || []).map(part => part.name).filter(Boolean).join('/')}`
}
function togglePinnedTarget() {
  if (pinnedTarget.value) { pinnedTarget.value = null; return }
  const target = right.value?.snapshot()
  if (!target?.account || target.mode !== 'list') { emit('toast', '请先在右栏打开目标目录', 'warn'); return }
  pinnedTarget.value = captureLocation(target)
}
async function showTarget(target) {
  if (!props.accounts.some(account => account.user_id === target.account.user_id && account.drive_id === target.account.drive_id)) { emit('toast', '目标账号已变更，请重新选择', 'warn'); return }
  rightId.value = target.account.user_id
  await nextTick()
  await right.value?.navigate({ dirId: target.dirId, name: target.path?.at(-1)?.name || '目标目录' })
  focused.value = 'right'
}
function startDrag(side, source, event) {
  if (!dual.value || busy.value || !source.files.length || !event.dataTransfer) { event.preventDefault(); return }
  cloudDrag.value = { side, source }
  event.dataTransfer.effectAllowed = 'move'
  event.dataTransfer.setData('application/x-mnemo-cloud-files', 'internal')
}
function overPane(side, event) {
  if (!cloudDrag.value) return
  event.preventDefault()
  const allowed = !busy.value && side !== cloudDrag.value.side && pane(side)?.snapshot()?.mode === 'list'
  dragOver.value = allowed ? side : ''
  if (event.dataTransfer) event.dataTransfer.dropEffect = allowed ? 'move' : 'none'
}
function leavePane(event) {
  if (!event.currentTarget.contains(event.relatedTarget)) dragOver.value = ''
}
function dropOnPane(side, folder) {
  const drag = cloudDrag.value
  endDrag()
  if (!drag || drag.side === side) return
  return transfer(side === 'right' ? 'right' : 'left', drag.source, folder, true)
}
async function refreshAccounts(source, target) {
  const sameAccount = source.account.user_id === target.account.user_id && source.account.drive_id === target.account.drive_id
  await Promise.all([left.value, right.value].filter(Boolean).flatMap(view => [
    view.invalidateDirectories(source.account.user_id, source.account.drive_id, [source.dirId, ...(sameAccount ? [target.dirId] : [])]),
    ...(sameAccount
      ? [] : [view.invalidateDirectories(target.account.user_id, target.account.drive_id, [target.dirId])]),
  ]))
}
async function transfer(direction, dragged = null, folder = null, moving = false) {
  if (busy.value) return
  const src = dragged || (direction === 'right' ? left : right).value?.snapshot()
  const destination = (direction === 'right' ? right : left).value?.snapshot()
  const dst = direction === 'right' && pinnedTarget.value ? pinnedTarget.value : destination && { ...destination, dirId: folder?.file_id || destination.dirId, path: folder ? [...destination.path, { id: folder.file_id, name: folder.name }] : destination.path }
  if (!src?.account || !dst?.account || !src.files.length || dst.mode !== 'list') { emit('toast', '请选择要操作的文件，并在另一栏打开目标目录', 'warn'); return }
  const problem = transferProblem(src, dst, props.providers, moving)
  if (problem) { emit('toast', problem, 'warn'); return }
  transferPlan.value = { source: captureLocation(src), target: captureLocation(dst), moving }
}
async function confirmTransfer() {
  const plan = transferPlan.value
  if (!plan || busy.value) return
  transferPlan.value = null
  const { source: src, target: dst, moving } = plan
  if (![src, dst].every(location => props.accounts.some(account => account.user_id === location.account.user_id && account.drive_id === location.account.drive_id))) { emit('toast', '操作账号已变更，请重新选择', 'warn'); return }
  busy.value = true
  try {
    const result = await transferFiles(src, dst, props.providers, moving)
    lastTarget.value = dst
    if (!result.queued) {
      await refreshAccounts(src, dst)
      if (result.completed.length !== src.files.length) throw new Error('部分项目未完成，请检查目录后重试')
      emit('toast', moving ? '已移动到目标目录' : '已复制到目标目录', 'success')
    } else {
      emit('toast', moving ? '已提交跨盘移动，可在传输页查看进度' : '已提交跨盘复制，可在传输页查看进度', 'success')
    }
  } catch(e) { emit('toast', String(e), 'error') }
  finally { busy.value = false }
}
let stopMigration
onMounted(() => {
  stopMigration = onEvent('migrate:progress', job => {
    if (!job || !['completed', 'partial', 'failed', 'canceled'].includes(job.status)) return
    refreshAccounts(
      { account: { user_id: job.srcUser, drive_id: job.srcDrive } },
      { account: { user_id: job.dstUser, drive_id: job.dstDrive }, dirId: job.dstParent },
    ).catch(error => emit('toast', String(error), 'error'))
  })
})
onBeforeUnmount(() => { stopMigration?.(); endDrag() })
function active() { return dual.value && focused.value === 'right' ? right.value : left.value }
defineExpose({
  selectAccount: reload => { focused.value = 'left'; if (reload) return left.value?.refresh() },
  navigateHistory: direction => active()?.navigateHistory(direction), refresh: () => active()?.refresh(),
  openMkdirModal: () => active()?.openMkdirModal(), openUploadModal: () => active()?.openUploadModal(),
  clearCache: () => { left.value?.clearCache(); right.value?.clearCache() },
  navigate: async location => { focused.value = 'left'; await nextTick(); await left.value?.navigate(location) },
})
</script>
<template>
  <div class="workspace-view">
    <div v-if="dual" class="workspace-controls">
      <button class="btn" :disabled="busy" @click="transfer('right')"><UiIcon name="copy" :size="14" />复制到右栏</button>
      <button class="btn" :disabled="busy" @click="transfer('left')"><UiIcon name="copy" :size="14" />复制到左栏</button>
      <UiSelect v-model="rightId" :options="options" placeholder="右侧账号" />
      <button class="btn" :aria-pressed="!!pinnedTarget" :disabled="busy" @click="togglePinnedTarget">{{ pinnedTarget ? '解除固定目标' : '固定右栏目标' }}</button>
      <button v-if="lastTarget" class="btn" @click="showTarget(lastTarget)">查看目标</button>
    </div>
    <div v-if="dual && pinnedTarget" class="pinned-target" role="status">固定目标：{{ locationLabel(pinnedTarget) }}。右栏导航不会改变此目标。<button class="btn sm" @click="showTarget(pinnedTarget)">定位</button></div>
    <Modal v-if="transferPlan" :title="transferPlan.moving ? '确认移动' : '确认复制'" width="520px" @close="transferPlan = null">
      <dl class="transfer-summary"><dt>来源</dt><dd>{{ locationLabel(transferPlan.source) }}</dd><dt>目标</dt><dd>{{ locationLabel(transferPlan.target) }}</dd><dt>项目</dt><dd>{{ transferPlan.source.files.length }} 项 · {{ transferPlan.source.files.slice(0, 3).map(file => file.name).join('、') }}{{ transferPlan.source.files.length > 3 ? '…' : '' }}</dd></dl>
      <p>{{ transferPlan.moving ? '移动会移除源文件；跨盘移动仅在目标确认成功后移除来源。此操作没有通用撤销功能。' : '复制会保留源文件。跨盘任务将在传输页执行。' }}</p>
      <template #actions><button class="btn" @click="transferPlan = null">取消</button><button class="btn primary" :class="{ danger: transferPlan.moving }" @click="confirmTransfer">{{ transferPlan.moving ? '确认移动' : '确认复制' }}</button></template>
    </Modal>
    <div class="workspace-panes" :class="{dual}">
      <div class="workspace-pane" :class="{focused:focused === 'left', 'cloud-drop-active': dragOver === 'left'}" @pointerdown.capture="focused = 'left'" @focusin="focused = 'left'" @dragover="overPane('left', $event)" @dragleave="leavePane"><PanView ref="left" :account="account" :accounts="accounts" :providers="providers" :keyboard-active="!dual || focused === 'left'" :cloud-drag-enabled="dual && !busy" :cloud-drag-active="!!cloudDrag" @cloud-drag-start="(source, event) => startDrag('left', source, event)" @cloud-drag-end="endDrag" @cloud-drop="folder => dropOnPane('left', folder)" @toast="(...args) => emit('toast', ...args)" @go="emit('go', $event)" /><div v-if="dragOver === 'left'" class="cloud-drop-hint">松开以移动</div></div>
      <div v-if="dual" class="workspace-pane workspace-pane-enter" :class="{focused:focused === 'right', 'cloud-drop-active': dragOver === 'right'}" @pointerdown.capture="focused = 'right'" @focusin="focused = 'right'" @dragover="overPane('right', $event)" @dragleave="leavePane"><PanView ref="right" :account="rightAccount" :accounts="accounts" :providers="providers" :keyboard-active="focused === 'right'" location-key="right" :cloud-drag-enabled="dual && !busy" :cloud-drag-active="!!cloudDrag" @cloud-drag-start="(source, event) => startDrag('right', source, event)" @cloud-drag-end="endDrag" @cloud-drop="folder => dropOnPane('right', folder)" @toast="(...args) => emit('toast', ...args)" @go="emit('go', $event)" /><div v-if="dragOver === 'right'" class="cloud-drop-hint">松开以移动</div></div>
    </div>
  </div>
</template>

<style scoped>
.workspace-controls { gap: 8px; }
.pinned-target { display: flex; align-items: center; gap: 8px; padding: 6px 12px; color: var(--text-secondary); font-size: 12px; overflow-wrap: anywhere; }
.transfer-summary { display: grid; grid-template-columns: auto minmax(0, 1fr); gap: 8px 16px; margin-bottom: 16px; }
.transfer-summary dd { margin: 0; overflow-wrap: anywhere; }
.workspace-controls .btn { display: inline-flex; align-items: center; gap: 6px; }
.workspace-pane { position: relative; min-width: 0; transition: box-shadow var(--motion-normal) var(--motion-ease), background-color var(--motion-fast) var(--motion-ease); }
.workspace-pane-enter { animation: workspace-pane-enter var(--motion-normal) var(--motion-ease); }
@keyframes workspace-pane-enter { from { opacity: 0; transform: translateX(6px); } }
.cloud-drop-active { outline: 2px dashed var(--color-primary); outline-offset: -4px; box-shadow: inset 0 0 0 4px color-mix(in srgb, var(--color-primary) 9%, transparent); }
.cloud-drop-hint { position: absolute; bottom: 16px; left: 50%; translate: -50% 0; padding: 8px 14px; border-radius: var(--radius-full); background: var(--bg-elevated); color: var(--color-primary); box-shadow: var(--shadow-modal); font-size: 12px; text-align: center; pointer-events: none; z-index: 10; animation: workspace-drop-hint var(--motion-normal) var(--motion-spring); }
@keyframes workspace-drop-hint { from { opacity: 0; transform: translateY(5px) scale(.96); } }
@media (prefers-reduced-motion: reduce) { .workspace-pane-enter { animation: none; } }
</style>
