import UserDAL from '../user/userdal'
import type { ITokenInfo } from '../user/userstore'
import { getDriveProviderIcon, getDriveProviderLabel, type DriveProvider } from './driveProvider'

export interface DriveAccountOption {
  user_id: string
  name: string
  provider: DriveProvider
  providerLabel: string
  icon: string
}

export const toDriveAccountOption = (token: ITokenInfo): DriveAccountOption => ({
  user_id: token.user_id,
  name: token.nick_name || token.user_name || token.name || token.user_id,
  provider: token.tokenfrom,
  providerLabel: getDriveProviderLabel(token.tokenfrom),
  icon: getDriveProviderIcon(token.tokenfrom)
})

export const loadDriveAccountOptions = async (): Promise<DriveAccountOption[]> => {
  const tokens = await UserDAL.GetUserListFromDB()
  return tokens.filter((token) => !!token.user_id).map(toDriveAccountOption)
}

export const getDriveAccountLabel = (userId: string): string => {
  if (!userId || userId == 'external') return '外部链接'
  const token = UserDAL.GetUserToken(userId)
  if (!token.user_id) return userId
  const account = toDriveAccountOption(token)
  return `${account.providerLabel} · ${account.name}`
}
