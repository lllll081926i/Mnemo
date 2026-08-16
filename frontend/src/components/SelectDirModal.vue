<script setup>
// 选择网盘目录：在指定账号内浏览文件夹树并选中目标目录。
import { ref, onMounted } from 'vue'
import { listDir, providerMetaOf } from '../api'
import Modal from './Modal.vue'
import UiIcon from './UiIcon.vue'

const props = defineProps({
  title: { type: String, default: '选择目录' },
  account: { type: Object, required: true },
  providers: { type: Array, default: () => [] },
})
const emit = defineEmits(['close', 'select'])

const rootKey = providerMetaOf(props.account, props.providers).rootKey || 'root'
const crumbs = ref([{ id: rootKey, name: providerMetaOf(props.account, props.providers).rootTitle || '根目录' }])
const dirs = ref([])
const loading = ref(false)
const error = ref('')

let loadSeq = 0
async function load(id) {
  const seq = ++loadSeq
  loading.value = true
  error.value = ''
  try {
    const list = await listDir(props.account.user_id, props.account.drive_id, id)
    if (seq !== loadSeq) return // 过期响应丢弃
    dirs.value = (list || []).filter((f) => f.isDir)
  } catch (e) {
    if (seq !== loadSeq) return
    error.value = String(e)
    dirs.value = []
  }
  if (seq === loadSeq) loading.value = false
}

function enter(d) {
  crumbs.value.push({ id: d.file_id, name: d.name })
  load(d.file_id)
}

function goUp() {
  if (crumbs.value.length > 1) {
    crumbs.value.pop()
    load(crumbs.value[crumbs.value.length - 1].id)
  }
}

function jump(i) {
  crumbs.value = crumbs.value.slice(0, i + 1)
  load(crumbs.value[i].id)
}

onMounted(() => load(rootKey))
</script>

<template>
  <Modal :title="title" width="480px" @close="emit('close')">
    <div class="pathbar" style="border:none;padding:0 0 8px">
      <template v-for="(c, i) in crumbs" :key="c.id + i">
        <span v-if="i" class="crumb-sep">/</span>
        <span class="crumb" @click="i < crumbs.length - 1 && jump(i)">{{ c.name }}</span>
      </template>
    </div>
    <div style="min-height:180px;max-height:300px;overflow-y:auto;border:1px solid var(--border-lighter);border-radius:var(--radius-md);padding:6px">
      <div v-if="loading && !dirs.length" class="skeleton-list">
        <div v-for="i in 4" :key="i" class="skeleton-row" style="height:34px">
          <div class="skeleton skeleton-icon" style="width:16px;height:16px"></div>
          <div class="skeleton skeleton-name" :style="{ width: (40 + (i * 13) % 40) + '%' }"></div>
        </div>
      </div>
      <div v-else-if="error" class="empty" style="padding:40px">{{ error }}</div>
      <div v-else-if="!dirs.length" class="empty" style="padding:40px">此目录下没有子文件夹</div>
      <div v-if="!loading && crumbs.length > 1" class="tree-node" style="color:var(--text-tertiary)" @click="goUp">
        <UiIcon name="up-level" :size="15" /><span class="tn-label">返回上级</span>
      </div>
      <div v-for="d in dirs" :key="d.file_id" class="tree-node" @click="enter(d)">
        <UiIcon name="folder" :size="15" /><span class="tn-label">{{ d.name }}</span>
        <span style="margin-left:auto;color:var(--text-tertiary)"><UiIcon name="chevron-right" :size="13" /></span>
      </div>
    </div>
    <template #actions>
      <button class="btn" @click="emit('close')">取消</button>
      <button class="btn primary" @click="emit('select', { id: crumbs[crumbs.length - 1].id, name: crumbs[crumbs.length - 1].name })">选择当前目录</button>
    </template>
  </Modal>
</template>
