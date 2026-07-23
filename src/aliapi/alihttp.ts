import { AxiosResponse } from 'axios'
import axios from '../axios'
import jschardet from 'jschardet'
import DebugLog from '../utils/debuglog'

export interface IUrlRespData {
  code: number
  header: string
  body: any
}

function BlobToString(body: Blob, encoding: string): Promise<string> {
  return new Promise((resolve) => {
    const reader = new FileReader()
    reader.readAsText(body, encoding)
    reader.onload = function() {
      resolve((reader.result as string) || '')
    }
  })
}

function BlobToBuff(body: Blob): Promise<ArrayBuffer | undefined> {
  return new Promise((resolve) => {
    const reader = new FileReader()
    reader.readAsArrayBuffer(body)
    reader.onload = function() {
      resolve(reader.result as ArrayBuffer)
    }
  })
}

function Sleep(msTime: number): Promise<{ success: true; time: number }> {
  return new Promise((resolve) =>
    setTimeout(
      () =>
        resolve({
          success: true,
          time: msTime
        }),
      msTime
    )
  )
}

function CatchError(error: any): IUrlRespData {
  const errorMessage = error.display_message || error.message || ''
  if (error.response) {
    DebugLog.mSaveWarning('HttpError status=' + error.response.status + ' message=' + errorMessage)
    return {
      code: error.response.status,
      header: JSON.stringify(error.response.headers || {}),
      body: error.response.data
    } as IUrlRespData
  }
  DebugLog.mSaveWarning('HttpError message=' + errorMessage)
  return { code: 600, header: '', body: 'NetError ' + errorMessage } as IUrlRespData
}

export default class AliHttp {
  static IsSuccess(code: number): Boolean {
    return code >= 200 && code <= 300
  }

  static HttpCodeBreak(code: number): Boolean {
    if (code >= 200 && code <= 300) return true
    if (code == 400) return true
    if (code > 402 && code <= 428) return true
    if (code == 409) return true
    return false
  }

  static async GetString(url: string, _user_id: string, fileSize: number, maxSize: number): Promise<IUrlRespData> {
    for (let i = 0; i <= 5; i++) {
      const resp = await AliHttp._GetString(url, fileSize, maxSize)
      if (AliHttp.HttpCodeBreak(resp.code)) return resp
      else if (i == 5) return resp
      else await Sleep(2000)
    }
    return { code: 609, header: '', body: 'NetError GetStringLost' }
  }

  private static _GetString(url: string, fileSize: number, maxSize: number): Promise<IUrlRespData> {
    const headers: any = {}
    const rangeSize = Math.min(fileSize, maxSize)
    if (rangeSize > 0) headers.Range = `bytes=0-${rangeSize - 1}`

    return axios
      .get(url, {
        withCredentials: false,
        responseType: 'blob',
        timeout: 30000,
        headers
      })
      .then((response: AxiosResponse) => {
        const data = response.data as Blob
        if (data.size == 0) {
          response.data = '文件是空的'
          return response
        }
        const test = data.slice(0, data.size > 10240 ? 10240 : data.size - 1)
        return BlobToBuff(test).then((abuff: ArrayBuffer | undefined) => {
          let encoding = 'utf-8'
          if (abuff && abuff.byteLength > 3) {
            const buff = Buffer.from(abuff)
            if (buff[0].toString(16).toLowerCase() == 'ef' && buff[1].toString(16).toLowerCase() == 'bb' && buff[2].toString(16).toLowerCase() == 'bf') {
              encoding = 'utf-8'
            } else if (buff[0] == 239 && buff[1] == 191 && buff[2] == 189) {
              encoding = 'GB2312'
            } else {
              try {
                const info = jschardet.detect(buff)
                encoding = info.encoding
                if (encoding == 'ascii') encoding = 'utf-8'
              } catch {
                encoding = 'utf-8'
              }
            }
          }
          return BlobToString(data, encoding).then((str) => {
            response.data = str
            return response
          })
        })
      })
      .then((response: AxiosResponse) => {
        const resp: IUrlRespData = {
          code: response.status,
          header: JSON.stringify(response.headers),
          body: response.data
        }

        if (typeof resp.body === 'string' && resp.body.length > 5) {
          const sub = resp.body.substring(0, Math.min(200, resp.body.length))
          if (sub.indexOf('{') >= 0 && sub.indexOf(':') > 0 && sub.indexOf('}') > 0 && sub.indexOf('"') > 0) {
            try {
              resp.body = JSON.stringify(JSON.parse(resp.body), undefined, 2)
            } catch {
            }
          }
        }
        return resp
      })
      .catch(function(err: any) {
        return CatchError(err)
      })
  }
}
