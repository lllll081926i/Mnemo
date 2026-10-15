<script setup>
defineProps({ details: { type: Object, default: () => ({}) } })
</script>

<template>
  <div class="playback-info">
    <strong>网页播放信息</strong>
    <span>流类型：{{ details.type || '尚未识别' }}</span>
    <span>画面尺寸：{{ details.width && details.height ? `${details.width} × ${details.height}` : '尚未读取' }}</span>
    <span>音轨：{{ Number.isInteger(details.audioTracks) ? `${details.audioTracks} 条（浏览器报告）` : '当前 WebView 未提供音轨数量' }}</span>
    <span>解码器、HDR 与硬件加速：未检测</span>
    <small>流类型不是实际编码格式。格式、音轨和 HDR 支持取决于当前系统与 WebView；应用不提供通用转码。不支持时请下载后使用本地播放器。</small>
  </div>
</template>

<style scoped>
.playback-info { display: grid; gap: 7px; padding: 10px 14px; max-width: 300px; white-space: normal; font-size: 12px; }
span, small { color: var(--text-secondary); }
</style>
