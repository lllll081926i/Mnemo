import { describe, expect, it } from 'vitest'
import { driveToolDriveIdForPlatform, driveToolPlatformMatches, driveToolRootIdFor, normalizeDriveToolDriveId, normalizeDriveToolPlatform } from '../drive-tools/directLinks'

describe('drive tool target helpers', () => {
  it('passes through retained drive ids', () => {
    expect(normalizeDriveToolDriveId('pikpak')).toBe('pikpak')
    expect(normalizeDriveToolDriveId('onedrive')).toBe('onedrive')
  })

  it('builds drive ids from token platform names', () => {
    expect(driveToolDriveIdForPlatform('pikpak', '')).toBe('pikpak')
    expect(driveToolDriveIdForPlatform('gdrive', '')).toBe('gdrive')
    expect(driveToolDriveIdForPlatform('gdrive', 'fallback')).toBe('fallback')
  })

  it('matches platform names against tokenfrom values', () => {
    expect(normalizeDriveToolPlatform('pikpak')).toBe('pikpak')
    expect(driveToolPlatformMatches('pikpak', 'pikpak')).toBe(true)
  })

  it('resolves provider root ids from retained drive ids', () => {
    expect(driveToolRootIdFor('pikpak')).toBe('pikpak_root')
    expect(driveToolRootIdFor('unknown')).toBe('root')
  })
})
