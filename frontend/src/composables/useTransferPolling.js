import { onActivated, onBeforeUnmount, onDeactivated, onMounted } from 'vue'
import { EventsOn } from '../api'

export function useTransferPolling({
  menu,
  activeDownloads,
  activeUploads,
  hasPendingOffline,
  onTransferEvent,
  onMigrate,
  refresh,
  refreshMigrateJobs,
  refreshOffline,
  onDeactivate,
  onDispose,
}) {
  let pollTimer = null
  let offlineTimer = null
  let eventDisposers = []
  let viewActive = false

  function bindEvents() {
    if (eventDisposers.length) return
    const transferOff = EventsOn('transfer:event', onTransferEvent)
    const migrateOff = EventsOn('migrate:progress', onMigrate)
    if (typeof transferOff === 'function') eventDisposers.push(transferOff)
    if (typeof migrateOff === 'function') eventDisposers.push(migrateOff)
  }

  function unbindEvents() {
    eventDisposers.forEach((dispose) => {
      try { dispose() } catch { /* 忽略 */ }
    })
    eventDisposers = []
  }

  function stopPolling() {
    clearTimeout(pollTimer)
    clearTimeout(offlineTimer)
    pollTimer = null
    offlineTimer = null
  }

  function startPolling() {
    stopPolling()
    const pollTransfers = async () => {
      if (!viewActive) return
      const hasActiveTransfers = activeDownloads.value.length || activeUploads.value.length
      if (!document.hidden && (hasActiveTransfers || menu.value === 'migrate')) {
        await refresh()
        if (menu.value === 'migrate') await refreshMigrateJobs()
      }
      pollTimer = setTimeout(
        pollTransfers,
        hasActiveTransfers || menu.value === 'migrate' ? 5000 : 15000
      )
    }
    const pollOffline = async () => {
      if (!viewActive) return
      const shouldQueryRemote = !document.hidden && menu.value === 'offline' && hasPendingOffline.value
      // 已完成、失败或取消的任务不会被持续轮询；用户手动刷新仍会强制获取云端状态。
      if (shouldQueryRemote) await refreshOffline({ remote: true })
      offlineTimer = setTimeout(pollOffline, shouldQueryRemote ? 12000 : 30000)
    }
    pollTimer = setTimeout(pollTransfers, 5000)
    offlineTimer = setTimeout(pollOffline, 8000)
  }

  function activateView() {
    if (viewActive) return
    viewActive = true
    bindEvents()
    refresh()
    refreshMigrateJobs()
    startPolling()
  }

  function deactivateView() {
    if (!viewActive) return
    viewActive = false
    stopPolling()
    onDeactivate()
    unbindEvents()
  }

  function isViewActive() {
    return viewActive
  }

  onMounted(activateView)
  onActivated(activateView)
  onDeactivated(deactivateView)
  onBeforeUnmount(() => {
    deactivateView()
    onDispose()
  })

  return {
    startPolling,
    stopPolling,
    isViewActive,
  }
}
