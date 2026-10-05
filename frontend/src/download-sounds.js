export const DEFAULT_DOWNLOAD_SOUND = 'sound-1'

export const DOWNLOAD_SOUNDS = [
  { value: 'sound-1', label: '提示音 1', url: new URL('./assets/audio/download-complete-1.mp3', import.meta.url).href },
  { value: 'sound-2', label: '提示音 2', url: new URL('./assets/audio/download-complete-2.mp3', import.meta.url).href },
]

export function downloadSoundOf(value) {
  return DOWNLOAD_SOUNDS.find(sound => sound.value === value) || DOWNLOAD_SOUNDS[0]
}

export function createDownloadSoundPlayer() {
  let audio = null
  let currentSound = ''

  function stop() {
    if (audio) audio.pause()
    audio = null
    currentSound = ''
  }

  async function play(value) {
    try {
      const sound = downloadSoundOf(value)
      if (currentSound !== sound.value) {
        stop()
        audio = new Audio(sound.url)
        currentSound = sound.value
      }
      audio.currentTime = 0
      await audio.play()
      return true
    } catch {
      return false
    }
  }

  return { play, stop }
}
