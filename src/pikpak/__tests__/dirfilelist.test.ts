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
})
