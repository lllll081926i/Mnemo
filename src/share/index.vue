<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Copy, KeyRound, Link2, Trash2 } from 'lucide-vue-next'
import { useAppStore, useUserStore } from '../store'
import { copyToClipboard } from '../utils/electronhelper'
import { humanDateTimeDateStr } from '../utils/format'
import message from '../utils/message'
import { GetShareUrlFormate } from '../utils/shareurl'
import { isShareHistoryActive, listShareHistory, removeShareHistory, shareHistoryProviderLabel, type ShareHistoryItem } from './sharehistory'

const appStore = useAppStore()
const userStore = useUserStore()

const records = ref<ShareHistoryItem[]>([])
const keyword = ref('')
const providerFilter = ref('all')
const statusFilter = ref<'all' | 'active' | 'expired'>('all')
const accountFilter = ref('all')

const reload = () => {
  records.value = listShareHistory()
}

watch(
  [() => appStore.appTab, () => userStore.user_id],
  ([activeTab]) => {
    if (activeTab == 'share') {
      if (appStore.GetAppTabMenu != 'ShareUnsupported') appStore.toggleTabMenu('share', 'ShareUnsupported')
      reload()
    }
  },
  { immediate: true }
)

const providerOptions = computed(() => Array.from(new Set(records.value.map((item) => item.provider))))
const accountOptions = computed(() => Array.from(new Set(records.value.map((item) => item.account).filter(Boolean))))

const filtered = computed(() => {
  const kw = keyword.value.trim().toLowerCase()
  return records.value.filter((item) => {
    if (providerFilter.value !== 'all' && item.provider !== providerFilter.value) return false
    if (accountFilter.value !== 'all' && item.account !== accountFilter.value) return false
    const active = isShareHistoryActive(item)
    if (statusFilter.value === 'active' && !active) return false
    if (statusFilter.value === 'expired' && active) return false
    if (!kw) return true
    return [item.share_name, item.account, item.share_url, item.pass_code].some((field) => String(field || '').toLowerCase().includes(kw))
  })
})

// 按账号聚合展示
const grouped = computed(() => {
  const groups = new Map<string, ShareHistoryItem[]>()
  for (const item of filtered.value) {
    const key = item.account || '未知账号'
    if (!groups.has(key)) groups.set(key, [])
    groups.get(key)!.push(item)
  }
  return Array.from(groups.entries()).map(([account, items]) => ({ account, provider: items[0]?.provider || '', items }))
})

const expirationText = (item: ShareHistoryItem) => {
  if (!item.expiration) return '永久有效'
  const expireAt = new Date(item.expiration).getTime()
  if (Number.isNaN(expireAt)) return '永久有效'
  return `${isShareHistoryActive(item) ? '有效期至' : '已过期于'} ${humanDateTimeDateStr(new Date(expireAt).toISOString())}`
}

const copyLink = (item: ShareHistoryItem) => {
  copyToClipboard(item.share_url)
  message.success('分享链接已复制')
}

const copyPassCode = (item: ShareHistoryItem) => {
  copyToClipboard(item.pass_code)
  message.success('提取码已复制')
}

const copyShareText = (item: ShareHistoryItem) => {
  copyToClipboard(GetShareUrlFormate(item.share_name, item.share_url, item.pass_code))
  message.success('分享内容已复制')
}

const removeRecord = (item: ShareHistoryItem) => {
  removeShareHistory(item.id)
  records.value = records.value.filter((entry) => entry.id !== item.id)
  message.success('已删除分享记录')
}
</script>

<template>
  <a-layout class="ui-workspace-shell">
    <a-layout-content class="mnemoright ui-workspace-content">
      <div v-if="records.length" class="share-page">
        <div class="share-toolbar">
          <a-input v-model="keyword" class="share-search" size="small" allow-clear placeholder="搜索文件名 / 账号 / 链接 / 提取码" />
          <a-select v-model="providerFilter" class="share-filter" size="small">
            <a-option value="all">全部网盘</a-option>
            <a-option v-for="provider in providerOptions" :key="provider" :value="provider">{{ shareHistoryProviderLabel(provider) }}</a-option>
          </a-select>
          <a-select v-model="accountFilter" class="share-filter" size="small">
            <a-option value="all">全部账号</a-option>
            <a-option v-for="account in accountOptions" :key="account" :value="account">{{ account }}</a-option>
          </a-select>
          <a-select v-model="statusFilter" class="share-filter" size="small">
            <a-option value="all">全部状态</a-option>
            <a-option value="active">有效</a-option>
            <a-option value="expired">已过期</a-option>
          </a-select>
        </div>

        <div v-if="grouped.length" class="share-groups">
          <section v-for="group in grouped" :key="group.account" class="share-group">
            <header class="share-group-header">
              <span class="share-group-provider">{{ shareHistoryProviderLabel(group.provider) }}</span>
              <span class="share-group-account">{{ group.account }}</span>
              <span class="share-group-count">{{ group.items.length }} 条</span>
            </header>
            <div v-for="item in group.items" :key="item.id" class="share-record" :class="{ expired: !isShareHistoryActive(item) }">
              <div class="share-record-main">
                <div class="share-record-name" :title="item.share_name">{{ item.share_name }}</div>
                <div class="share-record-meta">
                  <span>{{ expirationText(item) }}</span>
                  <span v-if="item.file_count > 1">{{ item.file_count }} 个文件</span>
                  <span>创建于 {{ humanDateTimeDateStr(new Date(item.created_at).toISOString()) }}</span>
                </div>
                <div class="share-record-link" :title="item.share_url"><Link2 :size="13" /><span>{{ item.share_url }}</span></div>
              </div>
              <div class="share-record-side">
                <span class="share-status" :class="isShareHistoryActive(item) ? 'active' : 'expired'">{{ isShareHistoryActive(item) ? '有效' : '已过期' }}</span>
                <span v-if="item.pass_code" class="share-passcode" title="提取码"><KeyRound :size="12" />{{ item.pass_code }}</span>
                <div class="share-record-actions">
                  <a-button type="text" size="mini" @click="copyLink(item)"><Copy :size="14" />链接</a-button>
                  <a-button v-if="item.pass_code" type="text" size="mini" @click="copyPassCode(item)"><Copy :size="14" />提取码</a-button>
                  <a-button type="text" size="mini" @click="copyShareText(item)"><Copy :size="14" />全部</a-button>
                  <a-button type="text" size="mini" status="danger" @click="removeRecord(item)"><Trash2 :size="14" /></a-button>
                </div>
              </div>
            </div>
          </section>
        </div>
        <div v-else class="share-no-match">没有匹配的分享记录</div>
      </div>

      <div v-else class="workspace-empty-state share-empty">
        <IconFont name="iconfenxiang" class="share-empty-icon" />
        <div class="share-empty-title">创建分享链接</div>
        <div class="share-empty-desc">在「网盘」中选中文件或文件夹后，使用顶部「分享」按钮创建链接。创建过的分享会记录在这里，按账号聚合展示。</div>
      </div>
    </a-layout-content>
  </a-layout>
</template>

<style scoped>
.share-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 12px;
  min-height: 60vh;
  padding: 32px;
  text-align: center;
  color: var(--color-text-2);
}
.share-empty-icon {
  font-size: 40px;
  opacity: 0.7;
}
.share-empty-title {
  font-size: 18px;
  font-weight: 600;
  color: var(--color-text-1);
}
.share-empty-desc {
  max-width: 420px;
  line-height: 1.6;
  font-size: 13px;
}

.share-page {
  display: flex;
  flex-direction: column;
  gap: 10px;
  height: 100%;
  padding: 12px 16px;
  overflow: auto;
}
.share-toolbar {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  align-items: center;
}
.share-search {
  flex: 1 1 240px;
  min-width: 200px;
}
.share-filter {
  width: 130px;
  flex: 0 0 auto;
}
.share-groups {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.share-group-header {
  display: flex;
  align-items: baseline;
  gap: 8px;
  padding: 2px 2px 6px;
  border-bottom: 1px solid var(--border-light);
}
.share-group-provider {
  font-size: 12px;
  font-weight: 600;
  color: var(--color-primary);
}
.share-group-account {
  font-size: 13px;
  font-weight: 600;
  color: var(--text-primary);
}
.share-group-count {
  font-size: 12px;
  color: var(--text-tertiary, var(--color-text-3));
}
.share-record {
  display: flex;
  gap: 12px;
  align-items: center;
  justify-content: space-between;
  padding: 8px 10px;
  border-bottom: 1px solid var(--border-light);
  border-radius: 6px;
}
.share-record:hover {
  background: var(--bg-hover);
}
.share-record.expired .share-record-name,
.share-record.expired .share-record-link {
  opacity: 0.55;
}
.share-record-main {
  display: flex;
  flex-direction: column;
  gap: 3px;
  min-width: 0;
  flex: 1 1 auto;
}
.share-record-name {
  overflow: hidden;
  font-size: 13px;
  font-weight: 600;
  color: var(--text-primary);
  text-overflow: ellipsis;
  white-space: nowrap;
}
.share-record-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 4px 12px;
  font-size: 12px;
  color: var(--color-text-3);
}
.share-record-link {
  display: inline-flex;
  gap: 4px;
  align-items: center;
  overflow: hidden;
  font-size: 12px;
  color: var(--color-text-3);
  text-overflow: ellipsis;
  white-space: nowrap;
}
.share-record-side {
  display: flex;
  flex: 0 0 auto;
  flex-direction: column;
  gap: 4px;
  align-items: flex-end;
}
.share-status {
  padding: 1px 8px;
  font-size: 11px;
  border-radius: 8px;
}
.share-status.active {
  color: #00a870;
  background: rgba(0, 168, 112, 0.12);
}
.share-status.expired {
  color: var(--color-text-3);
  background: var(--bg-subtle);
}
.share-passcode {
  display: inline-flex;
  gap: 3px;
  align-items: center;
  font-size: 12px;
  font-weight: 600;
  color: var(--color-primary);
  letter-spacing: 1px;
}
.share-record-actions {
  display: flex;
  gap: 2px;
  align-items: center;
}
.share-record-actions :deep(.arco-btn) {
  display: inline-flex;
  gap: 3px;
  align-items: center;
}
.share-no-match {
  padding: 40px 0;
  color: var(--color-text-3);
  font-size: 13px;
  text-align: center;
}
</style>
