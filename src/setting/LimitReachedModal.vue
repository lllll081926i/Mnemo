<script setup lang="ts">
import { ref } from 'vue'
import { X, Loader2, Sparkles } from 'lucide-vue-next'
import Config from '../config'
import { openExternal } from '../utils/electronhelper'
import message from '../utils/message'

defineProps<{ visible: boolean }>()
const emit = defineEmits<{ 'update:visible': [v: boolean] }>()

const upgrading = ref(false)
const isLoggedIn = ref(localStorage.getItem('app_user_authed') === '1')

async function handleUpgrade() {
  if (!Config.CREEM_API_KEY || !Config.CREEM_PRODUCT_ID) { message.error('Creem 未配置'); return }
  upgrading.value = true
  try {
    const apiBase = Config.CREEM_API_KEY.startsWith('creem_test_') ? 'https://test-api.creem.io' : 'https://api.creem.io'
    const resp = await fetch(`${apiBase}/v1/checkouts`, {
      method: 'POST', headers: { 'Content-Type': 'application/json', 'x-api-key': Config.CREEM_API_KEY },
      body: JSON.stringify({ productId: Config.CREEM_PRODUCT_ID, successUrl: 'boxplayer-auth://payment-success' }),
    })
    const data = await resp.json()
    if (data.checkoutUrl) openExternal(data.checkoutUrl)
    else message.error(data.message || '支付链接创建失败')
  } catch { message.error('网络请求失败') }
  finally { upgrading.value = false }
}
</script>

<template>
  <div v-if="visible" class="lim-mask" @click.self="emit('update:visible', false)">
    <div class="lim-modal">
      <button class="lim-close" @click="emit('update:visible', false)"><X :size="18" /></button>
      <div class="lim-icon"><Sparkles :size="32" /></div>
      <h2 class="lim-title">免费次数已用完</h2>
      <p class="lim-desc">升级 Pro 解锁无限 AI 搜索、文件整理和更多高级功能</p>

      <div class="lim-pricing">
        <div class="lim-col">
          <div class="lim-col-name">开源版</div>
          <div class="lim-col-price">免费</div>
          <ul class="lim-features">
            <li>文件管理 · 视频音乐播放</li>
            <li>AI 搜索 5次/天</li>
            <li>全网保存 5次/天</li>
          </ul>
        </div>
        <div class="lim-col lim-col-pro">
          <div class="lim-col-badge">PRO</div>
          <div class="lim-col-name">专业版</div>
          <div class="lim-col-price">$10 / 月</div>
          <ul class="lim-features">
            <li>AI 搜索 · 无限次</li>
            <li>AI 文件整理 · 查重</li>
            <li>AI 阅读 · 朗读 · 翻译</li>
            <li>全网资源一键保存</li>
          </ul>
          <button v-if="isLoggedIn" class="lim-btn" :disabled="upgrading" @click="handleUpgrade">
            <Loader2 v-if="upgrading" :size="14" class="lim-spin" /> <span v-else>升级 Pro</span>
          </button>
          <button v-else class="lim-btn lim-btn-disabled" @click="message.info('请先登录后再购买（设置 → 应用账户）')">
            登录后购买
          </button>
        </div>
      </div>

      <button class="lim-skip" @click="emit('update:visible', false)">暂不升级，继续使用免费版</button>
    </div>
  </div>
</template>

<style scoped>
.lim-mask{position:fixed;inset:0;background:rgba(0,0,0,.5);backdrop-filter:blur(3px);display:flex;align-items:center;justify-content:center;z-index:2000}
.lim-modal{position:relative;width:520px;max-width:94vw;max-height:90vh;overflow-y:auto;background:var(--color-bg-2);border:1px solid var(--color-border);border-radius:16px;padding:28px 24px 20px;text-align:center;box-shadow:0 16px 48px rgba(0,0,0,.2)}
.lim-close{position:absolute;top:12px;right:12px;display:flex;align-items:center;justify-content:center;width:28px;height:28px;padding:0;color:var(--color-text-4);background:transparent;border:0;border-radius:6px;cursor:pointer}
.lim-close:hover{background:var(--color-fill-2);color:var(--color-text-1)}
.lim-icon{color:rgb(var(--primary-6));margin-bottom:8px}
.lim-title{font-size:20px;font-weight:700;color:var(--color-text-1);margin:0 0 6px}
.lim-desc{font-size:13px;color:var(--color-text-3);margin:0 0 20px;line-height:1.5}

.lim-pricing{display:grid;grid-template-columns:1fr 1fr;gap:12px;margin-bottom:16px;text-align:left}
.lim-col{position:relative;padding:14px 12px;background:var(--color-fill-1);border:1px solid var(--color-border);border-radius:10px;display:flex;flex-direction:column}
.lim-col-pro{border-color:rgba(245,158,11,.4);background:linear-gradient(180deg,rgba(245,158,11,.08),var(--color-fill-1) 50%)}
.lim-col-badge{position:absolute;top:-9px;left:50%;transform:translateX(-50%);padding:1px 10px;font-size:10px;font-weight:700;color:#fff;background:linear-gradient(135deg,#f59e0b,#eab308);border-radius:99px}
.lim-col-name{font-size:14px;font-weight:700;color:var(--color-text-1);margin-bottom:4px;text-align:center}
.lim-col-price{font-size:17px;font-weight:700;color:rgb(var(--primary-6));margin-bottom:10px;text-align:center}
.lim-features{list-style:none;margin:0 0 8px;padding:0;flex:1;display:flex;flex-direction:column;gap:4px}
.lim-features li{font-size:11px;color:var(--color-text-3);padding-left:14px;position:relative}
.lim-features li::before{content:'✓';position:absolute;left:0;color:rgb(var(--success-6));font-weight:700;font-size:10px}
.lim-btn{width:100%;padding:8px 0;font-size:13px;font-weight:600;color:#fff;background:linear-gradient(135deg,#f59e0b,#eab308);border:0;border-radius:8px;cursor:pointer;font-family:inherit;display:flex;align-items:center;justify-content:center;gap:6px;margin-top:auto}
.lim-btn:hover:not(:disabled){opacity:.9}
.lim-btn:disabled{opacity:.5;cursor:default}
.lim-skip{display:block;width:100%;padding:6px 0;font-size:12px;color:var(--color-text-4);background:transparent;border:0;cursor:pointer;font-family:inherit}
.lim-skip:hover{color:var(--color-text-3)}
.lim-spin{animation:lim-spin 1s linear infinite}
@keyframes lim-spin{to{transform:rotate(360deg)}}
</style>
