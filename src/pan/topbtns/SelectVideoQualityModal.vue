<script setup lang="ts">
import { modalCloseAll } from '../../utils/modal'
import { PropType, ref } from 'vue'
import { ListVideo } from 'lucide-vue-next'
import MySwitch from '../../layout/MySwitch.vue'
import useSettingStore from '../../setting/settingstore'
import { IAliGetFileModel } from '../../aliapi/alimodels'
import { humanSize } from '../../utils/format'
import { IRawUrl } from '../../utils/proxyhelper'

const props = defineProps({
  visible: {
    type: Boolean,
    required: true
  },
  fileInfo: {
    type: Object as PropType<IAliGetFileModel>,
    required: true
  },
  qualityData: {
    type: Object as PropType<IRawUrl>,
    required: true
  },
  callback: {
    type: Function as PropType<(quality: string) => void>
  }
})

const settingStore = useSettingStore()
const okLoading = ref(false)
const qualitySelect = ref('')
const cb = (val: any) => {
  settingStore.updateStore(val)
}

const handleOpen = () => {
  qualitySelect.value = settingStore.uiVideoQuality
  if (!props.qualityData?.qualities.some(q => q.quality === qualitySelect.value)) {
    qualitySelect.value = props.qualityData?.qualities.find(v => v.width)?.quality || ''
  }
}
const handleHide = () => {
  modalCloseAll()
}
const handleClose = () => {
  qualitySelect.value = ''
  if (okLoading.value) {
    okLoading.value = false
  }
  modalCloseAll()
}
const handleOK = () => {
  okLoading.value = true
  if (settingStore.uiVideoQualityLastSelect) {
    cb({ uiVideoQuality: qualitySelect.value })
  }
  if (props.callback) {
    props.callback(qualitySelect.value)
  }
  okLoading.value = false
  modalCloseAll()
}
</script>

<template>
  <a-modal :visible='visible' modal-class='modalclass videoqualitymodal' :footer='false'
           :unmount-on-close='true' :mask-closable='false'
           @cancel='handleHide' @close='handleClose' @before-open='handleOpen'>
    <template #title>
      <span class='modaltitle'>选择播放清晰度</span>
    </template>
    <div class="mn-modal mn-modal-wide">
      <div class="mn-hero">
        <div class="mn-hero-icon"><ListVideo :size="20" /></div>
        <div class="mn-hero-text">
          <div class="mn-hero-title" :title="fileInfo.name">{{ fileInfo.name }}</div>
          <div class="mn-hero-sub">
            大小：{{ humanSize(fileInfo.size) }}
            &nbsp;·&nbsp; 分辨率：{{ fileInfo.media_width ? fileInfo.media_width + 'x' + fileInfo.media_height : '未知' }}
            <template v-if="fileInfo.media_duration">&nbsp;·&nbsp; 总时长：{{ fileInfo.media_duration }}</template>
            <template v-if="fileInfo.media_play_cursor">&nbsp;·&nbsp; 已看：{{ fileInfo.media_play_cursor }}</template>
          </div>
        </div>
      </div>
      <a-image v-if="fileInfo.thumbnail" class="quality-thumb" width="100%" height="150px" show-loader :src="fileInfo.thumbnail" />
      <div class="mn-section">
        <div class="mn-section-title">选择播放清晰度</div>
        <a-radio-group v-model:model-value="qualitySelect" class="quality-options">
          <a-radio v-for="(item, index) in qualityData.qualities" :key="index" :value="item.quality">
            <template #radio="{ checked }">
              <a-tag size="large" bordered color="orangered" :checked="checked" checkable>
                <template v-if="checked" #icon>
                  <IconFont name="iconcheck" />
                </template>
                {{ item.label }}
              </a-tag>
            </template>
          </a-radio>
        </a-radio-group>
      </div>
      <div class="mn-section">
        <div class="mn-section-title">清晰度快捷设置</div>
        <MySwitch :value='settingStore.uiVideoQualityTips' @update:value='cb({ uiVideoQualityTips: $event })'>
          观看视频前 提示选择清晰度
        </MySwitch>
        <MySwitch :value='settingStore.uiVideoQualityLastSelect' @update:value='cb({ uiVideoQualityLastSelect: $event })'>
          记忆上次选择的清晰度
        </MySwitch>
      </div>
      <div class='mn-footer'>
        <span class='mn-footer-spacer'></span>
        <a-button v-if='!okLoading' type='outline' size='small' @click='handleHide'>取消</a-button>
        <a-button type='primary' size='small' :loading='okLoading' @click='handleOK'>播放</a-button>
      </div>
    </div>
  </a-modal>
</template>

<style scoped>
.quality-thumb {
  overflow: hidden;
  border-radius: 8px;
}

.quality-options {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.quality-options :deep(.arco-radio) {
  margin-right: 0;
}
</style>