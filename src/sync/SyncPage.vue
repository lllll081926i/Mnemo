<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { Modal } from '@arco-design/web-vue'
import { ArrowLeftRight, FolderSync, Pencil, Play, Plus, ScrollText, Trash2 } from 'lucide-vue-next'
import useSyncStore from './syncstore'
import { persistTasks, refreshSyncTasks, runSyncTask } from './syncengine'
import { deleteSyncTaskData, validateSyncTaskOverlap } from './syncdal'
import { SYNC_DIRECTION_LABEL, SyncDirection, SyncTask, defaultSyncTask, newSyncTaskId } from './syncmodel'
import { modalSelectPanDir } from '../utils/modal'
import { resolveDriveProvider } from '../utils/driveProvider'
import { humanSize } from '../utils/format'
import message from '../utils/message'
import usePanTreeStore from '../pan/pantreestore'

const syncStore = useSyncStore()
const pantreeStore = usePanTreeStore()

const SUPPORTED_PROVIDERS = new Set(['pikpak', 'webdav', 's3'])

const editVisible = ref(false)
const editTask = ref<SyncTask | null>(null)
const logVisible = ref(false)
const logTaskId = ref('')
const logTaskName = computed(() => syncStore.tasks.find((item) => item.id === logTaskId.value)?.name || '')

const remoteSummary = (task: SyncTask) => task.remote_dir_name || task.remote_dir_id
const lastSyncText = (task: SyncTask) => {
  if (!task.last_sync_at) return '从未同步'
  const time = new Date(task.last_sync_at).toLocaleString()
  if (task.last_sync_status === 'ok') return `${time} · 成功`
  if (task.last_sync_status === 'partial') return `${time} · 部分失败`
  return `${time} · 失败`
}
const lastSyncClass = (task: SyncTask) => (task.last_sync_status === 'ok' ? 'ok' : task.last_sync_status ? 'err' : '')

const scheduleText = (task: SyncTask) => {
  const parts: string[] = []
  if (task.interval_min > 0) parts.push(`每 ${task.interval_min} 分钟`)
  if (task.sync_on_launch) parts.push('启动时')
  return parts.length ? parts.join(' + ') : '仅手动'
}

const providerOfTask = (task: SyncTask) => resolveDriveProvider({ userId: task.user_id, driveId: task.drive_id })
const isTaskUsable = (task: SyncTask) => task.user_id === pantreeStore.user_id && SUPPORTED_PROVIDERS.has(providerOfTask(task))

const openCreate = () => {
  if (!pantreeStore.user_id) {
    message.error('请先登录网盘账号')
    return
  }
  const provider = resolveDriveProvider({ userId: pantreeStore.user_id, driveId: pantreeStore.drive_id })
  if (!SUPPORTED_PROVIDERS.has(provider)) {
    message.error('同步功能目前支持 PikPak / WebDAV / S3，请先切换到这些网盘')
    return
  }
  editTask.value = {
    id: newSyncTaskId(),
    name: '',
    user_id: pantreeStore.user_id,
    drive_id: pantreeStore.drive_id,
    remote_dir_id: '',
    remote_dir_name: '',
    local_path: '',
    ...defaultSyncTask()
  } as SyncTask
  editVisible.value = true
}

const openEdit = (task: SyncTask) => {
  editTask.value = JSON.parse(JSON.stringify(task))
  editVisible.value = true
}

const pickLocalFolder = () => {
  if (!editTask.value) return
  window.WebShowOpenDialogSync?.({ title: '选择本地同步文件夹', buttonLabel: '选择', properties: ['openDirectory', 'createDirectory'], defaultPath: editTask.value.local_path || undefined }, (result: string[] | undefined) => {
    if (result && result[0] && editTask.value) editTask.value.local_path = result[0]
  })
}

const pickRemoteFolder = () => {
  if (!editTask.value) return
  modalSelectPanDir('sync', editTask.value.remote_dir_id || 'root', (user_id: string, drive_id: string, selectFile: any) => {
    if (!editTask.value || !selectFile) return
    editTask.value.user_id = user_id
    editTask.value.drive_id = drive_id
    editTask.value.remote_dir_id = selectFile.file_id
    editTask.value.remote_dir_name = selectFile.name || selectFile.file_id
  })
}

const saveTask = async () => {
  const task = editTask.value
  if (!task) return false
  if (!task.name.trim()) {
    message.error('请填写任务名称')
    return false
  }
  if (!task.local_path) {
    message.error('请选择本地同步文件夹')
    return false
  }
  if (!task.remote_dir_id) {
    message.error('请选择网盘同步文件夹')
    return false
  }
  const overlapError = validateSyncTaskOverlap(syncStore.tasks, task)
  if (overlapError) {
    message.error(overlapError)
    return false
  }
  const tasks = syncStore.tasks.filter((item) => item.id !== task.id)
  tasks.push(JSON.parse(JSON.stringify(task)))
  syncStore.mSaveTasks(tasks)
  await persistTasks()
  message.success('同步任务已保存')
  return true
}

const handleSave = async () => {
  if (await saveTask()) editVisible.value = false
}

const handleDelete = (task: SyncTask) => {
  Modal.confirm({
    title: '删除同步任务',
    content: `确定删除任务“${task.name}”吗？只删除任务配置，不会删除任何文件。`,
    okText: '删除',
    cancelText: '取消',
    okButtonProps: { status: 'danger' },
    onOk: async () => {
      const tasks = syncStore.tasks.filter((item) => item.id !== task.id)
      syncStore.mSaveTasks(tasks)
      await persistTasks()
      await deleteSyncTaskData(task.id)
      message.success('任务已删除')
    }
  })
}

const handleToggleEnabled = async (task: SyncTask, enabled: boolean) => {
  syncStore.mUpdateTask(task.id, { enabled })
  await persistTasks()
}

const handleRunNow = (task: SyncTask) => {
  if (!isTaskUsable(task)) {
    message.error('该任务绑定的网盘账号未登录或不支持同步')
    return
  }
  void runSyncTask(task, true)
}

const openLogs = (task: SyncTask) => {
  logTaskId.value = task.id
  logVisible.value = true
}

const logLevelClass = (level: string) => `sync-log-${level}`
const formatLogTime = (time: number) => new Date(time).toLocaleTimeString()

onMounted(() => {
  void refreshSyncTasks()
})
</script>

<template>
  <div class="syncpage">
    <header class="syncpage-head">
      <div class="syncpage-title">
        <FolderSync :size="18" />
        <strong>文件夹同步</strong>
        <span class="syncpage-sub">本地文件夹与网盘文件夹保持同步（PikPak / WebDAV / S3）</span>
      </div>
      <a-button type="primary" size="small" @click="openCreate">
        <template #icon><Plus :size="14" /></template>
        新建同步任务
      </a-button>
    </header>

    <div v-if="syncStore.loaded && syncStore.tasks.length === 0" class="syncpage-empty">
      <FolderSync :size="44" />
      <p>还没有同步任务</p>
      <span>新建一个任务，把本地文件夹和网盘文件夹关联起来</span>
    </div>

    <div v-else class="syncpage-list">
      <div v-for="task in syncStore.tasks" :key="task.id" class="sync-task" :class="{ disabled: !task.enabled }">
        <div class="sync-task-main">
          <div class="sync-task-name">
            <strong>{{ task.name }}</strong>
            <span class="sync-task-direction">{{ SYNC_DIRECTION_LABEL[task.direction] }}</span>
            <span v-if="task.user_id !== pantreeStore.user_id" class="sync-task-warn">绑定账号未登录</span>
          </div>
          <div class="sync-task-paths">
            <span class="sync-path" :title="task.local_path">本地：{{ task.local_path }}</span>
            <ArrowLeftRight :size="13" />
            <span class="sync-path" :title="remoteSummary(task)">网盘：{{ remoteSummary(task) }}</span>
          </div>
          <div class="sync-task-meta">
            <span>{{ scheduleText(task) }}</span>
            <span :class="['sync-task-last', lastSyncClass(task)]" :title="task.last_sync_message">{{ lastSyncText(task) }}</span>
          </div>
          <div v-if="syncStore.getRunState(task.id).running" class="sync-task-progress">
            <a-progress :percent="syncStore.getRunState(task.id).totalCount ? syncStore.getRunState(task.id).doneCount / syncStore.getRunState(task.id).totalCount : 0" size="small" />
            <span class="sync-task-phase">
              {{ syncStore.getRunState(task.id).phase }}
              <template v-if="syncStore.getRunState(task.id).currentFile">· {{ syncStore.getRunState(task.id).currentFile }}</template>
              <template v-if="syncStore.getRunState(task.id).transferred > 0">· 已传输 {{ humanSize(syncStore.getRunState(task.id).transferred) }}</template>
            </span>
          </div>
        </div>
        <div class="sync-task-actions">
          <a-switch :model-value="task.enabled" size="small" @change="(val: boolean | string | number) => handleToggleEnabled(task, !!val)" />
          <a-button type="text" size="small" :disabled="syncStore.getRunState(task.id).running" title="立即同步" @click="handleRunNow(task)">
            <template #icon><Play :size="15" /></template>
          </a-button>
          <a-button type="text" size="small" title="同步日志" @click="openLogs(task)">
            <template #icon><ScrollText :size="15" /></template>
          </a-button>
          <a-button type="text" size="small" title="编辑" @click="openEdit(task)">
            <template #icon><Pencil :size="15" /></template>
          </a-button>
          <a-button type="text" size="small" status="danger" title="删除" @click="handleDelete(task)">
            <template #icon><Trash2 :size="15" /></template>
          </a-button>
        </div>
      </div>
    </div>

    <a-modal v-model:visible="editVisible" title="同步任务设置" :ok-text="'保存'" :cancel-text="'取消'" unmount-on-close @ok="handleSave">
      <a-form v-if="editTask" :model="editTask" layout="vertical" class="sync-edit-form">
        <a-form-item label="任务名称" required>
          <a-input v-model="editTask.name" placeholder="例如：电影备份" :max-length="30" />
        </a-form-item>
        <a-form-item label="本地文件夹" required>
          <a-input v-model="editTask.local_path" placeholder="选择本地文件夹" readonly @click="pickLocalFolder">
            <template #append><a-button size="small" @click.stop="pickLocalFolder">选择</a-button></template>
          </a-input>
        </a-form-item>
        <a-form-item label="网盘文件夹" required>
          <a-input :model-value="editTask.remote_dir_name" placeholder="从当前网盘中选择文件夹" readonly @click="pickRemoteFolder">
            <template #append><a-button size="small" @click.stop="pickRemoteFolder">选择</a-button></template>
          </a-input>
        </a-form-item>
        <a-form-item label="同步方向">
          <a-radio-group v-model="editTask.direction" type="button">
            <a-radio v-for="(label, key) in SYNC_DIRECTION_LABEL" :key="key" :value="key">{{ label }}</a-radio>
          </a-radio-group>
        </a-form-item>
        <a-form-item label="自动同步">
          <div class="sync-edit-inline">
            <a-input-number v-model="editTask.interval_min" :min="0" :max="1440" placeholder="0">
              <template #suffix>分钟</template>
            </a-input-number>
            <a-checkbox v-model="editTask.sync_on_launch">应用启动时同步一次</a-checkbox>
          </div>
          <div class="sync-edit-tip">间隔填 0 表示不定时自动同步</div>
        </a-form-item>
        <a-form-item label="删除传播（危险）">
          <div class="sync-edit-inline">
            <a-switch v-model="editTask.delete_propagation" />
            <span class="sync-edit-tip">开启后，一边删除的文件另一边也会删除</span>
          </div>
          <div v-if="editTask.delete_propagation" class="sync-edit-inline">
            <a-input-number v-model="editTask.delete_threshold" :min="1" :max="10000">
              <template #prefix>单轮最多删除</template>
              <template #suffix>个</template>
            </a-input-number>
          </div>
        </a-form-item>
      </a-form>
    </a-modal>

    <a-drawer v-model:visible="logVisible" :title="`同步日志 · ${logTaskName}`" :width="480" unmount-on-close>
      <div v-if="!syncStore.getLogs(logTaskId).length" class="sync-log-empty">暂无日志</div>
      <div v-else class="sync-log-list">
        <div v-for="(entry, index) in [...syncStore.getLogs(logTaskId)].reverse()" :key="index" :class="['sync-log-item', logLevelClass(entry.level)]">
          <span class="sync-log-time">{{ formatLogTime(entry.time) }}</span>
          <span class="sync-log-text">{{ entry.text }}</span>
        </div>
      </div>
    </a-drawer>
  </div>
</template>

<style scoped>
.syncpage {
  display: flex;
  height: 100%;
  flex-direction: column;
  overflow: hidden;
  background: var(--color-bg-1);
}

.syncpage-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 16px;
  border-bottom: 1px solid var(--color-border-2);
}

.syncpage-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 14px;
}

.syncpage-sub {
  color: var(--color-text-3);
  font-size: 12px;
}

.syncpage-empty {
  display: flex;
  flex: 1;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 6px;
  color: var(--color-text-3);
}

.syncpage-empty p {
  margin: 8px 0 0;
  color: var(--color-text-2);
  font-size: 15px;
}

.syncpage-empty span {
  font-size: 12px;
}

.syncpage-list {
  flex: 1;
  overflow-y: auto;
  padding: 10px 16px;
}

.sync-task {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 10px;
  margin-bottom: 10px;
  padding: 12px 14px;
  border: 1px solid var(--color-border-2);
  border-radius: 8px;
  background: var(--color-bg-2);
}

.sync-task.disabled {
  opacity: 0.55;
}

.sync-task-main {
  min-width: 0;
  flex: 1;
}

.sync-task-name {
  display: flex;
  align-items: center;
  gap: 8px;
}

.sync-task-name strong {
  font-size: 14px;
}

.sync-task-direction {
  padding: 1px 8px;
  border-radius: 10px;
  background: rgba(var(--primary-6), 0.12);
  color: rgb(var(--primary-6));
  font-size: 11px;
}

.sync-task-warn {
  padding: 1px 8px;
  border-radius: 10px;
  background: var(--color-warning-light-1);
  color: rgb(var(--warning-6));
  font-size: 11px;
}

.sync-task-paths {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-top: 6px;
  color: var(--color-text-2);
  font-size: 12px;
}

.sync-path {
  overflow: hidden;
  max-width: 40%;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.sync-task-meta {
  display: flex;
  gap: 14px;
  margin-top: 6px;
  color: var(--color-text-3);
  font-size: 12px;
}

.sync-task-last.ok {
  color: rgb(var(--success-6));
}

.sync-task-last.err {
  color: rgb(var(--danger-6));
}

.sync-task-progress {
  margin-top: 8px;
}

.sync-task-phase {
  display: block;
  overflow: hidden;
  margin-top: 2px;
  color: var(--color-text-3);
  font-size: 11px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.sync-task-actions {
  display: flex;
  flex-shrink: 0;
  align-items: center;
  gap: 2px;
}

.sync-edit-form :deep(.arco-form-item) {
  margin-bottom: 12px;
}

.sync-edit-inline {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-top: 2px;
}

.sync-edit-inline .arco-input-number {
  width: 180px;
}

.sync-edit-tip {
  color: var(--color-text-3);
  font-size: 12px;
}

.sync-log-empty {
  padding: 30px 0;
  color: var(--color-text-3);
  text-align: center;
}

.sync-log-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.sync-log-item {
  display: flex;
  gap: 8px;
  font-size: 12px;
  line-height: 20px;
}

.sync-log-time {
  flex-shrink: 0;
  color: var(--color-text-3);
}

.sync-log-success {
  color: rgb(var(--success-6));
}

.sync-log-warning {
  color: rgb(var(--warning-6));
}

.sync-log-error {
  color: rgb(var(--danger-6));
}
</style>
