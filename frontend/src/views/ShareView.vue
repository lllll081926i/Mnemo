<script setup>
// 分享记录页：聚合全部账号的分享历史，按网盘 + 账号分组展示。
import { ref, shallowRef, computed, useId, watch, onMounted, onActivated, onDeactivated, onBeforeUnmount } from 'vue'
import { ListShareHistory, OpenBrowser, onEvent, providerOf, accountName, providerIconUrl, providerMetaOf, capsOf, formatTime, copyText, importShare, saveImportedShare, cancelShare } from '../api'
import UiIcon from '../components/UiIcon.vue'
import UiSelect from '../components/UiSelect.vue'
import Modal from '../components/Modal.vue'
import SelectDirModal from '../components/SelectDirModal.vue'
import ConfirmModal from '../components/ConfirmModal.vue'
import ShareImportHistory from '../components/ShareImportHistory.vue'
import { parseShareText } from '../workspace'
import { shareExpiry } from '../share-expiry'

const props = defineProps({
  accounts: { type: Array, default: () => [] },
  providers: { type: Array, default: () => [] },
})
const emit = defineEmits(['toast', 'navigate'])
const importedLocation = ref(null)
const lastImportReceipt = ref(null)

const history = shallowRef([])
const expiryNow = ref(Date.now())
let expiryTimer
const loading = ref(false)
const historyLoaded = ref(false)
const loadError = ref('')
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
const canceling = ref(new Set())
const confirmDialog = ref(null)

// 分享导入始终绑定一个明确的目标账号，避免多账号之间串用会话或目录。
const importOpen = ref(false)
const importStep = ref('form')
const importPhase = ref('idle')
const importBusy = computed(() => importPhase.value !== 'idle')
const importError = ref('')
const importTargetId = ref('')
const importForm = ref({ url: '', password: '' })
const importSession = shallowRef(null)
const importSelected = ref(new Set())
const importDirPick = ref(false)
const importDir = ref({ id: 'root', name: '根目录' })
const IMPORT_ROW_HEIGHT = 36
const IMPORT_VIEW_ROWS = 7
const IMPORT_OVERSCAN = 5
const importFileList = ref(null)
const importLinkInput = ref(null)
const importOptionPrefix = useId()
const importScrollRow = ref(0)
const importFocusIndex = ref(0)
const importFiles = computed(() => {
  const seen = new Set()
  return (importSession.value?.files || []).filter(file => {
    if (!file?.fileId || seen.has(file.fileId)) return false
    seen.add(file.fileId)
    return true
  })
})
const importWindowStart = computed(() => Math.max(0, importScrollRow.value - IMPORT_OVERSCAN))
const importWindowEnd = computed(() => Math.min(importFiles.value.length, importScrollRow.value + IMPORT_VIEW_ROWS + 1 + IMPORT_OVERSCAN))
const visibleImportFiles = computed(() => importFiles.value.slice(importWindowStart.value, importWindowEnd.value))
const importActiveIndex = computed(() => Math.min(importFiles.value.length - 1, Math.min(importScrollRow.value + IMPORT_VIEW_ROWS - 1, Math.max(importScrollRow.value, importFocusIndex.value))))
const importOptionId = index => `${importOptionPrefix}-${index}`
const allImportSelected = computed(() => importFiles.value.length > 0 && importSelected.value.size === importFiles.value.length)

watch(importStep, (step) => {
  if (!importOpen.value) return
  const target = step === 'files' ? importFileList.value : importLinkInput.value
  target?.focus({ preventScroll: true })
}, { flush: 'post' })

const detectedShare = computed(() => parseShareText(importForm.value.url))
const importAccounts = computed(() => (props.accounts || []).filter((account) => capsOf(account, props.providers).importShare && (!detectedShare.value.provider || providerOf(account.user_id) === detectedShare.value.provider)))
watch(() => [detectedShare.value.url, detectedShare.value.password], ([url, password], [oldUrl, oldPassword]) => {
  if (url !== oldUrl || password) importForm.value.password = password
  else if (oldPassword && importForm.value.password === oldPassword) importForm.value.password = ''
})
const importTarget = computed(() => importAccounts.value.find((account) => account.user_id === importTargetId.value) || null)
let importSeq = 0
watch(() => [importTarget.value?.user_id, importTarget.value?.drive_id], ([user, drive], [oldUser, oldDrive]) => {
  if (user === oldUser && drive === oldDrive) return
  if (importPhase.value !== 'saving') {
    ++importSeq
    importPhase.value = 'idle'
  }
  importError.value = ''
  importStep.value = 'form'
  importSession.value = null
  importSelected.value.clear()
  importDirPick.value = false
})
onBeforeUnmount(() => { ++importSeq })
const importDirRoot = computed(() => {
  const meta = providerMetaOf(importTarget.value, props.providers)
  return { id: meta.rootKey || 'root', name: meta.rootTitle || '根目录' }
})
const importAccountOptions = computed(() => importAccounts.value.map((account) => ({
  value: account.user_id,
  img: providerIconUrl(providerMetaOf(account, props.providers)),
  label: `${providerMetaOf(account, props.providers).label || providerOf(account.user_id)} · ${accountName(account)}`,
})))

function formatSize(size) {
  const value = Number(size) || 0
  if (value <= 0) return '0 B'
  if (value < 1024) return `${value} B`
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KB`
  if (value < 1024 * 1024 * 1024) return `${(value / 1024 / 1024).toFixed(1)} MB`
  return `${(value / 1024 / 1024 / 1024).toFixed(1)} GB`
}

watch(importAccounts, (list) => {
  if (!list.some((account) => account.user_id === importTargetId.value)) {
    importTargetId.value = list[0]?.user_id || ''
  }
}, { immediate: true })

function openImport() {
  if (importOpen.value) return
  importOpen.value = true
  importError.value = ''
  importStep.value = 'form'
  importSession.value = null
  importSelected.value.clear()
  importDir.value = importDirRoot.value
}

function closeImport() {
  if (importPhase.value === 'saving') return
  ++importSeq
  importPhase.value = 'idle'
  importError.value = ''
  importOpen.value = false
  importDirPick.value = false
  importSession.value = null
  importSelected.value.clear()
}

async function parseImport(event) {
  if (event?.isComposing || event?.keyCode === 229) return
  const url = detectedShare.value.url
  if (!url || !importTarget.value || importBusy.value) return
  const seq = ++importSeq
  importPhase.value = 'parsing'
  importError.value = ''
  try {
    const session = await importShare(importTarget.value.user_id, importTarget.value.drive_id, url, importForm.value.password.trim())
    if (seq !== importSeq) return
    if (!session || !Array.isArray(session.files) || !session.files.length) {
      throw new Error('分享中没有可导入的文件')
    }
    importSession.value = session
    if (!importFiles.value.length) {
      importSession.value = null
      throw new Error('分享中没有可导入的文件')
    }
    importSelected.value.clear()
    importScrollRow.value = 0
    importFocusIndex.value = 0
    importDir.value = importDirRoot.value
    importStep.value = 'files'
  } catch (e) {
    if (seq !== importSeq) return
    importError.value = String(e?.message || e)
  } finally {
    if (seq === importSeq) importPhase.value = 'idle'
  }
}

function toggleImportFile(file) {
  const id = file && file.fileId
  if (!id || importBusy.value) return
  const selected = importSelected.value
  if (selected.has(id)) selected.delete(id)
  else selected.add(id)
}

function toggleAllImportFiles(select = !allImportSelected.value) {
  if (importBusy.value) return
  if (select) importSelected.value = new Set(importFiles.value.map(file => file.fileId))
  else importSelected.value.clear()
}

function updateImportScroll(list) {
  importScrollRow.value = Math.floor(Math.max(0, list.scrollTop) / IMPORT_ROW_HEIGHT)
}

function focusImportFile(index) {
  const list = importFileList.value
  if (!list) return
  importFocusIndex.value = index
  const top = index * IMPORT_ROW_HEIGHT
  if (top < list.scrollTop) list.scrollTop = top
  else if (top + IMPORT_ROW_HEIGHT > list.scrollTop + list.clientHeight) list.scrollTop = top + IMPORT_ROW_HEIGHT - list.clientHeight
  updateImportScroll(list)
}

function selectImportFile(index) {
  if (importBusy.value) return
  importFileList.value?.focus({ preventScroll: true })
  focusImportFile(index)
  toggleImportFile(importFiles.value[index])
}

function onImportFileKey(event) {
  const last = importFiles.value.length - 1
  if (last < 0) return
  const index = importActiveIndex.value
  let next = index
  if (event.key === 'ArrowUp') next--
  else if (event.key === 'ArrowDown') next++
  else if (event.key === 'Home') next = 0
  else if (event.key === 'End') next = last
  else if (event.key === ' ') {
    event.preventDefault()
    if (!event.repeat) toggleImportFile(importFiles.value[index])
    return
  } else if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'a') {
    event.preventDefault()
    if (!allImportSelected.value) toggleAllImportFiles(true)
    return
  } else return
  event.preventDefault()
  focusImportFile(Math.max(0, Math.min(last, next)))
}

function backToImportForm() {
  if (importBusy.value) return
  importStep.value = 'form'
  importSession.value = null
  importSelected.value.clear()
}

async function saveImport() {
  if (!importTarget.value || !importSession.value || !importSelected.value.size || importBusy.value) return
  const seq = ++importSeq
  const target = importTarget.value
  const dir = importDir.value
  const selectedIds = [...importSelected.value]
  importPhase.value = 'saving'
  try {
    const saved = await saveImportedShare(
      target.user_id,
      target.drive_id,
      importSession.value,
      selectedIds,
      dir.id,
    )
    if (seq !== importSeq) return
    const items = saved?.items
    const sourceIDs = new Set(Array.isArray(items) ? items.map(item => item.sourceId) : [])
    if (!saved?.id || !Array.isArray(items) || items.length !== selectedIds.length || sourceIDs.size !== items.length || !selectedIds.every(id => sourceIDs.has(id))) {
      throw new Error('转存提交结果不完整，请先检查目标目录，避免重复转存')
    }
    lastImportReceipt.value = saved
    const completed = items.every(item => item.status === 'completed')
    const uncertain = items.some(item => !['completed', 'submitted'].includes(item.status)) || !saved.historySaved
    emit('toast', saved.message || (completed ? `已确认转存 ${items.length} 个项目` : `已提交转存 ${items.length} 个项目，等待网盘处理`), uncertain ? 'warn' : 'success')
    importedLocation.value = { userId: target.user_id, dirId: dir.id, name: dir.name }
    importOpen.value = false
    importDirPick.value = false
    importSession.value = null
    importSelected.value.clear()
  } catch (e) {
    if (seq !== importSeq) return
    emit('toast', String(e), 'error')
  } finally {
    if (seq === importSeq) importPhase.value = 'idle'
  }
}

let refreshSeq = 0
let refreshQueued = false
async function refresh() {
  if (!viewActive) return
  if (loading.value) { refreshQueued = true; return }
  const seq = ++refreshSeq
  loading.value = true
  loadError.value = ''
  try {
    do {
      refreshQueued = false
      try {
        const list = (await ListShareHistory('')) || []
        if (seq === refreshSeq && !refreshQueued) {
          history.value = list
          historyLoaded.value = true
        }
      } catch (error) {
        if (seq === refreshSeq && !refreshQueued) {
          loadError.value = '分享记录加载失败，请重试'
          emit('toast', String(error), 'error')
        }
      }
    } while (seq === refreshSeq && refreshQueued)
  } finally {
    if (seq === refreshSeq) loading.value = false
  }
}

function refreshFromToolbar() {
  if (!loading.value) return refresh()
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
  return `${h.account_id || ''}|${h.share_id || h.share_url || idx}`
}

function metaOf(pid) {
  const p = props.providers.find((x) => x.ID === pid)
  return (p && p.Meta) || {}
}

function labelOf(pid) {
  return metaOf(pid).label || pid || '未知网盘'
}

function accountOf(userId) {
  return accountsByID.value.get(userId) || null
}
const accountsByID = computed(() => new Map(props.accounts.map(account => [account.user_id, account])))

// 从记录中聚合出现过的 provider 列表
const providerOptions = computed(() => {
  const seen = new Set()
  for (const h of history.value) {
    const pid = h.provider || providerOf(h.account_id)
    if (pid) seen.add(pid)
  }
  return [...new Set([...props.accounts.map(a => providerOf(a.user_id)).filter(pid => seen.has(pid)), ...seen])]
})

// 账号筛选只列出有分享记录的账号
const accountOptions = computed(() => {
  const seen = new Map()
  for (const h of history.value) {
    if (!h.account_id || seen.has(h.account_id)) continue
    const acc = accountOf(h.account_id)
    seen.set(h.account_id, acc ? accountName(acc) : h.account_id)
  }
  const ids = [...new Set([...props.accounts.map(a => a.user_id).filter(id => seen.has(id)), ...seen.keys()])]
  return ids.map(id => ({ id, name: seen.get(id), img: providerIconUrl(providerMetaOf(accountOf(id) || { user_id: id }, props.providers)) }))
})

const searchIndex = computed(() => history.value.map(record => ({
  record,
  provider: record.provider || providerOf(record.account_id),
  terms: [record.share_name, record.share_url, record.share_pwd].map(value => String(value || '').toLowerCase()),
})))

const filtered = computed(() => {
  const k = kw.value.trim().toLowerCase()
  if (!k && !filterProvider.value && !filterAccount.value) return history.value
  const matches = []
  for (const { record, provider, terms } of searchIndex.value) {
    if (filterProvider.value && provider !== filterProvider.value) continue
    if (filterAccount.value && record.account_id !== filterAccount.value) continue
    if (!k || terms.some(term => term.includes(k))) matches.push(record)
  }
  return matches
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
        accountId: h.account_id,
        pid,
        icon: providerIconUrl(acc ? providerMetaOf(acc, props.providers) : metaOf(pid)),
        label: labelOf(pid),
        accName: acc ? accountName(acc) : h.account_id,
        items: [],
      })
    }
    map.get(key).items.push(h)
  }
  const ranks = new Map(props.accounts.map((a, index) => [a.user_id, index]))
  return [...map.values()].sort((a, b) => (ranks.get(a.accountId) ?? Infinity) - (ranks.get(b.accountId) ?? Infinity))
})

async function openLink(h) {
  if (!h.share_url) return
  try { await OpenBrowser(h.share_url) }
  catch (error) { emit('toast', '打开分享链接失败：' + String(error?.message || error), 'error') }
}

const copiedMap = ref({})

async function copy(text, tip, key) {
  try {
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
  } catch (error) {
    emit('toast', '复制失败：' + String(error?.message || error), 'error')
  }
}

function copyAll(h, key) {
  const lines = [h.share_name || '分享文件', h.share_url || '']
  if (h.share_pwd) lines.push('提取码: ' + h.share_pwd)
  copy(lines.join('\n'), '已复制分享信息', key)
}

function canCancel(h) {
  const account = accountOf(h.account_id)
  return Boolean(account && h.drive_id && h.share_id && capsOf(account, props.providers).cancelCreatedShares)
}

function isCancelling(h, idx) {
  return canceling.value.has(itemKey(h, idx))
}

function askCancel(h, idx) {
  if (!canCancel(h) || isCancelling(h, idx)) return
  confirmDialog.value = {
    title: '取消分享',
    message: '取消分享后，链接将立即失效，其他人将无法访问。确定取消？',
    okText: '确认取消',
    danger: true,
    onOk: () => cancelRemote(h, idx),
  }
}

async function cancelRemote(h, idx) {
  const key = itemKey(h, idx)
  if (canceling.value.has(key)) return
  canceling.value = new Set([...canceling.value, key])
  try {
    await cancelShare(h)
    history.value = history.value.filter((entry) => {
      if (entry.account_id !== h.account_id) return true
      return h.share_id
        ? entry.share_id !== h.share_id
        : entry.share_url !== h.share_url
    })
    emit('toast', '分享已取消', 'success')
  } catch (e) {
    emit('toast', String(e), 'error')
  } finally {
    const next = new Set(canceling.value)
    next.delete(key)
    canceling.value = next
  }
}

function closeConfirm() { confirmDialog.value = null }
function handleConfirmOk() {
  const callback = confirmDialog.value?.onOk
  closeConfirm()
  if (typeof callback === 'function') callback()
}

const offs = []
let viewActive = false
function activateView() {
  if (viewActive) return
  viewActive = true
  expiryNow.value = Date.now()
  expiryTimer = setInterval(() => { expiryNow.value = Date.now() }, 60000)
  refresh()
  offs.push(onEvent('account:changed', refresh))
  offs.push(onEvent('share:history-changed', refresh))
}
function deactivateView() {
  viewActive = false
  ++refreshSeq
  refreshQueued = false
  loading.value = false
  clearInterval(expiryTimer)
  offs.splice(0).forEach(off => off?.())
}
onMounted(activateView)
onActivated(activateView)
onDeactivated(deactivateView)
onBeforeUnmount(deactivateView)
</script>

<template>
  <div class="share-page">
    <div class="share-layout">
      <!-- 工具条 -->
      <header class="share-toolbar">
        <div class="share-toolbar-main">
          <span class="search-quick-wrap">
            <span class="sq-icon"><UiIcon name="search" :size="13" /></span>
            <input class="search-quick share-search" v-model="kwRaw" placeholder="搜索名称 / 链接 / 提取码" />
            <button v-if="kwRaw" class="sq-clear" title="清空搜索" @click="kwRaw = ''"><UiIcon name="close" :size="11" /></button>
          </span>
          <UiSelect
            v-model="filterProvider"
            class="share-filter"
            :options="[{ value: '', label: '全部网盘' }, ...providerOptions.map((pid) => ({ value: pid, label: labelOf(pid), img: providerIconUrl(metaOf(pid)) }))]"
          />
          <UiSelect
            v-model="filterAccount"
            class="share-filter"
            :options="[{ value: '', label: '全部账号' }, ...accountOptions.map((a) => ({ value: a.id, label: a.name, img: a.img }))]"
          />
        </div>
        <div class="share-toolbar-actions">
          <button v-if="importedLocation" class="tbtn" @click="emit('navigate', importedLocation)">打开目录</button>
          <button class="btn primary" @click="openImport">
            <UiIcon name="download" :size="14" />导入分享
          </button>
          <button class="tbtn" :aria-disabled="loading" :aria-busy="loading" @click="refreshFromToolbar">
            <span v-if="loading" class="spin" aria-hidden="true"></span><UiIcon v-else name="refresh" :size="14" />刷新
          </button>
        </div>
      </header>

      <section class="share-content">
        <ShareImportHistory :accounts="accounts" :latest="lastImportReceipt" @toast="(message, type) => emit('toast', message, type)" @navigate="location => emit('navigate', location)" />
        <div v-if="loadError" role="alert" class="share-refresh-error">
          <span>{{ loadError }}{{ history.length ? '，已保留上次加载的记录。' : '' }}</span>
          <button class="btn" :disabled="loading" @click="refresh">重新加载</button>
        </div>
        <!-- 骨架屏加载状态 -->
        <div v-if="loading && !historyLoaded" class="skeleton-list share-skeleton">
          <div v-for="i in 4" :key="i" class="skeleton-row share-skeleton-row">
            <div class="skeleton skeleton-icon share-skeleton-icon"></div>
            <div class="share-skeleton-copy">
              <div class="skeleton" :style="{ width: (30 + (i * 11) % 40) + '%', height: '14px' }"></div>
              <div class="skeleton" style="width:50%;height:10px"></div>
            </div>
            <div class="skeleton share-skeleton-action"></div>
          </div>
        </div>
        <div v-else-if="!groups.length && (!loadError || history.length)" class="share-empty content-reveal">
          <div class="share-empty-icon"><UiIcon name="link" :size="28" /></div>
          <span class="wes-title">{{ history.length ? '没有匹配的分享记录' : '还没有分享记录' }}</span>
          <template v-if="!history.length">
            <span class="wes-desc">在网盘中选中文件创建分享后，可在此查看链接、提取码及管理分享。</span>
            <span class="wes-desc">收到分享链接时，也可直接转存到支持的网盘。</span>
            <button class="btn" @click="openImport"><UiIcon name="download" :size="14" />导入分享</button>
          </template>
          <template v-else>
            <span class="wes-desc">未找到相关记录，可尝试更换关键词或清除筛选。</span>
            <button class="btn" @click="kwRaw = ''; kw = ''; filterProvider = ''; filterAccount = ''">清除筛选</button>
          </template>
        </div>

        <!-- 分组记录 -->
        <div v-else class="share-groups content-reveal">
          <section v-for="g in groups" :key="g.key" class="share-group">
            <header class="share-group-header">
              <img v-if="g.icon" :src="g.icon" class="share-group-icon" alt="" />
              <span class="share-group-provider">{{ g.label }}</span>
              <span class="share-group-account">{{ g.accName }}</span>
              <span class="share-group-count">{{ g.items.length }} 条</span>
            </header>

            <div v-for="(h, idx) in g.items" :key="itemKey(h, idx)" class="share-record">
              <div class="share-record-main">
                <div class="share-record-name" :title="h.share_name">{{ h.share_name || '未命名分享' }}</div>
                <div class="share-record-meta">
                  <span>创建于 {{ formatTime(h.created_at) }}</span>
                  <span :class="{ 'share-expiry-warning': shareExpiry(h, expiryNow).warning }">{{ shareExpiry(h, expiryNow).label }}</span>
                </div>
                <div class="share-record-link" :title="h.share_url"><UiIcon name="link" :size="12" /><span>{{ h.share_url }}</span></div>
              </div>
              <div class="share-record-side">
				<span class="sg-desc">{{ h.remote_status === 'active' ? '已创建' : '本地记录' }} · {{ h.share_policy === 'public' ? '持链接可访问' : '访问范围未记录' }} · {{ h.share_pwd ? '含提取码' : '未记录提取码' }}</span>
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
                  <button v-if="canCancel(h)" class="tbtn danger" :disabled="isCancelling(h, idx)" @click="askCancel(h, idx)">
                    <span v-if="isCancelling(h, idx)" class="spin"></span>
                    <template v-else><UiIcon name="trash" :size="13" /><span>取消</span></template>
                  </button>
                </div>
              </div>
            </div>
          </section>
        </div>
      </section>
    </div>

    <Modal v-if="importOpen" title="导入分享" width="620px" @close="closeImport">
    <div v-if="!accounts.some(a => capsOf(a, providers).importShare)" class="workspace-empty-state">
      <UiIcon name="cloud" :size="32" />
      <strong>请先添加支持分享导入的网盘账号</strong>
      <p>请先在左侧登录支持该网盘的账号，再粘贴分享链接。</p>
    </div>
    <div v-else-if="importStep === 'form'" key="form" class="content-reveal">
      <div class="field">
        <label>目标网盘账号</label>
        <UiSelect v-model="importTargetId" block :disabled="importBusy" :options="importAccountOptions" placeholder="选择账号" aria-label="目标网盘账号" />
      </div>
      <div class="field">
        <label :for="`${importOptionPrefix}-link`">分享链接</label>
        <textarea :id="`${importOptionPrefix}-link`" ref="importLinkInput" v-model="importForm.url" class="input" placeholder="粘贴链接或分享文案" :readonly="importBusy" rows="3" autofocus></textarea>
        <span class="hint" v-if="detectedShare.provider">已识别为 {{ providers.find(p => p.ID === detectedShare.provider)?.Meta?.label || detectedShare.provider }} 分享，仅列出兼容账号</span>
        <span v-if="!importAccounts.length" class="hint">未登录该网盘账号，请先添加账号后再试</span>
      </div>
      <div class="field">
        <label :for="`${importOptionPrefix}-password`">提取码（可选）</label>
        <input :id="`${importOptionPrefix}-password`" v-model="importForm.password" class="input" placeholder="没有提取码可留空" :readonly="importBusy" @keydown.enter="parseImport" />
      </div>
      <p v-if="importError" role="alert">{{ importError }}</p>
    </div>

    <div v-else key="selection" class="content-reveal">
      <div class="import-summary">
        <div><strong>{{ importSession?.shareUrl || importForm.url }}</strong></div>
        <span aria-live="polite">{{ importFiles.length }} 个项目 · 已选 {{ importSelected.size }} 个</span>
      </div>
      <div class="import-file-toolbar">
        <button class="tbtn xs" :disabled="importBusy" @click="toggleAllImportFiles()">{{ allImportSelected ? '取消全选' : '全选' }}</button>
      </div>
      <div ref="importFileList" class="import-file-list" :style="{ '--import-row-height': `${IMPORT_ROW_HEIGHT}px`, '--import-list-height': `${IMPORT_ROW_HEIGHT * IMPORT_VIEW_ROWS}px` }" role="listbox" aria-label="待转存文件" aria-multiselectable="true" :aria-disabled="importBusy" :aria-activedescendant="importActiveIndex >= 0 ? importOptionId(importActiveIndex) : undefined" tabindex="0" @scroll.passive="updateImportScroll($event.currentTarget)" @keydown="onImportFileKey">
        <div aria-hidden="true" :style="{ height: `${importWindowStart * IMPORT_ROW_HEIGHT}px` }"></div>
        <div v-for="(file, offset) in visibleImportFiles" :id="importOptionId(importWindowStart + offset)" :key="file.fileId" class="import-file-row" role="option" :aria-posinset="importWindowStart + offset + 1" :aria-setsize="importFiles.length" :aria-selected="importSelected.has(file.fileId)" :class="{ 'is-active': importActiveIndex === importWindowStart + offset }" @click="selectImportFile(importWindowStart + offset)">
          <span class="import-file-check" aria-hidden="true"><UiIcon name="check" :size="12" /></span>
          <UiIcon :name="file.isDir ? 'folder' : 'file'" :size="16" />
          <span class="import-file-name" :title="file.name">{{ file.name }}</span>
          <span class="import-file-size">{{ file.isDir ? '文件夹' : formatSize(file.size) }}</span>
        </div>
        <div aria-hidden="true" :style="{ height: `${(importFiles.length - importWindowEnd) * IMPORT_ROW_HEIGHT}px` }"></div>
      </div>
      <div class="field import-target-dir">
        <label>保存到</label>
        <div class="import-dir-row">
          <input class="input" :value="importDir.name" readonly />
          <button class="btn" :disabled="importBusy || !importTarget" @click="importDirPick = true">选择目录</button>
        </div>
      </div>
    </div>

    <template #actions>
      <button class="btn" :disabled="importPhase === 'saving'" @click="closeImport">取消</button>
      <template v-if="importStep === 'form'">
        <button class="btn primary" :disabled="!detectedShare.url || !importTarget" :aria-disabled="importBusy" :aria-busy="importPhase === 'parsing'" @click="parseImport">
          <span v-if="importPhase === 'parsing'" class="spin spin-on-primary"></span>{{ importPhase === 'parsing' ? '解析中…' : '解析分享' }}
        </button>
      </template>
      <template v-else>
        <button class="btn" :disabled="importBusy" @click="backToImportForm">返回</button>
        <button class="btn primary" :disabled="importBusy || !importSelected.size" @click="saveImport">
          <span v-if="importBusy" class="spin spin-on-primary"></span>{{ importBusy ? '导入中…' : '导入所选项目' }}
        </button>
      </template>
    </template>
  </Modal>

    <SelectDirModal
      v-if="importDirPick && importTarget"
      title="选择保存目录"
      :account="importTarget"
      :providers="providers"
      @close="importDirPick = false"
      @select="(dir) => { importDir = dir; importDirPick = false }"
      @toast="(message, type) => emit('toast', message, type)"
    />
    <ConfirmModal v-if="confirmDialog" :title="confirmDialog.title" :message="confirmDialog.message" :okText="confirmDialog.okText" :danger="confirmDialog.danger" @ok="handleConfirmOk" @cancel="closeConfirm" />
  </div>
</template>

<style scoped>
.share-expiry-warning { color: var(--color-warning); font-weight: 600; }
.share-refresh-error { display:flex; align-items:center; justify-content:space-between; gap:12px; padding:9px 12px; margin-bottom:12px; border:1px solid var(--border-light); border-radius:var(--radius-md); background:var(--bg-subtle); color:var(--text-secondary); font-size:12.5px; }
.share-refresh-error .btn { flex-shrink:0; }
.import-summary { display:flex; flex-direction:column; gap:4px; padding:9px 10px; margin-bottom:10px; border:1px solid var(--border-lighter); border-radius:var(--radius-md); color:var(--text-secondary); font-size:12px; }
.import-summary strong { overflow:hidden; text-overflow:ellipsis; white-space:nowrap; color:var(--text-primary); font-weight:600; }
.import-file-toolbar { display:flex; align-items:center; gap:8px; margin-bottom:6px; }
.import-file-list { box-sizing:content-box; max-height:var(--import-list-height); overflow-y:auto; overflow-anchor:none; scrollbar-gutter:stable; border:1px solid var(--border-lighter); border-radius:var(--radius-md); padding:0 4px; }
.import-file-list:focus-visible { outline:1px solid var(--border-focus); }
.import-file-row { box-sizing:border-box; display:flex; align-items:center; gap:8px; height:var(--import-row-height); padding:0 7px; border-radius:var(--radius-sm); cursor:pointer; transition:background-color var(--motion-fast) var(--motion-ease); }
.import-file-list:not([aria-disabled=true]) .import-file-row:hover { background:var(--bg-hover); }
.import-file-list[aria-disabled=true] .import-file-row { cursor:default; }
.import-file-row[aria-selected=true] { background:var(--bg-active); }
.import-file-list:focus-visible .import-file-row.is-active { box-shadow:inset 0 0 0 1px var(--border-focus); }
.import-file-check { display:grid; place-items:center; flex:0 0 16px; height:16px; border:1px solid var(--text-tertiary); border-radius:4px; color:var(--text-on-primary, white); transition:background-color var(--motion-fast) var(--motion-ease), border-color var(--motion-fast) var(--motion-ease); }
.import-file-check svg { opacity:0; transform:scale(.7); transition:opacity var(--motion-fast) var(--motion-ease), transform var(--motion-fast) var(--motion-spring); }
.import-file-row[aria-selected=true] .import-file-check { background:var(--color-primary); border-color:var(--color-primary); }
.import-file-row[aria-selected=true] .import-file-check svg { opacity:1; transform:scale(1); }
@media (prefers-reduced-motion: reduce) {
  .import-file-row, .import-file-check, .import-file-check svg { transition:none; }
}
.import-file-name { min-width:0; flex:1; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
.import-file-size { flex:0 0 auto; color:var(--text-tertiary); font-size:12px; }
.import-dir-row { display:flex; gap:6px; }
.import-dir-row .input { flex:1; }
</style>
