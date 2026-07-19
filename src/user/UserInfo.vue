<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { RefreshCw } from 'lucide-vue-next'
import { useUserStore } from '../store'
import UserDAL from '../user/userdal'
import message from '../utils/message'
import { humanSize } from '../utils/format'
import { getDriveProviderMeta } from '../utils/driveProvider'

const userStore = useUserStore()
const activeAvatarFailed = ref(false)
const refreshing = ref(false)

const activeProviderMeta = computed(() => getDriveProviderMeta(userStore.GetUserToken.tokenfrom))
const activeProviderIcon = computed(() => activeProviderMeta.value.icon)
const activeAccountName = computed(() => userStore.GetUserToken.nick_name || userStore.GetUserToken.user_name || activeProviderMeta.value.label)
const activeAccountDetail = computed(() => {
  const account = userStore.GetUserToken.user_name || ''
  return account && account !== activeAccountName.value ? account : ''
})
const activeAvatarUrl = computed(() => (activeAvatarFailed.value ? '' : userStore.GetUserToken.avatar || ''))
const activeAvatarText = computed(() => activeAccountName.value.trim().substring(0, 2) || activeProviderMeta.value.label.substring(0, 2))

const activeQuotaStats = computed(() => {
  const token = userStore.GetUserToken
  const total = Math.max(0, Number(token.total_size) || 0)
  const rawUsed = Math.max(0, Number(token.used_size) || 0)
  const rawRemaining = Math.max(0, Number(token.free_size) || 0)
  const used = total > 0 && rawUsed === 0 && rawRemaining > 0 ? Math.max(0, total - rawRemaining) : Math.min(rawUsed, total || rawUsed)
  const remaining = total > 0 ? Math.max(0, rawRemaining > 0 ? Math.min(rawRemaining, total) : total - used) : rawRemaining
  return { total, used, remaining }
})

const activeQuotaPercent = computed(() => {
  const { total, used } = activeQuotaStats.value
  if (!total) return 0
  return Math.max(0, Math.min(1, used / total))
})

const activeQuotaText = computed(() => {
  const { total, used, remaining } = activeQuotaStats.value
  if (total > 0) return `已用 ${humanSize(used)}，剩余 ${humanSize(remaining)}，总容量 ${humanSize(total)}`
  return userStore.GetUserToken.spaceinfo || '该网盘暂未返回容量信息'
})

const handleRefreshUserInfo = async () => {
  if (refreshing.value) return
  refreshing.value = true
  try {
    const success = await UserDAL.UserRefreshByUserFace(userStore.user_id, false)
    if (success) message.info('账号信息已刷新')
    else message.error('刷新账号信息失败')
  } catch {
    message.error('刷新账号信息失败')
  } finally {
    refreshing.value = false
  }
}

const handleAddAccount = () => {
  userStore.userShowLogin = true
}

watch(
  () => [userStore.user_id, userStore.GetUserToken.avatar],
  () => {
    activeAvatarFailed.value = false
  },
  { immediate: true }
)
</script>

<template>
  <a-popover v-if="userStore.userLogined" position="br" trigger="hover">
    <button type="button" class="current-drive-trigger" :title="`${activeProviderMeta.label} · ${activeAccountName}`" :aria-label="`当前网盘：${activeProviderMeta.label}，${activeAccountName}`">
      <a-avatar :size="28" class="current-drive-avatar">
        <img v-if="activeAvatarUrl" :src="activeAvatarUrl" alt="" @error="activeAvatarFailed = true" />
        <img v-else-if="activeProviderIcon" :src="activeProviderIcon" alt="" />
        <span v-else>{{ activeAvatarText }}</span>
      </a-avatar>
      <span v-if="activeAvatarUrl && activeProviderIcon" class="current-drive-badge" aria-hidden="true">
        <img :src="activeProviderIcon" alt="" />
      </span>
    </button>

    <template #content>
      <div class="current-drive-panel">
        <div class="current-drive-panel-head">
          <div class="current-drive-panel-avatar" aria-hidden="true">
            <img v-if="activeProviderIcon" :src="activeProviderIcon" alt="" />
            <span v-else>{{ activeProviderMeta.label.substring(0, 1) }}</span>
          </div>
          <div class="current-drive-identity">
            <strong :title="activeAccountName">{{ activeAccountName }}</strong>
            <span :title="activeAccountDetail || activeProviderMeta.label">
              {{ activeProviderMeta.label }}
              <template v-if="activeAccountDetail">· {{ activeAccountDetail }}</template>
            </span>
          </div>
          <button type="button" class="current-drive-refresh" title="刷新账号信息" aria-label="刷新账号信息" :disabled="refreshing" @click="handleRefreshUserInfo">
            <RefreshCw :size="14" :class="{ rotating: refreshing }" />
          </button>
        </div>

        <div v-if="activeQuotaStats.total > 0" class="current-drive-quota" :title="activeQuotaText">
          <div class="current-drive-quota-title">
            <span>空间占用</span>
            <strong>{{ Math.round(activeQuotaPercent * 100) }}%</strong>
          </div>
          <a-progress :percent="activeQuotaPercent" :show-text="false" size="small" />
          <div class="current-drive-quota-stats">
            <div>
              <span>已用</span>
              <strong>{{ humanSize(activeQuotaStats.used) }}</strong>
            </div>
            <div>
              <span>剩余</span>
              <strong>{{ humanSize(activeQuotaStats.remaining) }}</strong>
            </div>
            <div>
              <span>总容量</span>
              <strong>{{ humanSize(activeQuotaStats.total) }}</strong>
            </div>
          </div>
        </div>
        <div v-else class="current-drive-quota-empty">{{ activeQuotaText }}</div>
      </div>
    </template>
  </a-popover>

  <button v-else type="button" class="current-drive-add" @click="handleAddAccount">
    <IconFont name="iconadd" />
    <span>添加网盘</span>
  </button>
</template>

<style>
.current-drive-trigger,
.current-drive-add {
  display: inline-flex;
  align-items: center;
  min-width: 0;
  height: 32px;
  padding: 0 8px;
  color: var(--text-primary);
  font: inherit;
  background: transparent;
  border: 0;
  border-radius: var(--ui-control-radius);
  cursor: pointer;
}

.current-drive-trigger:hover,
.current-drive-trigger:focus-visible,
.current-drive-add:hover,
.current-drive-add:focus-visible {
  background: var(--bg-hover);
  outline: none;
}

.current-drive-trigger {
  position: relative;
  justify-content: center;
  width: 40px;
  padding: 0;
}

.current-drive-add {
  gap: 6px;
  color: var(--text-link);
  font-size: 12px;
  font-weight: 600;
}

.current-drive-avatar {
  color: var(--text-inverse);
  font-size: 11px;
  font-weight: 650;
  background: var(--color-primary);
}

.current-drive-avatar img,
.current-drive-badge img,
.current-drive-panel-avatar img {
  display: block;
  width: 100%;
  height: 100%;
  object-fit: contain;
}

.current-drive-badge {
  position: absolute;
  right: 1px;
  bottom: 0;
  display: grid;
  width: 15px;
  height: 15px;
  place-items: center;
  padding: 1px;
  background: var(--bg-surface);
  border: 1px solid var(--border-light);
  border-radius: 50%;
}

.current-drive-panel {
  box-sizing: border-box;
  width: min(300px, calc(100vw - 24px));
  padding: 4px 2px 2px;
  color: var(--text-primary);
}

.current-drive-panel-head {
  display: flex;
  align-items: center;
  min-width: 0;
  padding: 4px 2px 12px;
  gap: 10px;
}

.current-drive-panel-avatar {
  display: grid;
  width: 34px;
  height: 34px;
  flex: 0 0 34px;
  place-items: center;
  overflow: hidden;
  color: var(--text-secondary);
  font-size: 13px;
  font-weight: 650;
  background: var(--bg-subtle);
  border: 1px solid var(--border-light);
  border-radius: var(--ui-control-radius);
}

.current-drive-panel-avatar img {
  width: 24px;
  height: 24px;
}

.current-drive-identity {
  display: grid;
  flex: 1 1 auto;
  min-width: 0;
  gap: 3px;
}

.current-drive-identity strong,
.current-drive-identity span {
  display: block;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.current-drive-identity strong {
  color: var(--text-primary);
  font-size: 14px;
  font-weight: 650;
  line-height: 20px;
}

.current-drive-identity span {
  color: var(--text-secondary);
  font-size: 12px;
  line-height: 18px;
}

.current-drive-refresh {
  display: inline-grid;
  width: 28px;
  height: 28px;
  flex: 0 0 28px;
  place-items: center;
  padding: 0;
  color: var(--text-secondary);
  background: transparent;
  border: 0;
  border-radius: var(--ui-control-radius);
  cursor: pointer;
}

.current-drive-refresh:hover,
.current-drive-refresh:focus-visible {
  color: var(--text-primary);
  background: var(--bg-hover);
  outline: none;
}

.current-drive-refresh:disabled {
  cursor: default;
  opacity: 0.55;
}

.current-drive-refresh .rotating {
  animation: current-drive-refresh-spin 700ms linear infinite;
}

.current-drive-quota {
  padding: 12px 2px 4px;
  border-top: 1px solid var(--border-light);
}

.current-drive-quota-title {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 7px;
  color: var(--text-secondary);
  font-size: 12px;
  line-height: 18px;
}

.current-drive-quota-title strong {
  color: var(--text-primary);
  font-weight: 600;
}

.current-drive-quota .arco-progress-line {
  margin: 0;
}

.current-drive-quota-stats {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  margin-top: 10px;
  gap: 12px;
}

.current-drive-quota-stats > div {
  display: grid;
  min-width: 0;
  gap: 2px;
}

.current-drive-quota-stats span,
.current-drive-quota-stats strong {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.current-drive-quota-stats span {
  color: var(--text-tertiary);
  font-size: 11px;
  line-height: 16px;
}

.current-drive-quota-stats strong {
  color: var(--text-primary);
  font-size: 12px;
  font-weight: 600;
  line-height: 18px;
}

.current-drive-quota-empty {
  padding: 12px 2px 4px;
  color: var(--text-secondary);
  font-size: 12px;
  line-height: 18px;
  border-top: 1px solid var(--border-light);
}

@keyframes current-drive-refresh-spin {
  to {
    transform: rotate(360deg);
  }
}
</style>
