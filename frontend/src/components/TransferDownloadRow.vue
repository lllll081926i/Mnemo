<script setup>
import { computed } from 'vue'
import { formatBytes, formatSpeed, iconOf } from '../api'
import { downTaskStatus, downProgress } from '../download-display'
import UiIcon from './UiIcon.vue'
import TaskIssue from './TaskIssue.vue'

const props = defineProps({ task: { type: Object, required: true }, selected: Boolean })
const emit = defineEmits(['select', 'toggle', 'context', 'prioritize', 'pause', 'resume', 'reveal', 'cancel', 'remove'])
const name = computed(() => props.task.name || props.task.url || props.task.id)
const icon = computed(() => iconOf({ name: props.task.name }))
const running = computed(() => props.task.status === 'downloading')
const canPause = computed(() => running.value || props.task.status === 'queued')
const indeterminate = computed(() => (props.task.isDir && !props.task.scanDone) || (running.value && !!props.task.stage))
const status = computed(() => downTaskStatus(props.task))
const badge = computed(() => ({ downloading: 'primary', queued: 'primary', completed: 'success', failed: 'error', canceled: 'warn', paused: 'warn' }[props.task.status] || ''))
const progress = computed(() => downProgress(props.task))
</script>

<template>
  <div class="taskrow" :class="{ selected }" @click="emit('select', $event)" @contextmenu.prevent="emit('context', $event)">
    <div class="rangselect">
      <button class="btn-circle" :class="{ on: selected }" :aria-label="`选择 ${name}`" :aria-pressed="selected" type="button" @click.stop="emit('toggle')">
        <UiIcon :name="selected ? 'check' : 'plus'" :size="13" />
      </button>
    </div>
    <div class="fileicon" :class="'ft-' + icon"><UiIcon :name="icon" :size="20" /></div>
    <div class="filename">
      <div :title="task.localPath || task.url || ''">{{ name }}</div>
      <div class="fsub">{{ task.isDir ? `${task.scanDone ? '' : '正在扫描 · '}${task.completedFiles || 0}/${task.totalFiles || 0} 个文件${task.failedFiles ? ` · 失败 ${task.failedFiles}` : ''}` : task.localPath || task.url || '' }}</div>
      <slot />
    </div>
    <div class="filesize">{{ task.isDir && !task.scanDone ? '扫描中' : formatBytes(task.size) }}</div>
    <div class="downprogress">
      <div class="transfering-state">
        <p class="text-state"><span :key="running ? `downloading:${task.stage || ''}` : task.status" class="badge content-reveal" :class="badge">{{ status }}</span><span>{{ indeterminate ? '…' : `${progress}%` }}</span></p>
        <div class="progress-total" role="progressbar" :aria-label="`${name}：${status}`" :aria-valuenow="indeterminate ? undefined : progress">
          <div class="progress-current" :class="canPause ? 'active' : task.status === 'completed' ? 'succeed' : task.status === 'failed' ? 'error' : ''" :style="{ '--progress': progress / 100 }"></div>
        </div>
        <TaskIssue v-if="task.status === 'failed'" :issue="task.issue" :reason="task.error" action="下载文件" :user-id="task.user_id" />
      </div>
    </div>
    <div class="downspeed">{{ running ? formatSpeed(task.speed || 0) : '' }}</div>
    <div class="tactions" @click.stop>
      <button v-if="task.status !== 'completed' && !task.isDir" class="btn-circle" title="优先下载（暂停其他任务）" @click="emit('prioritize', task)"><UiIcon name="priority" :size="14" /></button>
      <button v-if="canPause" class="btn-circle" title="暂停" @click="emit('pause', task)"><UiIcon name="pause" :size="14" /></button>
      <button v-else-if="task.status === 'paused' || task.status === 'failed'" class="btn-circle" :title="task.status === 'failed' ? '重试' : '继续'" @click="emit('resume', task)"><UiIcon name="play" :size="14" /></button>
      <button v-if="task.localPath" class="btn-circle" title="打开所在目录" @click="emit('reveal', task)"><UiIcon name="folder" :size="14" /></button>
      <button v-if="task.status !== 'canceled'" class="btn-circle danger" title="取消" @click="emit('cancel', task)"><UiIcon name="x-circle" :size="14" /></button>
      <button class="btn-circle danger" title="删除记录" @click="emit('remove', task)"><UiIcon name="trash" :size="14" /></button>
    </div>
  </div>
</template>
