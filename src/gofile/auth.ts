import { humanSize } from '../utils/format'
import { buildDriveProviderDriveId, buildDriveProviderUserId } from '../utils/driveProvider'
import { createProviderToken } from '../utils/providerToken'
import { gofileRequestWithToken } from './client'

interface GofileAccount {
  id: string
  email?: string
  tier?: string
  premiumType?: string
  rootFolder: string
  subscriptionLimitStorage?: number
  statsCurrent?: { storage?: number }
}

export const loginGofile = async (accessToken: string) => {
  const normalizedToken = accessToken.trim()
  if (!normalizedToken) throw new Error('请输入 GoFile Account API Token')
  const idResponse = await gofileRequestWithToken<{ id: string }>(normalizedToken, '/accounts/getid')
  const accountId = idResponse.data?.id || ''
  if (!accountId) throw new Error('GoFile 账号 ID 获取失败')
  const accountResponse = await gofileRequestWithToken<GofileAccount>(normalizedToken, `/accounts/${encodeURIComponent(accountId)}`)
  const account = accountResponse.data
  if (!account?.rootFolder) throw new Error('GoFile 根目录信息获取失败')
  const used = Number(account.statsCurrent?.storage || 0)
  const total = Number(account.subscriptionLimitStorage || 0)
  const name = account.email || `GoFile ${accountId.slice(-6)}`
  return createProviderToken('gofile', {
    access_token: normalizedToken,
    token_type: 'Bearer',
    user_id: buildDriveProviderUserId('gofile', accountId),
    user_name: name,
    nick_name: name,
    name,
    default_drive_id: buildDriveProviderDriveId('gofile', accountId),
    provider_account_id: accountId,
    provider_root_id: account.rootFolder,
    role: account.tier || account.premiumType || '',
    used_size: used,
    total_size: total,
    free_size: total > used ? total - used : 0,
    spaceinfo: total ? `${humanSize(used)} / ${humanSize(total)}` : humanSize(used)
  })
}
