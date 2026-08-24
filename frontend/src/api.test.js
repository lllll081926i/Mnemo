import { describe, expect, it, vi } from 'vitest'
import {
  accountDetail,
  accountName,
  capsOf,
  extOf,
  formatBytes,
  login,
  notifyFileChange,
  openKindOf,
  onFileChange,
  providerOf,
} from './api'
import { info as writeLog } from './logger'
import { accountOrderKey, orderAccounts } from './appearance'

const providers = [
  { ID: 'pikpak', Meta: { label: 'PikPak' }, Capabilities: { quota: true, upload: true } },
  { ID: 'webdav', Meta: { label: 'WebDAV' }, Capabilities: { upload: true } },
]

describe('api 纯逻辑辅助函数', () => {
  it('登录 RPC 只传播后端错误，不在前端 API 层重复告警', async () => {
    const providerLogin = vi.fn().mockRejectedValue(new Error('provider rejected login'))
    Object.defineProperty(window, 'go', {
      configurable: true,
      value: { app: { App: { ProviderLogin: providerLogin } } },
    })
    const spy = vi.spyOn(console, 'warn').mockImplementation(() => {})
    try {
      await expect(login('pikpak', { username: 'test@example.com' })).rejects.toThrow('provider rejected login')
      expect(providerLogin).toHaveBeenCalledTimes(1)
      expect(spy).not.toHaveBeenCalled()
    } finally {
      spy.mockRestore()
    }
  })

  it('前端日志会脱敏并合并短时间内的重复动作', () => {
    const spy = vi.spyOn(console, 'info').mockImplementation(() => {})
    try {
      const fields = { access_token: 'secret-value', count: 1 }
      writeLog('test', '重复动作测试', fields)
      writeLog('test', '重复动作测试', fields)

      expect(spy).toHaveBeenCalledTimes(1)
      expect(spy.mock.calls[0][0]).toContain('[INFO] 重复动作测试 | scope=test')
      expect(spy.mock.calls[0][1]).toEqual({ access_token: '[REDACTED]', count: '1' })
    } finally {
      spy.mockRestore()
    }
  })

  it('文件变更通知会按账号、存储和目录精确分发，并支持取消订阅', () => {
    const received = []
    const off = onFileChange((change) => received.push(change))
    const result = notifyFileChange({
      user_id: 'dropbox_a',
      drive_id: 'dropbox:drive-a',
      directories: ['root', 'folder-a', 'root', ''],
      refreshTrash: true,
      minimumInterval: 1000,
    })

    expect(result).toMatchObject({
      userId: 'dropbox_a',
      driveId: 'dropbox:drive-a',
      directories: ['root', 'folder-a'],
      refreshTrash: true,
      minimumInterval: 1000,
    })
    expect(received).toEqual([result])

    off()
    notifyFileChange({ userId: 'dropbox_a', driveId: 'dropbox:drive-a', directory: 'folder-b' })
    expect(received).toHaveLength(1)

    const replayed = []
    const stopReplay = onFileChange((change) => replayed.push(change))
    expect(replayed).toHaveLength(1)
    expect(replayed[0].directories).toEqual(['folder-b'])
    stopReplay()
  })

  it('解析普通账号和挂载账号的 provider', () => {
    expect(providerOf('pikpak_user-1')).toBe('pikpak')
    expect(providerOf('webdav:mount-1')).toBe('webdav')
    expect(providerOf('s3:mount-1')).toBe('s3')
    expect(providerOf('')).toBe('')
  })

  it('账号排序同时兼容旧 user_id 偏好与多存储账号键', () => {
    const first = { user_id: 'pikpak_first', drive_id: 'drive-a' }
    const second = { user_id: 'dropbox_second', drive_id: 'drive-b' }
    const third = { user_id: 'webdav:third', drive_id: 'drive-c' }
    const mountedA = { user_id: 'webdav:shared', drive_id: 'drive-a' }
    const mountedB = { user_id: 'webdav:shared', drive_id: 'drive-b' }

    expect(accountOrderKey(second)).toBe('dropbox_second\u0000drive-b')
    expect(orderAccounts([first, second, third], ['webdav:third', 'dropbox_second'])).toEqual([third, second, first])
    expect(orderAccounts([first, second, third], [accountOrderKey(second)])).toEqual([second, first, third])
    expect(orderAccounts([mountedA, mountedB], [accountOrderKey(mountedB), accountOrderKey(mountedA)])).toEqual([mountedB, mountedA])
  })

  it('优先使用账号显示名并清理供应商前缀', () => {
    const account = {
      user_id: 'pan189_user-1',
      token: { nick_name: '189 · 家庭云' },
    }
    expect(accountName(account)).toBe('189')
    expect(accountDetail(account, providers)).toBe('pan189 · 189')
  })

  it('返回 provider 能力并兼容大小写字段', () => {
    expect(capsOf({ user_id: 'pikpak_user-1' }, providers)).toEqual({ quota: true, upload: true })
    expect(capsOf({ user_id: 'webdav:mount-1' }, providers)).toEqual({ upload: true })
    expect(capsOf({ user_id: 'missing_1' }, providers)).toEqual({})
  })

  it('格式化容量和扩展名边界', () => {
    expect(formatBytes(0)).toBe('0 B')
    expect(formatBytes(1024)).toBe('1.0 KB')
    expect(formatBytes(1024 * 1024)).toBe('1.0 MB')
    expect(extOf('README')).toBe('')
    expect(extOf('photo.JPG')).toBe('jpg')
  })

  it('按文件类型选择打开方式，目录优先', () => {
    expect(openKindOf({ isDir: true, name: 'movie.mp4' })).toBe('dir')
		 expect(openKindOf({ name: 'movie.mp4' })).toBe('video')
		expect(openKindOf({ name: 'stream.m3u8' })).toBe('video')
		expect(openKindOf({ name: 'movie.mkv' })).toBe('download')
		expect(openKindOf({ name: 'cover.png' })).toBe('image')
		expect(openKindOf({ name: 'animated.GIF' })).toBe('image')
		expect(openKindOf({ name: 'cover.heic' })).toBe('download')
		expect(openKindOf({ name: 'notes.md' })).toBe('text')
		expect(openKindOf({ name: 'manual.pdf' })).toBe('pdf')
		expect(openKindOf({ name: 'manual.bin', mime_type: 'application/pdf' })).toBe('pdf')
		expect(openKindOf({ name: 'archive.zip' })).toBe('download')
	})
})
