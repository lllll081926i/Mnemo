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
  await page.addInitScript(() => {
    const accounts = Array.from({length:20}, (_,i) => ({user_id:`webdav:account-${i}`,drive_id:`drive-${i}`,custom_name:`资料库 ${i+1}`,token:{},usage:{size:100000000,used:100000,status:'available'}}))
    accounts.push({user_id:'aliopen_one',drive_id:'ali',custom_name:'阿里资料',token:{}})
    const files = Array.from({length:10000},(_,i) => ({file_id:`file-${i}`,parent_file_id:'root',name:`资料-${String(i).padStart(5,'0')}.txt`,size:1024,time:1700000000,isDir:false}))
    const listeners = {}
    window.runtime = new Proxy({EventsOnMultiple:(name,fn) => { (listeners[name] ||= []).push(fn); return () => {} }, EventsEmit:(name,...args) => (listeners[name] || []).forEach(fn=>fn(...args)), Environment:async()=>({platform:'windows'}), WindowGetSize:async()=>({w:1440,h:1000})}, {get:(target,key)=>target[key] || (()=>{})})
    window.__qaCalls = []
    const api = {
      ListAccounts:async()=>accounts, ListProviders:async()=>[
        {ID:'webdav',Meta:{key:'webdav',label:'WebDAV',icon:'drive-icons/webdav.svg',rootKey:'root'},Capabilities:{download:true,upload:true,uploadMode:'queue',search:true,copy:true,createFolder:true,rename:true}},
        {ID:'aliopen',Meta:{key:'aliopen',label:'阿里云盘',icon:'drive-icons/aliopen.svg',rootKey:'root'},Capabilities:{importShare:true,download:true,upload:true,search:true}}
      ],
      GetSettings:async()=>({theme:'dark',autoUpdate:false,defaultTab:'pan'}), SaveSettings:async()=>{},
      RefreshAccount:async id=>accounts.find(a=>a.user_id===id), RefreshAccountNow:async id=>accounts.find(a=>a.user_id===id),
      ListDir:async(user,drive,dir)=>{await new Promise(r=>setTimeout(r,20));return dir==='root'? files : [files[0]]},
      ListFavorites:async()=>[],GetDirectoryCache:async()=>null,SaveDirectoryCache:async()=>{},DeleteDirectoryCache:async()=>{},
      ListDownloads:async()=>[],ListUploads:async()=>[],ListOfflineTasks:async()=>[],
      ListShareHistory:async()=>[], ListShareImportReceipts:async()=>[], GetLogPath:async()=>'',
      SetAccountCustomMeta:async(id,name,icon)=>{const a=accounts.find(a=>a.user_id===id);a.custom_name=name;a.custom_icon=icon;window.runtime.EventsEmit('account:changed')},
      SearchCachedFiles:async()=>[{userId:accounts[0].user_id,driveId:accounts[0].drive_id,parentId:'root',file:files[0],updatedAt:1700000000}],
      SearchFiles:async()=>[files[1]],
      ImportShare:async()=>({files:[{fileId:'import-1',name:'分享资料',size:10,isDir:false}]}), SaveImportedShareDetailed:async(user,drive,session,ids,parent)=>({id:'receipt',accountId:user,driveId:drive,parentId:parent,historySaved:true,items:ids.map(sourceId=>({sourceId,status:'submitted'}))}),
      PreviewMigration:async()=>({files:1,bytes:1024,conflicts:[],warnings:[]}),MigrateFiles:async()=>({id:'new-migration',status:'pending'}),
      ListSyncConfigs:async()=>[{id:'sync1',name:'资料同步',user_id:accounts[0].user_id,drive_id:accounts[0].drive_id,local_dir:'D:/QA',remote_dir:'root',direction:'two-way',enabled:false}],ListRunningSyncIDs:async()=>[],
      PreviewSync:async()=>({token:'test-token',changes:[{path:'资料.txt',action:'conflict',local:{size:10},remote:{size:20}}]}),RunSyncPlan:async()=>{},
      ListMigrateJobs:async()=>[{id:'migration1',srcUser:accounts[0].user_id,dstUser:accounts[1].user_id,dstParent:'root',status:'completed',fileIDs:['file-0'],completedFileIDs:['file-0'],items:{'file-0':{id:'file-0',name:'资料.txt',status:'completed',verification:'size'}}}],
      VerifyMigration:async()=>[{id:'file-0',status:'hash',detail:'sha1 哈希一致'}],
      ExportPreferences:async payload=>{window.__qaBackup=JSON.parse(payload);return 'mock.json'},
      ImportPreferences:async()=>JSON.stringify({format:'mnemo-preferences',version:1,preferences:{oledBackground:true,accountAliases:{},accountIcons:{},accountOrder:accounts.map(a=>a.user_id).reverse()},theme:'dark',favorites:[]}),
    }
    window.go={app:{App:new Proxy(api,{get:(target,key)=>(...args)=>{window.__qaCalls.push({method:key,args});return target[key] ? target[key](...args) : Promise.resolve(null)}})}}
  })
  try {
    await page.goto(process.env.QA_URL || 'http://127.0.0.1:5173')
    await page.locator('.fileitem').first().waitFor()
    if (process.env.QA_ACCESSIBILITY_ONLY === '1') {
      const accessibility = await assertKeyboardAccessibility(page)
      assert.deepEqual(errors, [], '无障碍验收不能出现页面异常')
      console.log(JSON.stringify({ passed: true, accessibility, errors }, null, 2))
      return
    }
    if (process.env.QA_DIALOG_LAYOUT_ONLY === '1') {
      const checked = []
      async function checkDialog(title) {
        await page.getByRole('dialog', { name: title, exact: true }).waitFor()
        await assertDialogCorners(page)
        checked.push(title)
        await page.getByRole('button', { name: '关闭对话框', exact: true }).click()
        await page.getByRole('dialog', { name: title, exact: true }).waitFor({ state: 'detached' })
      }
      for (const label of ['自定义', '账号信息']) {
        await page.locator('.rail-item').first().click({ button: 'right' })
        await page.getByRole('menuitem', { name: label, exact: true }).click()
        await checkDialog(label)
      }
      await page.getByRole('button', { name: '全盘搜索', exact: true }).click()
      await checkDialog('搜索')
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
    for (let i=0;i<10;i++) { await page.locator('.rail-item').nth(i).click(); await page.waitForTimeout(50) }
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
  } finally { await browser.close() }
}
main().catch(error=>{console.error(error);process.exitCode=1})
