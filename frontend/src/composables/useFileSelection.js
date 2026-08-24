import { computed, ref, watch } from 'vue'

export function useFileSelection(listShown) {
  const selected = ref([])
  const focusId = ref('')
  const rangeSelecting = ref(false)
  const rangeAnchor = ref('')
  const selectedIds = computed(() => new Set(selected.value.map((file) => file.file_id)))

  function isSelected(file) {
    return selectedIds.value.has(file.file_id)
  }

  function selectRange(fromId, toId) {
    const list = listShown.value
    const from = list.findIndex((file) => file.file_id === fromId)
    const to = list.findIndex((file) => file.file_id === toId)
    if (from < 0 || to < 0) return
    const [start, end] = from < to ? [from, to] : [to, from]
    selected.value = list.slice(start, end + 1)
  }

  function toggle(file, event) {
    if (rangeSelecting.value) {
      if (!rangeAnchor.value) {
        rangeAnchor.value = file.file_id
        focusId.value = file.file_id
        return
      }
      selectRange(rangeAnchor.value, file.file_id)
      rangeSelecting.value = false
      rangeAnchor.value = ''
      focusId.value = file.file_id
      return
    }
    if (event?.shiftKey && focusId.value && focusId.value !== file.file_id) {
      selectRange(focusId.value, file.file_id)
      focusId.value = file.file_id
      return
    }
    focusId.value = file.file_id
    if (event?.ctrlKey || event?.metaKey) {
      const index = selected.value.findIndex((item) => item.file_id === file.file_id)
      if (index >= 0) selected.value.splice(index, 1)
      else selected.value.push(file)
    } else {
      selected.value = isSelected(file) && selected.value.length === 1 ? [] : [file]
    }
  }

  function toggleRangeSelecting() {
    rangeSelecting.value = !rangeSelecting.value
    rangeAnchor.value = ''
  }

  const allSelected = computed(() => listShown.value.length > 0 && selected.value.length === listShown.value.length)

  function selectAll() {
    selected.value = [...listShown.value]
  }

  function toggleSelectAll() {
    allSelected.value ? (selected.value = []) : selectAll()
  }

  function invert() {
    selected.value = listShown.value.filter((file) => !isSelected(file))
  }

  watch(listShown, (list) => {
    const visible = new Set(list.map((file) => file.file_id))
    const next = selected.value.filter((file) => visible.has(file.file_id))
    if (next.length !== selected.value.length) selected.value = next
  })

  return {
    allSelected,
    focusId,
    invert,
    isSelected,
    rangeAnchor,
    rangeSelecting,
    selectAll,
    selected,
    toggle,
    toggleRangeSelecting,
    toggleSelectAll,
  }
}
