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
      expect.objectContaining({ provider: 'github', owner: 'lllll081926i', repo: 'Mnemo' })
    ]))
  })

  it('downloads an available update silently in the background', async () => {
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

    expect(dialog.showMessageBox).not.toHaveBeenCalled()
    expect(updater.downloadUpdate).toHaveBeenCalledTimes(1)
    expect(updater.allowPrerelease).toBe(true)
    expect(updater.autoDownload).toBe(false)
    expect(updater.autoInstallOnAppQuit).toBe(true)
  })

  it('prompts to restart after an update is downloaded', async () => {
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

    updater.emit('update-downloaded', { version: '4.0.12-beta' })
    await Promise.resolve()

    expect(dialog.showMessageBox).toHaveBeenCalledWith(expect.objectContaining({
      type: 'info',
      title: '更新已下载',
      message: '新版本 4.0.12-beta 已在后台下载完成',
      detail: '重启 App 即可完成更新安装。',
      buttons: ['重启安装', '稍后']
    }))
    expect(updater.quitAndInstall).toHaveBeenCalledWith(false, true)
  })
})
