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
  const itemEl = e.currentTarget
  let dragging = false
  let ghost = null
  let rafId = 0
  let heights = null   // Map<user_id, height>，拖动起点捕获的各项静止高度
  let gapPx = 0
  let top0 = 0         // 首项静止 top
  let px = e.clientX
  let py = e.clientY

  // 每帧最多一次：幽灵跟随 + 槽位换位（pointermove 频率高于渲染帧率）
  const frame = () => {
    rafId = 0
    if (ghost) {
      ghost.style.transform = `translate(${px - startX}px, ${py - startY}px) scale(1.06) rotate(-2deg)`
    }
    if (!heights || !liveList.value) return
    const target = slotAt(py)
    const cur = dragIdx.value
    if (target !== cur && cur >= 0) {
      const list = [...liveList.value]
      const [moved] = list.splice(cur, 1)
      list.splice(target, 0, moved)
      liveList.value = list
      dragIdx.value = target
    }
  }

  // 槽位 = 指针落入哪一项的区间；边界由捕获的各项高度按当前顺序累加，
  // 支持各项高度不一致（展开态有无用量条），且与动画中的 DOM 无关
  const slotAt = (y) => {
    const list = liveList.value
    let top = top0
    for (let i = 0; i < list.length; i++) {
      const h = heights.get(list[i].user_id) || 0
      if (y < top + h + gapPx) return i
      top += h + gapPx
    }
    return list.length - 1
  }

  // 拖动起点：冻结一切展开/收起过渡并等一帧让布局静止，再捕获几何与克隆
  const startDrag = () => {
    if (!dragging) return
    const items = [...listEl.querySelectorAll('.rail-item')]
    if (!items.length) return
    heights = new Map()
    items.forEach((el, i) => heights.set(liveList.value[i].user_id, el.getBoundingClientRect().height))
    gapPx = parseFloat(getComputedStyle(listEl).rowGap) || 0
    top0 = items[0].getBoundingClientRect().top
    const r = itemEl.getBoundingClientRect()
    ghost = itemEl.cloneNode(true)
    ghost.className = itemEl.className.replace('dragging', '').trim() + ' rail-ghost'
    ghost.style.width = r.width + 'px'
    ghost.style.left = r.left + 'px'
    ghost.style.top = r.top + 'px'
    listEl.closest('.account-rail').appendChild(ghost)
  }

  const onMove = (ev) => {
    px = ev.clientX
    py = ev.clientY
    if (!dragging) {
      if (Math.abs(py - startY) < 6 && Math.abs(px - startX) < 6) return
      dragging = true
      dragActive = true
      suppressClick = true
      clearTimeout(enterTimer)
      clearTimeout(leaveTimer)
      liveList.value = [...orderedAccounts.value]
      dragIdx.value = liveList.value.findIndex((a) => a.user_id === acc.user_id)
      // 冻结栏宽/项高过渡（进行中的展开动画立即到位），下一帧捕获静止几何
      listEl.closest('.account-rail').classList.add('rail-frozen')
      document.body.classList.add('rail-drag-active')
      requestAnimationFrame(startDrag)
    }
    if (!rafId) rafId = requestAnimationFrame(frame)
  }
  const onUp = () => {
    window.removeEventListener('pointermove', onMove)
    window.removeEventListener('pointerup', onUp)
    window.removeEventListener('pointercancel', onUp)
    if (rafId) cancelAnimationFrame(rafId)
    if (dragging && liveList.value) {
      setPref('accountOrder', liveList.value.map((a) => a.user_id))
      // click 紧跟 pointerup 触发，延后一帧清除以吞掉这次拖拽点击
      setTimeout(() => { suppressClick = false }, 0)
    }
    if (ghost) { ghost.remove(); ghost = null }
    listEl.closest('.account-rail')?.classList.remove('rail-frozen')
    document.body.classList.remove('rail-drag-active')
    dragActive = false
    liveList.value = null
    dragIdx.value = -1
    dragging = false
    heights = null
  }
  window.addEventListener('pointermove', onMove)
  window.addEventListener('pointerup', onUp)
  window.addEventListener('pointercancel', onUp)
}

function onItemClick(acc) {
  if (suppressClick) { suppressClick = false; return }
  emit('select', acc)
}

let dragActive = false

// 悬停快速平滑展开，移出后延迟收起；拖拽期间冻结展开状态，避免中途布局突变
function onRailEnter() {
  if (dragActive) return
  clearTimeout(leaveTimer)
  clearTimeout(enterTimer)
  enterTimer = setTimeout(() => { expanded.value = true }, 220)
}
function onRailLeave() {
  if (dragActive) return
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
