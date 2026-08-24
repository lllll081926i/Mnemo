import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'

const VIRTUAL_THRESHOLD = 200
const LIST_ROW_PITCH = 48
const VIRTUAL_OVERSCAN = 10

export function useVirtualFileList({
  files,
  getLoadSequence,
  loading,
  rowsShown,
  viewKey,
  viewMode,
  visibleFiles,
}) {
  const listEl = ref(null)
  const virtualScrollTop = ref(0)
  const virtualViewportHeight = ref(0)
  const gridColumnCount = ref(1)
  const gridRowPitch = ref(166)
  const scrollMemory = new Map()
  let scrollSaveFrame = 0
  let pendingScrollSequence = 0
  let resizeObserver = null

  function updateMetrics(element = listEl.value) {
    if (!element) return
    virtualViewportHeight.value = element.clientHeight || 0
    if (!element.classList.contains('gridlist')) return
    const template = getComputedStyle(element).gridTemplateColumns || ''
    const tracks = template.match(/(?:^|\s)[\d.]+px(?=\s|$)/g)
    gridColumnCount.value = Math.max(1, tracks?.length || Math.floor(Math.max(0, element.clientWidth - 24) / 140) || 1)
    const card = element.querySelector('.griditem')
    if (card) {
      const gap = Number.parseFloat(getComputedStyle(element).rowGap || '8') || 8
      gridRowPitch.value = Math.max(1, card.getBoundingClientRect().height + gap)
    }
  }

  function onScroll(event) {
    const element = event?.currentTarget || listEl.value
    if (element) virtualScrollTop.value = element.scrollTop
    if (scrollSaveFrame) return
    scrollSaveFrame = requestAnimationFrame(() => {
      scrollSaveFrame = 0
      if (listEl.value) scrollMemory.set(viewKey(), listEl.value.scrollTop)
    })
  }

  function markLoad(sequence) {
    pendingScrollSequence = sequence
  }

  function cancelPendingRestore() {
    pendingScrollSequence = 0
  }

  function resetScroll() {
    virtualScrollTop.value = 0
  }

  function virtualRange(total, pitch) {
    if (!total) return { start: 0, end: 0 }
    const viewport = Math.max(virtualViewportHeight.value, pitch * 8)
    const start = Math.max(0, Math.floor(Math.max(0, virtualScrollTop.value) / pitch) - VIRTUAL_OVERSCAN)
    const end = Math.min(total, Math.max(start + 1, Math.ceil((virtualScrollTop.value + viewport) / pitch) + VIRTUAL_OVERSCAN))
    return { start, end }
  }

  const listVirtualized = computed(() => viewMode.value === 'list' && visibleFiles.value.length > VIRTUAL_THRESHOLD)
  const gridVirtualized = computed(() => viewMode.value === 'grid' && visibleFiles.value.length > VIRTUAL_THRESHOLD)
  const listWindow = computed(() => listVirtualized.value
    ? virtualRange(visibleFiles.value.length, LIST_ROW_PITCH)
    : { start: 0, end: visibleFiles.value.length })
  const gridWindow = computed(() => {
    const total = visibleFiles.value.length
    const columns = Math.max(1, gridColumnCount.value)
    if (!gridVirtualized.value) return { start: 0, end: total }
    const rows = virtualRange(Math.ceil(total / columns), gridRowPitch.value)
    return { start: rows.start * columns, end: Math.min(total, rows.end * columns) }
  })
  const listRenderRows = computed(() => rowsShown.value.slice(listWindow.value.start, listWindow.value.end))
  const gridRenderRows = computed(() => rowsShown.value.slice(gridWindow.value.start, gridWindow.value.end))
  const listVirtualTop = computed(() => listWindow.value.start * LIST_ROW_PITCH)
  const listVirtualBottom = computed(() => Math.max(0, (visibleFiles.value.length - listWindow.value.end) * LIST_ROW_PITCH))
  const gridVirtualTop = computed(() => Math.floor(gridWindow.value.start / Math.max(1, gridColumnCount.value)) * gridRowPitch.value)
  const gridVirtualBottom = computed(() => {
    const columns = Math.max(1, gridColumnCount.value)
    const totalRows = Math.ceil(visibleFiles.value.length / columns)
    const renderedRows = Math.ceil((gridWindow.value.end - gridWindow.value.start) / columns)
    return Math.max(0, (totalRows - Math.floor(gridWindow.value.start / columns) - renderedRows) * gridRowPitch.value)
  })

  function reveal(index) {
    const element = listEl.value
    if (!element || index < 0) return
    const grid = viewMode.value === 'grid'
    const virtualized = grid ? gridVirtualized.value : listVirtualized.value
    if (virtualized) {
      const row = grid ? Math.floor(index / Math.max(1, gridColumnCount.value)) : index
      const pitch = grid ? gridRowPitch.value : LIST_ROW_PITCH
      const top = row * pitch
      if (top < element.scrollTop) element.scrollTop = top
      else if (top + pitch > element.scrollTop + element.clientHeight) element.scrollTop = Math.max(0, top + pitch - element.clientHeight)
      virtualScrollTop.value = element.scrollTop
    }
    nextTick(() => element.querySelector('.fileitem.focus, .griditem.focus')?.scrollIntoView({ block: 'nearest' }))
  }

  watch([loading, files], () => {
    if (loading.value || !pendingScrollSequence) return
    const sequence = pendingScrollSequence
    nextTick(() => {
      if (sequence !== getLoadSequence()) return
      pendingScrollSequence = 0
      if (!listEl.value) return
      const scrollTop = scrollMemory.get(viewKey()) || 0
      listEl.value.scrollTop = scrollTop
      virtualScrollTop.value = scrollTop
      updateMetrics()
    })
  })

  watch(listEl, (element) => {
    resizeObserver?.disconnect()
    resizeObserver = null
    if (!element) return
    nextTick(() => {
      if (listEl.value !== element) return
      updateMetrics(element)
      if (typeof ResizeObserver === 'undefined') return
      resizeObserver = new ResizeObserver(() => updateMetrics(element))
      resizeObserver.observe(element)
    })
  }, { flush: 'post' })

  watch([visibleFiles, viewMode], () => nextTick(() => updateMetrics()), { flush: 'post' })

  onBeforeUnmount(() => {
    resizeObserver?.disconnect()
    if (scrollSaveFrame) cancelAnimationFrame(scrollSaveFrame)
  })

  return {
    cancelPendingRestore,
    gridRenderRows,
    gridVirtualBottom,
    gridVirtualTop,
    gridVirtualized,
    listEl,
    listRenderRows,
    listVirtualBottom,
    listVirtualTop,
    listVirtualized,
    markLoad,
    onScroll,
    resetScroll,
    reveal,
  }
}
