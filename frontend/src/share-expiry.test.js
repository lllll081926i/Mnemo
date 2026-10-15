import { expect, it } from 'vitest'
import { shareExpiry } from './share-expiry'

it('分享期限仅根据已记录期限提示，不把缺失记录当成永久有效', () => {
  const now = Date.parse('2026-10-15T00:00:00Z')
  expect(shareExpiry({}, now).label).toBe('期限未记录')
  expect(shareExpiry({ expiration: '0' }, now).label).toBe('期限无法确认')
  expect(shareExpiry({ expiration: '2026-10-16T00:00:00Z' }, now).warning).toBe(true)
  expect(shareExpiry({ expiration: '7', created_at: now / 1000 - 8 * 86400 }, now).label).toContain('已到期')
  expect(shareExpiry({ expiration: 'bad' }, now).warning).toBe(false)
})
