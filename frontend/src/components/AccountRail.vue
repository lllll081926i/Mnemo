<script setup>
// 账号快切栏（复刻旧版 AccountRail）：默认 60px 窄图标栏，悬停展开为 220px 显示名称与用量。
import { computed, ref, onBeforeUnmount } from 'vue'
import { providerOf, accountName, providerIconUrl } from '../api'
import ContextMenu from './ContextMenu.vue'
import UiIcon from './UiIcon.vue'

const props = defineProps({
  accounts: { type: Array, default: () => [] },
  providers: { type: Array, default: () => [] },
  current: { type: Object, default: null },
})
const emit = defineEmits(['select', 'add', 'remove', 'info'])

const expanded = ref(false)
const menu = ref(null)
let enterTimer = null
let leaveTimer = null
onBeforeUnmount(() => { clearTimeout(enterTimer); clearTimeout(leaveTimer) })

// 悬停快速平滑展开，移出后延迟收起
function onRailEnter() {
  clearTimeout(leaveTimer)
  clearTimeout(enterTimer)
  enterTimer = setTimeout(() => { expanded.value = true }, 220)
}
function onRailLeave() {
  clearTimeout(enterTimer)
  leaveTimer = setTimeout(() => { expanded.value = false }, 200)
}

const groups = computed(() => {
  const map = new Map()
  for (const acc of props.accounts) {
    const pid = providerOf(acc.user_id)
    if (!map.has(pid)) map.set(pid, [])
    map.get(pid).push(acc)
  }
  const order = props.providers.map((p) => p.ID)
  return [...map.entries()]
    .sort((a, b) => {
      const ia = order.indexOf(a[0]), ib = order.indexOf(b[0])
      return (ia < 0 ? 99 : ia) - (ib < 0 ? 99 : ib)
    })
    .map(([pid, list]) => {
      const p = props.providers.find((x) => x.ID === pid)
      return { pid, label: p ? p.Meta.label : pid, list }
    })
})

function quotaPct(acc) {
  const u = acc.usage
  if (!u || !u.size) return 0
  return Math.min(100, Math.round((u.used / u.size) * 100))
}
function hasQuota(acc) { return acc.usage && acc.usage.size > 0 }

function iconOfAcc(acc) {
  const pid = providerOf(acc.user_id)
  const p = props.providers.find((x) => x.ID === pid)
  return p ? providerIconUrl(p.Meta) : ''
}

function labelOfAcc(acc) {
  const pid = providerOf(acc.user_id)
  const p = props.providers.find((x) => x.ID === pid)
  return p ? p.Meta.label : pid
}

function onCtx(e, acc) {
  menu.value = { x: e.clientX, y: e.clientY, acc }
}

function onMenu(action) {
  if (action === 'remove') emit('remove', menu.value.acc)
  else if (action === 'info') emit('info', menu.value.acc)
}
</script>

<template>
  <aside
    class="account-rail"
    :class="{ expanded }"
    @mouseenter="onRailEnter"
    @mouseleave="onRailLeave"
  >
    <div class="rail-list">
      <button
        v-for="acc in accounts"
        :key="acc.user_id"
        type="button"
        class="rail-item"
        :class="{ active: current && current.user_id === acc.user_id }"
        :title="`${labelOfAcc(acc)} · ${accountName(acc)}`"
        @click="emit('select', acc)"
        @contextmenu.prevent="onCtx($event, acc)"
      >
        <span class="rail-icon">
          <img v-if="iconOfAcc(acc)" :src="iconOfAcc(acc)" alt="" />
          <span v-else class="rail-fallback">{{ (accountName(acc) || '?')[0].toUpperCase() }}</span>
        </span>
        <span class="rail-meta">
          <span class="rail-name">{{ accountName(acc) }}</span>
          <span class="rail-sub" v-if="hasQuota(acc)">{{ acc.usage.usedStr }} / {{ acc.usage.sizeStr }}</span>
          <span class="rail-sub" v-else>{{ labelOfAcc(acc) }}</span>
          <span v-if="hasQuota(acc)" class="rail-quota"><i :style="{ width: quotaPct(acc) + '%' }"></i></span>
        </span>
      </button>
      <div v-if="!accounts.length" class="rail-empty">
        <span v-show="expanded">尚未登录网盘账号</span>
      </div>
    </div>

    <button type="button" class="rail-add" :title="'添加网盘账号'" @click="emit('add')">
      <UiIcon name="plus" :size="17" class="rail-add-icon" />
      <span class="rail-add-text">添加网盘</span>
    </button>

    <ContextMenu
      v-if="menu"
      :x="menu.x"
      :y="menu.y"
      :items="[
        { icon: 'info', label: '账号信息', action: 'info' },
        { icon: 'trash', label: '移除账号', danger: true, action: 'remove' },
      ]"
      @close="menu = null"
      @select="onMenu"
    />
  </aside>
</template>
