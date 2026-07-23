<script setup lang='ts'>
import { usePanTreeStore, useSettingStore } from '../../store'
import usePanFileStore from '../panfilestore'
import message from '../../utils/message'
import { modalCloseAll } from '../../utils/modal'
import { nextTick, PropType, ref } from 'vue'
import { Upload } from 'lucide-vue-next'
import UploadingDAL from '../../transfer/uploadingdal'
import AliFile from '../../aliapi/file'

const props = defineProps({
  visible: {
    type: Boolean,
    required: true
  },
  file_id: {
    type: String,
    required: true
  },
  filelist: {
    type: Array as PropType<string[]>,
    required: true
  },
  ispic: {
    type: Boolean,
    required: true
  },
  encType: {
    type: String,
    required: true
  }
})

const okLoading = ref(false)
const dirPath = ref('')
const file_id = ref('')
const settingStore = useSettingStore()

const cb = (val: any) => {
  settingStore.updateStore(val)
}

const handleOpen = () => {
  const panfileStore = usePanFileStore()
  file_id.value = props.ispic ? 'pic_root' : (panfileStore.DirID || props.file_id)
  const pantreeStore = usePanTreeStore()
  if (!file_id.value) {
    file_id.value = pantreeStore.selectDir.file_id
  }
  if (!file_id.value) {
    message.error('无法上传：当前文件夹无效，请重新打开目标文件夹')
    nextTick(() => {
      modalCloseAll()
    })
    return
  }
  let fileName = pantreeStore.selectDir.name
  AliFile.ApiFileGetPathString(pantreeStore.user_id, pantreeStore.drive_id, file_id.value, '/').then((data) => {
    dirPath.value = '/' + data + (props.ispic ? '/' + fileName : '')
  })
}

const handleClose = () => {
  if (okLoading.value) okLoading.value = false
  dirPath.value = ''
}

const handleHide = () => {
  modalCloseAll()
}
const handleOK = () => {
  const pantreeStore = usePanTreeStore()
  const settingStore = useSettingStore()
  UploadingDAL.aUploadLocalFiles(
    pantreeStore.user_id, pantreeStore.drive_id,
    file_id.value, props.filelist,
    settingStore.downUploadWhatExist,
    true, props.encType
  )
  modalCloseAll()
}
</script>

<template>
  <a-modal :visible='visible' modal-class='modalclass' :footer='false'
           :unmount-on-close='true' :mask-closable='false'
           @cancel='handleHide' @before-open='handleOpen' @close='handleClose'>
    <template #title>
      <span class='modaltitle'>
        {{ `${encType == 'enc' ? '加密' : encType == 'myenc' ? '私密' : ''}上传 文件/文件夹 到网盘` }}
      </span>
    </template>
    <div class='mn-modal'>
      <div class='mn-hero'>
        <div class='mn-hero-icon'><Upload :size='20' /></div>
        <div class='mn-hero-text'>
          <div class='mn-hero-title'>把 <span class='filelistcount'>{{ filelist.length }}</span> 个文件上传到网盘</div>
          <div class='mn-hero-sub'>{{ `${encType == 'enc' ? '加密上传' : encType == 'myenc' ? '私密上传' : '上传到当前网盘位置'}` }}</div>
        </div>
      </div>
      <div class='mn-panel'>
        <span class='mn-panel-label'>目标位置</span>
        <span class='mn-panel-value mn-panel-wrap'>{{ dirPath }}</span>
      </div>
      <div class='mn-section'>
        <div class='mn-section-title'>上传时遇到重名文件冲突</div>
        <div class='upload-conflict-row'>
          <a-select tabindex='-1' :style="{ flex: '1 1 auto' }" :model-value='settingStore.downUploadWhatExist'
                    @update:model-value='cb({ downUploadWhatExist: $event })'>
            <a-option value='ignore'>删除网盘内文件，继续上传</a-option>
            <a-option value='overwrite'>覆盖网盘内文件，继续上传</a-option>
            <a-option value='auto_rename'>保留网盘内文件，继续上传，重命名</a-option>
            <a-option value='refuse'>保留网盘内文件，不上传了</a-option>
          </a-select>
          <a-popover position='bottom'>
            <IconFont name="iconbulb" class='upload-conflict-tip' />
            <template #content>
              <div>
                默认：<span class='opred'>删除网盘内文件，继续上传</span>
                <hr />
                如果要上传的文件和网盘内已存在的文件重名了<br /><br />
                当内容完全一致(sha1相同)时，则<span class='opred'>无需</span>处理<br />
                反之，需要<span class='opred'>决定</span>如何处理<br />
                <hr />
                删除网盘内文件和覆盖网盘内文件区别是：<br />
                删除会在回收站有已删除记录可以还原文件<br />
                覆盖在回收站没有记录
              </div>
            </template>
          </a-popover>
        </div>
      </div>
      <div class='mn-footer'>
        <span class='mn-footer-spacer'></span>
        <a-button v-if='!okLoading' type='outline' size='small' @click='handleHide'>取消</a-button>
        <a-button type='primary' size='small' :loading='okLoading' @click='handleOK'>开始上传</a-button>
      </div>
    </div>
  </a-modal>
</template>

<style>
.filelistcount {
  color: rgb(var(--primary-6));
  margin: 0 2px;
}

.upload-conflict-row {
  display: flex;
  align-items: center;
  gap: 8px;
}

.upload-conflict-tip {
  flex: 0 0 auto;
  color: #ffc107dd;
  font-size: 18px;
  cursor: help;
}
</style>
