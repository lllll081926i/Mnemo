import fs from 'fs'
import path from 'path'
import { describe, expect, it } from 'vitest'

const root = process.cwd()
const read = (rel: string) => fs.readFileSync(path.join(root, rel), 'utf8')

describe('zip design port to Vue chrome', () => {
  it('ships design tokens with zip primary and surfaces', () => {
    const css = read('src/assets/design-tokens.css')
    expect(css).toContain('--color-primary: #4f46e5')
    expect(css).toContain('--bg-surface: #ffffff')
    expect(css).toContain('--text-disabled: #94a3b8')
    expect(css).toContain('--control-border: #cbd5e1')
    expect(css).toContain('--foot-bg: #4052b8')
    expect(css).toContain("body[arco-theme='dark']")
    // no tailwind import that can break vue transforms
    expect(css).not.toMatch(/@import\s+[\"']tailwindcss[\"']/)
  })

  it('App.vue imports design tokens after global.css', () => {
    const app = read('src/App.vue')
    expect(app).toContain("import './assets/global.css'")
    expect(app).toContain("import './assets/design-tokens.css'")
    const g = app.indexOf("import './assets/global.css'")
    const t = app.indexOf("import './assets/design-tokens.css'")
    expect(t).toBeGreaterThan(g)
  })

  it('keeps layout selectors out of the token file', () => {
    const css = read('src/assets/design-tokens.css')
    for (const selector of ['#xbyhead', '#xbyhead2', '#xbyfoot', '#footer2', '.xbyleft', '.xbyright', '.xbyleftmenu', '.toppanbtn', '.pan-drive-switcher', '.mantine-root']) {
      expect(css).not.toContain(selector)
    }
  })

  it('ships shared readable type and functional-control primitives after feature CSS', () => {
    const app = read('src/App.vue')
    const css = read('src/assets/layout-refactor.css')
    expect(app.indexOf("import './assets/layout-refactor.css'")).toBeGreaterThan(app.indexOf("import './assets/fileitem.css'"))
    expect(css).toContain('--ui-font-size: 13px')
    expect(css).toContain('--ui-control-height: 32px')
    expect(css).toContain('--color-text-1: var(--text-primary) !important')
    expect(css).toContain('--color-bg-popup: var(--bg-surface) !important')
    expect(css).toContain('color: var(--text-primary) !important')
    expect(css).toContain('border-color: var(--control-border) !important')
    expect(css).toContain('background: var(--control-bg) !important')
  })

  it('PageMain header uses Mnemo branding without an unexplained divider', () => {
    const page = read('src/layout/PageMain.vue')
    expect(page).toMatch(/class=["']title["']>Mnemo</)
    expect(page).not.toMatch(/class=["']head-divider["']/)
  })

  it('PanLeft includes drive switcher chrome from zip layout', () => {
    const pan = read('src/pan/PanLeft.vue')
    expect(pan).toContain('pan-drive-switcher')
    expect(pan).toContain('driveSwitcherLabel')
    expect(pan).toContain('refreshDriveAccounts')
    expect(pan).toContain('UserDAL.UserChange(userId)')
  })

  it('theme toggle syncs both arco-theme and html.dark class', () => {
    const store = read('src/store/appstore.ts')
    expect(store).toContain("document.documentElement.classList.add('dark')")
    expect(store).toContain("document.documentElement.classList.remove('dark')")
    expect(store).toContain("document.body.setAttribute('arco-theme', 'dark')")
  })
})
