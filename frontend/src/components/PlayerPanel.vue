<script setup>
// 网页播放器只保留浏览器/WebView 可解码路径：原生 MP4/WebM/Ogg，按需加载
// HLS.js 和 dash.js 的 MSE 流。所有远程请求都经 Go 侧本地会话代理。
import { ref, computed, onMounted, onBeforeUnmount, nextTick, watch } from 'vue'
import { playVideo, playVideoQuality, pinFileSnapshot, getPlayCursor, savePlayCursor, getSettings } from '../api'
import { getPrefs } from '../appearance'
import UiIcon from './UiIcon.vue'

const props = defineProps({
  account: { type: Object, required: true },
  file: { type: Object, required: true },
  files: { type: Array, default: () => [] },
})
const emit = defineEmits(['close', 'toast', 'select-file'])

const videoEl = ref(null)
const containerEl = ref(null)
const moreMenuEl = ref(null)
const progressEl = ref(null)
const loading = ref(true)
const error = ref('')
const playing = ref(false)
const position = ref(0)
const duration = ref(0)
const buffered = ref(0)
const volume = ref(getPrefs().defaultVolume ?? 50)
const muted = ref(false)
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
const moreMenuOpen = ref(false)
const subtitleScale = ref(1)
const subtitlePosition = ref('bottom')
const scrubVisible = ref(false)
const scrubX = ref(0)
const scrubTime = ref(0)
const centerPulse = ref(false)

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
  document.removeEventListener('keydown', onKeyDown)
  document.removeEventListener('fullscreenchange', onFullscreenChange)
  document.removeEventListener('pointerdown', onDocumentPointerDown, true)
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
  subtitleSources.value = normalizeSubtitles(preview && preview.subtitles)
  subtitleTracks.value = []
  subtitleEnabled.value = false
  currentSubtitle.value = ''
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
  v.volume = volume.value / 100
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
  volume.value = Math.round(v.volume * 100)
  muted.value = v.muted
}
function onPipEnter() { pipActive.value = true }
function onPipLeave() { pipActive.value = false }

// ---- controls ----
function togglePlay() {
  const v = videoEl.value
  if (!v) return
  centerPulse.value = true
  if (centerTimer) clearTimeout(centerTimer)
  centerTimer = setTimeout(() => { centerPulse.value = false }, 2000)
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
  const value = Number(e.target.value)
  volume.value = value
  const v = videoEl.value
  if (v) {
    v.volume = value / 100
    if (value > 0) v.muted = false
  }
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
  if (subtitleEnabled.value) selectSubtitle(-1)
  else if (subtitleTracks.value.length > 0) selectSubtitle(0)
}

function onSubtitleSelected(value) {
  selectSubtitle(value === 'off' ? -1 : Number(value))
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
    moreMenuOpen.value = false
    return
  }
  moreMenuOpen.value = false
  emit('select-file', file)
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
    case 'ArrowUp': e.preventDefault(); adjustVolume(0.05); break
    case 'ArrowDown': e.preventDefault(); adjustVolume(-0.05); break
    case 'KeyF': toggleFullscreen(); break
    case 'KeyM': toggleMute(); break
    case 'KeyP': togglePip(); break
    case 'KeyL': toggleLoop(); break
    case 'KeyS': screenshot(); break
    case 'KeyC': toggleSubtitle(); break
    case 'Escape':
      if (moreMenuOpen.value) moreMenuOpen.value = false
      else if (isFullscreen.value) toggleFullscreen()
      else if (!isFullscreen.value) emit('close')
      break
  }
  showControls.value = true
  scheduleHideControls()
}

function adjustVolume(delta) {
  const v = videoEl.value
  if (!v) return
  v.volume = Math.max(0, Math.min(1, v.volume + delta))
  volume.value = Math.round(v.volume * 100)
}

// ---- auto-hide controls ----
function scheduleHideControls() {
  if (controlsTimer) clearTimeout(controlsTimer)
  if (!playing.value) return
  controlsTimer = setTimeout(() => {
    if (!moreMenuOpen.value) showControls.value = false
  }, 2000)
}

function onMouseMove() {
  showControls.value = true
  scheduleHideControls()
}

function onMouseLeave() {
  if (playing.value) showControls.value = false
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
  if (event.target?.closest?.('.pp-settings-panel')) return
  if (Math.abs(event.deltaY) < 1) return
  adjustVolume(event.deltaY < 0 ? 0.05 : -0.05)
}

function onDocumentPointerDown(event) {
  if (moreMenuOpen.value && moreMenuEl.value && !moreMenuEl.value.contains(event.target)) {
    moreMenuOpen.value = false
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
      :class="{ 'cursor-hidden': !showControls && playing, fullscreen: isFullscreen }"
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
        <div v-if="loading" class="pp-state pp-loading"><span class="spin"></span><span>正在准备播放…</span></div>
        <div v-else-if="error" class="pp-state pp-error">
          <UiIcon name="warning" :size="26" />
          <span>{{ error }}</span>
          <button class="pp-retry" type="button" @click="startPlayback"><UiIcon name="refresh" :size="15" />重新加载</button>
        </div>
        <button v-if="!loading && !error && (!playing || centerPulse)" type="button" class="pp-center-play" :title="playing ? '暂停 (空格)' : '播放 (空格)'" @click="togglePlay"><UiIcon :name="playing ? 'pause' : 'play'" :size="28" /></button>
      </div>

      <header class="pp-topbar" :class="{ hidden: !showControls && playing }">
        <div class="pp-file-meta pp-window-drag">
          <span class="pp-file-icon"><UiIcon name="video" :size="17" /></span>
          <div class="pp-file-copy">
            <div class="pp-title" :title="file.name">{{ file.name }}</div>
            <div class="pp-source-label">{{ currentQualityLabel }}<span v-if="episodeFiles.length > 1"> · {{ episodeIndex + 1 }}/{{ episodeFiles.length }}</span></div>
          </div>
        </div>
        <div class="pp-top-actions">
          <button type="button" class="pp-icon pp-wide-action" title="截图 (S)" @click="screenshot"><UiIcon name="camera" :size="17" /></button>
          <button v-if="subtitleTracks.length" type="button" class="pp-icon pp-wide-action" :class="{ active: subtitleEnabled }" title="字幕 (C)" @click="toggleSubtitle"><UiIcon name="captions" :size="18" /></button>
          <button type="button" class="pp-icon pp-wide-action" :class="{ active: pipActive }" title="画中画 (P)" @click="togglePip"><UiIcon name="picture-in-picture" :size="18" /></button>
          <button type="button" class="pp-icon" title="播放设置" @click.stop="moreMenuOpen = !moreMenuOpen"><UiIcon name="settings" :size="18" /></button>
          <button type="button" class="pp-icon pp-close" title="关闭 (Esc)" @click="emit('close')"><UiIcon name="close" :size="18" /></button>
        </div>
      </header>

      <section v-if="!loading && !error" class="pp-bottom" :class="{ hidden: !showControls && playing }">
        <div ref="progressEl" class="pp-progress" :style="{ '--played': pct + '%', '--buffered': bufPct + '%' }" @pointermove="onProgressPointerMove" @pointerleave="onProgressPointerLeave">
          <div class="pp-progress-buffer"></div>
          <div class="pp-progress-fill"></div>
          <div v-if="scrubVisible" class="pp-scrub-preview" :style="{ left: scrubX + '%' }">
            <img v-if="thumbnailUrl" :src="thumbnailUrl" alt="" />
            <span>{{ fmtTime(scrubTime) }}</span>
          </div>
          <input type="range" class="pp-progress-input" min="0" :max="duration || 0" step="0.1" :value="position" aria-label="播放进度" @input="onSeekInput" />
        </div>
        <div class="pp-control-row">
          <div class="pp-control-group pp-left-controls">
            <button type="button" class="pp-icon pp-skip" title="快退 (←)" @click="seek(-seekStep)"><UiIcon name="back" :size="17" /></button>
            <button type="button" class="pp-play" :title="playing ? '暂停 (空格)' : '播放 (空格)'" @click="togglePlay"><UiIcon :name="playing ? 'pause' : 'play'" :size="19" /></button>
            <button type="button" class="pp-icon pp-skip" title="快进 (→)" @click="seek(seekStep)"><UiIcon name="forward" :size="17" /></button>
            <span class="pp-time">{{ fmtTime(position) }}<i>/</i>{{ fmtTime(duration) }}</span>
          </div>
          <div class="pp-control-group pp-right-controls">
            <div class="pp-volume">
              <button type="button" class="pp-icon" :title="muted ? '取消静音 (M)' : '静音 (M)'" @click="toggleMute"><UiIcon :name="muted || volume === 0 ? 'volume-x' : 'volume'" :size="18" /></button>
              <input type="range" class="pp-vol-input" min="0" max="100" :value="muted ? 0 : volume" aria-label="音量" @input="onVolume" />
            </div>
            <button type="button" class="pp-icon" :class="{ active: moreMenuOpen }" title="播放设置" @click.stop="moreMenuOpen = !moreMenuOpen"><UiIcon name="more-horizontal" :size="19" /></button>
            <button type="button" class="pp-icon pp-loop-control" :class="{ active: looping }" title="循环 (L)" @click="toggleLoop"><UiIcon name="refresh" :size="17" /></button>
            <button type="button" class="pp-icon" :title="isFullscreen ? '退出全屏 (F)' : '全屏 (F)'" @click="toggleFullscreen"><UiIcon :name="isFullscreen ? 'minimize' : 'maximize'" :size="18" /></button>
          </div>
        </div>
        <div v-if="moreMenuOpen" ref="moreMenuEl" class="pp-settings-panel" @click.stop>
          <div class="pp-settings-head"><span>播放设置</span><button class="pp-panel-close" type="button" @click="moreMenuOpen = false"><UiIcon name="close" :size="15" /></button></div>
          <div v-if="episodeFiles.length > 1" class="pp-setting-block">
            <div class="pp-setting-label">选集 <span>{{ episodeIndex + 1 }}/{{ episodeFiles.length }}</span></div>
            <div class="pp-option-grid pp-episode-grid">
              <button v-for="(episode, index) in episodeFiles" :key="episode.file_id" type="button" class="pp-option" :class="{ active: episode.file_id === file.file_id }" :title="episode.name" @click="selectEpisode(episode)">{{ index + 1 }}</button>
            </div>
          </div>
          <div v-if="qualities.length > 1" class="pp-setting-block"><div class="pp-setting-label">清晰度</div><div class="pp-option-grid"><button v-for="quality in qualities" :key="quality.value" type="button" class="pp-option pp-option-text" :class="{ active: quality.value === currentQuality }" @click="switchQuality(quality.value)">{{ quality.label }}</button></div></div>
          <div class="pp-setting-block"><div class="pp-setting-label">倍速</div><div class="pp-option-grid"><button v-for="item in speedOptions" :key="item.value" type="button" class="pp-option pp-option-text" :class="{ active: item.value === speed }" @click="onSpeed(item.value)">{{ item.label }}</button></div></div>
          <div v-if="subtitleTracks.length" class="pp-setting-block"><div class="pp-setting-label">字幕</div><div class="pp-option-grid"><button type="button" class="pp-option pp-option-text" :class="{ active: !subtitleEnabled }" @click="onSubtitleSelected('off')">关闭</button><button v-for="track in subtitleTracks" :key="track.index" type="button" class="pp-option pp-option-text" :class="{ active: subtitleEnabled && currentSubtitle === String(track.index) }" @click="onSubtitleSelected(String(track.index))">{{ track.label }}</button></div><div class="pp-subtitle-tools"><button type="button" class="pp-mini-option" @click="setSubtitleScale(subtitleScale - .1)">字幕 -</button><span>{{ Math.round(subtitleScale * 100) }}%</span><button type="button" class="pp-mini-option" @click="setSubtitleScale(subtitleScale + .1)">字幕 +</button><button type="button" class="pp-mini-option" :class="{ active: subtitlePosition === 'top' }" @click="setSubtitlePosition(subtitlePosition === 'top' ? 'bottom' : 'top')">{{ subtitlePosition === 'top' ? '顶部' : '底部' }}</button></div></div>
          <div class="pp-setting-actions"><button type="button" class="pp-menu-action" :class="{ active: looping }" @click="toggleLoop"><UiIcon name="refresh" :size="15" /><span>循环播放</span></button><button type="button" class="pp-menu-action" @click="screenshot"><UiIcon name="camera" :size="15" /><span>保存截图</span></button><button type="button" class="pp-menu-action" :class="{ active: pipActive }" @click="togglePip"><UiIcon name="picture-in-picture" :size="15" /><span>画中画</span></button></div>
        </div>
      </section>
    </div>
  </teleport>
</template>

<style scoped>
.player-panel {
  position: fixed;
  z-index: 520;
  inset: 0;
  display: block;
  overflow: hidden;
  background: #07080a;
  color: #f9f9f9;
  font-family: inherit;
  letter-spacing: 0;
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
.pp-video { width: 100%; height: 100%; object-fit: contain; cursor: pointer; }
.pp-state {
  position: absolute;
  z-index: 2;
  inset: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 12px;
  padding: 24px;
  color: rgba(249, 249, 249, .72);
  font-size: 14px;
  text-align: center;
}
.pp-error { color: #ff8c8c; }
.pp-loading .spin { width: 19px; height: 19px; border-width: 2px; }
.pp-retry {
  display: flex;
  align-items: center;
  gap: 7px;
  height: 32px;
  padding: 0 12px;
  border: 1px solid rgba(255, 255, 255, .2);
  border-radius: 6px;
  color: #f9f9f9;
  background: rgba(255, 255, 255, .08);
  font: inherit;
  font-size: 13px;
  cursor: pointer;
}
.pp-retry:hover { background: rgba(255, 255, 255, .15); }
.pp-center-play {
  position: absolute;
  z-index: 2;
  left: 50%;
  top: 50%;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 64px;
  height: 64px;
  padding-left: 3px;
  transform: translate(-50%, -50%);
  border: 1px solid rgba(255, 255, 255, .64);
  border-radius: 50%;
  color: #07080a;
  background: rgba(255, 255, 255, .92);
  cursor: pointer;
  box-shadow: 0 10px 30px rgba(0, 0, 0, .32);
}
.pp-center-play:hover { background: #fff; transform: translate(-50%, -50%) scale(1.04); }
.pp-topbar,
.pp-bottom {
  position: absolute;
  z-index: 3;
  left: 0;
  right: 0;
  display: flex;
  align-items: center;
  background: rgba(7, 8, 10, .84);
  backdrop-filter: blur(14px);
  -webkit-backdrop-filter: blur(14px);
  transition: opacity 160ms ease, transform 160ms ease;
}
.pp-topbar {
  top: 0;
  justify-content: space-between;
  min-height: 68px;
  padding: 12px 18px;
  border-bottom: 1px solid rgba(255, 255, 255, .08);
}
.pp-topbar.hidden { opacity: 0; transform: translateY(-8px); pointer-events: none; }
.pp-file-meta { display: flex; align-items: center; min-width: 0; flex: 1; gap: 10px; }
.pp-file-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 34px;
  height: 34px;
  border: 1px solid rgba(255, 255, 255, .13);
  border-radius: 6px;
  color: rgba(249, 249, 249, .86);
  background: rgba(255, 255, 255, .045);
  flex-shrink: 0;
}
.pp-file-copy { min-width: 0; }
.pp-title { overflow: hidden; color: #f9f9f9; font-size: 14px; font-weight: 600; line-height: 19px; text-overflow: ellipsis; white-space: nowrap; }
.pp-source-label { margin-top: 1px; color: rgba(249, 249, 249, .5); font-size: 11px; line-height: 15px; text-transform: uppercase; }
.pp-top-actions,
.pp-control-group,
.pp-volume { display: flex; align-items: center; }
.pp-top-actions { gap: 4px; margin-left: 12px; flex-shrink: 0; }
.pp-icon,
.pp-play {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 36px;
  height: 36px;
  margin: 0;
  padding: 0;
  border: 1px solid transparent;
  border-radius: 6px;
  color: rgba(249, 249, 249, .78);
  background: transparent;
  cursor: pointer;
  flex-shrink: 0;
  transition: opacity 140ms ease, background 140ms ease, border-color 140ms ease;
}
.pp-icon:hover { color: #fff; border-color: rgba(255, 255, 255, .12); background: rgba(255, 255, 255, .08); }
.pp-icon.active { color: #88c8ff; background: rgba(85, 179, 255, .14); }
.pp-close { margin-left: 2px; }
.pp-more { position: relative; }
.pp-popover {
  position: absolute;
  z-index: 12;
  top: calc(100% + 8px);
  right: 0;
  width: 230px;
  padding: 6px;
  border: 1px solid rgba(255, 255, 255, .12);
  border-radius: 7px;
  background: #101111;
  box-shadow: 0 18px 44px rgba(0, 0, 0, .48), inset 0 1px rgba(255, 255, 255, .04);
}
.pp-menu-action {
  display: flex;
  align-items: center;
  width: 100%;
  height: 34px;
  gap: 9px;
  padding: 0 8px;
  border: 0;
  border-radius: 5px;
  color: rgba(249, 249, 249, .78);
  background: transparent;
  font: inherit;
  font-size: 13px;
  text-align: left;
  cursor: pointer;
}
.pp-menu-action:hover { color: #fff; background: rgba(255, 255, 255, .075); }
.pp-menu-action.active { color: #88c8ff; }
.pp-menu-select { display: flex; align-items: center; justify-content: space-between; gap: 12px; min-height: 38px; padding: 3px 8px; color: rgba(249, 249, 249, .56); font-size: 12px; }
.pp-menu-select :deep(.uiselect-btn) { min-width: 92px; height: 28px; border-color: rgba(255, 255, 255, .13); background: rgba(255, 255, 255, .04); color: #f9f9f9; }
.pp-bottom {
  bottom: 0;
  flex-direction: column;
  align-items: stretch;
  gap: 11px;
  padding: 12px 18px 14px;
  border-top: 1px solid rgba(255, 255, 255, .08);
}
.pp-bottom.hidden { opacity: 0; transform: translateY(8px); pointer-events: none; }
.pp-progress { position: relative; height: 8px; cursor: pointer; }
.pp-progress::before,
.pp-progress-buffer,
.pp-progress-fill {
  position: absolute;
  top: 2px;
  right: 0;
  left: 0;
  height: 4px;
  border-radius: 3px;
}
.pp-progress::before { content: ''; background: rgba(255, 255, 255, .18); }
.pp-progress-buffer { right: auto; width: var(--buffered); background: rgba(255, 255, 255, .35); }
.pp-progress-fill { right: auto; width: var(--played); background: #f4f5f6; }
.pp-progress-input { position: absolute; inset: 0; width: 100%; height: 100%; margin: 0; opacity: 0; cursor: pointer; }
.pp-control-row { display: flex; align-items: center; justify-content: space-between; min-width: 0; gap: 14px; }
.pp-left-controls { min-width: 0; gap: 3px; }
.pp-right-controls { justify-content: flex-end; gap: 5px; min-width: 0; }
.pp-play { width: 38px; height: 38px; color: #07080a; background: #f4f5f6; border-color: #f4f5f6; }
.pp-play:hover { background: #fff; }
.pp-time { margin-left: 7px; color: rgba(249, 249, 249, .83); font-size: 12px; font-variant-numeric: tabular-nums; white-space: nowrap; }
.pp-time i { margin: 0 5px; color: rgba(249, 249, 249, .35); font-style: normal; }
.pp-volume { gap: 2px; }
.pp-vol-input { width: 72px; margin: 0 4px 0 0; accent-color: #f4f5f6; cursor: pointer; }
.pp-select { flex-shrink: 0; }
.pp-select :deep(.uiselect-btn) { min-width: 72px; height: 30px; border-color: transparent; border-radius: 6px; color: rgba(249, 249, 249, .78); background: transparent; }
.pp-select :deep(.uiselect-btn:hover),
.pp-select :deep(.uiselect-btn.open) { border-color: rgba(255, 255, 255, .14); background: rgba(255, 255, 255, .075); box-shadow: none; }
.pp-select :deep(.uiselect-arrow) { color: rgba(249, 249, 249, .5); }
:global(.uiselect-drop) { z-index: 600; }

@media (max-width: 760px) {
  .pp-topbar { min-height: 60px; padding: 10px 12px; }
  .pp-bottom { padding: 10px 12px 12px; }
  .pp-file-icon { width: 31px; height: 31px; }
  .pp-wide-action, .pp-loop-control, .pp-quality-select, .pp-subtitle-select { display: none; }
  .pp-vol-input { width: 56px; }
}
@media (max-width: 560px) {
  .pp-source-label, .pp-skip, .pp-vol-input, .pp-speed-select { display: none; }
  .pp-top-actions { gap: 2px; margin-left: 6px; }
  .pp-icon { width: 34px; height: 34px; }
  .pp-play { width: 36px; height: 36px; }
  .pp-time { margin-left: 4px; font-size: 11px; }
  .pp-bottom { gap: 8px; }
  .pp-center-play { width: 58px; height: 58px; }
}
</style>

<style scoped>
/* Immersive playback surface: keep the stage quiet and reveal controls only on demand. */
.player-panel {
  --player-accent: #4da3ff;
  --player-glass: rgba(0, 0, 0, .62);
  --subtitle-scale: 1;
  isolation: isolate;
  background: #000;
  color: #fff;
}
.pp-stage { background: #000; }
.pp-video { cursor: default; }
.pp-topbar, .pp-bottom {
  background: linear-gradient(180deg, rgba(0, 0, 0, .72), var(--player-glass));
  border: 0;
  backdrop-filter: blur(18px) saturate(120%);
  -webkit-backdrop-filter: blur(18px) saturate(120%);
}
.pp-topbar {
  min-height: 74px;
  padding: 14px 22px;
  --wails-draggable: drag;
}
.pp-window-drag { --wails-draggable: drag; }
.pp-top-actions, .pp-top-actions button, .pp-bottom button, .pp-bottom input, .pp-settings-panel { --wails-draggable: no-drag; }
.pp-topbar.hidden { transform: translateY(-18px); }
.pp-bottom.hidden { transform: translateY(18px); }
.pp-file-icon { width: 36px; height: 36px; border: 0; border-radius: 50%; color: rgba(255,255,255,.82); background: rgba(255,255,255,.1); }
.pp-title { font-size: 15px; font-weight: 600; }
.pp-source-label { color: rgba(255,255,255,.54); text-transform: none; }
.pp-icon, .pp-play, .pp-panel-close { width: 42px; height: 42px; border: 0; border-radius: 50%; color: rgba(255,255,255,.78); }
.pp-icon:hover, .pp-panel-close:hover { color: #fff; background: rgba(255,255,255,.14); }
.pp-icon.active { color: #fff; background: color-mix(in srgb, var(--player-accent) 28%, transparent); }
.pp-close { margin-left: 8px; }
.pp-center-play {
  width: 64px;
  height: 64px;
  border: 1px solid rgba(255,255,255,.5);
  color: #111;
  background: rgba(255,255,255,.92);
  box-shadow: 0 12px 42px rgba(0,0,0,.45), 0 0 0 8px rgba(255,255,255,.08);
  animation: pp-center-in 180ms ease-out;
}
.pp-center-play:hover { background: #fff; box-shadow: 0 14px 48px rgba(0,0,0,.52), 0 0 0 10px rgba(255,255,255,.11); }
@keyframes pp-center-in { from { opacity: 0; transform: translate(-50%, -50%) scale(.82); } to { opacity: 1; transform: translate(-50%, -50%) scale(1); } }
.pp-bottom { gap: 14px; padding: 15px 22px 18px; }
.pp-progress { height: 12px; }
.pp-progress::before, .pp-progress-buffer, .pp-progress-fill { top: 4px; height: 4px; border-radius: 999px; transition: height 120ms ease, top 120ms ease; }
.pp-progress::before { background: rgba(255,255,255,.25); }
.pp-progress-buffer { background: rgba(255,255,255,.45); }
.pp-progress-fill { background: var(--player-accent); box-shadow: 0 0 10px color-mix(in srgb, var(--player-accent) 45%, transparent); }
.pp-progress:hover::before, .pp-progress:hover .pp-progress-buffer, .pp-progress:hover .pp-progress-fill { top: 3px; height: 6px; }
.pp-progress-input { z-index: 2; }
.pp-progress-input::-webkit-slider-thumb { width: 14px; height: 14px; opacity: 0; border: 0; border-radius: 50%; background: #fff; appearance: none; }
.pp-progress:hover .pp-progress-input::-webkit-slider-thumb, .pp-progress-input:focus-visible::-webkit-slider-thumb { opacity: 1; }
.pp-scrub-preview { position: absolute; z-index: 5; bottom: 16px; display: flex; flex-direction: column; align-items: center; gap: 4px; min-width: 68px; padding: 4px; transform: translateX(-50%); border: 1px solid rgba(255,255,255,.2); border-radius: 6px; background: rgba(8,8,8,.9); box-shadow: 0 10px 24px rgba(0,0,0,.42); pointer-events: none; }
.pp-scrub-preview img { display: block; width: 140px; height: 78px; object-fit: cover; border-radius: 3px; }
.pp-scrub-preview span { color: #fff; font-size: 11px; font-variant-numeric: tabular-nums; }
.pp-play { width: 44px; height: 44px; color: #111; background: #fff; border-color: #fff; }
.pp-play:hover { background: #fff; }
.pp-time { color: rgba(255,255,255,.8); font-size: 12px; }
.pp-volume { position: relative; gap: 2px; }
.pp-vol-input { position: absolute; bottom: 37px; left: 8px; display: block; width: 34px; height: 86px; margin: 0; padding: 8px 0; opacity: 0; border: 1px solid rgba(255,255,255,.15); border-radius: 18px; background: rgba(0,0,0,.75); writing-mode: vertical-lr; direction: rtl; transition: opacity 140ms ease; }
.pp-volume:hover .pp-vol-input, .pp-vol-input:focus-visible { opacity: 1; }
.pp-vol-input::-webkit-slider-runnable-track { width: 4px; border-radius: 4px; background: rgba(255,255,255,.26); }
.pp-vol-input::-webkit-slider-thumb { width: 12px; height: 12px; border: 0; border-radius: 50%; background: #fff; appearance: none; }
.pp-settings-panel { position: fixed; z-index: 20; left: 50%; bottom: 88px; display: flex; flex-direction: column; width: min(560px, calc(100vw - 32px)); max-height: min(62vh, 520px); overflow: auto; padding: 14px; transform: translateX(-50%); border: 1px solid rgba(255,255,255,.14); border-radius: 14px; background: rgba(14,15,18,.94); box-shadow: 0 22px 70px rgba(0,0,0,.56); backdrop-filter: blur(24px); -webkit-backdrop-filter: blur(24px); }
.pp-settings-head { display: flex; align-items: center; justify-content: space-between; padding: 2px 2px 10px; color: #fff; font-size: 13px; font-weight: 600; }
.pp-panel-close { width: 30px; height: 30px; }
.pp-setting-block { padding: 10px 2px; border-top: 1px solid rgba(255,255,255,.09); }
.pp-setting-label { display: flex; justify-content: space-between; margin-bottom: 8px; color: rgba(255,255,255,.55); font-size: 11px; }
.pp-option-grid { display: flex; flex-wrap: wrap; gap: 6px; }
.pp-option { min-width: 36px; height: 32px; padding: 0 10px; border: 1px solid rgba(255,255,255,.12); border-radius: 7px; color: rgba(255,255,255,.72); background: rgba(255,255,255,.05); font: inherit; font-size: 12px; cursor: pointer; }
.pp-option:hover, .pp-option.active { border-color: color-mix(in srgb, var(--player-accent) 70%, transparent); color: #fff; background: color-mix(in srgb, var(--player-accent) 22%, transparent); }
.pp-episode-grid .pp-option { width: 36px; padding: 0; }
.pp-subtitle-tools { display: flex; align-items: center; gap: 6px; margin-top: 10px; color: rgba(255,255,255,.56); font-size: 11px; }
.pp-mini-option { height: 28px; padding: 0 9px; border: 1px solid rgba(255,255,255,.13); border-radius: 6px; color: rgba(255,255,255,.72); background: rgba(255,255,255,.05); font: inherit; font-size: 11px; cursor: pointer; }
.pp-mini-option:hover, .pp-mini-option.active { color: #fff; background: rgba(255,255,255,.14); }
.pp-setting-actions { display: flex; gap: 4px; padding-top: 8px; border-top: 1px solid rgba(255,255,255,.09); }
.pp-menu-action { display: inline-flex; align-items: center; gap: 7px; height: 32px; padding: 0 9px; border: 0; border-radius: 6px; color: rgba(255,255,255,.72); background: transparent; font: inherit; font-size: 12px; cursor: pointer; }
.pp-menu-action:hover, .pp-menu-action.active { color: #fff; background: rgba(255,255,255,.11); }
.player-panel video::cue { color: #fff; background: rgba(0,0,0,.72); text-shadow: 0 1px 3px #000; font-size: calc(1em * var(--subtitle-scale)); }
@media (max-width: 760px) {
  .pp-topbar { min-height: 62px; padding: 10px 12px; }
  .pp-bottom { padding: 10px 12px 12px; }
  .pp-wide-action, .pp-loop-control { display: none; }
  .pp-vol-input { display: none; }
  .pp-scrub-preview img { width: 104px; height: 58px; }
}
@media (max-width: 560px) {
  .pp-source-label, .pp-skip { display: none; }
  .pp-top-actions { gap: 2px; margin-left: 6px; }
  .pp-icon { width: 38px; height: 38px; }
  .pp-play { width: 40px; height: 40px; }
  .pp-time { margin-left: 4px; font-size: 11px; }
  .pp-bottom { gap: 8px; }
  .pp-center-play { width: 58px; height: 58px; }
  .pp-settings-panel { bottom: 72px; width: calc(100vw - 20px); }
}
</style>
