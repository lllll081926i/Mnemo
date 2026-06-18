<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { X, Loader2, Github, Chrome, Mail } from 'lucide-vue-next'
import message from '../utils/message'

const props = defineProps<{ visible: boolean }>()
const emit = defineEmits<{ 'update:visible': [v: boolean]; login: [user: { email: string; accessToken: string; refreshToken: string }] }>()

const loading = ref(false)
const emailInput = ref('')
const emailSent = ref(false)
const emailCode = ref('')
const oauthCallbackHandler = ref<((_event: any, config: any) => void) | null>(null)

const API_BASE = 'https://koodoreader.com'
const CALLBACK_URL = 'https://web.koodoreader.com/'
const OAUTH_CONFIG: Record<string, { clientId: string; authUrl: string }> = {
  github: {
    clientId: 'ce62c0e39738c6a93a3a',
    authUrl: 'https://github.com/login/oauth/authorize',
  },
  google: {
    clientId: '525890204281-hku7mk7hp109kd8q9ojeqmj7igsc0jv9.apps.googleusercontent.com',
    authUrl: 'https://accounts.google.com/o/oauth2/v2/auth',
  },
}

function buildOAuthUrl(service: 'github' | 'google'): string {
  const config = OAUTH_CONFIG[service]
  if (service === 'github') {
    const state = encodeURIComponent(JSON.stringify({ service: 'github' }))
    return `${config.authUrl}?client_id=${config.clientId}&redirect_uri=${encodeURIComponent(CALLBACK_URL)}&scope=user:email&state=${state}`
  }
  const state = encodeURIComponent(JSON.stringify({ service: 'google' }))
  return `${config.authUrl}?client_id=${config.clientId}&redirect_uri=${encodeURIComponent(CALLBACK_URL)}&response_type=code&scope=${encodeURIComponent('email profile')}&access_type=offline&prompt=consent&state=${state}`
}

async function loginWithProvider(service: string, code: string) {
  const deviceName = navigator.userAgent?.slice(0, 64) || 'Desktop'
  const resp = await fetch(`${API_BASE}/api/user/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      code,
      provider: service,
      redirect_uri: CALLBACK_URL,
      device_name: deviceName,
      device_type: 'Desktop',
      device_os: 'Unknown',
      locale: navigator.language || 'zh-CN',
      os_version: '',
      device_uuid: crypto.randomUUID?.() || Math.random().toString(36),
      app_version: '1.0.0',
    }),
  })
  const data = await resp.json()
  if (data.code === 200) {
    localStorage.setItem('koodo_access_token', data.data.access_token)
    localStorage.setItem('koodo_refresh_token', data.data.refresh_token)
    localStorage.setItem('koodo_is_authed', 'yes')
    emit('login', { email: data.data.email || '', accessToken: data.data.access_token, refreshToken: data.data.refresh_token })
    message.success('登录成功')
    emit('update:visible', false)
  } else {
    message.error(data.message || '登录失败')
  }
}

function handleOAuth(service: 'github' | 'google') {
  const url = buildOAuthUrl(service)
  if (window.WebOpenWindow) {
    window.WebOpenWindow({ url, title: `${service} 登录` })
  } else {
    window.open(url, '_blank')
  }
}

async function handleEmailLogin() {
  const email = emailInput.value.trim()
  if (!email || !email.includes('@')) { message.warning('请输入有效的邮箱地址'); return }
  loading.value = true
  try {
    const resp = await fetch(`${API_BASE}/api/user/email/code`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ email, device: 'Desktop' }),
    })
    const data = await resp.json()
    if (data.code === 200) {
      emailSent.value = true
      message.success('验证码已发送到你的邮箱')
    } else {
      message.error(data.message || '发送验证码失败')
    }
  } catch { message.error('网络请求失败') }
  finally { loading.value = false }
}

async function handleEmailVerify() {
  const email = emailInput.value.trim()
  const code = emailCode.value.trim()
  if (!code) { message.warning('请输入验证码'); return }
  loading.value = true
  try {
    await loginWithProvider('email', code)
  } catch { message.error('登录失败') }
  finally { loading.value = false }
}

function setupOAuthListener() {
  if (!window.Electron?.ipcRenderer) return
  oauthCallbackHandler.value = (_event: any, config: any) => {
    const code = config?.code
    const state = config?.state
    if (!code || !state) return
    try {
      const { service } = JSON.parse(decodeURIComponent(state.split('|')[1] || state))
      if (service) loginWithProvider(service, code)
    } catch {}
  }
  window.Electron.ipcRenderer.on('oauth-callback', oauthCallbackHandler.value)
}

onMounted(setupOAuthListener)
onUnmounted(() => {
  if (window.Electron?.ipcRenderer && oauthCallbackHandler.value) {
    window.Electron.ipcRenderer.removeListener('oauth-callback', oauthCallbackHandler.value)
  }
})
</script>

<template>
  <div v-if="visible" class="al-mask" @click.self="emit('update:visible', false)">
    <div class="al-card">
      <div class="al-header">
        <span>登录账户</span>
        <button class="al-close" @click="emit('update:visible', false)"><X :size="18" :stroke-width="2" /></button>
      </div>

      <div class="al-body">
        <!-- OAuth buttons -->
        <button class="al-btn al-btn-github" :disabled="loading" @click="handleOAuth('github')">
          <Github :size="18" :stroke-width="1.5" />
          <span>GitHub 登录</span>
        </button>
        <button class="al-btn al-btn-google" :disabled="loading" @click="handleOAuth('google')">
          <Chrome :size="18" :stroke-width="1.5" />
          <span>Google 登录</span>
        </button>

        <div class="al-divider"><span>或使用邮箱</span></div>

        <template v-if="!emailSent">
          <div class="al-field">
            <Mail :size="14" :stroke-width="1.5" class="al-field-icon" />
            <input v-model="emailInput" type="email" class="al-input" placeholder="输入邮箱地址" />
          </div>
          <button class="al-btn al-btn-email" :disabled="loading || !emailInput.trim()" @click="handleEmailLogin">
            <Loader2 v-if="loading" :size="16" :stroke-width="2" class="al-spin" />
            <span v-else>发送验证码</span>
          </button>
        </template>
        <template v-else>
          <div class="al-field">
            <input v-model="emailCode" type="text" class="al-input" placeholder="输入6位验证码" maxlength="6" @keydown.enter="handleEmailVerify" />
          </div>
          <button class="al-btn al-btn-email" :disabled="loading || emailCode.length < 4" @click="handleEmailVerify">
            <Loader2 v-if="loading" :size="16" :stroke-width="2" class="al-spin" />
            <span v-else>验证并登录</span>
          </button>
        </template>
      </div>
    </div>
  </div>
</template>

<style scoped>
.al-mask{position:fixed;inset:0;background:rgba(0,0,0,.4);display:flex;align-items:center;justify-content:center;z-index:1100}
.al-card{width:380px;max-width:92vw;background:var(--color-bg-2);border:1px solid var(--color-border);border-radius:14px;overflow:hidden;box-shadow:0 8px 32px rgba(0,0,0,.15)}
.al-header{display:flex;align-items:center;justify-content:space-between;padding:16px 20px;font-size:16px;font-weight:600;color:var(--color-text-1);border-bottom:1px solid var(--color-border)}
.al-close{display:flex;align-items:center;justify-content:center;width:28px;height:28px;padding:0;color:var(--color-text-3);background:transparent;border:0;border-radius:6px;cursor:pointer}
.al-close:hover{background:var(--color-fill-2);color:var(--color-text-1)}
.al-body{padding:20px;display:flex;flex-direction:column;gap:12px}
.al-btn{display:flex;align-items:center;justify-content:center;gap:10px;width:100%;padding:12px;font-size:14px;font-weight:500;border:1px solid var(--color-border);border-radius:10px;cursor:pointer;font-family:inherit;transition:all .15s;color:var(--color-text-1);background:var(--color-bg-1)}
.al-btn:hover:not(:disabled){background:var(--color-fill-1);border-color:var(--color-border-2)}
.al-btn:disabled{opacity:.5;cursor:default}
.al-btn-github{background:#24292e;color:#fff;border-color:#24292e}
.al-btn-github:hover:not(:disabled){background:#1a1f23}
.al-btn-google{background:#fff;color:#444;border-color:#ddd}
.al-btn-google:hover:not(:disabled){background:#f5f5f5}
.al-btn-email{background:rgb(var(--primary-6));color:#fff;border-color:transparent}
.al-btn-email:hover:not(:disabled){opacity:.9}
.al-divider{display:flex;align-items:center;gap:12px;font-size:12px;color:var(--color-text-4)}
.al-divider::before,.al-divider::after{content:'';flex:1;height:1px;background:var(--color-border)}
.al-field{display:flex;align-items:center;gap:10px;padding:10px 14px;background:var(--color-fill-1);border:1px solid var(--color-border);border-radius:10px}
.al-field-icon{color:var(--color-text-4);flex-shrink:0}
.al-input{flex:1;padding:0;font-size:14px;color:var(--color-text-1);background:transparent;border:0;outline:none;font-family:inherit}
.al-input::placeholder{color:var(--color-text-4)}
.al-spin{animation:al-spin 1s linear infinite}
@keyframes al-spin{to{transform:rotate(360deg)}}
</style>
