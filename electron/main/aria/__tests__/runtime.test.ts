import { afterEach, describe, expect, it, vi } from 'vitest'
import { ENGINE_RPC_PORT } from '@shared/constants'
import { getMotrixApplicationRpcPort, parseElectronProxyRules, syncMotrixApplicationProxy } from '../runtime'

describe('Motrix aria runtime', () => {
  afterEach(() => {
    delete (globalThis as any).motrixApplication
  })

  it('returns MotrixApplication rpc-listen-port when initialized', () => {
    ;(globalThis as any).motrixApplication = {
      configManager: {
        getSystemConfig: (key: string) => key === 'rpc-listen-port' ? 16999 : undefined
      }
    }

    expect(getMotrixApplicationRpcPort()).toBe(16999)
  })

  it('falls back to the Motrix default rpc port before initialization', () => {
    expect(getMotrixApplicationRpcPort()).toBe(ENGINE_RPC_PORT)
  })

  it('converts Electron resolved proxy rules to an aria all-proxy URL', () => {
    expect(parseElectronProxyRules('PROXY 127.0.0.1:7890; DIRECT')).toBe('http://127.0.0.1:7890')
    expect(parseElectronProxyRules('HTTPS proxy.example.com:443; DIRECT')).toBe('https://proxy.example.com:443')
    expect(parseElectronProxyRules('SOCKS5 127.0.0.1:1080')).toBe('socks5://127.0.0.1:1080')
    expect(parseElectronProxyRules('SOCKS 127.0.0.1:1080')).toBe('socks5://127.0.0.1:1080')
    expect(parseElectronProxyRules('DIRECT')).toBe('')
  })

  it('updates both persisted and running aria proxy configuration', async () => {
    const setSystemConfig = vi.fn()
    const changeGlobalOption = vi.fn().mockResolvedValue(undefined)
    ;(globalThis as any).motrixApplication = {
      configManager: { setSystemConfig },
      engineClient: { changeGlobalOption }
    }

    await syncMotrixApplicationProxy('http://127.0.0.1:7890')

    expect(setSystemConfig).toHaveBeenCalledWith({ 'all-proxy': 'http://127.0.0.1:7890', 'no-proxy': '' })
    expect(changeGlobalOption).toHaveBeenCalledWith({ 'all-proxy': 'http://127.0.0.1:7890', 'no-proxy': '' })
  })

  it('does not reject Electron proxy application when aria is temporarily unavailable', async () => {
    ;(globalThis as any).motrixApplication = {
      configManager: { setSystemConfig: vi.fn() },
      engineClient: { changeGlobalOption: vi.fn().mockRejectedValue(new Error('aria unavailable')) }
    }

    await expect(syncMotrixApplicationProxy('http://127.0.0.1:7890')).resolves.toBeUndefined()
  })
})
