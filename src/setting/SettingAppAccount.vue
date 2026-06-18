<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { Github, Chrome, Mail, Loader2, LogOut } from 'lucide-vue-next'
import { createClient } from '@supabase/supabase-js'
import Config from '../config'
import message from '../utils/message'
import { copyToClipboard } from '../utils/electronhelper'

const loading = ref(false)
const emailInput = ref('')
const codeSent = ref(false)
const emailCode = ref('')
const userEmail = ref(localStorage.getItem('app_user_email') || '')
const isLoggedIn = ref(localStorage.getItem('app_user_authed') === '1')

const CALLBACK_URL = 'boxplayer-auth://callback'

const supabase = Config.SUPABASE_URL && Config.SUPABASE_ANON_KEY
  ? createClient(Config.SUPABASE_URL, Config.SUPABASE_ANON_KEY)
  : null

function saveLogin(email: string) {
  localStorage.setItem('app_user_email', email)
  localStorage.setItem('app_user_authed', '1')
  userEmail.value = email
  isLoggedIn.value = true
}

function handleLogout() {
  localStorage.removeItem('app_user_email')
  localStorage.removeItem('app_user_authed')
  supabase?.auth.signOut().catch(() => {})
  userEmail.value = ''
  isLoggedIn.value = false
  message.success('已退出登录')
}

async function handleOAuth(provider: 'github' | 'google') {
  if (!supabase) { message.error('未配置 Supabase，请在 config.ts 填入 SUPABASE_URL 和 SUPABASE_ANON_KEY'); return }
  loading.value = true
  try {
    const { data, error } = await supabase.auth.signInWithOAuth({
      provider,
      options: { redirectTo: CALLBACK_URL, skipBrowserRedirect: false },
    })
    if (error) message.error(error.message)
    else if (data.url) {
      if (window.WebOpenWindow) window.WebOpenWindow({ url: data.url, title: `${provider} 登录` })
      else window.open(data.url, '_blank')
    }
  } finally { loading.value = false }
}

async function handleEmailSend() {
  const email = emailInput.value.trim()
  if (!email?.includes('@')) { message.warning('请输入有效邮箱'); return }
  if (!supabase) { message.error('未配置 Supabase'); return }
  loading.value = true
  try {
    const { error } = await supabase.auth.signInWithOtp({ email, options: { shouldCreateUser: true } })
    if (error) message.error(error.message)
    else { codeSent.value = true; message.success('验证码已发送') }
  } finally { loading.value = false }
}

async function handleEmailVerify() {
  const email = emailInput.value.trim()
  const token = emailCode.value.trim()
  if (!token) { message.warning('请输入验证码'); return }
  if (!supabase) return
  loading.value = true
  try {
    const { data, error } = await supabase.auth.verifyOtp({ email, token, type: 'email' })
    if (error) message.error(error.message)
    else if (data.user) { saveLogin(data.user.email || email); message.success('登录成功') }
  } finally { loading.value = false }
}

function setupCallbackListener() {
  if (!window.Electron?.ipcRenderer) return
  const handler = async (_e: any, params: { access_token?: string; refresh_token?: string }) => {
    if (!params.access_token || !supabase) return
    const { data, error } = await supabase.auth.setSession({
      access_token: params.access_token,
      refresh_token: params.refresh_token || '',
    })
    if (!error && data.user) { saveLogin(data.user.email || ''); message.success('登录成功') }
  }
  window.Electron.ipcRenderer.on('auth-callback', handler)
  onUnmounted(() => window.Electron.ipcRenderer?.removeListener('auth-callback', handler))
}

onMounted(() => { setupCallbackListener(); copyToClipboard('') })
</script>

<template>
  <div id="SettingAppAccount" class="setting-section">
    <h2>应用账户</h2>
    <p class="desc">登录以使用全部功能</p>

    <template v-if="isLoggedIn">
      <div class="account-logged-in">
        <div class="account-email">{{ userEmail }}</div>
        <div class="account-status">已登录</div>
        <button class="btn-logout" @click="handleLogout"><LogOut :size="14" :stroke-width="1.5" /> 退出登录</button>
      </div>
    </template>

    <template v-else>
      <div class="account-login-btns">
        <button class="btn-oauth btn-github" :disabled="loading" @click="handleOAuth('github')">
          <Github :size="16" :stroke-width="1.5" /><span>GitHub 登录</span>
        </button>
        <button class="btn-oauth btn-google" :disabled="loading" @click="handleOAuth('google')">
          <Chrome :size="16" :stroke-width="1.5" /><span>Google 登录</span>
        </button>
      </div>

      <div class="divider"><span>或邮箱验证码</span></div>

      <template v-if="!codeSent">
        <div class="email-row">
          <input v-model="emailInput" type="email" class="input" placeholder="输入邮箱" />
          <button class="btn-send" :disabled="loading || !emailInput.trim()" @click="handleEmailSend">
            <Loader2 v-if="loading" :size="14" class="spin" /> <span v-else>发送验证码</span>
          </button>
        </div>
      </template>
      <template v-else>
        <div class="email-row">
          <input v-model="emailCode" type="text" class="input" placeholder="输入验证码" maxlength="6" @keydown.enter="handleEmailVerify" />
          <button class="btn-send" :disabled="loading || emailCode.length < 4" @click="handleEmailVerify">
            <Loader2 v-if="loading" :size="14" class="spin" /> <span v-else>验证并登录</span>
          </button>
        </div>
      </template>
    </template>
  </div>
</template>

<style scoped>
.setting-section { padding: 16px 0; }
.setting-section h2 { font-size: 16px; font-weight: 700; color: var(--color-text-1); margin: 0 0 4px; }
.desc { font-size: 13px; color: var(--color-text-4); margin: 0 0 16px; }

.account-logged-in { display: flex; flex-direction: column; align-items: flex-start; gap: 8px; padding: 16px; background: var(--color-fill-1); border: 1px solid var(--color-border); border-radius: 10px; }
.account-email { font-size: 15px; font-weight: 600; color: var(--color-text-1); }
.account-status { font-size: 12px; color: rgb(var(--success-6)); }
.btn-logout { display: flex; align-items: center; gap: 6px; padding: 6px 14px; font-size: 12px; color: var(--color-text-3); background: var(--color-fill-2); border: 1px solid var(--color-border); border-radius: 6px; cursor: pointer; font-family: inherit; }
.btn-logout:hover { color: rgb(var(--danger-6)); background: rgba(var(--danger-6), .06); border-color: rgb(var(--danger-6)); }

.account-login-btns { display: flex; gap: 10px; margin-bottom: 16px; }
.btn-oauth { display: flex; align-items: center; gap: 8px; flex: 1; padding: 10px 14px; font-size: 13px; font-weight: 500; border: 1px solid var(--color-border); border-radius: 8px; cursor: pointer; font-family: inherit; transition: all .15s; }
.btn-oauth:disabled { opacity: .5; cursor: default; }
.btn-github { background: #24292e; color: #fff; border-color: #24292e; }
.btn-github:hover:not(:disabled) { opacity: .9; }
.btn-google { background: var(--color-bg-1); color: var(--color-text-1); }
.btn-google:hover:not(:disabled) { background: var(--color-fill-1); }

.divider { display: flex; align-items: center; gap: 10px; font-size: 12px; color: var(--color-text-4); margin-bottom: 12px; }
.divider::before, .divider::after { content: ''; flex: 1; height: 1px; background: var(--color-border); }

.email-row { display: flex; gap: 8px; }
.input { flex: 1; padding: 8px 12px; font-size: 13px; color: var(--color-text-1); background: var(--color-fill-1); border: 1px solid var(--color-border); border-radius: 8px; outline: none; font-family: inherit; }
.input:focus { border-color: rgb(var(--primary-6)); }
.btn-send { display: flex; align-items: center; gap: 4px; padding: 8px 16px; font-size: 13px; font-weight: 500; color: #fff; background: rgb(var(--primary-6)); border: 0; border-radius: 8px; cursor: pointer; font-family: inherit; white-space: nowrap; }
.btn-send:hover:not(:disabled) { opacity: .9; }
.btn-send:disabled { opacity: .4; cursor: default; }
.spin { animation: spin 1s linear infinite; }
@keyframes spin { to { transform: rotate(360deg); } }
</style>
