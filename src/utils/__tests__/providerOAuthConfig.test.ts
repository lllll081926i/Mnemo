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

  it('routes shared protocol callbacks by OAuth state and only marks opened URLs after shell success', () => {
    const login = read('src/user/UserLogin.vue')
    expect(login).toContain('const callbackProvider = oauthLoginProviders.find((provider) => oauthStates.get(provider) === state)')
    expect(login).toContain("oauthStates.delete(callbackProvider)")
    expect(login).toContain('await window.Electron.shell.openExternal(authUrl)')
  })
})
