import { defineStore } from 'pinia'
import { GetFocusNext, GetSelectedList, KeyboardSelectOne, MouseSelectOne, SelectAll } from '../utils/selecthelper'


export interface IUploadingModel {
  UploadID: number
  TaskID: number

  user_id: string


  localFilePath: string

  name: string

  sizeStr: string
  icon: string
  isDir: boolean

  uploadState: string

  speedStr: string

  Progress: number

  ProgressStr: string

  errorMessage: string
}

type Item = IUploadingModel

export interface UploadingState {

  ListLoading: boolean

  ListDataShow: Item[]

  ListDataRaw: Item[]

  AccountFilter: string


  ListSelected: Set<number>

  ListFocusKey: number

  ListSelectKey: number

  ListDataCount: number

  showTaskID: number

  ShowTaskName: string
}

type State = UploadingState
let KEY: 'UploadID' | 'TaskID' = 'UploadID'

const useUploadingStore = defineStore('uploading', {
  state: (): UploadingState => ({
    ListLoading: false,
    ListDataShow: [],
    ListDataRaw: [],
    AccountFilter: '',
    ListSelected: new Set<number>(),
    ListFocusKey: 0,
    ListSelectKey: 0,
    ListDataCount: 0,
    showTaskID: 0,
    ShowTaskName: ''
  }),

  getters: {
    ListDataUploadingCount(state: State): number {
      return state.ListDataRaw.length
    },

    IsListSelected(state: State): boolean {
      return state.ListSelected.size > 0
    },
    ListSelectedCount(state: State): number {
      return state.ListSelected.size
    },
    ListDataSelectCountInfo(state: State): string {
      return '已选中 ' + state.ListSelected.size + ' / ' + state.ListDataShow.length + ' 个'
    },
    IsListSelectedAll(state: State): boolean {
      return state.ListSelected.size > 0 && state.ListSelected.size == state.ListDataShow.length
    }
  },

  actions: {

    aLoadListData(TaskID: number, TaskName: string, list: Item[], count: number) {
      KEY = TaskID ? 'UploadID' : 'TaskID'
      this.ListDataRaw = list


      if (this.showTaskID == TaskID) {

        const oldSelected = this.ListSelected
        const newSelected = new Set<number>()
        let findFocusKey = false
        let findSelectKey = false
        let key = 0
        let listFocusKey = this.ListFocusKey
        let listSelectKey = this.ListSelectKey
        for (let i = 0, maxi = list.length; i < maxi; i++) {
          key = list[i][KEY]
          if (oldSelected.has(key)) newSelected.add(key)
          if (key == listFocusKey) findFocusKey = true
          if (key == listSelectKey) findSelectKey = true
        }

        if (!findFocusKey) listFocusKey = 0
        if (!findSelectKey) listSelectKey = 0

        this.$patch({ ListSelected: newSelected, ListFocusKey: listFocusKey, ListSelectKey: listSelectKey, ListDataCount: count })
      } else {
        this.$patch({ showTaskID: TaskID, ShowTaskName: TaskName, ListSelected: new Set<string>(), ListFocusKey: 0, ListSelectKey: 0, ListDataCount: count })
      }
      this.mRefreshListDataShow(true)
    },

    mShowTask(TaskID: number, TaskName: string) {
      KEY = TaskID ? 'UploadID' : 'TaskID'
      this.$patch({ showTaskID: TaskID, ShowTaskName: TaskName, ListSelected: new Set<number>(), ListFocusKey: 0, ListSelectKey: 0, ListDataRaw: [], ListDataShow: [] })
    },

    mSetAccountFilter(accountId: string) {
      if (this.AccountFilter == accountId) return
      this.$patch({ AccountFilter: accountId, ListSelected: new Set<number>(), ListFocusKey: 0, ListSelectKey: 0 })
      this.mRefreshListDataShow(true)
    },

    mRefreshListDataShow(refreshRaw: boolean) {
      if (!refreshRaw) {
        const listDataShow = this.ListDataShow.concat()
        Object.freeze(listDataShow)
        this.ListDataShow = listDataShow
        return
      }
      const filterSource = this.AccountFilter ? this.ListDataRaw.filter((item) => item.user_id == this.AccountFilter) : this.ListDataRaw
      const listDataShow = filterSource.concat()
      Object.freeze(listDataShow)
      this.ListDataShow = listDataShow
      const freezeList = this.ListDataShow
      const oldSelected = this.ListSelected
      const newSelected = new Set<number>()
      let key = 0
      for (let i = 0, maxi = freezeList.length; i < maxi; i++) {
        key = freezeList[i][KEY]
        if (oldSelected.has(key)) newSelected.add(key)
      }
      this.ListSelected = newSelected
    },

    mSelectAll() {
      this.$patch({ ListSelected: SelectAll(this.ListDataShow, KEY, this.ListSelected), ListFocusKey: 0, ListSelectKey: 0 })
      this.mRefreshListDataShow(false)
    },

    mMouseSelect(key: number, Ctrl: boolean, Shift: boolean) {
      if (this.ListDataShow.length == 0) return
      const data = MouseSelectOne(this.ListDataShow, KEY, this.ListSelected, this.ListFocusKey, this.ListSelectKey, key, Ctrl, Shift, 0)
      this.$patch({ ListSelected: data.selectedNew, ListFocusKey: data.focusLast, ListSelectKey: data.selectedLast })
      this.mRefreshListDataShow(false)
    },

    mKeyboardSelect(key: number, Ctrl: boolean, Shift: boolean) {
      if (this.ListDataShow.length == 0) return
      const data = KeyboardSelectOne(this.ListDataShow, KEY, this.ListSelected, this.ListFocusKey, this.ListSelectKey, key, Ctrl, Shift, 0)
      this.$patch({ ListSelected: data.selectedNew, ListFocusKey: data.focusLast, ListSelectKey: data.selectedLast })
      this.mRefreshListDataShow(false)
    },

    mRangSelect(lastkey: number, file_idList: number[]) {
      if (this.ListDataShow.length == 0) return
      const selectedNew = new Set<number>(this.ListSelected)
      for (let i = 0, maxi = file_idList.length; i < maxi; i++) {
        selectedNew.add(file_idList[i])
      }
      this.$patch({ ListSelected: selectedNew, ListFocusKey: lastkey, ListSelectKey: lastkey })
      this.mRefreshListDataShow(false)
    },

    GetSelected() {
      return GetSelectedList(this.ListDataShow, KEY, this.ListSelected)
    },

    GetSelectedFirst() {
      const list = GetSelectedList(this.ListDataShow, KEY, this.ListSelected)
      if (list.length > 0) return list[0]
      return undefined
    },

    mSetFocus(key: number) {
      this.ListFocusKey = key
      this.mRefreshListDataShow(false)
    },

    mGetFocus() {
      if (this.ListFocusKey > 0 && this.ListDataShow.length > 0) return this.ListDataShow[0][KEY]
      return this.ListFocusKey
    },

    mGetFocusNext(position: string) {
      return GetFocusNext(this.ListDataShow, KEY, this.ListFocusKey, position, 0)
    },

    mDeleteFiles(idList: number[]) {
      const fileMap = new Set(idList)
      const listDataRaw = this.ListDataRaw
      const newDataList: Item[] = []
      for (let i = 0, maxi = listDataRaw.length; i < maxi; i++) {
        const item = listDataRaw[i]
        if (!fileMap.has(item.UploadID)) {
          newDataList.push(item)
        }
      }
      this.ListDataRaw = newDataList
      this.mRefreshListDataShow(true)
    }
  }
})

export default useUploadingStore
