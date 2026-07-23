import { readFileSync } from 'fs'
import { resolve } from 'path'
import { describe, expect, it } from 'vitest'

const root = resolve(__dirname, '../../..')
const readSource = (file: string) => readFileSync(resolve(root, file), 'utf8')

describe('app startup rendering', () => {
  it('does not delay application or music windows with a splash screen', () => {
    const app = readSource('src/App.vue')
    const appStore = readSource('src/store/appstore.ts')
    const rendererEntry = readSource('src/main.ts')
    const windowSource = readSource('electron/main/core/window.ts')
    const ipcSource = readSource('electron/main/core/ipcEvent.ts')
    const workerHtml = readSource('worker.html')
    const workerEntry = readSource('src/workerpage/main.ts')

    expect(app).not.toContain('PageLoading')
    expect(app).not.toContain('PAGE_LOADING_SPLASH_MIN_MS')
    expect(appStore).not.toContain('PageLoading')
    expect(app).toContain("return h('div', { class: 'desktop-loading-empty' })")
    expect(app).toContain("const PageMain = defineAsyncComponent(() => import('./layout/PageMain.vue'))")
    expect(app).not.toContain('PageWorker')
    expect(rendererEntry).not.toContain("import { PageMain } from './layout/PageMain'")
    expect(rendererEntry).not.toContain("import { WorkerPage } from './workerpage/workercmd'")
    expect(rendererEntry).toContain("window.WebOnSetPage(async")
    expect(rendererEntry).toContain("await import('./layout/PageMain')")
    expect(rendererEntry).toContain("import('./layout/PageMain')")
    expect(workerHtml).toContain("src=\"/src/workerpage/main.ts\"")
    expect(workerEntry).toContain("import { WorkerPage } from './workercmd'")
    expect(workerEntry).toContain("setActivePinia(pinia)")
    expect(workerEntry).not.toContain("from '../main'")
    expect(workerEntry).not.toContain('@arco-design')
    expect(windowSource).not.toContain('splash')
    expect(windowSource).not.toContain('?splash=')
    expect(windowSource).toContain("page === 'worker' ? new URL('worker.html', devUrl).toString() : devUrl")
    expect(ipcSource).not.toContain("data.page === 'PageMusic' ? 'music' : undefined")
  })
})
