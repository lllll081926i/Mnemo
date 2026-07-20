import { existsSync } from 'fs'
import message from './message'
import is from 'electron-is'
import { spawn, SpawnOptions } from 'child_process'
import mpvAPI from '../module/node-mpv'
import AliFile from '../aliapi/file'
import AliFileCmd from '../aliapi/filecmd'
import AliDirFileList from '../aliapi/dirfilelist'
import { ITokenInfo, usePanFileStore, useSettingStore } from '../store'
import { createTmpFile, delTmpFile, GetExpiresTime } from './utils'
import { IAliGetFileModel } from '../aliapi/alimodels'
import { getEncType, getProxyUrl } from './proxyhelper'
import { CleanStringForCmd } from './filehelper'
import Db from './db'
import { humanTime } from './format'
import { isAliyunUser, isGuangyaUser, isPikPakUser, isQuarkUser } from '../aliapi/utils'
import { apiPikPakFileList, mapPikPakFileToAliModel } from '../pikpak/dirfilelist'
import { apiQuarkFileList, mapQuarkFileToAliModel } from '../quark/dirfilelist'
import { apiGuangyaFileList, mapGuangyaFileToAliModel } from '../guangya/dirfilelist'
import { getWebDavConnection, getWebDavConnectionId, isWebDavDrive, listWebDavDirectory } from './webdavClient'
import { buildDirectPlayerInvocation, isMpvCommand, redactMpvArgs } from './mpvPlayerPolicy'
import { findBestSubtitleMatch } from './subtitleMatching'

const canUseAliyunFileList = (userId: string) => isAliyunUser(userId)
const currentPlayerPlatform = () => (is.windows() ? 'win32' : is.macOS() ? 'darwin' : is.linux() ? 'linux' : process.platform)

const PlayerUtils = {
  filterSubtitleFile(name: string, subTitlesList: IAliGetFileModel[]) {
    return findBestSubtitleMatch(name, subTitlesList)
  },

  async getPlayCursor(user_id: string, drive_id: string, file_id: string) {
    if (isWebDavDrive(drive_id)) return undefined
    if (isPikPakUser(user_id) || drive_id === 'pikpak') return undefined
    if (isQuarkUser(user_id) || drive_id === 'quark') return undefined
    if (isGuangyaUser(user_id) || drive_id === 'guangya') return undefined
    // 获取文件信息
    const info = await AliFile.ApiFileInfo(user_id, drive_id, file_id)
    if (info && typeof info == 'string') {
      message.error('在线预览失败 获取文件信息出错：' + info)
      return undefined
    }
    let play_duration: number = 0
    if (info?.video_media_metadata) {
      play_duration = info?.video_media_metadata.duration
    } else if (info?.user_meta) {
      play_duration = info?.user_meta.duration
    }
    let play_cursor: number = 0
    if (info?.play_cursor) {
      play_cursor = info?.play_cursor
    } else if (info?.user_meta) {
      const meta = JSON.parse(info?.user_meta)
      if (meta.play_cursor) {
        play_cursor = parseFloat(meta.play_cursor)
      }
    }
    // 防止意外跳转
    if (play_duration > 0 && play_duration > 0 && play_cursor >= play_duration - 10) {
      play_cursor = play_duration - 10
    }
    return { info, play_duration, play_cursor }
  },
  async getDirFileList(user_id: string, drive_id: string, parent_file_id: string) {
    let items: IAliGetFileModel[] = []
    if (isWebDavDrive(drive_id)) {
      const connection = getWebDavConnection(getWebDavConnectionId(drive_id))
      if (connection) {
        items = await listWebDavDirectory(connection, parent_file_id || '/')
      }
    } else if (isPikPakUser(user_id) || drive_id === 'pikpak') {
      const parentId = parent_file_id && !parent_file_id.includes('root') ? parent_file_id : 'pikpak_root'
      const list = await apiPikPakFileList(user_id, parentId, 500)
      items = list.items.map(item => mapPikPakFileToAliModel(item, drive_id, parentId))
    } else if (isQuarkUser(user_id) || drive_id === 'quark') {
      const parentId = parent_file_id && !parent_file_id.includes('root') ? parent_file_id : '0'
      const list = await apiQuarkFileList(user_id, parentId, 500)
      items = list.items.map(item => mapQuarkFileToAliModel(item, drive_id, parentId))
    } else if (isGuangyaUser(user_id) || drive_id === 'guangya') {
      const parentId = parent_file_id && !parent_file_id.includes('root') ? parent_file_id : 'guangya_root'
      const list = await apiGuangyaFileList(user_id, parentId, 500)
      items = list.map(item => mapGuangyaFileToAliModel(item, drive_id, parentId))
    } else if (canUseAliyunFileList(user_id)) {
      const dir = await AliDirFileList.ApiDirFileList(user_id, drive_id, parent_file_id, '', 'name asc', '')
      items = dir.items
    } else {
      console.warn('[PlayerUtils] skip Aliyun file list for non-Aliyun source', {
        user_id,
        drive_id,
        parent_file_id
      })
    }
    const curDirFileList: IAliGetFileModel[] = []
    for (let item of items) {
      if (item.isDir) continue
      curDirFileList.push(item)
    }
    return curDirFileList.sort((a, b) => a.name.localeCompare(b.name, 'zh-CN'))
  },
  async createPlayListFile(
    user_id: string, file_id: string,
    quality: string, fileExt: string,
    fileList: IAliGetFileModel[], play_referer: string) {
    let contentStr = ''
    if (fileExt.includes('m3u8')) {
      let header = `#EXTM3U\r\n#EXT-X-ALLOW-CACHE:NO\r\n#EXTVLCOPT:http-referrer=${play_referer}\r\n#EXTVLCOPT:http-user-agent=custom-user-agent\r\n`
      let end = '#EXT-X-ENDLIST\r\n'
      let list = ''
      for (let item of fileList) {
        const encType = getEncType(item)
        const url = getProxyUrl({
          user_id: user_id,
          drive_id: item.drive_id,
          file_id: item.file_id,
          file_size: item.size,
          weifa: item.icon === 'weifa' ? 1 : 0,
          encType: encType,
          quality: quality
        })
        list += '#EXTINF:0,' + item.name + '\r\n' + url + '\r\n'
      }
      contentStr = header + list + end
    }
    if (fileExt == 'dpl') {
      let header = 'DAUMPLAYLIST'
      let playname = ''
      let topindex = 'topindex=0'
      let saveplaypos = `saveplaypos=0`
      let list = ''
      for (let index = 0; index < fileList.length; index++) {
        const item = fileList[index]
        const start = index + 1
        const encType = getEncType(item)
        const url = getProxyUrl({
          user_id: user_id,
          drive_id: item.drive_id,
          file_id: item.file_id,
          file_size: item.size,
          weifa: item.icon === 'weifa' ? 1 : 0,
          encType: encType,
          quality: quality
        })
        let title = CleanStringForCmd(item.name.trim())
        let listStr = `${start}*file*${url}\r\n${start}*title*${title}\r\n${start}*played*0\r\n`
        if (item.file_id === file_id) {
          playname = 'playname=' + url
        }
        list += listStr
      }
      contentStr = `${header}\r\n${playname}\r\n${topindex}\r\n${saveplaypos}\r\n${list}`
    }
    return createTmpFile(contentStr, 'play_list' + '.' + fileExt)
  },
  async mpvPlayer(token: ITokenInfo, binary: string, playArgs: any, otherArgs: any, options: SpawnOptions, exitCallBack: any) {
    let { file, fileList, playList } = otherArgs
    let {
      uiAutoColorVideo,
      uiVideoPlayerHistory,
      uiVideoPlayerExit,
      uiVideoSubtitleMode
    } = useSettingStore()
    let currentTime = 0
    let currentPlayIndex = otherArgs.currentPlayIndex
    let currentFileInfo = file
    let lastUpdateRecordTime = Date.now()
    let mpv: mpvAPI = new mpvAPI(
      {
        debug: false,
        verbose: false,
        binary: binary,
        auto_restart: true,
        spawnOptions: options
      },
      playArgs
    )
    try {
      await mpv.start()
      mpv.on('status', async (status: { property: string; value: any }) => {
        // console.error('status', status)
        if (status.property == 'duration' && status.value > 0) {
          if (uiVideoPlayerHistory && playList.length > 0) {
            let proxyInfo: any = await Db.getValueObject('ProxyInfo')
            if (proxyInfo && proxyInfo.play_cursor) {
              await mpv.seek(proxyInfo.play_cursor, 'absolute')
            } else {
              let playCursorInfo = await this.getPlayCursor(token.user_id, currentFileInfo.drive_id, currentFileInfo.file_id)
              if (playCursorInfo && playCursorInfo.play_cursor > 0) {
                await mpv.seek(playCursorInfo.play_cursor, 'absolute')
              }
            }
          }
        } else if (status.property == 'filename' && status.value) {
          // 自动加载字幕
          if (uiVideoSubtitleMode === 'auto') {
            if (otherArgs.subTitleFile && otherArgs.subTitleFile.subTitleUrl) {
              let { subTitleUrl, subTitleName } = otherArgs.subTitleFile
              await mpv.addSubtitles(subTitleUrl, 'select', subTitleName)
            } else {
              let subTitlesList = fileList.filter((file: any) => /srt|vtt|ass/.test(file.ext))
              if (subTitlesList.length > 0) {
                let subTitleFile = this.filterSubtitleFile(currentFileInfo.name, subTitlesList)
                if (subTitleFile) {
                  const data = await AliFile.ApiFileDownloadUrl(token.user_id, subTitleFile.drive_id, subTitleFile.file_id, 14400)
                  if (typeof data !== 'string' && data.url && data.url != '') {
                    const subtitleUrl = getProxyUrl({
                      user_id: token.user_id,
                      drive_id: subTitleFile.drive_id,
                      file_id: subTitleFile.file_id,
                      proxy_kind: 'subtitle',
                      proxy_url: data.url,
                      proxy_headers: data.headers ? JSON.stringify(data.headers) : undefined
                    })
                    await mpv.addSubtitles(subtitleUrl, 'select', subTitleFile.name || currentFileInfo.name)
                  }
                }
              }
            }
          }
        } else if (playList.length && status.property === 'playlist-pos' && status.value != -1) {
          // 保存历史
          if (uiVideoPlayerHistory && currentPlayIndex != status.value) {
            AliFile.ApiUpdateVideoTime(token.user_id, currentFileInfo.drive_id, currentFileInfo.file_id, currentTime)
          }
          if (otherArgs.subTitleFile) {
            delete otherArgs.subTitleFile
          }
          currentPlayIndex = status.value
          currentFileInfo = playList[status.value]
          // 自动标记
          const { drive_id, file_id, description } = currentFileInfo
          if (uiAutoColorVideo && !isPikPakUser(token) && !isQuarkUser(token) && !isGuangyaUser(token) && drive_id !== 'pikpak' && drive_id !== 'quark' && drive_id !== 'guangya' && (!description || !description.includes('ce74c3c'))) {
            AliFileCmd.ApiFileColorBatch(token.user_id, drive_id, description, 'ce74c3c', [file_id])
          }
        }
      })
      mpv.on('seek', async (timePosition) => {
        // console.log('seek', timePosition)
        if (!await mpv.isPaused()) return
        if (currentFileInfo.file_id && uiVideoPlayerHistory) {
          let recordTime = Date.now()
          if (recordTime - lastUpdateRecordTime > 2000) {
            lastUpdateRecordTime = recordTime
            AliFile.ApiUpdateVideoTime(token.user_id, currentFileInfo.drive_id, currentFileInfo.file_id, timePosition.end)
          }
        }
      })
      mpv.on('timeposition', (timeposition: number) => {
        // console.log('timeposition', currentTime)
        currentTime = timeposition
      })
      mpv.on('quit', async () => {
        await AliFile.ApiUpdateVideoTime(token.user_id, currentFileInfo.drive_id, currentFileInfo.file_id, currentTime)
        exitCallBack()
      })
      if (uiVideoPlayerExit) {
        mpv.on('stopped', async () => {
          message.info('播放完毕，自动退出软件', 8)
          await AliFile.ApiUpdateVideoTime(token.user_id, currentFileInfo.drive_id, currentFileInfo.file_id, currentTime)
          await mpv.quit()
        })
      }
    } catch (error: any) {
      console.error(error)
      if (error.errcode == 6) {
        message.error('播放失败，重复运行MPV播放器', 8)
      } else {
        message.error(`播放失败，${error.verbose}`)
      }
      exitCallBack()
    }
  },

  commandSpawn(commandStr: string, playArgs: any, options: SpawnOptions, exitCallBack: any) {
    const childProcess: any = spawn(commandStr, playArgs, {
      ...options,
      shell: false
    })
    childProcess.unref()
    if (exitCallBack) {
      childProcess.once('exit', async () => {
        exitCallBack()
      })
    }
  },

  async startPlayer(token: ITokenInfo, command: string, otherArgs: any) {
    if (!command) {
      message.error('请先在设置中选择或填写自定义播放器')
      return
    }
    if ((is.windows() || is.macOS()) && !existsSync(command)) {
      message.error(`启动失败，找不到文件, ${command}`)
      return
    }
    const isMPV = isMpvCommand(command)
    const isPotplayer = command.toLowerCase().includes('potplayer')
    const directMpvControl = (useSettingStore().uiVideoEnablePlayerList || useSettingStore().uiVideoPlayerHistory) && isMPV
    const argsToStr = (args: string) => String(args)
    const directInvocation = buildDirectPlayerInvocation(currentPlayerPlatform(), command)
    // 构造播放参数
    let { file, subTitleFile, rawData, quality } = otherArgs
    let encType = getEncType(file)
    let play_url = ''
    let play_referer = token.open_api_access_token ? 'https://openapi.alipan.com/' : 'https://www.aliyundrive.com/'
    let {
      uiVideoEnablePlayerList,
      uiVideoPlayerExit,
      uiVideoPlayerHistory,
      uiVideoPlayerParams
    } = useSettingStore()
    let playerArgs: any = []
    let options: SpawnOptions = { detached: !uiVideoPlayerExit }
    let subTitleUrl = ''
    let rawHeaders: Record<string, string> = {}
    const headerFields = () => Object.entries(rawHeaders)
      .filter(([key, value]) => !!key && !!value)
      .map(([key, value]) => `${key}: ${value}`)
    if (rawData) {
      // 加载转码的内嵌字幕
      if (rawData.subtitles && quality != 'Origin') {
        let subTitleData = rawData.subtitles.find((sub: any) => sub.language === 'chi') || rawData.subtitles[0]
        if (subTitleData?.url) {
          const subtitleHeaders = subTitleData.headers || rawData.headers
          subTitleUrl = getProxyUrl({
            user_id: token.user_id,
            drive_id: file.drive_id,
            file_id: file.file_id,
            proxy_kind: 'subtitle',
            proxy_url: subTitleData.url,
            proxy_headers: subtitleHeaders ? JSON.stringify(subtitleHeaders) : undefined
          })
        }
      }
      if (rawData.qualities) {
        const selectedQuality = rawData.qualities.find((q: any) => q.quality === quality) || rawData.qualities[0]
        play_url = selectedQuality?.url || ''
        rawHeaders = selectedQuality?.headers || rawData.headers || {}
      }
    }
    // 优先加载网盘内字幕文件
    if (subTitleFile && !subTitleFile.isDir) {
      const data = await AliFile.ApiFileDownloadUrl(token.user_id, subTitleFile.drive_id, subTitleFile.file_id, 14400)
      if (typeof data !== 'string' && data.url && data.url != '') {
        subTitleUrl = getProxyUrl({
          user_id: token.user_id,
          drive_id: subTitleFile.drive_id,
          file_id: subTitleFile.file_id,
          proxy_kind: 'subtitle',
          proxy_url: data.url,
          proxy_headers: data.headers ? JSON.stringify(data.headers) : undefined
        })
      }
    }
    // 获取播放进度
    let play_cursor = 0
    if (uiVideoPlayerHistory) {
      let playCursorInfo = await PlayerUtils.getPlayCursor(token.user_id, file.drive_id, file.file_id)
      if (playCursorInfo) {
        play_cursor = playCursorInfo.play_cursor
      } else {
        play_cursor = file.media_play_cursor ? parseInt(file.media_play_cursor) : 0
      }
    }
    otherArgs.subTitleFile = { subTitleUrl: subTitleUrl, subTitleName: subTitleFile?.name || 'chi' }
    if (isPotplayer) {
      playerArgs = [
        '/new',
        '/autoplay',
        `/referer=${argsToStr(play_referer)}`,
        `/title=${argsToStr(file.name)}`
      ]
      if (!uiVideoEnablePlayerList) {
        if (play_cursor > 0 && uiVideoPlayerHistory) {
          playerArgs.push(`/seek=${argsToStr(humanTime(play_cursor))}`)
        }
        if (subTitleUrl.length > 0) {
          playerArgs.push(`/sub=${argsToStr(subTitleUrl)}`)
        }
      }
      if (rawHeaders['User-Agent']) playerArgs.push(`/user_agent=${argsToStr(rawHeaders['User-Agent'])}`)
      const potHeaders = headerFields().filter(header => !/^user-agent\s*:/i.test(header))
      if (potHeaders.length) playerArgs.push(`/headers=${argsToStr(potHeaders.join('\\r\\n'))}`)
    } else if (isMPV) {
      playerArgs = [
        '--force-window=immediate',
        '--hwdec=auto',
        '--geometry=50%',
        '--autofit-larger=100%x100%',
        '--autofit-smaller=640',
        '--audio-pitch-correction=yes',
        '--keep-open-pause=no',
        '--alang=[en,eng,zh,chi,chs,sc,zho]',
        '--slang=[zh,chi,chs,sc,zho,en,eng]',
        `--force-media-title=${argsToStr(file.name)}`,
        `--referrer=${argsToStr(play_referer)}`,
        `--title=${argsToStr(file.name)}`
      ]
      if (!uiVideoEnablePlayerList) {
        if (play_cursor > 0 && uiVideoPlayerHistory) {
          playerArgs.push(`--start=${play_cursor}`)
        }
        if (subTitleUrl.length > 0) {
          playerArgs.push(`--sub-file=${argsToStr(subTitleUrl)}`)
        }
      }
      const mpvHeaders = headerFields().filter(header => !/^user-agent\s*:/i.test(header))
      if (rawHeaders['User-Agent']) playerArgs.push(`--user-agent=${argsToStr(rawHeaders['User-Agent'])}`)
      if (mpvHeaders.length) playerArgs.push(`--http-header-fields=${argsToStr(mpvHeaders.join(','))}`)
    }
    if (uiVideoPlayerParams.length > 0) {
      const params = uiVideoPlayerParams.replaceAll(/\s+/g, '').split(',')
      playerArgs.push(...params)
    }
    const playArgs: any[] = [...Array.from(new Set(playerArgs))]
    let fileList: IAliGetFileModel[] = []
    if (uiVideoEnablePlayerList) {
      if (file.compilation_id) {
        fileList = await this.getDirFileList(token.user_id, file.drive_id, file.parent_file_id)
      } else {
        fileList = usePanFileStore().ListDataRaw
      }
      otherArgs.fileList = fileList
      // console.log('getDirFileList', fileList)
      otherArgs.playList = fileList.filter((v: any) => v.category.includes('video'))
      otherArgs.playFileListPath = await this.createPlayListFile(
        token.user_id, file.file_id,
        quality, isPotplayer ? 'dpl' : 'm3u8',
        otherArgs.playList, play_referer
      )
      // console.log('tmpFile', tmpFile)
      if (isMPV) {
        otherArgs.currentPlayIndex = otherArgs.playList.findIndex((v: any) => v.file_id == file.file_id) || 0
        playArgs.unshift('--playlist-start=' + otherArgs.currentPlayIndex)
        playArgs.unshift('--playlist=' + otherArgs.playFileListPath)
      } else {
        playArgs.unshift(otherArgs.playFileListPath)
      }
    } else {
      playArgs.unshift(argsToStr(play_url))
    }
    console.warn('playArgs', redactMpvArgs(playArgs))
    const exitCallBack = () => {
      if (uiVideoEnablePlayerList) {
        delTmpFile(otherArgs.playFileListPath)
      }
      otherArgs = {}
    }
    if (!encType && play_url) {
      let info: any = {
        user_id: token.user_id,
        drive_id: file.drive_id,
        file_id: file.file_id,
        file_size: file.size,
        encType: encType,
        videoQuality: quality,
        expires_time: GetExpiresTime(play_url),
        proxy_url: play_url,
        proxy_headers: Object.keys(rawHeaders).length ? JSON.stringify(rawHeaders) : undefined,
        play_cursor: play_cursor
      }
      await Db.saveValueObject('ProxyInfo', info)
    }
    if (directMpvControl) {
      await this.mpvPlayer(token, directInvocation.binary, [...directInvocation.args, ...playArgs], otherArgs, options, exitCallBack)
    } else {
      this.commandSpawn(directInvocation.binary, [...directInvocation.args, ...playArgs], options, exitCallBack)
    }
  }
}
export default PlayerUtils
