<script setup>
import { computed } from 'vue'
const props = defineProps({ account: { type: Object, default: null } })
const labels = { authentication: '登录状态', download: '下载', upload: '上传', createFolder: '新建目录', createTextFile: '新建文本', createDateFolder: '新建日期目录', rename: '重命名', move: '移动', copy: '复制', permanentDelete: '永久删除', trashView: '回收站列表', trashRestore: '回收站恢复', trashPurge: '回收站删除', trashClear: '清空回收站', createShare: '创建分享', importShare: '分享转存', manageCreatedShares: '管理分享', cancelCreatedShares: '撤销分享', editCreatedShares: '编辑分享', offlineDownload: '云离线', search: '搜索', favorite: '收藏', quickTransfer: '秒传', cloudVideoPreview: '云端播放', enterpriseTrashPreview: '企业回收站预览', familyCloud: '家庭云' }
const states = computed(() => Object.entries(props.account?.capability_states || {}).filter(([key]) => labels[key]))
const statuses = { supported: '已支持', conditional: '有使用条件', unsupported: '不可用', unknown: '未验证' }
</script>

<template>
  <details v-if="states.length" class="account-capabilities">
    <summary>账号能力与使用条件</summary>
    <p>功能入口表示驱动已接入，不表示当前账号、套餐和每个目录都已通过权限验证。</p>
    <dl><template v-for="[key, value] in states" :key="key"><dt>{{ labels[key] }} · {{ statuses[value.state] || '未验证' }}</dt><dd>{{ value.reason }}</dd></template></dl>
  </details>
</template>

<style scoped>
.account-capabilities { padding: 6px 12px; font-size: 12px; color: var(--text-secondary); border-bottom: 1px solid var(--border-lighter); max-height: 35vh; overflow: auto; }
summary { cursor: pointer; color: var(--text-primary); }
dl { display: grid; grid-template-columns: minmax(100px, auto) 1fr; gap: 6px 12px; }
dt { font-weight: 600; } dd { margin: 0; overflow-wrap: anywhere; }
</style>
