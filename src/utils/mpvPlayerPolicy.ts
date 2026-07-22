export type PlayerPlatform = 'win32' | 'darwin' | 'linux' | string

export interface PlayerMediaSource {
  url?: string
  headers?: Record<string, string>
  qualities?: Array<{ quality?: string; url?: string; headers?: Record<string, string> }>
}

export function isMpvCommand(command: string): boolean {
  return (command || '').toLowerCase().includes('mpv')
}

export function shellSplit(command: string): string[] {
  const result: string[] = []
  let current = ''
  let quote: '"' | "'" | '' = ''
  let escaped = false

  for (const char of String(command || '').trim()) {
    if (escaped) {
      current += char
      escaped = false
      continue
    }
    if (char === '\\' && quote !== "'") {
      escaped = true
      continue
    }
    if ((char === '"' || char === "'") && (!quote || quote === char)) {
      quote = quote ? '' : char
      continue
    }
    if (!quote && /\s/.test(char)) {
      if (current) {
        result.push(current)
        current = ''
      }
      continue
    }
    current += char
  }

  if (current) result.push(current)
  return result
}

export function buildDirectPlayerInvocation(platform: PlayerPlatform, command: string): { binary: string; args: string[] } {
  if (platform === 'darwin') return { binary: 'open', args: ['-a', command, ...(command.includes('mpv.app') ? ['--args'] : [])] }
  if (platform === 'win32') return { binary: command, args: [] }
  const parts = shellSplit(command)
  return {
    binary: parts[0] || command,
    args: parts.slice(1)
  }
}

export function resolvePlayerMediaSource(rawData: PlayerMediaSource | null | undefined, quality: string): { url: string; headers: Record<string, string> } {
  const selectedQuality = rawData?.qualities?.find((item) => item.quality === quality) || rawData?.qualities?.[0]
  return {
    url: selectedQuality?.url || rawData?.url || '',
    headers: selectedQuality?.headers || rawData?.headers || {}
  }
}

export const normalizePlaylistStartIndex = (index: number): number => Math.max(0, index)

export function redactMpvArgs(args: any[]): any[] {
  return args.map((arg) => {
    const value = String(arg)
    if (value.includes('Authorization:')) return value.replace(/Authorization:\s*[^'"]+/i, 'Authorization: [REDACTED]')
    if (/access_token=|x-oss-signature=|X-Amz-Signature=/i.test(value)) return '[REDACTED_URL]'
    return arg
  })
}
