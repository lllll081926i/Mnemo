import type { ITokenInfo } from '../user/userstore'
import { createWebDavConnection, createWebDavUserToken, type WebDavConnectionConfig } from '../utils/webdavClient'

export interface NextcloudLoginInput {
  name: string
  url: string
  username: string
  password: string
  rootPath?: string
}

export const normalizeNextcloudWebDavUrl = (value: string, username: string) => {
  const url = new URL(value.trim())
  if (url.protocol !== 'https:' && url.protocol !== 'http:') throw new Error('Nextcloud 地址必须使用 HTTP 或 HTTPS')
  const pathname = url.pathname.replace(/\/+$/, '')
  if (/\/remote\.php\/(?:dav\/files\/[^/]+|webdav)$/i.test(pathname)) {
    url.pathname = pathname
  } else {
    const basePath = pathname === '/' ? '' : pathname
    url.pathname = `${basePath}/remote.php/dav/files/${encodeURIComponent(username.trim())}`
  }
  url.search = ''
  url.hash = ''
  return url.toString().replace(/\/$/, '')
}

export const createNextcloudConnection = (input: NextcloudLoginInput): WebDavConnectionConfig => createWebDavConnection({
  provider: 'nextcloud',
  name: input.name,
  url: normalizeNextcloudWebDavUrl(input.url, input.username),
  username: input.username,
  password: input.password,
  rootPath: input.rootPath
})

export const createNextcloudUserToken = (connection: WebDavConnectionConfig): ITokenInfo => createWebDavUserToken({ ...connection, provider: 'nextcloud' })
