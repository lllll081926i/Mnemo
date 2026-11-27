<script setup>
// 账号快切栏（复刻旧版 AccountRail）：默认 60px 窄图标栏，悬停展开为 220px 显示名称与用量。
import { computed, nextTick, ref, onBeforeUnmount, watch } from 'vue'
import { providerOf, accountName, providerIconUrl, providerMetaOf } from '../api'
import { useOrderedAccounts, setPref } from '../appearance'
import ContextMenu from './ContextMenu.vue'
import UiIcon from './UiIcon.vue'
import { accountHealth, healthLabels } from '../workspace'

const props = defineProps({
  accounts: { type: Array, default: () => [] },
  providers: { type: Array, default: () => [] },
  current: { type: Object, default: null },
})
const emit = defineEmits(['select', 'add', 'remove', 'info', 'rename', 'reauthorize', 'toast'])

const expanded = ref(false)
const railEl = ref(null)
const listEl = ref(null)
const menu = ref(null)
const activeIndicator = ref({ visible: false, x: 0, y: 0, width: 0, height: 0 })
const indicatorSliding = ref(false)
let hovering = false
let keyboardFocus = false
let cancelDrag = null
let clickTimer = null
let enterTimer = null
let leaveTimer = null
let railMoving = false
let layoutObserver = null
let resizeFrame = 0
let scrollAnchor = null
onBeforeUnmount(() => {
  cancelDrag?.()
  clearTimeout(enterTimer)
  clearTimeout(leaveTimer)
  clearTimeout(clickTimer)
  if (resizeFrame) cancelAnimationFrame(resizeFrame)
  resizeFrame = 0
  layoutObserver?.disconnect()
  window.removeEventListener('resize', onWindowResize)
})

// ---------- 手动拖拽排序（顺序存 localStorage prefs.accountOrder） ----------
const dragIdx = ref(-1)
const liveList = ref(null) // 拖拽中的实时顺序
const bumpMap = ref({})    // 被挤动项的碰撞果冻：user_id -> { dir, delay }
let suppressClick = false

const savedAccounts = useOrderedAccounts(() => props.accounts)
const orderedAccounts = computed(() => liveList.value || savedAccounts.value)
const activeSwitch = ref({ userId: '', direction: '' })
const displayAccounts = computed(() => new Map(props.accounts.map(acc => [acc.user_id, {
  name: accountName(acc),
  icon: providerIconUrl(providerMetaOf(acc, props.providers)),
}])))

const activeIndicatorStyle = computed(() => ({
  width: activeIndicator.value.width + 'px',
  height: activeIndicator.value.height + 'px',
  transform: `translate3d(${activeIndicator.value.x}px, ${activeIndicator.value.y}px, 0)`,
}))

function hideActiveIndicator() {
  activeIndicator.value = { ...activeIndicator.value, visible: false }
  indicatorSliding.value = false
}

// 仅在状态稳定后读取一次实际项目位置。尺寸无效时完全不渲染，杜绝残留的小方块。
function syncActiveIndicator(animate = false) {
  const listRoot = listEl.value?.$el || listEl.value
  if (!expanded.value || !railEl.value || !listRoot || !props.current?.user_id || railMoving || dragActive) {
    if (activeIndicator.value.visible) hideActiveIndicator()
    return
  }
  const item = listRoot.querySelector('.rail-item[data-selected="true"]:not(.rail-leave-active)')
  if (!item) { hideActiveIndicator(); return }
  if (item.offsetWidth < 1 || item.offsetHeight < 1) { hideActiveIndicator(); return }
  const next = {
    visible: true,
    x: item.offsetLeft,
    y: item.offsetTop,
    width: item.offsetWidth,
    height: item.offsetHeight,
  }
  const previous = activeIndicator.value
  if (previous.visible && ['x', 'y', 'width', 'height'].every(key => previous[key] === next[key])) return
  indicatorSliding.value = previous.visible && animate
  activeIndicator.value = next
}

function captureScrollAnchor() {
  const listRoot = listEl.value?.$el
  if (!listRoot?.scrollTop || dragActive) return
  const offset = listRoot.scrollTop
  const row = [...listRoot.querySelectorAll('.rail-item:not(.rail-leave-active)')]
    .find(item => item.offsetTop + item.offsetHeight > offset)
  if (row) scrollAnchor = { listRoot, row, inset: row.offsetTop - offset }
}

function clearScrollAnchor() { scrollAnchor = null }

function restoreScrollAnchor() {
  if (!scrollAnchor) return
  const { listRoot, row, inset } = scrollAnchor
  if (!listRoot.contains(row)) { clearScrollAnchor(); return }
  listRoot.scrollTop = row.offsetTop - inset
}

function onWindowResize() {
  if (railMoving) return
  if (typeof requestAnimationFrame !== 'function') {
    restoreScrollAnchor()
    syncActiveIndicator(false)
    return
  }
  if (resizeFrame) return
  resizeFrame = requestAnimationFrame(() => {
    resizeFrame = 0
    restoreScrollAnchor()
    syncActiveIndicator(false)
  })
}

function statusOfAccount(account) {
  return account.auth_state === 'reauth_required' ? 'auth' : accountHealth[account.user_id]?.status || 'ok'
}

watch([listEl, savedAccounts], () => {
  layoutObserver?.disconnect()
  const listRoot = listEl.value?.$el
  if (listRoot && typeof ResizeObserver !== 'undefined') {
    layoutObserver ||= new ResizeObserver(onWindowResize)
    layoutObserver.observe(listRoot)
    listRoot.querySelectorAll('.rail-item:not(.rail-leave-active)').forEach(item => layoutObserver.observe(item))
  }
  syncActiveIndicator(false)
}, { flush: 'post' })

// 账号切换只让新选中的图标沿切换方向轻微滑入。
// 不测量 DOM、不移动其他项目，展开/收起时也不会产生错位或掉帧。
watch(() => props.current?.user_id, (nextID, previousID) => {
  if (!nextID || !previousID || nextID === previousID) {
    activeSwitch.value = { userId: '', direction: '' }
    syncActiveIndicator(false)
    return
  }
  const nextIndex = orderedAccounts.value.findIndex(acc => acc.user_id === nextID)
  const previousIndex = orderedAccounts.value.findIndex(acc => acc.user_id === previousID)
  activeSwitch.value = {
    userId: nextID,
    direction: nextIndex > previousIndex ? 'down' : 'up',
  }
  syncActiveIndicator(true)
}, { flush: 'post' })

function onItemPointerDown(e, acc) {
  keyboardFocus = false
  if (e.button !== 0) return
  cancelDrag?.()
  clearTimeout(clickTimer)
  suppressClick = false
  menu.value = null
  const startX = e.clientX
  const startY = e.clientY
  const listEl = e.currentTarget.closest('.rail-list')
  const itemEl = e.currentTarget
  let dragging = false
  let ghost = null
  let rafId = 0
  let startRaf = 0
  let bumpRaf = 0
  let zoom = 1
  let listBounds = null
  let initialScrollTop = 0
  let lastFrameTime = null
  let dropAnimation = null
  let heights = null   // Map<user_id, height>，拖动起点捕获的各项静止高度
  let gapPx = 0
  let top0 = 0         // 首项静止 top
  let bottomLimit = 0  // 末项静止 bottom（拖动垂直范围上限）
  let ghostTop0 = 0    // 幽灵初始 top（X 始终锁定，只允许上下）
  let ghostY = 0       // 弹簧当前位置（top）
  let ghostHeight = 0
  let dropping = false
  let px = e.clientX
  let py = e.clientY

  // 持续 rAF 循环：幽灵以弹簧插值跟随指针（灵动 lag），并做槽位换位；
  // 指针停下后弹簧仍收敛到目标，不依赖 pointermove 频率
  const frame = (time) => {
    rafId = 0
    if (!ghost || dropping) return
    const edge = 40 * zoom
    const edgeDown = Math.min(1, Math.max(0, (py - listBounds.bottom + edge) / edge))
    const edgeUp = Math.min(1, Math.max(0, (listBounds.top + edge - py) / edge))
    const edgePressure = px >= listBounds.left && px <= listBounds.right ? edgeDown - edgeUp : 0
    let scrolling = false
    if (edgePressure && Number.isFinite(time)) {
      const elapsed = lastFrameTime === null ? 16 : Math.min(32, time - lastFrameTime)
      const previous = listEl.scrollTop
      const distance = Math.sign(edgePressure) * Math.max(1, Math.abs(edgePressure) * elapsed * 0.8)
      listEl.scrollTop += distance
      scrolling = listEl.scrollTop !== previous
    }
    lastFrameTime = Number.isFinite(time) ? time : null
    const scrollOffset = (listEl.scrollTop - initialScrollTop) * zoom
    const desired = Math.min(Math.max(ghostTop0 + (py - startY) + scrollOffset, top0), bottomLimit - ghostHeight)
    const visibleTarget = Math.min(Math.max(desired - scrollOffset, listBounds.top), Math.max(listBounds.top, listBounds.bottom - ghostHeight))
    ghostY += (visibleTarget - ghostY) * 0.32
    ghost.style.transform = `translate3d(0, ${(ghostY - ghostTop0) / zoom}px, 0) scale(1.04)`
    if (heights && ghostHeight > 0 && liveList.value) {
      const target = slotAt(desired)
      const cur = dragIdx.value
      if (target !== cur && cur >= 0) {
        const before = liveList.value
        const beforeIdx = new Map(before.map((a, i) => [a.user_id, i]))
        const list = [...before]
        const [moved] = list.splice(cur, 1)
        list.splice(target, 0, moved)
        liveList.value = list
        dragIdx.value = target
        // 碰撞果冻：被挤动的项朝移动方向“被顶一下”（压缩-回弹），
        // 延迟与 FLIP 波纹一致；清空再赋值以保证同向连续挤动时动画能重新播放
        const bumps = {}
        list.forEach((a, ni) => {
          if (a.user_id === moved.user_id) return
          const oi = beforeIdx.get(a.user_id)
          if (oi !== ni) {
            bumps[a.user_id] = { dir: ni > oi ? 'down' : 'up', delay: Math.min(Math.abs(ni - target) * 35, 140) }
          }
        })
        bumpMap.value = {}
        cancelAnimationFrame(bumpRaf)
        bumpRaf = requestAnimationFrame(() => { bumpMap.value = bumps })
      }
    }
    if (dragging && !dropping && (scrolling || Math.abs(visibleTarget - ghostY) > 0.1)) rafId = requestAnimationFrame(frame)
  }

  const slotAt = (y) => {
    let top = top0
    let slot = 0
    for (const account of liveList.value) {
      if (account.user_id === acc.user_id) continue
      const height = heights.get(account.user_id) || 0
      if (y < top + height / 2) break
      top += height + gapPx
      slot++
    }
    return slot
  }

  // 拖动起点：冻结一切展开/收起过渡并等一帧让布局静止，再捕获几何与克隆
  const startDrag = () => {
    startRaf = 0
    if (!dragging || ghost) return
    const items = [...listEl.querySelectorAll('.rail-item:not(.rail-leave-active)')]
    if (!items.length) return
    heights = new Map()
    items.forEach((el, i) => heights.set(liveList.value[i].user_id, el.getBoundingClientRect().height))
    zoom = listEl.currentCSSZoom || 1
    listBounds = listEl.getBoundingClientRect()
    initialScrollTop = listEl.scrollTop
    gapPx = (parseFloat(getComputedStyle(listEl).rowGap) || 0) * zoom
    top0 = items[0].getBoundingClientRect().top
    bottomLimit = items[items.length - 1].getBoundingClientRect().bottom
    const r = itemEl.getBoundingClientRect()
    bumpMap.value = {}
    ghostTop0 = r.top
    ghostY = r.top
    ghostHeight = r.height
    ghost = itemEl.cloneNode(true)
    ghost.tabIndex = -1
    ghost.setAttribute('aria-hidden', 'true')
    ghost.className = itemEl.className.replace('dragging', '').trim() + ' rail-ghost'
    ghost.style.width = r.width / zoom + 'px'
    ghost.style.height = r.height / zoom + 'px'
    ghost.style.left = r.left / zoom + 'px'
    ghost.style.top = r.top / zoom + 'px'
    listEl.closest('.account-rail').appendChild(ghost)
    if (!rafId) rafId = requestAnimationFrame(frame)
  }

  // 松手落位：幽灵以带回弹的过渡弹入目标槽位，落点即真实项位置，随后无缝替换
  const dropGhost = async () => {
    if (!ghost) return
    dropping = true
    let targetTop = top0 - (listEl.scrollTop - initialScrollTop) * zoom
    const list = liveList.value || []
    for (let i = 0; i < dragIdx.value && i < list.length; i++) {
      targetTop += (heights?.get(list[i].user_id) || 0) + gapPx
    }
    const clone = ghost
    clone.classList.add('dropping')
    const from = clone.style.transform
    const to = `translate3d(0, ${(targetTop - ghostTop0) / zoom}px, 0) scale(1)`
    clone.style.transform = to
    if (clone.animate && !window.matchMedia('(prefers-reduced-motion: reduce)').matches) {
      dropAnimation = clone.animate([{ transform: from }, { transform: to }], {
        duration: 260, easing: 'cubic-bezier(.25, 1.55, .4, 1)',
      })
      await dropAnimation.finished.catch(() => {})
    }
    if (ghost !== clone) return
    clone.remove()
    ghost = null
    listEl.closest('.account-rail')?.classList.remove('rail-frozen')
    liveList.value = null
    dragIdx.value = -1
    dragActive = false
    heights = null
    cancelDrag = null
    nextTick(() => syncActiveIndicator(false))
    if (hovering) onRailEnter()
    else onRailLeave()
  }

  const onMove = (ev) => {
    if (ev.pointerId !== undefined && e.pointerId !== undefined && ev.pointerId !== e.pointerId) return
    px = ev.clientX
    py = ev.clientY
    if (ghost && !dropping && !rafId) rafId = requestAnimationFrame(frame)
    if (!dragging) {
      if (Math.abs(py - startY) < 6 || Math.abs(py - startY) < Math.abs(px - startX)) return
      dragging = true
      dragActive = true
      hideActiveIndicator()
      suppressClick = true
      clearTimeout(enterTimer)
      clearTimeout(leaveTimer)
      liveList.value = [...orderedAccounts.value]
      dragIdx.value = liveList.value.findIndex((a) => a.user_id === acc.user_id)
      // 冻结栏宽/项高过渡（进行中的展开动画立即到位），下一帧捕获静止几何
      listEl.closest('.account-rail').classList.add('rail-frozen')
      document.body.classList.add('rail-drag-active')
      startRaf = requestAnimationFrame(startDrag)
    }
  }
  const onUp = (ev) => {
    if (ev?.pointerId !== undefined && e.pointerId !== undefined && ev.pointerId !== e.pointerId) return
    let commit = ev?.type === 'pointerup'
    window.removeEventListener('pointermove', onMove)
    window.removeEventListener('pointerup', onUp)
    window.removeEventListener('pointercancel', onUp)
    window.removeEventListener('blur', onUp)
    window.removeEventListener('keydown', onDragKey)
    cancelAnimationFrame(startRaf)
    if (commit && dragging && liveList.value) {
      if (Number.isFinite(ev.clientY)) py = ev.clientY
      if (!ghost) startDrag()
      cancelAnimationFrame(rafId)
      rafId = 0
      frame()
      try {
        setPref('accountOrder', liveList.value.map((a) => a.user_id))
      } catch {
        commit = false
        emit('toast', '网盘排序保存失败，已恢复原顺序，请重试')
      }
      clickTimer = setTimeout(() => { suppressClick = false }, 0)
    }
    cancelAnimationFrame(bumpRaf)
    if (rafId) { cancelAnimationFrame(rafId); rafId = 0 }
    if (!commit) {
      dropAnimation?.cancel()
      ghost?.remove()
      ghost = null
      suppressClick = false
    }
    if (ghost) dropGhost()
    else listEl.closest('.account-rail')?.classList.remove('rail-frozen')
    document.body.classList.remove('rail-drag-active')
    if (!ghost) {
      dragActive = false
      liveList.value = null
      dragIdx.value = -1
      heights = null
      cancelDrag = null
    }
    dragging = false
    if (!ghost) nextTick(() => syncActiveIndicator(false))
    bumpMap.value = {}
    if (!ghost && !hovering) scheduleCollapse()
  }
  const onDragKey = (ev) => {
    if (ev.key !== 'Escape') return
    ev.preventDefault()
    onUp()
  }
  cancelDrag = () => onUp()
  window.addEventListener('pointermove', onMove)
  window.addEventListener('pointerup', onUp)
  window.addEventListener('pointercancel', onUp)
  window.addEventListener('blur', onUp)
  window.addEventListener('keydown', onDragKey)
}

function onItemClick(acc) {
  if (suppressClick) { suppressClick = false; return }
  menu.value = null
  emit('select', acc)
}

let dragActive = false

watch(expanded, (value) => {
  clearScrollAnchor()
  captureScrollAnchor()
  railMoving = value
  hideActiveIndicator()
}, { flush: 'sync' })
watch(expanded, async (value, _, onCleanup) => {
  if (!value && !scrollAnchor) return
  let cancelled = false
  onCleanup(() => { cancelled = true })
  const listRoot = listEl.value?.$el
  const row = scrollAnchor?.row || listRoot?.querySelector('.rail-item:not(.rail-leave-active)')
  const animations = [
    ...(railEl.value?.getAnimations?.() || []),
    ...(listRoot?.getAnimations?.() || []),
    ...(row?.getAnimations?.({ subtree: true }) || []),
  ]
  const layoutTransitions = animations.filter(animation => /^(flex-basis|width|height|min-width|min-height|padding-.+|column-gap|row-gap)$/.test(animation.transitionProperty))
  await Promise.allSettled(layoutTransitions.map(animation => animation.finished))
  if (cancelled) return
  restoreScrollAnchor()
  clearScrollAnchor()
  railMoving = false
  syncActiveIndicator(false)
}, { flush: 'post' })
window.addEventListener('resize', onWindowResize)

// 悬停快速平滑展开，移出后延迟收起；拖拽期间冻结展开状态，避免中途布局突变
function onRailEnter() {
  hovering = true
  clearTimeout(leaveTimer)
  if (dragActive || expanded.value) return
  clearTimeout(enterTimer)
  if (railEl.value?.getAnimations?.().some(animation => animation.transitionProperty === 'flex-basis')) {
    expanded.value = true
    return
  }
  enterTimer = setTimeout(() => { expanded.value = true }, 160)
}
function onRailLeave() {
  hovering = false
  scheduleCollapse()
}
function scheduleCollapse() {
  if (dragActive || menu.value) return
  clearTimeout(enterTimer)
  clearTimeout(leaveTimer)
  leaveTimer = setTimeout(() => {
    if (!hovering && !menu.value && !(keyboardFocus && railEl.value?.contains(document.activeElement))) expanded.value = false
  }, 180)
}

watch(menu, (value) => {
  if (!value && !hovering) onRailLeave()
})
watch(() => props.accounts.map(a => a.user_id).sort().join('\n'), () => {
  cancelDrag?.()
  if (menu.value && !props.accounts.some(a => a.user_id === menu.value.acc.user_id)) menu.value = null
})

function onRailKey(e, acc) {
  keyboardFocus = true
  clearScrollAnchor()
  if (e.key === 'ContextMenu' || (e.shiftKey && e.key === 'F10')) {
    e.preventDefault()
    onCtx(e, acc)
    return
  }
  const buttons = [...railEl.value.querySelectorAll('.rail-item, .rail-add')]
  const index = buttons.indexOf(e.currentTarget)
  let next
  if (e.key === 'ArrowDown') next = (index + 1) % buttons.length
  else if (e.key === 'ArrowUp') next = (index - 1 + buttons.length) % buttons.length
  else if (e.key === 'Home') next = 0
  else if (e.key === 'End') next = buttons.length - 1
  else return
  e.preventDefault()
  buttons[next]?.focus()
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
  return displayAccounts.value.get(acc.user_id)?.icon
}

function labelOfAcc(acc) {
  const pid = providerOf(acc.user_id)
  const p = props.providers.find((x) => x.ID === pid)
  return p ? p.Meta.label : pid
}

function onCtx(e, acc) {
  if (!acc) return
  cancelDrag?.()
  clearTimeout(enterTimer)
  clearTimeout(leaveTimer)
  e.currentTarget.focus({ preventScroll: true })
  const rect = e.currentTarget.getBoundingClientRect()
  menu.value = { x: e.clientX || rect.right, y: e.clientY || rect.top, acc }
}

const menuItems = computed(() => {
  const acc = menu.value?.acc
  if (!acc) return []
  const currentAccount = props.accounts.find(account => account.user_id === acc.user_id) || acc
  return [
    ...(statusOfAccount(currentAccount) === 'auth' ? [{ icon: 'refresh', label: '重新登录', action: 'reauthorize' }] : []),
    { icon: 'info', label: '账号信息', action: 'info' },
    { icon: 'pencil', label: '自定义', action: 'rename' },
    { icon: 'trash', label: '移除账号', danger: true, action: 'remove' },
  ]
})

function onMenu(action) {
  if (action === 'remove') emit('remove', menu.value.acc)
  else if (action === 'info') emit('info', menu.value.acc)
  else if (action === 'rename') emit('rename', menu.value.acc)
  else if (action === 'reauthorize') emit('reauthorize', menu.value.acc)
}
</script>

<template>
  <aside
    ref="railEl"
    class="account-rail"
    :class="{ expanded }"
    @mouseenter="onRailEnter"
    @mouseleave="onRailLeave"
    @focusout="scheduleCollapse"
    @wheel.passive="clearScrollAnchor"
    @pointerdown.capture="clearScrollAnchor"
  >
    <TransitionGroup ref="listEl" name="rail" tag="div" class="rail-list" :class="{ reordering: dragIdx >= 0, 'has-indicator': activeIndicator.visible }">
      <span v-show="activeIndicator.visible" key="__active_indicator" class="rail-active-indicator" :class="{ ready: activeIndicator.visible, sliding: indicatorSliding }" :style="activeIndicatorStyle" aria-hidden="true"></span>
      <button
        v-for="(acc, i) in orderedAccounts"
        :key="acc.user_id"
        type="button"
        class="rail-item"
        :class="{ active: current && current.user_id === acc.user_id, ['active-enter-' + activeSwitch.direction]: activeSwitch.userId === acc.user_id, dragging: dragIdx === i, ['bump-' + (bumpMap[acc.user_id] || {}).dir]: bumpMap[acc.user_id] }"
        :data-selected="current?.user_id === acc.user_id ? 'true' : 'false'"
        :data-switch-direction="activeSwitch.userId === acc.user_id ? activeSwitch.direction : undefined"
        :style="dragIdx >= 0 && dragIdx !== i ? { transitionDelay: Math.min(Math.abs(i - dragIdx) * 35, 140) + 'ms' } : null"
        :title="`${labelOfAcc(acc)} · ${accountName(acc)}`"
        :aria-label="`${labelOfAcc(acc)} · ${accountName(acc)}`"
        :aria-description="statusOfAccount(acc) === 'auth' ? healthLabels.auth : undefined"
        :aria-current="current?.user_id === acc.user_id ? 'true' : undefined"
        aria-haspopup="menu"
        :aria-expanded="menu?.acc.user_id === acc.user_id"
        @pointerdown="onItemPointerDown($event, acc)"
        @click="onItemClick(acc)"
        @contextmenu.prevent="onCtx($event, acc)"
        @keydown="onRailKey($event, acc)"
      >
        <span class="rail-inner" :style="bumpMap[acc.user_id] ? { animationDelay: bumpMap[acc.user_id].delay + 'ms' } : null">
          <span class="rail-icon" :data-reauth="statusOfAccount(acc) === 'auth' || undefined">
            <img v-if="iconOfAcc(acc)" :src="iconOfAcc(acc)" alt="" />
            <span v-else class="rail-fallback">{{ (accountName(acc) || '?')[0].toUpperCase() }}</span>
          </span>
          <span class="rail-meta">
            <span class="rail-name">{{ accountName(acc) }}</span>
            <span v-if="statusOfAccount(acc) !== 'ok'" class="rail-sub rail-status" style="color:var(--color-warning)">{{ healthLabels[statusOfAccount(acc)] }}</span>
            <span class="rail-sub" v-if="hasQuota(acc)">{{ acc.usage.usedStr }} / {{ acc.usage.sizeStr }}</span>
            <span class="rail-sub" v-else-if="acc.usage?.type === 'unlimited'">总空间不限量</span>
            <span class="rail-sub" v-else>{{ labelOfAcc(acc) }}</span>
            <span v-if="hasQuota(acc)" class="rail-quota"><i :style="{ width: quotaPct(acc) + '%' }"></i></span>
          </span>
        </span>
      </button>
      <div v-if="!accounts.length" class="rail-empty" key="__empty">
        <span v-show="expanded">尚未登录网盘账号</span>
      </div>
    </TransitionGroup>
    <button type="button" class="rail-add" :title="'添加网盘账号'" @click="emit('add')" @keydown="onRailKey($event)">
      <UiIcon name="plus" :size="17" class="rail-add-icon" />
      <span class="rail-add-text">添加网盘</span>
    </button>

    <ContextMenu
      v-if="menu"
      :x="menu.x"
      :y="menu.y"
      :items="menuItems"
      @close="menu = null"
      @select="onMenu"
    />
  </aside>
</template>
