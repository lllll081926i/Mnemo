<script setup>
import { computed } from 'vue'
import { downTaskStatus, downProgress } from '../download-display'

const props = defineProps({ children: { type: Array, required: true }, open: Boolean, failedOnly: Boolean, limit: { type: Number, required: true }, trackProgress: Boolean })
const emit = defineEmits(['toggle', 'filter', 'more', 'retry'])
const filtered = computed(() => props.failedOnly ? props.children.filter(child => child.status === 'failed' || child.status === 'canceled') : props.children)
const visibleChildren = computed(() => filtered.value.slice(0, props.limit))
</script>

<template>
  <details class="folder-task-details" :open="open" @toggle="emit('toggle', $event.currentTarget.open)" @click.stop>
    <summary>查看文件（{{ Math.min(filtered.length, limit) }}/{{ filtered.length }}）</summary>
    <div v-if="open" class="content-reveal">
      <button class="btn sm" type="button" @click="emit('filter')">{{ failedOnly ? '查看全部' : '只看失败项' }}</button>
      <div v-for="child in visibleChildren" :key="child.id" class="folder-task-child" :title="child.error || child.localPath">
        <span>{{ child.name }} · {{ downTaskStatus(child) }} <template v-if="trackProgress && child.status === 'downloading'">{{ downProgress(child) }}%</template></span>
        <button v-if="['failed', 'paused'].includes(child.status)" class="btn sm" type="button" @click="emit('retry', child)">重试</button>
      </div>
      <button v-if="filtered.length > limit" class="btn sm" type="button" @click="emit('more')">显示更多</button>
    </div>
  </details>
</template>

<style scoped>
.folder-task-details { color: var(--text-secondary); font-size: 12px; max-height: 180px; overflow: auto; }
.folder-task-details summary { cursor: pointer; width: fit-content; color: var(--color-primary); }
.folder-task-child { display: flex; align-items: center; gap: 6px; min-width: 0; padding: 2px 0 2px 12px; }
.folder-task-child span { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
</style>
