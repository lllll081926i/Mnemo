import { describe, expect, it } from 'vitest'
import { canUseAliyunPreviewApi, resolveDriveProvider } from '../driveProvider'

describe('cloud139 document preview routing', () => {
  it('resolves cloud139 identifiers to the 139 provider', () => {
    expect(resolveDriveProvider({ driveId: 'cloud139' })).toBe('139')
    expect(resolveDriveProvider({ userId: 'cloud139_account-1' })).toBe('139')
  })

  it('only allows Aliyun accounts to use Aliyun preview APIs', () => {
    expect(canUseAliyunPreviewApi({ tokenfrom: 'aliyun' })).toBe(true)
    expect(canUseAliyunPreviewApi({ driveId: 'cloud139' })).toBe(false)
    expect(canUseAliyunPreviewApi({ driveId: 'cloud189' })).toBe(false)
    expect(canUseAliyunPreviewApi({ driveId: 'drive115' })).toBe(false)
    expect(canUseAliyunPreviewApi({ driveId: 'baidu' })).toBe(false)
  })
})
