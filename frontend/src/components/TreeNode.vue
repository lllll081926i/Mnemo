<script setup>
// 递归目录树节点：单击=展开/收起，双击=跳转到该目录。懒加载子目录，选中高亮。
// node: { file_id, name }；loadChildren(id) 由父级提供，返回目录数组（缓存于 tree map）。
import { computed, onBeforeUnmount } from 'vue'
import UiIcon from './UiIcon.vue'

const props = defineProps({
  node: { type: Object, required: true }, // { file_id, name }
  tree: { type: Object, required: true }, // id -> 子目录数组
  expanded: { type: Object, required: true },
  selectedId: { type: String, default: '' },
  depth: { type: Number, default: 1 },
})
const emit = defineEmits(['toggle', 'select', 'enter', 'leave', 'ctx'])

const children = computed(() => props.tree[props.node.file_id] || null)
const isOpen = computed(() => !!props.expanded[props.node.file_id])

// 单击与双击分离：单击延迟 240ms 执行展开/收起；双击取消单击并跳转
let clickTimer = null
function onClick() {
  if (clickTimer) { clearTimeout(clickTimer); clickTimer = null }
  clickTimer = setTimeout(() => { clickTimer = null; emit('toggle', props.node) }, 240)
}
function onDblClick() {
  if (clickTimer) { clearTimeout(clickTimer); clickTimer = null }
  emit('select', props.node)
}
onBeforeUnmount(() => { if (clickTimer) clearTimeout(clickTimer) })
</script>

<template>
  <div
    class="tree-node"
    :class="{ active: selectedId === node.file_id }"
    @click="onClick"
    @dblclick="onDblClick"
    @contextmenu.prevent="emit('ctx', $event, node)"
    @mouseenter="emit('enter', $event, node)"
    @mouseleave="emit('leave')"
  >
    <span class="tn-arrow" :class="{ open: isOpen }" @click.stop="emit('toggle', node)">
      <UiIcon name="chevron-right" :size="12" />
    </span>
    <UiIcon name="folder" :size="14" /><span class="tn-label">{{ node.name }}</span>
  </div>
  <div v-if="isOpen" class="tree-children">
    <div v-if="children && !children.length" class="tree-empty-tip">空文件夹</div>
    <TreeNode
      v-for="d in children || []"
      :key="d.file_id"
      :node="d"
      :tree="tree"
      :expanded="expanded"
      :selected-id="selectedId"
      :depth="depth + 1"
      @toggle="(n) => emit('toggle', n)"
      @select="(n) => emit('select', n)"
      @enter="(e, n) => emit('enter', e, n)"
      @leave="emit('leave')"
      @ctx="(e, n) => emit('ctx', e, n)"
    />
  </div>
</template>

<script>
export default { name: 'TreeNode' }
</script>

<style scoped>
.tree-empty-tip { padding: 4px 8px 4px 22px; font-size: 12.5px; color: var(--text-disabled); }
</style>
