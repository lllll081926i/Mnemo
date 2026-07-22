export interface AriaProgressErrorInput {
  status: string
  errorCode?: string
  errorMessage?: string
}

export interface DownloadErrorState {
  isFailed: boolean
  failedCode: number
  failedMessage: string
}

export interface DownloadProgressTask {
  Info: { size?: number | string }
  Down: {
    DownSize?: number
    IsCompleted?: boolean
    IsDowning?: boolean
  }
}

export interface RestoredDownloadState {
  DownState: string
  DownSpeed: number
  DownSpeedStr: string
  IsStop: boolean
  IsDowning: boolean
  IsCompleted: boolean
  IsFailed: boolean
  FailedCode: number
  FailedMessage: string
  AutoTry: number
  ManualRetryRequired?: boolean
}

export const resolveRestoredDownloadState = <T extends RestoredDownloadState>(down: T): T => {
  if (down.IsFailed) {
    return {
      ...down,
      IsDowning: false,
      IsCompleted: false,
      DownSpeed: 0,
      DownSpeedStr: '',
      ManualRetryRequired: true
    }
  }
  if (!down.IsStop && down.DownState !== '队列中') {
    return {
      ...down,
      IsDowning: false,
      IsCompleted: false,
      IsStop: false,
      DownState: '队列中',
      DownSpeed: 0,
      DownSpeedStr: '',
      IsFailed: false,
      FailedCode: 0,
      FailedMessage: '',
      AutoTry: 0,
      ManualRetryRequired: false
    }
  }
  return down
}

export const resolveDownloadTaskbarProgress = (tasks: DownloadProgressTask[], enabled = true): number => {
  if (!enabled) return -1
  const active = tasks.filter((task) => task.Down.IsDowning && !task.Down.IsCompleted)
  if (!active.length) return -1
  const totalBytes = active.reduce((sum, task) => sum + (Number(task.Info.size) || 0), 0)
  if (totalBytes <= 0) return -1
  const doneBytes = active.reduce((sum, task) => sum + (Number(task.Down.DownSize) || 0), 0)
  return Math.min(1, Math.max(0, doneBytes / totalBytes))
}

export const resolveAriaProgressErrorState = (
  input: AriaProgressErrorInput,
  formatError: (errorCode: string, errorMessage: string) => string
): DownloadErrorState => {
  if (input.status !== 'error') {
    return {
      isFailed: false,
      failedCode: 0,
      failedMessage: ''
    }
  }

  if (input.errorCode && input.errorCode !== '0') {
    return {
      isFailed: true,
      failedCode: parseInt(input.errorCode) || 0,
      failedMessage: formatError(input.errorCode, input.errorMessage || '')
    }
  }

  return {
    isFailed: true,
    failedCode: 0,
    failedMessage: '下载失败'
  }
}
