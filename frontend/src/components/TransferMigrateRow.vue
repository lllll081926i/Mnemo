<script setup>
import { computed } from 'vue'
import { formatBytes } from '../api'
import UiIcon from './UiIcon.vue'
import TaskIssue from './TaskIssue.vue'

const props = defineProps({
  job: { type: Object, required: true },
  sourceName: String,
  sourceIcon: String,
  targetName: String,
  targetIcon: String,
  statusText: { type: String, required: true },
})
const emit = defineEmits(['details', 'repair', 'cancel', 'resume', 'remove'])
const active = computed(() => ['pending', 'running'].includes(props.job.status))
const retryable = computed(() => ['partial', 'failed', 'canceled'].includes(props.job.status))
const remaining = computed(() => {
  if (!retryable.value) return 0
  const completed = new Set(props.job.completedFileIDs || [])
  return (props.job.fileIDs || []).filter(id => !completed.has(id)).length
})
const progress = computed(() => {
  const { totalBytes, processedBytes, total, processed } = props.job
  const ratio = totalBytes > 0 ? (processedBytes || 0) / totalBytes : total > 0 ? (processed || 0) / total : 0
  return Math.max(0, Math.min(100, Math.round(ratio * 100)))
})
const stageText = computed(() => {
  const { status, stage, stageProgress } = props.job
  if (status !== 'running') return props.statusText
  if ((stage === 'local-upload' && stageProgress >= 100) || (stage === 'uploading' && progress.value >= 100)) return '等待网盘确认'
  return ({ hashing: '计算哈希', checking: '检查秒传', preparing: '准备目标', resuming: '检查续传', downloading: '下载来源', scanning: '读取目录', cleaning: '核对并清理来源', 'local-upload': '上传目标' })[stage] || props.statusText
})
const shownProgress = computed(() => {
  const { stage, stageProgress } = props.job
  if (['hashing', 'local-upload'].includes(stage)) return `${stageProgress || 0}%`
  if (['checking', 'preparing', 'resuming', 'scanning', 'cleaning'].includes(stage)) return '…'
  return `${progress.value}%`
})
const progressDetail = computed(() => props.job.totalBytes > 0
  ? `${formatBytes(props.job.processedBytes || 0)} / ${formatBytes(props.job.totalBytes)}`
  : `${props.job.processed || 0} / ${props.job.total || 0} 个文件`)
const badge = computed(() => ({ completed: 'success', failed: 'error', running: 'primary' }[props.job.status] || 'warn'))
const fill = computed(() => ({ completed: 'succeed', failed: 'error', partial: 'warning' }[props.job.status] || 'active'))
</script>

<template>
  <div class="taskrow">
    <div class="rangselect"></div>
    <div class="fileicon"><UiIcon name="migrate" :size="20" style="color:var(--color-primary)" /></div>
    <div class="filename">
      <button class="tbtn account-inline" @click="emit('details', job.id)"><img :src="sourceIcon" alt="" />{{ sourceName }}<span>→</span><img :src="targetIcon" alt="" />{{ targetName }} · 详情</button>
      <div class="fsub">{{ (job.fileIDs || []).length }} 个文件<template v-if="job.status === 'running' && job.stageName"> · {{ job.stageName }}</template><template v-if="job.failed"> · 失败 {{ job.failed }}</template><template v-if="remaining"> · 剩余未完成 {{ remaining }} 项</template></div>
    </div>
    <div class="filesize"></div>
    <div class="downprogress">
      <div class="transfering-state">
        <p class="text-state"><span :key="stageText" class="badge content-reveal" :class="badge">{{ stageText }}</span><span>{{ shownProgress }}</span></p>
        <div class="progress-total" role="progressbar" aria-label="迁移总体进度" :aria-valuenow="job.totalBytes > 0 || job.total > 0 || !active ? progress : undefined">
          <div class="progress-current" :class="fill" :style="{ '--progress': progress / 100 }"></div>
        </div>
        <TaskIssue v-if="['failed', 'partial'].includes(job.status)" :issue="job.issue?.action === 'reauthorize' ? { ...job.issue, action: 'check_access' } : job.issue" :reason="job.message" action="迁移文件" write-operation />
        <div v-if="job.issue?.action === 'reauthorize'" class="migration-auth-actions">
          <button type="button" class="btn sm" @click="emit('repair', { action: 'reauthorize', userId: job.srcUser })">重新登录来源账号</button>
          <button type="button" class="btn sm" @click="emit('repair', { action: 'reauthorize', userId: job.dstUser })">重新登录目标账号</button>
        </div>
      </div>
    </div>
    <div class="downspeed">{{ progressDetail }}</div>
    <div class="tactions">
      <button v-if="active" class="btn-circle danger" title="取消迁移" @click="emit('cancel', job)"><UiIcon name="x-circle" :size="14" /></button>
      <template v-else>
        <button v-if="remaining" class="btn-circle" title="继续未完成任务" @click="emit('resume', job)"><UiIcon name="refresh" :size="14" /></button>
        <button class="btn-circle danger" title="删除记录" @click="emit('remove', job)"><UiIcon name="trash" :size="14" /></button>
      </template>
    </div>
  </div>
</template>
