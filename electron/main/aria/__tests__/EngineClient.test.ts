import { beforeEach, describe, expect, it, vi } from 'vitest'

const ariaCall = vi.fn().mockResolvedValue({})

vi.mock('aria2-lib', () => ({
  default: vi.fn().mockImplementation(() => ({
    open: vi.fn().mockResolvedValue(undefined),
    close: vi.fn().mockResolvedValue(undefined),
    call: ariaCall,
    on: vi.fn(),
    setMaxListeners: vi.fn()
  }))
}))

vi.mock('electron', () => ({
  app: {
    getPath: vi.fn(() => '/tmp'),
    getAppPath: vi.fn(() => '/tmp'),
    getLocale: vi.fn(() => 'zh-CN'),
    getLoginItemSettings: vi.fn(() => ({ openAtLogin: false }))
  }
}))

describe('EngineClient', () => {
  beforeEach(() => {
    ariaCall.mockReset()
    ariaCall.mockImplementation(() => Promise.resolve({}))
  })

  it('can be instantiated', async () => {
    const { default: EngineClient } = await import('../EngineClient')
    const client = new EngineClient({ port: 16800, secret: 'test' })
    expect(client).toBeDefined()
  })

  it('init() calls connect and does not throw', async () => {
    const { default: EngineClient } = await import('../EngineClient')
    const client = new EngineClient({ port: 16800, secret: 'test' })
    const connect = vi.spyOn(client, 'connect').mockImplementation(() => undefined)

    client.init()

    expect(connect).toHaveBeenCalledOnce()
  })

  it('waits for the dynamically loaded RPC client before calling it', async () => {
    const { default: EngineClient } = await import('../EngineClient')
    const client = new EngineClient({ port: 16800, secret: 'test' })
    client.init()

    await expect(client.call('getGlobalStat')).resolves.toEqual({})
    expect(ariaCall).toHaveBeenCalledWith('getGlobalStat')
  })

  it('waits for aria2 readiness before changing global options', async () => {
    ariaCall
      .mockRejectedValueOnce(new Error('fetch failed'))
      .mockResolvedValueOnce({ version: '1.37.0' })
      .mockResolvedValueOnce({})
    const { default: EngineClient } = await import('../EngineClient')
    const client = new EngineClient({ port: 16800, secret: 'test' })

    await expect(client.changeGlobalOption({ 'all-proxy': 'http://127.0.0.1:7890' }, { attempts: 2, delayMs: 0 })).resolves.toEqual({})
    expect(ariaCall.mock.calls).toEqual([
      ['getVersion'],
      ['getVersion'],
      ['changeGlobalOption', { 'all-proxy': 'http://127.0.0.1:7890' }]
    ])
  })

  it('propagates RPC failures so callers can retry or report them', async () => {
    ariaCall.mockRejectedValueOnce(new Error('fetch failed'))
    const { default: EngineClient } = await import('../EngineClient')
    const client = new EngineClient({ port: 16800, secret: 'test' })

    await expect(client.call('getGlobalStat')).rejects.toThrow('fetch failed')
  })
})
