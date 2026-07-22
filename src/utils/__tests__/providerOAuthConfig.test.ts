import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

const read = (path: string) => readFileSync(path, 'utf8')

describe('provider OAuth runtime configuration', () => {
  it('keeps runtime credential resolution in retained provider modules', () => {
    expect(read('src/pikpak/auth.ts')).toContain("PIKPAK_PROTOCOL_CLIENT_ID = 'YUMx5nI8ZU8Ap8pm'")
    expect(read('src/pikpak/auth.ts')).not.toContain('secrets.generated')
    expect(read('src/user/UserLogin.vue')).not.toContain('guangya')
  })

  it('does not retain Aliyun OAuth in the active login implementation', () => {
    const login = read('src/user/UserLogin.vue')
    expect(login).not.toContain('AliUser')
    expect(login).not.toContain('getAliyunOpenApiCredentials')
    expect(login).not.toContain('aliyun')
  })

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
    expect(login).not.toContain('<webview')
    expect(login).not.toContain('openPikPakCaptchaInBrowser')
    expect(login).not.toContain('window.Electron.shell.openExternal')
    expect(login).not.toContain('输入开发者账号')
    expect(login).not.toContain('uiOpenApiClientSecret')
  })

  it('injects complete confidential OAuth credentials into Windows release builds', () => {
    const workflow = read('.github/workflows/release.yml')
    const envExample = read('.env.example')
    expect(workflow).toContain('ONEDRIVE_CLIENT_SECRET: ${{ secrets.ONEDRIVE_CLIENT_SECRET }}')
    expect(workflow).toContain('DROPBOX_APP_SECRET: ${{ secrets.DROPBOX_APP_SECRET }}')
    expect(workflow).toContain('REQUIRED_RELEASE_SECRETS:')
    expect(workflow).toContain('DROPBOX_APP_SECRET')
    expect(envExample).toContain('ONEDRIVE_CLIENT_SECRET=')
    expect(envExample).toContain('DROPBOX_APP_SECRET=')
  })

  it('keeps the current drive id when a single-root provider node is selected', () => {
    const selector = read('src/pan/topbtns/SelectPanDirModal.vue')
    expect(selector).toContain('const selectedDriveId = GetDriveID(user_id.value, parentNode.key || key) || drive_id.value')
    expect(selector).toContain('drive_id: selectedDriveId')
  })
})
