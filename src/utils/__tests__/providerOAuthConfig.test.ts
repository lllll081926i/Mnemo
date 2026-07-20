import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

const read = (path: string) => readFileSync(path, 'utf8')

describe('provider OAuth runtime configuration', () => {
  it('keeps runtime credential resolution in retained provider modules', () => {
    expect(read('src/pikpak/auth.ts')).toContain('resolvePikPakCredentials')
    expect(read('src/guangya/auth.ts')).toContain('resolveGuangyaClientId')
  })

  it('keeps the Aliyun device signature byte-compatible after the curve implementation update', async () => {
    ;(globalThis as any).self = globalThis
    const { GetDeviceId, GetSignature } = await import('../../aliapi/utils')
    expect(GetDeviceId('123')).toBe('40bd0015-6308-5fc3-9165-329ea1ff5c5e')
    expect(GetDeviceId('test-user')).toBe('c56486f8-b638-563e-8425-1d0c8ab0b4fb')
    expect(GetDeviceId('中文')).toBe('7be2d2d2-0c10-5eee-8836-c9bc2b939890')
    expect(GetSignature(0, '123456789', 'device-test')).toEqual({
      publicKey: '0403f3d0259ffdd80d8545ddd49a4498f7433be93dd9b39f61ceb9f9722e08e613cc',
      signature: 'e6be8339a1b641178669889d167be7ce2b4ca0f7142ded830b98c5a5bfb0f9c771b10cdd3c229ca81817b6c1d9d2abe1ef628a4a5789f1895bb36c4d6d0d63a401'
    })
  }, 15000)

  it('keeps external URL opening behind the validated Electron IPC bridge', () => {
    const login = read('src/user/UserLogin.vue')
    const preload = read('electron/preload/index.ts')
    const mainIpc = read('electron/main/core/ipcEvent.ts')
    expect(login).not.toContain('window.Electron.shell.openExternal')
    expect(preload).toContain("ipcRenderer.invoke('WebOpenExternal', url)")
    expect(mainIpc).toContain("ipcMain.handle('WebOpenExternal'")
    expect(mainIpc).toContain("url.protocol !== 'https:' && url.protocol !== 'http:'")
  })

  it('keeps provider OAuth inside Mnemo instead of opening the system browser', () => {
    const login = read('src/user/UserLogin.vue')
    expect(login).toContain('window.WebOAuthBegin(provider)')
    expect(login).toContain('window.WebOAuthOpen(state, authUrl)')
    expect(login).toContain('window.WebOAuthOnCallback')
    expect(login).not.toContain('WebOpenExternal')
  })
})
