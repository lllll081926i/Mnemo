import path from 'path'

export type DownloadTaskStatus =
  | 'active' | 'waiting' | 'paused' | 'error' | 'complete' | 'removed' | string

export interface DownloadTaskFile {
  index: number
  path: string
  name: string
  length: number
  completedLength: number
  selected: boolean
}

export interface DownloadTask {
  gid: string
  status: DownloadTaskStatus
  totalLength: number
  completedLength: number
  uploadLength: number
  downloadSpeed: number
  uploadSpeed: number
  connections: number
  numPieces: number
  pieceLength: number
  dir: string
  files: DownloadTaskFile[]
  errorCode: string
  errorMessage: string
}

export interface DownloadGlobalStat {
  downloadSpeed: string
  uploadSpeed: string
  numActive: string
  numWaiting: string
  numStopped: string
  numStoppedTotal: string
}

const toNumber = (value: unknown): number => {
  const parsed = Number(value || 0)
  return Number.isFinite(parsed) ? parsed : 0
}

const toBoolean = (value: unknown): boolean => value === true || value === 'true'

export const normalizeTaskFiles = (files: any[] = []): DownloadTaskFile[] =>
  files.map((file) => ({
    index: toNumber(file.index),
    path: String(file.path || ''),
    name: path.basename(String(file.path || '')),
    length: toNumber(file.length),
    completedLength: toNumber(file.completedLength),
    selected: toBoolean(file.selected)
  }))

export const normalizeAriaTask = (task: any): DownloadTask => ({
  gid: String(task?.gid || ''),
  status: String(task?.status || ''),
  totalLength: toNumber(task?.totalLength),
  completedLength: toNumber(task?.completedLength),
  uploadLength: toNumber(task?.uploadLength),
  downloadSpeed: toNumber(task?.downloadSpeed),
  uploadSpeed: toNumber(task?.uploadSpeed),
  connections: toNumber(task?.connections),
  numPieces: toNumber(task?.numPieces),
  pieceLength: toNumber(task?.pieceLength),
  dir: String(task?.dir || ''),
  files: normalizeTaskFiles(task?.files),
  errorCode: String(task?.errorCode || ''),
  errorMessage: String(task?.errorMessage || '')
})
