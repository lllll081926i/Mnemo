<script setup lang="ts">
import { menuTrashSelectFile, topRestoreSelectedFile, topTrashDeleteAll } from '../topbtns/topbtn'
import useCurrentDriveProvider from '../useCurrentDriveProvider'

defineProps({
  dirtype: {
    type: String,
    required: true
  },
  isselected: {
    type: Boolean,
    required: true
  }
})

const { capabilities } = useCurrentDriveProvider()
</script>

<template>
  <div v-if="dirtype === 'trash' && !isselected && capabilities.trashClear" class="toppanbtn">
    <a-button type="text" size="small" tabindex="-1" class="danger" @click="topTrashDeleteAll">
      <IconFont name="iconqingkong" />
      清空回收站
    </a-button>
  </div>
  <div v-if="dirtype === 'trash' && isselected && (capabilities.trashRestore || capabilities.trashPurge)" class="toppanbtn">
    <a-button v-if="capabilities.trashRestore" type="text" size="small" tabindex="-1" @click="topRestoreSelectedFile">
      <IconFont name="iconrecover" />
      还原选中
    </a-button>
    <a-button v-if="capabilities.trashPurge" type="text" size="small" tabindex="-1" class="danger" @click="() => menuTrashSelectFile(false, true)">
      <IconFont name="iconrest" />
      彻底删除
    </a-button>
  </div>

</template>
