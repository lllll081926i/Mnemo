<script setup lang="ts">
import { computed } from 'vue'
import { usePanFileStore, usePanTreeStore } from '../../store'
import { isAliyunUser as isAliyunAccountUser, isBoxUser, isDropboxUser, isGuangyaUser, isOneDriveUser } from '../../aliapi/utils'
import { isWebDavDrive } from '../../utils/webdavClient'
import { isS3Drive } from '../../utils/s3Client'

import {
  menuAddAlbumSelectFile,
  menuCopyFileName,
  menuCopyFileTree,
  menuCopySelectedFile,
  menuCreatShare,
  menuDLNA,
  menuDownload,
  menuFavSelectFile,
  menuFileClearHistory,
  menuFileColorChange,
  menuFileEncTypeChange,
  menuJumpToDir,
  menuM3U8Download,
  menuTrashSelectFile,
  menuVideoXBT
} from '../topbtns/topbtn'
import { modalRename, modalShuXing } from '../../utils/modal'

const props = defineProps({
  dirtype: {
    type: String,
    required: true
  },
  isvideo: {
    type: Boolean,
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
const panTreeStore = usePanTreeStore()
const panFileStore = usePanFileStore()
const isAliyunAccount = computed(() => isAliyunAccountUser(panTreeStore.user_id || ''))
const isDropbox = computed(() => isDropboxUser(panTreeStore.user_id || '') || panTreeStore.drive_id === 'dropbox')
const isOneDrive = computed(() => isOneDriveUser(panTreeStore.user_id || '') || panTreeStore.drive_id === 'onedrive')
const isBox = computed(() => isBoxUser(panTreeStore.user_id || '') || panTreeStore.drive_id === 'box')
const isGuangya = computed(() => isGuangyaUser(panTreeStore.user_id || '') || panTreeStore.drive_id === 'guangya')
const isThirdPartyDrive = computed(() => isDropbox.value || isOneDrive.value || isBox.value || isGuangya.value)
const isShareSupported = computed(() => props.inputselectType.includes('resource') || isDropbox.value || isOneDrive.value || isBox.value || isGuangya.value)
const isWebDav = computed(() => isWebDavDrive(panTreeStore.drive_id || panTreeStore.selectDir.drive_id))
const isS3 = computed(() => isS3Drive(panTreeStore.drive_id || panTreeStore.selectDir.drive_id))
const isMountedStorage = computed(() => isWebDav.value || isS3.value)
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
const canMutateSelection = computed(() => isShowBtn.value)
const canSendToTrash = computed(() => !isMountedStorage.value && (canMutateSelection.value || props.dirtype === 'search'))
const canPermanentlyDelete = computed(() => isAliyunAccount.value || isMountedStorage.value)
const canShowDelete = computed(() => canSendToTrash.value || canPermanentlyDelete.value)
const canShowMore = computed(() => hasSelection.value && (canMutateSelection.value || !isPic.value || props.isvideo || props.dirtype === 'mypic'))
const canCreateShare = computed(() => hasSelection.value && !isPic.value && props.dirtype !== 'video' && props.dirtype !== 'search' && isShareSupported.value)
const canCreateQuickTransfer = computed(() => hasSelection.value && !isPic.value && props.dirtype !== 'video' && props.dirtype !== 'search' && isAliyunAccount.value)
</script>

<template>
  <div v-if="hasSelection && dirtype !== 'trash' && dirtype !== 'recover'" class="toppanbtn">
    <a-button v-if="!isPic && dirtype != 'video'" type="text" size="small" tabindex="-1" title="Ctrl+D" @click="() => menuDownload(istree)">
      <IconFont name="icondownload" />
      下载
    </a-button>
    <a-button v-if="canCreateShare" type="text" size="small" tabindex="-1" title="Ctrl+S" @click="() => menuCreatShare(istree, 'pan', 'resource_root')">
      <IconFont name="iconfenxiang" />
      分享
    </a-button>
    <a-button v-if="canCreateQuickTransfer" type="text" size="small" tabindex="-1" title="Ctrl+T" @click="() => menuCreatShare(istree, 'pan', 'backup_root')">
      <IconFont name="iconrss" />
      快传
    </a-button>
    <a-button v-if="!isPic && !isallfavored && isAliyunAccount" type="text" size="small" tabindex="-1" title="Ctrl+G" @click="() => menuFavSelectFile(istree, true)">
      <IconFont name="iconcrown" />
      收藏
    </a-button>
    <a-button v-if="!isPic && isallfavored && isAliyunAccount" type="text" size="small" tabindex="-1" title="Ctrl+G" @click="() => menuFavSelectFile(istree, false)">
      <IconFont name="iconcrown2" />
      取消收藏
    </a-button>
    <a-button v-if="isShowBtn" title="F2 / Ctrl+E" type="text" size="small" tabindex="-1" @click="() => modalRename(istree, isselectedmulti, isPic)">
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
        <a-dsubmenu v-if="isAliyunAccount || isMountedStorage" class="rightmenu" trigger="hover">
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
        <a-doption v-if="hasSelectedFile && inputpicType !== 'mypic' && dirtype === 'pic'" title="Ctrl+X" @click="() => menuAddAlbumSelectFile()">
          <template #icon><IconFont name="iconscissor" /></template>
          <template #default>移入相册</template>
        </a-doption>
        <a-doption v-if="hasSelectedFile && dirtype === 'mypic'" title="Ctrl+X" @click="() => menuTrashSelectFile(istree, false, true)">
          <template #icon><IconFont name="iconscissor" /></template>
          <template #default>移出相册</template>
        </a-doption>
        <a-doption v-if="canMutateSelection" title="Ctrl+X" @click="() => menuCopySelectedFile(istree, 'cut')">
          <template #icon><IconFont name="iconscissor" /></template>
          <template #default>移动到...</template>
        </a-doption>
        <a-doption v-if="canMutateSelection" title="Ctrl+C" @click="() => menuCopySelectedFile(istree, 'copy')">
          <template #icon><IconFont name="iconcopy" /></template>
          <template #default>复制到...</template>
        </a-doption>
        <a-doption v-if="!isPic" title="Ctrl+P" @click="() => modalShuXing(istree, dirtype.includes('pic'))">
          <template #icon><IconFont name="iconshuxing" /></template>
          <template #default>属性</template>
        </a-doption>
        <a-doption v-if="isvideo" @click="() => menuVideoXBT()">
          <template #icon><IconFont name="iconjietu" /></template>
          <template #default>雪碧图</template>
        </a-doption>
        <a-doption v-if="canMutateSelection && isAliyunAccount" type="text" size="small" tabindex="-1" title="Ctrl+M" @click="() => menuFileEncTypeChange(istree)">
          <template #icon><IconFont name="iconsafebox" /></template>
          <template #default>标记加密</template>
        </a-doption>
        <a-doption v-if="canMutateSelection && isallcolored && isAliyunAccount" type="text" size="small" tabindex="-1" title="Ctrl+M" @click="() => menuFileClearHistory(istree)">
          <template #icon><IconFont name="iconshipin" /></template>
          <template #default>清除历史</template>
        </a-doption>
        <a-doption v-if="canMutateSelection && isallcolored && !isMountedStorage && !isThirdPartyDrive" type="text" size="small" tabindex="-1" title="Ctrl+M" @click="() => menuFileColorChange(istree, '')">
          <template #icon><IconFont name="iconfangkuang" /></template>
          <template #default>清除标记</template>
        </a-doption>
        <a-doption v-if="isvideo" @click="() => menuDLNA()">
          <template #icon><IconFont name="icontouping2" /></template>
          <template #default>DLNA投屏</template>
        </a-doption>
        <a-doption v-if="isvideo" @click="() => menuM3U8Download()">
          <template #icon><IconFont name="iconluxiang" /></template>
          <template #default>M3U8下载</template>
        </a-doption>
        <a-doption v-if="hasSelection" @click="() => menuCopyFileName()">
          <template #icon><IconFont name="iconlist" /></template>
          <template #default>复制文件名</template>
        </a-doption>
        <a-doption v-if="!dirtype.includes('pic') && selectedOneDirectory && !isselectedmulti && isAliyunAccount" @click="() => menuCopyFileTree()">
          <template #icon><IconFont name="iconnode-tree1" /></template>
          <template #default>复制目录树</template>
        </a-doption>
      </template>
    </a-dropdown>
  </div>
</template>
<style></style>
