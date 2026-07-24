import { IAliGetFileModel } from '../aliapi/alimodels'
import AliFile from '../aliapi/file'
import AliFileCmd from '../aliapi/filecmd'
import { ITokenInfo, useAppStore, useFootStore, usePanFileStore, useSettingStore, useUserStore } from '../store'
import { IPageCode, IPageImage, IPageMusic, IPageMusicTrack, IPagePdf, IPageVideo, IPageVideoPlaylistEntry } from '../store/appstore'
import UserDAL from '../user/userdal'
import { clickWait } from './debounce'
import DebugLog from './debuglog'
import message from './message'
import { modalSelectPanDir, modalSelectVideoQuality } from './modal'
import PlayerUtils from './playerhelper'
import { getEncType, getProxyUrl, getRawUrl, isLocalProxyUrl } from './proxyhelper'
import { resolveFileExt } from './filetype'
import { isPikPakUser } from '../aliapi/utils'
import { isDriveProviderRootId, isDriveProviderSessionUsable, resolveDriveProvider } from './driveProvider'

async function resolveTokenForFile(file: IAliGetFileModel): Promise<ITokenInfo | undefined> {
  const explicitUserId = (file as any).user_id as string | undefined
  if (explicitUserId) {
    const token = await UserDAL.GetUserTokenFromDB(explicitUserId)
    if (isDriveProviderSessionUsable(token, { driveId: file.drive_id })) return token
  }
  const currentUserId = useUserStore().user_id
  const currentToken = await UserDAL.GetUserTokenFromDB(currentUserId)
  const driveId = file.drive_id || ''
  const driveProvider = resolveDriveProvider({ driveId, tokenfrom: currentToken?.tokenfrom, userId: currentToken?.user_id })
  const matchesCurrent = currentToken && (
    (driveId === 'pikpak' && isPikPakUser(currentToken))
    || (driveProvider !== 'unknown' && currentToken.tokenfrom === driveProvider)
    || (driveId.startsWith(`${currentToken.tokenfrom}:`))
  )
  if (matchesCurrent && isDriveProviderSessionUsable(currentToken, { driveId })) return currentToken

  const userList = await UserDAL.GetUserListFromDB()
  const matched = userList.find((token) => {
    if (driveId === 'pikpak' && isPikPakUser(token)) return true
    const provider = resolveDriveProvider({ driveId, tokenfrom: token.tokenfrom, userId: token.user_id })
    return provider !== 'unknown' && (token.tokenfrom === provider || driveId.startsWith(`${token.tokenfrom}:`))
  })
  if (!matched?.user_id) return currentToken || undefined
  return await UserDAL.GetUserTokenFromDB(matched.user_id) || undefined
}

const videoPlaylistExtensions = new Set(['3gp', 'avi', 'flv', 'm2ts', 'm4v', 'mkv', 'mov', 'mp4', 'mpeg', 'mpg', 'ts', 'webm', 'wmv'])

function buildSiblingVideoPlaylist(file: IAliGetFileModel, provided?: IPageVideoPlaylistEntry[]): IPageVideoPlaylistEntry[] {
  if (provided && provided.length > 0) return provided
  const rawItems = usePanFileStore().ListDataRaw || []
  const visibleItems = rawItems.filter((item: any) => !item.isDir)
  const sameDirectory = visibleItems.filter((item: any) => item.parent_file_id === file.parent_file_id)
  const candidates = (sameDirectory.length > 1 ? sameDirectory : visibleItems).filter((item: any) => {
    const name = String(item.name || item.file_name || item.html || '')
    const extension = resolveFileExt(name, item.ext || item.mime_extension || '', item.mime_type || '')
    return videoPlaylistExtensions.has(extension)
  })
  if (!candidates.some((item: any) => item.file_id === file.file_id)) candidates.push(file)
  if (candidates.length <= 1) return []
  return candidates
    .sort((left: any, right: any) => String(left.name || left.file_name || '').localeCompare(String(right.name || right.file_name || ''), undefined, { numeric: true, sensitivity: 'base' }))
    .map((item: any): IPageVideoPlaylistEntry => ({
      user_id: item.user_id || (file as any).user_id || '',
      drive_id: item.drive_id || file.drive_id,
      file_id: item.file_id,
      parent_file_id: item.parent_file_id || file.parent_file_id,
      file_name: item.name || item.file_name || item.html,
      html: item.name || item.file_name || item.html,
      ext: item.ext,
      description: item.description,
      play_cursor: item.play_cursor,
      password: item.file_id === file.file_id ? undefined : ''
    }))
}

const TEXT_PREVIEW_EXTS = new Set(['txt', 'text', 'log', 'csv', 'tsv', 'nfo', 'srt', 'vtt', 'ass', 'ssa'])
const PDF_PREVIEW_DRIVES = new Set(['pikpak', 'onedrive', 'dropbox', 'gdrive', 'gofile', 'webdav', 's3'])
const IMAGE_PREVIEW_EXTS = new Set(['bmp', 'gif', 'ico', 'jpeg', 'jpg', 'png', 'svg', 'webp'])
const AUDIO_PREVIEW_EXTS = new Set(['aac', 'ape', 'flac', 'm4a', 'mp3', 'ogg', 'wav', 'wma'])
const VIDEO_PREVIEW_EXTS = new Set([...videoPlaylistExtensions, 'asf', 'm3u8', 'mpd', 'mts', 'ogv', 'rmvb', 'vob'])
const CODE_PREVIEW_EXTS = new Set([
  'bash', 'c', 'cc', 'conf', 'cpp', 'cs', 'css', 'csv', 'go', 'h', 'hpp', 'html', 'ini', 'java', 'js', 'json', 'jsx', 'kt',
  'less', 'log', 'lua', 'md', 'markdown', 'php', 'py', 'rb', 'rs', 'sass', 'scss', 'sh', 'sql', 'swift', 'toml', 'ts', 'tsx',
  'txt', 'text', 'vue', 'xml', 'yaml', 'yml'
])

export function isSupportedPreviewExtension(fileExt: string): boolean {
  const ext = (fileExt || '').toLowerCase().replace(/^\./, '').trim()
  if (!ext) return false
  return IMAGE_PREVIEW_EXTS.has(ext)
    || AUDIO_PREVIEW_EXTS.has(ext)
    || VIDEO_PREVIEW_EXTS.has(ext)
    || TEXT_PREVIEW_EXTS.has(ext)
    || ext === 'pdf'
    || (CODE_PREVIEW_EXTS.has(ext) && Boolean(PrismExt(ext)))
}

function TextPreviewExt(fileExt: string): string {
  return TEXT_PREVIEW_EXTS.has((fileExt || '').toLowerCase().replace('.', '').trim()) ? 'plain' : ''
}

export async function menuOpenFile(
  file: IAliGetFileModel,
  password: string = '',
  options?: {
    customPlaylistLabel?: string
    customPlaylist?: IPageVideoPlaylistEntry[]
  }
): Promise<void> {
  if (clickWait('menuOpenFile', 500)) return
  const file_id = file.file_id
  let parent_file_id = file.parent_file_id
  if (isDriveProviderRootId({ driveId: file.drive_id }, parent_file_id)) parent_file_id = 'root'
  const drive_id = file.drive_id
  const fileProvider = resolveDriveProvider({ driveId: file.drive_id })
  const normalizedExt = resolveFileExt(file.name || '', file.ext || file.mime_extension || '', file.mime_type || '')
  if (!isSupportedPreviewExtension(normalizedExt)) {
    message.info('暂不支持打开此格式，请下载后使用其他应用查看')
    return
  }

  const codeExt = PrismExt(normalizedExt) || TextPreviewExt(normalizedExt)
  if (CODE_PREVIEW_EXTS.has(normalizedExt) && codeExt) {
    if (file.size >= 5 * 1024 * 1024) {
      message.info('文件较大，暂不支持在线打开，请下载后查看')
      return
    }
    await Code(file, codeExt, password)
    return
  }

  if (normalizedExt === 'pdf' && PDF_PREVIEW_DRIVES.has(fileProvider)) {
    await Pdf(file, password)
    return
  }
  if (IMAGE_PREVIEW_EXTS.has(normalizedExt)) {
    await Image(file, password)
    return
  }
  if (VIDEO_PREVIEW_EXTS.has(normalizedExt)) {
    const token = await resolveTokenForFile(file)
    if (!isDriveProviderSessionUsable(token, { driveId: file.drive_id })) {
      message.error('无法预览：网盘登录已失效，请重新登录后再试')
      return
    }
    // 选择字幕
    let subTitleFile: any
    const { uiVideoPlayer, uiVideoSubtitleMode } = useSettingStore()
    const useMacEmbeddedMpv = uiVideoPlayer === 'mpv' && window.platform === 'darwin'
    const listDataRaw: IAliGetFileModel[] = usePanFileStore().ListDataRaw || []
    const subTitlesList: IAliGetFileModel[] = listDataRaw.filter((file) => /^(srt|vtt|ass|ssa)$/i.test(resolveFileExt(file.name || '', file.ext || '', file.mime_type || '')))
    if (uiVideoPlayer !== 'web' && !useMacEmbeddedMpv) {
      if (uiVideoSubtitleMode === 'auto') {
        subTitleFile = PlayerUtils.filterSubtitleFile(file.name, subTitlesList)
      } else if (uiVideoSubtitleMode === 'select') {
        modalSelectPanDir(
          'select',
          parent_file_id,
          async (_user_id: string, _drive_id: string, selectFile: any) => {
            await Video(token, file, selectFile, password, options)
          },
          '',
          /srt|vtt|ass/
        )
        return
      }
    }
    await Video(token, file, subTitleFile, password, options)
    return
  }
  if (AUDIO_PREVIEW_EXTS.has(normalizedExt)) {
    await Audio(file, password)
    return
  }
  message.info('暂不支持打开此格式，请下载后使用其他应用查看')
}

async function Video(
  token: ITokenInfo,
  file: IAliGetFileModel,
  subTitleFile: any,
  password: string = '',
  options?: {
    customPlaylistLabel?: string
    customPlaylist?: IPageVideoPlaylistEntry[]
  }
): Promise<void> {
  if (file.icon == 'iconweifa') {
    message.error('无法预览：网盘已限制此文件')
    return
  }
  let desc = file.description
  const {
    uiAutoColorVideo,
    uiVideoQuality,
    uiVideoQualityTips,
    uiVideoEnablePlayerList,
    uiVideoPlayer,
    uiVideoPlayerPath
  } = useSettingStore()
  if (uiVideoPlayer === 'mpv' && window.platform !== 'darwin') {
    useSettingStore().updateStore({ uiVideoPlayer: 'other' })
    message.error('内置 MPV 仅支持 macOS，请在设置中选择“自定义播放软件”并指定播放器路径。')
    return
  }
  if (uiAutoColorVideo && !isPikPakUser(token) && file.drive_id !== 'pikpak' && (!desc || !desc.includes('ce74c3c'))) {
    AliFileCmd.ApiFileColorBatch(token.user_id, file.drive_id, file.description, 'ce74c3c', [file.file_id])
  }
  if (uiVideoPlayer == 'web' || (uiVideoPlayer === 'mpv' && window.platform === 'darwin')) {
    // 尽快开窗：进度/父目录不阻塞首帧（对齐官方客户端先起播放页再补元数据）
    const play_cursor = file.media_play_cursor ? parseInt(file.media_play_cursor) || 0 : 0
    const pageVideo: IPageVideo = {
      user_id: token.user_id,
      file_name: file.name,
      html: file.name,
      drive_id: file.drive_id,
      file_id: file.file_id,
      parent_file_id: file.parent_file_id,
      parent_file_name: '',
      expire_time: 0,
      password: password,
      encType: getEncType(file),
      play_cursor,
      custom_playlist_label: options?.customPlaylistLabel || '',
      custom_playlist: buildSiblingVideoPlaylist(file, options?.customPlaylist)
    }
    window.WebOpenWindow({ page: 'PageVideo', data: pageVideo, theme: 'dark' })
    // 后台补全播放进度，不阻塞开窗
    void PlayerUtils.getPlayCursor(token.user_id, file.drive_id, file.file_id).then((playCursorInfo) => {
      const store = useAppStore()
      if (playCursorInfo?.play_cursor && store.pageVideo?.file_id === file.file_id) {
        store.pageVideo.play_cursor = playCursorInfo.play_cursor
      }
    }).catch(() => undefined)
    return
  }
  message.loading('加载中...', 2)
  const isWindows = window.platform === 'win32'
  const isMacOrLinux = ['darwin', 'linux'].includes(window.platform)
  if (!isWindows && !isMacOrLinux) {
    message.error('当前系统不支持这种播放方式，请在设置中更换播放器')
    return
  }
  if (uiVideoPlayer === 'other' && !uiVideoPlayerPath.trim()) {
    message.error('请先在设置中选择或填写自定义播放器')
    return
  }
  let rawData: any = undefined
  let encType = getEncType(file)
  if (uiVideoQualityTips || !uiVideoEnablePlayerList) {
    rawData = await getRawUrl(token.user_id, file.drive_id, file.file_id, encType, password, file.icon == 'iconweifa', 'video')
    if (typeof rawData == 'string') {
      message.error('无法获取视频地址，请检查网络连接后重试')
      return
    }
    if (rawData.url.indexOf('x-oss-additional-headers=referer') > 0) {
      message.error('网盘登录已过期，请退出当前账号后重新登录')
      return
    }
  }
  let otherArgs: any = {
    file, subTitleFile,
    playList: [],
    fileList: [],
    playFileListPath: '',
    rawData, password,
    quality: uiVideoQuality
  }
  // 清晰度选择
  if (uiVideoQualityTips && !encType) {
    if (rawData) {
      modalSelectVideoQuality(file, rawData, async (quality: any) => {
        otherArgs.quality = quality
        await PlayerUtils.startPlayer(token, uiVideoPlayerPath, otherArgs)
      })
    } else {
      message.error('无法获取视频地址，请检查网络连接后重试')
    }
  } else {
    await PlayerUtils.startPlayer(token, uiVideoPlayerPath, otherArgs)
  }
}

async function Image(file: IAliGetFileModel, password: string = ''): Promise<void> {
  const token = await resolveTokenForFile(file)
  if (!isDriveProviderSessionUsable(token, { driveId: file.drive_id })) {
    message.error('无法预览：网盘登录已失效，请重新登录后再试')
    return
  }
  message.loading('加载中...', 2)
  const fileList = usePanFileStore().ListDataRaw
  const imageList = fileList.filter((v) => IMAGE_PREVIEW_EXTS.has(resolveFileExt(v.name || '', v.ext || '', v.mime_type || '')))
  if (imageList.length == 0) {
    message.error('无法打开文件：没有获取到预览地址，请稍后重试')
    return
  }

  const pageImage: IPageImage = {
    user_id: token.user_id,
    drive_id: file.drive_id,
    file_id: file.file_id,
    file_name: file.name,
    mode: useSettingStore().uiImageMode,
    password: password,
    imageList: imageList
  }
  window.WebOpenWindow({ page: 'PageImage', data: pageImage, theme: 'dark' })
}

async function Pdf(file: IAliGetFileModel, password: string = ''): Promise<void> {
  const token = await resolveTokenForFile(file)
  if (!isDriveProviderSessionUsable(token, { driveId: file.drive_id })) {
    message.error('无法预览：网盘登录已失效，请重新登录后再试')
    return
  }
  message.loading('加载中...', 2)
  const rawData = await getRawUrl(token.user_id, file.drive_id, file.file_id, getEncType(file), password, file.icon == 'iconweifa', 'other', 'Origin')
  if (typeof rawData === 'string' || !rawData.url) {
    message.error(typeof rawData === 'string' ? rawData : '无法打开 PDF：没有获取到预览地址，请稍后重试')
    return
  }
  const pagePdf: IPagePdf = {
    user_id: token.user_id,
    drive_id: file.drive_id,
    file_id: file.file_id,
    file_name: file.name,
    preview_url: getProxyUrl({
      user_id: token.user_id,
      drive_id: file.drive_id,
      file_id: file.file_id,
      file_size: rawData.size || file.size,
      proxy_url: rawData.url,
      proxy_headers: rawData.headers ? JSON.stringify(rawData.headers) : undefined,
      content_disposition: 'inline',
      file_name: file.name
    })
  }
  window.WebOpenWindow({ page: 'PagePdf', data: pagePdf, theme: 'dark' })
}

async function Audio(file: IAliGetFileModel, password: string = ''): Promise<void> {
  const weifa = file.icon == 'iconweifa'
  if (weifa) {
    message.error('无法预览：网盘已限制此文件')
    return
  }

  const token = await resolveTokenForFile(file)
  if (!isDriveProviderSessionUsable(token, { driveId: file.drive_id })) {
    message.error('无法预览：网盘登录已失效，请重新登录后再试')
    return
  }

  const ext = resolveFileExt(file.name || '', file.ext || file.mime_extension || '', file.mime_type || '')
  const listRaw = usePanFileStore().ListDataRaw || []
  const audioList = listRaw.filter((v) => !v.isDir && AUDIO_PREVIEW_EXTS.has(resolveFileExt(v.name || '', v.ext || v.mime_extension || '', v.mime_type || '')))
  const sourceList = audioList.length > 0 ? audioList : [file]

  const playlist: IPageMusicTrack[] = sourceList.map((item) => ({
    user_id: ((item as any).user_id as string) || token.user_id,
    drive_id: item.drive_id,
    file_id: item.file_id,
    parent_file_id: item.parent_file_id,
    file_name: item.name,
    ext: item.ext,
    size: item.size,
    category: item.category,
    icon: item.icon,
    thumbnail: item.thumbnail,
    description: item.description,
    encType: getEncType(item),
    password: item.file_id === file.file_id ? password : ''
  }))

  if (!playlist.find((t) => t.file_id === file.file_id)) {
    playlist.unshift({
      user_id: ((file as any).user_id as string) || token.user_id,
      drive_id: file.drive_id,
      file_id: file.file_id,
      parent_file_id: file.parent_file_id,
      file_name: file.name,
      ext: file.ext,
      size: file.size,
      category: file.category,
      icon: file.icon,
      thumbnail: file.thumbnail,
      description: file.description,
      encType: getEncType(file),
      password
    })
  }

  const pageMusic: IPageMusic = {
    user_id: token.user_id,
    drive_id: file.drive_id,
    file_id: file.file_id,
    parent_file_id: file.parent_file_id,
    parent_file_name: usePanFileStore().DirName || '',
    file_name: file.name,
    encType: getEncType(file),
    password,
    playlist
  }

  if (typeof ext === 'string' && ext.length > 0) {
    // ext intentionally captured for future use; no-op
  }

  if (typeof window !== 'undefined' && (window as any).WebOpenWindow) {
    const appStore = useAppStore()
    const theme = appStore.appTheme === 'system' ? (appStore.appDark ? 'dark' : 'light') : appStore.appTheme
    ;(window as any).WebOpenWindow({ page: 'PageMusic', data: pageMusic, theme })
  } else {
    // 兜底：旧版底部播放器
    const data = await getRawUrl(token.user_id, file.drive_id, file.file_id, getEncType(file), password, weifa, 'audio')
    if (typeof data != 'string') {
      const audioUrl = isLocalProxyUrl(data.url)
        ? data.url
        : getProxyUrl({
            user_id: token.user_id,
            drive_id: file.drive_id,
            file_id: file.file_id,
            file_size: data.size,
            encType: getEncType(file),
            password,
            quality: 'Origin',
            proxy_kind: 'audio',
            proxy_url: data.url,
            proxy_headers: data.headers ? JSON.stringify(data.headers) : undefined
          })
      useFootStore().mSaveAudioUrl(audioUrl)
    } else {
      message.error(data)
    }
  }
}

async function Code(file: IAliGetFileModel, codeExt: string, password: string = ''): Promise<void> {
  const token = await resolveTokenForFile(file)
  if (!isDriveProviderSessionUsable(token, { driveId: file.drive_id })) {
    message.error('无法预览：网盘登录已失效，请重新登录后再试')
    return
  }
  message.loading('加载中...', 2)
  const pageCode: IPageCode = {
    user_id: token.user_id,
    drive_id: file.drive_id,
    file_id: file.file_id,
    file_name: file.name,
    code_ext: codeExt,
    file_size: file.size,
    encType: getEncType(file),
    password: password
  }
  window.WebOpenWindow({ page: 'PageCode', data: pageCode, theme: 'dark' })
}


export function PrismExt(fileExt: string): string {
  const ext = '.' + fileExt.toLowerCase() + '.'
  const fext = fileExt.toLowerCase()
  let iscode = false
  let codeext = ''
  iscode = iscode || ';.markup.html.xml.svg.mathml.ssml.atom.rss.css.clike.javascript.js.abap.'.indexOf(ext) > 0
  iscode = iscode || ';.actionscript.ada.agda.al.antlr4.g4.apacheconf.apex.apl.applescript.abnf.'.indexOf(ext) > 0
  iscode = iscode || ';.aql.arduino.arff.asciidoc.adoc.aspnet.asm6502.autohotkey.autoit.bash.shell.'.indexOf(ext) > 0
  iscode = iscode || ';.basic.batch.bbcode.shortcode.birb.bison.bnfrbnf.brainfuck.brightscript.'.indexOf(ext) > 0
  iscode = iscode || ';.bro.bsl.oscript.c.csharp.cs.dotnet.cpp.cfscript.cfc.chaiscript.cil.clojure.cmake.'.indexOf(ext) > 0
  iscode = iscode || ';.cobol.coffeescript.coffee.concurnas.conc.csp.coq.crystal.css-extras.csv.cypher.n4jsd.'.indexOf(ext) > 0
  iscode = iscode || ';.d.dart.dataweave.dax.dhall.diff.django.jinja2.dns-zone-file.dns-zone..purs.purescript.'.indexOf(ext) > 0
  iscode = iscode || ';.docker.dockerfile.dot.gv.ebnf.editorconfig.eiffel.ejs.eta.elixir.elm.etlua.erb.erlang.'.indexOf(ext) > 0
  iscode = iscode || ';.fsharp.factor.false.firestore-security-rules.flow.fortran.ftl.gml.gamemakerlanguage.'.indexOf(ext) > 0
  iscode = iscode || ';.gcode.gdscript.gedcom.gherkin.git.glsl.go.graphql.groovy.haml.handlebars.hbs.'.indexOf(ext) > 0
  iscode = iscode || ';.haskell.hs.haxe.hcl.hlsl.hoon.http.hpkp.hsts.ichigojam.icon.icu-message-format.'.indexOf(ext) > 0
  iscode = iscode || ';.idris.idr.ignore.gitignore.hgignore.npmignore.inform7.ini.io.j.java.javadoc.javadoclike.'.indexOf(ext) > 0
  iscode = iscode || ';.javastacktrace.jexl.jolie.jq.jsdoc.js-extras.json.webmanifest.json5.jsonp.jsstacktrace.px.'.indexOf(ext) > 0
  iscode = iscode || ';.js-templates.julia.keyman.kotlin.kt.kts.kumir.kum.latex.tex.context.latte.less.lilypond.ly.'.indexOf(ext) > 0
  iscode = iscode || ';.liquid.lisp.emacs.elisp.emacs-lisp.livescript.llvm.log.lolcode.lua.makefile.markdown.md.'.indexOf(ext) > 0
  iscode = iscode || ';.markup-templating.matlab.mel.mizar.mongodb.monkey.moonscript.moon.n1ql.n4js.'.indexOf(ext) > 0
  iscode = iscode || ';.nand2tetris-hdl.naniscript.nani.nasm.neon.nevod.nginx.nim.nix.nsis.objectivec.objc.'.indexOf(ext) > 0
  iscode = iscode || ';.ocaml.opencl.openqasm.qasm.oz.parigp.parser.pascal.objectpascal.pascaligo.psl.pcaxis.'.indexOf(ext) > 0
  iscode = iscode || ';.peoplecode.pcode.perl.php.phpdoc.php-extras.plsql.powerquery.pq.mscript.powershell.'.indexOf(ext) > 0
  iscode = iscode || ';.processing.prolog.promql.properties.protobuf.pug.puppet.pure.purebasic.pbfasm.twig.'.indexOf(ext) > 0
  iscode = iscode || ';.python.py.qsharp.qs.q.qml.qore.r.racket.rkt.jsx.tsx.reason.regex.rego.renpy.rpy.rest.rip.'.indexOf(ext) > 0
  iscode = iscode || ';.robotframework.robot.ruby.rb.rust.sas.sass.scss.scala.scheme.shell-session.sh-session.sql.'.indexOf(ext) > 0
  iscode = iscode || ';.smali.smalltalk.smarty.sml.smlnj.solidity.sol.solution-file.sln.soy.sparql.rq.splunk-spl.sqf.'.indexOf(ext) > 0
  iscode = iscode || ';.squirrel.stan.iecst.stylus.swift.t4-templating.t4-cs.t4.t4-vb.tap.tcl.tt2.textile.toml.turtle.trig.'.indexOf(ext) > 0
  iscode = iscode || ';.typescript.ts.typoscript.tsconfig.unrealscript.uscript.uc.uri.url.v.vala.vbnet.velocity.verilog.'.indexOf(ext) > 0
  iscode = iscode || ';.vim.visual-basic.vb.vba.warpscript.wasm.wiki.wolfram.mathematica.nb.wl.xeora.xeoracube.'.indexOf(ext) > 0
  iscode = iscode || ';.xml-doc.xojo.xquery.yaml.yml.yang.zig.excel-formula.xlsx.xls.shellsession.roboconf.vhdl.'.indexOf(ext) > 0

  if (iscode) {
    codeext = fext
  } else {
    switch (fext) {
      case 'prettierrc':
        codeext = 'json'
        break
      case 'vue':
        codeext = 'javascript'
        break
      case 'h':
        codeext = 'c'
        break
      case 'as':
        codeext = 'actionscript'
        break
      case 'sh':
        codeext = 'bash'
        break
      case 'zsh':
        codeext = 'bash'
        break
      case 'bf':
        codeext = 'brainfuck'
        break
      case 'hpp':
        codeext = 'cpp'
        break
      case 'cc':
        codeext = 'cpp'
        break
      case 'hh':
        codeext = 'cpp'
        break
      case 'c++':
        codeext = 'cpp'
        break
      case 'h++':
        codeext = 'cpp'
        break
      case 'cxx':
        codeext = 'cpp'
        break
      case 'hxx':
        codeext = 'cpp'
        break
      case 'cson':
        codeext = 'coffeescript'
        break
      case 'iced':
        codeext = 'coffeescript'
        break
      case 'dns':
        codeext = 'dns-zone'
        break
      case 'zone':
        codeext = 'dns-zone'
        break
      case 'bind':
        codeext = 'dns-zone'
        break
      case 'plist':
        codeext = 'xml'
        break
      case 'xhtml':
        codeext = 'html'
        break
      case 'iml':
        codeext = 'xml'
        break
      case 'mk':
        codeext = 'makefile'
        break
      case 'mak':
        codeext = 'makefile'
        break
      case 'make':
        codeext = 'makefile'
        break
      case 'mkdown':
        codeext = 'markdown'
        break
      case 'mkd':
        codeext = 'markdown'
        break
      case 'nginxconf':
        codeext = 'nginx'
        break
      case 'nimrod':
        codeext = 'nim'
        break
      case 'mm':
        codeext = 'objectivec'
        break
      case 'obj-c':
        codeext = 'objectivec'
        break
      case 'obj-c++':
        codeext = 'objectivec'
        break
      case 'objective-c++':
        codeext = 'objectivec'
        break
      case 'ps':
        codeext = 'powershell'
        break
      case 'ps1':
        codeext = 'powershell'
        break
      case 'gyp':
        codeext = 'python'
        break
      case 'rs':
        codeext = 'rust'
        break
      case 'vb':
        codeext = 'vbnet'
        break
      case 'conf':
        codeext = 'ini'
        break
    }
  }
  return codeext
}
