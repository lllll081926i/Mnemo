import { useSettingStore } from '../store'
import FlowEnc from '../module/flow-enc'
import http, { Agent as HttpAgent, IncomingMessage, Server, ServerResponse } from 'http'
import Db from './db'
import https, { Agent as HttpsAgent } from 'https'
import { GetExpiresTime } from './utils'
import { decodeName } from '../module/flow-enc/utils'
import { IAliFileItem, IAliGetFileModel } from '../aliapi/alimodels'
import AliFile from '../aliapi/file'
import path from 'path'
import { localPwd } from './aria2c'
import os from 'os'
import DebugLog from './debuglog'
import message from './message'
import UserDAL from '../user/userdal'
import { buildUpstreamProxyHeaders } from './proxyHeaders'
import { shouldRefreshProxyUrl } from './proxyCache'
import { resolveDriveProvider } from './driveProvider'
import { isWebDavDrive } from './webdavClient'

// 默认maxFreeSockets=256
const httpsAgent = new HttpsAgent({ keepAlive: true })
const httpAgent = new HttpAgent({ keepAlive: true })

const detectProxyVideoType = (url: string) => {
  const lower = String(url || '').split('?')[0].split('#')[0].toLowerCase()
  if (lower.endsWith('.m3u8')) return 'm3u8'
  if (lower.endsWith('.mpd')) return 'mpd'
  if (lower.endsWith('.ts')) return 'ts'
  return ''
}

export interface IRawUrl {
  drive_id: string
  file_id: string
  url: string
  size: number
  headers?: Record<string, string>
  qualities: {
    html: string
    quality: string
    height: number
    width: number
    label: string
    value: string
    url: string
    type?: string
    headers?: Record<string, string>
    forceProxy?: boolean
  }[]
  subtitles: {
    language: string
    url: string
    headers?: Record<string, string>
  }[]
}

interface FileInfo {
  user_id: string
  drive_id?: string
  file_id?: string
  file_size?: number
  encType?: string
  proxy_headers?: string

  [key: string]: string | number | undefined
}

export function getIPAddress() {
  let ipv4 = ''
  const interfaces = os.networkInterfaces()
  for (const dev in interfaces) {
    let device = interfaces[dev]
    if (device) {
      device.forEach((details, alias) => {
        if (dev.includes('以太网') || dev == 'WLAN') {
          if (details.family == 'IPv4' && !details.internal
            && details.address.startsWith('192.168')) {
            ipv4 = details.address
            return
          }
        }
      })
    }
  }
  // console.log(ipv4)
  return ipv4 || '127.0.0.1'
}

export function getEncType(file: IAliGetFileModel | IAliFileItem | { description: string }): string {
  let description = file.description
  if (description) {
    if (description.includes('mnemoEncrypt1')) {
      return 'mnemoEncrypt1'
    } else if (description.includes('mnemoEncrypt2')) {
      return 'mnemoEncrypt2'
    }
  }
  return ''
}

export function getEncPassword(user_id: string, encType: string, inputpassword: string = ''): string {
  if (encType) {
    if (inputpassword) {
      return inputpassword
    }
    let settingStore = useSettingStore()
    if (encType == 'mnemoEncrypt1') {
      let ecnPassword = decodeName(localPwd, settingStore.securityEncType, settingStore.securityPassword)
      if (!ecnPassword) {
        ecnPassword = decodeName(user_id, settingStore.securityEncType, settingStore.securityPassword)
      }
      return ecnPassword || ''
    }
    return user_id
  }
  return ''
}

export function getFlowEnc(user_id: string, fileSize: number, encType: string, password: string = '') {
  if (!encType) return null
  let settingStore = useSettingStore()
  const securityPassword = getEncPassword(user_id, encType, password)
  const securityEncType = settingStore.securityEncType
  return new FlowEnc(securityPassword, securityEncType, fileSize)
}

export function getProxyUrl(info: FileInfo) {
  let { debugProxyHost, debugProxyPort } = useSettingStore()
  let proxyUrl = `http://${debugProxyHost}:${debugProxyPort}/proxy`
  let params = Object.keys(info).filter(v => info[v])
    .map((key: string) => `${encodeURIComponent(key)}=${encodeURIComponent(info[key]!!)}`)
  return `${proxyUrl}?${params.join('&')}`
}

export function isLocalProxyUrl(url: string) {
  const { debugProxyHost, debugProxyPort } = useSettingStore()
  return String(url || '').startsWith(`http://${debugProxyHost}:${debugProxyPort}/proxy?`)
}

export function getRedirectUrl(info: FileInfo) {
  let { debugProxyHost, debugProxyPort } = useSettingStore()
  let redirectUrl = `http://${debugProxyHost}:${debugProxyPort}/redirect`
  let params = Object.keys(info).filter(v => info[v])
    .map((key: string) => `${encodeURIComponent(key)}=${encodeURIComponent(info[key]!!)}`)
  return `${redirectUrl}?${params.join('&')}`
}

export async function getRawUrl(
  user_id: string,
  drive_id: string,
  file_id: string,
  encType: string = '',
  password: string = '',
  weifa: boolean = false,
  preview_type: string = '',
  quality: string = '',
  promotionSkuCode = ''
): Promise<string | IRawUrl> {
  let data: any = {
    drive_id: drive_id,
    file_id: file_id,
    url: '',
    size: 0,
    qualities: [],
    subtitles: []
  }
  let { uiVideoQuality, uiVideoPlayer, securityPreviewAutoDecrypt } = useSettingStore()
  // 违规视频也使用转码播放
  if (!encType && preview_type) {
    if (weifa || preview_type === 'video' || (preview_type === 'other' && quality != 'Origin')) {
      let proxyInfo = await Db.getValueObject('ProxyInfo') as any
      if (proxyInfo && proxyInfo.encType && proxyInfo.drive_id === drive_id && proxyInfo.file_id === file_id) {
        // 加密视频通过下载链接播放
      } else {
        let previewData = await AliFile.ApiVideoPreviewUrl(user_id, drive_id, file_id, promotionSkuCode)
        if (typeof previewData != 'string') {
          Object.assign(data, previewData)
          if (quality && quality != 'Origin') {
            const selectedQuality = data.qualities.find((q: any) => q.quality === quality) || data.qualities[0]
            data.url = selectedQuality?.url || ''
            if (selectedQuality?.headers) data.headers = selectedQuality.headers
            if (selectedQuality?.type) data.type = selectedQuality.type
          } else if (data.qualities.length > 0 && !data.url) {
            data.url = data.qualities[0].url
            if (data.qualities[0].headers) data.headers = data.qualities[0].headers
            if (data.qualities[0].type) data.type = data.qualities[0].type
          }
        }
      }
    } else if (preview_type === 'audio') {
      // Retained providers use original download / stream URLs (no Aliyun audio transcoding).
    }
  }
  // 违规文件无法获取地址
  const blockOriginFallback = (data as any).no_origin === true
  const needOriginQuality = !encType && preview_type === 'video' && !blockOriginFallback && !data.qualities.some((q: any) => q.quality === 'Origin')
  if ((!weifa && !data.url) || (uiVideoPlayer == 'web' && !blockOriginFallback) || needOriginQuality) {
    let downUrl = await AliFile.ApiFileDownloadUrl(user_id, drive_id, file_id, 14400)
    if (typeof downUrl != 'string') {
      if (getUrlFileName(downUrl.url).includes('wma')) {
        return '不支持预览的加密音频格式'
      }
      // PikPak 的临时直链必须走本地 Range 代理（长视频没有转码流时只能播原画，直连 CDN 会失败）
      const forceProxy = resolveDriveProvider({ userId: user_id, driveId: drive_id }) === 'pikpak' ? true : undefined
      if (!encType && preview_type && !blockOriginFallback && !data.qualities.some((q: any) => q.quality === 'Origin')) {
        data.qualities.unshift({ quality: 'Origin', html: '原画', label: '原画', value: '', url: downUrl.url, type: detectProxyVideoType(downUrl.url), forceProxy })
      }
      if (!data.url || quality === 'Origin' || (uiVideoQuality === 'Origin' && !blockOriginFallback)) {
        data.url = downUrl.url
      }
      data.size = downUrl.size
      if (downUrl.headers) data.headers = downUrl.headers
    } else {
      return downUrl as string
    }
  }
  if (preview_type == 'other') {
    return data
  }  else if (encType && securityPreviewAutoDecrypt) {
    // 代理播放
    data.url = getProxyUrl({
      user_id, drive_id, file_id, encType, password,
      file_size: data.size, quality: quality || uiVideoQuality,
      proxy_url: data.url
    })
    data.qualities.unshift({ quality: 'Origin', html: '原画', label: '原画', value: '', url: data.url })
  }
  return data
}

export function getUrlFileName(url: string) {
  let fileNameMatch = decodeURIComponent(url).match(/filename\*?=[^=;]*;?''([^&]+)/)
  if (fileNameMatch && fileNameMatch[1]) {
    return fileNameMatch[1]
  }
  return ''
}

export async function createProxyServer(port: number) {
  const url = require('url')
  const proxyServer: Server = http.createServer(async (clientReq: IncomingMessage, clientRes: ServerResponse) => {
    const { pathname, query } = url.parse(clientReq.url, true)
    const { user_id, drive_id, file_id, file_size, encType, password, weifa, quality, proxy_url, proxy_headers, proxy_kind, content_disposition, file_name } = query
    if (pathname === '/proxy') {
      const driveId = String(drive_id || '')
      const fileId = String(file_id || '')
      let { uiVideoQuality, securityEncType, securityFileNameAutoDecrypt } = useSettingStore()
      const selectQuality = quality || uiVideoQuality
      const refreshQuality = content_disposition === 'inline' ? 'Origin' : selectQuality

      // 解析上游地址；forceRefresh 时（链接过期/401/403）绕过缓存重新获取，对齐 rclone 的链接过期重取
      const resolveUpstream = async (forceRefresh: boolean): Promise<{ proxyUrl: string; resolvedProxyHeaders: string }> => {
        const proxyInfo: any = await Db.getValueObject('ProxyInfo')
        let proxyUrl = String(proxy_url || (proxyInfo && proxyInfo.proxy_url || '') || '')
        const isMatchingCache = proxyInfo && driveId === String(proxyInfo.drive_id || '') && fileId === String(proxyInfo.file_id || '')
        let resolvedProxyHeaders = String(proxy_headers || (isMatchingCache ? proxyInfo.proxy_headers : '') || '')
        if (forceRefresh || shouldRefreshProxyUrl({
          driveId,
          fileId,
          proxyUrl,
          selectQuality: String(selectQuality || ''),
          proxyInfo
        })) {
          const data = await getRawUrl(user_id, drive_id, file_id, encType, '', weifa, 'other', refreshQuality)
          if (typeof data != 'string' && data.url) {
            const subtitleData = data.subtitles.find((sub: any) => sub.language === 'chi') || data.subtitles[0]
            proxyUrl = data.url
            resolvedProxyHeaders = data.headers ? JSON.stringify(data.headers) : ''
            if (proxy_kind !== 'subtitle') {
              // 提前 60 秒判定过期，避免长视频播放到一半链接刚好失效（rclone 也有安全余量）
              const expiresTime = GetExpiresTime(proxyUrl)
              await Db.saveValueObject('ProxyInfo', {
                user_id, drive_id, file_id, file_size, encType,
                videoQuality: selectQuality,
                expires_time: expiresTime > 0 ? expiresTime - 60 * 1000 : 0,
                proxy_url: proxyUrl,
                proxy_headers: resolvedProxyHeaders,
                subtitle_url: subtitleData && subtitleData.url || ''
              } as FileInfo)
            }
          } else if (forceRefresh) {
            proxyUrl = ''
          }
        }
        return { proxyUrl, resolvedProxyHeaders }
      }

      // 是否需要解密
      let decryptTransform: any = null
      if (encType) {
        // 要定位请求文件的位置 bytes=xxx-
        const range = clientReq.headers.range
        const start = range ? parseInt(range.replace('bytes=', '').split('-')[0]) : 0
        const flowEnc = getFlowEnc(user_id, file_size, encType, password)!!
        decryptTransform = flowEnc.decryptTransform()
        if (start) {
          await flowEnc.setPosition(start)
        }
      }

      const maxAttempts = encType ? 1 : 2 // 加密流状态不可重入，不 retry
      for (let attempt = 0; attempt < maxAttempts; attempt++) {
        const { proxyUrl, resolvedProxyHeaders } = await resolveUpstream(attempt > 0)
        if (!proxyUrl) {
          if (!clientRes.headersSent) {
            clientRes.writeHead(404, { 'Content-Type': 'text/plain' })
          }
          clientRes.end()
          await Db.deleteValueObject('ProxyInfo')
          return
        }
        // 转码文件302重定向
        if (attempt === 0 && proxyUrl.includes('.aliyuncs.com') && !resolvedProxyHeaders && !encType) {
          clientRes.writeHead(302, { 'Location': proxyUrl })
          clientRes.end()
          return
        }

        const outcome = await new Promise<'done' | 'retry' | 'failed'>((resolvePromise) => {
          let settled = false
          const finish = (value: 'done' | 'retry' | 'failed') => {
            if (!settled) {
              settled = true
              resolvePromise(value)
            }
          }
          const upstreamHeaders = buildUpstreamProxyHeaders(clientReq.headers, resolvedProxyHeaders)
          const isHttps = proxyUrl.indexOf('https') >= 0
          const httpRequest = isHttps ? https : http
          const agentServer = httpRequest.request(proxyUrl, {
            method: clientReq.method,
            headers: upstreamHeaders,
            agent: isHttps ? httpsAgent : httpAgent
          }, (httpResp: any) => {
            const statusCode = Number(httpResp.statusCode || 502)
            // 上游返回 401/403：多半是签名链接过期，丢弃缓存换新链接重试一次（rclone 同样按过期重取）
            if ((statusCode === 401 || statusCode === 403) && attempt < maxAttempts - 1) {
              httpResp.resume()
              void Db.deleteValueObject('ProxyInfo')
              finish('retry')
              return
            }
            clientRes.statusCode = statusCode
            for (const key in httpResp.headers) {
              clientRes.setHeader(key, httpResp.headers[key])
            }
            if (content_disposition === 'inline') {
              const inlineFileName = String(file_name || getUrlFileName(proxyUrl) || 'preview')
              clientRes.setHeader('content-disposition', `inline; filename*=UTF-8''${encodeURIComponent(inlineFileName)};`)
            }
            if (clientRes.statusCode >= 300 && clientRes.statusCode < 400 && httpResp.headers.location) {
              const redirectUrl = new URL(httpResp.headers.location, proxyUrl).toString()
              clientRes.setHeader('location', getProxyUrl({
                user_id, drive_id, file_id, password, weifa,
                file_size, encType, quality, proxy_url: redirectUrl,
                proxy_headers: resolvedProxyHeaders,
                proxy_kind, content_disposition, file_name
              }))
            } else if (httpResp.headers['content-range'] && httpResp.statusCode === 200) {
              // 文件断点续传下载
              clientRes.statusCode = 206
            }
            // 解密文件名
            if (clientReq.method === 'GET' && clientRes.statusCode === 200 && encType && securityFileNameAutoDecrypt) {
              let fileName = getUrlFileName(proxyUrl)
              if (fileName) {
                let ext = path.extname(fileName)
                let securityPassword = getEncPassword(user_id, encType, password)
                let decName = decodeName(securityPassword, securityEncType, fileName.replace(ext, '')) || ''
                clientRes.setHeader('content-disposition', `attachment; filename*=UTF-8''${encodeURIComponent(decName + ext)};`)
              }
            }
            httpResp.on('end', () => {
              // 响应结束后清除超时定时器，避免误杀 keep-alive 复用连接上的其他请求
              agentServer.setTimeout(0)
              finish('done')
            })
            httpResp.on('error', () => {
              clientRes.end()
              finish(clientRes.headersSent ? 'failed' : 'retry')
            })
            if (decryptTransform) {
              httpResp.pipe(decryptTransform).pipe(clientRes)
            } else {
              httpResp.pipe(clientRes)
            }
          })
          // 上游空闲 60 秒无数据即断开，避免长视频播放被挂死的连接卡死（播放器会用 Range 重新拉取）
          agentServer.setTimeout(60 * 1000, () => agentServer.destroy(new Error('upstream idle timeout')))
          clientReq.pipe(agentServer)
          // 关闭解密流
          agentServer.on('close', () => {
            decryptTransform && decryptTransform.destroy()
          })
          agentServer.on('error', (e: Error) => {
            clientRes.end()
            console.log('proxyServer socket error: ' + e)
            finish(!clientRes.headersSent && attempt < maxAttempts - 1 ? 'retry' : 'failed')
          })
          // 重定向的请求 关闭时 关闭被重定向的请求
          clientRes.on('close', () => {
            agentServer.destroy()
          })
        })
        if (outcome !== 'retry') break
      }
      clientReq.on('error', (e: Error) => {
        console.log('client socket error: ' + e)
      })
    }
  })
  proxyServer.listen(port)
  proxyServer.on('error', (err: any) => {
    if (err.code === 'EADDRINUSE' || err.code === 'EACCES') {
      proxyServer.close()
      proxyServer.removeAllListeners('error')
      DebugLog.mSaveDanger(`端口：${port}已被占用，请前往【高级选项->刷新端口】`)
      message.error(`本地预览服务无法启动：端口 ${port} 已被占用。请关闭占用该端口的程序后重启应用`, 5)
    }
  })
  return proxyServer
}
