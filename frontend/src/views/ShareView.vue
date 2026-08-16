<script setup>
// 分享记录页：聚合全部账号的分享历史，按网盘 + 账号分组展示。
import { ref, computed, watch, onMounted, onBeforeUnmount } from 'vue'
import { ListShareHistory, OpenBrowser, onEvent, providerOf, accountName, providerIconUrl, formatTime, copyText } from '../api'
import UiIcon from '../components/UiIcon.vue'
import UiSelect from '../components/UiSelect.vue'

const props = defineProps({
  accounts: { type: Array, default: () => [] },
  providers: { type: Array, default: () => [] },
})
const emit = defineEmits(['toast'])

const history = ref([])
const loading = ref(false)
const kw = ref('')
const kwRaw = ref('')   // 输入原值，kw 为防抖后值
let kwTimer = null
watch(kwRaw, (v) => {
  clearTimeout(kwTimer)
  kwTimer = setTimeout(() => { kw.value = v }, 120)
})
onBeforeUnmount(() => clearTimeout(kwTimer))
const filterProvider = ref('')
const filterAccount = ref('')

let refreshSeq = 0
async function refresh() {
  const seq = ++refreshSeq
  loading.value = true
  try {
    const list = (await ListShareHistory('')) || []
    if (seq === refreshSeq) history.value = list
  } catch (e) {
    if (seq === refreshSeq) emit('toast', String(e), 'error')
  } finally {
    if (seq === refreshSeq) loading.value = false
  }
}

watch(() => props.accounts, (list) => {
  if (filterAccount.value && !list.some((a) => a.user_id === filterAccount.value)) {
    filterAccount.value = ''
  }
  if (filterProvider.value && !props.providers.some((p) => p.ID === filterProvider.value)) {
    filterProvider.value = ''
  }
}, { deep: true })

function itemKey(h, idx) {
  return `${h.account_id || ''}_${h.share_id || ''}_${h.share_url || ''}_${idx}`
}

function metaOf(pid) {
  const p = props.providers.find((x) => x.ID === pid)
  return (p && p.Meta) || {}
}

function labelOf(pid) {
  return metaOf(pid).label || pid || '未知网盘'
}

function accountOf(userId) {
  return props.accounts.find((a) => a.user_id === userId) || null
}

// 从记录中聚合出现过的 provider 列表
const providerOptions = computed(() => {
  const seen = new Set()
  for (const h of history.value) {
    const pid = h.provider || providerOf(h.account_id)
    if (pid) seen.add(pid)
  }
  return [...seen]
})

// 账号筛选只列出有分享记录的账号
const accountOptions = computed(() => {
  const seen = new Map()
  for (const h of history.value) {
    if (!h.account_id || seen.has(h.account_id)) continue
    const acc = accountOf(h.account_id)
    seen.set(h.account_id, acc ? accountName(acc) : h.account_id)
  }
  return [...seen.entries()].map(([id, name]) => ({ id, name }))
})

const filtered = computed(() => {
  const k = kw.value.trim().toLowerCase()
  return history.value.filter((h) => {
    const pid = h.provider || providerOf(h.account_id)
    if (filterProvider.value && pid !== filterProvider.value) return false
    if (filterAccount.value && h.account_id !== filterAccount.value) return false
    if (!k) return true
    return (
      String(h.share_name || '').toLowerCase().includes(k) ||
      String(h.share_url || '').toLowerCase().includes(k) ||
      String(h.share_pwd || '').toLowerCase().includes(k)
    )
  })
})

// 按 provider + account 分组
const groups = computed(() => {
  const map = new Map()
  for (const h of filtered.value) {
    const pid = h.provider || providerOf(h.account_id)
    const key = pid + '|' + h.account_id
    if (!map.has(key)) {
      const acc = accountOf(h.account_id)
      map.set(key, {
        key,
        pid,
        icon: providerIconUrl(metaOf(pid)),
        label: labelOf(pid),
        accName: acc ? accountName(acc) : h.account_id,
        items: [],
      })
    }
    map.get(key).items.push(h)
  }
  return [...map.values()]
})

function openLink(h) {
  if (!h.share_url) return
  OpenBrowser(h.share_url)
}

const copiedMap = ref({})

async function copy(text, tip, key) {
  const ok = await copyText(text)
  if (ok && key) {
    copiedMap.value[key] = true
    setTimeout(() => {
      const m = { ...copiedMap.value }
      delete m[key]
      copiedMap.value = m
    }, 1600)
  }
  emit('toast', ok ? tip : '复制失败', ok ? 'success' : 'error')
}

function copyAll(h, key) {
  const lines = [h.share_name || '分享文件', h.share_url || '']
  if (h.share_pwd) lines.push('提取码: ' + h.share_pwd)
  copy(lines.join('\n'), '已复制分享信息', key)
}

const offs = []
onMounted(() => {
  refresh()
  offs.push(onEvent('account:changed', refresh))
})
onBeforeUnmount(() => offs.forEach((off) => off && off()))
</script>

<template>
  <div class="share-page">
    <!-- 工具条 -->
    <div class="share-toolbar">
      <span class="search-quick-wrap">
        <span class="sq-icon"><UiIcon name="search" :size="13" /></span>
        <input class="search-quick share-search" style="width:240px" v-model="kwRaw" placeholder="搜索名称 / 链接 / 提取码" />
        <button v-if="kwRaw" class="sq-clear" title="清空搜索" @click="kwRaw = ''"><UiIcon name="close" :size="11" /></button>
      </span>
      <UiSelect
        v-model="filterProvider"
        style="width:150px"
        :options="[{ value: '', label: '全部网盘' }, ...providerOptions.map((pid) => ({ value: pid, label: labelOf(pid) }))]"
      />
      <UiSelect
        v-model="filterAccount"
        style="width:150px"
        :options="[{ value: '', label: '全部账号' }, ...accountOptions.map((a) => ({ value: a.id, label: a.name }))]"
      />
      <div style="flex:1"></div>
      <button class="tbtn" :disabled="loading" @click="refresh">
        <span v-if="loading" class="spin"></span><template v-else><UiIcon name="refresh" :size="14" />刷新</template>
      </button>
    </div>

    <!-- 骨架屏加载状态 -->
    <div v-if="loading && !history.length" class="skeleton-list" style="max-width:960px">
      <div v-for="i in 4" :key="i" class="skeleton-row" style="height:64px;border:1px solid var(--border-lighter);border-radius:var(--radius-md);margin-bottom:8px">
        <div class="skeleton skeleton-icon" style="width:32px;height:32px"></div>
        <div style="flex:1;display:flex;flex-direction:column;gap:6px">
          <div class="skeleton" :style="{ width: (30 + (i * 11) % 40) + '%', height: '14px' }"></div>
          <div class="skeleton" style="width:50%;height:10px"></div>
        </div>
        <div class="skeleton" style="width:120px;height:24px;border-radius:var(--radius-sm)"></div>
      </div>
    </div>
    <div v-else-if="!groups.length" class="workspace-empty-state" style="flex:1">
      <UiIcon name="link" :size="40" style="opacity:.4" />
      <span class="wes-title">{{ history.length ? '没有匹配的分享记录' : '还没有创建过分享记录' }}</span>
      <span v-if="!history.length" class="wes-desc">在「网盘」页选中文件后点击「分享」创建链接</span>
    </div>

    <!-- 分组记录 -->
    <div v-else class="share-groups">
      <section v-for="g in groups" :key="g.key" class="share-group">
        <header class="share-group-header">
          <img v-if="g.icon" :src="g.icon" style="width:16px;height:16px;border-radius:4px" alt="" />
          <span class="share-group-provider">{{ g.label }}</span>
          <span class="share-group-account">{{ g.accName }}</span>
          <span class="share-group-count">{{ g.items.length }} 条</span>
        </header>

        <div v-for="(h, idx) in g.items" :key="itemKey(h, idx)" class="share-record">
          <div class="share-record-main">
            <div class="share-record-name" :title="h.share_name">{{ h.share_name || '未命名分享' }}</div>
            <div class="share-record-meta">
              <span>创建于 {{ formatTime(h.created_at) }}</span>
            </div>
            <div class="share-record-link" :title="h.share_url"><UiIcon name="link" :size="12" /><span>{{ h.share_url }}</span></div>
          </div>
          <div class="share-record-side">
            <span v-if="h.share_pwd" class="share-passcode" title="提取码"><UiIcon name="info" :size="11" />{{ h.share_pwd }}</span>
            <div class="share-record-actions">
              <button class="btn-circle" title="打开分享链接" :disabled="!h.share_url" @click="openLink(h)"><UiIcon name="external" :size="14" /></button>
              <button class="tbtn" :disabled="!h.share_url" @click="copy(h.share_url, '已复制链接', 'url_' + itemKey(h, idx))">
                <UiIcon v-if="copiedMap['url_' + itemKey(h, idx)]" name="check" :size="13" class="icon-check-pop" />
                <UiIcon v-else name="copy" :size="13" />
                <span>{{ copiedMap['url_' + itemKey(h, idx)] ? '已复制' : '链接' }}</span>
              </button>
              <button v-if="h.share_pwd" class="tbtn" @click="copy(h.share_pwd, '已复制提取码', 'pwd_' + itemKey(h, idx))">
                <UiIcon v-if="copiedMap['pwd_' + itemKey(h, idx)]" name="check" :size="13" class="icon-check-pop" />
                <UiIcon v-else name="copy" :size="13" />
                <span>{{ copiedMap['pwd_' + itemKey(h, idx)] ? '已复制' : '提取码' }}</span>
              </button>
              <button class="tbtn" @click="copyAll(h, 'all_' + itemKey(h, idx))">
                <UiIcon v-if="copiedMap['all_' + itemKey(h, idx)]" name="check" :size="13" class="icon-check-pop" />
                <UiIcon v-else name="copy" :size="13" />
                <span>{{ copiedMap['all_' + itemKey(h, idx)] ? '已复制' : '全部' }}</span>
              </button>
            </div>
          </div>
        </div>
      </section>
    </div>
  </div>
</template>
