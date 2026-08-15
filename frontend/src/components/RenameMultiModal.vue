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

function splitName(f) {
  const name = f.name || ''
  if (f.isDir) return { base: name, ext: '' }
  const i = name.lastIndexOf('.')
  return i > 0 ? { base: name.slice(0, i), ext: name.slice(i) } : { base: name, ext: '' }
}

function applyRules(f, idx) {
  let { base, ext } = splitName(f)
  const r = rules.value
  if (r.replace.enable && r.replace.search) {
    try {
      if (r.replace.chkReg) {
        const flags = (r.replace.chkCase ? 'i' : '') + (r.replace.chkAll ? 'g' : '')
        base = base.replace(new RegExp(r.replace.search, flags), r.replace.newword)
      } else if (r.replace.chkAll) {
        base = base.split(r.replace.search).join(r.replace.newword)
      } else {
        base = base.replace(r.replace.search, r.replace.newword)
      }
    } catch { /* 非法正则跳过 */ }
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

const preview = computed(() =>
  props.files.map((f, i) => ({ old: f.name, next: applyRules(f, i), changed: applyRules(f, i) !== f.name }))
)
const changedCount = computed(() => preview.value.filter((p) => p.changed).length)

async function submit() {
  if (!changedCount.value || busy.value) return
  busy.value = true
  try {
    const changed = preview.value.filter((p) => p.changed)
    const refs = props.files.filter((f, i) => preview.value[i].changed).map((f) => ({ id: f.file_id, isDir: f.isDir }))
    const names = changed.map((p) => p.next)
    const results = await RenameBatch(props.account.user_id, props.account.drive_id, refs, names)
    // RenameResult 无 error 字段：以「结果缺失或名称未变」判定失败
    const list = results || []
    let ok = 0
    changed.forEach((p, i) => { if (list[i] && list[i].name === p.next) ok++ })
    const failedCount = changed.length - ok
    if (failedCount) emit('toast', `${ok} 成功，${failedCount} 失败`, 'error')
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

    <div class="rr-preview">
      <div class="rr-row rr-head"><span>原名称</span><span></span><span>新名称</span></div>
      <div v-for="(p, i) in preview" :key="i" class="rr-row" :class="{ changed: p.changed }">
        <span class="rr-name" :title="p.old">{{ p.old }}</span>
        <UiIcon name="chevron-right" :size="12" />
        <span class="rr-name" :title="p.next">{{ p.next }}</span>
      </div>
    </div>

    <template #actions>
      <span class="panel-desc" style="margin-right:auto">{{ changedCount }} / {{ files.length }} 项将修改</span>
      <button class="btn" @click="emit('close')">取消</button>
      <button class="btn primary" :disabled="!changedCount || busy" @click="submit">{{ busy ? '提交中…' : '应用重命名' }}</button>
    </template>
  </Modal>
</template>

<style scoped>
.rr-rules { display: grid; grid-template-columns: 1fr 1fr; gap: 0 20px; }
.rr-check { display: flex; align-items: center; gap: 6px; font-size: 13px; color: var(--text-secondary); margin-bottom: 6px; cursor: pointer; }
.rr-opt { display: inline-flex; align-items: center; gap: 4px; font-size: 13px; color: var(--text-tertiary); cursor: pointer; }
.rr-preview { max-height: 220px; overflow-y: auto; border: 1px solid var(--border-lighter); border-radius: var(--radius-md); margin-top: 4px; }
.rr-row { display: grid; grid-template-columns: 1fr 20px 1fr; align-items: center; gap: 6px; padding: 6px 12px; font-size: 13.5px; border-bottom: 1px solid var(--border-lighter); }
.rr-row:last-child { border-bottom: none; }
.rr-head { color: var(--text-tertiary); font-size: 12.5px; position: sticky; top: 0; background: var(--bg-elevated); }
.rr-row.changed .rr-name:last-child { color: var(--color-primary); font-weight: 500; }
.rr-name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
</style>
