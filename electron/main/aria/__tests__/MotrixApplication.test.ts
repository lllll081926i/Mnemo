import { describe, expect, it, vi } from 'vitest'

vi.mock('electron', () => ({
  app: {
    getPath: vi.fn(() => '/tmp/motrix-app-test'),
    getAppPath: vi.fn(() => '/tmp'),
    getLocale: vi.fn(() => 'zh-CN'),
    getLoginItemSettings: vi.fn(() => ({ openAtLogin: false })),
    setAsDefaultProtocolClient: vi.fn(),
    removeAsDefaultProtocolClient: vi.fn()
  },
  ipcMain: { handle: vi.fn(), on: vi.fn(), off: vi.fn() },
  BrowserWindow: { getAllWindows: vi.fn(() => []) },
  dialog: { showMessageBox: vi.fn() }
}))

vi.mock('child_process', () => ({
  spawn: vi.fn(() => ({
    pid: 999,
    on: vi.fn(),
    stdout: { on: vi.fn() },
    stderr: { on: vi.fn() },
    kill: vi.fn(() => true)
  }))
}))

vi.mock('aria2-lib', () => ({
  default: vi.fn().mockImplementation(() => ({
    open: vi.fn().mockResolvedValue(undefined),
    close: vi.fn().mockResolvedValue(undefined),
    call: vi.fn().mockResolvedValue({}),
    on: vi.fn(),
    setMaxListeners: vi.fn()
  }))
}))

describe('MotrixApplication', () => {
  it('can be instantiated', async () => {
    const { default: MotrixApplication } = await import('../MotrixApplication')
    const app = new MotrixApplication()
    expect(app).toBeDefined()
  })

  it('initialized flag starts false', async () => {
    const { default: MotrixApplication } = await import('../MotrixApplication')
    const app = new MotrixApplication() as any
    expect(app.initialized).toBe(false)
  })

  it('defers the aria engine until a download client requests it', async () => {
    const { default: MotrixApplication } = await import('../MotrixApplication')
    const app = new MotrixApplication()
    const startEngine = vi.spyOn(app, 'startEngine').mockImplementation(() => {})
    const initEngineClient = vi.spyOn(app, 'initEngineClient').mockImplementation(() => {
      ;(app as any).engineClient = { waitUntilReady: vi.fn().mockResolvedValue(undefined) }
    })

    await app.init()
    expect(startEngine).not.toHaveBeenCalled()
    expect(initEngineClient).not.toHaveBeenCalled()

    await Promise.all([app.ensureEngineReady(), app.ensureEngineReady()])
    expect(startEngine).toHaveBeenCalledTimes(1)
    expect(initEngineClient).toHaveBeenCalledTimes(1)
  })

  it('quit() resolves without throwing when not started', async () => {
    const { default: MotrixApplication } = await import('../MotrixApplication')
    const app = new MotrixApplication()
    await expect(app.quit()).resolves.not.toThrow()
  })
})
