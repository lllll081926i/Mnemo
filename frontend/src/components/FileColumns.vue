<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, ref, useId, watch } from 'vue'
import { formatBytes, iconOf } from '../api'
import UiIcon from './UiIcon.vue'

const props = defineProps({ columns: Array, selectedIds: Set, focusId: String, focusIndex: Number, dragEnabled: Boolean })
const emit = defineEmits(['select', 'open', 'context', 'clear', 'retry', 'drag-start', 'drag-end', 'drop'])
const stage = ref(null)
const height = ref(500)
const positions = ref(new Map())
const activeRows = ref(new Map())
const optionPrefix = useId()
const ROW_HEIGHT = 38
const OVERSCAN = 8
let observer
let pendingChildFocus
const windows = computed(previous => {
  const next = props.columns.map((column, index) => {
    const count = column.files.length
    const start = Math.max(0, Math.min((positions.value.get(column.id) || 0) - OVERSCAN, Math.max(0, count - 1)))
    const end = Math.min(count, start + Math.ceil(height.value / ROW_HEIGHT) + OVERSCAN * 2)
    const cached = previous?.[index]
    if (cached?.source === column && cached.start === start && cached.end === end) return cached
    return { ...column, source: column, start, end, rows: column.files.slice(start, end), bottom: Math.max(0, count - end) * ROW_HEIGHT }
  })
  return previous?.length === next.length && next.every((column, index) => column === previous[index]) ? previous : next
})
function recordScroll(id, scrollTop) {
  const row = Math.floor(scrollTop / ROW_HEIGHT)
  if ((positions.value.get(id) || 0) !== row) positions.value.set(id, row)
}
function activeRow(column) {
  if (column.loading || column.error || !column.rows.length) return -1
  const row = activeRows.value.get(column.id)
  if (row >= column.start && row < column.end) return row
  return Math.min(column.end - 1, positions.value.get(column.id) || 0)
}
const optionId = (columnIndex, row) => `${optionPrefix}-${columnIndex}-${row}`
function selectRow(column, columnIndex, row, event) {
  pendingChildFocus = null
  activeRows.value.set(column.id, row)
  event.currentTarget.closest('.column-list')?.focus({ preventScroll: true })
  emit('select', columnIndex, column.files[row], event)
}
function onListKey(event, column, columnIndex) {
  if (event.target !== event.currentTarget || event.isComposing || event.altKey || event.ctrlKey || event.metaKey) return
  const row = activeRow(column)
  if (row < 0) return
  const list = event.currentTarget
  const page = Math.max(1, Math.floor(list.clientHeight / ROW_HEIGHT))
  let next = row
  pendingChildFocus = null
  switch (event.key) {
    case 'ArrowUp': next--; break
    case 'ArrowDown': next++; break
    case 'Home': next = 0; break
    case 'End': next = column.files.length - 1; break
    case 'PageUp': next -= page; break
    case 'PageDown': next += page; break
    case 'Enter': {
      event.preventDefault()
      if (event.repeat) return
      const file = column.files[row]
      if (file.isDir) pendingChildFocus = { parent: list, columnIndex, id: file.file_id }
      emit('open', columnIndex, file, event)
      return
    }
    case ' ':
      event.preventDefault()
      if (!event.repeat) emit('select', columnIndex, column.files[row], event, false)
      return
    default: return
  }
  event.preventDefault()
  next = Math.max(0, Math.min(column.files.length - 1, next))
  revealRow(list, column.id, next)
}
function revealRow(list, id, row) {
  activeRows.value.set(id, row)
  const top = row * ROW_HEIGHT
  if (top < list.scrollTop) list.scrollTop = top
  else if (top + ROW_HEIGHT > list.scrollTop + list.clientHeight) list.scrollTop = top + ROW_HEIGHT - list.clientHeight
  recordScroll(id, list.scrollTop)
}
watch(() => [props.focusId, props.focusIndex], () => {
  const list = stage.value?.querySelector('.file-column:last-child .column-list')
  const column = props.columns.at(-1)
  if (list && column && props.focusIndex >= 0) revealRow(list, column.id, props.focusIndex)
}, { flush: 'post' })
watch(() => props.columns.map(column => column.id).join('\0'), async (nextPath, previousPath, onCleanup) => {
  let cancelled = false
  onCleanup(() => { cancelled = true })
  const previousFocus = stage.value?.contains(document.activeElement) ? document.activeElement : null
  const ids = new Set(props.columns.map(column => column.id))
  for (const map of [positions.value, activeRows.value]) {
    for (const id of map.keys()) if (!ids.has(id)) map.delete(id)
  }
  await nextTick()
  if (cancelled || !stage.value) return
  const request = pendingChildFocus
  if (request && props.columns[request.columnIndex + 1]?.id === request.id) {
    pendingChildFocus = null
    if (document.activeElement === request.parent) stage.value.querySelectorAll('.column-list')[request.columnIndex + 1]?.focus({ preventScroll: true })
  } else if (previousFocus && !previousFocus.isConnected && document.activeElement === document.body) {
    stage.value.querySelector('.file-column:last-child .column-list')?.focus({ preventScroll: true })
  }
  const enteringChild = previousPath && nextPath.startsWith(previousPath + '\0')
  if (enteringChild && stage.value.scrollTo && !window.matchMedia?.('(prefers-reduced-motion: reduce)').matches) {
    stage.value.scrollTo({ left: stage.value.scrollWidth, behavior: 'smooth' })
  } else stage.value.scrollLeft = stage.value.scrollWidth
})
onMounted(() => {
  if (typeof ResizeObserver !== 'undefined') {
    observer = new ResizeObserver(() => { height.value = Math.max(100, stage.value?.clientHeight - 36 || 500) })
    observer.observe(stage.value)
  }
  if (stage.value) stage.value.scrollLeft = stage.value.scrollWidth
})
onBeforeUnmount(() => observer?.disconnect())
</script>

<template>
  <div ref="stage" class="file-columns" aria-label="目录分栏浏览">
    <section v-for="(column, index) in windows" :key="column.id" v-memo="[column, index, activeRow(column), index === columns.length - 1, index === columns.length - 1 && selectedIds, index === columns.length - 1 && focusId, index === columns.length - 1 && dragEnabled]" class="file-column content-reveal" :aria-label="column.name">
      <header class="column-heading"><UiIcon name="folder" :size="13" /><span>{{ column.name }}</span><small>{{ column.files.length }}</small></header>
      <div class="column-list" :role="column.loading || column.error ? 'region' : 'listbox'" :aria-label="column.name" :aria-multiselectable="column.loading || column.error ? undefined : true" :aria-activedescendant="activeRow(column) >= 0 ? optionId(index, activeRow(column)) : undefined" tabindex="0" @keydown="onListKey($event, column, index)" @scroll.passive="recordScroll(column.id, $event.currentTarget.scrollTop)" @click.self="emit('clear')">
        <div v-if="column.loading" class="column-state" role="status"><span class="spin"></span>正在加载…</div>
        <div v-else-if="column.error" class="column-state" role="alert"><span>目录加载失败</span><button class="btn sm" @click="emit('retry', column.id)">重试</button></div>
        <template v-else>
          <div v-if="column.start" aria-hidden="true" :style="{ height: column.start * ROW_HEIGHT + 'px' }"></div>
          <button v-for="(file, offset) in column.rows" :id="optionId(index, column.start + offset)" :key="file.file_id" type="button" class="column-file" role="option" tabindex="-1" :aria-posinset="column.start + offset + 1" :aria-setsize="column.files.length" :aria-selected="column.activeId === file.file_id || (index === columns.length - 1 && selectedIds.has(file.file_id))" :data-active="activeRow(column) === column.start + offset" :class="{ focus: index === columns.length - 1 && focusId === file.file_id }" :title="file.name" :draggable="dragEnabled && index === columns.length - 1" @click="selectRow(column, index, column.start + offset, $event)" @dblclick="!file.isDir && emit('open', index, file, $event)" @contextmenu.prevent="emit('context', index, $event, file)" @dragstart="emit('drag-start', $event, file)" @dragend="emit('drag-end')" @drop="emit('drop', $event, file)">
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
.column-file { display: flex; align-items: center; gap: 8px; width: 100%; height: 38px; padding: 0 9px; border: 1px solid transparent; border-radius: var(--radius-xs); background: transparent; color: var(--text-primary); font: inherit; font-size: 13px; text-align: left; cursor: pointer; transition: background-color var(--motion-fast) var(--motion-ease), border-color var(--motion-fast) var(--motion-ease); }
.column-file > span { flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.column-file small, .column-arrow { flex: none; font-size: 11px; color: var(--text-tertiary); }
.column-file:hover { background: var(--bg-hover); }
.column-file[aria-selected=true] { background: var(--bg-active); }
.column-list:not(:focus) .column-file.focus, .column-list:focus .column-file[data-active=true] { border-color: var(--border-focus); }
.column-list:focus-visible { outline: 2px solid var(--border-focus); outline-offset: -2px; }
.column-state { display: flex; justify-content: center; align-items: center; flex-wrap: wrap; gap: 8px; padding: 36px 8px; font-size: 12px; color: var(--text-secondary); }
</style>
