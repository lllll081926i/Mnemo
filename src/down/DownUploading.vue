<script setup lang="ts">
import { KeyboardState, MouseState, useAppStore, useDowningStore, useKeyboardStore, useMouseStore, useWinStore } from '../store'
import { onHideRightMenuScroll, onShowRightMenu, TestCtrl, TestKey, TestKeyboardScroll, TestKeyboardSelect } from '../utils/keyboardhelper'
import { computed, ref, watch } from 'vue'
import useUploadingStore from './UploadingStore'

import { Tooltip as AntdTooltip } from 'ant-design-vue'
import UploadingDAL from '../transfer/uploadingdal'
import { TestButton } from '../utils/mosehelper'
import { xorWith } from 'lodash'
import { getDriveAccountLabel } from '../utils/driveAccount'

const viewlist = ref()
const appStore = useAppStore()
const winStore = useWinStore()
const uploadingStore = useUploadingStore()

const isUpload = computed(() => uploadingStore.ListDataUploadingCount > 0)
watch(isUpload, (value, oldValue) => {
  if (value !== oldValue && window.WebToElectron) {
    if (value) {
      window.WebToElectron({ cmd: 'preventSleep', flag: 1 })
    } else if (useDowningStore().ListDataDowningCount == 0) {
      window.WebToElectron({ cmd: 'preventSleep', flag: 0 })
    }
  }
})
const menuShowDir = ref(false)
const menuShowTask = ref(false)
uploadingStore.$subscribe((_m: any, state: any) => {
  menuShowTask.value = !uploadingStore.showTaskID
  const selectItem = uploadingStore.GetSelectedFirst()
  menuShowDir.value = (selectItem && selectItem.isDir && !uploadingStore.showTaskID) || false
})

const keyboardStore = useKeyboardStore()
keyboardStore.$subscribe((_m: any, state: KeyboardState) => {
  if (appStore.appTab != 'down' || appStore.GetAppTabMenu != 'UploadingRight') return

  if (TestCtrl('a', state.KeyDownEvent, () => uploadingStore.mSelectAll())) return

  if (TestKey('f5', state.KeyDownEvent, handleRefresh)) return
  if (TestKey('Backspace', state.KeyDownEvent, handleBack)) return
  if (TestKeyboardSelect(state.KeyDownEvent, viewlist.value, uploadingStore, UploadingDAL.aUploadingStartOne)) return
  if (TestKeyboardScroll(state.KeyDownEvent, viewlist.value, uploadingStore)) return
})

const rangIsSelecting = ref(false)
const rangSelectID = ref(-1)
const rangSelectStart = ref(-1)
const rangSelectEnd = ref(-1)
const rangSelectFiles = ref<{ [k: number]: any }>({})
const onSelectRangStart = () => {
  onHideRightMenuScroll()
  rangIsSelecting.value = !rangIsSelecting.value
  rangSelectID.value = -1
  rangSelectStart.value = -1
  rangSelectEnd.value = -1
  rangSelectFiles.value = {}
  uploadingStore.mRefreshListDataShow(false)
}
const onSelectReverse = () => {
  onHideRightMenuScroll()
  const listData = uploadingStore.ListDataShow
  const listSelected = uploadingStore.GetSelected()
  const reverseSelect = xorWith(listData, listSelected, (a, b) => a.UploadID === b.UploadID)
  uploadingStore.ListSelected.clear()
  uploadingStore.ListFocusKey = -1
  if (reverseSelect.length > 0) {
    uploadingStore.mRangSelect(
      reverseSelect[0].UploadID,
      reverseSelect.map((r) => r.UploadID)
    )
  }
  uploadingStore.mRefreshListDataShow(false)
}
const onSelectCancel = () => {
  onHideRightMenuScroll()
  uploadingStore.ListSelected.clear()
  uploadingStore.ListFocusKey = 0
  uploadingStore.mRefreshListDataShow(false)
}

const onSelectRang = (file_id: number) => {
  if (rangIsSelecting.value && rangSelectID.value != -1) {
    let startid = rangSelectID.value
    let endid = -1
    const s: { [k: string]: any } = {}
    const children = uploadingStore.ListDataShow
    let a = -1
    let b = -1
    for (let i = 0, maxi = children.length; i < maxi; i++) {
      if (children[i].UploadID == file_id) a = i
      if (children[i].UploadID == startid) b = i
      if (a > 0 && b > 0) break
    }
    if (a >= 0 && b >= 0) {
      if (a > b) {
        ;[a, b] = [b, a]
        endid = file_id
      } else {
        endid = startid
        startid = file_id
      }
      for (let n = a; n <= b; n++) {
        s[children[n].UploadID] = true
      }
    }

    rangSelectStart.value = startid
    rangSelectEnd.value = endid
    rangSelectFiles.value = s
    uploadingStore.mRefreshListDataShow(false)
  }
}

const mouseStore = useMouseStore()
mouseStore.$subscribe((_m: any, state: MouseState) => {
  if (appStore.appTab != 'down') return
  const mouseEvent = state.MouseEvent
  // console.log('MouseEvent', state.MouseEvent)
  if (
    TestButton(0, mouseEvent, () => {
      if (mouseEvent.srcElement) {
        // @ts-ignore
        const className = mouseEvent.srcElement.className
        if (className && className.toString().startsWith('arco-virtual-list')) {
          onSelectCancel()
        }
      }
    })
  )
    return
})

const handleRefresh = () => UploadingDAL.aReloadUploading()
const handleBack = () => UploadingDAL.mUploadingShowTaskBack()
const handleSelectAll = () => uploadingStore.mSelectAll()

const handleSelect = (UploadID: number, event: any, isCtrl: boolean = false) => {
  onHideRightMenuScroll()
  if (rangIsSelecting.value) {
    if (rangSelectID.value == -1) {
      if (!uploadingStore.ListSelected.has(UploadID)) {
        uploadingStore.mMouseSelect(UploadID, true, false)
      }
      rangSelectID.value = UploadID
      rangSelectStart.value = UploadID
      rangSelectFiles.value = { [UploadID]: true }
    } else {
      const start = rangSelectID.value
      const children = uploadingStore.ListDataShow
      let a = -1
      let b = -1
      for (let i = 0, maxi = children.length; i < maxi; i++) {
        if (children[i].UploadID == UploadID) a = i
        if (children[i].UploadID == start) b = i
        if (a > 0 && b > 0) break
      }
      const fileList: number[] = []
      if (a >= 0 && b >= 0) {
        if (a > b) [a, b] = [b, a]
        for (let n = a; n <= b; n++) {
          fileList.push(children[n].UploadID)
        }
      }
      uploadingStore.mRangSelect(UploadID, fileList)
      rangIsSelecting.value = false
      rangSelectID.value = -1
      rangSelectStart.value = -1
      rangSelectEnd.value = -1
      rangSelectFiles.value = {}
    }
    uploadingStore.mRefreshListDataShow(false)
  } else {
    uploadingStore.mMouseSelect(UploadID, event.ctrlKey || isCtrl, event.shiftKey)
    if (!uploadingStore.ListSelected.has(UploadID)) uploadingStore.ListFocusKey = -1
  }
}

const handleRightClick = (e: { event: MouseEvent; node: any }) => {
  const key = e.node.key
  if (!uploadingStore.ListSelected.has(key)) uploadingStore.mMouseSelect(key, false, false)
  onShowRightMenu('rightuploadingmenu', e.event.clientX, e.event.clientY)
}
</script>

<template>
  <div class="toppanbtns">
    <div class="toppanbtn">
      <a-button type="text" size="small" tabindex="-1" :disabled="uploadingStore.ListLoading" title="后退 Back Space" @click="handleBack">
        <template #icon>
          <IconFont name="iconarrow-left-2-icon" />
        </template>
      </a-button>
      <a-button type="text" size="small" tabindex="-1" :loading="uploadingStore.ListLoading" title="F5" @click="handleRefresh">
        <template #icon>
          <IconFont name="iconreload-1-icon" />
        </template>
      </a-button>
    </div>

    <div v-if="uploadingStore.IsListSelected" class="toppanbtn">
      <a-button type="text" size="small" tabindex="-1" @click="() => UploadingDAL.aUploadingStart(false, true)">
        <IconFont name="iconstart" />
        开始
      </a-button>
      <a-button type="text" size="small" tabindex="-1" @click="() => UploadingDAL.aUploadingStart(false, false)">
        <IconFont name="iconpause" />
        暂停
      </a-button>
      <a-button v-if="menuShowDir" type="text" size="small" tabindex="-1" @click="() => UploadingDAL.mUploadingShowTask()">
        <IconFont name="icongengduo1" />
        查看
      </a-button>
      <a-button type="text" size="small" tabindex="-1" @click="() => UploadingDAL.aUploadingDelete(false)">
        <IconFont name="icondelete" />
        清除
      </a-button>
    </div>
    <div v-else class="toppanbtn">
      <a-button type="text" size="small" tabindex="-1" @click="() => UploadingDAL.aUploadingStart(true, true)">
        <IconFont name="iconstart" />
        开始全部
      </a-button>
      <a-button type="text" size="small" tabindex="-1" @click="() => UploadingDAL.aUploadingStart(true, false)">
        <IconFont name="iconpause" />
        暂停全部
      </a-button>
      <a-button type="text" size="small" tabindex="-1" @click="() => UploadingDAL.aUploadingDelete(true)">
        <IconFont name="iconrest" />
        清空全部
      </a-button>
    </div>
  </div>
  <div class="toppanarea">
    <div style="margin: 0 3px">
      <AntdTooltip title="点击全选" placement="left">
        <a-button shape="circle" type="text" tabindex="-1" class="select all" title="Ctrl+A" @click="handleSelectAll">
          <IconFont :name="uploadingStore.IsListSelectedAll ? 'iconrsuccess' : 'iconpic2'" />
        </a-button>
      </AntdTooltip>
      <div class="selectInfo">{{ uploadingStore.ListDataSelectCountInfo }}</div>
      <div style="margin: 0 2px">
        <AntdTooltip placement="rightTop" v-if="uploadingStore.ListDataShow.length > 0">
          <a-button shape="square" type="text" tabindex="-1" class="qujian" :status="rangIsSelecting ? 'danger' : 'normal'" title="Ctrl+Q" @click="onSelectRangStart">
            {{ rangIsSelecting ? '取消选择' : '区间选择' }}
          </a-button>
          <template #title>
            <div>
              第1步: 点击 区间选择 这个按钮
              <br />
              第2步: 鼠标点击一个文件
              <br />
              第3步: 移动鼠标点击另外一个文件
            </div>
          </template>
        </AntdTooltip>
        <a-button shape="square" v-if="!rangIsSelecting && uploadingStore.ListSelected.size > 0 && uploadingStore.ListSelected.size < uploadingStore.ListDataShow.length" type="text" tabindex="-1" class="qujian" status="normal" @click="onSelectReverse">
          反向选择
        </a-button>
        <a-button shape="square" v-if="!rangIsSelecting && uploadingStore.ListSelected.size > 0" type="text" tabindex="-1" class="qujian" status="normal" @click="onSelectCancel">取消已选</a-button>
      </div>
    </div>
    <div class="toolbar-spacer"></div>
    <div v-if="!uploadingStore.AccountFilter && !uploadingStore.showTaskID" class="cell account">账号</div>
    <div class="downsize transfer-column-heading">大小</div>
    <div class="downprogress transfer-column-heading">进度</div>
    <div class="downspeed transfer-column-heading">速度</div>
    <div class="cell pr"></div>
  </div>
  <div class="toppanlist" style="position: relative" :style="{ height: winStore.GetListHeight }" @keydown.space.prevent="() => true">
    <a-list
      ref="viewlist"
      :bordered="false"
      :split="false"
      :max-height="winStore.GetListHeightNumber"
      :virtual-list-props="{
        height: winStore.GetListHeightNumber,
        fixedSize: true,
        estimatedSize: 50,
        threshold: 1,
        itemKey: 'UploadID'
      }"
      style="width: 100%"
      :data="uploadingStore.ListDataShow"
      tabindex="-1"
      @scroll="onHideRightMenuScroll"
    >
      <template #empty>
        <a-empty description="没有 需要上传 的任务" />
      </template>
      <template #item="{ item, index }">
        <div :key="item.UploadID" class="listitemdiv">
          <div
            :class="'fileitem ' + (uploadingStore.ListSelected.has(item.UploadID) ? ' selected' : '') + (uploadingStore.ListFocusKey == item.UploadID ? ' focus' : '') + (item.uploadState == 'hashing' || item.uploadState == 'running' ? ' running' : '')"
            @click="handleSelect(item.UploadID, $event)"
            @mouseover="onSelectRang(item.UploadID)"
            @dblclick="UploadingDAL.aUploadingStartOne(item.UploadID)"
            @contextmenu="(event: MouseEvent) => handleRightClick({ event, node: { key: item.UploadID } })"
          >
            <div :class="'rangselect ' + (rangSelectFiles[item.UploadID] ? (rangSelectStart == item.UploadID ? 'rangstart' : rangSelectEnd == item.UploadID ? 'rangend' : 'rang') : '')">
              <a-button shape="circle" type="text" tabindex="-1" class="select" :title="index" @click.prevent.stop="handleSelect(item.UploadID, $event, true)">
                <IconFont :name="uploadingStore.ListSelected.has(item.UploadID) ? 'iconrsuccess' : 'iconpic2'" />
              </a-button>
            </div>
            <div class="fileicon">
              <IconFont :name="item.icon" aria-hidden="true" />
            </div>
            <div class="filename">
              <div v-if="item.isDir && menuShowTask" :title="item.localFilePath" @click.stop="UploadingDAL.mUploadingShowTask(item.TaskID)">
                {{ item.name }}
              </div>
              <div v-else class="nopoint" :title="item.localFilePath">
                {{ item.name }}
              </div>
            </div>
            <div v-if="!uploadingStore.AccountFilter && !uploadingStore.showTaskID" class="cell account" :title="getDriveAccountLabel(item.user_id)">{{ getDriveAccountLabel(item.user_id) }}</div>
            <div class="downsize">{{ item.sizeStr }}</div>
            <div class="downprogress">
              <div class="transfering-state">
                <p class="text-state">{{ item.uploadState }} {{ item.ProgressStr }}</p>
                <div class="progress-total">
                  <div :class="'progress-current ' + (item.uploadState == 'success' ? ' succeed' : item.uploadState == 'error' ? ' error' : item.uploadState == '已暂停' ? '' : ' active')" :style="{ width: item.Progress + '%' }"></div>
                </div>
                <p v-if="item.isDir && menuShowTask" class="text-error pointer" :title="item.errorMessage" @click.stop="UploadingDAL.mUploadingShowTask(item.TaskID)">{{ item.errorMessage }}</p>
                <p v-else class="text-error" :title="item.errorMessage">{{ item.errorMessage }}</p>
              </div>
            </div>
            <div class="downspeed">{{ item.speedStr }}</div>
          </div>
        </div>
      </template>
    </a-list>
    <a-dropdown id="rightuploadingmenu" class="rightmenu" :popup-visible="true" tabindex="-1" :draggable="false" style="z-index: -1; left: -200px; opacity: 0">
      <template #content>
        <a-doption @click="() => UploadingDAL.aUploadingStart(false, true)">
          <template #icon><IconFont name="iconstart" /></template>
          <template #default>开始上传</template>
        </a-doption>

        <a-doption @click="() => UploadingDAL.aUploadingStart(false, false)">
          <template #icon><IconFont name="iconpause" /></template>
          <template #default>暂停上传</template>
        </a-doption>

        <a-doption v-if="menuShowDir" @click="() => UploadingDAL.mUploadingShowTask()">
          <template #icon><IconFont name="icongengduo1" /></template>
          <template #default>查看详情</template>
        </a-doption>

        <a-doption @click="() => UploadingDAL.aUploadingDelete(false)">
          <template #icon><IconFont name="icondelete" /></template>
          <template #default>清除上传</template>
        </a-doption>
      </template>
    </a-dropdown>
  </div>
</template>

<style scoped>
.fileitem .icondownload {
  color: var(--text-tertiary);
}

.fileitem .running .icondownload {
  color: var(--color-primary);
}

.nopoint {
  cursor: default !important;
  color: var(--color-text-1) !important;
}

.pointer {
  cursor: pointer;
}
</style>
