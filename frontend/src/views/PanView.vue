<script setup>
import { ref, watch, onMounted } from 'vue'
import {
  listDir, listTrash, search, mkdir, rename, trash, remove, restore,
  move, copy, favorite, download, createShare, uploadFiles, migrateFiles, formatBytes, formatTime,
  iconOf, extOf, onEvent,
} from '../api'

const props = defineProps({ account: Object, providerMeta: Object })
const emit = defineEmits(['toast'])

const files = ref([])
const dir = ref('root')
const pathStack = ref([])
const selected = ref([])
const keyword = ref('')
const mode = ref('list') // list | trash | search | favorite
const loading = ref(false)
const menu = ref(null)
const tree = ref({})
const expanded = ref({})

const caps = () => (props.account ? (props.providerMeta[providerOf()] || {}) : {})

function providerOf() {
  const acc = props.account
  if (!acc) return ''
  const uid = acc.UserID || ''
  for (const key of ['pikpak', 'onedrive', 'dropbox', 'pan123', 'lanzou', 'ilanzou', 'pan139', 'pan189', 'yike', 'aliopen', 'guangya']) {
    if (uid.startsWith(key + '_')) return key
  }
  if (uid.startsWith('webdav:')) return 'webdav'
  if (uid.startsWith('s3:')) return 's3'
  return ''
}

const c = caps()

async function load(d) {
  if (!props.account) return
  loading.value = true
  try {
    if (mode.value === 'trash') files.value = await listTrash(props.account.UserID, props.account.DriveID) || []
    else if (mode.value === 'search' && keyword.value) files.value = await search(props.account.UserID, props.account.DriveID, keyword.value) || []
    else files.value = await listDir(props.account.UserID, props.account.DriveID, d) || []
  } catch (e) {
    emit('toast', String(e), 'error')
  }
  loading.value = false
}

function open(file) {
  if (!file.isDir) {
    playOrDownload(file)
    return
  }
  pathStack.value.push(dir.value)
  dir.value = file.file_id
  load(dir.value)
}

function up() {
  if (!pathStack.value.length) return
  dir.value = pathStack.value.pop()
  load(dir.value)
}

function goSearch() {
  if (!keyword.value) return
  mode.value = 'search'
  load()
}

function showTrash() {
  mode.value = 'trash'
  load()
}

function backToList() {
  mode.value = 'list'
  keyword.value = ''
  load(dir.value)
}

function toggleSel(file) {
  const i = selected.value.findIndex((s) => s.file_id === file.file_id)
  if (i >= 0) selected.value.splice(i, 1)
  else selected.value.push(file)
}

function onCtx(e, file) {
  menu.value = { x: e.clientX, y: e.clientY, file }
}

function closeMenu() { menu.value = null }

async function act(kind) {
  const file = menu.value && menu.value.file
  closeMenu()
  if (!file || !props.account) return
  const uid = props.account.UserID, did = props.account.DriveID
  try {
    if (kind === 'download') await download(uid, did, file)
    else if (kind === 'rename') { const name = prompt('新名称', file.name); if (name) await rename(uid, did, file.file_id, name) }
    else if (kind === 'mkdir') { const name = prompt('文件夹名称'); if (name) await mkdir(uid, did, dir.value, name) }
    else if (kind === 'trash') await trash(uid, did, [file.file_id])
    else if (kind === 'delete') { if (confirm('永久删除？')) await remove(uid, did, [file.file_id]) }
    else if (kind === 'restore') await restore(uid, did, [file.file_id])
    else if (kind === 'favorite') await favorite(uid, did, true, [file.file_id])
    else if (kind === 'share') {
      const res = await createShare(uid, did, { fileIds: [file.file_id], shareName: file.name })
      if (res && res.ShareURL) { emit('toast', res.ShareURL); navigator.clipboard && navigator.clipboard.writeText(res.ShareURL) }
    }
    else if (kind === 'copy') { const target = prompt('目标文件夹 ID'); if (target) await copy(uid, did, [file.file_id], target) }
    else if (kind === 'move') { const target = prompt('目标文件夹 ID'); if (target) await move(uid, did, [file.file_id], target) }
    else if (kind === 'play') { await play(file) }
    else if (kind === 'upload') { const paths = prompt('本地路径（逗号分隔）'); if (paths) await uploadFiles(uid, did, dir.value, paths.split(',').map((s) => s.trim())) }
    else if (kind === 'migrate') {
      const target = prompt('目标账号 user_id、目标目录（空格分隔，留空目录=root）')
      if (target) {
        const parts = target.trim().split(/\s+/)
        const dstUser = parts[0]
        const dstParent = parts[1] || 'root'
        const dstDrive = prompt('目标账号 drive_id（回车自动）') || dstUser
        await migrateFiles(uid, did, dstUser, dstDrive, dstParent, [file.file_id], false)
      }
    }
    load(mode.value === 'trash' ? null : dir.value)
    emit('toast', '操作成功', 'success')
  } catch (e) {
    emit('toast', String(e), 'error')
  }
}

async function play(file) {
  const { PlayVideo, MediaProxy, LocalPreviewURL } = await import('../../wailsjs/go/app/App')
  try {
    await PlayVideo(props.account.UserID, props.account.DriveID, file.file_id)
    emit('toast', '已开始播放', 'success')
  } catch (e) {
    // fallback: try browser playback via preview proxy
    emit('toast', String(e), 'error')
  }
}

function playOrDownload(file) {
  const cat = file.category
  if (cat === 'video' || cat === 'audio' || cat === 'image' || cat === 'text' || extOf(file.name) === 'pdf') {
    play(file)
  } else {
    download(props.account.UserID, props.account.DriveID, file).then(() => emit('toast', '已加入下载', 'success'))
  }
}

function expand(id, name) {
  expanded.value[id] = !expanded.value[id]
  if (expanded.value[id] && !tree.value[id]) {
    listDir(props.account.UserID, props.account.DriveID, id).then((list) => { tree.value[id] = list })
  }
}

watch(() => props.account, (a) => {
  if (a) { dir.value = 'root'; pathStack.value = []; files.value = []; load('root') }
})

watch(() => props.providerMeta, () => { if (props.account) load(dir.value) })

onMounted(() => {
  onEvent('transfer:event', () => {})
})

defineExpose({ act })
</script>

<template>
  <div class="panel">
    <div class="panel-row">
      <button class="btn sm" @click="backToList" :disabled="mode !== 'search' && mode !== 'trash'">返回列表</button>
      <button v-if="c.TrashView" class="btn sm" @click="showTrash">回收站</button>
      <input class="input" style="flex:1;max-width:320px" v-model="keyword" placeholder="全盘搜索" @keyup.enter="goSearch" />
      <button class="btn sm" @click="goSearch">搜索</button>
      <div style="flex:1"></div>
      <button v-if="c.CreateFolder" class="btn sm" @click="act('mkdir')">新建文件夹</button>
      <button v-if="c.Upload" class="btn sm" @click="act('upload')">上传</button>
      <span class="fmeta">{{ files.length }} 项</span>
    </div>
    <div v-if="mode === 'trash'" class="panel-row" style="color:var(--text-secondary);font-size:12px">回收站视图</div>
    <div class="file-list" @click.self="closeMenu">
      <div class="file-head">
        <span></span><span>名称</span><span>大小</span><span>修改时间</span><span></span>
      </div>
      <div class="file-row dir-up" v-if="mode === 'list' && pathStack.length" @click="up">
        <span class="fi">⬆️</span><span class="fname ftext">返回上级</span><span></span><span></span><span></span>
      </div>
      <div
        v-for="f in files"
        :key="f.file_id"
        class="file-row"
        :class="{ selected: selected.some((s) => s.file_id === f.file_id) }"
        @click="toggleSel(f)"
        @dblclick="open(f)"
        @contextmenu.prevent="onCtx($event, f)"
      >
        <span class="fi">{{ iconOf(f) }}</span>
        <span class="fname"><span class="ftext">{{ f.name }}</span></span>
        <span class="fmeta">{{ f.isDir ? '-' : formatBytes(f.size) }}</span>
        <span class="fmeta">{{ formatTime(f.time) }}</span>
        <span class="fmeta" v-if="f.starred">★</span><span v-else></span>
      </div>
      <div v-if="!loading && !files.length" class="empty">空目录</div>
    </div>

    <div v-if="menu" class="ctx-menu" :style="{ left: menu.x + 'px', top: menu.y + 'px' }" @mouseleave="closeMenu">
      <div class="ctx-item" @click="act('download')">⬇️ 下载</div>
      <div class="ctx-item" @click="act('play')">▶️ 播放/预览</div>
      <div class="ctx-item" @click="act('rename')" v-if="c.Rename">✏️ 重命名</div>
      <div class="ctx-item" @click="act('share')" v-if="c.CreateShare">🔗 分享</div>
      <div class="ctx-item" @click="act('favorite')" v-if="c.Favorite">⭐ 收藏</div>
      <div class="ctx-sep"></div>
      <div class="ctx-item" @click="act('copy')" v-if="c.Copy">📋 复制到…</div>
      <div class="ctx-item" @click="act('move')" v-if="c.Move">📂 移动到…</div>
      <div class="ctx-item" @click="act('migrate')">🔄 跨盘迁移…</div>
      <div class="ctx-sep"></div>
      <div class="ctx-item" @click="act('trash')" v-if="c.RecycleBin">🗑️ 移入回收站</div>
      <div class="ctx-item" @click="act('restore')" v-if="mode === 'trash' && c.TrashRestore">♻️ 恢复</div>
      <div class="ctx-item danger" @click="act('delete')" v-if="c.PermanentDelete">❌ 永久删除</div>
    </div>
  </div>
</template>