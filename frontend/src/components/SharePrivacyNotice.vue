<script setup>
defineProps({ caps: { type: Object, required: true } })
</script>
<template>
  <section class="hint" aria-label="分享隐私与安全提示">
    <p>链接可转发，请勿分享敏感文件。</p>
    <details v-if="!caps.sharePassword || !caps.shareExpiration || !caps.cancelCreatedShares">
      <summary>分享设置说明</summary>
      <p v-if="!caps.sharePassword">此网盘不支持自定义提取码。</p>
      <p v-if="!caps.shareExpiration">有效期使用网盘默认设置。</p>
      <p v-if="!caps.cancelCreatedShares">取消分享请前往网盘官网。</p>
    </details>
  </section>
</template>

<style scoped>
summary { cursor: pointer; margin-top: 6px; }
</style>
