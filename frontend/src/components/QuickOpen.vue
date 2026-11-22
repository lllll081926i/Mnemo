<script setup>
// 全局快捷命令面板 (Ctrl+P / Command+P)：
// 支持模块跳转、网盘账号快切、快捷命令（刷新/上传/新建文件夹/明暗切换）
import { ref, computed, watch } from 'vue'
import { accountName, providerIconUrl, providerMetaOf, providerOf } from '../api'
import UiIcon from './UiIcon.vue'
import Modal from './Modal.vue'
import { recentItems, recentEnabled, clearRecentItems, setRecentEnabled } from '../recent-items'

const props = defineProps({
  show: { type: Boolean, default: false },
  accounts: { type: Array, default: () => [] },
  providers: { type: Array, default: () => [] },
  currentAccount: { type: Object, default: null },
})
const emit = defineEmits(['close', 'select-tab', 'select-account', 'action', 'navigate'])

const keyword = ref('')
const activeIndex = ref(0)
const listEl = ref(null)
const firstVisible = ref(0)
const visibleCount = ref(8)
const rowHeight = 54
const overscan = 3
const shortcutLabel = document.documentElement.dataset.platform === 'darwin' ? '⌘P' : 'Ctrl+P'

const baseItems = computed(() => {
  const items = [
    // 页面跳转
    { id: 'tab-pan', group: '导航', title: '打开网盘', sub: '文件浏览与管理', icon: 'drive', run: () => emit('select-tab', 'pan') },
    { id: 'tab-transfer', group: '导航', title: '打开传输', sub: '下载与上传队列', icon: 'download', run: () => emit('select-tab', 'transfer') },
    { id: 'tab-sync', group: '导航', title: '打开同步', sub: '本地与网盘文件夹同步', icon: 'refresh', run: () => emit('select-tab', 'sync') },
    { id: 'tab-share', group: '导航', title: '打开分享', sub: '分享历史与外链管理', icon: 'share', run: () => emit('select-tab', 'share') },
    { id: 'tab-settings', group: '导航', title: '打开设置', sub: '通用、网络与播放器设置', icon: 'settings', run: () => emit('select-tab', 'settings') },

    // 常用命令
    { id: 'cmd-theme', group: '命令', title: '切换深色/浅色主题', sub: '即时生效', icon: 'sun', run: () => emit('action', 'toggle-theme') },
    { id: 'cmd-refresh', group: '命令', title: '刷新当前网盘目录', sub: '重新加载文件列表 (F5)', icon: 'refresh', run: () => emit('action', 'refresh') },
    { id: 'cmd-mkdir', group: '命令', title: '新建文件夹', sub: '在当前目录下创建 (Ctrl+Shift+N)', icon: 'plus', run: () => emit('action', 'mkdir') },
    { id: 'cmd-upload', group: '命令', title: '上传文件/文件夹', sub: '选择本地文件上传 (Ctrl+U)', icon: 'upload', run: () => emit('action', 'upload') },
  ]

  if (recentEnabled.value) {
    const accounts = new Map(props.accounts.map(account => [JSON.stringify([account.user_id, account.drive_id]), account]))
    for (const entry of recentItems.value.filter(item => accounts.has(JSON.stringify([item.userId, item.driveId]))).slice(0, 20)) {
      items.push({ id: JSON.stringify(['recent', entry.userId, entry.driveId, entry.fileId]), group: '最近使用', title: entry.name,
        sub: `${entry.isDir ? '文件夹' : '文件'} · ${accountName(accounts.get(JSON.stringify([entry.userId, entry.driveId]))) || entry.userId}`,
        icon: entry.isDir ? 'folder' : 'file', run: () => emit('navigate', entry) })
    }
  }

  // 网盘账号
  for (const acc of props.accounts) {
    const meta = providerMetaOf(acc, props.providers)
    const label = (meta && meta.label) || providerOf(acc.user_id) || '网盘'
    items.push({
      id: JSON.stringify(['account', acc.user_id, acc.drive_id]),
      group: '网盘账号',
      title: '切换到: ' + accountName(acc),
      sub: label + ' · ' + acc.user_id,
      icon: 'cloud',
      img: providerIconUrl(meta),
      run: () => {
        emit('select-account', acc)
        emit('select-tab', 'pan')
      },
    })
  }

  return items.map(item => ({ ...item, searchText: `${item.title}\n${item.sub}\n${item.group}`.toLowerCase() }))
})

const filteredItems = computed(() => {
  if (!props.show) return []
  const kw = keyword.value.trim().toLowerCase()
  if (!kw) return baseItems.value
  return baseItems.value.filter(item => item.searchText.includes(kw))
})
const start = computed(() => Math.max(0, firstVisible.value - overscan))
const end = computed(() => Math.min(filteredItems.value.length, firstVisible.value + visibleCount.value + overscan + 1))
const visibleItems = computed(() => filteredItems.value.slice(start.value, end.value))
const activeOptionId = computed(() => activeIndex.value >= start.value && activeIndex.value < end.value ? `qo-option-${activeIndex.value}` : undefined)

watch(filteredItems, () => {
  activeIndex.value = 0
  firstVisible.value = 0
  if (listEl.value) listEl.value.scrollTop = 0
})

watch(() => props.show, visible => { if (visible) keyword.value = '' })

function updateViewport() {
  const list = listEl.value
  if (!list) return
  firstVisible.value = Math.floor(Math.max(0, list.scrollTop - 8) / rowHeight)
  if (list.clientHeight) visibleCount.value = Math.ceil(list.clientHeight / rowHeight)
}

watch(listEl, (list, _, onCleanup) => {
  if (!list) return
  updateViewport()
  if (typeof ResizeObserver === 'undefined') return
  const observer = new ResizeObserver(updateViewport)
  observer.observe(list)
  onCleanup(() => observer.disconnect())
}, { flush: 'post' })

function execute(item) {
  if (!item) return
  emit('close')
  item.run()
}

function onKeyDown(event) {
  if (event.isComposing || event.keyCode === 229) { event.stopPropagation(); return }
  if (event.altKey || event.shiftKey) return
  const total = filteredItems.value.length
  const jump = (event.ctrlKey || event.metaKey) && ['Home', 'End'].includes(event.key)
  if (!jump && (event.ctrlKey || event.metaKey)) return
  if (event.key === 'Enter') {
    event.preventDefault()
    if (!event.repeat && activeOptionId.value) execute(filteredItems.value[activeIndex.value])
    return
  }
  if (!jump && !['ArrowDown', 'ArrowUp', 'PageDown', 'PageUp'].includes(event.key)) return
  if (!total) return
  event.preventDefault()
  const current = activeOptionId.value ? activeIndex.value : firstVisible.value
  const page = Math.max(1, visibleCount.value - 1)
  const next = jump ? (event.key === 'Home' ? 0 : total - 1)
    : event.key === 'ArrowDown' ? (current + 1) % total
      : event.key === 'ArrowUp' ? (current - 1 + total) % total
        : current + (event.key === 'PageDown' ? page : -page)
  reveal(Math.max(0, Math.min(total - 1, next)))
}

function reveal(index) {
  activeIndex.value = index
  const list = listEl.value
  if (!list) return
  const top = index * rowHeight + 8
  const height = list.clientHeight || visibleCount.value * rowHeight
  if (top < list.scrollTop) list.scrollTop = top
  else if (top + rowHeight > list.scrollTop + height) list.scrollTop = top + rowHeight - height
  updateViewport()
}
</script>

<template>
  <Modal
    v-if="show"
    title="快捷命令面板"
    width="600px"
    mask-class="qo-mask"
    dialog-class="qo-panel"
    body-class="qo-body"
    initial-focus=".qo-input"
    hide-head
    @close="emit('close')"
  >
    <div class="qo-input-wrap">
      <UiIcon name="search" :size="18" class="qo-search-icon" />
      <input
        id="qo-input"
        v-model="keyword"
        class="qo-input"
        role="combobox"
        aria-label="搜索功能、命令或账号"
        aria-autocomplete="list"
        aria-expanded="true"
        aria-controls="qo-list"
        :aria-activedescendant="activeOptionId"
        placeholder="搜索功能、命令或账号"
        spellcheck="false"
        @keydown="onKeyDown"
      />
      <span class="qo-badge">{{ shortcutLabel }}</span>
      <button type="button" class="qo-close" title="关闭 (Esc)" aria-label="关闭快捷命令面板" @click="emit('close')"><UiIcon name="close" :size="15" /></button>
    </div>

    <div id="qo-list" ref="listEl" class="qo-list" role="listbox" aria-label="快捷命令" @scroll.passive="updateViewport">
      <div v-if="!filteredItems.length" class="qo-empty">
        <UiIcon name="search" :size="20" />
        <span>没有匹配的命令或账号</span>
      </div>
      <div v-else class="qo-track" :style="{ height: `${filteredItems.length * rowHeight}px` }">
      <div
        v-for="(item, offset) in visibleItems"
        :id="`qo-option-${start + offset}`"
        :key="item.id"
        class="qo-item"
        :class="{ active: start + offset === activeIndex }"
        :style="{ top: `${(start + offset) * rowHeight}px`, height: `${rowHeight}px` }"
        role="option"
        :aria-selected="start + offset === activeIndex"
        :aria-posinset="start + offset + 1"
        :aria-setsize="filteredItems.length"
        @click="execute(item)"
        @pointermove="($event.movementX || $event.movementY) && (activeIndex = start + offset)"
      >
        <img v-if="item.img" :src="item.img" class="qo-item-img" alt="" />
        <UiIcon v-else :name="item.icon" :size="17" class="qo-item-icon" />
        <div class="qo-item-text">
          <span class="qo-item-title">{{ item.title }}</span>
          <span class="qo-item-sub">{{ item.sub }}</span>
        </div>
        <span class="qo-item-group">{{ item.group }}</span>
      </div>
      </div>
    </div>
    <div class="qo-recent-controls">
      <span class="qo-hint"><kbd>↑</kbd><kbd>↓</kbd> 导航 <kbd>Enter</kbd> 打开 <kbd>Esc</kbd> 关闭</span>
      <button type="button" @click="setRecentEnabled(!recentEnabled)">{{ recentEnabled ? '暂停记录最近使用' : '开启记录最近使用' }}</button>
      <button v-if="recentItems.length" type="button" @click="clearRecentItems">清空记录</button>
    </div>
  </Modal>
</template>

<style scoped>
:deep(.qo-mask) {
  align-items: flex-start;
  padding-top: 12vh;
}
:deep(.qo-panel) {
  max-width: 92vw;
  background: var(--bg-elevated);
  border-radius: var(--radius-xl);
  box-shadow: var(--shadow-modal), 0 0 0 1px var(--border-light);
}
:deep(.qo-body) { display: flex; flex-direction: column; min-height: 0; overflow: hidden; }
.qo-input-wrap {
  display: flex;
  align-items: center;
  gap: 10px;
  min-height: 58px;
  height: 58px; padding: 0 0 0 17px;
  border-bottom: 1px solid var(--border-light);
  background: var(--bg-surface);
}
.qo-search-icon { color: var(--color-primary); flex-shrink: 0; }
.qo-close { display: inline-flex; align-items: center; justify-content: center; align-self: stretch; flex: 0 0 44px; margin: 0; padding: 0; border: 0; border-radius: 0; color: var(--text-secondary); background: transparent; cursor: pointer; }
.qo-close:hover { color: #fff; background: #c43d4b; }
.qo-close:focus-visible { outline: 2px solid var(--color-primary); outline-offset: -3px; }
.qo-input {
  flex: 1;
  min-width: 0;
  border: none;
  background: transparent;
  font-size: 14.5px;
  color: var(--text-primary);
  outline: none;
  font-family: inherit;
}
.qo-input::placeholder { color: var(--control-placeholder); }
.qo-badge {
  font-size: 10.5px;
  font-weight: 650;
  padding: 3px 7px;
  border-radius: var(--radius-sm);
  background: var(--bg-subtle);
  color: var(--text-tertiary);
  border: 1px solid var(--border-lighter);
  flex-shrink: 0;
}
.qo-list {
  max-height: min(400px, 55vh);
  overflow-y: auto;
  padding: 8px;
  overflow-anchor: none;
}
.qo-track { position: relative; }
.qo-recent-controls { display: flex; align-items: center; justify-content: flex-end; flex-wrap: wrap; gap: 10px 14px; min-height: 42px; padding: 8px 14px; border-top: 1px solid var(--border-light); background: var(--bg-surface); }
.qo-hint { display: inline-flex; align-items: center; gap: 4px; margin-right: auto; color: var(--text-tertiary); font-size: 11px; }
.qo-hint kbd { min-width: 18px; padding: 1px 4px; border: 1px solid var(--border-light); border-radius: var(--radius-xs); background: var(--bg-subtle); color: var(--text-secondary); font: inherit; text-align: center; }
.qo-recent-controls button { border: 0; border-radius: var(--radius-xs); padding: 4px 6px; background: transparent; color: var(--text-secondary); cursor: pointer; font: inherit; font-size: 11px; }
.qo-recent-controls button:hover { color: var(--color-primary); background: var(--bg-hover); }
@media (max-width: 640px) { .qo-hint { display: none; } .qo-recent-controls { justify-content: center; } }
.qo-empty {
  display: grid;
  justify-items: center;
  gap: 9px;
  padding: 40px 32px;
  text-align: center;
  font-size: 13px;
  color: var(--text-tertiary);
}
.qo-item {
  position: absolute;
  inset-inline: 0;
  box-sizing: border-box;
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 9px 11px;
  border-radius: var(--radius-md);
  cursor: pointer;
  transition: background-color var(--motion-fast) var(--motion-ease), box-shadow var(--motion-fast) var(--motion-ease), transform var(--motion-fast) var(--motion-spring);
}
.qo-item:hover { background: var(--bg-hover); transform: translateX(2px); }
.qo-item.active {
  background: var(--listselectbg);
  box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--color-primary) 20%, transparent);
}
.qo-item-icon { color: var(--color-primary); flex-shrink: 0; }
.qo-item-img { width: 18px; height: 18px; object-fit: contain; flex-shrink: 0; }
.qo-item-text {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.qo-item-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.qo-item.active .qo-item-title { color: var(--color-primary); font-weight: 600; }
.qo-item-sub {
  font-size: 11.5px;
  color: var(--text-tertiary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.qo-item-group {
  font-size: 10.5px;
  font-weight: 600;
  color: var(--text-tertiary);
  padding: 1px 6px;
  border-radius: 999px;
  background: var(--bg-subtle);
  flex-shrink: 0;
}
@media (prefers-reduced-motion: reduce) {
  .qo-item { transition: none; }
  .qo-item:hover { transform: none; }
}
</style>
