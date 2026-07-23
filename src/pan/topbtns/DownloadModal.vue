<script lang="ts">
import { modalCloseAll, modalDownload } from '../../utils/modal'
import { computed, defineComponent, ref } from 'vue'
import { Download } from 'lucide-vue-next'
import useSettingStore from "../../setting/settingstore";
import { menuDownload } from './topbtn'
import { isEmpty } from 'lodash'
import { getSystemDownloadsPath } from '../../utils/electronhelper'
import message from '../../utils/message'


export default defineComponent({
  components: { Download },
  props: {
    visible: {
      type: Boolean,
      required: true
    },
    istree: {
      type: Boolean,
      required: true
    },
  },
  setup(props) {
    const okLoading = ref(false)
    const settingStore = useSettingStore()
    const displayDownSavePath = computed(() => settingStore.downSavePath || getSystemDownloadsPath() || '系统默认下载文件夹')

    const handleOpen = () => {}

    const handleClose = async () => {
      modalCloseAll()
    }

    const handleHide = () => {
      modalCloseAll()
    }

    const handleOK = () => {
      const savePath = settingStore.AriaIsLocal ? (settingStore.downSavePath || getSystemDownloadsPath()) : settingStore.ariaSavePath
      if (isEmpty(savePath)) {
        message.error('无法确定下载位置，请先选择下载文件夹')
        return
      }
      menuDownload(props.istree, false)
      modalCloseAll()
    }

    const handleSelectDownSavePath = () => {
      if (window.WebShowOpenDialogSync) {
        window.WebShowOpenDialogSync(
            {
              title: '选择一个文件夹，把所有文件下载到此文件夹内',
              buttonLabel: '选择',
              properties: ['openDirectory', 'createDirectory'],
              defaultPath: displayDownSavePath.value
            },
            (result: string[] | undefined) => {
              if (result && result[0]) {
                settingStore.updateStore({ downSavePath: result[0] })
              }
            }
        )
      }
    }
    return { okLoading, settingStore, displayDownSavePath, handleOpen, handleClose, handleOK, handleHide, handleSelectDownSavePath }
  }
})
</script>

<template>
  <a-modal
      :visible="visible"
      modal-class="modalclass"
      :footer="false"
      :unmount-on-close="true"
      :mask-closable="false"
      @cancel="handleHide"
      @before-open="handleOpen"
      @close="handleClose">
    <template #title>
      <span class="modaltitle">从网盘下载 文件/文件夹 到本地</span>
    </template>
    <div class="mn-modal">
      <div class="mn-hero">
        <div class="mn-hero-icon"><Download :size="20" /></div>
        <div class="mn-hero-text">
          <div class="mn-hero-title">下载选中的文件 / 文件夹</div>
          <div class="mn-hero-sub">从网盘下载到本地文件夹</div>
        </div>
      </div>
      <div class="mn-panel">
        <span class="mn-panel-label">保存到</span>
        <span class="mn-panel-value" :title="displayDownSavePath">{{ displayDownSavePath }}</span>
        <a-button type="outline" size="mini" @click="handleSelectDownSavePath">更改</a-button>
      </div>
      <div class="mn-footer">
        <span class="mn-footer-spacer"></span>
        <a-button v-if="!okLoading" type="outline" size="small" @click="handleHide">取消</a-button>
        <a-button type="primary" size="small" :loading="okLoading" @click="handleOK">开始下载</a-button>
      </div>
    </div>
  </a-modal>
</template>

<style></style>
