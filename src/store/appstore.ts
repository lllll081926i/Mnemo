import { onHideRightMenu } from '../utils/keyboardhelper'
import { defineStore } from 'pinia'
import { IAliGetFileModel } from '../aliapi/alimodels'

export interface IPageOffice {
  user_id: string
  drive_id: string
  file_id: string
  file_name: string
  preview_url: string
  access_token: string
}

export interface IPagePdf {
  user_id: string
  drive_id: string
  file_id: string
  file_name: string
  preview_url: string
}

export interface IPageDocx {
  user_id: string
  drive_id: string
  file_id: string
  file_name: string
  preview_url: string
}

export interface IPageSheet {
  user_id: string
  drive_id: string
  file_id: string
  file_name: string
  preview_url: string
}

export interface IPageCode {
  user_id: string
  drive_id: string
  file_id: string
  file_name: string
  file_size: number
  code_ext: string
  encType: string
  password: string
}

export interface IPageImage {
  user_id: string
  drive_id: string
  file_id: string
  file_name: string
  mode: string
  password: string
  imageList: IAliGetFileModel[]
}

export interface IPageVideoXBT {
  user_id: string
  drive_id: string
  file_id: string
  file_name: string
}

export interface IPageVideoPlaylistEntry {
  user_id: string
  drive_id: string
  file_id: string
  parent_file_id: string
  file_name: string
  html: string
  ext?: string
  description?: string
  play_cursor?: number
  password?: string
  encType?: string
}

export interface IPageVideo {
  user_id: string
  drive_id: string
  file_id: string
  parent_file_id: string
  parent_file_name: string
  file_name: string
  html: string
  encType: string
  password: string
  expire_time: number
  play_cursor: number
  play_esposide?: number
  custom_playlist_label?: string
  custom_playlist?: IPageVideoPlaylistEntry[]
}

export interface IPageMusicTrack {
  user_id: string
  drive_id: string
  file_id: string
  parent_file_id: string
  file_name: string
  ext?: string
  size?: number
  category?: string
  icon?: string
  thumbnail?: string
  description?: string
  duration_ms?: number
  encType?: string
  password?: string
  local_url?: string
}

export interface IPageMusic {
  user_id: string
  drive_id: string
  file_id: string
  parent_file_id: string
  parent_file_name: string
  file_name: string
  encType: string
  password: string
  playlist: IPageMusicTrack[]
}

export interface AppState {
  appTheme: string

  appPage: string

  appTab: string

  lastMainTab: string

  appTabMenuMap: Map<string, string>
  appDark: boolean
  appShutDown: boolean

  pageOffice?: IPageOffice
  pagePdf?: IPagePdf
  pageDocx?: IPageDocx
  pageSheet?: IPageSheet
  pageCode?: IPageCode
  pageImage?: IPageImage
  pageVideoXBT?: IPageVideoXBT
  pageVideo?: IPageVideo
  pageMusic?: IPageMusic
}

const useAppStore = defineStore('app', {
  state: (): AppState => ({
    appTheme: 'light',
    appPage: 'PageLoading',
    appTab: 'pan',
    lastMainTab: 'pan',
    appTabMenuMap: new Map<string, string>([
      ['pan', 'wangpan'],
      ['down', 'DowningRight'],
      ['share', 'MyShareRight'],
      ['setting', 'general']
    ]),
    appDark: false,
    appShutDown: false
  }),

  getters: {
    GetAppTabMenu(state: AppState): string {
      return state.appTabMenuMap.get(state.appTab)!
    }
  },

  actions: {
    updateStore(partial: Partial<AppState>) {
      this.$patch(partial)
    },

    toggleTheme(theme: string) {
      // console.log('toggleTheme', theme, this)
      this.appTheme = theme
      const isDark = this.appTheme == 'dark' || (this.appTheme == 'system' && this.appDark)
      if (isDark) {
        document.body.setAttribute('arco-theme', 'dark')
        document.documentElement.classList.add('dark')
      } else {
        document.body.removeAttribute('arco-theme')
        document.documentElement.classList.remove('dark')
      }
    },

    toggleDark(dark: boolean) {
      console.log('toggleDark', dark, this)
      this.appDark = dark
      const isDark = this.appTheme == 'dark' || (this.appTheme == 'system' && dark)
      if (isDark) {
        document.body.setAttribute('arco-theme', 'dark')
        document.documentElement.classList.add('dark')
      } else {
        document.body.removeAttribute('arco-theme')
        document.documentElement.classList.remove('dark')
      }
    },

    togglePage(page: string) {
      if (page == this.appPage) return
      this.appPage = page
    },
    resetTab(defaultTab = 'pan') {
      const safeDefaultTab = ['pan', 'down', 'share', 'setting'].includes(defaultTab) ? defaultTab : 'pan'
      const mainTab = ['pan', 'down', 'share'].includes(defaultTab) ? defaultTab : 'pan'
      this.$patch({
        appTab: safeDefaultTab,
        lastMainTab: mainTab,
        appTabMenuMap: new Map<string, string>([
          ['pan', 'wangpan'],
          ['down', 'DowningRight'],
          ['share', 'MyShareRight'],
          ['setting', 'general']
        ])
      })
    },

    openSettings() {
      if (this.appTab === 'setting') return
      if (['pan', 'down', 'share'].includes(this.appTab)) this.lastMainTab = this.appTab
      this.appTab = 'setting'
      onHideRightMenu()
    },

    closeSettings() {
      this.appTab = ['pan', 'down', 'share'].includes(this.lastMainTab) ? this.lastMainTab : 'pan'
      onHideRightMenu()
    },

    toggleTab(tab: string) {
      if (tab === 'setting') {
        if (this.appTab === 'setting') {
          this.closeSettings()
          return
        }

        this.openSettings()
        return
      }

      if (this.appTab != tab) {
        this.appTab = tab
        if (['pan', 'down', 'share'].includes(tab)) this.lastMainTab = tab
        onHideRightMenu()
      }
    },

    toggleTabMenu(tab: string, menu: string) {
      if (this.appTab != tab) {
        if (tab === 'setting') this.openSettings()
        else {
          this.appTab = tab
          if (['pan', 'down', 'share'].includes(tab)) this.lastMainTab = tab
        }
      }
      // Map.set 不会触发 Pinia 响应式，必须替换引用
      const nextMap = new Map(this.appTabMenuMap)
      nextMap.set(tab, menu)
      this.appTabMenuMap = nextMap
      onHideRightMenu()
    },

    toggleTabSetting(tab: string, menu: string) {
      if (tab == this.appTab && this.appTabMenuMap.get(tab) == menu) return
      if (this.appTab != tab) {
        if (tab === 'setting') this.openSettings()
        else {
          this.appTab = tab
          if (['pan', 'down', 'share'].includes(tab)) this.lastMainTab = tab
        }
      }
      if (menu) {
        const nextMap = new Map(this.appTabMenuMap)
        nextMap.set(tab, menu)
        this.appTabMenuMap = nextMap
      }
    },

    toggleTabNext() {
      switch (this.appTab) {
        case 'pan':
          this.appTab = 'down'
          break
        case 'down':
          this.appTab = 'share'
          break
        case 'share':
          this.openSettings()
          break
        case 'setting':
          this.closeSettings()
          return
        default:
          this.appTab = 'pan'
          break
      }
      onHideRightMenu()
    },

    toggleTabNextMenu() {
      const next = (tab: string, menuList: string[]) => {
        const menu = this.appTabMenuMap.get(tab)!
        const nextMap = new Map(this.appTabMenuMap)
        for (let i = 0, maxi = menuList.length; i < maxi; i++) {
          if (menuList[i] == menu) {
            if (i + 1 >= menuList.length) nextMap.set(tab, menuList[0])
            else nextMap.set(tab, menuList[i + 1])
            break
          }
        }
        this.appTabMenuMap = nextMap
      }

      switch (this.appTab) {
        case 'pan': {
          next(this.appTab, ['wangpan', 'kuaijie'])
          break
        }
        case 'down': {
          next(this.appTab, ['DowningRight', 'DownedRight', 'UploadingRight', 'UploadedRight'])
          break
        }
        case 'share': {
          next(this.appTab, ['OtherShareRight', 'ShareHistoryRight', 'MyShareRight'])
          break
        }
        case 'setting': {
          next(this.appTab, ['general', 'account-security', 'files-playback', 'transfer', 'advanced'])
          break
        }
      }

      onHideRightMenu()
    }
  }
})

export default useAppStore
