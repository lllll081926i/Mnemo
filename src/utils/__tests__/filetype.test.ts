import { describe, expect, it } from 'vitest'
import { extFromFileName, extFromMimeType, normalizeFileExt, resolveFileExt } from '../filetype'

describe('normalizeFileExt', () => {
  it('keeps normal extensions and normalizes case/dot', () => {
    expect(normalizeFileExt('mp4')).toBe('mp4')
    expect(normalizeFileExt('.MP4')).toBe('mp4')
    expect(normalizeFileExt(' MKV ')).toBe('mkv')
  })

  it('rejects implausible extensions', () => {
    expect(normalizeFileExt('')).toBe('')
    expect(normalizeFileExt('a'.repeat(30))).toBe('')
    expect(normalizeFileExt('mp4 part2')).toBe('')
    expect(normalizeFileExt('mp4?fid=1')).toBe('')
    expect(normalizeFileExt('mp4&x')).toBe('')
  })
})

describe('extFromFileName', () => {
  it('extracts the extension from a normal file name', () => {
    expect(extFromFileName('clip.mp4')).toBe('mp4')
    expect(extFromFileName('archive.tar.gz')).toBe('gz')
    expect(extFromFileName('Movie.2024.1080p.WEB-DL.mkv')).toBe('mkv')
  })

  it('handles very long names without losing the extension', () => {
    const longName = `${'超长文件名'.repeat(60)}.MP4`
    expect(extFromFileName(longName)).toBe('mp4')
  })

  it('never returns the whole file name when there is no dot', () => {
    expect(extFromFileName('没有后缀的视频文件'.repeat(20))).toBe('')
    expect(extFromFileName('noext')).toBe('')
  })

  it('returns empty for dotfile-only or trailing-dot names', () => {
    expect(extFromFileName('.gitignore')).toBe('')
    expect(extFromFileName('name.')).toBe('')
    expect(extFromFileName('')).toBe('')
  })
})

describe('extFromMimeType', () => {
  it('maps common media mime types', () => {
    expect(extFromMimeType('video/mp4')).toBe('mp4')
    expect(extFromMimeType('video/x-matroska')).toBe('mkv')
    expect(extFromMimeType('audio/mpeg')).toBe('mp3')
    expect(extFromMimeType('image/jpeg')).toBe('jpg')
  })

  it('returns empty for unknown mime types', () => {
    expect(extFromMimeType('application/octet-stream')).toBe('')
    expect(extFromMimeType('')).toBe('')
  })
})

describe('resolveFileExt', () => {
  it('prefers the provider file_extension over the file name', () => {
    expect(resolveFileExt('被截断的超长文件名没有后缀', '.MKV')).toBe('mkv')
  })

  it('falls back to the file name extension', () => {
    expect(resolveFileExt('movie.webm')).toBe('webm')
  })

  it('falls back to mime_type when the name has no usable extension', () => {
    expect(resolveFileExt('没有后缀的视频'.repeat(30), '', 'video/mp4')).toBe('mp4')
  })

  it('returns empty when nothing can identify the type', () => {
    expect(resolveFileExt('unknown file', '', 'application/octet-stream')).toBe('')
  })
})
