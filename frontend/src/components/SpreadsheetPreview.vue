<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import UiSelect from './UiSelect.vue'

const props = defineProps({ bytes: Object, file: Object })
const loading = ref(true)
const error = ref('')
const workbook = shallowRef(null)
const activeSheet = ref('')
const stage = ref(null)
const sheetFade = ref(null)
const scrollTop = ref(0)
const scrollLeft = ref(0)
const viewportHeight = ref(600)
const viewportWidth = ref(800)
const ROW_HEIGHT = 28
const OVERSCAN_ROWS = 12
const COLUMN_WIDTH = 144
const OVERSCAN_COLUMNS = 2
const MAX_COLUMNS = 50
let library = null
let disposed = false
let stageObserver = null
let scrollFrame = null
let sheetAnimation = null

const sheetNames = computed(() => workbook.value?.SheetNames || [])
const sheetOptions = computed(() => sheetNames.value.map(name => ({ value: name, label: name })))
const sheet = computed(() => workbook.value?.Sheets[activeSheet.value])
const range = computed(() => {
  if (!library || !sheet.value?.['!ref']) return null
  try { return library.utils.decode_range(sheet.value['!ref']) } catch { return null }
})
const rowCount = computed(() => range.value ? Math.min(1_048_576, range.value.e.r + 1) : 0)
const columnCount = computed(() => range.value ? Math.min(MAX_COLUMNS, range.value.e.c + 1) : 0)
const rowHeaderWidth = computed(() => Math.max(46, String(rowCount.value).length * 8 + 17))
const columnWidth = computed(() => Math.max(COLUMN_WIDTH, Math.floor((viewportWidth.value - rowHeaderWidth.value) / (columnCount.value || 1))))
const visibleColumns = computed(() => Math.ceil(Math.max(0, viewportWidth.value - rowHeaderWidth.value) / columnWidth.value) + 1 + OVERSCAN_COLUMNS * 2)
const startColumn = computed(() => Math.min(Math.max(0, columnCount.value - visibleColumns.value), Math.max(0, Math.floor(scrollLeft.value / columnWidth.value) - OVERSCAN_COLUMNS)))
const endColumn = computed(() => Math.min(columnCount.value, startColumn.value + visibleColumns.value))
const leftWidth = computed(() => startColumn.value * columnWidth.value)
const rightWidth = computed(() => (columnCount.value - endColumn.value) * columnWidth.value)
const renderedColumnCount = computed(() => 1 + endColumn.value - startColumn.value + Number(Boolean(leftWidth.value)) + Number(Boolean(rightWidth.value)))
const visibleRows = computed(() => Math.ceil(viewportHeight.value / ROW_HEIGHT) + OVERSCAN_ROWS * 2)
const startRow = computed(() => Math.min(Math.max(0, rowCount.value - visibleRows.value), Math.max(0, Math.floor((scrollTop.value - ROW_HEIGHT) / ROW_HEIGHT) - OVERSCAN_ROWS)))
const endRow = computed(() => Math.min(rowCount.value, startRow.value + visibleRows.value))
const columns = computed(() => library ? Array.from({ length: endColumn.value - startColumn.value }, (_, offset) => library.utils.encode_col(startColumn.value + offset)) : [])
const rows = computed(() => {
  if (!library || !sheet.value) return []
  const end = endRow.value
  return Array.from({ length: Math.max(0, end - startRow.value) }, (_, offset) => {
    const number = startRow.value + offset
    const cells = columns.value.map(column => {
      const cell = sheet.value[`${column}${number + 1}`]
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

function resizeViewport() {
  if (!stage.value) return
  viewportHeight.value = stage.value.clientHeight || 600
  viewportWidth.value = stage.value.clientWidth || 800
}
function onScroll() {
  if (scrollFrame !== null) return
  scrollFrame = requestAnimationFrame(() => {
    scrollFrame = null
    if (!stage.value) return
    scrollTop.value = stage.value.scrollTop
    scrollLeft.value = stage.value.scrollLeft
  })
}
async function selectSheet(value) {
  if (value === activeSheet.value) return
  sheetAnimation?.cancel()
  activeSheet.value = value
  scrollTop.value = 0
  scrollLeft.value = 0
  if (stage.value) { stage.value.scrollTop = 0; stage.value.scrollLeft = 0 }
  await nextTick()
  if (disposed || activeSheet.value !== value || window.matchMedia?.('(prefers-reduced-motion: reduce)').matches) return
  sheetAnimation = sheetFade.value?.animate?.([{ opacity: 0.28 }, { opacity: 0 }], { duration: 150, easing: 'ease-out' })
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
onBeforeUnmount(() => {
  disposed = true
  stageObserver?.disconnect()
  window.removeEventListener('resize', resizeViewport)
  if (scrollFrame !== null) cancelAnimationFrame(scrollFrame)
  sheetAnimation?.cancel()
  workbook.value = null
})
</script>

<template>
  <div class="spreadsheet-viewer">
    <div class="spreadsheet-toolbar"><strong>表格预览</strong>
      <div v-if="sheetNames.length" class="spreadsheet-controls">
        <UiSelect :model-value="activeSheet" :options="sheetOptions" aria-label="工作表" @change="selectSheet" />
        <span aria-live="polite">共 {{ rowCount.toLocaleString() }} 行</span>
      </div>
    </div>
    <div v-if="loading" class="spreadsheet-state"><span class="spin"></span>正在读取表格…</div>
    <div v-else-if="error" class="spreadsheet-state" role="alert">{{ error }}</div>
    <p v-if="!loading && !error && range && range.e.c + 1 > MAX_COLUMNS" class="spreadsheet-note">为保持流畅，仅展示前 {{ MAX_COLUMNS }} 列</p>
    <div v-if="!loading && !error" class="spreadsheet-stage-shell">
    <div ref="stage" class="spreadsheet-stage" role="region" aria-label="表格内容" tabindex="0" @scroll.passive="onScroll">
      <table :aria-label="activeSheet" :aria-rowcount="rowCount + 1" :aria-colcount="columnCount + 1" :style="{ width: `${columnCount * columnWidth + rowHeaderWidth}px`, '--row-header-width': `${rowHeaderWidth}px` }">
        <colgroup>
          <col :style="{ width: `${rowHeaderWidth}px` }" />
          <col v-if="leftWidth" :style="{ width: `${leftWidth}px` }" />
          <col v-for="column in columns" :key="column" />
          <col v-if="rightWidth" :style="{ width: `${rightWidth}px` }" />
        </colgroup>
        <thead><tr aria-rowindex="1">
          <th scope="col" aria-label="行号" aria-colindex="1"></th>
          <th v-if="leftWidth" class="spreadsheet-column-spacer" aria-hidden="true"></th>
          <th v-for="(column, index) in columns" :key="column" scope="col" :aria-colindex="startColumn + index + 2">{{ column }}</th>
          <th v-if="rightWidth" class="spreadsheet-column-spacer" aria-hidden="true"></th>
        </tr></thead>
        <tbody>
          <tr v-if="startRow" class="spreadsheet-spacer" aria-hidden="true"><td :colspan="renderedColumnCount" :style="{ height: `${startRow * ROW_HEIGHT}px` }"></td></tr>
          <tr v-for="row in rows" :key="row.number" :class="{ 'spreadsheet-even': row.number % 2 === 0 }" :aria-rowindex="row.number + 1">
            <th scope="row" aria-colindex="1">{{ row.number }}</th>
            <td v-if="leftWidth" class="spreadsheet-column-spacer" aria-hidden="true"></td>
            <td v-for="(cell, index) in row.cells" :key="columns[index]" :aria-colindex="startColumn + index + 2" :title="cell">{{ cell }}</td>
            <td v-if="rightWidth" class="spreadsheet-column-spacer" aria-hidden="true"></td>
          </tr>
          <tr v-if="endRow < rowCount" class="spreadsheet-spacer" aria-hidden="true"><td :colspan="renderedColumnCount" :style="{ height: `${(rowCount - endRow) * ROW_HEIGHT}px` }"></td></tr>
        </tbody>
      </table>
      <div v-if="!rowCount" class="spreadsheet-state">此工作表没有内容</div>
    </div>
    <div ref="sheetFade" class="spreadsheet-fade" aria-hidden="true"></div>
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
.spreadsheet-stage-shell { position: relative; display: flex; flex: 1; min-height: 0; min-width: 0; }
.spreadsheet-stage { flex: 1; min-width: 0; overflow: auto; overflow-anchor: none; background: var(--bg-surface); }
.spreadsheet-fade { position: absolute; inset: 0; pointer-events: none; opacity: 0; background: var(--bg-surface); }
.spreadsheet-stage:focus-visible { outline: 2px solid var(--border-focus); outline-offset: -2px; }
.spreadsheet-note { flex-shrink: 0; margin: 0; padding: 8px 16px; color: var(--text-secondary); font-size: 12px; }
table { table-layout: fixed; border-collapse: separate; border-spacing: 0; min-width: 100%; font-size: 12px; font-variant-numeric: tabular-nums; }
th, td { box-sizing: border-box; min-width: 96px; max-width: 280px; height: 28px; line-height: 18px; padding: 4px 8px; border-right: 1px solid var(--border-lighter); border-bottom: 1px solid var(--border-lighter); overflow: hidden; white-space: nowrap; text-overflow: ellipsis; text-align: left; }
.spreadsheet-spacer td, .spreadsheet-column-spacer { padding: 0; border: 0; }
thead th { position: sticky; top: 0; z-index: 2; background: var(--bg-elevated); color: var(--text-secondary); text-align: center; }
tbody th { position: sticky; left: 0; z-index: 1; min-width: var(--row-header-width); max-width: var(--row-header-width); background: var(--bg-elevated); color: var(--text-tertiary); text-align: center; }
thead th:first-child { left: 0; z-index: 3; min-width: var(--row-header-width); max-width: var(--row-header-width); }
.spreadsheet-even td { background: color-mix(in srgb, var(--bg-hover) 45%, transparent); }
@media (max-width: 600px) { .spreadsheet-toolbar, .spreadsheet-controls { flex-wrap: wrap; } }
</style>
