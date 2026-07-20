import { describe, expect, it, vi } from 'vitest'
import { spawn } from 'child_process'

vi.mock('electron', () => ({
  app: {
    getPath: vi.fn(() => '/tmp/motrix-engine-test'),
    getAppPath: vi.fn(() => '/tmp'),
    getLocale: vi.fn(() => 'zh-CN'),
    getLoginItemSettings: vi.fn(() => ({ openAtLogin: false }))
  }
}))

vi.mock('child_process', () => ({
  spawn: vi.fn(() => ({
    pid: 12345,
    on: vi.fn(),
    once: vi.fn(),
    stdout: { on: vi.fn() },
    stderr: { on: vi.fn() },
    kill: vi.fn(() => true)
  }))
}))

describe('Engine', () => {
  it('can be instantiated with system and user configs', async () => {
    const { default: Engine } = await import('../Engine')
    const engine = new Engine({
      systemConfig: { 'rpc-listen-port': 16800, 'rpc-secret': 'test', dir: '/tmp' },
      userConfig: {}
    })
    expect(engine).toBeDefined()
  })

  it('stop() does not throw when not started', async () => {
    const { default: Engine } = await import('../Engine')
    const engine = new Engine({ systemConfig: {}, userConfig: {} })
    expect(() => engine.stop()).not.toThrow()
  })

  it('starts the background engine without opening a console window', async () => {
    const { default: Engine } = await import('../Engine')
    const engine = new Engine({ systemConfig: {}, userConfig: {} })
    vi.spyOn(engine, 'getEngineBinPath').mockReturnValue('aria2c')
    vi.spyOn(engine, 'getStartArgs').mockReturnValue([])
    vi.spyOn(engine, 'writePidFile').mockImplementation(() => {})

    engine.start()

    expect(spawn).toHaveBeenCalledWith(expect.any(String), expect.any(Array), expect.objectContaining({ windowsHide: true }))
  })
})
