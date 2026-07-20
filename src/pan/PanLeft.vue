<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watchEffect } from 'vue'

import { Tree as AntdTree } from 'ant-design-vue'
import { Modal } from '@arco-design/web-vue'
import collapseMotion from 'ant-design-vue/es/_util/collapseMotion'
import { Trash2 } from 'lucide-vue-next'
import usePanTreeStore, { PanTreeState } from './pantreestore'
import MySwitchTab from '../layout/MySwitchTab.vue'
import { KeyboardState, useAppStore, useKeyboardStore, usePanFileStore, useSettingStore } from '../store'
import PanDAL from './pandal'
import UserDAL from '../user/userdal'
import useUserStore from '../user/userstore'
import { onHideRightMenuScroll, onShowRightMenu, TestCtrl } from '../utils/keyboardhelper'
import DirLeftMenu from './menus/DirLeftMenu.vue'
import FolderPreviewPopover from './menus/FolderPreviewPopover.vue'
import { TreeNodeData } from '../store/treestore'
import { dropMoveSelectedFile, uploadLocalPaths } from './topbtns/topbtn'
import message from '../utils/message'
import { GetDriveType, isS3User, isWebDavUser } from '../aliapi/utils'
import useCurrentDriveProvider from './useCurrentDriveProvider'
import { loadDriveAccountOptions, toDriveAccountOption, type DriveAccountOption } from '../utils/driveAccount'
import { getWebDavConnectionId, removeWebDavConnection } from '../utils/webdavClient'
import { getS3ConnectionId, removeS3Connection } from '../utils/s3Client'
import { getDriveProviderSidebarEntries, getDriveSidebarIcon, isDriveSidebarKey } from '../utils/driveProvider'

const treeref = ref()
const inputselectType = ref('backup')
const appStore = useAppStore()
const pantreeStore = usePanTreeStore()
const settingStore = useSettingStore()
const userStore = useUserStore()
const { provider, capabilities: providerCapabilities } = useCurrentDriveProvider()
const treeViewportRef = ref<HTMLElement | null>(null)
const treeViewportHeight = ref(360)
const treeHeight = computed(() => Math.max(160, treeViewportHeight.value))
const quickHeight = computed(() => Math.max(120, treeViewportHeight.value - (providerCapabilities.value.colorTag ? 180 : 0)))
const treeMotion = collapseMotion('mnemo-tree-collapse', false)

const keyboardStore = useKeyboardStore()
keyboardStore.$subscribe((_m: any, state: KeyboardState) => {
  if (appStore.appTab != 'pan') return
  if (TestCtrl('1', state.KeyDownEvent, () => handleQuickSelect(1))) return
  if (TestCtrl('2', state.KeyDownEvent, () => handleQuickSelect(2))) return
  if (TestCtrl('3', state.KeyDownEvent, () => handleQuickSelect(3))) return
  if (TestCtrl('4', state.KeyDownEvent, () => handleQuickSelect(4))) return
  if (TestCtrl('5', state.KeyDownEvent, () => handleQuickSelect(5))) return
  if (TestCtrl('6', state.KeyDownEvent, () => handleQuickSelect(6))) return
  if (TestCtrl('7', state.KeyDownEvent, () => handleQuickSelect(7))) return
  if (TestCtrl('8', state.KeyDownEvent, () => handleQuickSelect(8))) return
  if (TestCtrl('9', state.KeyDownEvent, () => handleQuickSelect(9))) return
})

const switchValues = [
  { key: 'wangpan', title: '网盘文件', alt: '' },
  { key: 'kuaijie', title: '快捷方式', alt: '' }
]

let DriveID = pantreeStore.drive_id
pantreeStore.$subscribe((_m: any, state: PanTreeState) => {
  if (state.drive_id != DriveID) {
    DriveID = state.drive_id
    inputselectType.value = GetDriveType(state.user_id, state.drive_id).name
    folderPreviewRef.value?.cancel()
  }
})

const colorTreeData = ref<TreeNodeData[]>([])
watchEffect(() => {
  const list = settingStore.uiFileColorArray
  const nodeList: TreeNodeData[] = []
  for (let i = 0; i < list.length; i++) {
    nodeList.push({
      __v_skip: true,
      key: 'color' + list[i].key.replace('#', 'c') + ' ' + (list[i].title || list[i].key),
      parent_file_id: '',
      title: list[i].title || list[i].key,
      namesearch: list[i].key.replace('#', 'c'),
      children: [],
      isLeaf: true
    } as TreeNodeData)
  }
  Object.freeze(nodeList)
  colorTreeData.value = nodeList
})
watchEffect(() => {
  const scrollToDir = pantreeStore.scrollToDir
  if (scrollToDir) treeref.value.scrollTo({ key: scrollToDir, align: 'top', offset: 220 })
  pantreeStore.mSaveTreeScrollTo('')
})

const handleTreeRightClick = (e: { event: MouseEvent; node: any }) => {
  const { parent = undefined, key } = e.node
  if (key.startsWith('search')) return
  const isSingleRootDrive = provider.value !== 'aliyun'
  if (!isSingleRootDrive && key.length < 40) return
  pantreeStore.mTreeSelected(e)
  onShowRightMenu('leftpanmenu', e.event.clientX, e.event.clientY)
}

const onRowItemDragEnter = (ev: any) => {
  ev.stopPropagation()
  ev.preventDefault()
  ev.target.style.outline = '2px dotted #637dff'
  ev.target.style.background = 'rgba(var(--primary-6),0.3)'
  ev.dataTransfer.dropEffect = 'move'
}
const onRowItemDragLeave = (ev: any) => {
  ev.stopPropagation()
  ev.preventDefault()
  ev.target.style.outline = 'none'
  ev.target.style.background = ''
}
const onRowItemDragOver = (ev: any) => {
  ev.stopPropagation()
  ev.preventDefault()
}

const onRowItemDrop = (ev: any, data: any) => {
  ev.stopPropagation()
  ev.preventDefault()
  ev.target.style.outline = 'none'
  ev.target.style.background = ''
  const filesList = ev.dataTransfer.files
  if (filesList && filesList.length > 0) {
    const files: string[] = []
    for (let i = 0, maxi = filesList.length; i < maxi; i++) {
      const path = filesList[i].path
      files.push(path)
    }
    void uploadLocalPaths(files, data.key)
  } else {
    dropMoveSelectedFile(data.drive_id, data.key, true)
  }
}

const onQuickDrop = (ev: any) => {
  ev.preventDefault()
  ev.target.style.outline = 'none'
  ev.target.style.background = ''

  const list: { key: string; drive_id: string; drive_name: string; title: string }[] = []
  const selectedFile = usePanFileStore().GetSelected()
  for (let i = 0, maxi = selectedFile.length; i < maxi; i++) {
    if (selectedFile[i].isDir) {
      list.push({
        key: selectedFile[i].file_id,
        drive_id: selectedFile[i].drive_id,
        drive_name: GetDriveType(pantreeStore.user_id, selectedFile[i].drive_id).title,
        title: selectedFile[i].name
      })
    }
  }
  if (list.length == 0) {
    message.error('没有选择任何文件夹！')
    return
  }
  PanDAL.updateQuickFile(list)
}
const handleQuickDelete = (key: string) => {
  PanDAL.deleteQuickFile(key)
}
const handleQuickSelect = (index: number) => {
  const array = PanDAL.getQuickFileList()
  if (array.length >= index) {
    const key = array[index - 1].key
    const drive_id = array[index - 1].drive_id
    console.log('handleQuickSelect', array)
    PanDAL.aReLoadOneDirToShow(drive_id, key, true)
  }
}
const filterTreeData = computed(() => {
  const token = UserDAL.GetUserToken(pantreeStore.user_id || '')
  const entries = getDriveProviderSidebarEntries(provider.value, token, {
    hideResourceDrive: settingStore.securityHideResourceDrive,
    hideBackupDrive: settingStore.securityHideBackupDrive,
    hideAlbum: settingStore.securityHidePicDrive
  })
  const nodeMap = new Map(pantreeStore.treeData.map((item) => [String(item.key), item]))
  const roots = pantreeStore.treeData.filter((item) => !isDriveSidebarKey(String(item.key)))
  const sidebarNodes = entries.map((entry) => {
    const source = nodeMap.get(entry.key)
    return {
      ...(source || {}),
      __v_skip: true,
      key: entry.key,
      drive_id: entry.driveId || source?.drive_id,
      parent_file_id: '',
      title: entry.title,
      namesearch: '',
      icon: entry.icon,
      children: source?.children || [],
      isLeaf: entry.kind === 'feature' || entry.key === 'pic_root'
    } as TreeNodeData
  })
  return [...roots, ...sidebarNodes]
})

const folderPreviewRef = ref<{ open: (target: HTMLElement, params: any) => void; leave: () => void; cancel: () => void } | null>(null)

const isPreviewableNode = (data: TreeNodeData | undefined): boolean => {
  if (!settingStore.uiFolderPreviewEnabled) return false
  if (!data) return false
  const key = String(data.key || '')
  if (!key) return false
  if (isDriveSidebarKey(key)) return false
  if (key.startsWith('search') || key.startsWith('color')) return false
  if (data.isLeaf === true) {
    // leaf placeholder, but still might be a real folder; only block if no drive_id
  }
  const userId = pantreeStore.user_id || ''
  const isSingleRootDrive = provider.value !== 'aliyun'
  if (!isSingleRootDrive && key.length < 40) return false
  return true
}

const onTreeNodeEnter = (ev: MouseEvent, data: TreeNodeData) => {
  if (!isPreviewableNode(data)) return
  const target = ev.currentTarget as HTMLElement
  if (!target) return
  const driveId = data.drive_id || pantreeStore.drive_id
  const userId = pantreeStore.user_id || ''
  if (!userId || !driveId) return
  folderPreviewRef.value?.open(target, {
    user_id: userId,
    drive_id: driveId,
    file_id: data.key,
    name: data.title,
    path: (data as any).path || ''
  })
}

const onTreeNodeLeave = () => {
  folderPreviewRef.value?.leave()
}

const onTreeScroll = () => {
  onHideRightMenuScroll()
  folderPreviewRef.value?.cancel()
}

const driveSwitcherLabel = computed(() => {
  const userId = pantreeStore.user_id || ''
  if (!userId) return '选择网盘账号'
  try {
    const token = UserDAL.GetUserToken(userId)
    const account = toDriveAccountOption(token)
    return `${account.providerLabel} · ${account.name}`
  } catch {
    return '网盘账号'
  }
})

const currentDriveAccount = computed(() => {
  const userId = pantreeStore.user_id || ''
  if (!userId) return undefined
  try {
    return toDriveAccountOption(UserDAL.GetUserToken(userId))
  } catch {
    return undefined
  }
})
const driveAccounts = ref<DriveAccountOption[]>([])
const isSwitchingDrive = ref(false)
const isDriveSwitcherOpen = ref(false)
const driveSwitcherRef = ref<HTMLElement | null>(null)
const driveSwitcherWidth = ref(220)
let driveSwitcherObserver: ResizeObserver | undefined
let treeViewportObserver: ResizeObserver | undefined

onMounted(() => {
  if (driveSwitcherRef.value) {
    const syncWidth = () => {
      driveSwitcherWidth.value = Math.round(driveSwitcherRef.value?.getBoundingClientRect().width || 220)
    }
    syncWidth()
    driveSwitcherObserver = new ResizeObserver(syncWidth)
    driveSwitcherObserver.observe(driveSwitcherRef.value)
  }
  if (treeViewportRef.value) {
    const syncHeight = () => {
      const viewport = treeViewportRef.value
      if (!viewport) return
      const style = window.getComputedStyle(viewport)
      const verticalPadding = Number.parseFloat(style.paddingTop) + Number.parseFloat(style.paddingBottom)
      treeViewportHeight.value = Math.max(160, Math.floor(viewport.clientHeight - verticalPadding))
    }
    syncHeight()
    treeViewportObserver = new ResizeObserver(syncHeight)
    treeViewportObserver.observe(treeViewportRef.value)
  }
})

onBeforeUnmount(() => {
  driveSwitcherObserver?.disconnect()
  treeViewportObserver?.disconnect()
})

const refreshDriveAccounts = async (visible: boolean) => {
  isDriveSwitcherOpen.value = visible
  if (!visible) return
  driveAccounts.value = await loadDriveAccountOptions().catch(() => UserDAL.GetUserList().map(toDriveAccountOption))
}

const handleSwitchDriveAccount = async (userId: string) => {
  if (!userId || isSwitchingDrive.value) return
  if (userId === userStore.user_id) {
    isDriveSwitcherOpen.value = false
    return
  }
  isSwitchingDrive.value = true
  try {
    const changed = await UserDAL.UserChange(userId)
    if (changed) isDriveSwitcherOpen.value = false
    else message.error('切换网盘账号失败，请重新登录')
  } finally {
    isSwitchingDrive.value = false
  }
}

const handleRemoveDriveAccount = (account: DriveAccountOption) => {
  if (!account.user_id || isSwitchingDrive.value) return
  Modal.confirm({
    title: '移除网盘账号',
    content: `确定移除“${account.name}”吗？这只会删除本地登录信息，不会删除网盘中的文件。`,
    okText: '移除',
    cancelText: '取消',
    okButtonProps: { status: 'danger' },
    onOk: async () => {
      isSwitchingDrive.value = true
      try {
        const token = UserDAL.GetUserToken(account.user_id)
        if (isWebDavUser(token)) removeWebDavConnection(getWebDavConnectionId(token.default_drive_id || token.user_id))
        if (isS3User(token)) removeS3Connection(getS3ConnectionId(token.default_drive_id || token.user_id))
        if (account.user_id === userStore.user_id) await UserDAL.UserLogOff(account.user_id)
        else await UserDAL.UserClearFromDB(account.user_id)
        driveAccounts.value = await loadDriveAccountOptions().catch(() => UserDAL.GetUserList().map(toDriveAccountOption))
        message.success('网盘账号已从本地移除')
      } catch (error: any) {
        message.error(error?.message || '移除网盘账号失败')
        throw error
      } finally {
        isSwitchingDrive.value = false
      }
    }
  })
}

const handleOpenDriveLogin = () => {
  isDriveSwitcherOpen.value = false
  userStore.userShowLogin = true
}
</script>
<template>
  <aside class="pan-left" tabindex="-1" @keydown.tab.prevent="() => true">
    <a-dropdown trigger="click" position="br" :popup-visible="isDriveSwitcherOpen" @popup-visible-change="refreshDriveAccounts">
      <div ref="driveSwitcherRef" class="pan-drive-switcher">
        <button type="button" class="pan-drive-switcher-btn" :aria-expanded="isDriveSwitcherOpen">
          <span class="meta">
            <span class="drive-provider-mark" aria-hidden="true">
              <img v-if="currentDriveAccount?.icon" :src="currentDriveAccount.icon" alt="" />
              <IconFont v-else name="iconyunpan" />
            </span>
            <span class="label">{{ driveSwitcherLabel }}</span>
          </span>
          <IconFont name="icondown" />
        </button>
      </div>
      <template #content>
        <div class="pan-drive-menu" :style="{ '--pan-drive-menu-width': `${driveSwitcherWidth}px` }">
          <div v-if="driveAccounts.length" class="pan-drive-menu-list">
            <div v-for="account in driveAccounts" :key="account.user_id" class="pan-drive-menu-item" :class="{ active: account.user_id === userStore.user_id }">
              <button type="button" class="pan-drive-menu-account" :disabled="isSwitchingDrive" @click="handleSwitchDriveAccount(account.user_id)">
                <span class="drive-provider-mark" aria-hidden="true">
                  <img v-if="account.icon" :src="account.icon" alt="" />
                  <span v-else>{{ account.providerLabel.slice(0, 1) }}</span>
                </span>
                <span>{{ account.name }}</span>
                <small :title="account.detail">{{ account.detail }}</small>
                <IconFont v-if="account.user_id === userStore.user_id" name="iconrsuccess" class="current" />
              </button>
              <button type="button" class="pan-drive-menu-remove" :disabled="isSwitchingDrive" :title="`移除 ${account.name}`" :aria-label="`移除 ${account.name}`" @click="handleRemoveDriveAccount(account)">
                <Trash2 :size="14" />
              </button>
            </div>
          </div>
          <div v-else class="pan-drive-menu-empty">还没有登录网盘账号</div>
          <button type="button" class="pan-drive-menu-login" @click="handleOpenDriveLogin">
            <IconFont name="iconadd" />
            <span>添加网盘</span>
          </button>
        </div>
      </template>
    </a-dropdown>
    <div class="headswitch">
      <div class="bghr"></div>
      <div class="sw">
        <MySwitchTab :name="'panleft'" :tabs="switchValues" :value="appStore.GetAppTabMenu" @update:value="(val: string) => appStore.toggleTabMenu('pan', val)" />
      </div>
    </div>
    <div ref="treeViewportRef" class="treeleft">
      <a-tabs type="text" :direction="'horizontal'" class="hidetabs" :justify="true" :active-key="appStore.GetAppTabMenu">
        <a-tab-pane key="wangpan" title="1">
          <div v-if="!pantreeStore.user_id" class="pan-tree-empty">
            <IconFont name="iconyunpan" />
            <strong>登录后查看文件</strong>
            <a-button type="primary" size="small" @click="handleOpenDriveLogin">登录网盘账号</a-button>
          </div>
          <AntdTree
            v-else
            ref="treeref"
            :tabindex="-1"
            :focusable="false"
            class="dirtree"
            block-node
            selectable
            :auto-expand-parent="false"
            show-icon
            :height="treeHeight"
            :style="{ height: treeHeight + 'px' }"
            :item-height="30"
            :show-line="{ showLeafIcon: false }"
            :motion="treeMotion"
            :expanded-keys="pantreeStore.treeExpandedKeys"
            :selected-keys="pantreeStore.treeSelectedKeys"
            :tree-data="filterTreeData"
            @select="(_: any[], e: any) => pantreeStore.mTreeSelected(e, false)"
            @expand="(_: any[], e: any) => pantreeStore.mTreeExpand(e.node.key)"
            @right-click="handleTreeRightClick"
            @scroll="onTreeScroll">
            <template #switcherIcon>
              <IconFont name="iconarrow-right-2-icon" />
            </template>
            <template #icon="{ dataRef }">
              <IconFont :name="typeof dataRef.icon === 'string' ? dataRef.icon : getDriveSidebarIcon(String(dataRef.key || ''))" />
            </template>
            <template #title="{ dataRef }">
              <span
                v-if="provider !== 'aliyun' || String(dataRef.key).length == 40 || String(dataRef.key).includes('root') || String(dataRef.drive_id || '').startsWith('webdav:') || String(dataRef.drive_id || '').startsWith('s3:')"
                class="dirtitle treedragnode"
                @drop="onRowItemDrop($event, dataRef)"
                @dragover="onRowItemDragOver"
                @dragenter="onRowItemDragEnter"
                @dragleave="onRowItemDragLeave"
                @mouseenter="(ev: MouseEvent) => onTreeNodeEnter(ev, dataRef)"
                @mouseleave="onTreeNodeLeave">
                {{ dataRef.title }}
              </span>
              <span v-else class="dirtitle" @mouseenter="(ev: MouseEvent) => onTreeNodeEnter(ev, dataRef)" @mouseleave="onTreeNodeLeave">
                {{ dataRef.title }}
              </span>
            </template>
          </AntdTree>
        </a-tab-pane>
        <a-tab-pane key="kuaijie" title="2">
          <div class="shortcut-tree-layout">
            <AntdTree
              v-if="providerCapabilities.colorTag"
              :tabindex="-1"
              :focusable="false"
              class="colortree"
              block-node
              selectable
              :auto-expand-parent="false"
              show-icon
              :item-height="30"
              :show-line="false"
              :motion="treeMotion"
              :selected-keys="pantreeStore.treeSelectedKeys"
              :tree-data="colorTreeData"
              @select="(_: any[], e: any) => pantreeStore.mTreeSelected(e, true)">
              <template #icon="{ dataRef }">
                <IconFont name="iconwbiaoqian" :class="dataRef.namesearch" />
              </template>
              <template #title="{ dataRef }">
                <span :class="'dirtitle ' + dataRef.namesearch">标记 · {{ dataRef.title }}</span>
              </template>
            </AntdTree>
            <div class="quicktree-area" @drop="onQuickDrop($event)" @dragover="onRowItemDragOver" @dragenter="onRowItemDragEnter" @dragleave="onRowItemDragLeave">
              <AntdTree
                :tabindex="-1"
                :focusable="false"
                class="quicktree"
                block-node
                selectable
                :auto-expand-parent="false"
                show-icon
                :height="quickHeight"
                :style="{ height: quickHeight + 'px' }"
                :item-height="30"
                :show-line="false"
                :motion="treeMotion"
                :selected-keys="pantreeStore.treeSelectedKeys"
                :tree-data="pantreeStore.quickData"
                @select="(_: any[], e: any) => pantreeStore.mTreeSelected(e, true)">
                <template #icon>
                  <IconFont name="iconfile-folder" />
                </template>
                <template #title="{ dataRef }">
                  <div class="quickitem" @mouseenter="(ev: MouseEvent) => onTreeNodeEnter(ev, dataRef)" @mouseleave="onTreeNodeLeave">
                    <span class="quicktitle" :title="dataRef.namesearch">
                      {{ dataRef.title }}
                    </span>
                    <span class="quickbtn">
                      <a-button type="text" size="mini" title="删除快捷方式" aria-label="删除快捷方式" @click.stop="handleQuickDelete(dataRef.key)">
                        <IconFont name="icondelete" />
                      </a-button>
                    </span>
                  </div>
                </template>
              </AntdTree>
            </div>
          </div>
        </a-tab-pane>
      </a-tabs>
    </div>
    <DirLeftMenu :inputselectType="inputselectType" />
    <FolderPreviewPopover ref="folderPreviewRef" />
  </aside>
</template>

<style lang="less">
.pan-left .ant-tree.ant-tree-show-line .ant-tree-child-tree li:not(:last-child)::before {
  border-left: none !important;
}

.pan-left .ant-tree.ant-tree-show-line li:not(:last-child)::before {
  border-left: 1px dashed #d9d9d9;
  top: 10px;
  left: 11px;
}

.pan-left .dirtree .iconfont,
.pan-left .quicktree .iconfont {
  font-size: 17px;
  color: var(--text-secondary);
}

.pan-left .colortree .iconfont {
  font-size: 20px;
}

.pan-left .ant-tree-node-selected .ant-tree-iconEle .iconfont {
  color: rgb(var(--primary-6));
}

.pan-left .ant-tree-node-selected .ant-tree-title,
.pan-left .ant-tree-node-selected .ant-tree-title > span {
  color: rgb(var(--primary-6)) !important;
  font-weight: 500;
}

body[arco-theme='dark'] .pan-left .ant-tree-node-selected .ant-tree-title,
body[arco-theme='dark'] .pan-left .ant-tree-node-selected .ant-tree-title > span {
  color: rgb(255, 255, 255) !important;
}

.pan-left .quicktree .ant-tree-icon__customize .iconfont {
  font-size: 18px;
  margin-right: 2px;
}
</style>
