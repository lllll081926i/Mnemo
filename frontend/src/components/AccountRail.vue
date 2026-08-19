<script setup>
// 账号快切栏（复刻旧版 AccountRail）：默认 60px 窄图标栏，悬停展开为 220px 显示名称与用量。
import { computed, ref, onBeforeUnmount } from 'vue'
import { providerOf, accountName, providerIconUrl } from '../api'
import { getPrefs, setPref } from '../appearance'
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

// ---------- 手动拖拽排序（顺序存 localStorage prefs.accountOrder） ----------
const dragIdx = ref(-1)
const liveList = ref(null) // 拖拽中的实时顺序
let suppressClick = false

const orderedAccounts = computed(() => {
  if (liveList.value) return liveList.value
  const order = Array.isArray(getPrefs().accountOrder) ? getPrefs().accountOrder : []
  if (!order.length) return props.accounts
  const known = order
    .map((id) => props.accounts.find((a) => a.user_id === id))
    .filter(Boolean)
  const unknown = props.accounts.filter((a) => !order.includes(a.user_id))
  return [...known, ...unknown]
})

function onItemPointerDown(e, acc) {
  if (e.button !== 0) return
  const startX = e.clientX
  const startY = e.clientY
  const listEl = e.currentTarget.closest('.rail-list')
  let dragging = false
  const onMove = (ev) => {
    if (!dragging) {
      if (Math.abs(ev.clientY - startY) < 6 && Math.abs(ev.clientX - startX) < 6) return
      dragging = true
      suppressClick = true
      liveList.value = [...orderedAccounts.value]
      dragIdx.value = liveList.value.findIndex((a) => a.user_id === acc.user_id)
    }
    const items = listEl ? [...listEl.querySelectorAll('.rail-item')] : []
    let target = items.length - 1
    for (let i = 0; i < items.length; i++) {
      const r = items[i].getBoundingClientRect()
      if (ev.clientY < r.top + r.height / 2) { target = i; break }
    }
    const cur = dragIdx.value
    if (target !== cur && cur >= 0 && liveList.value) {
      const list = [...liveList.value]
      const [moved] = list.splice(cur, 1)
      list.splice(target, 0, moved)
      liveList.value = list
      dragIdx.value = target
    }
  }
  const onUp = () => {
    window.removeEventListener('pointermove', onMove)
    window.removeEventListener('pointerup', onUp)
    if (dragging && liveList.value) {
      setPref('accountOrder', liveList.value.map((a) => a.user_id))
      // click 紧跟 pointerup 触发，延后一帧清除以吞掉这次拖拽点击
      setTimeout(() => { suppressClick = false }, 0)
    }
    liveList.value = null
    dragIdx.value = -1
    dragging = false
  }
  window.addEventListener('pointermove', onMove)
  window.addEventListener('pointerup', onUp)
}

function onItemClick(acc) {
  if (suppressClick) { suppressClick = false; return }
  emit('select', acc)
}

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
    <TransitionGroup name="rail" tag="div" class="rail-list" :class="{ reordering: dragIdx >= 0 }">
      <button
        v-for="(acc, i) in orderedAccounts"
        :key="acc.user_id"
        type="button"
        class="rail-item"
        :class="{ active: current && current.user_id === acc.user_id, dragging: dragIdx === i }"
        :title="`${labelOfAcc(acc)} · ${accountName(acc)}`"
        @pointerdown="onItemPointerDown($event, acc)"
        @click="onItemClick(acc)"
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
      <div v-if="!accounts.length" class="rail-empty" key="__empty">
        <span v-show="expanded">尚未登录网盘账号</span>
      </div>
    </TransitionGroup>

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
