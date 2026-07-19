import UserDAL from '../user/userdal'
import type { ITokenInfo } from '../user/userstore'
import { getDriveProviderAccountId, getDriveProviderIcon, getDriveProviderLabel, type DriveProvider } from './driveProvider'

export interface DriveAccountOption {
  user_id: string
  name: string
  provider: DriveProvider
  providerLabel: string
  detail: string
  icon: string
}

const getAccountIdentifier = (token: ITokenInfo, name: string, providerLabel: string) => {
  const accountName = (token.user_name || '').trim()
  if (accountName && accountName !== name && accountName !== providerLabel) return accountName
  const accountId = getDriveProviderAccountId(token.user_id, token.tokenfrom)
  if (!accountId || accountId === name) return ''
  return accountId.length > 12 ? `••••${accountId.slice(-6)}` : accountId
}

export const toDriveAccountOption = (token: ITokenInfo): DriveAccountOption => {
  const name = token.nick_name || token.user_name || token.name || token.user_id
  const providerLabel = getDriveProviderLabel(token.tokenfrom)
  const identifier = getAccountIdentifier(token, name, providerLabel)
  return {
    user_id: token.user_id,
    name,
    provider: token.tokenfrom,
    providerLabel,
    detail: identifier ? `${providerLabel} · ${identifier}` : providerLabel,
    icon: getDriveProviderIcon(token.tokenfrom)
  }
}

export const loadDriveAccountOptions = async (): Promise<DriveAccountOption[]> => {
  const tokens = await UserDAL.GetUserListFromDB()
  return tokens
    .filter((token) => !!token.user_id)
    .map(toDriveAccountOption)
    .sort((a, b) => a.providerLabel.localeCompare(b.providerLabel) || a.name.localeCompare(b.name) || a.user_id.localeCompare(b.user_id))
}

export const getDriveAccountLabel = (userId: string): string => {
  if (!userId || userId == 'external') return '外部链接'
  const token = UserDAL.GetUserToken(userId)
  if (!token.user_id) return userId
  const account = toDriveAccountOption(token)
  return `${account.providerLabel} · ${account.name}`
}
