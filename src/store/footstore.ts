import { defineStore } from 'pinia'
import { ITokenInfo } from '../user/userstore'
import useAppStore from './appstore'

export interface FootState {
  audioUrl: string

  rightWidth: number

  panDirInfo: string

  uploadingInfo: string

  uploadTotalSpeed: string

  downloadingInfo: string

  downloadTotalSpeed: string

  loadingInfo: string

  panSpaceInfo: string

  picSpaceInfo: string

  ariaInfo: string

  updateDownloadProgress: number
}

const useFootStore = defineStore('foot', {
  state: (): FootState => ({
    audioUrl: '',
    rightWidth: 301,
    panDirInfo: '',
    uploadingInfo: '',
    uploadTotalSpeed: '',
    downloadingInfo: '',
    downloadTotalSpeed: '',
    loadingInfo: '',
    panSpaceInfo: '',
    picSpaceInfo: '',
    ariaInfo: '',
    updateDownloadProgress: 0
  }),

  getters: {
    GetSpaceInfo(state: FootState): string {
      if (state.loadingInfo) return ''
      const appTab = useAppStore().appTab
      if (appTab == 'pan') return state.panSpaceInfo
      return ''
    },
    GetInfo(state: FootState): string {
      if (state.audioUrl) return ''
      const appTab = useAppStore().appTab
      const appPage = useAppStore().GetAppTabMenu
      if (appTab == 'pan') return state.panDirInfo
      if (appPage == 'DowningRight') return state.downloadingInfo
      if (appPage == 'UploadingRight') return state.uploadingInfo
      return ''
    }
  },

  actions: {
    updateStore(partial: Partial<FootState>) {
      this.$patch(partial)
    },

    mSaveUploadingInfo(total: number) {
      this.uploadingInfo = total > 1000 ? '前 1000 / ' + total + ' 个' : ''
    },

    mSaveUploadTotalSpeedInfo(title: string) {
      this.uploadTotalSpeed = title
    },

    mSaveDownTotalSpeedInfo(title: string) {
      this.downloadTotalSpeed = title
    },

    mSaveAudioUrl(url: string) {
      this.audioUrl = url
    },

    mSaveLoading(title: string) {
      this.loadingInfo = title
    },

    mSaveAriaInfo(title: string) {
      this.ariaInfo = title
    },

    mSaveUpdateDownloadProgress(progress: number) {
      this.updateDownloadProgress = progress
    },

    mSaveUserInfo(token: ITokenInfo) {
      this.panSpaceInfo = '总空间 ' + token.spaceinfo
    },

    mSaveDirInfo(info: string) {
      this.panDirInfo = info
    }
  }
})

export default useFootStore
