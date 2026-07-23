<script setup lang="ts">
import { computed } from 'vue'
import { usePanFileStore } from '../../store'
import useCurrentDriveProvider from '../useCurrentDriveProvider'

import {
  menuCopyFileName,
  menuCopySelectedFile,
  menuCreatShare,
  menuDownload,
  menuJumpToDir,
  menuTrashSelectFile,
} from '../topbtns/topbtn'
import { modalRename, modalShuXing } from '../../utils/modal'

const props = defineProps({
  dirtype: {
    type: String,
    required: true
  },
  isselected: {
    type: Boolean,
    required: true
  },
  isselectedmulti: {
    type: Boolean,
    required: true
  },
  isallfavored: {
    type: Boolean,
    required: true
  },
  isallcolored: {
    type: Boolean,
    required: true
  },
  inputselectType: {
    type: String,
    required: true
  },
  inputpicType: {
    type: String,
    required: true
  }
})

const istree = false
const panFileStore = usePanFileStore()
const { provider, capabilities } = useCurrentDriveProvider()
const isShowBtn = computed(() => {
  return (props.dirtype === 'pic' && props.inputpicType != 'mypic') || props.dirtype === 'mypic' || ['search', 'color', 'pan'].includes(props.dirtype)
})
const isPic = computed(() => {
  return props.dirtype === 'pic' && props.inputpicType == 'mypic'
})
const selectedItems = computed(() => panFileStore.GetSelected())
const hasSelection = computed(() => props.isselected && selectedItems.value.length > 0)
const selectedOne = computed(() => (selectedItems.value.length === 1 ? selectedItems.value[0] : undefined))
const selectedOneDirectory = computed(() => !!selectedOne.value?.isDir)
const hasSelectedFile = computed(() => selectedItems.value.some((item: any) => !item.isDir))
const canRenameSelection = computed(() => isShowBtn.value && capabilities.value.rename)
const canMoveSelection = computed(() => isShowBtn.value && capabilities.value.move)
const canCopySelection = computed(() => isShowBtn.value && capabilities.value.copy)
const canMutateSelection = computed(() => canRenameSelection.value || canMoveSelection.value || canCopySelection.value)
const canSendToTrash = computed(() => capabilities.value.recycleBin && (canMutateSelection.value || props.dirtype === 'search'))
const canPermanentlyDelete = computed(() => capabilities.value.permanentDelete)
const canShowDelete = computed(() => canSendToTrash.value || canPermanentlyDelete.value)
const canShowMore = computed(() => hasSelection.value && (canMutateSelection.value || !isPic.value || props.dirtype === 'mypic'))
const canCreateShare = computed(() => hasSelection.value && !isPic.value && props.dirtype !== 'video' && props.dirtype !== 'search' && capabilities.value.createShare)
</script>

<template>
  <div v-if="hasSelection && dirtype !== 'trash' && dirtype !== 'recover'" class="toppanbtn">
    <a-button v-if="!isPic && dirtype != 'video' && capabilities.download" type="text" size="small" tabindex="-1" title="Ctrl+D" @click="() => menuDownload(istree)">
      <IconFont name="icondownload" />
      下载
    </a-button>
    <a-button v-if="canCreateShare" type="text" size="small" tabindex="-1" title="Ctrl+S" @click="() => menuCreatShare(istree, 'pan')">
      <IconFont name="iconfenxiang" />
      分享
    </a-button>
    <a-button v-if="canRenameSelection" title="F2 / Ctrl+E" type="text" size="small" tabindex="-1" @click="() => modalRename(istree, isselectedmulti, isPic)">
      <IconFont name="iconedit-square" />
      重命名
    </a-button>
    <a-button v-if="isselected && !isselectedmulti && (dirtype == 'favorite' || dirtype == 'search' || dirtype == 'color' || dirtype == 'trash' || dirtype == 'video')" type="text" size="small" tabindex="-1" title="Ctrl+R" @click="() => menuJumpToDir()">
      <IconFont name="icondakaiwenjianjia1" />
      打开位置
    </a-button>
    <a-dropdown v-if="canShowDelete" trigger="hover" class="rightmenu" position="bl">
      <a-button type="text" size="small" tabindex="-1" class="danger">
        <IconFont name="icondelete" />
        删除
        <IconFont name="icondown" />
      </a-button>
      <template #content>
        <a-doption v-if="canSendToTrash" title="Ctrl+Delete" class="danger" @click="() => menuTrashSelectFile(istree, false, isPic)">
          <template #icon><IconFont name="icondelete" /></template>
          <template #default>放回收站</template>
        </a-doption>
        <a-dsubmenu v-if="canPermanentlyDelete" class="rightmenu" trigger="hover">
          <template #default>
            <span class="arco-dropdown-option-icon"><IconFont name="iconrest" /></span>
            彻底删除
          </template>
          <template #content>
            <a-doption title="Ctrl+Shift+Delete" class="danger" @click="() => menuTrashSelectFile(istree, true, isPic)">
              <template #default>删除后无法还原</template>
            </a-doption>
          </template>
        </a-dsubmenu>
      </template>
    </a-dropdown>

    <a-dropdown v-if="canShowMore" trigger="hover" class="rightmenu" position="bl">
      <a-button type="text" size="small" tabindex="-1">
        更多
        <IconFont name="icondown" />
      </a-button>
      <template #content>
        <a-doption v-if="canMoveSelection" title="Ctrl+X" @click="() => menuCopySelectedFile(istree, 'cut')">
          <template #icon><IconFont name="iconscissor" /></template>
          <template #default>移动到...</template>
        </a-doption>
        <a-doption v-if="canCopySelection" title="Ctrl+C" @click="() => menuCopySelectedFile(istree, 'copy')">
          <template #icon><IconFont name="iconcopy" /></template>
          <template #default>复制到...</template>
        </a-doption>
        <a-doption v-if="!isPic" title="Ctrl+P" @click="() => modalShuXing(istree, dirtype.includes('pic'))">
          <template #icon><IconFont name="iconshuxing" /></template>
          <template #default>属性</template>
        </a-doption>
        <a-doption v-if="hasSelection" @click="() => menuCopyFileName()">
          <template #icon><IconFont name="iconlist" /></template>
          <template #default>复制文件名</template>
        </a-doption>
      </template>
    </a-dropdown>
  </div>
</template>
