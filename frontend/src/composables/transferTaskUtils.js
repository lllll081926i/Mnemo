import { formatBytes, formatSpeed } from '../api'

const DOWNLOAD_STATUS_TEXT = {
  queued: '排队中',
  downloading: '下载中',
  paused: '已暂停',
  completed: '已完成',
  failed: '失败',
  canceled: '已取消',
  uploading: '上传中',
}

const DOWNLOAD_STATUS_BADGE = {
  downloading: 'primary',
  queued: 'primary',
  completed: 'success',
  failed: 'error',
  canceled: 'warn',
  paused: 'warn',
}

export const downStatusText = (status) => DOWNLOAD_STATUS_TEXT[status] || status
export const statusText = downStatusText
export const downStatusBadge = (status) => DOWNLOAD_STATUS_BADGE[status] || ''

export function upStatus(task) {
  const upload = task.Upload || {}
  if (upload.IsCompleted) return '已完成'
  if (upload.IsFailed) return '失败'
  if (upload.IsStop) return '已停止'
  if (upload.IsDowning) return '上传中'
  return '排队中'
}

export const upStatusBadge = (task) => ({ 已完成: 'success', 失败: 'error', 已停止: 'warn' }[upStatus(task)] || 'primary')
export const upName = (task) => (task.Info && task.Info.name) || ((task.Info && task.Info.localFilePath) || '').split(/[\\/]/).pop() || task.UploadID
export const upSize = (task) => (task.Info && (task.Info.sizeStr || formatBytes(task.Info.size))) || ''
export const upSpeed = (task) => (task.Upload && (task.Upload.DownSpeedStr || formatSpeed(task.Upload.DownSpeed || 0))) || ''
export const upErr = (task) => (task.Upload && task.Upload.failedMessage) || ''

export const offProgress = (task) => Math.max(0, Math.min(100, Math.round(task.progress || 0)))
export const migBadge = (status) => ({ completed: 'success', failed: 'error', running: 'primary' }[status] || 'warn')
export const migStatusText = (status) => ({ pending: '等待中', running: '迁移中', completed: '已完成', partial: '部分完成', failed: '失败', canceled: '已取消' }[status] || status)
export const migCompletedTopLevel = (job) => (job.fileIDs || []).filter((id) => (job.completedFileIDs || []).includes(id)).length
export const migRemaining = (job) => Math.max(0, (job.fileIDs || []).length - migCompletedTopLevel(job))
export const migProgress = (job) => job.totalBytes > 0
  ? Math.min(100, Math.round(((job.processedBytes || 0) / job.totalBytes) * 100))
  : (job.total ? Math.min(100, Math.round(((job.processed || 0) / job.total) * 100)) : 0)
export const migProgressText = (job) => job.totalBytes > 0
  ? `${formatBytes(job.processedBytes || 0)} / ${formatBytes(job.totalBytes || 0)}`
  : `${job.processed || 0} / ${job.total || 0} 个文件`
