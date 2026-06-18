<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { Github, Chrome, Mail, Loader2, LogOut } from 'lucide-vue-next'
import { createClient } from '@supabase/supabase-js'
import Config from '../config'
import message from '../utils/message'
import { openExternal } from '../utils/electronhelper'

const loading = ref(false)
const emailInput = ref('')
const showEmail = ref(false)
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
      options: { redirectTo: CALLBACK_URL, skipBrowserRedirect: true },
    })
    if (error) message.error(error.message)
    else if (data.url) {
      openExternal(data.url)
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

onMounted(() => { setupCallbackListener() })
</script>

<template>
  <div id="SettingAppAccount" class="setting-section">
    <div class="sa-header">
      <span class="sa-title">应用账户</span>
      <span v-if="!isLoggedIn" class="sa-hint">GitHub / Google / 邮箱登录</span>
    </div>

    <template v-if="isLoggedIn">
      <div class="sa-logged">
        <span class="sa-email">{{ userEmail }}</span>
        <button class="sa-logout" @click="handleLogout"><LogOut :size="13" /> 退出</button>
      </div>
    </template>

    <template v-else>
      <div class="sa-oauth">
        <button class="sa-provider sa-gh" :disabled="loading" @click="handleOAuth('github')" title="GitHub"><Github :size="20" /></button>
        <button class="sa-provider sa-go" :disabled="loading" @click="handleOAuth('google')" title="Google"><Chrome :size="20" /></button>
        <button class="sa-provider sa-em" :class="{ active: showEmail }" :disabled="loading" @click="showEmail = !showEmail" title="邮箱"><Mail :size="20" /></button>
      </div>

      <div v-if="showEmail" class="sa-email-box">
        <template v-if="!codeSent">
          <input v-model="emailInput" type="email" placeholder="邮箱地址" />
          <button :disabled="loading || !emailInput.trim()" @click="handleEmailSend"><Loader2 v-if="loading" :size="13" class="spin" /><span v-else>发送验证码</span></button>
        </template>
        <template v-else>
          <input v-model="emailCode" type="text" placeholder="输入验证码" maxlength="6" @keydown.enter="handleEmailVerify" />
          <button :disabled="loading || emailCode.length < 4" @click="handleEmailVerify"><Loader2 v-if="loading" :size="13" class="spin" /><span v-else>验证并登录</span></button>
        </template>
      </div>
    </template>
  </div>
</template>

<style scoped>
.setting-section { padding: 8px 0; }
.sa-header { display: flex; align-items: baseline; gap: 10px; margin-bottom: 10px; }
.sa-title { font-size: 15px; font-weight: 700; color: var(--color-text-1); }
.sa-hint { font-size: 12px; color: var(--color-text-4); }

.sa-logged { display: flex; align-items: center; gap: 12px; padding: 8px 12px; background: var(--color-fill-1); border: 1px solid var(--color-border); border-radius: 8px; }
.sa-email { font-size: 14px; font-weight: 500; color: var(--color-text-1); }
.sa-logout { display: flex; align-items: center; gap: 4px; margin-left: auto; padding: 4px 10px; font-size: 11px; color: var(--color-text-4); background: transparent; border: 1px solid var(--color-border); border-radius: 5px; cursor: pointer; font-family: inherit; }
.sa-logout:hover { color: rgb(var(--danger-6)); border-color: rgb(var(--danger-6)); }

.sa-oauth { display: flex; gap: 12px; margin-bottom: 6px; }
.sa-provider { display: flex; align-items: center; justify-content: center; width: 40px; height: 40px; padding: 0; border: 1px solid var(--color-border); border-radius: 50%; cursor: pointer; font-family: inherit; transition: all .15s; }
.sa-provider:hover:not(:disabled) { transform: translateY(-1px); }
.sa-provider:disabled { opacity: .4; cursor: default; }
.sa-gh { background: #24292e; color: #fff; border-color: #24292e; }
.sa-gh:hover:not(:disabled) { box-shadow: 0 2px 8px rgba(36,41,46,.3); }
.sa-go { background: var(--color-bg-1); color: var(--color-text-3); }
.sa-go:hover:not(:disabled) { color: var(--color-text-1); border-color: var(--color-border-2); }
.sa-em { background: var(--color-bg-1); color: var(--color-text-3); }
.sa-em:hover:not(:disabled), .sa-em.active { color: rgb(var(--primary-6)); border-color: rgb(var(--primary-6)); }

.sa-email-box { display: flex; gap: 8px; margin-top: 6px; padding-top: 8px; border-top: 1px solid var(--color-border); }
.sa-email-box input { flex: 1; padding: 7px 10px; font-size: 12px; color: var(--color-text-1); background: var(--color-fill-1); border: 1px solid var(--color-border); border-radius: 7px; outline: none; font-family: inherit; min-width: 0; }
.sa-email-box input:focus { border-color: rgb(var(--primary-6)); }
.sa-email-box button { display: flex; align-items: center; gap: 3px; padding: 7px 12px; font-size: 12px; font-weight: 500; color: #fff; background: rgb(var(--primary-6)); border: 0; border-radius: 7px; cursor: pointer; font-family: inherit; white-space: nowrap; }
.sa-email-box button:hover:not(:disabled) { opacity: .9; }
.sa-email-box button:disabled { opacity: .4; cursor: default; }
.spin { animation: spin 1s linear infinite; }
@keyframes spin { to { transform: rotate(360deg); } }
</style>
