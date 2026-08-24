import { onBeforeUnmount } from 'vue'
import { savePlayCursor } from '../api'

export function usePlaybackCursor({ account, currentFileId, videoEl }) {
  let saveTimer = null
  let ended = false

  function clear(fileId = currentFileId()) {
    if (!fileId) return
    savePlayCursor(account().user_id, account().drive_id, fileId, 0).catch(() => {})
  }

  function save(fileId = currentFileId()) {
    if (ended) {
      clear(fileId)
      return
    }
    const video = videoEl.value
    if (!video || !video.currentTime || video.currentTime < 1 || !fileId) return
    savePlayCursor(account().user_id, account().drive_id, fileId, video.currentTime).catch(() => {})
  }

  function start() {
    if (saveTimer) clearInterval(saveTimer)
    saveTimer = setInterval(save, 5000)
  }

  function stop() {
    if (saveTimer) clearInterval(saveTimer)
    saveTimer = null
  }

  function markActive() { ended = false }
  function markEnded() { ended = true }
  function hasEnded() { return ended }

  onBeforeUnmount(() => {
    save()
    stop()
  })

  return { clear, hasEnded, markActive, markEnded, save, start, stop }
}
