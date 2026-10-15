<script setup>
defineProps({ caps: { type: Object, required: true } })
</script>
<template>
  <section class="hint" aria-label="分享隐私与撤销说明">
    <p>持有链接（及提取码）的人可能访问内容；这不是指定用户授权，请勿分享敏感文件。</p>
    <p v-if="!caps.sharePassword">当前服务不支持在此设置提取码。</p>
    <p v-if="!caps.shareExpiration">当前服务不支持在此设置有效期，请确认平台默认期限。</p>
    <p>{{ caps.cancelCreatedShares ? '创建后可从分享记录请求远端撤销；本机记录不代表完整云端历史。' : '当前服务不支持在应用内单独撤销链接；请使用官网管理或等待链接过期。' }}</p>
  </section>
</template>
