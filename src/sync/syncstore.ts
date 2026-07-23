import { defineStore } from 'pinia'
import { SyncTask } from './syncmodel'

export interface SyncRunState {
  running: boolean
  phase: string
  currentFile: string
  doneCount: number
  totalCount: number
  transferred: number
  transferredTotal: number
}

export interface SyncLogEntry {
  time: number
  level: 'info' | 'success' | 'warning' | 'error'
  text: string
}

export interface SyncState {
  tasks: SyncTask[]
  loaded: boolean
  runStates: Record<string, SyncRunState>
  logs: Record<string, SyncLogEntry[]>
}

const emptyRunState = (): SyncRunState => ({ running: false, phase: '', currentFile: '', doneCount: 0, totalCount: 0, transferred: 0, transferredTotal: 0 })

const MAX_LOGS = 200

const useSyncStore = defineStore('sync', {
  state: (): SyncState => ({
    tasks: [],
    loaded: false,
    runStates: {},
    logs: {}
  }),
  getters: {
    getRunState: (state: SyncState) => (taskId: string): SyncRunState => state.runStates[taskId] || emptyRunState(),
    getLogs: (state: SyncState) => (taskId: string): SyncLogEntry[] => state.logs[taskId] || []
  },
  actions: {
    mSaveTasks(tasks: SyncTask[]) {
      this.tasks = tasks
      this.loaded = true
    },
    mSaveRunState(taskId: string, patch: Partial<SyncRunState>) {
      this.runStates[taskId] = { ...(this.runStates[taskId] || emptyRunState()), ...patch }
    },
    mLog(taskId: string, level: SyncLogEntry['level'], text: string) {
      const list = this.logs[taskId] || []
      list.push({ time: Date.now(), level, text })
      if (list.length > MAX_LOGS) list.splice(0, list.length - MAX_LOGS)
      this.logs[taskId] = list
    },
    mClearLogs(taskId: string) {
      this.logs[taskId] = []
    },
    mUpdateTask(taskId: string, patch: Partial<SyncTask>) {
      const index = this.tasks.findIndex((item) => item.id === taskId)
      if (index >= 0) this.tasks[index] = { ...this.tasks[index], ...patch } as SyncTask
    }
  }
})

export default useSyncStore
