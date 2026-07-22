<script setup lang="ts">
import { computed, ref } from 'vue'
import { handleUpload } from '../topbtns/topbtn'
import { modalCreatNewDir } from '../../utils/modal'
import PanDAL from '../pandal'
import useCurrentDriveProvider from '../useCurrentDriveProvider'

const props = defineProps({
  dirtype: {
    type: String,
    required: true
  },
  inputselectType: {
    type: String,
    required: true
  },
  inputpicType: {
    type: String,
    required: true
  },
  isselected: {
    type: Boolean,
    required: true
  }
})

const videoSelectType = ref('recent')
const { capabilities } = useCurrentDriveProvider()

const isShowBtn = computed(() => {
  return (props.dirtype === 'pic' && props.inputpicType != 'mypic') || props.dirtype === 'mypic' || props.dirtype === 'pan'
})

const handleSelectAllCompilation = () => {
  videoSelectType.value = 'allComp'
  PanDAL.aReLoadOneDirToShow('', 'video.compilation', false)
}
const handleSelectRecentPlay = () => {
  videoSelectType.value = 'recent'
  PanDAL.aReLoadOneDirToShow('', 'video.recentplay', false)
}
</script>

<template>
  <div v-if="!isselected && dirtype == 'video'" class="toppanbtn" tabindex="-1">
    <a-space direction="horizontal">
      <a-button size="small" tabindex="-1" :type="videoSelectType === 'recent' ? 'secondary' : 'dashed'" @click="handleSelectRecentPlay">
        <IconFont name="iconfile_video" />
        正在观看
      </a-button>
      <a-button size="small" tabindex="-1" :type="videoSelectType === 'allComp' ? 'secondary' : 'dashed'" @click="handleSelectAllCompilation">
        <IconFont name="iconrss_video" />
        全部专辑
      </a-button>
    </a-space>
  </div>
  <div v-if="!isselected && ['pan', 'pic', 'mypic'].includes(dirtype)" class="toppanbtn">
    <a-dropdown v-if="dirtype !== 'pic' && capabilities.createFolder" trigger="hover" class="rightmenu" position="bl">
      <a-button type="text" size="small" tabindex="-1">
        <IconFont name="iconplus" />
        新建
        <IconFont name="icondown" />
      </a-button>
      <template #content>
        <a-dgroup title="普通新建">
          <a-doption v-if="capabilities.createFolder" value="newfolder" title="Ctrl+Shift+N" @click="() => modalCreatNewDir('folder')">
            <template #icon><IconFont name="iconfile-folder" /></template>
            <template #default>新建文件夹</template>
          </a-doption>
          <a-doption v-if="capabilities.createDateFolder" value="newdatefolder" @click="() => modalCreatNewDir('datefolder')">
            <template #icon><IconFont name="iconfolderadd" /></template>
            <template #default>日期+序号</template>
          </a-doption>
        </a-dgroup>
      </template>
    </a-dropdown>
    <a-dropdown v-if="isShowBtn && !dirtype.includes('pic') && capabilities.upload" trigger="hover" class="rightmenu" position="bl">
      <a-button type="text" size="small" tabindex="-1">
        <IconFont name="iconupload" />
        上传
        <IconFont name="icondown" />
      </a-button>
      <template #content>
        <a-dgroup title="普通上传">
          <a-doption value="uploadfile" title="Ctrl+U" @click="() => handleUpload('file')">
            <template #icon><IconFont name="iconwenjian" /></template>
            <template #default>上传文件</template>
          </a-doption>
          <a-doption value="uploaddir" title="Ctrl+Shift+U" @click="() => handleUpload('folder')">
            <template #icon><IconFont name="iconfile-folder" /></template>
            <template #default>上传文件夹</template>
          </a-doption>
        </a-dgroup>
      </template>
    </a-dropdown>
  </div>
</template>
