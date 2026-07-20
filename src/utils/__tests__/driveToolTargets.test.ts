import { describe, expect, it } from 'vitest'
import { driveToolDriveIdForPlatform, driveToolPlatformMatches, driveToolRootIdFor, normalizeDriveToolDriveId, normalizeDriveToolPlatform } from '../drive-tools/directLinks'

describe('drive tool target helpers', () => {
  it('normalizes provider aliases used by stored tokens', () => {
    expect(normalizeDriveToolDriveId('139')).toBe('cloud139')
    expect(normalizeDriveToolDriveId('189')).toBe('cloud189')
  })

  it('builds non-aliyun drive ids from token platform names', () => {
    expect(driveToolDriveIdForPlatform('guangya', '')).toBe('guangya')
  })

  it('normalizes AI requested platform aliases to tokenfrom values', () => {
    expect(normalizeDriveToolPlatform('cloud139')).toBe('139')
    expect(normalizeDriveToolPlatform('cloud189')).toBe('189')
    expect(driveToolPlatformMatches('139', 'cloud139')).toBe(true)
  })

  it('resolves provider root ids from normalized drive ids', () => {
    expect(driveToolRootIdFor('139')).toBe('cloud139_root')
    expect(driveToolRootIdFor('189')).toBe('cloud189_root')
  })
})
