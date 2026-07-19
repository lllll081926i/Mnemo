<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watchEffect } from 'vue'

import { Tree as AntdTree } from 'ant-design-vue'
import usePanTreeStore, { PanTreeState } from './pantreestore'
import MySwitchTab from '../layout/MySwitchTab.vue'
import { KeyboardState, useAppStore, useKeyboardStore, usePanFileStore, useSettingStore, useWinStore } from '../store'
import PanDAL from './pandal'
import UserDAL from '../user/userdal'
import type { ITokenInfo } from '../user/userstore'
import useUserStore from '../user/userstore'
import { onHideRightMenuScroll, onShowRightMenu, TestCtrl } from '../utils/keyboardhelper'
import DirLeftMenu from './menus/DirLeftMenu.vue'
import FolderPreviewPopover from './menus/FolderPreviewPopover.vue'
import { TreeNodeData } from '../store/treestore'
import { dropMoveSelectedFile } from './topbtns/topbtn'
import message from '../utils/message'
import { modalUpload } from '../utils/modal'
import { GetDriveType, isAliyunUser, isBaiduUser, isBoxUser, isCloud123User, isCloud139User, isCloud189User, isDrive115User, isDropboxUser, isGuangyaUser, isOneDriveUser, isPikPakUser, isQuarkUser, isS3User, isWebDavUser } from '../aliapi/utils'

const treeref = ref()
const inputselectType = ref('backup')
const winStore = useWinStore()
// header 48 + drive switcher ~50 + segmented ~44 + paddings
const treeHeight = computed(() => Math.max(220, winStore.height - 48 - 50 - 44 - 24))
const quickHeight = computed(() => Math.max(160, winStore.height - 48 - 50 - 44 - 24 - 210))
const appStore = useAppStore()
const pantreeStore = usePanTreeStore()
const settingStore = useSettingStore()
const userStore = useUserStore()
const isCloudUser = computed(() => isCloud123User(pantreeStore.user_id || ''))
const isAliyunAccount = computed(() => isAliyunUser(pantreeStore.user_id || UserDAL.GetUserToken(pantreeStore.user_id || '')))

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
  const isSingleRootDrive =
    isCloud123User(pantreeStore.user_id || '') ||
    isDrive115User(pantreeStore.user_id || '') ||
    isBaiduUser(pantreeStore.user_id || '') ||
    isPikPakUser(pantreeStore.user_id || '') ||
    isDropboxUser(pantreeStore.user_id || '') ||
    isOneDriveUser(pantreeStore.user_id || '') ||
    isBoxUser(pantreeStore.user_id || '') ||
    isWebDavUser(pantreeStore.user_id || '') ||
    isS3User(pantreeStore.user_id || '')
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
    modalUpload(data.key, files)
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
  const isCloudUser = isCloud123User(pantreeStore.user_id || '') || isPikPakUser(pantreeStore.user_id || '') || isDropboxUser(pantreeStore.user_id || '') || isOneDriveUser(pantreeStore.user_id || '') || isBoxUser(pantreeStore.user_id || '')
  const baseList = isCloudUser
    ? pantreeStore.treeData.filter((item) => {
        if (item.key === 'backup_root') return false
        if (item.key === 'resource_root') return false
        if (item.key === 'pic_root') return false
        if (
          (isPikPakUser(pantreeStore.user_id || '') || isDropboxUser(pantreeStore.user_id || '') || isOneDriveUser(pantreeStore.user_id || '') || isBoxUser(pantreeStore.user_id || '')) &&
          (item.key === 'video' || item.key === 'recover' || item.key === 'favorite')
        )
          return false
        return true
      })
    : pantreeStore.treeData.filter((item) => {
        if (!isAliyunAccount.value && (item.key === 'backup_root' || item.key === 'resource_root')) {
          return false
        }
        if (isBaiduUser(pantreeStore.user_id || '') && item.key === 'trash') {
          return false
        }
        if (!isAliyunAccount.value && (item.key === 'pic_root' || item.key === 'video' || item.key === 'favorite' || item.key === 'recover')) {
          return false
        }
        if (useSettingStore().securityHideBackupDrive && item.key === 'backup_root') {
          return false
        }
        if ((useSettingStore().securityHideBackupDrive || useSettingStore().securityHideResourceDrive) && item.key === 'resource_root') {
          return false
        }
        if (useSettingStore().securityHidePicDrive && item.key === 'pic_root') {
          return false
        }
        if (!usePanTreeStore().resource_drive_id && item.key === 'resource_root') {
          return false
        }
        return true
      })

  return baseList
})

const folderPreviewRef = ref<{ open: (target: HTMLElement, params: any) => void; leave: () => void; cancel: () => void } | null>(null)

const SPECIAL_KEYS = new Set(['trash', 'recover', 'favorite', 'video', 'pic_root', 'backup_root', 'resource_root'])

const isPreviewableNode = (data: TreeNodeData | undefined): boolean => {
  if (!settingStore.uiFolderPreviewEnabled) return false
  if (!data) return false
  const key = String(data.key || '')
  if (!key) return false
  if (SPECIAL_KEYS.has(key)) return false
  if (key.startsWith('search') || key.startsWith('color')) return false
  if (data.isLeaf === true) {
    // leaf placeholder, but still might be a real folder; only block if no drive_id
  }
  const userId = pantreeStore.user_id || ''
  const isSingleRootDrive =
    isCloud123User(userId) ||
    isDrive115User(userId) ||
    isBaiduUser(userId) ||
    isPikPakUser(userId) ||
    isDropboxUser(userId) ||
    isOneDriveUser(userId) ||
    isBoxUser(userId) ||
    isWebDavUser(userId) ||
    isS3User(userId) ||
    isQuarkUser(userId) ||
    isCloud139User(userId) ||
    isCloud189User(userId) ||
    isGuangyaUser(userId)
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
  const driveId = pantreeStore.drive_id || ''
  if (!userId) return '选择网盘账号'
  try {
    const token = UserDAL.GetUserToken(userId)
    const nick = token?.nick_name || token?.user_name || userId
    const driveTitle = GetDriveType(userId, driveId)?.title || '网盘'
    return `${driveTitle} · ${nick}`
  } catch {
    return '网盘账号'
  }
})

const driveAccounts = ref<ITokenInfo[]>([])
const isSwitchingDrive = ref(false)
const isDriveSwitcherOpen = ref(false)
const driveSwitcherRef = ref<HTMLElement | null>(null)
const driveSwitcherWidth = ref(220)
let driveSwitcherObserver: ResizeObserver | undefined

onMounted(() => {
  if (!driveSwitcherRef.value) return
  const syncWidth = () => {
    driveSwitcherWidth.value = Math.round(driveSwitcherRef.value?.getBoundingClientRect().width || 220)
  }
  syncWidth()
  driveSwitcherObserver = new ResizeObserver(syncWidth)
  driveSwitcherObserver.observe(driveSwitcherRef.value)
})

onBeforeUnmount(() => driveSwitcherObserver?.disconnect())

const refreshDriveAccounts = async (visible: boolean) => {
  isDriveSwitcherOpen.value = visible
  if (!visible) return
  driveAccounts.value = await UserDAL.GetUserListFromDB().catch(() => UserDAL.GetUserList())
}

const handleSwitchDriveAccount = async (userId: string) => {
  if (!userId || userId === userStore.user_id || isSwitchingDrive.value) return
  isSwitchingDrive.value = true
  try {
    const changed = await UserDAL.UserChange(userId)
    if (!changed) message.error('切换网盘账号失败，请重新登录')
  } finally {
    isSwitchingDrive.value = false
  }
}

const handleOpenDriveLogin = () => {
  userStore.userShowLogin = true
}
</script>
<template>
  <aside class="pan-left" tabindex="-1" @keydown.tab.prevent="() => true">
    <a-dropdown trigger="click" position="br" @popup-visible-change="refreshDriveAccounts">
      <div ref="driveSwitcherRef" class="pan-drive-switcher">
        <button type="button" class="pan-drive-switcher-btn" :aria-expanded="isDriveSwitcherOpen">
          <span class="meta">
            <IconFont name="iconyunpan" class="drive-icon" />
            <span class="label">{{ driveSwitcherLabel }}</span>
          </span>
          <IconFont name="icondown" />
        </button>
      </div>
      <template #content>
        <div class="pan-drive-menu" :style="{ '--pan-drive-menu-width': `${driveSwitcherWidth}px` }">
          <div v-if="driveAccounts.length" class="pan-drive-menu-list">
            <button v-for="account in driveAccounts" :key="account.user_id" type="button" class="pan-drive-menu-item" :class="{ active: account.user_id === userStore.user_id }" :disabled="isSwitchingDrive" @click="handleSwitchDriveAccount(account.user_id)">
              <IconFont name="iconyunpan" />
              <span>{{ account.nick_name || account.user_name || account.user_id }}</span>
              <small>{{ account.tokenfrom || '网盘' }}</small>
              <IconFont v-if="account.user_id === userStore.user_id" name="iconrsuccess" class="current" />
            </button>
          </div>
          <div v-else class="pan-drive-menu-empty">还没有登录网盘账号</div>
          <button type="button" class="pan-drive-menu-login" @click="handleOpenDriveLogin">登录网盘账号</button>
        </div>
      </template>
    </a-dropdown>
    <div class="headswitch">
      <div class="bghr"></div>
      <div class="sw">
        <MySwitchTab :name="'panleft'" :tabs="switchValues" :value="appStore.GetAppTabMenu" @update:value="(val: string) => appStore.toggleTabMenu('pan', val)" />
      </div>
    </div>
    <div class="treeleft">
      <a-tabs type="text" :direction="'horizontal'" class="hidetabs" :justify="true" :active-key="appStore.GetAppTabMenu">
        <a-tab-pane key="wangpan" title="1">
          <div v-if="!pantreeStore.user_id" class="pan-tree-empty">
            <IconFont name="iconyunpan" />
            <strong>登录后查看网盘文件</strong>
            <span>回收站、全盘搜索和文件目录会在登录后显示</span>
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
            :open-animation="{}"
            :expanded-keys="pantreeStore.treeExpandedKeys"
            :selected-keys="pantreeStore.treeSelectedKeys"
            :tree-data="filterTreeData"
            @select="(_: any[], e: any) => pantreeStore.mTreeSelected(e, false)"
            @expand="(_: any[], e: any) => pantreeStore.mTreeExpand(e.node.key)"
            @right-click="handleTreeRightClick"
            @scroll="onTreeScroll">
            <template #switcherIcon>
              <i class="ant-tree-switcher-icon iconfont Arrow" />
            </template>
            <template #icon>
              <IconFont name="iconfile-folder" />
            </template>
            <template #title="{ dataRef }">
              <span
                v-if="String(dataRef.key).length == 40 || String(dataRef.key).includes('root') || String(dataRef.drive_id || '').startsWith('webdav:') || String(dataRef.drive_id || '').startsWith('s3:')"
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
          <AntdTree
            :tabindex="-1"
            :focusable="false"
            class="colortree"
            block-node
            selectable
            :auto-expand-parent="false"
            show-icon
            :style="{ marginLeft: '-18px' }"
            :item-height="30"
            :show-line="false"
            :open-animation="{}"
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
              :style="{ height: quickHeight + 'px', marginLeft: '-18px' }"
              :item-height="30"
              :show-line="false"
              :open-animation="{}"
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
                    <a-button type="text" size="mini" @click.stop="handleQuickDelete(dataRef.key)">删除</a-button>
                  </span>
                </div>
              </template>
            </AntdTree>
          </div>
        </a-tab-pane>
      </a-tabs>
    </div>
    <DirLeftMenu :inputselectType="inputselectType" />
    <FolderPreviewPopover ref="folderPreviewRef" />
  </aside>
</template>

<style lang="less">
.treeleft {
  margin-left: -6px;
}

.dirtree {
  height: 100%;
}

.ant-tree.ant-tree-show-line .ant-tree-child-tree li:not(:last-child)::before {
  border-left: none !important;
}

.ant-tree.ant-tree-show-line li:not(:last-child)::before {
  border-left: 1px dashed #d9d9d9;
  top: 10px;
  left: 11px;
}

.dirtree .iconfont,
.sharetree .iconfont,
.quicktree .iconfont,
.videotree .iconfont {
  font-size: 20px;
}

.dirtree .iconfont.iconfile-folder,
.sharetree .iconfont.iconfile-folder,
.quicktree .iconfont.iconfile-folder,
.videotree .iconfont.iconfile-folder {
  color: #ffb74d;
  font-size: 20px;
}

.colortree .iconfont {
  font-size: 20px;
}

.dirtree .iconfont.iconrecover {
  color: #13c2c2;
}

.dirtree .iconfont.icondelete {
  color: #ff4d4fd9;
}

.dirtree .iconfont.iconsearch {
  color: #1890ff;
}

.dirtree .iconfont.iconcrown {
  color: #ffb74d;
}

.dirtree .iconfont.iconrss_video {
  color: #a760ef;
}

.dirtree .iconfont.iconjietu {
  color: #a77566;
}

.colortree .iconfont.iconrss_video {
  color: #a760ef;
}

.ant-tree .iconfile-folder {
  color: #ffb74d;
  font-size: 20px;
}

.dirtitle {
  white-space: nowrap;
  word-break: keep-all;
}

.dirtitle.treedragnode {
  width: 100%;
  display: inline-block;
}

.dirtree .ant-tree-list-holder-inner .ant-tree-node-content-wrapper {
  flex-wrap: nowrap !important;
  flex-shrink: 0 !important;
  display: flex;
}

.dirtree .ant-tree-list-holder {
  overflow-x: hidden;
}

.dirtree .ant-tree-title {
  flex-grow: 1;
}

.ant-tree-node-selected .ant-tree-title,
.ant-tree-node-selected .ant-tree-title > span {
  color: rgb(var(--primary-6)) !important;
  font-weight: 500;
}

body[arco-theme='dark'] .ant-tree-node-selected .ant-tree-title,
body[arco-theme='dark'] .ant-tree-node-selected .ant-tree-title > span {
  color: rgb(255, 255, 255) !important;
}

.headswitch {
  width: 100%;
  height: 56px;
  overflow: hidden;
  text-align: center;
  justify-content: center;
  position: relative;
  padding-top: 16px;
  padding-bottom: 6px;
  margin-left: -18px;
  flex-shrink: 0;
  flex-grow: 0;
}

.headswitch .bghr {
  position: absolute;
  left: 0;
  right: 0;
  top: 32px;
  border-bottom: 1px solid var(--color-neutral-3);
  z-index: -1;
}

.headswitch .sw {
  margin: 0 auto;
  background: var(--color-bg-1);
  width: fit-content;
}

.rootsearch {
  width: calc(100% - 151px) !important;
  float: right;
}

.rootsearch.arco-input-wrapper {
  background-color: transparent;
  border: 1px solid rgb(var(--primary-6)) !important;
}

.colortree {
  height: 180px;
  flex-shrink: 0;
  flex-grow: 0;
}

.quicktree .ant-tree-icon__customize .iconfont {
  font-size: 18px;
  margin-right: 2px;
}

.quicktree .ant-tree-node-content-wrapper {
  flex: auto;
  display: flex !important;
  flex-direction: row;
}

.quicktree .ant-tree-title {
  flex: auto;
  display: flex !important;
  flex-direction: row;
}

.quickitem {
  display: flex;
}

.quickitem .quicktitle {
  flex-shrink: 1;
  flex-grow: 1;
  display: -webkit-box;
  max-height: 24px;
  word-break: break-all;
  overflow: hidden;
  text-overflow: ellipsis;
  -webkit-line-clamp: 1;
}

.quickitem .quickbtn {
  flex-shrink: 0;
  flex-grow: 0;
  padding-left: 2px;
  padding-right: 2px;
  font-size: 12px;
  color: var(--color-text-3);
}

.quicktree .quickbtn .arco-btn-size-mini {
  padding: 0 4px;
}

.quicktree .quickbtn .arco-btn-size-mini:hover,
.quicktree .quickbtn .arco-btn-size-mini:active {
  color: #fff !important;
  background: rgba(255, 77, 79, 0.85) !important;
}
</style>
