import { describe, expect, it, vi } from 'vitest'

vi.mock('electron', () => ({
  app: {
    setAsDefaultProtocolClient: vi.fn(),
    removeAsDefaultProtocolClient: vi.fn(),
    isDefaultProtocolClient: vi.fn(() => false),
    getPath: vi.fn(() => '/tmp'),
    getAppPath: vi.fn(() => '/tmp'),
    getLocale: vi.fn(() => 'zh-CN'),
    getLoginItemSettings: vi.fn(() => ({ openAtLogin: false }))
  }
}))

describe('ProtocolManager', () => {
  it('can be instantiated with protocols config', async () => {
    const { default: ProtocolManager } = await import('../ProtocolManager')
    const mgr = new ProtocolManager({ protocols: { magnet: true, mo: false } })
    expect(mgr.protocols).toEqual({ mo: false, motrix: true })
  })

  it('init() does not throw', async () => {
    const { default: ProtocolManager } = await import('../ProtocolManager')
    const mgr = new ProtocolManager({ protocols: { mo: true } })
    expect(() => mgr.init()).not.toThrow()
  })

  it('ignores magnet resources after local BT support is removed', async () => {
    const { default: ProtocolManager } = await import('../ProtocolManager')
    const mgr = new ProtocolManager()
    const sendCommandToAll = vi.fn()
    ;(globalThis as any).motrixApplication = { sendCommandToAll }

    mgr.handle('magnet:?xt=urn:btih:abc')

    expect(sendCommandToAll).not.toHaveBeenCalled()
    delete (globalThis as any).motrixApplication
  })
})
