<script setup lang="ts">
import { menuCopySelectedFile, menuCreatShare, menuDownload, menuTrashSelectFile } from '../topbtns/topbtn'
import { modalRename, modalShuXing } from '../../utils/modal'
import PanDAL from '../pandal'
import { usePanTreeStore } from '../../store'
import TreeStore from '../../store/treestore'
import { computed } from 'vue'
import { isAliyunUser as isAliyunAccountUser, isBoxUser, isCloud123User, isDropboxUser, isGuangyaUser, isOneDriveUser } from '../../aliapi/utils'

const istree = true
const pantreeStore = usePanTreeStore()
const isCloudUser = computed(() => isCloud123User(pantreeStore.user_id || '') || pantreeStore.drive_id === 'cloud123')
const isAliyunAccount = computed(() => isAliyunAccountUser(pantreeStore.user_id || ''))
const isDropbox = computed(() => isDropboxUser(pantreeStore.user_id || '') || pantreeStore.drive_id === 'dropbox')
const isOneDrive = computed(() => isOneDriveUser(pantreeStore.user_id || '') || pantreeStore.drive_id === 'onedrive')
const isBox = computed(() => isBoxUser(pantreeStore.user_id || '') || pantreeStore.drive_id === 'box')
const isGuangya = computed(() => isGuangyaUser(pantreeStore.user_id || '') || pantreeStore.drive_id === 'guangya')
const isShareSupported = computed(() => props.inputselectType.includes('resource') || isDropbox.value || isOneDrive.value || isBox.value || isGuangya.value)

const props = defineProps({
  inputselectType: {
    type: String,
    required: true
  }
})

const handleRefresh = () => PanDAL.aReLoadOneDirToShow('', 'refresh', false)
const handleExpandAll = (isExpand: boolean) => {
  const drive_id = pantreeStore.drive_id
  const file_id = pantreeStore.selectDir.file_id
  const diridList = (() => {
    const result: string[] = []
    const visited = new Set<string>()
    const stack = [file_id]
    while (stack.length > 0) {
      const current = stack.pop() as string
      const children = TreeStore.GetDirChildDirID(drive_id, current)
      for (let i = 0; i < children.length; i++) {
        const child = children[i]
        if (visited.has(child)) continue
        visited.add(child)
        result.push(child)
        stack.push(child)
      }
    }
    return result
  })()
  pantreeStore.mTreeExpandAll(diridList, isExpand)
}

</script>

<template>
  <a-dropdown id="leftpanmenu" class="rightmenu" :popup-visible="true" style="z-index: -1; left: -200px; opacity: 0">
    <template #content>
      <a-dsubmenu id="leftpansubzhankai" class="rightmenu" trigger="hover">
        <template #default>
          <div @click.stop="() => {}">
            <span class="arco-dropdown-option-icon"><IconFont name="iconfenzhi1" /></span>目录
          </div>
        </template>
        <template #content>
          <a-doption @click="handleRefresh">
            <template #icon> <IconFont name="iconreload-1-icon" /> </template>
            <template #default>刷新</template>
          </a-doption>
          <a-doption @click="() => handleExpandAll(true)">
            <template #icon> <IconFont name="iconArrow-Down2" /> </template>
            <template #default>展开全部</template>
          </a-doption>
          <a-doption @click="() => handleExpandAll(false)">
            <template #icon> <IconFont name="iconArrow-Right2" /> </template>
            <template #default>折叠全部</template>
          </a-doption>
        </template>
      </a-dsubmenu>
      <a-doption @click="() => menuDownload(istree)">
        <template #icon> <IconFont name="icondownload" /> </template>
        <template #default>下载</template>
      </a-doption>
      <a-doption v-show="isShareSupported"
                 @click="() => menuCreatShare(istree, 'pan', 'resource_root')">
        <template #icon><IconFont name="iconfenxiang" /></template>
        <template #default>分享</template>
      </a-doption>
      <a-doption v-if="isAliyunAccount" @click="() => menuCreatShare(istree, 'pan', 'backup_root')">
        <template #icon><IconFont name="iconrss" /></template>
        <template #default>快传</template>
      </a-doption>

      <a-dsubmenu id="leftpansubmove" class="rightmenu" trigger="hover">
        <template #default>
          <div @click.stop="() => {}">
            <span class="arco-dropdown-option-icon"><IconFont name="iconmoveto" style="opacity: 0.8" /></span>移动
          </div>
        </template>
        <template #content>
          <a-doption @click="() => menuCopySelectedFile(istree, 'cut')">
            <template #icon> <IconFont name="iconscissor" /> </template>
            <template #default>移动到...</template>
          </a-doption>
          <a-doption @click="() => menuCopySelectedFile(istree, 'copy')">
            <template #icon> <IconFont name="iconcopy" /> </template>
            <template #default>复制到...</template>
          </a-doption>
          <a-doption class="danger" @click="() => menuTrashSelectFile(istree, false)">
            <template #icon> <IconFont name="icondelete" /> </template>
            <template #default>回收站</template>
          </a-doption>
        </template>
      </a-dsubmenu>

      <a-doption @click='() => modalRename(istree, false, false)'>
        <template #icon><IconFont name="iconedit-square" /></template>
        <template #default>重命名</template>
      </a-doption>

      <a-doption @click='() => modalShuXing(istree)'>
        <template #icon><IconFont name="iconshuxing" /></template>
        <template #default>属性</template>
      </a-doption>
    </template>
  </a-dropdown>
</template>
<style>
.ai-pro-badge { display: inline-flex; align-items: center; justify-content: center; border-radius: 999px; background: linear-gradient(135deg, #f59e0b, #f97316); color: #fff; font-weight: 700; line-height: 1; height: 14px; padding: 0 5px; font-size: 9px; vertical-align: middle; margin-left: 4px; }
</style>
