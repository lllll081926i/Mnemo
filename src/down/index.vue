<script setup lang="ts">
import { useAppStore } from '../store'
import DownDowning from './DownDowning.vue'
import DownDowned from './DownDowned.vue'
import DownUploading from './DownUploading.vue'
import DownUploaded from './DownUploaded.vue'

const appStore = useAppStore()
</script>

<template>
  <a-layout class="ui-workspace-shell">
    <a-layout-sider hide-trigger :width="220" class="xbyleft ui-workspace-rail">
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
.content-body {
  flex: 1;
  min-height: 0;
  overflow: hidden;
}
</style>
