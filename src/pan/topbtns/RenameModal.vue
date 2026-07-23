<script setup lang='ts'>
import { usePanFileStore, usePanTreeStore, useSettingStore } from '../../store'
import message from '../../utils/message'
import { modalCloseAll, modalPassword, modalRename } from '../../utils/modal'
import { CheckFileName, ClearFileName } from '../../utils/filehelper'
import { nextTick, reactive, ref } from 'vue'
import { IAliGetFileModel } from '../../aliapi/alimodels'
import AliFileCmd from '../../aliapi/filecmd'
import PanDAL from '../pandal'
import { getEncType } from '../../utils/proxyhelper'
import { EncodeEncName } from '../../aliapi/utils'

const props = defineProps({
  visible: {
    type: Boolean,
    required: true
  },
  istree: {
    type: Boolean,
    required: true
  },
  ispic: {
    type: Boolean,
    required: true
  }
})

const okLoading = ref(false)
const encType = ref('')
const isShare = ref(false)
const formRef = ref()

const form = reactive({
  file_id: '',
  parent_file_id: '',
  isDir: false,
  fileName: '',
  description: '',
  bakName: ''
})

let fileList: IAliGetFileModel[] = []

const handleOpen = () => {
  setTimeout(() => {
    document.getElementById('RenameInput')?.focus()
  }, 200)

  if (props.istree) {
    const pantreeStore = usePanTreeStore()
    fileList = [{
      ...pantreeStore.selectDir,
      isDir: true,
      ext: '',
      category: '',
      icon: '',
      sizeStr: '',
      timeStr: '',
      starred: false,
      thumbnail: ''
    } as IAliGetFileModel]
  } else {
    const panfileStore = usePanFileStore()
    fileList = panfileStore.GetSelected()
    if (fileList.length == 0) {
      const focus = panfileStore.mGetFocus()
      panfileStore.mKeyboardSelect(focus, false, false)
      fileList = panfileStore.GetSelected()
    }
  }
  if (fileList.length == 0) {
    form.file_id = ''
    form.parent_file_id = ''
    form.isDir = false
    form.fileName = ''
    form.description = ''
    form.bakName = ''
    nextTick(() => {
      modalCloseAll()
    })
  } else {
    let file = fileList[0]
    isShare.value = file.from_share_id !== undefined
    form.file_id = file.file_id
    form.parent_file_id = file.parent_file_id
    form.isDir = file.isDir
    form.fileName = file.name
    form.description = file.description
    form.bakName = file.name
  }
}

const handleClose = () => {
  if (okLoading.value) okLoading.value = false
  formRef.value.resetFields()
}

const file_rules = [
  { required: true, message: '文件名必填' },
  { minLength: 1, message: '文件夹不能为空' },
  { maxLength: 100, message: '文件夹太长(100)' },
  {
    validator: (value: string, cb: any) => {
      const chk = CheckFileName(value)
      if (chk) cb('文件名' + chk)
    }
  }
]

const album_rules = [
  { required: true, message: '相册名必填' },
  { minLength: 1, message: '相册名不能为空' },
  { maxLength: 100, message: '相册名太长(100)' },
  {
    validator: (value: string, cb: any) => {
      const chk = CheckFileName(value)
      if (chk) cb('相册名' + chk)
    }
  }
]

const handleMulti = () => {
  modalRename(props.istree, true, props.ispic)
}

const handleHide = () => {
  modalCloseAll()
}

const handleOK = () => {
  formRef.value.validate((data: any) => {
    if (data) return
    let newName = ClearFileName(form.fileName)
    if (!newName) {
      message.error(`请输入${props.ispic ? '相册' : '文件'}名称`)
      return
    }
    if (newName == form.bakName) {
      modalCloseAll()
      return
    }
    const pantreeStore = usePanTreeStore()
    if (!pantreeStore.user_id || !pantreeStore.drive_id || !pantreeStore.selectDir.file_id) {
      message.error(`无法重命名：${props.ispic ? '相册文件夹' : '当前文件夹'}无效，请重新打开后再试`)
      return
    }
    okLoading.value = true
    encType.value = getEncType({ description: form.description })
    if (encType.value) {
      if (isShare.value) {
        modalPassword('input', (success, inputpassword) => {
          success && handleRename(newName, encType.value, inputpassword)
        })
      } else if (!useSettingStore().securityPassword) {
        modalPassword('new', (success) => {
          success && handleRename(newName, encType.value)
        })
      } else if (useSettingStore().securityPassword && useSettingStore().securityPasswordConfirm) {
        modalPassword('confirm', (success) => {
          success && handleRename(newName, encType.value)
        })
      } else {
        handleRename(newName, encType.value)
      }
    } else {
      handleRename(newName)
    }
  })
}

const handleRename = (newName: string, encType: string = '', inputpassword: string = '') => {
  const pantreeStore = usePanTreeStore()
  if (!props.ispic) {
    let encName = newName
    if (encType) {
      encName = EncodeEncName(pantreeStore.user_id, newName, form.isDir, encType, inputpassword)
    }
    const renamePromise = AliFileCmd.ApiRenameBatch(pantreeStore.user_id, pantreeStore.drive_id, [form.file_id], [encName])

    renamePromise
      .then((data: any) => {
        if (data.length == 1) {
          if (encType && data[0]) data[0].name = newName
          usePanTreeStore().mRenameFiles(data)
          if (!props.istree) usePanFileStore().mRenameFiles(data)
          PanDAL.RefreshPanTreeAllNode(pantreeStore.drive_id)
          message.success('重命名完成')
        } else {
          message.error('无法重命名，请检查是否存在同名文件')
        }
      })
      .catch((err: any) => {
        message.error(`无法重命名：${err?.message || err || '请稍后重试'}`)
      })
      .then(() => {
        modalCloseAll()
      })
  } else {
    okLoading.value = false
    message.error('当前版本不支持相册操作')
  }
}
</script>

<template>
  <a-modal :visible='visible' modal-class='modalclass' :footer='false' :unmount-on-close='true' :mask-closable='false'
           @cancel='handleHide' @before-open='handleOpen' @close='handleClose'>
    <template #title>
      <span class='modaltitle'>{{ ispic ? '重命名相册' : '重命名一个文件' }}</span>
    </template>
    <div class='rename-modal'>
      <div v-if='form.bakName' class='rename-old' :title='form.bakName'>
        <span class='rename-old-label'>当前名称</span>
        <span class='rename-old-name'>{{ form.bakName }}</span>
      </div>
      <a-form ref='formRef' :model='form' layout='vertical'>
        <a-form-item field='fileName' :rules='ispic ? album_rules : file_rules' class='rename-item'>
          <template #label>{{ ispic ? '相册名' : '新名称' }}</template>
          <a-input v-model='form.fileName' :placeholder='form.bakName' allow-clear
                   :input-attrs="{ id: 'RenameInput', autofocus: 'autofocus' }" />
          <div class='rename-hint'>不要包含特殊字符 &lt; &gt; : * ?  / ' "</div>
        </a-form-item>
        <a-form-item v-if='ispic' field='description' label='相册描述' class='rename-item'>
          <a-textarea v-model='form.description' placeholder='修改相册描述' show-word-limit
                      @keydown='(e:any) => e.stopPropagation()' :disabled="encType!=''" />
        </a-form-item>
      </a-form>
      <div class='rename-footer'>
        <a-button v-if='!ispic' type='text' size='small' @click='handleMulti'>批量重命名…</a-button>
        <div style='flex-grow: 1'></div>
        <a-button v-if='!okLoading' type='outline' size='small' @click='handleHide'>取消</a-button>
        <a-button type='primary' size='small' :loading='okLoading' @click='handleOK'>重命名</a-button>
      </div>
    </div>
  </a-modal>
</template>

<style scoped>
.rename-modal {
  display: flex;
  flex-direction: column;
  gap: 12px;
  width: 460px;
  max-width: calc(100vw - 64px);
}

.rename-old {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
  padding: 7px 10px;
  background: var(--color-fill-2);
  border-radius: 6px;
}

.rename-old-label {
  flex: 0 0 auto;
  font-size: 12px;
  color: var(--color-text-3);
}

.rename-old-name {
  overflow: hidden;
  font-size: 13px;
  color: var(--color-text-1);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.rename-item {
  margin-bottom: 0;
}

.rename-item :deep(.arco-form-item-label) {
  font-size: 13px;
  font-weight: 600;
  color: var(--color-text-2);
}

.rename-hint {
  margin-top: 6px;
  font-size: 12px;
  color: var(--color-text-3);
}

.rename-footer {
  display: flex;
  align-items: center;
  gap: 8px;
}
</style>
