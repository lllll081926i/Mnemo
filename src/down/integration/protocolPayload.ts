export type ExternalDownloadSourceType = 'url'

export interface ExternalDownloadPayload {
  source: string
  sourceType: ExternalDownloadSourceType
}

export const parseExternalDownloadPayload = (value: string): ExternalDownloadPayload | null => {
  const source = value.trim()
  if (!source) return null
  if (!/^https?:\/\//i.test(source)) return null
  if (/\.torrent(?:[?#].*)?$/i.test(source)) return null
  return { source, sourceType: 'url' }
}
