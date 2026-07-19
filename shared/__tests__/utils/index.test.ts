import { describe, expect, it } from 'vitest'
import { bytesToSize, separateConfig } from '../../utils'

describe('bytesToSize', () => {
  it('formats bytes', () => {
    expect(bytesToSize(1024)).toBe('1.0 KB')
    expect(bytesToSize(1024 * 1024)).toBe('1.0 MB')
    expect(bytesToSize(0)).toBe('0 B')
  })
})

describe('separateConfig', () => {
  it('splits config into system and user parts', () => {
    const { system, user, others } = separateConfig({ split: 8, theme: 'light', 'bt-tracker': 'x' } as any)
    expect(system.split).toBe(8)
    expect(user.theme).toBe('light')
    expect(others['bt-tracker']).toBe('x')
  })
})
