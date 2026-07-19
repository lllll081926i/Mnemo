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
    expect(page).toMatch(/class=["']head-divider["']/)
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
    expect(pan).toContain('pan-drive-switcher')
    expect(pan).toContain('winStore.height - 48 - 50 - 44 - 24')
    expect(pan).toContain('UserDAL.UserChange(userId)')
    expect(pan).toContain('登录后查看文件')
    expect(pan).not.toContain('回收站、全盘搜索和文件目录会在登录后显示')
    expect(pan).not.toContain('创建快捷方式(Ctrl 1-9)')
  })

  it('down/share/setting use 220 rail shells', () => {
    expect(read('src/down/index.vue')).toContain(':width="220"')
    expect(read('src/share/index.vue')).toContain(':width="220"')
    expect(read('src/setting/index.vue')).toContain('ui-page-shell')
    expect(read('src/assets/layout-refactor.css')).toContain('--layout-rail-width: 220px')
    expect(read('src/share/index.vue')).not.toContain('ShareBottleFish')
    expect(read('src/setting/index.vue')).not.toContain('SettingAPI')
  })

  it('uses spacing for individual settings and dividers only between setting groups', () => {
    const css = read('src/assets/layout-refactor.css')
    const setting = read('src/setting/index.vue')
    expect(css).toContain('.ui-plain-list {\n  display: grid;\n  gap: 2px;')
    expect(css).not.toContain(
      '.ui-plain-row {\n  display: grid;\n  grid-template-columns: minmax(160px, var(--layout-row-label-width)) minmax(0, 1fr);\n  gap: var(--layout-row-gap);\n  align-items: center;\n  min-height: var(--layout-row-min-height);\n  padding: 6px 0;\n  border-bottom'
    )
    expect(setting).toContain('.settings-section + .settings-section')
    expect(setting).toContain('border-top: 1px solid var(--border-light)')
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

  it('keeps all setting groups in one scrollable document with sidebar anchors', () => {
    const setting = read('src/setting/index.vue')
    expect(setting).toContain('scrollToSection')
    expect(setting).toContain('IntersectionObserver')
    expect(setting).toContain('data-settings-section')
    expect(setting).toContain('v-for="section in visibleSections"')
    expect(setting).toContain("section.key !== 'aliyun' || isAliyunUser")
    expect(setting).toContain('scroll-margin-top: 12px')
    expect(setting).toContain('.settings-section + .settings-section')
    expect(setting).toContain('border-top: 1px solid var(--border-light)')
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
})
