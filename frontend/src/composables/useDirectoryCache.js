import {
  DeleteDirectoryCache,
  ListDirPage,
  SaveDirectoryCache,
  listDir,
  providerOf,
} from '../api'

const CACHE_MAX = 200
const CACHE_TTL_MS = 10 * 60 * 1000
const PAGE_REQUEST_DELAY_MS = 180

function cacheKeyPart(value) {
  return encodeURIComponent(String(value ?? ''))
}

function paginationUnsupported(errorValue) {
  const message = String(errorValue?.message || errorValue || '').toLowerCase()
  return message.includes('listpaged') && (message.includes('not supported') || message.includes('不支持'))
}

function waitForNextPage() {
  return new Promise((resolve) => setTimeout(resolve, PAGE_REQUEST_DELAY_MS))
}

export function useDirectoryCache() {
  const entries = new Map()
  const pendingWrites = new Map()
  const dirtyKeys = new Set()
  const directoryVersions = new Map()
  const modeVersions = new Map()
  let epoch = 0

  function directoryKey(userID, driveID, mode, id, keyword) {
    return [providerOf(userID), userID, driveID, mode, id || '', keyword || ''].map(cacheKeyPart).join('|')
  }

  function modeKey(userID, driveID, mode) {
    return [providerOf(userID), userID, driveID, mode].map(cacheKeyPart).join('|')
  }

  function currentEpoch() {
    return epoch
  }

  function versionOf(key) {
    return directoryVersions.get(key) || 0
  }

  function modeVersionOf(key) {
    return modeVersions.get(key) || 0
  }

  function isCurrent(key, expectedEpoch, version, currentModeKey = '', modeVersion = 0) {
    return expectedEpoch === epoch && version === versionOf(key) &&
      (!currentModeKey || modeVersion === modeVersionOf(currentModeKey))
  }

  function put(key, files) {
    entries.set(key, { files: files || [], at: Date.now() })
    if (entries.size > CACHE_MAX) entries.delete(entries.keys().next().value)
  }

  function get(key) {
    if (dirtyKeys.has(key)) return null
    const cached = entries.get(key)
    if (!cached) return null
    if (Date.now() - cached.at > CACHE_TTL_MS) {
      entries.delete(key)
      return null
    }
    return cached
  }

  function queueWrite(key, action) {
    const previous = pendingWrites.get(key) || Promise.resolve()
    const next = previous.catch(() => {}).then(action)
    pendingWrites.set(key, next)
    next.finally(() => {
      if (pendingWrites.get(key) === next) pendingWrites.delete(key)
    }).catch(() => {})
    return next
  }

  function persist(key, files, expectedEpoch = epoch, version = versionOf(key)) {
    return queueWrite(key, () => {
      if (!isCurrent(key, expectedEpoch, version)) return
      return Promise.resolve(SaveDirectoryCache(key, files || [])).then(() => {
        if (isCurrent(key, expectedEpoch, version)) dirtyKeys.delete(key)
      })
    })
  }

  function isPersistableMode(mode) {
    return mode === 'list'
  }

  function invalidateDirectory(userID, driveID, mode, id) {
    const prefix = directoryKey(userID, driveID, mode, id, '')
    for (const key of entries.keys()) if (key.startsWith(prefix)) entries.delete(key)
    const key = directoryKey(userID, driveID, mode, id, '')
    directoryVersions.set(key, versionOf(key) + 1)
    if (dirtyKeys.has(key)) return
    dirtyKeys.add(key)
    queueWrite(key, () => DeleteDirectoryCache(key)).catch(() => {})
  }

  function invalidateMode(userID, driveID, mode) {
    const prefix = [providerOf(userID), userID, driveID, mode].map(cacheKeyPart).join('|') + '|'
    const key = modeKey(userID, driveID, mode)
    modeVersions.set(key, modeVersionOf(key) + 1)
    for (const entryKey of entries.keys()) if (entryKey.startsWith(prefix)) entries.delete(entryKey)
  }

  function isDirty(key) {
    return dirtyKeys.has(key)
  }

  async function listProgressively(userID, driveID, id, isCurrentRequest, onPage) {
    const combined = []
    const seenMarkers = new Set()
    let marker = ''
    for (let pageIndex = 0; pageIndex < 10000; pageIndex++) {
      let page
      try {
        page = await ListDirPage(userID, driveID, id, marker)
      } catch (error) {
        if (pageIndex === 0 && paginationUnsupported(error)) return (await listDir(userID, driveID, id)) || []
        throw error
      }
      if (!isCurrentRequest()) return combined
      combined.push(...(Array.isArray(page?.items) ? page.items : []))
      onPage(combined)
      const nextMarker = String(page?.nextMarker || '')
      if (!nextMarker) return combined
      if (seenMarkers.has(nextMarker)) throw new Error('目录分页游标重复')
      seenMarkers.add(nextMarker)
      marker = nextMarker
      await waitForNextPage()
    }
    throw new Error('目录分页超过安全上限')
  }

  function clear() {
    epoch++
    entries.clear()
    dirtyKeys.clear()
    directoryVersions.clear()
    modeVersions.clear()
  }

  return {
    clear,
    currentEpoch,
    directoryKey,
    get,
    invalidateDirectory,
    invalidateMode,
    isCurrent,
    isDirty,
    isPersistableMode,
    listProgressively,
    modeKey,
    modeVersionOf,
    persist,
    put,
    versionOf,
  }
}
