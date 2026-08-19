<script setup>
// 网页播放器只保留浏览器/WebView 可解码路径：原生 MP4/WebM/Ogg，按需加载
// HLS.js 和 dash.js 的 MSE 流。所有远程请求都经 Go 侧本地会话代理。
import { ref, computed, onMounted, onBeforeUnmount, nextTick, watch } from 'vue'
import { playVideo, playVideoQuality, pinFileSnapshot, getPlayCursor, savePlayCursor, getSettings, previewUrl } from '../api'
import { getPrefs } from '../appearance'
import { srtToVtt, parseSup, SupRenderer } from '../player/subtitles'
import UiIcon from './UiIcon.vue'

const props = defineProps({
  account: { type: Object, required: true },
  file: { type: Object, required: true },
  files: { type: Array, default: () => [] },
})
const emit = defineEmits(['close', 'toast', 'select-file'])

const videoEl = ref(null)
const containerEl = ref(null)
const progressEl = ref(null)
const loading = ref(true)
const error = ref('')
const playing = ref(false)
const position = ref(0)
const duration = ref(0)
const buffered = ref(0)
const volume = ref(Math.min(200, Math.max(0, getPrefs().defaultVolume ?? 100)))
const muted = ref(false)
const brightness = ref(100)
const speed = ref(getPrefs().defaultSpeed || 1)
const seekStep = getPrefs().seekStep || 10
const qualities = ref([])
const currentQuality = ref('')
const src = ref('')
const streamType = ref('')
const looping = ref(false)
const isFullscreen = ref(false)
const pipActive = ref(false)
const showControls = ref(true)
const subtitleSources = ref([])
const subtitleTracks = ref([])
const currentSubtitle = ref('')
const subtitleEnabled = ref(false)
const activeMenu = ref('')
const subtitleScale = ref(1)
const subtitlePosition = ref('bottom')
const scrubVisible = ref(false)
const scrubX = ref(0)
const scrubTime = ref(0)
const centerPulse = ref(false)
const osdVisible = ref(false)
const osdIcon = ref('volume')
const osdText = ref('')
const osdPct = ref(0)
const supCanvasEl = ref(null)
const localSubInput = ref(null)
const extraTextSubs = ref([]) // 网盘同名字幕 + 本地文本字幕（srt/vtt）
const supTracks = ref([])     // SUP 图形字幕：{ label, url }
const supActive = ref(false)
const assTracks = ref([])     // ASS/SSA 特效字幕：{ label, url | content }
const assActive = ref(false)
const fsAnim = ref('') // 'in' | 'out'：全屏切换过渡动画

let unmounted = false
let playbackSeq = 0
let sourceSeq = 0
let controlsTimer = null
let saveTimer = null
let pendingResume = 0
let pendingAutoplay = true
let suppressVideoErrors = false
let hlsPlayer = null
let dashPlayer = null
let hlsRecoveryAttempts = 0
let dashRecoveryAttempts = 0
let activeSourceURL = ''
let playbackEnded = false
let centerTimer = null
let osdTimer = null
let audioCtx = null
let gainNode = null
let supRenderer = null
let fsAnimTimer = null
let assRenderer = null

const SPEEDS = [0.5, 0.75, 1, 1.25, 1.5, 2]
const speedOptions = SPEEDS.map((s) => ({ value: s, label: s + 'x' }))
const UNSUPPORTED_WEB_CONTAINERS = new Set(['avi', 'flv', 'm2ts', 'mkv', 'mpg', 'mpeg', 'mts', 'rm', 'rmvb', 'ts', 'wmv'])
const episodeFiles = computed(() => (props.files || []).filter((candidate) => !candidate?.isDir && isVideoFile(candidate)))
const thumbnailUrl = computed(() => String(props.file?.thumbnail || props.file?.thumbnail_url || props.file?.thumb_url || '').trim())
const episodeIndex = computed(() => episodeFiles.value.findIndex((candidate) => candidate.file_id === props.file?.file_id))
const currentQualityLabel = computed(() => qualities.value.find((quality) => quality.value === currentQuality.value)?.label || currentQuality.value || (streamType.value || '网页播放').toUpperCase())

onMounted(() => {
  document.addEventListener('keydown', onKeyDown)
  document.addEventListener('fullscreenchange', onFullscreenChange)
  document.addEventListener('pointerdown', onDocumentPointerDown, true)
  window.addEventListener('resize', onWindowResize)
  startPlayback()
})
watch(() => props.file?.file_id, (nextId, previousId) => {
  if (!nextId || nextId === previousId || unmounted) return
  saveCursor(previousId)
  if (saveTimer) {
    clearInterval(saveTimer)
    saveTimer = null
  }
  startPlayback()
})
onBeforeUnmount(() => {
  unmounted = true
  playbackSeq++
  sourceSeq++
  saveCursor()
  destroyAdaptivePlayers()
  if (saveTimer) clearInterval(saveTimer)
  if (controlsTimer) clearTimeout(controlsTimer)
  if (centerTimer) clearTimeout(centerTimer)
  if (osdTimer) clearTimeout(osdTimer)
  if (audioCtx) { try { audioCtx.close() } catch {}; audioCtx = null; gainNode = null }
  if (fsAnimTimer) clearTimeout(fsAnimTimer)
  stopSup()
  destroyAss()
  document.removeEventListener('keydown', onKeyDown)
  document.removeEventListener('fullscreenchange', onFullscreenChange)
  document.removeEventListener('pointerdown', onDocumentPointerDown, true)
  window.removeEventListener('resize', onWindowResize)
})

// 全屏切换：窗口级变化比较生硬，用短暂的缩放+淡入过渡
watch(isFullscreen, (full) => {
  fsAnim.value = ''
  if (containerEl.value) void containerEl.value.offsetWidth
  fsAnim.value = full ? 'in' : 'out'
  if (fsAnimTimer) clearTimeout(fsAnimTimer)
  fsAnimTimer = setTimeout(() => { fsAnim.value = '' }, 360)
})

function isVideoFile(file) {
  const category = String(file?.category || file?.kind || '').toLowerCase()
  if (category === 'video') return true
  const name = String(file?.name || '')
  const index = name.lastIndexOf('.')
  return index > 0 && ['mp4', 'm4v', 'webm', 'ogg', 'ogv', 'mov', 'm3u8', 'mpd', '3gp', 'avi', 'mkv', 'flv', 'm2ts', 'mpg', 'mpeg', 'mts', 'rm', 'rmvb', 'ts', 'wmv'].includes(name.slice(index + 1).toLowerCase())
}

async function startPlayback() {
  const seq = ++playbackSeq
  sourceSeq++
  if (saveTimer) {
    clearInterval(saveTimer)
    saveTimer = null
  }
  destroyAdaptivePlayers()
  loading.value = true
  error.value = ''
  playing.value = false
  position.value = 0
  buffered.value = 0
  duration.value = 0
  src.value = ''
  activeSourceURL = ''
  playbackEnded = false
  streamType.value = ''
  subtitleSources.value = []
  subtitleTracks.value = []
  extraTextSubs.value = []
  supTracks.value = []
  assTracks.value = []
  stopSup()
  destroyAss()
  pendingResume = 0
  clearVideoSource()
  try {
    await pinFileSnapshot(props.account.user_id, props.account.drive_id, props.file)
    if (unmounted || seq !== playbackSeq) return
    const settings = await getSettings().catch(() => null)
    let resumeAt = 0
    if (!settings || settings.playbackResume !== false) {
      resumeAt = await getPlayCursor(props.account.user_id, props.account.drive_id, props.file.file_id).catch(() => 0)
    }
    if (unmounted || seq !== playbackSeq) return
    const preview = await playVideo(props.account.user_id, props.account.drive_id, props.file.file_id)
    if (unmounted || seq !== playbackSeq) return
    setQualityOptions(preview)
    if (preview && Number.isFinite(preview.duration) && preview.duration > 0) duration.value = preview.duration
    await mountCloudSubtitles()
    if (unmounted || seq !== playbackSeq) return
    loading.value = false
    await nextTick()
    if (unmounted || seq !== playbackSeq) return
    await loadPlaybackSource(preview, resumeAt, true, seq)
  } catch (e) {
    if (!unmounted && seq === playbackSeq) error.value = readablePlaybackError(e)
  } finally {
    if (!unmounted && seq === playbackSeq && loading.value) loading.value = false
  }
}

async function loadPlaybackSource(preview, resumeAt, autoplay, parentSeq) {
  const url = String(preview && preview.url || '').trim()
  if (!url) throw new Error('未获取到可播放的视频地址')
  const v = videoEl.value
  if (!v) throw new Error('播放器尚未初始化')
  const loadSeq = ++sourceSeq
  destroyAdaptivePlayers()
  clearVideoSource()
  src.value = ''
  activeSourceURL = url
  playbackEnded = false
  pendingResume = Math.max(0, Number(resumeAt) || 0)
  pendingAutoplay = Boolean(autoplay)
  streamType.value = normalizeStreamType(preview)
  subtitleSources.value = [...normalizeSubtitles(preview && preview.subtitles), ...extraTextSubs.value]
  subtitleTracks.value = []
  subtitleEnabled.value = false
  currentSubtitle.value = ''
  stopSup()
  destroyAss()
  await nextTick()
  if (unmounted || parentSeq !== playbackSeq || loadSeq !== sourceSeq) return

  if (streamType.value === 'hls') {
    await loadHLS(v, url, loadSeq)
    return
  }
  if (streamType.value === 'dash') {
    await loadDASH(v, url, loadSeq)
    return
  }
  if (UNSUPPORTED_WEB_CONTAINERS.has(streamType.value)) {
    throw new Error(`网页播放器不支持 ${streamType.value.toUpperCase()} 容器，请下载后使用本地播放器打开`)
  }
  await loadNativeSource(v, url, loadSeq)
}

async function loadNativeSource(v, url, loadSeq) {
  src.value = url
  await nextTick()
  if (unmounted || loadSeq !== sourceSeq || videoEl.value !== v) return
  suppressVideoErrors = false
  v.load()
}

async function loadHLS(v, url, loadSeq) {
  if (canPlayNativeHLS(v)) {
    await loadNativeSource(v, url, loadSeq)
    return
  }
  const Hls = await getHlsConstructor()
  if (unmounted || loadSeq !== sourceSeq) return
  if (!Hls || !Hls.isSupported()) throw new Error('当前系统 WebView 不支持 HLS 播放')
  const player = new Hls({
    enableWorker: true,
    lowLatencyMode: false,
    capLevelToPlayerSize: true,
    maxBufferLength: 30,
    maxMaxBufferLength: 60,
    backBufferLength: 30,
  })
  hlsPlayer = player
  hlsRecoveryAttempts = 0
  suppressVideoErrors = false
  player.on(Hls.Events.MEDIA_ATTACHED, () => {
    if (!unmounted && player === hlsPlayer && loadSeq === sourceSeq) player.loadSource(url)
  })
  player.on(Hls.Events.ERROR, (_, data) => handleHLSError(Hls, player, data, loadSeq))
  player.attachMedia(v)
}

async function loadDASH(v, url, loadSeq, retrying = false) {
  if (!window.MediaSource) throw new Error('当前系统 WebView 不支持 DASH 播放')
  const dashjs = await getDashConstructor()
  if (unmounted || loadSeq !== sourceSeq) return
  const player = dashjs.MediaPlayer().create()
  dashPlayer = player
  if (!retrying) dashRecoveryAttempts = 0
  suppressVideoErrors = false
  player.updateSettings({
    streaming: {
      buffer: {
        bufferTimeAtTopQuality: 20,
        bufferTimeAtTopQualityLongForm: 30,
        stableBufferTime: 12,
      },
      fastSwitchEnabled: false,
    },
  })
  player.on(dashjs.MediaPlayer.events.ERROR, (event) => handleDASHError(player, event, loadSeq))
  player.initialize(v, url, false)
}

function handleHLSError(Hls, player, data, loadSeq) {
  if (!data || !data.fatal || player !== hlsPlayer || loadSeq !== sourceSeq) return
  if (data.type === Hls.ErrorTypes.NETWORK_ERROR && hlsRecoveryAttempts < 1) {
    hlsRecoveryAttempts++
    player.startLoad()
    return
  }
  if (data.type === Hls.ErrorTypes.MEDIA_ERROR && hlsRecoveryAttempts < 1) {
    hlsRecoveryAttempts++
    player.recoverMediaError()
    return
  }
  failPlayback('HLS 流加载失败，请检查网络或重新获取播放地址')
}

function handleDASHError(player, event, loadSeq) {
  if (player !== dashPlayer || loadSeq !== sourceSeq) return
  const retryURL = activeSourceURL
  if (dashRecoveryAttempts < 1 && retryURL && videoEl.value) {
    dashRecoveryAttempts++
    try {
      dashPlayer = null
      player.reset()
      void loadDASH(videoEl.value, retryURL, loadSeq, true).catch((e) => failPlayback(readablePlaybackError(e)))
      return
    } catch {}
  }
  const detail = event && event.error && event.error.message ? `：${event.error.message}` : ''
  failPlayback('DASH 流加载失败' + detail)
}

function failPlayback(message) {
  destroyAdaptivePlayers()
  playing.value = false
  error.value = message
}

function destroyAdaptivePlayers() {
  const hls = hlsPlayer
  hlsPlayer = null
  if (hls) {
    try { hls.destroy() } catch {}
  }
  const dash = dashPlayer
  dashPlayer = null
  if (dash) {
    try { dash.reset() } catch {}
  }
}

function clearVideoSource() {
  const v = videoEl.value
  if (!v) return
  suppressVideoErrors = true
  try {
    v.pause()
    v.removeAttribute('src')
    v.load()
  } catch {}
}

let hlsConstructorPromise = null
async function getHlsConstructor() {
  if (!hlsConstructorPromise) hlsConstructorPromise = import('hls.js').then((mod) => mod.default)
  return hlsConstructorPromise
}

let dashConstructorPromise = null
async function getDashConstructor() {
  if (!dashConstructorPromise) dashConstructorPromise = import('dashjs').then((mod) => mod.default || mod)
  return dashConstructorPromise
}

function normalizeStreamType(preview) {
  const declared = String(preview && preview.stream_type || '').trim().toLowerCase()
  if (declared === 'm3u8') return 'hls'
  if (declared === 'mpd') return 'dash'
  if (declared) return declared
  const ext = extensionOf(props.file.name)
  if (ext === 'm3u8') return 'hls'
  if (ext === 'mpd') return 'dash'
  if (['m4v', 'mov', '3gp'].includes(ext)) return 'mp4'
  return ext
}

function canPlayNativeHLS(v) {
  return Boolean(v.canPlayType('application/vnd.apple.mpegurl') || v.canPlayType('application/x-mpegURL'))
}

function extensionOf(name) {
  const value = String(name || '')
  const index = value.lastIndexOf('.')
  return index > 0 ? value.slice(index + 1).toLowerCase() : ''
}

function normalizeSubtitles(value) {
  if (!Array.isArray(value)) return []
  return value
    .map((track, index) => ({
      url: String(track && track.url || '').trim(),
      language: String(track && track.language || 'und').trim() || 'und',
      label: String(track && track.language || `字幕 ${index + 1}`).trim() || `字幕 ${index + 1}`,
    }))
    .filter((track) => track.url)
}

function setQualityOptions(preview) {
  const seen = new Set()
  qualities.value = (preview && Array.isArray(preview.qualities) ? preview.qualities : [])
    .map((q) => ({ value: String(q.value || q.quality || q.label || '').trim(), label: q.label || q.quality || q.value || '原画' }))
    .filter((q) => q.value && !seen.has(q.value) && seen.add(q.value))
  currentQuality.value = String(preview && preview.current_quality || (qualities.value[0] && qualities.value[0].value) || '')
}

function onLoaded() {
  const v = videoEl.value
  if (!v) return
  playbackEnded = false
  updateDuration(v)
  applyVolume()
  v.playbackRate = speed.value
  if (pendingResume > 0 && (!Number.isFinite(v.duration) || pendingResume < v.duration)) v.currentTime = pendingResume
  pendingResume = 0
  const autoplay = pendingAutoplay
  pendingAutoplay = false
  if (autoplay) v.play().catch(() => {})
  if (saveTimer) clearInterval(saveTimer)
  saveTimer = setInterval(saveCursor, 5000)
  onTracksChange()
}

function updateDuration(v) {
  if (Number.isFinite(v.duration) && v.duration > 0) duration.value = v.duration
}

function onDurationChange() {
  const v = videoEl.value
  if (v) updateDuration(v)
}

function onLoadedData() {
  onTracksChange()
}

function onTimeUpdate() {
  const v = videoEl.value
  if (!v) return
  if (playbackEnded && (!Number.isFinite(v.duration) || v.currentTime < v.duration)) playbackEnded = false
  position.value = v.currentTime
  updateBuffered(v)
  if (supActive.value) renderSupFrame()
}

function onProgress() {
  const v = videoEl.value
  if (v) updateBuffered(v)
}

function updateBuffered(v) {
  try {
    if (v.buffered.length > 0) buffered.value = v.buffered.end(v.buffered.length - 1)
  } catch {}
}

function onPlay() { playbackEnded = false; playing.value = true; scheduleHideControls() }
function onPause() { playing.value = false; showControls.value = true }
function onEnded() {
  playing.value = false
  if (!looping.value) {
    playbackEnded = true
    clearPlayCursor()
    showControls.value = true
  }
}
function onError() {
  if (loading.value || suppressVideoErrors || hlsPlayer || dashPlayer) return
  const code = videoEl.value && videoEl.value.error && videoEl.value.error.code
  error.value = code === 4 ? '当前网页播放器不支持此视频的容器或编解码' : '视频加载失败，请检查网络或重新获取播放地址'
}
function onVolumeChange() {
  const v = videoEl.value
  if (!v) return
  // 增益链接管后 video.volume 恒为 1，音量以 volume ref 为准
  if (!gainNode) volume.value = Math.round(v.volume * 100)
  muted.value = v.muted
}
function onPipEnter() { pipActive.value = true }
function onPipLeave() { pipActive.value = false }

// ---- controls ----
function togglePlay() {
  const v = videoEl.value
  if (!v) return
  if (audioCtx && audioCtx.state === 'suspended') audioCtx.resume().catch(() => {})
  centerPulse.value = true
  if (centerTimer) clearTimeout(centerTimer)
  centerTimer = setTimeout(() => { centerPulse.value = false }, 700)
  if (v.paused) v.play().catch(() => {})
  else v.pause()
}

function seek(delta) {
  const v = videoEl.value
  if (!v || !Number.isFinite(v.duration)) return
  playbackEnded = false
  v.currentTime = Math.max(0, Math.min(v.duration, v.currentTime + delta))
}

function onSeekInput(e) {
  const v = videoEl.value
  if (!v) return
  const target = Number(e.target.value)
  if (!Number.isFinite(target)) return
  playbackEnded = false
  v.currentTime = target
  position.value = target
}

function onVolume(e) {
  volume.value = Number(e.target.value)
  applyVolume()
  showOsd(muted.value || volume.value === 0 ? 'volume-x' : 'volume', `音量 ${muted.value ? 0 : volume.value}%`, (muted.value ? 0 : volume.value) / 2)
}

// 音量 0–200%：100% 以内用原生 volume，超过后接入 WebAudio 增益链
function ensureAudioChain() {
  const v = videoEl.value
  if (gainNode || !v) return
  try {
    const Ctx = window.AudioContext || window.webkitAudioContext
    if (!Ctx) return
    audioCtx = audioCtx || new Ctx()
    if (audioCtx.state === 'suspended') audioCtx.resume().catch(() => {})
    const source = audioCtx.createMediaElementSource(v)
    gainNode = audioCtx.createGain()
    source.connect(gainNode)
    gainNode.connect(audioCtx.destination)
  } catch {
    gainNode = null
  }
}

function applyVolume() {
  const v = videoEl.value
  if (!v) return
  const level = volume.value
  if (level > 100) ensureAudioChain()
  if (gainNode) {
    v.volume = 1
    gainNode.gain.value = level / 100
  } else {
    v.volume = Math.min(100, level) / 100
  }
  if (level > 0) v.muted = false
}

function showOsd(icon, text, pct) {
  osdIcon.value = icon
  osdText.value = text
  osdPct.value = Math.max(0, Math.min(100, pct))
  osdVisible.value = true
  if (osdTimer) clearTimeout(osdTimer)
  osdTimer = setTimeout(() => { osdVisible.value = false }, 900)
}

function toggleMute() {
  const v = videoEl.value
  if (v) v.muted = !v.muted
}

function onSpeed(value) {
  speed.value = Number(value)
  const v = videoEl.value
  if (v) v.playbackRate = speed.value
}

function toggleLoop() {
  looping.value = !looping.value
  const v = videoEl.value
  if (v) v.loop = looping.value
}

function toggleFullscreen() {
  const el = containerEl.value
  const video = videoEl.value
  if (!el) return
  const wailsRuntime = window.runtime
  if (wailsRuntime && typeof wailsRuntime.WindowFullscreen === 'function') {
    try {
      if (isFullscreen.value) {
        wailsRuntime.WindowUnfullscreen?.()
        isFullscreen.value = false
      } else {
        wailsRuntime.WindowFullscreen()
        isFullscreen.value = true
      }
      return
    } catch {}
  }
  if (document.fullscreenElement === el) {
    if (typeof document.exitFullscreen === 'function') document.exitFullscreen().catch?.(() => {})
    return
  }
  if (typeof el.requestFullscreen === 'function') {
    const request = el.requestFullscreen()
    if (request && typeof request.catch === 'function') request.catch(() => emit('toast', '当前系统不支持全屏播放', 'error'))
    return
  }
  // WebKit's desktop/iOS WebView exposes the legacy video-only API.
  if (video && typeof video.webkitEnterFullscreen === 'function') {
    video.webkitEnterFullscreen()
    return
  }
  emit('toast', '当前系统不支持全屏播放', 'error')
}

function onFullscreenChange() {
  isFullscreen.value = document.fullscreenElement === containerEl.value
}

function onWebkitFullscreenEnter() { isFullscreen.value = true }
function onWebkitFullscreenLeave() { isFullscreen.value = false }

function onWebkitPresentationModeChange() {
  const video = videoEl.value
  if (!video) return
  pipActive.value = video.webkitPresentationMode === 'picture-in-picture'
}

async function togglePip() {
  const v = videoEl.value
  if (!v) return
  try {
    if (document.pictureInPictureElement) await document.exitPictureInPicture()
    else if (document.pictureInPictureEnabled) await v.requestPictureInPicture()
    else if (typeof v.webkitSetPresentationMode === 'function') v.webkitSetPresentationMode('picture-in-picture')
    else throw new Error('当前系统 WebView 不支持画中画')
  } catch (e) {
    emit('toast', '画中画不可用: ' + String(e), 'error')
  }
}

function screenshot() {
  const v = videoEl.value
  if (!v || !v.videoWidth) return
  try {
    const canvas = document.createElement('canvas')
    canvas.width = v.videoWidth
    canvas.height = v.videoHeight
    const context = canvas.getContext('2d')
    if (!context) return
    context.drawImage(v, 0, 0)
    canvas.toBlob((blob) => {
      if (!blob) return
      const url = URL.createObjectURL(blob)
      const link = document.createElement('a')
      link.href = url
      link.download = (props.file.name || 'screenshot').replace(/\.[^.]+$/, '') + '-' + Math.floor(position.value) + 's.png'
      link.click()
      setTimeout(() => URL.revokeObjectURL(url), 1000)
      emit('toast', '截图已保存')
    }, 'image/png')
  } catch (e) {
    emit('toast', '当前视频无法截图: ' + String(e), 'error')
  }
}

// ---- subtitle ----
function onTracksChange() {
  const v = videoEl.value
  if (!v) return
  const tracks = []
  let selected = -1
  for (let index = 0; index < v.textTracks.length; index++) {
    const track = v.textTracks[index]
    tracks.push({ index, label: track.label || `字幕 ${index + 1}`, kind: track.kind })
    if (track.mode === 'showing') selected = index
  }
  subtitleTracks.value = tracks
  subtitleEnabled.value = selected >= 0
  currentSubtitle.value = selected >= 0 ? String(selected) : ''
  setSubtitlePosition(subtitlePosition.value)
  // 自动挂载同名字幕偏好：有可用文本轨且未选择时默认开启第一条
  if (selected < 0 && tracks.length > 0 && !supActive.value && getPrefs().autoLoadSubtitles !== false) selectSubtitle(0)
}

function selectSubtitle(index) {
  const v = videoEl.value
  if (!v) return
  for (let trackIndex = 0; trackIndex < v.textTracks.length; trackIndex++) {
    v.textTracks[trackIndex].mode = trackIndex === index ? 'showing' : 'disabled'
  }
  subtitleEnabled.value = index >= 0
  currentSubtitle.value = index >= 0 ? String(index) : ''
}

function toggleSubtitle() {
  if (supActive.value) { stopSup(); currentSubtitle.value = ''; return }
  if (assActive.value) { destroyAss(); currentSubtitle.value = ''; return }
  if (subtitleEnabled.value) selectSubtitle(-1)
  else if (subtitleTracks.value.length > 0) selectSubtitle(0)
}

function onSubtitleSelected(value) {
  stopSup()
  destroyAss()
  selectSubtitle(value === 'off' ? -1 : Number(value))
}

// ---- 网盘同名字幕 / 本地字幕 / SUP 图形字幕 ----
function isSubtitleSibling(file, base) {
  const name = String(file?.name || '').toLowerCase()
  if (!name.startsWith(base + '.')) return false
  return ['srt', 'vtt', 'ass', 'ssa', 'sup'].includes(extensionOf(name))
}

async function mountCloudSubtitles() {
  const base = String(props.file.name || '').replace(/\.[^.]+$/, '').toLowerCase()
  if (!base) return
  const siblings = (props.files || []).filter((f) => !f.isDir && f.file_id !== props.file.file_id && isSubtitleSibling(f, base))
  for (const sibling of siblings) {
    const ext = extensionOf(sibling.name)
    try {
      const url = await previewUrl(props.account.user_id, props.account.drive_id, sibling.file_id)
      if (unmounted) return
      if (ext === 'sup') {
        supTracks.value = [...supTracks.value, { label: sibling.name, url }]
      } else if (ext === 'ass' || ext === 'ssa') {
        assTracks.value = [...assTracks.value, { label: sibling.name, url }]
      } else {
        // 代理按扩展名自动完成 srt→vtt 转换
        extraTextSubs.value = [...extraTextSubs.value, { url, label: sibling.name }]
      }
    } catch { /* 单个字幕获取失败不影响其余 */ }
  }
}

function textBlobUrl(text) {
  return URL.createObjectURL(new Blob([text], { type: 'text/vtt' }))
}

async function selectSup(index) {
  const track = supTracks.value[index]
  if (!track) return
  selectSubtitle(-1)
  destroyAss()
  try {
    const buffer = await (await fetch(track.url)).arrayBuffer()
    const sets = parseSup(buffer)
    if (!sets.length) throw new Error('no display sets')
    if (!supRenderer) supRenderer = new SupRenderer(supCanvasEl.value)
    supRenderer.load(sets)
    supActive.value = true
    currentSubtitle.value = 'sup:' + index
    renderSupFrame()
  } catch {
    emit('toast', 'SUP 字幕解析失败', 'error')
  }
}

function stopSup() {
  supActive.value = false
  if (supRenderer) supRenderer.stop()
}

// ---- ASS/SSA 特效字幕（libass WASM 渲染，保留定位/颜色/卡拉 OK 等全部特效） ----
let jassubPromise = null
async function getJassub() {
  if (!jassubPromise) {
    jassubPromise = Promise.all([
      import('jassub'),
      import('jassub/dist/wasm/jassub-worker.js?url'),
      import('jassub/dist/wasm/jassub-worker.wasm?url'),
      import('jassub/dist/default.woff2?url'),
    ]).then(([mod, worker, wasm, font]) => ({
      JASSUB: mod.default,
      workerUrl: worker.default,
      wasmUrl: wasm.default,
      fontUrl: font.default,
    }))
  }
  return jassubPromise
}

async function selectAss(index) {
  const track = assTracks.value[index]
  if (!track) return
  selectSubtitle(-1)
  stopSup()
  try {
    const content = track.content || await (await fetch(track.url)).text()
    if (unmounted) return
    const { JASSUB, workerUrl, wasmUrl, fontUrl } = await getJassub()
    if (unmounted) return
    destroyAss()
    assRenderer = new JASSUB({ video: videoEl.value, subContent: content, workerUrl, wasmUrl, fonts: [fontUrl] })
    assActive.value = true
    currentSubtitle.value = 'ass:' + index
  } catch {
    emit('toast', 'ASS 字幕加载失败', 'error')
  }
}

function destroyAss() {
  assActive.value = false
  if (assRenderer) {
    try { assRenderer.destroy() } catch {}
    assRenderer = null
  }
}

function onWindowResize() {
  if (supActive.value) renderSupFrame()
}

function renderSupFrame() {
  const v = videoEl.value
  const canvas = supCanvasEl.value
  if (!v || !canvas || !supRenderer) return
  supRenderer.renderAt(v.currentTime, computeSubtitleBox(v, canvas))
}

// SUP 按 16:9 规格制作：渲染框锁定为视频内容矩形内的最大 16:9 区域，其余保持透明
function computeSubtitleBox(v, canvas) {
  const cw = Math.round(v.clientWidth)
  const ch = Math.round(v.clientHeight)
  if (!cw || !ch) return null
  if (canvas.width !== cw || canvas.height !== ch) { canvas.width = cw; canvas.height = ch }
  const vw = v.videoWidth || 16
  const vh = v.videoHeight || 9
  const scale = Math.min(cw / vw, ch / vh)
  const dw = vw * scale
  const dh = vh * scale
  const dx = (cw - dw) / 2
  const dy = (ch - dh) / 2
  let bw = dw
  let bh = dw * 9 / 16
  if (bh > dh) { bh = dh; bw = dh * 16 / 9 }
  return { x: dx + (dw - bw) / 2, y: dy + (dh - bh) / 2, w: bw, h: bh }
}

function onLocalSubtitlePicked(event) {
  const picked = event.target.files && event.target.files[0]
  event.target.value = ''
  if (!picked) return
  const ext = extensionOf(picked.name)
  const label = picked.name + '（本地）'
  if (ext === 'sup') {
    supTracks.value = [...supTracks.value, { label, url: URL.createObjectURL(picked) }]
    selectSup(supTracks.value.length - 1)
    return
  }
  if (ext === 'ass' || ext === 'ssa') {
    const reader = new FileReader()
    reader.onload = () => {
      assTracks.value = [...assTracks.value, { label, content: String(reader.result || '') }]
      selectAss(assTracks.value.length - 1)
    }
    reader.readAsText(picked)
    return
  }
  const reader = new FileReader()
  reader.onload = () => {
    const text = String(reader.result || '')
    const vtt = srtToVtt(text)
    const url = textBlobUrl(vtt)
    const entry = { url, language: 'und', label }
    extraTextSubs.value = [...extraTextSubs.value, entry]
    subtitleSources.value = [...subtitleSources.value, entry]
    nextTick(() => {
      const v = videoEl.value
      if (v && v.textTracks.length > 0) selectSubtitle(v.textTracks.length - 1)
    })
  }
  reader.readAsText(picked)
}

function setSubtitleScale(value) {
  subtitleScale.value = Math.max(0.8, Math.min(1.5, Number(value) || 1))
}

function setSubtitlePosition(value) {
  subtitlePosition.value = value === 'top' ? 'top' : 'bottom'
  const video = videoEl.value
  if (!video) return
  for (let index = 0; index < video.textTracks.length; index++) {
    const cues = video.textTracks[index].cues
    if (!cues) continue
    for (let cueIndex = 0; cueIndex < cues.length; cueIndex++) {
      try { cues[cueIndex].line = subtitlePosition.value === 'top' ? 10 : 90 } catch {}
    }
  }
}

function selectEpisode(file) {
  if (!file || file.file_id === props.file?.file_id) {
    activeMenu.value = ''
    return
  }
  activeMenu.value = ''
  emit('select-file', file)
}

// ---- popover menus ----
function toggleMenu(name) {
  activeMenu.value = activeMenu.value === name ? '' : name
  if (activeMenu.value) showControls.value = true
}

function closeMenu() {
  activeMenu.value = ''
}

// ---- save cursor ----
function saveCursor(fileId = props.file?.file_id) {
  if (playbackEnded) {
    clearPlayCursor(fileId)
    return
  }
  const v = videoEl.value
  if (!v || !v.currentTime || v.currentTime < 1) return
  if (!fileId) return
  savePlayCursor(props.account.user_id, props.account.drive_id, fileId, v.currentTime).catch(() => {})
}

function clearPlayCursor(fileId = props.file?.file_id) {
  if (!fileId) return
  savePlayCursor(props.account.user_id, props.account.drive_id, fileId, 0).catch(() => {})
}

async function switchQuality(quality) {
  if (!quality || quality === currentQuality.value) return
  const v = videoEl.value
  const wasPlaying = Boolean(v && !v.paused)
  const currentTime = v ? v.currentTime : 0
  const seq = playbackSeq
  try {
    const preview = await playVideoQuality(props.account.user_id, props.account.drive_id, props.file.file_id, quality)
    if (unmounted || seq !== playbackSeq) return
    setQualityOptions(preview)
    error.value = ''
    await loadPlaybackSource(preview, currentTime, wasPlaying, seq)
  } catch (e) {
    emit('toast', readablePlaybackError(e), 'error')
  }
}

// ---- keyboard ----
function onKeyDown(e) {
  if (e.target && /^(INPUT|TEXTAREA|SELECT|BUTTON)$/.test(e.target.tagName)) return
  switch (e.code) {
    case 'Space': e.preventDefault(); togglePlay(); break
    case 'ArrowLeft': e.preventDefault(); seek(-seekStep); break
    case 'ArrowRight': e.preventDefault(); seek(seekStep); break
    case 'ArrowUp': e.preventDefault(); adjustVolume(5); break
    case 'ArrowDown': e.preventDefault(); adjustVolume(-5); break
    case 'KeyF': toggleFullscreen(); break
    case 'KeyM': toggleMute(); break
    case 'KeyP': togglePip(); break
    case 'KeyL': toggleLoop(); break
    case 'KeyS': screenshot(); break
    case 'KeyC': toggleSubtitle(); break
    case 'Escape':
      if (activeMenu.value) activeMenu.value = ''
      else if (isFullscreen.value) toggleFullscreen()
      else if (!isFullscreen.value) emit('close')
      break
  }
  showControls.value = true
  scheduleHideControls()
}

function adjustVolume(delta) {
  if (!videoEl.value) return
  volume.value = Math.max(0, Math.min(200, volume.value + delta))
  applyVolume()
  showOsd(muted.value || volume.value === 0 ? 'volume-x' : 'volume', `音量 ${muted.value ? 0 : volume.value}%`, (muted.value ? 0 : volume.value) / 2)
}

function adjustBrightness(delta) {
  brightness.value = Math.max(20, Math.min(200, brightness.value + delta))
  showOsd('sun', `亮度 ${brightness.value}%`, brightness.value / 2)
}

// ---- auto-hide controls ----
function scheduleHideControls() {
  if (controlsTimer) clearTimeout(controlsTimer)
  if (!playing.value) return
  controlsTimer = setTimeout(() => {
    if (!activeMenu.value) showControls.value = false
  }, 2600)
}

function onMouseMove() {
  showControls.value = true
  scheduleHideControls()
}

function onMouseLeave() {
  if (playing.value && !activeMenu.value) showControls.value = false
}

function onProgressPointerMove(event) {
  const element = progressEl.value
  if (!element || !duration.value) return
  const rect = element.getBoundingClientRect()
  const ratio = Math.max(0, Math.min(1, (event.clientX - rect.left) / rect.width))
  scrubX.value = ratio * 100
  scrubTime.value = ratio * duration.value
  scrubVisible.value = true
}

function onProgressPointerLeave() { scrubVisible.value = false }

function onWheel(event) {
  if (event.target?.closest?.('.pp-pop')) return
  if (Math.abs(event.deltaY) < 1) return
  const el = containerEl.value
  const half = el ? el.getBoundingClientRect().width / 2 : window.innerWidth / 2
  const step = event.deltaY < 0 ? 5 : -5
  if (event.clientX < half) adjustBrightness(step)
  else adjustVolume(step)
}

function onDocumentPointerDown(event) {
  if (activeMenu.value && !event.target.closest('.pp-menu-root')) {
    activeMenu.value = ''
  }
}

function readablePlaybackError(errorValue) {
  const message = String(errorValue && errorValue.message || errorValue || '')
  return message || '视频加载失败，请重试'
}

function fmtTime(seconds) {
  if (!seconds || !Number.isFinite(seconds)) return '00:00'
  const h = Math.floor(seconds / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  const s = Math.floor(seconds % 60)
  if (h > 0) return `${h}:${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`
  return String(m).padStart(2, '0') + ':' + String(s).padStart(2, '0')
}

const pct = computed(() => duration.value > 0 ? Math.min(100, (position.value / duration.value) * 100) : 0)
const bufPct = computed(() => duration.value > 0 ? Math.min(100, (buffered.value / duration.value) * 100) : 0)
</script>

<template>
  <teleport to="body">
    <div
      ref="containerEl"
      class="player-panel"
      :class="{ 'cursor-hidden': !showControls && playing, fullscreen: isFullscreen, [`fs-anim-${fsAnim}`]: !!fsAnim }"
      :style="{ '--subtitle-scale': subtitleScale }"
      @mousemove="onMouseMove"
      @mouseleave="onMouseLeave"
      @wheel="onWheel"
    >
      <div class="pp-stage">
        <video
          ref="videoEl"
          v-show="!loading && !error"
          :src="src"
          class="pp-video"
          :style="{ filter: 'brightness(' + brightness / 100 + ')' }"
          @loadedmetadata="onLoaded"
          @loadeddata="onLoadedData"
          @durationchange="onDurationChange"
          @timeupdate="onTimeUpdate"
          @progress="onProgress"
          @play="onPlay"
          @pause="onPause"
          @ended="onEnded"
          @error="onError"
          @volumechange="onVolumeChange"
          @enterpictureinpicture="onPipEnter"
          @leavepictureinpicture="onPipLeave"
          @webkitpresentationmodechanged="onWebkitPresentationModeChange"
          @webkitbeginfullscreen="onWebkitFullscreenEnter"
          @webkitendfullscreen="onWebkitFullscreenLeave"
          @click="togglePlay"
          @dblclick="toggleFullscreen"
          preload="metadata"
          crossorigin="anonymous"
          playsinline
        >
          <track
            v-for="track in subtitleSources"
            :key="track.url"
            kind="subtitles"
            :src="track.url"
            :srclang="track.language"
            :label="track.label"
            @load="onTracksChange"
            @error="onTracksChange"
          />
        </video>
        <canvas v-show="supActive" ref="supCanvasEl" class="pp-sup-canvas"></canvas>
        <div v-if="loading" class="pp-state"><span class="pp-spinner"></span></div>
        <div v-else-if="error" class="pp-state pp-error">
          <UiIcon name="warning" :size="28" />
          <span class="pp-error-text">{{ error }}</span>
          <button class="pp-retry" type="button" @click="startPlayback"><UiIcon name="refresh" :size="14" />重新加载</button>
        </div>
        <transition name="pp-osd">
          <div v-if="osdVisible" class="pp-osd">
            <UiIcon :name="osdIcon" :size="17" />
            <span class="pp-osd-text">{{ osdText }}</span>
            <div class="pp-osd-bar"><i :style="{ width: osdPct + '%' }"></i></div>
          </div>
        </transition>
        <transition name="pp-center-fade">
          <button
            v-if="!loading && !error && (!playing || centerPulse)"
            type="button"
            class="pp-center"
            :title="playing ? '暂停 (空格)' : '播放 (空格)'"
            @click="togglePlay"
          ><UiIcon :name="playing ? 'pause' : 'play'" :size="32" /></button>
        </transition>
      </div>

      <header class="pp-topbar" :class="{ hidden: !showControls && playing }">
        <div class="pp-file-meta pp-window-drag">
          <div class="pp-title" :title="file.name">{{ file.name }}</div>
          <div class="pp-sub">{{ currentQualityLabel }}<span v-if="episodeFiles.length > 1"> · 第 {{ episodeIndex + 1 }} 集 / 共 {{ episodeFiles.length }} 集</span></div>
        </div>
        <div class="pp-top-actions">
          <button type="button" class="pp-btn" title="截图 (S)" @click="screenshot"><UiIcon name="camera" :size="19" /></button>
          <button type="button" class="pp-btn" :class="{ active: pipActive }" title="画中画 (P)" @click="togglePip"><UiIcon name="picture-in-picture" :size="20" /></button>
          <button type="button" class="pp-btn" title="关闭 (Esc)" @click="emit('close')"><UiIcon name="close" :size="20" /></button>
        </div>
      </header>

      <section v-if="!loading && !error" class="pp-bottom" :class="{ hidden: !showControls && playing }">
        <div
          ref="progressEl"
          class="pp-progress"
          :style="{ '--played': pct + '%', '--buffered': bufPct + '%' }"
          @pointermove="onProgressPointerMove"
          @pointerleave="onProgressPointerLeave"
        >
          <div class="pp-progress-buffer"></div>
          <div class="pp-progress-fill"><span class="pp-progress-thumb"></span></div>
          <div v-if="scrubVisible" class="pp-scrub" :style="{ left: scrubX + '%' }">
            <img v-if="thumbnailUrl" :src="thumbnailUrl" alt="" />
            <span>{{ fmtTime(scrubTime) }}</span>
          </div>
          <input type="range" class="pp-progress-input" min="0" :max="duration || 0" step="0.1" :value="position" aria-label="播放进度" @input="onSeekInput" />
        </div>

        <div class="pp-controls">
          <div class="pp-group">
            <button type="button" class="pp-btn pp-skip" :title="`快退 ${seekStep}s (←)`" @click="seek(-seekStep)"><UiIcon name="rewind" :size="21" /></button>
            <button type="button" class="pp-btn pp-play-main" :title="playing ? '暂停 (空格)' : '播放 (空格)'" @click="togglePlay"><UiIcon :name="playing ? 'pause' : 'play'" :size="24" /></button>
            <button type="button" class="pp-btn pp-skip" :title="`快进 ${seekStep}s (→)`" @click="seek(seekStep)"><UiIcon name="fast-forward" :size="21" /></button>
            <div class="pp-vol">
              <button type="button" class="pp-btn" :title="muted ? '取消静音 (M)' : '静音 (M)'" @click="toggleMute"><UiIcon :name="muted || volume === 0 ? 'volume-x' : 'volume'" :size="20" /></button>
              <div class="pp-vol-slider">
                <input type="range" min="0" max="200" :value="muted ? 0 : volume" :style="{ '--vol-fill': (muted ? 0 : volume) / 2 + '%' }" aria-label="音量" @input="onVolume" />
                <span class="pp-vol-value">{{ muted ? 0 : volume }}%</span>
              </div>
            </div>
            <span class="pp-time">{{ fmtTime(position) }}<i>/</i>{{ fmtTime(duration) }}</span>
          </div>

          <div class="pp-group pp-right">
            <div class="pp-menu-root">
              <button type="button" class="pp-btn pp-text-btn" :class="{ active: activeMenu === 'speed' }" title="播放速度" @click.stop="toggleMenu('speed')">{{ speed }}x</button>
              <div v-if="activeMenu === 'speed'" class="pp-pop">
                <div class="pp-pop-title">播放速度</div>
                <button v-for="item in speedOptions" :key="item.value" type="button" class="pp-pop-item" :class="{ on: item.value === speed }" @click="onSpeed(item.value); closeMenu()">
                  <span class="pp-pop-check"><UiIcon v-if="item.value === speed" name="check" :size="14" /></span>{{ item.label }}
                </button>
              </div>
            </div>

            <div class="pp-menu-root">
              <button type="button" class="pp-btn" :class="{ active: activeMenu === 'subtitle' || subtitleEnabled || supActive }" title="字幕 (C)" @click.stop="toggleMenu('subtitle')"><UiIcon name="captions" :size="20" /></button>
              <div v-if="activeMenu === 'subtitle'" class="pp-pop">
                <div class="pp-pop-title">字幕</div>
                <button type="button" class="pp-pop-item" :class="{ on: !subtitleEnabled && !supActive }" @click="onSubtitleSelected('off'); closeMenu()">
                  <span class="pp-pop-check"><UiIcon v-if="!subtitleEnabled && !supActive" name="check" :size="14" /></span>关闭
                </button>
                <button v-for="track in subtitleTracks" :key="track.index" type="button" class="pp-pop-item" :class="{ on: subtitleEnabled && currentSubtitle === String(track.index) }" @click="onSubtitleSelected(String(track.index)); closeMenu()">
                  <span class="pp-pop-check"><UiIcon v-if="subtitleEnabled && currentSubtitle === String(track.index)" name="check" :size="14" /></span><span class="pp-pop-item-label">{{ track.label }}</span>
                </button>
                <button v-for="(track, index) in supTracks" :key="'sup-' + index" type="button" class="pp-pop-item" :class="{ on: supActive && currentSubtitle === 'sup:' + index }" @click="selectSup(index); closeMenu()">
                  <span class="pp-pop-check"><UiIcon v-if="supActive && currentSubtitle === 'sup:' + index" name="check" :size="14" /></span><span class="pp-pop-item-label">{{ track.label }}</span><span class="pp-pop-tag">SUP</span>
                </button>
                <button v-for="(track, index) in assTracks" :key="'ass-' + index" type="button" class="pp-pop-item" :class="{ on: assActive && currentSubtitle === 'ass:' + index }" @click="selectAss(index); closeMenu()">
                  <span class="pp-pop-check"><UiIcon v-if="assActive && currentSubtitle === 'ass:' + index" name="check" :size="14" /></span><span class="pp-pop-item-label">{{ track.label }}</span><span class="pp-pop-tag">ASS</span>
                </button>
                <div v-if="!subtitleTracks.length && !supTracks.length && !assTracks.length" class="pp-pop-empty">无可用字幕</div>
                <div class="pp-pop-divider"></div>
                <button type="button" class="pp-pop-item" @click="localSubInput && localSubInput.click(); closeMenu()">
                  <span class="pp-pop-check"></span><UiIcon name="upload" :size="14" />加载本地字幕…
                </button>
                <template v-if="subtitleTracks.length">
                  <div class="pp-pop-divider"></div>
                  <div class="pp-pop-tools">
                    <span class="pp-pop-tools-label">大小</span>
                    <button type="button" class="pp-tool" @click="setSubtitleScale(subtitleScale - .1)">−</button>
                    <span class="pp-tool-value">{{ Math.round(subtitleScale * 100) }}%</span>
                    <button type="button" class="pp-tool" @click="setSubtitleScale(subtitleScale + .1)">＋</button>
                    <span class="pp-pop-tools-label">位置</span>
                    <button type="button" class="pp-tool pp-tool-wide" @click="setSubtitlePosition(subtitlePosition === 'top' ? 'bottom' : 'top')">{{ subtitlePosition === 'top' ? '顶部' : '底部' }}</button>
                  </div>
                </template>
              </div>
            </div>

            <div class="pp-menu-root">
              <button type="button" class="pp-btn" :class="{ active: activeMenu === 'episodes' }" title="播放列表" @click.stop="toggleMenu('episodes')"><UiIcon name="list" :size="20" /></button>
              <div v-if="activeMenu === 'episodes'" class="pp-pop pp-pop-list">
                <div class="pp-pop-title">播放列表 <span class="pp-pop-count">{{ episodeIndex + 1 }}/{{ episodeFiles.length }}</span></div>
                <button v-for="(episode, index) in episodeFiles" :key="episode.file_id" type="button" class="pp-pop-item pp-episode" :class="{ on: episode.file_id === file.file_id }" :title="episode.name" @click="selectEpisode(episode)">
                  <span class="pp-episode-no">{{ index + 1 }}</span>
                  <span class="pp-episode-name">{{ episode.name }}</span>
                  <UiIcon v-if="episode.file_id === file.file_id" name="play" :size="11" />
                </button>
              </div>
            </div>

            <div v-if="qualities.length > 1" class="pp-menu-root">
              <button type="button" class="pp-btn pp-text-btn" :class="{ active: activeMenu === 'quality' }" title="清晰度" @click.stop="toggleMenu('quality')">{{ currentQualityLabel }}</button>
              <div v-if="activeMenu === 'quality'" class="pp-pop">
                <div class="pp-pop-title">清晰度</div>
                <button v-for="quality in qualities" :key="quality.value" type="button" class="pp-pop-item" :class="{ on: quality.value === currentQuality }" @click="switchQuality(quality.value); closeMenu()">
                  <span class="pp-pop-check"><UiIcon v-if="quality.value === currentQuality" name="check" :size="14" /></span>{{ quality.label }}
                </button>
              </div>
            </div>

            <button type="button" class="pp-btn" :class="{ active: looping }" title="循环播放 (L)" @click="toggleLoop"><UiIcon name="refresh" :size="18" /></button>
            <button type="button" class="pp-btn" :title="isFullscreen ? '退出全屏 (F)' : '全屏 (F)'" @click="toggleFullscreen"><UiIcon :name="isFullscreen ? 'minimize' : 'maximize'" :size="20" /></button>
          </div>
        </div>
      </section>
      <input ref="localSubInput" type="file" accept=".srt,.vtt,.ass,.ssa,.sup" class="pp-hidden-input" @change="onLocalSubtitlePicked" />
    </div>
  </teleport>
</template>

<style scoped>
/* —— Netflix / Apple TV 沉浸路线：渐变遮罩、细线进度、玻璃二级菜单 —— */
.player-panel {
  --pp-white: rgba(255, 255, 255, .92);
  --pp-dim: rgba(255, 255, 255, .55);
  --pp-glass: rgba(16, 16, 16, .78);
  --pp-hover: rgba(255, 255, 255, .12);
  --subtitle-scale: 1;
  position: fixed;
  z-index: 520;
  inset: 0;
  overflow: hidden;
  isolation: isolate;
  background: #000;
  color: #fff;
  font-family: inherit;
  letter-spacing: 0;
  user-select: none;
}
.player-panel.cursor-hidden { cursor: none; }

.pp-stage {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  background: #000;
}
.pp-video { width: 100%; height: 100%; object-fit: contain; }
.player-panel video::cue {
  color: #fff;
  background: rgba(0, 0, 0, .55);
  text-shadow: 0 1px 4px rgba(0, 0, 0, .9);
  font-size: calc(1em * var(--subtitle-scale));
}

/* 全屏切换过渡：进入由小放大，退出由大收小 */
.player-panel.fs-anim-in .pp-stage { animation: pp-fs-in 340ms cubic-bezier(.22, .9, .3, 1) both; }
.player-panel.fs-anim-out .pp-stage { animation: pp-fs-out 340ms cubic-bezier(.22, .9, .3, 1) both; }
@keyframes pp-fs-in { from { opacity: .25; transform: scale(.93); } to { opacity: 1; transform: scale(1); } }
@keyframes pp-fs-out { from { opacity: .25; transform: scale(1.06); } to { opacity: 1; transform: scale(1); } }

/* SUP 字幕覆盖层：与视频同区，16:9 锁定由 JS 计算 */
.pp-sup-canvas {
  position: absolute;
  z-index: 1;
  inset: 0;
  width: 100%;
  height: 100%;
  pointer-events: none;
}
.pp-hidden-input { display: none; }
.pp-pop-item-label { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.pp-pop-tag {
  flex-shrink: 0;
  padding: 1px 6px;
  border-radius: 5px;
  color: rgba(255, 255, 255, .75);
  background: rgba(255, 255, 255, .14);
  font-size: 10px;
  font-weight: 700;
  letter-spacing: .05em;
}
.pp-pop-empty { padding: 8px 10px; color: var(--pp-dim); font-size: 12px; }

/* 状态层 */
.pp-state {
  position: absolute;
  z-index: 2;
  inset: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 16px;
  padding: 24px;
  color: var(--pp-dim);
  font-size: 14px;
  text-align: center;
}
.pp-spinner {
  width: 34px;
  height: 34px;
  border: 2.5px solid rgba(255, 255, 255, .18);
  border-top-color: #fff;
  border-radius: 50%;
  animation: pp-spin 800ms linear infinite;
}
@keyframes pp-spin { to { transform: rotate(360deg); } }
.pp-error { color: rgba(255, 130, 130, .95); }
.pp-error-text { max-width: 480px; line-height: 1.6; }
.pp-retry {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  height: 34px;
  padding: 0 16px;
  border: 1px solid rgba(255, 255, 255, .28);
  border-radius: 999px;
  color: #fff;
  background: transparent;
  font: inherit;
  font-size: 13px;
  cursor: pointer;
  transition: background .16s ease, border-color .16s ease;
}
.pp-retry:hover { background: rgba(255, 255, 255, .12); border-color: rgba(255, 255, 255, .5); }

/* 悬浮大按钮 */
.pp-center {
  position: absolute;
  z-index: 2;
  left: 50%;
  top: 50%;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 78px;
  height: 78px;
  padding: 0 0 0 4px;
  transform: translate(-50%, -50%);
  border: 2px solid rgba(255, 255, 255, .9);
  border-radius: 50%;
  color: #fff;
  background: rgba(0, 0, 0, .6);
  backdrop-filter: blur(10px);
  -webkit-backdrop-filter: blur(10px);
  cursor: pointer;
  box-shadow: 0 12px 40px rgba(0, 0, 0, .5);
  transition: transform .18s ease, background .18s ease;
}
.pp-center:hover { transform: translate(-50%, -50%) scale(1.07); background: rgba(0, 0, 0, .72); }
.pp-center-fade-enter-active, .pp-center-fade-leave-active { transition: opacity .22s ease; }
.pp-center-fade-enter-from, .pp-center-fade-leave-to { opacity: 0; }

/* 顶栏 / 底栏：纯渐变遮罩，无实体条 */
.pp-topbar, .pp-bottom {
  position: absolute;
  z-index: 3;
  left: 0;
  right: 0;
  display: flex;
  transition: opacity .3s ease, transform .3s ease;
}
.pp-topbar {
  top: 0;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  padding: 20px 24px 48px;
  background: linear-gradient(180deg, rgba(0, 0, 0, .68), rgba(0, 0, 0, .28) 55%, transparent);
  --wails-draggable: drag;
}
.pp-window-drag { --wails-draggable: drag; }
.pp-topbar button, .pp-bottom, .pp-bottom button, .pp-bottom input, .pp-pop { --wails-draggable: no-drag; }
.pp-topbar.hidden { opacity: 0; transform: translateY(-12px); pointer-events: none; }
.pp-bottom.hidden { opacity: 0; transform: translateY(12px); pointer-events: none; }

.pp-file-meta { min-width: 0; flex: 1; padding-top: 2px; }
.pp-title {
  overflow: hidden;
  color: var(--pp-white);
  font-size: 15.5px;
  font-weight: 600;
  line-height: 1.4;
  text-overflow: ellipsis;
  white-space: nowrap;
  text-shadow: 0 1px 8px rgba(0, 0, 0, .6);
}
.pp-sub { margin-top: 2px; color: var(--pp-dim); font-size: 12px; }
.pp-top-actions { display: flex; align-items: center; gap: 2px; flex-shrink: 0; }

/* 通用图标按钮 */
.pp-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 40px;
  height: 40px;
  margin: 0;
  padding: 0;
  border: 0;
  border-radius: 50%;
  color: rgba(255, 255, 255, .82);
  background: transparent;
  cursor: pointer;
  flex-shrink: 0;
  transition: color .15s ease, background .15s ease, transform .15s ease;
}
.pp-btn:hover { color: #fff; background: var(--pp-hover); }
.pp-btn:active { transform: scale(.92); }
.pp-btn.active { color: #fff; background: rgba(255, 255, 255, .2); }
.pp-text-btn {
  width: auto;
  min-width: 40px;
  padding: 0 10px;
  border-radius: 999px;
  font-size: 13px;
  font-weight: 600;
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}

/* 底栏 */
.pp-bottom {
  bottom: 0;
  flex-direction: column;
  align-items: stretch;
  padding: 48px 24px 14px;
  background: linear-gradient(0deg, rgba(0, 0, 0, .78), rgba(0, 0, 0, .34) 55%, transparent);
}

/* 细线进度条 */
.pp-progress { position: relative; height: 16px; cursor: pointer; }
.pp-progress::before, .pp-progress-buffer, .pp-progress-fill {
  position: absolute;
  top: 50%;
  right: 0;
  left: 0;
  height: 3px;
  transform: translateY(-50%);
  border-radius: 999px;
  transition: height .14s ease;
}
.pp-progress::before { content: ''; background: rgba(255, 255, 255, .22); }
.pp-progress-buffer { right: auto; width: var(--buffered); background: rgba(255, 255, 255, .38); }
.pp-progress-fill { right: auto; width: var(--played); background: #fff; }
.pp-progress:hover::before, .pp-progress:hover .pp-progress-buffer, .pp-progress:hover .pp-progress-fill { height: 5px; }
.pp-progress-thumb {
  position: absolute;
  top: 50%;
  right: -6px;
  width: 12px;
  height: 12px;
  transform: translateY(-50%) scale(0);
  border-radius: 50%;
  background: #fff;
  box-shadow: 0 1px 6px rgba(0, 0, 0, .5);
  transition: transform .14s ease;
}
.pp-progress:hover .pp-progress-thumb { transform: translateY(-50%) scale(1); }
.pp-progress-input { position: absolute; z-index: 2; inset: 0; width: 100%; height: 100%; margin: 0; opacity: 0; cursor: pointer; }
.pp-scrub {
  position: absolute;
  z-index: 5;
  bottom: 20px;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 5px;
  padding: 5px;
  transform: translateX(-50%);
  border: 1px solid rgba(255, 255, 255, .16);
  border-radius: 10px;
  background: var(--pp-glass);
  backdrop-filter: blur(20px);
  -webkit-backdrop-filter: blur(20px);
  box-shadow: 0 14px 36px rgba(0, 0, 0, .5);
  pointer-events: none;
}
.pp-scrub img { display: block; width: 148px; height: 83px; object-fit: cover; border-radius: 6px; }
.pp-scrub span { color: #fff; font-size: 11.5px; font-weight: 600; font-variant-numeric: tabular-nums; }

/* 控制行 */
.pp-controls { display: flex; align-items: center; justify-content: space-between; gap: 16px; min-width: 0; }
.pp-group { display: flex; align-items: center; gap: 2px; min-width: 0; }
.pp-right { justify-content: flex-end; gap: 4px; }
.pp-play-main { width: 46px; height: 46px; color: #fff; }
.pp-play-main:hover { transform: scale(1.08); background: var(--pp-hover); }
.pp-time {
  margin-left: 10px;
  color: rgba(255, 255, 255, .78);
  font-size: 12.5px;
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
  text-shadow: 0 1px 4px rgba(0, 0, 0, .5);
}
.pp-time i { margin: 0 6px; color: rgba(255, 255, 255, .32); font-style: normal; }

/* 音量：悬停展开横向滑杆 */
.pp-vol { display: flex; align-items: center; }
.pp-vol-slider {
  width: 0;
  overflow: hidden;
  display: flex;
  align-items: center;
  transition: width .2s ease;
}
.pp-vol:hover .pp-vol-slider, .pp-vol-slider:focus-within { width: 132px; }
.pp-vol-slider input {
  width: 84px;
  margin-left: 2px;
  height: 3px;
  appearance: none;
  -webkit-appearance: none;
  border-radius: 999px;
  background: linear-gradient(90deg, #fff var(--vol-fill, 50%), rgba(255, 255, 255, .28) var(--vol-fill, 50%));
  cursor: pointer;
}
.pp-vol-slider input::-webkit-slider-thumb {
  width: 12px;
  height: 12px;
  appearance: none;
  -webkit-appearance: none;
  border: 0;
  border-radius: 50%;
  background: #fff;
  box-shadow: 0 1px 5px rgba(0, 0, 0, .5);
}
.pp-vol-value {
  margin-left: 8px;
  min-width: 36px;
  color: rgba(255, 255, 255, .85);
  font-size: 12px;
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}

/* 屏幕提示药丸 */
.pp-osd {
  position: absolute;
  z-index: 4;
  top: 84px;
  left: 50%;
  display: flex;
  align-items: center;
  gap: 9px;
  height: 38px;
  padding: 0 14px;
  transform: translateX(-50%);
  border: 1px solid rgba(255, 255, 255, .12);
  border-radius: 999px;
  color: #fff;
  background: rgba(0, 0, 0, .6);
  backdrop-filter: blur(16px);
  -webkit-backdrop-filter: blur(16px);
  box-shadow: 0 10px 30px rgba(0, 0, 0, .45);
  font-size: 12.5px;
  font-weight: 600;
  font-variant-numeric: tabular-nums;
  pointer-events: none;
}
.pp-osd-text { white-space: nowrap; }
.pp-osd-bar {
  width: 64px;
  height: 3px;
  overflow: hidden;
  border-radius: 999px;
  background: rgba(255, 255, 255, .25);
}
.pp-osd-bar i { display: block; height: 100%; border-radius: 999px; background: #fff; transition: width .1s ease; }
.pp-osd-enter-active, .pp-osd-leave-active { transition: opacity .22s ease, transform .22s ease; }
.pp-osd-enter-from, .pp-osd-leave-to { opacity: 0; transform: translateX(-50%) translateY(-6px); }

/* 二级弹出菜单：玻璃拟态 */
.pp-menu-root { position: relative; }
.pp-pop {
  position: absolute;
  z-index: 20;
  right: 0;
  bottom: calc(100% + 14px);
  min-width: 208px;
  max-width: 320px;
  max-height: min(46vh, 380px);
  overflow-y: auto;
  padding: 8px;
  border: 1px solid rgba(255, 255, 255, .1);
  border-radius: 14px;
  background: var(--pp-glass);
  backdrop-filter: blur(28px) saturate(150%);
  -webkit-backdrop-filter: blur(28px) saturate(150%);
  box-shadow: 0 20px 56px rgba(0, 0, 0, .6);
  animation: pp-pop-in .18s cubic-bezier(.2, .9, .3, 1.2);
  scrollbar-width: thin;
  scrollbar-color: rgba(255, 255, 255, .25) transparent;
}
.pp-pop::-webkit-scrollbar { width: 5px; }
.pp-pop::-webkit-scrollbar-thumb { border-radius: 999px; background: rgba(255, 255, 255, .22); }
@keyframes pp-pop-in { from { opacity: 0; transform: translateY(8px) scale(.96); } to { opacity: 1; transform: none; } }
.pp-pop-title {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 6px 10px 8px;
  color: var(--pp-dim);
  font-size: 11.5px;
  font-weight: 600;
  letter-spacing: .04em;
}
.pp-pop-count { font-variant-numeric: tabular-nums; }
.pp-pop-item {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  min-height: 36px;
  padding: 0 10px;
  border: 0;
  border-radius: 9px;
  color: rgba(255, 255, 255, .85);
  background: transparent;
  font: inherit;
  font-size: 13px;
  text-align: left;
  cursor: pointer;
  transition: background .13s ease;
}
.pp-pop-item:hover { background: var(--pp-hover); color: #fff; }
.pp-pop-item.on { color: #fff; font-weight: 600; }
.pp-pop-check { display: inline-flex; width: 16px; flex-shrink: 0; color: #fff; }
.pp-pop-divider { height: 1px; margin: 6px 4px; background: rgba(255, 255, 255, .1); }
.pp-pop-list { width: 300px; }
.pp-episode { gap: 10px; }
.pp-episode-no {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 22px;
  height: 22px;
  padding: 0 4px;
  border-radius: 6px;
  color: var(--pp-dim);
  background: rgba(255, 255, 255, .08);
  font-size: 11px;
  font-variant-numeric: tabular-nums;
  flex-shrink: 0;
}
.pp-pop-item.on .pp-episode-no { color: #000; background: #fff; }
.pp-episode-name { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

/* 字幕工具行 */
.pp-pop-tools { display: flex; align-items: center; gap: 6px; padding: 4px 10px 6px; }
.pp-pop-tools-label { color: var(--pp-dim); font-size: 11.5px; }
.pp-tool {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 28px;
  height: 26px;
  padding: 0 6px;
  border: 1px solid rgba(255, 255, 255, .16);
  border-radius: 7px;
  color: rgba(255, 255, 255, .85);
  background: rgba(255, 255, 255, .06);
  font: inherit;
  font-size: 12px;
  cursor: pointer;
  transition: background .13s ease;
}
.pp-tool:hover { background: rgba(255, 255, 255, .16); color: #fff; }
.pp-tool-wide { padding: 0 10px; margin-left: 2px; }
.pp-tool-value { min-width: 38px; color: rgba(255, 255, 255, .7); font-size: 11.5px; text-align: center; font-variant-numeric: tabular-nums; }

/* 响应式 */
@media (max-width: 760px) {
  .pp-topbar { padding: 14px 14px 40px; }
  .pp-bottom { padding: 40px 14px 10px; }
  .pp-skip, .pp-vol-slider { display: none; }
  .pp-scrub img { width: 112px; height: 63px; }
  .pp-pop-list { width: min(300px, calc(100vw - 28px)); }
}
@media (max-width: 560px) {
  .pp-sub, .pp-text-btn { display: none; }
  .pp-time { margin-left: 6px; font-size: 11.5px; }
  .pp-btn { width: 36px; height: 36px; }
  .pp-play-main { width: 42px; height: 42px; }
  .pp-center { width: 66px; height: 66px; }
  .pp-title { font-size: 14px; }
}
</style>
