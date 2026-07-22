import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'
import { buildDirectPlayerInvocation, normalizePlaylistStartIndex, redactMpvArgs, resolvePlayerMediaSource, shellSplit } from '../mpvPlayerPolicy'

describe('mpvPlayerPolicy', () => {
  it('builds direct-spawn invocations without a system shell', () => {
    expect(buildDirectPlayerInvocation('win32', 'C:\\Players\\mpv.exe')).toEqual({ binary: 'C:\\Players\\mpv.exe', args: [] })
    expect(buildDirectPlayerInvocation('darwin', '/Applications/mpv.app')).toEqual({ binary: 'open', args: ['-a', '/Applications/mpv.app', '--args'] })
    expect(buildDirectPlayerInvocation('linux', '/usr/bin/mpv')).toEqual({ binary: '/usr/bin/mpv', args: [] })
    expect(buildDirectPlayerInvocation('linux', 'mpv --profile=mnemo')).toEqual({ binary: 'mpv', args: ['--profile=mnemo'] })
    expect(buildDirectPlayerInvocation('linux', "'/opt/mpv player/mpv' --profile=mnemo")).toEqual({ binary: '/opt/mpv player/mpv', args: ['--profile=mnemo'] })
  })

  it('splits simple quoted Linux command strings', () => {
    expect(shellSplit("mpv --title='A B' --flag")).toEqual(['mpv', '--title=A B', '--flag'])
  })

  it('redacts tokens before logging MPV arguments', () => {
    expect(redactMpvArgs(['--http-header-fields=Authorization: Bearer secret-token', 'https://example.test/a.mkv?x-oss-signature=secret'])).toEqual([
      '--http-header-fields=Authorization: [REDACTED]',
      '[REDACTED_URL]'
    ])
  })

  it('uses direct media URLs when no transcoded qualities exist', () => {
    expect(resolvePlayerMediaSource({ url: 'https://media.test/direct', headers: { Authorization: 'Bearer token' } }, 'Origin')).toEqual({
      url: 'https://media.test/direct',
      headers: { Authorization: 'Bearer token' }
    })
  })

  it('selects a requested quality and keeps playlist indexes non-negative', () => {
    expect(resolvePlayerMediaSource({ url: 'https://media.test/direct', qualities: [{ quality: '720p', url: 'https://media.test/720p', headers: { Referer: 'https://media.test' } }] }, '720p')).toEqual({
      url: 'https://media.test/720p',
      headers: { Referer: 'https://media.test' }
    })
    expect(normalizePlaylistStartIndex(-1)).toBe(0)
    expect(normalizePlaylistStartIndex(2)).toBe(2)
  })

  it('starts and probes the bundled MPV process without a command shell', () => {
    const source = readFileSync('src/module/node-mpv/lib/util.js', 'utf8')
    expect(source).toContain("execFile(options.binary || 'mpv', ['--version']")
    expect(source).not.toMatch(/shell:\s*true|\bexec\(/)
  })
})
