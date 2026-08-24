import { computed, ref, watch } from 'vue'
import { upName } from './transferTaskUtils'

export function useTransferTaskModel({ downloads, uploads, filterUser }) {
  const taskFilterRaw = ref('')
  const taskFilter = ref('')
  const selectedIds = ref(new Set())
  let taskFilterTimer = null

  watch(taskFilterRaw, (value) => {
    clearTimeout(taskFilterTimer)
    taskFilterTimer = setTimeout(() => {
      taskFilter.value = value
    }, 120)
  })

  const byKeyword = (list, nameOf) => {
    const keyword = taskFilter.value.trim().toLowerCase()
    if (!keyword) return list
    return list.filter((task) =>
      (nameOf(task) || '').toLowerCase().includes(keyword) ||
      String(task.url || '').toLowerCase().includes(keyword)
    )
  }
  const byAccount = (list) => filterUser.value
    ? list.filter((task) => task.user_id === filterUser.value)
    : list
  const byUploadAccount = (list) => filterUser.value
    ? list.filter((task) => task.UserID === filterUser.value || (task.Info && task.Info.user_id === filterUser.value))
    : list

  const activeDownloads = computed(() =>
    byKeyword(byAccount(downloads.value.filter((task) => task.status !== 'completed')), (task) => task.name)
  )
  const doneDownloads = computed(() =>
    byKeyword(byAccount(downloads.value.filter((task) => task.status === 'completed')), (task) => task.name)
  )
  const activeUploads = computed(() =>
    byKeyword(byUploadAccount(uploads.value.filter((task) => task.Upload && !task.Upload.IsCompleted)), upName)
  )
  const doneUploads = computed(() =>
    byKeyword(byUploadAccount(uploads.value.filter((task) => task.Upload && task.Upload.IsCompleted)), upName)
  )

  const selectedTasks = computed(() =>
    activeDownloads.value.filter((task) => selectedIds.value.has(task.id))
  )
  const allActiveSelected = computed(() =>
    activeDownloads.value.length > 0 &&
    activeDownloads.value.every((task) => selectedIds.value.has(task.id))
  )

  function toggleSelectAllActive() {
    selectedIds.value = allActiveSelected.value
      ? new Set()
      : new Set(activeDownloads.value.map((task) => task.id))
  }

  function toggleSelect(id) {
    const next = new Set(selectedIds.value)
    next.has(id) ? next.delete(id) : next.add(id)
    selectedIds.value = next
  }

  function onItemClick(event, id) {
    if (event.ctrlKey || event.metaKey) toggleSelect(id)
    else selectedIds.value = new Set([id])
  }

  function disposeTaskModel() {
    clearTimeout(taskFilterTimer)
    taskFilterTimer = null
  }

  return {
    taskFilterRaw,
    activeDownloads,
    doneDownloads,
    activeUploads,
    doneUploads,
    selectedIds,
    selectedTasks,
    allActiveSelected,
    toggleSelectAllActive,
    toggleSelect,
    onItemClick,
    disposeTaskModel,
  }
}
