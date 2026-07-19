<script setup lang="ts">
import {
  menuAddAlbumSelectFile,
  menuCopyFileName,
  menuCopyFileTree,
  menuCopySelectedFile,
  menuCreatShare,
  menuDownload,
  menuFileClearHistory,
  menuFileColorChange,
  menuFileEncTypeChange,
  menuJumpToDir,
  menuM3U8Download,
  menuTrashSelectFile,
  menuVideoXBT
} from '../topbtns/topbtn'
import { modalRename, modalShuXing } from '../../utils/modal'
import { useSettingStore } from '../../store'
import { computed } from 'vue'
import useCurrentDriveProvider from '../useCurrentDriveProvider'

let istree = false
const settingStore = useSettingStore()

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
const canCreateShare = computed(() => capabilities.value.createShare && (provider.value !== 'aliyun' || props.inputselectType.includes('resource')))
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
      <a-doption v-if="capabilities.quickTransfer" @click="() => menuCreatShare(istree, 'pan', 'backup_root')">
        <template #icon><IconFont name="iconrss" /></template>
        <template #default>快传</template>
      </a-doption>

      <a-dsubmenu v-if="dirtype !== 'pic' && capabilities.colorTag" id="rightpansubbiaoji" class="rightmenu" trigger="hover">
        <template #default>
          <div @click.stop="() => {}">
            <span class="arco-dropdown-option-icon">
              <IconFont name="iconwbiaoqian" style="opacity: 0.8" />
            </span>
            标记
          </div>
        </template>
        <template #content>
          <a-doption v-for="item in settingStore.uiFileColorArray" :key="item.key" @click="() => menuFileColorChange(istree, item.key)">
            <template #icon><IconFont name="iconcheckbox-full" :style="{ color: item.key }" /></template>
            <template #default>{{ item.title || item.key }}</template>
          </a-doption>

          <a-doption @click="() => menuFileColorChange(istree, '#e74c3c')">
            <template #icon><IconFont name="iconcheckbox-full" style="color: #e74c3c" /></template>
            <template #default>视频红</template>
          </a-doption>
          <a-doption @click="() => menuFileColorChange(istree, '')">
            <template #icon><IconFont name="iconfangkuang" /></template>
            <template #default>清除标记</template>
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
          <a-doption v-if="isShowBtn && inputpicType !== 'mypic' && dirtype !== 'pan'" @click="() => menuAddAlbumSelectFile()">
            <template #icon><IconFont name="iconmoveto" /></template>
            <template #default>移入相册</template>
          </a-doption>
          <a-doption v-if="dirtype === 'mypic'" @click="() => menuTrashSelectFile(istree, false, true)">
            <template #icon><IconFont name="iconqingkong" /></template>
            <template #default>移出相册</template>
          </a-doption>
          <a-doption v-if="isShowBtn && capabilities.move" @click="() => menuCopySelectedFile(istree, 'cut')">
            <template #icon><IconFont name="iconscissor" /></template>
            <template #default>移动到...</template>
          </a-doption>
          <a-doption v-if="isShowBtn && capabilities.copy" @click="() => menuCopySelectedFile(istree, 'copy')">
            <template #icon><IconFont name="iconcopy" /></template>
            <template #default>复制到...</template>
          </a-doption>
          <a-doption v-if="isShowBtn && capabilities.encryption" type="text" size="small" tabindex="-1" title="Ctrl+M" @click="() => menuFileEncTypeChange(istree)">
            <template #icon><IconFont name="iconsafebox" /></template>
            <template #default>标记加密</template>
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
          <a-doption v-if="isvideo" @click="() => menuVideoXBT()">
            <template #icon><IconFont name="iconjietu" /></template>
            <template #default>雪碧图</template>
          </a-doption>
          <a-doption v-if="isShowBtn && capabilities.encryption" type="text" size="small" tabindex="-1" title="Ctrl+M" @click="() => menuFileEncTypeChange(istree)">
            <template #icon><IconFont name="iconsafebox" /></template>
            <template #default>标记加密</template>
          </a-doption>
          <a-doption v-if="isShowBtn && capabilities.playbackHistory" type="text" size="small" tabindex="-1" title="Ctrl+M" @click="() => menuFileClearHistory(istree)">
            <template #icon><IconFont name="iconshipin" /></template>
            <template #default>清除历史</template>
          </a-doption>
          <a-doption v-if="isvideo" @click="() => menuM3U8Download()">
            <template #icon><IconFont name="iconluxiang" /></template>
            <template #default>转码链接</template>
          </a-doption>
          <a-doption v-if="isselected" @click="() => menuCopyFileName()">
            <template #icon><IconFont name="iconlist" /></template>
            <template #default>复制文件名</template>
          </a-doption>
          <a-doption v-if="isselected && !isselectedmulti && capabilities.copyTree" @click="() => menuCopyFileTree()">
            <template #icon><IconFont name="iconnode-tree1" /></template>
            <template #default>复制目录树</template>
          </a-doption>
        </template>
      </a-dsubmenu>
    </template>
  </a-dropdown>
</template>
