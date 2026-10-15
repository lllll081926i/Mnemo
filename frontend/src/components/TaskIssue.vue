<script setup>
import { computed } from 'vue'

const props = defineProps({ issue: { type: Object, default: null }, writeOperation: { type: Boolean, default: false } })
const emit = defineEmits(['repair'])
const actions = {
  reauthorize: '重新登录原账号后，手动恢复任务',
  network_settings: '检查网络和代理设置后，手动恢复任务',
  check_disk: '本地磁盘空间不足，请清理后重试',
  check_local_path: '请检查本地文件路径、权限或是否被占用',
  refresh_directory: '文件可能已移动或删除，请刷新目录',
  check_target: '请检查目标目录文件状态',
  check_access: '请检查文件访问权限、链接有效期或账号状态',
  inspect_capability: '当前网盘不支持此操作',
  manual_resume: '请核对断点后手动继续任务',
  replace_source: '下载链接已失效，请重新获取链接',
  check_source: '文件校验未通过，请重新下载',
  wait: '请求过于频繁或被网盘限流，请稍后重试',
}
const actionText = computed(() => actions[props.issue?.action] || '请核对错误详情后重试')
const deadline = computed(() => {
  const timestamp = Number(props.issue?.retryAt)
  if (!Number.isFinite(timestamp) || timestamp <= 0) return ''
  const date = new Date(timestamp * 1000)
  return Number.isFinite(date.getTime()) ? date.toLocaleString() : ''
})
</script>

<template>
  <details v-if="issue?.code" class="task-issue">
    <summary>处理建议 · {{ issue.code }}</summary>
    <p>{{ issue.message }}</p>
    <p>{{ actionText }}</p>
    <p v-if="deadline">网盘限制参考恢复时间：{{ deadline }}（到时不代表已解除限制，请稍后重试）</p>
    <p v-if="writeOperation">重试前确认目标目录，避免重复写入。</p>
    <button v-if="['network_settings', 'reauthorize'].includes(issue.action)" type="button" class="btn sm" @click="emit('repair', issue.action)">{{ issue.action === 'reauthorize' ? '重新登录账号' : '网络设置' }}</button>
  </details>
</template>

<style scoped>
.task-issue { font-size: 12px; color: var(--text-secondary); overflow-wrap: anywhere; }
.task-issue summary { cursor: pointer; color: var(--text-primary); }
.task-issue p { margin-top: 4px; }
.task-issue button { margin-top: 6px; }
</style>
