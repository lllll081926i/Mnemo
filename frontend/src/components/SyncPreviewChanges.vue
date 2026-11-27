<script setup>
import { computed, ref, watch } from 'vue'
import UiSelect from './UiSelect.vue'

const props = defineProps({
  changes: { type: Array, default: () => [] },
  choices: { type: Object, required: true },
  policy: { type: String, default: 'keep-both' },
  busy: Boolean,
})
const emit = defineEmits(['choose'])
const actionLabels = { upload: '上传', download: '下载', 'rename-local': '重命名本地', 'rename-remote': '重命名网盘', 'delete-local': '删除本地', 'delete-remote': '删除网盘', conflict: '冲突' }
const actionOptions = [{ value: '', label: '全部操作' }, ...Object.entries(actionLabels).map(([value, label]) => ({ value, label }))]
const conflictOptions = [{ value: 'keep-both', label: '保留两份' }, { value: 'local', label: '使用本地' }, { value: 'remote', label: '使用网盘' }]
const policyLabels = { local: '本地', remote: '网盘', 'keep-both': '保留两份' }
const filter = ref('')
const action = ref('')
const page = ref(1)
const list = ref(null)
const pageSize = 40
const normalizedPaths = computed(() => props.changes.map(change => change.path.toLowerCase()))
const filtered = computed(() => {
  const term = filter.value.trim().toLowerCase()
  const selectedAction = action.value
  if (!term && !selectedAction) return props.changes
  const paths = term ? normalizedPaths.value : null
  return props.changes.filter((change, index) => (!selectedAction || change.action === selectedAction) && (!paths || paths[index].includes(term)))
})
const pages = computed(() => Math.max(1, Math.ceil(filtered.value.length / pageSize)))
const current = computed(() => Math.min(page.value, pages.value))
const visible = computed(() => filtered.value.slice((current.value - 1) * pageSize, current.value * pageSize))
watch([filter, action, () => props.changes], () => { page.value = 1 })
watch([filter, action, current, () => props.changes], () => { if (list.value) list.value.scrollTop = 0 }, { flush: 'post' })
const formatDate = timestamp => timestamp ? new Date(timestamp * 1000).toLocaleString() : '未知'
</script>

<template>
  <div class="sync-preview-filters">
    <input v-model="filter" class="input" aria-label="筛选同步路径" placeholder="筛选同步路径" />
    <UiSelect v-model="action" aria-label="筛选同步操作" :options="actionOptions" />
  </div>
  <div ref="list" class="workspace-results sync-preview-list" role="region" aria-label="同步差异" tabindex="0">
    <div v-if="visible.length" :key="`${action}:${current}`" class="content-reveal" role="list">
      <div v-for="change in visible" :key="change.path" class="workspace-result" role="listitem">
        <span>
          <strong>{{ change.fromPath ? `${change.fromPath} → ${change.path}` : change.path }}</strong>
          <small>{{ actionLabels[change.action] }} · 本地 {{ change.local?.size ?? '不存在' }} / 网盘 {{ change.remote?.size ?? '不存在' }}</small>
          <small v-if="change.action === 'conflict'">修改时间：本地 {{ formatDate(change.local?.modTime) }} / 网盘 {{ formatDate(change.remote?.modTime) }}</small>
        </span>
        <UiSelect v-if="change.action === 'conflict'" class="sync-preview-conflict" :model-value="choices[change.path]" :disabled="busy" :aria-label="`冲突策略：${change.path}`" :placeholder="`默认：${policyLabels[policy] || policyLabels['keep-both']}`" :options="conflictOptions" @update:model-value="emit('choose', change.path, $event)" />
      </div>
    </div>
    <p v-else class="sync-preview-empty">{{ changes.length ? '没有匹配的差异' : '没有需要处理的变化' }}</p>
  </div>
  <div v-if="changes.length" class="sync-preview-pages" role="group" aria-label="同步差异分页">
    <span role="status">{{ filtered.length }} 项 · 第 {{ current }} / {{ pages }} 页</span>
    <button type="button" class="btn sm" :aria-disabled="current === 1" @click="page = Math.max(1, current - 1)">上一页</button>
    <button type="button" class="btn sm" :aria-disabled="current === pages" @click="page = Math.min(pages, current + 1)">下一页</button>
  </div>
</template>

<style scoped>
.sync-preview-filters, .sync-preview-pages { display:flex; flex-wrap:wrap; align-items:center; gap:8px; margin-block:8px; }
.sync-preview-filters .input { flex:1; min-width:140px; }
.sync-preview-list { max-height:min(40vh, 340px); border:1px solid var(--border-lighter); border-radius:var(--radius-md); scrollbar-gutter:stable; }
.sync-preview-conflict { flex-shrink:0; }
.sync-preview-empty { padding:10px; color:var(--text-secondary); }
.sync-preview-pages span { margin-right:auto; color:var(--text-secondary); font-size:12px; }
.sync-preview-pages [aria-disabled=true] { opacity:.5; cursor:default; }
</style>
