<script setup lang='ts'>
import { IAliGetFileModel } from '../../aliapi/alimodels'
import { modalCloseAll } from '../../utils/modal'
import { computed, PropType, reactive, ref } from 'vue'
import dayjs from 'dayjs'
import { usePanTreeStore, useSettingStore } from '../../store'
import { humanDateTime, randomSharePassword } from '../../utils/format'
import message from '../../utils/message'
import AliShare from '../../aliapi/share'
import { ArrayKeyList } from '../../utils/utils'
import { copyToClipboard } from '../../utils/electronhelper'
import { GetShareUrlFormate } from '../../utils/shareurl'
import { getDriveProviderCapabilities, resolveDriveProvider, type DriveProvider } from '../../utils/driveProvider'

const formRef = ref()
const okLoading = ref(false)
const okBatchLoading = ref(false)
const settingStore = useSettingStore()
const shareType = ref()
const activeProvider = ref<DriveProvider>('unknown')
const supportsShareExpiration = computed(() => ['pikpak', 'onedrive', 'dropbox', 'gofile'].includes(activeProvider.value))
const supportsSharePassword = computed(() => ['pikpak', 'onedrive', 'dropbox'].includes(activeProvider.value))
const supportsShareSettings = computed(() => supportsShareExpiration.value || supportsSharePassword.value)
const supportsCombinedShare = computed(() => !['onedrive', 'dropbox', 'gdrive', 'gofile'].includes(activeProvider.value))

const form = reactive({
  expiration: '',
  share_pwd: '',
  share_name: '',
  mutil: false
})
const props = defineProps({
  visible: {
    type: Boolean,
    required: true
  },
  sharetype: {
    type: String,
    required: true
  },
  driveType: {
    type: String,
    required: true
  },
  filelist: {
    type: Array as PropType<IAliGetFileModel[]>,
    required: true
  }
})
const getShareType = (): any => ({ type: 's', title: '分享' })

const handleOpen = async () => {
  const pantreeStore = usePanTreeStore()
  activeProvider.value = resolveDriveProvider({ userId: pantreeStore.user_id, driveId: pantreeStore.drive_id })
  form.share_name = props.filelist[0].name
  shareType.value = getShareType()
  let share_pwd = ''
  if (settingStore.uiSharePassword == 'random') share_pwd = randomSharePassword()
  else if (settingStore.uiSharePassword == 'last') share_pwd = localStorage.getItem('share_pwd') || ''
  form.share_pwd = supportsSharePassword.value ? share_pwd : ''

  let expiration = Date.now()
  if (settingStore.uiShareDays == 'always') expiration = 0
  else if (settingStore.uiShareDays == 'week') expiration += 7 * 24 * 60 * 60 * 1000
  else expiration += 30 * 24 * 60 * 60 * 1000

  form.expiration = supportsShareExpiration.value && expiration > 0 ? humanDateTime(expiration) : ''
}

const handleClose = () => {
  if (okLoading.value) okLoading.value = false
  if (okBatchLoading.value) okBatchLoading.value = false
  formRef.value.resetFields()
}

const handleHide = () => {
  modalCloseAll()
}
const handleOK = async (multi: boolean) => {
  const pantreeStore = usePanTreeStore()
  if (!pantreeStore.user_id || !pantreeStore.drive_id || !pantreeStore.selectDir.file_id) {
    message.error('新建文件失败 父文件夹错误')
    return
  }
  const mindate = new Date()
  mindate.setMinutes(mindate.getMinutes() + 2)
  let expiration = form.expiration
  if (expiration) expiration = new Date(expiration) < mindate ? mindate.toISOString() : new Date(expiration).toISOString()
  else expiration = ''
  let share_name = form.share_name.trim().replaceAll('"', '')
  share_name = share_name.replace(/[<>:"\\|?*]+/g, '')
  share_name = share_name.replace(/[\f\n\r\t\v]/g, '')
  while (share_name.endsWith(' ') || share_name.endsWith('.')) share_name = share_name.substring(0, share_name.length - 1)
  if (shareType.value.type === 's' && share_name.length < 1) {
    message.error('分享链接标题不能为空')
    return
  }
  const share_pwd = form.share_pwd
  const user_id = pantreeStore.user_id
  const drive_id = pantreeStore.drive_id
  const provider = resolveDriveProvider({ userId: user_id, driveId: drive_id })
  const file_id_list = ArrayKeyList<string>('file_id', props.filelist)
  if (!multi && file_id_list.length > 1 && !supportsCombinedShare.value) {
    message.error('当前网盘仅支持为每个文件单独创建分享链接')
    return
  }
  if (supportsSharePassword.value) localStorage.setItem('share_pwd', share_pwd)
  if (!multi) {
    okLoading.value = true
    let result = undefined
    let url = ''
    result = await AliShare.ApiCreatShare(user_id, drive_id, expiration, share_pwd, share_name, file_id_list)
    if (typeof result == 'string') {
      okLoading.value = false
      message.error(result)
      return
    }
    url = GetShareUrlFormate(result.share_name, result.share_url, result.share_pwd || '')
    message.success('创建分享链接成功，分享链接已复制到剪切板')
    copyToClipboard(url)
    okLoading.value = false
    modalCloseAll()
  } else {
    okBatchLoading.value = true
    let url = ''
    let sharedCount = 0
    let result = undefined
    for (let i = 0; i < file_id_list.length; i++) {
      result = await AliShare.ApiCreatShare(user_id, drive_id, expiration, share_pwd, share_name, file_id_list.slice(i, i + 1))
      if (typeof result == 'string') {
        okBatchLoading.value = false
        message.error(result)
        continue
      }
      sharedCount += 1
      url += GetShareUrlFormate(result.share_name, result.share_url, result.share_pwd) + '\n'
    }
    message.success('创建 ' + sharedCount + '条 分享链接成功，分享链接已复制到剪切板')
    copyToClipboard(url)
    okBatchLoading.value = false
    modalCloseAll()
  }
}
</script>

<template>
  <a-modal :visible='visible' modal-class='modalclass createsharelinkmodal' :footer='false'
           :unmount-on-close='true' :mask-closable='false'
           @cancel='handleHide' @before-open='handleOpen' @close='handleClose'>
    <template #title>
      <span class='modaltitle'>
        创建{{ shareType.title }}链接
        <span class='titletips'> (已选择{{ filelist.length }}个文件) </span>
      </span>
    </template>
    <div class='modalbody' style='width: 440px'>
      <a-form ref='formRef' :model='form' layout='vertical'>
        <a-form-item field='share_name'>
          <template #label>
            <template v-if='shareType.type === "s"'> {{ shareType.title }}链接标题：</template>
            <template v-else> {{ shareType.title }}文件：</template>
            <span class='opblue' style='margin-left: 0; font-size: 12px' v-if='shareType.type === "s"'> 修改后的标题只有自己可见 </span>
            <span class='opblue' style='margin-left: 0; font-size: 12px' v-else> 🎉快传支持发送所有格式的文件 </span>
          </template>
          <a-input v-model.trim='form.share_name' :placeholder='form.share_name' />
        </a-form-item>

        <template v-if='shareType.type === "s" && supportsShareSettings'>
          <a-row>
            <a-col v-if='supportsShareExpiration' flex='200px'> 有效期：</a-col>
            <a-col v-if='supportsShareExpiration && supportsSharePassword' flex='12px'></a-col>
            <a-col v-if='supportsSharePassword' flex='100px'> 提取码：</a-col>
            <a-col flex='auto'></a-col>
          </a-row>
          <a-row>
            <a-col v-if='supportsShareExpiration' flex='200px'>
              <a-form-item field='expiration'>
                <a-date-picker
                  v-model='form.expiration'
                  style='width: 200px; margin: 0'
                  show-time
                  placeholder='永久有效'
                  value-format='YYYY-MM-DD HH:mm:ss'
                  :shortcuts="[
                    { label: '永久', value: () => '' },
                    { label: '3小时', value: () => dayjs().add(3, 'hour') },
                    { label: '1天', value: () => dayjs().add(1, 'day') },
                    { label: '3天', value: () => dayjs().add(3, 'day') },
                    { label: '7天', value: () => dayjs().add(7, 'day') },
                    { label: '30天', value: () => dayjs().add(30, 'day') }
                  ]" />
              </a-form-item>
            </a-col>
            <a-col v-if='supportsShareExpiration && supportsSharePassword' flex='12px'></a-col>
            <a-col v-if='supportsSharePassword' flex='120px'>
              <a-form-item field='share_pwd' :rules="[{ length: 4, message: '提取码必须是4个字符' }]">
                <a-input v-model='form.share_pwd' tabindex='-1' placeholder='没有不填' />
              </a-form-item>
            </a-col>
            <a-col flex='auto'></a-col>
          </a-row>
        </template>
      </a-form>
    </div>
    <div class='modalfoot'>
      <a-button v-if='filelist.length > 1' type='outline' size='small' :loading='okBatchLoading' @click='handleOK(true)'>
        为每个文件单独创建
      </a-button>
      <div style='flex-grow: 1'></div>
      <a-button v-if='!okLoading' type='outline' size='small' @click='handleHide'>取消</a-button>
      <a-button v-if="supportsCombinedShare || filelist.length === 1 || shareType.type === 't'" type='primary' size='small' :loading='okLoading' @click='handleOK(false)'>
        创建{{ shareType.title }}链接
      </a-button>
    </div>
  </a-modal>
</template>

<style scoped>
.iconbulb {
  display: inline-block;
  height: 22px;
  margin-left: 4px;
  color: #ffc107dd;
  font-size: 18px;
  line-height: 22px;
  cursor: help;
}
</style>
