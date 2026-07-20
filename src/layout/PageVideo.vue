<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import Artplayer from 'artplayer'
import type { Option } from 'artplayer/types/option'
import { ListVideo, Maximize2, Minus, Pin, PinOff, X } from 'lucide-vue-next'
import { useAppStore, useSettingStore } from '../store'
import type { IPageVideoPlaylistEntry } from '../store/appstore'
import AliFile from '../aliapi/file'
import MpvEmbeddedSurface from '../components/MpvEmbeddedSurface.vue'
import { getEncType, getProxyUrl, getRawUrl, isLocalProxyUrl, type IRawUrl } from '../utils/proxyhelper'
import message from '../utils/message'

interface VideoQuality {
  html: string
  quality: string
  url: string
  type?: string
  headers?: Record<string, string>
  default?: boolean
}

interface ResolvedVideoSource {
  url: string
  type: string
  headers: Record<string, string>
  quality: string
  qualityLabel: string
  qualities: VideoQuality[]
  subtitles: IRawUrl['subtitles']
  size: number
}

const appStore = useAppStore()
const settingStore = useSettingStore()
const pageVideo = appStore.pageVideo!
const playerElement = ref<HTMLDivElement | null>(null)
const loading = ref(true)
const errorText = ref('')
const isTop = ref(false)
const playlistVisible = ref(false)
const currentQuality = ref('')
const currentQualityLabel = ref('')
const resolvedQualities = ref<VideoQuality[]>([])
const mpvUrl = ref('')
const mpvHeaders = ref<Record<string, string>>({})
const mpvStatus = ref<any>(null)
const mpvExternalSubtitle = ref<{ url: string; title?: string } | undefined>()
const useEmbeddedMpv = settingStore.uiVideoPlayer === 'mpv' && window.platform === 'darwin'
const playlist = ref<IPageVideoPlaylistEntry[]>(pageVideo?.custom_playlist?.length ? [...pageVideo.custom_playlist] : [])
let art: Artplayer | null = null
let hls: { destroy: () => void } | null = null
let dashPlayer: any = null
let loadToken = 0
let streamingLoadToken = 0
let jassubPlayer: Artplayer | null = null
let pendingPosition = Number(pageVideo?.play_cursor || 0)
let lastSavedSecond = -1

const currentPlaylistIndex = computed(() => playlist.value.findIndex((item) => item.file_id === pageVideo.file_id))

function inferVideoType(url: string, hint = '') {
  if (hint) return hint.toLowerCase()
  const pathname = String(url || '').split('?')[0].split('#')[0].toLowerCase()
  if (pathname.endsWith('.m3u8')) return 'm3u8'
  if (pathname.endsWith('.mpd')) return 'mpd'
  if (pathname.endsWith('.ts')) return 'ts'
  return ''
}

function destroyStreamingPlayers() {
  streamingLoadToken++
  hls?.destroy()
  hls = null
  if (dashPlayer) {
    try { dashPlayer.reset() } catch {}
    dashPlayer = null
  }
}

function playHls(video: HTMLMediaElement, url: string) {
  destroyStreamingPlayers()
  const token = streamingLoadToken
  void import('hls.js').then(({ default: HlsJs }) => {
    if (token !== streamingLoadToken) return
    if (!HlsJs.isSupported()) {
      video.src = url
      return
    }
    const player = new HlsJs()
    if (token !== streamingLoadToken) {
      player.destroy()
      return
    }
    hls = player
    player.loadSource(url)
    player.attachMedia(video)
  }).catch(() => {
    if (token === streamingLoadToken) video.src = url
  })
}

function playDash(video: HTMLMediaElement, url: string) {
  destroyStreamingPlayers()
  const token = streamingLoadToken
  void import('dashjs').then((dashjs) => {
    if (token !== streamingLoadToken) return
    const player = dashjs.MediaPlayer().create()
    if (token !== streamingLoadToken) {
      player.reset()
      return
    }
    dashPlayer = player
    player.initialize(video, url, true)
  }).catch(() => {
    if (token === streamingLoadToken) errorText.value = 'DASH 播放组件加载失败'
  })
}

function toLocalFileUrl(filePath: string) {
  const normalized = String(filePath || '').replace(/\\/g, '/')
  return encodeURI(normalized.startsWith('/') ? `file://${normalized}` : `file:///${normalized}`)
}

function qualityMatchesPreference(item: VideoQuality, preference: string) {
  const values = [item.quality, item.html].map((value) => String(value || '').toUpperCase())
  return values.some((value) => value === preference.toUpperCase() || value.includes(preference.toUpperCase()))
}

function selectQuality(qualities: VideoQuality[]) {
  const preferred = settingStore.uiVideoQuality || 'Origin'
  return qualities.find((item) => qualityMatchesPreference(item, preferred)) || qualities[0]
}

function playableUrl(item: VideoQuality, size: number, fileId = pageVideo.file_id) {
  if (!item.headers || Object.keys(item.headers).length === 0 || isLocalProxyUrl(item.url)) return item.url
  return getProxyUrl({
    user_id: pageVideo.user_id,
    drive_id: pageVideo.drive_id,
    file_id: fileId,
    file_size: size,
    encType: pageVideo.encType,
    password: pageVideo.password,
    quality: item.quality || 'Origin',
    proxy_url: item.url,
    proxy_headers: JSON.stringify(item.headers)
  })
}

async function resolveCurrentSource(): Promise<ResolvedVideoSource> {
  if (pageVideo.drive_id === 'local') {
    return {
      url: toLocalFileUrl(pageVideo.file_id),
      type: inferVideoType(pageVideo.file_id),
      headers: {},
      quality: 'Origin',
      qualityLabel: '本地',
      qualities: [],
      subtitles: [],
      size: 0
    }
  }

  const data = await getRawUrl(pageVideo.user_id, pageVideo.drive_id, pageVideo.file_id, pageVideo.encType, pageVideo.password, false, 'video', '', 'thirdParty')
  if (typeof data === 'string') throw new Error(data || '获取视频地址失败')
  const rawQualities: VideoQuality[] = data.qualities.length
    ? data.qualities.map((item) => ({
      html: item.html || item.label || item.quality || '视频',
      quality: item.quality || item.value || 'Origin',
      url: item.url,
      type: item.type,
      headers: item.headers || data.headers
    }))
    : data.url ? [{ html: '原画', quality: 'Origin', url: data.url, headers: data.headers }] : []
  if (!rawQualities.length) throw new Error('未获取到可用的视频地址')
  const selected = rawQualities.find((item) => item.quality === currentQuality.value) || selectQuality(rawQualities)
  selected.default = true
  return {
    url: useEmbeddedMpv ? selected.url : playableUrl(selected, data.size),
    type: inferVideoType(selected.url, selected.type),
    headers: selected.headers || {},
    quality: selected.quality,
    qualityLabel: selected.html,
    qualities: rawQualities,
    subtitles: data.subtitles || [],
    size: data.size
  }
}

function subtitleUrl(subtitle: IRawUrl['subtitles'][number]) {
  if (!subtitle.headers || Object.keys(subtitle.headers).length === 0) return subtitle.url
  return getProxyUrl({
    user_id: pageVideo.user_id,
    drive_id: pageVideo.drive_id,
    file_id: pageVideo.file_id,
    proxy_kind: 'subtitle',
    proxy_url: subtitle.url,
    proxy_headers: JSON.stringify(subtitle.headers)
  })
}

function hasAssSubtitle(source: ResolvedVideoSource) {
  return source.subtitles.some((subtitle) => /\.(ass|ssa)(?:$|[?#])/i.test(subtitle.url))
}

async function installJassubPlugin(player: Artplayer, source: ResolvedVideoSource, token: number) {
  if (!hasAssSubtitle(source) || jassubPlayer === player) return
  try {
    const [pluginModule, workerModule, wasmModule, modernWasmModule, fontModule] = await Promise.all([
      import('artplayer-plugin-jassub'),
      import('jassub/dist/jassub-worker.js?url'),
      import('jassub/dist/jassub-worker.wasm?url'),
      import('jassub/dist/jassub-worker-modern.wasm?url'),
      import('jassub/dist/default.woff2?url')
    ])
    if (art !== player || token !== loadToken) return
    player.plugins.add(pluginModule.default({
      workerUrl: workerModule.default,
      wasmUrl: wasmModule.default,
      modernWasmUrl: modernWasmModule.default,
      availableFonts: { 'liberation sans': fontModule.default },
      fallbackFont: 'liberation sans',
      subContent: '[Script Info]\nScriptType: v4.00+'
    }))
    jassubPlayer = player
  } catch (error) {
    console.warn('ASS 字幕渲染组件加载失败:', error)
  }
}

function installQualityControl(source: ResolvedVideoSource) {
  if (!art || source.qualities.length < 2) return
  art.controls.update({
    name: 'quality',
    position: 'right',
    index: 20,
    html: source.qualityLabel,
    selector: source.qualities.map((item) => ({ ...item, default: item.quality === source.quality })),
    onSelect: (item: any) => {
      if (!art) return item.html
      currentQuality.value = item.quality
      currentQualityLabel.value = item.html
      const nextType = inferVideoType(item.url, item.type)
      art.type = nextType as any
      const position = art.currentTime || 0
      pendingPosition = position
      void art.switchQuality(playableUrl(item, source.size)).then(() => {
        if (art && pendingPosition > 0) art.currentTime = pendingPosition
      })
      return item.html
    }
  })
}

async function installSubtitleControl(source: ResolvedVideoSource) {
  if (!art || !source.subtitles.length || settingStore.uiVideoSubtitleMode === 'close') return
  const tracks = source.subtitles.map((item, index) => ({
    html: item.language || `字幕 ${index + 1}`,
    url: subtitleUrl(item),
    default: index === 0
  }))
  art.controls.update({
    name: 'subtitleList',
    position: 'right',
    index: 15,
    html: tracks[0].html,
    selector: tracks,
    onSelect: (item: any) => {
      void art?.subtitle.switch(item.url, { name: item.html, escape: false })
      if (art) art.subtitle.show = true
      return item.html
    }
  })
  await art.subtitle.switch(tracks[0].url, { name: tracks[0].html, escape: false })
  art.subtitle.show = true
}

function createArtplayer() {
  if (!playerElement.value) return
  const options: Option = {
    container: playerElement.value,
    url: '',
    volume: 0.8,
    autoplay: true,
    autoMini: true,
    loop: false,
    flip: true,
    playbackRate: true,
    aspectRatio: true,
    setting: true,
    hotkey: true,
    pip: true,
    airplay: true,
    mutex: true,
    fullscreen: true,
    fullscreenWeb: false,
    screenshot: true,
    subtitle: { escape: false },
    customType: {
      m3u8: (video, url) => playHls(video, url),
      ts: (video, url) => playHls(video, url),
      mpd: (video, url) => playDash(video, url)
    }
  }
  art = new Artplayer(options)
  art.on('ready', () => {
    if (!art) return
    if (pendingPosition > 0) art.currentTime = pendingPosition
    void art.play().catch(() => undefined)
  })
  art.on('video:loadedmetadata', () => {
    if (art && pendingPosition > 0) {
      art.currentTime = pendingPosition
      pendingPosition = 0
    }
  })
  art.on('video:timeupdate', () => {
    if (!art || art.video.paused) return
    const second = Math.floor(art.currentTime || 0)
    if (second > 0 && second % 10 === 0 && second !== lastSavedSecond) {
      lastSavedSecond = second
      void saveProgress(second)
    }
  })
  art.on('video:pause', () => { void saveProgress() })
  art.on('video:ended', () => { void stepPlaylist(1) })
  art.on('video:error', () => {
    errorText.value = '当前视频无法播放'
  })
}

async function loadCurrentVideo() {
  const token = ++loadToken
  loading.value = true
  errorText.value = ''
  document.title = pageVideo.file_name || '视频在线预览'
  try {
    const source = await resolveCurrentSource()
    if (token !== loadToken) return
    currentQuality.value = source.quality
    currentQualityLabel.value = source.qualityLabel
    resolvedQualities.value = source.qualities
    if (useEmbeddedMpv) {
      mpvUrl.value = source.url
      mpvHeaders.value = source.headers
      mpvExternalSubtitle.value = source.subtitles[0] ? { url: subtitleUrl(source.subtitles[0]), title: source.subtitles[0].language } : undefined
    } else if (art) {
      await installJassubPlugin(art, source, token)
      if (token !== loadToken || !art) return
      destroyStreamingPlayers()
      art.type = source.type as any
      art.url = source.url
      installQualityControl(source)
      await installSubtitleControl(source)
    }
  } catch (error: any) {
    if (token !== loadToken) return
    errorText.value = error?.message || String(error)
    message.error(errorText.value)
  } finally {
    if (token === loadToken) loading.value = false
  }
}

async function saveProgress(position = art?.currentTime || mpvStatus.value?.position || 0) {
  const second = Math.floor(Number(position || 0))
  if (second <= 0 || pageVideo.drive_id === 'local') return
  pageVideo.play_cursor = second
  try {
    await AliFile.ApiUpdateVideoTime(pageVideo.user_id, pageVideo.drive_id, pageVideo.file_id, second)
  } catch (error) {
    console.warn('保存视频进度失败:', error)
  }
}

function applyPlaylistItem(item: IPageVideoPlaylistEntry) {
  pageVideo.user_id = item.user_id || pageVideo.user_id
  pageVideo.drive_id = item.drive_id || pageVideo.drive_id
  pageVideo.file_id = item.file_id
  pageVideo.parent_file_id = item.parent_file_id || pageVideo.parent_file_id
  pageVideo.file_name = item.file_name
  pageVideo.html = item.html || item.file_name
  pageVideo.encType = item.encType ? getEncType({ description: item.encType }) : getEncType({ description: item.description || '' })
  pageVideo.password = item.password || ''
  pageVideo.play_cursor = Number(item.play_cursor || 0)
  pendingPosition = pageVideo.play_cursor
  currentQuality.value = ''
  lastSavedSecond = -1
}

async function selectPlaylistItem(fileId: string) {
  const item = playlist.value.find((entry) => entry.file_id === fileId)
  if (!item || item.file_id === pageVideo.file_id) return
  await saveProgress()
  applyPlaylistItem(item)
  await loadCurrentVideo()
  playlistVisible.value = false
}

async function stepPlaylist(step: number) {
  if (playlist.value.length < 2) return
  const index = currentPlaylistIndex.value
  if (index < 0) return
  const next = index + step
  if (next < 0 || next >= playlist.value.length) return
  await selectPlaylistItem(playlist.value[next].file_id)
}

async function selectMpvQuality(quality: string) {
  if (!quality || quality === currentQuality.value) return
  await saveProgress(mpvStatus.value?.position)
  pendingPosition = Number(mpvStatus.value?.position || 0)
  currentQuality.value = quality
  await loadCurrentVideo()
}

function handleMpvStatus(status: any) {
  mpvStatus.value = status
  const second = Math.floor(Number(status?.position || 0))
  if (!status?.paused && second > 0 && second % 10 === 0 && second !== lastSavedSecond) {
    lastSavedSecond = second
    void saveProgress(second)
  }
}

function handleTop() {
  window.WebToWindow?.({ cmd: 'top' }, (result: string) => { isTop.value = result === 'top' })
}

function handleMinimize() {
  window.WebToWindow?.({ cmd: 'minsize' })
}

function handleMaximize() {
  window.WebToWindow?.({ cmd: 'maxsize' })
}

async function handleClose() {
  await saveProgress()
  if (useEmbeddedMpv) await window.WebMpvEmbeddedControl?.({ action: 'stop' }).catch(() => undefined)
  if (window.WebToWindow) window.WebToWindow({ cmd: 'close' })
  else window.close()
}

function handleKeydown(event: KeyboardEvent) {
  if (event.altKey && event.key.toLowerCase() === 'f4') void handleClose()
  else if (event.altKey && event.key.toLowerCase() === 'm') window.WebToWindow?.({ cmd: 'minsize' })
  else if (event.altKey && event.key === 'Enter') window.WebToWindow?.({ cmd: 'maxsize' })
  else if (event.altKey && event.key.toLowerCase() === 't') handleTop()
}

onMounted(async () => {
  document.body.setAttribute('arco-theme', 'dark')
  window.addEventListener('keydown', handleKeydown, true)
  if (!useEmbeddedMpv) createArtplayer()
  await loadCurrentVideo()
})

onBeforeUnmount(() => {
  loadToken++
  window.removeEventListener('keydown', handleKeydown, true)
  void saveProgress()
  if (useEmbeddedMpv) void window.WebMpvEmbeddedControl?.({ action: 'stop' })
  destroyStreamingPlayers()
  art?.destroy(false)
  art = null
  jassubPlayer = null
})
</script>

<template>
  <div class="video-preview">
    <header class="video-titlebar q-electron-drag">
      <span class="video-title">{{ pageVideo?.file_name || '视频在线预览' }}</span>
      <div class="window-actions q-electron-no-drag">
        <button v-if="playlist.length > 1" type="button" title="播放列表" @click="playlistVisible = !playlistVisible"><ListVideo :size="17" /></button>
        <button type="button" :title="isTop ? '取消置顶' : '置顶'" @click="handleTop"><PinOff v-if="isTop" :size="16" /><Pin v-else :size="16" /></button>
        <button type="button" title="最小化" @click="handleMinimize"><Minus :size="17" /></button>
        <button type="button" title="最大化" @click="handleMaximize"><Maximize2 :size="15" /></button>
        <button type="button" title="关闭" @click="handleClose"><X :size="18" /></button>
      </div>
    </header>

    <main class="video-content">
      <MpvEmbeddedSurface
        v-if="useEmbeddedMpv && mpvUrl"
        :current-file-id="pageVideo.file_id"
        :current-quality="currentQuality"
        :external-subtitle="mpvExternalSubtitle"
        :headers="mpvHeaders"
        :playlist="playlist.map((item) => ({ file_id: item.file_id, html: item.html, name: item.file_name, default: item.file_id === pageVideo.file_id }))"
        :qualities="resolvedQualities"
        :quality-label="currentQualityLabel"
        :start-position="pendingPosition || pageVideo.play_cursor || 0"
        :title="pageVideo.file_name"
        :url="mpvUrl"
        @error="(value) => errorText = value"
        @playlist-next="stepPlaylist(1)"
        @playlist-prev="stepPlaylist(-1)"
        @playlist-select="selectPlaylistItem"
        @quality-select="selectMpvQuality"
        @status="handleMpvStatus"
      />
      <div v-show="!useEmbeddedMpv" ref="playerElement" class="art-player" />
      <div v-if="loading" class="player-state">正在获取播放地址...</div>
      <div v-else-if="errorText" class="player-state error">{{ errorText }}</div>

      <aside v-if="playlistVisible && playlist.length > 1" class="video-playlist q-electron-no-drag">
        <div class="playlist-heading">播放列表 <span>{{ playlist.length }}</span></div>
        <button
          v-for="(item, index) in playlist"
          :key="item.file_id"
          :class="{ active: item.file_id === pageVideo.file_id }"
          type="button"
          @click="selectPlaylistItem(item.file_id)"
        >
          <span>{{ String(index + 1).padStart(2, '0') }}</span>
          <strong>{{ item.html || item.file_name }}</strong>
        </button>
      </aside>
    </main>
  </div>
</template>

<style scoped>
.video-preview {
  display: flex;
  width: 100vw;
  height: 100vh;
  flex-direction: column;
  overflow: hidden;
  background: #000;
  color: #fff;
}

.video-titlebar {
  position: relative;
  z-index: 20;
  display: flex;
  height: 40px;
  flex: 0 0 40px;
  align-items: center;
  border-bottom: 1px solid #2c2f34;
  background: #202226;
}

.video-title {
  min-width: 0;
  flex: 1;
  overflow: hidden;
  padding-left: 14px;
  color: #c5c8cd;
  font-size: 12px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.window-actions {
  display: flex;
  height: 100%;
  align-items: center;
}

.window-actions button {
  display: inline-flex;
  width: 42px;
  height: 100%;
  align-items: center;
  justify-content: center;
  border: 0;
  background: transparent;
  color: #c5c8cd;
  cursor: pointer;
}

.window-actions button:hover {
  background: #383c42;
  color: #fff;
}

.window-actions button:last-child:hover {
  background: #c42b1c;
}

.video-content {
  position: relative;
  min-height: 0;
  flex: 1;
}

.art-player {
  width: 100%;
  height: 100%;
}

.player-state {
  position: absolute;
  inset: 0;
  z-index: 8;
  display: flex;
  align-items: center;
  justify-content: center;
  background: #000;
  color: #aeb3ba;
  font-size: 14px;
  pointer-events: none;
}

.player-state.error {
  color: #ff9b93;
}

.video-playlist {
  position: absolute;
  top: 0;
  right: 0;
  bottom: 0;
  z-index: 15;
  width: min(340px, 42vw);
  overflow: auto;
  border-left: 1px solid #383c42;
  background: rgba(27, 29, 33, .97);
  box-shadow: -14px 0 38px rgba(0, 0, 0, .34);
}

.playlist-heading {
  position: sticky;
  top: 0;
  padding: 17px 16px 12px;
  background: #1b1d21;
  font-size: 14px;
  font-weight: 650;
}

.playlist-heading span {
  margin-left: 5px;
  color: #777e87;
  font-size: 12px;
  font-weight: 400;
}

.video-playlist button {
  display: grid;
  width: 100%;
  min-height: 46px;
  padding: 8px 14px;
  grid-template-columns: 28px minmax(0, 1fr);
  align-items: center;
  gap: 8px;
  border: 0;
  background: transparent;
  color: #aeb3ba;
  cursor: pointer;
  text-align: left;
}

.video-playlist button:hover,
.video-playlist button.active {
  background: #30343a;
  color: #fff;
}

.video-playlist button span {
  color: #737a83;
  font-size: 11px;
}

.video-playlist button strong {
  overflow: hidden;
  font-size: 13px;
  font-weight: 500;
  text-overflow: ellipsis;
  white-space: nowrap;
}

@media (max-width: 640px) {
  .video-playlist {
    width: 78vw;
  }

  .window-actions button {
    width: 36px;
  }
}
</style>
