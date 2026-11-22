// Run with PLAYWRIGHT_MODULE pointing to an installed playwright module.
// Uses local mock data only; never connects to real cloud accounts.
const { chromium } = require(process.env.PLAYWRIGHT_MODULE || 'playwright')
const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const output = process.env.QA_OUTPUT || path.join(require('node:os').tmpdir(), 'mnemo-workspace-qa')
fs.mkdirSync(output, { recursive: true })

async function assertDialogCorners(page) {
  const offsets = await page.locator('.modal').evaluateAll(panels => panels.map(panel => {
    const close = panel.querySelector('.modal-window-close')
    const bounds = panel.getBoundingClientRect()
    const button = close.getBoundingClientRect()
    return { title: panel.getAttribute('aria-label'), top: button.top - bounds.top, right: bounds.right - button.right }
  }))
  for (const offset of offsets) assert(Math.abs(offset.top) < 1 && Math.abs(offset.right) < 1, `二级弹窗关闭键没有贴角：${JSON.stringify(offset)}`)
}

async function assertKeyboardAccessibility(page) {
  await page.getByTitle('设置 (Alt+5)').click()
  await page.getByRole('combobox', { name: '默认排序', exact: true }).waitFor()
  const duplicateIds = await page.locator('[id]').evaluateAll(elements => {
    const ids = elements.map(element => element.id)
    return ids.filter((id, index) => ids.indexOf(id) !== index)
  })
  assert.deepEqual(duplicateIds, [], '页面控件不能使用重复 ID')
  const unnamed = await page.locator('[role="switch"], input, [role="combobox"]').evaluateAll(elements => elements.filter(element => !element.getAttribute('aria-label') && !element.getAttribute('aria-labelledby')).map(element => element.outerHTML))
  assert.deepEqual(unnamed, [], '设置控件必须有名称')
  const checked = []
  for (const viewport of [{ width: 1024, height: 680 }, { width: 3840, height: 2160 }]) {
    await page.setViewportSize(viewport)
    for (const theme of ['light', 'dark', 'oled']) {
      for (const zoom of [1, 1.25, 1.5]) {
        await page.evaluate(({ theme, zoom }) => {
          document.documentElement.classList.toggle('dark', theme !== 'light')
          document.documentElement.classList.toggle('oled', theme === 'oled')
          document.documentElement.style.zoom = String(zoom)
        }, { theme, zoom })
        const select = page.getByRole('combobox', { name: '默认排序', exact: true })
        await select.focus()
        await select.press('Enter')
        await page.getByRole('listbox', { name: '默认排序', exact: true }).waitFor()
        await select.press('End')
        assert(await select.evaluate(element => document.activeElement === element && !!document.getElementById(element.getAttribute('aria-activedescendant'))), '键盘焦点和活动选项必须保持关联')
        await select.press('Escape')
        await page.getByRole('listbox', { name: '默认排序', exact: true }).waitFor({ state: 'detached' })
        checked.push({ ...viewport, theme, zoom })
      }
    }
  }
  await page.evaluate(() => { document.documentElement.style.zoom = '' })
  await page.setViewportSize({ width: 1024, height: 680 })
  await page.emulateMedia({ forcedColors: 'active', reducedMotion: 'reduce' })
  const select = page.getByRole('combobox', { name: '默认排序', exact: true })
  await select.focus()
  assert(await select.evaluate(element => { const style = getComputedStyle(element); return style.outlineStyle !== 'none' && parseFloat(style.outlineWidth) >= 2 }), '高对比度模式必须保留可见焦点')
  await page.getByRole('button', { name: '同步', exact: true }).click()
  const opener = page.getByTitle('编辑', { exact: true })
  await opener.click()
  const dialog = page.getByRole('dialog', { name: '编辑同步任务', exact: true })
  await dialog.waitFor()
  const choice = dialog.getByRole('combobox').first()
  await choice.focus()
  await choice.press('Enter')
  await page.getByRole('listbox').waitFor()
  await choice.press('Escape')
  await page.getByRole('listbox').waitFor({ state: 'detached' })
  assert(await dialog.isVisible(), '关闭下拉框不能同时关闭弹窗')
  await choice.press('Escape')
  await dialog.waitFor({ state: 'detached' })
  assert(await opener.evaluate(element => document.activeElement === element), '关闭弹窗必须恢复触发按钮焦点')
  return { checked, forcedColors: true, reducedMotion: true }
}

async function main() {
  const browser = await chromium.launch({ channel: 'msedge', headless: true })
  const page = await browser.newPage({ viewport: { width: 1440, height: 1000 } })
  const errors = []
  page.on('pageerror', error => errors.push(error.message))
  await page.addInitScript(({ themeQA, railMotionQA, previewQA, startupQA, shareImportQA, offlineQA, fileScrollQA }) => {
    const accounts = Array.from({length:20}, (_,i) => ({user_id:`webdav:account-${i}`,drive_id:`drive-${i}`,custom_name:`资料库 ${i+1}`,token:{},usage:{size:100000000,used:100000,status:'available'}}))
    if (railMotionQA) accounts[0].auth_state = 'reauth_required'
    accounts.push({user_id:'aliopen_one',drive_id:'ali',custom_name:'阿里资料',token:{}})
    const files = Array.from({length:10000},(_,i) => ({file_id:`file-${i}`,parent_file_id:'root',name:`资料-${String(i).padStart(5,'0')}.txt`,size:1024,time:1700000000,isDir:false}))
    if (fileScrollQA) for (let index = 150; index < files.length; index += 4) files[index].name += ' — 需要在网格里显示两行的项目资料归档文件'
    if (previewQA) {
      files[0].thumbnail = 'data:image/svg+xml,' + encodeURIComponent('<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24"><rect width="24" height="24" fill="navy"/></svg>')
      files[1].thumbnail = 'data:image/png;base64,AA=='
    }
    const folders = [10, 2, 1].map(i => ({file_id:`folder-${i}`, parent_file_id:'root', name:`项目${i}`, isDir:true}))
    const listeners = {}
    window.__qaListenerCount = name => listeners[name]?.size || 0
    window.runtime = new Proxy({EventsOnMultiple:(name,fn) => { (listeners[name] ||= new Set()).add(fn); return () => listeners[name].delete(fn) }, EventsEmit:(name,...args) => [...(listeners[name] || [])].forEach(fn=>fn(...args)), Environment:async()=>({platform:'windows'}), WindowGetSize:async()=>({w:1440,h:1000})}, {get:(target,key)=>target[key] || (()=>{})})
    window.__qaCalls = []
    const api = {
      ListAccounts:async()=>structuredClone(accounts), ListProviders:async()=>[
        {ID:'webdav',Meta:{key:'webdav',label:'WebDAV',icon:'drive-icons/webdav.svg',rootKey:'root'},Capabilities:{download:true,upload:true,uploadMode:'queue',search:true,copy:true,createFolder:true,rename:true,offlineDownload:offlineQA}},
        {ID:'aliopen',Meta:{key:'aliopen',label:'阿里云盘',icon:'drive-icons/aliopen.svg',rootKey:'root'},Capabilities:{importShare:true,download:true,upload:true,search:true}}
      ],
      GetSettings:async()=>startupQA ? new Promise(resolve => { window.__qaFinishSettings = resolve }) : ({theme:'dark',autoUpdate:false,defaultTab:'pan'}), SaveSettings:async()=>{},
      RefreshAccount:async id=>accounts.find(a=>a.user_id===id), RefreshAccountNow:async id=>accounts.find(a=>a.user_id===id),
      ReconnectMountedAccount: async id => {
        const account = accounts.find(item => item.user_id === id)
        account.auth_state = ''
        window.runtime.EventsEmit('account:changed', account)
        return account
      },
      ListDir:async(user,drive,dir)=>{if (window.__qaListDirGate) await window.__qaListDirGate; await new Promise(r=>setTimeout(r,20));return dir==='root'? (themeQA ? [...(window.__qaRootPrefix || []), ...folders, ...files] : files) : themeQA && dir.startsWith('folder-') ? [{file_id:`child-${dir}`,parent_file_id:dir,name:'会议资料',isDir:true},files[0]] : [files[0]]},
      ListFavorites:async()=>[],GetDirectoryCache:async()=>null,SaveDirectoryCache:async()=>{},DeleteDirectoryCache:async()=>{},
      ListDownloads:async()=>themeQA ? [
        {id:'active',user_id:accounts[0].user_id,name:'设计资料.pdf',size:104857600,status:'downloading',progress:45,speed:5242880},
        {id:'paused',user_id:accounts[0].user_id,name:'项目归档.zip',size:209715200,status:'paused',progress:23},
        {id:'failed',user_id:accounts[0].user_id,name:'会议记录.docx',size:1048576,status:'failed',progress:12,issue:{code:'NETWORK_UNAVAILABLE',message:'网络请求中断',action:'network_settings'}}
      ] : [],
      ListUploads:async()=>themeQA ? [
        {UploadID:'hashing',user_id:accounts[0].user_id,Info:{name:'线性代数.pdf',size:130000000},Upload:{IsDowning:true,stage:'hashing',stageProgress:48}},
        {UploadID:'confirming',user_id:accounts[0].user_id,Info:{name:'参考资料.zip',size:20971520},Upload:{IsDowning:true,stage:'uploading',DownProcess:100}},
        {UploadID:'review',user_id:accounts[0].user_id,Info:{name:'待核对资料.pdf',size:1048576},Upload:{IsFailed:true,attempt:{state:'unknown'},issue:{code:'UPLOAD_RESULT_UNKNOWN',action:'check_target'}}}
      ] : [], ListOfflineTasks:async()=>[],
      ListShareHistory:async()=>[], ListShareImportReceipts:async()=>[], GetLogPath:async()=>'',
      SetAccountCustomMeta:async(id,name,icon)=>{const a=accounts.find(a=>a.user_id===id);a.custom_name=name;a.custom_icon=icon;window.runtime.EventsEmit('account:changed')},
      SearchCachedFiles:async()=>[{userId:accounts[0].user_id,driveId:accounts[0].drive_id,parentId:'root',file:files[0],updatedAt:1700000000}],
      SearchFiles:async()=>[files[1]],
      ImportShare:async()=>({files:shareImportQA ? files.map(file=>({fileId:file.file_id,name:file.name,size:file.size,isDir:file.isDir})) : [{fileId:'import-1',name:'分享资料',size:10,isDir:false}]}), SaveImportedShareDetailed:async(user,drive,session,ids,parent)=>({id:'receipt',accountId:user,driveId:drive,parentId:parent,historySaved:true,items:ids.map(sourceId=>({sourceId,status:'submitted'}))}),
      PreviewMigration:async()=>({files:1,bytes:1024,conflicts:[],warnings:[]}),MigrateFiles:async()=>({id:'new-migration',status:'pending'}),
      ListSyncConfigs:async()=>[{id:'sync1',name:'资料同步',user_id:accounts[0].user_id,drive_id:accounts[0].drive_id,local_dir:'D:/QA',remote_dir:'root',direction:'two-way',enabled:false}],ListRunningSyncIDs:async()=>[],
      PreviewSync:async()=>({token:'test-token',changes:[{path:'资料.txt',action:'conflict',local:{size:10},remote:{size:20}}]}),RunSyncPlan:async()=>{},
      ListMigrateJobs:async()=>[{id:'migration1',srcUser:accounts[0].user_id,dstUser:accounts[1].user_id,dstParent:'root',status:'completed',fileIDs:['file-0'],completedFileIDs:['file-0'],items:{'file-0':{id:'file-0',name:'资料.txt',status:'completed',verification:'size'}}}],
      VerifyMigration:async()=>[{id:'file-0',status:'hash',detail:'sha1 哈希一致'}],
      ExportPreferences:async payload=>{window.__qaBackup=JSON.parse(payload);return 'mock.json'},
      ImportPreferences:async()=>JSON.stringify({format:'mnemo-preferences',version:1,preferences:{oledBackground:true,accountAliases:{},accountIcons:{},accountOrder:accounts.map(a=>a.user_id).reverse()},theme:'dark',favorites:[]}),
    }
    window.go={app:{App:new Proxy(api,{get:(target,key)=>(...args)=>{window.__qaCalls.push({method:key,args});return target[key] ? target[key](...args) : Promise.resolve(null)}})}}
  }, { themeQA: ['QA_THEME_ONLY', 'QA_PAGE_MOTION_ONLY', 'QA_FILE_PREVIEW_ONLY', 'QA_FILE_SCROLL_ONLY', 'QA_COLUMNS_KEYBOARD_ONLY'].some(flag => process.env[flag] === '1'), railMotionQA: process.env.QA_RAIL_MOTION_ONLY === '1', previewQA: process.env.QA_FILE_PREVIEW_ONLY === '1', startupQA: process.env.QA_STARTUP_ONLY === '1', shareImportQA: process.env.QA_SHARE_IMPORT_ONLY === '1', offlineQA: process.env.QA_TRANSFER_OFFLINE_ONLY === '1', fileScrollQA: process.env.QA_FILE_SCROLL_ONLY === '1' })
  try {
    if (process.env.QA_PAN_TOOLBAR_ONLY === '1') {
      await page.setViewportSize({ width: 510, height: 720 })
      await page.goto(process.env.QA_URL || 'http://127.0.0.1:5173')
      await page.locator('.fileitem').first().waitFor()
      const toolbar = page.getByRole('group', { name: '文件操作', exact: true }).first()
      const sort = toolbar.getByTitle('排序方式', { exact: true })
      await sort.press('ArrowUp')
      const menu = page.getByRole('menu')
      await menu.waitFor()
      assert(await menu.getByRole('menuitem').last().evaluate(element => document.activeElement === element))
      const waitForMenuToSettle = () => page.waitForFunction(() => {
        const menu = document.querySelector('.ctx-menu')
        return menu && !menu.classList.contains('toolbar-dropdown-enter-active') && getComputedStyle(menu).transform === 'none'
      })
      await waitForMenuToSettle()
      const [buttonRect, menuRect] = await Promise.all([sort.boundingBox(), menu.boundingBox()])
      assert(Math.abs(menuRect.y - buttonRect.y - buttonRect.height - 4) < 1, `排序菜单应贴着触发器展开：${JSON.stringify({ buttonRect, menuRect })}`)
      assert(menuRect.x >= 7 && menuRect.x + menuRect.width <= 503, '菜单应保持在视口内')
      await page.keyboard.press('Escape')
      await menu.waitFor({ state: 'detached' })
      assert(await sort.evaluate(element => document.activeElement === element))
      const listView = toolbar.getByTitle('列表视图', { exact: true })
      const searchButton = toolbar.getByTitle('全盘搜索 (Ctrl/⌘+F)', { exact: true })
      await sort.press('ArrowDown')
      await menu.waitFor()
      await page.keyboard.press('Tab')
      await menu.waitFor({ state: 'detached' })
      assert(await listView.evaluate(element => document.activeElement === element), 'Tab 应关闭排序菜单并聚焦下一个工具栏按钮')
      await sort.press('ArrowDown')
      await menu.waitFor()
      await page.keyboard.press('Shift+Tab')
      await menu.waitFor({ state: 'detached' })
      assert(await searchButton.evaluate(element => document.activeElement === element), 'Shift+Tab 应关闭排序菜单并聚焦上一个工具栏按钮')
      await toolbar.evaluate(element => { element.style.marginTop = '400px' })
      await sort.press('ArrowDown')
      await waitForMenuToSettle()
      const [raisedButton, raisedMenu] = await Promise.all([sort.boundingBox(), menu.boundingBox()])
      assert(Math.abs(raisedButton.y - raisedMenu.y - raisedMenu.height - 4) < 1, '底部空间不足时，排序菜单应贴着按钮向上展开')
      await page.keyboard.press('Escape')
      await menu.waitFor({ state: 'detached' })
      await toolbar.evaluate(element => { element.style.removeProperty('margin-top') })
      const search = toolbar.getByTitle('全盘搜索 (Ctrl/⌘+F)', { exact: true })
      const selection = toolbar.getByTitle('选择', { exact: true })
      const selectionBefore = await selection.boundingBox()
      const toolbarBefore = await toolbar.boundingBox()
      const toolbarLayout = await toolbar.evaluate(element => Array.from(element.querySelectorAll('button'), button => [button.title, button.offsetWidth, button.offsetLeft, button.offsetTop]))
      await page.locator('.fileitem').first().click()
      assert.deepEqual(await selection.boundingBox(), selectionBefore, '选择按钮切换状态时不能移动或改变大小')
      assert.equal((await toolbar.boundingBox()).height, toolbarBefore.height, `窄窗选择文件时不应推动文件列表：${JSON.stringify({ toolbarBefore, toolbarLayout })}`)
      await toolbar.getByRole('button', { name: '取消选择', exact: true }).click()
      assert(await selection.evaluate(element => document.activeElement === element), '取消选择后焦点应返回选择按钮')
      await page.locator('.fileitem').first().click()
      await selection.press('Escape')
      assert.equal(await toolbar.getByRole('button', { name: '取消选择', exact: true }).count(), 0, '工具栏有焦点时按 Escape 也应取消选择')
      assert(await selection.evaluate(element => document.activeElement === element), '按 Escape 取消选择后焦点应返回选择按钮')
      await sort.press('Control+f')
      assert(await toolbar.getByRole('textbox', { name: '搜索当前网盘文件' }).evaluate(element => document.activeElement === element), '工具栏有焦点时按 Ctrl+F 应聚焦网盘搜索框')
      await page.keyboard.press('Escape')
      await search.click()
      const exitSearch = toolbar.getByRole('button', { name: '退出搜索', exact: true })
      await exitSearch.click()
      assert(await search.evaluate(element => document.activeElement === element), '空搜索也应支持退出并归还焦点')
      await search.click()
      const input = toolbar.getByRole('textbox', { name: '搜索当前网盘文件' })
      await input.fill('资料')
      assert(await toolbar.evaluate(element => element.scrollWidth <= element.clientWidth + 1), '窄窗内工具条不能横向溢出')
      await input.press('Escape')
      assert(await search.evaluate(element => document.activeElement === element))
      const refresh = toolbar.getByTitle('刷新 (F5)', { exact: true })
      await page.waitForFunction(() => document.querySelector('.pan-refresh')?.getAttribute('aria-busy') === 'false')
      await page.evaluate(() => {
        window.__qaListDirGate = new Promise(resolve => { window.__qaReleaseListDir = resolve })
      })
      await refresh.press('F5')
      assert.equal(await refresh.getAttribute('aria-busy'), 'true', '工具栏有焦点时按 F5 应刷新，且有缓存时仍应显示忙碌状态')
      await refresh.press('Enter')
      assert(await refresh.evaluate(element => document.activeElement === element), '刷新期间不能丢失焦点')
      await page.evaluate(() => {
        window.__qaReleaseListDir()
        window.__qaListDirGate = null
      })
      await page.waitForFunction(() => document.querySelector('.pan-refresh')?.getAttribute('aria-busy') === 'false')
      assert(await refresh.evaluate(element => document.activeElement === element), '刷新不能丢失焦点')
      const row = page.locator('.fileitem').first()
      await row.click({ button: 'right' })
      await menu.waitFor({ timeout: 3000 })
      assert(await menu.evaluate(async element => {
        await new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)))
        return element.isConnected && element.contains(document.activeElement)
      }), '窄窗口右键未选文件后，菜单不能被此前排队的滚动关闭')
      await page.keyboard.press('Escape')
      await menu.waitFor({ state: 'detached' })
      await toolbar.getByRole('button', { name: '取消选择', exact: true }).click()
      const grid = toolbar.getByTitle('网格视图', { exact: true })
      await grid.click()
      assert.equal(await grid.getAttribute('aria-pressed'), 'true')
      await page.waitForFunction(() => {
        const viewSwitch = document.querySelector('.pan-file-toolbar .view-switch')
        const highlight = new DOMMatrixReadOnly(getComputedStyle(viewSwitch, '::before').transform)
        return highlight.m41 === viewSwitch.firstElementChild.offsetWidth
      })
      const markRangeStart = async () => {
        await selection.click()
        await menu.getByRole('menuitem', { name: '区间选择', exact: true }).click()
        await page.locator('.griditem').first().click()
      }
      await markRangeStart()
      const anchor = page.locator('.griditem.anchor-node')
      const motion = await anchor.evaluate(element => {
        const pulse = getComputedStyle(element, '::after')
        return { iterations: pulse.animationIterationCount, name: pulse.animationName, outline: getComputedStyle(element).outlineStyle }
      })
      assert.deepEqual(motion, { iterations: '1', name: 'pulse-anchor', outline: 'solid' }, '区间起点应只提示一次并保留边框')
      await page.keyboard.press('Escape')
      await anchor.waitFor({ state: 'detached' })
      await page.emulateMedia({ reducedMotion: 'reduce' })
      assert.equal(await grid.evaluate(element => getComputedStyle(element.parentElement, '::before').transitionProperty), 'none')
      await markRangeStart()
      assert.equal(await anchor.evaluate(element => getComputedStyle(element, '::after').animationName), 'none', '减少动态效果时不播放区间起点动效')
      assert.equal(await anchor.evaluate(element => getComputedStyle(element).outlineStyle), 'solid', '减少动态效果时保留起点边框')
      assert.deepEqual(errors, [])
      console.log(JSON.stringify({ passed: true, narrowViewport: 510, anchoredMenuInBothDirections: true, stableSelection: true, keyboardFocusPreserved: true, cachedRefreshBusy: true, pressedView: true, reducedMotion: true }, null, 2))
      return
    }
    if (process.env.QA_QUICK_OPEN_ONLY === '1') {
      await page.addInitScript(() => {
        window.go.app.App.ListAccounts = async () => Array.from({ length: 300 }, (_, index) => ({
          user_id: `webdav:quick-${index}`, drive_id: `quick-${index}`, custom_name: `快捷资料库-${index}`, token: {},
        }))
      })
      await page.goto(process.env.QA_URL || 'http://127.0.0.1:5173')
      const trigger = page.getByRole('button', { name: '快捷命令面板 (Ctrl+P)' })
      await trigger.click()
      const dialog = page.getByRole('dialog', { name: '快捷命令面板' })
      const input = dialog.getByRole('combobox', { name: '搜索功能、命令或账号' })
      const options = dialog.getByRole('option')
      await dialog.locator('[aria-setsize="309"]').first().waitFor()
      assert(await options.count() <= 16)
      await input.dispatchEvent('keydown', { key: 'Enter', isComposing: true, bubbles: true, cancelable: true })
      assert(await dialog.isVisible(), '输入法确认候选词不能执行命令')
      await input.press('Control+End')
      const active = dialog.locator('[role="option"][aria-selected="true"]')
      assert.match(await active.innerText(), /快捷资料库-299/)
      assert.equal(await input.getAttribute('aria-activedescendant'), await active.getAttribute('id'))
      assert(await options.count() <= 16)
      await input.fill('快捷资料库-12')
      assert.match(await active.innerText(), /快捷资料库-12\b/)
      assert.equal(await active.getAttribute('aria-posinset'), '1')
      await input.fill('不存在的命令-xyz')
      assert.equal(await options.count(), 0)
      assert.equal(await input.getAttribute('aria-activedescendant'), null)
      await input.press('Escape')
      await dialog.waitFor({ state: 'detached' })
      assert(await trigger.evaluate(element => document.activeElement === element))
      assert.deepEqual(errors, [])
      console.log(JSON.stringify({ passed: true, accounts: 300, virtualRowsAtMost: 16, imeEnterSafe: true, restoresFocus: true }, null, 2))
      return
    }
    if (process.env.QA_GLOBAL_SEARCH_ONLY === '1') {
      await page.addInitScript(() => {
        window.go.app.App.SearchCachedFiles = async () => []
        window.go.app.App.SearchFilesCancelable = async (_request, account) => {
          if (account === 'webdav:account-0') return new Promise(resolve => {
            window.__qaReleaseSearch = () => resolve(Array.from({ length: 100 }, (_, index) => ({ file_id: `insert-${index}`, name: `补充结果-${index}.txt`, parent_file_id: 'root' })))
          })
          if (account === 'webdav:account-1') return Array.from({ length: 10000 }, (_, index) => ({ file_id: `search-${index}`, name: `搜索文件-${index}.txt`, parent_file_id: 'root' }))
          return []
        }
      })
      await page.goto(process.env.QA_URL || 'http://127.0.0.1:5173')
      await page.getByRole('textbox', { name: '搜索网盘文件', exact: true }).fill('搜索文件')
      await page.getByRole('textbox', { name: '搜索网盘文件', exact: true }).press('Enter')
      const dialog = page.getByRole('dialog', { name: '搜索', exact: true })
      const input = dialog.getByRole('textbox', { name: '跨盘搜索名称' })
      const list = dialog.getByRole('listbox', { name: '搜索结果' })
      const rows = list.getByRole('option')
      await dialog.getByRole('checkbox', { name: '同时在线搜索' }).check()
      await input.press('Enter')
      await dialog.getByText(/10[,\s]?000 项结果/).waitFor()
      assert(await rows.count() <= 19)
      await input.press('ArrowDown')
      const first = list.getByRole('option').filter({ hasText: '搜索文件-0.txt' })
      assert(await first.evaluate(element => document.activeElement === element))
      await list.evaluate(element => { element.scrollTop = 5000 * 56 + 13 })
      const middle = list.getByRole('option').filter({ hasText: '搜索文件-5000.txt' })
      await middle.waitFor()
      assert(await first.evaluate(element => document.activeElement === element), '滚动不能丢失键盘焦点')
      assert(await rows.count() <= 19)
      const previousTop = await middle.evaluate(element => element.getBoundingClientRect().top - element.closest('[role=listbox]').getBoundingClientRect().top)
      await page.evaluate(() => window.__qaReleaseSearch())
      await dialog.getByText(/10[ ,]?100 项结果/).waitFor()
      assert.equal(await list.evaluate(element => element.scrollTop), 5100 * 56 + 13, '更早账号的结果插入时应保留滚动锚点')
      const currentTop = await middle.evaluate(element => element.getBoundingClientRect().top - element.closest('[role=listbox]').getBoundingClientRect().top)
      assert(Math.abs(currentTop - previousTop) < 1, '同一文件在视口内的位置应保持稳定')
      assert(await first.evaluate(element => document.activeElement === element))
      await page.keyboard.press('End')
      const last = list.getByRole('option').filter({ hasText: '搜索文件-9999.txt' })
      assert(await last.evaluate(element => document.activeElement === element))
      assert.equal(await last.getAttribute('aria-posinset'), '10100')
      await page.keyboard.press('Home')
      await page.keyboard.press('ArrowUp')
      assert(await input.evaluate(element => document.activeElement === element))
      assert.deepEqual(errors, [])
      console.log(JSON.stringify({ passed: true, searchResults: 10100, virtualRowsAtMost: 19, focusSurvivesScroll: true, lastResultKeyboardReachable: true, anchorSurvivesInsertion: true }, null, 2))
      return
    }
    if (process.env.QA_VIDEO_CONTROLS_ONLY === '1') {
      const fixture = path.join(fs.mkdtempSync(path.join(output, 'video-')), 'controls.mp4')
      require('node:child_process').execFileSync(process.env.FFMPEG || 'ffmpeg', ['-v', 'error', '-f', 'lavfi', '-i', 'color=c=navy:s=320x180:r=10:d=30', '-threads', '1', '-c:v', 'libx264', '-pix_fmt', 'yuv420p', fixture])
      const bytes = fs.readFileSync(fixture)
      await page.route('**/qa-controls.mp4', route => route.fulfill({ contentType: 'video/mp4', body: bytes }))
      await page.addInitScript(size => {
        const files = Array.from({ length: 10000 }, (_, index) => ({ file_id: `video-${index}`, name: `视频-${index}.mp4`, size }))
        const file = files[5000]
        window.go.app.PreviewHost = {
          Config: async () => ({ kind: 'video', account: { user_id: 'webdav:account-0', drive_id: 'drive-0' }, file, files }),
          Invoke: async method => method === 'PlayVideo' ? { url: URL.createObjectURL(await (await fetch('/qa-controls.mp4')).blob()), stream_type: 'mp4' } : null,
        }
      }, bytes.length)
      await page.goto(process.env.QA_URL || 'http://127.0.0.1:5173')
      const slider = page.getByRole('slider', { name: '播放进度' })
      const controls = page.locator('.pp-bottom')
      const video = page.locator('.pp-video')
      await page.waitForFunction(() => document.querySelector('.pp-video')?.duration === 30)
      await video.evaluate(element => element.pause())
      const initial = await video.evaluate(element => element.currentTime)
      const preview = async value => {
        await slider.dispatchEvent('pointerdown', { button: 0 })
        await slider.evaluate((element, time) => {
          element.value = time
          element.dispatchEvent(new Event('input', { bubbles: true }))
        }, value)
      }
      await preview(20)
      assert.equal(await slider.getAttribute('aria-valuetext'), '00:20 / 00:30')
      assert.equal(await video.evaluate(element => element.currentTime), initial)
      await slider.press('Escape')
      assert.equal(await video.evaluate(element => element.currentTime), initial)
      await preview(18)
      await slider.dispatchEvent('pointerup', { button: 0 })
      await page.waitForFunction(() => document.querySelector('.pp-video')?.currentTime === 18 && !document.querySelector('.pp-video').seeking)
      await slider.evaluate(element => element.blur())
      await page.mouse.move(300, 200)
      assert.equal(await controls.evaluate(element => element.classList.contains('hidden')), false)
      await video.evaluate(element => element.play())
      await page.waitForFunction(() => document.querySelector('.pp-progress')?.classList.contains('gliding'))
      const fill = page.locator('.pp-progress-fill')
      assert.equal(await fill.evaluate(element => getComputedStyle(element).transitionProperty), 'transform')
      await page.emulateMedia({ reducedMotion: 'reduce' })
      assert.equal(await fill.evaluate(element => getComputedStyle(element).transitionProperty), 'none')
      await page.waitForFunction(() => document.querySelector('.pp-bottom')?.classList.contains('hidden'))
      await slider.focus()
      assert.equal(await controls.evaluate(element => element.classList.contains('hidden')), false)
      const playlistButton = page.getByTitle('播放列表', { exact: true })
      const playlist = page.getByRole('listbox', { name: '播放列表' })
      await playlistButton.click()
      const selected = playlist.getByRole('option', { selected: true })
      assert.equal(await selected.getAttribute('aria-posinset'), '5001')
      assert.equal(await selected.getAttribute('aria-setsize'), '10000')
      assert(await playlist.getByRole('option').count() <= 16)
      await playlist.evaluate(element => { element.scrollTop = 2500 * 36 })
      await playlist.getByRole('option', { name: '视频-2500.mp4', exact: true }).waitFor()
      assert(await playlist.getByRole('option').count() <= 16)
      assert.equal(await selected.count(), 0, '滚出视口的当前集不应继续挂载')
      await playlist.press('Escape')
      assert(await playlistButton.evaluate(element => document.activeElement === element))
      await playlistButton.click()
      await playlist.press('End')
      const last = playlist.getByRole('option', { name: '视频-9999.mp4', exact: true })
      assert.equal(await playlist.getAttribute('aria-activedescendant'), await last.getAttribute('id'))
      assert(await playlist.getByRole('option').count() <= 16)
      await playlist.press('Enter')
      await page.getByRole('dialog', { name: '视频预览：视频-9999.mp4' }).waitFor()
      assert.deepEqual(errors, [])
      console.log(JSON.stringify({ passed: true, escapeCancelsSeek: true, seekOnRelease: true, keyboardRevealsControls: true, reducedMotion: true, videoPlaylistItems: 10000, virtualRowsAtMost: 16 }, null, 2))
      return
    }
    if (process.env.QA_AUDIO_PREVIEW_ONLY === '1') {
      const sampleRate = 8000
      const dataSize = 30 * sampleRate * 2
      const wav = Buffer.alloc(44 + dataSize)
      wav.write('RIFF', 0)
      wav.writeUInt32LE(wav.length - 8, 4)
      wav.write('WAVEfmt ', 8)
      wav.writeUInt32LE(16, 16)
      wav.writeUInt16LE(1, 20)
      wav.writeUInt16LE(1, 22)
      wav.writeUInt32LE(sampleRate, 24)
      wav.writeUInt32LE(sampleRate * 2, 28)
      wav.writeUInt16LE(2, 32)
      wav.writeUInt16LE(16, 34)
      wav.write('data', 36)
      wav.writeUInt32LE(dataSize, 40)
      await page.route('**/qa-audio/*.wav', route => {
        const range = /^bytes=(\d+)-(\d*)$/.exec(route.request().headers().range || '')
        if (!range) return route.fulfill({ contentType: 'audio/wav', headers: { 'Accept-Ranges': 'bytes' }, body: wav })
        const start = Number(range[1])
        const end = Math.min(wav.length - 1, range[2] ? Number(range[2]) : wav.length - 1)
        return route.fulfill({ status: 206, contentType: 'audio/wav', headers: { 'Accept-Ranges': 'bytes', 'Content-Range': `bytes ${start}-${end}/${wav.length}` }, body: wav.subarray(start, end + 1) })
      })
      await page.addInitScript(() => {
        const ids = ['first', 'second', 'third', ...Array.from({ length: 9997 }, (_, index) => `track-${index}`)]
        const files = ids.map(id => ({ file_id: id, name: `${id}.wav`, size: 480044 }))
        window.go.app.PreviewHost = {
          Config: async () => ({ kind: 'audio', account: { user_id: 'webdav:account-0', drive_id: 'drive-0' }, file: files[0], files }),
          Invoke: async (method, args) => {
            if (method !== 'PreviewURL') return null
            if (args[2] === 'second') await new Promise(resolve => { window.__qaReleaseSecond = resolve })
            return `${location.origin}/qa-audio/${args[2]}.wav`
          },
        }
      })
      await page.goto(process.env.QA_URL || 'http://127.0.0.1:5173')
      const slider = page.getByRole('slider', { name: '播放进度' })
      const audio = page.locator('.pv-audio-el')
      const fill = page.locator('.pv-audio-fill')
      await page.waitForFunction(() => {
        const element = document.querySelector('audio')
        return element?.duration === 30 && element.seekable.length && element.seekable.end(0) >= 18
      })
      await audio.evaluate(element => element.pause())
      const initial = await audio.evaluate(element => element.currentTime)
      const preview = async target => {
        await slider.dispatchEvent('pointerdown', { button: 0 })
        await slider.evaluate((input, value) => { input.value = value; input.dispatchEvent(new Event('input', { bubbles: true })) }, target)
      }
      await preview(20)
      assert.equal(await audio.evaluate(element => element.currentTime), initial, '拖动中不能提前跳播')
      assert.match(await slider.getAttribute('aria-valuetext'), /^0:20 /)
      await slider.dispatchEvent('pointercancel')
      assert.equal(await audio.evaluate(element => element.currentTime), initial, '取消拖动不能跳播')
      assert.match(await slider.getAttribute('aria-valuetext'), /^0:00 /)
      await preview(18)
      await slider.dispatchEvent('pointerup', { button: 0 })
      await page.waitForFunction(() => document.querySelector('audio')?.currentTime === 18 && !document.querySelector('audio').seeking)
      await page.getByTitle('播放 (空格)', { exact: true }).click()
      await page.waitForFunction(() => document.querySelector('.pv-audio-track')?.classList.contains('gliding'))
      assert.equal(await fill.evaluate(element => getComputedStyle(element).transitionProperty), 'transform')
      await page.emulateMedia({ reducedMotion: 'reduce' })
      assert.equal(await fill.evaluate(element => getComputedStyle(element).transitionProperty), 'none')
      const next = page.getByTitle('下一曲', { exact: true })
      await next.click()
      await page.waitForFunction(() => !!window.__qaReleaseSecond)
      assert.equal(await slider.getAttribute('aria-valuetext'), '0:00 / 0:00')
      assert.equal(await audio.count(), 0)
      await next.click()
      await page.waitForFunction(() => document.querySelector('audio')?.dataset.fileId === 'third' && document.querySelector('audio').duration === 30)
      await page.evaluate(() => window.__qaReleaseSecond())
      await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))))
      assert.equal(await audio.getAttribute('data-file-id'), 'third', '过期切歌请求不能覆盖当前音频')
      const playlistButton = page.getByTitle('播放列表', { exact: true })
      await playlistButton.click()
      const playlist = page.getByRole('listbox', { name: '播放列表' })
      assert.equal(await playlist.getByRole('option', { selected: true }).getAttribute('aria-posinset'), '3')
      await playlist.press('End')
      assert(await playlist.getByRole('option').count() <= 16)
      await playlist.press('Enter')
      await page.waitForFunction(() => document.querySelector('audio')?.dataset.fileId === 'track-9996')
      assert(await playlistButton.evaluate(element => document.activeElement === element))
      assert.deepEqual(errors, [])
      console.log(JSON.stringify({ passed: true, cancelWithoutSeek: true, seekOnRelease: true, reducedMotion: true, rapidTrackChange: true, audioPlaylistItems: 10000, virtualRowsAtMost: 16 }, null, 2))
      return
    }
    if (process.env.QA_PPTX_PREVIEW_ONLY === '1') {
      const Zip = require('jszip')
      const zip = new Zip()
      const count = 40
      const indices = Array.from({ length: count }, (_, index) => index + 1)
      const rel = 'http://schemas.openxmlformats.org/officeDocument/2006/relationships'
      const pkg = 'http://schemas.openxmlformats.org/package/2006/relationships'
      const ppt = 'http://schemas.openxmlformats.org/presentationml/2006/main'
      zip.file('[Content_Types].xml', '<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/ppt/presentation.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml"/></Types>')
      zip.file('_rels/.rels', `<Relationships xmlns="${pkg}"><Relationship Id="rId1" Type="${rel}/officeDocument" Target="ppt/presentation.xml"/></Relationships>`)
      zip.file('ppt/presentation.xml', `<p:presentation xmlns:p="${ppt}" xmlns:r="${rel}"><p:sldIdLst>${indices.map(number => `<p:sldId id="${255 + number}" r:id="rId${number}"/>`).join('')}</p:sldIdLst><p:sldSz cx="9144000" cy="5143500"/></p:presentation>`)
      zip.file('ppt/_rels/presentation.xml.rels', `<Relationships xmlns="${pkg}">${indices.map(number => `<Relationship Id="rId${number}" Type="${rel}/slide" Target="slides/slide${number}.xml"/>`).join('')}</Relationships>`)
      for (const number of indices) zip.file(`ppt/slides/slide${number}.xml`, `<p:sld xmlns:p="${ppt}"><p:cSld><p:spTree><p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr/></p:spTree></p:cSld></p:sld>`)
      const bytes = await zip.generateAsync({ type: 'nodebuffer' })
      await page.route('**/qa-preview.pptx', route => route.fulfill({ contentType: 'application/octet-stream', body: bytes }))
      await page.addInitScript(size => {
        const file = { file_id: 'pptx', name: '预览.pptx', size }
        window.go.app.PreviewHost = {
          Config: async () => ({ kind: 'pptx', account: { user_id: 'webdav:account-0', drive_id: 'drive-0' }, file }),
          Invoke: async method => method === 'PreviewURL' ? `${location.origin}/qa-preview.pptx` : null,
        }
      }, bytes.length)
      await page.setViewportSize({ width: 900, height: 640 })
      await page.goto(process.env.QA_URL || 'http://127.0.0.1:5173')
      const stage = page.getByRole('region', { name: '幻灯片内容' })
      const jump = page.getByRole('textbox', { name: '跳转到幻灯片页码' })
      const target = stage.locator('[data-slide-index="29"] [data-mounted="1"]')
      await stage.locator('[data-slide-index="0"] [data-mounted="1"]').waitFor()
      await jump.fill('30')
      await jump.press('Enter')
      await target.waitFor()
      assert(await jump.evaluate(element => document.activeElement === element), '跳页不能抢走焦点')
      assert(await stage.locator('[data-mounted="1"]').count() < 12, '跳页不能挂载整份演示文稿')
      const measure = () => stage.evaluate(element => {
        const item = element.querySelector('[data-slide-index="29"]')
        const bounds = item.getBoundingClientRect()
        return { width: bounds.width, progress: (element.getBoundingClientRect().top + element.clientHeight / 2 - bounds.top) / (item.nextElementSibling.getBoundingClientRect().top - bounds.top) }
      })
      const before = await measure()
      await stage.evaluate(element => {
        const host = element.querySelector('.pptx-host')
        const button = document.querySelector('[aria-label="放大幻灯片"]')
        window.__pptxRebuilds = 0
        const observer = new MutationObserver(records => {
          for (const record of records) if ([...record.removedNodes].some(node => node.dataset?.slideIndex === '0')) window.__pptxRebuilds++
        })
        observer.observe(host, { childList: true })
        for (let count = 0; count < 3; count++) button.click()
      })
      await page.waitForFunction(width => document.querySelector('[data-slide-index="29"]')?.getBoundingClientRect().width > width * 1.28, before.width)
      await target.waitFor()
      assert.equal(await page.evaluate(() => window.__pptxRebuilds), 1)
      const after = await measure()
      assert(Math.abs(after.progress - before.progress) < 0.02, `缩放后阅读位置偏移：${JSON.stringify({ before, after })}`)
      await page.setViewportSize({ width: 1100, height: 640 })
      await page.waitForFunction(width => document.querySelector('[data-slide-index="29"]')?.getBoundingClientRect().width > width + 100, after.width)
      await target.waitFor()
      assert(Math.abs((await measure()).progress - before.progress) < 0.02, '窗口缩放应保留当前页内的位置')
      assert.deepEqual(errors, [])
      console.log(JSON.stringify({ passed: true, slides: count, jumpedToSlide: 30, rebuildsForThreeClicks: 1, zoomPositionPreserved: true, resizePositionPreserved: true }, null, 2))
      return
    }
    if (process.env.QA_DOCX_PREVIEW_ONLY === '1') {
      const Zip = require('jszip')
      const zip = new Zip()
      zip.file('[Content_Types].xml', '<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>')
      zip.file('_rels/.rels', '<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>')
      const paragraphs = Array.from({ length: 30 }, (_, index) => `${index ? '<w:p><w:r><w:br w:type="page"/></w:r></w:p>' : ''}<w:p><w:r><w:t>第 ${index + 1} 页内容</w:t></w:r></w:p>`).join('')
      zip.file('word/document.xml', `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>${paragraphs}<w:sectPr><w:pgSz w:w="11906" w:h="16838"/><w:pgMar w:top="1440" w:right="1440" w:bottom="1440" w:left="1440"/></w:sectPr></w:body></w:document>`)
      const bytes = await zip.generateAsync({ type: 'nodebuffer' })
      await page.route('**/qa-preview.docx', route => route.fulfill({ contentType: 'application/octet-stream', body: bytes }))
      await page.addInitScript(size => {
        const file = { file_id: 'docx', name: '预览.docx', size }
        window.go.app.PreviewHost = {
          Config: async () => ({ kind: 'docx', account: { user_id: 'webdav:account-0', drive_id: 'drive-0' }, file }),
          Invoke: async method => method === 'PreviewURL' ? `${location.origin}/qa-preview.docx` : null,
        }
      }, bytes.length)
      await page.setViewportSize({ width: 780, height: 640 })
      await page.goto(process.env.QA_URL || 'http://127.0.0.1:5173')
      const stage = page.getByRole('region', { name: 'Word 文档内容' })
      const jump = page.getByRole('textbox', { name: '跳转到 Word 页码' })
      const fit = page.getByRole('button', { name: '适合宽度' })
      const pages = stage.locator('section.docx')
      await stage.getByText('第 18 页内容').waitFor()
      assert.equal(await pages.count(), 30)
      await jump.fill('18')
      await jump.press('Enter')
      assert(await jump.evaluate(element => document.activeElement === element), '跳页不能抢走焦点')
      const measure = () => stage.evaluate(element => {
        const sections = element.querySelectorAll('section.docx')
        return {
          center: (element.scrollTop + element.clientHeight / 2) / Number(element.firstElementChild.style.zoom),
          targetTop: sections[17].getBoundingClientRect().top - element.getBoundingClientRect().top,
          width: sections[0].getBoundingClientRect().width,
        }
      })
      const before = await measure()
      assert(Math.abs(before.targetTop - 12) < 2, '跳页应将目标页定位到阅读区顶部')
      await page.getByRole('button', { name: '放大 Word' }).click()
      assert.equal(await fit.getAttribute('aria-pressed'), 'false')
      const enlarged = await measure()
      assert(enlarged.width > before.width && Math.abs(enlarged.center - before.center) < 3, '缩放应保留阅读位置')
      await fit.click()
      assert.equal(await fit.getAttribute('aria-pressed'), 'true')
      await page.setViewportSize({ width: 960, height: 640 })
      await page.waitForFunction(width => document.querySelector('section.docx')?.getBoundingClientRect().width > width + 20, before.width)
      const wider = await measure()
      assert(Math.abs(wider.center - before.center) < 3, '窗口缩放应保留阅读位置')
      assert.deepEqual(errors, [])
      console.log(JSON.stringify({ passed: true, pages: 30, jumpedToPage: 18, keyboardFocusPreserved: true, zoomAnchorError: enlarged.center - before.center, resizeAnchorError: wider.center - before.center }, null, 2))
      return
    }
    if (process.env.QA_SPREADSHEET_PREVIEW_ONLY === '1') {
      const xlsx = require('xlsx')
      const workbook = xlsx.utils.book_new()
      const sheet = xlsx.utils.aoa_to_sheet([['起点']])
      xlsx.utils.sheet_add_aoa(sheet, [['终点']], { origin: 'AX100000' })
      xlsx.utils.book_append_sheet(workbook, sheet, '大表')
      xlsx.utils.book_append_sheet(workbook, xlsx.utils.aoa_to_sheet([['汇总值']]), '汇总')
      const bytes = xlsx.write(workbook, { type: 'buffer', bookType: 'xlsx' })
      await page.route('**/qa-preview.xlsx', route => route.fulfill({ contentType: 'application/octet-stream', body: bytes }))
      await page.addInitScript(size => {
        const file = { file_id: 'xlsx', name: '预览.xlsx', size }
        window.go.app.PreviewHost = {
          Config: async () => ({ kind: 'xlsx', account: { user_id: 'webdav:account-0', drive_id: 'drive-0' }, file }),
          Invoke: async method => method === 'PreviewURL' ? `${location.origin}/qa-preview.xlsx` : null,
        }
      }, bytes.length)
      await page.setViewportSize({ width: 1024, height: 768 })
      await page.goto(process.env.QA_URL || 'http://127.0.0.1:5173')
      const stage = page.getByRole('region', { name: '表格内容' })
      const select = page.getByRole('combobox', { name: '工作表', exact: true })
      await stage.getByRole('cell', { name: '起点' }).waitFor()
      const firstCell = await stage.getByRole('cell', { name: '起点' }).boundingBox()
      const rows = await stage.locator('tbody tr[aria-rowindex]').count()
      const cells = await stage.locator('tbody td[aria-colindex]').count()
      assert(cells < rows * 16, '大表不应创建视口外的所有列')
      await stage.evaluate(element => { element.scrollTo(element.scrollWidth, element.scrollHeight) })
      const last = stage.getByRole('cell', { name: '终点' })
      await last.waitFor()
      const geometry = await last.evaluate(cell => {
        const scroller = cell.closest('.spreadsheet-stage')
        const bounds = scroller.getBoundingClientRect()
        const corner = scroller.querySelector('thead th').getBoundingClientRect()
        const rowHead = cell.parentElement.querySelector('th').getBoundingClientRect()
        const cellBox = cell.getBoundingClientRect()
        return {
          sticky: Math.abs(corner.x - bounds.x) < 1 && Math.abs(corner.y - bounds.y) < 1 && Math.abs(rowHead.x - bounds.x) < 1,
          visible: cellBox.right <= bounds.right + 1 && cellBox.bottom <= bounds.bottom + 1 && cellBox.left >= rowHead.right - 1 && cellBox.top >= corner.bottom - 1,
          width: cellBox.width,
        }
      })
      assert(geometry.sticky && geometry.visible, `表尾或冻结表头位置错误：${JSON.stringify(geometry)}`)
      assert(Math.abs(firstCell.width - geometry.width) < 1, '横向滚动不能改变列宽')
      await select.press('ArrowDown')
      await select.press('Enter')
      await stage.getByRole('cell', { name: '汇总值' }).waitFor()
      assert(await select.evaluate(element => document.activeElement === element), '切换工作表不能抢走焦点')
      assert.deepEqual(await stage.evaluate(element => [element.scrollTop, element.scrollLeft]), [0, 0])
      await page.emulateMedia({ reducedMotion: 'reduce' })
      await select.press('ArrowUp')
      await select.press('Enter')
      await stage.getByRole('cell', { name: '起点' }).waitFor()
      assert.equal(await page.locator('.spreadsheet-fade').evaluate(element => element.getAnimations().length), 0)
      assert.deepEqual(errors, [])
      console.log(JSON.stringify({ passed: true, rows, cells, rowOnlyCells: rows * 50, frozenHeaders: true, keyboardFocusPreserved: true, reducedMotion: true }, null, 2))
      return
    }
    if (process.env.QA_PDF_PREVIEW_ONLY === '1') {
      const objects = ['<< /Type /Catalog /Pages 2 0 R >>', '<< /Type /Pages /Kids [3 0 R 4 0 R 5 0 R] /Count 3 >>', ...Array.from({ length: 3 }, () => '<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Resources << >> /Contents 6 0 R >>'), '<< /Length 0 >>\nstream\n\nendstream']
      let pdf = '%PDF-1.7\n'
      const offsets = objects.map((object, index) => {
        const offset = pdf.length
        pdf += `${index + 1} 0 obj\n${object}\nendobj\n`
        return offset
      })
      const xref = pdf.length
      pdf += `xref\n0 ${objects.length + 1}\n0000000000 65535 f \n${offsets.map(offset => `${String(offset).padStart(10, '0')} 00000 n \n`).join('')}trailer\n<< /Size ${objects.length + 1} /Root 1 0 R >>\nstartxref\n${xref}\n%%EOF\n`
      await page.route('**/qa-preview.pdf', route => route.fulfill({ contentType: 'application/pdf', body: pdf }))
      await page.addInitScript(() => {
        const file = { file_id: 'pdf', name: '预览.pdf', size: 700 }
        window.go.app.PreviewHost = {
          Config: async () => ({ kind: 'pdf', account: { user_id: 'webdav:account-0', drive_id: 'drive-0' }, file }),
          Invoke: async method => method === 'PreviewURL' ? `${location.origin}/qa-preview.pdf` : null,
        }
      })
      await page.goto(process.env.QA_URL || 'http://127.0.0.1:5173')
      const jump = page.getByRole('textbox', { name: '跳转到 PDF 页码' })
      const fit = page.getByRole('button', { name: '适合宽度' })
      const first = page.locator('.pdfViewer .page[data-page-number="1"]')
      await first.locator('canvas').waitFor()
      const previousWidth = await first.evaluate(element => element.offsetWidth)
      await page.setViewportSize({ width: 1000, height: 900 })
      await page.waitForFunction(width => document.querySelector('.pdfViewer .page')?.offsetWidth < width, previousWidth)
      await jump.fill('3')
      await jump.press('Enter')
      await page.locator('.pdfViewer .page[data-page-number="3"] canvas').waitFor()
      assert(await jump.evaluate(element => document.activeElement === element), '页码跳转不能抢走焦点')
      assert.equal(await jump.inputValue(), '3')
      await page.getByRole('button', { name: '放大 PDF' }).click()
      assert.equal(await fit.getAttribute('aria-pressed'), 'false')
      await fit.click()
      assert.equal(await fit.getAttribute('aria-pressed'), 'true')
      assert.deepEqual(errors, [])
      console.log(JSON.stringify({ passed: true, jumpedToPage: 3, fitTracksWidth: true, keyboardFocusPreserved: true }, null, 2))
      return
    }
    if (process.env.QA_TEXT_PREVIEW_ONLY === '1') {
      await page.route('**/qa-large-text.txt', route => route.fulfill({
        contentType: 'text/plain; charset=utf-8',
        body: Array.from({ length: 100000 }, (_, index) => index === 99999 ? '末行标记' : `日志行 ${index}`).join('\r\n'),
      }))
      await page.addInitScript(() => {
        const file = { file_id: 'large', name: '日志.txt', size: 1800000 }
        window.go.app.PreviewHost = {
          Config: async () => ({ kind: 'text', account: { user_id: 'webdav:account-0', drive_id: 'drive-0' }, file }),
          Invoke: async method => method === 'PreviewURL' ? `${location.origin}/qa-large-text.txt` : null,
        }
      })
      await page.goto(process.env.QA_URL || 'http://127.0.0.1:5173')
      const view = page.locator('.pv-code-view')
      await page.getByText('100000 行', { exact: true }).waitFor()
      const before = await view.evaluate(element => {
        element.scrollTop = element.scrollHeight
        const text = element.querySelector('.pv-code-text')
        const selection = getSelection()
        selection.selectAllChildren(text)
        const selected = selection.toString()
        selection.removeAllRanges()
        return { nodes: element.querySelectorAll('*').length, allSelected: selected.startsWith('日志行 0') && selected.endsWith('末行标记') }
      })
      assert(before.nodes <= 400 && before.allSelected, JSON.stringify(before))
      const offsets = await view.evaluate(async element => {
        await new Promise(requestAnimationFrame)
        const gutter = document.querySelector('.pv-gutter')
        return { content: element.scrollTop, gutter: gutter.scrollTop, height: element.scrollHeight, gutterHeight: gutter.scrollHeight, client: element.clientHeight, gutterClient: gutter.clientHeight }
      })
      assert(Math.abs(offsets.gutter - offsets.content) < 1, JSON.stringify(offsets))
      await page.getByTitle('文本查找 (Ctrl+F)', { exact: true }).click()
      const search = page.getByPlaceholder('在文本中查找… (Esc 关闭)')
      await search.fill('末行标记')
      await view.locator('mark').filter({ hasText: '末行标记' }).waitFor()
      assert.equal(await view.locator('mark').count(), 1)
      await search.fill('日志行')
      await page.getByText('1 / 99999 个匹配', { exact: true }).waitFor()
      assert.equal(await view.locator('mark').count(), 1, '高频词只需要渲染当前高亮')
      await search.press('Shift+Enter')
      await page.getByText('99999 / 99999 个匹配', { exact: true }).waitFor()
      assert(await view.locator('mark.current').evaluate(mark => {
        const match = mark.getBoundingClientRect()
        const bounds = mark.closest('.pv-code-view').getBoundingClientRect()
        return match.top >= bounds.top && match.bottom <= bounds.bottom
      }), '上一处应把最后一处匹配滚动到视口')
      await search.press('Enter')
      await page.getByText('1 / 99999 个匹配', { exact: true }).waitFor()
      await search.press('Escape')
      assert(await page.getByTitle('文本查找 (Ctrl+F)').evaluate(element => document.activeElement === element), '关闭查找应恢复键盘焦点')
      assert.deepEqual(errors, [])
      console.log(JSON.stringify({ passed: true, lines: 100000, previewElements: before.nodes, nativeSelection: before.allSelected, gutterSynced: true, searchMatches: 99999, denseHighlights: 1, wrappedNavigation: true }, null, 2))
      return
    }
    if (process.env.QA_IMAGE_TRANSITION_ONLY === '1' || process.env.QA_IMAGE_FILMSTRIP_ONLY === '1' || process.env.QA_IMAGE_GESTURES_ONLY === '1') {
      const filmstrip = process.env.QA_IMAGE_FILMSTRIP_ONLY === '1'
      const gestures = process.env.QA_IMAGE_GESTURES_ONLY === '1'
      await page.addInitScript(({ filmstrip, gestures }) => {
        const files = filmstrip
          ? Array.from({ length: 10000 }, (_, index) => ({ file_id: `image-${index}`, name: `图片-${index}.png`, size: 200 }))
          : ['red', 'green', 'blue'].map(color => ({ file_id: color, name: `${color}.png`, size: 200 }))
        window.go.app.PreviewHost = {
          Config: async () => ({ kind: 'image', account: { user_id: 'webdav:account-0', drive_id: 'drive-0' }, file: files[0], files }),
          Invoke: async (method, args) => method === 'CachedPreviewImageURL'
            ? 'data:image/svg+xml,' + encodeURIComponent(`<svg xmlns="http://www.w3.org/2000/svg" width="${gestures ? 1600 : 300}" height="${gestures ? 400 : 200}"><rect width="100%" height="100%" fill="${filmstrip ? 'blue' : args[2].file_id}"/></svg>`)
            : null,
        }
      }, { filmstrip, gestures })
      await page.goto(process.env.QA_URL || 'http://127.0.0.1:5173')
      const layers = page.locator('.pv-img-layer')
      await layers.first().waitFor()
      if (gestures) {
        const stage = page.locator('.pv-stage')
        const image = layers.first().locator('img')
        await page.waitForFunction(() => document.querySelector('.pv-img-layer')?.getAnimations().every(animation => animation.playState === 'finished'))
        const before = await image.boundingBox()
        assert(Math.abs(before.width / before.height - 4) < 0.01, '宽图适配不能拉伸图片')
        const point = { x: before.x + before.width * 0.6, y: before.y + before.height * 0.4 }
        const wheel = async deltaY => {
          await stage.dispatchEvent('wheel', { deltaY, clientX: point.x, clientY: point.y, bubbles: true, cancelable: true })
          await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))))
          return image.boundingBox()
        }
        const small = await wheel(-4)
        const large = await wheel(-100)
        assert(small.width / before.width < 1.02 && small.width > before.width, '小幅滚轮输入不应变成固定倍率跳变')
        assert(large.width / small.width > 1.15, '大幅滚轮输入应有更明显的缩放')
        assert(Math.abs((point.x - large.x) / large.width - 0.6) < 0.002, '横向光标锚点偏移')
        assert(Math.abs((point.y - large.y) / large.height - 0.4) < 0.002, '纵向光标锚点偏移')
        assert.equal(await image.evaluate(element => getComputedStyle(element).transitionProperty), 'none')
        await page.mouse.move(point.x, point.y)
        await page.mouse.down()
        await page.mouse.move(point.x + 50, point.y + 25, { steps: 3 })
        await page.mouse.up()
        const dragged = await image.boundingBox()
        assert(Math.abs(dragged.x - large.x - 50) < 1 && Math.abs(dragged.y - large.y - 25) < 1, '拖拽应跟随指针并保留最终位置')
        await page.emulateMedia({ reducedMotion: 'reduce' })
        await page.getByRole('button', { name: '1:1', exact: true }).click()
        assert(Math.abs((await image.boundingBox()).width - 1600) < 1, '1:1 应显示原始像素尺寸')
        await page.getByTitle('适配窗口 (0)', { exact: true }).click()
        await page.getByTitle('顺时针旋转 90° (R)', { exact: true }).click()
        const rotated = await image.boundingBox()
        const viewport = await stage.boundingBox()
        assert(rotated.y >= viewport.y && rotated.y + rotated.height <= viewport.y + viewport.height - 76, '旋转后应按新方向适配')
        await stage.locator('.pv-edge.right').evaluate(button => button.click())
        await layers.locator('img[alt="green.png"]').waitFor()
        assert.equal(await layers.count(), 1, '减少动态效果时不应保留转场旧图层')
        assert.equal(await layers.first().evaluate(element => element.getAnimations().length), 0)
        assert.deepEqual(errors, [])
        console.log(JSON.stringify({ passed: true, cursorAnchor: true, proportionalWheel: true, directDrag: true, rotatedFit: true, reducedMotion: true }, null, 2))
        return
      }
      if (filmstrip) {
        await page.getByTitle('缩略图', { exact: true }).click()
        const strip = page.getByRole('listbox', { name: '图片缩略图' })
        const options = strip.getByRole('option')
        await strip.getByRole('option', { name: '图片-0.png' }).waitFor()
        const firstCount = await options.count()
        const transform = await layers.first().locator('img').evaluate(image => image.style.transform)
        await strip.hover()
        await page.mouse.wheel(0, 46000)
        await page.waitForFunction(() => document.querySelector('.pv-filmstrip')?.scrollLeft > 40000 && Number(document.querySelector('.pv-filmstrip [role=option]')?.getAttribute('aria-posinset')) > 500)
        const midCount = await options.count()
        assert(Math.max(firstCount, midCount) <= 30, '缩略图数量必须受视口大小限制')
        assert.equal(await layers.first().locator('img').evaluate(image => image.style.transform), transform, '滚动缩略图条不能缩放主图')
        await strip.press('End')
        const last = strip.getByRole('option', { name: '图片-9999.png' })
        await last.waitFor()
        assert.equal(await strip.getAttribute('aria-activedescendant'), await last.getAttribute('id'))
        await strip.press('Enter')
        await page.locator('.pv-img-layer:not(.leaving) img[alt="图片-9999.png"]').waitFor()
        assert.equal(await last.getAttribute('aria-selected'), 'true')
        await strip.press('Home')
        await strip.press('Space')
        await page.locator('.pv-img-layer:not(.leaving) img[alt="图片-0.png"]').waitFor()
        assert.deepEqual(errors, [])
        console.log(JSON.stringify({ passed: true, images: 10000, firstCount, midCount, keyboardSelection: true }, null, 2))
        return
      }
      for (const color of ['green', 'blue', 'red']) {
        const sample = await page.evaluate(async color => {
          document.querySelector('.pv-edge.right').click()
          for (let frame = 0; frame < 30; frame++) {
            await new Promise(requestAnimationFrame)
            const current = [...document.querySelectorAll('.pv-img-layer')]
            if (current.at(-1)?.querySelector('img').alt !== `${color}.png`) continue
            return { layers: current.length, outgoingOpacity: Number(getComputedStyle(current[0]).opacity), incomingAnimated: current.at(-1).getAnimations().some(animation => animation.animationName?.startsWith('pv-layer-in')) }
          }
          return null
        }, color)
        assert.deepEqual(sample, { layers: 2, outgoingOpacity: 1, incomingAnimated: true })
      }
      await layers.nth(1).waitFor({ state: 'detached' })
      assert.equal(await layers.first().locator('img').getAttribute('alt'), 'red.png')
      assert.deepEqual(errors, [])
      console.log(JSON.stringify({ passed: true, maxLayers: 2, opaqueOutgoing: true, finalLayers: await layers.count() }, null, 2))
      return
    }
    await page.goto(process.env.QA_URL || 'http://127.0.0.1:5173')
    await page.locator('.fileitem').first().waitFor()
    if (process.env.QA_SHARE_IMPORT_ONLY === '1') {
      await page.getByRole('button', { name: '分享', exact: true }).click()
      await page.evaluate(() => {
        let attempts = 0
        window.go.app.App.ImportShare = () => {
          attempts++
          if (attempts === 1) return new Promise(resolve => { window.__qaFinishImportParse = resolve })
          if (attempts === 2) return Promise.reject(new Error('读取失败'))
          return { files: Array.from({ length: 10000 }, (_, index) => ({
            fileId: `file-${index}`, name: `资料-${String(index).padStart(5, '0')}.txt`, size: 1024, isDir: false,
          })) }
        }
      })
      const opener = page.getByRole('button', { name: '导入分享', exact: true }).first()
      const dialog = page.getByRole('dialog', { name: '导入分享' })
      await opener.click()
      const link = dialog.getByRole('textbox', { name: '分享链接' })
      await link.fill('https://www.alipan.com/s/abc')
      const password = dialog.getByRole('textbox', { name: '提取码（可选）' })
      await password.dispatchEvent('keydown', { key: 'Enter', isComposing: true, bubbles: true, cancelable: true })
      assert.equal(await dialog.getByRole('button', { name: '解析中…' }).count(), 0, '确认输入法候选词不能触发解析')
      const parse = dialog.getByRole('button', { name: '解析分享', exact: true })
      await parse.click()
      await dialog.getByRole('button', { name: '解析中…' }).waitFor()
      await page.keyboard.press('Escape')
      await dialog.waitFor({ state: 'detached' })
      assert(await opener.evaluate(element => document.activeElement === element))
      await opener.click()
      await parse.press('Enter')
      const alert = dialog.getByRole('alert')
      await alert.waitFor()
      await page.evaluate(() => window.__qaFinishImportParse({ files: [{ fileId: 'stale', name: '旧结果' }] }))
      assert(await alert.isVisible())
      assert(await parse.evaluate(element => document.activeElement === element))
      await parse.click()
      const list = page.getByRole('listbox', { name: '待转存文件' })
      const options = list.getByRole('option')
      const first = list.getByRole('option', { name: /资料-00000/ })
      const last = list.getByRole('option', { name: /资料-09999/ })
      await first.waitFor()
      assert(await list.evaluate(element => document.activeElement === element), '解析后应聚焦文件清单')
      const firstCount = await options.count()
      await list.press('Control+a')
      await page.getByText('10000 个项目 · 已选 10000 个', { exact: true }).waitFor()
      await list.press('End')
      await last.waitFor()
      assert.equal(await list.getAttribute('aria-activedescendant'), await last.getAttribute('id'))
      await list.press('Space')
      assert.equal(await last.getAttribute('aria-selected'), 'false')
      await list.press('Home')
      await first.waitFor()
      assert.equal(await first.getAttribute('aria-selected'), 'true')
      await list.hover()
      await page.mouse.wheel(0, 175000)
      await page.waitForFunction(() => {
        const list = document.querySelector('.import-file-list')
        return list.scrollTop > 100000 && Number(list.querySelector('[role=option]')?.getAttribute('aria-posinset')) > 1
      })
      const middleCount = await options.count()
      assert(Math.max(firstCount, middleCount) <= 18, '滚动时只应渲染视口附近的文件')
      assert.notEqual(await options.first().getAttribute('aria-posinset'), '1')
      await list.press('End')
      await last.waitFor()
      assert.equal(await last.getAttribute('aria-selected'), 'false')
      const checkmark = last.locator('.import-file-check svg')
      assert.notEqual(await checkmark.evaluate(element => getComputedStyle(element).transitionDuration), '0s')
      await page.emulateMedia({ reducedMotion: 'reduce' })
      assert.equal(await checkmark.evaluate(element => getComputedStyle(element).transitionProperty), 'none')
      await page.getByRole('button', { name: '导入所选项目', exact: true }).click()
      await list.waitFor({ state: 'detached' })
      const ids = await page.evaluate(() => window.__qaCalls.find(call => call.method === 'SaveImportedShareDetailed')?.args[3])
      assert.equal(ids.length, 9999)
      assert.equal(ids[0], 'file-0')
      assert.equal(ids.at(-1), 'file-9998')
      assert.deepEqual(errors, [])
      console.log(JSON.stringify({ passed: true, files: 10000, renderedAtStart: firstCount, renderedDuringScroll: middleCount, submitted: ids.length, reducedMotion: true, staleParseIgnored: true, imeEnterSafe: true }, null, 2))
      return
    }
    if (process.env.QA_STARTUP_ONLY === '1') {
      await page.emulateMedia({ colorScheme: 'light' })
      assert(await page.evaluate(() => typeof window.__qaFinishSettings === 'function' && window.__qaListenerCount('nav:tab') > 0))
      await page.evaluate(() => window.runtime.EventsEmit('nav:tab', 'transfer'))
      const activeTab = page.locator('.top-tabs [aria-current="page"]')
      await page.waitForFunction(() => document.querySelector('.top-tabs [aria-current="page"]')?.textContent.includes('传输'))
      await page.evaluate(() => window.__qaFinishSettings({ theme: 'dark', defaultTab: 'share', autoUpdate: false }))
      await page.waitForFunction(() => document.documentElement.classList.contains('dark'))
      assert.equal(await activeTab.innerText(), '传输', '迟到的默认页面不能覆盖启动期间的主动导航')
      await page.reload()
      await page.locator('.fileitem').first().waitFor()
      await page.evaluate(() => window.__qaFinishSettings({ theme: 'light', defaultTab: 'sync', autoUpdate: false }))
      await page.waitForFunction(() => document.querySelector('.top-tabs [aria-current="page"]')?.textContent.includes('同步'))
      assert.deepEqual(errors, [])
      console.log(JSON.stringify({ passed: true, accountsBeforeSettings: true, navigationDuringStartup: true, defaultTabApplied: true }, null, 2))
      return
    }
    if (process.env.QA_SETTINGS_SCROLL_ONLY === '1') {
      await page.setViewportSize({ width: 1024, height: 680 })
      await page.getByTitle('设置 (Alt+5)').click()
      const content = page.locator('.settings-body')
      const tabs = page.locator('.settings-nav .sn-item')
      await page.locator('#sg-update').waitFor()
      for (const zoom of [1, 1.5]) {
        await page.evaluate(scale => { document.documentElement.style.zoom = scale }, zoom)
        await content.evaluate(root => root.scrollTo({ top: 0, behavior: 'instant' }))
        await tabs.last().click()
        const active = await content.evaluate(root => new Promise(resolve => {
          const labels = []
          const started = performance.now()
          const sample = () => {
            labels.push(document.querySelector('.settings-nav [aria-current="location"]').textContent)
            if (root.scrollTop >= root.scrollHeight - root.clientHeight - 1 || performance.now() - started > 2000) resolve(labels)
            else requestAnimationFrame(sample)
          }
          requestAnimationFrame(sample)
        }))
        assert(active.every(label => label.includes('关于与更新')), '跳转不能高亮途经分类')
        assert(await content.evaluate(root => root.scrollTop >= root.scrollHeight - root.clientHeight - 1), '需滚到末尾')
        await page.emulateMedia({ reducedMotion: 'reduce' })
        await tabs.nth(2).click()
        const offset = await content.evaluate(root => {
          const section = root.querySelector('#sg-transfer').getBoundingClientRect()
          return section.top - root.getBoundingClientRect().top
        })
        assert(Math.abs(offset - 20 * zoom) < 2, `跳转后保留标题间距：${offset}`)
        await page.emulateMedia({ reducedMotion: 'no-preference' })
      }
      assert.deepEqual(errors, [])
      console.log(JSON.stringify({ passed: true, zoomLevels: [1, 1.5] }, null, 2))
      return
    }
    if (process.env.QA_POPUP_PLACEMENT_ONLY === '1') {
      await page.setViewportSize({ width: 1024, height: 680 })
      await page.emulateMedia({ reducedMotion: 'reduce' })
      const checked = []
      for (const zoom of [1, 1.25, 1.5]) {
        await page.evaluate(scale => { document.documentElement.style.zoom = scale }, zoom)
        await page.getByRole('button', { name: '网盘', exact: true }).click()
        const row = page.locator('.fileitem').first()
        await row.waitFor()
        const openMenu = (clientX, clientY) => row.evaluate((el, point) => el.dispatchEvent(new MouseEvent('contextmenu', { ...point, button: 2, bubbles: true, cancelable: true })), { clientX, clientY })
        await openMenu(480, 100)
        const menu = page.getByRole('menu', { name: '操作菜单', exact: true })
        await menu.waitFor()
        const menuBounds = await menu.boundingBox()
        assert(Math.abs(menuBounds.x - 480) < 2 && Math.abs(menuBounds.y - 100) < 2, `右键菜单应贴近鼠标：${JSON.stringify(menuBounds)}`)
        await page.keyboard.press('Escape')
        await menu.waitFor({ state: 'detached' })
        await openMenu(1018, 677)
        await menu.waitFor()
        const edge = await menu.boundingBox()
        assert(edge.x + edge.width <= 1024 && edge.y + edge.height <= 680, '右键菜单不能越过窗口边缘')
        await page.keyboard.press('Escape')
        await page.getByTitle('设置 (Alt+5)').click()
        const select = page.getByRole('combobox', { name: '默认排序', exact: true })
        await select.click()
        const popup = page.getByRole('listbox', { name: '默认排序', exact: true })
        await popup.waitFor()
        const anchor = await select.boundingBox()
        const drop = await popup.boundingBox()
        const up = await popup.getAttribute('data-placement') === 'up'
        const gap = up ? anchor.y - drop.y - drop.height : drop.y - anchor.y - anchor.height
        assert(Math.abs(gap - 4 * zoom) < 2, `选择器必须贴着触发器：${gap}`)
        assert(drop.y >= 0 && drop.y + drop.height <= 680, '下拉选项不能越过窗口边缘')
        await page.keyboard.press('Escape')
        await popup.waitFor({ state: 'detached' })
        assert(await select.evaluate(el => document.activeElement === el), '关闭选择器后焦点必须回到触发器')
        checked.push(zoom)
      }
      await page.setViewportSize({ width: 900, height: 290 })
      await page.getByRole('button', { name: '网盘', exact: true }).click()
      await page.locator('.fileitem').first().evaluate(el => el.dispatchEvent(new MouseEvent('contextmenu', { clientX: 840, clientY: 250, button: 2, bubbles: true, cancelable: true })))
      const compactMenu = page.getByRole('menu', { name: '操作菜单', exact: true })
      await compactMenu.waitFor()
      const compactBounds = await compactMenu.boundingBox()
      assert(compactBounds.y + compactBounds.height <= 290, `短窗口放大后菜单不能超出边缘：${JSON.stringify(compactBounds)}`)
      await page.keyboard.press('End')
      const lastItem = compactMenu.locator('[role="menuitem"]:enabled').last()
      assert(await lastItem.evaluate(element => {
        const menu = element.closest('[role="menu"]')
        return element === document.activeElement && element.getBoundingClientRect().bottom <= menu.getBoundingClientRect().bottom
      }), 'End 应把末项滚动到菜单可见范围内')
      assert.deepEqual(errors, [])
      console.log(JSON.stringify({ passed: true, zoomLevels: checked }, null, 2))
      return
    }
    if (process.env.QA_RENAME_PREVIEW_ONLY === '1') {
      await page.locator('.file-list').press('Control+a')
      await page.locator('.fileitem').first().click({ button: 'right' })
      const rename = page.getByRole('menuitem', { name: '批量重命名 (10000)', exact: true })
      await rename.click({ timeout: 5000 })
      const dialog = page.getByRole('dialog', { name: '批量重命名', exact: true })
      await dialog.getByPlaceholder('查找内容').fill('资料')
      await dialog.getByPlaceholder('替换为').fill('报告')
      const preview = dialog.getByRole('region', { name: '重命名预览' })
      await preview.getByText('报告-00000.txt', { exact: true }).waitFor()
      await preview.evaluate(root => { root.scrollTop = root.scrollHeight })
      await preview.getByText('报告-09999.txt', { exact: true }).waitFor()
      const visible = await preview.locator('.rr-row.changed').count()
      assert(visible <= 16)
      await dialog.getByText('10000 / 10000 项将重命名').waitFor()
      assert.deepEqual(errors, [])
      console.log(JSON.stringify({ passed: true, files: 10000, renderedRowsAtEnd: visible }, null, 2))
      return
    }
    if (process.env.QA_FILE_KEYS_ONLY === '1') {
      const verifyNavigation = async selector => {
        const list = page.locator(selector)
        const waitFocus = pos => page.waitForFunction(({ selector, pos }) => {
          const viewport = document.querySelector(selector)
          const row = document.getElementById(viewport?.getAttribute('aria-activedescendant'))
          if (!row || Number(row.getAttribute('aria-posinset')) !== pos) return false
          const bounds = viewport.getBoundingClientRect()
          const item = row.getBoundingClientRect()
          return item.top >= bounds.top - 1 && item.bottom <= bounds.bottom + 1
        }, { selector, pos }, { timeout: 5000 }).catch(async error => {
          const state = await list.evaluate(viewport => {
            const active = document.getElementById(viewport.getAttribute('aria-activedescendant'))
            return {
              focusedPosition: active?.getAttribute('aria-posinset'),
              scrollTop: viewport.scrollTop,
              scrollHeight: viewport.scrollHeight,
              viewport: viewport.getBoundingClientRect().toJSON(),
              item: active?.getBoundingClientRect().toJSON(),
            }
          })
          throw new Error(`${selector} 应滚动到第 ${pos} 项：${JSON.stringify(state)}`, { cause: error })
        })
        await list.press('Home')
        await waitFocus(1)
        await list.press('Shift+End')
        await waitFocus(10000)
        await page.getByText('已选 10000 项', { exact: true }).waitFor()
        await list.press('Shift+ArrowLeft')
        await waitFocus(9999)
        await page.getByText('已选 9999 项', { exact: true }).waitFor()
        await list.press('Escape')
        await page.getByText('已选 9999 项', { exact: true }).waitFor({ state: 'detached' })
      }
      await verifyNavigation('.file-list:not(.gridlist)')
      await page.getByTitle('网格视图', { exact: true }).click()
      await verifyNavigation('.file-list.gridlist')
      assert.deepEqual(errors, [])
      console.log(JSON.stringify({ passed: true, files: 10000, layouts: ['list', 'grid'] }, null, 2))
      return
    }
    if (process.env.QA_COLUMNS_KEYBOARD_ONLY === '1') {
      await page.getByTitle('分栏视图', { exact: true }).click()
      const columns = page.locator('.file-columns')
      const root = columns.getByRole('listbox').first()
      const activeOption = box => box.locator('[data-active=true]')
      await root.getByRole('option', { name: '项目1', exact: true }).waitFor()
      await root.press('End')
      assert.equal(await activeOption(root).getAttribute('aria-posinset'), '10003')
      assert.equal(await root.getAttribute('aria-activedescendant'), await activeOption(root).getAttribute('id'))
      assert(await root.getByRole('option').count() < 100)
      await root.press('Space')
      assert.equal(await activeOption(root).getAttribute('aria-selected'), 'true')
      await root.press('Home')
      await root.press('Space')
      assert.equal(await columns.getByRole('listbox').count(), 1, '空格只选择目录，不打开目录')
      await root.press('Enter')
      const child = columns.getByRole('listbox', { name: '项目1' })
      await child.getByRole('option', { name: '会议资料', exact: true }).waitFor()
      assert(await child.evaluate(element => document.activeElement === element), '键盘打开文件夹后应聚焦新分栏')
      await page.keyboard.press('Shift+Tab')
      assert(await root.evaluate(element => document.activeElement === element), '每列应只占用一个 Tab 停靠点')
      await page.keyboard.press('Tab')
      await page.keyboard.press('Backspace')
      await child.waitFor({ state: 'detached' })
      assert(await root.evaluate(element => document.activeElement === element), '返回上级后仍应保留分栏焦点')
      assert.deepEqual(errors, [])
      console.log(JSON.stringify({ passed: true, files: 10003, lastFileReachable: true, childFocus: true, tabStopsPerColumn: 1 }, null, 2))
      return
    }
    if (process.env.QA_FILE_SCROLL_ONLY === '1') {
      const list = '.file-list:not(.gridlist)'
      const grid = '.file-list.gridlist'
      const waitTop = (selector, top) => page.waitForFunction(({ selector, top }) => Math.abs((document.querySelector(selector)?.scrollTop ?? -100) - top) <= 1, { selector, top })
      const scrollTo = async (selector, top) => {
        await page.locator(selector).evaluate(async (element, value) => {
          element.scrollTop = value
          await new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)))
        }, top)
        await waitTop(selector, top)
      }
      await scrollTo(list, 144000)
      const refresh = page.getByTitle('刷新 (F5)', { exact: true })
      const readListAnchor = () => page.locator(list).evaluate(element => {
        const top = element.getBoundingClientRect().top
        const row = Array.from(element.querySelectorAll('.fileitem')).find(entry => entry.getBoundingClientRect().bottom > top)
        return { name: row.querySelector('.filename').textContent.trim(), offset: row.getBoundingClientRect().top - top }
      })
      const listAnchor = await readListAnchor()
      await page.evaluate(() => { window.__qaRootPrefix = [{ file_id: 'insert-list', name: '000 新目录', isDir: true }] })
      await refresh.click()
      await waitTop(list, 144048)
      assert.deepEqual(await readListAnchor(), listAnchor, '列表插入一行后应保留阅读位置')
      await page.evaluate(() => { window.__qaRootPrefix = [] })
      await refresh.click()
      await waitTop(list, 144000)
      assert.deepEqual(await readListAnchor(), listAnchor, '列表删除一行后应保留阅读位置')
      await page.getByTitle('网格视图', { exact: true }).click()
      await waitTop(grid, 0)
      const gridViewport = page.locator(grid)
      const gridExtent = () => gridViewport.evaluate(element => element.scrollHeight)
      await gridViewport.locator('.griditem').first().waitFor()
      const initialExtent = await gridExtent()
      await scrollTo(grid, 22000)
      assert.equal(await gridExtent(), initialExtent, '混合长短文件名时，虚拟网格的滚动高度不能随渲染窗口变化')
      const readGridAnchor = () => gridViewport.evaluate(element => {
        const top = element.getBoundingClientRect().top
        const card = Array.from(element.querySelectorAll('.griditem')).find(entry => entry.getBoundingClientRect().bottom > top)
        return { name: card.querySelector('.gridname').textContent.trim(), offset: card.getBoundingClientRect().top - top }
      })
      const originalAnchor = await readGridAnchor()
      await gridViewport.evaluate(element => {
        const columns = getComputedStyle(element).gridTemplateColumns.split(' ').length
        window.__qaRootPrefix = Array.from({ length: columns }, (_, index) => ({ file_id: `insert-${index}`, name: `000 新目录 ${index}`, isDir: true }))
      })
      await refresh.click()
      await page.waitForFunction(previous => document.querySelector('.grid-virtual').scrollHeight > previous, initialExtent)
      assert.deepEqual(await readGridAnchor(), originalAnchor, '在视口上方插入一行后，应保留正在阅读的文件及其偏移')
      await page.evaluate(() => { window.__qaRootPrefix = [] })
      await refresh.click()
      await page.waitForFunction(previous => document.querySelector('.grid-virtual').scrollHeight === previous, initialExtent)
      assert.deepEqual(await readGridAnchor(), originalAnchor, '删除视口上方一行后，应保留正在阅读的位置')
      await page.getByTitle('列表视图', { exact: true }).click()
      await waitTop(list, 144000)
      await page.getByTitle('网格视图', { exact: true }).click()
      await waitTop(grid, 22000)
      await page.locator('.pan-left .tn-label').getByText('项目1', { exact: true }).click()
      await page.locator('.griditem').getByText('会议资料', { exact: true }).waitFor()
      await waitTop(grid, 0)
      await page.locator('.pan-left').getByText('全部文件', { exact: true }).click()
      await waitTop(grid, 22000)
      assert(await page.locator('.griditem').count() < 300, '恢复到目录深处时仍须限制节点数量')
      const visibleFile = () => gridViewport.evaluate(element => {
        const viewport = element.getBoundingClientRect()
        return Array.from(element.querySelectorAll('.griditem')).find(card => card.getBoundingClientRect().bottom > viewport.top + 1)?.getAttribute('aria-posinset')
      })
      const anchor = await visibleFile()
      assert(anchor, '缩放前应记录当前可见文件')
      const anchorVisible = () => gridViewport.locator(`.griditem[aria-posinset="${anchor}"]`).evaluateAll(cards => cards.some(card => {
        const viewport = card.parentElement.getBoundingClientRect()
        const bounds = card.getBoundingClientRect()
        return bounds.bottom > viewport.top + 1 && bounds.top < viewport.bottom
      }))
      await page.setViewportSize({ width: 840, height: 720 })
      await page.waitForFunction(previous => document.querySelector('.grid-virtual')?.scrollHeight > previous, initialExtent)
      assert(await anchorVisible(), '窗口变窄时应保留原来可见的文件')
      await page.setViewportSize({ width: 1440, height: 1000 })
      await page.waitForFunction(previous => document.querySelector('.grid-virtual')?.scrollHeight === previous, initialExtent)
      assert(await anchorVisible(), '窗口恢复宽度时应保留原来可见的文件')
      const divider = page.locator('.pan-resizer')
      const dividerBounds = await divider.boundingBox()
      const dividerX = dividerBounds.x + dividerBounds.width / 2
      const dividerY = dividerBounds.y + dividerBounds.height / 2
      await page.mouse.move(dividerX, dividerY)
      await page.mouse.down()
      await page.mouse.move(dividerX + 195, dividerY, { steps: 8 })
      await page.mouse.up()
      await page.waitForFunction(previous => document.querySelector('.grid-virtual')?.scrollHeight > previous, initialExtent)
      assert(await anchorVisible(), '拖动目录树分隔线时应保留原来可见的文件')
      await divider.press('Home')
      await page.waitForFunction(previous => document.querySelector('.grid-virtual')?.scrollHeight === previous, initialExtent)
      await scrollTo(grid, initialExtent - await gridViewport.evaluate(element => element.clientHeight))
      const lastCard = gridViewport.locator('.griditem').last()
      assert.equal(await lastCard.getAttribute('aria-posinset'), await lastCard.getAttribute('aria-setsize'), '滚到底部时最后一个文件必须出现')
      assert.equal(await gridExtent(), initialExtent, '滚到底部不能改变滚动高度')
      await page.setViewportSize({ width: 840, height: 720 })
      await page.waitForFunction(previous => document.querySelector('.grid-virtual')?.scrollHeight > previous, initialExtent)
      const narrowExtent = await gridExtent()
      await waitTop(grid, narrowExtent - await gridViewport.evaluate(element => element.clientHeight))
      await gridViewport.press('Home')
      await waitTop(grid, 0)
      await gridViewport.press('End')
      const lastOptionId = await gridViewport.getAttribute('aria-activedescendant')
      assert(lastOptionId && lastOptionId === await lastCard.getAttribute('id'), '改变列数后仍可用 End 聚焦最后一个文件')
      assert.equal(await gridExtent(), narrowExtent, '窄窗下切换渲染窗口不能改变滚动高度')
      assert.deepEqual(errors, [])
      console.log(JSON.stringify({ passed: true, listTop: 144000, gridTop: 22000, newFolderTop: 0, initialExtent, narrowExtent, resizeAnchor: anchor, lastFileReachable: true }, null, 2))
      return
    }
    if (process.env.QA_FILE_PREVIEW_ONLY === '1') {
      const ready = page.locator('.fileitem').filter({ hasText: '资料-00000.txt' }).locator('.file-thumbnail img.ready')
      await ready.waitFor()
      const broken = page.locator('.fileitem').filter({ hasText: '资料-00001.txt' }).locator('.file-thumbnail')
      await broken.locator('img').waitFor({ state: 'detached' })
      await broken.locator('.file-thumbnail-placeholder:not(.ready)').waitFor()
      const node = page.locator('.pan-left .tn-label').getByText('项目1', { exact: true })
      const preview = page.locator('.folder-preview')
      await node.hover()
      await preview.getByText('会议资料', { exact: true }).waitFor()
      await page.mouse.move(1000, 500)
      await preview.waitFor({ state: 'detached' })
      await node.hover()
      await preview.getByText('会议资料', { exact: true }).waitFor()
      const calls = await page.evaluate(() => window.__qaCalls.filter(call => call.method === 'ListDir' && call.args[2] === 'folder-1').length)
      assert.equal(calls, 1, '再次悬停同一目录必须复用读取结果')
      await page.locator('.pan-left').dispatchEvent('scroll')
      await preview.waitFor({ state: 'detached' })
      assert.deepEqual(errors, [])
      console.log(JSON.stringify({ passed: true, folderRequests: calls, thumbnailFallback: true, previewClosesOnScroll: true }, null, 2))
      return
    }
    if (process.env.QA_THEME_ONLY === '1') {
      assert.deepEqual(await page.locator('.top-tabs button').allTextContents(), ['网盘', '传输', '同步', '分享'])
      assert.equal(await page.locator('.top-tabs [aria-current="page"]').innerText(), '网盘')
      await page.getByRole('textbox', { name: '搜索网盘文件', exact: true }).fill('资料')
      await page.getByRole('textbox', { name: '搜索网盘文件', exact: true }).press('Enter')
      await page.locator('.workspace-result').first().waitFor()
      assert.equal(await page.getByRole('textbox', { name: '跨盘搜索名称' }).inputValue(), '资料')
      await page.getByRole('button', { name: '关闭对话框', exact: true }).click()
      await page.getByTitle('分栏视图', { exact: true }).click()
      assert(await page.locator('.column-file').count() < 100, '分栏大目录必须使用虚拟列表')
      assert.deepEqual((await page.locator('.column-file').allTextContents()).slice(0, 3).map(text => text.trim()), ['项目1', '项目2', '项目10'])
      await page.locator('.column-file').filter({ hasText: '项目2' }).click()
      await page.locator('.file-column').nth(1).getByTitle('会议资料', { exact: true }).waitFor()
      await page.locator('.file-column').nth(1).getByTitle('会议资料', { exact: true }).click()
      await page.locator('.file-column').nth(2).waitFor()
      await page.screenshot({ path: path.join(output, 'columns-dark.png') })
      await page.locator('.file-column').first().getByTitle('项目10', { exact: true }).click()
      await page.waitForFunction(() => document.querySelectorAll('.file-column').length === 2 && document.querySelector('.file-column:last-child .column-heading').textContent.includes('项目10'))
      await page.locator('.pathbar .crumb').first().click()
      await page.waitForFunction(() => document.querySelectorAll('.file-column').length === 1)
      const list = page.locator('.column-list')
      await list.evaluate(element => { element.scrollTop = 38000 })
      await page.waitForFunction(() => document.querySelector('.column-file')?.textContent.includes('009'))
      assert(await page.locator('.column-file').count() < 100, '滚动不能渲染整个目录')
      await list.evaluate(element => { element.scrollTop = 0 })
      await page.locator('.rail-list .rail-icon').nth(1).click()
      await page.waitForFunction(() => document.querySelectorAll('.file-column').length === 1 && document.querySelector('.column-file')?.textContent.includes('项目1'))
      const themes = []
      for (const style of ['paper', 'mono']) {
        for (const mode of ['light', 'dark']) {
          await page.getByTitle('设置 (Alt+5)').click()
          await page.locator(`.theme-style-card[data-style="${style}"]`).click()
          await page.getByRole('button', { name: mode === 'light' ? '浅色' : '深色', exact: true }).click()
          await page.waitForFunction(({ style, mode }) => document.documentElement.dataset.theme === style && document.documentElement.classList.contains('dark') === (mode === 'dark'), { style, mode })
          await page.getByRole('button', { name: '网盘', exact: true }).click()
          await page.screenshot({ path: path.join(output, `${style}-${mode}.png`) })
          const colors = await page.evaluate(() => {
            const css = getComputedStyle(document.documentElement)
            return { bg: css.getPropertyValue('--bg-base').trim(), fg: css.getPropertyValue('--text-primary').trim(), button: css.getPropertyValue('--color-primary-strong').trim(), buttonText: css.getPropertyValue('--text-on-primary').trim() }
          })
          themes.push({ style, mode, ...colors })
        }
      }
      await page.getByTitle('设置 (Alt+5)').click()
      await page.screenshot({ path: path.join(output, 'theme-settings.png') })
      await page.getByRole('button', { name: '跟随系统', exact: true }).click()
      await page.emulateMedia({ colorScheme: 'light' })
      await page.waitForFunction(() => !document.documentElement.classList.contains('dark'))
      await page.emulateMedia({ colorScheme: 'dark' })
      await page.waitForFunction(() => document.documentElement.classList.contains('dark'))
      await page.getByRole('button', { name: '传输', exact: true }).click()
      await page.locator('.taskrow').first().waitFor()
      await page.waitForFunction(() => Number(getComputedStyle(document.querySelector('.down-content')).opacity) === 1)
      await page.screenshot({ path: path.join(output, 'transfer-dark.png') })
      await page.getByRole('tab', { name: /正在上传/ }).click()
      await page.getByText('计算哈希', { exact: true }).waitFor()
      await page.getByText('等待网盘确认', { exact: true }).waitFor()
      await page.waitForFunction(() => Number(getComputedStyle(document.querySelector('.down-content')).opacity) === 1)
      assert.equal(await page.locator('.taskrow details').count(), 0, '错误详情必须使用统一通知')
      await page.screenshot({ path: path.join(output, 'upload-dark.png') })
      const layouts = []
      for (const width of [1440, 1024, 800, 680]) {
        await page.setViewportSize({ width, height: 768 })
        const clipped = await page.locator('.topbar button, .topbar input').evaluateAll(elements => elements.filter(element => { const rect = element.getBoundingClientRect(); return rect.width > 0 && (rect.left < 0 || rect.right > innerWidth) }).map(element => element.outerHTML))
        assert.deepEqual(clipped, [], `标题栏控件被裁剪: ${width}`)
        assert(await page.locator('.top-tabs .active').count() === 1, '导航必须标识当前页面')
        const filename = await page.locator('.taskrow .filename').first().boundingBox()
        assert(filename.width >= 100, `窄窗口必须保留文件名：${width}`)
        layouts.push(width)
      }
      await page.screenshot({ path: path.join(output, 'navigation-680.png') })
      assert.deepEqual(errors, [], '主题与分栏验收不能出现页面异常')
      console.log(JSON.stringify({ passed: true, themes, layouts, errors, output }, null, 2))
      return
    }
    if (process.env.QA_SYNC_PREVIEW_ONLY === '1') {
      await page.evaluate(() => {
        let attempts = 0
        window.go.app.App.PreviewSync = async () => {
          attempts++
          if (attempts === 1) return new Promise(resolve => { window.__qaFinishPreview = resolve })
          if (attempts === 2) throw new Error('连接中断')
          return { token: 'large-plan', changes: Array.from({ length: 10_000 }, (_, index) => ({
            path: `同步资料-${index}.txt`, action: index === 0 || index === 9999 ? 'conflict' : 'upload', local: { size: 10 }, remote: { size: 20 },
          })) }
        }
      })
      await page.getByRole('button', { name: '同步', exact: true }).click()
      const opener = page.getByTitle('预览同步')
      await opener.click()
      const dialog = page.getByRole('dialog', { name: '同步预览' })
      await dialog.getByText('正在比较本地与网盘文件…').waitFor()
      assert.equal(await dialog.getByText(/无删除操作/).count(), 0)
      assert(await dialog.getByRole('button', { name: '执行', exact: true }).isDisabled())
      await page.keyboard.press('Escape')
      await dialog.waitFor({ state: 'detached' })
      assert(await opener.evaluate(element => document.activeElement === element))
      await opener.click()
      await dialog.getByText(/无法加载同步预览/).waitFor()
      await page.evaluate(() => window.__qaFinishPreview({ token: 'stale-plan', changes: [] }))
      assert.equal(await dialog.getByText(/无删除操作/).count(), 0, '已关闭的预览响应不能覆盖当前失败状态')
      await dialog.getByRole('button', { name: '重试预览' }).press('Enter')
      const list = dialog.getByRole('region', { name: '同步差异' })
      const next = dialog.getByRole('button', { name: '下一页' })
      await list.getByText('同步资料-0.txt', { exact: true }).waitFor()
      assert(await dialog.getByRole('button', { name: '刷新', exact: true }).evaluate(element => document.activeElement === element))
      assert.equal(await list.getByRole('listitem').count(), 40)
      await dialog.getByRole('combobox', { name: '冲突策略：同步资料-0.txt' }).click()
      await page.getByRole('option', { name: '使用本地', exact: true }).click()
      await next.press('Enter')
      await list.getByText('同步资料-40.txt', { exact: true }).waitFor()
      assert(await next.evaluate(element => document.activeElement === element), '翻页应保留键盘焦点')
      await dialog.getByRole('combobox', { name: '筛选同步操作' }).click()
      await page.getByRole('option', { name: '冲突', exact: true }).click()
      assert.equal(await list.getByRole('listitem').count(), 2)
      await dialog.getByRole('textbox', { name: '筛选同步路径' }).fill('9999.txt')
      await list.getByText('同步资料-9999.txt', { exact: true }).waitFor()
      assert.equal(await list.getByRole('listitem').count(), 1)
      await dialog.getByRole('combobox', { name: '冲突策略：同步资料-9999.txt' }).click()
      await page.getByRole('option', { name: '使用网盘', exact: true }).click()
      await dialog.getByRole('button', { name: '执行', exact: true }).click()
      await dialog.waitFor({ state: 'detached' })
      const call = await page.evaluate(() => window.__qaCalls.find(item => item.method === 'RunSyncPlan')?.args)
      assert.deepEqual(call, ['sync1', 'large-plan', { '同步资料-0.txt': 'local', '同步资料-9999.txt': 'remote' }])
      assert.deepEqual(errors, [])
      console.log(JSON.stringify({ passed: true, changes: 10_000, renderedPerPage: 40, choicesPersisted: true, keyboardFocusPreserved: true, stalePreviewIgnored: true, inlineRetry: true }, null, 2))
      return
    }
    if (process.env.QA_SYNC_LIFECYCLE_ONLY === '1') {
      const jump = name => page.getByRole('button', { name, exact: true }).click()
      await jump('同步')
      await page.locator('.sync-task').waitFor()
      assert.equal(await page.evaluate(() => window.__qaListenerCount('sync:progress')), 1)
      await page.evaluate(() => {
        window.runtime.EventsEmit('sync:state', { id: 'sync1', running: true })
        for (let done = 1; done <= 500; done++) window.runtime.EventsEmit('sync:progress', { id: 'sync1', done, total: 600 })
      })
      await page.getByText('500 / 600（83%）', { exact: true }).waitFor()
      await jump('网盘')
      await page.waitForFunction(() => window.__qaListenerCount('sync:progress') === 0 && window.__qaListenerCount('sync:state') === 0)
      await page.evaluate(() => {
        window.runtime.EventsEmit('sync:progress', { id: 'sync1', done: 550, total: 600 })
        window.go.app.App.ListRunningSyncIDs = async () => ['sync1']
      })
      await jump('同步')
      await page.getByText('同步中，正在等待进度…', { exact: true }).waitFor()
      assert.equal(await page.evaluate(() => window.__qaListenerCount('sync:progress')), 1)
      const row = page.locator('.sync-task')
      const stop = page.getByTitle('停止同步')
      await stop.focus()
      const waitingHeight = await row.evaluate(element => element.offsetHeight)
      const transition = await row.evaluate(element => new Promise(resolve => {
        window.runtime.EventsEmit('sync:progress', { id: 'sync1', done: 565, total: 600 })
        requestAnimationFrame(() => requestAnimationFrame(() => {
          resolve(element.querySelector('.progress-current').getAnimations().some(animation => animation.transitionProperty === 'transform'))
        }))
      }))
      await page.getByText('565 / 600（94%）', { exact: true }).waitFor()
      assert(transition, '首条同步进度应平滑填充')
      assert.equal(await row.evaluate(element => element.offsetHeight), waitingHeight, '首条进度不能改变卡片高度')
      assert(await stop.evaluate(element => document.activeElement === element), '同步进度不能抢占键盘焦点')
      await page.evaluate(() => {
        window.runtime.EventsEmit('sync:progress', { id: 'sync1', done: 600, total: 600 })
        window.runtime.EventsEmit('sync:state', { id: 'sync1', running: false, status: 'completed' })
      })
      await page.locator('.sync-task-progress').waitFor({ state: 'detached' })
      await page.getByTitle('预览同步').waitFor()
      assert.deepEqual(errors, [])
      console.log(JSON.stringify({ passed: true, progressEvents: 500, hiddenSubscribers: 0, visibleSubscribers: 1, firstProgressAnimated: transition, cardHeightStable: true, keyboardFocusPreserved: true }, null, 2))
      return
    }
    if (process.env.QA_TRANSFER_OFFLINE_ONLY === '1') {
      await page.evaluate(() => {
        const getTask = progress => ({ task_id: 'offline-1', file_name: '离线资料', drive_id: 'drive-0', status: 'running', progress })
        window.go.app.App.ListOfflineTasks = async () => [getTask(10)]
        let initial = true
        window.go.app.App.RefreshOfflineTasks = async () => {
          if (initial) { initial = false; return [getTask(60)] }
          return new Promise(resolve => { window.__qaFinishOffline = () => resolve([getTask(80)]) })
        }
      })
      await page.getByRole('button', { name: '传输', exact: true }).click()
      await page.getByRole('tab', { name: /云离线/ }).click()
      const bar = page.getByRole('progressbar', { name: '离线资料：进行中' })
      const refresh = page.locator('.offline-refresh')
      await page.waitForFunction(() => document.querySelector('[role=progressbar]')?.getAttribute('aria-valuenow') === '60')
      await refresh.press('Enter')
      await refresh.press('Enter')
      assert.equal(await refresh.getAttribute('aria-busy'), 'true')
      assert.equal(await bar.getAttribute('aria-valuenow'), '60')
      await page.waitForFunction(() => typeof window.__qaFinishOffline === 'function')
      const motion = await bar.evaluate(element => new Promise(resolve => {
        window.__qaFinishOffline()
        requestAnimationFrame(() => requestAnimationFrame(() => resolve(element.querySelector('.progress-current').getAnimations().some(animation => animation.transitionProperty === 'transform'))))
      }))
      assert(motion, '云离线进度应平滑推进')
      assert.equal(await bar.getAttribute('aria-valuenow'), '80')
      assert.equal(await refresh.getAttribute('aria-busy'), 'false')
      assert(await refresh.evaluate(element => document.activeElement === element), '刷新后应保留键盘焦点')
      assert.deepEqual(errors, [])
      console.log(JSON.stringify({ passed: true, keptProgress: 60, refreshedProgress: 80, progressMotion: motion, keyboardFocusPreserved: true }, null, 2))
      return
    }
    if (process.env.QA_TRANSFER_MIGRATE_ONLY === '1') {
      await page.evaluate(() => {
        window.go.app.App.ListMigrateJobs = async () => Array.from({ length: 30 }, (_, index) => ({
          id: `migration-${index}`, srcUser: `webdav:account-${index % 20}`, dstUser: 'aliopen_one',
          dstParent: 'root', status: 'running', stage: 'uploading', total: 100, processed: 10,
        }))
      })
      await page.getByRole('button', { name: '传输', exact: true }).click()
      await page.getByRole('tab', { name: /迁移/ }).click()
      const rows = page.locator('.taskrow')
      const bar = rows.first().getByRole('progressbar')
      const details = rows.first().getByRole('button', { name: /详情/ })
      await bar.waitFor()
      assert.equal(await rows.count(), 30)
      await details.focus()
      const motion = await bar.evaluate(element => new Promise(resolve => {
        const job = { id: 'migration-0', srcUser: 'webdav:account-0', dstUser: 'aliopen_one', dstParent: 'root', status: 'running', stage: 'uploading', total: 100 }
        for (let processed = 11; processed <= 65; processed++) window.runtime.EventsEmit('migrate:progress', { ...job, processed })
        requestAnimationFrame(() => requestAnimationFrame(() => resolve(element.querySelector('.progress-current').getAnimations().some(animation => animation.transitionProperty === 'transform'))))
      }))
      assert(motion, '迁移总体进度应平滑推进')
      assert.equal(await bar.getAttribute('aria-valuenow'), '65')
      assert.equal(await rows.nth(1).getByRole('progressbar').getAttribute('aria-valuenow'), '10')
      assert(await details.evaluate(element => document.activeElement === element), '迁移进度不能抢占键盘焦点')
      await page.evaluate(() => {
        const items = Array.from({ length: 10_000 }, (_, index) => ({ id: `file-${index}`, name: `迁移资料-${index}.txt`, status: 'completed' }))
        window.go.app.App.VerifyMigration = async () => items.map(({ id }) => ({ id, status: id === 'file-9999' ? 'mismatch' : 'size', detail: id === 'file-9999' ? '哈希不一致' : '文件大小一致' }))
        window.runtime.EventsEmit('migrate:progress', { id: 'migration-0', srcUser: 'webdav:account-0', dstUser: 'aliopen_one', dstParent: 'root', status: 'completed', total: 100, processed: 100, items: Object.fromEntries(items.map(item => [item.id, item])) })
      })
      await details.press('Enter')
      const dialog = page.getByRole('dialog', { name: '迁移详情' })
      await dialog.waitFor()
      const files = dialog.getByRole('region', { name: '迁移文件记录' })
      const next = dialog.getByRole('button', { name: '下一页' })
      await files.getByText('迁移资料-0.txt', { exact: true }).waitFor()
      assert.equal(await files.getByRole('listitem').count(), 50)
      await files.evaluate(element => { element.scrollTop = element.scrollHeight })
      await next.press('Enter')
      await files.getByText('迁移资料-50.txt', { exact: true }).waitFor()
      assert.equal(await files.evaluate(element => element.scrollTop), 0)
      assert(await next.evaluate(element => document.activeElement === element), '翻页后应保留键盘焦点')
      await dialog.getByRole('textbox', { name: '按文件名筛选' }).fill('迁移资料-9999.txt')
      await files.getByText('迁移资料-9999.txt', { exact: true }).waitFor()
      assert.equal(await files.getByRole('listitem').count(), 1)
      await dialog.getByRole('button', { name: '校验文件完整性' }).press('Enter')
      const results = dialog.getByRole('region', { name: '迁移校验结果' })
      await results.waitFor()
      assert.equal(await results.getByRole('listitem').count(), 50)
      await dialog.getByRole('textbox', { name: '按文件名筛选' }).fill('迁移资料-9999.txt')
      await results.getByText('哈希不一致', { exact: true }).waitFor()
      assert.deepEqual(errors, [])
      console.log(JSON.stringify({ passed: true, jobs: 30, files: 10_000, renderedPerPage: 50, progressMotion: motion, keyboardFocusPreserved: true }, null, 2))
      return
    }
    if (process.env.QA_TRANSFER_UPLOAD_ONLY === '1') {
      await page.evaluate(() => {
        window.go.app.App.ListUploads = async () => Array.from({ length: 40 }, (_, index) => ({
          UploadID: `upload-${index}`, Info: { name: `上传资料 ${index}`, size: 1024 },
          Upload: { DownState: 'uploading', IsDowning: true, IsCompleted: false, issue: null, stage: 'uploading', DownProcess: 10 },
        }))
      })
      await page.getByRole('button', { name: '传输', exact: true }).click()
      await page.getByRole('tab', { name: /正在上传/ }).click()
      const first = page.getByRole('progressbar', { name: /上传资料 0：/ })
      const second = page.getByRole('progressbar', { name: /上传资料 1：/ })
      const cancel = page.locator('.taskrow').first().getByTitle('取消上传')
      await first.waitFor()
      await cancel.focus()
      const motion = await first.evaluate(element => new Promise(resolve => {
        for (let progress = 11; progress <= 75; progress++) {
          window.runtime.EventsEmit('transfer:event', { kind: 'upload', task: { id: 'upload-0', status: 'uploading', stage: 'uploading', progress, speed: 1024 } })
        }
        requestAnimationFrame(() => requestAnimationFrame(() => {
          const fill = element.querySelector('.progress-current')
          const scale = new DOMMatrixReadOnly(getComputedStyle(fill).transform).a
          resolve({ scale, transitioning: fill.getAnimations().some(animation => animation.transitionProperty === 'transform') })
        }))
      }))
      assert(motion.transitioning && motion.scale >= .1 && motion.scale < .75, '进度条应朝最新进度平滑推进')
      assert.equal(await first.getAttribute('aria-valuenow'), '75')
      assert.equal(await second.getAttribute('aria-valuenow'), '10')
      assert(await cancel.evaluate(element => document.activeElement === element), '实时进度不能抢占操作焦点')
      await page.emulateMedia({ reducedMotion: 'reduce' })
      await page.evaluate(() => window.runtime.EventsEmit('transfer:event', { kind: 'upload', task: { id: 'upload-0', status: 'uploading', stage: 'checking' } }))
      await page.getByRole('progressbar', { name: '上传资料 0：检查秒传' }).waitFor()
      assert.equal(await first.getAttribute('aria-valuenow'), null)
      const stage = page.locator('.taskrow').first().locator('.text-state .badge')
      assert.equal(await stage.evaluate(element => getComputedStyle(element).animationName), 'none')
      assert.deepEqual(errors, [])
      console.log(JSON.stringify({ passed: true, uploads: 40, progress: 75, animated: motion.transitioning, focusPreserved: true, reducedMotion: true }, null, 2))
      return
    }
    if (process.env.QA_TRANSFER_FOLDER_ONLY === '1') {
      await page.evaluate(() => {
        window.go.app.App.ListDownloads = async () => Array.from({ length: 20 }, (_, index) => {
          const id = `folder-${index}`
          return [{ id, name: `目录任务 ${index}`, isDir: true, status: 'downloading', progress: 10, scanDone: true, totalFiles: 140 },
            ...Array.from({ length: 140 }, (_, child) => ({ id: `${id}-${child}`, groupId: id, name: `文件 ${child}`, status: 'queued' }))]
        }).flat()
      })
      await page.getByRole('button', { name: '传输', exact: true }).click()
      const folders = page.locator('.folder-task-details')
      await folders.first().waitFor()
      assert.equal(await folders.count(), 20)
      const children = page.locator('.folder-task-child')
      assert.equal(await children.count(), 0, '折叠的目录不能渲染子任务')
      const select = page.getByRole('button', { name: '选择 目录任务 0', exact: true })
      await select.press('Space')
      assert.equal(await select.getAttribute('aria-pressed'), 'true')
      const bar = page.getByRole('progressbar', { name: '目录任务 0：下载中' })
      const progressMotion = await bar.evaluate(element => new Promise(resolve => {
        window.runtime.EventsEmit('transfer:event', { kind: 'download', task: { id: 'folder-0', status: 'downloading', progress: 65 } })
        requestAnimationFrame(() => requestAnimationFrame(() => {
          const fill = element.querySelector('.progress-current')
          resolve(fill.getAnimations().some(animation => animation.transitionProperty === 'transform'))
        }))
      }))
      assert(progressMotion, '下载进度条应平滑推进')
      assert.equal(await bar.getAttribute('aria-valuenow'), '65')
      assert(await select.evaluate(element => document.activeElement === element), '下载进度不能抢占键盘焦点')
      const summary = folders.first().locator('summary')
      await summary.press('Enter')
      await children.first().waitFor()
      assert.equal(await children.count(), 100, '仅展开目录的首批子任务应被渲染')
      await folders.first().getByRole('button', { name: '显示更多', exact: true }).click()
      assert.equal(await children.count(), 140)
      await page.evaluate(() => window.runtime.EventsEmit('transfer:event', { kind: 'download', task: { id: 'folder-0-0', status: 'downloading', progress: 45 } }))
      await children.first().getByText(/45%/).waitFor()
      await summary.press('Enter')
      await children.first().waitFor({ state: 'detached' })
      assert.equal(await children.count(), 0)
      assert.deepEqual(errors, [])
      console.log(JSON.stringify({ passed: true, folders: 20, children: 2800, renderedWhenClosed: 0, renderedWhenOpened: 100, renderedAfterShowMore: 140, progressMotion, keyboardFocusPreserved: true }, null, 2))
      return
    }
    if (process.env.QA_PAGE_MOTION_ONLY === '1') {
      const routeButtons = ['网盘', '传输', '同步', '分享']
      const jump = index => index === 4 ? page.getByTitle('设置 (Alt+5)') : page.getByRole('button', { name: routeButtons[index], exact: true })
      const selectors = ['.workspace-panes', '.down-layout', '.syncpage', '.share-page', '.settings-view']
      for (let index = 0; index < selectors.length; index++) {
        await jump(index).click()
        await page.locator(selectors[index]).waitFor()
      }
      await jump(1).click()
      const progress = page.locator('.progress-current').first()
      await progress.waitFor()
      assert.equal(await progress.evaluate(el => el.style.getPropertyValue('--progress')), '0.45')
      await page.evaluate(() => window.runtime.EventsEmit('transfer:event', { kind: 'download', task: { id: 'active', status: 'downloading', progress: 83 } }))
      await page.waitForFunction(() => document.querySelector('.progress-current')?.style.getPropertyValue('--progress') === '0.83')
      const scale = await progress.evaluate(el => {
        const style = getComputedStyle(el)
        return { width: el.offsetWidth, track: el.parentElement.clientWidth, properties: style.transitionProperty }
      })
      assert.equal(scale.width, scale.track, '传输进度更新不能改变布局宽度')
      assert.match(scale.properties, /^transform\b/)
      const switches = await page.evaluate(async selectors => {
        const host = document.querySelector('.page-host')
        const order = [0, 2, 4, 3, 1, 0]
        let emptyFrames = 0
        let duplicateFrames = 0
        let wrongFrames = 0
        for (const index of order) {
          window.dispatchEvent(new KeyboardEvent('keydown', { altKey: true, code: `Digit${index + 1}`, bubbles: true }))
          await new Promise(resolve => requestAnimationFrame(() => {
            const count = host.querySelectorAll(':scope > .page-view').length
            if (count === 0) emptyFrames++
            if (count > 1) duplicateFrames++
            if (!host.querySelector(selectors[index])) wrongFrames++
            resolve()
          }))
        }
        return { emptyFrames, duplicateFrames, wrongFrames }
      }, selectors)
      assert.deepEqual(switches, { emptyFrames: 0, duplicateFrames: 0, wrongFrames: 0 }, '快速切页不能留下空帧或旧页面')
      await jump(0).click()
      await page.locator('.fileitem').first().waitFor()
      const fileList = page.locator('.file-list').first()
      await fileList.evaluate(el => { el.scrollTop = 250000 })
      await page.waitForFunction(() => Number(document.querySelector('.file-list .fileitem')?.getAttribute('aria-posinset')) > 4000)
      const renderedFiles = await page.locator('.fileitem').count()
      assert(renderedFiles > 0 && renderedFiles < 100, '万条文件滚动时只能渲染视口附近项目')
      await page.emulateMedia({ reducedMotion: 'reduce' })
      assert.equal(await fileList.evaluate(el => getComputedStyle(el).animationName), 'none')
      await jump(4).click()
      await page.locator('.settings-column').waitFor()
      assert.equal(await page.locator('.settings-column').evaluate(el => getComputedStyle(el).animationName), 'none')
      assert.deepEqual(errors, [])
      console.log(JSON.stringify({ passed: true, pages: selectors.length, switches, renderedFiles, scale, reducedMotion: true }, null, 2))
      return
    }
    if (process.env.QA_PERFORMANCE_ONLY === '1') {
      await page.locator('.fileitem').first().waitFor()
      await page.mouse.move(900, 100)
      await page.waitForTimeout(500)
      const metrics = await page.evaluate(async () => {
        const main = document.querySelector('.page-host')
        const selected = document.querySelector('.rail-item[data-selected="true"]')
        const rail = document.querySelector('.account-rail')
        const samples = []
        const sampleAnimation = () => new Promise(resolve => {
          const deadline = performance.now() + 550
          const frame = now => {
            const rect = main.getBoundingClientRect()
            samples.push({ left: rect.left, width: rect.width })
            if (now < deadline) requestAnimationFrame(frame)
            else resolve()
          }
          requestAnimationFrame(frame)
        })
        for (let cycle = 0; cycle < 4; cycle++) {
          rail.dispatchEvent(new MouseEvent('mouseenter'))
          await sampleAnimation()
          if (!rail.querySelector('.rail-active-indicator.ready') || !selected.classList.contains('active')) throw new Error('选中背景丢失')
          rail.dispatchEvent(new MouseEvent('mouseleave'))
          await sampleAnimation()
        }
        return {
          samples: samples.length,
          viewportPositions: [...new Set(samples.map(sample => `${sample.left}:${sample.width}`))],
          renderedFiles: document.querySelectorAll('.fileitem').length,
          panelTransition: getComputedStyle(rail).transitionProperty,
        }
      })
      assert(metrics.viewportPositions.includes('60:1380') && metrics.viewportPositions.includes('220:1220'), '保留原版推挤展开与收起')
      assert.match(metrics.panelTransition, /\bflex-basis\b/, '侧栏继续使用推挤式过渡')
      assert(metrics.renderedFiles > 0 && metrics.renderedFiles < 100, '万条文件必须保持虚拟渲染')
      await page.locator('.rail-list .rail-icon').first().hover()
      await page.waitForTimeout(450)
      await page.screenshot({ path: path.join(output, 'performance-rail-expanded.png') })
      await page.mouse.move(900, 100)
      await page.waitForTimeout(450)
      await page.screenshot({ path: path.join(output, 'performance-rail-collapsed.png') })
      assert.deepEqual(errors, [], '性能验收不能出现页面异常')
      console.log(JSON.stringify({ passed: true, metrics, errors, output }, null, 2))
      return
    }
    if (process.env.QA_RAIL_DRAG_SCROLL_ONLY === '1') {
      await page.setViewportSize({ width: 1024, height: 680 })
      await page.evaluate(() => { document.documentElement.style.zoom = '1.5' })
      const list = page.locator('.rail-list')
      const items = list.locator('.rail-item')
      await items.first().hover()
      await page.waitForFunction(() => Math.abs(document.querySelector('.account-rail').getBoundingClientRect().width - 330) < 1)
      const order = () => items.evaluateAll(rows => rows.map(row => row.textContent.trim()))
      const original = await order()
      const first = await items.first().boundingBox()
      const bounds = await list.boundingBox()
      const x = first.x + first.width / 2
      await page.mouse.move(x, first.y + first.height / 2)
      await page.mouse.down()
      await page.mouse.move(x, bounds.y + bounds.height - 3, { steps: 8 })
      await page.waitForFunction(() => document.querySelector('.rail-list').scrollTop > 280)
      await page.mouse.up()
      await page.locator('.rail-ghost').waitFor({ state: 'detached' })
      const reordered = await order()
      assert(reordered.indexOf(original[0]) > 6, '拖到下边缘应能越过首屏账号')
      const visible = await items.evaluateAll(rows => {
        const root = rows[0].parentElement.getBoundingClientRect()
        return rows.findIndex(row => row.getBoundingClientRect().top > root.top + 65)
      })
      assert(visible > 0)
      const next = await items.nth(visible).boundingBox()
      const scrollBefore = await list.evaluate(root => root.scrollTop)
      await page.mouse.move(x, next.y + next.height / 2)
      await page.mouse.down()
      await page.mouse.move(x, bounds.y + 3, { steps: 8 })
      await page.waitForFunction(previous => document.querySelector('.rail-list').scrollTop < previous - 90, scrollBefore)
      await page.keyboard.press('Escape')
      await page.mouse.up()
      assert.deepEqual(await order(), reordered, '取消拖动不应保存临时顺序')
      assert.deepEqual(errors, [])
      console.log(JSON.stringify({ passed: true, zoom: 1.5, targetIndex: reordered.indexOf(original[0]) }, null, 2))
      return
    }
    if (process.env.QA_RAIL_SCROLL_ONLY === '1') {
      const checked = []
      for (const zoom of [1, 1.25, 1.5]) {
        await page.setViewportSize({ width: 1024, height: 680 })
        await page.evaluate(zoom => { document.documentElement.style.zoom = String(zoom) }, zoom)
        await page.locator('.rail-list .rail-icon').first().hover()
        await page.waitForTimeout(600)
        await page.locator('.rail-list .rail-icon').nth(2).click()
        await page.waitForTimeout(400)
        const bounds = await page.locator('.rail-list').evaluate(async list => {
          list.scrollTop = 0
          await new Promise(requestAnimationFrame)
          const first = list.querySelector('.rail-item').getBoundingClientRect()
          const rail = list.getBoundingClientRect()
          const indicator = list.querySelector('.rail-active-indicator')
          if (!indicator) throw new Error('缺少选中底色')
          const selected = list.querySelector('.rail-item[data-selected="true"]')
          const initial = indicator.getBoundingClientRect().top - selected.getBoundingClientRect().top
          const offsets = []
          for (const offset of [8, 42, 97, 180, 45, 0]) {
            list.scrollTop = offset
            await new Promise(requestAnimationFrame)
            offsets.push(indicator.getBoundingClientRect().top - selected.getBoundingClientRect().top)
          }
          return { firstTop: first.top, listTop: rail.top, firstHeight: first.height, initial, offsets, zoom: getComputedStyle(document.documentElement).zoom }
        })
        assert(bounds.firstTop >= bounds.listTop - 1, `首个账号被裁剪：${JSON.stringify(bounds)}`)
        assert(bounds.firstHeight >= 44 * zoom - 1, `账号项目被压缩：${JSON.stringify(bounds)}`)
        for (const offset of bounds.offsets) assert(Math.abs(offset - bounds.initial) < 1, `滚动时底色滞后：${JSON.stringify(bounds)}`)
        assert(Math.abs(bounds.initial) < 2, `底色位置未与账号对齐：${JSON.stringify(bounds)}`)
        const list = page.locator('.rail-list')
        const anchor = await list.evaluate(element => {
          element.scrollTop = 190
          const row = [...element.querySelectorAll('.rail-item')].find(item => item.offsetTop + item.offsetHeight > element.scrollTop)
          return { index: [...element.querySelectorAll('.rail-item')].indexOf(row), inset: row.offsetTop - element.scrollTop }
        })
        const measureAnchor = () => list.evaluate((element, index) => element.querySelectorAll('.rail-item')[index].offsetTop - element.scrollTop, anchor.index)
        await page.mouse.move(900, 100)
        await page.waitForTimeout(500)
        const collapsedInset = await measureAnchor()
        assert(Math.abs(collapsedInset - anchor.inset) < 2, `收起时滚动锚点偏移：${collapsedInset - anchor.inset}`)
        await page.mouse.move(20, 110)
        await page.locator('.rail-active-indicator.ready').waitFor({ state: 'visible' })
        const expandedInset = await measureAnchor()
        assert(Math.abs(expandedInset - anchor.inset) < 2, `重新展开时滚动锚点偏移：${expandedInset - anchor.inset}`)
        checked.push({ zoom, ...bounds, collapsedAnchorDrift: collapsedInset - anchor.inset, expandedAnchorDrift: expandedInset - anchor.inset })
        await list.evaluate(element => { element.scrollTop = 0 })
        await page.mouse.move(900, 100)
        await page.waitForTimeout(500)
      }
      await page.screenshot({ path: path.join(output, 'rail-scroll.png') })
      assert.deepEqual(errors, [], '侧栏滚动不能产生页面异常')
      console.log(JSON.stringify({ passed: true, checked }, null, 2))
      return
    }
    if (process.env.QA_RAIL_MOTION_ONLY === '1') {
      const rail = page.locator('.account-rail')
      const icon = rail.locator('.rail-list .rail-icon').first()
      const waitWidth = width => page.waitForFunction(expected => Math.abs(document.querySelector('.account-rail').getBoundingClientRect().width - expected) < 0.05, width)
      const layout = () => rail.evaluate(element => {
        const box = element.getBoundingClientRect()
        const content = document.querySelector('.page-host').getBoundingClientRect()
        const selected = element.querySelector('.rail-item.active')
        const indicator = element.querySelector('.rail-active-indicator.ready')
        const status = selected.querySelector('.rail-status')
        const highlight = (indicator || selected).getBoundingClientRect()
        const row = selected.getBoundingClientRect()
        const badge = getComputedStyle(selected.querySelector('.rail-icon'), '::after')
        return { width: box.width, seam: content.left - box.right, rowHeight: row.height, status: status?.textContent, badgeVisible: badge.content !== 'none' && parseFloat(badge.width) > 0, background: getComputedStyle(indicator || selected).backgroundColor, highlightOffset: [highlight.left - row.left, highlight.top - row.top] }
      })
      await icon.waitFor()
      assert.equal((await layout()).width, 60)
      assert((await layout()).badgeVisible, '收起时仍应显示登录失效标记')
      await rail.evaluate(element => element.style.setProperty('--rail-duration', '600ms'))
      await icon.hover()
      await page.waitForFunction(() => { const width = document.querySelector('.account-rail').getBoundingClientRect().width; return width > 85 && width < 210 })
      const opening = await layout()
      await page.waitForFunction(() => document.querySelector('.account-rail').getAnimations().some(animation => animation.transitionProperty === 'flex-basis' && animation.currentTime > 330 && animation.currentTime < 550))
      assert.equal(await rail.locator('.rail-active-indicator.ready').count(), 0, '栏宽仍在变化时不能提前换到底板')
      await waitWidth(220)
      await rail.locator('.rail-active-indicator.ready').waitFor()
      const open = await layout()
      await rail.evaluate(element => element.style.removeProperty('--rail-duration'))
      await page.mouse.move(900, 100)
      await page.waitForFunction(() => { const width = document.querySelector('.account-rail').getBoundingClientRect().width; return width > 85 && width < 210 })
      const closing = await layout()
      await icon.hover()
      await waitWidth(220)
      await rail.locator('.rail-active-indicator.ready').waitFor()
      const reopened = await layout()
      for (const sample of [opening, open, closing, reopened]) {
        assert.equal(sample.status, '登录失效', '缓存目录加载成功不能隐藏已确认的失效提示')
        assert(sample.badgeVisible, '展开和收起过程中失效标记不能闪失')
        assert(Math.abs(sample.seam) < 1, '侧栏和文件区须同步移动')
        assert(sample.highlightOffset.every(value => Math.abs(value) < 1), `高亮必须与当前账号对齐：${JSON.stringify(sample)}`)
        assert.match(sample.background, /^rgb\(/, '高亮切换时不能透明或闪淡')
      }
      assert(opening.rowHeight > 36 && opening.rowHeight < open.rowHeight, '行高需要平滑变化')
      await page.mouse.move(900, 100)
      await waitWidth(60)
      await icon.click({ button: 'right' })
      const reauthorize = page.getByRole('menuitem', { name: '重新登录', exact: true })
      await reauthorize.click()
      const dialog = page.getByRole('dialog', { name: '重新登录', exact: true })
      await dialog.waitFor()
      assert.equal(await dialog.getByRole('tab').count(), 1, '重新登录必须锁定原网盘')
      await dialog.getByLabel('新密码或令牌', { exact: true }).fill('local-test-password')
      await dialog.getByRole('button', { name: '保存连接', exact: true }).click()
      await dialog.waitFor({ state: 'detached' })
      await rail.locator('.rail-status').waitFor({ state: 'detached' })
      assert(!(await layout()).badgeVisible, '重新登录成功后清除失效标记')
      const reconnect = await page.evaluate(() => window.__qaCalls.find(call => call.method === 'ReconnectMountedAccount'))
      assert.equal(reconnect?.args[0], 'webdav:account-0', '必须更新原账号')

      await page.evaluate(() => { document.documentElement.style.zoom = '1.25' })
      await icon.hover()
      await rail.locator('.rail-active-indicator.ready').waitFor()
      const rows = rail.locator('.rail-list .rail-item')
      const first = await rows.first().boundingBox()
      const third = await rows.nth(2).boundingBox()
      await page.mouse.move(first.x + first.width / 2, first.y + first.height / 2)
      await page.mouse.down()
      await page.mouse.move(first.x + first.width / 2, third.y + third.height / 2, { steps: 8 })
      await rail.locator('.rail-ghost').waitFor()
      await page.mouse.move(900, third.y + third.height / 2)
      await page.evaluate(() => window.addEventListener('pointerup', () => {
        const [animation] = document.querySelector('.rail-ghost.dropping')?.getAnimations() || []
        if (!animation) return
        animation.pause()
        animation.currentTime = animation.effect.getTiming().duration - 1
      }, { once: true }))
      await page.mouse.up()
      assert.equal(await rail.locator('.rail-ghost.dropping').count(), 1, '松手后应进入回弹动画')
      await page.waitForTimeout(350)
      const drop = await rail.evaluate(element => {
        const clone = element.querySelector('.rail-ghost')
        const row = element.querySelector('.rail-item.dragging')
        const landing = clone.getBoundingClientRect()
        const slot = row.getBoundingClientRect()
        return { expanded: element.classList.contains('expanded'), offset: ['left', 'top', 'width', 'height'].map(key => landing[key] - slot[key]) }
      })
      assert(drop.expanded, '回弹完成前不能收起侧栏')
      assert(drop.offset.every(value => Math.abs(value) < 1), `缩放后拖拽落点必须贴合行：${JSON.stringify(drop)}`)
      await rail.locator('.rail-ghost.dropping').evaluate(clone => clone.getAnimations().forEach(animation => animation.finish()))
      await rail.locator('.rail-ghost').waitFor({ state: 'detached' })
      await waitWidth(75)
      assert.deepEqual(errors, [])
      console.log(JSON.stringify({ passed: true, opening, open, closing, reopened, drop, errors }, null, 2))
      return
    }
    if (process.env.QA_ACCOUNT_ORDER_ONLY === '1') {
      const labels = await page.locator('.rail-list .rail-item').evaluateAll(items => items.map(item => item.getAttribute('aria-label')))
      const expectedLabels = [labels[1], labels[2], labels[0], ...labels.slice(3)]
      const saved = await page.evaluate(() => {
        const items = [...document.querySelectorAll('.rail-list .rail-item')]
        const first = items[0].getBoundingClientRect()
        const target = items[2].getBoundingClientRect()
        const pointer = { pointerId: 7, button: 0, clientX: first.left + first.width / 2, bubbles: true }
        items[0].dispatchEvent(new PointerEvent('pointerdown', { ...pointer, clientY: first.top + first.height / 2 }))
        window.dispatchEvent(new PointerEvent('pointermove', { ...pointer, clientY: target.top + target.height / 2 }))
        window.dispatchEvent(new PointerEvent('pointerup', { ...pointer, clientY: target.top + target.height / 2 }))
        return JSON.parse(localStorage.getItem('mnemo.prefs')).accountOrder
      })
      assert.deepEqual(saved.slice(0, 3), ['webdav:account-1', 'webdav:account-2', 'webdav:account-0'], '快速松手必须同步保存最终顺序')
      await page.evaluate(() => window.runtime.EventsEmit('account:changed'))
      await page.waitForTimeout(400)
      assert.deepEqual(await page.locator('.rail-list .rail-item').evaluateAll(items => items.map(item => item.getAttribute('aria-label'))), expectedLabels, '账号刷新不能覆盖拖拽顺序')
      assert.equal(await page.locator('.rail-ghost').count(), 0, '拖动结束必须移除幽灵')
      await page.reload()
      await page.locator('.fileitem').first().waitFor()
      assert.deepEqual(await page.locator('.rail-list .rail-item').evaluateAll(items => items.map(item => item.getAttribute('aria-label'))), expectedLabels, '页面重载后必须恢复保存顺序')
      assert.deepEqual(errors, [], '排序保存不能出现页面异常')
      console.log(JSON.stringify({ passed: true, accountOrder: saved, reloadRestored: true, errors }, null, 2))
      return
    }
    if (process.env.QA_KEYBOARD_ONLY === '1') {
      await page.getByRole('button', { name: '切换双栏', exact: true }).click()
      const lists = page.locator('.workspace-pane .file-list')
      const left = lists.first()
      const right = lists.nth(1)
      await right.getByRole('option').first().waitFor()
      const leftId = await left.getByRole('option').first().getAttribute('id')
      assert.notEqual(leftId, await right.getByRole('option').first().getAttribute('id'))
      const focusedPosition = list => list.evaluate(element => {
        const option = document.getElementById(element.getAttribute('aria-activedescendant'))
        return element.contains(option) ? Number(option.getAttribute('aria-posinset')) : null
      })
      await right.press('End')
      assert.equal(await focusedPosition(right), Number(await right.getByRole('option').last().getAttribute('aria-setsize')))
      await right.press('Home')
      assert.equal(await focusedPosition(right), 1)
      await right.press('PageDown')
      assert(await focusedPosition(right) > 1)
      assert.equal(await left.getAttribute('aria-activedescendant'), null)
      assert.deepEqual(errors, [])
      console.log(JSON.stringify({ passed: true, dualPaneKeyboard: true, errors }, null, 2))
      return
    }
    if (process.env.QA_TASK_ISSUE_ONLY === '1') {
      await page.evaluate(() => window.runtime.EventsEmit('transfer:event', { kind: 'upload', task: { id: 'background-upload', user_id: 'webdav:account-0', status: 'failed', issue: { code: 'UPLOAD_RESULT_UNKNOWN', message: '网盘可能已接收文件', action: 'check_target' } } }))
      await page.locator('.toast').filter({ hasText: '上传结果尚未确认' }).waitFor()
      assert.equal(await page.locator('.toast').filter({ hasText: 'UPLOAD_RESULT_UNKNOWN' }).count(), 0, '通知不能暴露内部错误码')
      await page.evaluate(() => {
        window.go.app.App.ListDownloads = async () => [{ id: 'network-task', user_id: 'webdav:account-0', name: '网络失败任务.txt', status: 'failed', error: 'HTTP 401', issue: { code: 'NETWORK_UNAVAILABLE', message: '网络请求中断', action: 'network_settings' } }]
      })
      await page.getByRole('button', { name: '传输', exact: true }).click()
      assert.equal(await page.locator('.taskrow details').count(), 0, '任务行不能内联错误详情')
      await page.getByRole('button', { name: '查看通知', exact: true }).first().click()
      await page.locator('.toast').getByRole('button', { name: '网络设置', exact: true }).click()
      await page.getByRole('combobox', { name: '默认排序', exact: true }).waitFor()
      assert.deepEqual(errors, [], '任务处理入口不能出现页面异常')
      console.log(JSON.stringify({ passed: true, taskRepair: 'network_settings', errors }, null, 2))
      return
    }
    if (process.env.QA_SELECT_MOTION_ONLY === '1') {
      await page.setViewportSize({ width: 1050, height: 590 })
      await page.getByRole('button', { name: '同步', exact: true }).click()
      await page.getByTitle('编辑', { exact: true }).click()
      const select = page.getByRole('combobox', { name: '请选择账号' })
      const list = page.getByRole('listbox', { name: '请选择账号' })
      await page.waitForFunction(() => getComputedStyle(document.querySelector('.modal')).transform === 'none')
      const moveTrigger = top => select.evaluate((button, y) => {
        button.style.transition = 'none'
        button.style.transform = 'none'
        button.style.transform = `translateY(${y - button.getBoundingClientRect().top}px)`
        window.dispatchEvent(new Event('resize'))
      }, top)
      const waitForPlacement = placement => page.waitForFunction(expected => {
        const popup = document.querySelector('.uiselect-drop')
        return popup?.dataset.placement === expected && getComputedStyle(popup).transform === 'none'
      }, placement)
      await select.focus()
      await moveTrigger(350)
      await select.press('Enter')
      await waitForPlacement('up')
      const [buttonAbove, popupAbove] = await Promise.all([select.boundingBox(), list.boundingBox()])
      assert(Math.abs(buttonAbove.y - popupAbove.y - popupAbove.height - 4) < 1)
      const pageScroll = () => page.evaluate(() => [document.scrollingElement.scrollTop, document.querySelector('.modal-body').scrollTop])
      const scrollBefore = await pageScroll()
      await select.press('End')
      assert(await list.evaluate(element => element.scrollTop > 0))
      assert.deepEqual(await pageScroll(), scrollBefore)
      assert.equal(await select.getAttribute('aria-activedescendant'), await list.getByRole('option').last().getAttribute('id'))
      await moveTrigger(72)
      await waitForPlacement('down')
      const [buttonBelow, popupBelow] = await Promise.all([select.boundingBox(), list.boundingBox()])
      assert(Math.abs(popupBelow.y - buttonBelow.y - buttonBelow.height - 4) < 1)
      const closing = await list.elementHandle()
      await select.press('Escape')
      assert(await closing.evaluate(element => element.inert))
      assert(await select.evaluate(button => document.activeElement === button))
      assert.deepEqual(errors, [])
      console.log(JSON.stringify({ passed: true, anchoredBothWays: true, scrollIsolated: true, exitInert: true }, null, 2))
      return
    }
    if (process.env.QA_MODAL_STACK_ONLY === '1') {
      await page.evaluate(() => {
        window.go.app.App.ListDir = async (_user, _drive, id) => {
          if (id === 'root') return Array.from({ length: 1000 }, (_, index) => ({ file_id: `branch-${index}`, name: `目录-${String(index).padStart(4, '0')}`, isDir: true }))
          return id === 'branch-999' ? [{ file_id: 'nested', name: '子目录', isDir: true }] : []
        }
      })
      await page.getByRole('button', { name: '同步', exact: true }).click()
      const opener = page.getByTitle('编辑', { exact: true })
      await opener.click()
      const parent = page.locator('.modal[aria-label="编辑同步任务"]')
      const picker = parent.getByRole('button', { name: '选择目录' }).last()
      await picker.click()
      const child = page.locator('.modal[aria-label="选择网盘目录"]')
      await child.waitFor()
      assert(await parent.evaluate(element => element.parentElement.inert), '底层弹窗应禁用交互')
      const cdp = await page.context().newCDPSession(page)
      const { root: { nodeId } } = await cdp.send('DOM.getDocument')
      const parentNode = await cdp.send('DOM.querySelector', { nodeId, selector: '.modal[aria-label="编辑同步任务"]' })
      const { nodes } = await cdp.send('Accessibility.getPartialAXTree', { nodeId: parentNode.nodeId, fetchRelatives: false })
      assert.equal(nodes[0]?.ignored, true, '底层弹窗不应出现在可访问树中')
      await cdp.detach()
      assert(await child.evaluate(element => element.contains(document.activeElement)))
      const directoryList = child.locator('.directory-list')
      await directoryList.getByRole('option').first().waitFor()
      await directoryList.press('End')
      const folder = directoryList.getByRole('option', { name: '目录-0999' })
      const assertRestoredFolder = async () => {
        await folder.waitFor()
        const position = await folder.evaluate(element => {
          const list = element.closest('.directory-list')
          const bounds = element.getBoundingClientRect()
          const viewport = list.getBoundingClientRect()
          return { focused: document.activeElement === list, active: list.getAttribute('aria-activedescendant') === element.id, scrollTop: list.scrollTop, top: bounds.top - viewport.top, bottom: viewport.bottom - bounds.bottom }
        })
        assert(position.focused && position.active && position.scrollTop > 0 && position.top >= -1 && position.bottom >= -1, `返回后应保留目录焦点及可见滚动位置：${JSON.stringify(position)}`)
      }
      await directoryList.press('Enter')
      await directoryList.getByRole('option', { name: '子目录' }).waitFor()
      await directoryList.press('ArrowLeft')
      await assertRestoredFolder()
      await directoryList.press('Enter')
      await directoryList.getByRole('option', { name: '子目录' }).waitFor()
      await directoryList.press('Enter')
      await child.getByText('暂无子文件夹').waitFor()
      await child.locator('.directory-crumb').first().click()
      await assertRestoredFolder()
      const leaving = await child.evaluate(async element => {
        const mask = element.parentElement
        document.activeElement.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true }))
        await new Promise(resolve => requestAnimationFrame(resolve))
        const center = document.elementFromPoint(innerWidth / 2, innerHeight / 2)
        return { inert: mask.inert, interceptsPointer: mask.contains(center) }
      })
      assert.deepEqual(leaving, { inert: true, interceptsPointer: false })
      assert(await picker.evaluate(element => document.activeElement === element), '退场时焦点应已回到父弹窗')
      await picker.dispatchEvent('keydown', { key: 'Escape', repeat: true, bubbles: true, cancelable: true })
      assert(await page.getByRole('dialog', { name: '编辑同步任务' }).isVisible())
      await picker.press('Escape')
      await parent.waitFor({ state: 'detached' })
      assert(await opener.evaluate(element => document.activeElement === element))
      assert.deepEqual(errors, [])
      console.log(JSON.stringify({ passed: true, leaving, focusRestored: true, directoryAnchorRestored: true, repeatedEscapeIgnored: true }, null, 2))
      return
    }
    if (process.env.QA_ACCESSIBILITY_ONLY === '1') {
      const accessibility = await assertKeyboardAccessibility(page)
      assert.deepEqual(errors, [], '无障碍验收不能出现页面异常')
      console.log(JSON.stringify({ passed: true, accessibility, errors }, null, 2))
      return
    }
    if (process.env.QA_DIALOG_LAYOUT_ONLY === '1') {
      const checked = []
      await page.evaluate(() => {
        window.__modalEntrance = []
        document.addEventListener('transitionrun', event => {
          if (event.target.matches('.modal-fade-enter-active > .modal') && event.propertyName === 'transform') window.__modalEntrance.push(event.target.getAttribute('aria-label'))
        })
      })
      async function checkDialog(title) {
        await page.getByRole('dialog', { name: title, exact: true }).waitFor()
        await assertDialogCorners(page)
        checked.push(title)
        await page.getByRole('button', { name: '关闭对话框', exact: true }).click()
        await page.getByRole('dialog', { name: title, exact: true }).waitFor({ state: 'detached' })
      }
      for (const label of ['自定义', '账号信息']) {
        await page.locator('.rail-list .rail-icon').first().click({ button: 'right' })
        await page.getByRole('menuitem', { name: label, exact: true }).click()
        await checkDialog(label)
      }
      await page.getByRole('button', { name: '全盘搜索', exact: true }).click()
      await page.waitForFunction(() => window.__modalEntrance.includes('搜索'))
      await checkDialog('搜索')
      await page.emulateMedia({ reducedMotion: 'reduce' })
      await page.getByRole('button', { name: '全盘搜索', exact: true }).click()
      await page.getByRole('dialog', { name: '搜索', exact: true }).waitFor()
      assert.equal(await page.getByRole('dialog', { name: '搜索', exact: true }).evaluate(el => getComputedStyle(el).transform), 'none')
      await page.keyboard.press('Escape')
      await page.getByRole('dialog', { name: '搜索', exact: true }).waitFor({ state: 'detached' })
      await page.emulateMedia({ reducedMotion: 'no-preference' })
      await page.getByRole('button', { name: '分享', exact: true }).click()
      await page.getByRole('button', { name: '导入分享', exact: true }).first().click()
      await checkDialog('导入分享')
      await page.getByRole('button', { name: '同步', exact: true }).click()
      await page.getByTitle('编辑', { exact: true }).click()
      await checkDialog('编辑同步任务')
      await page.getByTitle('预览同步').click()
      await checkDialog('同步预览')
      await page.getByTitle('设置 (Alt+5)').click()
      await page.getByRole('button', { name: '恢复', exact: true }).click()
      await page.locator('.modal').waitFor()
      await assertDialogCorners(page)
      checked.push('恢复确认')
      await page.getByRole('button', { name: '关闭对话框', exact: true }).click()
      assert.deepEqual(errors, [], '二级弹窗布局检查不应产生浏览器错误')
      console.log(JSON.stringify({ passed: true, checked, errors }, null, 2))
      return
    }
    assert(await page.locator('.fileitem').count() < 100, '10k files must stay virtualized')
    for (let i=0;i<10;i++) { await page.locator('.rail-list .rail-icon').nth(i).click(); await page.waitForTimeout(50) }
    await page.mouse.move(1000,100); await page.waitForTimeout(400)
    assert(!await page.locator('.account-rail').evaluate(el=>el.classList.contains('expanded')), 'rail must collapse')
    await page.getByRole('button',{name:'切换双栏',exact:true}).click()
    await page.locator('.workspace-pane').nth(1).locator('.fileitem').first().waitFor()
    await page.locator('.workspace-pane').nth(1).locator('.fileitem').first().click()
    await page.keyboard.press('Control+a')
    assert.equal(await page.locator('.workspace-pane').nth(0).locator('.fileitem.selected').count(),0,'selection must stay in focused pane')
    await page.screenshot({path:path.join(output,'dual-dark.png')})
    await page.getByRole('button',{name:'全盘搜索',exact:true}).click()
    await page.getByRole('textbox', { name: '跨盘搜索名称' }).fill('资料')
    await assertDialogCorners(page)
    await page.locator('.modal').getByRole('button',{name:'搜索',exact:true}).click()
    await page.locator('.workspace-result').first().waitFor()
    await page.locator('.workspace-result').first().click()
    await page.waitForTimeout(300)
    await page.getByRole('button',{name:'分享',exact:true}).click()
    await page.getByRole('button',{name:'导入分享',exact:true}).first().click()
    await page.getByPlaceholder('粘贴链接或分享文案').fill('资料 https://www.alipan.com/s/abc 提取码：A1b2')
    await assertDialogCorners(page)
    assert.equal(await page.getByPlaceholder('没有提取码可留空').inputValue(),'A1b2')
    await page.getByRole('button',{name:'解析分享',exact:true}).click()
    await page.getByText('分享资料',{exact:true}).waitFor()
    await page.screenshot({path:path.join(output,'share.png')})
    await page.getByRole('button',{name:'取消',exact:true}).click()
    await page.getByRole('button',{name:'同步',exact:true}).click()
    await page.getByTitle('预览同步').click()
    await page.getByText('资料.txt',{exact:true}).waitFor()
    await assertDialogCorners(page)
    await page.screenshot({path:path.join(output,'sync.png')})
    await page.getByRole('button',{name:'执行',exact:true}).click()
    await page.getByTitle('设置 (Alt+5)').click()
    await page.getByRole('button',{name:'恢复',exact:true}).click()
    await assertDialogCorners(page)
    await page.locator('.modal').getByRole('button',{name:'确定',exact:true}).click()
    await page.waitForTimeout(300)
    assert(await page.locator('html').evaluate(el=>el.classList.contains('oled')),'OLED preference restored')
    await page.locator('#sg-general').getByRole('button',{name:'导出',exact:true}).click()
    await page.waitForFunction(()=>window.__qaBackup)
    assert.equal(await page.evaluate(()=>window.__qaBackup.format),'mnemo-preferences')
    await page.screenshot({path:path.join(output,'settings-oled.png')})
    await page.setViewportSize({width:1024,height:768})
    await page.getByRole('button',{name:'网盘',exact:true}).click()
    await page.locator('.workspace-panes').waitFor({state:'visible'})
    await page.waitForTimeout(500)
    await page.screenshot({path:path.join(output,'dual-1024.png')})
    assert.deepEqual(errors,[], 'no page errors')
    console.log(JSON.stringify({passed:true,errors,output},null,2))
  } catch (error) {
    await page.screenshot({ path: path.join(output, 'failure.png'), timeout: 2000 }).catch(() => {})
    const body = await page.locator('body').evaluate(element => element.textContent.slice(0, 1600), undefined, { timeout: 2000 }).catch(() => '')
    console.error(JSON.stringify({ errors, body }))
    throw error
  } finally { await browser.close() }
}
main().catch(error=>{console.error(error);process.exitCode=1})
