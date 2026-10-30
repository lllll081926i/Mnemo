<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { formatBytes, iconOf } from '../api'
import UiIcon from './UiIcon.vue'

const props = defineProps({ columns: Array, selectedIds: Set, focusId: String, dragEnabled: Boolean })
const emit = defineEmits(['select', 'open', 'context', 'clear', 'retry', 'drag-start', 'drag-end', 'drop'])
const stage = ref(null)
const height = ref(500)
const positions = ref({})
const ROW_HEIGHT = 38
const OVERSCAN = 8
let observer
const windows = computed(() => props.columns.map(column => {
  const count = column.files.length
  const start = Math.max(0, Math.min(Math.floor((positions.value[column.id] || 0) / ROW_HEIGHT) - OVERSCAN, Math.max(0, count - 1)))
  const end = Math.min(count, start + Math.ceil(height.value / ROW_HEIGHT) + OVERSCAN * 2)
  return { ...column, start, end, rows: column.files.slice(start, end), bottom: Math.max(0, count - end) * ROW_HEIGHT }
}))
function reveal() {
  const list = stage.value?.querySelector('.file-column:last-child .column-list')
  if (!list || !props.focusId) return
  const index = props.columns.at(-1)?.files.findIndex(file => file.file_id === props.focusId)
  if (index == null || index < 0) return
  const top = index * ROW_HEIGHT
  if (top < list.scrollTop) list.scrollTop = top
  else if (top + ROW_HEIGHT > list.scrollTop + list.clientHeight) list.scrollTop = top + ROW_HEIGHT - list.clientHeight
  positions.value = { ...positions.value, [props.columns.at(-1).id]: list.scrollTop }
}
watch(() => props.focusId, () => nextTick(reveal))
watch(() => props.columns.map(column => column.id).join('\0'), async () => {
  const ids = new Set(props.columns.map(column => column.id))
  positions.value = Object.fromEntries(Object.entries(positions.value).filter(([id]) => ids.has(id)))
  await nextTick()
  if (stage.value) stage.value.scrollLeft = stage.value.scrollWidth
})
onMounted(() => {
  if (typeof ResizeObserver !== 'undefined') {
    observer = new ResizeObserver(() => { height.value = Math.max(100, stage.value?.clientHeight - 36 || 500) })
    observer.observe(stage.value)
  }
  if (stage.value) stage.value.scrollLeft = stage.value.scrollWidth
})
onBeforeUnmount(() => observer?.disconnect())
defineExpose({ reveal })
</script>

<template>
  <div ref="stage" class="file-columns" aria-label="目录分栏浏览">
    <section v-for="(column, index) in windows" :key="column.id" class="file-column" :aria-label="column.name">
      <header class="column-heading"><UiIcon name="folder" :size="13" /><span>{{ column.name }}</span><small>{{ column.files.length }}</small></header>
      <div class="column-list" @scroll.passive="positions = { ...positions, [column.id]: $event.currentTarget.scrollTop }" @click.self="emit('clear')">
        <div v-if="column.loading" class="column-state" role="status"><span class="spin"></span>正在加载…</div>
        <div v-else-if="column.error" class="column-state" role="alert"><span>目录加载失败</span><button class="btn sm" @click="emit('retry', column.id)">重试</button></div>
        <template v-else>
          <div v-if="column.start" aria-hidden="true" :style="{ height: column.start * ROW_HEIGHT + 'px' }"></div>
          <button v-for="file in column.rows" :key="file.file_id" type="button" class="column-file" :class="{ selected: column.activeId === file.file_id || (index === columns.length - 1 && selectedIds.has(file.file_id)), focus: index === columns.length - 1 && focusId === file.file_id }" :title="file.name" :draggable="dragEnabled && index === columns.length - 1" @click="emit('select', index, file, $event)" @dblclick="!file.isDir && emit('open', index, file, $event)" @contextmenu.prevent="emit('context', index, $event, file)" @dragstart="emit('drag-start', $event, file)" @dragend="emit('drag-end')" @drop="emit('drop', $event, file)">
            <UiIcon :name="iconOf(file)" :class="'ft-' + iconOf(file)" :size="17" />
            <span>{{ file.name }}</span>
            <UiIcon v-if="file.isDir" name="chevron-right" :size="13" class="column-arrow" />
            <small v-else>{{ formatBytes(file.size) }}</small>
          </button>
          <div v-if="column.bottom" aria-hidden="true" :style="{ height: column.bottom + 'px' }"></div>
          <div v-if="!column.files.length" class="column-state">空目录</div>
        </template>
      </div>
    </section>
  </div>
</template>

<style scoped>
.file-columns { display: flex; flex: 1; min-height: 0; overflow-x: auto; background: var(--bg-base); }
.file-column { display: flex; flex-direction: column; width: 270px; min-width: 230px; flex: 0 0 270px; min-height: 0; border-right: 1px solid var(--border-light); }
.file-column:last-child { flex-grow: 1; }
.column-heading { display: flex; align-items: center; gap: 7px; height: 36px; flex: none; padding: 0 13px; border-bottom: 1px solid var(--border-lighter); color: var(--text-secondary); font-size: 12px; }
.column-heading > span { flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.column-heading small { color: var(--text-tertiary); font-variant-numeric: tabular-nums; }
.column-list { flex: 1; min-height: 0; overflow-y: auto; padding-inline: 6px; scrollbar-gutter: stable; overflow-anchor: none; }
.column-file { display: flex; align-items: center; gap: 8px; width: 100%; height: 38px; padding: 0 9px; border: 1px solid transparent; border-radius: var(--radius-xs); background: transparent; color: var(--text-primary); font: inherit; font-size: 13px; text-align: left; cursor: pointer; }
.column-file > span { flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.column-file small, .column-arrow { flex: none; font-size: 11px; color: var(--text-tertiary); }
.column-file:hover { background: var(--bg-hover); }
.column-file.selected { background: var(--bg-active); }
.column-file.focus { border-color: var(--border-focus); }
.column-state { display: flex; justify-content: center; align-items: center; flex-wrap: wrap; gap: 8px; padding: 36px 8px; font-size: 12px; color: var(--text-secondary); }
</style>
