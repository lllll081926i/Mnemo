export interface ProxyRefreshState {
  driveId: string
  fileId: string
  proxyUrl: string
  selectQuality: string
  proxyInfo?: {
    drive_id?: string
    file_id?: string
    expires_time?: number
    videoQuality?: string
  }
}

export const shouldRefreshProxyUrl = (state: ProxyRefreshState): boolean => {
  const expiresAt = Number(state.proxyInfo?.expires_time || 0)
  const needRefreshUrl = state.proxyInfo && (state.driveId !== state.proxyInfo.drive_id || state.fileId !== state.proxyInfo.file_id || (expiresAt > 0 && expiresAt <= Date.now()))
  const changeVideoQuality = state.proxyInfo && state.proxyInfo.videoQuality && state.selectQuality !== state.proxyInfo.videoQuality
  return !state.proxyUrl || !!needRefreshUrl || !!changeVideoQuality
}
