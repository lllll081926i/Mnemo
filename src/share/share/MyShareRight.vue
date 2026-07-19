<script setup lang="ts">
import { computed, h, ref } from 'vue'
import { KeyboardState, MouseState, useAppStore, useKeyboardStore, useMouseStore, useMyShareStore, useWinStore } from '../../store'
import { humanCount } from '../../utils/format'
import ShareDAL from './ShareDAL'
import { onHideRightMenuScroll, onShowRightMenu, TestCtrl, TestKey, TestKeyboardScroll, TestKeyboardSelect } from '../../utils/keyboardhelper'
import { copyToClipboard, openExternal } from '../../utils/electronhelper'
import message from '../../utils/message'
import AliShare from '../../aliapi/share'

import { Tooltip as AntdTooltip } from 'ant-design-vue'
import { modalEditShareLink, modalShowShareLink } from '../../utils/modal'
import { ArrayKeyList } from '../../utils/utils'
import { GetShareUrlFormate } from '../../utils/shareurl'
import { TestButton } from '../../utils/mosehelper'
import { xorWith } from 'lodash'
import { Modal } from '@arco-design/web-vue'
import { getDriveProviderCapabilities } from '../../utils/driveProvider'
import type { IManagedShareItem } from './MyShareStore'

const viewlist = ref()
const inputsearch = ref()

const appStore = useAppStore()
const winStore = useWinStore()
const myshareStore = useMyShareStore()
const selectedShares = computed(() => myshareStore.GetSelected())
const selectedAccountIds = computed(() => new Set(selectedShares.value.map((item) => item.account_id)))
const canEditSelected = computed(() => selectedShares.value.length > 0 && selectedAccountIds.value.size == 1 && selectedShares.value.every((item) => getDriveProviderCapabilities(item.account_provider).editCreatedShares))
const canCancelSelected = computed(() => selectedShares.value.length > 0 && selectedShares.value.every((item) => getDriveProviderCapabilities(item.account_provider).cancelCreatedShares))
const canCancelVisibleShares = computed(() => myshareStore.ListDataShow.some((item) => getDriveProviderCapabilities(item.account_provider).cancelCreatedShares))

const keyboardStore = useKeyboardStore()
keyboardStore.$subscribe((_m: any, state: KeyboardState) => {
  if (appStore.appTab != 'share' || appStore.GetAppTabMenu != 'MyShareRight') return

  if (TestCtrl('a', state.KeyDownEvent, () => myshareStore.mSelectAll())) return
  if (TestCtrl('b', state.KeyDownEvent, handleBrowserLink)) return
  if (TestCtrl('c', state.KeyDownEvent, handleCopySelectedLink)) return
  if (canCancelSelected.value && TestCtrl('Delete', state.KeyDownEvent, () => handleDeleteSelectedLink('selected'))) return
  if (canEditSelected.value && TestCtrl('e', state.KeyDownEvent, handleEdit)) return
  if (canEditSelected.value && TestKey('f2', state.KeyDownEvent, handleEdit)) return
  if (TestCtrl('f', state.KeyDownEvent, () => inputsearch.value.focus())) return
  if (TestKey('f3', state.KeyDownEvent, () => inputsearch.value.focus())) return
  if (TestKey(' ', state.KeyDownEvent, () => inputsearch.value.focus())) return
  if (TestKey('f5', state.KeyDownEvent, handleRefresh)) return

  if (TestKeyboardSelect(state.KeyDownEvent, viewlist.value, myshareStore, undefined)) return
  if (TestKeyboardScroll(state.KeyDownEvent, viewlist.value, myshareStore)) return
})

const mouseStore = useMouseStore()
mouseStore.$subscribe((_m: any, state: MouseState) => {
  if (appStore.appTab != 'share') return
  const mouseEvent = state.MouseEvent
  // console.log('MouseEvent', state.MouseEvent)
  if (
    TestButton(0, mouseEvent, () => {
      if (mouseEvent.srcElement) {
        // @ts-ignore
        if (mouseEvent.srcElement.className && mouseEvent.srcElement.className.toString().startsWith('arco-virtual-list')) {
          onSelectCancel()
        }
      }
    })
  )
    return
})

const rangIsSelecting = ref(false)
const rangSelectID = ref('')
const rangSelectStart = ref('')
const rangSelectEnd = ref('')
const rangSelectFiles = ref<{ [k: string]: any }>({})
const onSelectRangStart = () => {
  onHideRightMenuScroll()
  rangIsSelecting.value = !rangIsSelecting.value
  rangSelectID.value = ''
  rangSelectStart.value = ''
  rangSelectEnd.value = ''
  rangSelectFiles.value = {}
  myshareStore.mRefreshListDataShow(false)
}
const onSelectCancel = () => {
  onHideRightMenuScroll()
  myshareStore.ListSelected.clear()
  myshareStore.ListFocusKey = ''
  myshareStore.mRefreshListDataShow(false)
}
const onSelectReverse = () => {
  onHideRightMenuScroll()
  const listData = myshareStore.ListDataShow
  const listSelected = myshareStore.GetSelected()
  const reverseSelect = xorWith(listData, listSelected, (a, b) => a.share_key === b.share_key)
  myshareStore.ListSelected.clear()
  myshareStore.ListFocusKey = ''
  if (reverseSelect.length > 0) {
    myshareStore.mRangSelect(
      reverseSelect[0].share_key,
      reverseSelect.map((r) => r.share_key)
    )
  }
  myshareStore.mRefreshListDataShow(false)
}
const onSelectRang = (shareKey: string) => {
  if (rangIsSelecting.value && rangSelectID.value != '') {
    let startid = rangSelectID.value
    let endid = ''
    const s: { [k: string]: any } = {}
    const children = myshareStore.ListDataShow
    let a = -1
    let b = -1
    for (let i = 0, maxi = children.length; i < maxi; i++) {
      if (children[i].share_key == shareKey) a = i
      if (children[i].share_key == startid) b = i
      if (a > 0 && b > 0) break
    }
    if (a >= 0 && b >= 0) {
      if (a > b) {
        ;[a, b] = [b, a]
        endid = shareKey
      } else {
        endid = startid
        startid = shareKey
      }
      for (let n = a; n <= b; n++) {
        s[children[n].share_key] = true
      }
    }
    rangSelectStart.value = startid
    rangSelectEnd.value = endid
    rangSelectFiles.value = s
    myshareStore.mRefreshListDataShow(false)
  }
}

const handleRefresh = () => ShareDAL.aReloadAllMyShare(true)
const handleSelectAll = () => myshareStore.mSelectAll()
const handleOrder = (order: string) => myshareStore.mOrderListData(order)
const handleSelect = (shareKey: string, event: any, isCtrl: boolean = false) => {
  onHideRightMenuScroll()
  if (rangIsSelecting.value) {
    if (!rangSelectID.value) {
      if (!myshareStore.ListSelected.has(shareKey)) {
        myshareStore.mMouseSelect(shareKey, true, false)
      }
      rangSelectID.value = shareKey
      rangSelectStart.value = shareKey
      rangSelectFiles.value = { [shareKey]: true }
    } else {
      const start = rangSelectID.value
      const children = myshareStore.ListDataShow
      let a = -1
      let b = -1
      for (let i = 0, maxi = children.length; i < maxi; i++) {
        if (children[i].share_key == shareKey) a = i
        if (children[i].share_key == start) b = i
        if (a > 0 && b > 0) break
      }
      const fileList: string[] = []
      if (a >= 0 && b >= 0) {
        if (a > b) [a, b] = [b, a]
        for (let n = a; n <= b; n++) {
          fileList.push(children[n].share_key)
        }
      }
      myshareStore.mRangSelect(shareKey, fileList)
      rangIsSelecting.value = false
      rangSelectID.value = ''
      rangSelectStart.value = ''
      rangSelectEnd.value = ''
      rangSelectFiles.value = {}
    }
    myshareStore.mRefreshListDataShow(false)
  } else {
    myshareStore.mMouseSelect(shareKey, event.ctrlKey || isCtrl, event.shiftKey)
    if (!myshareStore.ListSelected.has(shareKey)) {
      myshareStore.ListFocusKey = ''
    }
  }
}

const handleClickName = (share: IManagedShareItem) => {
  if (getDriveProviderCapabilities(share.account_provider).editCreatedShares) handleEdit(share)
  else handleOpenLink(share)
}
const handleEdit = (share: any) => {
  let list: IManagedShareItem[]
  if (share && share.share_key) {
    list = [share]
  } else {
    list = myshareStore.GetSelected()
  }
  const accountIds = new Set(list.map((item) => item.account_id))
  const canEdit = list.length > 0 && accountIds.size == 1 && list.every((item) => getDriveProviderCapabilities(item.account_provider).editCreatedShares)
  if (canEdit) modalEditShareLink(list)
  else {
    message.error(list.length > 0 ? '只能修改同一账号下支持编辑的分享链接' : '没有选中任何分享链接！')
  }
}
const handleOpenLink = (input?: IManagedShareItem) => {
  const share = input?.share_key ? input : myshareStore.GetSelectedFirst()
  if (!share) {
    message.error('没有选中分享链接！')
  } else {
    modalShowShareLink(share.share_id, share.share_pwd, '', false, [])
  }
}
const handleCopySelectedLink = () => {
  const list = myshareStore.GetSelected()
  let link = ''
  for (let i = 0, maxi = list.length; i < maxi; i++) {
    const item = list[i]
    link += GetShareUrlFormate(item.share_name, item.share_url, item.share_pwd) + '\n'
  }
  if (list.length == 0) {
    message.error('没有选中分享链接！')
  } else {
    copyToClipboard(link)
    message.success('分享链接已复制到剪切板(' + list.length.toString() + ')')
  }
}
const handleBrowserLink = () => {
  const first = myshareStore.GetSelectedFirst()
  if (!first) return
  if (first.share_url) openExternal(first.share_url)
  if (first.share_pwd) {
    copyToClipboard(first.share_pwd)
    message.success('提取码已复制到剪切板')
  }
}
const handleDeleteSelectedLink = (delby: any) => {
  const name = delby == 'selected' ? '取消选中的分享' : delby == 'expired' ? '清理全部过期已失效' : '清理全部文件已删除'
  let list: IManagedShareItem[]
  if (delby == 'selected') {
    list = myshareStore.GetSelected()
  } else {
    list = []
    const allList = myshareStore.AccountFilter ? myshareStore.ListDataRaw.filter((item) => item.account_id == myshareStore.AccountFilter) : myshareStore.ListDataRaw
    let item: IManagedShareItem
    for (let i = 0, maxi = allList.length; i < maxi; i++) {
      item = allList[i]
      if (!getDriveProviderCapabilities(item.account_provider).cancelCreatedShares) continue
      if (delby == 'expired') {
        if (item.expired && item.first_file) list.push(item)
      } else {
        if (!item.first_file) list.push(item)
      }
    }
  }
  if (list.length == 0) {
    message.error('没有需要清理的分享链接！')
    return
  }
  const cancelShares = async () => {
    const accountGroups = new Map<string, IManagedShareItem[]>()
    for (const item of list) {
      const group = accountGroups.get(item.account_id) || []
      group.push(item)
      accountGroups.set(item.account_id, group)
    }
    const deletedKeys: string[] = []
    for (const [accountId, items] of accountGroups) {
      const success = await AliShare.ApiCancelShareBatch(accountId, ArrayKeyList<string>('share_id', items))
      const successIds = new Set(success)
      deletedKeys.push(...items.filter((item) => successIds.has(item.share_id)).map((item) => item.share_key))
    }
    myshareStore.mDeleteFiles(deletedKeys)
    if (deletedKeys.length > 0) message.success(`${name}成功 ${deletedKeys.length} 条`)
  }
  if (delby == 'selected') {
    Modal.open({
      title: name,
      okText: '继续',
      bodyStyle: { minWidth: '340px' },
      content: () =>
        h('div', {
          style: 'color: red',
          innerText: '该操作不可逆，是否继续？'
        }),
      onOk: cancelShares
    })
  } else {
    cancelShares()
  }
}

const handleSearchInput = (value: string) => {
  myshareStore.mSearchListData(value)
  viewlist.value.scrollIntoView(0)
}
const handleSearchEnter = (event: any) => {
  event.target.blur()
  viewlist.value.scrollIntoView(0)
}
const handleRightClick = (e: { event: MouseEvent; node: any }) => {
  const key = e.node.key

  if (!myshareStore.ListSelected.has(key)) myshareStore.mMouseSelect(key, false, false)
  onShowRightMenu('rightmysharemenu', e.event.clientX, e.event.clientY)
}
</script>

<template>
  <div class="toppanbtns">
    <div v-if="!myshareStore.IsListSelected" class="toppanbtn">
      <a-button type="text" size="small" tabindex="-1" :loading="myshareStore.ListLoading" title="F5" @click="handleRefresh">
        <template #icon><IconFont name="iconreload-1-icon" /></template>
        刷新
      </a-button>
    </div>
    <div v-if="myshareStore.IsListSelected" class="toppanbtn">
      <a-button v-if="canEditSelected" type="text" size="small" tabindex="-1" title="F2 / Ctrl+E" @click="handleEdit">
        <IconFont name="iconedit-square" />
        修改
      </a-button>
      <a-button type="text" size="small" tabindex="-1" title="Ctrl+O" @click="handleOpenLink">
        <IconFont name="iconchakan" />
        查看
      </a-button>
      <a-button type="text" size="small" tabindex="-1" title="Ctrl+C" @click="handleCopySelectedLink">
        <IconFont name="iconcopy" />
        复制链接
      </a-button>
      <a-button type="text" size="small" tabindex="-1" title="Ctrl+B" @click="handleBrowserLink">
        <IconFont name="iconchrome" />
        浏览器
      </a-button>
      <a-button v-if="canCancelSelected" type="text" size="small" tabindex="-1" class="danger" title="Ctrl+Delete" @click="handleDeleteSelectedLink('selected')">
        <IconFont name="icondelete" />
        取消分享
      </a-button>
    </div>
    <div v-else-if="canCancelVisibleShares" class="toppanbtn">
      <a-dropdown trigger="hover" position="bl" @select="handleDeleteSelectedLink">
        <a-button type="text" size="small" tabindex="-1">
          <IconFont name="iconrest" />
          清理全部
          <IconFont name="icondown" />
        </a-button>
        <template #content>
          <a-doption :value="'expired'" class="danger">删除全部 过期已失效</a-doption>
          <a-doption :value="'deleted'" class="danger">删除全部 文件已删除</a-doption>
        </template>
      </a-dropdown>
    </div>
    <div class="toolbar-spacer"></div>
    <div class="toppanbtn">
      <a-input-search
        ref="inputsearch"
        tabindex="-1"
        size="small"
        title="Ctrl+F / F3 / Space"
        placeholder="快速筛选"
        allow-clear
        @clear="(e: any) => handleSearchInput('')"
        v-model="myshareStore.ListSearchKey"
        @input="(val: any) => handleSearchInput(val as string)"
        @press-enter="handleSearchEnter"
        @keydown.esc=";($event.target as any).blur()"
      />
    </div>
  </div>
  <div class="toppanarea">
    <div class="list-selection-primary">
      <AntdTooltip title="点击全选" placement="left">
        <a-button shape="circle" type="text" tabindex="-1" class="select all" title="Ctrl+A" @click="handleSelectAll">
          <IconFont :name="myshareStore.IsListSelectedAll ? 'iconrsuccess' : 'iconpic2'" />
        </a-button>
      </AntdTooltip>
      <div class="selectInfo">{{ myshareStore.ListDataSelectCountInfo }}</div>
      <div class="list-selection-actions">
        <AntdTooltip placement="rightTop" v-if="myshareStore.ListDataShow.length > 0">
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
        <a-button shape="square" v-if="!rangIsSelecting && myshareStore.ListSelected.size > 0 && myshareStore.ListSelected.size < myshareStore.ListDataShow.length" type="text" tabindex="-1" class="qujian" status="normal" @click="onSelectReverse">
          反向选择
        </a-button>
        <a-button shape="square" v-if="!rangIsSelecting && myshareStore.ListSelected.size > 0" type="text" tabindex="-1" class="qujian" status="normal" @click="onSelectCancel">取消已选</a-button>
      </div>
    </div>
    <div class="toolbar-spacer"></div>
    <div v-if="!myshareStore.AccountFilter" class="cell account">账号</div>
    <div class="cell tiquma">提取码</div>
    <div :class="'cell sharetime order ' + (myshareStore.ListOrderKey == 'state' ? 'active' : '')" @click="handleOrder('state')">
      有效期
      <IconFont name="iconxia" />
    </div>
    <div :class="'cell count order ' + (myshareStore.ListOrderKey == 'preview' ? 'active' : '')" @click="handleOrder('preview')">
      浏览数
      <IconFont name="iconxia" />
    </div>
    <div :class="'cell count order responsive-column-tertiary ' + (myshareStore.ListOrderKey == 'download' ? 'active' : '')" @click="handleOrder('download')">
      下载数
      <IconFont name="iconxia" />
    </div>
    <div :class="'cell count order responsive-column-secondary ' + (myshareStore.ListOrderKey == 'save' ? 'active' : '')" @click="handleOrder('save')">
      转存数
      <IconFont name="iconxia" />
    </div>
    <div :class="'cell sharetime order ' + (myshareStore.ListOrderKey == 'time' ? 'active' : '')" @click="handleOrder('time')">
      创建时间
      <IconFont name="iconxia" />
    </div>
    <div class="cell pr"></div>
  </div>
  <div class="toppanlist" @keydown.space.prevent="() => true">
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
        itemKey: 'share_key'
      }"
      :data="myshareStore.ListDataShow"
      :loading="myshareStore.ListLoading"
      tabindex="-1"
      @scroll="onHideRightMenuScroll"
    >
      <template #empty>
        <a-empty description="暂无分享链接" />
      </template>

      <template #item="{ item, index }">
        <div :key="item.share_key" class="listitemdiv">
          <div
            :class="'fileitem' + (myshareStore.ListSelected.has(item.share_key) ? ' selected' : '') + (myshareStore.ListFocusKey == item.share_key ? ' focus' : '')"
            @click="handleSelect(item.share_key, $event)"
            @mouseover="onSelectRang(item.share_key)"
            @contextmenu="(event: MouseEvent) => handleRightClick({ event, node: { key: item.share_key } })"
          >
            <div :class="'rangselect ' + (rangSelectFiles[item.share_key] ? (rangSelectStart == item.share_key ? 'rangstart' : rangSelectEnd == item.share_key ? 'rangend' : 'rang') : '')">
              <a-button shape="circle" type="text" tabindex="-1" class="select" :title="index" @click.prevent.stop="handleSelect(item.share_key, $event, true)">
                <IconFont :name="myshareStore.ListSelected.has(item.share_key) ? 'iconrsuccess' : 'iconpic2'" />
              </a-button>
            </div>
            <div class="fileicon">
              <IconFont :name="item.icon" aria-hidden="true" />
            </div>
            <div class="filename workspace-primary-name">
              <div :title="item.share_url || 'https://www.aliyundrive.com/s/' + item.share_id" @click="handleClickName(item)">
                {{ item.share_name }}
              </div>
            </div>
            <div v-if="!myshareStore.AccountFilter" class="cell account" :title="item.account_name">{{ item.account_name }}</div>
            <div class="cell tiquma">{{ item.share_pwd || '无' }}</div>
            <div v-if="item.status == 'forbidden'" class="cell sharestate forbidden">分享违规</div>
            <div v-else-if="item.expired" class="cell sharestate expired">过期失效</div>
            <div v-else-if="!item.first_file" class="cell sharestate deleted">文件已删</div>
            <div v-else class="cell sharestate active">{{ item.share_msg }}</div>
            <div class="cell count">{{ humanCount(item.preview_count) }}</div>
            <div class="cell count responsive-column-tertiary">{{ humanCount(item.download_count) }}</div>
            <div class="cell count responsive-column-secondary">{{ humanCount(item.save_count) }}</div>

            <div class="cell sharetime">{{ item.created_at.replace(' ', '\n') }}</div>
          </div>
        </div>
      </template>
    </a-list>
    <a-dropdown id="rightmysharemenu" class="rightmenu" :popup-visible="true" style="z-index: -1; left: -200px; opacity: 0">
      <template #content>
        <a-doption v-if="canEditSelected" @click="handleEdit">
          <template #icon><IconFont name="iconedit-square" /></template>
          <template #default>修改</template>
        </a-doption>
        <a-doption @click="handleOpenLink">
          <template #icon><IconFont name="iconchakan" /></template>
          <template #default>查看</template>
        </a-doption>

        <a-doption @click="handleCopySelectedLink">
          <template #icon><IconFont name="iconcopy" /></template>
          <template #default>复制链接</template>
        </a-doption>
        <a-doption @click="handleBrowserLink">
          <template #icon><IconFont name="iconchrome" /></template>
          <template #default>浏览器</template>
        </a-doption>

        <a-doption v-if="canCancelSelected" class="danger" @click="handleDeleteSelectedLink('selected')">
          <template #icon><IconFont name="icondelete" /></template>
          <template #default>取消分享</template>
        </a-doption>
      </template>
    </a-dropdown>
  </div>
</template>
