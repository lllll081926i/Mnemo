import { ENGINE_RPC_PORT } from '@shared/constants'

export const getMotrixApplicationRpcPort = (): number => {
  const app = (globalThis as any).motrixApplication
  const port = app?.configManager?.getSystemConfig?.('rpc-listen-port')
  return Number.isFinite(Number(port)) && Number(port) > 0 ? Number(port) : ENGINE_RPC_PORT
}

const proxyProtocols: Record<string, string> = {
  PROXY: 'http',
  HTTP: 'http',
  HTTPS: 'https',
  SOCKS: 'socks5',
  SOCKS5: 'socks5',
  SOCKS4: 'socks4'
}

export const parseElectronProxyRules = (rules: string): string => {
  for (const rule of String(rules || '').split(';')) {
    const [type = '', endpoint = ''] = rule.trim().split(/\s+/, 2)
    const protocol = proxyProtocols[type.toUpperCase()]
    if (protocol && endpoint) return `${protocol}://${endpoint}`
  }
  return ''
}

export const syncMotrixApplicationProxy = async (proxyUrl: string): Promise<void> => {
  const app = (globalThis as any).motrixApplication
  if (!app) return
  const config = { 'all-proxy': proxyUrl, 'no-proxy': '' }
  app.configManager?.setSystemConfig?.(config)
  try {
    await app.engineClient?.changeGlobalOption?.(config)
  } catch {}
}
