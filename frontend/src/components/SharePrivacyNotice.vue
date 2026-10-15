<script setup>
defineProps({ caps: { type: Object, required: true } })
</script>
<template>
  <section class="hint" aria-label="分享隐私与安全提示">
    <p>任何获得链接（及提取码）的人均可访问内容，请勿分享敏感文件。</p>
    <p v-if="!caps.sharePassword">此网盘不支持自定义提取码。</p>
    <p v-if="!caps.shareExpiration">此网盘不支持自定义有效期，将使用网盘默认设置。</p>
    <p>{{ caps.cancelCreatedShares ? '创建后可在分享记录中取消分享。' : '此网盘不支持在应用内取消分享，请前往官网管理。' }}</p>
  </section>
</template>
