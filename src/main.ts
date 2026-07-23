import { createApp } from 'vue'
import App from './App.vue'
import ArcoVue from '@arco-design/web-vue'
import store, { useAppStore, useSettingStore } from './store'
import '@arco-themes/vue-gi-demo/css/arco.css'
import message from './utils/message'
import DebugLog from './utils/debuglog'

window.onerror = function (errorMessage, scriptURI, lineNo, columnNo, error) {
  try {
    if (errorMessage
      && typeof errorMessage === 'string') {
      if (errorMessage.indexOf('ResizeObserver') >= 0
        || errorMessage.indexOf('listen EADDRINUSE') >= 0
        || errorMessage.indexOf('connect ENOENT') >= 0) {
        return true
      }
    }
    // DebugLog.mSaveDanger('onerror')
    if (typeof errorMessage === 'string') DebugLog.mSaveDanger(errorMessage)
    if (error) {
      DebugLog.mSaveDanger('onerror', error)
    }
    message.error('当前操作没有完成，请重试；如果问题持续出现，请在设置中打开日志查看原因')
  } catch {}
  return true
}

window.addEventListener('unhandledrejection', function (event) {
  try {
    if (event.reason && event.reason.message && event.reason.message.indexOf('oauth/authorize?') > 0) {
      event.stopPropagation()
      event.preventDefault()
      return
    }

    // DebugLog.mSaveDanger('unhandledrejection')
    const reason = event.reason
    if (reason && reason.message) {
      if (/no supported source/i.test(reason.message)) {
        message.error('不支持当前媒体类型', 1)
        event.stopPropagation()
        event.preventDefault()
        return
      }
      DebugLog.mSaveDanger('unhandledrejection', reason)
      message.error('当前操作没有完成，请重试；如果问题持续出现，请在设置中打开日志查看原因', 1)
    }
    if (!reason) DebugLog.mSaveDanger('unhandledrejection', JSON.stringify(event))
  } catch {}
  event.stopPropagation()
  event.preventDefault()
})

const app = createApp(App)
import IconFont from './components/IconFont.vue'
app.component('IconFont', IconFont)
app.config.errorHandler = function (err: any, vm, info) {
  try {
    if (typeof err === 'string') {
      DebugLog.mSaveDanger('errorHandler', err)
    } else {
      DebugLog.mSaveDanger('errorHandler', err)
    }
    message.error('页面运行出现问题，请重试；如果问题持续出现，请重启应用', 1)
  } catch {}
  return true
}
app.use(ArcoVue, {})
app.use(store)
app.mount('#app')


window.WinMsgToUpload = function (event: any) {
  window.WebUploadPort.send(event)
}

window.WebUploadPort.onMessage((data: any) => {
  Promise.resolve().then(() => {
    try {
      if (window.WinMsg) window.WinMsg(data)
    } catch {}
  })
})

window.WebOnSetPage(async (args: any) => {
  console.log('setPage', args.page, args)
  const appStore = useAppStore()
  const settingStore = useSettingStore()
  if (args.theme && settingStore) appStore.toggleTheme(args.theme)

  if (args.page == 'PageMain') {
    try {
      const { PageMain } = await import('./layout/PageMain')
      PageMain()
    } catch (error) {
      DebugLog.mSaveDanger('PageMainLoad', error)
      return
    }
    window.IsMainPage = true
  } else if (args.page == 'PageCode') {
    appStore.pageCode = args.data
  } else if (args.page == 'PagePdf') {
    appStore.pagePdf = args.data
  } else if (args.page == 'PageImage') {
    appStore.pageImage = args.data
  } else if (args.page == 'PageVideo') {
    appStore.pageVideo = args.data
  } else if (args.page == 'PageMusic') {
    appStore.pageMusic = args.data
  }
  if (args.page) appStore.togglePage(args.page)
})

window.WebOnSetTheme((args: any) => {
  const appStore = useAppStore()
  appStore.toggleDark(args.dark)
})
