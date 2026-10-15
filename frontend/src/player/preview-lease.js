function sessionURL(value) {
  try {
    const parsed = new URL(value)
    if (parsed.protocol !== 'http:' || parsed.hostname !== '127.0.0.1' || !parsed.port || parsed.username || parsed.password) return ''
    if (!/^\/stream\/[A-Za-z0-9_-]+$/.test(parsed.pathname)) return ''
    const token = parsed.searchParams.get('t')
    if (!token) return ''
    parsed.search = new URLSearchParams({ t: token }).toString()
    parsed.hash = ''
    return parsed.href
  } catch { return '' }
}

function previewURLs(preview) {
  const values = typeof preview === 'string' ? [preview] : [preview?.url, ...(preview?.subtitles || []).map(track => track?.url)]
  return new Set(values.map(sessionURL).filter(Boolean))
}

export async function releasePreviewURL(value) {
  const target = sessionURL(value)
  if (!target) return
  try {
    await fetch(target, { method: 'DELETE', keepalive: true, credentials: 'omit', cache: 'no-store' })
  } catch {}
}

export function createPreviewLease(release = releasePreviewURL) {
  let owned = new Set()
  let disposed = false
  function relinquish(values) {
    for (const value of values) {
      try { Promise.resolve(release(value)).catch(() => {}) } catch {}
    }
  }
  function discard(preview) {
    relinquish([...previewURLs(preview)].filter(value => !owned.has(value)))
  }
  function clear() {
    const previous = owned
    owned = new Set()
    relinquish(previous)
  }
  return {
    replace(preview) {
      if (disposed) { discard(preview); return }
      const next = previewURLs(preview)
      const previous = owned
      owned = next
      relinquish([...previous].filter(value => !next.has(value)))
    },
    add(value) {
      if (disposed) { discard(value); return }
      for (const target of previewURLs(value)) owned.add(target)
    },
    discard,
    clear,
    dispose() { disposed = true; clear() },
  }
}

export function createPreviewLeasePool(release = releasePreviewURL) {
  const leases = new Map()
  let disposed = false
  function clear() {
    for (const lease of leases.values()) lease.dispose()
    leases.clear()
  }
  return {
    acquire(key) {
      if (leases.has(key)) return leases.get(key)
      const lease = createPreviewLease(release)
      if (disposed) lease.dispose()
      else leases.set(key, lease)
      return lease
    },
    retain(keys) {
      const retained = new Set(keys)
      for (const [key, lease] of leases) {
        if (retained.has(key)) continue
        lease.dispose()
        leases.delete(key)
      }
    },
    dispose() { disposed = true; clear() },
  }
}
