<script setup>
import { computed, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import UiSelect from './UiSelect.vue'

const props = defineProps({ bytes: Object, file: Object })
const loading = ref(true)
const error = ref('')
const workbook = shallowRef(null)
const activeSheet = ref('')
const stage = ref(null)
const scrollTop = ref(0)
const viewportHeight = ref(600)
const ROW_HEIGHT = 28
const OVERSCAN_ROWS = 12
const MAX_COLUMNS = 50
let library = null
let disposed = false
let stageObserver = null

const sheetNames = computed(() => workbook.value?.SheetNames || [])
const sheet = computed(() => workbook.value?.Sheets[activeSheet.value])
const range = computed(() => {
  if (!library || !sheet.value?.['!ref']) return null
  try { return library.utils.decode_range(sheet.value['!ref']) } catch { return null }
})
const rowCount = computed(() => range.value ? Math.min(1_048_576, range.value.e.r + 1) : 0)
const columnCount = computed(() => range.value ? Math.min(MAX_COLUMNS, range.value.e.c + 1) : 0)
// 保留全部行的滚动高度，仅创建视口及相邻行的单元格。
const visibleRows = computed(() => Math.ceil(viewportHeight.value / ROW_HEIGHT) + OVERSCAN_ROWS * 2)
const startRow = computed(() => Math.min(Math.max(0, rowCount.value - visibleRows.value), Math.max(0, Math.floor((scrollTop.value - ROW_HEIGHT) / ROW_HEIGHT) - OVERSCAN_ROWS)))
const endRow = computed(() => Math.min(rowCount.value, startRow.value + visibleRows.value))
const columns = computed(() => library ? Array.from({ length: columnCount.value }, (_, index) => library.utils.encode_col(index)) : [])
const rows = computed(() => {
  if (!library || !sheet.value) return []
  const end = endRow.value
  return Array.from({ length: Math.max(0, end - startRow.value) }, (_, offset) => {
    const number = startRow.value + offset
    const cells = columns.value.map((_, column) => {
      const cell = sheet.value[library.utils.encode_cell({ r: number, c: column })]
      if (!cell) return ''
      if (cell.v == null && cell.f) return `=${cell.f}`
      try { return library.utils.format_cell(cell) } catch { return String(cell.w ?? cell.v ?? '') }
    })
    return { number: number + 1, cells }
  })
})

async function openWorkbook() {
  try {
    library = await import('xlsx')
    if (disposed) return
    const parsed = library.read(props.bytes, { type: 'array', cellText: true, cellDates: true })
    if (!parsed.SheetNames.length) throw new Error('文件中没有可显示的工作表')
    workbook.value = parsed
    activeSheet.value = parsed.SheetNames[0]
  } catch (cause) {
    if (!disposed) error.value = String(cause?.message || cause)
  } finally {
    if (!disposed) loading.value = false
  }
}

function resizeViewport() { if (stage.value) viewportHeight.value = stage.value.clientHeight || 600 }
function selectSheet(value) {
  activeSheet.value = value
  scrollTop.value = 0
  if (stage.value) { stage.value.scrollTop = 0; stage.value.scrollLeft = 0 }
}
watch(stage, element => {
  stageObserver?.disconnect()
  if (!element) return
  resizeViewport()
  if (typeof ResizeObserver !== 'undefined') {
    stageObserver = new ResizeObserver(resizeViewport)
    stageObserver.observe(element)
  }
})
onMounted(openWorkbook)
onMounted(() => window.addEventListener('resize', resizeViewport))
onBeforeUnmount(() => { disposed = true; stageObserver?.disconnect(); window.removeEventListener('resize', resizeViewport); workbook.value = null })
</script>

<template>
  <div class="spreadsheet-viewer">
    <div class="spreadsheet-toolbar"><strong>表格预览</strong>
      <div v-if="sheetNames.length" class="spreadsheet-controls">
        <UiSelect :model-value="activeSheet" :options="sheetNames.map(name => ({ value: name, label: name }))" @change="selectSheet" />
        <span>共 {{ rowCount.toLocaleString() }} 行</span>
      </div>
    </div>
    <div v-if="loading" class="spreadsheet-state"><span class="spin"></span>正在读取表格…</div>
    <div v-else-if="error" class="spreadsheet-state" role="alert">{{ error }}</div>
    <p v-if="!loading && !error && range && range.e.c + 1 > MAX_COLUMNS" class="spreadsheet-note">为保持流畅，仅展示前 {{ MAX_COLUMNS }} 列</p>
    <div v-if="!loading && !error" ref="stage" class="spreadsheet-stage" @scroll.passive="scrollTop = $event.currentTarget.scrollTop">
      <table :aria-rowcount="rowCount + 1" :style="{ width: `${columnCount * 144 + 46}px` }"><colgroup><col style="width: 46px" /><col v-for="column in columns" :key="column" /></colgroup><thead><tr aria-rowindex="1"><th scope="col"></th><th v-for="column in columns" :key="column" scope="col">{{ column }}</th></tr></thead>
        <tbody>
          <tr v-if="startRow" class="spreadsheet-spacer" aria-hidden="true"><td :colspan="columnCount + 1" :style="{ height: `${startRow * ROW_HEIGHT}px` }"></td></tr>
          <tr v-for="row in rows" :key="row.number" :class="{ 'spreadsheet-even': row.number % 2 === 0 }" :aria-rowindex="row.number + 1"><th scope="row">{{ row.number }}</th><td v-for="(cell, index) in row.cells" :key="index" :title="cell">{{ cell }}</td></tr>
          <tr v-if="endRow < rowCount" class="spreadsheet-spacer" aria-hidden="true"><td :colspan="columnCount + 1" :style="{ height: `${(rowCount - endRow) * ROW_HEIGHT}px` }"></td></tr>
        </tbody>
      </table>
      <div v-if="!rowCount" class="spreadsheet-state">此工作表没有内容</div>
    </div>
  </div>
</template>

<style scoped>
.spreadsheet-viewer { flex: 1; min-height: 0; display: flex; flex-direction: column; }
.spreadsheet-toolbar { min-height: 48px; padding: 7px 16px; display: flex; align-items: center; justify-content: space-between; gap: 12px; border-bottom: 1px solid var(--border-light); background: var(--bg-surface); }
.spreadsheet-toolbar strong { font-size: 12px; color: var(--text-secondary); white-space: nowrap; }
.spreadsheet-controls { display: flex; align-items: center; justify-content: flex-end; gap: 8px; font-size: 12px; color: var(--text-secondary); font-variant-numeric: tabular-nums; }
.spreadsheet-controls :deep(.uiselect) { min-width: 100px; max-width: 200px; }
.spreadsheet-state { flex: 1; display: flex; align-items: center; justify-content: center; gap: 10px; color: var(--text-secondary); font-size: 13px; }
.spreadsheet-stage { flex: 1; min-height: 0; overflow: auto; overflow-anchor: none; background: var(--bg-surface); }
.spreadsheet-note { flex-shrink: 0; margin: 0; padding: 8px 16px; color: var(--text-secondary); font-size: 12px; }
table { table-layout: fixed; border-collapse: separate; border-spacing: 0; min-width: 100%; font-size: 12px; font-variant-numeric: tabular-nums; }
th, td { box-sizing: border-box; min-width: 96px; max-width: 280px; height: 28px; line-height: 18px; padding: 4px 8px; border-right: 1px solid var(--border-lighter); border-bottom: 1px solid var(--border-lighter); overflow: hidden; white-space: nowrap; text-overflow: ellipsis; text-align: left; }
.spreadsheet-spacer td { padding: 0; border: 0; }
thead th { position: sticky; top: 0; z-index: 2; background: var(--bg-elevated); color: var(--text-secondary); text-align: center; }
tbody th { position: sticky; left: 0; z-index: 1; min-width: 46px; width: 46px; max-width: 46px; background: var(--bg-elevated); color: var(--text-tertiary); text-align: center; }
thead th:first-child { left: 0; z-index: 3; min-width: 46px; width: 46px; max-width: 46px; }
.spreadsheet-even td { background: color-mix(in srgb, var(--bg-hover) 45%, transparent); }
@media (max-width: 600px) { .spreadsheet-toolbar, .spreadsheet-controls { flex-wrap: wrap; } }
</style>
