<script setup lang="ts">
import { watch } from 'vue'
import { useAppStore, useUserStore } from '../store'

const appStore = useAppStore()
const userStore = useUserStore()

watch(
  [() => appStore.appTab, () => userStore.user_id],
  ([activeTab]) => {
    if (activeTab == 'share' && appStore.GetAppTabMenu != 'ShareUnsupported') {
      appStore.toggleTabMenu('share', 'ShareUnsupported')
    }
  },
  { immediate: true }
)
</script>

<template>
  <a-layout class="ui-workspace-shell">
    <a-layout-content class="mnemoright ui-workspace-content">
      <div class="workspace-empty-state share-empty">
        <IconFont name="iconfenxiang" class="share-empty-icon" />
        <div class="share-empty-title">创建分享链接</div>
        <div class="share-empty-desc">在「网盘」中选中文件或文件夹后，使用顶部「分享」按钮创建链接。当前版本不提供分享列表管理与导入。</div>
      </div>
    </a-layout-content>
  </a-layout>
</template>

<style scoped>
.share-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 12px;
  min-height: 60vh;
  padding: 32px;
  text-align: center;
  color: var(--color-text-2);
}
.share-empty-icon {
  font-size: 40px;
  opacity: 0.7;
}
.share-empty-title {
  font-size: 18px;
  font-weight: 600;
  color: var(--color-text-1);
}
.share-empty-desc {
  max-width: 420px;
  line-height: 1.6;
  font-size: 13px;
}
</style>
