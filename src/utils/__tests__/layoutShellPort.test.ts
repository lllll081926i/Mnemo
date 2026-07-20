import fs from 'fs'
import path from 'path'
import { describe, expect, it } from 'vitest'

const root = process.cwd()
// Normalize CRLF so multi-line toContain expectations pass on Windows CI checkouts.
const read = (rel: string) => fs.readFileSync(path.join(root, rel), 'utf8').replace(/\r\n/g, '\n')

describe('deep layout shell port', () => {
  it('loads the Aliyun Office SDK only inside the Office preview window', () => {
    const html = read('index.html')
    const office = read('src/layout/PageOffice.vue')

    expect(html).not.toContain('aliyun-web-office-sdk.min.js')
    expect(office).toContain("const ALIYUN_OFFICE_SDK_URL = 'https://g.alicdn.com/IMM/office-js/1.1.5/aliyun-web-office-sdk.min.js'")
    expect(office).toContain('loadAliyunOfficeSdk')
    expect(office).toContain('office-preview-state')
  })

  it('loads Prism only for formatted code previews', () => {
    const html = read('index.html')
    const code = read('src/layout/PageCode.vue')

    expect(html).not.toContain("src='/prism.js'")
    expect(html).not.toContain("href='/prism-vsc-dark-plus.css'")
    expect(code).toContain("const PRISM_SCRIPT_URL = './prism.js'")
    expect(code).toContain('loadPrism')
    expect(code).toContain('format.value = !noformat')
    expect(code).toContain('await nextTick()')
  })

  it('loads pinyin search support only for the main drive window', () => {
    const html = read('index.html')
    const mainRuntime = read('src/layout/PageMain.ts')
    const utils = read('src/utils/utils.ts')

    expect(html).not.toContain("src='/pinyinlite_full.min.js'")
    expect(mainRuntime).toContain('await LoadPinyinLite()')
    expect(utils).toContain("const PINYIN_LITE_URL = './pinyinlite_full.min.js'")
    expect(utils).toContain("if (typeof pinyinlite !== 'function') return input.toLowerCase()")
  })

  it('main shell only exposes pan/down/share/setting tabs', () => {
    const page = read('src/layout/PageMain.vue')
    const panes = [...page.matchAll(/a-tab-pane key=["']([^"']+)["']/g)].map((m) => m[1])
    expect(panes).toEqual(['pan', 'down', 'share', 'setting'])
    expect(page).toContain("const Down = defineAsyncComponent(() => import('../down/index.vue'))")
    expect(page).toContain("const Share = defineAsyncComponent(() => import('../share/index.vue'))")
    expect(page).toContain("const Setting = defineAsyncComponent(() => import('../setting/index.vue'))")
    expect(page).toContain('lazy-load')
    expect(page).not.toContain("key: 'media-server'")
    expect(page).not.toContain("key: 'rss'")
    expect(page).toMatch(/class=["']title["']>Mnemo</)
    expect(page).not.toMatch(/class=["']head-divider["']/)
    expect(page).toContain('workspace-page-enter 160ms ease-out')
    expect(page).toContain('@media (prefers-reduced-motion: reduce)')
    const userInfo = read('src/user/UserInfo.vue')
    expect(userInfo).toContain('current-drive-trigger')
    expect(userInfo).toContain('current-drive-avatar')
    expect(userInfo).toContain('activeAccountName')
    expect(userInfo).toContain('activeQuotaText')
    expect(userInfo).toContain('current-drive-panel')
    expect(userInfo).toContain('activeQuotaStats')
    expect(userInfo).toContain('该网盘暂未返回容量信息')
    expect(userInfo).not.toContain('切换到其他账号')
    expect(userInfo).not.toContain('三方APP权益购买')
    expect(userInfo).not.toContain('const activeProvider =')
    expect(userInfo).not.toContain('v-model:active-key="activeProvider"')
  })

  it('layout css owns shared header and rail tokens without a footer reservation', () => {
    const css = read('src/assets/layout-refactor.css')
    expect(css).toContain('#mnemohead')
    expect(css).toContain('--layout-header-height: 48px')
    expect(css).toContain('height: var(--layout-header-height)')
    expect(css).toContain('--layout-footer-height: 0px')
    expect(css).not.toContain('#mnemofoot')
    expect(css).toContain('--layout-rail-width: 220px')
  })

  it('keeps the full titlebar draggable while interactive controls remain clickable', () => {
    const css = read('src/assets/global.css')
    expect(css).toContain('#mnemohead,\n.q-electron-drag')
    expect(css).toContain('-webkit-app-region: drag')
    expect(css).toContain('.q-electron-no-drag')
    expect(css).toContain('#mnemohead button')
    expect(css).toContain('#mnemohead [role=\'button\']')
    expect(css).not.toContain('.q-electron-drag ul,')
    expect(css).not.toContain('.q-electron-drag li,')
  })

  it('pan left has drive switcher and recalculated tree height', () => {
    const pan = read('src/pan/PanLeft.vue')
    const split = read('src/layout/MySplit.vue')
    const switchTab = read('src/layout/MySwitchTab.vue')
    const css = read('src/assets/layout-refactor.css')
    expect(pan).toContain('pan-drive-switcher')
    expect(pan).toContain('treeViewportRef')
    expect(pan).toContain('loadDriveAccountOptions')
    expect(pan).toContain('添加网盘')
    expect(pan).toContain('UserDAL.UserChange(userId)')
    expect(pan).toContain('登录后查看文件')
    expect(pan).not.toContain('回收站、全盘搜索和文件目录会在登录后显示')
    expect(pan).not.toContain('创建快捷方式(Ctrl 1-9)')
    expect(pan).not.toContain("marginLeft: '-18px'")
    expect(pan).not.toContain('flex-shrink: 0 !important')
    expect(split).toContain('expandedSize')
    expect(split).not.toContain("splitSize.value = '240px'")
    expect(switchTab).toContain('--segment-count')
    expect(switchTab).not.toContain('offsetWidth')
    expect(switchTab).not.toContain('offsetLeft')
    expect(css).toContain('.MySplit.is-resizing .arco-split-pane')
    expect(css).toContain('.pan-left .ant-tree-node-content-wrapper')
    expect(css).toContain('#mnemohead2 .arco-menu-horizontal .arco-menu-inner')
    expect(css).toContain('overflow: hidden !important')
  })

  it('keeps the transfer rail compact while share and setting use the standard shell', () => {
    expect(read('src/down/index.vue')).toContain(':width="192"')
    expect(read('src/down/index.vue')).toContain('--layout-rail-width: 192px')
    expect(read('src/share/index.vue')).toContain(':width="220"')
    expect(read('src/setting/index.vue')).toContain('ui-page-shell')
    expect(read('src/assets/layout-refactor.css')).toContain('--layout-rail-width: 220px')
    expect(read('src/share/index.vue')).not.toContain('ShareBottleFish')
    expect(read('src/setting/index.vue')).not.toContain('SettingAPI')
  })

  it('uses spacing for individual settings and dividers only between setting groups', () => {
    const css = read('src/assets/layout-refactor.css')
    const setting = read('src/setting/index.vue')
    const transfer = read('src/setting/SettingDown.vue')
    const debug = read('src/setting/SettingDebug.vue')
    expect(css).toContain('.ui-plain-list {\n  display: grid;\n  gap: 2px;')
    expect(css).not.toContain('.ui-compact-grid')
    expect(css).not.toContain('.ui-compact-field')
    expect(transfer).not.toContain('ui-compact-grid')
    expect(setting).toContain('min-width: 0')
    expect(setting).toContain('grid-template-columns: var(--layout-rail-width) minmax(0, 1fr)')
    expect(setting).not.toContain('@media (max-width: 760px)')
    expect(debug).not.toContain('服务地址')
    expect(debug).not.toContain('服务端口')
    expect(debug).not.toContain('createProxyServer')
    expect(css).toContain('--layout-row-min-height: 36px')
    expect(css).not.toContain(
      '.ui-plain-row {\n  display: grid;\n  grid-template-columns: minmax(160px, var(--layout-row-label-width)) minmax(0, 1fr);\n  gap: var(--layout-row-gap);\n  align-items: center;\n  min-height: var(--layout-row-min-height);\n  padding: 6px 0;\n  border-bottom'
    )
    expect(setting).toContain('.settings-section + .settings-section')
    expect(setting).toContain('border-top: 1px solid var(--border-light)')
    expect(setting).not.toContain('<h1>设置</h1>')
    expect(setting).not.toContain('<component :is="section.icon"')
    expect(setting).toContain('background: var(--bg-surface)')
  })

  it('writes four-level rotating file logs instead of rendering a log list', () => {
    const component = read('src/setting/SettingLog.vue')
    const logger = read('src/utils/debuglog.ts')
    const settingStore = read('src/setting/settingstore.ts')
    const appStore = read('src/store/appstore.ts')
    const page = read('src/layout/PageMain.vue')
    const stores = read('src/store/index.ts')
    const db = read('src/utils/dbcache.ts')

    expect(component).toContain('debugLogEnabled')
    expect(component).toContain('debugLogLevel')
    expect(component).toContain('debugLogMaxSizeMB')
    expect(component).toContain('日志位置')
    expect(component).not.toContain('运行日志')
    expect(component).not.toContain('logList')
    expect(component).not.toContain('mSaveLogClear')
    expect(logger).toContain("export type DebugLogLevel = 'debug' | 'info' | 'warn' | 'error'")
    expect(logger).toContain('maxSizeMB: 5')
    expect(logger).toContain("danger: 'error'")
    expect(logger).toContain("warning: 'warn'")
    expect(logger).toContain("success: 'info'")
    expect(logger).toContain('unlink(this.logPath)')
    expect(settingStore).toContain("debugLogLevel: 'info'")
    expect(settingStore).toContain('debugLogMaxSizeMB: 5')
    expect(appStore).not.toContain('DebugLog.aLoadFromDB()')
    expect(page).not.toContain('DebugLog.aLoadFromDB()')
    expect(stores).not.toContain('useLogStore')
    expect(db).not.toContain('IStateDebugLog')
    expect(db).not.toContain('saveLog(')
  })

  it('routes built-in media and subtitle requests through provider-aware proxy paths', () => {
    const video = read('src/layout/PageVideo.vue')
    const music = read('src/layout/PageMusic.vue')
    const image = read('src/layout/PageImage.vue')
    const fileApi = read('src/aliapi/file.ts')
    const openFile = read('src/utils/openfile.ts')
    const player = read('src/utils/playerhelper.ts')
    const proxy = read('src/utils/proxyhelper.ts')
    const properties = read('src/pan/topbtns/ShuXingModal.vue')

    expect(video).toContain('proxy_headers: JSON.stringify(item.headers)')
    expect(video).toContain("proxy_kind: 'subtitle'")
    expect(music).toContain('proxy_headers: data.headers ? JSON.stringify(data.headers) : undefined')
    expect(fileApi).toContain("proxy_kind: 'subtitle'")
    expect(player).toContain('findBestSubtitleMatch')
    expect(player).toContain("proxy_kind: 'subtitle'")
    expect(image).toContain("resolveDriveProvider({ userId: pageImage.user_id, driveId: item.drive_id }) !== 'aliyun'")
    expect(image).not.toContain('thirdPartyImageDrives')
    expect(openFile.match(/proxy_headers: rawData\.headers \? JSON\.stringify\(rawData\.headers\) : undefined/g)).toHaveLength(5)
    expect(proxy).toContain("resolvedProxyHeaders = data.headers ? JSON.stringify(data.headers) : ''")
    expect(proxy).toContain('proxy_headers: resolvedProxyHeaders')
    expect(proxy).toContain('buildUpstreamProxyHeaders(clientReq.headers, resolvedProxyHeaders)')
    expect(proxy).not.toContain("console.info('proxy query:")
    expect(properties).toContain('fileInfo.value.drive_id || pantreeStore.drive_id')
    expect(properties).toContain('proxy_headers: data.headers ? JSON.stringify(data.headers) : undefined')
  })

  it('loads streaming and ASS playback runtimes only for the media that requires them', () => {
    const video = read('src/layout/PageVideo.vue')

    expect(video).not.toContain("import HlsJs from 'hls.js'")
    expect(video).not.toContain("import * as dashjs from 'dashjs'")
    expect(video).not.toContain("import artplayerPluginJassub from 'artplayer-plugin-jassub'")
    expect(video).toContain("void import('hls.js')")
    expect(video).toContain("void import('dashjs')")
    expect(video).toContain("import('artplayer-plugin-jassub')")
    expect(video).toContain('function hasAssSubtitle(source: ResolvedVideoSource)')
    expect(video).toContain('await installJassubPlugin(art, source, token)')
    expect(fs.existsSync(path.join(root, 'src/module/video-plugins/artplayer-plugin-libass/src/index.js'))).toBe(false)
  })

  it('does not generate unused AI or legacy service configuration', () => {
    const secretsGenerator = read('scripts/generate-secrets.mjs')
    const config = read('src/config.ts')
    const secretsExample = read('src/secrets.example.ts')
    const envExample = read('.env.example')

    expect(secretsGenerator).not.toContain('MNEMO_AI_API_URL')
    expect(config).not.toContain('MNEMO_AI_API_URL')
    expect(secretsExample).not.toContain('MNEMO_AI_API_URL')
    expect(envExample).not.toContain('TMDB_API_KEY')
    expect(envExample).not.toContain('BOXPLAYER_')
    expect(envExample).not.toContain('SUPABASE_')
  })

  it('keeps active workspace toolbars free of fixed spacer nodes', () => {
    const pages = [
      'src/pan/PanRight.vue',
      'src/down/DownDowning.vue',
      'src/down/DownDowned.vue',
      'src/down/DownUploading.vue',
      'src/down/DownUploaded.vue',
      'src/share/share/MyShareRight.vue',
      'src/share/share/OtherShareRight.vue',
      'src/share/share/ShareHistoryRight.vue'
    ]
    for (const page of pages) {
      const source = read(page)
      expect(source).not.toMatch(/height:\s*(7|9|14)px/)
      expect(source).not.toMatch(/toppanbtns[^>]*height:\s*26px/)
      expect(source).not.toContain('icondian')
      expect(source).not.toMatch(/\n\s*<div><\/div>\n/)
    }
  })

  it('keeps drive action menus on the shared provider capability model', () => {
    const menus = ['src/pan/menus/PanTopbtn.vue', 'src/pan/menus/FileTopbtn.vue', 'src/pan/menus/FileRightMenu.vue', 'src/pan/menus/TrashTopbtn.vue', 'src/pan/menus/TrashRightMenu.vue']
    for (const menu of menus) {
      const source = read(menu)
      expect(source).toContain('useCurrentDriveProvider')
      expect(source).not.toContain('isMountedStorage')
      expect(source).not.toContain('isThirdPartyDrive')
    }
    const rightMenu = read('src/pan/menus/FileRightMenu.vue')
    expect(rightMenu).not.toContain('MediaScanner')
    expect(rightMenu).not.toContain('MusicScanner')
    expect(rightMenu).not.toContain('BookScanner')
    expect(rightMenu).not.toContain('扫描数据')
    expect(rightMenu).not.toContain('AI 重刮削')
  })

  it('appstore only keeps four main tabs', () => {
    const store = read('src/store/appstore.ts')
    expect(store).toContain("['pan', 'wangpan']")
    expect(store).toContain("['setting', 'general']")
    expect(store).toContain('openSettings()')
    expect(store).toContain('closeSettings()')
    expect(store).not.toContain("['rss', 'RssXiMa']")
    expect(store).toContain("classList.add('dark')")
  })

  it('settings has direct close actions and an escape shortcut', () => {
    const setting = read('src/setting/index.vue')
    const page = read('src/layout/PageMain.vue')
    expect(setting).toContain('appStore.closeSettings()')
    expect(setting).toContain('aria-label="关闭设置"')
    expect(page).toContain("event.key === 'Escape' && appStore.appTab === 'setting'")
  })

  it('keeps only retained providers in the shared login flow', () => {
    const login = read('src/user/UserLogin.vue')
    expect(login).toContain("['aliyun', '139', '189', 'guangya', 'pikpak', 'quark', 'onedrive', 'dropbox', 'gdrive', 'gofile', 'webdav', 's3']")
    expect(login).not.toMatch(/cloud123|baidu|drive115|boxClient/i)
  })

  it('keeps provider login configuration and QR rendering inside the app', () => {
    const login = read('src/user/UserLogin.vue')
    expect(login).not.toContain('v-model="cloud123AppId"')
    expect(login).not.toContain('v-model="cloud123AppSecret"')
    expect(login).not.toContain('v-model="baiduAppId"')
    expect(login).not.toContain('v-model="baiduAppSecret"')
    expect(login).not.toContain('v-model="dropboxAppKey"')
    expect(login).not.toContain('v-model="onedriveClientId"')
    expect(login).not.toContain('v-model="boxClientId"')
    expect(login).not.toContain('v-model="pikpakClientId"')
    expect(login).not.toContain('v-model="guangyaClientId"')
    expect(login).toContain('PikPak 邮箱 / 手机号 / 用户名')
    expect(login).toContain('手机号，例如 +86 13800138000')
    expect(login).toContain('<a-image width="250"')
    expect(login).toContain('AntQRCode v-if="quarkQrUrl"')
    expect(login).toContain('class="qrcodeframe quark-qrcodeframe"')
    expect(login).toContain('.quark-qrcodeframe')
    expect(login).toContain('align-items: center')
    expect(login).not.toContain('api.qrserver.com')
    expect(login).not.toContain('请先在 src/')
  })

  it('keeps all setting groups in one scrollable document with sidebar anchors', () => {
    const setting = read('src/setting/index.vue')
    expect(setting).toContain('scrollToSection')
    expect(setting).toContain('IntersectionObserver')
    expect(setting).toContain('data-settings-section')
    expect(setting).toContain('v-for="section in visibleSections"')
    expect(setting).toContain("section.key !== 'aliyun' || hasAliyunAccount")
    expect(setting).toContain('scroll-margin-top: 12px')
    expect(setting).toContain('.settings-section + .settings-section')
    expect(setting).toContain('border-top: 1px solid var(--border-light)')
    expect(setting).not.toContain('SettingWebDav')
    expect(setting).not.toContain('SettingS3')
    expect(read('src/setting/SettingAliyun.vue')).toContain('uiShowPanRootFirst')
    expect(read('src/setting/SettingAliyun.vue')).toContain('copyCookies')
    expect(read('src/setting/SettingPan.vue')).not.toContain('value="backup"')
    expect(read('src/setting/SettingAccount.vue')).not.toContain('aliyundrive.com')
  })

  it('does not expose removed Aria or BT controls in the active transfer settings', () => {
    const setting = read('src/setting/index.vue')
    expect(setting).not.toContain('SettingAria')
    expect(setting).not.toContain('SettingDownloadAdvanced')
  })

  it('does not reserve a global footer or workspace title rows', () => {
    const page = read('src/layout/PageMain.vue')
    const pan = read('src/pan/PanRight.vue')
    const down = read('src/down/index.vue')
    expect(page).not.toContain('<a-layout-footer')
    expect(page).not.toContain('id="mnemofoot"')
    expect(pan).not.toContain('DirTopPath')
    expect(down).not.toContain('content-header')
    expect(down).not.toContain('activeTitle')
  })

  it('keeps developer tools unavailable in packaged windows and login webviews', () => {
    const windowSource = read('electron/main/core/window.ts')
    const loginSource = read('src/user/UserLogin.vue')

    expect(windowSource).toContain('devTools: DEBUGGING && devTools')
    expect(windowSource).toContain('devTools: DEBUGGING')
    expect(windowSource).toContain('handleWebView(win, allowDevTools)')
    expect(windowSource).toContain('if (allowDevTools) {\n    registerDevToolsShortcut(win.webContents)')
    expect(windowSource).toContain('disableDevTools(win.webContents)')
    expect(windowSource).toContain('disableDevTools(webContent)')
    expect(windowSource).toContain('if (!allowDevTools) webPreferences.devTools = false')
    expect(windowSource).not.toContain('readerWindow')
    expect(loginSource).not.toContain('webview.openDevTools')
    expect(loginSource).toContain('cleanupAliyunLoginWebview()')
    expect(loginSource).not.toContain('webview.stop()')
    expect(loginSource).not.toContain('unmount-on-close')
  })

  it('keeps background execution only for the hidden transfer workers', () => {
    const launch = read('electron/main/launch.ts')
    const windowSource = read('electron/main/core/window.ts')

    expect(launch).not.toContain("appendSwitch('disable-renderer-backgrounding')")
    expect(windowSource).toContain('backgroundThrottling: boolean = true')
    expect(windowSource).toContain('backgroundThrottling,')
    expect(windowSource).toContain("createElectronWindow(10, 10, false, 'main', 'dark', false, false)")
  })

  it('starts the upload worker only when a message needs it and preserves the first message', () => {
    const windowSource = read('electron/main/core/window.ts')
    const rendererEntry = read('src/main.ts')

    expect(windowSource).toContain("ipcMain.on('ensureUploadWorker', () => {")
    expect(windowSource).toContain("ipcMain.on('uploadWorkerReady', (event) => {")
    expect(windowSource).toContain("AppWindow.mainWindow?.webContents.send('clearUploadPort')")
    expect(windowSource).not.toContain('createDownload')
    expect(windowSource).not.toContain('downloadWindow')
    expect(rendererEntry).not.toContain('setDownloadPort')
    expect(rendererEntry).toContain('const pendingUploadMessages: any[] = []')
    expect(rendererEntry).toContain("window.Electron.ipcRenderer.send('ensureUploadWorker')")
    expect(rendererEntry).toContain('flushPendingUploadMessages()')
    expect(rendererEntry).toContain("window.Electron.ipcRenderer.on('clearUploadPort'")
    expect(read('src/workerpage/workercmd.ts')).toContain("window.Electron.ipcRenderer.send('uploadWorkerReady')")
  })

  it('starts the packaged window without global TLS bypasses or remote icon fonts', () => {
    const launch = read('electron/main/launch.ts')
    const windowSource = read('electron/main/core/window.ts')
    const html = read('index.html')
    const treeViews = [
      'src/pan/PanLeft.vue',
      'src/pan/topbtns/ArchiveModal.vue',
      'src/pan/topbtns/RenameMultiModal.vue',
      'src/pan/topbtns/SelectPanDirModal.vue',
      'src/share/share/ShowShareLinkModal.vue'
    ]

    expect(launch).not.toContain('NODE_TLS_REJECT_UNAUTHORIZED')
    expect(launch).not.toContain("appendSwitch('ignore-certificate-errors')")
    expect(launch).not.toContain("appendSwitch('no-sandbox')")
    expect(launch).not.toContain("appendSwitch('disable-site-isolation-trials')")
    expect(launch).not.toContain('OutOfBlinkCors')
    expect(launch.indexOf('createMainWindow()')).toBeLessThan(launch.indexOf("loadCrxExtension(getStaticPath('crx')"))
    expect(launch).toContain('if (!app.isPackaged)')
    expect(windowSource).toContain('webPreferences.webSecurity = true')
    expect(windowSource).toContain('webPreferences.allowRunningInsecureContent = false')
    expect(html).not.toContain('at.alicdn.com')
    expect(read('electron/main/core/ipcEvent.ts')).toContain('dialog.showOpenDialogSync(')
    expect(read('electron/main/core/ipcEvent.ts')).toContain('dialog.showSaveDialogSync(')
    expect(read('electron/main/core/ipcEvent.ts')).toContain("cmdArguments.push('/s', '/f', '/t', '0')")
    expect(read('electron/main/index.ts')).toContain('__mnemoLaunchInstance')
    expect(read('electron/main/core/ipcEvent.ts')).toContain("Symbol.for('mnemo.ipc-events-registered')")
    expect(read('electron/main/core/ipcEvent.ts')).toContain("icon: getStaticPath('icon_256x256.ico')")
    for (const view of treeViews) {
      const source = read(view)
      expect(source).toContain('<IconFont name="iconarrow-right-2-icon" />')
      expect(source).not.toMatch(/<i[^>]*iconfont[^>]*>/)
    }
  })

  it('uses the operating system proxy by default and applies proxy modes explicitly', () => {
    const settingStore = read('src/setting/settingstore.ts')
    const settingPage = read('src/setting/SettingProxy.vue')
    const mainRuntime = read('src/layout/PageMain.ts')
    const ipc = read('electron/main/core/ipcEvent.ts')
    const preload = read('electron/preload/index.ts')
    const launch = read('electron/main/launch.ts')
    const ariaRuntime = read('electron/main/aria/runtime.ts')

    expect(settingStore).toContain("type ProxyType = 'system' | 'none' | 'http' | 'https' | 'socks5' | 'socks5h'")
    expect(settingStore).toContain("proxyType: 'system'")
    expect(settingStore).toContain("defaultValue(val.proxyType, ['system', 'none', 'http', 'https', 'socks5', 'socks5h'])")
    expect(settingStore).not.toContain('proxyUseProxy: boolean')
    expect(settingStore).not.toContain('proxyUseProxy: false')
    expect(settingStore).toContain("Object.hasOwn(val, 'proxyUseProxy')")
    expect(settingStore).toContain("mode: 'system'")
    expect(settingStore).toContain("mode: 'direct'")
    expect(settingStore).toContain("mode: 'fixed_servers'")
    expect(mainRuntime).toContain('await useSettingStore().WebSetProxy()')
    expect(mainRuntime.indexOf('await useSettingStore().WebSetProxy()')).toBeLessThan(mainRuntime.indexOf('aReloadDowning()'))
    expect(settingPage).toContain('<a-option value="system">系统代理</a-option>')
    expect(settingPage).toContain('<a-option value="none">直连</a-option>')
    expect(settingPage).not.toContain('MySwitch')
    expect(settingPage).not.toContain("from 'node:https'")
    expect(preload).toContain("ipcRenderer.invoke('WebSetProxy', data)")
    expect(ipc).toContain("ipcMain.handle('WebSetProxy'")
    expect(ipc).toContain("mode: 'system'")
    expect(ipc).toContain("mode: 'direct'")
    expect(ipc).toContain("mode: 'fixed_servers'")
    expect(ipc).toContain('closeAllConnections()')
    expect(ipc).toContain('resolveProxy(')
    expect(ipc).toContain('syncMotrixApplicationProxy(ariaProxy)')
    expect(ipc).not.toContain('console.log(JSON.stringify(data))')
    expect(launch).not.toContain("appendSwitch('no-proxy-server')")
    expect(launch).not.toContain("appendSwitch('proxy-bypass-list', '*')")
    expect(ariaRuntime).toContain("const config = { 'all-proxy': proxyUrl, 'no-proxy': '' }")
  })

  it('builds Electron from TypeScript sources even when stale JavaScript output exists', () => {
    const vite = read('vite.config.ts')
    expect(vite).toContain("const sourceExtensions = ['.mjs', '.mts', '.ts', '.tsx', '.js', '.jsx', '.json']")
    expect(vite).toContain('extensions: sourceExtensions')
    expect(vite.match(/resolve: \{ alias: sharedAlias, extensions: sourceExtensions \}/g)).toHaveLength(2)
    expect(vite).toContain('strictPort: false')
    expect(vite).toContain('onstart({ reload })')
    expect(vite).toContain('reload()')
    expect(vite).toContain("find: /^crypto$/")
    expect(vite).toContain("path.resolve(__dirname, 'src/utils/rendererCryptoCompat.ts')")
    const cryptoCompat = read('src/utils/rendererCryptoCompat.ts')
    expect(cryptoCompat).toContain("const crypto = avoidParseRequire('node:crypto')")
    expect(cryptoCompat).toContain('export const createHash = crypto.createHash')
    expect(cryptoCompat).not.toContain('crypto.fips')
  })
})
