import { describe, expect, it, vi, beforeEach } from 'vitest'
import path from 'path'
import os from 'os'
import fs from 'fs'

vi.mock('electron', () => ({
  app: {
    getPath: vi.fn(() => path.join(os.tmpdir(), 'motrix-test-config')),
    getAppPath: vi.fn(() => os.tmpdir()),
    getLocale: vi.fn(() => 'zh-CN'),
    getLoginItemSettings: vi.fn(() => ({ openAtLogin: false }))
  }
}))

const tmpDir = path.join(os.tmpdir(), 'motrix-test-config')

beforeEach(() => {
  fs.mkdirSync(tmpDir, { recursive: true })
  try { fs.unlinkSync(path.join(tmpDir, 'system.json')) } catch {}
  try { fs.unlinkSync(path.join(tmpDir, 'user.json')) } catch {}
})

describe('ConfigManager', () => {
  it('initializes system config with defaults', async () => {
    const { default: ConfigManager } = await import('../ConfigManager')
    const cm = new ConfigManager()
    expect(cm.getSystemConfig('rpc-listen-port')).toBe(16800)
    expect(typeof cm.getSystemConfig('rpc-secret')).toBe('string')
    expect(cm.getSystemConfig('rpc-secret').length).toBeGreaterThan(0)
  })

  it('initializes user config with defaults', async () => {
    const { default: ConfigManager } = await import('../ConfigManager')
    const cm = new ConfigManager()
    expect(cm.getUserConfig('protocols')).toEqual({ mo: true, motrix: true })
    expect(typeof cm.getUserConfig('locale')).toBe('string')
  })

  it('removes obsolete BT and UPnP settings from existing config', async () => {
    fs.writeFileSync(path.join(tmpDir, 'user.json'), JSON.stringify({
      'auto-sync-tracker': false,
      'enable-upnp': false,
      'keep-seeding': true,
      'protocols': { mo: true, motrix: true, magnet: true },
      'proxy': { enable: true, server: 'http://127.0.0.1:7890', scope: ['download', 'update-trackers'] },
      'locale': 'zh-CN'
    }))
    fs.writeFileSync(path.join(tmpDir, 'system.json'), JSON.stringify({
      'bt-tracker': 'udp://tracker.example.com:80',
      'dht-listen-port': 6881,
      'rpc-listen-port': 16800
    }))
    const { default: ConfigManager } = await import('../ConfigManager')
    const cm = new ConfigManager()
    expect(cm.getUserConfig('auto-sync-tracker')).toBeUndefined()
    expect(cm.getUserConfig('enable-upnp')).toBeUndefined()
    expect(cm.getUserConfig('keep-seeding')).toBeUndefined()
    expect(cm.getUserConfig('protocols')).toEqual({ mo: true, motrix: true })
    expect(cm.getUserConfig('proxy').scope).toEqual(['download'])
    expect(cm.getSystemConfig('bt-tracker')).toBeUndefined()
    expect(cm.getSystemConfig('dht-listen-port')).toBeUndefined()
  })

  it('setSystemConfig and getSystemConfig round-trips', async () => {
    const { default: ConfigManager } = await import('../ConfigManager')
    const cm = new ConfigManager()
    cm.setSystemConfig('split', 8)
    expect(cm.getSystemConfig('split')).toBe(8)
  })

  it('setUserConfig and getUserConfig round-trips', async () => {
    const { default: ConfigManager } = await import('../ConfigManager')
    const cm = new ConfigManager()
    cm.setUserConfig('theme', 'light')
    expect(cm.getUserConfig('theme')).toBe('light')
  })
})
