<script setup>
defineProps({ details: { type: Object, default: () => ({}) } })
</script>

<template>
  <div class="playback-info">
    <strong>播放信息</strong>
    <span>流类型：{{ details.type || '尚未识别' }}</span>
    <span>画面尺寸：{{ details.width && details.height ? `${details.width} × ${details.height}` : '尚未读取' }}</span>
    <span v-if="Number.isInteger(details.audioTracks)">音轨：{{ details.audioTracks }} 条</span>
    <small>数据来自播放器；无法播放时可下载后打开。</small>
  </div>
</template>

<style scoped>
.playback-info { display: grid; gap: 7px; padding: 10px 14px; max-width: 300px; white-space: normal; font-size: 12px; }
span, small { color: var(--text-secondary); }
</style>
