<script setup>
import { computed } from 'vue'

const props = defineProps({
  taskId: { type: String, required: true },
  name: { type: String, default: '' },
  progressById: { type: Object, required: true },
})

const progress = computed(() => props.progressById[props.taskId])
const total = computed(() => Number.isFinite(progress.value?.total) ? Math.max(0, progress.value.total) : 0)
const done = computed(() => Number.isFinite(progress.value?.done) ? Math.max(0, progress.value.done) : 0)
const percentage = computed(() => total.value ? Math.min(100, Math.round(done.value / total.value * 100)) : 0)
</script>

<template>
  <div class="sync-task-progress content-reveal">
    <div class="progress-total" role="progressbar" :aria-label="`${name}同步进度`" :aria-valuenow="total ? percentage : undefined">
      <div class="progress-current active" :style="{ '--progress': percentage / 100 }"></div>
    </div>
    <div class="sync-task-meta sync-progress-label">
      <span v-if="!progress" role="status">同步中，正在等待进度…</span>
      <span v-else-if="!total">同步中 · 已处理 {{ done }} 项</span>
      <span v-else>{{ done }} / {{ total }}（{{ percentage }}%）</span>
    </div>
  </div>
</template>

<style scoped>
.sync-progress-label { margin-top:4px; font-variant-numeric:tabular-nums; }
</style>
