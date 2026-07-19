<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useAppStore, useDownedStore, useDowningStore, useUploadedStore, useUploadingStore, useUserStore } from '../store'
import DownDowning from './DownDowning.vue'
import DownDowned from './DownDowned.vue'
import DownUploading from './DownUploading.vue'
import DownUploaded from './DownUploaded.vue'
import { loadDriveAccountOptions, type DriveAccountOption } from '../utils/driveAccount'
import UploadingDAL from '../transfer/uploadingdal'

const appStore = useAppStore()
const userStore = useUserStore()
const downingStore = useDowningStore()
const downedStore = useDownedStore()
const uploadingStore = useUploadingStore()
const uploadedStore = useUploadedStore()
const accounts = ref<DriveAccountOption[]>([])
const activeAccountId = ref('')

const hasExternalTasks = computed(() => [...downingStore.ListDataRaw, ...downedStore.ListDataRaw].some((item) => !item.Info.user_id || item.Info.user_id == 'external'))
const applyAccountFilter = (userId: string) => {
  activeAccountId.value = userId
  downingStore.mSetAccountFilter(userId)
  downedStore.mSetAccountFilter(userId)
  uploadedStore.mSetAccountFilter(userId)
  uploadingStore.mSetAccountFilter(userId)
  if (uploadingStore.showTaskID) UploadingDAL.mUploadingShowTaskBack()
}

watch(
  [() => appStore.appTab, () => userStore.user_id],
  async ([activeTab]) => {
    if (activeTab != 'down') return
    accounts.value = await loadDriveAccountOptions()
    if (activeAccountId.value && !accounts.value.some((account) => account.user_id == activeAccountId.value) && activeAccountId.value != 'external') applyAccountFilter('')
  },
  { immediate: true }
)
</script>

<template>
  <a-layout class="ui-workspace-shell">
    <a-layout-sider hide-trigger :width="192" class="xbyleft ui-workspace-rail transfer-rail">
      <a-menu class="xbyleftmenu rail-menu" :selected-keys="[appStore.GetAppTabMenu]" @update:selected-keys="appStore.toggleTabMenu('down', $event[0])">
        <a-menu-item key="DowningRight">
          <template #icon><IconFont name="icondownload" /></template>
          正在下载
        </a-menu-item>
        <a-menu-item key="DownedRight">
          <template #icon><IconFont name="icondesktop" /></template>
          已下载
        </a-menu-item>
        <a-menu-item key="UploadingRight">
          <template #icon><IconFont name="iconcloud-upload" /></template>
          正在上传
        </a-menu-item>
        <a-menu-item key="UploadedRight">
          <template #icon><IconFont name="iconcloud_success" /></template>
          已上传
        </a-menu-item>
      </a-menu>
      <div class="rail-filter-group">账号筛选</div>
      <div class="rail-filter-list">
        <button type="button" class="rail-filter-item" :class="{ active: !activeAccountId }" @click="applyAccountFilter('')">
          <IconFont name="iconcloud" />
          <span>全部账号</span>
        </button>
        <button v-for="account in accounts" :key="account.user_id" type="button" class="rail-filter-item" :class="{ active: activeAccountId == account.user_id }" :title="`${account.providerLabel} · ${account.name}`" @click="applyAccountFilter(account.user_id)">
          <img v-if="account.icon" :src="account.icon" alt="" />
          <IconFont v-else name="iconcloud" />
          <span>{{ account.providerLabel }} · {{ account.name }}</span>
        </button>
        <button v-if="hasExternalTasks" type="button" class="rail-filter-item" :class="{ active: activeAccountId == 'external' }" @click="applyAccountFilter('external')">
          <IconFont name="iconlink2" />
          <span>外部链接</span>
        </button>
      </div>
    </a-layout-sider>
    <a-layout-content class="xbyright ui-workspace-content">
      <div class="content-body">
        <a-tabs type="text" direction="horizontal" class="hidetabs" :justify="true" :active-key="appStore.GetAppTabMenu">
          <a-tab-pane key="DowningRight" title="1"><DownDowning /></a-tab-pane>
          <a-tab-pane key="DownedRight" title="2"><DownDowned /></a-tab-pane>
          <a-tab-pane key="UploadingRight" title="3"><DownUploading /></a-tab-pane>
          <a-tab-pane key="UploadedRight" title="4"><DownUploaded /></a-tab-pane>
        </a-tabs>
      </div>
    </a-layout-content>
  </a-layout>
</template>

<style scoped>
.transfer-rail {
  --layout-rail-width: 192px;
}

.content-body {
  flex: 1;
  min-height: 0;
  overflow: hidden;
}
</style>
