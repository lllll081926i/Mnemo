import levenshtein from 'fast-levenshtein'

const subtitleQualifierPattern = /\b(?:zh(?:[-_ ]?(?:cn|tw|hans|hant))?|chs|cht|chi|cn|sc|tc|en|eng|english|forced|default|simplified|traditional)\b|简体|繁体|中文|中英|双语/gi
const releaseQualifierPattern = /\b(?:360p|480p|540p|720p|1080p|1440p|2160p|4k|8k|web[-_. ]?dl|webrip|bluray|bdrip|hdr|dv|x26[45]|h26[45]|hevc|av1)\b/gi

export const normalizeSubtitleStem = (value: string): string => {
  const withoutExtension = String(value || '').replace(/\.[^.\\/]+$/, '')
  return withoutExtension
    .normalize('NFKC')
    .toLowerCase()
    .replace(subtitleQualifierPattern, ' ')
    .replace(releaseQualifierPattern, ' ')
    .replace(/[^\p{L}\p{N}]+/gu, ' ')
    .trim()
    .replace(/\s+/g, ' ')
}

const getMatchScore = (videoName: string, subtitleName: string): number => {
  const video = normalizeSubtitleStem(videoName)
  const subtitle = normalizeSubtitleStem(subtitleName)
  if (!video || !subtitle) return Number.POSITIVE_INFINITY
  if (video === subtitle) return 0

  const shorter = video.length <= subtitle.length ? video : subtitle
  const longer = video.length > subtitle.length ? video : subtitle
  if (shorter.length >= 4 && longer.includes(shorter) && shorter.length / longer.length >= 0.62) return 0.12
  if (Math.min(video.length, subtitle.length) < 3) return Number.POSITIVE_INFINITY

  return levenshtein.get(video, subtitle, { useCollator: true }) / Math.max(video.length, subtitle.length)
}

export const findBestSubtitleMatch = <T extends { name?: string; html?: string }>(videoName: string, subtitles: T[]): T | undefined => {
  let best: { item: T; score: number } | undefined
  for (const item of subtitles) {
    const score = getMatchScore(videoName, item.name || item.html || '')
    if (!best || score < best.score) best = { item, score }
  }
  return best && best.score <= 0.34 ? best.item : undefined
}
