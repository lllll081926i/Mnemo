import { describe, expect, it } from 'vitest'
import { createNextcloudConnection, createNextcloudUserToken, normalizeNextcloudWebDavUrl } from '../auth'
import { buildWebDavDownloadUrl, getWebDavConnectionId, getWebDavRequestHeaders, isWebDavDrive } from '../../utils/webdavClient'

describe('Nextcloud WebDAV adapter', () => {
  it('builds a user-scoped DAV endpoint from a server base URL', () => {
    expect(normalizeNextcloudWebDavUrl('https://cloud.example.com/', 'alice@example.com')).toBe('https://cloud.example.com/remote.php/dav/files/alice%40example.com')
    expect(normalizeNextcloudWebDavUrl('https://cloud.example.com/nextcloud/remote.php/webdav', 'alice')).toBe('https://cloud.example.com/nextcloud/remote.php/webdav')
  })

  it('creates independent account and drive ids', () => {
    const first = createNextcloudConnection({ name: 'Work', url: 'https://cloud.example.com', username: 'alice', password: 'app-password' })
    const second = createNextcloudConnection({ name: 'Home', url: 'https://cloud.example.com', username: 'bob', password: 'app-password' })
    const firstToken = createNextcloudUserToken(first)
    const secondToken = createNextcloudUserToken(second)

    expect(firstToken.tokenfrom).toBe('nextcloud')
    expect(firstToken.default_drive_id).toMatch(/^nextcloud:/)
    expect(firstToken.default_drive_id).not.toBe(secondToken.default_drive_id)
    expect(isWebDavDrive(firstToken.default_drive_id)).toBe(true)
    expect(getWebDavConnectionId(firstToken.default_drive_id)).toBe(first.id)
  })

  it('keeps credentials out of download URLs and returns an auth header', () => {
    const connection = createNextcloudConnection({ name: 'Cloud', url: 'https://cloud.example.com/base', username: 'alice', password: 'secret' })
    const url = buildWebDavDownloadUrl(connection, '/Photos/a b.jpg')
    const parsed = new URL(url)

    expect(parsed.username).toBe('')
    expect(parsed.password).toBe('')
    expect(decodeURIComponent(parsed.pathname)).toBe('/base/remote.php/dav/files/alice/Photos/a b.jpg')
    expect(getWebDavRequestHeaders(connection).Authorization).toBe(`Basic ${btoa('alice:secret')}`)
  })
})
