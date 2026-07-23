import { EventEmitter } from 'node:events'
import fs from 'node:fs'
import path from 'node:path'
import { describe, expect, it, vi } from 'vitest'
import { createAutoUpdateController } from '../autoUpdate'

class FakeUpdater extends EventEmitter {
  autoDownload = true
  autoInstallOnAppQuit = true
  allowPrerelease = false
  checkForUpdates = vi.fn()
  downloadUpdate = vi.fn().mockResolvedValue(undefined)
  quitAndInstall = vi.fn()
}

describe('createAutoUpdateController', () => {
  it('uses the Mnemo GitHub repository as the release feed', () => {
    const builder = JSON.parse(fs.readFileSync(path.resolve(process.cwd(), 'electron-builder.json'), 'utf8'))
    expect(builder.publish).toEqual(expect.arrayContaining([
      expect.objectContaining({ provider: 'github', owner: 'lllll081926i', repo: 'Mnemo', releaseType: 'release' })
    ]))
  })

  it('publishes a latest stable release with every required provider credential', () => {
    const workflow = fs.readFileSync(path.resolve(process.cwd(), '.github/workflows/release.yml'), 'utf8')
    expect(workflow).toContain('ONEDRIVE_CLIENT_ID')
    expect(workflow).toContain('DROPBOX_APP_KEY')
    expect(workflow).toContain('GOOGLE_DRIVE_CLIENT_ID')
    expect(workflow).not.toMatch(/BAIDU_|CLOUD123_|DRIVE115_|BOX_CLIENT/)
    expect(workflow).toContain('unset CSC_LINK CSC_KEY_PASSWORD')
    expect(workflow).toContain('-f prerelease=')
    expect(workflow).toContain('-f draft=false')
  })

  it('asks before downloading an available update', async () => {
    const updater = new FakeUpdater()
    const dialog = {
      showMessageBox: vi.fn().mockResolvedValue({ response: 0 })
    }

    createAutoUpdateController({
      updater,
      dialog,
      logger: { info: vi.fn(), warn: vi.fn() },
      currentVersion: '4.0.11-beta',
      isPackaged: true
    })

    updater.emit('update-available', { version: '4.0.12-beta', releaseNotes: '修复若干问题' })
    await Promise.resolve()
    await Promise.resolve()

    expect(dialog.showMessageBox).toHaveBeenCalledWith(expect.objectContaining({
      type: 'info',
      title: '检测到新版本',
      message: '检测到新版本 4.0.12-beta，是否下载并安装？',
      buttons: ['下载并安装', '暂不更新'],
      defaultId: 0,
      cancelId: 1
    }))
    expect(updater.downloadUpdate).toHaveBeenCalledTimes(1)
    expect(updater.allowPrerelease).toBe(true)
    expect(updater.autoDownload).toBe(false)
    expect(updater.autoInstallOnAppQuit).toBe(false)
  })

  it('does not download when the user declines the update', async () => {
    const updater = new FakeUpdater()
    const dialog = {
      showMessageBox: vi.fn().mockResolvedValue({ response: 1 })
    }

    createAutoUpdateController({
      updater,
      dialog,
      logger: { info: vi.fn(), warn: vi.fn() },
      currentVersion: '4.0.11',
      isPackaged: true
    })

    updater.emit('update-available', { version: '4.0.12' })
    await Promise.resolve()
    await Promise.resolve()

    expect(updater.downloadUpdate).not.toHaveBeenCalled()
    updater.emit('update-downloaded', { version: '4.0.12' })
    expect(updater.quitAndInstall).not.toHaveBeenCalled()
  })

  it('installs immediately after an accepted update is downloaded and relaunches the app', async () => {
    const updater = new FakeUpdater()
    const dialog = {
      showMessageBox: vi.fn().mockResolvedValue({ response: 0 })
    }

    createAutoUpdateController({
      updater,
      dialog,
      logger: { info: vi.fn(), warn: vi.fn() },
      currentVersion: '4.0.11-beta',
      isPackaged: true
    })

    updater.emit('update-available', { version: '4.0.12-beta' })
    await Promise.resolve()
    await Promise.resolve()
    updater.emit('update-downloaded', { version: '4.0.12-beta' })

    expect(updater.quitAndInstall).toHaveBeenCalledWith(false, true)
  })
})
