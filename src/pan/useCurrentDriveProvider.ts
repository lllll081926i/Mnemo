import { computed } from 'vue'
import UserDAL from '../user/userdal'
import { getDriveProviderCapabilities, resolveDriveProvider } from '../utils/driveProvider'
import usePanTreeStore from './pantreestore'

export default function useCurrentDriveProvider() {
  const panTreeStore = usePanTreeStore()
  const context = computed(() => {
    const userId = panTreeStore.user_id || ''
    const token = UserDAL.GetUserToken(userId)
    return { tokenfrom: token?.tokenfrom, userId, driveId: panTreeStore.drive_id || panTreeStore.selectDir.drive_id }
  })
  const provider = computed(() => resolveDriveProvider(context.value))
  const capabilities = computed(() => getDriveProviderCapabilities(context.value))
  return { provider, capabilities }
}
