const statusLabels = { queued: '排队中', downloading: '下载中', paused: '已暂停', completed: '已完成', failed: '失败', canceled: '已取消' }
const stageLabels = { resolving: '获取下载链接', connecting: '建立连接', scanning: '扫描目录' }

export const downStatusText = status => statusLabels[status] || status

export function downTaskStatus(task) {
  if (task.status !== 'downloading') return downStatusText(task.status)
  if (task.stage === 'verifying') return `校验内容 ${task.stageProgress || 0}%`
  return stageLabels[task.stage] || downStatusText(task.status)
}

export const downProgress = task => Number.isFinite(task.progress) ? Math.max(0, Math.min(100, Math.round(task.progress))) : 0
