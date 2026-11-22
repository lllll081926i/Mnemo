<script setup>
import { computed } from 'vue'
import { formatSpeed } from '../api'
import UiIcon from './UiIcon.vue'
import TaskIssue from './TaskIssue.vue'

const props = defineProps({
  task: { type: Object, required: true },
  name: { type: String, required: true },
  icon: { type: String, required: true },
  size: { type: String, default: '' },
  needsReview: Boolean,
})
const emit = defineEmits(['navigate', 'resume', 'review', 'cancel'])

const upload = computed(() => props.task.Upload || {})
const status = computed(() => {
  const current = upload.value
  if (current.IsCompleted) return '已完成'
  if (!current.IsDowning && props.needsReview) return '待核对'
  if (current.IsFailed) return '失败'
  if (current.IsStop) return '已停止'
  if (current.stage === 'hashing') return '计算哈希'
  if (current.stage === 'preparing') return '准备目录'
  if (current.stage === 'checking') return '检查秒传'
  if (current.stage === 'resuming') return '检查续传'
  if (current.stage === 'uploading' && current.DownProcess >= 100) return '等待网盘确认'
  return current.IsDowning ? '上传中' : '排队中'
})
const badge = computed(() => ({ 已完成: 'success', 失败: 'error', 已停止: 'warn', 待核对: 'warn' }[status.value] || 'primary'))
const indeterminate = computed(() => ['preparing', 'checking', 'resuming'].includes(upload.value.stage))
const progress = computed(() => {
  if (indeterminate.value) return 0
  const value = upload.value.stage === 'hashing' ? upload.value.stageProgress : upload.value.DownProcess
  return Number.isFinite(value) ? Math.max(0, Math.min(100, value)) : 0
})
const speed = computed(() => upload.value.DownSpeedStr || formatSpeed(upload.value.DownSpeed || 0))

function openTarget() {
  const { user_id, Info } = props.task
  emit('navigate', { userId: user_id, driveId: Info?.drive_id, dirId: upload.value.attempt?.parentId || Info?.parent_file_id, name: '上传目标' })
}
</script>

<template>
  <div class="taskrow">
    <div class="rangselect"></div>
    <div class="fileicon" :class="'ft-' + icon"><UiIcon :name="icon" :size="20" /></div>
    <div class="filename">
      <div>{{ name }}</div>
      <div class="fsub">{{ task.Info?.localFilePath || '' }}</div>
    </div>
    <div class="filesize">{{ size }}</div>
    <div class="downprogress">
      <div class="transfering-state">
        <p class="text-state"><span :key="status" class="badge content-reveal" :class="badge">{{ status }}</span><span>{{ indeterminate ? '…' : `${progress}%` }}</span></p>
        <div class="progress-total" role="progressbar" :aria-label="`${name}：${status}`" :aria-valuenow="indeterminate ? undefined : progress">
          <div class="progress-current" :class="needsReview ? 'warning' : upload.IsFailed ? 'error' : 'active'" :style="{ '--progress': progress / 100 }"></div>
        </div>
        <TaskIssue v-if="!upload.IsCompleted && !upload.IsDowning" :issue="upload.issue" :reason="upload.failedMessage" action="上传文件" :user-id="task.user_id" write-operation />
      </div>
    </div>
    <div class="downspeed">{{ speed }}</div>
    <div class="tactions">
      <template v-if="!upload.IsDowning && (upload.IsFailed || upload.IsStop)">
        <button v-if="needsReview" class="btn sm" title="打开上传目标目录" @click="openTarget">目标目录</button>
        <button class="btn-circle" :title="needsReview ? '复核上传结果，不重新提交' : '重试上传'" @click="emit('resume', task)"><UiIcon name="refresh" :size="14" /></button>
        <button v-if="needsReview" class="btn sm" @click="emit('review', task)">核对后重试</button>
      </template>
      <button class="btn-circle danger" title="取消上传" @click="emit('cancel', task)"><UiIcon name="x-circle" :size="14" /></button>
    </div>
  </div>
</template>
