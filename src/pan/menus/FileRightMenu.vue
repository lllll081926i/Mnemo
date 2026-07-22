<script setup lang="ts">
import {
  menuCopyFileName,
  menuCopySelectedFile,
  menuCreatShare,
  menuDownload,
  menuFavSelectFile,
  menuFileColorChange,
  menuJumpToDir,
  menuTrashSelectFile,
} from '../topbtns/topbtn'
import { modalRename, modalShuXing } from '../../utils/modal'
import { computed } from 'vue'
import useCurrentDriveProvider from '../useCurrentDriveProvider'
import { usePanFileStore, usePanTreeStore, useSettingStore } from '../../store'
import PanDAL from '../pandal'

let istree = false

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
  inputselectType: {
    type: String,
    required: true
  },
  inputpicType: {
    type: String,
    required: true
  }
})

const isShowBtn = computed(() => {
  return (props.dirtype === 'pic' && props.inputpicType != 'mypic') || props.dirtype === 'mypic' || props.dirtype === 'pan'
})
const isPic = computed(() => {
  return props.dirtype === 'pic' && props.inputpicType == 'mypic'
})
const { provider, capabilities } = useCurrentDriveProvider()
const panFileStore = usePanFileStore()
const panTreeStore = usePanTreeStore()
const settingStore = useSettingStore()
const selectedItems = computed(() => panFileStore.GetSelected())
const allInLocalShortcut = computed(() => {
  void panTreeStore.quickData
  return selectedItems.value.length > 0 && selectedItems.value.every((item) => PanDAL.hasQuickFile(item, true))
})
const localShortcutMode = computed(() => !capabilities.value.favorite)
const localTagMode = computed(() => !capabilities.value.colorTag)
const tagMenuTitle = computed(() => localTagMode.value ? '本地标签' : '标签')
const clearTagTitle = computed(() => localTagMode.value ? '清除本地标签' : '清除标签')
const canCreateShare = computed(() => capabilities.value.createShare)
</script>

<template>
  <a-dropdown id="rightpanmenu" class="rightmenu" :popup-visible="true" style="z-index: -1; left: -200px; opacity: 0">
    <template #content>
      <a-doption v-if="capabilities.download" @click="() => menuDownload(istree)">
        <template #icon><IconFont name="icondownload" /></template>
        <template #default>下载</template>
      </a-doption>
      <a-doption v-if="canCreateShare" @click="() => menuCreatShare(istree, 'pan', 'resource_root')">
        <template #icon><IconFont name="iconfenxiang" /></template>
        <template #default>分享</template>
      </a-doption>
      <a-doption v-if="!isPic && capabilities.favorite && !isallfavored" @click="() => menuFavSelectFile(istree, true)">
        <template #icon><IconFont name="iconcrown" /></template>
        <template #default>收藏</template>
      </a-doption>
      <a-doption v-if="!isPic && capabilities.favorite && isallfavored" @click="() => menuFavSelectFile(istree, false)">
        <template #icon><IconFont name="iconcrown2" /></template>
        <template #default>取消收藏</template>
      </a-doption>
      <a-doption v-if="!isPic && localShortcutMode && !allInLocalShortcut" @click="() => menuFavSelectFile(istree, true)">
        <template #icon><IconFont name="iconcrown" /></template>
        <template #default>加入快捷方式</template>
      </a-doption>
      <a-doption v-if="!isPic && localShortcutMode && allInLocalShortcut" @click="() => menuFavSelectFile(istree, false)">
        <template #icon><IconFont name="iconcrown2" /></template>
        <template #default>移出快捷方式</template>
      </a-doption>
      <a-dsubmenu v-if="!isPic" class="rightmenu" trigger="hover">
        <template #default>
          <span class="arco-dropdown-option-icon"><IconFont name="iconwbiaoqian" /></span>
          {{ tagMenuTitle }}
        </template>
        <template #content>
          <a-doption v-for="color in settingStore.uiFileColorArray" :key="color.key" @click="() => menuFileColorChange(istree, color.key)">
            <template #icon><span class="local-tag-swatch" :style="{ backgroundColor: color.key }"></span></template>
            <template #default>{{ color.title || color.key }}</template>
          </a-doption>
          <a-doption @click="() => menuFileColorChange(istree, '')">
            <template #icon><IconFont name="iconclose" /></template>
            <template #default>{{ clearTagTitle }}</template>
          </a-doption>
        </template>
      </a-dsubmenu>

      <a-dsubmenu v-if="dirtype != 'video'" id="rightpansubmove" class="rightmenu" trigger="hover">
        <template #default>
          <div @click.stop="() => {}">
            <span class="arco-dropdown-option-icon">
              <IconFont name="iconmoveto" style="opacity: 0.8" />
            </span>
            操作
          </div>
        </template>
        <template #content>
          <a-doption v-if="isShowBtn && capabilities.move" @click="() => menuCopySelectedFile(istree, 'cut')">
            <template #icon><IconFont name="iconscissor" /></template>
            <template #default>移动到...</template>
          </a-doption>
          <a-doption v-if="isShowBtn && capabilities.copy" @click="() => menuCopySelectedFile(istree, 'copy')">
            <template #icon><IconFont name="iconcopy" /></template>
            <template #default>复制到...</template>
          </a-doption>
          <a-doption v-if="capabilities.recycleBin && ((isShowBtn && dirtype !== 'mypic') || dirtype === 'search')" class="danger" @click="() => menuTrashSelectFile(istree, false, isPic)">
            <template #icon><IconFont name="icondelete" /></template>
            <template #default>放回收站</template>
          </a-doption>
          <a-dsubmenu v-if="dirtype !== 'mypic' && capabilities.permanentDelete" class="rightmenu" trigger="hover">
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
      </a-dsubmenu>

      <a-doption v-if="dirtype != 'video' && capabilities.rename" @click="() => modalRename(istree, isselectedmulti, dirtype.includes('pic'))">
        <template #icon><IconFont name="iconedit-square" /></template>
        <template #default>重命名</template>
      </a-doption>

      <a-doption v-if="!isPic" @click="() => modalShuXing(istree, dirtype.includes('pic'))">
        <template #icon><IconFont name="iconshuxing" /></template>
        <template #default>属性</template>
      </a-doption>
      <a-dsubmenu v-if="!dirtype.includes('pic')" id="rightpansubmore" class="rightmenu" trigger="hover">
        <template #default>
          <div @click.stop="() => {}">
            <span class="arco-dropdown-option-icon">
              <IconFont name="icongengduo1" style="opacity: 0.8" />
            </span>
            更多
          </div>
        </template>
        <template #content>
          <a-doption v-if="isselected && !isselectedmulti && (dirtype == 'favorite' || dirtype == 'search' || dirtype == 'color' || dirtype == 'video')" @click="() => menuJumpToDir()">
            <template #icon><IconFont name="icondakaiwenjianjia1" /></template>
            <template #default>打开位置</template>
          </a-doption>
          <a-doption v-if="isselected" @click="() => menuCopyFileName()">
            <template #icon><IconFont name="iconlist" /></template>
            <template #default>复制文件名</template>
          </a-doption>
        </template>
      </a-dsubmenu>
    </template>
  </a-dropdown>
</template>
