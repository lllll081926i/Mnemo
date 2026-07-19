import { describe, expect, it } from 'vitest'
import { findBestSubtitleMatch } from '../subtitleMatching'

describe('findBestSubtitleMatch', () => {
  it('matches language-suffixed subtitles to the same video', () => {
    const subtitle = findBestSubtitleMatch('The.Movie.2024.1080p.mkv', [{ name: 'The.Movie.2024.zh-CN.srt' }, { name: 'Another.Movie.zh-CN.srt' }])
    expect(subtitle?.name).toBe('The.Movie.2024.zh-CN.srt')
  })

  it('matches the closest episode and rejects unrelated subtitles', () => {
    const subtitles = [{ name: 'Show.S01E01.chs.ass' }, { name: 'Show.S01E02.chs.ass' }]
    expect(findBestSubtitleMatch('Show.S01E02.2160p.WEB-DL.mkv', subtitles)?.name).toBe('Show.S01E02.chs.ass')
    expect(findBestSubtitleMatch('Completely.Different.Movie.mkv', subtitles)).toBeUndefined()
  })

  it('does not fuzzy-match unrelated short names', () => {
    expect(findBestSubtitleMatch('1.mp4', [{ name: '2.srt' }])).toBeUndefined()
  })
})
