import fs from 'fs'
import path from 'path'
import { describe, expect, it } from 'vitest'

const root = process.cwd()
const read = (rel: string) => fs.readFileSync(path.join(root, rel), 'utf8')

describe('deep layout shell port', () => {
  it('main shell only exposes pan/down/share/setting tabs', () => {
    const page = read('src/layout/PageMain.vue')
    const panes = [...page.matchAll(/a-tab-pane key=["']([^"']+)["']/g)].map((m) => m[1])
    expect(panes).toEqual(['pan', 'down', 'share', 'setting'])
    expect(page).not.toContain("key: 'media-server'")
    expect(page).not.toContain("key: 'rss'")
    expect(page).toMatch(/class=["']title["']>Mnemo</)
    expect(page).not.toMatch(/class=["']head-divider["']/)
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
    expect(css).toContain('#xbyhead')
    expect(css).toContain('--layout-header-height: 48px')
    expect(css).toContain('height: var(--layout-header-height)')
    expect(css).toContain('--layout-footer-height: 0px')
    expect(css).not.toContain('#xbyfoot')
    expect(css).toContain('--layout-rail-width: 220px')
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
    expect(css).toContain('#xbyhead2 .arco-menu-horizontal .arco-menu-inner')
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
    expect(setting).toContain('min-width: var(--layout-content-width)')
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
    expect(setting).toContain("body:not([arco-theme='dark']) .settings-sidebar")
    expect(setting).toContain('background: #fff')
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
    const fileApi = read('src/aliapi/file.ts')
    const player = read('src/utils/playerhelper.ts')

    expect(video).toContain('needsProviderProxy')
    expect(video).toContain('isLocalProxyUrl(url)')
    expect(music).toContain('proxy_headers: d.headers ? JSON.stringify(d.headers) : undefined')
    expect(fileApi).toContain("proxy_kind: 'subtitle'")
    expect(player).toContain('findBestSubtitleMatch')
    expect(player).toContain("proxy_kind: 'subtitle'")
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

  it('keeps Box in the shared login provider flow', () => {
    const login = read('src/user/UserLogin.vue')
    expect(login).toContain("'dropbox', 'onedrive', 'box', 'webdav', 's3'")
    expect(login).toContain("loginProvider.value === 'box'")
    expect(login).toContain("loginProvider === 'box'")
    expect(login).toContain('createBoxPkceVerifier')
    expect(login).toContain('buildBoxAuthUrl')
    expect(login).toContain('exchangeBoxCodeForToken')
    expect(login).toContain('submitBoxCode(code)')
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
    expect(page).not.toContain('id="xbyfoot"')
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
    expect(windowSource).toContain('handleWebView(AppWindow.readerWindow, DEBUGGING)')
    expect(loginSource).toContain('if (import.meta.env.DEV)')
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
  })
})
