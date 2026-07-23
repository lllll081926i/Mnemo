<script lang="ts">
import AliFileCmd from '../../aliapi/filecmd'
import { usePanTreeStore, useSettingStore } from '../../store'
import message from '../../utils/message'
import { modalCloseAll } from '../../utils/modal'
import { CheckFileName, ClearFileName } from '../../utils/filehelper'
import { defineComponent, PropType, reactive, ref } from 'vue'
import { FolderPlus } from 'lucide-vue-next'
import PanDAL from '../pandal'

export default defineComponent({
  components: { FolderPlus },
  props: {
    visible: {
      type: Boolean,
      required: true
    },
    dirtype: {
      type: String,
      required: true
    },
    encType: {
      type: String,
      required: true
    },
    parentdirid: {
      type: String
    },
    callback: {
      type: Function as PropType<(newdirid: string) => void>
    }
  },
  setup(props) {
    const okLoading = ref(false)
    const formRef = ref()

    const form = reactive({
      dirName: '',
      dirIndex: 1
    })
    const handleOpen = () => {
      setTimeout(() => {
        document.getElementById('CreatNewDirInput')?.focus()
      }, 200)

      if (props.dirtype == 'datefolder') {
        let dirName = ''
        let dirIndex = 1

        const date = new Date(Date.now())
        const y = date.getFullYear().toString()
        let m: number | string = date.getMonth() + 1
        m = m < 10 ? '0' + m.toString() : m.toString()
        let d: number | string = date.getDate()
        d = d < 10 ? '0' + d.toString() : d.toString()
        let h: number | string = date.getHours()
        h = h < 10 ? '0' + h.toString() : h.toString()
        let minute: number | string = date.getMinutes()
        minute = minute < 10 ? '0' + minute.toString() : minute.toString()
        let second: number | string = date.getSeconds()
        second = second < 10 ? '0' + second.toString() : second.toString()

        const settingStore = useSettingStore()
        dirName = settingStore.uiTimeFolderFormate.replace(/yyyy/gi, y).replace(/MM/g, m).replace(/dd/gi, d).replace(/HH/gi, h).replace(/mm/g, minute).replace(/ss/gi, second)
        if (settingStore.uiTimeFolderFormate.indexOf('#') >= 0) {
          dirIndex = settingStore.uiTimeFolderIndex
          dirName = dirName.replace(/#{1,}/g, function (val) {
            return dirIndex.toString().padStart(val.length, '0')
          })
        }
        form.dirName = dirName
        form.dirIndex = dirIndex
      } else {
        form.dirName = ''
        form.dirIndex = 1
      }
    }

    const handleClose = () => {
      
      if (okLoading.value) okLoading.value = false
      formRef.value.resetFields()
    }

    const rules = [
      { required: true, message: '文件夹名必填' },
      { minLength: 1, message: '文件夹名不能为空' },
      { maxLength: 100, message: '文件夹名太长(100)' },
      {
        validator: (value: string, cb: any) => {
          const chk = CheckFileName(value)
          if (chk) cb('文件夹名' + chk)
        }
      }
    ]

    return { okLoading, form, formRef, handleOpen, handleClose, rules }
  },
  methods: {
    handleHide() {
      modalCloseAll()
    },
    handleOK() {
      this.formRef.validate((data: any) => {
        if (data) return 

        const pantreeStore = usePanTreeStore()
        if (!pantreeStore.user_id || !pantreeStore.drive_id || !pantreeStore.selectDir.file_id) {
          message.error('无法新建文件夹：当前目录无效，请先打开网盘中的文件夹')
          return
        }

        const newName = ClearFileName(this.form.dirName)
        if (!newName) {
          message.error('请输入文件夹名称')
          return
        }

        this.okLoading = true
        let newdirid = ''
        const parentId = this.parentdirid || pantreeStore.selectDir.file_id
        const createRequest = AliFileCmd.ApiCreatNewForder(pantreeStore.user_id, pantreeStore.drive_id, parentId, newName, this.encType)

        createRequest
          .then((data) => {
            if (data.error) message.error(`无法新建文件夹：${data.error}`)
            else {
              newdirid = data.file_id
              message.success('文件夹已创建')
              if (this.form.dirIndex) useSettingStore().updateStore({ uiTimeFolderIndex: this.form.dirIndex + 1 })
              if (!this.parentdirid || pantreeStore.selectDir.file_id == this.parentdirid) {
                
                PanDAL.aReLoadOneDirToShow('', 'refresh', false)
              } else {
                
                return PanDAL.GetDirFileList(pantreeStore.user_id, pantreeStore.drive_id, this.parentdirid, '')
              }
            }
          })
          .catch((err: any) => {
            message.error(`无法新建文件夹：${err.message || '请检查网络连接后重试'}`)
          })
          .then(() => {
            modalCloseAll()
            if (this.callback) this.callback(newdirid)
          })
      })
    }
  }
})
</script>

<template>
  <a-modal :visible="visible" modal-class="modalclass" :footer="false" :unmount-on-close="true" :mask-closable="false" @cancel="handleHide" @before-open="handleOpen" @close="handleClose">
    <template #title>
      <span class="modaltitle">新建文件夹</span>
    </template>
    <div class="mn-modal">
      <div class="mn-hero">
        <div class="mn-hero-icon"><FolderPlus :size="20" /></div>
        <div class="mn-hero-text">
          <div class="mn-hero-title">新建文件夹</div>
          <div class="mn-hero-sub">在当前位置创建一个新的文件夹</div>
        </div>
      </div>
      <a-form ref="formRef" :model="form" layout="vertical" class="mn-form">
        <a-form-item field="dirName" :rules="rules" label="文件夹名">
          <a-input v-model.trim="form.dirName" placeholder="例如：新建文件夹" allow-clear :input-attrs="{ id: 'CreatNewDirInput', autofocus: 'autofocus' }" />
        </a-form-item>
      </a-form>
      <div class="mn-hint">不要包含特殊字符 &lt; &gt; : * ? \ / ' "</div>
      <div class="mn-footer">
        <span class="mn-footer-spacer"></span>
        <a-button v-if="!okLoading" type="outline" size="small" @click="handleHide">取消</a-button>
        <a-button type="primary" size="small" :loading="okLoading" @click="handleOK">创建</a-button>
      </div>
    </div>
  </a-modal>
</template>

<style></style>
