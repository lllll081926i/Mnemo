<script setup>
// 文件夹同步页：本地文件夹与网盘目录的双向/单向同步任务管理。
import { ref, shallowRef, computed, onMounted, onActivated, onDeactivated, onBeforeUnmount } from 'vue'
import { ListSyncConfigs, SaveSyncConfig, DeleteSyncConfig, PreviewSync, RunSync, RunSyncPlan, CancelSync, ListRunningSyncIDs, onEvent, accountName, providerMetaOf, providerIconUrl, PickDirectory } from '../api'
import Modal from '../components/Modal.vue'
import SegTabs from '../components/SegTabs.vue'
import TaskIssue from '../components/TaskIssue.vue'
import { ListSyncRunStates } from '../api'
import SelectDirModal from '../components/SelectDirModal.vue'
import UiIcon from '../components/UiIcon.vue'
import UiSelect from '../components/UiSelect.vue'
import ConfirmModal from '../components/ConfirmModal.vue'
import SyncPreviewChanges from '../components/SyncPreviewChanges.vue'
import SyncTaskProgress from '../components/SyncTaskProgress.vue'
import { summarizeSync } from '../sync-risk'

const props = defineProps({
  account: { type: Object, default: null },
  accounts: { type: Array, default: () => [] },
  providers: { type: Array, default: () => [] },
})
const emit = defineEmits(['toast', 'repair'])

const confirmDialog = ref(null)
const preview = shallowRef(null), previewJob = ref(null), conflictChoices = ref(Object.create(null))
const previewPhase = ref('idle')
const previewBusy = computed(() => previewPhase.value !== 'idle')
const previewError = ref('')
let previewRequest = 0
const riskSummary = computed(() => summarizeSync(preview.value?.changes))
function chooseConflict(path, strategy) { conflictChoices.value[path] = strategy }
function closePreview() {
  if (previewPhase.value === 'running') return
  ++previewRequest
  previewPhase.value = 'idle'
  previewJob.value = null
  preview.value = null
  previewError.value = ''
  conflictChoices.value = Object.create(null)
}
async function showPreview(job) {
  if (!job || previewBusy.value || running.value.has(job.id)) return
  const request = ++previewRequest
  previewPhase.value = 'loading'
  previewJob.value = job
  preview.value = null
  previewError.value = ''
  conflictChoices.value = Object.create(null)
  try {
    const plan = await PreviewSync(job.id)
    if (request === previewRequest) preview.value = plan
  } catch (error) {
    if (request === previewRequest) previewError.value = `无法加载同步预览：${String(error?.message || error)}`
  } finally {
    if (request === previewRequest) previewPhase.value = 'idle'
  }
}
async function executePreview() {
  if (!preview.value || previewBusy.value || preview.value.blocked) return
  if (riskSummary.value.deletes > 0) {
    const token = preview.value.token
    confirmDialog.value = { title: '确认同步删除', danger: true, message: `即将删除本地 ${riskSummary.value['delete-local']} 项、网盘 ${riskSummary.value['delete-remote']} 项（删除后可能无法找回）。是否继续？`, onOk: async () => {
      confirmDialog.value = null
      if (preview.value?.token !== token) { emit('toast', '同步预览已变化，请重新确认', 'warn'); return }
      await runPreview()
    } }
    return
  }
  return runPreview()
}
async function runPreview() {
  if (!preview.value || !previewJob.value || previewBusy.value || preview.value.blocked) return
  const job = previewJob.value, plan = preview.value
  previewPhase.value = 'running'
  try {
    await RunSyncPlan(job.id, plan.token, { ...conflictChoices.value })
    previewPhase.value = 'idle'
    closePreview()
    emit('toast', '同步完成', 'success')
  } catch (error) {
    preview.value = null
    previewError.value = `同步未完成：${String(error?.message || error)}。请重新生成预览。`
  } finally { previewPhase.value = 'idle' }
}

const jobs = ref([])
const runStates = ref({})
const loadingJobs = ref(false)
const jobsLoaded = ref(false)
const jobsError = ref('')
const progress = ref({}) // id -> { done, total }
const running = ref(new Set())
const toggling = ref(new Set())
let runningSnapshot = null
let runStateUpdates = null
let viewActive = false
let progressFrame = null
const pendingProgress = new Map()

const showEdit = ref(false)
const showDirPick = ref(false)
const editingId = ref('')
let editVersion = 0
const form = ref(emptyForm())

const dirOptions = [
  { key: 'two-way', label: '双向同步' },
  { key: 'push', label: '单向上传 (本地 → 网盘)' },
  { key: 'pull', label: '单向下载 (网盘 → 本地)' },
]

const dirLabel = { 'two-way': '双向', push: '单向上传', pull: '单向下载' }
const dirArrow = { 'two-way': '⇄', push: '→', pull: '←' }

function emptyForm() {
  return { name: '', local_dir: '', user_id: '', drive_id: '', remote_dir: 'root', remote_name: '根目录', direction: 'two-way', intervalMin: 0, deletePropagation: false, conflictPolicy: 'keep-both' }
}

let refreshSeq = 0
async function refresh() {
  if (!viewActive) return
  const seq = ++refreshSeq
  loadingJobs.value = true
  jobsError.value = ''
  await Promise.all([loadJobs(seq), loadRunStates(seq)])
}

async function loadJobs(seq) {
  try {
    const list = await ListSyncConfigs()
    if (seq !== refreshSeq) return
    jobs.value = list || []
    jobsLoaded.value = true
  } catch (error) {
    if (seq === refreshSeq) {
      jobsError.value = String(error)
      emit('toast', jobsError.value, 'error')
    }
  } finally {
    if (seq === refreshSeq) loadingJobs.value = false
  }
}

async function loadRunStates(seq) {
  const updates = new Map()
  runStateUpdates = updates
  try {
    const states = await ListSyncRunStates()
    if (seq === refreshSeq) runStates.value = { ...states, ...Object.fromEntries(updates) }
  } catch {
    if (seq === refreshSeq) emit('toast', '同步运行记录读取失败，任务配置仍可使用', 'error')
  } finally {
    if (runStateUpdates === updates) runStateUpdates = null
  }
}

function accountOf(job) {
  return props.accounts.find((a) => a.user_id === job.user_id) || null
}

const formAccount = computed(() => props.accounts.find((a) => a.user_id === form.value.user_id) || null)

function openCreate() {
  ++editVersion
  showDirPick.value = false
  editingId.value = ''
  form.value = emptyForm()
  if (props.account) {
    form.value.user_id = props.account.user_id
    form.value.drive_id = props.account.drive_id
  } else if (props.accounts.length) {
    form.value.user_id = props.accounts[0].user_id
    form.value.drive_id = props.accounts[0].drive_id
  }
  showEdit.value = true
}

function openEdit(job) {
  ++editVersion
  showDirPick.value = false
  editingId.value = job.id
  form.value = {
    name: job.name || '',
    local_dir: job.local_dir || '',
    user_id: job.user_id || '',
    drive_id: job.drive_id || '',
    remote_dir: job.remote_dir || 'root',
    remote_name: job.remote_name || (job.remote_dir && job.remote_dir !== 'root' ? job.remote_dir : '根目录'),
    direction: job.direction || 'two-way',
    intervalMin: Number(job.intervalMin) || 0,
    deletePropagation: !!job.deletePropagation,
    conflictPolicy: job.conflictPolicy || 'keep-both',
  }
  showEdit.value = true
}

function onFormAccount() {
  showDirPick.value = false
  const acc = formAccount.value
  if (acc) {
    form.value.drive_id = acc.drive_id
    form.value.remote_dir = 'root'
    form.value.remote_name = '根目录'
  }
}

const pickingLocalDir = ref(false)
// 选择本地同步文件夹（原生目录选择对话框）
async function pickLocalDir() {
  if (pickingLocalDir.value) return
  const version = editVersion
  const currentForm = form.value
  pickingLocalDir.value = true
  try {
    const dir = await PickDirectory('选择本地文件夹', currentForm.local_dir || '')
    if (dir && showEdit.value && version === editVersion && form.value === currentForm) currentForm.local_dir = dir
  } catch {
    if (showEdit.value && version === editVersion) emit('toast', '无法打开本地目录选择器，请重试', 'error')
  } finally { pickingLocalDir.value = false }
}

function closeEdit() {
  ++editVersion
  showEdit.value = false
  showDirPick.value = false
}

const savingEdit = ref(false)
async function save() {
  if (savingEdit.value) return
  if (!form.value.name.trim()) { emit('toast', '请填写任务名称', 'error'); return }
  if (!form.value.local_dir.trim()) { emit('toast', '请选择本地文件夹', 'error'); return }
  if (!formAccount.value) { emit('toast', '请选择关联的网盘账号', 'error'); return }
  savingEdit.value = true
  const version = editVersion
  const id = editingId.value || 'sync-' + Date.now() + '-' + Math.random().toString(36).slice(2, 8)
  try {
    await SaveSyncConfig({
      id,
      name: form.value.name.trim(),
      user_id: form.value.user_id,
      drive_id: form.value.drive_id,
      local_dir: form.value.local_dir.trim(),
      remote_dir: form.value.remote_dir || 'root',
      remote_name: form.value.remote_name || '',
       direction: form.value.direction,
       enabled: editingId.value ? (jobs.value.find((j) => j.id === editingId.value) || {}).enabled !== false : true,
       intervalMin: Math.max(0, Math.floor(Number(form.value.intervalMin) || 0)),
       deletePropagation: !!form.value.deletePropagation,
       conflictPolicy: form.value.conflictPolicy,
    })
    if (version === editVersion) closeEdit()
    refresh()
    emit('toast', '已保存同步任务', 'success')
  } catch (e) { emit('toast', String(e), 'error') }
  finally { savingEdit.value = false }
}

async function toggle(job) {
  if (running.value.has(job.id) || toggling.value.has(job.id)) return
  toggling.value = new Set([...toggling.value, job.id])
  const next = { ...job, enabled: !job.enabled }
  try {
    await SaveSyncConfig(next)
    job.enabled = next.enabled
  } catch (e) { emit('toast', String(e), 'error') }
  finally {
    const pending = new Set(toggling.value); pending.delete(job.id); toggling.value = pending
  }
}

async function run(job) {
  if (running.value.has(job.id)) return
  recordRunning(job.id, true)
  emit('toast', `开始同步「${job.name}」…`, 'success')
  try {
    await RunSync(job.id)
    emit('toast', `「${job.name}」同步完成`, 'success')
  } catch (e) {
    const message = String(e)
    if (/cancel|取消|停止/i.test(message)) emit('toast', `「${job.name}」已停止`, 'info')
    else emit('toast', message, 'error')
  }
  recordRunning(job.id, false)
}

async function cancel(job) {
  try {
    const accepted = await CancelSync(job.id)
    if (accepted) emit('toast', `正在停止「${job.name}」…`, 'info')
    else emit('toast', '任务已结束', 'info')
  } catch (e) {
    emit('toast', String(e), 'error')
  }
}

function remove(job) {
  if (running.value.has(job.id) || toggling.value.has(job.id)) {
    emit('toast', '任务正在同步或更新中，请稍后再删除', 'warn')
    return
  }
  confirmDialog.value = { message: `确定删除同步任务「${job.name}」？此操作不影响本地及网盘文件。`, onOk: async () => {
    confirmDialog.value = null
    if (running.value.has(job.id) || toggling.value.has(job.id)) {
      emit('toast', '任务正在同步或更新中，请稍后再删除', 'warn')
      return
    }
    try {
      await DeleteSyncConfig(job.id)
      refresh()
      emit('toast', '已删除同步任务', 'success')
    } catch (e) { emit('toast', String(e), 'error') }
  }, danger: true, title: '删除同步任务' }
}

const offs = []
function recordRunning(id, isRunning) {
  if (!viewActive) return
  runningSnapshot?.set(id, isRunning)
  if (isRunning) running.value.add(id)
  else {
    running.value.delete(id)
    pendingProgress.delete(id)
    delete progress.value[id]
  }
}

function queueProgress(event) {
  if (!viewActive || !event?.id) return
  pendingProgress.set(event.id, { done: event.done || 0, total: event.total || 0 })
  if (progressFrame !== null) return
  progressFrame = requestAnimationFrame(() => {
    progressFrame = null
    for (const [id, next] of pendingProgress) progress.value[id] = next
    pendingProgress.clear()
  })
}

function activateView() {
  if (viewActive) return
  viewActive = true
  const updates = new Map()
  runningSnapshot = updates
  offs.push(onEvent('sync:progress', queueProgress))
  offs.push(onEvent('account:changed', refresh))
  offs.push(onEvent('sync:state', (ev) => {
    if (!viewActive || !ev?.id) return
    recordRunning(ev.id, ev.running)
    const state = { status: ev.running ? 'running' : ev.status, issue: ev.issue }
    runStateUpdates?.set(ev.id, state)
    runStates.value[ev.id] = state
  }))
  refresh()
  ListRunningSyncIDs().then((ids) => {
    if (runningSnapshot !== updates) return
    const current = new Set(ids || [])
    for (const [id, isRunning] of updates) {
      if (isRunning) current.add(id)
      else current.delete(id)
    }
    running.value = current
  }).catch(() => {}).finally(() => {
    if (runningSnapshot === updates) runningSnapshot = null
  })
}

function deactivateView() {
  if (!viewActive) return
  viewActive = false
  if (previewPhase.value === 'loading') closePreview()
  runningSnapshot = null
  runStateUpdates = null
  ++refreshSeq
  loadingJobs.value = false
  offs.splice(0).forEach((off) => off && off())
  if (progressFrame !== null) cancelAnimationFrame(progressFrame)
  progressFrame = null
  pendingProgress.clear()
  progress.value = {}
}

onMounted(activateView)
onActivated(activateView)
onDeactivated(deactivateView)
onBeforeUnmount(deactivateView)
</script>

<template>
  <div class="syncpage">
    <Modal v-if="previewJob" title="同步预览" width="760px" @close="closePreview">
      <p>{{ previewJob.name }}<template v-if="preview"> · {{ preview.changes?.length || 0 }} 项变化</template></p>
      <p class="hint">本地：{{ previewJob.local_dir }} → 网盘：{{ previewJob.remote_name || previewJob.remote_dir }}</p>
      <p v-if="previewPhase === 'loading'" role="status">正在比较本地与网盘文件…</p>
      <p v-if="previewPhase === 'running'" role="status">正在执行同步计划…</p>
      <p v-if="previewError" role="alert">{{ previewError }}</p>
      <template v-if="preview">
        <p class="hint">上传 {{ riskSummary.upload }} · 下载 {{ riskSummary.download }} · 冲突 {{ riskSummary.conflict }} · 删除本地 {{ riskSummary['delete-local'] }} · 删除网盘 {{ riskSummary['delete-remote'] }}</p>
        <p v-if="riskSummary.deletes" role="alert">本次同步包含删除操作。网盘或本地删除可能无法恢复，请确认。</p><p v-else class="hint">本次计划无删除操作；冲突文件按预设策略处理。</p>
        <p v-if="preview.blocked" role="alert">{{ preview.blocked }}</p>
      </template>
      <SyncPreviewChanges v-if="preview" :key="preview.token" :changes="preview.changes" :choices="conflictChoices" :policy="previewJob.conflictPolicy" :busy="previewBusy" @choose="chooseConflict" />
      <p class="hint">执行前会重新比对文件状态。为保障数据安全，若单次删除文件超过 50% 将自动终止同步。</p>
      <template #actions><button class="btn" :aria-disabled="previewBusy" @click="showPreview(previewJob)">{{ previewError ? '重试预览' : '刷新' }}</button><button class="btn primary" :disabled="previewBusy || !preview || !!preview.blocked" @click="executePreview">执行</button></template>
    </Modal>
    <header class="syncpage-head">
      <div class="syncpage-title">
        <UiIcon name="refresh" :size="18" />
        <strong>文件夹同步</strong>
        <span class="syncpage-sub">自动同步本地与网盘文件</span>
      </div>
      <button class="btn primary sm" :disabled="!accounts.length" @click="openCreate">
        <UiIcon name="plus" :size="13" /> 新建同步任务
      </button>
    </header>

    <div v-if="loadingJobs && !jobsLoaded" class="workspace-empty-state" style="flex:1" role="status">
      <span class="wes-title">正在加载同步任务…</span>
    </div>
    <div v-else-if="jobsError && !jobs.length" class="workspace-empty-state" style="flex:1" role="alert">
      <span class="wes-title">同步任务加载失败</span>
      <span class="wes-desc">{{ jobsError }}</span>
      <button class="btn" @click="refresh">重试</button>
    </div>
    <div v-else-if="!jobs.length" class="workspace-empty-state" style="flex:1">
      <UiIcon name="refresh" :size="40" />
      <span class="wes-title">还没有同步任务</span>
      <span class="wes-desc">新建任务后，即可在本地文件夹与网盘之间自动保持文件同步</span>
    </div>

    <div v-else class="syncpage-list content-reveal">
      <div v-for="job in jobs" :key="job.id" class="sync-task" :class="{ disabled: !job.enabled }">
        <div class="sync-task-main">
          <div class="sync-task-name">
            <strong>{{ job.name }}</strong>
            <span class="sync-task-direction">{{ dirLabel[job.direction] || job.direction }}</span>
            <span v-if="!accountOf(job)" class="sync-task-warn">账号未登录或已失效</span>
          </div>
          <div class="sync-task-paths">
            <span class="sync-path" :title="job.local_dir">本地：{{ job.local_dir }}</span>
            <span style="color:var(--text-tertiary);flex-shrink:0">{{ dirArrow[job.direction] || '⇄' }}</span>
            <span class="sync-path account-inline" :title="job.remote_name || job.remote_dir"><img v-if="accountOf(job)" :src="providerIconUrl(providerMetaOf(accountOf(job), providers))" alt="" />{{ accountOf(job) ? accountName(accountOf(job)) + ' / ' : '' }}{{ job.remote_name || (job.remote_dir === 'root' ? '根目录' : job.remote_dir) }}</span>
          </div>
          <SyncTaskProgress v-if="running.has(job.id)" :task-id="job.id" :name="job.name" :progress-by-id="progress" />
          <TaskIssue v-if="!running.has(job.id)" :issue="runStates[job.id]?.issue" action="同步文件" :user-id="job.user_id" write-operation />
        </div>
        <div class="sync-task-actions">
          <button type="button" class="switch" role="switch" :aria-label="'自动同步：' + job.name" :aria-checked="!!job.enabled" :class="{ on: job.enabled }" :disabled="running.has(job.id) || toggling.has(job.id)" :title="toggling.has(job.id) ? '正在保存…' : job.enabled ? '停用自动同步' : '启用自动同步'" @click="toggle(job)"></button>
          <button class="btn-circle" :disabled="toggling.has(job.id) || (!running.has(job.id) && !accountOf(job))" :title="running.has(job.id) ? '停止同步' : '预览同步'" @click="running.has(job.id) ? cancel(job) : showPreview(job)">
            <UiIcon :name="running.has(job.id) ? 'close' : 'play'" :size="14" />
          </button>
          <button class="btn-circle" title="编辑" :disabled="toggling.has(job.id)" @click="openEdit(job)"><UiIcon name="pencil" :size="14" /></button>
          <button class="btn-circle danger" title="删除" :disabled="toggling.has(job.id)" @click="remove(job)"><UiIcon name="trash" :size="14" /></button>
        </div>
      </div>
    </div>

    <Modal v-if="showEdit" :title="editingId ? '编辑同步任务' : '新建同步任务'" width="480px" @close="closeEdit">
      <div class="field">
        <label>任务名称</label>
        <input class="input" style="width:100%" v-model="form.name" placeholder="如：工作文档同步" />
      </div>
      <div class="field">
        <label>本地文件夹</label>
        <div style="display:flex;gap:8px">
          <input class="input" style="flex:1" v-model="form.local_dir" placeholder="选择本地文件夹" />
          <button class="btn sm" :aria-busy="pickingLocalDir" :aria-disabled="pickingLocalDir" @click="pickLocalDir">{{ pickingLocalDir ? '选择中…' : '选择目录' }}</button>
        </div>
      </div>
      <div class="field">
        <label>绑定账号</label>
        <UiSelect
          v-model="form.user_id"
          block
          placeholder="请选择账号"
          :options="accounts.map((a) => ({ value: a.user_id, label: `${providerMetaOf(a, providers).label} · ${accountName(a)}`, img: providerIconUrl(providerMetaOf(a, providers)) }))"
          @change="onFormAccount"
        />
      </div>
      <div class="field">
        <label>网盘目录</label>
        <div style="display:flex;gap:8px">
          <input class="input" style="flex:1" :value="form.remote_name" readonly placeholder="根目录" />
          <button class="btn sm" :disabled="!formAccount" @click="showDirPick = true">选择目录</button>
        </div>
      </div>
      <div class="field">
        <label>同步方向</label>
        <SegTabs v-model="form.direction" :options="dirOptions" />
      </div>
      <div class="field">
        <label>自动同步间隔（分钟，填 0 则仅手动同步）</label>
        <input class="input" type="number" min="0" step="1" v-model.number="form.intervalMin" placeholder="0" />
      </div>
      <div class="field"><label>文件冲突策略</label><UiSelect v-model="form.conflictPolicy" :options="[{value:'keep-both',label:'保留两份'},{value:'local',label:'使用本地文件'},{value:'remote',label:'使用网盘文件'}]" /><span class="hint">自动同步时生效；手动预览时也可单独调整</span></div>
      <div class="field switch-row">
        <label>同步删除</label>
        <button type="button" class="switch" :class="{ on: form.deletePropagation }" role="switch" aria-label="同步删除" :aria-checked="form.deletePropagation" @click="form.deletePropagation = !form.deletePropagation"></button>
        <span class="hint">在一侧删除文件时，另一侧也同步删除</span>
      </div>
      <template #actions>
        <button class="btn" :disabled="savingEdit" @click="closeEdit">取消</button>
        <button class="btn primary" :disabled="savingEdit" @click="save">
          <span v-if="savingEdit" class="spin spin-on-primary"></span>
          <span>{{ savingEdit ? '保存中…' : '保存' }}</span>
        </button>
      </template>
    </Modal>

    <SelectDirModal
      v-if="showDirPick && formAccount"
      title="选择网盘目录"
      :account="formAccount"
      :providers="providers"
      @close="showDirPick = false"
      @select="(d) => { if (showEdit && showDirPick) { form.remote_dir = d.id; form.remote_name = d.name; showDirPick = false } }"
      @toast="(msg, type) => emit('toast', msg, type)"
    />

    <ConfirmModal
      v-if="confirmDialog"
      :title="confirmDialog.title"
      :message="confirmDialog.message"
      :danger="confirmDialog.danger"
      @ok="confirmDialog.onOk(); confirmDialog = null"
      @cancel="confirmDialog = null"
    />
  </div>
</template>

<style scoped>
.sync-task-actions .switch:disabled { opacity: .55; cursor: not-allowed; transform: none; }
</style>
