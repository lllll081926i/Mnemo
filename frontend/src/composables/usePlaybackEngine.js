import { nextTick, onBeforeUnmount, ref } from 'vue'

const UNSUPPORTED_WEB_CONTAINERS = new Set(['avi', 'flv', 'm2ts', 'mkv', 'mpg', 'mpeg', 'mts', 'rm', 'rmvb', 'ts', 'wmv'])
let hlsConstructorPromise = null
let dashConstructorPromise = null

export function extensionOf(name) {
  const value = String(name || '')
  const index = value.lastIndexOf('.')
  return index > 0 ? value.slice(index + 1).toLowerCase() : ''
}

export function normalizeStreamType(preview, fileName = '') {
  const declared = String(preview?.stream_type || '').trim().toLowerCase()
  if (declared === 'm3u8') return 'hls'
  if (declared === 'mpd') return 'dash'
  if (declared) return declared
  const ext = extensionOf(fileName)
  if (ext === 'm3u8') return 'hls'
  if (ext === 'mpd') return 'dash'
  if (['m4v', 'mov', '3gp'].includes(ext)) return 'mp4'
  return ext
}

export function normalizeSubtitles(value) {
  if (!Array.isArray(value)) return []
  return value
    .map((track, index) => ({
      url: String(track?.url || '').trim(),
      language: String(track?.language || 'und').trim() || 'und',
      label: String(track?.language || `字幕 ${index + 1}`).trim() || `字幕 ${index + 1}`,
    }))
    .filter((track) => track.url)
}

async function getHlsConstructor() {
  // 必须保持动态 import，避免 HLS.js 进入首屏主包。
  if (!hlsConstructorPromise) hlsConstructorPromise = import('hls.js').then((module) => module.default)
  return hlsConstructorPromise
}

async function getDashConstructor() {
  // DASH 同样只在实际播放 MPD 时加载。
  if (!dashConstructorPromise) dashConstructorPromise = import('dashjs').then((module) => module.default || module)
  return dashConstructorPromise
}

export function usePlaybackEngine({
  currentFileName,
  error,
  extraTextSubtitles,
  onBeforeSourceLoad,
  playing,
  readableError = (errorValue) => String(errorValue?.message || errorValue || ''),
  resetTextSubtitles,
  videoEl,
}) {
  const src = ref('')
  const streamType = ref('')
  let sourceSequence = 0
  let disposed = false
  let hlsPlayer = null
  let dashPlayer = null
  let hlsRecoveryAttempts = 0
  let dashRecoveryAttempts = 0
  let activeSourceURL = ''
  let suppressVideoErrors = false
  let pendingResume = 0
  let pendingAutoplay = true

  function isCurrent(sequence) {
    return !disposed && sequence === sourceSequence
  }

  function clearVideoSource() {
    const video = videoEl.value
    if (!video) return
    suppressVideoErrors = true
    try {
      video.pause()
      video.removeAttribute('src')
      video.load()
    } catch {}
  }

  function destroyAdaptivePlayers() {
    const hls = hlsPlayer
    hlsPlayer = null
    if (hls) try { hls.destroy() } catch {}
    const dash = dashPlayer
    dashPlayer = null
    if (dash) try { dash.reset() } catch {}
  }

  function fail(message) {
    destroyAdaptivePlayers()
    playing.value = false
    error.value = message
  }

  async function loadNative(video, url, sequence) {
    src.value = url
    await nextTick()
    if (!isCurrent(sequence) || videoEl.value !== video) return
    suppressVideoErrors = false
    video.load()
  }

  function canPlayNativeHLS(video) {
    return Boolean(video.canPlayType('application/vnd.apple.mpegurl') || video.canPlayType('application/x-mpegURL'))
  }

  function handleHLSError(Hls, player, data, sequence) {
    if (!data?.fatal || player !== hlsPlayer || !isCurrent(sequence)) return
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
    fail('HLS 流加载失败，请检查网络或重新获取播放地址')
  }

  async function loadHLS(video, url, sequence) {
    if (canPlayNativeHLS(video)) return loadNative(video, url, sequence)
    const Hls = await getHlsConstructor()
    if (!isCurrent(sequence)) return
    if (!Hls?.isSupported()) throw new Error('当前系统 WebView 不支持 HLS 播放')
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
      if (player === hlsPlayer && isCurrent(sequence)) player.loadSource(url)
    })
    player.on(Hls.Events.ERROR, (_, data) => handleHLSError(Hls, player, data, sequence))
    player.attachMedia(video)
  }

  function handleDASHError(player, event, sequence) {
    if (player !== dashPlayer || !isCurrent(sequence)) return
    const retryURL = activeSourceURL
    if (dashRecoveryAttempts < 1 && retryURL && videoEl.value) {
      dashRecoveryAttempts++
      try {
        dashPlayer = null
        player.reset()
        void loadDASH(videoEl.value, retryURL, sequence, true).catch((loadError) => fail(readableError(loadError)))
        return
      } catch {}
    }
    const detail = event?.error?.message ? `：${event.error.message}` : ''
    fail('DASH 流加载失败' + detail)
  }

  async function loadDASH(video, url, sequence, retrying = false) {
    if (!window.MediaSource) throw new Error('当前系统 WebView 不支持 DASH 播放')
    const dashjs = await getDashConstructor()
    if (!isCurrent(sequence)) return
    const player = dashjs.MediaPlayer().create()
    dashPlayer = player
    if (!retrying) dashRecoveryAttempts = 0
    suppressVideoErrors = false
    player.updateSettings({
      streaming: {
        buffer: { bufferTimeAtTopQuality: 20, bufferTimeAtTopQualityLongForm: 30, stableBufferTime: 12 },
        fastSwitchEnabled: false,
      },
    })
    player.on(dashjs.MediaPlayer.events.ERROR, (event) => handleDASHError(player, event, sequence))
    player.initialize(video, url, false)
  }

  async function loadSource(preview, resumeAt, autoplay, parentIsCurrent = () => true) {
    const url = String(preview?.url || '').trim()
    if (!url) throw new Error('未获取到可播放的视频地址')
    const video = videoEl.value
    if (!video) throw new Error('播放器尚未初始化')
    const sequence = ++sourceSequence
    destroyAdaptivePlayers()
    clearVideoSource()
    src.value = ''
    activeSourceURL = url
    pendingResume = Math.max(0, Number(resumeAt) || 0)
    pendingAutoplay = Boolean(autoplay)
    streamType.value = normalizeStreamType(preview, currentFileName())
    resetTextSubtitles(normalizeSubtitles(preview?.subtitles), extraTextSubtitles.value)
    onBeforeSourceLoad()
    await nextTick()
    if (!isCurrent(sequence) || !parentIsCurrent()) return

    if (streamType.value === 'hls') return loadHLS(video, url, sequence)
    if (streamType.value === 'dash') return loadDASH(video, url, sequence)
    if (UNSUPPORTED_WEB_CONTAINERS.has(streamType.value)) {
      throw new Error(`网页播放器不支持 ${streamType.value.toUpperCase()} 容器，请下载后使用本地播放器打开`)
    }
    return loadNative(video, url, sequence)
  }

  function consumeLoadIntent() {
    const intent = { resumeAt: pendingResume, autoplay: pendingAutoplay }
    pendingResume = 0
    pendingAutoplay = false
    return intent
  }

  function handleVideoError(loading) {
    if (loading || suppressVideoErrors || hlsPlayer || dashPlayer) return false
    const code = videoEl.value?.error?.code
    error.value = code === 4
      ? '当前网页播放器不支持此视频的容器或编解码'
      : '视频加载失败，请检查网络或重新获取播放地址'
    return true
  }

  function reset() {
    sourceSequence++
    destroyAdaptivePlayers()
    clearVideoSource()
    src.value = ''
    streamType.value = ''
    activeSourceURL = ''
    pendingResume = 0
  }

  onBeforeUnmount(() => {
    disposed = true
    reset()
  })

  return { consumeLoadIntent, destroyAdaptivePlayers, handleVideoError, loadSource, reset, src, streamType }
}
