import { describe, expect, it, vi } from 'vitest'

vi.mock('electron', () => ({
  app: {
    getPath: vi.fn(() => '/tmp/motrix-utils-test'),
    getAppPath: vi.fn(() => '/Users/test/mnemo'),
    isPackaged: false
  }
}))

vi.mock('electron-is', () => ({
  default: {
    dev: () => true
  }
}))

describe('aria utils', () => {
  it('resolves dev engine binaries from the static engine directory', async () => {
    const { getAria2BinPath, getAria2ConfPath } = await import('../utils')

    const bin = getAria2BinPath('darwin', 'arm64').replace(/\\/g, '/')
    const conf = getAria2ConfPath('darwin', 'arm64').replace(/\\/g, '/')

    // Path prefix may be mock app path or real project cwd (static always exists in repo).
    // Assert layout only: .../static/engine/<platform>/<arch>/aria2c
    expect(bin).toMatch(/\/static\/engine\/darwin\/arm64\/aria2c$/)
    expect(conf).toMatch(/\/static\/engine\/darwin\/arm64\/aria2\.conf$/)
  })
})
