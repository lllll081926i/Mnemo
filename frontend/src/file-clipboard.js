import { shallowRef, ref } from 'vue'
import { uniqueFileSelection } from './file-transfer'

export const fileClipboard = shallowRef(null)
export const clipboardBusy = ref(false)

export function setFileClipboard(source, moving) {
  if (clipboardBusy.value || fileClipboard.value?.jobId || source?.mode === 'trash') return false
  const files = uniqueFileSelection(source?.files || []).map(file => ({ ...file,
    parent_file_id: file.parent_file_id || source.dirId, download_url: '', thumbnail: '',
  }))
  if (!source?.account || !files.length) return false
  fileClipboard.value = {
    account: { user_id: source.account.user_id, drive_id: source.account.drive_id },
    label: source.label || '', dirId: source.dirId, path: (source.path || []).map(part => ({ ...part })),
    files, moving: !!moving, mode: source.mode,
  }
  return true
}

export function clearFileClipboard() {
  if (!clipboardBusy.value) fileClipboard.value = null
}

export function retainClipboardFiles(source, completed) {
  if (fileClipboard.value !== source) return
  const remaining = source.files.filter(file => !completed.includes(file.file_id))
  fileClipboard.value = remaining.length ? { ...source, files: remaining } : null
}

export function clipboardPasteSource(source, target) {
  const key = JSON.stringify([target.account.user_id, target.account.drive_id, target.dirId])
  const completed = source.completedByTarget?.[key] || []
  return { ...source, files: source.files.filter(file => !completed.includes(file.file_id)) }
}

export function rememberClipboardCopy(source, target, completed, finished = false) {
  if (fileClipboard.value !== source) return
  const key = JSON.stringify([target.account.user_id, target.account.drive_id, target.dirId])
  const previous = source.completedByTarget?.[key] || []
  const confirmed = [...new Set([...previous, ...completed])]
  const completedByTarget = { ...source.completedByTarget }
  if (finished) delete completedByTarget[key]
  else completedByTarget[key] = confirmed
  fileClipboard.value = { ...source, completedByTarget }
}

export function clipboardMigrationEvent(job) {
  const source = fileClipboard.value
  if (!source?.jobId || source.jobId !== job?.id) return
  if (job.status === 'completed') fileClipboard.value = source.moving ? null : { ...source, jobId: '' }
  else fileClipboard.value = { ...source, jobStatus: job.status }
}
