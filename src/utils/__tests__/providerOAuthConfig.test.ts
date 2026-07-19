import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

const read = (path: string) => readFileSync(path, 'utf8')

describe('provider OAuth runtime configuration', () => {
  it('builds 123 and Baidu authorization URLs from runtime credentials and state', async () => {
    const { buildCloud123AuthUrl } = await import('../cloud123')
    const { buildBaiduAuthUrl } = await import('../baidu')
    const cloud123 = new URL(buildCloud123AuthUrl('cloud123-client', 'cloud123-state'))
    const baidu = new URL(buildBaiduAuthUrl('baidu-client', 'baidu-state'))
    expect(cloud123.searchParams.get('client_id')).toBe('cloud123-client')
    expect(cloud123.searchParams.get('state')).toBe('cloud123-state')
    expect(baidu.searchParams.get('client_id')).toBe('baidu-client')
    expect(baidu.searchParams.get('state')).toBe('baidu-state')
  })

  it('keeps runtime credential resolution in each provider module', () => {
    expect(read('src/utils/cloud123.ts')).toContain('resolveCloud123OAuthCredentials')
    expect(read('src/utils/baidu.ts')).toContain('resolveBaiduOAuthCredentials')
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
  })

  it('routes shared protocol callbacks by OAuth state and only marks opened URLs after shell success', () => {
    const login = read('src/user/UserLogin.vue')
    expect(login).toContain('const callbackProvider = oauthLoginProviders.find((provider) => oauthStates.get(provider) === state)')
    expect(login).toContain("oauthStates.delete(callbackProvider)")
    expect(login).toContain('await window.Electron.shell.openExternal(authUrl)')
  })
})
