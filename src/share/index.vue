<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import MyShareRight from './share/MyShareRight.vue'
import OtherShareRight from './share/OtherShareRight.vue'
import ShareHistoryRight from './share/ShareHistoryRight.vue'
import { useAppStore, useMyShareStore, useUserStore } from '../store'
import ShareDAL, { type IShareAccountSummary } from './share/ShareDAL'

const appStore = useAppStore()
const userStore = useUserStore()
const myShareStore = useMyShareStore()
const shareAccounts = ref<IShareAccountSummary[]>([])
const activeMenuKey = ref('overview')

const createdShareAccounts = computed(() => shareAccounts.value.filter((account) => account.capabilities.manageCreatedShares))
const importedShareAccounts = computed(() => shareAccounts.value.filter((account) => account.capabilities.manageImportedShares))
const historyAccounts = computed(() => shareAccounts.value.filter((account) => account.capabilities.shareHistory))
const hasImportedShares = computed(() => importedShareAccounts.value.length > 0)
const activeHistoryAccountId = computed(() => (activeMenuKey.value.startsWith('history:') ? activeMenuKey.value.slice('history:'.length) : ''))

const accountMenuKey = (userId: string) => `account:${userId}`
const historyMenuKey = (userId: string) => `history:${userId}`
const accountShareCount = (userId: string) => myShareStore.ListDataRaw.filter((item) => item.account_id == userId).length

const setWorkspaceTab = (tab: string) => {
  if (appStore.GetAppTabMenu != tab) appStore.toggleTabMenu('share', tab)
}

const applyActiveMenu = async (force: boolean = false) => {
  const key = activeMenuKey.value
  if (key == 'overview') {
    myShareStore.mSetAccountFilter('')
    setWorkspaceTab('MyShareRight')
    return
  }
  if (key == 'imported') {
    setWorkspaceTab('OtherShareRight')
    await ShareDAL.aReloadOtherShare()
    return
  }
  if (key.startsWith('account:')) {
    myShareStore.mSetAccountFilter(key.slice('account:'.length))
    setWorkspaceTab('MyShareRight')
    return
  }
  if (key.startsWith('history:')) {
    const accountId = key.slice('history:'.length)
    setWorkspaceTab('ShareHistoryRight')
    await ShareDAL.aReloadShareHistory(accountId, force)
    return
  }
  if (key == 'unsupported') setWorkspaceTab('ShareUnsupported')
}

const normalizeActiveMenu = () => {
  const accountId = activeMenuKey.value.startsWith('account:') ? activeMenuKey.value.slice('account:'.length) : ''
  const historyId = activeMenuKey.value.startsWith('history:') ? activeMenuKey.value.slice('history:'.length) : ''
  if (accountId && !createdShareAccounts.value.some((account) => account.user_id == accountId)) activeMenuKey.value = 'overview'
  if (historyId && !historyAccounts.value.some((account) => account.user_id == historyId)) activeMenuKey.value = 'overview'

  if (createdShareAccounts.value.length == 0) {
    if (hasImportedShares.value) activeMenuKey.value = 'imported'
    else if (historyAccounts.value.length > 0) activeMenuKey.value = historyMenuKey(historyAccounts.value[0].user_id)
    else activeMenuKey.value = 'unsupported'
  } else if (activeMenuKey.value == 'unsupported') {
    activeMenuKey.value = 'overview'
  }
}

const loadShareWorkspace = async (force: boolean = false) => {
  shareAccounts.value = await ShareDAL.aReloadAllMyShare(force)
  normalizeActiveMenu()
  await applyActiveMenu(force)
}

const handleMenuSelect = async (keys: string[]) => {
  activeMenuKey.value = keys[0]
  await applyActiveMenu(false)
}

watch(
  [() => appStore.appTab, () => userStore.user_id],
  ([activeTab]) => {
    if (activeTab == 'share') loadShareWorkspace(false)
  },
  { immediate: true }
)
</script>

<template>
  <a-layout class="ui-workspace-shell">
    <a-layout-sider hide-trigger :width="220" class="xbyleft ui-workspace-rail share-rail">
      <a-menu :selected-keys="[activeMenuKey]" class="xbyleftmenu rail-menu share-account-menu" @update:selected-keys="handleMenuSelect">
        <template v-if="createdShareAccounts.length > 0 || hasImportedShares">
          <div class="share-rail-group">总览</div>
          <a-menu-item v-if="createdShareAccounts.length > 0" key="overview">
            <template #icon><IconFont name="iconfenxiang" /></template>
            <span class="share-menu-label">全部分享</span>
            <span class="share-menu-count">{{ myShareStore.ListDataRaw.length }}</span>
          </a-menu-item>
          <a-menu-item v-if="hasImportedShares" key="imported">
            <template #icon><IconFont name="iconfenxiang1" /></template>
            <span class="share-menu-label">导入链接</span>
          </a-menu-item>
        </template>

        <div v-for="account in shareAccounts" :key="account.user_id" class="share-account-group">
          <div class="share-account-heading" :title="`${account.providerLabel} · ${account.name}`">
            <img v-if="account.icon" :src="account.icon" alt="" />
            <IconFont v-else name="iconfenxiang" />
            <span>{{ account.providerLabel }} · {{ account.name }}</span>
          </div>
          <a-menu-item v-if="account.capabilities.manageCreatedShares" :key="accountMenuKey(account.user_id)">
            <template #icon><IconFont name="iconlink2" /></template>
            <span class="share-menu-label">分享预览</span>
            <span class="share-menu-count">{{ accountShareCount(account.user_id) }}</span>
          </a-menu-item>
          <a-menu-item v-if="account.capabilities.shareHistory" :key="historyMenuKey(account.user_id)">
            <template #icon><IconFont name="iconfenxiang1" /></template>
            <span class="share-menu-label">历史导入</span>
          </a-menu-item>
        </div>
      </a-menu>
    </a-layout-sider>

    <a-layout-content class="xbyright ui-workspace-content">
      <a-tabs type="text" direction="horizontal" class="hidetabs" :justify="true" :active-key="appStore.GetAppTabMenu">
        <a-tab-pane v-if="createdShareAccounts.length > 0" key="MyShareRight" title="shares">
          <MyShareRight />
        </a-tab-pane>
        <a-tab-pane v-if="hasImportedShares" key="OtherShareRight" title="imported">
          <OtherShareRight />
        </a-tab-pane>
        <a-tab-pane v-if="activeHistoryAccountId" key="ShareHistoryRight" title="history">
          <ShareHistoryRight :account-id="activeHistoryAccountId" />
        </a-tab-pane>
        <a-tab-pane v-if="shareAccounts.length == 0" key="ShareUnsupported" title="unsupported">
          <div class="workspace-empty-state">暂无可管理分享的账号</div>
        </a-tab-pane>
      </a-tabs>
    </a-layout-content>
  </a-layout>
</template>

<style scoped>
.share-rail-group {
  padding: 12px 16px 4px;
  color: var(--text-tertiary);
  font-size: 11px;
  font-weight: 600;
}

.share-rail {
  flex: 0 0 clamp(208px, 19vw, 248px) !important;
  width: clamp(208px, 19vw, 248px) !important;
  min-width: 208px !important;
  max-width: 248px !important;
  overflow-x: hidden !important;
}

.share-account-menu {
  width: 100%;
  min-width: 0;
  overflow-x: hidden;
}

.share-account-menu :deep(.arco-menu-inner) {
  width: 100%;
  min-width: 0;
  box-sizing: border-box;
}

.share-account-menu :deep(.arco-menu-item) {
  min-width: 0;
  gap: 0;
}

.share-account-menu :deep(.arco-menu-item .arco-menu-icon) {
  flex: 0 0 22px;
}

.share-account-group {
  padding-top: 8px;
  border-top: 1px solid var(--border-lighter);
}

.share-account-heading {
  display: flex;
  align-items: center;
  gap: 7px;
  min-width: 0;
  padding: 4px 16px 3px;
  color: var(--text-secondary);
  font-size: 11px;
}

.share-account-heading img {
  width: 16px;
  height: 16px;
  flex: 0 0 16px;
  object-fit: contain;
}

.share-account-heading span {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.share-menu-label {
  flex: 1 1 auto;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.share-menu-count {
  flex: 0 0 auto;
  margin-left: auto;
  color: var(--text-tertiary);
  font-size: 11px;
  font-variant-numeric: tabular-nums;
}
</style>
