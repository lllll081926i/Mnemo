<script setup>
// 批量重命名弹窗（复刻旧版 RenameMultiModal + renamemulti 规则的精简实现）：
// 替换 / 添加前后缀 / 删除 / 序号，实时预览旧名 → 新名，通过 RenameBatch 提交。
import { ref, computed } from 'vue'
import { RenameBatch } from '../api'
import Modal from './Modal.vue'
import UiIcon from './UiIcon.vue'

const props = defineProps({
  account: { type: Object, required: true },
  files: { type: Array, required: true }, // 选中的 file 对象数组
})
const emit = defineEmits(['close', 'done', 'toast'])

const rules = ref({
  replace: { enable: true, search: '', newword: '', chkCase: true, chkAll: true, chkReg: false },
  add: { enable: false, before: '', after: '' },
  del: { enable: false, search: '' },
  index: { enable: false, format: '', minlen: 0, beginindex: 1 },
})
const busy = ref(false)
const previewScrollRow = ref(0)
const PREVIEW_ROW_HEIGHT = 32
const PREVIEW_VISIBLE_ROWS = 8
const PREVIEW_OVERSCAN = 4

function splitName(f) {
  const name = f.name || ''
  if (f.isDir) return { base: name, ext: '' }
  const i = name.lastIndexOf('.')
  return i > 0 ? { base: name.slice(0, i), ext: name.slice(i) } : { base: name, ext: '' }
}

const replacementPattern = computed(() => {
  const replace = rules.value.replace
  if (!replace.enable || !replace.search || !replace.chkReg) return null
  const flags = (replace.chkCase ? 'i' : '') + (replace.chkAll ? 'g' : '')
  try { return new RegExp(replace.search, flags) }
  catch { return false }
})

function applyRules(f, idx, pattern) {
  let { base, ext } = splitName(f)
  const r = rules.value
  if (r.replace.enable && r.replace.search) {
    if (pattern) base = base.replace(pattern, r.replace.newword)
    else if (!r.replace.chkReg && r.replace.chkAll) base = base.split(r.replace.search).join(r.replace.newword)
    else if (!r.replace.chkReg) base = base.replace(r.replace.search, r.replace.newword)
  }
  if (r.del.enable && r.del.search) base = base.split(r.del.search).join('')
  if (r.add.enable) base = (r.add.before || '') + base + (r.add.after || '')
  if (r.index.enable && r.index.format.includes('#')) {
    const num = String(r.index.beginindex + idx)
    const padded = r.index.minlen > 0 ? num.padStart(r.index.minlen, '0') : num
    const seq = r.index.format.replace(/#+/, padded)
    base = base + seq
  }
  return base + ext
}

const preview = computed(() => {
  let changed = 0
  const pattern = replacementPattern.value
  const names = props.files.map((file, index) => {
    const next = applyRules(file, index, pattern)
    if (next !== file.name) changed++
    return next
  })
  return { names, changed }
})
const previewStart = computed(() => Math.max(0, Math.min(previewScrollRow.value, props.files.length - PREVIEW_VISIBLE_ROWS) - PREVIEW_OVERSCAN))
const previewEnd = computed(() => Math.min(props.files.length, previewStart.value + PREVIEW_VISIBLE_ROWS + PREVIEW_OVERSCAN * 2))
const visiblePreview = computed(() => props.files.slice(previewStart.value, previewEnd.value).map((file, offset) => ({
  file,
  next: preview.value.names[previewStart.value + offset],
})))

async function submit() {
  if (!preview.value.changed || replacementPattern.value === false || busy.value) return
  busy.value = true
  try {
    const refs = []
    const names = []
    const planned = preview.value.names
    props.files.forEach((file, index) => {
      if (planned[index] === file.name) return
      refs.push({ id: file.file_id, isDir: file.isDir })
      names.push(planned[index])
    })
    const results = await RenameBatch(props.account.user_id, props.account.drive_id, refs, names)
    // RenameResult 失败路径 FileID 为空，成功时有值（后端约定）
    const list = results || []
    let ok = 0
    refs.forEach((_, index) => { if (list[index]?.file_id) ok++ })
    const failedCount = refs.length - ok
    if (failedCount) emit('toast', `${ok} 项成功，${failedCount} 项失败`, 'error')
    else emit('toast', `已重命名 ${ok} 项`, 'success')
    emit('done')
    emit('close')
  } catch (e) {
    emit('toast', String(e), 'error')
  }
  busy.value = false
}
</script>

<template>
  <Modal title="批量重命名" width="640px" @close="emit('close')">
    <div class="rr-rules">
      <div class="field">
        <label class="rr-check"><input type="checkbox" v-model="rules.replace.enable" /> 替换</label>
        <div class="panel-row">
          <input class="input" style="flex:1" v-model="rules.replace.search" placeholder="查找内容" :disabled="!rules.replace.enable" />
          <span style="color:var(--text-tertiary)">→</span>
          <input class="input" style="flex:1" v-model="rules.replace.newword" placeholder="替换为" :disabled="!rules.replace.enable" />
        </div>
        <div class="panel-row" style="margin-top:6px">
          <label class="rr-opt"><input type="checkbox" v-model="rules.replace.chkCase" :disabled="!rules.replace.enable" /> 忽略大小写</label>
          <label class="rr-opt"><input type="checkbox" v-model="rules.replace.chkAll" :disabled="!rules.replace.enable" /> 全部替换</label>
          <label class="rr-opt"><input type="checkbox" v-model="rules.replace.chkReg" :disabled="!rules.replace.enable" /> 正则</label>
        </div>
      </div>
      <div class="field">
        <label class="rr-check"><input type="checkbox" v-model="rules.del.enable" /> 删除文字</label>
        <input class="input" style="width:100%" v-model="rules.del.search" placeholder="要删除的文字" :disabled="!rules.del.enable" />
      </div>
      <div class="field">
        <label class="rr-check"><input type="checkbox" v-model="rules.add.enable" /> 添加前后缀</label>
        <div class="panel-row">
          <input class="input" style="flex:1" v-model="rules.add.before" placeholder="前缀" :disabled="!rules.add.enable" />
          <input class="input" style="flex:1" v-model="rules.add.after" placeholder="后缀（扩展名前）" :disabled="!rules.add.enable" />
        </div>
      </div>
      <div class="field">
        <label class="rr-check"><input type="checkbox" v-model="rules.index.enable" /> 添加序号</label>
        <div class="panel-row">
          <input class="input" style="flex:1" v-model="rules.index.format" placeholder="格式，如 _第#集" :disabled="!rules.index.enable" />
          <input class="input" style="width:80px" type="number" min="0" v-model.number="rules.index.beginindex" placeholder="起始" :disabled="!rules.index.enable" title="起始序号" />
          <input class="input" style="width:80px" type="number" min="0" v-model.number="rules.index.minlen" placeholder="位数" :disabled="!rules.index.enable" title="补齐位数" />
        </div>
      </div>
    </div>

    <p v-if="replacementPattern === false" class="panel-desc" role="alert">查找内容不是有效的正则表达式。</p>
    <div class="rr-preview" role="region" aria-label="重命名预览" tabindex="0" :style="{ '--rr-row-height': PREVIEW_ROW_HEIGHT + 'px' }" @scroll.passive="previewScrollRow = Math.floor($event.currentTarget.scrollTop / PREVIEW_ROW_HEIGHT)">
      <div class="rr-row rr-head"><span>原名称</span><span></span><span>新名称</span></div>
      <div v-if="previewStart" aria-hidden="true" :style="{ height: previewStart * PREVIEW_ROW_HEIGHT + 'px' }"></div>
      <div v-for="entry in visiblePreview" :key="entry.file.file_id" class="rr-row" :class="{ changed: entry.next !== entry.file.name }">
        <span class="rr-name" :title="entry.file.name">{{ entry.file.name }}</span>
        <UiIcon name="chevron-right" :size="12" />
        <span class="rr-name" :title="entry.next">{{ entry.next }}</span>
      </div>
      <div v-if="previewEnd < files.length" aria-hidden="true" :style="{ height: (files.length - previewEnd) * PREVIEW_ROW_HEIGHT + 'px' }"></div>
    </div>

    <template #actions>
      <span class="panel-desc" style="margin-right:auto">{{ preview.changed }} / {{ files.length }} 项将重命名</span>
      <button class="btn" @click="emit('close')">取消</button>
      <button class="btn primary" :disabled="!preview.changed || replacementPattern === false || busy" @click="submit">{{ busy ? '处理中…' : '应用重命名' }}</button>
    </template>
  </Modal>
</template>

<style scoped>
.rr-rules { display: grid; grid-template-columns: 1fr 1fr; gap: 0 20px; }
.rr-check { display: flex; align-items: center; gap: 6px; font-size: 13px; color: var(--text-secondary); margin-bottom: 6px; cursor: pointer; }
.rr-opt { display: inline-flex; align-items: center; gap: 4px; font-size: 13px; color: var(--text-tertiary); cursor: pointer; }
.rr-preview { max-height: 220px; overflow-y: auto; border: 1px solid var(--border-lighter); border-radius: var(--radius-md); margin-top: 4px; }
.rr-row { display: grid; grid-template-columns: minmax(0, 1fr) 20px minmax(0, 1fr); align-items: center; gap: 6px; height: var(--rr-row-height); padding: 0 12px; font-size: 13.5px; border-bottom: 1px solid var(--border-lighter); }
.rr-row:last-child { border-bottom: none; }
.rr-head { color: var(--text-tertiary); font-size: 12.5px; position: sticky; top: 0; background: var(--bg-elevated); }
.rr-row.changed .rr-name:last-child { color: var(--color-primary); font-weight: 500; }
.rr-name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
</style>
