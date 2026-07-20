<script setup lang="ts">
import { menuTrashSelectFile, topRestoreSelectedFile } from '../topbtns/topbtn'
import useCurrentDriveProvider from '../useCurrentDriveProvider'

defineProps({
  dirtype: {
    type: String,
    required: true
  }
})
const { capabilities } = useCurrentDriveProvider()
</script>

<template>
  <a-dropdown id="rightpantrashmenu" class="rightmenu" :popup-visible="true" style="z-index: -1; left: -200px; opacity: 0">
    <template #content>
      <a-doption v-if="dirtype === 'trash' && capabilities.trashRestore" @click="topRestoreSelectedFile">
        <template #icon><IconFont name="iconrecover" /></template>
        <template #default>还原选中</template>
      </a-doption>

      <a-doption v-if="dirtype === 'trash' && capabilities.trashPurge" @click="() => menuTrashSelectFile(false, true)">
        <template #icon><IconFont name="iconrest" /></template>
        <template #default>彻底删除</template>
      </a-doption>
    </template>
  </a-dropdown>
</template>
