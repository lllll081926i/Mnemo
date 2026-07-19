import { describe, expect, it } from 'vitest'
import { buildAriaAddOptions } from './aria2AddOptions'

describe('buildAriaAddOptions', () => {
  it('keeps HTTP-only options on URI downloads', () => {
    expect(buildAriaAddOptions({
      gid: 'g1',
      dir: '/tmp',
      split: 4,
      referer: 'https://www.aliyundrive.com/drive',
      userAgent: 'Chrome',
      headers: ['Authorization: Bearer token'],
      outFileName: 'movie.mkv'
    })).toEqual({
      gid: 'g1',
      dir: '/tmp',
      split: 4,
      out: 'movie.mkv',
      referer: 'https://www.aliyundrive.com/drive',
      'user-agent': 'Chrome',
      header: ['Authorization: Bearer token']
    })
  })

  it('adds proxy options to HTTP downloads', () => {
    expect(buildAriaAddOptions({
      gid: 'g1',
      dir: '/tmp',
      split: 4,
      referer: '',
      userAgent: '',
      headers: [],
      outFileName: 'archive.zip',
      allProxy: 'http://127.0.0.1:7890'
    })).toEqual({
      gid: 'g1',
      dir: '/tmp',
      split: 4,
      out: 'archive.zip',
      'all-proxy': 'http://127.0.0.1:7890'
    })
  })
})
