<script setup lang="ts">
import { menuCopySelectedFile, menuCreatShare, menuDownload, menuTrashSelectFile } from '../topbtns/topbtn'
import { modalRename, modalShuXing } from '../../utils/modal'
import PanDAL from '../pandal'
import { usePanTreeStore } from '../../store'
import TreeStore from '../../store/treestore'
import { computed } from 'vue'
import useCurrentDriveProvider from '../useCurrentDriveProvider'

const istree = true
const pantreeStore = usePanTreeStore()
const { capabilities } = useCurrentDriveProvider()
const isShareSupported = computed(() => capabilities.value.createShare)
const canDelete = computed(() => capabilities.value.recycleBin || capabilities.value.permanentDelete)

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
      <a-doption v-if="capabilities.download" @click="() => menuDownload(istree)">
        <template #icon> <IconFont name="icondownload" /> </template>
        <template #default>下载</template>
      </a-doption>
      <a-doption v-if="isShareSupported"
                 @click="() => menuCreatShare(istree, 'pan', 'resource_root')">
        <template #icon><IconFont name="iconfenxiang" /></template>
        <template #default>分享</template>
      </a-doption>

      <a-dsubmenu v-if="capabilities.move || capabilities.copy || canDelete" id="leftpansubmove" class="rightmenu" trigger="hover">
        <template #default>
          <div @click.stop="() => {}">
            <span class="arco-dropdown-option-icon"><IconFont name="iconmoveto" style="opacity: 0.8" /></span>移动
          </div>
        </template>
        <template #content>
          <a-doption v-if="capabilities.move" @click="() => menuCopySelectedFile(istree, 'cut')">
            <template #icon> <IconFont name="iconscissor" /> </template>
            <template #default>移动到...</template>
          </a-doption>
          <a-doption v-if="capabilities.copy" @click="() => menuCopySelectedFile(istree, 'copy')">
            <template #icon> <IconFont name="iconcopy" /> </template>
            <template #default>复制到...</template>
          </a-doption>
          <a-doption v-if="canDelete" class="danger" @click="() => menuTrashSelectFile(istree, capabilities.permanentDelete && !capabilities.recycleBin)">
            <template #icon> <IconFont name="icondelete" /> </template>
            <template #default>{{ capabilities.recycleBin ? '放回收站' : '彻底删除' }}</template>
          </a-doption>
        </template>
      </a-dsubmenu>

      <a-doption v-if="capabilities.rename" @click='() => modalRename(istree, false, false)'>
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

