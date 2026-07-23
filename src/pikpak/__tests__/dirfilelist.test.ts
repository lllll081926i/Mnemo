import { describe, expect, it } from 'vitest'
import { buildPikPakVideoQualities, mapPikPakFileToAliModel } from '../dirfilelist'

(globalThis as any).pinyinlite = (input: string) => input.split('').map((char) => [char])

describe('mapPikPakFileToAliModel', () => {
  it('maps a PikPak folder into the shared cloud file model', () => {
    const model = mapPikPakFileToAliModel({
      id: 'folder-id',
      parent_id: '',
      kind: 'drive#folder',
      name: 'Movies',
      size: '0',
      modified_time: '2026-05-09T08:00:00.000Z',
      created_time: '2026-05-08T08:00:00.000Z'
    }, 'pikpak', 'pikpak_root')

    expect(model.drive_id).toBe('pikpak')
    expect(model.file_id).toBe('folder-id')
    expect(model.parent_file_id).toBe('pikpak_root')
    expect(model.name).toBe('Movies')
    expect(model.isDir).toBe(true)
    expect(model.icon).toBe('iconfile-folder')
  })

  it('keeps file size, thumbnail, and web download url for files', () => {
    const model = mapPikPakFileToAliModel({
      id: 'file-id',
      parent_id: 'folder-id',
      kind: 'drive#file',
      name: 'clip.mp4',
      size: '1048576',
      modified_time: '2026-05-09T08:00:00.000Z',
      thumbnail_link: 'https://example.com/thumb.jpg',
      web_content_link: 'https://example.com/download.mp4'
    }, 'pikpak', 'folder-id')

    expect(model.drive_id).toBe('pikpak')
    expect(model.file_id).toBe('file-id')
    expect(model.parent_file_id).toBe('folder-id')
    expect(model.ext).toBe('mp4')
    expect(model.size).toBe(1048576)
    expect(model.sizeStr).toBe('1.00MB')
    expect(model.isDir).toBe(false)
    expect(model.thumbnail).toBe('https://example.com/thumb.jpg')
    expect(model.description).toContain('pikpak_download:https%3A%2F%2Fexample.com%2Fdownload.mp4')
  })

  it('keeps the extension for very long file names so videos stay playable', () => {
    const longName = `${'【超长合集】某个特别特别长的视频标题第01集'.repeat(10)}.MP4`
    const model = mapPikPakFileToAliModel({
      id: 'long-file',
      kind: 'drive#file',
      name: longName,
      size: '1024'
    }, 'pikpak', 'pikpak_root')

    expect(model.ext).toBe('mp4')
    expect(model.category).toMatch(/^video/)
    expect(model.icon).toBe('iconfile_video')
  })

  it('uses the provider file_extension when the name lost its suffix', () => {
    const model = mapPikPakFileToAliModel({
      id: 'truncated-file',
      kind: 'drive#file',
      name: '一个被截断的超长文件名没有任何后缀'.repeat(5),
      file_extension: '.MKV',
      size: '1024'
    }, 'pikpak', 'pikpak_root')

    expect(model.ext).toBe('mkv')
    expect(model.category).toMatch(/^video/)
  })

  it('falls back to mime_type when neither name nor file_extension identifies the type', () => {
    const model = mapPikPakFileToAliModel({
      id: 'mime-file',
      kind: 'drive#file',
      name: '没有后缀的视频文件'.repeat(10),
      mime_type: 'video/mp4',
      size: '1024'
    }, 'pikpak', 'pikpak_root')

    expect(model.ext).toBe('mp4')
    expect(model.mime_type).toBe('video/mp4')
    expect(model.category).toMatch(/^video/)
  })

  it('never treats the whole extension-less file name as an extension', () => {
    const model = mapPikPakFileToAliModel({
      id: 'noext-file',
      kind: 'drive#file',
      name: '完全没有扩展名的普通文件'.repeat(8),
      size: '1024'
    }, 'pikpak', 'pikpak_root')

    expect(model.ext).toBe('')
    expect(model.category).toBe('others')
  })
})

describe('buildPikPakVideoQualities', () => {
  it('uses the original file link when an origin media link points at a different fid', () => {
    const qualities = buildPikPakVideoQualities({
      id: 'file-id',
      name: 'long-video.mp4',
      links: { 'application/octet-stream': { url: 'https://download.example/file.mp4?fid=file-id&expire=1999999999' } },
      medias: [
        {
          is_origin: true,
          is_visible: true,
          link: { url: 'https://media.example/origin.mp4?fid=other-file&expire=1999999999' },
          video: { width: 1920, height: 1080 }
        },
        {
          is_visible: true,
          resolution_name: '720P',
          link: { url: 'https://media.example/transcode.mp4?fid=file-id&expire=1999999999' },
          video: { width: 1280, height: 720 }
        }
      ]
    }, true)

    expect(qualities.find((item) => item.quality === 'Origin')).toMatchObject({
      url: 'https://download.example/file.mp4?fid=file-id&expire=1999999999',
      forceProxy: true
    })
    expect(qualities.filter((item) => item.quality === 'FHD')).toHaveLength(0)
  })

  it('derives missing video height from resolution_name and forces PikPak streams through the local range proxy', () => {
    const qualities = buildPikPakVideoQualities({
      id: 'file-id',
      name: 'long-video.mp4',
      medias: [{
        is_visible: true,
        resolution_name: '720P',
        link: { url: 'https://media.example/long-video' },
        video: { width: 1280 }
      }]
    }, false)

    expect(qualities).toEqual([
      expect.objectContaining({ quality: 'HD', height: 720, url: 'https://media.example/long-video', forceProxy: true })
    ])
  })

  it('detects mpegts transcode streams as ts and appends origin as last-resort quality for non-VIP', () => {
    const qualities = buildPikPakVideoQualities({
      id: 'file-id',
      name: 'long-video.mp4',
      links: { 'application/octet-stream': { url: 'https://download.example/file.mp4?fid=file-id&expire=1999999999' } },
      medias: [{
        is_visible: true,
        resolution_name: '720P',
        link: { url: 'https://media.example/transcode?fid=file-id&expire=1999999999', type: 'video/mp2t' },
        video: { width: 1280, height: 720, video_type: 'mpegts' }
      }]
    }, false)

    expect(qualities).toHaveLength(2)
    expect(qualities[0]).toMatchObject({ quality: 'HD', type: 'ts', forceProxy: true })
    expect(qualities[1]).toMatchObject({ quality: 'Origin', url: 'https://download.example/file.mp4?fid=file-id&expire=1999999999', forceProxy: true })
  })
})
